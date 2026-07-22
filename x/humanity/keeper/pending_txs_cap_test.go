package keeper

import (
	"fmt"
	"testing"
)

// TestLoadPendingTxs_CapsAtMaxTxsPerBlock is the correctness proof for
// SCALING_ARCHITECTURE.md Phase 9's per-block transaction cap: with more
// than maxTxsPerBlock rows genuinely pending, a single LoadPendingTxs call
// must return AT MOST maxTxsPerBlock of them -- never the whole backlog in
// one shot, which is exactly the unbounded-block risk TestBlockCostAtScale
// measured the cost of.
func TestLoadPendingTxs_CapsAtMaxTxsPerBlock(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	cs := NewChainState("unused-pending-txs-cap-test-1.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}
	drainAllPendingTxs(t, cs)

	const total = maxTxsPerBlock + 500
	for i := 0; i < total; i++ {
		tx := Transaction{Type: "transfer", Wallet: "0xcap0000000000000000000000000000000c001", To: "0xcap0000000000000000000000000000000c002", Amount: 1, TxHash: fmt.Sprintf("0xcap-%d", i)}
		if err := cs.SavePendingTx(tx); err != nil {
			t.Fatalf("SavePendingTx %d: %v", i, err)
		}
	}

	txs, ids := cs.LoadPendingTxs()
	if len(txs) > maxTxsPerBlock {
		t.Fatalf("LoadPendingTxs returned %d transactions, want at most maxTxsPerBlock=%d", len(txs), maxTxsPerBlock)
	}
	if len(txs) != maxTxsPerBlock {
		t.Fatalf("with %d genuinely pending (more than the cap), expected exactly maxTxsPerBlock=%d on this call, got %d", total, maxTxsPerBlock, len(txs))
	}
	if len(ids) != len(txs) {
		t.Fatalf("ids/txs length mismatch: %d ids, %d txs", len(ids), len(txs))
	}

	// The remainder must still be there, untouched, for the NEXT call --
	// no transaction may ever be silently dropped by the cap, only deferred.
	txs2, _ := cs.LoadPendingTxs()
	wantRemainder := total - maxTxsPerBlock
	if len(txs2) != wantRemainder {
		t.Fatalf("second LoadPendingTxs call: got %d, want the exact remainder %d (%d total - %d cap) -- a TX was lost or duplicated by the cap", len(txs2), wantRemainder, total, maxTxsPerBlock)
	}

	// A third call must find nothing left.
	txs3, _ := cs.LoadPendingTxs()
	if len(txs3) != 0 {
		t.Fatalf("third LoadPendingTxs call: got %d, want 0 (backlog should be fully drained)", len(txs3))
	}
}

// TestLoadPendingTxs_UnderCapReturnsAll proves the cap never holds back
// transactions when there are fewer than maxTxsPerBlock pending -- the
// common case at realistic (non-50k-TPS) load must be completely
// unaffected by this change.
func TestLoadPendingTxs_UnderCapReturnsAll(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	cs := NewChainState("unused-pending-txs-cap-test-2.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}
	drainAllPendingTxs(t, cs)

	const total = 25 // comfortably under maxTxsPerBlock
	for i := 0; i < total; i++ {
		tx := Transaction{Type: "transfer", Wallet: "0xcap0000000000000000000000000000000c101", To: "0xcap0000000000000000000000000000000c102", Amount: 1, TxHash: fmt.Sprintf("0xcap-under-%d", i)}
		if err := cs.SavePendingTx(tx); err != nil {
			t.Fatalf("SavePendingTx %d: %v", i, err)
		}
	}

	txs, _ := cs.LoadPendingTxs()
	if len(txs) != total {
		t.Fatalf("got %d transactions, want exactly %d -- the cap must never hold back a small backlog", len(txs), total)
	}
}

// drainAllPendingTxs repeatedly calls LoadPendingTxs until the backlog is
// empty, giving a test a clean starting point against the shared bench DB
// (which otherwise accumulates pending_txs rows across separate test runs,
// same reasoning as pool_flush_test.go/evm_mirror_flush_test.go's own
// leftover-state fixes).
func drainAllPendingTxs(t *testing.T, cs *ChainState) {
	t.Helper()
	for i := 0; i < 100; i++ { // hard cap so a real bug here fails fast, not hangs
		txs, _ := cs.LoadPendingTxs()
		if len(txs) == 0 {
			return
		}
	}
	t.Fatal("drainAllPendingTxs: backlog still non-empty after 100 drain calls -- unexpectedly large leftover state")
}
