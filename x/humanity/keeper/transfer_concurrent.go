package keeper

import (
	"context"
	"fmt"
	"math"
)

// transferConcurrent is SCALING_ARCHITECTURE.md Phase 5's real concurrency
// win: a transfer that only touches its sender and recipient can run under
// cs.mu.RLock() plus two per-shard account locks (see
// shardedAccounts.TryLockAddrs) instead of cs.mu.Lock()'s full exclusivity —
// letting many transfers execute genuinely in parallel across CPU cores
// and DB connections, as long as their touched addresses don't collide.
//
// MEASURED (local Postgres, 100 concurrent senders, 10k transfers,
// TestSimulateMaxTPS_Ingestion / TestSimulateMaxTPS_IngestionDisjointRecipients):
//   - disjoint sender/recipient pairs (P2P-style, no shard collisions):
//     ~2,150 TPS with this path wired in vs. ~946 TPS on the batcher
//     alone — roughly +2.3x.
//   - many senders paying one shared recipient (faucet/pool/exchange/
//     merchant-style, every transfer wants the SAME shard): ~960 TPS
//     either way — no regression, because TryLockAddrs bails instantly
//     instead of queuing (see its own comment for why an earlier,
//     blocking-lock version of this path measured roughly HALF the
//     batcher-alone throughput on exactly this workload).
//
// These two numbers bound the real range: this path never makes things
// worse than the batcher alone, and helps a lot specifically when
// concurrent transfers don't contend for the same account.
//
// Deliberately narrow in scope. A transfer needs the four tokenomics pool
// addresses too (see transferLocked/runAtomicWithOutbox's touched-address
// list) whenever settling demurrage or enforcing the wealth cap actually
// credits them — and those four addresses are FIXED, shared by every
// single transfer, so locking them defensively on every call would
// serialize almost every pair of concurrent transfers against each other
// on the same four shards, defeating the entire point of sharding. Instead
// this function checks, twice (once optimistically before any lock, once
// authoritatively after acquiring the sender/recipient shard locks),
// whether THIS specific transfer would actually need to touch a pool
// address; if either check says yes, it returns applied=false and the
// caller falls back to the existing, proven cs.mu.Lock()-based
// transferLocked path (via runTransferBatcher) instead of trying to
// extend this fast path to lock pool shards too.
//
// Returns (fromLost, toLost, applied, err):
//   - applied=false: this path was not eligible (cold account, would touch
//     a pool address, no DB, etc.) — the caller MUST fall back to the slow
//     path. err is always nil in this case; nothing was attempted.
//   - applied=true, err!=nil: the fast path genuinely ran and genuinely
//     failed (insufficient balance, a real DB error) — a real error for
//     the caller to surface, not a "try the slow path" signal.
//   - applied=true, err==nil: success.
//
// WIRED IN as of this commit: TransferAtomic tries this first, ahead of the
// batcher (see its own comment). This is still new code with no production
// traffic history — behavior-preserving in the sense that every eligible
// transfer produces byte-identical account/StateRoot effects to the slow
// path (proven by the balance-conservation property tests alongside it),
// but the CONCURRENCY itself has NOT been validated under the
// staging/chaos campaign SCALING_ARCHITECTURE.md's own Teststrategie
// section calls non-negotiable before a production deploy. It ships here,
// on this branch, race-detector-clean under every test in this package —
// not yet proven under real production load or a dedicated chaos campaign
// (forced commit failures, forced partial-batch failures under sustained
// concurrent load) — do not treat this as a substitute for that staging
// validation before this branch is considered for a production deploy.
func (cs *ChainState) transferConcurrent(from, to string, amount float64, pendingTxTemplate Transaction) (fromLost, toLost float64, applied bool, err error) {
	if from == to {
		return 0, 0, true, fmt.Errorf("self-transfer not allowed")
	}
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, 0, true, fmt.Errorf("invalid transfer amount: %v", amount)
	}
	if cs.db == nil {
		return 0, 0, false, nil // no DB to make atomic with — let the slow (no-DB) path handle it
	}

	// cs.mu.RLock(), not Lock(): lets this run alongside other concurrent
	// transferConcurrent calls, while still fully excluding replay/swap/
	// distribution/snapshot/the slow transfer path (all cs.mu.Lock()) —
	// Go's sync.RWMutex guarantees no Lock() holder and no RLock() holder
	// ever run at the same time, in either direction.
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// Pre-check phase: existence only, via plain Get() (self-locking per
	// call, released immediately -- no shard lock held once it returns).
	// Deliberately does NOT read anything through the returned
	// *AccountState pointer, only the comma-ok bool (an ordinary local
	// copy, never shared memory). Dereferencing the pointer's fields here
	// (checking Balance, calling effectiveBalance, etc.) would race
	// against a DIFFERENT concurrent transferConcurrent call's own
	// publish-phase writes to that same account below (*fromAcc =
	// fromScratch etc.), if this address happens to be one of THAT call's
	// from/to too -- caught by -race the first time this ran under real
	// overlapping-address contention. Every field read now happens
	// exclusively after LockAddrs, via GetLocked, synchronized against
	// every other concurrent caller touching the same address.
	//
	// wealthCapAmountLocked internally Ranges over every shard -- it and
	// anything else that does the same MUST run before LockAddrs below,
	// never after: Range locks every shard in turn, including whichever
	// ones LockAddrs is about to hold, and Go's sync.Mutex is not
	// reentrant. It only touches cs.humanCount/global aggregate state,
	// never from/to's own structs, so it carries none of the race above.
	if _, ok := cs.accounts.Get(from); !ok {
		return 0, 0, false, nil // cold sender -- slow path warms it via ensureAccountLoadedCtx
	}
	if _, ok := cs.accounts.Get(to); !ok {
		return 0, 0, false, nil // cold recipient
	}
	capAmt, hasCapAmt := cs.wealthCapAmountLocked()

	// TryLockAddrs, not LockAddrs: this fast path holds whatever it
	// acquires across a real DB round trip below (Begin/save/Commit), so
	// blocking here would mean every OTHER transfer contending for the
	// same shard (e.g. many senders paying one popular recipient) queues
	// behind that whole round trip -- measured directly to be roughly 2x
	// SLOWER than just letting the batcher handle that traffic, since the
	// batcher amortizes commits across many transfers and this path
	// can't while blocked on one shard. Bailing to applied=false instead
	// sends contended traffic straight to the batcher, where it belongs;
	// only genuinely uncontended (typically disjoint) transfers pay for
	// and benefit from this path at all.
	unlock, ok := cs.accounts.TryLockAddrs(from, to)
	if !ok {
		return 0, 0, false, nil // one or both shards contended -- let the batcher have it
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
	// First (and only) point where either account's fields are actually
	// read: both shard locks are held here, and every OTHER concurrent
	// call touching either address blocks on its own LockAddrs call until
	// this one's unlock() runs -- so these reads are synchronized against
	// every possible concurrent writer, not just this call's own earlier
	// existence pre-check.
	if effectiveBalance(fromAcc) != fromAcc.Balance || effectiveBalance(toAcc) != toAcc.Balance {
		return 0, 0, false, nil // demurrage would settle -> touches a pool address
	}
	if fromAcc.Balance.Float() < amount {
		return 0, 0, true, fmt.Errorf("insufficient balance")
	}
	if hasCapAmt && toAcc.Balance.Float()+amount > capAmt {
		return 0, 0, false, nil // would overflow the wealth cap -> touches a pool address
	}

	tx, err := cs.db.Begin()
	if err != nil {
		return 0, 0, true, fmt.Errorf("could not begin concurrent transfer transaction: %w", err)
	}
	ctx := withTx(context.Background(), tx)

	// Mutate scratch COPIES, never the live fromAcc/toAcc pointers, until
	// every DB write and the commit itself have succeeded — so most
	// failure paths need no revert at all, since the live accounts were
	// never touched. The one thing a scratch copy does NOT protect is
	// updateAccountLeafLocked's mutation of cs.accountSetXOR (a global
	// field, folded in the moment a save succeeds, independent of which
	// struct pointer was passed to it) — revertAccountSetXOR below undoes
	// exactly that, for exactly the account(s) whose scratch save already
	// succeeded, whenever this function bails out after that point.
	fromScratch := *fromAcc
	toScratch := *toAcc
	fromLeafBefore := fromAcc.leafHash
	toLeafBefore := toAcc.leafHash
	fromSaved := false
	toSaved := false

	revertAccountSetXOR := func(leafAfter, leafBefore [32]byte) {
		cs.accountSetXORMu.Lock()
		xorInto(&cs.accountSetXOR, leafAfter)
		xorInto(&cs.accountSetXOR, leafBefore)
		cs.accountSetXORMu.Unlock()
	}
	abortWithRollback := func() {
		tx.Rollback()
		if fromSaved {
			revertAccountSetXOR(fromScratch.leafHash, fromLeafBefore)
		}
		if toSaved {
			revertAccountSetXOR(toScratch.leafHash, toLeafBefore)
		}
	}

	fromScratch.Balance = fromScratch.Balance.Sub(NewDecimal(amount))
	touchActivity(&fromScratch)
	if err := cs.saveAccountToDBCtx(ctx, &fromScratch); err != nil {
		abortWithRollback()
		return 0, 0, true, fmt.Errorf("could not save sender account: %w", err)
	}
	fromSaved = true

	toScratch.Balance = toScratch.Balance.Add(NewDecimal(amount))
	touchActivity(&toScratch)
	if err := cs.saveAccountToDBCtx(ctx, &toScratch); err != nil {
		abortWithRollback()
		return 0, 0, true, fmt.Errorf("could not save recipient account: %w", err)
	}
	toSaved = true

	pendingTxTemplate.FromDemurrageLost = 0
	pendingTxTemplate.ToDemurrageLost = 0
	if err := savePendingTxExec(tx, pendingTxTemplate); err != nil {
		abortWithRollback()
		return 0, 0, true, fmt.Errorf("could not queue outbox tx: %w", err)
	}

	if err := tx.Commit(); err != nil {
		// Postgres has already rolled the transaction back server-side on
		// a failed commit (same reasoning as runAtomicWithOutbox's own
		// commit-failure handling) — no explicit tx.Rollback() call here,
		// but the accountSetXOR side effects of both successful scratch
		// saves above still need undoing.
		revertAccountSetXOR(fromScratch.leafHash, fromLeafBefore)
		revertAccountSetXOR(toScratch.leafHash, toLeafBefore)
		return 0, 0, true, fmt.Errorf("commit failed: %w", err)
	}

	// Commit succeeded — publish the mutated state to the live accounts.
	// Still holding both shard locks (unlock() is deferred above), so this
	// is atomic from every other goroutine's perspective: nobody can
	// observe fromAcc/toAcc mid-update.
	*fromAcc = fromScratch
	*toAcc = toScratch

	cs.markEVMMirrorDirtyForAddrsLocked(from, to)
	return 0, 0, true, nil
}

// wealthCapAmountLocked returns (avg*multiplier, true) if a wealth cap is
// currently in effect, or (0, false) if not (no humans registered yet —
// see getAverageBalanceLocked's own comment). Split out of
// enforceWealthCapLockedCtx purely so transferConcurrent's pre-check and
// its post-lock re-validation can reuse the exact same computation the
// slow path already does, without needing an account to check it against
// yet. Caller must hold cs.mu (read or write lock — this only reads).
func (cs *ChainState) wealthCapAmountLocked() (amt float64, ok bool) {
	avg := cs.getAverageBalanceLocked()
	if avg <= 0 {
		return 0, false
	}
	return avg * cs.bootstrapMultiplierLocked(), true
}

// markEVMMirrorDirtyForAddrsLocked is transferConcurrent's counterpart to
// transferLocked's cs.syncBalanceLocked(V7_CONTRACT_ADDR, from, to,
// validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr) call —
// narrowed to just the two addresses this fast path actually touched (the
// four pool addresses are untouched by definition whenever this path was
// eligible at all). markEVMMirrorDirtyLocked is documented safe to call
// under any locking (or none) — see its own comment — so this needs no
// special treatment for running under cs.mu.RLock() instead of Lock().
func (cs *ChainState) markEVMMirrorDirtyForAddrsLocked(from, to string) {
	if cs.db == nil {
		return
	}
	cs.markEVMMirrorDirtyLocked(V7_CONTRACT_ADDR, from, to)
}
