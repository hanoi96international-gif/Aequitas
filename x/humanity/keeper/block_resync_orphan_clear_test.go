package keeper

import "testing"

// TestRefreshBootHeightAfterSnapshotImport_ClearsStaleOrphans is the
// regression guard for the fourth layer of the 2026-07-04 incident: a
// resync wipes dag.blocks/dag.tips but, until this fix, never touched
// dag.orphans/orphanFirstSeen/orphanAttempts. Every orphan queued before a
// resync references a missing-parent hash from this node's OWN pre-resync
// (possibly isolated, dead-end) history — none of it is reachable from the
// fresh checkpoint going forward. Confirmed live: once the SelfFetched
// exemption (block.go, AddPeerBlock) started correctly making
// fetchMissingAncestors' resolutions stick instead of silently discarding
// them, these stale pre-resync entries stopped being harmless noise and
// started costing real resolution attempts and orphanAbandonAfter budget —
// a fresh-checkpoint node kept walking backward through a dozens-deep
// pre-resync orphan chain that could never connect to anything, crowding
// out attention that should have gone to the live tip. A resync must clear
// the orphan queue along with dag.blocks/dag.tips.
func TestRefreshBootHeightAfterSnapshotImport_ClearsStaleOrphans(t *testing.T) {
	dag := newOrphanTestDAG()
	dag.state = &ChainState{}
	staleOrphan := &Block{Hash: "stale-orphan-child", Height: 500, ParentHashes: []string{"pre-resync-missing-parent"}, Proposer: "0xhonest"}
	dag.queueOrphan("pre-resync-missing-parent", staleOrphan)
	if _, tracked := dag.orphanAge("pre-resync-missing-parent"); !tracked {
		t.Fatal("precondition failed: expected the stale orphan to be tracked before the resync")
	}

	dag.RefreshBootHeightAfterSnapshotImport(true)

	if _, tracked := dag.orphanAge("pre-resync-missing-parent"); tracked {
		t.Fatal("a resync must clear pre-existing orphan queue entries — they reference dead-end, pre-resync history that can never connect to the fresh checkpoint")
	}
	dag.orphansMu.Lock()
	n := len(dag.orphans)
	dag.orphansMu.Unlock()
	if n != 0 {
		t.Fatalf("dag.orphans must be empty right after a resync, got %d entr(ies)", n)
	}
}
