package marketsignals

import (
	"strings"
	"testing"
	"time"
)

func TestBreakoutAgent_GoesLongInAnUptrend(t *testing.T) {
	s := trendSeries(400, 3, 0.005)
	a := NewBreakoutAgent()
	sig := a.Evaluate(NewView(s, len(s.Candles)))
	if sig.Dir != Long {
		t.Fatalf("breakout in a sustained uptrend = %s (%s), want long", sig.Dir, sig.Note)
	}
	if sig.Strength <= 0 || sig.Strength > 1 {
		t.Fatalf("strength %v outside (0,1]", sig.Strength)
	}
}

func TestBreakoutAgent_StaysOutOfARange(t *testing.T) {
	s := flatSeries(400, 1.0)
	a := NewBreakoutAgent()
	// Sweep the whole range-bound series: a breakout agent that fires even
	// occasionally in chop bleeds fees, and the ATR filter exists precisely
	// to make that impossible rather than merely unlikely.
	for n := a.Warmup(); n <= len(s.Candles); n++ {
		if sig := a.Evaluate(NewView(s, n)); sig.Dir != Flat {
			t.Fatalf("breakout fired %s at bar %d of a range-bound market: %s", sig.Dir, n-1, sig.Note)
		}
	}
}

func TestReversionAgent_FadesACapitulationWick(t *testing.T) {
	s := flatSeries(300, 1.0)
	// A violent flush that closes well off the low: the low was rejected,
	// which is the evidence this agent trades on.
	appendCandle(s, 100, 100.2, 96, 98, 0, 0)

	a := NewReversionAgent()
	sig := a.Evaluate(NewView(s, len(s.Candles)))
	if sig.Dir != Long {
		t.Fatalf("reversion after a rejected flush = %s (%s), want long", sig.Dir, sig.Note)
	}
}

func TestReversionAgent_RequiresRejection(t *testing.T) {
	s := flatSeries(300, 1.0)
	// Same stretched close, same bar range, but the close sits ON the low —
	// nothing was rejected, so there is no absorbed seller to fade. Fading
	// this is just catching a falling knife with extra steps. The range is
	// kept wide (via the upper wick) so that ATR, and therefore the regime
	// gate, matches the test above and only the wick rule differs.
	appendCandle(s, 100, 102, 98, 98, 0, 0)

	a := NewReversionAgent()
	sig := a.Evaluate(NewView(s, len(s.Candles)))
	if sig.Dir != Flat {
		t.Fatalf("reversion took a %s on a close-at-lows bar: %s", sig.Dir, sig.Note)
	}
	if !strings.Contains(sig.Note, "rejection wick") {
		t.Fatalf("note %q should explain the missing rejection wick", sig.Note)
	}
}

func TestReversionAgent_StandsDownInATrend(t *testing.T) {
	// The trade that kills mean-reversion systems: shorting strength in a
	// market that is going up and not coming back.
	s := trendSeries(400, 5, 0.01)
	a := NewReversionAgent()
	for n := a.Warmup(); n <= len(s.Candles); n++ {
		if sig := a.Evaluate(NewView(s, n)); sig.Dir == Short {
			t.Fatalf("reversion shorted a trending market at bar %d: %s", n-1, sig.Note)
		}
	}
}

func TestFlowAgent_RefusesToGuessMissingFlow(t *testing.T) {
	s := randomWalkSeries(200, 11, 0.02) // no BuyVolume/SellVolume anywhere
	a := NewFlowAgent()
	sig := a.Evaluate(NewView(s, len(s.Candles)))
	if sig.Dir != Flat {
		t.Fatalf("flow agent took a %s position with no taker data at all", sig.Dir)
	}
	if !strings.Contains(sig.Note, "does not guess flow") {
		t.Fatalf("note %q should say the agent stood down for lack of data", sig.Note)
	}
}

// absorptionSeries builds the textbook bullish divergence: price grinds to a
// new low on every bar, but the selling that drove the first half of the move
// stops and reverses in the second half. The final low is therefore made on
// buying pressure, not on supply.
func absorptionSeries() *Series {
	s := &Series{Symbol: "ABSORB", Interval: time.Hour}
	t0 := time.Unix(1_700_000_000, 0).UTC()
	price := 100.0
	for i := 0; i < 35; i++ {
		open := price
		price = open - 0.3
		buy, sell := 100.0, 900.0
		if i >= 20 {
			buy, sell = 900.0, 100.0
		}
		s.Candles = append(s.Candles, Candle{
			Time: t0.Add(time.Duration(i) * time.Hour),
			Open: open, High: open + 0.05, Low: price - 0.05, Close: price,
			Volume: buy + sell, BuyVolume: buy, SellVolume: sell,
		})
	}
	return s
}

func TestFlowAgent_SpotsAbsorptionAtNewLows(t *testing.T) {
	s := absorptionSeries()
	a := NewFlowAgent()
	sig := a.Evaluate(NewView(s, len(s.Candles)))
	if sig.Dir != Long {
		t.Fatalf("flow agent = %s (%s), want long on a new low that delta refuses to confirm",
			sig.Dir, sig.Note)
	}
}

func TestFundingAgent_FadesACrowdedLong(t *testing.T) {
	s := randomWalkSeries(260, 13, 0.01)
	s.Funding = make([]float64, len(s.Candles))
	for i := range s.Funding {
		s.Funding[i] = 0.00005 // an unremarkable baseline
	}
	// Three consecutive bars of longs paying far above anything in the
	// trailing window.
	for i := len(s.Funding) - 3; i < len(s.Funding); i++ {
		s.Funding[i] = 0.003
	}

	a := NewFundingAgent()
	sig := a.Evaluate(NewView(s, len(s.Candles)))
	if sig.Dir != Short {
		t.Fatalf("funding agent = %s (%s), want short against a crowded long", sig.Dir, sig.Note)
	}
}

func TestFundingAgent_IgnoresASingleSpike(t *testing.T) {
	s := randomWalkSeries(260, 17, 0.01)
	s.Funding = make([]float64, len(s.Candles))
	for i := range s.Funding {
		s.Funding[i] = 0.00005
	}
	s.Funding[len(s.Funding)-1] = 0.003 // one print, not a position

	a := NewFundingAgent()
	if sig := a.Evaluate(NewView(s, len(s.Candles))); sig.Dir != Flat {
		t.Fatalf("funding agent took a %s on a single funding print: %s", sig.Dir, sig.Note)
	}
}

func TestFundingAgent_StandsDownWithoutFundingData(t *testing.T) {
	s := randomWalkSeries(260, 19, 0.01)
	a := NewFundingAgent()
	sig := a.Evaluate(NewView(s, len(s.Candles)))
	if sig.Dir != Flat || !strings.Contains(sig.Note, "no funding data") {
		t.Fatalf("want a flat signal explaining the missing data, got %s (%s)", sig.Dir, sig.Note)
	}
}
