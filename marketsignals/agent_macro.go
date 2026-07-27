package marketsignals

import (
	"fmt"
	"time"
)

// MacroAgent is the political and macroeconomic calendar's voice, and it is
// deliberately the most modest agent here.
//
// Two claims it does NOT make:
//
//   - It does not predict outcomes. Whether a central bank cuts, whether a
//     party wins, how a court rules — these are not forecastable from price
//     history, and a system that bets on them is not trading, it is gambling
//     with extra steps. What the agent does with a pending binary event is
//     get out of its way.
//   - It does not trade the instant of release. The first move after a number
//     prints belongs to systems measuring their latency in microseconds, sited
//     next to the matching engine. Any backtest that claims those seconds is
//     describing a trade that was never available. ReactionDelay makes that
//     refusal explicit rather than accidental.
//
// What is left is defensible and boring: shrink risk into scheduled binary
// events, and afterwards lean with the SURPRISE — the gap between what was
// expected and what arrived — over the following hours, which is where
// post-announcement drift lives if it lives anywhere.
type MacroAgent struct {
	// MinImportance is the calendar level that matters (3 = the market stops
	// to watch).
	MinImportance int

	// BlackoutBefore is how long before a qualifying event risk is cut. The
	// asymmetry with BlackoutAfter is intentional: the run-up is long because
	// positioning drains liquidity well ahead of the print, while the
	// aftermath resolves fast.
	BlackoutBefore time.Duration
	BlackoutAfter  time.Duration
	// BlackoutScale is the position multiplier inside the window. Zero means
	// fully flat.
	BlackoutScale float64

	// ReactionDelay is how long after the event the agent will begin to act.
	// It exists to make the unreachable first minutes unreachable in the
	// backtest too.
	ReactionDelay time.Duration
	// ReactionWindow is how long the drift claim remains open.
	ReactionWindow time.Duration

	// MinSurprise is the absolute surprise score required to lean at all.
	MinSurprise float64

	// RiskBeta maps risk appetite onto this instrument: +1 for a risk asset
	// (crypto, equities), -1 for something that rallies when risk comes off
	// (a safe-haven or an inverse product).
	RiskBeta float64
}

// NewMacroAgent returns the agent with conservative defaults: flat for four
// hours ahead of a top-tier event, silent for the first half hour after it,
// then a one-day drift window.
func NewMacroAgent() *MacroAgent {
	return &MacroAgent{
		MinImportance:  3,
		BlackoutBefore: 4 * time.Hour,
		BlackoutAfter:  30 * time.Minute,
		BlackoutScale:  0,
		ReactionDelay:  30 * time.Minute,
		ReactionWindow: 24 * time.Hour,
		MinSurprise:    0.35,
		RiskBeta:       1,
	}
}

func (a *MacroAgent) Name() string   { return "macro" }
func (a *MacroAgent) Family() Family { return FamilyMacro }

// Warmup is one bar: the calendar does not need price history to be
// meaningful. The backtester still applies its own volatility warmup.
func (a *MacroAgent) Warmup() int { return 1 }

// ModulateRisk implements the agent's primary job. It never raises risk —
// Scale is clamped into [0,1] — because nothing on a calendar is a reason to
// bet bigger, only a reason to bet less.
func (a *MacroAgent) ModulateRisk(v View) RiskAdjustment {
	if v.Len() == 0 {
		return NoAdjustment(a.Name(), "no bars")
	}
	now := v.Now()
	events := v.Events()
	if len(events) == 0 {
		return NoAdjustment(a.Name(), "no calendar for this market")
	}

	for _, e := range events {
		if e.Importance < a.MinImportance {
			continue
		}
		// The window straddles the event: risk comes off before it and stays
		// off while the immediate dust settles.
		//
		// The pre-event half applies to SCHEDULED events only. You cannot
		// reduce risk ahead of something nobody knew was coming, and an
		// unscheduled event that granted a blackout beforehand would be
		// hindsight wearing the costume of prudence. The View's masking
		// already hides such events until they land, so this is belt and
		// braces — but a caller passing an unmasked calendar straight to this
		// method deserves the same answer, not a silently different one.
		from := e.Time
		if e.Scheduled {
			from = e.Time.Add(-a.BlackoutBefore)
		}
		to := e.Time.Add(a.BlackoutAfter)
		if now.Before(from) || now.After(to) {
			continue
		}

		scale := clamp(a.BlackoutScale, 0, 1)
		when := "ahead of"
		if !now.Before(e.Time) {
			when = "immediately after"
		}
		return RiskAdjustment{
			Scale:  scale,
			Veto:   scale == 0,
			Source: a.Name(),
			Reason: fmt.Sprintf("%s %s (%s, %s) — the outcome is not forecastable and the gap risk is",
				when, e.Title, e.Region, e.Kind),
		}
	}
	return NoAdjustment(a.Name(), "no top-tier event in the blackout window")
}

// Evaluate leans with a realised surprise, once the unreachable window has
// passed.
func (a *MacroAgent) Evaluate(v View) Signal {
	if v.Len() == 0 {
		return Flatf(a.Name(), a.Family(), "no bars")
	}
	now := v.Now()
	events := v.Events()
	if len(events) == 0 {
		return Flatf(a.Name(), a.Family(), "series carries no political or macro calendar")
	}

	e, ok := MostRecentResolved(events, now, a.MinImportance)
	if !ok {
		return Flatf(a.Name(), a.Family(), "no resolved top-tier event yet")
	}

	since := now.Sub(e.Time)
	if since < a.ReactionDelay {
		return Flatf(a.Name(), a.Family(),
			"%s landed %s ago — inside the window that belongs to colocated systems",
			e.Title, since.Round(time.Minute))
	}
	if since > a.ReactionWindow {
		return Flatf(a.Name(), a.Family(), "last top-tier event was %s ago — priced in",
			since.Round(time.Hour))
	}

	surprise, ok := e.SurpriseScore()
	if !ok {
		return Flatf(a.Name(), a.Family(),
			"%s resolved but carries no scorable surprise — refusing to invent one", e.Title)
	}
	if absf(surprise) < a.MinSurprise {
		return Flatf(a.Name(), a.Family(), "%s came in near consensus (surprise %s)",
			e.Title, f2(surprise))
	}

	lean := surprise * a.RiskBeta
	dir := Long
	if lean < 0 {
		dir = Short
	}

	// Conviction decays across the drift window: the information is most
	// valuable when freshest and is worth nothing by the far edge.
	freshness := 1 - float64(since-a.ReactionDelay)/float64(a.ReactionWindow-a.ReactionDelay)
	strength := clamp(absf(lean)*clamp(freshness, 0, 1), 0, 1)

	return Signal{
		Agent: a.Name(), Family: a.Family(), Dir: dir, Strength: strength,
		Note: fmt.Sprintf("%s (%s) surprised %s vs consensus %s ago",
			e.Title, e.Region, f2(surprise), since.Round(time.Minute)),
	}
}
