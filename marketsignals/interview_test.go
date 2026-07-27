package marketsignals

import (
	"strings"
	"testing"
)

// TestPanel_HiresNobodyOnNoise is the panel's defining behaviour. Given a
// market with no structure at all, the correct number of hires is zero — not
// "the least bad one". A selection process that always produces a winner is
// not selecting, it is ranking, and deploying the top of a field that failed
// is how a research pipeline turns into a loss.
func TestPanel_HiresNobodyOnNoise(t *testing.T) {
	s := withFlow(randomWalkSeries(4000, 909, 0.015))
	s.Funding = make([]float64, len(s.Candles))

	p := NewPanel()
	ivs, err := p.Conduct(s,
		AgentStrategy{Agent: NewBreakoutAgent()},
		AgentStrategy{Agent: NewReversionAgent()},
		AgentStrategy{Agent: NewFlowAgent()},
		AgentStrategy{Agent: NewFibonacciAgent()},
		AgentStrategy{Agent: NewPatternAgent()},
		NewEnsemble(),
	)
	if err != nil {
		t.Fatalf("conduct: %v", err)
	}
	if got := Hired(ivs); len(got) != 0 {
		t.Fatalf("hired %v on a pure random walk", got)
	}
	for _, iv := range ivs {
		if len(iv.FailureReasons()) == 0 {
			t.Fatalf("%s was not hired but no criterion is recorded as failed", iv.Candidate)
		}
	}
	t.Log("\n" + p.Report(ivs))
}

// TestPanel_RequiresEveryCriterion: criteria are not averaged. A candidate
// with a spectacular headline number that fails one hard requirement must
// still be rejected, because the failure modes are not interchangeable — a
// strategy whose edge evaporates at realistic slippage does not become
// deployable by having a good Sharpe elsewhere.
func TestPanel_RequiresEveryCriterion(t *testing.T) {
	s := constantGrowthSeries(600, 0.004)

	p := NewPanel()
	// A market that only goes up makes always-long look magnificent, and it
	// will clear most criteria comfortably.
	ivs, err := p.Conduct(s, alwaysLong{})
	if err != nil {
		t.Fatalf("conduct: %v", err)
	}
	if len(ivs) != 1 {
		t.Fatalf("got %d interviews, want 1", len(ivs))
	}
	iv := ivs[0]

	passed := 0
	for _, a := range iv.Assessments {
		if a.Passed {
			passed++
		}
	}
	if passed == 0 {
		t.Fatal("a strategy that is long a market rising every single bar should pass " +
			"several criteria; the panel appears to reject unconditionally")
	}
	// It never trades, so the evidence criterion must fail and the hire must
	// not happen regardless of how good the equity curve looks.
	if iv.Hired {
		t.Fatalf("hired a buy-and-hold with %d trades on a synthetic straight line:\n%s",
			iv.Result.Metrics.Trades, p.Report(ivs))
	}
	if !strings.Contains(strings.Join(iv.FailureReasons(), " "), "enough evidence") {
		t.Fatalf("failure reasons %v should include the trade-count criterion", iv.FailureReasons())
	}
}

// TestPanel_StressTestUsesHarsherSlippage confirms the cost stress round is
// actually harsher, rather than reporting the same run twice.
func TestPanel_StressTestUsesHarsherSlippage(t *testing.T) {
	s := withFlow(randomWalkSeries(2500, 4242, 0.02))

	p := NewPanel()
	ivs, err := p.Conduct(s, AgentStrategy{Agent: NewBreakoutAgent()})
	if err != nil {
		t.Fatalf("conduct: %v", err)
	}
	iv := ivs[0]
	if iv.Result.Metrics.Turnover <= 0 {
		t.Fatal("candidate never traded; the stress comparison proves nothing")
	}
	if !(iv.Stressed.Metrics.TotalReturn < iv.Result.Metrics.TotalReturn) {
		t.Fatalf("stressed return %.4f is not worse than the base %.4f — the stress round "+
			"is not applying the harsher slippage",
			iv.Stressed.Metrics.TotalReturn, iv.Result.Metrics.TotalReturn)
	}
}

// TestPanel_MoreCandidatesRaisesTheBar encodes the counterintuitive property
// that makes this honest: searching harder must make the bar HIGHER, not
// lower. The same candidate, judged among many, needs a stronger result.
func TestPanel_MoreCandidatesRaisesTheBar(t *testing.T) {
	s := withFlow(randomWalkSeries(3000, 55, 0.015))
	bt := NewBacktester()

	alone, err := bt.SelectBest(s, 4, AgentStrategy{Agent: NewBreakoutAgent()})
	if err != nil {
		t.Fatalf("solo: %v", err)
	}
	crowd, err := bt.SelectBest(s, 4,
		AgentStrategy{Agent: NewBreakoutAgent()},
		AgentStrategy{Agent: NewReversionAgent()},
		AgentStrategy{Agent: NewFlowAgent()},
		AgentStrategy{Agent: NewFibonacciAgent()},
		AgentStrategy{Agent: NewPatternAgent()},
	)
	if err != nil {
		t.Fatalf("field: %v", err)
	}

	if alone.SelectionHurdle != 0 {
		t.Fatalf("a single candidate involves no selection, so the hurdle should be 0, got %.4f",
			alone.SelectionHurdle)
	}
	if !(crowd.SelectionHurdle > 0) {
		t.Fatalf("hurdle with five candidates is %.4f; trying more strategies must cost "+
			"something", crowd.SelectionHurdle)
	}

	find := func(sel Selection, name string) Candidate {
		for _, c := range sel.Candidates {
			if c.Strategy.Name() == name {
				return c
			}
		}
		t.Fatalf("%s missing from selection", name)
		return Candidate{}
	}
	solo, infield := find(alone, "breakout"), find(crowd, "breakout")
	if solo.Result.Metrics.Sharpe != infield.Result.Metrics.Sharpe {
		t.Fatal("the same strategy on the same data produced different Sharpes")
	}
	if !(infield.DeflatedSharpe <= solo.DeflatedSharpe) {
		t.Fatalf("identical performance was judged MORE convincing in a field of five "+
			"(%.4f) than alone (%.4f)", infield.DeflatedSharpe, solo.DeflatedSharpe)
	}
}
