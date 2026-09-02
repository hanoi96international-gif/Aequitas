package keeper

import (
	"sync/atomic"
	"time"
)

// Where do a transfer's 78ms actually go? None of the obvious answers survive.
//
// TransferAtomic measures 78.5ms under load while every mechanism that could
// plausibly cost that much has been measured and cleared:
//
//	TryLockAddrs never waits          fast_path_pct 99.82%, so almost nothing
//	                                  is bailing on a contended shard
//	enqueueWALFlushLocked is an append under its own small mutex, non-blocking
//	the exclusive state lock          exclusive_busy_pct 0.35%
//	wal.Append's group commit         sync_avg 6.8ms, writer at ~46% duty
//
// Adding those up gives roughly 7ms, not 78. So something between the start of
// TransferAtomic and its return is costing an order of magnitude more than the
// parts anyone has looked at, and no instrument covers it.
//
// This splits the fast path the same way rpc_phase_stats.go split the request,
// and for the same reason: the phases sum to the whole, so the answer comes
// out as a subtraction rather than a hypothesis. The residual bucket is the
// important one — if `other` holds most of the time, the cost is in code none
// of the named phases cover, and that is worth knowing precisely.

var (
	txPhasePrecheckNanos atomic.Int64 // eligibility checks before locking
	// Precheck broken down. It was 46ms before the blocking Get calls came
	// out and is still 23ms, which is more than the WAL sync -- and nothing
	// distinguishes its three parts. The suspect is cs.mu.RLock(): a Go
	// RWMutex gives writers priority, flushWALBatch holds RLock across its
	// entire 37ms Postgres transaction with up to 32 workers doing so at once,
	// and the moment block production calls Lock() every subsequent reader
	// queues behind it. exclusive_busy_pct measures the HOLD (0.35%), never
	// the stall it induces, so that number has been quietly misread as
	// evidence the exclusive lock is cheap.
	txPhaseQueueNanos   atomic.Int64 // WALFlushQueueDepth (its own small mutex)
	txPhaseRLockNanos   atomic.Int64 // cs.mu.RLock() acquisition
	txPhaseCapNanos     atomic.Int64 // wealthCapAmountLocked (humanCountMu)
	txPhaseLockNanos    atomic.Int64 // TryLockAddrs itself
	txPhaseApplyNanos   atomic.Int64 // in-memory balance mutation + encode
	txPhaseAppendNanos  atomic.Int64 // wal.Append -- waits for the group commit
	txPhaseEnqueueNanos atomic.Int64 // handing the item to the flush queue
	txPhaseTotalNanos   atomic.Int64 // the whole fast path
	txPhaseCount        atomic.Int64

	// Appends slower than this are counted separately: the writer's sync_max
	// is 316ms against a 6.8ms average, so a mean alone would hide whether
	// transfers routinely land on the tail or only rarely.
	txPhaseSlowAppends atomic.Int64
)

const slowAppendThreshold = 20 * time.Millisecond

type transferPhases struct {
	precheck, lock, apply, append_, enqueue time.Duration
	queue, rlock, cap                       time.Duration
	start                                   time.Time
}

func beginTransferPhases() *transferPhases {
	return &transferPhases{start: time.Now()}
}

func (p *transferPhases) record() {
	if p == nil {
		return
	}
	txPhasePrecheckNanos.Add(int64(p.precheck))
	txPhaseQueueNanos.Add(int64(p.queue))
	txPhaseRLockNanos.Add(int64(p.rlock))
	txPhaseCapNanos.Add(int64(p.cap))
	txPhaseLockNanos.Add(int64(p.lock))
	txPhaseApplyNanos.Add(int64(p.apply))
	txPhaseAppendNanos.Add(int64(p.append_))
	txPhaseEnqueueNanos.Add(int64(p.enqueue))
	txPhaseTotalNanos.Add(int64(time.Since(p.start)))
	txPhaseCount.Add(1)
	if p.append_ >= slowAppendThreshold {
		txPhaseSlowAppends.Add(1)
	}
}

// TransferPhaseStats reports the fast path split so the unexplained time can
// be named instead of guessed at.
//
// Read `other_ms` first. It is total minus every named phase: whatever is left
// is time spent in code no instrument covers, and if it dominates then every
// hypothesis about locks, syncs and queues is looking in the wrong place.
func TransferPhaseStats() map[string]interface{} {
	n := txPhaseCount.Load()
	if n == 0 {
		return map[string]interface{}{"transfers": 0}
	}

	total := msPer(txPhaseTotalNanos.Load(), n)
	pre := msPer(txPhasePrecheckNanos.Load(), n)
	lock := msPer(txPhaseLockNanos.Load(), n)
	apply := msPer(txPhaseApplyNanos.Load(), n)
	app := msPer(txPhaseAppendNanos.Load(), n)
	enq := msPer(txPhaseEnqueueNanos.Load(), n)

	other := total - pre - lock - apply - app - enq
	if other < 0 {
		other = 0
	}

	return map[string]interface{}{
		"transfers":   n,
		"total_ms":    total,
		"precheck_ms": pre,
		// The three parts of precheck. They sum to it; whichever dominates is
		// the next thing to fix.
		"pre_queue_ms":    msPer(txPhaseQueueNanos.Load(), n),
		"pre_rlock_ms":    msPer(txPhaseRLockNanos.Load(), n),
		"pre_cap_ms":      msPer(txPhaseCapNanos.Load(), n),
		"lock_ms":         lock,
		"apply_ms":        apply,
		"wal_append_ms":   app,
		"enqueue_ms":      enq,
		"other_ms":        other,
		"slow_appends":    txPhaseSlowAppends.Load(),
		"slow_append_pct": float64(txPhaseSlowAppends.Load()) / float64(n) * 100,
	}
}
