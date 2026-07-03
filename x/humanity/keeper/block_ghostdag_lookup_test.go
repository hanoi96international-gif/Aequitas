package keeper

import "testing"

// TestGhostdagBlockLookup_MemoryHit verifies the fast path (block resident in
// dag.blocks) never touches dag.state, and still works when dag.state is nil
// — the same contract the raw `dag.blocks[hash]` access had before
// ghostdagBlockLookup replaced it in computeGHOSTDAGState/ghostdagMergeSet/
// ghostdagIsAncestor.
func TestGhostdagBlockLookup_MemoryHit(t *testing.T) {
	dag := newGhostdagTestDAG()
	want := &Block{Hash: "h1", Height: 1}
	dag.blocks["h1"] = want

	got := dag.ghostdagBlockLookup("h1")
	if got != want {
		t.Fatalf("ghostdagBlockLookup returned %v, want the in-memory block %v", got, want)
	}
}

// TestGhostdagBlockLookup_MissNoState verifies a genuinely-missing hash with
// no DB backing (dag.state == nil, the same setup every other GHOSTDAG scale
// test in this package uses) returns nil rather than panicking — matching
// the prior raw map access's `ok == false` behavior exactly.
func TestGhostdagBlockLookup_MissNoState(t *testing.T) {
	dag := newGhostdagTestDAG()
	if got := dag.ghostdagBlockLookup("does-not-exist"); got != nil {
		t.Fatalf("ghostdagBlockLookup() = %v, want nil for a hash absent from both dag.blocks and dag.state", got)
	}
}
