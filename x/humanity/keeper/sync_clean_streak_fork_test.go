package keeper

import (
	"testing"
	"time"
)

// Regression guard for the 2026-07-25 "nothing merges anywhere" incident.
//
// Both secondaries reached cleanSyncStreakThreshold and resumed producing
// while having merged literally nothing (foreign_attach count 0, ~1000
// blocks/cycle arriving, every one orphaned). They then finalized their own
// branch, which put the real common ancestor below their own finality floor —
// where isFinalityViolation rejects every peer block and lowerDeepScanFloor
// may not search. Unrecoverable without a full snapshot resync.
//
// The mechanism was the IsWithinOrphanGrace exemption in doSyncOnce: a
// deferred block did not reset the streak, and on a live chain EVERY newly
// produced peer block is younger than the grace window, so a permanently
// forked node's cycles looked "clean" forever.
//
// The rule that closes it is countUnresolvedDeferrals: a deferral is benign if
// and only if the block is actually IN THE DAG by the end of the cycle, after
// fetchMissingAncestors has had its chance. No timer, no grace window, no
// "did we merge anything" proxy — either the block is there or it is not.

func newDeferralTestDAG(present ...string) *BlockDAG {
	dag := &BlockDAG{blocks: make(map[string]*Block, len(present))}
	for _, h := range present {
		dag.blocks[h] = &Block{Hash: h}
	}
	return dag
}

// The incident itself: a forked node defers the peer's whole live chain,
// nothing resolves, and the cycle must NOT be scored as clean.
func TestUnresolvedDeferrals_ForkedNodeMustNotCountCleanCycle(t *testing.T) {
	dag := newDeferralTestDAG() // nothing merged
	deferred := []string{"0xa", "0xb", "0xc"}

	if got := dag.countUnresolvedDeferrals(deferred); got != len(deferred) {
		t.Fatalf("every deferred block is still missing, so all %d must count as unresolved, got %d — "+
			"anything less lets a permanently forked node climb to the clean-streak threshold", len(deferred), got)
	}
}

// The case the exemption exists for: an ordered paged catch-up splits a block
// from its parent across a page boundary, fetchMissingAncestors closes the gap
// within the same cycle, and the restart must still reach the threshold as
// fast as it did before any of this.
func TestUnresolvedDeferrals_ResolvedBacklogCountsClean(t *testing.T) {
	dag := newDeferralTestDAG("0xa", "0xb", "0xc")

	if got := dag.countUnresolvedDeferrals([]string{"0xa", "0xb", "0xc"}); got != 0 {
		t.Fatalf("all deferred blocks were merged by the end of the cycle, so none may count "+
			"as unresolved, got %d — otherwise every restart stalls production again", got)
	}
}

// A single unresolved block is enough. The old time-based guard had a
// `totalAdded > 0` early-out that scored the cycle clean whenever ANY block
// merged, so a node orphaning the peer's entire chain while landing one
// unrelated gap-fill still passed. It must not.
func TestUnresolvedDeferrals_PartialResolutionIsNotClean(t *testing.T) {
	dag := newDeferralTestDAG("0xa", "0xb") // 0xc never arrives

	if got := dag.countUnresolvedDeferrals([]string{"0xa", "0xb", "0xc"}); got != 1 {
		t.Fatalf("one deferred block is still missing and must count, got %d — progress on "+
			"other blocks is exactly the hole the previous time-based guard had", got)
	}
}

// Nothing deferred means there is no deferral evidence to judge (e.g. a
// genuinely idle peer with nothing new), which must be left alone.
func TestUnresolvedDeferrals_NoDeferralsIsAlwaysClean(t *testing.T) {
	dag := newDeferralTestDAG("0xa")

	if got := dag.countUnresolvedDeferrals(nil); got != 0 {
		t.Fatalf("a cycle with nothing deferred has nothing to judge, got %d", got)
	}
}

// A fresh process has an empty DAG map. The check must not panic or misreport
// on it — a booting node deferring its first page is the normal start of every
// catch-up, and it is correctly "unresolved" until the parents arrive.
func TestUnresolvedDeferrals_FreshDAGDoesNotPanic(t *testing.T) {
	dag := &BlockDAG{}

	if got := dag.countUnresolvedDeferrals([]string{"0xa"}); got != 1 {
		t.Fatalf("a block deferred against an empty DAG is unresolved, got %d", got)
	}
}

// The superseded guard is retained in the source purely as documentation of a
// fixed defect. This pins the defect so nobody reinstates it as a gate: it
// declares a forked cycle clean the moment anything at all merged.
func TestDeferralsAreNotResolving_RetainedOnlyAsDocumentedDefect(t *testing.T) {
	dag := &BlockDAG{}
	dag.lastSuccessfulPeerSyncAt.Store(time.Now().Unix() - int64(proposerBreakerOrphanGrace.Seconds()) - 60)

	if deferralsAreNotResolving(dag, 500, 1) {
		t.Fatal("the old guard is expected to return false here — that is precisely its defect, " +
			"and countUnresolvedDeferrals is what doSyncOnce must use instead")
	}
}
