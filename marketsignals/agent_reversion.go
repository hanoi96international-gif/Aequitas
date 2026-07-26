package marketsignals

import "math"

// ReversionAgent fades forced sellers, not weak prices. Those are different
// things, and conflating them is why most mean-reversion systems eventually
// short a bull market to zero equity.
//
// It only speaks when all of these hold:
//
//   - The market is NOT trending: the fast and slow EMAs are within
//     MaxTrendATR ATRs of each other. Reversion in a trend is a losing bet
//     with a good hit rate, which is the most dangerous combination there is,
//     because it pays out steadily right up until the trade that erases it.
//   - The close is at least MinZ standard deviations from its own mean.
//   - The bar shows REJECTION: a wick of at least MinWickFrac of the bar's
//     range on the side the move came from. That wick is the actual evidence
//     — it is what a liquidation cascade being absorbed by resting bids looks
//     like on a chart. Without it, an extended close is just a market going
//     somewhere, and there is nothing to fade.
type ReversionAgent struct {
	Lookback    int // window for the mean and its dispersion
	ATRPeriod   int
	MinZ        float64 // how stretched, in standard deviations
	MinWickFrac float64 // rejection wick as a fraction of the bar's range
	FastEMA     int
	SlowEMA     int
	MaxTrendATR float64 // EMA separation above which we consider it a trend
}

// NewReversionAgent returns the agent with untuned round-number defaults.
func NewReversionAgent() *ReversionAgent {
	return &ReversionAgent{
		Lookback:    50,
		ATRPeriod:   20,
		MinZ:        2.0,
		MinWickFrac: 0.4,
		FastEMA:     20,
		SlowEMA:     100,
		MaxTrendATR: 1.0,
	}
}

func (a *ReversionAgent) Name() string   { return "reversion" }
func (a *ReversionAgent) Family() Family { return FamilyReversion }

func (a *ReversionAgent) Warmup() int {
	return maxInt(a.Lookback, maxInt(a.ATRPeriod+1, a.SlowEMA)) + 1
}

func (a *ReversionAgent) Evaluate(v View) Signal {
	if v.Len() < a.Warmup() {
		return Flatf(a.Name(), a.Family(), "warming up (%d/%d bars)", v.Len(), a.Warmup())
	}
	cs := v.Candles()
	last := cs[len(cs)-1]
	closes := v.Closes()

	atr := ATR(cs, a.ATRPeriod)
	if math.IsNaN(atr) || atr <= 0 {
		return Flatf(a.Name(), a.Family(), "ATR undefined")
	}

	// Regime gate first — cheapest way to not take the trade that kills you.
	fast, slow := EMA(closes, a.FastEMA), EMA(closes, a.SlowEMA)
	if math.IsNaN(fast) || math.IsNaN(slow) {
		return Flatf(a.Name(), a.Family(), "EMA undefined")
	}
	if sep := math.Abs(fast-slow) / atr; sep > a.MaxTrendATR {
		return Flatf(a.Name(), a.Family(), "trending regime (EMA separation %s ATR) — standing down", f2(sep))
	}

	z := ZScore(closes, a.Lookback)
	if math.IsNaN(z) {
		return Flatf(a.Name(), a.Family(), "dispersion undefined")
	}
	rng := last.Range()
	if rng <= 0 {
		return Flatf(a.Name(), a.Family(), "zero-range bar")
	}

	switch {
	case z <= -a.MinZ:
		wick := last.LowerWick() / rng
		if wick < a.MinWickFrac {
			return Flatf(a.Name(), a.Family(), "stretched down (z=%s) but no rejection wick", f2(z))
		}
		return Signal{
			Agent: a.Name(), Family: a.Family(), Dir: Long,
			// Conviction rises with how stretched price is, saturating at
			// twice the entry threshold.
			Strength: clamp((-z-a.MinZ)/a.MinZ, 0, 1),
			Note:     "capitulation low: z=" + f2(z) + ", lower wick " + pct(wick) + " of range",
		}
	case z >= a.MinZ:
		wick := last.UpperWick() / rng
		if wick < a.MinWickFrac {
			return Flatf(a.Name(), a.Family(), "stretched up (z=%s) but no rejection wick", f2(z))
		}
		return Signal{
			Agent: a.Name(), Family: a.Family(), Dir: Short,
			Strength: clamp((z-a.MinZ)/a.MinZ, 0, 1),
			Note:     "blow-off high: z=" + f2(z) + ", upper wick " + pct(wick) + " of range",
		}
	default:
		return Flatf(a.Name(), a.Family(), "within normal range (z=%s)", f2(z))
	}
}
