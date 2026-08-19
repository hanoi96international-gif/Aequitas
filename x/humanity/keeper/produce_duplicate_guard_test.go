package keeper

import (
	"os"
	"strings"
	"testing"
)

// The double-production guard has been removed and restored once already, and
// both directions were wrong for the same reason: the condition it tested,
// "chain_blocks already holds a block from me at this height", is true both
// when a second instance of this validator is running AND when this node
// simply wrote that block itself a moment ago. A 45-second window separated
// the two by guessing; producedHeights separates them by knowing.
//
// These tests pin the distinction, because getting it wrong in one direction
// suspends honest validators for a slow deploy and in the other halts the node
// permanently.

func TestProduceGuard_HeightsThisProcessWroteAreOwned(t *testing.T) {
	dag := newGhostdagTestDAG()

	// A DAG that has produced nothing owns nothing — and must answer that
	// without panicking even before the map exists, because the guard consults
	// it on the very first tick.
	if dag.ownsProducedHeight(1) {
		t.Error("a fresh process claims to have produced height 1")
	}

	dag.noteProducedHeight(41)
	dag.noteProducedHeight(42)

	for _, tc := range []struct {
		height int64
		mine   bool
	}{{41, true}, {42, true}, {43, false}, {0, false}} {
		if got := dag.ownsProducedHeight(tc.height); got != tc.mine {
			t.Errorf("height %d: owned=%v, want %v", tc.height, got, tc.mine)
		}
	}
}

// The guard must no longer be gated on how long this process has been alive.
// That window is what suspended a third-party validator for a rolling deploy
// that took longer than 45 seconds — an operator doing nothing wrong.
func TestProduceGuard_NoLongerTimeBoxed(t *testing.T) {
	src, err := os.ReadFile("block.go")
	if err != nil {
		t.Fatalf("read block.go: %v", err)
	}
	text := string(src)

	if strings.Contains(text, "postBootDuplicateGuardWindow") {
		t.Error("the duplicate-production guard is time-boxed again. A window cannot " +
			"tell a redeploy overlap from ordinary operation — that is what producedHeights " +
			"is for, and what the 2026-07-24 halt and the five equivocation incidents " +
			"before it were both caused by.")
	}

	idx := strings.Index(text, "HasBlockFromProposerAtHeight(proposer,")
	if idx < 0 {
		t.Fatal("the durable duplicate check is gone entirely — this test needs updating")
	}
	// The durable check must be qualified by "and this process did not write it".
	window := text[max(0, idx-600):idx]
	if !strings.Contains(window, "ownsProducedHeight(") {
		t.Error("HasBlockFromProposerAtHeight is consulted without first checking " +
			"producedHeights — the guard will fire on this node's own blocks and halt " +
			"production, exactly as it did on 2026-07-24")
	}
}

// The map must not grow without bound on a node that runs for months.
func TestProduceGuard_OwnedHeightsArePruned(t *testing.T) {
	dag := newGhostdagTestDAG()
	for h := int64(1); h <= 500; h++ {
		dag.noteProducedHeight(h)
	}
	if !dag.ownsProducedHeight(500) {
		t.Fatal("noteProducedHeight did not record height 500")
	}

	src, err := os.ReadFile("block.go")
	if err != nil {
		t.Fatalf("read block.go: %v", err)
	}
	prune := string(src)[strings.Index(string(src), "func (dag *BlockDAG) pruneOldDAGBlocks()"):]
	if !strings.Contains(prune[:min(4000, len(prune))], "producedHeights") {
		t.Error("pruneOldDAGBlocks does not trim producedHeights — it grows by one entry " +
			"per produced block for the life of the process")
	}
}
