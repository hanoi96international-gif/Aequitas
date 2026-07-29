package keeper

import "sync/atomic"

// Why block production stays gated.
//
// Contabo2 reached its peers (lag 1 block) and still produced nothing: the
// gate fired 160 times in three minutes. cleanSyncStreak has to reach three
// CONSECUTIVE clean cycles with EVERY trusted seed, and exactly two things
// reset it — but which one is doing it here has been inferred from logs three
// times and guessed wrong twice, so this counts them instead.
//
// The two are not interchangeable:
//
//   - sawUnmergedBlocks means a block failed for a reason other than a parent
//     still in flight: bad signature, unauthorized proposer, finality
//     violation, far-ahead fork. An immediate reset is correct there and
//     nothing should soften it.
//   - unresolvedDeferrals means blocks whose parents did not arrive within the
//     grace window. That is the ambiguous one — a backlog and a fork both look
//     like this, which is what isShrinkingBacklog exists to separate.
//
// escapes counts how often that separation actually fired. A gate that stays
// shut while escapes stays zero means the escape is unreachable; a gate that
// stays shut while escapes climbs means the escape works and something else
// holds the streak down.
var syncStreakStats struct {
	resetsUnmerged  atomic.Int64
	resetsDeferrals atomic.Int64
	resetsBoth      atomic.Int64
	cleanCycles     atomic.Int64
	backlogEscapes  atomic.Int64
	gateSkips       atomic.Int64
	agedOrphansSeen atomic.Int64
}

// noteStreakOutcome records one doSyncOnce verdict.
// agedOrphans is the count this cycle routed away from the immediate-reset
// path by PR #108 — orphans whose parent had been missing longer than the
// grace window. It is reported separately because it is the quantity that
// change was supposed to move: if it stays zero on a node that still will not
// merge, the fix is not engaging there and something else is wrong.
func noteStreakOutcome(sawUnmerged bool, unresolved int, escaped bool, agedOrphans int) {
	if agedOrphans > 0 {
		syncStreakStats.agedOrphansSeen.Add(int64(agedOrphans))
	}
	if escaped {
		syncStreakStats.backlogEscapes.Add(1)
	}
	switch {
	case sawUnmerged && unresolved > 0:
		syncStreakStats.resetsBoth.Add(1)
	case sawUnmerged:
		syncStreakStats.resetsUnmerged.Add(1)
	case unresolved > 0 && !escaped:
		syncStreakStats.resetsDeferrals.Add(1)
	default:
		syncStreakStats.cleanCycles.Add(1)
	}
}

// noteGateSkip records one refusal to produce.
func noteGateSkip() { syncStreakStats.gateSkips.Add(1) }

// SyncStreakStats reports what is holding the production gate shut.
func SyncStreakStats() map[string]interface{} {
	return map[string]interface{}{
		"clean_cycles":     syncStreakStats.cleanCycles.Load(),
		"resets_unmerged":  syncStreakStats.resetsUnmerged.Load(),
		"resets_deferrals": syncStreakStats.resetsDeferrals.Load(),
		"resets_both":      syncStreakStats.resetsBoth.Load(),
		"backlog_escapes":  syncStreakStats.backlogEscapes.Load(),
		"gate_skips":       syncStreakStats.gateSkips.Load(),
	}
}
