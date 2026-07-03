package keeper

import "testing"

// makeSinceTestBlocks builds a canonically-sorted (height ASC, blueScore
// DESC, hash ASC) slice of blocks for selectBlocksSince tests — mirroring
// the order GetBlocks()/LoadBlocksSinceFromDB both produce before calling it.
func makeSinceTestBlocks(specs ...[3]interface{}) []*Block {
	out := make([]*Block, 0, len(specs))
	for _, s := range specs {
		out = append(out, &Block{
			Height:    int64(s[0].(int)),
			BlueScore: int64(s[1].(int)),
			Hash:      s[2].(string),
		})
	}
	return out
}

// TestSelectBlocksSince_NoCursor verifies the plain "Height > minHeight"
// case (no after_hash), capped at limit — the common case for a node
// syncing forward from its own current height.
func TestSelectBlocksSince_NoCursor(t *testing.T) {
	blocks := makeSinceTestBlocks(
		[3]interface{}{10, 5, "a"},
		[3]interface{}{11, 5, "b"},
		[3]interface{}{12, 5, "c"},
	)
	got := selectBlocksSince(blocks, 10, "", 10)
	if len(got) != 2 || got[0].Hash != "b" || got[1].Hash != "c" {
		t.Fatalf("expected blocks b,c (height > 10), got %+v", got)
	}
}

// TestSelectBlocksSince_RespectsLimit verifies the page is capped even when
// more matching blocks exist beyond it.
func TestSelectBlocksSince_RespectsLimit(t *testing.T) {
	blocks := makeSinceTestBlocks(
		[3]interface{}{10, 5, "a"},
		[3]interface{}{11, 5, "b"},
		[3]interface{}{12, 5, "c"},
	)
	got := selectBlocksSince(blocks, 9, "", 1)
	if len(got) != 1 || got[0].Hash != "a" {
		t.Fatalf("expected exactly 1 block (a), got %+v", got)
	}
}

// TestSelectBlocksSince_CursorIncludesSameHeightSiblings is the P1-02
// regression this whole cursor mechanism exists for: a page of same-height
// sibling blocks must not silently skip the rest of that height's siblings
// on the next request. Cursor (10, "b") must return "c" (same height, comes
// after "b") and everything at height 11+, but never "a" (before the cursor).
func TestSelectBlocksSince_CursorIncludesSameHeightSiblings(t *testing.T) {
	blocks := makeSinceTestBlocks(
		[3]interface{}{10, 5, "a"},
		[3]interface{}{10, 5, "b"},
		[3]interface{}{10, 5, "c"},
		[3]interface{}{11, 5, "d"},
	)
	got := selectBlocksSince(blocks, 10, "b", 10)
	if len(got) != 2 || got[0].Hash != "c" || got[1].Hash != "d" {
		t.Fatalf("expected c,d (after cursor b at height 10, plus height 11), got %+v", got)
	}
}

// TestSelectBlocksSince_CursorNotFound falls back to "nothing at or below
// minHeight qualifies" when the exact cursor hash isn't in this window
// (e.g. it was on a page already consumed) — still returns anything above
// minHeight, matching the no-cursor case for higher heights.
func TestSelectBlocksSince_CursorNotFound(t *testing.T) {
	blocks := makeSinceTestBlocks(
		[3]interface{}{10, 5, "a"},
		[3]interface{}{11, 5, "b"},
	)
	got := selectBlocksSince(blocks, 10, "nonexistent-hash", 10)
	if len(got) != 1 || got[0].Hash != "b" {
		t.Fatalf("expected only b (height > minHeight, cursor never matched), got %+v", got)
	}
}

// TestSelectBlocksSince_EmptyInput handles an empty source slice (e.g. the
// DB fallback found nothing in range) without panicking.
func TestSelectBlocksSince_EmptyInput(t *testing.T) {
	got := selectBlocksSince(nil, 0, "", 50)
	if len(got) != 0 {
		t.Fatalf("expected no blocks from empty input, got %+v", got)
	}
}

// TestGetBlocksSince_NoStateFallsBackToMemory verifies GetBlocksSince never
// panics and correctly serves the in-memory window when dag.state is nil
// (no DB configured) — the DB-fallback branch (see GetBlocksSince's own
// comment for the production bug it fixes: pruneOldDAGBlocks evicts old
// blocks from dag.blocks, so a min_height below the current prune cutoff
// must not be answered from the wrong, current-tip in-memory window) is
// only reachable when dag.state != nil; this confirms the nil-state guard.
func TestGetBlocksSince_NoStateFallsBackToMemory(t *testing.T) {
	dag := newGhostdagTestDAG() // dag.state is nil by construction here
	dag.mu.Lock()
	dag.blocks["tip1"] = &Block{Hash: "tip1", Height: 100, ParentHashes: []string{}}
	dag.mu.Unlock()
	got := dag.GetBlocksSince(0, "", 10)
	if len(got) != 1 || got[0].Hash != "tip1" {
		t.Fatalf("expected in-memory fallback to return the one known block, got %+v", got)
	}
}
