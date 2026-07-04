package keeper

import "testing"

// TestRefreshBootHeightAfterSnapshotImport_PlainRestartWithRealAnchorIsCheckpointBacked
// is the regression guard for the 2026-07-05 fix, the real root cause behind
// most of the night's recurring instability: on a PLAIN restart
// (resyncHappened=false — any routine no-resync-needed code deploy),
// bootHeightCheckpointBacked used to stay unconditionally false, even though
// NewBlockchain's own startup load (LoadBlocksFromDB) already restores a
// real block at exactly dag.bootHeight for any node that has ever produced
// or synced normally. That false pessimism made deepScanFloor() fall back
// to a full genesis walk (floor 0) on every plain restart — confirmed live:
// a node mid-genesis-walk was found adding real historical blocks in
// ascending order from height ~10900 while its actual tip was past 188000.
// When dag.blocks genuinely contains a non-stub block at dag.bootHeight,
// a plain restart must be recognized as checkpoint-backed too.
func TestRefreshBootHeightAfterSnapshotImport_PlainRestartWithRealAnchorIsCheckpointBacked(t *testing.T) {
	dag := newOrphanTestDAG()
	dag.state = &ChainState{}
	dag.bootHeight = 500
	dag.blocks["real-anchor"] = &Block{Hash: "real-anchor", Height: 500, Proposer: "0xhonest"}

	dag.RefreshBootHeightAfterSnapshotImport(false) // plain restart, no resync

	if !dag.BootHeightCheckpointBacked() {
		t.Fatal("a plain restart with a genuine block already in dag.blocks at bootHeight must be recognized as checkpoint-backed")
	}
}

// TestRefreshBootHeightAfterSnapshotImport_PlainRestartWithoutRealAnchorStaysUnbacked
// verifies the safety net still holds when dag.blocks does NOT actually
// contain a block at bootHeight (e.g. a pruned window that doesn't reach
// back that far) — must stay unbacked, exactly like before this fix,
// falling through to the safe (if slower) genesis-walk path rather than
// silently trusting a height nothing backs.
func TestRefreshBootHeightAfterSnapshotImport_PlainRestartWithoutRealAnchorStaysUnbacked(t *testing.T) {
	dag := newOrphanTestDAG()
	dag.state = &ChainState{}
	dag.bootHeight = 500
	// dag.blocks deliberately has nothing at height 500.

	dag.RefreshBootHeightAfterSnapshotImport(false)

	if dag.BootHeightCheckpointBacked() {
		t.Fatal("without a genuine block at bootHeight in dag.blocks, a plain restart must NOT be marked checkpoint-backed")
	}
}

// TestRefreshBootHeightAfterSnapshotImport_SyntheticStubAtBootHeightDoesNotCount
// verifies a synthetic-checkpoint stub (BridgeHistoricalGap's placeholder,
// never a real verified block) at bootHeight does not falsely satisfy the
// anchor check — only a genuine block counts.
func TestRefreshBootHeightAfterSnapshotImport_SyntheticStubAtBootHeightDoesNotCount(t *testing.T) {
	dag := newOrphanTestDAG()
	dag.state = &ChainState{}
	dag.bootHeight = 500
	dag.blocks["stub-hash"] = &Block{Hash: "stub-hash", Height: 500, Proposer: "synthetic-checkpoint"}

	dag.RefreshBootHeightAfterSnapshotImport(false)

	if dag.BootHeightCheckpointBacked() {
		t.Fatal("a synthetic-checkpoint stub at bootHeight must not count as a genuine anchor")
	}
}
