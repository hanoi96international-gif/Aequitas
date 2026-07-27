package marketsignals

import (
	"fmt"
	"math"
)

// PatternAgent trades formations, and it does exactly one thing that most
// pattern trading does not: it waits for the break.
//
// A double top is not a signal. It is a hypothesis, and the market tests it
// at the neckline. Anticipating the break — shorting the second peak because
// the shape "looks complete" — converts a defined trigger into a guess, and
// gives up the only objective element the technique has.
//
// So the agent requires, on closed bars only:
//
//   - A pattern whose every swing was confirmed at or before ReadyAt.
//   - The current bar CLOSING through the trigger by at least MinBreakATR
//     ATRs. An intrabar poke through a neckline is the most common shape of a
//     failed break, and taking it converts a filter into a coin flip.
//   - The break happening within BreakWindow bars of the pattern becoming
//     ready. A neckline broken forty bars later is a different market.
type PatternAgent struct {
	SwingStrength int
	ATRPeriod     int
	Tolerance     float64 // relative tolerance for "same level", e.g. 0.02
	MinBreakATR   float64
	BreakWindow   int
	// MinHeightATR filters out formations too small to be worth their costs:
	// a double top three ticks tall is chart noise with a name.
	MinHeightATR float64
}

// NewPatternAgent returns the agent with conventional, untuned settings.
func NewPatternAgent() *PatternAgent {
	return &PatternAgent{
		SwingStrength: 5,
		ATRPeriod:     20,
		Tolerance:     0.02,
		MinBreakATR:   0.25,
		BreakWindow:   30,
		MinHeightATR:  1.5,
	}
}

func (a *PatternAgent) Name() string   { return "pattern" }
func (a *PatternAgent) Family() Family { return FamilyStructure }

func (a *PatternAgent) Warmup() int {
	// Five alternating swings need roughly ten fractal windows of history.
	return maxInt(a.ATRPeriod+1, 10*a.SwingStrength) + 1
}

func (a *PatternAgent) Evaluate(v View) Signal {
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
	patterns := DetectPatterns(swings, a.Tolerance)
	if len(patterns) == 0 {
		return Flatf(a.Name(), a.Family(), "no formation in the confirmed swing sequence")
	}

	best := Signal{Agent: a.Name(), Family: a.Family()}
	pending := 0

	for _, p := range patterns {
		// The pattern must have been visible before the bar that breaks it.
		if p.ReadyAt >= lastIdx {
			continue
		}
		if lastIdx-p.ReadyAt > a.BreakWindow {
			continue
		}
		if p.Height() < a.MinHeightATR*atr {
			continue
		}
		pending++

		var beyond float64
		if p.Bullish {
			beyond = (last.Close - p.Trigger) / atr
		} else {
			beyond = (p.Trigger - last.Close) / atr
		}
		if beyond < a.MinBreakATR {
			continue
		}

		dir := Long
		if !p.Bullish {
			dir = Short
		}
		strength := clamp(beyond/(2*a.MinBreakATR), 0, 1)
		if strength <= best.Strength && best.Dir != Flat {
			continue
		}
		best.Dir = dir
		best.Strength = strength
		best.Note = fmt.Sprintf("%s broken by %s ATR (trigger %.6f, measured target %.6f)",
			p.Kind, f2(beyond), p.Trigger, p.Target)
	}

	if best.Dir == Flat {
		if pending > 0 {
			return Flatf(a.Name(), a.Family(),
				"%d formation(s) in play but none broken by %s ATR — a neckline is not a signal until it gives way",
				pending, f2(a.MinBreakATR))
		}
		return Flatf(a.Name(), a.Family(), "formations present but stale, too small, or not yet visible")
	}
	return best
}
