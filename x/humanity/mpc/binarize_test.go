package mpc

import (
	"math"
	"math/rand"
	"testing"
)

// unitVector returns a random unit vector of the given dimension.
func unitVector(rng *rand.Rand, dim int) []float64 {
	v := make([]float64, dim)
	var norm float64
	for i := range v {
		v[i] = rng.NormFloat64()
		norm += v[i] * v[i]
	}
	norm = math.Sqrt(norm)
	for i := range v {
		v[i] /= norm
	}
	return v
}

// rotateToward returns a unit vector at the given angle from base, by mixing
// in an orthogonal direction. This is how a "same person, slightly different
// capture" is simulated with a known ground truth.
func rotateToward(base, other []float64, theta float64) []float64 {
	// Gram-Schmidt: strip the base component out of other, normalise.
	var dot float64
	for i := range base {
		dot += base[i] * other[i]
	}
	perp := make([]float64, len(base))
	var norm float64
	for i := range base {
		perp[i] = other[i] - dot*base[i]
		norm += perp[i] * perp[i]
	}
	norm = math.Sqrt(norm)
	out := make([]float64, len(base))
	for i := range base {
		out[i] = math.Cos(theta)*base[i] + math.Sin(theta)*(perp[i]/norm)
	}
	return out
}

func hamming(a, b []uint8) int {
	d := 0
	for i := range a {
		if a[i] != b[i] {
			d++
		}
	}
	return d
}

// TestSketchIsDeterministic is the property everything else rests on: an
// enrolment stored today must still compare against a capture taken next year.
// If sketching were non-deterministic, every returning human would look like a
// stranger and the whole registry would silently stop working.
func TestSketchIsDeterministic(t *testing.T) {
	p, err := NewProjections("test-seed", 256, 64)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(1))
	v := unitVector(rng, 64)

	first, err := p.Sketch(v)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		again, err := p.Sketch(v)
		if err != nil {
			t.Fatal(err)
		}
		if hamming(first, again) != 0 {
			t.Fatal("the same embedding produced two different sketches — enrolments would decay over time")
		}
	}

	// A second Projections built from the same seed must agree, or a restart
	// would invalidate the registry.
	q, err := NewProjections("test-seed", 256, 64)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := q.Sketch(v)
	if err != nil {
		t.Fatal(err)
	}
	if hamming(first, rebuilt) != 0 {
		t.Error("projections rebuilt from the same seed produced a different sketch — " +
			"a process restart would make every stored enrolment incomparable")
	}
}

func TestDifferentSeedsGiveDifferentSketchSpaces(t *testing.T) {
	a, _ := NewProjections("seed-a", 256, 64)
	b, _ := NewProjections("seed-b", 256, 64)
	v := unitVector(rand.New(rand.NewSource(2)), 64)

	sa, _ := a.Sketch(v)
	sb, _ := b.Sketch(v)
	if hamming(sa, sb) == 0 {
		t.Error("two different seeds produced identical sketches — the seed is supposed to define " +
			"a distinct sketch space, and sharing one across deployments would let sketches be " +
			"compared that never should be")
	}
}

// TestHammingTracksAngle is the claim that makes this bridge legitimate: for
// unit vectors the probability of a differing bit is theta/pi, so a Hamming
// threshold really is a statement about the angle between two faces.
//
// It is checked across the whole range rather than at one point, because a
// bridge that is only right near zero would pass a naive test and still
// mis-rank everything in the region where decisions are actually made.
func TestHammingTracksAngle(t *testing.T) {
	const dim, bits = 128, 4096
	p, err := NewProjections("angle-test", bits, dim)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(7))
	base := unitVector(rng, dim)
	other := unitVector(rng, dim)

	for _, theta := range []float64{0.1, 0.3, math.Pi / 6, math.Pi / 4, math.Pi / 3, math.Pi / 2} {
		rotated := rotateToward(base, other, theta)

		sa, err := p.Sketch(base)
		if err != nil {
			t.Fatal(err)
		}
		sb, err := p.Sketch(rotated)
		if err != nil {
			t.Fatal(err)
		}

		got := float64(hamming(sa, sb)) / float64(bits)
		want := theta / math.Pi
		// Sampling error is sqrt(p(1-p)/bits); allow a generous multiple so
		// this does not flake, while still failing on a real bias.
		tolerance := 5 * math.Sqrt(want*(1-want)/float64(bits))
		if tolerance < 0.02 {
			tolerance = 0.02
		}
		if math.Abs(got-want) > tolerance {
			t.Errorf("theta=%.3f: bit-difference rate %.4f, expected about %.4f (tolerance %.4f) — "+
				"the sketch no longer estimates the angle, so a Hamming threshold would not mean "+
				"what it was calibrated to mean", theta, got, want, tolerance)
		}
	}
}

// TestEstimateCosineInvertsTheSketch checks the conversion callers use to
// think in the units the matching service already uses.
func TestEstimateCosineInvertsTheSketch(t *testing.T) {
	const dim, bits = 128, 4096
	p, _ := NewProjections("cosine-test", bits, dim)
	rng := rand.New(rand.NewSource(11))
	base := unitVector(rng, dim)
	other := unitVector(rng, dim)

	for _, theta := range []float64{0.2, math.Pi / 4, math.Pi / 3} {
		rotated := rotateToward(base, other, theta)
		sa, _ := p.Sketch(base)
		sb, _ := p.Sketch(rotated)

		est, err := EstimateCosine(hamming(sa, sb), bits)
		if err != nil {
			t.Fatal(err)
		}
		trueCos := math.Cos(theta)
		if math.Abs(est-trueCos) > 0.06 {
			t.Errorf("theta=%.3f: estimated cosine %.4f, true %.4f", theta, est, trueCos)
		}
	}
}

// TestThresholdForCosineRoundTrips pins the direction of the rounding, which
// is a safety property rather than a numeric detail: rounding must never make
// the matcher flag MORE people than the calibrated boundary says.
func TestThresholdForCosineRoundTrips(t *testing.T) {
	const bits = 512
	for _, cos := range []float64{0.9, 0.7, 0.5, 0.3, 0.0} {
		h, err := ThresholdForCosine(cos, bits)
		if err != nil {
			t.Fatal(err)
		}
		back, err := EstimateCosine(h, bits)
		if err != nil {
			t.Fatal(err)
		}
		if back < cos-1e-9 {
			t.Errorf("cosine %.2f -> hamming %d -> cosine %.4f: rounding went the permissive way; "+
				"it must round toward flagging fewer people, because a wrongly flagged human is the "+
				"error this project treats as the costly one", cos, h, back)
		}
	}
}

func TestSketchRejectsWrongDimension(t *testing.T) {
	p, _ := NewProjections("dim-test", 64, 128)
	if _, err := p.Sketch(make([]float64, 64)); err == nil {
		t.Error("an embedding of the wrong dimension must be refused, not silently sketched")
	}
}

func TestSketchRejectsNonFiniteValues(t *testing.T) {
	p, _ := NewProjections("nan-test", 64, 8)
	v := make([]float64, 8)
	v[3] = math.NaN()
	if _, err := p.Sketch(v); err == nil {
		t.Error("a NaN component must be refused: a broken capture should not become an arbitrary code")
	}
	v[3] = math.Inf(1)
	if _, err := p.Sketch(v); err == nil {
		t.Error("an infinite component must be refused")
	}
}

func TestNewProjectionsRejectsBadParameters(t *testing.T) {
	if _, err := NewProjections("", 64, 8); err == nil {
		t.Error("an empty seed must be refused — the seed is the identity of the sketch space")
	}
	if _, err := NewProjections("s", 0, 8); err == nil {
		t.Error("zero bits must be refused")
	}
	if _, err := NewProjections("s", 63, 8); err == nil {
		t.Error("a bit count that is not a multiple of 8 must be refused")
	}
	if _, err := NewProjections("s", 64, 0); err == nil {
		t.Error("zero dimensions must be refused")
	}
}

// TestEndToEndSameAndDifferentPerson is the whole chain in one test: two
// ArcFace-shaped embeddings go in, are sketched, secret-shared, compared
// through the MPC protocol, and the single bit that comes out is the right
// one — with no party ever holding an embedding.
func TestEndToEndSameAndDifferentPerson(t *testing.T) {
	const dim, bits, parties = EmbeddingDim, 256, 3
	p, err := NewProjections("end-to-end", bits, dim)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(23))

	enrolled := unitVector(rng, dim)
	elsewhere := unitVector(rng, dim)
	// A re-capture of the same person: a small angle. A stranger: a large one.
	recapture := rotateToward(enrolled, elsewhere, 0.25)
	stranger := rotateToward(enrolled, elsewhere, 1.3)

	// Decide in cosine, the unit the matching service and any calibration
	// speak, and convert once.
	threshold, err := ThresholdForCosine(0.80, bits)
	if err != nil {
		t.Fatal(err)
	}

	sketchOf := func(v []float64) *SharedTemplate {
		t.Helper()
		code, err := p.Sketch(v)
		if err != nil {
			t.Fatal(err)
		}
		st, err := NewSharedTemplate(code, parties)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}

	base := sketchOf(enrolled)

	for _, tc := range []struct {
		name string
		v    []float64
		want bool
	}{
		{"same person, different capture", recapture, true},
		{"a stranger", stranger, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stores := newStores(t, TriplesPerComparison(bits)+8, parties)
			res, err := SecureMatch(base, sketchOf(tc.v), threshold, stores)
			if err != nil {
				t.Fatalf("SecureMatch: %v", err)
			}
			if res.Similar != tc.want {
				t.Errorf("Similar = %v, want %v (threshold %d bits of %d)",
					res.Similar, tc.want, threshold, bits)
			}
		})
	}
}
