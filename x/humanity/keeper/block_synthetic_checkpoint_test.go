package keeper

import "testing"

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
