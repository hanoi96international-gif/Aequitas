package keeper

import "testing"

// TestIsCatchingUp_FreshGenesisViaSyncTarget is the regression guard for the
// Contabo "stuck at height 0, never syncs" stall. A node booting from a fresh
// DB has bootHeight == 0, so the old bootHeight-only catch-up check reported
// "not catching up" even while the network was tens of thousands of blocks
// ahead. That made AddPeerBlock feed the per-proposer breaker on the primary's
// own far-ahead pushed blocks, trip it, and then drop the ordered-sync blocks
// too — the node could never advance past genesis. syncTargetHeight (set live
// from the seed's reported height) must be an equally valid catch-up signal.
func TestIsCatchingUp_FreshGenesisViaSyncTarget(t *testing.T) {
	dag := newGhostdagTestDAG()

	// Fresh node: genesis only, nothing imported.
	dag.height = 0
	dag.bootHeight = 0

	if dag.isCatchingUpLocked() {
		t.Fatalf("with no bootHeight and no sync target, a node is not catching up")
	}

	// The seed reported a far-ahead height at startup (fetchAndSetSyncTarget).
	dag.syncTargetHeight.Store(77409)
	if !dag.isCatchingUpLocked() {
		t.Fatalf("a fresh node (bootHeight 0) far behind the seed's reported height MUST be treated as catching up")
	}

	// Once caught up to within the 10-block buffer, the gate closes again so the
	// breaker resumes protecting a synced node from real diverged forks.
	dag.height = 77405
	if dag.isCatchingUpLocked() {
		t.Fatalf("within 10 of the sync target the node is caught up — must not stay in catch-up mode")
	}
}

// TestIsCatchingUp_BootHeightStillWorks confirms the pre-existing
// post-RESYNC_FROM_SNAPSHOT path (high bootHeight, low dag.height) is
// unchanged by the syncTargetHeight addition.
func TestIsCatchingUp_BootHeightStillWorks(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.height = 0
	dag.bootHeight = 23093 // snapshot_import_height, no live sync target set

	if !dag.isCatchingUpLocked() {
		t.Fatalf("a node with dag.height far below bootHeight is catching up (post-resync path)")
	}
	dag.height = 23090
	if dag.isCatchingUpLocked() {
		t.Fatalf("within 10 of bootHeight the node is caught up")
	}
}

// TestFarAheadFrontier_SnapshotBootstrap is the regression guard for the
// Contabo "stuck at height 0 behind a synthetic checkpoint" stall. After a
// snapshot bootstrap the node holds a stub at bootHeight but dag.height stays 0
// (the stub is never a tip), so the far-ahead cap must be measured against
// bootHeight — otherwise the peer's earliest real block (one above the snapshot
// boundary, whose parent IS the stub) is rejected as far-ahead and can never
// attach.
func TestFarAheadFrontier_SnapshotBootstrap(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.height = 0
	dag.bootHeight = 76137 // snapshot boundary; stub lives here, dag.height stays 0

	frontier := dag.farAheadFrontier()
	if frontier != 76137 {
		t.Fatalf("frontier must be max(height, bootHeight)=76137, got %d", frontier)
	}
	// The primary's earliest real block (#76138, parent = the stub) must NOT be
	// far-ahead: 76138 <= 76137 + maxOrphanHeightGap.
	if int64(76138) > frontier+maxOrphanHeightGap {
		t.Fatalf("block #76138 (attaches to the snapshot stub) must not be far-ahead against frontier %d", frontier)
	}
	// A genuine runaway fork far above the snapshot boundary is still rejected.
	if int64(76137+maxOrphanHeightGap+1) <= frontier+maxOrphanHeightGap {
		t.Fatalf("a block beyond bootHeight+cap must still count as far-ahead")
	}
}

// TestFarAheadFrontier_NoSnapshotUsesHeight confirms an ordinary synced node
// (no snapshot gap) still measures far-ahead against its own height.
func TestFarAheadFrontier_NoSnapshotUsesHeight(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.height = 5000
	dag.bootHeight = 0
	if got := dag.farAheadFrontier(); got != 5000 {
		t.Fatalf("with bootHeight 0 the frontier is dag.height=5000, got %d", got)
	}
}

// TestIsCatchingUp_SyncedNodeNotShielded guards against the inverse mistake:
// a fully-synced node (no bootHeight gap, sync target cleared to 0) must report
// "not catching up" so the breaker still trips on a genuine diverged-fork
// flood.
func TestIsCatchingUp_SyncedNodeNotShielded(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.height = 77400
	dag.bootHeight = 77400
	dag.syncTargetHeight.Store(0) // gate cleared once caught up

	if dag.isCatchingUpLocked() {
		t.Fatalf("a caught-up node must not be in catch-up mode — the breaker must stay armed")
	}
}
