package marketsignals

import (
	"strings"
	"testing"
)

// stubAgent lets the ensemble's gating be tested against exact inputs rather
// than against whatever the real agents happen to think about synthetic data.
type stubAgent struct {
	name string
	fam  Family
	sig  Signal
}

func (s stubAgent) Name() string   { return s.name }
func (s stubAgent) Family() Family { return s.fam }
func (s stubAgent) Warmup() int    { return 1 }
func (s stubAgent) Evaluate(View) Signal {
	out := s.sig
	out.Agent, out.Family = s.name, s.fam
	return out
}

func stub(name string, fam Family, dir Direction, strength float64) stubAgent {
	return stubAgent{name: name, fam: fam, sig: Signal{Dir: dir, Strength: strength}}
}

func ensembleView() View {
	s := trendSeries(50, 1, 0.005)
	return NewView(s, len(s.Candles))
}

// TestEnsemble_RejectsSingleFamilyAgreement is the reason the Family field
// exists. Three trend agents agreeing is one opinion sampled three times, and
// an ensemble that counts it as three has quietly tripled its confidence
// without acquiring any new information.
func TestEnsemble_RejectsSingleFamilyAgreement(t *testing.T) {
	e := NewEnsemble()
	e.Agents = []Agent{
		stub("trend-a", FamilyTrend, Long, 1.0),
		stub("trend-b", FamilyTrend, Long, 1.0),
		stub("trend-c", FamilyTrend, Long, 1.0),
	}
	res := e.Evaluate(ensembleView())
	if res.Dir != Flat {
		t.Fatalf("ensemble took a %s on three agents from one family: %s", res.Dir, res.Note)
	}
	if !strings.Contains(res.Note, "one opinion") {
		t.Fatalf("note %q should name the correlation problem", res.Note)
	}
}

func TestEnsemble_AcceptsCrossFamilyAgreement(t *testing.T) {
	e := NewEnsemble()
	e.Agents = []Agent{
		stub("trend-a", FamilyTrend, Long, 0.8),
		stub("flow-a", FamilyFlow, Long, 0.9),
	}
	res := e.Evaluate(ensembleView())
	if res.Dir != Long {
		t.Fatalf("ensemble = %s (%s), want long from two independent families", res.Dir, res.Note)
	}
	if res.Strength <= 0 || res.Strength > 1 {
		t.Fatalf("combined strength %v outside (0,1]", res.Strength)
	}
}

// TestEnsemble_VetoesCrossFamilyConflict: when two genuinely different reads
// of the market contradict each other, the position that expresses what we
// know is no position — not the weighted average of a yes and a no.
func TestEnsemble_VetoesCrossFamilyConflict(t *testing.T) {
	e := NewEnsemble()
	e.Agents = []Agent{
		stub("trend-a", FamilyTrend, Long, 1.0),
		stub("trend-b", FamilyTrend, Long, 1.0),
		stub("flow-a", FamilyFlow, Short, 0.2),
	}
	res := e.Evaluate(ensembleView())
	if res.Dir != Flat {
		t.Fatalf("ensemble took a %s despite a cross-family objection: %s", res.Dir, res.Note)
	}
	if !strings.Contains(res.Note, "veto") {
		t.Fatalf("note %q should record the veto", res.Note)
	}
}

func TestEnsemble_RequiresMinimumStrength(t *testing.T) {
	e := NewEnsemble()
	e.Agents = []Agent{
		stub("trend-a", FamilyTrend, Long, 0.1),
		stub("flow-a", FamilyFlow, Long, 0.1),
	}
	if res := e.Evaluate(ensembleView()); res.Dir != Flat {
		t.Fatalf("ensemble acted on strength below its floor: %s", res.Note)
	}
}

func TestEnsemble_AddingAgentsDoesNotInflateStrength(t *testing.T) {
	view := ensembleView()

	two := NewEnsemble()
	two.Agents = []Agent{
		stub("trend-a", FamilyTrend, Long, 0.8),
		stub("flow-a", FamilyFlow, Long, 0.8),
	}
	four := NewEnsemble()
	four.Agents = []Agent{
		stub("trend-a", FamilyTrend, Long, 0.8),
		stub("trend-b", FamilyTrend, Long, 0.8),
		stub("flow-a", FamilyFlow, Long, 0.8),
		stub("flow-b", FamilyFlow, Long, 0.8),
	}

	a, b := two.Evaluate(view), four.Evaluate(view)
	if a.Strength != b.Strength {
		t.Fatalf("strength moved from %v to %v purely by adding correlated members — "+
			"conviction must come from the market, not from the roster size", a.Strength, b.Strength)
	}
}

func TestEnsemble_ReportsEveryMemberEvenWhenFlat(t *testing.T) {
	e := NewEnsemble()
	res := e.Evaluate(NewView(trendSeries(400, 2, 0.005), 400))
	if len(res.Members) != len(e.Agents) {
		t.Fatalf("got %d member signals for %d agents — a silent agent is indistinguishable "+
			"from a missing one when auditing a trade", len(res.Members), len(e.Agents))
	}
	for _, m := range res.Members {
		if m.Note == "" {
			t.Fatalf("agent %q returned no explanation", m.Agent)
		}
	}
}
