package marketsignals

import (
	"math"
	"testing"
	"time"
)

// alwaysLong is a strategy with no opinion to test — it exists so the
// backtester's arithmetic can be checked against numbers computed by hand.
type alwaysLong struct{}

func (alwaysLong) Name() string       { return "always-long" }
func (alwaysLong) Warmup() int        { return 2 }
func (alwaysLong) Decide(View) Signal { return Signal{Agent: "always-long", Dir: Long, Strength: 1} }

// constantGrowthSeries rises by exactly rate every bar with no dispersion, so
// realised volatility is zero and the risk manager's sizing collapses to a
// single predictable number. That makes the whole pipeline hand-checkable.
func constantGrowthSeries(n int, rate float64) *Series {
	s := &Series{Symbol: "CONST", Interval: time.Hour}
	t := time.Unix(1_700_000_000, 0).UTC()
	open := 100.0
	for i := 0; i < n; i++ {
		close := open * (1 + rate)
		s.Candles = append(s.Candles, Candle{
			Time: t, Open: open, High: close, Low: open, Close: close, Volume: 1,
		})
		open = close
		t = t.Add(time.Hour)
	}
	return s
}

// TestBacktester_ArithmeticIsHandCheckable pins down every number in the
// simulation loop at once: the position the risk manager takes, the cost
// charged on the change, and the two bars the return is measured between.
func TestBacktester_ArithmeticIsHandCheckable(t *testing.T) {
	const rate = 0.01
	s := constantGrowthSeries(60, rate)

	bt := NewBacktester()
	res, err := bt.Run(s, alwaysLong{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// Zero realised volatility floors at MinVol, so size is fully determined:
	//   (TargetVol / MinVol) * strength * KellyFraction = (0.20/0.10)*1*0.20
	r := bt.Risk
	wantPos := (r.TargetVol / r.MinVol) * 1.0 * r.KellyFraction
	if math.Abs(res.Steps[0].Position-wantPos) > 1e-12 {
		t.Fatalf("position %.12f, want %.12f", res.Steps[0].Position, wantPos)
	}

	// First step pays to establish the whole position; later steps hold it
	// and pay nothing.
	wantCost := bt.Costs.CostFraction(wantPos)
	if math.Abs(res.Steps[0].Cost-wantCost) > 1e-12 {
		t.Fatalf("entry cost %.12f, want %.12f", res.Steps[0].Cost, wantCost)
	}
	if res.Steps[1].Cost != 0 {
		t.Fatalf("holding an unchanged position cost %.12f, want 0", res.Steps[1].Cost)
	}

	// Return is the position times the open-to-open move, less costs.
	if got, want := res.Steps[0].Return, wantPos*rate-wantCost; math.Abs(got-want) > 1e-12 {
		t.Fatalf("first return %.12f, want %.12f", got, want)
	}
	if got, want := res.Steps[1].Return, wantPos*rate; math.Abs(got-want) > 1e-12 {
		t.Fatalf("second return %.12f, want %.12f", got, want)
	}
}

// gapSeries makes every odd-numbered bar open 5% above the previous close and
// then go nowhere. All of the movement in this market happens in the gap
// between one bar's close and the next bar's open — which is precisely the
// move that is NOT available to trade.
func gapSeries(n int) *Series {
	s := &Series{Symbol: "GAP", Interval: time.Hour}
	t := time.Unix(1_700_000_000, 0).UTC()
	open := 100.0
	for i := 0; i < n; i++ {
		close := open // flat bar: everything happens in the gaps
		s.Candles = append(s.Candles, Candle{
			Time: t, Open: open, High: open * 1.001, Low: open * 0.999, Close: close, Volume: 1,
		})
		if (i+1)%2 == 1 {
			open = close * 1.05
		}
		t = t.Add(time.Hour)
	}
	return s
}

// clairvoyant goes long exactly on the bars immediately preceding a gap up.
// It is a perfect forecaster of the next move — and it must still make no
// money, because the move happens before it can be filled.
type clairvoyant struct{}

func (clairvoyant) Name() string { return "clairvoyant" }
func (clairvoyant) Warmup() int  { return 2 }
func (clairvoyant) Decide(v View) Signal {
	if (v.Len()-1)%2 == 0 {
		return Signal{Agent: "clairvoyant", Dir: Long, Strength: 1}
	}
	return Signal{Agent: "clairvoyant", Dir: Flat}
}

// TestBacktester_CannotTradeTheGapItPredicted is the sharpest test of the
// execution model. A backtester that fills at the decision bar's close would
// hand this strategy 5% per trade, and its equity curve would be a rocket. In
// reality the close is only knowable once trading at it is over, so the fill
// happens at the next open — after the gap — and the perfect forecast is
// worth exactly nothing minus fees.
func TestBacktester_CannotTradeTheGapItPredicted(t *testing.T) {
	s := gapSeries(120)
	res, err := NewBacktester().Run(s, clairvoyant{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	for _, st := range res.Steps {
		if st.Return > 0.001 {
			t.Fatalf("bar %d returned %.4f — the backtester is filling before the gap, "+
				"which is the profit nobody could have taken", st.Index, st.Return)
		}
	}
	if res.Metrics.TotalReturn >= 0 {
		t.Fatalf("total return %.4f: a strategy that only ever trades unreachable moves "+
			"must lose exactly its costs", res.Metrics.TotalReturn)
	}
}

func TestBacktester_CostsAreActuallyCharged(t *testing.T) {
	s := withFlow(randomWalkSeries(1500, 77, 0.02))

	free := NewBacktester()
	free.Costs = Costs{}
	paid := NewBacktester()

	strat := AgentStrategy{Agent: NewBreakoutAgent()}

	a, err := free.Run(s, strat)
	if err != nil {
		t.Fatalf("free run: %v", err)
	}
	b, err := paid.Run(s, strat)
	if err != nil {
		t.Fatalf("paid run: %v", err)
	}
	if a.Metrics.Turnover <= 0 {
		t.Fatal("strategy never traded; the cost comparison proves nothing")
	}
	if !(b.Metrics.TotalReturn < a.Metrics.TotalReturn) {
		t.Fatalf("costed run returned %.4f vs %.4f free — costs are not reaching the equity curve",
			b.Metrics.TotalReturn, a.Metrics.TotalReturn)
	}
}

// TestBacktester_NoEdgeOnNoise is the sanity check that the whole package is
// built to survive. These agents are given a market with no structure in it
// whatsoever. If any of them shows a convincing edge here, the finding is
// about a bug in this code, not about the market.
func TestBacktester_NoEdgeOnNoise(t *testing.T) {
	s := withFlow(randomWalkSeries(4000, 2024, 0.015))
	s.Funding = make([]float64, len(s.Candles))
	for i := range s.Funding {
		s.Funding[i] = 0.0001 * math.Sin(float64(i)/13)
	}

	bt := NewBacktester()
	sel, err := bt.SelectBest(s, 4,
		AgentStrategy{Agent: NewBreakoutAgent()},
		AgentStrategy{Agent: NewReversionAgent()},
		AgentStrategy{Agent: NewFlowAgent()},
		AgentStrategy{Agent: NewFundingAgent()},
		NewEnsemble(),
	)
	if err != nil {
		t.Fatalf("select: %v", err)
	}

	for _, c := range sel.Candidates {
		if c.DeflatedSharpe >= 0.95 {
			t.Errorf("%s claims a real edge (P=%.3f) on a pure random walk — "+
				"the evaluation is flattering noise", c.Strategy.Name(), c.DeflatedSharpe)
		}
	}
	t.Log("\n" + sel.Report())
}

// TestSelectBest_HurdleRisesWithTrials encodes the reason SelectBest exists:
// the more strategies you try, the better the best one looks for free, so the
// bar it has to clear must rise with the number of attempts.
func TestSelectBest_HurdleRisesWithTrials(t *testing.T) {
	sd := 0.05
	few := ExpectedMaxSharpe(3, sd)
	many := ExpectedMaxSharpe(500, sd)
	if !(many > few) {
		t.Fatalf("hurdle for 500 trials (%.4f) is not above that for 3 (%.4f)", many, few)
	}
	if ExpectedMaxSharpe(1, sd) != 0 {
		t.Fatal("a single trial involves no selection and must carry no hurdle")
	}
}

func TestBacktester_RejectsTooShortASeries(t *testing.T) {
	s := trendSeries(40, 1, 0.01)
	if _, err := NewBacktester().Run(s, AgentStrategy{Agent: NewFundingAgent()}); err == nil {
		t.Fatal("expected an error rather than a backtest over a handful of warmup bars")
	}
}

func TestBacktester_RejectsMisorderedSeries(t *testing.T) {
	s := trendSeries(200, 1, 0.01)
	s.Candles[100].Time = s.Candles[99].Time // duplicate timestamp
	if _, err := NewBacktester().Run(s, alwaysLong{}); err == nil {
		t.Fatal("expected validation to reject a series whose bars are not strictly ordered")
	}
}

func TestWalkForward_PartitionsEveryBar(t *testing.T) {
	s := withFlow(trendSeries(2000, 9, 0.003))
	bt := NewBacktester()
	strat := AgentStrategy{Agent: NewBreakoutAgent()}

	full, err := bt.Run(s, strat)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	folds, err := bt.WalkForward(s, strat, 5)
	if err != nil {
		t.Fatalf("walk-forward: %v", err)
	}
	if len(folds) != 5 {
		t.Fatalf("got %d folds, want 5", len(folds))
	}

	total := 0
	for _, f := range folds {
		total += f.Metrics.Bars
	}
	if total != len(full.Returns) {
		t.Fatalf("folds cover %d bars but the run produced %d — a fold split that drops or "+
			"duplicates bars misreports out-of-sample performance", total, len(full.Returns))
	}
}
