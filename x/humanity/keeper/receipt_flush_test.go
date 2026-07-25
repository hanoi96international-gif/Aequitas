package keeper

import (
	"testing"
	"time"
)

// Regression guards for the 2026-07-25 profiling finding: SaveTxReceipt must
// buffer rather than write, and nothing may read as "not found" while it sits
// in that buffer.
//
// It was the last synchronous per-transfer Postgres round trip on the RPC
// request path. The profile after the first two fixes:
//
//	sendRawTransaction        13.93s  51.96%
//	  database/sql.(*DB).Exec  6.07s  22.64%   <- this
//	database/sql.withLock      6.68s  24.92%
//
// Removing the first two round trips moved peak throughput 211 -> 372/s but
// left the sustained mean flat, because one serialising round trip per request
// is enough to hold the connection pool.
//
// These tests pin the buffering and the read-through, which is where the
// correctness risk of deferring actually lives. The flush statement itself
// needs a real database and is not exercised here.

func newReceiptBufTestState() *ChainState {
	// db stays nil: bufferTxReceipt and lookupBufferedReceipt are pure
	// in-memory, and a nil db makes flushTxReceipts return before touching
	// anything, so the lazily-started worker can never reach a fake handle.
	return &ChainState{}
}

// The whole point: a receipt goes into the buffer, not into Postgres.
func TestSaveTxReceipt_BuffersInsteadOfWriting(t *testing.T) {
	cs := newReceiptBufTestState()

	cs.bufferTxReceipt(pendingReceipt{
		txHash: "0xabc", fromAddr: "0xfrom", toAddr: "0xto",
		status: "0x1", createdAt: 1234,
	})

	cs.receiptBufMu.Lock()
	n := len(cs.receiptBuf)
	cs.receiptBufMu.Unlock()
	if n != 1 {
		t.Fatalf("expected the receipt to be buffered, got %d entries", n)
	}
}

// A buffered receipt must be readable immediately. Without the read-through in
// GetTxReceipt, a receipt written moments ago would report "not found" until
// the next flush — an internal batching detail leaking out as a visible
// inconsistency.
func TestLookupBufferedReceipt_ReadsBeforeFlush(t *testing.T) {
	cs := newReceiptBufTestState()
	cs.bufferTxReceipt(pendingReceipt{
		txHash: "0xabc", fromAddr: "0xfrom", toAddr: "0xto",
		status: "0x1", contractAddr: "0xdeployed", createdAt: 1234,
	})

	r, ok := cs.lookupBufferedReceipt("0xABC") // deliberately different case
	if !ok {
		t.Fatal("a buffered receipt must be findable before the flush, and by any casing")
	}
	if r.status != "0x1" || r.fromAddr != "0xfrom" || r.contractAddr != "0xdeployed" {
		t.Fatalf("buffered receipt came back altered: %+v", r)
	}
}

// The single-row statement this replaced was ON CONFLICT (tx_hash) DO UPDATE
// SET status. The buffer has to reproduce that: a later write for the same
// hash — a success receipt superseded by a failure one, which
// sendRawTransaction does on its error path — must overwrite, not accumulate.
func TestBufferTxReceipt_LaterWriteForSameHashWins(t *testing.T) {
	cs := newReceiptBufTestState()

	cs.bufferTxReceipt(pendingReceipt{txHash: "0xabc", status: "0x1", createdAt: 1})
	cs.bufferTxReceipt(pendingReceipt{txHash: "0xabc", status: "0x0", createdAt: 2})

	cs.receiptBufMu.Lock()
	n := len(cs.receiptBuf)
	cs.receiptBufMu.Unlock()
	if n != 1 {
		t.Fatalf("the same hash twice must produce one buffered row, got %d", n)
	}
	r, _ := cs.lookupBufferedReceipt("0xabc")
	if r.status != "0x0" {
		t.Fatalf("the later write must win (ON CONFLICT DO UPDATE semantics), got status %q", r.status)
	}
}

// An unknown hash must simply be absent, not a zero-valued hit — GetTxReceipt
// uses the boolean to decide whether to fall through to the database.
func TestLookupBufferedReceipt_UnknownHashMisses(t *testing.T) {
	cs := newReceiptBufTestState()
	cs.bufferTxReceipt(pendingReceipt{txHash: "0xabc", status: "0x1"})

	if _, ok := cs.lookupBufferedReceipt("0xdef"); ok {
		t.Fatal("an unknown hash must miss so the caller falls through to the database")
	}
}

// A nil db must make the flush a no-op rather than panicking: nodes and tests
// run without one, and the worker ticks regardless once started.
func TestFlushTxReceipts_NilDBIsANoOp(t *testing.T) {
	cs := newReceiptBufTestState()
	cs.bufferTxReceipt(pendingReceipt{txHash: "0xabc", status: "0x1"})

	cs.flushTxReceipts()

	// The buffer is deliberately left intact: with no database there is
	// nowhere to write, and dropping the entries would lose them silently.
	if _, ok := cs.lookupBufferedReceipt("0xabc"); !ok {
		t.Fatal("a nil-db flush must not discard buffered receipts")
	}
	time.Sleep(20 * time.Millisecond) // let the lazily-started worker tick once
}
