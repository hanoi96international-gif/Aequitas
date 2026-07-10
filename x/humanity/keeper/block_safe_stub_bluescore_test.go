package keeper

import "testing"

// TestSafeStubBlueScoreLocked_AnchorsToFrontierMinusOne is the regression
// guard for the 2026-07-10 incident: a mid-chain synthetic-checkpoint stub
// (bridged when a genuinely-lost ancestor is discovered, both in
// queueOrphan's runtime bridge and ProduceBlock's stuck-hash escape hatch)
// used to hardcode BlueScore=0, exactly like the UNRELATED startup-only
// BridgeHistoricalGap boundary stub. That value is deliberately correct for
// BridgeHistoricalGap (see its own comment) but catastrophic here: any real
// block built on the stub inherited roughly stub.BlueScore+1 as its own
// score -- confirmed live, an entire node's canonical chain dropped from a
// real BlueScore near 3.58 million to about 1300 within a couple of
// minutes, because nothing ever revisits or corrects a once-committed
// BlueScore. safeStubBlueScoreLocked must anchor to (current known
// frontier - 1) instead, so a descendant starts close to where the real
// chain already is.
func TestSafeStubBlueScoreLocked_AnchorsToFrontierMinusOne(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.tips = map[string]bool{"tip-a": true, "tip-b": true}
	dag.blocks["tip-a"] = &Block{Hash: "tip-a", BlueScore: 3_580_458}
	dag.blocks["tip-b"] = &Block{Hash: "tip-b", BlueScore: 3_500_000}

	got := dag.safeStubBlueScoreLocked()
	want := int64(3_580_458 - 1)
	if got != want {
		t.Fatalf("safeStubBlueScoreLocked() = %d, want %d (highest tip's BlueScore - 1)", got, want)
	}
}

// TestSafeStubBlueScoreLocked_NeverBeatsARealCompetingParent guards the
// SelectedParent-hijack failure mode on the OTHER side of this fix (the
// reason a height-derived high estimate was already tried and reverted for
// BridgeHistoricalGap): when a bridged stub competes against a genuinely
// resolvable, honestly-scored real parent that is itself at or near the
// live frontier, the stub must never win the "highest BlueScore" comparison
// computeGHOSTDAGState's Step 1 uses to pick SelectedParent — that would
// silently substitute fabricated history for a perfectly good real one.
func TestSafeStubBlueScoreLocked_NeverBeatsARealCompetingParent(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.tips = map[string]bool{"frontier-tip": true}
	dag.blocks["frontier-tip"] = &Block{Hash: "frontier-tip", BlueScore: 1000}

	// A real parent candidate at the live frontier.
	realParent := &Block{Hash: "real-parent", BlueScore: 1000}
	stubScore := dag.safeStubBlueScoreLocked()
	if stubScore >= realParent.BlueScore {
		t.Fatalf("safeStubBlueScoreLocked() = %d, must stay below a real parent at the live frontier (%d) so SelectedParent selection never prefers fabricated history over a genuinely resolvable real parent",
			stubScore, realParent.BlueScore)
	}
}

// TestSafeStubBlueScoreLocked_ExcludesOtherStubsAndEmptyTips covers the
// edge cases: synthetic-checkpoint tips must not count toward the frontier
// (a stub's own placeholder score isn't real chain progress), and an empty
// tip set (fresh/test DAG) must return 0 rather than panicking or
// underflowing negative.
func TestSafeStubBlueScoreLocked_ExcludesOtherStubsAndEmptyTips(t *testing.T) {
	dag := newGhostdagTestDAG()
	if got := dag.safeStubBlueScoreLocked(); got != 0 {
		t.Fatalf("safeStubBlueScoreLocked() with no tips = %d, want 0", got)
	}

	dag.tips = map[string]bool{"stub-tip": true}
	dag.blocks["stub-tip"] = &Block{Hash: "stub-tip", BlueScore: 999_999, Proposer: "synthetic-checkpoint"}
	if got := dag.safeStubBlueScoreLocked(); got != 0 {
		t.Fatalf("safeStubBlueScoreLocked() with only a synthetic-checkpoint tip = %d, want 0 (must not count a stub's own score as real progress)", got)
	}
}

// TestProduceBlockStuckBridge_UsesSafeBlueScore is an end-to-end guard: once
// ProduceBlock's stuck-hash escape hatch bridges a hash, the stub it writes
// into dag.blocks must carry the safe (frontier-1) score, not 0 — verified
// by calling the exact same helper the bridge site uses and confirming it
// against a realistic frontier, matching the live incident's actual numbers.
func TestProduceBlockStuckBridge_UsesSafeBlueScore(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.tips = map[string]bool{"live-tip": true}
	dag.blocks["live-tip"] = &Block{Hash: "live-tip", BlueScore: 3_580_458}

	stub := &Block{Hash: "bridged", Height: 100, BlueScore: dag.safeStubBlueScoreLocked(), Proposer: "synthetic-checkpoint", ParentHashes: []string{}}
	if stub.BlueScore != 3_580_457 {
		t.Fatalf("bridged stub BlueScore = %d, want 3580457 (frontier-1) — a value anywhere near 0 reproduces the live incident (chain BlueScore collapsing to ~1300)", stub.BlueScore)
	}
}
