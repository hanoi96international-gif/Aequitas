package keeper

import (
	"testing"
	"time"
)

// Regression guard for the 2026-07-25 "es merged gar nix, wir drehen uns im
// Kreis" incident.
//
// Both secondaries detected their fork correctly, once a minute, and were held
// off healing by autoHealCooldown. Contabo2's own log, verbatim:
//
//	[AUTO-HEAL] ⏸ Divergence detected and actionable, but SUPPRESSED by the
//	30m0s cooldown for another 22m15s ... this node is on an isolated fork
//
// repeated at ascending heights 1852371 -> 1853751 — roughly 26 minutes of
// confirmed, settled, actionable fork with healing switched off. Contabo1 was
// 11 minutes into the same window having attached 0 of 5092 received blocks.
//
// The cooldown exists to protect a slow but EVENTUALLY SUCCESSFUL catch-up from
// being yanked into a fresh resync. A node that has merged nothing since its
// last resync is the opposite of that, and these tests pin the distinction.

func TestEffectiveAutoHealCooldown_FailedResyncGetsTheShortRetry(t *testing.T) {
	dag := &BlockDAG{}
	lastAt := time.Now().Unix() - 120
	// Not one peer block merged since the resync ran: it did not reattach us.
	dag.lastSuccessfulPeerSyncAt.Store(lastAt - 300)

	got, shortened := dag.effectiveAutoHealCooldown(lastAt)
	if !shortened || got != autoHealFailedResyncRetry {
		t.Fatalf("a resync that merged nothing must not buy a full %s of downtime, got %s (shortened=%v)",
			autoHealCooldown, got, shortened)
	}
}

func TestEffectiveAutoHealCooldown_ProgressSinceResyncKeepsFullCooldown(t *testing.T) {
	dag := &BlockDAG{}
	lastAt := time.Now().Unix() - 120
	// Peer blocks HAVE merged since the resync — this may well be the slow
	// catch-up the full cooldown exists to protect.
	dag.lastSuccessfulPeerSyncAt.Store(lastAt + 30)

	got, shortened := dag.effectiveAutoHealCooldown(lastAt)
	if shortened || got != autoHealCooldown {
		t.Fatalf("a node that is merging peer blocks must keep the full %s cooldown, got %s (shortened=%v)",
			autoHealCooldown, got, shortened)
	}
}

// Exactly equal timestamps mean the last merge was at or before the resync, so
// nothing has merged since. Boundary matters: a resync and a merge landing in
// the same second is not evidence of reattachment.
func TestEffectiveAutoHealCooldown_EqualTimestampsCountAsNoProgress(t *testing.T) {
	dag := &BlockDAG{}
	lastAt := time.Now().Unix() - 120
	dag.lastSuccessfulPeerSyncAt.Store(lastAt)

	if _, shortened := dag.effectiveAutoHealCooldown(lastAt); !shortened {
		t.Fatal("a merge no newer than the resync itself is not evidence that the resync worked")
	}
}

// A node that has never resynced hits the caller's own lastAt > 0 guard and
// never reaches a cooldown at all; this pins that the helper stays neutral
// there rather than reporting a shortened window for a fresh boot.
func TestEffectiveAutoHealCooldown_NeverResyncedIsNeutral(t *testing.T) {
	dag := &BlockDAG{}
	dag.lastSuccessfulPeerSyncAt.Store(0)

	got, shortened := dag.effectiveAutoHealCooldown(0)
	if shortened || got != autoHealCooldown {
		t.Fatalf("a node that never resynced must not be reported as a failed resync, got %s (shortened=%v)", got, shortened)
	}
}

// The shortened window must stay comfortably longer than a resync plus
// reattachment, so it corrects a stuck node without becoming a resync storm.
func TestAutoHealFailedResyncRetry_IsShorterButNotInstant(t *testing.T) {
	if autoHealFailedResyncRetry >= autoHealCooldown {
		t.Fatalf("the failed-resync retry must be shorter than the normal cooldown (%s vs %s)",
			autoHealFailedResyncRetry, autoHealCooldown)
	}
	if autoHealFailedResyncRetry < time.Minute {
		t.Fatalf("under a minute leaves no room for a resync to complete and reattach — got %s",
			autoHealFailedResyncRetry)
	}
}
