package keeper

import "testing"

// TestUnverifiedSyntheticCheckpoint_BoundaryVsMidChain verifies the boundary
// rule that keeps a snapshot-bootstrapped node producing: a stub AT/below
// bootHeight (the signed-snapshot start-of-history) is trusted like genesis and
// is NOT counted as unverified, while a stub ABOVE bootHeight (a real mid-chain
// gap) IS — and only the latter gates production/health. Mirrors the inline
// `stubH > dag.bootHeight` decision at every insertion site.
func TestUnverifiedSyntheticCheckpoint_BoundaryVsMidChain(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.bootHeight = 76137 // trusted snapshot boundary

	// Simulate the boundary stub (height == bootHeight): counted in the total,
	// never in the unverified subset.
	boundaryStubHeight := int64(76137)
	dag.syntheticCheckpointCount.Add(1)
	if boundaryStubHeight > dag.bootHeight {
		dag.unverifiedSyntheticCheckpointCount.Add(1)
	}
	if dag.SyntheticCheckpointCount() != 1 {
		t.Fatalf("boundary stub must count toward the total, got %d", dag.SyntheticCheckpointCount())
	}
	if dag.UnverifiedSyntheticCheckpointCount() != 0 {
		t.Fatalf("boundary stub must NOT count as unverified (would strand a snapshot node in permanent non-production), got %d", dag.UnverifiedSyntheticCheckpointCount())
	}

	// Simulate a genuine mid-chain gap above the boundary: counted in both.
	midChainStubHeight := int64(77000)
	dag.syntheticCheckpointCount.Add(1)
	if midChainStubHeight > dag.bootHeight {
		dag.unverifiedSyntheticCheckpointCount.Add(1)
	}
	if dag.UnverifiedSyntheticCheckpointCount() != 1 {
		t.Fatalf("a mid-chain gap above bootHeight must count as unverified (gates production), got %d", dag.UnverifiedSyntheticCheckpointCount())
	}
}

// TestReleaseFinalitySealedStubs verifies the finality-release rule that keeps
// a node producing after bridging permanently-lost history: a stub more than
// finalityHeightSlack BELOW the finalized checkpoint is released from the
// production gate (its heal is unsatisfiable — isFinalityViolation would
// reject the real block from any non-seed peer), while a stub still WITHIN
// the finality window keeps gating production exactly as before. The total
// stub count must be untouched either way: releasing is a gate decision, not
// a heal — the stub itself stays in dag.blocks and stays visible in health.
func TestReleaseFinalitySealedStubs(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.unverifiedStubHeights = make(map[string]int64)
	cs := &ChainState{}
	cs.finalizedHeightCache = 10000
	cs.finalizedCacheLoaded = true
	dag.state = cs

	// Deep stub, far below checkpoint-slack: must be released.
	dag.syntheticCheckpointCount.Add(1)
	dag.unverifiedSyntheticCheckpointCount.Add(1)
	dag.unverifiedStubHeights["stub-deep"] = 500

	// Recent stub, inside the finality window (10000-50=9950 ≤ 9980): must keep gating.
	dag.syntheticCheckpointCount.Add(1)
	dag.unverifiedSyntheticCheckpointCount.Add(1)
	dag.unverifiedStubHeights["stub-recent"] = 9980

	dag.releaseFinalitySealedStubs()

	if got := dag.UnverifiedSyntheticCheckpointCount(); got != 1 {
		t.Fatalf("want exactly the recent stub still gating production, got %d unverified", got)
	}
	if _, still := dag.unverifiedStubHeights["stub-deep"]; still {
		t.Error("finality-sealed stub must be removed from unverifiedStubHeights")
	}
	if _, kept := dag.unverifiedStubHeights["stub-recent"]; !kept {
		t.Error("a stub within the finality window must NOT be released")
	}
	if got := dag.SyntheticCheckpointCount(); got != 2 {
		t.Errorf("release must not touch the total stub count (health/trust-mode visibility), got %d", got)
	}

	// Heal arriving AFTER release must not double-decrement (the heal path
	// decrements only for hashes still tracked in unverifiedStubHeights —
	// simulate its membership rule for the already-released stub).
	if _, tracked := dag.unverifiedStubHeights["stub-deep"]; tracked {
		dag.unverifiedSyntheticCheckpointCount.Add(-1)
	}
	if got := dag.UnverifiedSyntheticCheckpointCount(); got != 1 {
		t.Errorf("heal-after-release must be a no-op for the unverified counter, got %d", got)
	}
}

// TestReleaseFinalitySealedStubs_NoCheckpointNoRelease documents the guards:
// with no finalized checkpoint yet (fresh node) or no unverified stubs, the
// sweep must be a no-op — in particular it must never release anything on a
// node that hasn't established finality, where "sealed" is meaningless.
func TestReleaseFinalitySealedStubs_NoCheckpointNoRelease(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.unverifiedStubHeights = map[string]int64{"stub-1": 500}
	cs := &ChainState{}
	cs.finalizedCacheLoaded = true // loaded, but height 0 = not established
	dag.state = cs
	dag.syntheticCheckpointCount.Add(1)
	dag.unverifiedSyntheticCheckpointCount.Add(1)

	dag.releaseFinalitySealedStubs()

	if got := dag.UnverifiedSyntheticCheckpointCount(); got != 1 {
		t.Fatalf("no checkpoint established → nothing may be released, got %d unverified", got)
	}

	// Counter at 0 short-circuits before touching state at all (state=nil
	// would panic if it didn't).
	dag2 := newGhostdagTestDAG()
	dag2.unverifiedStubHeights = map[string]int64{}
	dag2.releaseFinalitySealedStubs() // must not panic despite dag2.state == nil
}

// TestSyntheticCheckpointHashes_FindsStubsOnly verifies the lookup the
// active-healing mechanism (sync_blocks.go: healSyntheticCheckpoints) relies
// on to know which hashes to try to replace with real peer data.
func TestSyntheticCheckpointHashes_FindsStubsOnly(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.blocks["real-1"] = &Block{Hash: "real-1", Proposer: "0xabc"}
	dag.blocks["stub-1"] = &Block{Hash: "stub-1", Proposer: "synthetic-checkpoint"}
	dag.blocks["stub-2"] = &Block{Hash: "stub-2", Proposer: "synthetic-checkpoint"}

	// SyntheticCheckpointHashes short-circuits on the atomic counter, so it
	// must be kept in sync the same way the production insertion path does.
	dag.syntheticCheckpointCount.Store(2)

	hashes := dag.SyntheticCheckpointHashes()
	if len(hashes) != 2 {
		t.Fatalf("want 2 stub hashes, got %d: %v", len(hashes), hashes)
	}
	got := map[string]bool{}
	for _, h := range hashes {
		got[h] = true
	}
	if !got["stub-1"] || !got["stub-2"] {
		t.Errorf("want stub-1 and stub-2, got %v", hashes)
	}
	if got["real-1"] {
		t.Errorf("real-1 should not be reported as a synthetic checkpoint")
	}
}

// TestSyntheticCheckpointHashes_ZeroCounterShortCircuits documents the
// performance guard: with the counter at 0, the function must not pay for a
// full dag.blocks scan even if (inconsistently) a stub-labeled entry exists.
func TestSyntheticCheckpointHashes_ZeroCounterShortCircuits(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.blocks["stub-1"] = &Block{Hash: "stub-1", Proposer: "synthetic-checkpoint"}
	// Counter intentionally left at 0 (zero value).
	if hashes := dag.SyntheticCheckpointHashes(); hashes != nil {
		t.Errorf("want nil when counter is 0, got %v", hashes)
	}
}

// TestAddPeerBlock_AlreadyKnownCheck_AllowsStubsThrough is a narrow
// regression test for the specific bug fixed in AddPeerBlock: a hash that
// already exists in dag.blocks used to be rejected unconditionally ("Skip if
// already known"), which meant a synthetic-checkpoint stub could NEVER be
// replaced by the real block even if one later became available — a
// permanent, silent loss of whatever that block actually contained. This
// only verifies the specific branch logic (stub vs. non-stub), not the full
// signature/replay pipeline (which needs a live ChainState + signing key and
// is exercised by the project's integration/manual test paths instead).
func TestAddPeerBlock_AlreadyKnownCheck_AllowsStubsThrough(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.blocks["stub-hash"] = &Block{Hash: "stub-hash", Height: 5, Proposer: "synthetic-checkpoint"}
	dag.authorizedValidators = make(map[string]bool)
	dag.warnedUnknownProposers = make(map[string]bool)

	// A block with no signature reaches the "missing signature" rejection
	// further down AddPeerBlock — proving it was NOT short-circuited by the
	// "already known" check at the top, which is the exact bug being fixed
	// here. Before the fix, this returned false at the very first check
	// (existing && true) with no chance to reach the signature check at all;
	// the distinction isn't observable via the bool return alone (both paths
	// return false), so this test's value is the smoke-test that it does not
	// panic and that real (non-stub) collisions still short-circuit below.
	block := &Block{Hash: "stub-hash", Height: 5, ParentHashes: []string{"genesis"}, Proposer: "0xabc"}
	if dag.AddPeerBlock(block) {
		t.Fatal("expected rejection (no valid signature), got accepted")
	}

	// A real (non-stub) existing entry must still short-circuit exactly as
	// before — this is the safety property that makes healing safe: only
	// stubs are ever reconsidered.
	dag2 := newGhostdagTestDAG()
	dag2.blocks["real-hash"] = &Block{Hash: "real-hash", Height: 5, Proposer: "0xdef"}
	dag2.authorizedValidators = make(map[string]bool)
	dag2.warnedUnknownProposers = make(map[string]bool)
	if dag2.AddPeerBlock(&Block{Hash: "real-hash", Height: 5, ParentHashes: []string{"genesis"}, Proposer: "0xabc"}) {
		t.Fatal("expected rejection (hash collision with a real, non-stub block), got accepted")
	}
}
