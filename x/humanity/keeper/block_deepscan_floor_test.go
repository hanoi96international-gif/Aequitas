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

// TestDeepScanFloor_NotCheckpointBackedStaysBelowBootHeight is the core
// regression guard: confirmed live via a byte-for-byte reproduction against
// the real primary — a plain-restart node's BootHeight (182445) was
// EXACTLY the peer's real common-ancestor height. Every block fetched
// above it was present and correctly height-ordered in the response, yet
// AddPeerBlock still rejected all of them: the one boundary block itself
// could never be fetched (min_height's exclusive semantics permanently
// excluded it), so the gap could never close no matter how many deepScan
// passes ran.
//
// UPDATED (2026-07-24): the invariant this incident actually requires is
// that the floor sits STRICTLY BELOW BootHeight, so the boundary block at
// BootHeight is itself fetchable. It does NOT require the floor to be 0 —
// and asserting 0 turned the deepest possible search into the default for
// the most ordinary event there is, a plain restart. That cost was measured
// live on Contabo1 the same evening: merging normally for 27 minutes, then
// restarted by a deploy and immediately back to zero attaches at a frozen
// height, re-walking ~1.78M blocks from genesis. deepScanFloor now starts
// just below BootHeight instead, which keeps this incident closed (the
// boundary block is in range) while making a redeploy one short sweep;
// lowerDeepScanFloor still halves the floor toward finalityFloorLimit on
// any sweep that reaches the peer's tip with blocks left unmerged, so a
// genuinely deep common ancestor is still reachable — see
// TestDeepScanFloor_RemainsRecoverableByLowering.
func TestDeepScanFloor_NotCheckpointBackedStaysBelowBootHeight(t *testing.T) {
	const boot = 182445
	dag := &BlockDAG{bootHeight: boot, bootHeightCheckpointBacked: false}
	got := dag.deepScanFloor()
	if got >= boot {
		t.Fatalf("non-checkpoint-backed deepScanFloor() = %d, want strictly below BootHeight %d — min_height is EXCLUSIVE, so a floor at BootHeight permanently excludes the exact height most likely to be the real common ancestor", got, boot)
	}
	if got == 0 {
		t.Fatalf("deepScanFloor() = 0 — a plain restart must not re-walk from genesis by default; that is the 2026-07-24 redeploy incident")
	}
}
