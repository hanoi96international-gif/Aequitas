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
// deferred block does not reset the streak, and on a live chain EVERY newly
// produced peer block is younger than the grace window, so a permanently
// forked node's cycles looked "clean" forever.
//
// These tests pin the distinguishing rule that closes it: deferrals only count
// as benign catch-up while they are actually resolving.

func TestDeferralsNotResolving_ForkedNodeMustNotCountCleanCycle(t *testing.T) {
	dag := &BlockDAG{}
	// Nothing has merged for well beyond the grace window the deferrals are
	// being forgiven under — the live signature of a fork.
	dag.lastSuccessfulPeerSyncAt.Store(time.Now().Unix() - int64(proposerBreakerOrphanGrace.Seconds()) - 60)

	if !deferralsAreNotResolving(dag, 500, 0) {
		t.Fatal("a cycle that deferred 500 blocks, merged none, and has seen no merge " +
			"for longer than the grace window must NOT count as a clean sync cycle — " +
			"that is exactly the forked state that let both secondaries resume producing")
	}
}

func TestDeferralsResolving_HealthyBacklogStillCountsClean(t *testing.T) {
	dag := &BlockDAG{}
	// A merge just happened: this is an ordinary restart backlog draining,
	// which must keep reaching the threshold as fast as before the fix.
	dag.lastSuccessfulPeerSyncAt.Store(time.Now().Unix())

	if deferralsAreNotResolving(dag, 500, 0) {
		t.Fatal("an ordinary catch-up backlog whose deferrals ARE resolving must still " +
			"count as clean — otherwise every restart stalls production again")
	}
}

func TestDeferralsNotResolving_CycleThatMergedSomethingIsAlwaysClean(t *testing.T) {
	dag := &BlockDAG{}
	dag.lastSuccessfulPeerSyncAt.Store(time.Now().Unix() - int64(proposerBreakerOrphanGrace.Seconds()) - 60)

	// totalAdded > 0: real progress happened this very cycle, so however stale
	// the global timestamp looks, this peer is demonstrably merging.
	if deferralsAreNotResolving(dag, 500, 1) {
		t.Fatal("a cycle that merged at least one block must never be treated as a fork")
	}
}

func TestDeferralsNotResolving_NoDeferralsIsAlwaysClean(t *testing.T) {
	dag := &BlockDAG{}
	dag.lastSuccessfulPeerSyncAt.Store(time.Now().Unix() - int64(proposerBreakerOrphanGrace.Seconds()) - 60)

	if deferralsAreNotResolving(dag, 0, 0) {
		t.Fatal("a cycle with nothing deferred has no deferral evidence to judge and " +
			"must be left alone (e.g. a genuinely idle peer with nothing new)")
	}
}

func TestDeferralsNotResolving_NeverMergedYetIsNotTreatedAsFork(t *testing.T) {
	dag := &BlockDAG{}
	// Zero means "no successful merge recorded yet" (fresh process), not "the
	// last merge was at the epoch" — treating it as a fork would block every
	// node's very first catch-up.
	dag.lastSuccessfulPeerSyncAt.Store(0)

	if deferralsAreNotResolving(dag, 500, 0) {
		t.Fatal("a node that has not merged anything YET (fresh boot) must not be " +
			"misread as forked")
	}
}
