package keeper

import (
	"sync/atomic"
	"time"
)

// Counters for the batch nonce pre-reservation, reported next to the cost it
// is meant to remove.
//
// Read `covered_pct` first. The optimisation only applies to runs of
// consecutive nonces from one sender, so a client that batches differently
// gets none of it — and in that case nonce_ms staying at 19.6 ms is the
// correct outcome, not a regression to chase.

var (
	batchNonceRuns     atomic.Int64 // range reservations performed
	batchNonceCovered  atomic.Int64 // transactions covered by them
	batchNonceNanos    atomic.Int64 // time spent in those reservations
	batchNonceLongest  atomic.Int64 // longest run reserved at once
	batchNonceFallback atomic.Int64 // transactions left to the per-item path
)

func noteBatchNonceRun(run int, d time.Duration) {
	batchNonceRuns.Add(1)
	batchNonceCovered.Add(int64(run))
	batchNonceNanos.Add(int64(d))
	for {
		prev := batchNonceLongest.Load()
		if int64(run) <= prev || batchNonceLongest.CompareAndSwap(prev, int64(run)) {
			break
		}
	}
}

func noteBatchNonceFallback() { batchNonceFallback.Add(1) }

// BatchNonceStats reports how much of the nonce cost the range reservation
// actually removed.
func BatchNonceStats() map[string]interface{} {
	runs := batchNonceRuns.Load()
	covered := batchNonceCovered.Load()
	fell := batchNonceFallback.Load()

	out := map[string]interface{}{
		"range_reservations": runs,
		"txs_covered":        covered,
		"txs_per_item_path":  fell,
		"longest_run":        batchNonceLongest.Load(),
		"covered_pct":        0.0,
		"avg_run":            0.0,
		// Per RANGE, not per transaction: this is the one round trip that
		// replaced up to `longest_run` of them.
		"reservation_ms": msPer(batchNonceNanos.Load(), runs),
	}
	if total := covered + fell; total > 0 {
		out["covered_pct"] = float64(covered) / float64(total) * 100
	}
	if runs > 0 {
		out["avg_run"] = float64(covered) / float64(runs)
	}
	return out
}
