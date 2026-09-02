package keeper

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// How much of the time is every concurrent transfer locked out?
//
// cs.mu is not really a data lock. Concurrent transfers take it in READ mode
// and protect themselves against each other with per-account shard locks
// (see transferConcurrentWAL); operations that cannot tolerate that — block
// replay, distributions, snapshots — take it in WRITE mode. So it is a
// concurrent-mode versus exclusive-mode gate.
//
// Go's RWMutex blocks NEW readers as soon as a writer is queued, which is what
// keeps writers from starving. The consequence is that an exclusive holder
// stops every transfer on the node for as long as it holds, and replay holds it
// for a whole block: config backup, rollback snapshot, every delta including
// its database writes, the StateRoot comparison, and any rollback. That scope
// is deliberate — the comment at the lock explains that rollback atomicity
// depends on it — so it cannot simply be narrowed.
//
// Measured under load on Contabo2 (2026-07-26): 67 goroutines waiting in
// sync.RWMutex.RLock, the single largest lock wait once the RPC server's own
// mutex had been sharded away. What was NOT known is how much of the wall
// clock the exclusive side actually occupies, and that is the number that
// decides whether touching a deliberately-atomic scope is worth the risk.
//
// If exclusive mode occupies a small fraction, the waiting readers are a
// queueing artefact and the answer lies elsewhere. If it occupies a large
// fraction, concurrent throughput is capped at whatever is left over, and no
// amount of work on the transfer path can exceed that ceiling.

var (
	exclusiveHoldNanos atomic.Int64 // cumulative time spent holding cs.mu exclusively
	exclusiveHoldCount atomic.Int64 // how many exclusive holds
	exclusiveHoldMax   atomic.Int64 // longest single hold
	exclusiveStatsFrom atomic.Int64 // when measurement started, for the fraction

	// Per-caller breakdown. The totals alone cannot answer the question that
	// now matters: after the EVM mirror stopped taking this lock,
	// exclusive_busy_pct fell to 0.65% -- yet transfers still wait 18.05ms
	// acquiring cs.mu.RLock() across 1,851 holds. Go blocks new readers the
	// moment a writer ARRIVES, so with holds this short the count matters more
	// than the duration, and "who arrives 1,851 times" is not something the
	// aggregate can say.
	//
	// sync.Map because the label set is tiny, fixed at compile time, and read
	// far less often than written.
	exclusiveByLabel sync.Map // label -> *labelStat
)

type labelStat struct {
	nanos atomic.Int64
	count atomic.Int64
	max   atomic.Int64
}

// exclusiveHoldWarnThreshold is when a single hold is worth a log line. At
// BLOCK_TIME=1s, a hold of a quarter second means a quarter of that second had
// no transfers running at all.
const exclusiveHoldWarnThreshold = 250 * time.Millisecond

// trackExclusiveHold records one exclusive hold. Call it with the time the
// lock was ACQUIRED, deferred right after the Lock():
//
//	dag.state.mu.Lock()
//	defer trackExclusiveHold(time.Now(), "replay")
//	defer dag.state.mu.Unlock()
//
// Deliberately measures from acquisition rather than from the attempt: the
// question is how long OTHER goroutines are shut out, not how long this one
// queued.
func trackExclusiveHold(acquired time.Time, what string) {
	held := time.Since(acquired)
	exclusiveStatsFrom.CompareAndSwap(0, acquired.UnixNano())
	exclusiveHoldNanos.Add(int64(held))
	exclusiveHoldCount.Add(1)

	v, _ := exclusiveByLabel.LoadOrStore(what, &labelStat{})
	st := v.(*labelStat)
	st.nanos.Add(int64(held))
	st.count.Add(1)
	for {
		prev := st.max.Load()
		if int64(held) <= prev || st.max.CompareAndSwap(prev, int64(held)) {
			break
		}
	}
	for {
		prev := exclusiveHoldMax.Load()
		if int64(held) <= prev || exclusiveHoldMax.CompareAndSwap(prev, int64(held)) {
			break
		}
	}
	if held >= exclusiveHoldWarnThreshold {
		fmt.Printf("[LOCK] ⚠ %s held the exclusive state lock for %s — every concurrent transfer on this node was blocked for that entire time\n",
			what, held.Round(time.Millisecond))
	}
}

// ExclusiveLockStats reports what share of the wall clock has been spent with
// every concurrent transfer locked out.
//
// busy_pct is the number that matters: it is an upper bound on how much of the
// time the node could accept transfers at all, no matter how well the transfer
// path itself performs.
func ExclusiveLockStats() map[string]interface{} {
	from := exclusiveStatsFrom.Load()
	count := exclusiveHoldCount.Load()
	total := exclusiveHoldNanos.Load()
	out := map[string]interface{}{
		"exclusive_holds":    count,
		"exclusive_total_ms": total / int64(time.Millisecond),
		"exclusive_max_ms":   exclusiveHoldMax.Load() / int64(time.Millisecond),
		"exclusive_avg_ms":   int64(0),
		"exclusive_busy_pct": 0.0,
		"measured_over_secs": int64(0),
	}
	if count > 0 {
		out["exclusive_avg_ms"] = total / count / int64(time.Millisecond)
	}
	if from > 0 {
		window := time.Since(time.Unix(0, from))
		out["measured_over_secs"] = int64(window.Seconds())
		if window > 0 {
			out["exclusive_busy_pct"] = float64(total) / float64(window) * 100
		}
	}

	// Per caller. Read the COUNT column first: a writer that arrives often
	// convoys readers often, however briefly it holds.
	by := map[string]interface{}{}
	exclusiveByLabel.Range(func(k, v interface{}) bool {
		st := v.(*labelStat)
		c := st.count.Load()
		e := map[string]interface{}{
			"holds":    c,
			"total_ms": st.nanos.Load() / int64(time.Millisecond),
			"max_ms":   st.max.Load() / int64(time.Millisecond),
			"avg_ms":   int64(0),
		}
		if c > 0 {
			e["avg_ms"] = st.nanos.Load() / c / int64(time.Millisecond)
		}
		by[k.(string)] = e
		return true
	})
	out["by_caller"] = by
	return out
}
