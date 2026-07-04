package keeper

import (
	"testing"
	"time"
)

// withBreakerTuningReset saves proposerBreakerFailThreshold and
// proposerBreakerOrphanGrace and restores both after the test, since
// TuneProposerBreakerForBlockTime mutates them together in one call — a
// test that only restores the one value it's specifically checking would
// leak the other's mutation into whichever test runs next.
func withBreakerTuningReset(t *testing.T) {
	t.Helper()
	origThreshold := proposerBreakerFailThreshold
	origGrace := proposerBreakerOrphanGrace
	t.Cleanup(func() {
		proposerBreakerFailThreshold = origThreshold
		proposerBreakerOrphanGrace = origGrace
	})
}

// TestTuneProposerBreakerForBlockTime_ScalesUpAtFasterCadence is the
// regression guard for the 2026-07-04 finding: proposerBreakerFailThreshold
// was a bare 40, implicitly tuned against a 2s BLOCK_TIME (see its own
// comment) — the wall-clock time to accumulate 40 failures shrinks in
// direct proportion to BLOCK_TIME, so a much faster cadence (confirmed
// live: 1.5s) tripped the breaker during perfectly ordinary propagation
// jitter, well before genuine divergence could be distinguished from it.
// At half the tuning baseline (1s vs. the 2s baseline), the threshold must
// scale up by 2x speedup * the 3x extra safety factor (2026-07-05 — the
// first-pass exact-2x scaling still wasn't enough; confirmed live: Contabo
// 2 kept tripping against both other validators at 1s even at threshold 80).
func TestTuneProposerBreakerForBlockTime_ScalesUpAtFasterCadence(t *testing.T) {
	withBreakerTuningReset(t)
	TuneProposerBreakerForBlockTime(1 * time.Second)
	if proposerBreakerFailThreshold != 240 {
		t.Fatalf("proposerBreakerFailThreshold = %d after tuning for 1s BLOCK_TIME, want 240 (2x speedup * 3x extra safety * baseline 40)", proposerBreakerFailThreshold)
	}
}

// TestTuneProposerBreakerForBlockTime_GraceWidensAtFasterCadence verifies
// proposerBreakerOrphanGrace also widens at a faster-than-baseline
// BLOCK_TIME — real cross-provider propagation+processing delay is a
// roughly fixed wall-clock quantity, so a faster cadence needs MORE grace
// in block-count terms to cover the same fixed real-world delay.
func TestTuneProposerBreakerForBlockTime_GraceWidensAtFasterCadence(t *testing.T) {
	withBreakerTuningReset(t)
	TuneProposerBreakerForBlockTime(1 * time.Second)
	if proposerBreakerOrphanGrace != 48*time.Second {
		t.Fatalf("proposerBreakerOrphanGrace = %v after tuning for 1s BLOCK_TIME, want 48s (2x speedup * 3x extra safety * baseline 8s)", proposerBreakerOrphanGrace)
	}
}

// TestTuneProposerBreakerForBlockTime_GraceNeverLowersBelowOriginal mirrors
// the threshold's own never-lowers guarantee for the grace period.
func TestTuneProposerBreakerForBlockTime_GraceNeverLowersBelowOriginal(t *testing.T) {
	withBreakerTuningReset(t)
	TuneProposerBreakerForBlockTime(6 * time.Second)
	if proposerBreakerOrphanGrace != 8*time.Second {
		t.Fatalf("proposerBreakerOrphanGrace = %v after tuning for a slower-than-baseline 6s BLOCK_TIME, want unchanged at 8s", proposerBreakerOrphanGrace)
	}
}

// TestTuneProposerBreakerForBlockTime_NeverLowersBelowOriginal verifies a
// slower-than-baseline BLOCK_TIME (e.g. the 6s value proven stable for
// hours) does not shrink the threshold below its original, already-proven
// 40 — this fix must only ever make the breaker MORE forgiving at faster
// cadence, never less forgiving at a slower one.
func TestTuneProposerBreakerForBlockTime_NeverLowersBelowOriginal(t *testing.T) {
	withBreakerTuningReset(t)
	TuneProposerBreakerForBlockTime(6 * time.Second)
	if proposerBreakerFailThreshold != 40 {
		t.Fatalf("proposerBreakerFailThreshold = %d after tuning for a slower-than-baseline 6s BLOCK_TIME, want unchanged at 40", proposerBreakerFailThreshold)
	}
}

// TestTuneProposerBreakerForBlockTime_AtBaselineIsNoOp verifies tuning at
// exactly the original 2s baseline leaves both values at their already-
// proven defaults — this fix must be a strict no-op for the cadence every
// other breaker constant in this file was already tuned against.
func TestTuneProposerBreakerForBlockTime_AtBaselineIsNoOp(t *testing.T) {
	withBreakerTuningReset(t)
	TuneProposerBreakerForBlockTime(2 * time.Second)
	if proposerBreakerFailThreshold != 40 {
		t.Fatalf("proposerBreakerFailThreshold = %d after tuning at the 2s baseline, want unchanged at 40", proposerBreakerFailThreshold)
	}
	if proposerBreakerOrphanGrace != 8*time.Second {
		t.Fatalf("proposerBreakerOrphanGrace = %v after tuning at the 2s baseline, want unchanged at 8s", proposerBreakerOrphanGrace)
	}
}
