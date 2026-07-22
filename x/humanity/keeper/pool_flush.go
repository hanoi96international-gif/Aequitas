package keeper

import (
	"fmt"
	"time"
)

// poolFlushInterval is how often the background worker persists whatever
// pool-address credits have accumulated in memory since the last flush —
// see SCALING_ARCHITECTURE.md Phase 3. Picked from the doc's own suggested
// "alle 1-5 Sekunden" range: short enough that the crash-loss window (any
// pool credit not yet durably in Postgres if the process dies before the
// next tick) stays small in absolute terms, long enough that a burst of
// many transfers in one window still collapses into a single DB round trip
// instead of one per transfer.
const poolFlushInterval = 2 * time.Second

// markPoolAccountsDirtyLocked records that the four tokenomics-pool
// AccountState pointers in cs.accounts have a balance change not yet
// written to Postgres, and lazily starts the background worker that will
// eventually persist it. Caller must hold cs.mu (called from
// distributeSwapFee, itself always invoked with cs.mu already held).
func (cs *ChainState) markPoolAccountsDirtyLocked() {
	cs.poolFlushDirty.Store(true)
	cs.ensurePoolFlushWorkerStarted()
}

// ensurePoolFlushWorkerStarted lazily starts the one background goroutine
// that periodically flushes pool-address credits — lazy (not started in
// NewChainState), exactly like ensureTransferBatcherStarted, so a node (or
// a test) that never triggers a fee event never pays for an idle goroutine.
func (cs *ChainState) ensurePoolFlushWorkerStarted() {
	cs.poolFlushOnce.Do(func() {
		SafeGoroutine("poolFlushWorker", cs.runPoolFlushWorker)
	})
}

// runPoolFlushWorker ticks every poolFlushInterval and flushes whatever pool
// credits have accumulated since the previous tick. Runs for the lifetime of
// the process once started — matches every other ticker-based background
// worker in this codebase (see e.g. autoheal.go, block.go's
// pruneOldDAGBlocks-ticker).
func (cs *ChainState) runPoolFlushWorker() {
	ticker := time.NewTicker(poolFlushInterval)
	defer ticker.Stop()
	for range ticker.C {
		cs.flushPoolAccountsIfDirty()
	}
}

// flushPoolAccountsIfDirty is the actual flush, split out from
// runPoolFlushWorker so tests (and FlushPoolAccountsNow, used on graceful
// shutdown) can trigger it directly without waiting on the ticker.
//
// The dirty check-and-clear happens via CompareAndSwap on the atomic flag,
// entirely without cs.mu -- the common case (nothing changed since the last
// flush) then costs nothing but one atomic load, not a lock acquisition.
// Only when there is real work does this take cs.mu, and it holds that lock
// for the ENTIRE snapshot+DB-write (deliberately NOT released around the
// network round trip): saveAccountsToDBBatch mutates cs.accountSetXOR, which
// is only ever safe to touch under cs.mu (see ChainState's own field
// comment) -- releasing the lock around the write would race that mutation
// against every other cs.mu-protected accountSetXOR update in this file,
// exactly the class of bug this codebase has already been bitten by more
// than once (see SCALING_ARCHITECTURE.md's "Historischer Kontext"). Holding
// cs.mu here is a deliberate, safe trade-off: one Postgres round trip every
// poolFlushInterval, shared across however many fee events happened in that
// window, instead of one lock-free write per event -- a large reduction in
// total round trips even though this specific one still serializes against
// other operations, same as every write in this codebase already does today.
func (cs *ChainState) flushPoolAccountsIfDirty() {
	if !cs.poolFlushDirty.CompareAndSwap(true, false) {
		return
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	accs := make([]*AccountState, 0, 4)
	for _, addr := range []string{validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr} {
		if acc, ok := cs.accounts.Get(addr); ok {
			accs = append(accs, acc)
		}
	}
	if len(accs) == 0 {
		return
	}
	if err := cs.saveAccountsToDBBatch(accs); err != nil {
		// Persistence failed (transient DB issue) -- re-mark dirty so the
		// NEXT tick retries. The in-memory balances (and accountSetXOR) are
		// already correct regardless of this failure, since
		// updateAccountLeafLocked ran synchronously at credit time; only
		// Postgres durability is delayed further.
		cs.poolFlushDirty.Store(true)
		fmt.Printf("[POOL-FLUSH] ✗ could not persist pending pool credits (will retry next tick): %v\n", err)
	}
}

// FlushPoolAccountsNow forces an immediate flush of any pending pool-address
// credits, bypassing the ticker. Used on graceful shutdown (see main.go) so
// a routine restart never depends on poolFlushInterval's normal "eventually
// consistent" window. Safe to call even if the flush worker was never
// started (nothing to flush yet -- CompareAndSwap simply reports false).
func (cs *ChainState) FlushPoolAccountsNow() {
	cs.flushPoolAccountsIfDirty()
}
