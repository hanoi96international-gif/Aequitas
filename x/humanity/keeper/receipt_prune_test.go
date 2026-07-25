package keeper

import (
	"database/sql"
	"testing"
	"time"
)

// Regression guard for the 2026-07-25 profiling finding: the receipt prune must
// not run on the request path.
//
// It used to be an inline cs.db.Exec on every SaveTxReceipt call, i.e. on every
// transfer, and the query is a full-table anti-join with a sort:
//
//	DELETE FROM evm_tx_receipts WHERE tx_hash NOT IN (
//	    SELECT tx_hash FROM evm_tx_receipts ORDER BY created_at DESC LIMIT 10000)
//
// A 30s CPU profile of the live node under load put database/sql.(*DB).Exec at
// 27.43% cumulative inside sendRawTransaction, with database/sql.withLock at
// 28.69% — requests serialising on pooled connections — while total CPU use was
// only ~0.61 cores. The node was waiting on Postgres, not computing.
//
// These tests pin the RATE LIMITER rather than the SQL: the query is unchanged
// and the retained-row cap is unchanged, so the only thing that could silently
// regress is the gate that keeps it off the hot path.

// The first call in a fresh process must be allowed through, and the ones
// immediately after it must not be. Without this the fix would degrade to "one
// prune per transfer" again the moment the interval logic drifted.
func TestMaybePruneTxReceipts_RateLimitsAfterTheFirstCall(t *testing.T) {
	receiptPruneLastAt.Store(0)
	receiptPruneRunning.Store(false)

	// A non-nil db is required to get past the nil guard; it is never actually
	// queried, because the interval gate is what these assertions are about and
	// the prune itself runs in a goroutine that SafeGoroutine contains.
	cs := &ChainState{db: &sql.DB{}}

	before := receiptPruneLastAt.Load()
	cs.maybePruneTxReceipts()
	first := receiptPruneLastAt.Load()
	if first == before {
		t.Fatal("the first call must claim the prune slot (timestamp should advance)")
	}

	// Every immediately-following call is inside the interval and must be a
	// no-op — this is the property that takes the query off the per-transfer
	// path, so it is the one worth pinning.
	for i := 0; i < 100; i++ {
		cs.maybePruneTxReceipts()
	}
	if got := receiptPruneLastAt.Load(); got != first {
		t.Fatalf("calls inside receiptPruneInterval must not re-claim the slot: %d != %d", got, first)
	}
}

// Once the interval has elapsed the prune is allowed again. Expressed by moving
// the stored timestamp back rather than by sleeping, so the test stays fast and
// does not depend on wall-clock timing.
func TestMaybePruneTxReceipts_RunsAgainAfterTheInterval(t *testing.T) {
	receiptPruneRunning.Store(false)
	stale := time.Now().Unix() - int64(receiptPruneInterval.Seconds()) - 1
	receiptPruneLastAt.Store(stale)

	cs := &ChainState{db: &sql.DB{}}
	cs.maybePruneTxReceipts()

	if got := receiptPruneLastAt.Load(); got == stale {
		t.Fatalf("a call after receiptPruneInterval (%s) must claim the slot again", receiptPruneInterval)
	}
}

// A nil db must return before claiming the slot or spawning anything.
// SafeGoroutine would recover a nil-db panic, but it writes a full stack trace
// into the log for a condition that is entirely ordinary in tests and in a node
// started without a database — so it is checked, not recovered.
func TestMaybePruneTxReceipts_NilDBIsANoOp(t *testing.T) {
	receiptPruneLastAt.Store(0)
	receiptPruneRunning.Store(false)

	cs := &ChainState{}
	cs.maybePruneTxReceipts()

	if got := receiptPruneLastAt.Load(); got != 0 {
		t.Fatalf("a nil db must not even claim the prune slot, got timestamp %d", got)
	}
	if receiptPruneRunning.Load() {
		t.Fatal("a nil db must not leave the single-flight flag set")
	}
	// Nothing was spawned, so nothing can panic later either.
	time.Sleep(50 * time.Millisecond)
}

// The retained-row cap must stay what the inline version used. The fix changed
// the cadence deliberately and the retention accidentally-not-at-all; if
// someone later "tidies" this constant, receipts start disappearing sooner than
// getTransactionReceipt callers expect.
func TestReceiptPruneKeep_Unchanged(t *testing.T) {
	if receiptPruneKeep != 10000 {
		t.Fatalf("receiptPruneKeep = %d, want 10000 — the inline prune this replaced kept exactly 10,000 rows", receiptPruneKeep)
	}
}
