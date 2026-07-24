package keeper

import "testing"

// newDeepScanFloorTestDAG builds the minimal DAG deepScanFloor reads:
// bootHeight plus the checkpoint-backed flag.
func newDeepScanFloorTestDAG(bootHeight int64, checkpointBacked bool) *BlockDAG {
	dag := newGhostdagTestDAG()
	dag.bootHeight = bootHeight
	dag.bootHeightCheckpointBacked = checkpointBacked
	return dag
}

// TestDeepScanFloor_PlainRestartDoesNotFallBackToGenesis is the regression
// guard for the 2026-07-24 finding that an ordinary redeploy cost many
// minutes of total non-merging.
//
// BootHeightCheckpointBacked is true ONLY in the resync branch, so every
// plain restart took the fallback — which used to be a bare 0. doSyncOnce
// starts each restart with an empty peerSyncHeight, falls into its
// `minHeight < 0` branch and adopts this floor, so the node re-walked its
// entire history from genesis while attaching nothing. Confirmed live on
// Contabo1: merging happily for 27 minutes, restarted by a deploy, then 0
// attaches at a frozen height.
func TestDeepScanFloor_PlainRestartDoesNotFallBackToGenesis(t *testing.T) {
	const boot = 1781853 // Contabo1's real height when the deploy restarted it
	dag := newDeepScanFloorTestDAG(boot, false)

	got := dag.deepScanFloor()
	if got == 0 {
		t.Fatal("a plain restart must not re-walk from genesis — that is the whole incident")
	}
	if got >= boot {
		t.Fatalf("floor %d must stay strictly BELOW bootHeight %d: min_height is EXCLUSIVE, so a floor at bootHeight permanently excludes a common ancestor sitting exactly there (the 2026-07-04 incident)", got, boot)
	}
	if want := int64(boot - plainRestartDeepScanMargin); got != want {
		t.Fatalf("floor = %d, want %d (bootHeight - plainRestartDeepScanMargin)", got, want)
	}
}

// TestDeepScanFloor_CheckpointBackedIsUnchanged pins that the resync path
// keeps its exact previous behaviour: a checkpoint-seeded node has a real,
// verified block at bootHeight, so the floor sits there and not below.
func TestDeepScanFloor_CheckpointBackedIsUnchanged(t *testing.T) {
	const boot = 1780411
	dag := newDeepScanFloorTestDAG(boot, true)

	if got := dag.deepScanFloor(); got != boot {
		t.Fatalf("checkpoint-backed floor = %d, want bootHeight %d unchanged", got, boot)
	}
}

// TestDeepScanFloor_YoungChainStillStartsAtGenesis covers the boundary: a
// chain shorter than the margin has its whole history inside one sweep
// anyway, and the subtraction must not go negative.
func TestDeepScanFloor_YoungChainStillStartsAtGenesis(t *testing.T) {
	for _, boot := range []int64{0, 1, plainRestartDeepScanMargin - 1, plainRestartDeepScanMargin} {
		dag := newDeepScanFloorTestDAG(boot, false)
		if got := dag.deepScanFloor(); got != 0 {
			t.Errorf("bootHeight %d: floor = %d, want 0 (never negative, and a young chain is one sweep regardless)", boot, got)
		}
	}
}

// TestDeepScanFloor_RemainsRecoverableByLowering is the argument that makes
// the higher starting floor safe at all: lowerDeepScanFloor must still be
// able to walk it down toward finalityFloorLimit when a full sweep from it
// reaches the peer's tip and STILL leaves blocks unmerged. Without that
// escape hatch a too-high floor would be a permanent wall rather than a
// starting guess.
func TestDeepScanFloor_RemainsRecoverableByLowering(t *testing.T) {
	const boot = 1781853
	const peer = "https://primary.example"
	dag := newDeepScanFloorTestDAG(boot, false)
	dag.deepScanFloorOverride = make(map[string]int64)
	dag.lastDeepScanAt = make(map[string]int64)

	start := dag.deepScanFloor()
	prev := start
	for i := 0; i < 6; i++ {
		dag.lowerDeepScanFloor(peer, prev)
		next := dag.effectiveDeepScanFloor(peer)
		if next >= prev {
			t.Fatalf("sweep %d: floor did not decrease (%d -> %d) — a too-high floor must stay recoverable", i, prev, next)
		}
		prev = next
	}
	if prev >= start/2 {
		t.Fatalf("after six sweeps the floor only reached %d from %d — halving toward finalityFloorLimit is not converging", prev, start)
	}
}
