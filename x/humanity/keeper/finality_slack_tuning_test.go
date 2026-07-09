package keeper

import (
	"testing"
	"time"
)

func withFinalitySlackTuningReset(t *testing.T) {
	t.Helper()
	origDepth := finalityBlueScoreDepth
	origSlack := finalityHeightSlack
	t.Cleanup(func() {
		finalityBlueScoreDepth = origDepth
		finalityHeightSlack = origSlack
	})
}

// TestTuneFinalitySlackForBlockTime_ScalesUpAtFasterCadence is the
// regression guard for the live 2026-07-09/10 3-node non-convergence
// incident: finalityHeightSlack/finalityBlueScoreDepth were the one
// constant pair the 2026-07-04 BLOCK_TIME scaling pass missed. At
// BLOCK_TIME=1s the bare 50 gave only 50 seconds of tolerance before
// isFinalityViolation permanently (not just temporarily, unlike the
// proposer breaker) rejects a legitimate peer block — confirmed live to be
// far below the real measured cross-provider attach latency (100+
// seconds). Uses the identical scaling formula as
// TuneProposerBreakerForBlockTime for the same 2s baseline / 3x safety
// factor, so 1s BLOCK_TIME must produce the same 2x*3x=6x multiplier: 50 *
// 6 = 300.
func TestTuneFinalitySlackForBlockTime_ScalesUpAtFasterCadence(t *testing.T) {
	withFinalitySlackTuningReset(t)
	TuneFinalitySlackForBlockTime(1 * time.Second)
	if finalityHeightSlack != 300 {
		t.Fatalf("finalityHeightSlack = %d after tuning for 1s BLOCK_TIME, want 300 (2x speedup * 3x extra safety * baseline 50)", finalityHeightSlack)
	}
	if finalityBlueScoreDepth != 300 {
		t.Fatalf("finalityBlueScoreDepth = %d after tuning for 1s BLOCK_TIME, want 300 (2x speedup * 3x extra safety * baseline 50)", finalityBlueScoreDepth)
	}
}

// TestTuneFinalitySlackForBlockTime_NeverLowersBelowOriginal mirrors the
// proposer breaker's own never-lowers guarantee: a slower-than-baseline
// BLOCK_TIME must keep the original, already-proven 50 rather than
// shrinking it (which would make finality MORE trigger-happy, not less).
func TestTuneFinalitySlackForBlockTime_NeverLowersBelowOriginal(t *testing.T) {
	withFinalitySlackTuningReset(t)
	TuneFinalitySlackForBlockTime(6 * time.Second)
	if finalityHeightSlack != 50 {
		t.Fatalf("finalityHeightSlack = %d after tuning for a slower-than-baseline 6s BLOCK_TIME, want unchanged at 50", finalityHeightSlack)
	}
	if finalityBlueScoreDepth != 50 {
		t.Fatalf("finalityBlueScoreDepth = %d after tuning for a slower-than-baseline 6s BLOCK_TIME, want unchanged at 50", finalityBlueScoreDepth)
	}
}

// TestTuneFinalitySlackForBlockTime_AtBaselineIsNoOp verifies tuning at
// exactly the 2s baseline every other breaker constant in this file was
// tuned against leaves both values unchanged.
func TestTuneFinalitySlackForBlockTime_AtBaselineIsNoOp(t *testing.T) {
	withFinalitySlackTuningReset(t)
	TuneFinalitySlackForBlockTime(2 * time.Second)
	if finalityHeightSlack != 50 {
		t.Fatalf("finalityHeightSlack = %d after tuning at the 2s baseline, want unchanged at 50", finalityHeightSlack)
	}
}
