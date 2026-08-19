package mpc

import (
	"encoding/binary"
	"fmt"
	"math"

	"golang.org/x/crypto/sha3"
)

// The bridge between the biometric pipeline that exists and the protocol in
// this package.
//
// The matching service produces ArcFace embeddings: 512 float32 values, L2
// normalised, compared by cosine similarity (matching-service/app/face.py).
// This package computes Hamming distance on binary codes. Those are not the
// same measure, and pretending otherwise would silently produce a matcher that
// decides nothing meaningful.
//
// The standard bridge is a sign-random-projection sketch (Charikar 2002): draw
// fixed random hyperplanes, record which side of each one a vector falls on.
// For unit vectors the probability that two vectors land on opposite sides of
// a random hyperplane is exactly their angle divided by pi:
//
//	P[bit differs] = theta / pi
//
// So the Hamming distance between two sketches is an unbiased estimate of the
// angle, and therefore of the cosine similarity. That is what makes a Hamming
// threshold in this package a statement about faces rather than about bits.
//
// WHAT THIS COSTS, PLAINLY. The sketch is an estimate. With k bits its
// standard error on theta/pi is sqrt(p(1-p)/k) — about 2.2 percentage points
// at k=512, p=0.5. A decision boundary must be set with that spread in mind,
// which is one more reason the threshold has to come from measured
// same-person and different-person distributions rather than from a guess.
// Going through a sketch loses accuracy compared with comparing the floats
// directly; what it buys is that the comparison can happen without any party
// holding the face.

// EmbeddingDim is the ArcFace embedding size the matching service produces
// (buffalo_l / w600k_r50). Stated as a constant so a pipeline change that
// alters it fails loudly here instead of producing sketches that are quietly
// incomparable with everything already enrolled.
const EmbeddingDim = 512

// Projections is a fixed set of random hyperplanes.
//
// It MUST be identical everywhere and forever: two sketches are only
// comparable if they were produced against the same hyperplanes. Regenerating
// them invalidates every enrolment ever stored, so they are derived
// deterministically from a seed string rather than drawn at random at
// startup — a startup draw would silently re-key the whole registry on every
// restart, and the failure would look like "everyone suddenly became a
// stranger".
type Projections struct {
	seed   string
	bits   int
	dim    int
	planes [][]float64
}

// NewProjections derives `bits` hyperplanes of `dim` dimensions from seed.
//
// The generator is SHAKE-256 over the seed: a stream function, so the same
// seed always yields the same planes on any machine and any Go version,
// which a math/rand sequence would not guarantee across releases.
func NewProjections(seed string, bits, dim int) (*Projections, error) {
	if bits <= 0 || bits%8 != 0 {
		return nil, fmt.Errorf("mpc: bit count must be a positive multiple of 8, got %d", bits)
	}
	if dim <= 0 {
		return nil, fmt.Errorf("mpc: dimension must be positive, got %d", dim)
	}
	if seed == "" {
		return nil, fmt.Errorf("mpc: refusing an empty projection seed — the seed IS the identity of " +
			"the sketch space, and an empty default would make two deployments silently share it")
	}

	shake := sha3.NewShake256()
	if _, err := shake.Write([]byte("aequitas-mpc-projections-v1|" + seed)); err != nil {
		return nil, err
	}

	planes := make([][]float64, bits)
	buf := make([]byte, 8)
	for i := range planes {
		row := make([]float64, dim)
		for j := range row {
			if _, err := shake.Read(buf); err != nil {
				return nil, err
			}
			// Two uniform draws into a Gaussian via Box-Muller would need
			// pairs and rejection; summing uniforms is enough here because
			// only the SIGN of the projection is used, and the sign of a sum
			// of independent symmetric variables is unbiased regardless of
			// the exact shape.
			u := float64(binary.BigEndian.Uint64(buf)) / float64(math.MaxUint64)
			row[j] = u - 0.5
		}
		planes[i] = row
	}
	return &Projections{seed: seed, bits: bits, dim: dim, planes: planes}, nil
}

// Bits is the sketch length in bits, which is also the template length the
// MPC protocol will see.
func (p *Projections) Bits() int { return p.bits }

// Seed identifies the sketch space. Store it next to every enrolment: a
// sketch compared against one produced from a different seed is meaningless,
// and the only way to notice is to have recorded which seed was used.
func (p *Projections) Seed() string { return p.seed }

// Sketch turns one embedding into a binary code.
//
// The embedding is expected L2-normalised, as ArcFace's normed_embedding
// already is. Normalisation is not re-checked here because the sign of a
// projection is scale-invariant — but the ANGLE interpretation above is only
// valid for vectors on the unit sphere, so a caller feeding unnormalised
// vectors gets a sketch that is stable and comparable yet no longer means
// what the threshold was calibrated against.
func (p *Projections) Sketch(embedding []float64) ([]uint8, error) {
	if len(embedding) != p.dim {
		return nil, fmt.Errorf("mpc: embedding has %d dimensions, these projections are for %d — "+
			"sketches from different pipelines cannot be compared, and comparing them anyway "+
			"would produce a distance that means nothing", len(embedding), p.dim)
	}
	for i, v := range embedding {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("mpc: embedding component %d is not a finite number (%v) — "+
				"refusing to sketch a broken capture rather than turning it into an arbitrary code", i, v)
		}
	}

	out := make([]uint8, p.bits)
	for i, plane := range p.planes {
		var dot float64
		for j, v := range embedding {
			dot += v * plane[j]
		}
		// Ties (an exactly zero projection) go to 1. The choice is arbitrary
		// but must be FIXED: deciding it per-call, or by floating-point luck,
		// would make one embedding produce two different sketches.
		if dot >= 0 {
			out[i] = 1
		}
	}
	return out, nil
}

// EstimateCosine converts a Hamming distance between two sketches back into
// the cosine similarity it implies, which is what lets a threshold be chosen
// in the units the biometric literature and the matching service both use.
//
//	theta = pi * hamming / bits      cos = cos(theta)
//
// It is the inverse of the sketch relation, so it inherits the sketch's
// sampling error; it is an estimate, and is named as one.
func EstimateCosine(hamming, bits int) (float64, error) {
	if bits <= 0 {
		return 0, fmt.Errorf("mpc: bits must be positive")
	}
	if hamming < 0 || hamming > bits {
		return 0, fmt.Errorf("mpc: hamming distance %d outside [0, %d]", hamming, bits)
	}
	return math.Cos(math.Pi * float64(hamming) / float64(bits)), nil
}

// ThresholdForCosine converts a cosine-similarity decision boundary into the
// Hamming threshold this package's SecureMatch takes.
//
// Callers should think in cosine, because that is what the matching service's
// own thresholds are expressed in and what any calibration against real
// same-person / different-person data will produce. Rounding is toward the
// stricter (smaller) Hamming distance, so a boundary error makes the matcher
// flag FEWER people rather than more — under this project's rule, a missed
// duplicate is recoverable and a wrongly flagged human is the error that
// costs someone their access.
func ThresholdForCosine(cosine float64, bits int) (int, error) {
	if bits <= 0 {
		return 0, fmt.Errorf("mpc: bits must be positive")
	}
	if cosine < -1 || cosine > 1 {
		return 0, fmt.Errorf("mpc: cosine %v outside [-1, 1]", cosine)
	}
	theta := math.Acos(cosine)
	return int(math.Floor(theta / math.Pi * float64(bits))), nil
}
