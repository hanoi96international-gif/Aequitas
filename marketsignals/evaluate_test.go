package marketsignals

import (
	"math"
	"strings"
	"testing"
)

// TestPanel_JudgesBooksAndStrategiesInOneField closes the integrity gap this
// file exists for. Everything in this package refuses to deploy an unproven
// strategy; before Runnable, the book — the more complex object, with more
// ways to be wrong — was the one thing no bar applied to.
func TestPanel_JudgesBooksAndStrategiesInOneField(t *testing.T) {
	u := correlatedUniverse(10, 1200, 0.3, 202)
	series := withFlow(randomWalkSeries(1200, 303, 0.015))

	xs := NewCrossSectional()
	xs.MinNames = 8

	p := NewPanel()
	ivs, err := p.ConductRunnables(
		StrategyRunnable{Series: series, Strategy: AgentStrategy{Agent: NewBreakoutAgent()}},
		StrategyRunnable{Series: series, Strategy: AgentStrategy{Agent: NewReversionAgent()}},
		PortfolioRunnable{Universe: u, Allocator: xs},
	)
	if err != nil {
		t.Fatalf("conduct: %v", err)
	}
	if len(ivs) != 3 {
		t.Fatalf("interviewed %d candidates, want 3", len(ivs))
	}

	// Every candidate faces the same five criteria, book or not.
	for _, iv := range ivs {
		if len(iv.Assessments) != 5 {
			t.Fatalf("%s faced %d criteria, want the same 5 everything else faces",
				iv.Candidate, len(iv.Assessments))
		}
	}
	t.Log("\n" + p.Report(ivs))
}

// TestPortfolioRunnable_StatesTheSurvivorshipCaveat. Every statistical control
// here asks how much of a result is selection WITHIN the data. None can see a
// problem in which data exists, and a universe has one that a single
// instrument does not.
func TestPortfolioRunnable_StatesTheSurvivorshipCaveat(t *testing.T) {
	u := correlatedUniverse(10, 900, 0.3, 404)
	xs := NewCrossSectional()
	xs.MinNames = 8

	r := PortfolioRunnable{Universe: u, Allocator: xs}
	caveats := strings.Join(r.Caveats(), " ")
	for _, want := range []string{"survivors", "universe choice"} {
		if !strings.Contains(caveats, want) {
			t.Fatalf("caveats %q should mention %q", caveats, want)
		}
	}

	// A single instrument carries no such problem and should claim none.
	if got := (StrategyRunnable{Strategy: AgentStrategy{Agent: NewBreakoutAgent()}}).Caveats(); len(got) != 0 {
		t.Fatalf("a single-instrument candidate claimed caveats: %v", got)
	}

	// And the caveat reaches the report, where somebody will read it.
	p := NewPanel()
	ivs, err := p.ConductRunnables(r)
	if err != nil {
		t.Fatalf("conduct: %v", err)
	}
	if !strings.Contains(p.Report(ivs), "survivors") {
		t.Fatalf("the report omits the caveat:\n%s", p.Report(ivs))
	}
}

// TestPanel_MoreCandidatesRaisesTheBarForBooksToo: searching harder must cost
// something on the portfolio path exactly as it does on the single-instrument
// one.
func TestPanel_MoreCandidatesRaisesTheBarForBooksToo(t *testing.T) {
	u := correlatedUniverse(10, 1500, 0.25, 505)

	solo := PortfolioRunnable{Universe: u, Allocator: func() Allocator {
		c := NewCrossSectional()
		c.MinNames = 8
		return c
	}()}

	p := NewPanel()
	alone, err := p.ConductRunnables(solo)
	if err != nil {
		t.Fatalf("solo: %v", err)
	}

	field := []Runnable{solo}
	for _, c := range AllocatorGrid()[:8] {
		c.MinNames = 8
		field = append(field, PortfolioRunnable{Universe: u, Allocator: c})
	}
	crowd, err := p.ConductRunnables(field...)
	if err != nil {
		t.Fatalf("field: %v", err)
	}

	find := func(ivs []Interview, name string) Interview {
		for _, iv := range ivs {
			if iv.Candidate == name {
				return iv
			}
		}
		t.Fatalf("%s missing from the field", name)
		return Interview{}
	}
	a := find(alone, solo.Name())
	b := find(crowd, solo.Name())

	if a.Result.Metrics.Sharpe != b.Result.Metrics.Sharpe {
		t.Fatal("the same book on the same data produced different Sharpes")
	}
	pa := detailOf(a, "edge survives selection")
	pb := detailOf(b, "edge survives selection")
	if pa == pb {
		t.Fatalf("identical performance was judged identically alone and in a field of nine:\n"+
			"  %s\n  %s", pa, pb)
	}
}

func detailOf(iv Interview, criterion string) string {
	for _, a := range iv.Assessments {
		if a.Criterion == criterion {
			return a.Detail
		}
	}
	return ""
}

// TestFoldsOf_DecomposesTheRunItClaimsTo. Slicing an existing run rather than
// re-running is more correct as well as cheaper: a re-run differs in
// path-dependent state, so its folds would not sum to the run they describe.
func TestFoldsOf_DecomposesTheRunItClaimsTo(t *testing.T) {
	res, err := NewBacktester().Run(withFlow(trendSeries(1500, 61, 0.004)),
		AgentStrategy{Agent: NewBreakoutAgent()})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	folds, err := FoldsOf(res, 5)
	if err != nil {
		t.Fatalf("folds: %v", err)
	}
	total := 0
	for _, f := range folds {
		total += f.Metrics.Bars
	}
	if total != len(res.Returns) {
		t.Fatalf("folds cover %d bars, the run produced %d — a split that drops or duplicates "+
			"bars misreports out-of-sample performance", total, len(res.Returns))
	}
	if folds[0].FromBar != res.Steps[0].Index {
		t.Fatalf("first fold starts at bar %d, run starts at %d", folds[0].FromBar, res.Steps[0].Index)
	}

	if _, err := FoldsOf(res, 1); err == nil {
		t.Fatal("expected an error for a single fold")
	}
	if _, err := FoldsOf(Result{}, 5); err == nil {
		t.Fatal("expected an error for an empty run")
	}
}

// TestRunnable_StressRoundReachesEveryCandidate: a book must not be able to
// hide an optimistic cost assumption from the round designed to catch it.
func TestRunnable_StressRoundReachesEveryCandidate(t *testing.T) {
	u := correlatedUniverse(10, 1200, 0.3, 606)
	xs := NewCrossSectional()
	xs.MinNames = 8

	p := NewPanel()
	ivs, err := p.ConductRunnables(PortfolioRunnable{Universe: u, Allocator: xs})
	if err != nil {
		t.Fatalf("conduct: %v", err)
	}
	iv := ivs[0]
	if iv.Result.Metrics.Turnover == 0 {
		t.Fatal("the book never traded; the stress comparison proves nothing")
	}
	if !(iv.Stressed.Metrics.TotalReturn < iv.Result.Metrics.TotalReturn) {
		t.Fatalf("stressed return %.4f is not worse than the base %.4f — the harsher costs "+
			"are not reaching the book",
			iv.Stressed.Metrics.TotalReturn, iv.Result.Metrics.TotalReturn)
	}
}

// TestPanel_HiresNobodyOnANoiseUniverse is the portfolio path's negative
// control at the hiring bar, which is where it matters: a book that cannot be
// rejected is a book that will be deployed.
func TestPanel_HiresNobodyOnANoiseUniverse(t *testing.T) {
	u := correlatedUniverse(12, 1500, 0.25, 707)

	var field []Runnable
	for _, c := range AllocatorGrid() {
		c.MinNames = 8
		field = append(field, PortfolioRunnable{Universe: u, Allocator: c})
	}

	p := NewPanel()
	ivs, err := p.ConductRunnables(field...)
	if err != nil {
		t.Fatalf("conduct: %v", err)
	}
	if got := Hired(ivs); len(got) != 0 {
		t.Fatalf("hired %v from a universe of correlated random walks", got)
	}
	t.Logf("%d allocator variants interviewed on noise, none hired", len(ivs))
}

func TestAllocatorGrid_ProducesDistinctNamedVariants(t *testing.T) {
	grid := AllocatorGrid()
	if len(grid) < 10 {
		t.Fatalf("grid has %d variants; too few to be a search", len(grid))
	}
	seen := map[string]bool{}
	for _, c := range grid {
		n := c.Name()
		if seen[n] {
			t.Fatalf("two variants share the name %q — a field of identically named "+
				"candidates is a report nobody can act on", n)
		}
		seen[n] = true
		if !strings.HasPrefix(n, "xs/") {
			t.Fatalf("name %q does not identify the allocator family", n)
		}
	}
}

func TestPanel_ConductRunnablesRejectsAnEmptyField(t *testing.T) {
	if _, err := NewPanel().ConductRunnables(); err == nil {
		t.Fatal("expected an error for an empty field")
	}
}

// TestPortfolioRunnable_HonoursPerSymbolCosts confirms a book's expensive name
// stays expensive through the evaluation path, not only in a direct backtest.
func TestPortfolioRunnable_HonoursPerSymbolCosts(t *testing.T) {
	u := correlatedUniverse(10, 1200, 0.3, 808)
	xs := NewCrossSectional()
	xs.MinNames = 8

	cheap := PortfolioRunnable{Universe: u, Allocator: xs}
	pricey := PortfolioRunnable{Universe: u, Allocator: xs,
		PerSymbolCosts: map[string]CostModel{"A": DefaultAMMCosts(150_000, 300_000)}}

	a, err := cheap.RunWith(DefaultCosts())
	if err != nil {
		t.Fatalf("cheap: %v", err)
	}
	b, err := pricey.RunWith(DefaultCosts())
	if err != nil {
		t.Fatalf("pricey: %v", err)
	}
	if !(b.Metrics.TotalReturn < a.Metrics.TotalReturn) {
		t.Fatalf("per-symbol costs did not survive the Runnable path: %.4f vs %.4f",
			b.Metrics.TotalReturn, a.Metrics.TotalReturn)
	}
	if math.IsNaN(b.Metrics.Sharpe) {
		t.Fatal("stressed run produced a NaN Sharpe")
	}
}
