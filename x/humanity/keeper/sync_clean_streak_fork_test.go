package keeper

import (
	"testing"
	"time"
)

// Regression guards for BOTH halves of the 2026-07-25 incident, which was
// caused twice in one evening by the same gate judged two different wrong ways.
//
// Half one — too lenient. The IsWithinOrphanGrace exemption forgave a deferred
// block outright, so both secondaries reached cleanSyncStreakThreshold having
// merged literally nothing (foreign_attach 0, ~1000 blocks/cycle arriving,
// every one orphaned), resumed producing on their own branch, finalized it, and
// put the real common ancestor below their own finality floor.
//
// Half two — too strict. The first fix asked whether the blocks deferred THIS
// cycle were in the DAG by the end of THIS cycle. On a live chain that is
// always false and never means a fork: the newest blocks arrive by push while
// their parents are still on the wire. Contabo1, 16 blocks off the primary and
// merging normally, printed "21 of 21 deferred block(s) still not in the DAG"
// and "Not yet 3 consecutive clean sync cycles" once a second, and neither
// secondary produced a block at all.
//
// What separates the two cases is TIME, not the cycle boundary — which is what
// reconcileDeferrals measures.

func newDeferralTestDAG(present ...string) *BlockDAG {
	dag := &BlockDAG{blocks: make(map[string]*Block, len(present))}
	for _, h := range present {
		dag.blocks[h] = &Block{Hash: h}
	}
	return dag
}

// age makes a peer's tracked deferrals look older than they are, standing in
// for cycles that have already elapsed.
func (dag *BlockDAG) age(nodeURL string, by time.Duration) {
	dag.deferredWatchMu.Lock()
	for h, seen := range dag.deferredWatch[nodeURL] {
		dag.deferredWatch[nodeURL][h] = seen - int64(by.Seconds())
	}
	dag.deferredWatchMu.Unlock()
}

// Half two: a block deferred right now is a parent in flight, not a fork. This
// is the case that deadlocked production, and it must score clean.
func TestReconcileDeferrals_FreshDeferralIsNotYetEvidence(t *testing.T) {
	dag := newDeferralTestDAG()

	if got := dag.reconcileDeferrals("peerA", []string{"0xa", "0xb"}); got != 0 {
		t.Fatalf("a deferral seen for the first time this cycle must not count, got %d — "+
			"at BLOCK_TIME=1s every cycle ends with tip blocks whose parents are still "+
			"on the wire, so counting them means the streak can never reach the threshold", got)
	}
}

// Half one: a deferral that outlives the grace is exactly the forked state —
// a parent on a branch this node will never receive.
func TestReconcileDeferrals_DeferralOutlivingTheGraceCounts(t *testing.T) {
	dag := newDeferralTestDAG()
	dag.reconcileDeferrals("peerA", []string{"0xa", "0xb"})
	dag.age("peerA", proposerBreakerOrphanGrace+30*time.Second)

	if got := dag.reconcileDeferrals("peerA", nil); got != 2 {
		t.Fatalf("both deferrals have outlived %s without arriving and must count as a fork, got %d",
			proposerBreakerOrphanGrace, got)
	}
}

// The ordinary healthy path: the parent lands a moment later, so the block is
// in the DAG by the next cycle and stops being tracked at all.
func TestReconcileDeferrals_ResolvedDeferralIsDroppedAndClean(t *testing.T) {
	dag := newDeferralTestDAG()
	dag.reconcileDeferrals("peerA", []string{"0xa"})
	dag.age("peerA", proposerBreakerOrphanGrace+30*time.Second)

	// The parent arrived; the block is now in the DAG.
	dag.mu.Lock()
	dag.blocks["0xa"] = &Block{Hash: "0xa"}
	dag.mu.Unlock()

	if got := dag.reconcileDeferrals("peerA", nil); got != 0 {
		t.Fatalf("a deferral that resolved must not count however old it got, got %d", got)
	}
	dag.deferredWatchMu.Lock()
	n := len(dag.deferredWatch["peerA"])
	dag.deferredWatchMu.Unlock()
	if n != 0 {
		t.Fatalf("a resolved deferral must be forgotten, %d still tracked — otherwise the "+
			"watch list grows without bound on a healthy node", n)
	}
}

// A node merging fine except for one permanently unreachable parent is still
// forked, and progress elsewhere must not mask it. This is the hole the earlier
// time-based guard had via its totalAdded > 0 early-out.
func TestReconcileDeferrals_PartialResolutionStillCounts(t *testing.T) {
	dag := newDeferralTestDAG()
	dag.reconcileDeferrals("peerA", []string{"0xa", "0xb", "0xc"})
	dag.age("peerA", proposerBreakerOrphanGrace+30*time.Second)

	dag.mu.Lock()
	dag.blocks["0xa"] = &Block{Hash: "0xa"}
	dag.blocks["0xb"] = &Block{Hash: "0xb"}
	dag.mu.Unlock()

	if got := dag.reconcileDeferrals("peerA", nil); got != 1 {
		t.Fatalf("the one block that never arrived must still count, got %d", got)
	}
}

// Peers are judged independently: one forked peer must not hold down the
// streak of a peer that is merging perfectly well.
func TestReconcileDeferrals_PeersAreIndependent(t *testing.T) {
	dag := newDeferralTestDAG()
	dag.reconcileDeferrals("forked", []string{"0xa"})
	dag.reconcileDeferrals("healthy", []string{"0xb"})
	dag.age("forked", proposerBreakerOrphanGrace+30*time.Second)
	dag.age("healthy", proposerBreakerOrphanGrace+30*time.Second)

	dag.mu.Lock()
	dag.blocks["0xb"] = &Block{Hash: "0xb"}
	dag.mu.Unlock()

	if got := dag.reconcileDeferrals("forked", nil); got != 1 {
		t.Fatalf("the forked peer must count, got %d", got)
	}
	if got := dag.reconcileDeferrals("healthy", nil); got != 0 {
		t.Fatalf("the healthy peer must stay clean, got %d", got)
	}
}

// A resync replaces the chain wholesale, so every tracked hash refers to
// history this node deliberately no longer has. Keeping them would condemn the
// fresh chain for a whole grace window after the resync that fixed it.
func TestForgetDeferralWatch_ClearsEverything(t *testing.T) {
	dag := newDeferralTestDAG()
	dag.reconcileDeferrals("peerA", []string{"0xa"})
	dag.age("peerA", proposerBreakerOrphanGrace+30*time.Second)

	dag.forgetDeferralWatch()

	if got := dag.reconcileDeferrals("peerA", nil); got != 0 {
		t.Fatalf("after a resync no pre-resync deferral may count, got %d", got)
	}
}

// The watch list is bounded: a huge restart backlog must not grow it without
// limit, and past the cap the already-tracked hashes answer the question.
func TestReconcileDeferrals_WatchListIsBounded(t *testing.T) {
	dag := newDeferralTestDAG()
	huge := make([]string, maxTrackedDeferralsPerPeer+500)
	for i := range huge {
		huge[i] = string(rune('a'+i%26)) + "-" + time.Duration(i).String()
	}
	dag.reconcileDeferrals("peerA", huge)

	dag.deferredWatchMu.Lock()
	n := len(dag.deferredWatch["peerA"])
	dag.deferredWatchMu.Unlock()
	if n > maxTrackedDeferralsPerPeer {
		t.Fatalf("watch list grew to %d, past the %d cap", n, maxTrackedDeferralsPerPeer)
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
			"and reconcileDeferrals is what doSyncOnce must use instead")
	}
}
