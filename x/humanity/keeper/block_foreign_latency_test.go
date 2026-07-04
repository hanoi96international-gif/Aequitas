package keeper

import (
	"testing"
	"time"
)

// TestRecordForeignAttachLatency_AccumulatesAndTracksMax verifies the
// accumulator correctly sums samples and tracks the running max between log
// flushes — the pure bookkeeping half of recordForeignAttachLatency,
// exercised without waiting for the real 30s log interval.
func TestRecordForeignAttachLatency_AccumulatesAndTracksMax(t *testing.T) {
	dag := &BlockDAG{}
	// A freshly-constructed dag's lastForeignLatencyLogAt is the zero value,
	// which the rate-limit check treats as "long overdue" and flushes
	// immediately on the very first sample (correct real-world behavior —
	// show the first reading right away rather than waiting a full
	// interval). Seed it to "now" so this test can observe several samples
	// accumulate within the same window instead.
	dag.lastForeignLatencyLogAt.Store(time.Now().Unix())
	dag.recordForeignAttachLatency(100)
	dag.recordForeignAttachLatency(400)
	dag.recordForeignAttachLatency(250)

	dag.foreignLatencyMu.Lock()
	count, sum, maxMs := dag.foreignLatencyCount, dag.foreignLatencySumMs, dag.foreignLatencyMaxMs
	dag.foreignLatencyMu.Unlock()

	if count != 3 {
		t.Fatalf("foreignLatencyCount = %d, want 3", count)
	}
	if sum != 750 {
		t.Fatalf("foreignLatencySumMs = %d, want 750 (100+400+250)", sum)
	}
	if maxMs != 400 {
		t.Fatalf("foreignLatencyMaxMs = %d, want 400 (the largest sample)", maxMs)
	}
}

// TestRecordForeignAttachLatency_LogFlushResetsAccumulator verifies that
// once the rate-limit gate opens (simulated by backdating
// lastForeignLatencyLogAt), the next sample triggers a flush that resets
// count/sum/max back to zero for the next window.
func TestRecordForeignAttachLatency_LogFlushResetsAccumulator(t *testing.T) {
	dag := &BlockDAG{}
	dag.recordForeignAttachLatency(100)
	dag.lastForeignLatencyLogAt.Store(0) // force the next sample to flush

	dag.recordForeignAttachLatency(200)

	dag.foreignLatencyMu.Lock()
	count, sum, maxMs := dag.foreignLatencyCount, dag.foreignLatencySumMs, dag.foreignLatencyMaxMs
	dag.foreignLatencyMu.Unlock()

	if count != 0 || sum != 0 || maxMs != 0 {
		t.Fatalf("after a log flush the accumulator must reset to zero, got count=%d sum=%d max=%d", count, sum, maxMs)
	}
}
