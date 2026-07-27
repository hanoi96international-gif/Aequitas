package marketsignals

import (
	"fmt"
	"sort"
)

// Ensemble combines agents into one opinion, and its job is mostly to say
// nothing. The failure mode it exists to prevent is agreement that only looks
// like corroboration: five indicators computed from the same closing prices
// will agree constantly, and averaging them produces a confident signal built
// on one piece of evidence counted five times.
//
// So agreement is counted in FAMILIES, not agents. MinFamilies is the real
// control here; MinAgents alone would let a bank of trend followers vote
// itself into a position.
type Ensemble struct {
	Agents  []Agent
	Weights map[string]float64 // by agent name; absent means 1

	// Modulators shrink the position without voting on direction. They are
	// kept separate from Agents on purpose: something that only ever says
	// "not now" would otherwise have to be encoded as a permanent Flat vote,
	// where it would be indistinguishable from an agent that simply has no
	// opinion, and its warning would be silently discarded.
	Modulators []RiskModulator

	MinAgents   int     // how many agents must point the same way
	MinFamilies int     // how many DISTINCT families must be among them
	MinStrength float64 // combined strength floor, below which we stay flat
	// VetoOnConflict makes any opposing signal from a different family cancel
	// the trade outright. When two independent methods disagree, the honest
	// position is no position, not the weighted average of a yes and a no.
	VetoOnConflict bool
}

// NewEnsemble wires the four market agents with the conservative gating that
// makes the whole thing quiet: two agents from two different families, with
// any cross-family disagreement vetoing the trade.
func NewEnsemble() *Ensemble {
	macro := NewMacroAgent()
	return &Ensemble{
		Agents: []Agent{
			NewBreakoutAgent(),
			NewReversionAgent(),
			NewFlowAgent(),
			NewFundingAgent(),
			NewFibonacciAgent(),
			NewPatternAgent(),
			macro,
		},
		// The macro agent appears in both lists deliberately. Its directional
		// opinion after a surprise is one vote among many; its judgement that
		// a referendum lands in two hours is not a vote at all and must not be
		// outvoted by six chart readings.
		Modulators:     []RiskModulator{macro},
		MinAgents:      2,
		MinFamilies:    2,
		MinStrength:    0.3,
		VetoOnConflict: true,
	}
}

// ModulateRisk combines every modulator by taking the MINIMUM scale rather
// than the average. Risk warnings do not offset each other: two independent
// reasons to be small are not one reason to be medium.
func (e *Ensemble) ModulateRisk(v View) RiskAdjustment {
	out := NoAdjustment("ensemble", "no modulator asked for less risk")
	for _, m := range e.Modulators {
		adj := m.ModulateRisk(v)
		scale := clamp(adj.Scale, 0, 1)
		if adj.Veto {
			scale = 0
		}
		if scale < out.Scale {
			out = RiskAdjustment{Scale: scale, Veto: scale == 0, Source: adj.Source, Reason: adj.Reason}
		}
	}
	return out
}

// Warmup is the longest warmup among the members: until every agent can
// speak, the ensemble's family-diversity requirement is measuring which
// agents have enough history rather than what the market is doing.
func (e *Ensemble) Warmup() int {
	w := 0
	for _, a := range e.Agents {
		w = maxInt(w, a.Warmup())
	}
	return w
}

func (e *Ensemble) weight(name string) float64 {
	if e.Weights == nil {
		return 1
	}
	if w, ok := e.Weights[name]; ok {
		return w
	}
	return 1
}

// EnsembleResult carries the decision plus every input to it, so a trade can
// be explained after the fact rather than reconstructed.
type EnsembleResult struct {
	Signal
	Members  []Signal
	Families []Family
}

// Evaluate polls every member and applies the gates.
func (e *Ensemble) Evaluate(v View) EnsembleResult {
	res := EnsembleResult{}
	res.Agent = "ensemble"

	longW, shortW := 0.0, 0.0
	longFams, shortFams := map[Family]bool{}, map[Family]bool{}
	longN, shortN := 0, 0

	for _, a := range e.Agents {
		s := a.Evaluate(v)
		res.Members = append(res.Members, s)
		if s.Dir == Flat {
			continue
		}
		w := e.weight(a.Name()) * s.Strength
		if s.Dir == Long {
			longW += w
			longN++
			longFams[s.Family] = true
		} else {
			shortW += w
			shortN++
			shortFams[s.Family] = true
		}
	}

	if e.VetoOnConflict && longN > 0 && shortN > 0 {
		// Same-family disagreement is ordinary parameter noise; cross-family
		// disagreement means two genuinely different reads of the market
		// contradict each other, and that is information — namely, that we
		// do not know.
		if !sameFamilySet(longFams, shortFams) {
			res.Dir = Flat
			res.Note = fmt.Sprintf("veto: %s disagree with %s", famList(longFams), famList(shortFams))
			return res
		}
	}

	dir, n, fams, weight := Long, longN, longFams, longW
	if shortW > longW {
		dir, n, fams, weight = Short, shortN, shortFams, shortW
	}

	res.Families = famSlice(fams)

	switch {
	case n < e.MinAgents:
		res.Dir = Flat
		res.Note = fmt.Sprintf("only %d agent(s) agree, need %d", n, e.MinAgents)
		return res
	case len(fams) < e.MinFamilies:
		res.Dir = Flat
		res.Note = fmt.Sprintf("all %d signals come from the %s family alone — that is one opinion, not %d",
			n, famList(fams), n)
		return res
	}

	// Normalise by the number of agreeing agents so that adding members to
	// the ensemble does not inflate strength on its own.
	strength := clamp(weight/float64(n), 0, 1)
	if strength < e.MinStrength {
		res.Dir = Flat
		res.Note = fmt.Sprintf("agreement is weak (%s < %s)", f2(strength), f2(e.MinStrength))
		return res
	}

	res.Dir = dir
	res.Strength = strength
	res.Note = fmt.Sprintf("%d agents across %d families (%s)", n, len(fams), famList(fams))
	return res
}

func sameFamilySet(a, b map[Family]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for f := range a {
		if !b[f] {
			return false
		}
	}
	return true
}

func famSlice(m map[Family]bool) []Family {
	out := make([]Family, 0, len(m))
	for f := range m {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func famList(m map[Family]bool) string {
	fs := famSlice(m)
	out := ""
	for i, f := range fs {
		if i > 0 {
			out += "+"
		}
		out += string(f)
	}
	return out
}
