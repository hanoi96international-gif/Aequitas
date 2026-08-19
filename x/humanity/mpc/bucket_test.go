package mpc

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"
)

// TestSecureComparisonCostIsNotWhatLimitsScale corrects a measurement that
// was wrong in a way worth recording.
//
// A first benchmark timed ONE call and reported ~90 ms, which extrapolated to
// 23 years per registration at population scale. That number was an artefact:
// the first call also builds the threshold interpolation polynomial (cached
// afterwards) and generated its own multiplication triples, which belong to
// the offline phase and not to anything a human waits for. Measured warm and
// with the offline work excluded, one comparison costs a few hundred
// microseconds.
//
// The conclusion survived the correction — a linear scan is still impossible —
// but it was impossible by a factor of hundreds, not tens of thousands. The
// lesson is kept here because a benchmark that measures setup instead of work
// sends an architecture in the wrong direction, confidently.
func TestSecureComparisonCostIsNotWhatLimitsScale(t *testing.T) {
	const bits, parties = 512, 3
	a, err := NewSharedTemplate(make([]uint8, bits), parties)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSharedTemplate(make([]uint8, bits), parties)
	if err != nil {
		t.Fatal(err)
	}
	mkStores := func() []*TripleStore {
		tr, err := GenerateTriples(TriplesPerComparison(bits), parties)
		if err != nil {
			t.Fatal(err)
		}
		s := make([]*TripleStore, parties)
		for i := range s {
			s[i] = NewTripleStore(tr[i])
		}
		return s
	}

	// Warm: the polynomial is built once per deployment, not per comparison.
	if _, err := SecureMatch(a, b, 100, mkStores()); err != nil {
		t.Fatal(err)
	}

	// Enough iterations that the total clearly exceeds the platform's clock
	// granularity — on Windows a handful of 200 µs runs measures as zero.
	const runs = 200
	stores := make([][]*TripleStore, runs)
	for i := range stores {
		stores[i] = mkStores()
	}
	start := time.Now()
	for i := 0; i < runs; i++ {
		if _, err := SecureMatch(a, b, 100, stores[i]); err != nil {
			t.Fatal(err)
		}
	}
	perComparison := time.Since(start) / runs

	linearDays := perComparison.Seconds() * 8e9 / 86400
	t.Logf("warm online cost per comparison: %v", perComparison)
	t.Logf("linear scan at 8e9 enrolments: %.0f days per registration", linearDays)

	if perComparison <= 0 {
		t.Skip("clock granularity too coarse to time this on the current platform")
	}
	if linearDays < 1 {
		t.Errorf("a linear scan at population scale now costs %.2f days per registration. "+
			"Verify before concluding that bucketing is unnecessary.", linearDays)
	}
}

// TestBucketingCollapsesTheComparisonCount is the counterpart: the same
// population, but only the plausible candidates are compared.
func TestBucketingCollapsesTheComparisonCount(t *testing.T) {
	cfg := BucketConfig{PrefixBits: 20}
	const population = 8_000_000_000

	linear := float64(population)
	bucketed := cfg.EstimateComparisons(population, 2)

	t.Logf("linear:   %.0f comparisons per registration", linear)
	t.Logf("bucketed: %.0f comparisons per registration (20-bit prefix, 2 probes)", bucketed)
	t.Logf("reduction factor: %.0fx", linear/bucketed)

	if bucketed > 100_000 {
		t.Errorf("bucketed cost is %.0f comparisons — still far too many to be practical; "+
			"the prefix needs to be wider or the probe count lower", bucketed)
	}
}

// TestSameSketchLandsInSameBucket is the property everything rests on.
func TestSameSketchLandsInSameBucket(t *testing.T) {
	cfg := BucketConfig{PrefixBits: 12}
	sketch := make([]uint8, 64)
	for i := range sketch {
		sketch[i] = uint8(i % 2)
	}
	first, err := cfg.KeyFor(sketch)
	if err != nil {
		t.Fatal(err)
	}
	again, err := cfg.KeyFor(sketch)
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Fatal("the same sketch produced two different bucket keys — enrolments would scatter")
	}
}

// TestNearbySketchesShareABucketOrANeighbour is the claim that makes bucketing
// a similarity index rather than a hash table: a capture of the same person,
// with a little noise, must be found — in the same bucket or in one of the
// probed neighbours.
func TestNearbySketchesShareABucketOrANeighbour(t *testing.T) {
	const dim, bits = 128, 256
	cfg := BucketConfig{PrefixBits: 10}
	const probes = 10 // every single-bit neighbour of a 10-bit prefix

	p, err := NewProjections("bucket-test", bits, dim)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(5))

	found, total := 0, 40
	for i := 0; i < total; i++ {
		base := unitVector(rng, dim)
		other := unitVector(rng, dim)
		// A small angle: the same person, a different capture.
		recapture := rotateToward(base, other, 0.15)

		baseSketch, err := p.Sketch(base)
		if err != nil {
			t.Fatal(err)
		}
		reSketch, err := p.Sketch(recapture)
		if err != nil {
			t.Fatal(err)
		}

		baseKey, err := cfg.KeyFor(baseSketch)
		if err != nil {
			t.Fatal(err)
		}
		reKey, err := cfg.KeyFor(reSketch)
		if err != nil {
			t.Fatal(err)
		}

		for _, probe := range cfg.MultiProbe(reKey, probes) {
			if probe == baseKey {
				found++
				break
			}
		}
	}

	rate := float64(found) / float64(total)
	t.Logf("recaptures found within bucket+neighbours: %d/%d (%.0f%%)", found, total, rate*100)
	if rate < 0.5 {
		t.Errorf("only %.0f%% of same-person recaptures land in a searched bucket. Bucketing "+
			"would hide most duplicates: they are never compared, so they are never found. "+
			"Widen the probe count or narrow the prefix.", rate*100)
	}
}

// TestDistantSketchesRarelyCollide checks the other half: if unrelated people
// landed in the same bucket routinely, bucketing would save nothing.
func TestDistantSketchesRarelyCollide(t *testing.T) {
	const dim, bits = 128, 256
	cfg := BucketConfig{PrefixBits: 12}
	p, err := NewProjections("collision-test", bits, dim)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(9))

	collisions, total := 0, 200
	reference := unitVector(rng, dim)
	refSketch, _ := p.Sketch(reference)
	refKey, _ := cfg.KeyFor(refSketch)

	for i := 0; i < total; i++ {
		stranger := unitVector(rng, dim)
		s, _ := p.Sketch(stranger)
		k, _ := cfg.KeyFor(s)
		if k == refKey {
			collisions++
		}
	}

	// With a 12-bit prefix the chance of an unrelated collision is about
	// 1/4096; a handful in 200 draws would already be surprising.
	if collisions > total/10 {
		t.Errorf("%d of %d strangers collided with the reference bucket — the prefix is not "+
			"spreading the population, so buckets will grow until the scan is linear again",
			collisions, total)
	}
	t.Logf("stranger collisions: %d/%d", collisions, total)
}

// TestBucketedRegistryReturnsOnlyPlausibleCandidates exercises the registry.
func TestBucketedRegistryReturnsOnlyPlausibleCandidates(t *testing.T) {
	cfg := BucketConfig{PrefixBits: 8}
	reg := NewBucketedRegistry(cfg, 1)

	mk := func(id string, key BucketKey) BucketedEnrollment {
		tmpl, err := NewSharedTemplate([]uint8{1, 0, 1, 0}, 3)
		if err != nil {
			t.Fatal(err)
		}
		return BucketedEnrollment{Enrollment: Enrollment{ID: id, Template: tmpl}, Key: key}
	}

	if err := reg.Add(mk("same-bucket", 0b00000101)); err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(mk("one-bit-away", 0b00000100)); err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(mk("far-away", 0b11110000)); err != nil {
		t.Fatal(err)
	}

	got := reg.Candidates(0b00000101)
	ids := map[string]bool{}
	for _, e := range got {
		ids[e.ID] = true
	}
	if !ids["same-bucket"] {
		t.Error("the exact bucket was not searched")
	}
	if !ids["one-bit-away"] {
		t.Error("the probed neighbour was not searched — probes are not being applied")
	}
	if ids["far-away"] {
		t.Error("an unrelated bucket was searched; that is the cost bucketing exists to avoid")
	}
	if reg.Total() != 3 {
		t.Errorf("Total() = %d, want 3", reg.Total())
	}
}

func TestBucketConfigRejectsUnusableSettings(t *testing.T) {
	if err := (BucketConfig{PrefixBits: 0}).Validate(512); err == nil {
		t.Error("zero prefix bits must be refused")
	}
	if err := (BucketConfig{PrefixBits: 40}).Validate(512); err == nil {
		t.Error("a prefix wider than the key type must be refused")
	}
	if err := (BucketConfig{PrefixBits: 512}).Validate(512); err == nil {
		t.Error("a prefix as wide as the sketch must be refused — every capture would be alone")
	}
}

// TestBucketSizesSurfaceSkew pins the operational signal: if one bucket grows
// without bound the scan silently becomes linear again.
func TestBucketSizesSurfaceSkew(t *testing.T) {
	cfg := BucketConfig{PrefixBits: 8}
	reg := NewBucketedRegistry(cfg, 0)
	tmpl, _ := NewSharedTemplate([]uint8{1, 1}, 3)
	for i := 0; i < 25; i++ {
		_ = reg.Add(BucketedEnrollment{
			Enrollment: Enrollment{ID: fmt.Sprintf("crowded-%d", i), Template: tmpl},
			Key:        7,
		})
	}
	_ = reg.Add(BucketedEnrollment{Enrollment: Enrollment{ID: "lonely", Template: tmpl}, Key: 200})

	sizes := reg.BucketSizes()
	if len(sizes) == 0 || sizes[0] != 25 {
		t.Fatalf("largest bucket = %v, want 25 first", sizes)
	}
	if math.Abs(float64(sizes[len(sizes)-1]-1)) > 0 {
		t.Errorf("smallest bucket = %d, want 1", sizes[len(sizes)-1])
	}
}
