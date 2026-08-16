package mpc

import "fmt"

// Triple is ONE party's shares of a Beaver multiplication triple: values a, b
// and c with c = a*b, each additively shared across the parties.
//
// Addition of shares is free, multiplication is not: the product of two secrets
// cannot be obtained from local products of shares. A Beaver triple is the
// standard way out — it lets the parties trade one pre-computed random product
// for one real one, revealing only two blinded values that are uniformly
// random and therefore carry no information about the secrets.
type Triple struct {
	A, B, C Element
}

// TripleStore hands out triples for one party. Each triple must be used at
// most ONCE: the blinded openings d = x-a and e = y-b are only uniformly random
// because a and b are fresh. Reusing a triple across two multiplications lets
// an observer subtract the two openings and recover the difference of the
// secrets — so exhaustion is an error, never a wrap-around.
type TripleStore struct {
	triples []Triple
	used    int
}

// NewTripleStore wraps a party's pre-generated triples.
func NewTripleStore(t []Triple) *TripleStore { return &TripleStore{triples: t} }

// Next consumes one triple.
func (s *TripleStore) Next() (Triple, error) {
	if s.used >= len(s.triples) {
		return Triple{}, fmt.Errorf("mpc: multiplication triples exhausted after %d uses — "+
			"refusing to reuse one, because a reused triple stops blinding and leaks the "+
			"difference of the two secrets it was used on; generate more in the offline phase",
			s.used)
	}
	t := s.triples[s.used]
	s.used++
	return t, nil
}

// Remaining reports how many triples are left, so a caller can size its
// offline phase against the work it is about to do rather than discovering the
// shortfall halfway through a match.
func (s *TripleStore) Remaining() int { return len(s.triples) - s.used }

// GenerateTriples produces count triples shared across n parties.
//
// TRUST NOTE, stated rather than buried: this is the "trusted dealer" offline
// phase. Whoever runs it knows a, b and c in the clear. That party can, if it
// also sees the openings exchanged during a later multiplication, recover the
// inputs of that multiplication. It therefore MUST NOT be one of the computing
// parties, and it must discard the plaintext triples after distribution.
//
// The stronger constructions (oblivious transfer, or homomorphic encryption as
// in the SPDZ family) remove the dealer entirely and are the correct end state
// for production. They are a much larger build; this is the honest interim,
// and it is already a categorical improvement over the status quo, where a
// single server saw every template in the clear.
func GenerateTriples(count, n int) ([][]Triple, error) {
	if n < 2 {
		return nil, fmt.Errorf("mpc: %d parties cannot hide anything", n)
	}
	if count <= 0 {
		return nil, fmt.Errorf("mpc: triple count must be positive, got %d", count)
	}
	out := make([][]Triple, n)
	for i := range out {
		out[i] = make([]Triple, count)
	}
	for k := 0; k < count; k++ {
		a, err := randomElement()
		if err != nil {
			return nil, err
		}
		b, err := randomElement()
		if err != nil {
			return nil, err
		}
		c := Mul(a, b)

		as, err := Split(a, n)
		if err != nil {
			return nil, err
		}
		bs, err := Split(b, n)
		if err != nil {
			return nil, err
		}
		cs, err := Split(c, n)
		if err != nil {
			return nil, err
		}
		for i := 0; i < n; i++ {
			out[i][k] = Triple{A: as[i], B: bs[i], C: cs[i]}
		}
	}
	return out, nil
}

// MulShares multiplies two shared secrets.
//
// x and y are the parties' shares of the two secrets; stores[i] is party i's
// triple supply. The returned Shares are the parties' shares of x*y.
//
// The protocol, per party i:
//
//	d_i = x_i - a_i        e_i = y_i - b_i        (local)
//	d   = Σ d_i            e   = Σ e_i            (opened — one round)
//	z_i = c_i + d*b_i + e*a_i  (+ d*e for exactly one party)
//
// d and e are the secrets blinded by fresh uniform randomness, so opening them
// reveals nothing. The d*e term is public and must be added by exactly ONE
// party, or the sum would count it n times.
//
// This function performs the opening internally, which makes it a simulation of
// the round rather than a distributed implementation: it necessarily sees every
// party's shares. Keeping the round explicit here is deliberate — it is the
// single place a networked version has to change, and it documents exactly what
// crosses the wire (two field elements per multiplication, nothing else).
func MulShares(x, y Shares, stores []*TripleStore) (Shares, error) {
	n := len(x)
	if len(y) != n || len(stores) != n {
		return nil, fmt.Errorf("mpc: party count mismatch (x=%d y=%d stores=%d)", len(x), len(y), len(stores))
	}

	triples := make([]Triple, n)
	dShares := make(Shares, n)
	eShares := make(Shares, n)
	for i := 0; i < n; i++ {
		t, err := stores[i].Next()
		if err != nil {
			return nil, err
		}
		triples[i] = t
		dShares[i] = Sub(x[i], t.A)
		eShares[i] = Sub(y[i], t.B)
	}

	// The one communication round: every party broadcasts d_i and e_i.
	d := Reconstruct(dShares)
	e := Reconstruct(eShares)

	out := make(Shares, n)
	for i := 0; i < n; i++ {
		z := Add(triples[i].C, Mul(d, triples[i].B))
		z = Add(z, Mul(e, triples[i].A))
		if i == 0 {
			// Public term, added once by a single designated party.
			z = Add(z, Mul(d, e))
		}
		out[i] = z
	}
	return out, nil
}
