package keeper

import (
	"fmt"
	"sync/atomic"
)

// Keep the wallet-lookup index off the block path.
//
// WHAT IT COSTS THERE. IndexBlockTransactions writes one row per transaction
// in the block. At maxTxsPerBlock that is up to 10,000 INSERTs, and it runs at
// two places that can least afford them:
//
//   - ProduceBlock, right after the block is persisted
//   - replayTransactions, while the EXCLUSIVE state lock is held. That path
//     was measured holding the lock for 4.697s on a full block, with its own
//     log line saying "every concurrent transfer on this node was blocked for
//     that entire time".
//
// chain_tx_block_index is the largest table on the box at 2,228 MB, and the
// day's measurements put disk I/O as the binding constraint on throughput:
// CPU sits at about one core of six, GC accounts for 0.15% of wall time, four
// separate lock-side changes moved nothing, and an independent fsync probe
// sharing only the device halved its own rate under load (305 -> 151/s). The
// node saturates its own disk, and this is one of the largest writers doing it.
//
// WHY IT IS SAFE TO DEFER. The index is not consensus. It answers "which block
// was this transaction in" for eth_getTransactionByHash and
// eth_getTransactionReceipt. Both call sites already ignore its error, and
// both say why: "A failure here is logged, not fatal -- the block itself is
// valid and committed, and a missing index entry degrades to the pre-existing
// fallback behaviour rather than rejecting anything." Deferring it by a moment
// is strictly weaker than the failure the code already tolerates.
//
// WHY IT DROPS RATHER THAN BLOCKS WHEN FULL. Falling back to a synchronous
// write when the queue is full would reintroduce the stall at exactly the
// moment it hurts most -- the queue is only ever full because the node is
// already behind. A dropped entry costs one wallet lookup the fallback path;
// a synchronous write under load costs every transfer on the node.
//
// Ordering is preserved per block because one worker consumes the queue in
// order. The index is keyed by tx_hash with ON CONFLICT DO NOTHING, so a
// replay of the same block is idempotent either way.

type txIndexJob struct {
	height    int64
	blockHash string
	txs       []Transaction
}

// txIndexQueueDepth is measured in BLOCKS, not rows. A handful is enough to
// absorb a burst; more would only let the index fall further behind the chain
// while consuming memory holding transaction slices.
const txIndexQueueDepth = 32

var (
	txIndexCh      chan txIndexJob
	txIndexStarted atomic.Bool
	txIndexDropped atomic.Int64
	txIndexQueued  atomic.Int64
	txIndexWritten atomic.Int64
)

// IndexBlockTransactionsAsync hands the work to a background worker and
// returns immediately. Never blocks the caller.
func (cs *ChainState) IndexBlockTransactionsAsync(height int64, blockHash string, txs []Transaction) {
	if cs.db == nil || len(txs) == 0 || blockHash == "" {
		return
	}
	cs.ensureTxIndexWorker()
	select {
	case txIndexCh <- txIndexJob{height: height, blockHash: blockHash, txs: txs}:
		txIndexQueued.Add(1)
	default:
		// Full: the writer is behind. Drop, and say so once per block rather
		// than per row.
		n := txIndexDropped.Add(1)
		fmt.Printf("[TX-INDEX] queue full — not indexing block #%d (%d dropped so far). "+
			"eth_getTransactionByHash falls back for these; the chain is unaffected\n", height, n)
	}
}

func (cs *ChainState) ensureTxIndexWorker() {
	if txIndexStarted.Swap(true) {
		return
	}
	txIndexCh = make(chan txIndexJob, txIndexQueueDepth)
	SafeGoroutine("txBlockIndexWriter", func() {
		for job := range txIndexCh {
			if err := cs.IndexBlockTransactions(job.height, job.blockHash, job.txs); err != nil {
				fmt.Printf("[TX-INDEX] ⚠ could not index block #%d for wallet lookups: %v\n",
					job.height, err)
				continue
			}
			txIndexWritten.Add(1)
		}
	})
}

// TxIndexStats reports whether the index is keeping up, so a wallet lookup
// falling back can be told apart from a bug.
func TxIndexStats() map[string]interface{} {
	return map[string]interface{}{
		"queued":  txIndexQueued.Load(),
		"written": txIndexWritten.Load(),
		"dropped": txIndexDropped.Load(),
		"depth":   len(txIndexCh),
		"cap":     txIndexQueueDepth,
	}
}
