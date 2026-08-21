package keeper

import (
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

// WAL flush tuning, made measurable and settable without a rebuild.
//
// WHY. A mutex profile taken on Contabo2 under 597-sender load attributed
// 45.21% of ALL lock contention in the node to one place: flushWALBatch
// holding cs.accounts.LockAddrs across its whole Postgres transaction. That
// hold is deliberate and must not be narrowed — the function's own comment
// records two earlier attempts to release those locks early, both of which
// reopened Postgres deadlocks (40P01), measured at up to 23.6% failed
// transfers at 500 concurrent senders.
//
// What CAN change is how many addresses a single flush covers. walFlushMaxBatch
// is 4,000, so one flush can touch up to 8,000 distinct addresses. Against the
// 1,195-account working set the load test drives, that batch dedupes to
// essentially the ENTIRE address space — every flush holds nearly every shard
// the fast path wants, and every transfer that collides falls through to the
// slower batcher path. transfer_wal.go's own comment already predicted exactly
// this ("a 4,000-item batch drawn from only 200 addresses dedupes to nearly the
// WHOLE address space"), and deliberately kept 4,000 anyway because real
// traffic was far below either synthetic topology at the time. That same
// comment asks for both parameters to be re-measured "once Contabo2's actual
// active-address count has grown enough to make the large-topology numbers more
// representative than the small one" — which is precisely the condition a
// sustained 597-sender run creates.
//
// WHY ENV RATHER THAN A NEW CONSTANT. The right value is a property of the
// traffic, not of the code: the previous measurement found +32% at 1,000 pairs
// and -14% at 100 pairs for the SAME setting. Picking one number in the source
// would just repeat that mistake at a new topology. Making them settable lets
// the value be swept against real load on real hardware, and lets an operator
// retune later without a deploy.
//
// Defaults are unchanged, so a node that sets nothing behaves exactly as before.

// walFlushStats records what the flush loop actually did, so a swept value is
// judged on the mechanism rather than on throughput alone. Throughput on this
// node has repeatedly moved less than its own run-to-run spread, which has
// twice nearly caused a real effect to be dismissed — direct counters settle
// that where a before/after comparison cannot.
var walFlushStats struct {
	flushes    atomic.Int64
	items      atomic.Int64
	addrs      atomic.Int64
	holdNanos  atomic.Int64
	maxHoldMs  atomic.Int64
	dbNanos    atomic.Int64
	maxBatchNo atomic.Int64

	// Per-phase, because "the Postgres transaction is 27 of the 30ms" is not
	// yet an answer -- that window also contains pure Go work (building two
	// multi-row statements, JSON-marshaling one outbox row per item) which
	// needs no lock at all and could move out of the critical section. Without
	// splitting it, the fix would be a guess between "make the database faster"
	// and "stop doing CPU work while holding 346 account shards".
	snapNanos   atomic.Int64
	acctSQLNs   atomic.Int64
	acctExecNs  atomic.Int64
	outboxSQLNs atomic.Int64
	outboxExecN atomic.Int64
	commitNanos atomic.Int64
}

// noteWALFlushPhases records the breakdown of one flush's database window.
func noteWALFlushPhases(snap, acctSQL, acctExec, outboxSQL, outboxExec, commit time.Duration) {
	walFlushStats.snapNanos.Add(int64(snap))
	walFlushStats.acctSQLNs.Add(int64(acctSQL))
	walFlushStats.acctExecNs.Add(int64(acctExec))
	walFlushStats.outboxSQLNs.Add(int64(outboxSQL))
	walFlushStats.outboxExecN.Add(int64(outboxExec))
	walFlushStats.commitNanos.Add(int64(commit))
}

// applyWALTuningFromEnv overrides the flush parameters when asked. Called from
// initWALIfEnabled, i.e. before any flush worker starts, so the values are
// fixed for the process's lifetime and no worker can observe a torn change.
func applyWALTuningFromEnv() {
	if v := envPositiveInt("AEQUITAS_WAL_FLUSH_BATCH"); v > 0 {
		fmt.Printf("[WAL] flush batch size overridden: %d -> %d\n", walFlushMaxBatch, v)
		walFlushMaxBatch = v
	}
	if v := envPositiveInt("AEQUITAS_WAL_FLUSH_CONCURRENCY"); v > 0 {
		fmt.Printf("[WAL] flush concurrency overridden: %d -> %d\n", walFlushConcurrency, v)
		walFlushConcurrency = v
	}
	if v := envPositiveInt("AEQUITAS_WAL_QUEUE_DEPTH"); v > 0 {
		fmt.Printf("[WAL] flush queue depth overridden: %d -> %d\n", walFlushMaxQueueDepth, v)
		walFlushMaxQueueDepth = v
	}

	// The interval, not the batch cap, decides how much of the address space one
	// flush freezes.
	//
	// Measured under load on 2026-08-21: items_per_flush was 402 against a
	// cfg_batch of 4,000 -- the cap was never reached, so it bound nothing, which
	// is why sweeping it moved almost nothing. What did move is addrs_per_flush:
	// 404 of a ~716-account working set, held 43ms on average and up to 712ms. A
	// flush takes whatever arrived during the interval, so at 3,000 transfers/s a
	// 100ms interval collects ~300 of them and, after deduplication, most of the
	// addresses the fast path wants.
	//
	// Shorter means fewer addresses frozen for less time, which is the only lever
	// left on the hold a mutex profile put at 45.90% of this node's entire lock
	// contention. The hold itself must not be narrowed -- flushWALBatch's comment
	// records two attempts that reopened Postgres deadlocks.
	//
	// Env rather than a constant, like the others here: the right value depends on
	// arrival rate and working-set size, both properties of the traffic.
	// walFlushInterval's own comment asks for TestSustainedWAL_QueueConvergence
	// (AEQUITAS_WAL_SUSTAINED_BENCH=1) to be re-run after any change near this
	// code, because round 1 of that investigation found short intervals trading
	// throughput for boundedness.
	if v := envPositiveInt("AEQUITAS_WAL_FLUSH_INTERVAL_MS"); v > 0 {
		fmt.Printf("[WAL] flush interval overridden: %s -> %dms\n", walFlushInterval, v)
		walFlushInterval = time.Duration(v) * time.Millisecond
	}
}

// envPositiveInt reads a positive integer, or 0 when unset or unusable. A
// malformed value is reported rather than silently ignored: a tuning run that
// measures the default while believing it measured an override is worse than
// one that fails loudly, and this project has lost a full day to exactly that
// class of mistake.
func envPositiveInt(name string) int {
	raw := os.Getenv(name)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		fmt.Printf("[WAL] ⚠ %s=%q is not a positive integer — IGNORED, the default stays in effect\n", name, raw)
		return 0
	}
	return n
}

// noteWALFlush records one completed flush. dbDur is the time inside the
// Postgres transaction; hold is the whole time the address shard locks were
// held, which is what other goroutines actually wait on.
func noteWALFlush(items, addrs int, hold, dbDur time.Duration) {
	walFlushStats.flushes.Add(1)
	walFlushStats.items.Add(int64(items))
	walFlushStats.addrs.Add(int64(addrs))
	walFlushStats.holdNanos.Add(int64(hold))
	walFlushStats.dbNanos.Add(int64(dbDur))
	if ms := hold.Milliseconds(); ms > walFlushStats.maxHoldMs.Load() {
		walFlushStats.maxHoldMs.Store(ms)
	}
	if int64(items) > walFlushStats.maxBatchNo.Load() {
		walFlushStats.maxBatchNo.Store(int64(items))
	}
}

// WALFlushStats reports the flush loop's behaviour for an operator endpoint.
//
// The averages are what the sweep is judged on: addrs_per_flush says how much
// of the address space a single flush freezes, and hold_avg_ms says for how
// long. Those two multiplied are the contention this whole exercise is about;
// throughput alone cannot separate them.
func WALFlushStats() map[string]interface{} {
	n := walFlushStats.flushes.Load()
	out := map[string]interface{}{
		"flushes":         n,
		"items":           walFlushStats.items.Load(),
		"max_batch_items": walFlushStats.maxBatchNo.Load(),
		"hold_max_ms":     walFlushStats.maxHoldMs.Load(),
		"cfg_batch":       walFlushMaxBatch,
		"cfg_interval_ms": walFlushInterval.Milliseconds(),
		"cfg_concurrency": walFlushConcurrency,
		"cfg_queue_depth": walFlushMaxQueueDepth,
	}
	if n > 0 {
		out["items_per_flush"] = walFlushStats.items.Load() / n
		out["addrs_per_flush"] = walFlushStats.addrs.Load() / n
		out["hold_avg_ms"] = (walFlushStats.holdNanos.Load() / n) / 1e6
		out["db_avg_ms"] = (walFlushStats.dbNanos.Load() / n) / 1e6
		// Microseconds, not milliseconds: the point of the split is to separate
		// phases that may well be under 1ms each, and rounding them to whole
		// milliseconds would report most of them as 0.
		out["p1_snapshot_us"] = (walFlushStats.snapNanos.Load() / n) / 1e3
		out["p2_acct_sql_us"] = (walFlushStats.acctSQLNs.Load() / n) / 1e3
		out["p3_acct_exec_us"] = (walFlushStats.acctExecNs.Load() / n) / 1e3
		out["p4_outbox_sql_us"] = (walFlushStats.outboxSQLNs.Load() / n) / 1e3
		out["p5_outbox_exec_us"] = (walFlushStats.outboxExecN.Load() / n) / 1e3
		out["p6_commit_us"] = (walFlushStats.commitNanos.Load() / n) / 1e3
	}
	return out
}
