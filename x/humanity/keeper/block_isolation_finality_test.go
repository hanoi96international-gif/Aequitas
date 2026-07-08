package keeper

import (
	"testing"
	"time"
)

// newIsolationTestDAG builds a minimal BlockDAG for exercising
// selfProducedFinalityAllowed/recordForeignMerge in isolation.
func newIsolationTestDAG() *BlockDAG {
	return &BlockDAG{
		authorizedValidators: make(map[string]bool),
		selfProposer:         "0xself",
	}
}

// TestSelfProducedFinalityAllowed_SoloNetworkAlwaysAllowed is the regression
// guard for the ORIGINAL 2026-07-03 fix this gate must never break: a node
// that knows of no other authorized validator (a genuinely solo network) must
// keep advancing its own finality checkpoint freely, exactly as before this
// gate existed — otherwise the "finalized_height stuck at 80094 through
// 50,000+ of its own self-produced blocks" bug reappears for anyone running
// alone.
func TestSelfProducedFinalityAllowed_SoloNetworkAlwaysAllowed(t *testing.T) {
	dag := newIsolationTestDAG()
	dag.authorizedValidators["0xself"] = true // only self known
	if !dag.selfProducedFinalityAllowed() {
		t.Fatal("a solo network (no other known validator) must always be allowed to advance its own checkpoint")
	}
}

// TestSelfProducedFinalityAllowed_NeverMergedWithKnownPeerIsPaused is the
// regression guard for the 2026-07-04 Contabo 2 permanent-isolation
// incident: a node that knows about another authorized validator but has
// NEVER successfully merged one of its blocks in must not hardcode its own
// isolated history as permanent — this is the exact state that let Contabo 2
// wall itself off from the real chain within under an hour of a fresh resync.
func TestSelfProducedFinalityAllowed_NeverMergedWithKnownPeerIsPaused(t *testing.T) {
	dag := newIsolationTestDAG()
	dag.authorizedValidators["0xself"] = true
	dag.authorizedValidators["0xpeer"] = true
	if dag.selfProducedFinalityAllowed() {
		t.Fatal("a node with a known peer but zero recorded foreign merges must pause self-hardening, not advance freely")
	}
}

// TestSelfProducedFinalityAllowed_RecentForeignMergeAllows verifies that once
// a real peer block has merged in recently, self-produced blocks are trusted
// to advance the checkpoint again — the healthy, actively-merging case.
func TestSelfProducedFinalityAllowed_RecentForeignMergeAllows(t *testing.T) {
	dag := newIsolationTestDAG()
	dag.authorizedValidators["0xself"] = true
	dag.authorizedValidators["0xpeer"] = true
	dag.recordForeignMerge()
	if !dag.selfProducedFinalityAllowed() {
		t.Fatal("a node that just merged a foreign validator's block must be allowed to advance its own checkpoint")
	}
}

// TestSelfProducedFinalityAllowed_StaleForeignMergeIsPaused verifies the
// pause reactivates once the last real merge falls outside
// isolatedFinalityPauseWindow — a node that WAS healthy but has since gone
// dark from its peers must stop hardening again, not coast forever on one
// old merge.
func TestSelfProducedFinalityAllowed_StaleForeignMergeIsPaused(t *testing.T) {
	dag := newIsolationTestDAG()
	dag.authorizedValidators["0xself"] = true
	dag.authorizedValidators["0xpeer"] = true
	dag.lastForeignMergeAt.Store(time.Now().Add(-2 * isolatedFinalityPauseWindow).Unix())
	if dag.selfProducedFinalityAllowed() {
		t.Fatal("a foreign merge older than isolatedFinalityPauseWindow must no longer justify advancing the checkpoint")
	}
}

// TestSelfProducedFinalityAllowed_ResumesImmediatelyAfterMerge verifies the
// gate is not a one-way trip: a paused node that successfully merges a real
// peer block again is unfrozen on the very next check, matching
// recordForeignMerge's call site in AddPeerBlock's commit path.
func TestSelfProducedFinalityAllowed_ResumesImmediatelyAfterMerge(t *testing.T) {
	dag := newIsolationTestDAG()
	dag.authorizedValidators["0xself"] = true
	dag.authorizedValidators["0xpeer"] = true
	dag.lastForeignMergeAt.Store(time.Now().Add(-2 * isolatedFinalityPauseWindow).Unix())
	if dag.selfProducedFinalityAllowed() {
		t.Fatal("precondition failed: expected paused before the fresh merge")
	}
	dag.recordForeignMerge()
	if !dag.selfProducedFinalityAllowed() {
		t.Fatal("a fresh foreign merge must immediately resume self-hardening, not stay paused")
	}
}

// TestIsIsolatedFromPeers_NoRecentMergeWithKnownPeer is the regression guard
// for the 2026-07-08 incident: distributionSyncHealthIssue must refuse to
// run the daily distribution on a node that's isolated (self-only
// producing), even though its own peer-sync polling can keep succeeding.
// IsIsolatedFromPeers is main.go's entry point for that check — this proves
// it correctly reports "isolated" for the exact state that caused two
// nodes to independently win TryLockDistribution's race that day.
func TestIsIsolatedFromPeers_NoRecentMergeWithKnownPeer(t *testing.T) {
	dag := newIsolationTestDAG()
	dag.authorizedValidators["0xself"] = true
	dag.authorizedValidators["0xpeer"] = true
	if !dag.IsIsolatedFromPeers() {
		t.Fatal("a node with a known peer but zero recorded foreign merges must report isolated")
	}
}

// TestIsIsolatedFromPeers_RecentMergeNotIsolated verifies the healthy case:
// a node that just merged a real peer block must not be flagged isolated,
// so distribution proceeds normally on any actively-merging node.
func TestIsIsolatedFromPeers_RecentMergeNotIsolated(t *testing.T) {
	dag := newIsolationTestDAG()
	dag.authorizedValidators["0xself"] = true
	dag.authorizedValidators["0xpeer"] = true
	dag.recordForeignMerge()
	if dag.IsIsolatedFromPeers() {
		t.Fatal("a node that just merged a foreign validator's block must not report isolated")
	}
}

// TestIsIsolatedFromPeers_SoloNetworkNeverIsolated verifies a genuinely solo
// network (no other known validator) is never flagged isolated — matching
// selfProducedFinalityAllowed's own solo-network exemption, so a single-node
// deployment's distribution is never blocked by this check.
func TestIsIsolatedFromPeers_SoloNetworkNeverIsolated(t *testing.T) {
	dag := newIsolationTestDAG()
	dag.authorizedValidators["0xself"] = true
	if dag.IsIsolatedFromPeers() {
		t.Fatal("a solo network (no other known validator) must never report isolated")
	}
}
