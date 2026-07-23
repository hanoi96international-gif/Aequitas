package keeper

import (
	"context"
	"fmt"
	"math"
	"runtime/debug"
)

// parallelBatchPoolSize bounds how many transferBatchCh-collected batches
// can be dispatched to their own goroutine at once (see runTransferBatcher's
// own comment) — whether they end up on the parallel shard-locked path
// (processTransferBatchConcurrent) or fall back to the serialized one
// (processTransferBatch).
//
// FIX (2026-07-23, 50k-TPS-goal investigation): started at 8, which caused
// real "pq: sorry, too many clients already" (53300) failures under the
// full test suite — a batch DESTINED to fall back still calls
// processTransferBatch, which opens its own Postgres connection
// (cs.db.Begin(), inside runAtomicWithOutbox) BEFORE acquiring cs.mu.Lock(),
// so several concurrently-dispatched fallback-bound batches can each grab a
// connection and then sit queued on the Go-level mutex, holding that
// connection open and idle the whole time — zero real parallelism (the
// underlying work is still fully serialized by cs.mu.Lock() regardless),
// just pool pressure the old single-goroutine design never had. Reducing
// this alone to 4 did NOT fix it (still failed 5/5 repeated full-suite
// -race runs) -- state.go's own MaxOpenConns setting (see its FIX comment)
// was the actual lever: this package's test suite creates many separate
// ChainState/*sql.DB pool instances, several of which leave background
// activity running past their own test's return, and MaxOpenConns=40 (also
// tried, also reverted) let overlapping instances blow past Postgres's
// max_connections=100 in aggregate. With MaxOpenConns back at its original,
// conservative 20, 4 is both safe (5 consecutive clean full-suite -race
// runs) AND the better throughput number anyway: measured on
// TestSimulateMaxTPS_MultipleHotRecipients (tps_bench_test.go), 4 clearly
// outperformed the earlier (unsafe) 8-at-MaxOpenConns=40 combination,
// consistent with 4 being this sandbox's actual CPU core count — more
// truly-parallel CPU-bound batch processing than available cores doesn't
// help throughput, it just adds scheduling overhead.
const parallelBatchPoolSize = 4

// processTransferBatchConcurrent is processTransferBatch's counterpart:
// instead of always serializing every collected batch behind cs.mu.Lock()
// (processTransferBatch — one goroutine's worth of DB work at a time,
// every OTHER pending batch waiting its turn regardless of whether their
// addresses even overlap), this runs a batch entirely under
// transferConcurrent's own mechanism: cs.mu.RLock() (excludes only the
// cs.mu.Lock()-based paths — replay/snapshot/direct-no-DB) plus
// TryLockAddrs (sharded_accounts.go) for every address the WHOLE BATCH
// touches, held for this batch's entire own DB transaction. Two batches
// (or a batch and a transferConcurrent call) whose touched-address sets
// are disjoint can now have their own Postgres transactions open and
// committing SIMULTANEOUSLY — real parallelism the old design structurally
// couldn't offer, since it funneled every batcher-routed transfer through
// one goroutine no matter how unrelated the addresses were.
//
// SAFETY ARGUMENT (why this can never deadlock, in-memory OR in Postgres):
// TryLockAddrs acquires every distinct shard in a fixed, deterministic
// order (ascending shard index) and never blocks — it bails instantly if
// any shard is contended, the same "give up, don't wait" contract
// transferConcurrent's own comment already establishes for 2 addresses,
// unchanged here for N. That means any two Postgres transactions opened by
// this mechanism (across however many concurrent batches, plus every
// transferConcurrent call, which uses the exact same primitive) are
// GUARANTEED to have disjoint touched-row sets for their entire overlap in
// wall-clock time — holding a row here means holding its shard lock, and
// shard locks are mutually exclusive per shard by construction, so neither
// transaction can ever be holding a row the other one wants. A Postgres
// deadlock fundamentally requires two transactions to want overlapping
// resources; with the row sets always disjoint, that precondition can
// never arise.
//
// This is the SAME invariant every other Postgres-writing path in this
// codebase already upholds — either full cs.mu.Lock() exclusivity
// (runAtomicWithOutbox and everything built on it: transferLocked,
// processTransferBatch, distributeSwapFeeCtx, the EVM-mirror/pool/WAL
// background flush workers), or cs.mu.RLock() plus shard locks held for
// the WHOLE transaction (transferConcurrent). This function extends that
// same invariant from 2 addresses to N, it does not introduce a new one.
// This is exactly the invariant an EARLIER attempt this session (decoupling
// the WAL flush path's DB write from cs.mu, since fully reverted) failed to
// uphold: that design read its snapshot under cs.mu.RLock() alone, with NO
// per-address shard-lock exclusion held across its own DB write — so two
// concurrent flush batches (or a flush batch and transferConcurrent) could
// end up wanting overlapping rows at the same time. That was a real,
// measured deadlock class (up to 23.6% failure rate at 500 concurrent
// senders), which is why it was reverted rather than fixed under time
// pressure. Every address this function touches is shard-locked for its
// entire DB transaction — closing exactly the gap that redesign left open.
//
// Deliberately narrow in scope, same reasoning as transferConcurrent's own:
// if ANY member of the batch would need to touch one of the 4 tokenomics
// pool addresses (demurrage settlement or wealth-cap enforcement), the
// WHOLE batch bails to the caller's fallback — extending the shard-lock set
// to cover the pool addresses would serialize nearly every batch against
// those same 4 shards, defeating the entire point of sharding.
//
// Returns handled=false if this batch could not run this way at all (a
// cold account anywhere, a contended shard anywhere, or any member would
// touch a pool address) — the caller MUST fall back to
// processTransferBatch, completely unchanged, which already handles every
// case this one doesn't. Returns handled=true once every request's result
// has been delivered to its own result channel — including the
// all-or-nothing case where one member's business-logic failure
// (insufficient balance, self-transfer, invalid amount) fails every member
// in the batch, the same contract processTransferBatch's own comment
// documents.
func (cs *ChainState) processTransferBatchConcurrent(batch []*transferBatchRequest) (handled bool) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[PANIC RECOVERED] processTransferBatchConcurrent: %v\n%s\n", r, debug.Stack())
			for _, req := range batch {
				select {
				case req.result <- transferBatchResult{err: fmt.Errorf("internal error processing transfer batch: %v", r)}:
				default:
				}
			}
			handled = true
		}
	}()

	if cs.db == nil {
		return false
	}

	// Pre-validate every member's own input, independent of any account
	// state — same checks transferMutateLocked/transferConcurrent make,
	// hoisted up front so a malformed request never reaches the
	// lock-acquisition phase below. Unlike an eligibility bail, these are
	// real, decided failures: the whole batch fails with a descriptive
	// error rather than falling back (falling back would just reach the
	// exact same rejection one step later, via transferMutateLocked).
	for i, req := range batch {
		if req.from == req.to {
			failWholeBatch(batch, fmt.Errorf("batch member %d/%d (%s -> %s) failed: self-transfer not allowed", i+1, len(batch), req.from, req.to))
			return true
		}
		if req.amount <= 0 || math.IsNaN(req.amount) || math.IsInf(req.amount, 0) {
			failWholeBatch(batch, fmt.Errorf("batch member %d/%d (%s -> %s) failed: invalid transfer amount: %v", i+1, len(batch), req.from, req.to, req.amount))
			return true
		}
	}

	touchedSet := make(map[string]bool, len(batch)*2)
	for _, req := range batch {
		touchedSet[req.from] = true
		touchedSet[req.to] = true
	}
	touched := make([]string, 0, len(touchedSet))
	for a := range touchedSet {
		touched = append(touched, a)
	}

	// cs.mu.RLock(), not Lock(): see transferConcurrent's own comment —
	// lets this run alongside other concurrent shard-locked callers while
	// still fully excluding replay/swap/distribution/snapshot/the slow
	// no-fast-path transfer route (all cs.mu.Lock()).
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// Existence pre-check for every touched address, same reasoning as
	// transferConcurrent's own: a cold address here means the slow path's
	// own ensureAccountLoadedCtx needs to warm it, which needs an active
	// transaction (cs.activeTx) this path never has.
	for _, a := range touched {
		if _, ok := cs.accounts.Get(a); !ok {
			return false
		}
	}
	// MUST run before TryLockAddrs below, never after — see
	// transferConcurrent's own comment: it internally Ranges over every
	// shard, which would self-deadlock against shards this call is about
	// to hold via TryLockAddrs (Go's sync.Mutex is not reentrant).
	capAmt, hasCapAmt := cs.wealthCapAmountLocked()

	// TryLockAddrs, not LockAddrs: never block on a contended shard — bail
	// to the caller's fallback instead, same reasoning as
	// transferConcurrent's own (see its comment for the measured cost of
	// blocking here instead).
	unlock, ok := cs.accounts.TryLockAddrs(touched...)
	if !ok {
		return false
	}
	defer unlock()

	// Mutate scratch COPIES, never the live accounts, until every DB write
	// and the commit itself have succeeded — mirrors transferConcurrent's
	// own fromScratch/toScratch pattern, generalized to every touched
	// address in the batch. leafBefore records each account's pre-call
	// leafHash so a later DB failure can precisely undo whichever
	// accounts' accountSetXOR contribution saveAccountsToDBBatchCtx
	// already folded in before discovering some OTHER account in the same
	// call conflicted (see its own doc comment) — same technique
	// considered (and, for transferConcurrent's narrower 2-account case,
	// found to be a net throughput LOSS and reverted) earlier this
	// session; kept here because this function's whole reason to exist is
	// batching many accounts into one round trip, where the multi-row
	// call is unambiguously the right tool, not a regression risk.
	scratch := make(map[string]*AccountState, len(touched))
	leafBefore := make(map[string][32]byte, len(touched))
	for _, a := range touched {
		acc, _ := cs.accounts.GetLocked(a)
		cp := *acc
		scratch[a] = &cp
		leafBefore[a] = acc.leafHash
	}

	results := make([]transferBatchResult, len(batch))
	for i, req := range batch {
		fromAcc := scratch[req.from]
		toAcc := scratch[req.to]
		// First (and only) point where either account's fields are
		// actually read: every touched shard is locked here, so these
		// reads are synchronized against every possible concurrent
		// writer, not just this call's own earlier existence pre-check —
		// same reasoning as transferConcurrent's own equivalent comment.
		if effectiveBalance(fromAcc) != fromAcc.Balance || effectiveBalance(toAcc) != toAcc.Balance {
			return false // demurrage would settle somewhere in the batch -- bail, nothing touched yet
		}
		if fromAcc.Balance.Float() < req.amount {
			failWholeBatch(batch, fmt.Errorf("batch member %d/%d (%s -> %s) failed: insufficient balance", i+1, len(batch), req.from, req.to))
			return true
		}
		if hasCapAmt && toAcc.Balance.Float()+req.amount > capAmt {
			return false // would overflow the wealth cap somewhere in the batch -- bail, nothing touched yet
		}
		fromAcc.Balance = fromAcc.Balance.Sub(NewDecimal(req.amount))
		touchActivity(fromAcc)
		toAcc.Balance = toAcc.Balance.Add(NewDecimal(req.amount))
		touchActivity(toAcc)
		results[i] = transferBatchResult{}
	}

	tx, err := cs.db.Begin()
	if err != nil {
		failWholeBatch(batch, fmt.Errorf("could not begin concurrent batch transaction: %w", err))
		return true
	}
	ctx := withTx(context.Background(), tx)

	accsToSave := make([]*AccountState, 0, len(scratch))
	for _, acc := range scratch {
		accsToSave = append(accsToSave, acc)
	}
	// revertLeaves undoes accountSetXOR's contribution from whichever
	// accounts saveAccountsToDBBatchCtx's internal loop actually folded in
	// (its own updateAccountLeafLocked call, per successfully-saved
	// account) before this batch aborts for any reason after that point —
	// the leafHash-changed comparison is exactly the signal
	// saveAccountsToDBBatchCtx's own success path already depends on, so
	// it reliably distinguishes "this account's DB write and leaf update
	// actually happened" from "it never got that far."
	revertLeaves := func() {
		for _, acc := range accsToSave {
			before := leafBefore[acc.Address]
			if acc.leafHash != before {
				cs.accountSetXORMu.Lock()
				xorInto(&cs.accountSetXOR, acc.leafHash)
				xorInto(&cs.accountSetXOR, before)
				cs.accountSetXORMu.Unlock()
			}
		}
	}

	if err := cs.saveAccountsToDBBatchCtx(ctx, accsToSave); err != nil {
		tx.Rollback()
		revertLeaves()
		failWholeBatch(batch, fmt.Errorf("could not batch-save %d account(s): %w", len(accsToSave), err))
		return true
	}

	pendingTxs := make([]Transaction, 0, len(batch))
	for _, req := range batch {
		pendingTx := req.pendingTxTemplate
		pendingTx.FromDemurrageLost = 0
		pendingTx.ToDemurrageLost = 0
		pendingTxs = append(pendingTxs, pendingTx)
	}
	if err := savePendingTxsBatchExec(cs.dbExecCtx(ctx), pendingTxs); err != nil {
		tx.Rollback()
		revertLeaves()
		failWholeBatch(batch, fmt.Errorf("could not batch-insert %d outbox row(s): %w", len(pendingTxs), err))
		return true
	}

	if err := tx.Commit(); err != nil {
		// Postgres has already rolled the transaction back server-side on
		// a failed commit (same reasoning as runAtomicWithOutbox's and
		// transferConcurrent's own commit-failure handling) — no explicit
		// tx.Rollback() call here, but the accountSetXOR side effects of
		// whichever scratch saves already succeeded above still need
		// undoing.
		revertLeaves()
		failWholeBatch(batch, fmt.Errorf("commit failed: %w", err))
		return true
	}

	// Commit succeeded — publish every touched account's scratch copy to
	// the live object. Still holding every shard lock (unlock() deferred
	// above), so this is atomic from every other goroutine's perspective:
	// nobody can observe a live account mid-update.
	for addr, acc := range scratch {
		live, _ := cs.accounts.GetLocked(addr)
		*live = *acc
	}

	touchedAddrs := make([]string, 0, len(touched)+4)
	touchedAddrs = append(touchedAddrs, touched...)
	touchedAddrs = append(touchedAddrs, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr)
	cs.syncBalanceLocked(V7_CONTRACT_ADDR, touchedAddrs...)
	cs.save()
	fmt.Printf("[STATE] ✓ Batch committed: %d transfer(s), %d account(s) updated (parallel)\n", len(batch), len(accsToSave))

	for i, req := range batch {
		req.result <- results[i]
	}
	return true
}

// failWholeBatch delivers err to every request in batch — the
// all-or-nothing failure contract processTransferBatch's own comment
// documents, shared here since processTransferBatchConcurrent has several
// early-exit points that need the identical behavior.
func failWholeBatch(batch []*transferBatchRequest, err error) {
	for _, req := range batch {
		req.result <- transferBatchResult{err: err}
	}
}
