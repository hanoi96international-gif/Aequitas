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
// The table has a CREATE, an INSERT and a SELECT, and nothing that ever
// removes a row. Every produced block adds one: measured on Contabo2 under
// load at 1,786 rows for 1,079 MB, about 620 KB each. At one block per second
// that is roughly 50 GB per day on a 96 GB disk.
//
// This is not hypothetical on this machine. Contabo2 hit 100% earlier the same
// day from a different unbounded store (91 GB of Docker build cache, 474 MB
// free, with Postgres writing to it), and a node whose filesystem is full
// cannot commit or write its WAL.
//
// WHY PRUNING IS SAFE HERE. A batch exists so this node can serve a block's
// body to a peer that received the header only. stripBlocksForPeer already
// checks LoadTxBatch and sends the block WHOLE when the body is not available,
// so a pruned batch costs bandwidth and nothing else — it is exactly the
// behaviour that existed before bodies-by-reference. Nothing in consensus,
// replay or wallet lookups reads this table: transactions live in
// chain_blocks.transactions and are indexed in chain_tx_block_index
// independently.
//
// WHY A ROW CAP RATHER THAN AN AGE. Rows differ in size by three orders of
// magnitude between idle traffic and load, so an age-based window bounds
// nothing in the case that matters. A row count bounds the worst case
// directly, and the worst case is what filled the disk.

// txBatchKeepRows is how many of the most recent batches to keep. Sized for the
// range a catching-up peer actually asks for; older blocks simply travel whole.
const txBatchKeepRows = 10000

// txBatchPruneInterval is how often the sweep runs. Frequent enough that a
// sustained-load burst cannot outrun it by much, rare enough to be invisible.
const txBatchPruneInterval = 10 * time.Minute

var (
	txBatchPruneOnce sync.Once
	txBatchPruned    atomic.Int64
	txBatchPruneRuns atomic.Int64
)

// txBatchKeep reads the cap, allowing an operator to widen it on a node with
// room to spare or narrow it on one without.
func txBatchKeep() int {
	if v := os.Getenv("AEQUITAS_TX_BATCH_KEEP_ROWS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
		fmt.Printf("[TX-BATCH] ⚠ AEQUITAS_TX_BATCH_KEEP_ROWS=%q is not a positive integer — IGNORED, keeping the default %d\n", v, txBatchKeepRows)
	}
	return txBatchKeepRows
}

// ensureTxBatchPrunerStarted brings the sweep up on first use, so a node that
// never stores a batch never starts a goroutine.
func (cs *ChainState) ensureTxBatchPrunerStarted() {
	if cs.db == nil {
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

// pruneTxBatches deletes everything outside the newest txBatchKeep() rows.
//
// Deliberately not run inside any caller's transaction: this is housekeeping,
// and joining someone else's transaction would make an unrelated write wait on
// it. A failure is logged and retried at the next tick — the table being
// oversized for another ten minutes is not an incident.
func (cs *ChainState) pruneTxBatches() {
	if cs.db == nil {
		return
	}
	keep := txBatchKeep()
	res, err := cs.db.Exec(
		`DELETE FROM chain_tx_batches WHERE root NOT IN (
		   SELECT root FROM chain_tx_batches ORDER BY created_at DESC LIMIT $1
		 )`, keep)
	txBatchPruneRuns.Add(1)
	if err != nil {
		fmt.Printf("[TX-BATCH] ⚠ could not prune chain_tx_batches (table stays oversized until the next sweep): %v\n", err)
		return
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		txBatchPruned.Add(n)
		fmt.Printf("[TX-BATCH] pruned %d batch bodies, keeping the newest %d\n", n, keep)
	}
}

// TxBatchPruneStats reports what the sweep has removed.
func TxBatchPruneStats() map[string]interface{} {
	return map[string]interface{}{
		"rows_pruned": txBatchPruned.Load(),
		"sweeps":      txBatchPruneRuns.Load(),
		"keep_rows":   txBatchKeep(),
	}
}
