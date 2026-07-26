package marketsignals

import "math"

// FlowAgent looks for the disagreement between price and the trades that
// produced it. When price grinds to a new low but cumulative taker delta does
// not confirm — sellers are no longer the ones pushing — the move is running
// on thin liquidity rather than fresh supply, and it tends not to hold.
//
// This is the only agent here that can be genuinely early rather than
// confirming, which is also why it is the easiest to fool yourself with. Two
// guards:
//
//   - It refuses to run without a real taker split. Reconstructing buy/sell
//     volume from whether a candle closed green is not order flow; it is the
//     price series wearing a disguise, and an agent built on it silently
//     duplicates whatever the price agents already said.
//   - It requires the divergence to be visible over a full lookback window,
//     not between two adjacent bars, where it is mostly noise.
type FlowAgent struct {
	Lookback     int     // window in which to compare price and delta extremes
	MinFlowBars  float64 // fraction of the window that must carry real flow data
	MinDeltaGapZ float64 // how far delta must fail to confirm, in delta sigmas
	ATRPeriod    int
}

// NewFlowAgent returns the agent with untuned round-number defaults.
func NewFlowAgent() *FlowAgent {
	return &FlowAgent{
		Lookback:     30,
		MinFlowBars:  0.8,
		MinDeltaGapZ: 1.0,
		ATRPeriod:    20,
	}
}

func (a *FlowAgent) Name() string   { return "flow" }
func (a *FlowAgent) Family() Family { return FamilyFlow }

func (a *FlowAgent) Warmup() int { return maxInt(a.Lookback, a.ATRPeriod+1) + 1 }

func (a *FlowAgent) Evaluate(v View) Signal {
	if v.Len() < a.Warmup() {
		return Flatf(a.Name(), a.Family(), "warming up (%d/%d bars)", v.Len(), a.Warmup())
	}
	cs := v.Candles()
	win := cs[len(cs)-a.Lookback:]

	// Refuse to operate on a feed that does not actually report taker sides.
	withFlow := 0
	for _, c := range win {
		if c.HasFlow() {
			withFlow++
		}
	}
	if frac := float64(withFlow) / float64(len(win)); frac < a.MinFlowBars {
		return Flatf(a.Name(), a.Family(),
			"no taker-side data (%s of window) — this agent does not guess flow from candles", pct(frac))
	}

	delta := CumulativeDelta(win)
	deltaSteps := make([]float64, 0, len(win))
	for _, c := range win {
		deltaSteps = append(deltaSteps, c.BuyVolume-c.SellVolume)
	}
	dsd := StdDev(deltaSteps, len(deltaSteps))
	if math.IsNaN(dsd) || dsd <= 0 {
		return Flatf(a.Name(), a.Family(), "flat delta distribution")
	}

	lows := make([]float64, len(win))
	highs := make([]float64, len(win))
	for i, c := range win {
		lows[i], highs[i] = c.Low, c.High
	}

	lastIdx := len(win) - 1

	// Bullish divergence: price prints the window's low on the final bar,
	// while cumulative delta sits meaningfully above its own low for the
	// window — the new price low was NOT made by new selling.
	if ArgMin(lows) == lastIdx {
		gap := (delta[lastIdx] - delta[ArgMin(delta)]) / dsd
		if gap >= a.MinDeltaGapZ {
			return Signal{
				Agent: a.Name(), Family: a.Family(), Dir: Long,
				Strength: clamp(gap/(2*a.MinDeltaGapZ), 0, 1),
				Note:     "new price low unconfirmed by delta (+" + f2(gap) + " sigma of absorption)",
			}
		}
		return Flatf(a.Name(), a.Family(), "new low, delta confirms it (%s sigma)", f2(gap))
	}

	// Bearish divergence: new price high that buyers did not pay for.
	if ArgMax(highs) == lastIdx {
		gap := (delta[ArgMax(delta)] - delta[lastIdx]) / dsd
		if gap >= a.MinDeltaGapZ {
			return Signal{
				Agent: a.Name(), Family: a.Family(), Dir: Short,
				Strength: clamp(gap/(2*a.MinDeltaGapZ), 0, 1),
				Note:     "new price high unconfirmed by delta (-" + f2(gap) + " sigma of distribution)",
			}
		}
		return Flatf(a.Name(), a.Family(), "new high, delta confirms it (%s sigma)", f2(gap))
	}

	return Flatf(a.Name(), a.Family(), "no price extreme to diverge from")
}
