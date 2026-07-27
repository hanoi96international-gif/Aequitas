package marketsignals

import "math"

// PatternKind names a chart formation.
type PatternKind string

const (
	DoubleTop         PatternKind = "double top"
	DoubleBottom      PatternKind = "double bottom"
	HeadShoulders     PatternKind = "head and shoulders"
	InverseHeadShldrs PatternKind = "inverse head and shoulders"
	AscendingTriangle PatternKind = "ascending triangle"
	DescTriangle      PatternKind = "descending triangle"
)

// Pattern is a formation detected from CONFIRMED swings only.
//
// Trigger is the level whose break activates the pattern, and it is fixed at
// detection time — before the break. That ordering is the entire defence
// against the way pattern trading usually deceives: on a historical chart one
// can always find the shape that explains the move that already happened, and
// place the neckline wherever it makes the trade look good. Here the geometry
// is settled by swings that were all confirmed earlier, and the break either
// happens against that level or it does not.
type Pattern struct {
	Kind    PatternKind
	Bullish bool
	// Trigger is the price that must be closed through to activate.
	Trigger float64
	// Target is the measured objective — the formation's height projected
	// from the trigger. It is a convention, not a forecast.
	Target float64
	// Swings are the points defining the shape, oldest first.
	Swings []Swing
	// ReadyAt is the bar from which the pattern is fully known: the
	// confirmation bar of its last swing. A break before this instant is not
	// tradeable, because the shape was not yet visible.
	ReadyAt int
}

// Height is the formation's extent, used for the measured target.
func (p Pattern) Height() float64 { return math.Abs(p.Target - p.Trigger) }

// DetectPatterns scans an alternating swing sequence for formations.
//
// tol is the relative tolerance for calling two levels "equal" (0.02 = 2%),
// which is what makes a double top a double top rather than two unrelated
// highs. Every returned pattern's ReadyAt is derived from its swings'
// ConfirmedAt, never from the bar the caller happens to be standing on.
func DetectPatterns(swings []Swing, tol float64) []Pattern {
	var out []Pattern
	out = append(out, detectDoubles(swings, tol)...)
	out = append(out, detectHeadShoulders(swings, tol)...)
	out = append(out, detectTriangles(swings, tol)...)
	return out
}

func near(a, b, tol float64) bool {
	if a == 0 {
		return b == 0
	}
	return math.Abs(a-b)/math.Abs(a) <= tol
}

func readyAt(ss ...Swing) int {
	r := 0
	for _, s := range ss {
		r = maxInt(r, s.ConfirmedAt)
	}
	return r
}

// detectDoubles finds two same-side extremes at a comparable level, separated
// by one opposing swing that becomes the neckline.
func detectDoubles(sw []Swing, tol float64) []Pattern {
	var out []Pattern
	for i := 0; i+2 < len(sw); i++ {
		a, mid, b := sw[i], sw[i+1], sw[i+2]
		if a.High != b.High || a.High == mid.High {
			continue
		}
		if !near(a.Price, b.Price, tol) {
			continue
		}
		height := math.Abs(a.Price - mid.Price)
		if height <= 0 {
			continue
		}
		if a.High {
			out = append(out, Pattern{
				Kind: DoubleTop, Bullish: false,
				Trigger: mid.Price, Target: mid.Price - height,
				Swings: []Swing{a, mid, b}, ReadyAt: readyAt(a, mid, b),
			})
		} else {
			out = append(out, Pattern{
				Kind: DoubleBottom, Bullish: true,
				Trigger: mid.Price, Target: mid.Price + height,
				Swings: []Swing{a, mid, b}, ReadyAt: readyAt(a, mid, b),
			})
		}
	}
	return out
}

// detectHeadShoulders finds five alternating swings whose middle same-side
// extreme exceeds its two neighbours, with those neighbours at a comparable
// level. The neckline is the LINE through the two opposing swings, evaluated
// where it matters rather than flattened to a single price — a sloping
// neckline is the normal case and treating it as horizontal moves the trigger
// by a meaningful amount in exactly the trades that are marginal.
func detectHeadShoulders(sw []Swing, tol float64) []Pattern {
	var out []Pattern
	for i := 0; i+4 < len(sw); i++ {
		s1, t1, head, t2, s2 := sw[i], sw[i+1], sw[i+2], sw[i+3], sw[i+4]
		if s1.High != head.High || head.High != s2.High || s1.High == t1.High {
			continue
		}
		if !near(s1.Price, s2.Price, tol) {
			continue
		}
		if head.High {
			if !(head.Price > s1.Price && head.Price > s2.Price) {
				continue
			}
		} else {
			if !(head.Price < s1.Price && head.Price < s2.Price) {
				continue
			}
		}

		neckAtHead := lineAt(t1, t2, head.Index)
		neckAtEnd := lineAt(t1, t2, s2.Index)
		height := math.Abs(head.Price - neckAtHead)
		if height <= 0 {
			continue
		}

		p := Pattern{
			Trigger: neckAtEnd,
			Swings:  []Swing{s1, t1, head, t2, s2},
			ReadyAt: readyAt(s1, t1, head, t2, s2),
		}
		if head.High {
			p.Kind, p.Bullish, p.Target = HeadShoulders, false, neckAtEnd-height
		} else {
			p.Kind, p.Bullish, p.Target = InverseHeadShldrs, true, neckAtEnd+height
		}
		out = append(out, p)
	}
	return out
}

// lineAt evaluates the straight line through two swings at a bar index.
func lineAt(a, b Swing, index int) float64 {
	if b.Index == a.Index {
		return (a.Price + b.Price) / 2
	}
	slope := (b.Price - a.Price) / float64(b.Index-a.Index)
	return a.Price + slope*float64(index-a.Index)
}

// detectTriangles finds a flat boundary on one side with a converging
// boundary on the other: ascending (flat highs, rising lows) and descending
// (flat lows, falling highs). The flat side is the trigger, because that is
// the level the market has repeatedly failed to clear and where the resting
// orders sit.
func detectTriangles(sw []Swing, tol float64) []Pattern {
	var out []Pattern
	for i := 0; i+3 < len(sw); i++ {
		w := sw[i : i+4]
		var highs, lows []Swing
		for _, s := range w {
			if s.High {
				highs = append(highs, s)
			} else {
				lows = append(lows, s)
			}
		}
		if len(highs) < 2 || len(lows) < 2 {
			continue
		}
		h1, h2 := highs[len(highs)-2], highs[len(highs)-1]
		l1, l2 := lows[len(lows)-2], lows[len(lows)-1]

		flatHighs := near(h1.Price, h2.Price, tol)
		flatLows := near(l1.Price, l2.Price, tol)
		risingLows := l2.Price > l1.Price && !flatLows
		fallingHighs := h2.Price < h1.Price && !flatHighs

		switch {
		case flatHighs && risingLows:
			height := h2.Price - l1.Price
			out = append(out, Pattern{
				Kind: AscendingTriangle, Bullish: true,
				Trigger: math.Max(h1.Price, h2.Price),
				Target:  math.Max(h1.Price, h2.Price) + height,
				Swings:  append([]Swing(nil), w...), ReadyAt: readyAt(w...),
			})
		case flatLows && fallingHighs:
			height := h1.Price - l2.Price
			out = append(out, Pattern{
				Kind: DescTriangle, Bullish: false,
				Trigger: math.Min(l1.Price, l2.Price),
				Target:  math.Min(l1.Price, l2.Price) - height,
				Swings:  append([]Swing(nil), w...), ReadyAt: readyAt(w...),
			})
		}
	}
	return out
}
