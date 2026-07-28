package keeper

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Bounding chain_tx_batches, which had no upper limit at all.
//
// The table has a CREATE, an INSERT and a SELECT, and nothing that ever removes
// a row. Every produced block adds one. Measured on Contabo2: 1,786 rows for
// 1,079 MB, about 620 KB each. At one block per second under load that is
// roughly 50 GB per day on a 96 GB disk.
//
// Not hypothetical on this machine. Contabo2 hit 100% earlier the same day from
// a different unbounded store — 91 GB of Docker build cache, 474 MB free, with
// Postgres writing to it — and the deploy that exposed it died with "no space
// left on device".
//
// WHY PRUNING IS SAFE. A batch exists so this node can serve a block's body to a
// peer that received the header only. stripBlocksForPeer already checks
// LoadTxBatch and sends the block WHOLE when the body is unavailable, so a
// pruned batch costs bandwidth and nothing else — exactly the behaviour that
// existed before bodies-by-reference. Nothing in consensus, replay or wallet
// lookups reads this table: transactions live in chain_blocks.transactions and
// are indexed in chain_tx_block_index independently.
//
// WHY A BYTE BUDGET AND NOT A ROW CAP. This shipped first as a row cap, and the
// numbers immediately showed why that was wrong: 1,786 rows already occupied
// 1,079 MB, so a 10,000-row cap would have permitted 6 GB while claiming to
// bound the table. Row sizes here differ by three orders of magnitude between
// idle traffic and load, which makes any count-based bound meaningless in
// exactly the case that fills the disk. Bytes are what runs out.
//
// Measured against live rows (sum of pg_column_size) rather than physical
// relation size: a DELETE does not return pages to the table until VACUUM runs,
// so a loop watching physical size would keep deleting until the table was
// empty.

// txBatchMaxBytes is the budget for live batch bodies. Generous enough for the
// range a catching-up peer actually asks for, small enough that it cannot crowd
// out Postgres on the volume this runs on.
const txBatchMaxBytes int64 = 1 << 30 // 1 GiB

// txBatchDeleteChunk bounds one DELETE so a sweep cannot lock a large part of
// the table at once.
const txBatchDeleteChunk = 200

// txBatchPruneInterval is how often the sweep runs. Frequent enough that a
// sustained-load burst cannot outrun it by much, rare enough to be invisible.
const txBatchPruneInterval = 10 * time.Minute

var (
	txBatchPruneOnce sync.Once
	txBatchPruned    atomic.Int64
	txBatchPruneRuns atomic.Int64
)

// txBatchKeep reads the byte budget, so an operator can widen it on a node with
// room to spare or narrow it on one without.
func txBatchKeep() int64 {
	if v := os.Getenv("AEQUITAS_TX_BATCH_MAX_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
		fmt.Printf("[TX-BATCH] ⚠ AEQUITAS_TX_BATCH_MAX_BYTES=%q is not a positive integer — IGNORED, keeping the default %d\n", v, txBatchMaxBytes)
	}
	return txBatchMaxBytes
}

// StartTxBatchPruner brings the sweep up.
//
// Called from node startup rather than from SaveTxBatch, which is where it
// first sat: SaveTxBatch returns early for a block carrying no transactions, so
// on an idle chain the pruner never started — precisely when an oversized table
// would sit there untouched. A janitor that only runs while the system is busy
// is not a janitor.
func (cs *ChainState) StartTxBatchPruner() {
	if cs == nil || cs.db == nil {
		return
	}
	txBatchPruneOnce.Do(func() {
		SafeGoroutine("txBatchPruner", func() {
			// One sweep at startup clears whatever a previous process left
			// behind, then settle into the interval.
			cs.pruneTxBatches()
			t := time.NewTicker(txBatchPruneInterval)
			defer t.Stop()
			for range t.C {
				cs.pruneTxBatches()
			}
		})
	})
}

// pruneTxBatches drops the oldest bodies until live bytes fit the budget.
//
// Deliberately outside any caller's transaction: this is housekeeping, and
// joining someone else's transaction would make an unrelated write wait on it.
// A failure is logged and retried at the next tick — the table being oversized
// for another ten minutes is not an incident.
func (cs *ChainState) pruneTxBatches() {
	if cs.db == nil {
		return
	}
	budget := txBatchKeep()
	txBatchPruneRuns.Add(1)

	// Bounded so a pathological state cannot spin here forever; whatever is
	// left over is picked up at the next tick.
	for i := 0; i < 200; i++ {
		var live int64
		if err := cs.db.QueryRow(`SELECT COALESCE(sum(pg_column_size(txs)),0) FROM chain_tx_batches`).Scan(&live); err != nil {
			fmt.Printf("[TX-BATCH] ⚠ could not size chain_tx_batches (stays oversized until the next sweep): %v\n", err)
			return
		}
		if live <= budget {
			return
		}
		res, err := cs.db.Exec(
			`DELETE FROM chain_tx_batches WHERE root IN (
			   SELECT root FROM chain_tx_batches ORDER BY created_at ASC LIMIT $1
			 )`, txBatchDeleteChunk)
		if err != nil {
			fmt.Printf("[TX-BATCH] ⚠ could not prune chain_tx_batches: %v\n", err)
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			// Over budget with nothing left to drop means a single body is
			// larger than the whole budget. Deleting further would empty the
			// table for no gain, so stop and say so rather than thrash.
			fmt.Printf("[TX-BATCH] ⚠ %d bytes of bodies exceed the %d-byte budget but nothing older remains to drop\n", live, budget)
			return
		}
		txBatchPruned.Add(n)
		fmt.Printf("[TX-BATCH] pruned %d batch bodies (%d bytes live, budget %d)\n", n, live, budget)
	}
}

// TxBatchPruneStats reports what the sweep has removed.
func TxBatchPruneStats() map[string]interface{} {
	return map[string]interface{}{
		"rows_pruned":  txBatchPruned.Load(),
		"sweeps":       txBatchPruneRuns.Load(),
		"budget_bytes": txBatchKeep(),
	}
}
