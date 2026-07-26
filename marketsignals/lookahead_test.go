package marketsignals

import (
	"testing"
)

// The tests in this file are the ones that make every other number in the
// package worth reading. A backtest that can see the future produces
// beautiful, meaningless results, and the leak is usually a single index —
// invisible in review, decisive in the output. So rather than inspecting the
// code for lookahead, these tests establish it empirically: rewrite the
// future and check that the past did not notice.

// TestView_CannotReachPastItsEnd confirms the structural guarantee that the
// View API rests on: the slice it hands out is capped, so a caller cannot
// re-slice their way to a bar the agent is not entitled to see.
func TestView_CannotReachPastItsEnd(t *testing.T) {
	s := trendSeries(100, 1, 0.01)
	v := NewView(s, 40)

	if got := len(v.Candles()); got != 40 {
		t.Fatalf("view exposes %d bars, want 40", got)
	}
	if got := cap(v.Candles()); got != 40 {
		t.Fatalf("view slice has capacity %d, want 40 — an uncapped slice can be "+
			"re-sliced into the future by any caller", got)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("At(40) on a 40-bar view must panic, not return bar 40")
			}
		}()
		_ = v.At(40)
	}()
}

// TestAgents_IgnoreRewrittenFuture is the core proof. It takes a series,
// produces every agent's signal at each bar, then replaces every bar after a
// cut point with wildly different data and recomputes. Signals at or before
// the cut must be bit-identical: an agent that changes its mind about bar 300
// because bar 500 changed is reading the future.
func TestAgents_IgnoreRewrittenFuture(t *testing.T) {
	const cut = 300

	agents := []Agent{
		NewBreakoutAgent(),
		NewReversionAgent(),
		NewFlowAgent(),
		NewFundingAgent(),
	}

	original := withFlow(randomWalkSeries(600, 42, 0.02))
	original.Funding = make([]float64, len(original.Candles))
	for i := range original.Funding {
		original.Funding[i] = 0.0001 * float64(i%7)
	}

	// A future that looks nothing like the past: a violent crash with
	// inverted flow and extreme funding. If any agent leaks, this moves it.
	rewritten := &Series{
		Symbol:   original.Symbol,
		Interval: original.Interval,
		Candles:  append([]Candle(nil), original.Candles...),
		Funding:  append([]float64(nil), original.Funding...),
	}
	price := rewritten.Candles[cut].Close
	for i := cut + 1; i < len(rewritten.Candles); i++ {
		open := price
		price = open * 0.95
		c := &rewritten.Candles[i]
		c.Open, c.Close = open, price
		c.High, c.Low = open*1.001, price*0.99
		c.BuyVolume, c.SellVolume = 10, 5000
		c.Volume = c.BuyVolume + c.SellVolume
		rewritten.Funding[i] = 0.05
	}

	for _, a := range agents {
		for n := a.Warmup(); n <= cut+1; n++ {
			want := a.Evaluate(NewView(original, n))
			got := a.Evaluate(NewView(rewritten, n))
			if want.Dir != got.Dir || want.Strength != got.Strength {
				t.Fatalf("%s at bar %d changed after the FUTURE was rewritten: "+
					"was %s/%.4f, now %s/%.4f — this agent reads bars it cannot have",
					a.Name(), n-1, want.Dir, want.Strength, got.Dir, got.Strength)
			}
		}
	}
}

// TestBacktester_DecisionsIgnoreRewrittenFuture applies the same treatment to
// the full pipeline, including risk sizing. Steps well before the cut must
// take identical positions.
//
// The comparison stops three bars short of the cut on purpose rather than out
// of caution: a step's SIZE legitimately depends on the equity path, the
// equity path depends on returns realised two bars later, so steps adjacent
// to the cut are allowed to differ without any lookahead being involved.
func TestBacktester_DecisionsIgnoreRewrittenFuture(t *testing.T) {
	const cut = 400

	base := withFlow(trendSeries(800, 7, 0.004))
	rewritten := &Series{
		Symbol:   base.Symbol,
		Interval: base.Interval,
		Candles:  append([]Candle(nil), base.Candles...),
	}
	price := rewritten.Candles[cut].Close
	for i := cut + 1; i < len(rewritten.Candles); i++ {
		open := price
		price = open * 1.10
		c := &rewritten.Candles[i]
		c.Open, c.Close = open, price
		c.High, c.Low = price*1.01, open*0.99
	}

	bt := NewBacktester()
	strat := AgentStrategy{Agent: NewBreakoutAgent()}

	a, err := bt.Run(base, strat)
	if err != nil {
		t.Fatalf("run on base: %v", err)
	}
	b, err := bt.Run(rewritten, strat)
	if err != nil {
		t.Fatalf("run on rewritten: %v", err)
	}

	compared := 0
	for i := range a.Steps {
		if a.Steps[i].Index > cut-3 {
			break
		}
		if a.Steps[i].Target != b.Steps[i].Target {
			t.Fatalf("bar %d took position %.6f originally and %.6f after the future was "+
				"rewritten — the pipeline leaks", a.Steps[i].Index, a.Steps[i].Target, b.Steps[i].Target)
		}
		compared++
	}
	if compared < 100 {
		t.Fatalf("only %d steps compared; the test is not exercising enough of the run", compared)
	}
}

// TestDonchian_ExcludesCurrentBar guards the specific off-by-one that turns a
// breakout backtest into a tautology: if the channel includes the bar under
// test, that bar can never break it, and the agent's entries become
// impossible to take.
func TestDonchian_ExcludesCurrentBar(t *testing.T) {
	cs := []Candle{
		{High: 10, Low: 9, Open: 9.5, Close: 9.5},
		{High: 11, Low: 10, Open: 10.5, Close: 10.5},
		{High: 12, Low: 11, Open: 11.5, Close: 11.5},
		// The current bar has by far the highest high; it must not count.
		{High: 99, Low: 11, Open: 12, Close: 98},
	}
	high, low := Donchian(cs, 3)
	if high != 12 {
		t.Fatalf("Donchian high = %g, want 12 — the current bar's high of 99 leaked in", high)
	}
	if low != 9 {
		t.Fatalf("Donchian low = %g, want 9", low)
	}
}
