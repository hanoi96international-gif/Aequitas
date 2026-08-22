package keeper

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
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
	// At is the instant the transfer actually happened, in unix seconds.
	//
	// FIX (pre-launch audit 2026-08-16): the record used to carry no timestamp
	// at all, so recoverFromWAL had nothing it COULD stamp the activity clock
	// with and called plain touchActivity(acc) — nowUnix(), i.e. whenever the
	// node happened to restart. Every crash-recovered transfer therefore reset
	// both participants' demurrage clock forward by the length of the outage,
	// handing them unearned demurrage-free time at the tokenomics pools'
	// expense, purely as a function of how long the node was down. Block
	// replay already avoids exactly this by stamping the block's own timestamp
	// (see touchActivityAt's doc comment); WAL recovery could not, for want of
	// this field.
	//
	// Records written before this field existed decode as 0, and
	// touchActivityAt treats a non-positive value as "use now" — the old
	// behaviour, confined to WAL files that predate the upgrade.
	At int64 `json:"at"`
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

// walFlushInterval originally mirrored evmMirrorFlushInterval/
// poolFlushInterval's own reasoning (short enough that Explorer/other-
// validator lag stays close to unnoticeable, long enough that a burst
// collapses into one round trip) at 500ms. That reasoning fit those two
// paths -- EVM-mirror sync is documented as just a display cache, pool
// distribution is periodic, not per-transfer -- but not this one: WAL
// reconciliation is now the highest-volume path in the system.
//
// INVESTIGATION (2026-07-23, 50k-TPS-target profiling), two rounds:
//
// Round 1 (short benchmark, misleading): a CPU profile of
// TestSimulateMaxTPS_WarmSteadyState (8-20s, ~40,000 TPS) showed
// enqueueWALFlushLocked at 27.8% of CPU time, dominated by
// runtime.growslice/memmove -- the original 500-items/500ms drain rate
// (1,000/sec) is far below real fast-path throughput, so the queue grows
// during any sustained load. Three fixes tried against that SAME short
// benchmark (25,000/500ms, 500/5ms, 5,000/50ms) each measured WORSE than
// the 500/500ms original -- traced to flushWALBatch's cs.mu.Lock()
// (full exclusivity, required for correctness, see that function's own
// comment) meaning a flush worker that actually keeps pace spends more
// aggregate time blocking the fast path within a short window than one
// that barely flushes at all in that same short window. Reverted at the
// time rather than ship a change chosen by a benchmark that rewards NOT
// reconciling promptly.
//
// Round 2 (TestSustainedWAL_QueueConvergence, actually decisive): the
// real problem with round 1 wasn't the fixes, it was the benchmark --
// 8-20s is too short for cs.walFlushQueue's own growth to show up as
// anything but "more lock contention." A genuinely sustained run (100
// disjoint warm pairs, 20 REAL seconds, queue depth sampled every 500ms)
// settles the question directly instead of inferring it from CPU time:
//   - original 500ms/500:  16,780 TPS, queue depth 66,433 -> 273,751
//     across the run (first third vs. last third average) -- CONFIRMED
//     unbounded growth under real sustained load, not just a theoretical
//     risk. Over a real deployment's uptime (hours/days, not 20s) this
//     is unrecoverable memory growth and ever-increasing Explorer/other-
//     validator staleness.
//   - 100ms/2,000:         16,633 TPS (statistically the same as
//     original -- no measurable throughput cost), queue depth 1,329 ->
//     1,740 -- STABLE, two orders of magnitude smaller than original's
//     final depth and not still climbing.
//   - 20ms/1,000:          10,583 TPS (real cost this time), queue depth
//     109 -> 116 -- also stable, tighter still, but not worth the
//     throughput loss given 100ms/2,000 already achieves stability at
//     zero measured cost.
//
// Applied: 100ms/2,000 gets the actual goal (bounded, keeping pace with
// real throughput) at no measured cost versus the original, unlike
// round 1's attempts which all traded real throughput for boundedness.
// This is why TestSustainedWAL_QueueConvergence exists as a permanent,
// re-runnable check in this repo -- rerun it (AEQUITAS_WAL_SUSTAINED_BENCH=1)
// after any future change near this code, since round 1 shows short
// benchmarks alone actively mislead for this specific trade-off.
//
// var, not const: lets TestSustainedWAL_QueueConvergence compare several
// interval/batch combinations without hand-editing this file and
// rebuilding for each one. Nothing outside a _test.go file in this
// package ever assigns to these.
var walFlushInterval = 100 * time.Millisecond

// walFlushMaxBatch bounds how many queued items one flush transaction
// writes — same rationale as transferBatchMaxSize/wal.MaxBatchSize: bounds
// how long a single flush's DB transaction runs, and now additionally
// caps how bad a Postgres-outage catch-up flush can get. See
// walFlushInterval's own comment (2026-07-23 investigation, both rounds)
// for the measurements behind this specific value.
//
// MEASURED (2026-07-24, Runde 6/7): raised 2,000 -> 4,000 after A/B testing
// against TestSustainedWAL_QueueConvergence at TWO address-space sizes,
// since this parameter's effect turned out to depend heavily on how much
// the queue's own addresses overlap:
//   - 1,000 pairs (2,000 addresses, closer to a real, spread-out user
//     base): 23,340 -> 30,868 TPS at walFlushConcurrency=4 -- a real,
//     substantial gain (+32%), fewer, larger flushes amortize the
//     Postgres round trip better without needing more concurrency.
//   - 100 pairs (200 addresses, this package's own default topology,
//     deliberately adversarial -- concentrated, continuous load on very
//     few accounts): 11,999 -> 10,321 TPS -- a real but modest cost
//     (-14%), because a 4,000-item batch drawn from only 200 addresses
//     dedupes to nearly the WHOLE address space, so this flush's
//     LockAddrs (transfer_wal.go's own flushWALBatch) ends up holding
//     nearly every shard the fast path could want, for longer, more
//     often. 3,000 measured statistically identical to 4,000 at this
//     small topology (10,301 vs 10,321) -- the cost is a step function
//     once a batch can cover most of a small address space, not a smooth
//     continuum, so there was no better compromise value to find between
//     2,000 and 4,000.
//
// Kept at 4,000, not reverted: real Contabo2 traffic today is far below
// EITHER synthetic topology (14 humans, sporadic -- not 100+ accounts
// continuously hammering each other), so the -14% cost has zero current
// real-world effect, while the +32% gain matters increasingly as usage
// grows toward the 50k-TPS target this whole investigation exists for.
// walFlushConcurrency (below) was NOT raised alongside this -- see its
// own comment for why higher concurrency specifically (not batch size)
// was the parameter that caused a much larger regression at the small
// topology (11,999 -> 8,558 at concurrency=16), and was reverted.
var walFlushMaxBatch = 4000

// walFlushMaxQueueDepth bounds cs.walFlushQueue's own size: once this many
// WAL-durable transfers are waiting for Postgres reconciliation, NEW
// transfers stop taking the WAL fast path (see transferConcurrentWAL's own
// eligibility check, right after the cs.wal nil-check) and fall through to
// the existing, fully synchronous batcher path instead -- slower per
// transfer, but with no unbounded-queue risk, exactly the same kind of
// fallback transferConcurrentWAL already takes for any other ineligibility
// reason (cold account, contended shard, pending demurrage, ...).
//
// FIX (2026-07-24, 50k-TPS sustained-load investigation, continued): once
// flushWALBatch itself stopped fully blocking the fast path during its own
// Postgres round trip (see that function's own 2026-07-24 FIX comment),
// measured fast-path input throughput rose substantially -- but the flush
// worker's OWN drain capacity (bound by how fast a single sequential
// Begin/Upsert/Insert/Commit round trip against Postgres can actually run)
// did not rise to match. Re-running TestSustainedWAL_QueueConvergence with
// the lock fix alone, no cap, showed EVERY tested config -- including
// 100ms/2,000, previously confirmed stable -- growing unbounded again,
// because blocking the whole fast path during every flush had
// (accidentally) been the ONLY thing throttling input to roughly drain
// capacity. Removing that block without replacing its backpressure effect
// reopens the exact unbounded-growth risk 100ms/2,000 was tuned to close.
// This cap replaces the lost backpressure with an explicit, principled
// one: bounded queue depth BY DESIGN regardless of how input/drain rates
// shift with load, hardware, or future changes near this code, instead of
// relying on an interval/batch-size tuning that Round 1 already showed is
// workload-sensitive and easy to get backwards (see walFlushInterval's own
// comment). 20,000 = 10x walFlushMaxBatch: generous enough to absorb a
// real burst without spilling to the batcher on ordinary jitter, small
// enough to bound worst-case Explorer/other-validator staleness to a few
// seconds at any measured drain rate in this investigation.
//
// A first attempt combining this cap with the lock fix, WITHOUT the
// deterministic row-lock order fix (see saveAccountsToDBBatchCtx's own FIX
// comment, state.go), surfaced a real Postgres deadlock between this
// flush's UPSERT and the batcher's own UPDATE under the resulting spike in
// batcher-fallback traffic -- throughput collapsed to 2,500-3,800 TPS with
// real transfer failures, worse than not having this cap at all. That is
// now fixed at the row-order level (both this file's flushWALBatch and
// state.go's saveAccountsToDBBatchCtx sort their touched addresses before
// writing), which is a precondition for this cap being safe to combine
// with the lock fix. Re-run TestSustainedWAL_QueueConvergence
// (AEQUITAS_WAL_SUSTAINED_BENCH=1) after any future change to flush
// timing, batch size, this cap, or the row-sort fix.
var walFlushMaxQueueDepth = 20000

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
	applyWALTuningFromEnv()
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
	// Phase clock. Recorded only when this path actually applies the transfer,
	// so a bail to the batcher does not dilute the averages with work it never
	// did. See transfer_phase_stats.go for what is unexplained and why.
	ph := beginTransferPhases()
	phMark := time.Now()
	// Backpressure: see walFlushMaxQueueDepth's own comment. A clean bail to
	// the batcher, same shape as any other ineligibility below -- nothing
	// has been mutated or appended to the WAL yet at this point.
	if cs.WALFlushQueueDepth() >= walFlushMaxQueueDepth {
		return 0, 0, false, nil
	}
	ph.queue = time.Since(phMark)
	if from == to {
		return 0, 0, true, fmt.Errorf("self-transfer not allowed")
	}
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, 0, true, fmt.Errorf("invalid transfer amount: %v", amount)
	}

	rlockMark := time.Now()
	cs.mu.RLock()
	ph.rlock = time.Since(rlockMark)
	defer cs.mu.RUnlock()

	// FIX (2026-08-22, measured): two shardedAccounts.Get calls used to sit
	// here, checking that both accounts exist before bothering with the lock.
	//
	// Get takes the shard mutex with a BLOCKING s.mu.Lock(). Those are the very
	// mutexes flushWALBatch holds, via LockAddrs, for its entire Postgres
	// transaction -- measured at a 37ms average hold across 371 addresses. So
	// every transfer waited on the flush right here, and the phase split showed
	// exactly that:
	//
	//	precheck    46.41ms   <- these two Gets
	//	wal_append  20.62ms
	//	lock         0.003ms  <- TryLockAddrs, by then uncontended
	//	apply        0.014ms
	//	enqueue      0.46ms
	//
	// 69% of a transfer, spent waiting for a lock the next line is specifically
	// designed NOT to wait for. TryLockAddrs exists to bail instantly to the
	// batcher on a contended shard rather than serialize behind a solo commit
	// (see its own comment for the 2x slowdown that motivated it) -- and these
	// two lines defeated it, because by the time the blocking Get returned the
	// shard was free and TryLockAddrs always succeeded. The 99.82% fast-path
	// rate was not evidence of low contention; it was evidence that transfers
	// waited instead of bailing.
	//
	// Deleting them changes no behaviour. GetLocked repeats both existence
	// checks under the lock a few lines below and returns the identical
	// (0, 0, false, nil) bail, so this was duplicated work whose only effect
	// was the wait.
	capMark := time.Now()
	capAmt, hasCapAmt := cs.wealthCapAmountLocked()
	ph.cap = time.Since(capMark)
	ph.precheck = time.Since(phMark)

	phMark = time.Now()
	unlock, ok := cs.accounts.TryLockAddrs(from, to)
	ph.lock = time.Since(phMark)
	if !ok {
		return 0, 0, false, nil
	}
	defer unlock()
	phMark = time.Now()

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

	// One instant, recorded in the WAL and used for the live stamp below, so a
	// crash-recovered replay reproduces exactly what this node did rather than
	// stamping its own restart time (see walTransferRecord.At).
	at := nowUnix()
	payload, err := json.Marshal(walTransferRecord{From: from, To: to, Amount: amount, TxHash: pendingTxTemplate.TxHash, At: at})
	if err != nil {
		return 0, 0, false, nil // encode failure -- nothing mutated, safe to fall back
	}
	ph.apply = time.Since(phMark)
	phMark = time.Now()
	seq, err := cs.wal.Append(payload)
	ph.append_ = time.Since(phMark)
	phMark = time.Now()
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
	touchActivityAt(fromAcc, at)
	cs.updateAccountLeafLocked(fromAcc)

	toAcc.Balance = toAcc.Balance.Add(NewDecimal(amount))
	toAcc.WALSeq = seq
	touchActivityAt(toAcc, at)
	cs.updateAccountLeafLocked(toAcc)

	pendingTxTemplate.Wallet = from
	pendingTxTemplate.To = to
	pendingTxTemplate.Amount = amount
	pendingTxTemplate.FromDemurrageLost = 0
	pendingTxTemplate.ToDemurrageLost = 0
	cs.enqueueWALFlushLocked(from, to, pendingTxTemplate)
	ph.enqueue = time.Since(phMark)
	ph.record()
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

// WALFlushQueueDepth returns how many WAL-durable transfers are currently
// waiting for async Postgres reconciliation. Diagnostic-only (a sustained-
// load test samples this over time to see whether it converges to a
// steady depth or grows without bound — see walFlushInterval's own
// 2026-07-23 investigation comment for why that distinction matters and
// why it isn't resolved yet); nothing in the normal transfer path reads
// this.
func (cs *ChainState) WALFlushQueueDepth() int {
	cs.walFlushMu.Lock()
	defer cs.walFlushMu.Unlock()
	return len(cs.walFlushQueue)
}

func (cs *ChainState) ensureWALFlushWorkerStarted() {
	cs.walFlushOnce.Do(func() {
		cs.walFlushStopCh = make(chan struct{})
		cs.walFlushSem = make(chan struct{}, walFlushConcurrency)
		SafeGoroutine("walFlushWorker", cs.runWALFlushWorker)
	})
}

// walFlushConcurrency bounds how many flushWALBatch calls can have their
// own Postgres transaction open at once.
//
// FIX (2026-07-24, 50k-TPS-goal investigation, continued): with a single
// sequential flush per tick (the original design), the flush worker's own
// drain rate was capped by how long ONE flushWALBatch round trip (Begin/
// Upsert/Insert/Commit against Postgres) takes, regardless of
// walFlushInterval -- a slow individual flush just made the effective
// cadence slower, since the next tick's work couldn't start until the
// current one returned (time.Ticker drops ticks it can't deliver, it
// doesn't queue them). Once flushWALBatch stopped needing cs.mu's full
// exclusivity (that function's own 2026-07-24 FIX comment) and instead
// only holds cs.accounts.LockAddrs for its own touched addresses, nothing
// about running SEVERAL flushes concurrently is unsafe any more: two
// concurrent flushes either touch disjoint addresses (shard locks never
// contend, both proceed fully in parallel) or overlap on some address
// (LockAddrs simply blocks the second until the first releases -- still
// correct, just serializes on the specific contended shard, and the
// wal_seq monotonic guard in the UPSERT tolerates whichever of the two
// commits second being a same-or-stale value safely, see flushWALQueue's
// own comment). This turns the drain rate from "one flush's worth per
// however long that flush takes" into "up to walFlushConcurrency flushes'
// worth in parallel" -- a real throughput multiplier under sustained load,
// not just a latency improvement.
//
// 4, matching parallelBatchPoolSize's own already-measured-safe value
// (transfer_batch_concurrent.go): same reasoning applies here -- this
// sandbox's CPU core count, and Postgres connection pool headroom
// (MaxOpenConns=20, already shared with the batcher's own up-to-4
// concurrent transactions plus other periodic flush workers) argue for a
// modest, not maximal, bound.
//
// MEASURED (2026-07-24): TestSustainedWAL_QueueConvergence's own default
// topology (100 hot pairs, 200 total addresses) showed no clear win from
// this alone -- concurrency=1 and concurrency=4 landed in the same noise
// band (roughly 14,000-17,000 TPS either way), because 200 addresses is
// small enough that concurrently-dispatched flushes frequently contend on
// the SAME shard locks, capping how much true parallelism is available.
// A fairer A/B, temporarily widening that same test to 1,000 pairs (2,000
// addresses -- still tiny next to a real deployment, but enough to show
// the effect) at the 100ms/2,000 config: concurrency=1 measured 20,989
// TPS at 69.1% fast-path share; concurrency=4 measured 22,800 TPS at
// 89.2% fast-path share -- a real, reproducible improvement in both
// throughput and how much of the workload the queue depth cap forces
// onto the slower batcher. Zero failed transfers in every configuration
// tested either way. Re-run TestSustainedWAL_QueueConvergence
// (AEQUITAS_WAL_SUSTAINED_BENCH=1) after changing this value.
//
// NOT raised further (2026-07-24, Runde 7): after walFlushMaxBatch was
// separately raised to 4,000 (see that var's own comment), tried pushing
// this alongside it -- 8/12/16/20, each combined with walFlushMaxBatch=
// 4,000, measured at the SAME 1,000-pair topology: 8 -> 27,880 TPS,
// 12 -> 31,555 TPS, 16 -> 34,560 TPS, 20 -> 33,983 TPS (flat/regressing
// past 16 -- MaxOpenConns=20 shared with the batcher's own up to 4
// concurrent transactions becomes the binding limit right around there).
// Genuinely more throughput at large scale. BUT re-tested at this
// package's own default 100-pair/200-address topology, EVERY one of
// those raised values measured WORSE than concurrency=4, not just
// unchanged: 8 -> 8,817 TPS, 16 -> 8,558 TPS, both clearly below
// concurrency=4's 11,999 TPS at the same 200 addresses -- unlike
// walFlushMaxBatch's -14% cost at small scale, raising concurrency alone
// cost -25% to -29% there, a materially worse trade for a real Contabo2
// deployment whose CURRENT traffic (14 humans) is closer to that small,
// concentrated topology than to a spread-out 2,000+-address one. Kept at
// 4 for exactly the reason walFlushMaxBatch's own comment gives for going
// the other way on that parameter: don't trade a real, present-day cost
// for a future gain when the real deployment hasn't grown into needing it
// yet. Revisit both together, re-measured, once Contabo2's actual active-
// address count has grown enough to make the large-topology numbers more
// representative than the small one.
var walFlushConcurrency = 4

func (cs *ChainState) runWALFlushWorker() {
	ticker := time.NewTicker(walFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			select {
			case cs.walFlushSem <- struct{}{}:
				cs.walFlushWG.Add(1)
				go func() {
					defer cs.walFlushWG.Done()
					defer func() { <-cs.walFlushSem }()
					cs.flushWALQueue()
				}()
			default:
				// Every concurrent flush slot is already busy -- skip this
				// tick rather than pile up an unbounded number of goroutines
				// each waiting for a semaphore slot. The queue just waits a
				// little longer; walFlushMaxQueueDepth (this file) is the
				// actual backstop against it growing without bound, not this
				// tick cadence.
			}
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
//
// FIX (2026-07-24, concurrent flush dispatch): also waits on cs.walFlushWG
// after closing the stop channel -- runWALFlushWorker's loop exits as soon
// as walFlushStopCh closes, but with flushes now dispatched concurrently
// (walFlushConcurrency, this file), up to that many flushWALBatch calls
// could still be mid-transaction at that exact moment. Without waiting,
// this function could return while one of those goroutines is still
// writing to the SHARED test Postgres DB, reopening exactly the cross-test
// corruption this function exists to prevent (see its own comment above).
func (cs *ChainState) stopWALFlushWorkerForTest() {
	cs.walFlushStopOnce.Do(func() {
		if cs.walFlushStopCh != nil {
			close(cs.walFlushStopCh)
		}
	})
	cs.walFlushWG.Wait()
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
//
// FIX (2026-07-24, CPU-profile investigation, continued): draining used to
// always just re-slice from n (`cs.walFlushQueue = cs.walFlushQueue[n:]`),
// which is O(1) but silently DISCARDS the first n slots of capacity every
// single time -- Go slice capacity after `s[n:]` is `cap(s) - n`, not
// `cap(s)`, so the array's own front portion becomes permanently
// unreachable through cs.walFlushQueue even though its memory is still
// held alive by whatever's left. Repeated over many drain cycles, this
// erodes the queue's real headroom faster and faster, forcing
// enqueueWALFlushLocked's own append to hit runtime.growslice (a full
// copy of the current live queue into a freshly, larger-allocated array)
// far more often than the queue's actual (now-bounded, thanks to
// walFlushMaxQueueDepth) logical size would require -- confirmed via CPU
// profile: enqueueWALFlushLocked accounted for over half of all
// runtime.growslice time under TestSustainedWAL_QueueConvergence's 100ms/
// 4,000 config, ~4% of total CPU in that run, essentially unchanged in
// kind from Round 1's original finding even though the queue itself no
// longer grows unbounded. Now compacts (copies the remaining tail into a
// FRESH array with real headroom) whenever the remaining capacity is
// getting tight, instead of unconditionally re-slicing every time --
// keeps the common case (plenty of headroom left) exactly as cheap as
// before, and replaces the eventual, ever-more-frequent growslice
// reallocation with a bounded, predictable compaction instead.
//
// Correctness note for the failure-retry path below (`append(batch,
// cs.walFlushQueue...)`): this remains safe regardless of whether the
// queue was compacted or not by the time a flush attempt fails. batch is
// a fixed view taken before this function returns and is never mutated
// by anything else; cs.walFlushQueue may or may not still alias batch's
// own backing array depending on what happened while flushWALBatch ran
// (concurrent enqueues, a previous compaction) -- Go's append semantics
// reconstruct the correct combined result either way (a harmless in-
// place self-copy if still aliased, an ordinary cross-array copy if not),
// so this fix changes capacity/efficiency, never the retry's correctness.
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
	// Then bound the batch by how much of the ADDRESS SPACE it will freeze.
	// walFlushMaxBatch caps items, which is not the thing that costs: measured
	// under load, 393 items locked 371 distinct addresses for 37ms, and that
	// hold is what every transfer collides with. Off by default -- see
	// wal_flush_addr_cap.go.
	n = limitBatchByAddrs(cs.walFlushQueue, n, walFlushMaxAddrs())
	batch := cs.walFlushQueue[:n]
	rest := cs.walFlushQueue[n:]
	if cap(rest) < walFlushMaxBatch {
		// Less than one full batch's worth of headroom left -- the next
		// enqueue cycle would very likely force runtime.growslice anyway,
		// so compact now while this mutex is already held. New capacity
		// sized for a couple more enqueue bursts before this triggers
		// again, not the full walFlushMaxQueueDepth -- most deployments
		// never need the queue anywhere near that deep, so always
		// reserving for the worst case here would waste memory in the
		// common case for no benefit.
		compacted := make([]walFlushItem, len(rest), len(rest)+2*walFlushMaxBatch)
		copy(compacted, rest)
		cs.walFlushQueue = compacted
	} else {
		cs.walFlushQueue = rest
	}
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
// Locking, two layers, each protecting against a DIFFERENT concurrent
// writer, BOTH held for the function's entire duration (snapshot AND DB
// write, released only via defer at the very end):
//
//  1. cs.mu.RLock(). This is what protects against the cs.mu.Lock()-based
//     SLOW path (transferLocked/batcher, e.g. a demurrage-settling
//     transfer touching the same account): the slow path's own
//     saveAccountToDBCtx write is guarded by the OPTIMISTIC-LOCK `version`
//     column, completely independent of this flush's `wal_seq` guard --
//     neither guard knows about the other's dimension, so if the slow
//     path's DB write landed BETWEEN this flush's snapshot read and its
//     own DB write, this flush's `wal_seq < $3` check would still pass
//     (wal_seq never changes on a slow-path write) and silently clobber
//     the slow path's newer balance with this flush's stale snapshot -- a
//     genuine lost update. RLock() is sufficient here (not just Lock())
//     because sync.RWMutex already blocks any Lock() acquisition for as
//     long as an RLock() is outstanding, regardless of which goroutine
//     holds it or how many others hold it concurrently -- the slow path
//     cannot proceed past its own cs.mu.Lock() call until this flush's
//     RLock() is released, identical protection to full Lock() for THIS
//     specific race.
//  2. cs.accounts.LockAddrs(...) (the same shard-lock primitive
//     transfer_concurrent.go/this file's own TryLockAddrs use) for every
//     address this batch touches, held for the WHOLE function, matching
//     processTransferBatchConcurrent's own documented discipline
//     (transfer_batch_concurrent.go, its "SAFETY ARGUMENT" comment) EXACTLY
//     -- this is not a stylistic choice, it's the one invariant every
//     Postgres-writing path in this codebase upholds so that any two
//     concurrent cs.mu.RLock()-based writers (this flush,
//     processTransferBatchConcurrent, transferConcurrent) are GUARANTEED
//     to have disjoint touched-row sets for their entire time in Postgres:
//     holding a row means holding its shard lock, shard locks are
//     mutually exclusive per shard, so two such writers can never both be
//     mid-transaction against the same row at once, which makes a
//     Postgres-level deadlock between them structurally impossible --
//     regardless of what row order Postgres's own query planner happens to
//     choose internally, which is NOT something application code can
//     reliably predict or control (see FIX below for what learning that
//     the hard way looked like).
//
// FIX (2026-07-24, 50k-TPS-goal sustained-load investigation): an earlier
// version of this function held cs.mu.Lock() (full exclusivity, blocking
// EVERY concurrent transfer, fast path included) for the whole operation.
// Replacing it with cs.mu.RLock() alone (still correct per layer 1 above)
// measured real throughput gains (TestSustainedWAL_QueueConvergence,
// 100ms/2,000: 16,633 -> 19,300 TPS) but reopened unbounded queue growth --
// the full Lock() had been an ACCIDENTAL backpressure mechanism (blocking
// the whole fast path during every flush implicitly throttled input to
// roughly drain capacity). walFlushMaxQueueDepth (this file) replaces that
// with an explicit cap instead.
//
// A second attempt released cs.accounts.LockAddrs right after the snapshot
// read, before the DB write -- reasoning (wrongly) that the DB write only
// touches a local, already-copied `snapshots` map, so nothing further
// needed protecting. This missed layer 2's REAL purpose entirely: it
// isn't about protecting the read, it's about making this function's own
// PostgreSQL transaction provably non-overlapping with every other
// concurrent writer's rows for as long as it's open. Releasing early
// reopened precisely the deadlock class transfer_batch_concurrent.go's own
// comment already documents from an EARLIER, separate attempt at this same
// idea (measured then at up to 23.6% failure at 500 concurrent senders,
// reverted) -- confirmed again here live: "pq: deadlock detected (40P01)"
// between this function's UPSERT and saveAccountsToDBBatchCtx's UPDATE
// under sustained load, hundreds of failed transfers per run. A
// deterministic SQL-level row sort (saveAccountsToDBBatchCtx's own FIX
// comment, state.go) measurably reduced but did NOT eliminate it --
// confirming the fix belongs at the Go lock-ordering level (provable,
// plan-independent), not by hoping a particular query shape happens to
// preserve input order through Postgres's optimizer. Holding LockAddrs for
// the WHOLE transaction, as implemented now, is what actually closes it:
// this function now upholds the exact same invariant
// processTransferBatchConcurrent already established as mandatory for any
// cs.mu.RLock()-based Postgres writer in this codebase. See
// SCALING_ARCHITECTURE.md's Runde-3/4 updates for the full measured
// history of both attempts.
//
// Separately, FIX (2026-07-23, TPS-benchmark investigation): this used to
// write one account UPSERT and one outbox INSERT per item via N separate
// round-trip Exec calls inside the single open tx -- for a full
// walFlushMaxBatch batch, measured at 150-230ms wall-clock per flush at
// the OLD 500-item batch size, ALL of it spent holding the lock(s)
// described above. Now issues exactly ONE multi-row UPSERT (via
// VALUES(...),(...),... + EXCLUDED, same technique saveAccountsToDBBatchCtx
// already uses) and ONE multi-row outbox INSERT per flush instead of one
// statement per item -- same guarantees (the wal_seq < EXCLUDED.wal_seq
// guard is evaluated per row, identically to the old per-statement WHERE
// clause), just far less time spent per flush regardless of lock scope.
func (cs *ChainState) flushWALBatch(batch []walFlushItem) error {
	if len(batch) == 0 {
		return nil
	}
	addrSet := make(map[string]struct{}, len(batch)*2)
	for _, item := range batch {
		addrSet[item.from] = struct{}{}
		addrSet[item.to] = struct{}{}
	}
	// Sorted, not just deduplicated: see saveAccountsToDBBatchCtx's own FIX
	// comment (state.go) -- this function's UPSERT and that function's
	// UPDATE both need a shared, deterministic row-touch order to avoid a
	// Postgres-level deadlock when they (or two of this file's own flushes,
	// were that ever possible) touch overlapping addresses concurrently.
	addrList := make([]string, 0, len(addrSet))
	for addr := range addrSet {
		addrList = append(addrList, addr)
	}
	sort.Strings(addrList)

	type walSnapshot struct {
		balance float64
		walSeq  uint64
	}

	cs.mu.RLock()
	defer cs.mu.RUnlock()

	snapshots := make(map[string]walSnapshot, len(addrList))
	// Held for this whole function via defer, NOT released after the
	// snapshot loop -- see this function's own doc comment (layer 2) for
	// why that's the actual fix, not an optional hardening.
	unlockAddrs := cs.accounts.LockAddrs(addrList...)
	// Measured, not estimated: a mutex profile put 45.21% of the node's entire
	// lock contention on this one hold, and the two numbers that explain it —
	// how much of the address space one flush freezes, and for how long — were
	// not recorded anywhere. Registered BEFORE the unlock defer so it runs
	// after it (defers are LIFO) and the interval covers the whole hold.
	lockAcquired := time.Now()
	var dbStart time.Time
	// Phase timers. Split because "27 of the 30ms is the database window" is
	// not yet an answer: that window also holds pure Go work (two multi-row
	// statements built by hand, one json.Marshal per item) which needs no lock
	// and could move out of the critical section entirely. Which fix is right
	// depends on which phase actually costs.
	var phSnapshot, phAcctSQL, phAcctExec, phOutboxSQL, phOutboxExec, phCommit time.Duration
	phMark := time.Now()
	defer func() {
		var dbDur time.Duration
		if !dbStart.IsZero() {
			dbDur = time.Since(dbStart)
		}
		noteWALFlush(len(batch), len(addrList), time.Since(lockAcquired), dbDur)
		noteWALFlushPhases(phSnapshot, phAcctSQL, phAcctExec, phOutboxSQL, phOutboxExec, phCommit)
	}()
	defer unlockAddrs()
	for _, addr := range addrList {
		acc, ok := cs.accounts.GetLocked(addr)
		if !ok {
			return fmt.Errorf("flushWALBatch: address %s vanished from cs.accounts between apply and flush -- this should never happen", addr)
		}
		snapshots[addr] = walSnapshot{balance: acc.Balance.Float(), walSeq: acc.WALSeq}
	}

	phSnapshot = time.Since(phMark)

	tx, err := cs.db.Begin()
	if err != nil {
		return fmt.Errorf("could not begin WAL flush transaction: %w", err)
	}
	dbStart = time.Now()
	phMark = dbStart
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
	for _, addr := range addrList {
		snap := snapshots[addr]
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
	phAcctSQL = time.Since(phMark)
	phMark = time.Now()
	if _, err := cs.dbExecCtx(ctx).Exec(acctQuery, acctArgs...); err != nil {
		tx.Rollback()
		return fmt.Errorf("could not upsert %d account(s) during WAL flush: %w", len(snapshots), err)
	}

	phAcctExec = time.Since(phMark)
	phMark = time.Now()

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
	phOutboxSQL = time.Since(phMark)
	phMark = time.Now()
	if _, err := cs.dbExecCtx(ctx).Exec(txQuery, txArgs...); err != nil {
		tx.Rollback()
		return fmt.Errorf("could not queue %d outbox tx(s) during WAL flush: %w", len(batch), err)
	}
	phOutboxExec = time.Since(phMark)
	phMark = time.Now()

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("WAL flush commit failed: %w", err)
	}
	phCommit = time.Since(phMark)
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
// walRecoveryFloorKey is the chain_config key holding the highest WAL
// sequence number that must NEVER be replayed again, because the account
// state it would apply to was replaced wholesale after that record was
// written — see markWALSupersededByStateReplacement.
const walRecoveryFloorKey = "wal_recovery_floor_seq"

// markWALSupersededByStateReplacement records the WAL's current head sequence
// as a permanent recovery floor. Call it from EVERY path that replaces
// chain_accounts wholesale (snapshot import, authoritative resync).
//
// FIX (P0, 2026-07-25 night — live state corruption on Contabo2, the one node
// running the WAL fast path): recoverFromWAL's idempotency test is per
// account, `acc.WALSeq >= record.Seq`, seeded from chain_accounts.wal_seq. A
// snapshot resync REPLACES chain_accounts — and AccountState.WALSeq is
// `json:"-"`, so the snapshot cannot carry it. Every account therefore came
// back with wal_seq = 0 while the WAL file (on a host volume, deliberately
// surviving container recreation) still held the full history. The next boot
// found nothing "already reflected" and re-applied ALL of it on top of the
// fresh, correct snapshot state. Confirmed live, verbatim from the boot log:
//
//	[WAL] Read 153429 record(s): 0 already reflected in Postgres (skipped,
//	      idempotent), 153429 reapplied to in-memory state
//
// Measured damage on that node: 294 of 315 accounts diverged from the other
// two validators, 74 of them driven NEGATIVE, while the total supply stayed
// exactly conserved (13408.384238 on every node) — the signature of transfers
// applied twice rather than of lost or invented funds. That divergence showed
// up as a permanent StateRoot mismatch (account_set_xor was the ONLY differing
// component) and kept the node from producing blocks at all.
//
// A per-account marker cannot survive the table being replaced, so the floor
// lives in chain_config, which the snapshot import does not clear. Records at
// or below it are superseded by definition: the snapshot is the newer, signed,
// authoritative truth for every account they touch.
//
// Deliberately runs even when the WAL is DISABLED on this node: the file
// outlives the flag, so a resync performed with the flag off must still fence
// off the history a later re-enable would otherwise replay.
func (cs *ChainState) markWALSupersededByStateReplacement() {
	if cs.db == nil {
		return
	}
	head := uint64(0)
	if cs.wal != nil {
		head = cs.wal.HeadSeq()
	} else {
		// WAL not open in this process (disabled, or a boot-time resync that
		// runs before initWALIfEnabled): read the head straight off the file,
		// so the floor still covers everything already written to it.
		path := os.Getenv("AEQUITAS_WAL_PATH")
		if path == "" {
			path = "aequitas_transfers.wal"
		}
		if _, err := os.Stat(path); err != nil {
			return // no WAL file at all — nothing to fence off
		}
		if _, _, err := wal.ReplayFile(path, func(entry wal.Entry) error {
			if entry.Seq > head {
				head = entry.Seq
			}
			return nil
		}); err != nil {
			fmt.Printf("[WAL] ⚠ could not read %s to record a recovery floor after state replacement: %v — a later WAL recovery may re-apply superseded records\n", path, err)
			return
		}
	}
	if head == 0 {
		return
	}
	if err := cs.setConfigValueDB(walRecoveryFloorKey, strconv.FormatUint(head, 10)); err != nil {
		fmt.Printf("[WAL] ⚠ could not persist recovery floor %d: %v\n", head, err)
		return
	}
	fmt.Printf("[WAL] ✓ Recovery floor set to seq %d — account state was just replaced wholesale, so every WAL record up to that point is superseded and will never be replayed again\n", head)
}

// walRecoveryFloor reads the floor recorded by
// markWALSupersededByStateReplacement (0 if never set).
func (cs *ChainState) walRecoveryFloor() uint64 {
	v := cs.getConfigValueDB(walRecoveryFloorKey)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func (cs *ChainState) recoverFromWAL(path string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Records at or below the floor were superseded by a wholesale state
	// replacement — see markWALSupersededByStateReplacement for the incident.
	floor := cs.walRecoveryFloor()

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

	// `at` is the recorded instant of the original transfer, NOT now — see
	// walTransferRecord.At. Stamping nowUnix() here credited every recovered
	// account with the entire outage as demurrage-free time.
	applyFrom := func(acc *AccountState, seq uint64, amount float64, at int64) bool {
		seedWALSeq(acc)
		if acc.WALSeq >= seq {
			return false
		}
		acc.Balance = acc.Balance.Sub(NewDecimal(amount))
		touchActivityAt(acc, at)
		acc.WALSeq = seq
		cs.updateAccountLeafLocked(acc)
		return true
	}
	applyTo := func(acc *AccountState, seq uint64, amount float64, at int64) bool {
		seedWALSeq(acc)
		if acc.WALSeq >= seq {
			return false
		}
		acc.Balance = acc.Balance.Add(NewDecimal(amount))
		touchActivityAt(acc, at)
		acc.WALSeq = seq
		cs.updateAccountLeafLocked(acc)
		return true
	}

	reappliedCount := 0
	supersededCount := 0
	readCount, truncated, err := wal.ReplayFile(path, func(entry wal.Entry) error {
		if entry.Seq <= floor {
			// Superseded by a wholesale state replacement — the per-account
			// WALSeq test below cannot recognize this, because the resync
			// replaced the very table it reads that marker from.
			supersededCount++
			return nil
		}
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
		fromApplied := applyFrom(fromAcc, entry.Seq, rec.Amount, rec.At)
		toApplied := applyTo(toAcc, entry.Seq, rec.Amount, rec.At)
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
		fmt.Printf("[WAL] Read %d record(s) from %s: %d superseded by a state replacement (below recovery floor %d), %d already reflected in Postgres (skipped, idempotent), %d reapplied to in-memory state and queued for reconciliation (tail-truncated=%v means the process crashed mid-append on the last record, which is expected and already handled)\n",
			readCount, path, supersededCount, floor, readCount-supersededCount-reappliedCount, reappliedCount, truncated)
	}
	return nil
}
