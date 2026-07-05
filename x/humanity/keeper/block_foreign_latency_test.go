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

// TestAddPeerBlock_MeasuresRawArrivalLatencyEvenWhenBreakerDropsBlock is the
// regression guard for the 2026-07-05 finding: recordForeignAttachLatency
// alone produced zero samples for exactly the direction that was failing
// (a node whose circuit breaker is open drops every foreign block before
// ever reaching that measurement point). The raw arrival measurement at
// AddPeerBlock's entry must fire regardless of what happens afterward —
// verified here against a block from a proposer whose breaker is already
// tripped (guaranteed rejection).
func TestAddPeerBlock_MeasuresRawArrivalLatencyEvenWhenBreakerDropsBlock(t *testing.T) {
	dag := newOrphanTestDAG()
	dag.state = &ChainState{}
	dag.selfProposer = "0xself"
	dag.proposerBreakerUntil = map[string]int64{"0xbadf00d": time.Now().Add(time.Hour).UnixNano()}
	// Prevent the zero-value log timestamp from immediately flushing (and
	// resetting) the accumulator this test is about to check — see the
	// identical seeding in TestRecordForeignAttachLatency_AccumulatesAndTracksMax.
	dag.lastRawArrivalLatencyLogAt.Store(time.Now().Unix())

	blk := &Block{Hash: "h1", Height: 1, Proposer: "0xbadf00d", ProducedAtMs: time.Now().UnixMilli() - 250}
	if dag.AddPeerBlock(blk) {
		t.Fatal("a block from a proposer whose breaker is already tripped must be rejected")
	}

	dag.rawArrivalLatencyMu.Lock()
	count := dag.rawArrivalLatencyCount
	dag.rawArrivalLatencyMu.Unlock()
	if count != 1 {
		t.Fatalf("rawArrivalLatencyCount = %d, want 1 — raw arrival latency must be measured even for a block the circuit breaker goes on to drop", count)
	}
}
