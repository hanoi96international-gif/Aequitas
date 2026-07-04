package keeper

import (
	"testing"
	"time"
)

// TestTuneProposerBreakerForBlockTime_ScalesUpAtFasterCadence is the
// regression guard for the 2026-07-04 finding: proposerBreakerFailThreshold
// was a bare 40, implicitly tuned against a 2s BLOCK_TIME (see its own
// comment) — the wall-clock time to accumulate 40 failures shrinks in
// direct proportion to BLOCK_TIME, so a much faster cadence (confirmed
// live: 1.5s) tripped the breaker during perfectly ordinary propagation
// jitter, well before genuine divergence could be distinguished from it.
// At half the tuning baseline (1s vs. the 2s baseline), the threshold must
// roughly double to preserve the same wall-clock exposure window.
func TestTuneProposerBreakerForBlockTime_ScalesUpAtFasterCadence(t *testing.T) {
	original := proposerBreakerFailThreshold
	defer func() { proposerBreakerFailThreshold = original }()

	TuneProposerBreakerForBlockTime(1 * time.Second)
	if proposerBreakerFailThreshold < 79 || proposerBreakerFailThreshold > 81 {
		t.Fatalf("proposerBreakerFailThreshold = %d after tuning for 1s BLOCK_TIME, want ~80 (2x the 2s-baseline 40)", proposerBreakerFailThreshold)
	}
}

// TestTuneProposerBreakerForBlockTime_NeverLowersBelowOriginal verifies a
// slower-than-baseline BLOCK_TIME (e.g. the 6s value proven stable for
// hours) does not shrink the threshold below its original, already-proven
// 40 — this fix must only ever make the breaker MORE forgiving at faster
// cadence, never less forgiving at a slower one.
func TestTuneProposerBreakerForBlockTime_NeverLowersBelowOriginal(t *testing.T) {
	original := proposerBreakerFailThreshold
	defer func() { proposerBreakerFailThreshold = original }()

	TuneProposerBreakerForBlockTime(6 * time.Second)
	if proposerBreakerFailThreshold != 40 {
		t.Fatalf("proposerBreakerFailThreshold = %d after tuning for a slower-than-baseline 6s BLOCK_TIME, want unchanged at 40", proposerBreakerFailThreshold)
	}
}

// TestTuneProposerBreakerForBlockTime_AtBaselineIsNoOp verifies tuning at
// exactly the original 2s baseline leaves the threshold at its already-
// proven value — this fix must be a strict no-op for the cadence every
// other breaker constant in this file was already tuned against.
func TestTuneProposerBreakerForBlockTime_AtBaselineIsNoOp(t *testing.T) {
	original := proposerBreakerFailThreshold
	defer func() { proposerBreakerFailThreshold = original }()

	TuneProposerBreakerForBlockTime(2 * time.Second)
	if proposerBreakerFailThreshold != 40 {
		t.Fatalf("proposerBreakerFailThreshold = %d after tuning at the 2s baseline, want unchanged at 40", proposerBreakerFailThreshold)
	}
}
