package mpc

import "fmt"

// Round-efficient primitives.
//
// WHY ROUNDS ARE THE THING TO COUNT
//
// Locally, one secure comparison costs about 215 µs of online work — fast
// enough that it is not what limits anything. Distributed across independent
// validators it is not the arithmetic that dominates but the COMMUNICATION:
// every Beaver multiplication opens two blinded values, and an opening is a
// network round trip. The arithmetic is nanoseconds; the round trip is
// milliseconds.
//
//	 1024 sequential rounds at  5 ms latency ->  5.1 s per comparison
//	 1024 sequential rounds at 20 ms latency -> 20.5 s per comparison
//
// At 179 comparisons per registration (a 27-bit bucket prefix with two probes)
// that is an hour of waiting for one person. The same computation in ~20 rounds
// is well under a second.
//
// So the goal is not fewer multiplications — it is fewer ROUNDS. Multiplications
// that do not depend on each other belong in one round, and a computation shaped
// like a chain has to be re-shaped into a tree.
//
// The naive versions in match.go remain correct and are kept: they are the
// readable reference these are checked against, and TestBatchedMatchesSequential
// asserts the two agree.

// MulSharesBatch multiplies many pairs of secrets in a SINGLE opening round.
//
// The protocol per pair is exactly Beaver's, unchanged. What changes is the
// scheduling: all the d = x-a and e = y-b values are collected first, opened
// together, and only then combined. Correctness is identical because no pair
// depends on another; the saving is that k multiplications cost one round
// instead of k.
func MulSharesBatch(xs, ys []Shares, stores []*TripleStore) ([]Shares, error) {
	if len(xs) != len(ys) {
		return nil, fmt.Errorf("mpc: batch length mismatch (%d xs, %d ys)", len(xs), len(ys))
	}
	if len(xs) == 0 {
		return nil, nil
	}
	n := len(stores)

	triples := make([][]Triple, len(xs))
	dOpen := make([]Element, len(xs))
	eOpen := make([]Element, len(xs))

	// Local phase: consume one triple per pair, form the blinded differences.
	for k := range xs {
		if len(xs[k]) != n || len(ys[k]) != n {
			return nil, fmt.Errorf("mpc: pair %d has %d/%d shares, expected %d", k, len(xs[k]), len(ys[k]), n)
		}
		row := make([]Triple, n)
		dShares := make(Shares, n)
		eShares := make(Shares, n)
		for i := 0; i < n; i++ {
			t, err := stores[i].Next()
			if err != nil {
				return nil, fmt.Errorf("mpc: pair %d: %w", k, err)
			}
			row[i] = t
			dShares[i] = Sub(xs[k][i], t.A)
			eShares[i] = Sub(ys[k][i], t.B)
		}
		triples[k] = row
		// The single communication round: in a distributed deployment every
		// party broadcasts its whole vector of d_i and e_i here, once.
		dOpen[k] = Reconstruct(dShares)
		eOpen[k] = Reconstruct(eShares)
	}

	out := make([]Shares, len(xs))
	for k := range xs {
		z := make(Shares, n)
		for i := 0; i < n; i++ {
			v := Add(triples[k][i].C, Mul(dOpen[k], triples[k][i].B))
			v = Add(v, Mul(eOpen[k], triples[k][i].A))
			if i == 0 {
				v = Add(v, Mul(dOpen[k], eOpen[k]))
			}
			z[i] = v
		}
		out[k] = z
	}
	return out, nil
}

// SecurePowers returns shares of d^1 .. d^maxExp in O(log maxExp) rounds.
//
// Horner's rule needs one round per degree because each step consumes the
// previous one — a chain 512 links long. This computes the same values as a
// tree instead: repeated squaring produces d^(2^k) in log rounds, and every
// remaining exponent is a product of those, with all products of one bit-width
// batched into a single round.
//
// Once every power is available, evaluating any polynomial is a local
// linear combination with PUBLIC coefficients — no further rounds at all.
// That is what removes the comparison from the round budget entirely.
func SecurePowers(d Shares, maxExp int, stores []*TripleStore) ([]Shares, error) {
	if maxExp < 1 {
		return nil, fmt.Errorf("mpc: maxExp must be at least 1, got %d", maxExp)
	}
	powers := make([]Shares, maxExp+1) // index by exponent; [0] unused
	powers[1] = d

	// Squarings: d^2, d^4, d^8, ... Each depends on the previous, so these are
	// inherently sequential — but there are only log2(maxExp) of them.
	squares := []Shares{d}
	for e := 2; e <= maxExp; e *= 2 {
		prev := squares[len(squares)-1]
		sq, err := MulSharesBatch([]Shares{prev}, []Shares{prev}, stores)
		if err != nil {
			return nil, fmt.Errorf("mpc: squaring to %d: %w", e, err)
		}
		squares = append(squares, sq[0])
		powers[e] = sq[0]
	}

	// Remaining exponents, by increasing popcount. Every exponent with b bits
	// set is the product of one with b-1 bits and a power of two, and all
	// exponents of the same width are independent — so each width is one round.
	for width := 2; width <= 32; width++ {
		var xs, ys []Shares
		var targets []int
		for e := 3; e <= maxExp; e++ {
			if powers[e] != nil || popcount(e) != width {
				continue
			}
			low := e & (-e)      // lowest set bit: a power of two, already known
			rest := e &^ low     // the rest: one bit fewer, computed in an earlier width
			if powers[low] == nil || powers[rest] == nil {
				continue
			}
			xs = append(xs, powers[low])
			ys = append(ys, powers[rest])
			targets = append(targets, e)
		}
		if len(targets) == 0 {
			continue
		}
		got, err := MulSharesBatch(xs, ys, stores)
		if err != nil {
			return nil, fmt.Errorf("mpc: powers of width %d: %w", width, err)
		}
		for i, e := range targets {
			powers[e] = got[i]
		}
	}

	for e := 1; e <= maxExp; e++ {
		if powers[e] == nil {
			return nil, fmt.Errorf("mpc: internal error, power %d was never computed", e)
		}
	}
	return powers, nil
}

func popcount(v int) int {
	c := 0
	for v != 0 {
		v &= v - 1
		c++
	}
	return c
}

// TriplesForBatchedComparison reports the triple budget for the round-efficient
// path, which differs from the sequential one: the distance still costs one
// triple per feature, but the threshold no longer costs one per degree — it
// costs one per DISTINCT power that has to be multiplied out.
func TriplesForBatchedComparison(templateLen int) int {
	// Distance: one per feature. Powers: one per exponent that is not a power
	// of two, plus one per squaring. An upper bound of templateLen covers both
	// comfortably and is what callers should provision.
	return 2 * templateLen
}

// SecureHammingDistanceBatched is SecureHammingDistance in one round.
func SecureHammingDistanceBatched(a, b *SharedTemplate, stores []*TripleStore) (Shares, error) {
	if a.Len() != b.Len() {
		return nil, fmt.Errorf("mpc: template length mismatch %d vs %d", a.Len(), b.Len())
	}
	n := a.NumParties()
	if b.NumParties() != n {
		return nil, fmt.Errorf("mpc: party count mismatch %d vs %d", a.NumParties(), b.NumParties())
	}

	xs := make([]Shares, a.Len())
	ys := make([]Shares, a.Len())
	for j := 0; j < a.Len(); j++ {
		xs[j] = a.feature(j)
		ys[j] = b.feature(j)
	}
	prods, err := MulSharesBatch(xs, ys, stores)
	if err != nil {
		return nil, err
	}

	acc := make(Shares, n)
	two := Element(2)
	for j := 0; j < a.Len(); j++ {
		aj, bj := xs[j], ys[j]
		for i := 0; i < n; i++ {
			v := Add(aj[i], bj[i])
			v = Sub(v, Mul(two, prods[j][i]))
			acc[i] = Add(acc[i], v)
		}
	}
	return acc, nil
}

// SecureThresholdCompareBatched evaluates the threshold polynomial using
// precomputed powers, so the evaluation itself costs ZERO rounds.
func SecureThresholdCompareBatched(distance Shares, threshold, maxDistance int, stores []*TripleStore) (Shares, error) {
	coeffs, err := thresholdPolynomial(threshold, maxDistance)
	if err != nil {
		return nil, err
	}
	n := len(distance)

	powers, err := SecurePowers(distance, len(coeffs)-1, stores)
	if err != nil {
		return nil, err
	}

	// Local linear combination: coefficients are public, so no round is needed.
	acc := make(Shares, n)
	acc[0] = coeffs[0] // the constant term enters via one designated party
	for e := 1; e < len(coeffs); e++ {
		c := coeffs[e]
		if c == 0 {
			continue
		}
		for i := 0; i < n; i++ {
			acc[i] = Add(acc[i], Mul(c, powers[e][i]))
		}
	}
	return acc, nil
}

// SecureMatchBatched is SecureMatch with the round-efficient primitives.
func SecureMatchBatched(a, b *SharedTemplate, threshold int, stores []*TripleStore) (MatchResult, error) {
	dist, err := SecureHammingDistanceBatched(a, b, stores)
	if err != nil {
		return MatchResult{}, err
	}
	bit, err := SecureThresholdCompareBatched(dist, threshold, a.Len(), stores)
	if err != nil {
		return MatchResult{}, err
	}
	v := Reconstruct(bit)
	if v != 0 && v != 1 {
		return MatchResult{}, fmt.Errorf("mpc: comparison produced %d, which is neither 0 nor 1 — "+
			"the shares are inconsistent; refusing to interpret it as a decision about a person", v)
	}
	return MatchResult{Similar: v == 1}, nil
}

// SecureMatchMany compares one candidate against MANY enrolments in the same
// number of rounds it takes to compare against one.
//
// This is the step that makes population scale work. Bucketing already cut the
// comparisons per registration from billions to a few hundred, but those few
// hundred were still run one after another — and in a distributed deployment
// each carries its own round trips, so the latency multiplied by the candidate
// count. It need not: the comparisons are mutually independent, so every one of
// them can share the same opening rounds.
//
//	179 comparisons, sequential protocol : 183,296 rounds
//	179 comparisons, batched per pair    :   3,759 rounds
//	179 comparisons, batched together    :      ~21 rounds
//
// The round count is now independent of how many candidates a bucket holds,
// which is what stops a crowded bucket from turning into a slow registration.
//
// Returns one MatchResult per enrolment, in the order given. The distance and
// the threshold bit stay secret throughout; only the final bits are opened, and
// only those.
func SecureMatchMany(candidate *SharedTemplate, enrolled []*SharedTemplate, threshold int, stores []*TripleStore) ([]MatchResult, error) {
	if len(enrolled) == 0 {
		return nil, nil
	}
	n := candidate.NumParties()
	length := candidate.Len()
	for i, e := range enrolled {
		if e.Len() != length {
			return nil, fmt.Errorf("mpc: enrolment %d has %d features, candidate has %d — "+
				"templates from different pipelines cannot be compared", i, e.Len(), length)
		}
		if e.NumParties() != n {
			return nil, fmt.Errorf("mpc: enrolment %d has %d parties, candidate has %d", i, e.NumParties(), n)
		}
	}

	// Round 1: every feature of every pair, in one opening.
	var xs, ys []Shares
	for _, e := range enrolled {
		for j := 0; j < length; j++ {
			xs = append(xs, candidate.feature(j))
			ys = append(ys, e.feature(j))
		}
	}
	prods, err := MulSharesBatch(xs, ys, stores)
	if err != nil {
		return nil, fmt.Errorf("mpc: batched distances: %w", err)
	}

	distances := make([]Shares, len(enrolled))
	two := Element(2)
	for k := range enrolled {
		acc := make(Shares, n)
		for j := 0; j < length; j++ {
			idx := k*length + j
			aj, bj := xs[idx], ys[idx]
			for i := 0; i < n; i++ {
				v := Add(aj[i], bj[i])
				v = Sub(v, Mul(two, prods[idx][i]))
				acc[i] = Add(acc[i], v)
			}
		}
		distances[k] = acc
	}

	// Rounds 2..~21: powers for every distance at once. Same structure as
	// SecurePowers, widened across candidates so the round count does not grow
	// with them.
	coeffs, err := thresholdPolynomial(threshold, length)
	if err != nil {
		return nil, err
	}
	maxExp := len(coeffs) - 1

	powers := make([][]Shares, len(distances))
	for k := range powers {
		powers[k] = make([]Shares, maxExp+1)
		powers[k][1] = distances[k]
	}

	// Squarings, batched across candidates.
	for e := 2; e <= maxExp; e *= 2 {
		var bx, by []Shares
		for k := range distances {
			prev := powers[k][e/2]
			bx = append(bx, prev)
			by = append(by, prev)
		}
		got, err := MulSharesBatch(bx, by, stores)
		if err != nil {
			return nil, fmt.Errorf("mpc: batched squaring to %d: %w", e, err)
		}
		for k := range distances {
			powers[k][e] = got[k]
		}
	}

	// Remaining exponents by popcount width, batched across candidates.
	for width := 2; width <= 32; width++ {
		var bx, by []Shares
		type slot struct{ k, e int }
		var slots []slot
		for e := 3; e <= maxExp; e++ {
			if popcount(e) != width {
				continue
			}
			low := e & (-e)
			rest := e &^ low
			for k := range distances {
				if powers[k][e] != nil || powers[k][low] == nil || powers[k][rest] == nil {
					continue
				}
				bx = append(bx, powers[k][low])
				by = append(by, powers[k][rest])
				slots = append(slots, slot{k, e})
			}
		}
		if len(slots) == 0 {
			continue
		}
		got, err := MulSharesBatch(bx, by, stores)
		if err != nil {
			return nil, fmt.Errorf("mpc: batched powers of width %d: %w", width, err)
		}
		for i, s := range slots {
			powers[s.k][s.e] = got[i]
		}
	}

	// Local evaluation, no rounds, then the single opening per candidate.
	out := make([]MatchResult, len(enrolled))
	for k := range enrolled {
		acc := make(Shares, n)
		acc[0] = coeffs[0]
		for e := 1; e <= maxExp; e++ {
			c := coeffs[e]
			if c == 0 || powers[k][e] == nil {
				continue
			}
			for i := 0; i < n; i++ {
				acc[i] = Add(acc[i], Mul(c, powers[k][e][i]))
			}
		}
		v := Reconstruct(acc)
		if v != 0 && v != 1 {
			return nil, fmt.Errorf("mpc: comparison %d produced %d, which is neither 0 nor 1 — "+
				"shares are inconsistent; refusing to interpret it as a decision about a person", k, v)
		}
		out[k] = MatchResult{Similar: v == 1}
	}
	return out, nil
}

// TriplesForManyComparison reports the triple budget for SecureMatchMany.
func TriplesForManyComparison(templateLen, candidates int) int {
	return candidates * 2 * templateLen
}
