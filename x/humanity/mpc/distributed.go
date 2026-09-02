package mpc

import (
	"context"
	"fmt"
)

// The distributed comparison: what each party actually runs.
//
// Everything above this file computes with every party's shares in one place,
// which is fine for testing a protocol and useless as a deployment — a process
// holding all the shares can reconstruct every template. Here each party holds
// exactly its own row and never sees another's, and the only thing that leaves
// a machine is the blinded openings described in transport.go.
//
// The shape of the computation is identical to SecureMatchMany, and
// deliberately so: the same batching, the same round count, the same decision.
// TestDistributedAgreesWithLocal asserts they agree, because two
// implementations of a rule that decides who counts as a person is exactly the
// kind of divergence that must not be allowed to appear.

// PartyTemplate is one party's row of a shared template.
//
// This is what a validator stores: a vector of field elements that is
// uniformly random on its own and reveals nothing about the biometric it came
// from.
type PartyTemplate []Element

// DistributedMatcher runs comparisons for ONE party.
type DistributedMatcher struct {
	Session   *Session
	Threshold int
}

// CompareMany compares one candidate against many enrolments.
//
// Every party calls this with its own rows, in the same order, and gets back
// the same public decisions. The distance is never opened, the threshold bit is
// opened once per candidate, and nothing else is revealed.
func (m *DistributedMatcher) CompareMany(ctx context.Context, candidate PartyTemplate, enrolled []PartyTemplate) ([]MatchResult, error) {
	if len(enrolled) == 0 {
		return nil, nil
	}
	length := len(candidate)
	for i, e := range enrolled {
		if len(e) != length {
			return nil, fmt.Errorf("mpc: enrolment %d has %d features, candidate has %d — "+
				"templates from different pipelines cannot be compared", i, len(e), length)
		}
	}

	// Round 1: every feature of every pair at once.
	xs := make([]Element, 0, len(enrolled)*length)
	ys := make([]Element, 0, len(enrolled)*length)
	for _, e := range enrolled {
		for j := 0; j < length; j++ {
			xs = append(xs, candidate[j])
			ys = append(ys, e[j])
		}
	}
	prods, err := m.Session.MulBatch(ctx, xs, ys)
	if err != nil {
		return nil, fmt.Errorf("mpc: distances: %w", err)
	}

	two := Element(2)
	distances := make([]Element, len(enrolled))
	for k := range enrolled {
		var acc Element
		for j := 0; j < length; j++ {
			idx := k*length + j
			v := Add(xs[idx], ys[idx])
			v = Sub(v, Mul(two, prods[idx]))
			acc = Add(acc, v)
		}
		distances[k] = acc
	}

	coeffs, err := thresholdPolynomial(m.Threshold, length)
	if err != nil {
		return nil, err
	}
	maxExp := len(coeffs) - 1

	// Powers of every distance, as a tree, batched across candidates so the
	// round count does not grow with them.
	//
	// Every exponent up to the degree is computed, including ones a sparse
	// polynomial would skip. That looks wasteful and is not: the interpolated
	// threshold polynomial was measured at 513 of 513 coefficients nonzero for
	// a 512-bit sketch, and the same at every threshold tried. There is no
	// sparsity here to exploit, so a needed-set optimisation would add a
	// transitive-closure pass and save nothing.
	powers := make([][]Element, len(distances))
	haveExp := make([][]bool, len(distances))
	for k := range powers {
		powers[k] = make([]Element, maxExp+1)
		haveExp[k] = make([]bool, maxExp+1)
		powers[k][1] = distances[k]
		haveExp[k][1] = true
	}

	for e := 2; e <= maxExp; e *= 2 {
		bx := make([]Element, len(distances))
		by := make([]Element, len(distances))
		for k := range distances {
			bx[k] = powers[k][e/2]
			by[k] = powers[k][e/2]
		}
		got, err := m.Session.MulBatch(ctx, bx, by)
		if err != nil {
			return nil, fmt.Errorf("mpc: squaring to %d: %w", e, err)
		}
		for k := range distances {
			powers[k][e] = got[k]
			haveExp[k][e] = true
		}
	}

	for width := 2; width <= 32; width++ {
		var bx, by []Element
		type slot struct{ k, e int }
		var slots []slot
		for e := 3; e <= maxExp; e++ {
			if popcount(e) != width {
				continue
			}
			low := e & (-e)
			rest := e &^ low
			for k := range distances {
				if haveExp[k][e] || !haveExp[k][low] || !haveExp[k][rest] {
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
		got, err := m.Session.MulBatch(ctx, bx, by)
		if err != nil {
			return nil, fmt.Errorf("mpc: powers of width %d: %w", width, err)
		}
		for i, s := range slots {
			powers[s.k][s.e] = got[i]
			haveExp[s.k][s.e] = true
		}
	}

	// Local evaluation with public coefficients: no round.
	verdicts := make([]Element, len(enrolled))
	first := m.Session.Transport.Index() == 0
	for k := range enrolled {
		var acc Element
		if first {
			acc = coeffs[0]
		}
		for e := 1; e <= maxExp; e++ {
			if coeffs[e] == 0 || !haveExp[k][e] {
				continue
			}
			acc = Add(acc, Mul(coeffs[e], powers[k][e]))
		}
		verdicts[k] = acc
	}

	// The one and only opening of a decision, batched into a single round.
	m.Session.round++
	opened, err := OpenVia(ctx, m.Session.Transport, m.Session.round, verdicts)
	if err != nil {
		return nil, fmt.Errorf("mpc: opening verdicts: %w", err)
	}

	out := make([]MatchResult, len(enrolled))
	for k, v := range opened {
		if v != 0 && v != 1 {
			return nil, fmt.Errorf("mpc: comparison %d produced %d, which is neither 0 nor 1 — "+
				"the parties are not consistent; refusing to interpret it as a decision about a "+
				"person", k, v)
		}
		out[k] = MatchResult{Similar: v == 1}
	}
	return out, nil
}

// SplitTemplateForParties splits a plaintext sketch into one row per party.
//
// Runs where the biometric still exists in the clear — on the capture device
// or in the enrolling client — and then each row goes to exactly one party.
// After this function returns, no single machine can reconstruct the template
// again, and that is the entire security argument of the system.
func SplitTemplateForParties(sketch []uint8, parties int) ([]PartyTemplate, error) {
	if len(sketch) == 0 {
		return nil, fmt.Errorf("mpc: empty sketch")
	}
	rows := make([]PartyTemplate, parties)
	for i := range rows {
		rows[i] = make(PartyTemplate, len(sketch))
	}
	for j, b := range sketch {
		if b > 1 {
			return nil, fmt.Errorf("mpc: sketch bit %d is %d, expected 0 or 1", j, b)
		}
		sh, err := Split(Element(b), parties)
		if err != nil {
			return nil, err
		}
		for i := range sh {
			rows[i][j] = sh[i]
		}
	}
	return rows, nil
}
