package keeper

import (
	"testing"
	"time"
)

// Guards the 2026-07-26 fix for Contabo1 producing nothing over TWO stale
// deferrals.
//
// Measured live: Contabo1 at height 1939945, byte-identical state root with
// both peers, orphan_missing_parents = 0, and zero AddPeerBlock rejections in
// 4000 log lines — yet clean_sync_streak was 1059 against Contabo2 and exactly
// 0 against the primary, printing "2 deferred block(s) have now gone
// unresolved for longer than 48s" once a second and producing nothing.
//
// The two hashes were deferred during the unknown-proposer fork earlier that
// day and abandoned from the orphan queue when it healed. That left them in
// neither dag.blocks nor the orphan queue, and reconcileDeferrals had no path
// for that state: the presence test needs dag.blocks, and the 2026-07-25 moot
// test reads heights out of queuedOrphanHeights(), which only knows blocks
// still sitting in the queue. Both escape hatches shut at once, forever.

// newDeferralTestDAG builds only what reconcileDeferrals touches. state is nil
// on purpose so finalityFloor stays 0 and the moot-by-finality path is out of
// the way — this is about the "no longer queued" path.
func newStaleDeferralTestDAG() *BlockDAG {
	return &BlockDAG{
		blocks:        make(map[string]*Block),
		orphans:       make(map[string][]*Block),
		deferredWatch: make(map[string]map[string]int64),
	}
}

const deferralTestPeer = "https://aequitas.digital"

// residueWatchEntry backdates a hash past orphanAbandonAfter — the point at
// which this codebase already considers an orphan unresolvable.
func residueWatchEntry(dag *BlockDAG, peer, hash string) {
	if dag.deferredWatch[peer] == nil {
		dag.deferredWatch[peer] = make(map[string]int64)
	}
	dag.deferredWatch[peer][hash] = time.Now().Unix() - int64(orphanAbandonAfter.Seconds()) - 60
}

// TestReconcileDeferrals_AbandonedDeferralIsDropped is the regression guard.
// A hash that has left the orphan queue without ever reaching dag.blocks must
// stop counting: this node is no longer waiting for it and nothing will ever
// act on it again. Before the fix it counted as fork evidence forever and
// pinned the peer's clean-sync streak at zero.
func TestReconcileDeferrals_AbandonedDeferralIsDropped(t *testing.T) {
	dag := newStaleDeferralTestDAG()
	const abandoned = "aaaa000000000000000000000000000000000000000000000000000000000001"
	residueWatchEntry(dag, deferralTestPeer, abandoned)
	// Deliberately in NEITHER dag.blocks NOR dag.orphans — exactly Contabo1's
	// live state for those two hashes.

	if stale := dag.reconcileDeferrals(deferralTestPeer, nil); stale != 0 {
		t.Fatalf("an abandoned deferral must not count as fork evidence, got stale=%d", stale)
	}
	if _, still := dag.deferredWatch[deferralTestPeer][abandoned]; still {
		t.Fatal("an abandoned deferral must be removed from the watch list, or it will be re-counted on the very next cycle")
	}
}

// TestReconcileDeferrals_QueuedStaleDeferralStillCounts is the other half, and
// the more important one: the fix must not reintroduce the 2026-07-25
// regression where deferrals were forgiven outright and a forked node reached
// the clean-sync threshold on a peer it could merge nothing from. A deferral
// still sitting in the orphan queue, past the grace window, is exactly that
// evidence and must still count.
func TestReconcileDeferrals_QueuedStaleDeferralStillCounts(t *testing.T) {
	dag := newStaleDeferralTestDAG()
	const queued = "bbbb000000000000000000000000000000000000000000000000000000000002"
	residueWatchEntry(dag, deferralTestPeer, queued)
	dag.orphans["some-missing-parent"] = []*Block{{Hash: queued, Height: 100}}

	if stale := dag.reconcileDeferrals(deferralTestPeer, nil); stale != 1 {
		t.Fatalf("a deferral still queued and past the grace window is fork evidence and must count, got stale=%d", stale)
	}
	if _, still := dag.deferredWatch[deferralTestPeer][queued]; !still {
		t.Fatal("a still-queued deferral must stay on the watch list")
	}
}

// TestReconcileDeferrals_ResolvedDeferralIsDropped keeps the pre-existing
// contract explicit: a deferral that actually merged is resolved, whether or
// not it is still in the queue.
func TestReconcileDeferrals_ResolvedDeferralIsDropped(t *testing.T) {
	dag := newStaleDeferralTestDAG()
	const resolved = "cccc000000000000000000000000000000000000000000000000000000000003"
	residueWatchEntry(dag, deferralTestPeer, resolved)
	dag.blocks[resolved] = &Block{Hash: resolved, Height: 100}

	if stale := dag.reconcileDeferrals(deferralTestPeer, nil); stale != 0 {
		t.Fatalf("a deferral present in dag.blocks is resolved, got stale=%d", stale)
	}
	if _, still := dag.deferredWatch[deferralTestPeer][resolved]; still {
		t.Fatal("a resolved deferral must leave the watch list")
	}
}

// TestReconcileDeferrals_FreshDeferralIsNotDroppedByTheNewCheck guards the
// obvious way to get this wrong: a deferral recorded THIS cycle is queued as
// an orphan by construction (that is why IsWithinOrphanGrace was true), so the
// new "no longer queued" test must not fire on it. It is simply not stale yet.
func TestReconcileDeferrals_FreshDeferralIsNotDroppedByTheNewCheck(t *testing.T) {
	dag := newStaleDeferralTestDAG()
	const fresh = "dddd000000000000000000000000000000000000000000000000000000000004"
	dag.orphans["parent-in-flight"] = []*Block{{Hash: fresh, Height: 100}}

	if stale := dag.reconcileDeferrals(deferralTestPeer, []string{fresh}); stale != 0 {
		t.Fatalf("a deferral first seen this cycle is not yet past the grace window, got stale=%d", stale)
	}
	if _, tracked := dag.deferredWatch[deferralTestPeer][fresh]; !tracked {
		t.Fatal("a fresh deferral must be tracked so it can become fork evidence if it never resolves")
	}
}
