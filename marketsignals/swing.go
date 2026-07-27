package marketsignals

import "math"

// Swing is a confirmed local extreme in the price path — the raw material for
// every structural read: Fibonacci legs, necklines, trendlines, patterns.
//
// The Index/ConfirmedAt split is the whole reason this type exists rather
// than a bare float. A swing high at bar 100 is not knowable at bar 100. It
// becomes knowable only once enough later bars have closed WITHOUT exceeding
// it — at bar 100+Strength. Chart-pattern code that treats the swing as
// available at bar 100 is reading the future, and it is the single most
// common defect in technical-analysis backtests, because on a printed chart
// the swing is simply *there* and the delay is invisible.
type Swing struct {
	Index       int
	Price       float64
	High        bool // true for a swing high, false for a swing low
	ConfirmedAt int  // the first bar at which this swing could be known
}

// FindSwings locates fractal extremes that are CONFIRMED within cs.
//
// A swing high at i must strictly exceed the highs of the `strength` bars on
// each side. Requiring bars on the right is what creates the confirmation
// lag, and swings whose right-hand window extends past the end of cs are
// omitted entirely: they may still be invalidated by a bar that has not
// happened.
//
// Because cs is always a View's already-truncated slice, "past the end of cs"
// means "in the future", and this function therefore cannot return a swing
// the caller was not entitled to.
func FindSwings(cs []Candle, strength int) []Swing {
	if strength < 1 || len(cs) < 2*strength+1 {
		return nil
	}
	var out []Swing
	// Stop at len(cs)-strength: any later candidate lacks its full right-hand
	// confirmation window.
	for i := strength; i < len(cs)-strength; i++ {
		isHigh, isLow := true, true
		for j := i - strength; j <= i+strength; j++ {
			if j == i {
				continue
			}
			if cs[j].High >= cs[i].High {
				isHigh = false
			}
			if cs[j].Low <= cs[i].Low {
				isLow = false
			}
		}
		if isHigh {
			out = append(out, Swing{Index: i, Price: cs[i].High, High: true, ConfirmedAt: i + strength})
		}
		if isLow {
			out = append(out, Swing{Index: i, Price: cs[i].Low, High: false, ConfirmedAt: i + strength})
		}
	}
	return out
}

// Alternating collapses a swing list so highs and lows strictly alternate,
// keeping the most extreme swing of each run.
//
// Raw fractal detection happily reports three consecutive highs during a
// staircase advance. Every structural pattern below — legs, necklines,
// shoulders — assumes an alternating zig-zag, and feeding it consecutive
// same-side swings silently produces shapes that are not there.
func Alternating(swings []Swing) []Swing {
	var out []Swing
	for _, s := range swings {
		if len(out) == 0 {
			out = append(out, s)
			continue
		}
		last := &out[len(out)-1]
		if last.High != s.High {
			out = append(out, s)
			continue
		}
		// Same side as the previous swing: keep whichever is more extreme.
		if (s.High && s.Price > last.Price) || (!s.High && s.Price < last.Price) {
			*last = s
		}
	}
	return out
}

// SwingsAsOf returns the alternating swing sequence a view is entitled to at
// its final bar. This is the entry point every structural agent uses; going
// to FindSwings directly risks forgetting the alternation step.
func SwingsAsOf(v View, strength int) []Swing {
	return Alternating(FindSwings(v.Candles(), strength))
}

// Leg is a directional move between two consecutive confirmed swings — the
// impulse a retracement retraces.
type Leg struct {
	From Swing
	To   Swing
	Up   bool
}

// Size is the leg's absolute price extent.
func (l Leg) Size() float64 { return math.Abs(l.To.Price - l.From.Price) }

// Bars is the leg's duration in bars.
func (l Leg) Bars() int { return l.To.Index - l.From.Index }

// LastLeg returns the most recent completed impulse: the move between the two
// most recent alternating swings. Reports false when there are not yet two.
func LastLeg(swings []Swing) (Leg, bool) {
	if len(swings) < 2 {
		return Leg{}, false
	}
	from, to := swings[len(swings)-2], swings[len(swings)-1]
	if from.High == to.High {
		return Leg{}, false // caller skipped Alternating
	}
	return Leg{From: from, To: to, Up: to.Price > from.Price}, true
}

// PriorLevels returns the prices of swings before the given index, newest
// first — used to score confluence, where a level that several independent
// swings already respected carries more weight than a bare ratio.
func PriorLevels(swings []Swing, beforeIndex int) []float64 {
	var out []float64
	for i := len(swings) - 1; i >= 0; i-- {
		if swings[i].Index < beforeIndex {
			out = append(out, swings[i].Price)
		}
	}
	return out
}
