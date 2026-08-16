package mpc

import (
	"fmt"
	"sync"
)

// SharedTemplate is one biometric template as held by the whole party set:
// parties[i][j] is party i's share of feature j. No party's row is meaningful
// on its own.
type SharedTemplate struct {
	parties []Vector
}

// NewSharedTemplate splits a plaintext binary template across n parties.
//
// The template must be a binary code (each feature 0 or 1) — that is what the
// Hamming distance below is defined on, and it is the representation iris and
// similar codes use. Values outside {0,1} are rejected rather than silently
// producing a distance that means nothing.
func NewSharedTemplate(template []uint8, n int) (*SharedTemplate, error) {
	if len(template) == 0 {
		return nil, fmt.Errorf("mpc: empty template")
	}
	field := make([]Element, len(template))
	for i, b := range template {
		if b > 1 {
			return nil, fmt.Errorf("mpc: feature %d is %d — templates must be binary codes; "+
				"the Hamming distance this package computes is only meaningful on {0,1}", i, b)
		}
		field[i] = Element(b)
	}
	parties, err := SplitVector(field, n)
	if err != nil {
		return nil, err
	}
	return &SharedTemplate{parties: parties}, nil
}

// FromShares rebuilds the handle from per-party rows, e.g. after loading each
// validator's stored share of an existing registration.
func FromShares(parties []Vector) (*SharedTemplate, error) {
	if len(parties) < 2 {
		return nil, fmt.Errorf("mpc: %d parties cannot hide a template", len(parties))
	}
	width := len(parties[0])
	for i, p := range parties {
		if len(p) != width {
			return nil, fmt.Errorf("mpc: party %d holds %d features, party 0 holds %d", i, len(p), width)
		}
	}
	return &SharedTemplate{parties: parties}, nil
}

// Party returns party i's row, which is what that validator stores.
func (t *SharedTemplate) Party(i int) Vector { return t.parties[i] }

// Len is the number of features.
func (t *SharedTemplate) Len() int { return len(t.parties[0]) }

// NumParties is the size of the party set.
func (t *SharedTemplate) NumParties() int { return len(t.parties) }

// feature returns every party's share of feature j.
func (t *SharedTemplate) feature(j int) Shares {
	out := make(Shares, len(t.parties))
	for i := range t.parties {
		out[i] = t.parties[i][j]
	}
	return out
}

// TriplesPerComparison reports how many multiplication triples one
// SecureMatch call consumes for a template of the given length. Callers should
// size their offline phase with this rather than guessing.
//
// One triple per feature for the distance (each XOR is one multiplication),
// plus `length` more for the threshold comparison's Horner evaluation.
func TriplesPerComparison(length int) int { return 2 * length }

// SecureHammingDistance computes the shared Hamming distance between two
// shared templates without revealing either.
//
// For bits a and b, a XOR b = a + b - 2ab. Addition is free; the single
// product per feature is what costs a triple. The result is the distance,
// still shared — nobody has learned it.
func SecureHammingDistance(a, b *SharedTemplate, stores []*TripleStore) (Shares, error) {
	if a.Len() != b.Len() {
		return nil, fmt.Errorf("mpc: template length mismatch %d vs %d", a.Len(), b.Len())
	}
	if a.NumParties() != b.NumParties() {
		return nil, fmt.Errorf("mpc: party count mismatch %d vs %d", a.NumParties(), b.NumParties())
	}
	n := a.NumParties()

	acc := make(Shares, n)
	two := Element(2)
	for j := 0; j < a.Len(); j++ {
		aj := a.feature(j)
		bj := b.feature(j)

		prod, err := MulShares(aj, bj, stores)
		if err != nil {
			return nil, fmt.Errorf("mpc: feature %d: %w", j, err)
		}
		for i := 0; i < n; i++ {
			// a + b - 2ab, computed locally on each party's shares.
			v := Add(aj[i], bj[i])
			v = Sub(v, Mul(two, prod[i]))
			acc[i] = Add(acc[i], v)
		}
	}
	return acc, nil
}

// thresholdPolynomial builds the coefficients of the unique polynomial f of
// degree <= maxDistance with
//
//	f(d) = 1  for d < threshold
//	f(d) = 0  for threshold <= d <= maxDistance
//
// evaluated by Lagrange interpolation over the points 0..maxDistance.
//
// This is how the comparison stays private. Opening the distance itself would
// leak far more than the answer: an adversary who can register repeatedly and
// read distances back can triangulate a stored template. Evaluating a
// polynomial keeps everything inside the sharing, so the ONLY value ever
// revealed is the single bit "similar / not similar".
//
// Cost is linear in maxDistance, which is fine for the template sizes here and
// is precomputed once per (threshold, maxDistance) pair.
func thresholdPolynomial(threshold, maxDistance int) ([]Element, error) {
	if threshold < 0 || threshold > maxDistance+1 {
		return nil, fmt.Errorf("mpc: threshold %d outside [0, %d]", threshold, maxDistance+1)
	}

	// Interpolation is O(maxDistance^2) and depends only on the pair
	// (threshold, maxDistance), which is fixed for a deployment — computing it
	// per comparison would dominate the cost of the whole protocol.
	key := [2]int{threshold, maxDistance}
	polyCacheMu.Lock()
	if cached, ok := polyCache[key]; ok {
		polyCacheMu.Unlock()
		return cached, nil
	}
	polyCacheMu.Unlock()

	points := maxDistance + 1
	coeffs := make([]Element, points)

	for k := 0; k < points; k++ {
		yk := Element(0)
		if k < threshold {
			yk = 1
		}
		if yk == 0 {
			continue
		}
		// Lagrange basis L_k(x) = Π_{m != k} (x - m) / (k - m), accumulated
		// into the coefficient vector.
		basis := make([]Element, points)
		basis[0] = 1
		degree := 0
		denom := Element(1)
		for m := 0; m < points; m++ {
			if m == k {
				continue
			}
			// Multiply the basis polynomial by (x - m):
			//   new[i] = old[i-1] + (-m)*old[i]
			// Iterating downward keeps old[i-1] untouched until it is used.
			negM := Neg(Element(uint64(m)))
			for i := degree + 1; i >= 1; i-- {
				basis[i] = Add(basis[i-1], Mul(basis[i], negM))
			}
			basis[0] = Mul(basis[0], negM)
			degree++
			denom = Mul(denom, Sub(Element(uint64(k)), Element(uint64(m))))
		}
		invDenom, err := Inv(denom)
		if err != nil {
			return nil, fmt.Errorf("mpc: degenerate interpolation at k=%d: %w", k, err)
		}
		for i := 0; i < points; i++ {
			coeffs[i] = Add(coeffs[i], Mul(Mul(basis[i], invDenom), yk))
		}
	}

	polyCacheMu.Lock()
	polyCache[key] = coeffs
	polyCacheMu.Unlock()
	return coeffs, nil
}

var (
	polyCacheMu sync.Mutex
	polyCache   = map[[2]int][]Element{}
)

// SecureThresholdCompare evaluates the threshold polynomial on a shared
// distance, returning shares of 1 (distance < threshold, i.e. a probable same
// person) or 0.
//
// Horner's rule keeps this at one multiplication per degree. Every
// multiplication is between two secrets, so each consumes a triple.
func SecureThresholdCompare(distance Shares, threshold, maxDistance int, stores []*TripleStore) (Shares, error) {
	coeffs, err := thresholdPolynomial(threshold, maxDistance)
	if err != nil {
		return nil, err
	}
	n := len(distance)

	// Horner: start at the highest coefficient (public), fold in the secret
	// distance one degree at a time.
	acc := make(Shares, n)
	acc[0] = coeffs[len(coeffs)-1] // a public constant enters via one party

	for idx := len(coeffs) - 2; idx >= 0; idx-- {
		acc, err = MulShares(acc, distance, stores)
		if err != nil {
			return nil, fmt.Errorf("mpc: horner step %d: %w", idx, err)
		}
		acc[0] = Add(acc[0], coeffs[idx])
	}
	return acc, nil
}

// MatchResult is the only thing the protocol ever reveals about a comparison.
type MatchResult struct {
	// Similar is true when the two templates are within the threshold — i.e.
	// probably the same person.
	//
	// DELIBERATELY NOT A VERDICT. Per this project's own rule, a match is a
	// reason to review a registration, never grounds for permanent rejection:
	// a false match must always be recoverable by the human it concerns.
	Similar bool
}

// SecureMatch runs the full comparison: distance, then threshold, then the
// single-bit opening.
//
// The distance itself is never opened. maxDistance is the template length,
// since Hamming distance cannot exceed it.
func SecureMatch(a, b *SharedTemplate, threshold int, stores []*TripleStore) (MatchResult, error) {
	if a.Len() != b.Len() {
		return MatchResult{}, fmt.Errorf("mpc: template length mismatch %d vs %d", a.Len(), b.Len())
	}
	dist, err := SecureHammingDistance(a, b, stores)
	if err != nil {
		return MatchResult{}, err
	}
	bit, err := SecureThresholdCompare(dist, threshold, a.Len(), stores)
	if err != nil {
		return MatchResult{}, err
	}
	// The one and only opening in the whole protocol.
	v := Reconstruct(bit)
	if v != 0 && v != 1 {
		return MatchResult{}, fmt.Errorf("mpc: comparison produced %d, which is neither 0 nor 1 — "+
			"the shares are inconsistent (mismatched triples, a reused triple, or a party that "+
			"deviated from the protocol); refusing to interpret it as a decision about a person", v)
	}
	return MatchResult{Similar: v == 1}, nil
}
