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

	// doSyncBalanceLocked needs cs.mu (write lock — it can page in a cold
	// account via ensureAccountLoaded), and does its own DB write outside
	// any caller's transaction (the whole point of deferring this past the
	// original operation's own commit/rollback — see syncBalanceLocked's
	// comment). One lock acquisition per contract group, not per address.
	// Instrumented because it was not: exclusive_lock_stats covered two of
	// the package's 85 exclusive sites, and this one -- a DISPLAY-ONLY mirror
	// for eth_call, by its own documentation -- takes the node's global write
	// lock every 2 seconds and holds it across a DB write per dirty address.
	// Under load every transfer marks two addresses dirty, so that is the
	// whole working set, every cycle. Transfers measured 45.88ms of an
	// 87.80ms transfer just acquiring cs.mu.RLock(), and Go's RWMutex blocks
	// every new reader as soon as a writer is waiting.
	cs.mu.Lock()
	// AFTER the Lock, not before: trackExclusiveHold measures how long other
	// goroutines are shut out, not how long this worker queued. Timing from
	// the attempt would inflate busy_pct and make it incomparable with the
	// block-replay figure it sits next to.
	exclusiveAcquired := time.Now()
	defer trackExclusiveHold(exclusiveAcquired, "EVM mirror flush")
	defer cs.mu.Unlock()
	for contractAddr, addrs := range byContract {
		cs.doSyncBalanceLocked(contractAddr, addrs...)
	}
}

// FlushEVMMirrorNow forces an immediate flush of any pending EVM-mirror
// syncs, bypassing the ticker. Used on graceful shutdown (see main.go,
// alongside FlushPoolAccountsNow) so a routine restart never depends on
// evmMirrorFlushInterval's normal display-lag window. Safe to call even if
// the flush worker was never started.
func (cs *ChainState) FlushEVMMirrorNow() {
	cs.flushEVMMirrorDirty()
}
