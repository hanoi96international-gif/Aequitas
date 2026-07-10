package keeper

import (
	"testing"
	"time"
)

// newIsolationTestDAG builds a minimal BlockDAG for exercising
// selfProducedFinalityAllowed/recordForeignMerge in isolation.
func newIsolationTestDAG() *BlockDAG {
	return &BlockDAG{
		authorizedValidators:    make(map[string]bool),
		selfProposer:            "0xself",
		lastSeenFromValidator:   make(map[string]int64),
		lastMergedFromValidator: make(map[string]int64),
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

// TestRecordForeignSeenAndMerge_RoundTrip is the baseline sanity check for
// the two new per-validator recorders: each must independently update the
// timestamp for exactly the address it was called with.
func TestRecordForeignSeenAndMerge_RoundTrip(t *testing.T) {
	dag := newIsolationTestDAG()
	dag.recordForeignSeen("0xPeer")
	dag.recordForeignMergeForProposer("0xPeer")
	dag.foreignValidatorActivityMu.Lock()
	seenAt, seen := dag.lastSeenFromValidator["0xpeer"]
	mergedAt, merged := dag.lastMergedFromValidator["0xpeer"]
	dag.foreignValidatorActivityMu.Unlock()
	if !seen || seenAt == 0 {
		t.Fatal("recordForeignSeen must record a non-zero timestamp under the lower-cased address")
	}
	if !merged || mergedAt == 0 {
		t.Fatal("recordForeignMergeForProposer must record a non-zero timestamp under the lower-cased address")
	}
}

// TestSelfProducedFinalityAllowed_SeenButNotMergedValidatorBlocksHardening is
// the core regression guard for the 2026-07-10 Primary/Contabo1 permanent-
// partial-merge incident: lastForeignMergeAt alone (dag-wide) stayed fresh —
// and hardening never paused — the entire time Primary was completely walled
// off from Contabo1, simply because it kept merging a DIFFERENT validator
// (cd20) fine. A validator we're still actively hearing FROM but can't merge
// must pause hardening even while some OTHER validator is merging cleanly.
func TestSelfProducedFinalityAllowed_SeenButNotMergedValidatorBlocksHardening(t *testing.T) {
	dag := newIsolationTestDAG()
	dag.authorizedValidators["0xself"] = true
	dag.authorizedValidators["0xhealthy"] = true
	dag.authorizedValidators["0xisolated"] = true
	dag.recordForeignMerge()                    // dag-wide: satisfied (matches cd20 merging fine)
	dag.recordForeignMergeForProposer("0xhealthy") // 0xhealthy is genuinely merging
	dag.recordForeignSeen("0xisolated")            // 0xisolated's blocks keep arriving...
	// ...but recordForeignMergeForProposer("0xisolated") is deliberately never called —
	// exactly the Contabo1 case: seen constantly, never successfully merged.
	if dag.selfProducedFinalityAllowed() {
		t.Fatal("a validator we're still hearing from but can't merge must pause hardening, even while a different validator merges cleanly")
	}
}

// TestSelfProducedFinalityAllowed_UnseenValidatorNeverBlocksHardening
// verifies the safety property that makes this fix sound: validator
// addresses are never de-registered (see AuthorizedValidatorList's own
// comment), so a validator that has simply gone quiet/retired (no recent
// lastSeenFromValidator entry at all) must NOT freeze checkpoint hardening
// for the whole network forever — only a validator we're demonstrably still
// hearing from counts.
func TestSelfProducedFinalityAllowed_UnseenValidatorNeverBlocksHardening(t *testing.T) {
	dag := newIsolationTestDAG()
	dag.authorizedValidators["0xself"] = true
	dag.authorizedValidators["0xhealthy"] = true
	dag.authorizedValidators["0xretired"] = true // known, but nothing has been seen or merged from it, ever
	dag.recordForeignMerge()
	dag.recordForeignMergeForProposer("0xhealthy")
	if !dag.selfProducedFinalityAllowed() {
		t.Fatal("a validator with no recent activity at all (never seen, never merged) must not block hardening — it's presumed gone quiet, not actively isolated")
	}
}

// TestSelfProducedFinalityAllowed_StaleSeenValidatorDoesNotBlock verifies a
// "seen" entry older than isolatedFinalityPauseWindow ages out the same way
// lastForeignMergeAt itself does — a validator we heard from once, long ago,
// must not permanently block hardening just because it was never re-seen.
func TestSelfProducedFinalityAllowed_StaleSeenValidatorDoesNotBlock(t *testing.T) {
	dag := newIsolationTestDAG()
	dag.authorizedValidators["0xself"] = true
	dag.authorizedValidators["0xhealthy"] = true
	dag.authorizedValidators["0xoncepeer"] = true
	dag.recordForeignMerge()
	dag.recordForeignMergeForProposer("0xhealthy")
	dag.foreignValidatorActivityMu.Lock()
	dag.lastSeenFromValidator["0xoncepeer"] = time.Now().Add(-2 * isolatedFinalityPauseWindow).Unix()
	dag.foreignValidatorActivityMu.Unlock()
	if !dag.selfProducedFinalityAllowed() {
		t.Fatal("a stale (out-of-window) seen entry with no merge must not block hardening — it must age out like lastForeignMergeAt does")
	}
}

// TestSelfProducedFinalityAllowed_SeenAndMergedValidatorAllowsHardening is
// the healthy multi-validator case: every known validator that's actively
// seen is also actively merged — hardening must proceed normally.
func TestSelfProducedFinalityAllowed_SeenAndMergedValidatorAllowsHardening(t *testing.T) {
	dag := newIsolationTestDAG()
	dag.authorizedValidators["0xself"] = true
	dag.authorizedValidators["0xpeerA"] = true
	dag.authorizedValidators["0xpeerB"] = true
	dag.recordForeignMerge()
	dag.recordForeignSeen("0xpeerA")
	dag.recordForeignMergeForProposer("0xpeerA")
	dag.recordForeignSeen("0xpeerB")
	dag.recordForeignMergeForProposer("0xpeerB")
	if !dag.selfProducedFinalityAllowed() {
		t.Fatal("every known validator being both seen and merged recently must allow hardening to proceed")
	}
}
