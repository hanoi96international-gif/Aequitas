package keeper

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// registerHumanConcurrent is SCALING_ARCHITECTURE.md Phase 8's first slice:
// the same shard-locked, non-blocking-lock treatment transferConcurrent
// (Phase 5) gave the transfer path, applied to registration. Runs under
// cs.mu.RLock() plus one per-shard account lock (see
// shardedAccounts.TryLockAddrs) instead of cs.mu.Lock()'s full exclusivity,
// so registrations can happen genuinely in parallel with each other and
// with concurrent transfers, as long as their touched addresses don't
// collide.
//
// Registration is structurally DIFFERENT from Transfer in one important
// way Transfer never had to deal with: every eligible registration touches
// two pieces of truly global state unconditionally, not as an edge case --
// cs.humanCount (every registration increments it) and, when the caller
// supplies a nullifier, cs.nullifierSetXOR/cs.nullifiers (every proof-based
// registration writes one). Unlike the four tokenomics pool addresses
// (which most transfers never touch, so excluding those cases still left
// the fast path useful for the common case), these can't be excluded --
// they're the rule, not the exception. Both are protected by their own
// small dedicated mutexes (humanCountMu, nullifiersMu -- see their own
// field comments in state.go), the same accountSetXORMu pattern Phase 5
// already established, so acquiring them here is cheap and uncontended
// relative to every existing cs.mu.Lock()-based caller.
//
// Unlike Transfer, registration can also target a genuinely COLD address
// (the common case: a brand-new user's very first chain interaction) --
// excluding cold addresses the way transferConcurrent does would make this
// path nearly useless in practice, so this function does its own lightweight
// Postgres existence check for a cold address instead of unconditionally
// bailing. That check is a plain read (no cs.accounts shard lock touched),
// safe to run at any point under cs.mu.RLock() without the Range-before-
// TryLockAddrs ordering constraint wealthCapAmountLocked's own comment
// describes.
//
// Returns (applied, err):
//   - applied=false: this path was not eligible (would touch a pool address
//     via the wealth cap, a real DB error while checking cold-address
//     state) -- the caller MUST fall back to the slow,
//     cs.mu.Lock()-based RegisterHumanAtomic path. err is always nil in
//     this case; nothing was attempted.
//   - applied=true, err!=nil: the fast path genuinely ran and genuinely
//     failed (already registered, nullifier reused by a different wallet,
//     a real DB error) -- a real error for the caller to surface.
//   - applied=true, err==nil: success.
//
// NOT staging-validated, same status as every other Phase 5/7 fast path
// this session built -- see transfer_concurrent.go's own doc comment for
// what that means and why.
func (cs *ChainState) registerHumanConcurrent(address string, pendingTx Transaction) (applied bool, err error) {
	if cs.db == nil {
		return false, nil
	}
	address = strings.ToLower(address)

	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// Pre-check: already registered? Fast, lock-free reject either from a
	// warm account or (for a cold one) a direct, read-only Postgres check --
	// exactly the fact ensureAccountLoadedCtx would establish for the slow
	// path, but without that function's cs.accounts.Set (which needs the
	// shard lock, acquired further down instead).
	warmAcc, warm := cs.accounts.Get(address)
	if warm {
		if warmAcc.IsHuman {
			return true, fmt.Errorf("already registered")
		}
	} else {
		var isHuman bool
		err := cs.db.QueryRow(`SELECT is_human FROM chain_accounts WHERE lower(address) = $1`, address).Scan(&isHuman)
		if err != nil && err != sql.ErrNoRows {
			return false, nil // DB hiccup -- bail to the slow, proven path rather than guess
		}
		if err == nil && isHuman {
			return true, fmt.Errorf("already registered")
		}
	}

	// Wealth-cap pre-check: Range-based (via getAverageBalanceLocked), must
	// run before TryLockAddrs below, never after -- same reasoning as
	// transferConcurrent's own wealthCapAmountLocked call.
	capAmt, hasCapAmt := cs.wealthCapAmountLocked()
	if hasCapAmt && 1000 > capAmt {
		return false, nil // the flat 1,000 AEQ grant alone exceeds the cap -> touches a pool address
	}

	unlock, ok := cs.accounts.TryLockAddrs(address)
	if !ok {
		return false, nil
	}
	defer unlock()

	acc, existed := cs.accounts.GetLocked(address)
	if existed {
		if acc.IsHuman {
			return true, fmt.Errorf("already registered")
		}
	} else {
		acc = &AccountState{Address: address}
	}

	// Mutate a scratch COPY, never the live acc pointer, until the whole DB
	// transaction has committed -- same "scratch copy, publish only on
	// success" pattern transferConcurrent uses, for the same reason: unlike
	// transferConcurrentWAL (whose durability point, a WAL append, comes
	// BEFORE any mutation), this path's durability point is the Postgres
	// commit below, which can still fail after real work is done.
	scratch := *acc
	scratch.IsHuman = true
	scratch.Balance = scratch.Balance.Add(NewDecimal(1000))
	touchActivity(&scratch)

	// Re-validate the cap authoritatively under lock: the pre-check above
	// ran without holding the shard lock, so this address's balance (if it
	// already existed, e.g. received a transfer before ever registering)
	// could have changed in the gap.
	if hasCapAmt && scratch.Balance.Float() > capAmt {
		return false, nil
	}

	tx, err := cs.db.Begin()
	if err != nil {
		return true, fmt.Errorf("could not begin concurrent registration transaction: %w", err)
	}
	ctx := withTx(context.Background(), tx)

	leafBefore := acc.leafHash // zero value for a genuinely new account -- accountLeaf's own contract
	saved := false
	abortWithRollback := func() {
		tx.Rollback()
		if saved {
			cs.accountSetXORMu.Lock()
			xorInto(&cs.accountSetXOR, scratch.leafHash)
			xorInto(&cs.accountSetXOR, leafBefore)
			cs.accountSetXORMu.Unlock()
		}
	}

	if err := cs.saveAccountToDBCtx(ctx, &scratch); err != nil {
		abortWithRollback()
		return true, fmt.Errorf("could not save account: %w", err)
	}
	saved = true

	if pendingTx.Nullifier != "" {
		if err := cs.SaveNullifier(ctx, pendingTx.Nullifier, address); err != nil {
			abortWithRollback()
			return true, err
		}
	}

	pendingTx.Wallet = address
	if err := savePendingTxExec(cs.dbExecCtx(ctx), pendingTx); err != nil {
		abortWithRollback()
		return true, fmt.Errorf("could not queue outbox tx: %w", err)
	}

	if err := tx.Commit(); err != nil {
		// Postgres has already rolled back server-side on a failed commit
		// (same reasoning as transferConcurrent's own commit-failure
		// handling) -- no explicit tx.Rollback() here, but the accountSetXOR
		// side effect of the successful scratch save above still needs
		// undoing.
		cs.accountSetXORMu.Lock()
		xorInto(&cs.accountSetXOR, scratch.leafHash)
		xorInto(&cs.accountSetXOR, leafBefore)
		cs.accountSetXORMu.Unlock()
		return true, fmt.Errorf("commit failed: %w", err)
	}

	// Commit succeeded -- publish. Still holding the shard lock (unlock() is
	// deferred above), so this is atomic from every other goroutine's
	// perspective.
	*acc = scratch
	if !existed {
		cs.accounts.SetLocked(address, acc)
	}

	// humanCountMu/nullifiersMu: see their own field comments in state.go --
	// this is the concurrent path those mutexes exist for.
	cs.humanCountMu.Lock()
	cs.humanCount++
	cs.humanCountMu.Unlock()

	cs.markEVMMirrorDirtyLocked(V7_CONTRACT_ADDR, address)

	fmt.Printf("[STATE] ✓ Human registered (concurrent path): %s | Balance: %.2f AEQ\n", address, acc.Balance.Float())
	return true, nil
}
