package keeper

import "testing"

// Regression test for the definitive root cause of the 2026-07-06 3-node
// non-merging incident: isFinalityViolation exempted FromSync blocks but not
// SelfFetched ones, even though both the circuit breaker (block.go) and the
// suspension gate (block.go) already treat them identically — a block this
// node deliberately fetched (via fetchMissingAncestors, to resolve a real
// orphan) can never be irrelevant old history, by definition something else
// needs it as a direct ancestor right now. Without this exemption, a
// genuinely-needed ancestor fetched from a non-seed peer (FromSync only
// applies to a configured seed) was rejected forever once the local
// finalized checkpoint climbed past it — which happens quickly under
// continuous self-production — permanently orphaning every descendant block
// and preventing the whole network from merging.
func TestIsFinalityViolation_SelfFetchedAncestorExempt(t *testing.T) {
	cs := newTestState()
	cs.SetFinalizedCheckpoint("deadbeef", 1000, 5000)
	dag := &BlockDAG{state: cs}

	oldBlock := &Block{Height: 500, Hash: "old-ancestor"} // far below 1000-50

	if !dag.isFinalityViolation(oldBlock) {
		t.Fatal("an ordinary old block (not FromSync, not SelfFetched) must still be a finality violation")
	}

	oldBlock.SelfFetched = true
	if dag.isFinalityViolation(oldBlock) {
		t.Fatal("a deliberately self-fetched ancestor must be exempt from the finality gate, regardless of how far below the checkpoint it is")
	}

	oldBlock.SelfFetched = false
	oldBlock.FromSync = true
	if dag.isFinalityViolation(oldBlock) {
		t.Fatal("a FromSync (trusted seed) block must remain exempt, as before")
	}
}

func TestIsFinalityViolation_RecentBlockNeverAViolation(t *testing.T) {
	cs := newTestState()
	cs.SetFinalizedCheckpoint("deadbeef", 1000, 5000)
	dag := &BlockDAG{state: cs}

	recentBlock := &Block{Height: 990, Hash: "recent"} // within finalityHeightSlack of 1000
	if dag.isFinalityViolation(recentBlock) {
		t.Fatal("a block within finalityHeightSlack of the checkpoint must never be a violation, self-fetched or not")
	}
}
