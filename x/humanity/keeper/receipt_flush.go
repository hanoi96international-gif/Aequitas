package keeper

import (
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// Deferred, batched writes for evm_tx_receipts.
//
// FIX (P0 for throughput, 2026-07-25 — third and last finding from the live
// CPU profile, after the receipt prune and the EVM display mirror):
// SaveTxReceipt did a synchronous single-row INSERT on the RPC request path,
// once per transaction. With the other two writes gone it was the only
// per-transfer Postgres round trip left, and the profile said so plainly:
//
//	sendRawTransaction        13.93s  51.96%
//	  database/sql.(*DB).Exec  6.07s  22.64%   <- this
//	database/sql.withLock      6.68s  24.92%   <- queueing for a pooled conn
//
// Removing the first two round trips moved peak throughput 211 -> 372/s but
// left the sustained mean flat, because one serialising round trip per request
// is enough to hold the pool: every transfer still had to wait its turn.
//
// WHY DEFERRING IS SAFE HERE, which is not obvious and deserves stating:
// getTransactionReceipt answers from the in-memory txStatus/txSenders/txTos
// maps first and only falls back to this table when the hash is unknown to
// them. That fallback exists for ONE reason -- surviving a node restart, as
// SaveTxReceipt's own doc comment says ("so MetaMask can retrieve it after a
// node restart"). A receipt sitting in the buffer is therefore still fully
// answerable: the maps have it, and GetTxReceipt below checks the buffer
// before the database anyway, so even a caller that skips the maps sees it.
//
// What the window genuinely costs: a receipt written less than
// receiptFlushInterval before an UNCLEAN process kill is lost. A clean
// shutdown is covered by FlushTxReceiptsNow. That is a real if narrow
// regression against the synchronous version, and it is the price of not
// serialising every transfer behind a round trip. It cannot corrupt anything:
// evm_tx_receipts is a lookup table for wallet UX, never the ledger, and a
// missing row degrades to exactly what an unknown hash already degrades to.

// receiptFlushInterval is how often buffered receipts are written.
//
// Deliberately shorter than evmMirrorFlushInterval (2s): a wallet polls
// getTransactionReceipt within a second or two of submitting, so the durable
// row should exist well inside a plausible restart-and-poll window. 250ms is
// still ~250 transfers' worth of batching at the throughput observed, which is
// where nearly all of the round-trip saving comes from -- the difference
// between 250ms and 2s is a rounding error on the saving and a real difference
// to the loss window.
const receiptFlushInterval = 250 * time.Millisecond

// pendingReceipt is one buffered row. Mirrors the columns of the statement it
// replaced exactly, including created_at, which is captured at SaveTxReceipt
// time rather than at flush time so the stored timestamp keeps meaning "when
// the transaction happened" and not "when the batch drained".
type pendingReceipt struct {
	txHash, fromAddr, toAddr, status, contractAddr string
	createdAt                                      int64
}

// bufferTxReceipt records a receipt for the next flush.
func (cs *ChainState) bufferTxReceipt(r pendingReceipt) {
	cs.receiptBufMu.Lock()
	if cs.receiptBuf == nil {
		cs.receiptBuf = make(map[string]pendingReceipt)
	}
	// Keyed by hash: a later write for the same transaction overwrites the
	// earlier one, which is what ON CONFLICT (tx_hash) DO UPDATE did.
	cs.receiptBuf[r.txHash] = r
	cs.receiptBufMu.Unlock()
	cs.ensureReceiptFlushWorkerStarted()
}

// lookupBufferedReceipt returns a receipt still waiting to be written, so a
// read never has to know whether the flush has happened yet.
func (cs *ChainState) lookupBufferedReceipt(txHash string) (pendingReceipt, bool) {
	cs.receiptBufMu.Lock()
	defer cs.receiptBufMu.Unlock()
	r, ok := cs.receiptBuf[strings.ToLower(txHash)]
	return r, ok
}

// ensureReceiptFlushWorkerStarted starts the single flush goroutine lazily —
// same reasoning as ensureEVMMirrorFlushWorkerStarted and
// ensureTransferBatcherStarted: a node or test that never writes a receipt
// never pays for an idle goroutine.
func (cs *ChainState) ensureReceiptFlushWorkerStarted() {
	cs.receiptFlushOnce.Do(func() {
		SafeGoroutine("receiptFlushWorker", func() {
			ticker := time.NewTicker(receiptFlushInterval)
			defer ticker.Stop()
			for range ticker.C {
				cs.flushTxReceipts()
			}
		})
	})
}

// flushTxReceipts drains the buffer into ONE multi-row INSERT.
//
// Uses unnest over five arrays rather than a VALUES list for the same reason
// savePendingTxsBatchExec does (see its comment): the statement text stays a
// fixed size regardless of how many rows are being written, so neither
// Postgres' parser nor lib/pq's own escaping cost grows with batch size.
func (cs *ChainState) flushTxReceipts() {
	if cs.db == nil {
		return
	}
	cs.receiptBufMu.Lock()
	if len(cs.receiptBuf) == 0 {
		cs.receiptBufMu.Unlock()
		return
	}
	rows := make([]pendingReceipt, 0, len(cs.receiptBuf))
	for _, r := range cs.receiptBuf {
		rows = append(rows, r)
	}
	cs.receiptBuf = nil
	cs.receiptBufMu.Unlock()

	hashes := make([]string, len(rows))
	froms := make([]string, len(rows))
	tos := make([]string, len(rows))
	statuses := make([]string, len(rows))
	contracts := make([]string, len(rows))
	createdAts := make([]int64, len(rows))
	for i, r := range rows {
		hashes[i] = r.txHash
		froms[i] = r.fromAddr
		tos[i] = r.toAddr
		statuses[i] = r.status
		contracts[i] = r.contractAddr
		createdAts[i] = r.createdAt
	}

	if _, err := cs.db.Exec(
		`INSERT INTO evm_tx_receipts (tx_hash, from_addr, to_addr, status, contract_addr, created_at)
		 SELECT * FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::bigint[])
		 ON CONFLICT (tx_hash) DO UPDATE SET status = EXCLUDED.status`,
		pq.Array(hashes), pq.Array(froms), pq.Array(tos),
		pq.Array(statuses), pq.Array(contracts), pq.Array(createdAts),
	); err != nil {
		// Put them back rather than dropping: the next tick retries, and a
		// transient database hiccup must not silently lose receipts that the
		// in-memory maps will stop answering for after a restart. Entries
		// written in the meantime win, since they are strictly newer.
		fmt.Printf("[EVM] receipt flush failed for %d receipt(s): %v — retrying at the next interval\n", len(rows), err)
		cs.receiptBufMu.Lock()
		if cs.receiptBuf == nil {
			cs.receiptBuf = make(map[string]pendingReceipt, len(rows))
		}
		for _, r := range rows {
			if _, newer := cs.receiptBuf[r.txHash]; !newer {
				cs.receiptBuf[r.txHash] = r
			}
		}
		cs.receiptBufMu.Unlock()
	}
}

// FlushTxReceiptsNow forces an immediate flush, bypassing the ticker. Called on
// graceful shutdown alongside FlushEVMMirrorNow and FlushPoolAccountsNow, so a
// routine restart never depends on receiptFlushInterval's window. Safe to call
// even if the worker was never started.
func (cs *ChainState) FlushTxReceiptsNow() {
	cs.flushTxReceipts()
}
