package marketsignals

import (
	"fmt"
	"math"
)

// RiskManager turns conviction into size. This is the part that decides
// whether a set of mediocre signals survives, and it matters more than the
// signals do: a good edge sized wrong goes to zero, while a weak edge sized
// correctly merely underperforms.
//
// Three mechanisms, in the order they bind:
//
//  1. Volatility targeting. Size is set so the POSITION's expected volatility
//     equals TargetVol, not so the notional is constant. A fixed notional
//     means the position risks five times as much in a violent month as in a
//     quiet one, which is how a strategy's realised risk ends up being
//     decided by the market's mood rather than by its author.
//  2. Fractional Kelly. Even a correctly estimated edge should be sized well
//     below full Kelly, because the edge is estimated with error, and Kelly
//     is brutally asymmetric: overbetting past the optimum loses money in the
//     long run even when the edge is real.
//  3. Hard caps and a drawdown kill switch, which override both.
type RiskManager struct {
	TargetVol     float64 // annualised volatility target for the position, e.g. 0.20
	KellyFraction float64 // fraction of Kelly to actually bet, e.g. 0.25
	MaxLeverage   float64 // absolute cap on |position| as a multiple of equity
	VolLookback   int     // bars used to estimate realised volatility
	MaxDrawdown   float64 // equity drawdown from peak that forces flat, e.g. 0.20
	// MinVol floors the volatility estimate. Without it, a quiet stretch
	// makes the denominator tiny and the position enormous — right before the
	// move that ended the quiet stretch.
	MinVol float64
}

// NewRiskManager returns conservative defaults: a fifth of Kelly, 20%
// annualised target vol, no leverage above 1x, flat at a 20% drawdown.
func NewRiskManager() *RiskManager {
	return &RiskManager{
		TargetVol:     0.20,
		KellyFraction: 0.20,
		MaxLeverage:   1.0,
		VolLookback:   30,
		MaxDrawdown:   0.20,
		MinVol:        0.10,
	}
}

// Size converts a signal into a target position as a signed fraction of
// equity. drawdown is the current fractional decline from the equity peak.
func (r *RiskManager) Size(sig Signal, v View, drawdown float64) SizedSignal {
	out := SizedSignal{Signal: sig}

	if sig.Dir == Flat || sig.Strength <= 0 {
		out.Reason = "no directional signal"
		return out
	}

	// The kill switch outranks everything. A strategy in a deep drawdown is
	// either broken or in a regime it does not understand, and both cases
	// call for the same response.
	if drawdown >= r.MaxDrawdown {
		out.Reason = fmt.Sprintf("kill switch: drawdown %s at or past limit %s",
			pct(drawdown), pct(r.MaxDrawdown))
		return out
	}

	vol := RealizedVol(v.Closes(), r.VolLookback, v.BarsPerYear())
	if math.IsNaN(vol) {
		out.Reason = "volatility undefined — refusing to size blind"
		return out
	}
	vol = math.Max(vol, r.MinVol)

	// Vol targeting, scaled by conviction, then cut to a fraction of Kelly.
	size := (r.TargetVol / vol) * sig.Strength * r.KellyFraction
	size = math.Min(size, r.MaxLeverage)

	out.Target = size * float64(sig.Dir)
	out.Reason = fmt.Sprintf("vol %s vs target %s, strength %s, %s Kelly → %s of equity",
		pct(vol), pct(r.TargetVol), f2(sig.Strength), pct(r.KellyFraction), pct(math.Abs(out.Target)))
	return out
}
