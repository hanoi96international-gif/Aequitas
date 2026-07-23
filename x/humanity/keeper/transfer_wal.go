package keeper

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/wal"
)

// ============================================================================
// SCALING_ARCHITECTURE.md Phase 7 — REAL async cutover for a narrow slice.
//
// NOT STAGING-VALIDATED. Read this whole comment before touching anything in
// this file, changing AEQUITAS_WAL_ENABLED in any deployed environment, or
// removing this warning.
//
// Every other change in this session (Phases 1-6, 9) preserved Postgres as
// the SYNCHRONOUS source of truth — a bug in that work could produce a wrong
// or rejected transfer, provably caught by balance-conservation property
// tests, but could never lose a transfer that had already been accepted.
// This file is different in kind: when AEQUITAS_WAL_ENABLED=1, a WAL append
// — not a Postgres commit — becomes the durability point for eligible
// transfers (transferConcurrentWAL below). Postgres becomes an asynchronous
// mirror, reconciled by a background flush worker (flushWALQueue) on a
// short delay. If the reconciliation design here has a bug that single-
// session testing doesn't catch, the failure mode is not "rejected
// transfer" — it is a balance that is correct on this node's local WAL but
// never reaches Postgres (so never relayed to other validators via
// pending_txs), or, in the worst case, a genuinely lost or double-applied
// mutation. That is exactly the risk class SCALING_ARCHITECTURE.md's own
// "Status: NICHT implementieren" line and its Phase-7-specific warnings are
// about.
//
// Default is OFF (AEQUITAS_WAL_ENABLED unset). Enabling it is an explicit
// operator decision, the same class of decision as BLOCK_TIME (main.go) —
// never flip it on a live node without having read this comment in full.
//
// What this file does NOT implement, on purpose (deliberately narrowed
// scope, same discipline as every other phase this session):
//   - Compaction (wal.TruncateBefore is never called here). The WAL file
//     grows without bound for the lifetime of the process. Fine for testing;
//     NOT fine for any real deployment — this is the single largest gap
//     before this could ever be considered for staging, let alone
//     production.
//   - Any subsystem other than the narrow-eligibility transfer fast path
//     transferConcurrentWAL already inherits from transferConcurrent (no
//     demurrage settlement, no wealth-cap crediting, warm accounts only —
//     see that function's own scope-narrowing comment). Swap, liquidity,
//     registration, distribution, guardian/escrow, slashing all remain
//     exactly as before, fully synchronous against Postgres.
//   - Multi-node soak testing, real crash testing (process kill, not a
//     simulated one via two ChainState instances in a test), or disk-
//     failure injection. See transfer_wal_test.go for what WAS tested.
// ============================================================================

// walTransferRecord is the payload encoded into each WAL record for a
// WAL-durable transfer. Deliberately minimal: just enough to
// deterministically reapply the same balance delta during crash recovery —
// mirrors the transfer-relevant fields of Transaction, not the whole
// struct (FromDemurrageLost/ToDemurrageLost are always zero for this path
// by construction, exactly like transferConcurrent's own outbox row).
type walTransferRecord struct {
	From   string  `json:"from"`
	To     string  `json:"to"`
	Amount float64 `json:"amount"`
	TxHash string  `json:"tx_hash"`
}

// walFlushItem is one WAL-durable transfer waiting for its async Postgres
// reconciliation: an outbox row (pending_txs) that still needs writing, and
// a hint of which addresses' balances should be (re-)synced alongside it.
// Account balances themselves are NOT carried in this struct — the flush
// worker reads each address's CURRENT value fresh from cs.accounts at flush
// time (see flushWALQueue), so multiple queued items touching the same
// address naturally coalesce into one row write instead of one per item.
type walFlushItem struct {
	from, to string
	tx       Transaction
}

// walFlushInterval mirrors evmMirrorFlushInterval/poolFlushInterval's own
// reasoning: short enough that Postgres/Explorer/other-validator lag stays
// close to unnoticeable, long enough that a burst of WAL-durable transfers
// collapses into one round trip instead of one per transfer.
const walFlushInterval = 500 * time.Millisecond

// walFlushMaxBatch bounds how many queued items one flush transaction
// writes — same rationale as transferBatchMaxSize/wal.MaxBatchSize: bounds
// how long a single flush's DB transaction runs.
const walFlushMaxBatch = 500

// initWALIfEnabled opens (or creates) the local WAL file when
// AEQUITAS_WAL_ENABLED=1 is set, replays any records not yet reflected in
// Postgres (crash recovery), and leaves cs.wal set so transferConcurrentWAL
// activates. Called once from NewChainState, AFTER loadFromDB — replay
// needs cs.accounts already warm (or able to warm on demand via
// ensureAccountLoadedCtx) and cs.db already connected. No-op (cs.wal stays
// nil) if the env var is unset, if there is no DB connection (Postgres is
// this design's reconciliation target — a WAL with nothing to reconcile
// into isn't this phase's scope), or if opening/replaying the WAL fails
// (logged, not fatal: the node continues on the existing, proven paths
// exactly as if this file did not exist).
func (cs *ChainState) initWALIfEnabled() {
	if os.Getenv("AEQUITAS_WAL_ENABLED") != "1" {
		return
	}
	if cs.db == nil {
		fmt.Println("[WAL] AEQUITAS_WAL_ENABLED=1 but no DB connection — WAL fast path needs Postgres to reconcile into, skipping")
		return
	}
	path := os.Getenv("AEQUITAS_WAL_PATH")
	if path == "" {
		path = "aequitas_transfers.wal"
	}
	fmt.Printf("[WAL] AEQUITAS_WAL_ENABLED=1 — opening %s (NOT staging-validated, see transfer_wal.go's own warning)\n", path)
	if err := cs.recoverFromWAL(path); err != nil {
		fmt.Printf("[WAL] ✗ recovery failed, WAL fast path stays DISABLED for this process: %v\n", err)
		return
	}
	w, err := wal.Open(path)
	if err != nil {
		fmt.Printf("[WAL] ✗ could not open %s, WAL fast path stays DISABLED for this process: %v\n", path, err)
		return
	}
	cs.wal = w
	fmt.Println("[WAL] ✓ WAL fast path active for eligible transfers")
}

// transferConcurrentWAL is transferConcurrent's WAL-durable counterpart:
// same eligibility scope (see that function's own doc comment — no
// demurrage settlement, no wealth-cap crediting, warm accounts only, same
// TryLockAddrs non-blocking shard locking so a contended address falls
// straight to the batcher instead of queuing), but the durability point is
// a wal.Append, not a Postgres commit.
//
// This makes the post-durability portion of this function SIMPLER than
// transferConcurrent's: transferConcurrent mutates a scratch copy and only
// publishes it after a Postgres commit succeeds, because Postgres was the
// thing that could still fail after real work was done. Here, once
// wal.Append returns nil, the mutation is UNCONDITIONALLY going to happen —
// either right now (this call) or later (crash recovery replay) — so there
// is no failure path left to guard against, no scratch copy, no revert.
//
// Returns (fromLost, toLost, applied, err) with the exact same contract as
// transferConcurrent — see that function's doc comment.
func (cs *ChainState) transferConcurrentWAL(from, to string, amount float64, pendingTxTemplate Transaction) (fromLost, toLost float64, applied bool, err error) {
	if cs.wal == nil {
		return 0, 0, false, nil
	}
	if from == to {
		return 0, 0, true, fmt.Errorf("self-transfer not allowed")
	}
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, 0, true, fmt.Errorf("invalid transfer amount: %v", amount)
	}

	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if _, ok := cs.accounts.Get(from); !ok {
		return 0, 0, false, nil
	}
	if _, ok := cs.accounts.Get(to); !ok {
		return 0, 0, false, nil
	}
	capAmt, hasCapAmt := cs.wealthCapAmountLocked()

	unlock, ok := cs.accounts.TryLockAddrs(from, to)
	if !ok {
		return 0, 0, false, nil
	}
	defer unlock()

	fromAcc, ok := cs.accounts.GetLocked(from)
	if !ok {
		return 0, 0, false, nil
	}
	toAcc, ok := cs.accounts.GetLocked(to)
	if !ok {
		return 0, 0, false, nil
	}
	if effectiveBalance(fromAcc) != fromAcc.Balance || effectiveBalance(toAcc) != toAcc.Balance {
		return 0, 0, false, nil
	}
	if fromAcc.Balance.Float() < amount {
		return 0, 0, true, fmt.Errorf("insufficient balance")
	}
	if hasCapAmt && toAcc.Balance.Float()+amount > capAmt {
		return 0, 0, false, nil
	}

	payload, err := json.Marshal(walTransferRecord{From: from, To: to, Amount: amount, TxHash: pendingTxTemplate.TxHash})
	if err != nil {
		return 0, 0, false, nil // encode failure -- nothing mutated, safe to fall back
	}
	seq, err := cs.wal.Append(payload)
	if err != nil {
		// Nothing mutated yet -- a WAL append failure (disk full, closed WAL,
		// etc.) is a clean bail to the existing, proven paths, same as any
		// other ineligibility. Not a hard error surfaced to the end user.
		return 0, 0, false, nil
	}

	// From here on the transfer IS durable regardless of anything below --
	// mutate the LIVE pointers directly (not a scratch copy: there is
	// nothing left that can fail and need reverting).
	fromAcc.Balance = fromAcc.Balance.Sub(NewDecimal(amount))
	fromAcc.WALSeq = seq
	touchActivity(fromAcc)
	cs.updateAccountLeafLocked(fromAcc)

	toAcc.Balance = toAcc.Balance.Add(NewDecimal(amount))
	toAcc.WALSeq = seq
	touchActivity(toAcc)
	cs.updateAccountLeafLocked(toAcc)

	pendingTxTemplate.Wallet = from
	pendingTxTemplate.To = to
	pendingTxTemplate.Amount = amount
	pendingTxTemplate.FromDemurrageLost = 0
	pendingTxTemplate.ToDemurrageLost = 0
	cs.enqueueWALFlushLocked(from, to, pendingTxTemplate)
	cs.markEVMMirrorDirtyForAddrsLocked(from, to)

	return 0, 0, true, nil
}

// enqueueWALFlushLocked records one WAL-durable transfer as needing async
// Postgres reconciliation and lazily starts the flush worker. Cheap and
// uses its own small mutex (not cs.mu, already held by the caller as
// RLock) so this never contends with anything else — same shape as
// markEVMMirrorDirtyLocked.
func (cs *ChainState) enqueueWALFlushLocked(from, to string, tx Transaction) {
	cs.walFlushMu.Lock()
	cs.walFlushQueue = append(cs.walFlushQueue, walFlushItem{from: from, to: to, tx: tx})
	cs.walFlushMu.Unlock()
	cs.ensureWALFlushWorkerStarted()
}

func (cs *ChainState) ensureWALFlushWorkerStarted() {
	cs.walFlushOnce.Do(func() {
		cs.walFlushStopCh = make(chan struct{})
		SafeGoroutine("walFlushWorker", cs.runWALFlushWorker)
	})
}

func (cs *ChainState) runWALFlushWorker() {
	ticker := time.NewTicker(walFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cs.flushWALQueue()
		case <-cs.walFlushStopCh:
			return
		}
	}
}

// stopWALFlushWorkerForTest stops the background WAL-flush-reconciliation
// worker, if one was ever started. Test-only: a real process never needs
// this (exiting the whole process already stops every goroutine with it) —
// exists purely because a test that constructs and then deliberately
// abandons a ChainState (crash-recovery simulation, see
// transfer_wal_test.go) would otherwise leak a goroutine that keeps ticking
// and writing to the SHARED test Postgres DB after the test function that
// created it has already returned, corrupting a LATER, unrelated test's
// assumptions about DB state at an unpredictable wall-clock moment. Safe to
// call even if the worker was never started (cs.walFlushStopCh stays nil),
// and safe to call more than once (e.g. once explicitly at a test's "crash"
// point, once more via newWALTestState's t.Cleanup) — walFlushStopOnce
// guards the actual close, since closing an already-closed channel panics.
// Callers must ensure no concurrent enqueueWALFlushLocked call can still be
// racing this (true for every test call site: always invoked after that
// instance's own synchronous WAL calls have already completed).
func (cs *ChainState) stopWALFlushWorkerForTest() {
	cs.walFlushStopOnce.Do(func() {
		if cs.walFlushStopCh != nil {
			close(cs.walFlushStopCh)
		}
	})
}

// flushWALQueue drains up to walFlushMaxBatch queued items and reconciles
// them into Postgres in ONE transaction: every touched address's CURRENT
// balance/WALSeq (read fresh from cs.accounts, not from the queued item —
// see walFlushItem's own comment) is UPSERTed with a monotonic guard
// (never regresses wal_seq backward, so an out-of-order retry can never
// clobber newer data), and every queued item's outbox row is inserted
// alongside it in the SAME transaction — exactly the same "state mutation
// and outbox insert commit or roll back together" pattern
// runAtomicWithOutbox already uses elsewhere in this codebase, just for a
// batch of already-applied, already-durable (via the WAL) mutations
// instead of one about to be applied.
//
// On any failure, the WHOLE drained batch is put back at the front of the
// queue for the next tick to retry — Postgres being down for a while never
// loses a WAL-durable transfer, it only delays when Postgres (and every
// consumer of it: Explorer, other validators via pending_txs/block relay)
// learns about it. See this file's top-level comment for why that lag is
// an explicit, documented consequence of this design, not a bug.
func (cs *ChainState) flushWALQueue() {
	if cs.db == nil {
		return
	}
	cs.walFlushMu.Lock()
	if len(cs.walFlushQueue) == 0 {
		cs.walFlushMu.Unlock()
		return
	}
	n := len(cs.walFlushQueue)
	if n > walFlushMaxBatch {
		n = walFlushMaxBatch
	}
	batch := cs.walFlushQueue[:n]
	cs.walFlushQueue = cs.walFlushQueue[n:]
	cs.walFlushMu.Unlock()

	if err := cs.flushWALBatch(batch); err != nil {
		fmt.Printf("[WAL] ✗ flush of %d item(s) failed, will retry next tick: %v\n", len(batch), err)
		cs.walFlushMu.Lock()
		cs.walFlushQueue = append(batch, cs.walFlushQueue...)
		cs.walFlushMu.Unlock()
	}
}

// flushWALBatch performs the actual reconciliation write for one batch.
// Split out from flushWALQueue so tests can call it directly, synchronously,
// without waiting on the ticker (same reasoning as flushEVMMirrorDirty's
// own split from runEVMMirrorFlushWorker).
//
// Holds cs.mu.Lock() — full exclusivity, not RLock — for the ENTIRE
// snapshot-then-write sequence, not just the snapshot read. This is
// deliberate and NOT optional: an earlier version of this function only
// held RLock() to snapshot balances, then released it before the DB write.
// That left a real race window against the cs.mu.Lock()-based slow path
// (transferLocked/batcher, e.g. a demurrage-settling transfer touching the
// same account): the slow path's own saveAccountToDBCtx write is guarded
// by the OPTIMISTIC-LOCK `version` column, completely independent of this
// flush's `wal_seq` guard — neither guard knows about the other's
// dimension, so if the slow path's DB write landed BETWEEN this flush's
// snapshot read and its own DB write, this flush's `wal_seq < $3` check
// would still pass (wal_seq never changes on a slow-path write) and
// silently clobber the slow path's newer balance with this flush's stale
// snapshot -- a genuine lost update. Holding cs.mu.Lock() for the whole
// operation makes the flush atomic relative to every cs.mu-based writer in
// the system, eliminating that window by construction, at the cost of
// this batched, periodic (every walFlushInterval) operation briefly
// excluding other transfers -- the same tradeoff the ORIGINAL
// runTransferBatcher design already accepts for its own DB round trip, just
// applied here to the WAL reconciliation path instead.
//
// FIX (2026-07-23, TPS-benchmark investigation): this used to write one
// account UPSERT and one outbox INSERT per item via N separate round-trip
// Exec calls inside the single open tx -- for a full walFlushMaxBatch (500)
// batch, measured at 150-230ms wall-clock per flush (confirmed via timing
// instrumentation during this investigation), ALL of it spent holding the
// cs.mu.Lock() described above. At walFlushInterval (500ms), that is 30-45%
// of total wall-clock time spent fully stopping every other transfer in the
// system -- directly cancelling out much of the throughput benefit WAL was
// built for (removing Postgres round-trip latency from the critical path),
// confirmed live: the WAL path benchmarked AT OR BELOW the non-WAL shard-
// lock baseline before this fix. Now issues exactly ONE multi-row UPSERT
// (via VALUES(...),(...),... + EXCLUDED, same technique
// saveAccountsToDBBatchCtx already uses) and ONE multi-row outbox INSERT
// per flush instead of up to 1000 individual statements -- same guarantees
// (the wal_seq < EXCLUDED.wal_seq guard is evaluated per row, identically
// to the old per-statement WHERE clause), same cs.mu.Lock() scope, just far
// less time spent holding it.
func (cs *ChainState) flushWALBatch(batch []walFlushItem) error {
	if len(batch) == 0 {
		return nil
	}
	addrs := make(map[string]struct{}, len(batch)*2)
	for _, item := range batch {
		addrs[item.from] = struct{}{}
		addrs[item.to] = struct{}{}
	}

	type walSnapshot struct {
		balance float64
		walSeq  uint64
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	snapshots := make(map[string]walSnapshot, len(addrs))
	for addr := range addrs {
		acc, ok := cs.accounts.Get(addr)
		if !ok {
			return fmt.Errorf("flushWALBatch: address %s vanished from cs.accounts between apply and flush -- this should never happen", addr)
		}
		snapshots[addr] = walSnapshot{balance: acc.Balance.Float(), walSeq: acc.WALSeq}
	}

	tx, err := cs.db.Begin()
	if err != nil {
		return fmt.Errorf("could not begin WAL flush transaction: %w", err)
	}
	ctx := withTx(context.Background(), tx)

	// Single multi-row UPSERT for every touched account. is_human/
	// tusd_balance/lp_shares are intentionally not in the VALUES list, same
	// as the old per-row statement -- they default to the schema's own
	// defaults for a brand-new row, correct here because
	// transferConcurrentWAL's own eligibility check already guarantees this
	// address is warm with no demurrage/pool interaction pending.
	// See saveAccountsToDBBatchCtx's FIX comment (state.go) for the
	// Sprintf+Join -> strings.Builder+writeDollarParam technique applied
	// here and to the outbox INSERT below.
	var acctValuesSQL strings.Builder
	acctValuesSQL.Grow(len(snapshots) * 40) // "($NNN::text,$NNN::double precision,$NNN::bigint)," rounded up
	acctArgs := make([]interface{}, 0, len(snapshots)*3)
	i := 0
	for addr, snap := range snapshots {
		if i > 0 {
			acctValuesSQL.WriteByte(',')
		}
		n := i * 3
		acctValuesSQL.WriteByte('(')
		writeDollarParam(&acctValuesSQL, n+1, "::text,")
		writeDollarParam(&acctValuesSQL, n+2, "::double precision,")
		writeDollarParam(&acctValuesSQL, n+3, "::bigint")
		acctValuesSQL.WriteByte(')')
		acctArgs = append(acctArgs, addr, snap.balance, snap.walSeq)
		i++
	}
	acctQuery := `INSERT INTO chain_accounts (address, balance, wal_seq, version)
SELECT address, balance, wal_seq, 1 FROM (VALUES ` + acctValuesSQL.String() + `) AS v(address, balance, wal_seq)
ON CONFLICT (address) DO UPDATE
SET balance = EXCLUDED.balance, wal_seq = EXCLUDED.wal_seq
WHERE chain_accounts.wal_seq < EXCLUDED.wal_seq`
	if _, err := cs.dbExecCtx(ctx).Exec(acctQuery, acctArgs...); err != nil {
		tx.Rollback()
		return fmt.Errorf("could not upsert %d account(s) during WAL flush: %w", len(snapshots), err)
	}

	// Single multi-row outbox INSERT for every item in the batch.
	var txValuesSQL strings.Builder
	txValuesSQL.Grow(len(batch) * 12) // "($NNNN,$NNNN)," rounded up
	txArgs := make([]interface{}, 0, len(batch)*2)
	now := time.Now().Unix()
	for j, item := range batch {
		data, err := json.Marshal(item.tx)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("could not marshal outbox tx for %s->%s during WAL flush: %w", item.from, item.to, err)
		}
		if j > 0 {
			txValuesSQL.WriteByte(',')
		}
		n := j * 2
		txValuesSQL.WriteByte('(')
		writeDollarParam(&txValuesSQL, n+1, ",")
		writeDollarParam(&txValuesSQL, n+2, "")
		txValuesSQL.WriteByte(')')
		txArgs = append(txArgs, string(data), now)
	}
	txQuery := `INSERT INTO pending_txs (tx_json, created_at) VALUES ` + txValuesSQL.String()
	if _, err := cs.dbExecCtx(ctx).Exec(txQuery, txArgs...); err != nil {
		tx.Rollback()
		return fmt.Errorf("could not queue %d outbox tx(s) during WAL flush: %w", len(batch), err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("WAL flush commit failed: %w", err)
	}
	return nil
}

// FlushWALNow forces an immediate flush of any pending WAL reconciliation,
// bypassing the ticker -- for graceful shutdown, same role as
// FlushEVMMirrorNow/FlushPoolAccountsNow. Safe to call even if the flush
// worker was never started or cs.wal is nil (flushWALQueue is a no-op with
// an empty queue).
func (cs *ChainState) FlushWALNow() {
	cs.flushWALQueue()
}

// recoverFromWAL replays path (if it exists) against already-loaded
// Postgres state, reapplying any record whose effect is not yet reflected
// in a touched account's WALSeq. Idempotent by construction: a record is
// applied to a given account side if and only if that account's WALSeq
// (seeded from chain_accounts.wal_seq the first time this replay touches
// it) is less than the record's own Seq -- replaying the same file twice,
// or replaying past records already reconciled to Postgres before a crash,
// is always a safe no-op for whichever side(s) were already caught up.
//
// Called once from initWALIfEnabled, before wal.Open lets new appends
// start -- ReplayFile reading the same records Open's own tail-corruption
// scan will later see is intentional and safe (ReplayFile is read-only).
// Runs under cs.mu.Lock() (full exclusivity): this happens once at startup
// before the node serves any traffic, so there is no concurrent caller to
// coordinate with -- matches every other startup-time recovery path in
// this codebase (e.g. RecoverFromEscrow's own callers).
func (cs *ChainState) recoverFromWAL(path string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	seeded := make(map[string]bool)
	seedWALSeq := func(acc *AccountState) {
		if seeded[acc.Address] {
			return
		}
		seeded[acc.Address] = true
		var seq int64
		cs.db.QueryRow(`SELECT COALESCE(wal_seq,0) FROM chain_accounts WHERE lower(address) = $1`, acc.Address).Scan(&seq)
		acc.WALSeq = uint64(seq)
	}

	applyFrom := func(acc *AccountState, seq uint64, amount float64) bool {
		seedWALSeq(acc)
		if acc.WALSeq >= seq {
			return false
		}
		acc.Balance = acc.Balance.Sub(NewDecimal(amount))
		touchActivity(acc)
		acc.WALSeq = seq
		cs.updateAccountLeafLocked(acc)
		return true
	}
	applyTo := func(acc *AccountState, seq uint64, amount float64) bool {
		seedWALSeq(acc)
		if acc.WALSeq >= seq {
			return false
		}
		acc.Balance = acc.Balance.Add(NewDecimal(amount))
		touchActivity(acc)
		acc.WALSeq = seq
		cs.updateAccountLeafLocked(acc)
		return true
	}

	reappliedCount := 0
	readCount, truncated, err := wal.ReplayFile(path, func(entry wal.Entry) error {
		var rec walTransferRecord
		if jsonErr := json.Unmarshal(entry.Payload, &rec); jsonErr != nil {
			return fmt.Errorf("decode WAL record seq %d: %w", entry.Seq, jsonErr)
		}
		cs.ensureAccountLoadedCtx(context.Background(), rec.From)
		cs.ensureAccountLoadedCtx(context.Background(), rec.To)
		fromAcc, ok := cs.accounts.Get(rec.From)
		if !ok {
			return fmt.Errorf("WAL record seq %d: unknown sender %s", entry.Seq, rec.From)
		}
		toAcc, ok := cs.accounts.Get(rec.To)
		if !ok {
			return fmt.Errorf("WAL record seq %d: unknown recipient %s", entry.Seq, rec.To)
		}
		fromApplied := applyFrom(fromAcc, entry.Seq, rec.Amount)
		toApplied := applyTo(toAcc, entry.Seq, rec.Amount)
		if fromApplied || toApplied {
			reappliedCount++
			if cs.db != nil {
				// FIX (found via local crash-recovery drill, 2026-07-23): this used
				// to append directly to cs.walFlushQueue instead of going through
				// enqueueWALFlushLocked -- which also calls
				// ensureWALFlushWorkerStarted(). Confirmed live in a local kill-9
				// test: a record reapplied here (crash landed after a WAL append
				// but before that item's periodic flush) stayed in-memory-correct
				// forever but NEVER reached Postgres, because nothing else in a
				// freshly-restarted, otherwise-idle process ever starts the flush
				// worker on its own. Beyond the visible "Explorer/other-validator
				// view never catches up" lag this file's top comment already
				// documents, this is worse than that documented lag: a node whose
				// OWN next block includes this transaction's outbox row would
				// never mint it (row never reached pending_txs), while its
				// StateRoot already reflects the recovered balance change -- a
				// mismatch other nodes replaying only the transactions they
				// actually received could not reproduce, i.e. a real, permanent
				// fork risk for this validator, not just eventual-consistency lag.
				cs.enqueueWALFlushLocked(rec.From, rec.To, Transaction{Type: "transfer", Wallet: rec.From, To: rec.To, Amount: rec.Amount, TxHash: rec.TxHash})
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("WAL replay failed: %w", err)
	}
	if readCount > 0 {
		fmt.Printf("[WAL] Read %d record(s) from %s: %d already reflected in Postgres (skipped, idempotent), %d reapplied to in-memory state and queued for reconciliation (tail-truncated=%v means the process crashed mid-append on the last record, which is expected and already handled)\n",
			readCount, path, readCount-reappliedCount, reappliedCount, truncated)
	}
	return nil
}
