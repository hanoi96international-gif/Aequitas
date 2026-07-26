package marketsignals

import "math"

// BreakoutAgent trades the one crypto effect with the longest out-of-sample
// track record outside of academia: sustained directional moves continue more
// often than a random walk says they should, because leverage forces
// liquidation cascades and passive flows chase.
//
// Three conditions must hold together, and each one exists to kill a
// different failure mode of naive breakout trading:
//
//   - Price closes beyond the Donchian channel of the PRECEDING bars, so the
//     bar being tested cannot define the level it breaks.
//   - The distance beyond the channel is at least MinATRBreak ATRs. In chop,
//     price nicks the channel edge constantly; without this filter a breakout
//     system pays fees to be whipsawed and calls it a strategy.
//   - The fast/slow EMA pair agrees with the direction. A breakout against
//     the prevailing trend is usually the second leg of a range, not a
//     beginning.
type BreakoutAgent struct {
	Channel     int     // Donchian lookback, in bars
	ATRPeriod   int     // ATR lookback, in bars
	MinATRBreak float64 // how far beyond the channel counts as a real break
	FastEMA     int
	SlowEMA     int
}

// NewBreakoutAgent returns the agent with defaults that are deliberately
// round numbers rather than tuned ones. Anything tuned on the same data it is
// evaluated on is a curve fit; the honest workflow is to start here and let
// walk-forward evaluation say whether it survives.
func NewBreakoutAgent() *BreakoutAgent {
	return &BreakoutAgent{
		Channel:     55,
		ATRPeriod:   20,
		MinATRBreak: 0.25,
		FastEMA:     20,
		SlowEMA:     100,
	}
}

func (a *BreakoutAgent) Name() string   { return "breakout" }
func (a *BreakoutAgent) Family() Family { return FamilyTrend }

func (a *BreakoutAgent) Warmup() int {
	return maxInt(a.Channel+1, maxInt(a.ATRPeriod+1, a.SlowEMA)) + 1
}

func (a *BreakoutAgent) Evaluate(v View) Signal {
	if v.Len() < a.Warmup() {
		return Flatf(a.Name(), a.Family(), "warming up (%d/%d bars)", v.Len(), a.Warmup())
	}
	cs := v.Candles()
	last := cs[len(cs)-1]

	chHigh, chLow := Donchian(cs, a.Channel)
	atr := ATR(cs, a.ATRPeriod)
	if math.IsNaN(chHigh) || math.IsNaN(atr) || atr <= 0 {
		return Flatf(a.Name(), a.Family(), "channel or ATR undefined")
	}

	closes := v.Closes()
	fast, slow := EMA(closes, a.FastEMA), EMA(closes, a.SlowEMA)
	if math.IsNaN(fast) || math.IsNaN(slow) {
		return Flatf(a.Name(), a.Family(), "EMA undefined")
	}

	// Breakout distance measured in ATRs, so "a real break" means the same
	// thing whether the asset moves 2% or 20% on a normal day.
	upBreak := (last.Close - chHigh) / atr
	downBreak := (chLow - last.Close) / atr

	switch {
	case upBreak >= a.MinATRBreak && fast > slow:
		return Signal{
			Agent: a.Name(), Family: a.Family(), Dir: Long,
			// Saturating at 2 ATRs: beyond that the move is more likely to be
			// a blow-off than extra conviction, so more distance stops buying
			// more size.
			Strength: clamp(upBreak/2, 0, 1),
			Note:     "closed above channel by " + f2(upBreak) + " ATR with trend",
		}
	case downBreak >= a.MinATRBreak && fast < slow:
		return Signal{
			Agent: a.Name(), Family: a.Family(), Dir: Short,
			Strength: clamp(downBreak/2, 0, 1),
			Note:     "closed below channel by " + f2(downBreak) + " ATR with trend",
		}
	case upBreak > 0 || downBreak > 0:
		return Flatf(a.Name(), a.Family(), "channel touched but break too small or against trend")
	default:
		return Flatf(a.Name(), a.Family(), "inside channel")
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
