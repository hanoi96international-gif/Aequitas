package keeper

import (
	"fmt"
	"strings"
	"sync/atomic"
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
	if _, da := cs.receiptBuf[r.txHash]; !da && len(cs.receiptBuf) >= receiptBufMax {
		// Voll: eine beliebige alte Quittung verdraengen (die Map hat keine
		// Ordnung; jede ist gleich alt genug). Siehe flushTxReceipts.
		for k := range cs.receiptBuf {
			delete(cs.receiptBuf, k)
			break
		}
		n := receiptVerworfen.Add(1)
		if jetzt := time.Now().Unix(); jetzt-receiptVerworfenLogAt.Load() >= 60 {
			receiptVerworfenLogAt.Store(jetzt)
			fmt.Printf("[EVM] ⚠ receipt buffer full (%d) -- dropping receipts (%d so far); the database is not taking them\n", receiptBufMax, n)
		}
	}
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

// flushTxReceipts drains the buffer in Stuecken von receiptFlushChunk Zeilen.
//
// Uses unnest over five arrays rather than a VALUES list for the same reason
// savePendingTxsBatchExec does (see its comment): the statement text stays a
// fixed size regardless of how many rows are being written, so neither
// Postgres' parser nor lib/pq's own escaping cost grows with batch size.
//
// GEMESSEN AM 12.09.2026, C2: die erste Fassung schrieb den GANZEN Puffer in
// EINER Anweisung. Unter Last stockte die Datenbank einmal, die Anweisung lief
// in das 5-s-Zeitlimit der Verbindung, und der Puffer wurde komplett wieder
// eingereiht -- inzwischen 1.525.615 Quittungen. Von da an scheiterte jeder
// Versuch am selben Limit, alle 17 s, 398-mal in 90 Minuten: ein Puffer, der
// nur noch wachsen kann, sechs Felder mit je 1,5 Millionen Strings bei jedem
// Anlauf neu gebaut (384 MB auf dem Heap), und Postgres 5 s je Anlauf
// beschaeftigt -- auf dem Knoten, der ohnehin der schwaechere ist. Nach einem
// Neustart waeren alle diese Quittungen weg gewesen.
//
// Darum jetzt: Stuecke, die bequem unter das Zeitlimit passen; bei einem
// Fehler wird nur zurueckgelegt, was noch nicht geschrieben ist; und der
// Puffer ist gedeckelt (receiptBufMax), weil ein Knoten ohne Datenbank sonst
// mit den Quittungen im Speicher waechst, bis ihn der Kernel beendet. Eine
// verworfene Quittung kostet einem Wallet einen Nachschlag; ein toter Knoten
// kostet die Kette einen Validator.

const (
	// receiptFlushChunk: Zeilen je INSERT. 5.000 Zeilen brauchen im Normalfall
	// zweistellige Millisekunden -- weit unter den 5 s der Verbindung.
	receiptFlushChunk = 5000
)

// receiptBufMax: mehr Quittungen haelt der Puffer nicht. Bei 10.000
// Ueberweisungen je Sekunde sind das gut drei Minuten ohne Datenbank.
// Variable, damit ein Test den Deckel erreichen kann.
var receiptBufMax = 2_000_000

// receiptSchreibeFn ist die Nahtstelle fuer Tests ohne Datenbank.
var receiptSchreibeFn = (*ChainState).schreibeReceipts

var (
	receiptFlushGeschrieben atomic.Int64
	receiptFlushFehler      atomic.Int64
	receiptVerworfen        atomic.Int64
	receiptVerworfenLogAt   atomic.Int64
)

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

	for off := 0; off < len(rows); off += receiptFlushChunk {
		ende := off + receiptFlushChunk
		if ende > len(rows) {
			ende = len(rows)
		}
		if err := receiptSchreibeFn(cs, rows[off:ende]); err != nil {
			receiptFlushFehler.Add(1)
			// Zurueck in den Puffer -- nur das Ungeschriebene, und nichts
			// ueberschreiben, was inzwischen neuer hereinkam.
			rest := rows[off:]
			fmt.Printf("[EVM] receipt flush failed for %d receipt(s) (%d written first, %d kept for the next interval): %v\n",
				ende-off, off, len(rest), err)
			cs.receiptBufMu.Lock()
			if cs.receiptBuf == nil {
				cs.receiptBuf = make(map[string]pendingReceipt, len(rest))
			}
			for _, r := range rest {
				if _, newer := cs.receiptBuf[r.txHash]; !newer {
					cs.receiptBuf[r.txHash] = r
				}
			}
			cs.receiptBufMu.Unlock()
			return
		}
		receiptFlushGeschrieben.Add(int64(ende - off))
	}
}

// schreibeReceipts schreibt ein Stueck in einer Anweisung.
func (cs *ChainState) schreibeReceipts(rows []pendingReceipt) error {
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
	_, err := cs.db.Exec(
		`INSERT INTO evm_tx_receipts (tx_hash, from_addr, to_addr, status, contract_addr, created_at)
		 SELECT * FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::bigint[])
		 ON CONFLICT (tx_hash) DO UPDATE SET status = EXCLUDED.status`,
		pq.Array(hashes), pq.Array(froms), pq.Array(tos),
		pq.Array(statuses), pq.Array(contracts), pq.Array(createdAts),
	)
	return err
}

// ReceiptFlushStand fuer /api/health/combined.
func (cs *ChainState) ReceiptFlushStand() map[string]interface{} {
	cs.receiptBufMu.Lock()
	puffer := len(cs.receiptBuf)
	cs.receiptBufMu.Unlock()
	return map[string]interface{}{
		"bedeutung": "Quittungen (evm_tx_receipts) werden gepuffert und in Stuecken von " +
			fmt.Sprint(receiptFlushChunk) + " geschrieben. fehler zaehlt gescheiterte Stuecke; " +
			"verworfen zaehlt Quittungen, die der volle Puffer (" + fmt.Sprint(receiptBufMax) + ") verdraengt hat. " +
			"puffer sollte nach Last binnen Sekunden auf 0 fallen.",
		"puffer":      puffer,
		"geschrieben": receiptFlushGeschrieben.Load(),
		"fehler":      receiptFlushFehler.Load(),
		"verworfen":   receiptVerworfen.Load(),
		"stueck":      receiptFlushChunk,
		"deckel":      receiptBufMax,
	}
}

// FlushTxReceiptsNow forces an immediate flush, bypassing the ticker. Called on
// graceful shutdown alongside FlushEVMMirrorNow and FlushPoolAccountsNow, so a
// routine restart never depends on receiptFlushInterval's window. Safe to call
// even if the worker was never started.
func (cs *ChainState) FlushTxReceiptsNow() {
	cs.flushTxReceipts()
}
