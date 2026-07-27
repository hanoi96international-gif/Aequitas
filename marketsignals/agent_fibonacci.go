package marketsignals

import (
	"fmt"
	"math"
)

// FibRatios are the retracement levels, and they are worth being honest
// about: there is no mechanism by which 0.618 has power over a market. What
// they have is attention. Enough participants place orders at these levels
// that liquidity genuinely clusters there, which makes them real as a
// coordination point and not as a law of nature.
//
// That distinction matters for how the agent is built. A self-fulfilling
// level works only while price ARRIVES at it in a state where the resting
// orders can absorb the flow — so this agent never trades a level on touch
// alone. It requires the level to hold, visibly, on a closed bar. If the
// crowd's bid is not there, the level was just a number.
var FibRatios = []float64{0.236, 0.382, 0.5, 0.618, 0.786}

// FibLevel is one retracement level of a leg.
type FibLevel struct {
	Ratio float64
	Price float64
}

// Retracements projects the standard ratios onto a leg. For an up leg the
// levels descend from the high; for a down leg they ascend from the low.
func Retracements(l Leg) []FibLevel {
	out := make([]FibLevel, 0, len(FibRatios))
	size := l.To.Price - l.From.Price // signed
	for _, r := range FibRatios {
		out = append(out, FibLevel{Ratio: r, Price: l.To.Price - size*r})
	}
	return out
}

// Extension projects a target beyond the leg's end — 1.618 of the leg
// measured from its origin.
func Extension(l Leg, ratio float64) float64 {
	return l.From.Price + (l.To.Price-l.From.Price)*ratio
}

// FibonacciAgent trades continuation from a retracement that HELD.
//
// The sequence it requires, in order, on closed bars only:
//
//  1. A confirmed impulse leg between two alternating swings, large enough to
//     matter (MinLegATR) — retracements of noise are noise.
//  2. Price trades into the retracement band (ZoneLow..ZoneHigh of the leg,
//     0.5–0.618 by default).
//  3. The most recent closed bar REJECTS the zone: it traded into the band
//     and closed back out of it, on the leg's side. A close still inside the
//     band is not evidence, it is a market that has not decided.
//  4. The 0.786 level has not been closed through. Past that, the "retracement"
//     is a reversal wearing the wrong name, and holding a continuation trade
//     through it is how a small loss becomes the whole position.
//
// Direction is always WITH the leg. Fibonacci is a continuation tool here,
// not a reversal tool; using the same levels to argue both ways is what makes
// the technique unfalsifiable in most hands.
type FibonacciAgent struct {
	SwingStrength int
	ATRPeriod     int
	MinLegATR     float64 // leg must span at least this many ATRs
	ZoneLow       float64 // near edge of the retracement band
	ZoneHigh      float64 // far edge
	Invalidation  float64 // ratio beyond which the leg is dead
	// ConfluenceATR is how close a prior swing must sit to the touched level
	// to count as confirming it, in ATRs.
	ConfluenceATR float64
	MaxLegAgeBars int // a leg older than this has stopped describing the market
}

// NewFibonacciAgent returns the agent with conventional, untuned settings.
func NewFibonacciAgent() *FibonacciAgent {
	return &FibonacciAgent{
		SwingStrength: 5,
		ATRPeriod:     20,
		MinLegATR:     3.0,
		ZoneLow:       0.5,
		ZoneHigh:      0.618,
		Invalidation:  0.786,
		ConfluenceATR: 0.5,
		MaxLegAgeBars: 120,
	}
}

func (a *FibonacciAgent) Name() string   { return "fibonacci" }
func (a *FibonacciAgent) Family() Family { return FamilyStructure }

func (a *FibonacciAgent) Warmup() int {
	return maxInt(a.ATRPeriod+1, 6*a.SwingStrength) + 1
}

func (a *FibonacciAgent) Evaluate(v View) Signal {
	if v.Len() < a.Warmup() {
		return Flatf(a.Name(), a.Family(), "warming up (%d/%d bars)", v.Len(), a.Warmup())
	}
	cs := v.Candles()
	last := cs[len(cs)-1]
	lastIdx := len(cs) - 1

	atr := ATR(cs, a.ATRPeriod)
	if math.IsNaN(atr) || atr <= 0 {
		return Flatf(a.Name(), a.Family(), "ATR undefined")
	}

	swings := SwingsAsOf(v, a.SwingStrength)
	leg, ok := LastLeg(swings)
	if !ok {
		return Flatf(a.Name(), a.Family(), "no confirmed impulse leg yet")
	}
	if leg.Size() < a.MinLegATR*atr {
		return Flatf(a.Name(), a.Family(), "last leg spans only %s ATR (need %s)",
			f2(leg.Size()/atr), f2(a.MinLegATR))
	}
	if age := lastIdx - leg.To.Index; age > a.MaxLegAgeBars {
		return Flatf(a.Name(), a.Family(), "leg is %d bars stale", age)
	}

	size := leg.To.Price - leg.From.Price // signed
	priceAt := func(ratio float64) float64 { return leg.To.Price - size*ratio }

	near, far := priceAt(a.ZoneLow), priceAt(a.ZoneHigh)
	invalid := priceAt(a.Invalidation)

	// Invalidation first: a dead leg must not be traded on any other reading.
	if leg.Up && last.Close < invalid {
		return Flatf(a.Name(), a.Family(), "closed below the %s retracement — leg invalidated",
			f2(a.Invalidation))
	}
	if !leg.Up && last.Close > invalid {
		return Flatf(a.Name(), a.Family(), "closed above the %s retracement — leg invalidated",
			f2(a.Invalidation))
	}

	zoneLo, zoneHi := math.Min(near, far), math.Max(near, far)
	touched := last.Low <= zoneHi && last.High >= zoneLo
	if !touched {
		return Flatf(a.Name(), a.Family(), "price is not at the %s–%s retracement band",
			f2(a.ZoneLow), f2(a.ZoneHigh))
	}

	// The rejection requirement: traded into the band, closed back out on the
	// leg's side.
	if leg.Up && last.Close <= zoneHi {
		return Flatf(a.Name(), a.Family(), "in the retracement band but not yet rejected from it")
	}
	if !leg.Up && last.Close >= zoneLo {
		return Flatf(a.Name(), a.Family(), "in the retracement band but not yet rejected from it")
	}

	dir := Long
	if !leg.Up {
		dir = Short
	}

	// Base conviction on how cleanly the bar rejected the zone, then add
	// confluence: a level that independent prior swings already respected is
	// more than an arithmetic ratio.
	rejection := math.Abs(last.Close-zoneHi) / atr
	if !leg.Up {
		rejection = math.Abs(zoneLo-last.Close) / atr
	}
	strength := clamp(rejection, 0, 0.7)

	confluence := 0
	for _, lvl := range PriorLevels(swings, leg.From.Index) {
		if math.Abs(lvl-((zoneLo+zoneHi)/2)) <= a.ConfluenceATR*atr {
			confluence++
		}
	}
	if confluence > 0 {
		strength = clamp(strength+0.15*float64(confluence), 0, 1)
	}

	note := fmt.Sprintf("%s leg of %s ATR rejected from its %s–%s band",
		map[bool]string{true: "up", false: "down"}[leg.Up],
		f2(leg.Size()/atr), f2(a.ZoneLow), f2(a.ZoneHigh))
	if confluence > 0 {
		note += fmt.Sprintf(" with %d prior swing(s) at the same level", confluence)
	}

	return Signal{Agent: a.Name(), Family: a.Family(), Dir: dir, Strength: strength, Note: note}
}
