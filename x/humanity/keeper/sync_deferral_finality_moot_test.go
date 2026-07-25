package keeper

import (
	"testing"
	"time"
)

// Regression guard for the 2026-07-25 night incident on Contabo1: a single
// deferred block whose height had fallen below the finality floor
// (finalized height - finalityHeightSlack) kept the node's clean-sync streak
// at zero FOREVER. The block sat in the orphan queue waiting on a parent that
// only existed in the DB (below the in-memory window), AddPeerBlock's
// below-boot/finality gates deliberately never store such a block in
// dag.blocks, and reconcileDeferrals' only notion of "resolved" was presence
// in dag.blocks — so a fully-synced, byte-identical node produced nothing for
// hours over one moot historical straggler. Finality has already passed such
// a block: isFinalityViolation would reject it outright if it re-arrived, so
// it must not be counted as fork evidence either.
func TestReconcileDeferrals_BelowFinalityFloorIsMoot(t *testing.T) {
	origSlack := finalityHeightSlack
	defer func() { finalityHeightSlack = origSlack }()
	finalityHeightSlack = 50

	dag := newDeferralTestDAG()
	cs := newTestState()
	dag.state = cs

	// Finalized checkpoint at 1000 → floor at 950.
	cs.finalizedMu.Lock()
	cs.finalizedHeightCache = 1000
	cs.finalizedBlueScoreCache = 1
	cs.finalizedCacheLoaded = true
	cs.finalizedMu.Unlock()

	// The stale straggler: queued as an orphan at height 100, far below the
	// floor. Its own hash is what doSyncOnce recorded in the deferral watch.
	dag.orphansMu.Lock()
	dag.orphans = map[string][]*Block{
		"0xmissing-parent": {{Hash: "0xmoot-block", Height: 100}},
	}
	dag.orphansMu.Unlock()

	dag.reconcileDeferrals("peerA", []string{"0xmoot-block"})
	dag.age("peerA", 10*time.Minute) // way past any grace window

	if got := dag.reconcileDeferrals("peerA", nil); got != 0 {
		t.Fatalf("a deferred block below the finality floor must be moot, not fork evidence: got %d stale — "+
			"this is exactly the one-stale-orphan state that silenced Contabo1's production for hours", got)
	}
	dag.deferredWatchMu.Lock()
	n := len(dag.deferredWatch["peerA"])
	dag.deferredWatchMu.Unlock()
	if n != 0 {
		t.Fatalf("a moot deferral must be dropped from the watch entirely, %d still tracked", n)
	}
}

// The floor must not excuse anything ABOVE it: a deferred block past the
// grace window at a height finality has NOT passed is still exactly the fork
// evidence reconcileDeferrals exists to catch.
func TestReconcileDeferrals_AboveFinalityFloorStillCounts(t *testing.T) {
	origSlack := finalityHeightSlack
	defer func() { finalityHeightSlack = origSlack }()
	finalityHeightSlack = 50

	dag := newDeferralTestDAG()
	cs := newTestState()
	dag.state = cs

	cs.finalizedMu.Lock()
	cs.finalizedHeightCache = 1000
	cs.finalizedBlueScoreCache = 1
	cs.finalizedCacheLoaded = true
	cs.finalizedMu.Unlock()

	// Height 990 is above the 950 floor — a genuinely open deferral.
	dag.orphansMu.Lock()
	dag.orphans = map[string][]*Block{
		"0xmissing-parent": {{Hash: "0xlive-block", Height: 990}},
	}
	dag.orphansMu.Unlock()

	dag.reconcileDeferrals("peerA", []string{"0xlive-block"})
	dag.age("peerA", 10*time.Minute)

	if got := dag.reconcileDeferrals("peerA", nil); got != 1 {
		t.Fatalf("an aged deferral above the finality floor must still count as fork evidence, got %d stale", got)
	}
}
