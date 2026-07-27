package keeper

import (
	"sync/atomic"
	"time"
)

// Where does a transfer actually spend its time?
//
// Everything measurable had been measured and none of it explained the ceiling:
// no lock contention (13 goroutines), no connection waits (wait_count 0), only
// 6% of CPU samples in database syscalls after nonce reservations were batched,
// and 244% of 600% available CPU — three and a half cores idle. Throughput sat
// at ~1,264/s regardless.
//
// The load generator drives 72 concurrent senders, and each of its JSON-RPC
// batches carries 100 transfers from ONE sender, which the dispatch loop must
// apply in order because their nonces are consecutive. So the concurrency the
// node ever sees is 72, and 1,264/s across 72 senders implies about 57ms per
// transfer. That number is a division, not a measurement — this makes it one.
//
// The path split matters just as much. TransferAtomic tries the WAL fast path
// first and falls back to the batcher, where a caller waits on a channel for
// its batch. 75 goroutines were sitting in chan receive, so the fallback is
// worth counting rather than assuming.

var (
	transferLatencyNanos atomic.Int64
	transferLatencyCount atomic.Int64
	transferLatencyMax   atomic.Int64
)

func recordTransferLatency(d time.Duration) {
	transferLatencyNanos.Add(int64(d))
	transferLatencyCount.Add(1)
	for {
		prev := transferLatencyMax.Load()
		if int64(d) <= prev || transferLatencyMax.CompareAndSwap(prev, int64(d)) {
			break
		}
	}
}

// TransferPathStats reports how transfers are routed and how long they take.
//
// fast_path_pct is the first thing to read: a low value means transfers are
// queueing in the batcher rather than taking the WAL path, which would explain
// both the chan-receive goroutines and the latency.
func TransferPathStats() map[string]interface{} {
	fast := transferFastPathApplied.Load()
	slow := transferFastPathFallback.Load()
	count := transferLatencyCount.Load()
	total := transferLatencyNanos.Load()

	out := map[string]interface{}{
		"fast_path_applied":  fast,
		"fallback_to_batch":  slow,
		"fast_path_pct":      0.0,
		"transfers_measured": count,
		"avg_latency_ms":     0.0,
		"max_latency_ms":     transferLatencyMax.Load() / int64(time.Millisecond),
	}
	if fast+slow > 0 {
		out["fast_path_pct"] = float64(fast) / float64(fast+slow) * 100
	}
	if count > 0 {
		// Milliseconds with one decimal: the interesting range here is single
		// digits versus tens, and integer milliseconds would hide that.
		out["avg_latency_ms"] = float64(total) / float64(count) / float64(time.Millisecond)
	}
	return out
}
