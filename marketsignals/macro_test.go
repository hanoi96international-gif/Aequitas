package marketsignals

import (
	"math"
	"testing"
	"time"
)

func ptr(f float64) *float64 { return &f }

// eventSeries builds a flat hourly market with a calendar attached, so that
// event timing can be reasoned about against exact bar boundaries.
func eventSeries(bars int, events ...Event) *Series {
	s := &Series{Symbol: "MACRO", Interval: time.Hour, Events: events}
	t := time.Unix(1_700_000_000, 0).UTC()
	for i := 0; i < bars; i++ {
		s.Candles = append(s.Candles, Candle{
			Time: t.Add(time.Duration(i) * time.Hour),
			Open: 100, High: 101, Low: 99, Close: 100, Volume: 1,
		})
	}
	return s
}

func seriesStart() time.Time { return time.Unix(1_700_000_000, 0).UTC() }

// TestView_NowIsTheBarsCloseNotItsOpen pins the timing convention every
// event comparison depends on. A view over n bars stands at the CLOSE of bar
// n-1; using its open time instead would hand the agent a full bar's worth of
// events it could not have seen, which for hourly bars is exactly the window
// that matters around a release.
func TestView_NowIsTheBarsCloseNotItsOpen(t *testing.T) {
	s := eventSeries(10)
	v := NewView(s, 5)
	wantOpen := seriesStart().Add(4 * time.Hour)
	if got := v.Last().Time; !got.Equal(wantOpen) {
		t.Fatalf("last bar opens at %s, want %s", got, wantOpen)
	}
	if got, want := v.Now(), wantOpen.Add(time.Hour); !got.Equal(want) {
		t.Fatalf("view Now() = %s, want the bar's close at %s", got, want)
	}
}

// TestMaskEvents_HidesOutcomesButNotDates encodes the distinction the whole
// macro agent rests on: a scheduled event's DATE is public knowledge months
// ahead, while its OUTCOME is not knowable until it lands.
func TestMaskEvents_HidesOutcomesButNotDates(t *testing.T) {
	now := seriesStart()
	future := Event{
		Time: now.Add(6 * time.Hour), Kind: KindCentralBank, Region: "US",
		Title: "FOMC decision", Importance: 3, Scheduled: true,
		Consensus: ptr(5.0), Actual: ptr(4.5), Surprise: ptr(0.8),
	}

	got := MaskEvents([]Event{future}, now)
	if len(got) != 1 {
		t.Fatalf("a scheduled future event must stay visible; got %d entries", len(got))
	}
	e := got[0]
	if e.Title != "FOMC decision" || e.Importance != 3 {
		t.Fatal("the event's date and identity should survive masking")
	}
	if e.Consensus == nil {
		t.Fatal("consensus is published in advance and must survive masking")
	}
	if e.Actual != nil {
		t.Fatalf("the ACTUAL outcome leaked before the event: %v", *e.Actual)
	}
	if e.Surprise != nil {
		t.Fatalf("the surprise score leaked before the event: %v", *e.Surprise)
	}

	// Once it has landed, everything is visible.
	after := MaskEvents([]Event{future}, future.Time)
	if after[0].Actual == nil || after[0].Surprise == nil {
		t.Fatal("a resolved event must expose its outcome")
	}
}

func TestMaskEvents_UnscheduledEventsAreInvisibleUntilTheyLand(t *testing.T) {
	now := seriesStart()
	shock := Event{
		Time: now.Add(2 * time.Hour), Kind: KindGeopolitics, Region: "EU",
		Title: "emergency sanctions package", Importance: 3, Scheduled: false,
		Surprise: ptr(-0.9),
	}

	if got := MaskEvents([]Event{shock}, now); len(got) != 0 {
		t.Fatalf("an unscheduled future event was visible: %+v — nobody had it on a "+
			"calendar, so no strategy could have positioned for it", got)
	}
	if got := MaskEvents([]Event{shock}, shock.Time); len(got) != 1 {
		t.Fatal("an unscheduled event must appear once it has happened")
	}
}

func TestMaskEvents_SortsAndDoesNotMutateTheOriginal(t *testing.T) {
	now := seriesStart().Add(100 * time.Hour)
	later := Event{Time: now.Add(-time.Hour), Title: "b", Importance: 3, Scheduled: true, Actual: ptr(1)}
	earlier := Event{Time: now.Add(-5 * time.Hour), Title: "a", Importance: 3, Scheduled: true, Actual: ptr(2)}
	in := []Event{later, earlier}

	got := MaskEvents(in, now)
	if got[0].Title != "a" || got[1].Title != "b" {
		t.Fatalf("events not sorted chronologically: %v, %v", got[0].Title, got[1].Title)
	}
	if in[0].Title != "b" {
		t.Fatal("MaskEvents reordered the caller's slice")
	}
}

// TestMaskEvents_DoesNotLeakThroughTheReturnedCopies guards the subtle version
// of the same bug: masking that nils out the pointer on a copy while the copy
// still shares the original's backing pointer would leave the outcome
// reachable.
func TestMaskEvents_DoesNotLeakThroughTheReturnedCopies(t *testing.T) {
	now := seriesStart()
	actual := 4.5
	future := Event{
		Time: now.Add(time.Hour), Title: "CPI", Importance: 3, Scheduled: true,
		Consensus: ptr(3.0), Actual: &actual,
	}
	got := MaskEvents([]Event{future}, now)
	if got[0].Actual != nil {
		t.Fatal("outcome reachable through the masked copy")
	}
	if future.Actual == nil || *future.Actual != 4.5 {
		t.Fatal("masking destroyed the caller's own data")
	}
}

func TestEvent_SurpriseScoreArithmetic(t *testing.T) {
	// A hot inflation print: actual above consensus, and for inflation an
	// upside surprise is risk-OFF, so RiskOnSign is -1.
	hot := Event{
		Kind: KindInflation, Consensus: ptr(3.0), Actual: ptr(3.1),
		TypicalMiss: 0.2, RiskOnSign: -1,
	}
	got, ok := hot.SurpriseScore()
	if !ok {
		t.Fatal("a release with consensus, actual and a sign must be scorable")
	}
	if math.Abs(got-(-0.5)) > 1e-9 {
		t.Fatalf("surprise %.4f, want -0.5 ((3.1-3.0)/0.2 * -1)", got)
	}

	// Extreme misses clamp rather than running off the scale.
	hot.Actual = ptr(4.0)
	if got, _ := hot.SurpriseScore(); got != -1 {
		t.Fatalf("a five-sigma miss scored %.4f, want it clamped to -1", got)
	}

	// A qualitative event carries a human judgement instead.
	ruling := Event{Kind: KindCourtRuling, Surprise: ptr(0.7)}
	if got, ok := ruling.SurpriseScore(); !ok || got != 0.7 {
		t.Fatalf("qualitative surprise = %.4f (ok=%v), want 0.7", got, ok)
	}

	// No consensus and no judgement: not scorable, and must say so rather
	// than defaulting to zero, which reads as "came in exactly as expected".
	if _, ok := (Event{Kind: KindElection}).SurpriseScore(); ok {
		t.Fatal("an event with no consensus and no judgement must not be scorable")
	}
}

func TestMacroAgent_CutsRiskAheadOfATopTierEvent(t *testing.T) {
	// The event lands at bar 20's close; the view sits at bar 12's close,
	// well inside a four-hour blackout.
	eventAt := seriesStart().Add(20 * time.Hour)
	s := eventSeries(40, Event{
		Time: eventAt, Kind: KindElection, Region: "US", Title: "general election",
		Importance: 3, Scheduled: true,
	})

	a := NewMacroAgent()
	adj := a.ModulateRisk(NewView(s, 18)) // now = start + 18h, event in 2h
	if adj.Scale != 0 || !adj.Veto {
		t.Fatalf("scale %.2f (veto=%v) two hours before a top-tier election — the outcome is "+
			"not forecastable and the gap risk is real", adj.Scale, adj.Veto)
	}

	// Well before the window, risk is untouched.
	if got := a.ModulateRisk(NewView(s, 5)); got.Scale != 1 {
		t.Fatalf("scale %.2f fifteen hours out; the blackout is four hours", got.Scale)
	}
}

func TestMacroAgent_ScaleStaysWithinBounds(t *testing.T) {
	eventAt := seriesStart().Add(20 * time.Hour)
	s := eventSeries(40, Event{
		Time: eventAt, Kind: KindCentralBank, Title: "rate decision",
		Importance: 3, Scheduled: true,
	})

	a := NewMacroAgent()
	for _, bad := range []float64{-5, 1.5, 42} {
		a.BlackoutScale = bad
		got := a.ModulateRisk(NewView(s, 18))
		if got.Scale < 0 || got.Scale > 1 {
			t.Fatalf("BlackoutScale %v produced Scale %v — a modulator may only ever remove "+
				"risk, never add it", bad, got.Scale)
		}
	}
}

func TestMacroAgent_RefusesTheUnreachableWindow(t *testing.T) {
	// Event at bar 20's close; view at bar 21's close is 1 hour later, but
	// the agent's own reaction delay is 30 minutes, so this is allowed —
	// while a delay longer than the elapsed time must silence it.
	eventAt := seriesStart().Add(20 * time.Hour)
	s := eventSeries(40, Event{
		Time: eventAt, Kind: KindInflation, Region: "US", Title: "CPI",
		Importance: 3, Scheduled: true,
		Consensus: ptr(3.0), Actual: ptr(4.0), TypicalMiss: 0.2, RiskOnSign: -1,
	})

	a := NewMacroAgent()
	a.ReactionDelay = 4 * time.Hour // pretend the unreachable window is longer
	sig := a.Evaluate(NewView(s, 21))
	if sig.Dir != Flat {
		t.Fatalf("agent took a %s inside the window it declares unreachable: %s", sig.Dir, sig.Note)
	}
	if !containsStr(sig.Note, "colocated") {
		t.Fatalf("note %q should say why the window is off limits", sig.Note)
	}
}

func TestMacroAgent_LeansAgainstAHotInflationPrint(t *testing.T) {
	eventAt := seriesStart().Add(20 * time.Hour)
	s := eventSeries(40, Event{
		Time: eventAt, Kind: KindInflation, Region: "US", Title: "CPI",
		Importance: 3, Scheduled: true,
		Consensus: ptr(3.0), Actual: ptr(4.0), TypicalMiss: 0.2, RiskOnSign: -1,
	})

	a := NewMacroAgent()
	// Two hours after the print: past both the blackout and the reaction delay.
	sig := a.Evaluate(NewView(s, 22))
	if sig.Dir != Short {
		t.Fatalf("agent = %s (%s); a hot inflation print is risk-off for a risk asset",
			sig.Dir, sig.Note)
	}

	// An inverse instrument flips the lean without changing the calendar.
	a.RiskBeta = -1
	if got := a.Evaluate(NewView(s, 22)); got.Dir != Long {
		t.Fatalf("with RiskBeta -1 the agent = %s, want long", got.Dir)
	}
}

func TestMacroAgent_WillNotInventASurprise(t *testing.T) {
	eventAt := seriesStart().Add(20 * time.Hour)
	s := eventSeries(40, Event{
		Time: eventAt, Kind: KindElection, Region: "US", Title: "general election",
		Importance: 3, Scheduled: true, // resolved, but nobody scored the outcome
	})

	sig := NewMacroAgent().Evaluate(NewView(s, 24))
	if sig.Dir != Flat {
		t.Fatalf("agent took a %s on an event with no scorable outcome: %s", sig.Dir, sig.Note)
	}
	if !containsStr(sig.Note, "refusing to invent") {
		t.Fatalf("note %q should record the refusal", sig.Note)
	}
}

func TestMacroAgent_StandsDownWithoutACalendar(t *testing.T) {
	s := eventSeries(40)
	a := NewMacroAgent()
	if got := a.Evaluate(NewView(s, 30)); got.Dir != Flat {
		t.Fatalf("agent took a %s with no calendar at all", got.Dir)
	}
	if got := a.ModulateRisk(NewView(s, 30)); got.Scale != 1 {
		t.Fatalf("scale %.2f with no calendar; absence of data is not a reason to shrink",
			got.Scale)
	}
}

// TestMacroAgent_CannotSeeAnUnscheduledShockComing is the end-to-end version
// of the masking rule: a geopolitical shock with a known outcome sitting in
// the series must not move the agent before it happens.
func TestMacroAgent_CannotSeeAnUnscheduledShockComing(t *testing.T) {
	shockAt := seriesStart().Add(30 * time.Hour)
	s := eventSeries(40, Event{
		Time: shockAt, Kind: KindGeopolitics, Region: "EU", Title: "surprise sanctions",
		Importance: 3, Scheduled: false, Surprise: ptr(-1),
	})
	// A second copy whose event is marked scheduled=false but which we hand to
	// the agent directly, to test the agent's own guard rather than the mask's.
	unmasked := eventSeries(40, Event{
		Time: shockAt, Kind: KindGeopolitics, Region: "EU", Title: "surprise sanctions",
		Importance: 3, Scheduled: false, Surprise: ptr(-1),
	})

	a := NewMacroAgent()
	for n := 1; n <= 30; n++ {
		v := NewView(s, n)
		// Strictly before: at the instant the shock lands it is legitimately
		// known, and reacting then is not foresight.
		if !v.Now().Before(shockAt) {
			continue
		}
		if sig := a.Evaluate(v); sig.Dir != Flat {
			t.Fatalf("agent took a %s at %s, before an unscheduled shock at %s",
				sig.Dir, v.Now(), shockAt)
		}
		if adj := a.ModulateRisk(v); adj.Scale != 1 {
			t.Fatalf("agent cut risk at %s ahead of an unscheduled shock — that is a "+
				"blackout it could not have known to take", v.Now())
		}
		// Same question asked of the raw calendar, bypassing the View's
		// masking: the agent itself must refuse a pre-event blackout for an
		// unscheduled event rather than relying on the mask to hide it.
		if adj := a.ModulateRisk(NewView(unmasked, n)); adj.Scale != 1 {
			t.Fatalf("with an unmasked calendar the agent blacked out at %s ahead of an "+
				"unscheduled shock", v.Now())
		}
	}
}

func TestEnsemble_RiskModulationTakesTheMinimum(t *testing.T) {
	e := NewEnsemble()
	e.Modulators = []RiskModulator{
		stubModulator{scale: 0.8, source: "a"},
		stubModulator{scale: 0.3, source: "b"},
		stubModulator{scale: 1.0, source: "c"},
	}
	got := e.ModulateRisk(NewView(eventSeries(10), 10))
	if got.Scale != 0.3 {
		t.Fatalf("combined scale %.2f, want the minimum 0.30 — two independent reasons to "+
			"be small are not one reason to be medium", got.Scale)
	}
	if got.Source != "b" {
		t.Fatalf("attributed to %q, want the binding modulator %q", got.Source, "b")
	}
}

type stubModulator struct {
	scale  float64
	source string
}

func (m stubModulator) ModulateRisk(View) RiskAdjustment {
	return RiskAdjustment{Scale: m.scale, Source: m.source, Reason: "stub"}
}

// TestBacktester_AppliesRiskModulation proves the blackout reaches the
// simulated position rather than merely being reported.
func TestBacktester_AppliesRiskModulation(t *testing.T) {
	s := constantGrowthSeries(80, 0.01)

	plain, err := NewBacktester().Run(s, alwaysLong{})
	if err != nil {
		t.Fatalf("plain run: %v", err)
	}
	damped, err := NewBacktester().Run(s, modulatedLong{scale: 0.25})
	if err != nil {
		t.Fatalf("modulated run: %v", err)
	}

	for i := range plain.Steps {
		want := plain.Steps[i].Position * 0.25
		if math.Abs(damped.Steps[i].Position-want) > 1e-12 {
			t.Fatalf("bar %d: position %.6f, want %.6f after a 0.25 modulation",
				plain.Steps[i].Index, damped.Steps[i].Position, want)
		}
	}
}

// modulatedLong is alwaysLong plus a constant risk cut.
type modulatedLong struct{ scale float64 }

func (modulatedLong) Name() string       { return "modulated-long" }
func (modulatedLong) Warmup() int        { return 2 }
func (modulatedLong) Decide(View) Signal { return Signal{Dir: Long, Strength: 1} }
func (m modulatedLong) ModulateRisk(View) RiskAdjustment {
	return RiskAdjustment{Scale: m.scale, Source: "test", Reason: "constant cut"}
}
