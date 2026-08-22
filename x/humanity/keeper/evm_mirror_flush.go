package keeper

import (
	"strings"
	"time"
)

// evmMirrorFlushInterval is how often the background worker drains
// evmMirrorDirty and performs the deferred EVM-mirror writes — see
// SCALING_ARCHITECTURE.md Phase 6. Same value and same reasoning as
// poolFlushInterval (pool_flush.go): short enough that the display lag
// stays unnoticeable, long enough that a burst of transfers collapses into
// one round trip instead of one per transfer.
const evmMirrorFlushInterval = 2 * time.Second

// evmMirrorFlushChunk is how many addresses one exclusive hold covers.
//
// Sized against what was measured rather than picked round: a flush under load
// carried ~371 addresses in a single hold. At 64 that becomes six short holds,
// each leaving a gap for waiting transfers, without turning one stall into
// hundreds of lock acquisitions with their own overhead.
const evmMirrorFlushChunk = 64

// evmMirrorDirtyKey identifies one (address, contract) pair awaiting an
// async EVM-mirror-sync flush.
type evmMirrorDirtyKey struct {
	addr, contractAddr string
}

// markEVMMirrorDirtyLocked records addrs as needing an EVM-mirror refresh
// for contractAddr and lazily starts the flush worker. Cheap and lock-free
// with respect to cs.mu (uses its own small mutex) so this can run inline
// on every transfer without adding contention on the lock every other
// operation already needs. Safe to call whether or not the caller currently
// holds cs.mu (today's callers all do, via syncBalanceLocked, but nothing
// here depends on that).
func (cs *ChainState) markEVMMirrorDirtyLocked(contractAddr string, addrs ...string) {
	contractAddr = strings.ToLower(contractAddr)
	cs.evmMirrorDirtyMu.Lock()
	if cs.evmMirrorDirty == nil {
		cs.evmMirrorDirty = make(map[evmMirrorDirtyKey]struct{})
	}
	for _, addr := range addrs {
		cs.evmMirrorDirty[evmMirrorDirtyKey{strings.ToLower(addr), contractAddr}] = struct{}{}
	}
	cs.evmMirrorDirtyMu.Unlock()
	cs.ensureEVMMirrorFlushWorkerStarted()
}

// ensureEVMMirrorFlushWorkerStarted lazily starts the one background
// goroutine that periodically flushes pending EVM-mirror syncs — lazy (not
// started in NewChainState), exactly like ensureTransferBatcherStarted and
// ensurePoolFlushWorkerStarted, so a node (or test) that never triggers a
// balance-affecting operation never pays for an idle goroutine.
func (cs *ChainState) ensureEVMMirrorFlushWorkerStarted() {
	cs.evmMirrorFlushOnce.Do(func() {
		SafeGoroutine("evmMirrorFlushWorker", cs.runEVMMirrorFlushWorker)
	})
}

// runEVMMirrorFlushWorker ticks every evmMirrorFlushInterval and flushes
// whatever addresses have accumulated since the previous tick. Runs for the
// lifetime of the process once started, matching every other ticker-based
// background worker in this codebase.
func (cs *ChainState) runEVMMirrorFlushWorker() {
	ticker := time.NewTicker(evmMirrorFlushInterval)
	defer ticker.Stop()
	for range ticker.C {
		cs.flushEVMMirrorDirty()
	}
}

// flushEVMMirrorDirty drains evmMirrorDirty and performs the deferred
// writes, grouped by contract address (doSyncBalanceLocked's batched INSERT
// is per-contract). Split out from runEVMMirrorFlushWorker so tests (and a
// future graceful-shutdown hook, mirroring FlushPoolAccountsNow) can trigger
// it directly without waiting on the ticker.
func (cs *ChainState) flushEVMMirrorDirty() {
	if cs.db == nil {
		return
	}
	cs.evmMirrorDirtyMu.Lock()
	if len(cs.evmMirrorDirty) == 0 {
		cs.evmMirrorDirtyMu.Unlock()
		return
	}
	byContract := make(map[string][]string)
	for k := range cs.evmMirrorDirty {
		byContract[k.contractAddr] = append(byContract[k.contractAddr], k.addr)
	}
	cs.evmMirrorDirty = nil
	cs.evmMirrorDirtyMu.Unlock()

	// MEASURED 2026-08-22, and it is the reason this is chunked.
	//
	// Instrumenting this call site (exclusive_lock_stats covered two of the
	// package's 85 exclusive sites) moved exclusive_busy_pct from 0.21% to
	// 50.26% on Contabo1, with a single hold of 8,221 ms. Half the wall clock,
	// the node could not run a transfer or replay a block at all.
	//
	// It ran as ONE cs.mu.Lock() over every dirty address. Under load each
	// transfer marks two addresses dirty, so a 2-second cycle covers the whole
	// working set, and doSyncBalanceLocked writes to Postgres per address --
	// all of it under the node's global write lock. Go's RWMutex blocks every
	// new reader the moment a writer waits, so transfers measured 45.88 ms of
	// an 87.80 ms transfer just acquiring cs.mu.RLock().
	//
	// This is a DISPLAY-ONLY mirror. It backs eth_call and MetaMask, feeds no
	// StateRoot and no consensus. A cosmetic subsystem was holding the lock the
	// payment path runs on.
	//
	// WHY CHUNKING IS SAFE HERE, where it would not be everywhere: the flush is
	// already eventually consistent by construction. It reads cs.accounts fresh
	// at flush time, after whatever marked an address dirty has committed or
	// rolled back, and anything modified between chunks is re-marked and picked
	// up next cycle -- see syncBalanceLocked's own comment. So splitting the
	// work changes when the mirror catches up, never what it converges to. The
	// lock is still taken; it is just no longer held across the entire set.
	//
	// Still one Lock per chunk rather than per address: doSyncBalanceLocked can
	// page in a cold account via ensureAccountLoaded, which needs the write
	// lock, and a lock acquisition per address would trade one long stall for
	// thousands of short ones.
	for contractAddr, addrs := range byContract {
		for start := 0; start < len(addrs); start += evmMirrorFlushChunk {
			stop := start + evmMirrorFlushChunk
			if stop > len(addrs) {
				stop = len(addrs)
			}
			cs.flushEVMMirrorChunk(contractAddr, addrs[start:stop])
		}
	}
}

// flushEVMMirrorChunk holds the exclusive lock for one bounded slice and
// releases it before the next, so a transfer arriving mid-flush waits for one
// chunk rather than for the entire dirty set.
//
// Separated into its own function so the unlock is a defer rather than a
// hand-placed call inside a loop: an early return added later would otherwise
// leave the node's global write lock held forever.
func (cs *ChainState) flushEVMMirrorChunk(contractAddr string, addrs []string) {
	cs.mu.Lock()
	// AFTER the Lock, not before: trackExclusiveHold measures how long other
	// goroutines are shut out, not how long this worker queued. Timing from the
	// attempt would inflate busy_pct and make it incomparable with the
	// block-replay figure it is reported beside.
	acquired := time.Now()
	defer trackExclusiveHold(acquired, "EVM mirror flush")
	defer cs.mu.Unlock()
	cs.doSyncBalanceLocked(contractAddr, addrs...)
}

// FlushEVMMirrorNow forces an immediate flush of any pending EVM-mirror
// syncs, bypassing the ticker. Used on graceful shutdown (see main.go,
// alongside FlushPoolAccountsNow) so a routine restart never depends on
// evmMirrorFlushInterval's normal display-lag window. Safe to call even if
// the flush worker was never started.
func (cs *ChainState) FlushEVMMirrorNow() {
	cs.flushEVMMirrorDirty()
}
