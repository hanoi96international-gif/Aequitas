package marketsignals

import (
	"math"
	"testing"
	"time"
)

// peakSeries builds a flat market with one pronounced high at peakIdx, so
// that swing confirmation timing can be checked against an exact bar.
func peakSeries(n, peakIdx int) *Series {
	s := &Series{Symbol: "PEAK", Interval: time.Hour}
	t := time.Unix(1_700_000_000, 0).UTC()
	for i := 0; i < n; i++ {
		high, low := 101.0, 99.0
		if i == peakIdx {
			high, low = 120.0, 99.5
		}
		s.Candles = append(s.Candles, Candle{
			Time: t.Add(time.Duration(i) * time.Hour),
			Open: 100, High: high, Low: low, Close: 100, Volume: 1,
		})
	}
	return s
}

// TestFindSwings_WithholdsUnconfirmedExtremes is the structural test for all
// pattern and Fibonacci work. A high at bar 10 is not knowable at bar 10: it
// is only a swing once `strength` later bars have closed without exceeding
// it. Code that reports it earlier is reading bars that have not happened,
// and on a printed chart that error is invisible because the peak is simply
// there.
func TestFindSwings_WithholdsUnconfirmedExtremes(t *testing.T) {
	const peak, strength = 10, 3
	s := peakSeries(40, peak)

	// One bar short of the full right-hand confirmation window.
	early := FindSwings(s.Candles[:peak+strength], strength)
	for _, sw := range early {
		if sw.Index == peak {
			t.Fatalf("the swing at bar %d was reported at bar %d, before its %d confirming "+
				"bars had closed", peak, peak+strength-1, strength)
		}
	}

	// Exactly enough history: it may now be reported.
	ready := FindSwings(s.Candles[:peak+strength+1], strength)
	found := false
	for _, sw := range ready {
		if sw.Index == peak && sw.High {
			found = true
			if sw.ConfirmedAt != peak+strength {
				t.Fatalf("ConfirmedAt = %d, want %d", sw.ConfirmedAt, peak+strength)
			}
			if sw.Price != 120 {
				t.Fatalf("swing price %v, want 120", sw.Price)
			}
		}
	}
	if !found {
		t.Fatalf("the swing at bar %d was not reported even with its full confirmation window", peak)
	}
}

func TestAlternating_CollapsesConsecutiveSameSideSwings(t *testing.T) {
	in := []Swing{
		{Index: 1, Price: 10, High: true},
		{Index: 3, Price: 14, High: true}, // more extreme; should win
		{Index: 5, Price: 4, High: false},
		{Index: 7, Price: 2, High: false}, // more extreme; should win
		{Index: 9, Price: 20, High: true},
	}
	got := Alternating(in)
	if len(got) != 3 {
		t.Fatalf("got %d alternating swings, want 3: %+v", len(got), got)
	}
	if got[0].Price != 14 || got[1].Price != 2 || got[2].Price != 20 {
		t.Fatalf("kept the wrong extremes: %+v", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i].High == got[i-1].High {
			t.Fatalf("swings %d and %d are on the same side", i-1, i)
		}
	}
}

func TestRetracements_Arithmetic(t *testing.T) {
	up := Leg{
		From: Swing{Index: 0, Price: 100, High: false},
		To:   Swing{Index: 10, Price: 200, High: true},
		Up:   true,
	}
	levels := Retracements(up)
	want := map[float64]float64{0.382: 161.8, 0.5: 150, 0.618: 138.2}
	for _, l := range levels {
		if w, ok := want[l.Ratio]; ok && math.Abs(l.Price-w) > 1e-9 {
			t.Fatalf("up leg %.3f retracement = %.4f, want %.4f", l.Ratio, l.Price, w)
		}
	}

	// A down leg retraces upward from its low by the same fractions.
	down := Leg{
		From: Swing{Index: 0, Price: 200, High: true},
		To:   Swing{Index: 10, Price: 100, High: false},
	}
	for _, l := range Retracements(down) {
		if l.Ratio == 0.5 && math.Abs(l.Price-150) > 1e-9 {
			t.Fatalf("down leg 0.5 retracement = %.4f, want 150", l.Price)
		}
		if l.Ratio == 0.618 && math.Abs(l.Price-161.8) > 1e-9 {
			t.Fatalf("down leg 0.618 retracement = %.4f, want 161.8", l.Price)
		}
	}

	if got := Extension(up, 1.618); math.Abs(got-261.8) > 1e-9 {
		t.Fatalf("1.618 extension = %.4f, want 261.8", got)
	}
}

func TestDetectPatterns_DoubleTopGeometry(t *testing.T) {
	sw := []Swing{
		{Index: 10, Price: 120, High: true, ConfirmedAt: 15},
		{Index: 20, Price: 100, High: false, ConfirmedAt: 25},
		{Index: 30, Price: 121, High: true, ConfirmedAt: 35}, // within 2% of the first
	}
	got := DetectPatterns(sw, 0.02)
	if len(got) != 1 || got[0].Kind != DoubleTop {
		t.Fatalf("got %+v, want exactly one double top", got)
	}
	p := got[0]
	if p.Bullish {
		t.Fatal("a double top is a bearish formation")
	}
	if p.Trigger != 100 {
		t.Fatalf("trigger %v, want the intervening low at 100", p.Trigger)
	}
	// Measured target projects the formation's height below the neckline.
	if math.Abs(p.Target-80) > 1e-9 {
		t.Fatalf("target %v, want 80 (neckline 100 less the 20-point height)", p.Target)
	}
	// The pattern cannot be known before its last swing was confirmed.
	if p.ReadyAt != 35 {
		t.Fatalf("ReadyAt %d, want 35 — the confirmation bar of the final swing", p.ReadyAt)
	}
}

func TestDetectPatterns_RejectsUnequalPeaks(t *testing.T) {
	sw := []Swing{
		{Index: 10, Price: 120, High: true, ConfirmedAt: 15},
		{Index: 20, Price: 100, High: false, ConfirmedAt: 25},
		{Index: 30, Price: 150, High: true, ConfirmedAt: 35}, // 25% higher: not a double top
	}
	for _, p := range DetectPatterns(sw, 0.02) {
		if p.Kind == DoubleTop {
			t.Fatalf("two peaks 25%% apart were called a double top: %+v", p)
		}
	}
}

func TestDetectPatterns_HeadAndShouldersNecklineSlopes(t *testing.T) {
	sw := []Swing{
		{Index: 0, Price: 110, High: true, ConfirmedAt: 5},
		{Index: 10, Price: 100, High: false, ConfirmedAt: 15},
		{Index: 20, Price: 130, High: true, ConfirmedAt: 25}, // head
		{Index: 30, Price: 104, High: false, ConfirmedAt: 35},
		{Index: 40, Price: 111, High: true, ConfirmedAt: 45}, // shoulder, ~level with the first
	}
	got := DetectPatterns(sw, 0.02)
	var hs *Pattern
	for i := range got {
		if got[i].Kind == HeadShoulders {
			hs = &got[i]
		}
	}
	if hs == nil {
		t.Fatalf("no head and shoulders detected in %+v", got)
	}
	// Neckline through (10,100) and (30,104): slope 0.2/bar, so at bar 40 it
	// sits at 100 + 0.2*30 = 106. A flattened neckline would put the trigger
	// at 100 or 104 and move every entry by several points.
	if math.Abs(hs.Trigger-106) > 1e-9 {
		t.Fatalf("trigger %.4f, want 106 from the sloping neckline", hs.Trigger)
	}
	if hs.Bullish {
		t.Fatal("head and shoulders is bearish")
	}
}

// breakoutOfDoubleBottom builds a market that forms a double bottom and then
// closes decisively through the neckline.
func doubleBottomSeries() *Series {
	s := &Series{Symbol: "DB", Interval: time.Hour}
	t := time.Unix(1_700_000_000, 0).UTC()
	add := func(high, low, close float64) {
		s.Candles = append(s.Candles, Candle{
			Time: t, Open: (high + low) / 2, High: high, Low: low, Close: close, Volume: 1,
		})
		t = t.Add(time.Hour)
	}
	for i := 0; i < 30; i++ { // establish history
		add(105, 95, 100)
	}
	for i := 0; i < 6; i++ {
		add(103, 97, 100)
	}
	add(101, 80, 85) // first low
	for i := 0; i < 8; i++ {
		add(112, 98, 110)
	}
	add(118, 105, 115) // the intervening high — the neckline
	for i := 0; i < 8; i++ {
		add(112, 98, 100)
	}
	add(101, 81, 86) // second low, within tolerance of the first
	for i := 0; i < 8; i++ {
		add(112, 95, 110)
	}
	for i := 0; i < 8; i++ { // break decisively above the neckline
		add(135, 115, 132)
	}
	return s
}

func TestPatternAgent_WaitsForTheBreak(t *testing.T) {
	s := doubleBottomSeries()
	a := NewPatternAgent()
	a.SwingStrength = 3

	// Somewhere in this series a formation exists before it is triggered. The
	// agent must never go long purely because the shape is complete: the
	// neckline is the test, and anticipating it discards the only objective
	// element the technique has.
	sawPending := false
	for n := a.Warmup(); n < len(s.Candles); n++ {
		sig := a.Evaluate(NewView(s, n))
		if sig.Dir == Flat && containsStr(sig.Note, "none broken") {
			sawPending = true
		}
		if sig.Dir != Flat {
			// Once it does fire, it must be on a bar that closed through the
			// trigger, which the note records.
			if !containsStr(sig.Note, "broken by") {
				t.Fatalf("agent took a %s without a recorded break: %s", sig.Dir, sig.Note)
			}
		}
	}
	if !sawPending {
		t.Fatal("the series never presented an untriggered formation; the test proves nothing")
	}
}

func TestFibonacciAgent_RequiresRejectionNotJustATouch(t *testing.T) {
	// A clean impulse up, then a pullback that closes INSIDE the golden zone:
	// price is there, but nothing has held yet.
	s := &Series{Symbol: "FIB", Interval: time.Hour}
	tm := time.Unix(1_700_000_000, 0).UTC()
	add := func(open, high, low, close float64) {
		s.Candles = append(s.Candles, Candle{
			Time: tm, Open: open, High: high, Low: low, Close: close, Volume: 1,
		})
		tm = tm.Add(time.Hour)
	}
	for i := 0; i < 40; i++ {
		if i == 38 {
			add(100, 101, 95, 100) // one clean swing low to anchor the leg
			continue
		}
		add(100, 101, 99, 100)
	}
	for i := 0; i < 10; i++ { // impulse from 100 to 200
		p := 100 + float64(i+1)*10
		add(p-10, p, p-11, p)
	}
	for i := 0; i < 6; i++ { // drift back into the 0.5-0.618 band (138-150)
		add(150, 152, 144, 145)
	}

	a := NewFibonacciAgent()
	a.SwingStrength = 3
	sig := a.Evaluate(NewView(s, len(s.Candles)))
	if sig.Dir != Flat {
		t.Fatalf("agent took a %s while price was still sitting inside the retracement band: %s",
			sig.Dir, sig.Note)
	}
	if !containsStr(sig.Note, "not yet rejected") {
		t.Fatalf("note %q should say the band was reached but has not held — a flat signal "+
			"for some unrelated reason would let this test pass while the rule is broken", sig.Note)
	}
}

func TestFibonacciAgent_AbandonsAnInvalidatedLeg(t *testing.T) {
	s := &Series{Symbol: "FIB", Interval: time.Hour}
	tm := time.Unix(1_700_000_000, 0).UTC()
	add := func(open, high, low, close float64) {
		s.Candles = append(s.Candles, Candle{
			Time: tm, Open: open, High: high, Low: low, Close: close, Volume: 1,
		})
		tm = tm.Add(time.Hour)
	}
	for i := 0; i < 40; i++ {
		if i == 38 {
			add(100, 101, 95, 100) // one clean swing low to anchor the leg
			continue
		}
		add(100, 101, 99, 100)
	}
	for i := 0; i < 10; i++ {
		p := 100 + float64(i+1)*10
		add(p-10, p, p-11, p)
	}
	// Retrace far past 0.786 (which sits at 121.4): the up leg is dead, and
	// holding a continuation long through it is how a small loss becomes the
	// whole position.
	for i := 0; i < 6; i++ {
		add(120, 121, 105, 108)
	}

	a := NewFibonacciAgent()
	a.SwingStrength = 3
	sig := a.Evaluate(NewView(s, len(s.Candles)))
	if sig.Dir == Long {
		t.Fatalf("agent stayed long past the invalidation level: %s", sig.Note)
	}
	if !containsStr(sig.Note, "invalidated") {
		t.Fatalf("note %q should record the invalidation", sig.Note)
	}
}

func containsStr(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
