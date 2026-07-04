package keeper

import "testing"

// TestDeepScanFloor_CheckpointBackedUsesBootHeight is the regression guard
// for the 2026-07-04 permanent-non-convergence incident, found live with
// instrumented diagnostics: /api/blocks?min_height=N is EXCLUSIVE
// (Height > N). Using BootHeight() as deepScan's floor is only sound when a
// checkpoint-seeded resync already placed a real, verified block at that
// exact height — dag.blocks' anchor for the first fetched block above it
// to attach to. This test locks in the fast, safe path for that case.
func TestDeepScanFloor_CheckpointBackedUsesBootHeight(t *testing.T) {
	dag := &BlockDAG{bootHeight: 12345, bootHeightCheckpointBacked: true}
	if got := dag.deepScanFloor(); got != 12345 {
		t.Fatalf("checkpoint-backed deepScanFloor() = %d, want BootHeight() = 12345", got)
	}
}

// TestDeepScanFloor_NotCheckpointBackedFallsBackToGenesis is the core
// regression guard: confirmed live via a byte-for-byte reproduction against
// the real primary — a plain-restart node's BootHeight (182445) was
// EXACTLY the peer's real common-ancestor height. Every block fetched
// above it was present and correctly height-ordered in the response, yet
// AddPeerBlock still rejected all of them: the one boundary block itself
// could never be fetched (min_height's exclusive semantics permanently
// excluded it), so the gap could never close no matter how many deepScan
// passes ran. Without checkpoint backing, the floor must fall back to 0
// (deepScan's original pre-checkpoint-seeding behavior) so a genuinely
// isolated node can still find its real common ancestor, however deep.
func TestDeepScanFloor_NotCheckpointBackedFallsBackToGenesis(t *testing.T) {
	dag := &BlockDAG{bootHeight: 182445, bootHeightCheckpointBacked: false}
	if got := dag.deepScanFloor(); got != 0 {
		t.Fatalf("non-checkpoint-backed deepScanFloor() = %d, want 0 (genesis walk) — using BootHeight() here permanently excludes the exact height most likely to be the real common ancestor", got)
	}
}
