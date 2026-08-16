package mpc

import (
	"errors"
	"fmt"
)

// Shares holds one additive sharing of a single secret: Shares[i] belongs to
// party i, and their sum modulo Prime is the secret.
//
// Any strict subset sums to a uniformly random value, so a coalition of up to
// n-1 parties learns nothing at all — not "learns something computationally
// hard to invert", but nothing. That property is what lets a validator set run
// this on real biometric templates without any validator being able to
// identify anyone.
type Shares []Element

// Split splits secret into n additive shares.
//
// n-1 shares are drawn uniformly at random and the last is chosen to make the
// sum come out right. n must be at least 2; a "sharing" among one party is
// just the plaintext, and silently accepting it would be a trap for a caller
// that miscounted its parties.
func Split(secret Element, n int) (Shares, error) {
	if n < 2 {
		return nil, fmt.Errorf("mpc: refusing to split a secret among %d parties — "+
			"a single share IS the secret; at least 2 parties are required for it to be hidden", n)
	}
	out := make(Shares, n)
	acc := Element(0)
	for i := 0; i < n-1; i++ {
		r, err := randomElement()
		if err != nil {
			return nil, err
		}
		out[i] = r
		acc = Add(acc, r)
	}
	out[n-1] = Sub(secret, acc)
	return out, nil
}

// Reconstruct sums shares back into the secret.
func Reconstruct(s Shares) Element {
	acc := Element(0)
	for _, v := range s {
		acc = Add(acc, v)
	}
	return acc
}

// Vector is one party's shares of a whole template: Vector[j] is this party's
// share of feature j.
type Vector []Element

// SplitVector splits a full template into one Vector per party.
//
// The returned slice is indexed by party: out[i][j] is party i's share of
// feature j. Every feature is shared independently — reusing randomness across
// features would let a party correlate them and recover structure.
func SplitVector(template []Element, n int) ([]Vector, error) {
	if len(template) == 0 {
		return nil, errors.New("mpc: empty template")
	}
	out := make([]Vector, n)
	for i := range out {
		out[i] = make(Vector, len(template))
	}
	for j, v := range template {
		sh, err := Split(v, n)
		if err != nil {
			return nil, err
		}
		for i := range sh {
			out[i][j] = sh[i]
		}
	}
	return out, nil
}

// ReconstructVector is the inverse of SplitVector. Present for tests and for
// recovery tooling; the production matching path never calls it, and that is
// the whole point — see the package comment.
func ReconstructVector(parties []Vector) (Vector, error) {
	if len(parties) == 0 {
		return nil, errors.New("mpc: no parties")
	}
	width := len(parties[0])
	for i, p := range parties {
		if len(p) != width {
			return nil, fmt.Errorf("mpc: party %d holds %d features, party 0 holds %d — "+
				"mismatched shares cannot be combined", i, len(p), width)
		}
	}
	out := make(Vector, width)
	for j := 0; j < width; j++ {
		acc := Element(0)
		for _, p := range parties {
			acc = Add(acc, p[j])
		}
		out[j] = acc
	}
	return out, nil
}

// AddVec returns the element-wise sum of two share vectors held by the SAME
// party. Addition is free in additive sharing: no communication, no triples.
func AddVec(a, b Vector) (Vector, error) {
	if len(a) != len(b) {
		return nil, fmt.Errorf("mpc: vector length mismatch %d vs %d", len(a), len(b))
	}
	out := make(Vector, len(a))
	for i := range a {
		out[i] = Add(a[i], b[i])
	}
	return out, nil
}

// SubVec returns the element-wise difference, also free.
func SubVec(a, b Vector) (Vector, error) {
	if len(a) != len(b) {
		return nil, fmt.Errorf("mpc: vector length mismatch %d vs %d", len(a), len(b))
	}
	out := make(Vector, len(a))
	for i := range a {
		out[i] = Sub(a[i], b[i])
	}
	return out, nil
}

// ScaleVec multiplies every element by a PUBLIC constant. Also free — only
// multiplication of two SECRETS needs a triple (see beaver.go).
func ScaleVec(a Vector, k Element) Vector {
	out := make(Vector, len(a))
	for i := range a {
		out[i] = Mul(a[i], k)
	}
	return out
}
