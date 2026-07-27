package marketsignals

import (
	"fmt"
	"sort"
	"time"
)

// EventKind classifies a political or macroeconomic event.
type EventKind string

const (
	KindElection    EventKind = "election"
	KindCentralBank EventKind = "central_bank"
	KindInflation   EventKind = "inflation"
	KindEmployment  EventKind = "employment"
	KindRegulatory  EventKind = "regulatory"
	KindCourtRuling EventKind = "court_ruling"
	KindGeopolitics EventKind = "geopolitics"
	KindTrade       EventKind = "trade"
)

// Event is one dated item on the political/macro calendar.
//
// The single most important distinction in this struct is between an event's
// DATE and its OUTCOME. A general election, a rate decision or a CPI release
// is on the calendar months ahead; knowing that it is coming is not
// foresight, it is a newspaper. Knowing what it will say is foresight, and
// the View masks it — see MaskEvents. Conflating the two is how event-driven
// backtests come to show returns that were never available.
type Event struct {
	// Time is when the event lands (for a release, its publication instant).
	Time time.Time `json:"time"`
	Kind EventKind `json:"kind"`
	// Region is the economy or jurisdiction, e.g. "US", "EU", "CN".
	Region string `json:"region"`
	Title  string `json:"title"`

	// Importance is 1 (minor) to 3 (the market stops to watch). Only level-3
	// events trigger a risk blackout.
	Importance int `json:"importance"`

	// Scheduled marks an event whose DATE was publicly known in advance. A
	// scheduled event appears in the calendar before it happens, without its
	// outcome. An unscheduled one — an emergency cut, a coup, a surprise
	// indictment — appears only once it has landed, because that is the only
	// point at which anyone knew of it.
	Scheduled bool `json:"scheduled"`

	// Consensus and Actual describe quantitative releases. Both nil means the
	// event is qualitative and Surprise carries the judgement instead.
	Consensus *float64 `json:"consensus,omitempty"`
	Actual    *float64 `json:"actual,omitempty"`
	// TypicalMiss is the ordinary absolute distance between consensus and
	// actual for this release, used to scale a surprise into comparable
	// units. Zero falls back to the consensus's own magnitude.
	TypicalMiss float64 `json:"typical_miss,omitempty"`

	// RiskOnSign says which way a HIGHER-than-expected actual pushes risk
	// appetite: +1 when an upside surprise is risk-on (a strong employment
	// print), -1 when it is risk-off (a hot inflation print that implies
	// tighter policy). This lives in the data rather than in a lookup table
	// in code, because the mapping genuinely differs by regime and whoever
	// assembles the calendar is the one who should own that judgement.
	RiskOnSign int `json:"risk_on_sign,omitempty"`

	// Surprise is set directly for qualitative events — an election result, a
	// court ruling, a sanctions package — on a scale where +1 is maximally
	// risk-on and -1 maximally risk-off. It is a human judgement and is
	// recorded as such rather than dressed up as a measurement.
	Surprise *float64 `json:"surprise,omitempty"`
}

// Resolved reports whether the event has landed as of now.
func (e Event) Resolved(now time.Time) bool { return !e.Time.After(now) }

// SurpriseScore returns the event's risk-appetite surprise on [-1, 1], and
// whether one could be computed at all. Positive is risk-on.
func (e Event) SurpriseScore() (float64, bool) {
	if e.Surprise != nil {
		return clamp(*e.Surprise, -1, 1), true
	}
	if e.Consensus == nil || e.Actual == nil || e.RiskOnSign == 0 {
		return 0, false
	}
	scale := e.TypicalMiss
	if scale <= 0 {
		// Fall back to a tenth of the consensus level: a crude but honest
		// stand-in that at least keeps the units sane. A calendar that cares
		// about this number should supply TypicalMiss.
		scale = absf(*e.Consensus) * 0.1
	}
	if scale <= 0 {
		return 0, false
	}
	miss := (*e.Actual - *e.Consensus) / scale
	return clamp(miss*float64(sign(e.RiskOnSign)), -1, 1), true
}

// Validate rejects calendar entries that would quietly corrupt a backtest.
func (e Event) Validate() error {
	if e.Time.IsZero() {
		return fmt.Errorf("event %q has no time", e.Title)
	}
	if e.Importance < 1 || e.Importance > 3 {
		return fmt.Errorf("event %q: importance %d outside 1..3", e.Title, e.Importance)
	}
	if e.RiskOnSign != 0 && e.RiskOnSign != 1 && e.RiskOnSign != -1 {
		return fmt.Errorf("event %q: RiskOnSign %d must be -1, 0 or +1", e.Title, e.RiskOnSign)
	}
	if e.Surprise != nil && (*e.Surprise < -1 || *e.Surprise > 1) {
		return fmt.Errorf("event %q: surprise %g outside [-1,1]", e.Title, *e.Surprise)
	}
	return nil
}

// MaskEvents returns the calendar as of now, enforcing the one rule that
// makes an event-driven backtest meaningful:
//
//   - A scheduled event is visible before it happens, but ONLY its date,
//     kind, region and importance. Consensus survives (it is published in
//     advance); Actual and Surprise do not exist yet and are stripped.
//   - An unscheduled event is invisible until it has landed. Nobody had it on
//     a calendar, so no strategy could have positioned for it.
//
// The returned events are copies; the caller cannot reach the unmasked
// originals through them.
func MaskEvents(events []Event, now time.Time) []Event {
	out := make([]Event, 0, len(events))
	for _, e := range events {
		resolved := e.Resolved(now)
		if !resolved {
			if !e.Scheduled {
				continue // nobody knew this was coming
			}
			// Known date, unknown outcome.
			e.Actual = nil
			e.Surprise = nil
		}
		out = append(out, e)
	}
	// Callers assume chronological order; a calendar assembled from several
	// sources rarely arrives that way.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out
}

// NextScheduled returns the earliest event at or after now with at least the
// given importance, and whether one exists.
func NextScheduled(events []Event, now time.Time, minImportance int) (Event, bool) {
	for _, e := range events {
		if e.Importance >= minImportance && e.Time.After(now) {
			return e, true
		}
	}
	return Event{}, false
}

// MostRecentResolved returns the latest event at or before now with at least
// the given importance, and whether one exists.
func MostRecentResolved(events []Event, now time.Time, minImportance int) (Event, bool) {
	var best Event
	found := false
	for _, e := range events {
		if e.Importance >= minImportance && e.Resolved(now) {
			if !found || e.Time.After(best.Time) {
				best, found = e, true
			}
		}
	}
	return best, found
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func sign(i int) int {
	if i < 0 {
		return -1
	}
	if i > 0 {
		return 1
	}
	return 0
}
