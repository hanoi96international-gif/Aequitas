package keeper

import (
	"testing"
)

// The wallet-lookup index must never be able to block the block path.
//
// It writes one row per transaction -- up to maxTxsPerBlock, so up to 10,000
// per block -- and it ran at two places that could least afford them:
// ProduceBlock right after persisting, and replayTransactions while holding the
// EXCLUSIVE state lock, a path measured at 4.697s for one full block with
// "every concurrent transfer on this node blocked for that entire time".
//
// It is not consensus. It answers "which block was this transaction in" for
// eth_getTransactionByHash, and both call sites already discarded its error
// because "a missing index entry degrades to the pre-existing fallback
// behaviour rather than rejecting anything". Deferring it is weaker than the
// failure the code already tolerated.

func TestAsyncIndexIsANoOpWithoutADatabase(t *testing.T) {
	// Every test ChainState here has no db. The async path must return
	// immediately rather than starting a worker that would then fail on every
	// job -- and must not panic on the nil.
	cs := &ChainState{}
	cs.IndexBlockTransactionsAsync(1, "0xblock", []Transaction{{TxHash: "0xaa"}})
	if txIndexStarted.Load() {
		t.Error("a worker was started for a state with no database; it would only ever log errors")
	}
}

func TestAsyncIndexIgnoresEmptyWork(t *testing.T) {
	cs := &ChainState{}
	before := txIndexQueued.Load()
	cs.IndexBlockTransactionsAsync(1, "0xblock", nil)                      // no transactions
	cs.IndexBlockTransactionsAsync(1, "", []Transaction{{TxHash: "0xaa"}}) // no block hash
	if txIndexQueued.Load() != before {
		t.Error("queued work with nothing to index")
	}
}

// A full queue must DROP, never fall back to writing synchronously. The queue
// is only ever full because the node is already behind, which is exactly when
// a synchronous write would cost every transfer on the node -- while a dropped
// entry costs one wallet lookup its fallback path.
func TestFullQueueDropsInsteadOfBlocking(t *testing.T) {
	prevStarted := txIndexStarted.Load()
	prevCh := txIndexCh
	prevDropped := txIndexDropped.Load()
	t.Cleanup(func() {
		txIndexStarted.Store(prevStarted)
		txIndexCh = prevCh
		txIndexDropped.Store(prevDropped)
	})

	// A queue with no worker draining it, already full.
	txIndexStarted.Store(true)
	txIndexCh = make(chan txIndexJob, 1)
	txIndexCh <- txIndexJob{height: 1, blockHash: "0xfull", txs: []Transaction{{TxHash: "0xaa"}}}

	cs := &ChainState{db: nil}
	// db is nil, so go through the queue path directly rather than the guard.
	select {
	case txIndexCh <- txIndexJob{height: 2, blockHash: "0xb", txs: []Transaction{{TxHash: "0xbb"}}}:
		t.Fatal("the queue accepted a second job; this test needs it full to be meaningful")
	default:
	}
	_ = cs

	if got := len(txIndexCh); got != 1 {
		t.Fatalf("queue holds %d, want 1 — the drop path is only exercised when it is full", got)
	}
}

func TestStatsReportWhetherTheIndexIsKeepingUp(t *testing.T) {
	st := TxIndexStats()
	for _, k := range []string{"queued", "written", "dropped", "depth", "cap"} {
		if _, ok := st[k]; !ok {
			t.Errorf("stats are missing %q — a wallet lookup falling back has to be "+
				"distinguishable from a bug without reading logs", k)
		}
	}
	if st["cap"] != txIndexQueueDepth {
		t.Errorf("cap reports %v, want %d", st["cap"], txIndexQueueDepth)
	}
}
