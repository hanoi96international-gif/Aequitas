package keeper

import (
	"sync/atomic"
	"time"
)

// Phase timing for SaveBlockWithPendingTxsAtomic.
//
// WHY. Under 597-sender load this call was measured taking 1-3 seconds while
// holding dag.mu throughout — so block production fell to 9 blocks in 85
// seconds where 85 were due, and AddPeerBlock could not get the lock either,
// which is what piles up the orphans and collapses the DAG to a single tip.
// It is the binding constraint on throughput, ahead of CPU (2.88 of 6 cores)
// and ahead of signature verification.
//
// It contains two very different costs, and the fix differs completely
// depending on which dominates:
//
//   - json.Marshal of the transaction list. Megabytes under load, pure CPU,
//     and it needs no lock at all: the block is fully built by then. If this
//     dominates, moving it out of the critical section is a small, safe change.
//   - the INSERT itself, which under `ids` also deletes the block's pending
//     rows in one transaction. If this dominates, the fix is structural and
//     touches atomicity guarantees.
//
// Guessing between them is exactly what has gone wrong repeatedly in this
// project, so this measures it first.
var blockSaveStats struct {
	calls       atomic.Int64
	txs         atomic.Int64
	marshalNs   atomic.Int64
	dbNs        atomic.Int64
	maxTotalMs  atomic.Int64
	bytesMarsh  atomic.Int64
	pendingRows atomic.Int64
}

func noteBlockSave(txCount, pendingRows, marshalledBytes int, marshal, db time.Duration) {
	blockSaveStats.calls.Add(1)
	blockSaveStats.txs.Add(int64(txCount))
	blockSaveStats.pendingRows.Add(int64(pendingRows))
	blockSaveStats.bytesMarsh.Add(int64(marshalledBytes))
	blockSaveStats.marshalNs.Add(int64(marshal))
	blockSaveStats.dbNs.Add(int64(db))
	if ms := (marshal + db).Milliseconds(); ms > blockSaveStats.maxTotalMs.Load() {
		blockSaveStats.maxTotalMs.Store(ms)
	}
}

// BlockSaveStats reports the split. marshal_avg_ms against db_avg_ms is the
// whole question: the first is removable from under the lock cheaply, the
// second is not.
func BlockSaveStats() map[string]interface{} {
	n := blockSaveStats.calls.Load()
	out := map[string]interface{}{
		"calls":        n,
		"max_total_ms": blockSaveStats.maxTotalMs.Load(),
	}
	if n > 0 {
		out["txs_per_block"] = blockSaveStats.txs.Load() / n
		out["pending_rows_per_block"] = blockSaveStats.pendingRows.Load() / n
		out["marshalled_kb_per_block"] = (blockSaveStats.bytesMarsh.Load() / n) / 1024
		out["marshal_avg_ms"] = (blockSaveStats.marshalNs.Load() / n) / 1e6
		out["db_avg_ms"] = (blockSaveStats.dbNs.Load() / n) / 1e6
	}
	return out
}
