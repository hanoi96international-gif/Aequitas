package keeper

import "testing"

// TestLatestBlock_PicksHighestBlueScoreNotHeight is the regression guard for
// the 2026-07-04 "blue scores don't match across nodes" incident: LatestBlock
// (used by /api/status's latest_hash) used to pick the tip with the highest
// HEIGHT, while canonicalBlockAtHeightLocked (used by /api/blocks/canonical,
// the authoritative endpoint) and ProduceBlock's own parent selection both
// pick by highest BLUE_SCORE. Confirmed live: the same node's /api/status and
// /api/blocks/canonical disagreed about the canonical tip at the same height,
// at the same moment, because they used two different selection rules. A tip
// with a lower height but higher blue_score (representing more accumulated
// GHOSTDAG merge work) must be reported as "latest", matching the rest of the
// codebase's canonical ordering everywhere.
func TestLatestBlock_PicksHighestBlueScoreNotHeight(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.tips = map[string]bool{"low-height-high-score": true, "high-height-low-score": true}
	dag.blocks["low-height-high-score"] = &Block{Hash: "low-height-high-score", Height: 100, BlueScore: 5000}
	dag.blocks["high-height-low-score"] = &Block{Hash: "high-height-low-score", Height: 101, BlueScore: 4000}

	got := dag.LatestBlock()
	if got == nil || got.Hash != "low-height-high-score" {
		t.Fatalf("LatestBlock() = %+v, want the higher-BlueScore tip regardless of its lower height", got)
	}
}

// TestLatestBlock_TieBreaksOnHashWhenBlueScoreMatches verifies the
// deterministic hash tie-break (needed so two nodes holding the identical
// tip set always agree, regardless of Go's randomized map iteration order)
// still applies when BlueScore is equal.
func TestLatestBlock_TieBreaksOnHashWhenBlueScoreMatches(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.tips = map[string]bool{"bbb": true, "aaa": true}
	dag.blocks["bbb"] = &Block{Hash: "bbb", Height: 100, BlueScore: 5000}
	dag.blocks["aaa"] = &Block{Hash: "aaa", Height: 100, BlueScore: 5000}

	got := dag.LatestBlock()
	if got == nil || got.Hash != "aaa" {
		t.Fatalf("LatestBlock() = %+v, want the lexicographically lowest hash on a BlueScore tie", got)
	}
}
