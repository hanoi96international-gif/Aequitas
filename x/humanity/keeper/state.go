package keeper

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/wal"
	"github.com/lib/pq"
)

// timeNowFunc is a seam for time.Now(), letting demurrage timing be
// mocked in tests without needing to thread a clock through every call.
var timeNowFunc = time.Now

// processStartTime records when this process started. Used by
// resetDBStateForBootstrap to refuse RESET_DB_STATE=true on accidental
// crash-recovery restarts that retain the env var.
var processStartTime = time.Now()

type AccountState struct {
	Address string  `json:"address"`
	Balance Decimal `json:"balance"`
	IsHuman bool    `json:"is_human"`
	// TUsdBalance is the account's holding of tUSD — a simulated, chain-native
	// test-dollar token used to exercise the swap/liquidity-pool mechanism
	// without touching any real external currency or bridge. See PoolState
	// below for the actual AEQ<->tUSD liquidity pool this balance interacts
	// with.
	TUsdBalance Decimal `json:"tusd_balance"`
	// LPShares is this account's claim on the liquidity pool, in the same
	// units as PoolState.TotalLPShares. An account's withdrawable amount at
	// any moment is (LPShares / TotalLPShares) * each reserve — see
	// RemoveLiquidity. This is the standard Uniswap v2 share-accounting
	// model: shares are minted on deposit and burned on withdrawal, so each
	// LP's claim automatically reflects fees/price-impact accumulated by the
	// pool since they joined, without needing per-LP bookkeeping of "their"
	// specific tokens.
	LPShares Decimal `json:"lp_shares"`
	// LastActivityAt is the Unix timestamp (seconds) of this account's most
	// recent AEQ-moving action (registration, sending/receiving a transfer,
	// swapping, or adding/removing liquidity). Demurrage (see ApplyDemurrage)
	// is calculated live from how long it's been since this timestamp — the
	// balance shown to the user is always computed fresh from Balance and
	// this timestamp, rather than being eaten away by a periodic background
	// job. Touching the account in any of those ways resets this timestamp,
	// which is the whole point: money that's actively circulating doesn't
	// decay, only money that's sitting idle does.
	LastActivityAt int64 `json:"last_activity_at"`
	// Demurrage14DayWarningShown tracks whether the one-time "your balance
	// starts decaying in 14 days" notice has already been surfaced for the
	// CURRENT grace period. Reset back to false by touchActivity whenever
	// the account's clock restarts (any AEQ-moving action), so the warning
	// can fire again for the next idle period rather than being a permanent
	// one-time-ever flag.
	Demurrage14DayWarningShown bool `json:"demurrage_14_day_warning_shown"`
	// FaucetClaimed is set permanently to true once an account has claimed the
	// tUSD test faucet. Unlike the old TUsdBalance>0 check, this flag is never
	// reset by spending tUSD, so a wallet cannot re-claim by draining its balance.
	FaucetClaimed bool  `json:"faucet_claimed"`
	Version       int64 `json:"-"` // optimistic lock version, not serialized
	// WALSeq is the highest WAL sequence number (see transfer_wal.go /
	// SCALING_ARCHITECTURE.md Phase 7) whose effect this account's Balance
	// currently reflects. Zero for every account unless AEQUITAS_WAL_ENABLED
	// is set — populated from chain_accounts.wal_seq only when an account is
	// touched by WAL replay/recovery, and advanced in-memory by
	// transferConcurrentWAL on every WAL-durable mutation. Its sole purpose
	// is making crash recovery idempotent: replaying a WAL record whose
	// effect is already reflected (WALSeq >= the record's Seq) must be a
	// no-op, not a double-application — see recoverFromWAL's own comment.
	// Never serialized to the JSON snapshot path (WAL/Postgres reconciliation
	// is DB-only, see initWALIfEnabled), so json:"-" here matches Version.
	WALSeq uint64 `json:"-"`
	// leafHash caches this account's current contribution to the incremental
	// state-root accumulator (see accountLeaf / ChainState.accountSetXOR). It
	// is the leaf that was last XORed INTO the accumulator for this account,
	// so a mutation can XOR the old leaf out before XORing the new one in
	// without an O(N) rescan. Derived state: never serialized, recomputed on
	// load. Zero value ([32]byte{}) means "no contribution counted yet"
	// (brand-new account, or an account excluded from the root because it is
	// non-human with all-zero balances).
	leafHash [32]byte `json:"-"`
}

// PoolState holds the two reserves of the single AEQ<->tUSD liquidity pool.
// Pricing follows the constant-product formula (reserveAEQ * reserveTUSD =
// k), the same model Uniswap v2 popularized: the more of one side someone
// swaps in, the worse the price gets for the next unit, which is what
// makes the pool self-balancing without needing an oracle or admin to set
// a price. A 0.1% fee is taken from every swap's input amount before the
// constant-product math runs, and is distributed across the four pools
// from the original tokenomics design (validators/LPs/UBI/treasury) —
// see DistributeSwapFee. Ordinary AEQ-to-AEQ transfers (state.Transfer)
// are NOT touched by this fee; it only applies to swaps through this pool.
type PoolState struct {
	ReserveAEQ  Decimal `json:"reserve_aeq"`
	ReserveTUSD Decimal `json:"reserve_tusd"`
	// TotalLPShares is the sum of every account's LPShares. Starts at 0; the
	// very first deposit mints sqrt(amountAEQ * amountTUSD) shares (the
	// standard Uniswap v2 formula — using the geometric mean means the
	// first depositor's chosen ratio doesn't let them mint an arbitrarily
	// large or small initial share count by gaming the two amounts).
	TotalLPShares Decimal `json:"total_lp_shares"`
}

type ChainState struct {
	mu sync.RWMutex
	// accounts is a *shardedAccounts (see sharded_accounts.go /
	// SCALING_ARCHITECTURE.md Phase 2) rather than a plain map. Every
	// access still happens under cs.mu, exactly like the map it replaced
	// -- this migration is behavior-preserving on its own; it only lays
	// the groundwork for a LATER phase where specific operations can use
	// shardedAccounts' own per-shard locks instead of cs.mu.
	accounts *shardedAccounts
	pool     *PoolState
	db       *sql.DB
	useDB    bool
	// ghostdagColumnsOnce guards the one-time chain_blocks GHOSTDAG-column
	// migration. It used to run on EVERY SaveBlockToDB / SaveGHOSTDAGState call —
	// three `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` statements each, i.e. six
	// per accepted peer block. Even as no-ops those are six extra DB round trips
	// AND six brief ACCESS EXCLUSIVE locks on chain_blocks, serialized against
	// every INSERT/SELECT on that table. Over the primary's cross-project public
	// DB proxy (~380ms/round-trip, confirmed live) that added ~2.3s of lock-held
	// latency to every peer block — the dominant cause (on top of the block save
	// itself) of the multi-second ProduceBlock cadence and the resulting failure
	// to merge with peers. The columns only ever need creating once per process.
	ghostdagColumnsOnce sync.Once
	// replayedColumnOnce guards the one-time chain_blocks.replayed migration
	// — same rationale as ghostdagColumnsOnce (avoid an ALTER TABLE on every
	// block save). See ensureReplayedColumn's own comment for what this
	// column is for.
	replayedColumnOnce sync.Once
	// txBatchTableOnce/txBatches back the body store that lets a block travel
	// without its transactions (roadmap step 4 — see tx_batch.go).
	txBatchTableOnce sync.Once
	txBatches        *txBatchCache
	// txBlockIndexOnce guards the tx -> including-block index, without which
	// eth_getTransactionByHash and eth_getTransactionReceipt answer with
	// placeholders and wallets mark landed transactions as failed — see
	// tx_block_index.go for the live report that uncovered it.
	txBlockIndexOnce sync.Once
	nullifiers       map[string]string // nullifier hex → wallet address (in-memory cache)
	// nullifiersMu guards cs.nullifiers (a plain, non-sharded Go map — unlike
	// cs.accounts, nullifiers were never migrated to a per-key-lockable
	// structure) and cs.nullifierSetXOR's mutation together, specifically for
	// SCALING_ARCHITECTURE.md Phase 8's concurrent-registration path
	// (register_concurrent.go): that path holds cs.mu.RLock(), not Lock(), so
	// multiple registrations can run genuinely in parallel — and Go's native
	// maps are not safe for concurrent access even to different keys, let
	// alone the same one. Every EXISTING cs.mu.Lock()-based caller
	// (tryClaimNullifierLocked, releaseNullifierLocked, snapshot import,
	// startup load) already has full exclusivity from cs.mu itself, so
	// acquiring this extra mutex is uncontended and free for them — it's the
	// ACTUAL synchronization only for IsNullifierUsed (already RLock-based
	// before this field existed) and SaveNullifier's callers under RLock.
	nullifiersMu sync.Mutex
	// accountSetXOR / nullifierSetXOR are incremental commitments to the FULL
	// account set and nullifier set, maintained by XORing each element's leaf
	// hash in on add and out on remove. They make StateRoot O(1) instead of
	// O(N): the old stateRootLocked iterated cs.accounts, which is capped at
	// maxInMemAccounts (5M) — so beyond that cap different nodes loaded
	// different subsets and computed DIFFERENT roots for the SAME chain,
	// silently breaking consensus at scale. These accumulators cover every
	// account/nullifier in the DB regardless of what is currently resident in
	// memory (see rebuildStateAccumulators, which seeds them from a full DB
	// scan at startup and after every bulk reset). Guarded by cs.mu like the
	// maps they summarize; snapshotted/restored across block rollback via
	// blockRollbackSnapshot.
	accountSetXOR   [32]byte
	nullifierSetXOR [32]byte
	// accountSetXORMu additionally guards accountSetXOR's own mutation
	// (updateAccountLeafLocked) specifically for SCALING_ARCHITECTURE.md
	// Phase 5's concurrent-transfer path (transferConcurrent, see
	// transfer_concurrent.go): that path holds cs.mu.RLock() plus a small
	// set of per-shard account locks, NOT cs.mu.Lock(), so multiple
	// transfers can run genuinely in parallel -- but accountSetXOR is one
	// single global field every one of them still mutates. Every existing
	// cs.mu.Lock()-based caller (replay, swap, distribution, snapshot,
	// the non-concurrent transfer path) already has full exclusivity from
	// cs.mu itself, so acquiring this extra mutex inside
	// updateAccountLeafLocked is uncontended and free for them -- it's
	// the ACTUAL synchronization only for the concurrent-transfer case,
	// where multiple RLock()-holding goroutines can otherwise race on
	// this field at the same instant.
	accountSetXORMu sync.Mutex
	// humanCount is TotalSupply's cheap source of truth (TotalSupply ==
	// humanCount * 1000, an already-documented invariant) — see
	// humanCountLocked's own comment for why it's maintained the exact same
	// way accountSetXOR is (full-DB-scan reseed on bulk reset, targeted
	// adjustment on the one live mutation path that can change it) rather
	// than as an independent counter incremented wherever convenient.
	//
	// humanCountMu guards humanCount's own mutation/read, for the identical
	// reason accountSetXORMu exists (see its own comment): SCALING_ARCHITECTURE.md
	// Phase 8's concurrent-registration path (registerHumanConcurrent, see
	// register_concurrent.go) increments this field under cs.mu.RLock(), not
	// Lock() -- and the EXISTING transferConcurrent/transferConcurrentWAL
	// fast paths already READ it (via wealthCapAmountLocked ->
	// getAverageBalanceLocked -> humanCountLocked) under their own
	// cs.mu.RLock() too. Every cs.mu.Lock()-based caller (the non-concurrent
	// registration path, rebuildStateAccumulators) already has full
	// exclusivity, so this extra mutex is uncontended and free for them.
	humanCountMu sync.Mutex
	humanCount   int64
	// activeTx, when non-nil, is the transaction every DB write inside the
	// CURRENT cs.mu-locked operation must use instead of cs.db directly —
	// see dbExec() and runAtomicWithOutbox. Only ever set/cleared while
	// cs.mu is held (write-locked), so reading it without separate
	// synchronization is safe: at most one goroutine can be inside a
	// cs.mu-locked region at a time, and that goroutine is the only one
	// that could have set it.
	activeTx *sql.Tx
	// activeTxOwnerGID pins activeTx to the goroutine that opened it. The
	// comment above ("only one goroutine can be inside a cs.mu-locked region")
	// stopped being the whole story once the 50k-TPS work added code paths
	// that write WITHOUT holding cs.mu (sharded accounts, concurrent
	// transfer/registration, background flushers): any of those calling a
	// not-yet-migrated helper (plain dbExec(), ctx without a tx) while a
	// replay/atomic op has activeTx set would silently join THAT goroutine's
	// *sql.Tx — two goroutines on one Postgres connection, which desyncs the
	// wire protocol (`pq: unexpected Parse response "(D) DataRow"` / `driver:
	// bad connection`, confirmed live on Contabo2 twice on 2026-07-25, each
	// time poisoning a consensus block). dbExecCtx now only returns activeTx
	// to its owner goroutine; every other goroutine falls back to the cs.db
	// pool (its own connection, no corruption) and logs loudly so the
	// offending call path can be found and migrated. Set/cleared exclusively
	// via setActiveTx.
	activeTxOwnerGID atomic.Int64
	// activeTxMisuseLogAt rate-limits the cross-goroutine warning above.
	activeTxMisuseLogAt atomic.Int64

	// transferBatchCh/transferBatchOnce back TransferAtomic's group-commit
	// path (see runTransferBatcher's own comment) — coalesces concurrent
	// TransferAtomic callers into shared DB transactions so N transfers pay
	// roughly one fsync-commit instead of N, without changing cs.mu's
	// single-writer serialization model or touching any other atomic
	// operation (swap/liquidity/registration/distribution keep using
	// runAtomicWithOutbox exactly as before, one call at a time).
	transferBatchCh chan *transferBatchRequest
	// Batched nonce reservation; see nonce_batch.go for why the single-row
	// version became the largest single item in the CPU profile.
	nonceBatchOnce    sync.Once
	nonceBatchCh      chan *nonceReserveRequest
	transferBatchOnce sync.Once

	// parallelBatchSem bounds how many transferBatchCh-collected batches can
	// have their own DB transaction open at once via
	// processTransferBatchConcurrent (transfer_batch_concurrent.go) — see
	// that function's own comment for the shard-locked parallel-batch
	// mechanism this backs. Created alongside transferBatchCh, in the same
	// transferBatchOnce.Do block.
	parallelBatchSem chan struct{}

	// evmMirrorQueueMaybeNonEmpty is a cheap, deliberately-imprecise (never
	// falsely "empty", may lag "non-empty" true a little) signal for
	// syncBalanceLocked: skip the per-transfer evm_mirror_sync_queue DELETE
	// round trip entirely when nothing is queued for retry (the
	// overwhelming common case — that table only ever gets a row when an
	// EVM mirror write actually FAILED). QueueEVMMirrorSync sets this true;
	// RetryEVMMirrorSyncQueue's own periodic pass sets it false once it
	// observes the table is empty. THROUGHPUT (2026-07-22): profiling
	// TestSimulateMaxTPS_Ingestion showed syncBalanceLocked as >50% of
	// per-transfer time even after batching its writes — this was the
	// single largest remaining lever found. Zero value (false) is safe on
	// a fresh node (queue genuinely empty); on restart with pre-existing
	// queued entries from before the crash, those are still found and
	// cleared by RetryEVMMirrorSyncQueue's own unconditional table query
	// regardless of this flag — it just means syncBalanceLocked's
	// per-transfer skip doesn't kick in for those specific leftover
	// addresses until the first periodic pass or a fresh failure sets this
	// true. No correctness impact either way: this flag only gates a
	// best-effort cleanup DELETE, never the actual balance ledger.
	evmMirrorQueueMaybeNonEmpty atomic.Bool

	// evmMirrorDirty/evmMirrorDirtyMu/evmMirrorFlushOnce back syncBalanceLocked's
	// deferred write (see evm_mirror_flush.go / SCALING_ARCHITECTURE.md
	// Phase 6) -- guarded by their OWN small mutex, not cs.mu, since marking
	// an address dirty must be cheap enough to do inline on every transfer
	// without adding any new contention on the lock every other operation
	// already needs.
	evmMirrorDirtyMu   sync.Mutex
	evmMirrorDirty     map[evmMirrorDirtyKey]struct{}
	evmMirrorFlushOnce sync.Once

	// receiptBuf/receiptBufMu/receiptFlushOnce back SaveTxReceipt's deferred
	// write -- same dirty-buffer-plus-periodic-worker shape as evmMirrorDirty
	// above, and for the same reason: it used to be a synchronous INSERT per
	// transaction on the RPC request path, which a live CPU profile showed as
	// the last remaining per-transfer Postgres round trip. Keyed by tx hash so
	// the buffer reproduces the ON CONFLICT DO UPDATE semantics of the single
	// -row statement it replaces: a later write for the same hash (e.g. a
	// success receipt superseded by a failure one) simply overwrites the
	// buffered entry, exactly as the database would have.
	receiptBufMu     sync.Mutex
	receiptBuf       map[string]pendingReceipt
	receiptFlushOnce sync.Once

	// poolFlushDirty/poolFlushOnce back distributeSwapFee's deferred pool
	// persistence (see pool_flush.go / SCALING_ARCHITECTURE.md Phase 3):
	// with a real DB, a pool-address credit updates cs.accounts and
	// cs.accountSetXOR immediately (in-memory only, cheap) but skips the
	// synchronous Postgres round trip that used to happen on every single
	// transfer's demurrage settlement — poolFlushDirty just marks "the 4
	// pool rows are stale in the DB", and a periodic background worker
	// (started lazily, like transferBatchOnce) batches however many
	// credits accumulated into ONE write per flush interval instead of
	// one per transfer.
	poolFlushDirty atomic.Bool
	poolFlushOnce  sync.Once

	// degradedMu guards bootstrapDegradedReason. Set by main.go when
	// snapshot bootstrap/resync's EVM mirror migration step fails (see
	// ImportSnapshotFromURL/ResyncFromSnapshotURL) — Go-state itself is
	// fine (the source of truth), but eth_call/V7 contract storage reads
	// may be stale until the next successful migration. Surfaced via
	// /api/health/combined so this doesn't silently sit unnoticed (audit
	// 2026-06-28 recheck 5, P1-3: "Startup/Bootstrap sollte ... mindestens
	// einen Health-Status degraded setzen").
	degradedMu              sync.RWMutex
	bootstrapDegradedReason string

	// accountsLoadFailed is set true if loadFromDB's SELECT against
	// chain_accounts failed (even after retry) at startup. See loadFromDB's
	// own comment for the production incident this guards against:
	// main.go's "is this a fresh node?" check used to be TotalHumans()==0,
	// which is indistinguishable from "the query that would have told us
	// otherwise just failed" — a node with real history that hit a
	// transient DB hiccup at exactly the wrong moment during startup looked
	// identical to a genuinely brand-new node, and got bootstrap-imported
	// (or worse, authoritatively resynced) from a peer snapshot as if it
	// had no history at all.
	accountsLoadFailed bool

	// finalizedMu guards the in-memory finalized-checkpoint cache (P0,
	// cadence audit 2026-07-03) — see GetFinalizedCheckpoint's comment.
	// This is the only place finalized_height/finalized_blue_score/
	// finalized_hash are read from or written to; every writer
	// (SetFinalizedCheckpoint, ResetFinalizedCheckpoint) keeps it in sync,
	// so it is always current for the lifetime of this process.
	finalizedMu             sync.RWMutex
	finalizedCacheLoaded    bool
	finalizedHeightCache    int64
	finalizedBlueScoreCache int64
	finalizedHashCache      string

	// penaltyMu guards the in-memory validator_penalties cache (P0, cadence
	// 2026-07-03 night) — see IsValidatorSuspended. Same design as the
	// finalized-checkpoint cache above: the table is only ever written by
	// THIS process (RecordEquivocationAndSuspend, initSlashingTables'
	// activation cleanup), both writers keep the cache in sync, so a
	// load-once cache is always current for the process lifetime. Before
	// this cache, IsValidatorSuspended was a synchronous Postgres round trip
	// executed under dag.mu for every non-FromSync peer block — the primary
	// bottleneck behind the 3-5s cadence (see AddPeerBlock's GATE ORDER
	// comment).
	penaltyMu          sync.RWMutex
	penaltyCacheLoaded bool
	penaltyCache       map[string]validatorPenalty

	// wal is the optional local write-ahead log backing transferConcurrentWAL
	// (see transfer_wal.go / SCALING_ARCHITECTURE.md Phase 7) — nil unless
	// AEQUITAS_WAL_ENABLED=1 was set at startup. When nil, TransferAtomic
	// behaves exactly as it did before this field existed (transferConcurrent,
	// the Postgres-durable fast path, or the batcher). When set, a WAL append
	// — not a synchronous Postgres commit — becomes the durability point for
	// eligible transfers. See transferConcurrentWAL's own doc comment for the
	// full design and its explicit NOT-staging-validated status: this changes
	// real durability semantics, unlike every other change in this session.
	wal *wal.WAL

	// walFlushMu/walFlushQueue/walFlushOnce back the async Postgres
	// reconciliation for WAL-durable transfers (transfer_wal.go) — same
	// dirty-queue-plus-periodic-worker shape as evmMirrorDirty/poolFlushDirty
	// above, guarded by their own mutex for the same reason (cheap enough to
	// touch inline on every WAL-durable transfer without contending cs.mu).
	walFlushMu       sync.Mutex
	walFlushQueue    []walFlushItem
	walFlushOnce     sync.Once
	walFlushStopCh   chan struct{} // see stopWALFlushWorkerForTest's own comment
	walFlushStopOnce sync.Once     // makes stopWALFlushWorkerForTest safe to call more than once
	// walFlushSem/walFlushWG back concurrent flush dispatch — see
	// runWALFlushWorker's own FIX comment (transfer_wal.go, 2026-07-24) for
	// why a single sequential flush-per-tick became the binding throughput
	// ceiling once flushWALBatch stopped needing cs.mu's full exclusivity.
	walFlushSem chan struct{}
	walFlushWG  sync.WaitGroup
}

// validatorPenalty is one cached validator_penalties row — everything
// IsValidatorSuspended needs to answer without touching the DB.
type validatorPenalty struct {
	banned         bool
	suspendedUntil int64
	lastOffenseAt  int64
}

// AccountsLoadFailed reports whether loadFromDB's startup query against
// chain_accounts failed (even after retry) — see its own comment and the
// accountsLoadFailed field's comment for why main.go must check this before
// treating TotalHumans()==0 as "this is a genuinely fresh node".
func (cs *ChainState) AccountsLoadFailed() bool {
	return cs.accountsLoadFailed
}

// SetBootstrapDegraded records why this node's EVM mirror may be stale
// after a snapshot bootstrap/resync. Pass "" to clear it once a later
// retry succeeds.
func (cs *ChainState) SetBootstrapDegraded(reason string) {
	cs.degradedMu.Lock()
	cs.bootstrapDegradedReason = reason
	cs.degradedMu.Unlock()
}

// BootstrapDegradedReason returns the last recorded bootstrap-degraded
// reason, or "" if none.
func (cs *ChainState) BootstrapDegradedReason() string {
	cs.degradedMu.RLock()
	defer cs.degradedMu.RUnlock()
	return cs.bootstrapDegradedReason
}

// sqlExecutor is satisfied by both *sql.DB and *sql.Tx (identical method
// sets for the subset used here) — lets every existing call site that
// writes via cs.dbExec() transparently participate in an active
// transaction without its own signature needing to change.
type sqlExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

// dbExec returns the executor in-progress writes should use: the active
// transaction if runAtomicWithOutbox started one for the current
// operation, otherwise cs.db directly (today's existing behavior,
// unchanged for every caller that isn't part of an atomic operation).
// Callers must still guard on cs.useDB/cs.db==nil exactly as before this
// existed — this does not change the no-DB-mode contract at all.
//
// Thin wrapper around dbExecCtx with an empty context — see that function's
// comment for why this exists and what it's the first step of. Every
// existing call site keeps compiling and behaving identically; nothing
// about this signature changes what dbExec() already did.
func (cs *ChainState) dbExec() sqlExecutor {
	return cs.dbExecCtx(context.Background())
}

// txKey is the context.Context key for the active *sql.Tx of the current
// atomic operation. Unexported, zero-size, unique struct type (not a plain
// string) so it can never collide with a key some other package might
// store in the same context — the standard Go idiom for context keys.
type txKey struct{}

// withTx returns a context carrying tx as the active transaction dbExecCtx
// should use — see dbExecCtx's own comment for the migration this is part
// of.
func withTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// txFromContext returns ctx's active transaction, or nil if none was set
// (context.Background(), or any context nothing ever called withTx on).
func txFromContext(ctx context.Context) *sql.Tx {
	tx, _ := ctx.Value(txKey{}).(*sql.Tx)
	return tx
}

// dbExecCtx is dbExec's context-aware replacement — see
// SCALING_ARCHITECTURE.md's Phase 5/7 "Update" section for the problem
// this exists to eventually solve: cs.activeTx is a single, ChainState-wide
// field every atomic operation currently shares, which makes genuine
// concurrent execution of multiple such operations impossible (a second
// operation's Begin() would silently overwrite the field a first,
// still-in-flight operation is relying on). Replacing every implicit
// cs.activeTx read with an explicit, per-operation ctx value is the
// prerequisite for that — but doing so all at once, across every atomic
// subsystem (transfer, swap, liquidity, registration, distribution,
// guardian, slashing, snapshot import/resync, AND block replay — the single
// most consensus-critical, historically bug-prone code in this repo) in one
// pass would be exactly the kind of sweeping, hard-to-review rewrite this
// project's own "Anti-Pattern" and "Historischer Kontext" sections warn
// against.
//
// So this migrates gradually, one call chain at a time, EACH one verified
// by the full test suite before moving to the next: a shared low-level
// function (ensureAccountLoaded, saveAccountToDB, etc.) gets a new
// `*Ctx`-suffixed sibling that takes ctx explicitly and is the real
// implementation; the original name becomes a thin `context.Background()`
// wrapper around it, so every NOT-yet-migrated caller anywhere in the
// codebase keeps compiling and behaving identically, untouched. Until
// runAtomicWithOutbox itself is migrated to stop setting cs.activeTx and
// instead pass a real per-operation ctx through fn(), dbExecCtx falls back
// to cs.activeTx exactly like dbExec always did — so at every intermediate
// point in this migration, behavior is provably unchanged; only once EVERY
// call site is migrated (a separate, much larger future step, tracked as
// its own task) does removing cs.activeTx and enabling real concurrent
// execution become possible.
func (cs *ChainState) dbExecCtx(ctx context.Context) sqlExecutor {
	if tx := txFromContext(ctx); tx != nil {
		return tx
	}
	// Guard (P0, 2026-07-25 night — see activeTxOwnerGID's field comment):
	// the activeTx fallback is only ever correct for the goroutine that
	// opened the transaction. Any OTHER goroutine landing here used to be
	// silently handed a *sql.Tx someone else is actively using — two
	// goroutines interleaving on one Postgres connection, corrupting the
	// wire protocol and (confirmed live, twice in one evening) getting a
	// consensus block rejected on a poisoned connection. Hand foreign
	// goroutines the pool instead: their write commits standalone — exactly
	// what it would have done before anyone happened to have a transaction
	// open — and the loud, stack-carrying log below identifies the call
	// path that still needs migrating to an explicit ctx/tx.
	if cs.activeTx != nil {
		gid := curGoroutineID()
		if owner := cs.activeTxOwnerGID.Load(); owner == 0 || owner == gid {
			return cs.activeTx
		}
		nowNano := time.Now().UnixNano()
		last := cs.activeTxMisuseLogAt.Load()
		if nowNano-last > int64(5*time.Second) && cs.activeTxMisuseLogAt.CompareAndSwap(last, nowNano) {
			fmt.Printf("[DB-GUARD] ✗ dbExec from goroutine %d while goroutine %d holds the active transaction — routing this write to the pool instead, to prevent Postgres wire-protocol corruption. Migrate this call path to an explicit ctx/tx. (rate-limited) Stack:\n%s\n",
				gid, cs.activeTxOwnerGID.Load(), debug.Stack())
		}
	}
	return cs.db
}

// setActiveTx is the ONLY way activeTx may be set or cleared — it records
// the owning goroutine alongside the transaction so dbExecCtx can refuse to
// hand the tx to any other goroutine (see activeTxOwnerGID's field comment).
func (cs *ChainState) setActiveTx(tx *sql.Tx) {
	if tx != nil {
		cs.activeTxOwnerGID.Store(curGoroutineID())
	} else {
		cs.activeTxOwnerGID.Store(0)
	}
	cs.activeTx = tx // the one exempt assignment — every other site must call setActiveTx
}

// curGoroutineID parses this goroutine's id from the runtime stack header
// ("goroutine N [running]:"). Only called on paths that already hold or are
// about to open a DB transaction, so the ~µs stack peek is negligible next
// to the Postgres round trip it protects.
func curGoroutineID() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	fields := strings.Fields(string(buf[:n]))
	if len(fields) >= 2 {
		if id, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
			return id
		}
	}
	return -1 // unparseable (should never happen) — never matches a real owner
}

// activeTxCtx is dbExecCtx's counterpart for the handful of callers (e.g.
// savePoolToDB) that need the raw *sql.Tx itself, not just something
// satisfying sqlExecutor — e.g. to call Commit/Rollback, or to decide
// whether to start a fresh transaction at all. Same ctx-then-cs.activeTx
// fallback order as dbExecCtx, for the same reason.
func (cs *ChainState) activeTxCtx(ctx context.Context) *sql.Tx {
	if tx := txFromContext(ctx); tx != nil {
		return tx
	}
	return cs.activeTx
}

// DB returns the underlying *sql.DB for admin operations that need raw queries.
// Most code should use dbExec() instead so atomic transactions are respected.
func (cs *ChainState) DB() *sql.DB { return cs.db }

// P3-FIX: stateMu, acquireStateLock, and releaseStateLock were dead code
// (never called). Removed to eliminate a future deadlock trap where a
// developer might call acquireStateLock inside a function already holding cs.mu.

// beginStateTx starts a SERIALIZABLE PostgreSQL transaction for critical
// state mutations. The caller is responsible for calling Commit or Rollback.
// Returns nil if no DB is configured (in-memory/file mode).
func (cs *ChainState) beginStateTx() *sql.Tx {
	if cs.db == nil {
		return nil
	}
	// P0-7: SERIALIZABLE prevents phantom reads that could violate
	// totalSupply = humans * 1000 invariant under concurrent writes.
	tx, err := cs.db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		fmt.Printf("[DB] Warning: could not begin state tx: %v\n", err)
		return nil
	}
	return tx
}

// P3-9: validate pool addresses at startup to catch typos early
func validatePoolAddresses() {
	for _, addr := range []string{validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr} {
		if len(addr) != 42 || addr[:2] != "0x" {
			panic("invalid pool address: " + addr)
		}
	}
}

func NewChainState(dataFile string) *ChainState {
	validatePoolAddresses()
	cs := &ChainState{
		txBatches:  newTxBatchCache(),
		accounts:   newShardedAccounts(),
		nullifiers: make(map[string]string),
	}

	// Try PostgreSQL first
	if os.Getenv("RESET_STATE") == "true" && os.Getenv("DATABASE_URL") != "" {
		fmt.Println("⚠ RESET_STATE=true is set but DATABASE_URL is active — DB is NOT wiped by this flag.")
		fmt.Println("  To reset a DB-backed node, run DELETE queries directly in the PostgreSQL console.")
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL != "" {
		// Add sslmode if not present
		if !strings.Contains(dbURL, "sslmode") {
			if strings.Contains(dbURL, "?") {
				dbURL += "&sslmode=disable"
			} else {
				dbURL += "?sslmode=disable"
			}
		}
		// FIX (2026-06-30, confirmed live in production): this connection had
		// NO timeout of any kind — no connect_timeout, no statement_timeout, no
		// pool limits — and every call site uses db.Exec/db.QueryRow (not the
		// *Context variants), so nothing in this codebase could ever cancel a
		// stuck query. A single transient network hiccup against the remote
		// Postgres (acela.proxy.rlwy.net — a Railway-managed proxy, not
		// localhost) could hang a query indefinitely, and since DB writes like
		// SaveGHOSTDAGState/SaveBlockToDB run WHILE dag.mu is held, that one
		// stuck connection silently froze the entire node: ProduceBlock,
		// AddPeerBlock, and every dag.mu-dependent API endpoint (status,
		// blocks, health) all queued behind it with zero log output and no
		// timeout to ever break the wait. Confirmed live: primary went
		// unresponsive on /api/status etc. for extended periods while static,
		// non-dag.mu pages kept responding normally — consistent with a
		// single stuck goroutine holding dag.mu, not a deadlock or crash.
		// statement_timeout is a Postgres-side GUC (recognized by lib/pq as a
		// connection string parameter): the SERVER itself cancels any query
		// running longer than this and returns an error, which is the only
		// way to bound an already-in-flight query — a client-side context
		// timeout can abandon the wait locally but can't stop network-level
		// blocking the same way. connect_timeout bounds the initial TCP/auth
		// handshake the same way for a connection that hasn't been
		// established yet.
		// FIX (audit 2026-06-30 monster audit, P2-01): used to gate BOTH
		// params on a single `strings.Contains(dbURL, "statement_timeout")`
		// check — a DSN that already specified statement_timeout (but not
		// connect_timeout) skipped appending connect_timeout entirely, and a
		// DSN with connect_timeout but not statement_timeout got both
		// appended, duplicating connect_timeout as a query parameter. Checked
		// independently now so each is added iff it's actually missing.
		appendParam := func(url, param, value string) string {
			if strings.Contains(url, param) {
				return url
			}
			sep := "&"
			if !strings.Contains(url, "?") {
				sep = "?"
			}
			return url + sep + param + "=" + value
		}
		dbURL = appendParam(dbURL, "statement_timeout", "30000")
		dbURL = appendParam(dbURL, "connect_timeout", "10")
		db, err := sql.Open("postgres", dbURL)
		if err == nil {
			// Bound the pool itself: an unlimited pool under a burst of
			// concurrent saves (e.g. catching up on a flood of peer blocks
			// after being offline) can pile up far more open connections than
			// the remote Postgres's own max_connections allows, and stale
			// connections were never recycled (ConnMaxLifetime defaults to
			// "forever"). These limits are generous, not tight — the goal is
			// "bounded", not "minimal".
			//
			// FIX (P0, cadence 2026-07-03 night): MaxIdleConns must equal
			// MaxOpenConns. At MaxIdle=5, any burst that used >5 connections
			// (concurrent block save + LoadPendingTxs/StateRoot pair + API
			// reads + sync loops) CLOSED the extras on release, and the next
			// burst re-dialed them. Over the primary's remote DB proxy a
			// fresh connection costs TCP+TLS+auth ≈ 5-6 round trips ≈ 1.5s —
			// confirmed live by LoadPendingTxs stalls of 1.72-1.74s (setup +
			// one 0.26s query) in ProduceBlock's phase breakdown, directly
			// inflating block cadence. Idle connections to our own dedicated
			// Postgres are effectively free (max_connections ~100); paying
			// 1.5s handshakes on the block-production hot path is not.
			// ConnMaxLifetime 5min→30min for the same reason: it force-closed
			// every connection 12x/hour, re-paying the handshake each time —
			// 30min still recycles through proxy restarts, 6x cheaper.
			//
			// THROUGHPUT (2026-07-23, TPS-benchmark investigation): tried
			// raising this from 20 to 40, then to 80, chasing
			// transferConcurrent's (transfer_concurrent.go) and
			// processTransferBatchConcurrent's (transfer_batch_concurrent.go)
			// own connection demand — both hold a pool connection for their
			// whole own DB transaction and, unlike the old single-goroutine
			// batcher, run genuinely in parallel across many goroutines. At
			// 20, an isolated disjoint-recipient TPS benchmark run barely
			// moved between 100 and 2000 concurrent senders, a flat ceiling
			// tracking the connection pool rather than useful work.
			//
			// Both 40 and 80 were reverted after the SAME failure mode
			// under the full `go test ./x/humanity/keeper/...` suite: "pq:
			// sorry, too many clients already" (53300). This package's own
			// test suite creates many separate ChainState/*sql.DB pool
			// instances across different test functions, several of which
			// leave background activity running past their own test's
			// return (a WAL crash-recovery test that deliberately abandons
			// a ChainState mid-test; the parallel batch dispatch goroutines
			// this file's own runTransferBatcher spawns) -- overlapping
			// instances each independently allowed up to 40 (or 80)
			// connections blew past Postgres's own max_connections=100 in
			// aggregate, something 20 each never did even under repeated
			// (5+) full-suite -race runs. This is a genuine signal about
			// shared-Postgres reality generally (migration tools,
			// monitoring, a second node process during a restart), not
			// just a test-suite artifact -- kept at the original,
			// conservative 20 rather than trading verified stability for
			// throughput that this sandbox's 4 CPU cores mostly couldn't
			// use past this point anyway (see processTransferBatchConcurrent's
			// own comment for where the real throughput gain from this
			// session's investigation ended up coming from instead).
			//
			// MEASURED 2026-07-27: that reasoning is sound for the test suite
			// and wrong for a production node. A CPU profile taken under load
			// on Contabo2 showed the node running at 198% of 600% available
			// CPU — four of six cores idle — with 20.8% of samples in
			// syscalls and ReserveNonce alone accounting for 23.9%
			// cumulative. The load generator drives 72 concurrent senders,
			// each making a synchronous round trip per transaction, against
			// this pool of 20. They queue for a connection, and the node is
			// latency-bound on the database rather than short of CPU.
			//
			// The two situations the comment above conflates are different.
			// The test suite runs MANY ChainState instances in one process
			// against one shared Postgres, so a per-instance cap of 20 is
			// what keeps their aggregate under max_connections. A production
			// node is ONE instance with its own Postgres container, where 20
			// is simply a throttle nothing asked for.
			//
			// So the default stays exactly 20 — tests, and any deployment
			// that sets nothing, behave precisely as before — and production
			// can raise it per box, where the operator knows that node's
			// max_connections. Same shape as every other per-box decision
			// here (BLOCK_TIME, AEQUITAS_WAL_ENABLED, the RPC rate limit).
			maxConns := intFromEnv("AEQUITAS_DB_MAX_CONNS", 20)
			db.SetMaxOpenConns(maxConns)
			// Idle tracks open: a connection returned to the pool and then
			// closed because idle capacity is smaller would have to be
			// re-established on the next request, which is the very round
			// trip this is trying to avoid.
			db.SetMaxIdleConns(maxConns)
			db.SetConnMaxLifetime(30 * time.Minute)
			err = db.Ping()
			if err == nil {
				cs.db = db
				cs.useDB = true
				cs.initDB()
				if os.Getenv("RESET_DB_STATE") == "true" {
					cs.resetDBStateForBootstrap()
				}
				if os.Getenv("CLEAR_REGISTRATIONS") == "true" {
					cs.clearRegistrationsFromDB()
				}
				cs.loadFromDB()
				fmt.Println("✓ ChainState using PostgreSQL")
				cs.initWALIfEnabled()
				// chain_tx_batches has no DELETE anywhere and grows with every
				// produced block — see tx_batch_prune.go. Started here rather
				// than on first use: SaveTxBatch returns early for a block
				// carrying no transactions, so on an idle chain the sweep would
				// never run, which is exactly when an oversized table sits
				// untouched.
				cs.StartTxBatchPruner()
				return cs
			}
		}
		fmt.Printf("⚠ PostgreSQL failed: %v - using file\n", err)
	}

	// Fallback to file
	cs.useDB = false
	if os.Getenv("RESET_STATE") == "true" {
		fmt.Println("✓ RESET_STATE=true — starting fresh")
		os.Remove(dataFile)
	} else {
		cs.loadFromFile(dataFile)
	}
	return cs
}

func (cs *ChainState) initDB() {
	// P3-10: log schema migration errors instead of silently ignoring them.
	dbExec := func(q string, args ...interface{}) {
		if _, err := cs.db.Exec(q, args...); err != nil {
			fmt.Printf("[DB] initDB warning: %v\n", err)
		}
	}
	dbExec(`CREATE TABLE IF NOT EXISTS evm_contracts (
address TEXT PRIMARY KEY,
bytecode TEXT NOT NULL,
deployer TEXT,
deployed_at TIMESTAMP DEFAULT NOW()
)`)
	dbExec(`CREATE TABLE IF NOT EXISTS evm_storage (
address TEXT NOT NULL,
slot TEXT NOT NULL,
value TEXT NOT NULL,
PRIMARY KEY (address, slot)
)`)
	dbExec(`CREATE TABLE IF NOT EXISTS evm_nonces (
address TEXT PRIMARY KEY,
nonce BIGINT DEFAULT 0
)`)
	dbExec(`CREATE TABLE IF NOT EXISTS chain_accounts (
address TEXT PRIMARY KEY,
balance FLOAT NOT NULL DEFAULT 0,
is_human BOOLEAN NOT NULL DEFAULT false
)`)
	// tusd_balance added separately (ALTER instead of being in the original
	// CREATE TABLE) so this upgrade doesn't require recreating the table on
	// chains that already have chain_accounts from before this feature.
	// P2-3 fix: ADD COLUMN before ALTER TYPE — on a fresh DB the column must
	// exist before we can change its type. IF NOT EXISTS makes both safe to run
	// on existing DBs too.
	dbExec(`ALTER TABLE chain_accounts ADD COLUMN IF NOT EXISTS tusd_balance FLOAT NOT NULL DEFAULT 0`)
	dbExec(`ALTER TABLE chain_accounts ADD COLUMN IF NOT EXISTS lp_shares FLOAT NOT NULL DEFAULT 0`)
	dbExec(`ALTER TABLE chain_accounts ADD COLUMN IF NOT EXISTS last_activity_at BIGINT NOT NULL DEFAULT 0`)
	dbExec(`ALTER TABLE chain_accounts ADD COLUMN IF NOT EXISTS demurrage_14_day_warning_shown BOOLEAN NOT NULL DEFAULT false`)
	dbExec(`ALTER TABLE chain_accounts ADD COLUMN IF NOT EXISTS faucet_claimed BOOLEAN NOT NULL DEFAULT false`)
	dbExec(`ALTER TABLE chain_accounts ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 0`)
	// wal_seq (SCALING_ARCHITECTURE.md Phase 7, transfer_wal.go): the highest
	// WAL sequence number this row's balance reflects. Only ever written by
	// the WAL flush worker and only ever read during WAL crash recovery —
	// every other existing read/write path in this file ignores it
	// entirely, so this column is additive and cannot change behavior when
	// AEQUITAS_WAL_ENABLED is unset (the default).
	dbExec(`ALTER TABLE chain_accounts ADD COLUMN IF NOT EXISTS wal_seq BIGINT NOT NULL DEFAULT 0`)
	// Upgrade balance columns to NUMERIC(20,6) for exact decimal storage.
	dbExec(`ALTER TABLE chain_accounts ALTER COLUMN balance TYPE NUMERIC(20,6) USING balance::NUMERIC(20,6)`)
	dbExec(`ALTER TABLE chain_accounts ALTER COLUMN tusd_balance TYPE NUMERIC(20,6) USING tusd_balance::NUMERIC(20,6)`)
	dbExec(`ALTER TABLE chain_accounts ALTER COLUMN lp_shares TYPE NUMERIC(20,6) USING lp_shares::NUMERIC(20,6)`)
	// Links a ZK proof commitment to the wallet that successfully registered
	// with it, so the app can ask "did MY proof get registered, and to which
	// wallet?" instead of guessing from a global, unfiltered list.
	dbExec(`CREATE TABLE IF NOT EXISTS bio_registrations (
commitment TEXT PRIMARY KEY,
wallet_address TEXT NOT NULL,
tx_hash TEXT,
registered_at TIMESTAMP DEFAULT NOW()
)`)
	// bio_hash lets the app poll "did MY device's identity hash get
	// registered yet, and to which wallet?" — needed because, under the new
	// flow where the proof is generated on the website (after MetaMask
	// supplies the real wallet), the app itself never computes a commitment
	// and so can't poll by one. It only ever knows its own bio_hash.
	dbExec(`ALTER TABLE bio_registrations ADD COLUMN IF NOT EXISTS bio_hash TEXT`)
	// FIX: old index only excluded NULL but not '' — multiple rows with
	// bio_hash='' (block-replay without real bio_hash) all conflicted.
	// Drop and recreate with the correct partial condition.
	// FIX (AQT-NEW-P2-01): if a legacy DB already has duplicate non-empty
	// bio_hash values, CREATE UNIQUE INDEX silently fails (initDB only
	// warns) and the node starts without the index. Deduplicate first:
	// back up duplicates into bio_registrations_bio_hash_dedup_backup,
	// keep the row with the earliest registered_at per bio_hash, delete
	// the rest. DROP+CREATE then always succeeds.
	dbExec(`CREATE TABLE IF NOT EXISTS bio_registrations_bio_hash_dedup_backup (
commitment    TEXT PRIMARY KEY,
wallet_address TEXT NOT NULL,
bio_hash      TEXT,
registered_at TIMESTAMP,
backed_up_at  TIMESTAMP DEFAULT NOW()
)`)
	// FIX (BRUTAL-P1-02): the old MIN(registered_at)/MIN(commitment) pair is
	// computed from two independent aggregates that need not come from the same
	// row — e.g. Row A (at=10, commitment=z) and Row B (at=20, commitment=a)
	// produce (10, a), a phantom row that never exists. The "winner" could be
	// wrong or every duplicate might be deleted. Use ROW_NUMBER() OVER (PARTITION
	// BY bio_hash ORDER BY registered_at ASC NULLS LAST, commitment ASC) to pick
	// the single canonical oldest row per bio_hash, then back up and delete
	// everything with rn > 1.
	dbExec(`INSERT INTO bio_registrations_bio_hash_dedup_backup
(commitment, wallet_address, bio_hash, registered_at)
SELECT a.commitment, a.wallet_address, a.bio_hash, a.registered_at
FROM (
  SELECT commitment, wallet_address, bio_hash, registered_at,
         ROW_NUMBER() OVER (
           PARTITION BY bio_hash
           ORDER BY registered_at ASC NULLS LAST, commitment ASC
         ) AS rn
  FROM bio_registrations
  WHERE bio_hash IS NOT NULL AND bio_hash != ''
) a
WHERE a.rn > 1
ON CONFLICT (commitment) DO NOTHING`)
	dbExec(`DELETE FROM bio_registrations
WHERE commitment IN (
  SELECT commitment FROM (
    SELECT commitment,
           ROW_NUMBER() OVER (
             PARTITION BY bio_hash
             ORDER BY registered_at ASC NULLS LAST, commitment ASC
           ) AS rn
    FROM bio_registrations
    WHERE bio_hash IS NOT NULL AND bio_hash != ''
  ) ranked
  WHERE rn > 1
)`)
	dbExec(`DROP INDEX IF EXISTS uidx_bio_registrations_bio_hash`)
	dbExec(`CREATE UNIQUE INDEX IF NOT EXISTS uidx_bio_registrations_bio_hash ON bio_registrations(bio_hash) WHERE bio_hash IS NOT NULL AND bio_hash != ''`)
	// Scale indices for 8B registrations: fast lookup by wallet without full scans.
	dbExec(`CREATE INDEX IF NOT EXISTS idx_bio_registrations_wallet ON bio_registrations(lower(wallet_address))`)
	dbExec(`CREATE INDEX IF NOT EXISTS idx_nullifiers_wallet ON nullifiers(lower(wallet_address))`)
	// Partial index on is_human lets distributeUBIPoolLocked enumerate all
	// registered humans from the DB without a full chain_accounts table scan.
	dbExec(`CREATE INDEX IF NOT EXISTS idx_chain_accounts_is_human ON chain_accounts(address) WHERE is_human = true`)
	// FIX (P3, beta-launch audit 2026-07-05): distributeLPPoolLocked and
	// checkAndMoveToEscrowLocked used to only iterate cs.accounts (the
	// in-memory cache), so a genuinely-inactive human or LP holder whose
	// account had been evicted (or never loaded) beyond maxInMemAccounts was
	// silently skipped — a scale-dependent correctness gap, same class as the
	// one distributeUBIPoolLocked already closed above by querying the DB
	// directly. These two indexes make the equivalent DB queries fast for
	// those two functions too — see their own comments.
	dbExec(`CREATE INDEX IF NOT EXISTS idx_chain_accounts_lp_shares ON chain_accounts(address) WHERE lp_shares > 0`)
	dbExec(`CREATE INDEX IF NOT EXISTS idx_chain_accounts_is_human_activity ON chain_accounts(last_activity_at) WHERE is_human = true`)
	// Single-row table holding the AEQ<->tUSD pool reserves. A fixed id=1 row
	// is used instead of a key-value table since there's only ever one pool
	// right now — simpler queries, and trivial to extend to multiple pools
	// later (id column is already there) if more pairs are ever added.
	dbExec(`CREATE TABLE IF NOT EXISTS liquidity_pool (
id INTEGER PRIMARY KEY DEFAULT 1,
reserve_aeq FLOAT NOT NULL DEFAULT 0,
reserve_tusd FLOAT NOT NULL DEFAULT 0,
total_lp_shares FLOAT NOT NULL DEFAULT 0
)`)
	dbExec(`ALTER TABLE liquidity_pool ADD COLUMN IF NOT EXISTS total_lp_shares FLOAT NOT NULL DEFAULT 0`)
	// Upgrade liquidity_pool columns to NUMERIC(20,6) for exact decimal storage
	// (same migration applied to chain_accounts columns above). Must run AFTER the
	// ADD COLUMN statements so that the column definitely exists before ALTER TYPE.
	dbExec(`ALTER TABLE liquidity_pool ALTER COLUMN reserve_aeq TYPE NUMERIC(20,6) USING reserve_aeq::NUMERIC(20,6)`)
	dbExec(`ALTER TABLE liquidity_pool ALTER COLUMN reserve_tusd TYPE NUMERIC(20,6) USING reserve_tusd::NUMERIC(20,6)`)
	dbExec(`ALTER TABLE liquidity_pool ALTER COLUMN total_lp_shares TYPE NUMERIC(20,6) USING total_lp_shares::NUMERIC(20,6)`)
	// nullifiers stores the one-way SHA256 derivative of each identity's bioHash.
	// Checked at registration time to prevent the same biometric from registering
	// with a second wallet. The nullifier itself never reveals the bioHash.
	dbExec(`CREATE TABLE IF NOT EXISTS nullifiers (
nullifier TEXT PRIMARY KEY,
wallet_address TEXT NOT NULL,
registered_at TIMESTAMP DEFAULT NOW()
)`)
	dbExec(`CREATE TABLE IF NOT EXISTS chain_config (
key TEXT PRIMARY KEY,
value TEXT NOT NULL
)`)
	// synthetic_checkpoint_events: durable audit trail for every
	// synthetic-checkpoint stub this node has ever inserted (see
	// BridgeHistoricalGap and queueOrphan's runtime-bridge branch). A stub
	// is a trust bypass — it satisfies a parent-existence check without any
	// verified header/signature/StateRoot behind it (audit 2026-06-30
	// monster audit, P0-02/P1-05). Previously the only record was a stdout
	// log line that scrolled away; this makes "did this node ever trust a
	// gap instead of proving it, and which hash/height" answerable after
	// the fact via a DB query, independent of log retention.
	dbExec(`CREATE TABLE IF NOT EXISTS synthetic_checkpoint_events (
id BIGSERIAL PRIMARY KEY,
stub_hash TEXT NOT NULL,
stub_height BIGINT NOT NULL,
source TEXT NOT NULL,
created_at BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW())
)`)
	// registration_recovery: written by registerOnV7 when the EVM transaction
	// succeeds but RegisterHumanAtomic fails after 3 retries. The background
	// RetryRegistrationRecoveries goroutine processes these until recovered_at
	// is set. This is the Variant-C fix for BRUTAL-P1-01 — see register.go.
	dbExec(`CREATE TABLE IF NOT EXISTS registration_recovery (
id BIGSERIAL PRIMARY KEY,
wallet TEXT NOT NULL,
evm_tx_hash TEXT NOT NULL,
nullifier TEXT,
pending_tx_json TEXT,
created_at BIGINT NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
attempt_count INT NOT NULL DEFAULT 0,
last_attempt_at BIGINT,
recovered_at BIGINT,
last_error TEXT
)`)
	dbExec(`CREATE INDEX IF NOT EXISTS idx_registration_recovery_unrecovered
ON registration_recovery(created_at) WHERE recovered_at IS NULL`)

	// Pending block transactions — persisted so they survive node restarts.
	// Without this, transfers via sendRawTransaction update Go-state/DB but
	// pendingTxs (in-memory) is lost on restart → secondary nodes never get
	// the TX in a block → balances permanently diverge across nodes.
	dbExec(`CREATE TABLE IF NOT EXISTS pending_txs (
id          SERIAL PRIMARY KEY,
tx_json     TEXT   NOT NULL,
created_at  BIGINT NOT NULL DEFAULT 0,
included_at BIGINT NOT NULL DEFAULT 0
)`)
	// FIX (audit 2026-06-28 recheck 5, P1-2): included_at lets LoadPendingTxs
	// mark a row as claimed atomically in the same query that selects it
	// (UPDATE ... RETURNING), instead of select-now/delete-later. See
	// LoadPendingTxs/ClearPendingTxs (evm_storage.go) for the duplicate-
	// processing risk this closes — a failed ClearPendingTxs delete used to
	// mean the row got loaded AGAIN by the next ProduceBlock call and
	// included in a second block.
	dbExec(`ALTER TABLE pending_txs ADD COLUMN IF NOT EXISTS included_at BIGINT NOT NULL DEFAULT 0`)
	// FIX (P1-01): track which block included each TX so ResetStaleIncludedPendingTxs
	// can skip rows whose block is already in chain_blocks, preventing the crash
	// window between SaveBlockToDB and ClearPendingTxs from causing double-inclusion.
	dbExec(`ALTER TABLE pending_txs ADD COLUMN IF NOT EXISTS included_block_hash TEXT`)
	// FIX (AQT-NEW-P2-02): rows with corrupt tx_json loop forever if they stay
	// in pending_txs — LoadPendingTxs unmarshal fails, ResetStaleIncludedPendingTxs
	// requeues them, next cycle same error. Preserve them here for diagnosis.
	dbExec(`CREATE TABLE IF NOT EXISTS pending_txs_dead_letter (
id         BIGINT PRIMARY KEY,
tx_json    TEXT NOT NULL,
created_at BIGINT NOT NULL DEFAULT 0,
failed_at  BIGINT NOT NULL DEFAULT 0,
fail_reason TEXT NOT NULL DEFAULT ''
)`)

	// FIX (audit 2026-06-28 full recheck, P1-3): block headers (dag.blocks/
	// dag.tips in block.go) used to be purely in-memory, reset to genesis on
	// every restart — recovery relied entirely on either the
	// max_block_height config counter (a bare number, not the actual block
	// data) or re-fetching blocks from a peer via HTTP-SYNC. A single node
	// that produces a block and crashes before broadcasting it to any peer
	// (or before any peer is even connected, e.g. a lone bootstrap node)
	// permanently loses that block: ClearPendingTxs had already removed its
	// explanatory pending_txs outbox rows, and nothing else durably recorded
	// that the block — or the TXs it carried — ever existed, even though the
	// account-state effects of those TXs were already committed to
	// chain_accounts earlier (at mutation time, before block assembly).
	// This table makes block headers themselves durable on the node that
	// produced or accepted them, independent of any peer, closing that gap.
	// See SaveBlockToDB/LoadBlocksFromDB and their call sites in block.go.
	dbExec(`CREATE TABLE IF NOT EXISTS chain_blocks (
hash          TEXT PRIMARY KEY,
height        BIGINT NOT NULL,
parent_hashes TEXT NOT NULL,
proposer      TEXT NOT NULL,
timestamp     BIGINT NOT NULL,
humans        INT NOT NULL DEFAULT 0,
state_root    TEXT NOT NULL DEFAULT '',
signature     TEXT NOT NULL DEFAULT '',
transactions  TEXT NOT NULL DEFAULT '[]',
created_at    TIMESTAMP DEFAULT NOW()
)`)
	dbExec(`CREATE INDEX IF NOT EXISTS idx_chain_blocks_height ON chain_blocks (height)`)

	// FIX (audit 2026-06-28 recheck 4, P1-5): notifyProofServer (register.go)
	// used to be pure fire-and-forget — a failed call (proof server down,
	// network blip) meant the proof server's bio_hashes table silently
	// never learned about this registration, with nothing durable
	// recording that the sync was ever attempted or that it failed. The
	// chain's own nullifier check remains the actual security boundary (a
	// duplicate registration is still rejected on-chain regardless), so
	// this gap couldn't let a duplicate human actually register — but it
	// could let the proof server keep generating (wasted, expensive) ZK
	// proofs for a biometric the chain would reject anyway, since the
	// proof server's own early duplicate-check never learned about it.
	// This table makes failed sync attempts durable so a periodic retry
	// job (see RetryProofServerSyncQueue) can actually catch up later
	// instead of the gap being permanent.
	dbExec(`CREATE TABLE IF NOT EXISTS proof_server_sync_queue (
bio_hash_key TEXT PRIMARY KEY,
wallet_address TEXT NOT NULL,
attempts INT NOT NULL DEFAULT 1,
last_error TEXT NOT NULL DEFAULT '',
created_at TIMESTAMP DEFAULT NOW(),
last_attempt_at TIMESTAMP DEFAULT NOW()
)`)
	// P2-4 fix: exponential-backoff columns. next_retry_at is the unix
	// timestamp after which the entry is eligible for retry (NULL = due now,
	// for rows created before this migration). dead=TRUE means the entry
	// has hit retryQueueMaxAttempts and requires manual intervention.
	dbExec(`ALTER TABLE proof_server_sync_queue ADD COLUMN IF NOT EXISTS next_retry_at BIGINT`)
	dbExec(`ALTER TABLE proof_server_sync_queue ADD COLUMN IF NOT EXISTS dead BOOLEAN NOT NULL DEFAULT FALSE`)

	// FIX (audit 2026-06-28 recheck 4, P1-6): syncBalanceLocked's
	// SaveStorageSlot writes (balanceOf/isHuman/lastActivity/lastDemurrage
	// EVM mirror slots) used to discard or only log their errors, with
	// nothing durable recording a failure — Go-state (the source of truth)
	// could be correct while the EVM mirror silently stayed stale forever.
	// See syncBalanceLocked's own comment (evm_storage.go) for why this
	// queue exists instead of folding these writes into the same SQL
	// transaction as the Go-state mutation they mirror.
	dbExec(`CREATE TABLE IF NOT EXISTS evm_mirror_sync_queue (
address TEXT NOT NULL,
contract_addr TEXT NOT NULL,
attempts INT NOT NULL DEFAULT 1,
last_error TEXT NOT NULL DEFAULT '',
created_at TIMESTAMP DEFAULT NOW(),
last_attempt_at TIMESTAMP DEFAULT NOW(),
PRIMARY KEY (address, contract_addr)
)`)
	// Same P2-4 backoff columns as proof_server_sync_queue above.
	dbExec(`ALTER TABLE evm_mirror_sync_queue ADD COLUMN IF NOT EXISTS next_retry_at BIGINT`)
	dbExec(`ALTER TABLE evm_mirror_sync_queue ADD COLUMN IF NOT EXISTS dead BOOLEAN NOT NULL DEFAULT FALSE`)

	// EVM transaction receipts — persisted so MetaMask can get correct
	// receipts after a node restart (avoids "Senden fehlgeschlagen" for
	// transactions that actually succeeded before the node restarted).
	dbExec(`CREATE TABLE IF NOT EXISTS evm_tx_receipts (
tx_hash    TEXT PRIMARY KEY,
from_addr  TEXT NOT NULL,
to_addr    TEXT,
status     TEXT NOT NULL DEFAULT '0x1',
created_at BIGINT NOT NULL
)`)
	// FIX: contract_addr was never persisted, so getTransactionReceipt lost
	// "contractAddress" for deployment TXs after every restart (deployedContracts
	// is in-memory only) — MetaMask/explorers would then show a deployment
	// receipt with contractAddress: null. ADD COLUMN IF NOT EXISTS is safe to
	// run against an existing table created before this column existed.
	dbExec(`ALTER TABLE evm_tx_receipts ADD COLUMN IF NOT EXISTS contract_addr TEXT`)
	// Keep only the last 10000 receipts to prevent unbounded growth.
	// Old receipts are pruned in SaveTxReceipt.

	cs.InitSwapNoncesTable()
	cs.InitValidatorKeysTable()
	cs.InitGiniSnapshotsTable()
	cs.InitPriceSnapshotsTable()
	if err := cs.InitGuardianTables(); err != nil {
		fmt.Printf("[DB] FATAL: InitGuardianTables failed: %v\n", err)
		panic(err)
	}
	// validator_slots and registered_nodes are created on first use inside
	// BindValidatorSlot / RegisterNode, but GetValidatorKeyPairsForSync and
	// IncrementBlockCount query/update them unconditionally on every sync cycle
	// and every accepted block.  Ensure both tables (and their late-added columns)
	// exist at startup so those calls never fail on a fresh DB.
	dbExec(`CREATE TABLE IF NOT EXISTS validator_slots (
operator_wallet TEXT PRIMARY KEY,
signing_address TEXT NOT NULL,
claimed_at TIMESTAMP DEFAULT NOW()
)`)
	dbExec(`ALTER TABLE validator_slots ADD COLUMN IF NOT EXISTS binding_signature TEXT DEFAULT ''`)
	dbExec(`CREATE TABLE IF NOT EXISTS registered_nodes (
wallet_address TEXT PRIMARY KEY,
signing_address TEXT DEFAULT '',
registered_at TIMESTAMP DEFAULT NOW(),
blocks_produced BIGINT NOT NULL DEFAULT 0
)`)
	dbExec(`ALTER TABLE registered_nodes ADD COLUMN IF NOT EXISTS blocks_produced BIGINT NOT NULL DEFAULT 0`)
	dbExec(`ALTER TABLE registered_nodes ADD COLUMN IF NOT EXISTS signing_address TEXT DEFAULT ''`)

	// Slashing tables — safe to call repeatedly (CREATE IF NOT EXISTS / ALTER IF NOT EXISTS).
	cs.initSlashingTables()
}

// resetDBStateForBootstrap is an explicit operator escape hatch for secondary
// nodes that must discard a divergent local DB before importing a signed
// bootstrap snapshot. It intentionally refuses to run on the primary or without
// BOOTSTRAP_SNAPSHOT_URL so RESET_DB_STATE cannot silently wipe a production
// chain database.
func (cs *ChainState) resetDBStateForBootstrap() {
	if cs.db == nil {
		return
	}
	if os.Getenv("IS_PRIMARY_NODE") == "true" {
		fmt.Println("[DB-RESET] Refused: RESET_DB_STATE=true on IS_PRIMARY_NODE=true")
		return
	}
	// Only honour within the first 5 minutes of startup.
	// An accidentally-retained RESET_DB_STATE would otherwise wipe the DB
	// on every Railway crash-recovery restart.
	if time.Since(processStartTime) > 5*time.Minute {
		fmt.Println("[DB-RESET] Refused: RESET_DB_STATE=true but process started >5 minutes ago — ignoring to prevent accidental wipe on restart")
		return
	}
	if os.Getenv("BOOTSTRAP_SNAPSHOT_URL") == "" {
		fmt.Println("[DB-RESET] Refused: RESET_DB_STATE=true requires BOOTSTRAP_SNAPSHOT_URL")
		return
	}
	// FIX (audit 2026-06-28 recheck 5, P2-5): all the gates above are still
	// just env vars — a wrong Railway/Render service selection, a copy-paste
	// from another deployment's env file, or a forgotten redeploy can set
	// them by accident. Require one more, explicitly-named confirmation
	// shared with clearRegistrationsFromDB, so a single accidental var
	// can't trigger either destructive path alone.
	if os.Getenv("ALLOW_DESTRUCTIVE_MAINTENANCE") != "true" {
		fmt.Println("[DB-RESET] Refused: RESET_DB_STATE=true also requires ALLOW_DESTRUCTIVE_MAINTENANCE=true")
		return
	}

	tables := []string{
		"pending_txs", // prevent stale TXs from polluting post-reset state
		"bio_registrations",
		"nullifiers",
		"bio_hashes",
		"evm_contracts",
		"evm_storage",
		"evm_nonces",
		"evm_tx_receipts",
		"registered_nodes",
		"validator_keys",
		"liquidity_pool",
		"swap_nonces",
		"price_snapshots",
		"gini_snapshots",
		"guardians",
		"escrow_accounts",
		"chain_accounts",
		"chain_config",
		"v6_balances",
		"v6_commitments",
		"v6_humans",
		"v6_state",
		// FIX: same reasoning as clearRegistrationsFromDB — without this, a
		// stale relationship-slot snapshot survives the reset and gets
		// blindly restored into evm_storage on the next automatic V7
		// redeploy, reintroducing isHuman/balanceOf entries this reset was
		// supposed to remove.
		"evm_upgrade_relationship_slots",
	}

	fmt.Println("[DB-RESET] RESET_DB_STATE=true — truncating local secondary DB before snapshot bootstrap")
	// FIX: track every failure instead of just printing an easy-to-miss
	// "Warning" and continuing as if nothing happened. A reset whose whole
	// purpose is to guarantee a clean slate before importing a snapshot must
	// not silently end in "Done" when some tables were never actually
	// truncated — that's exactly the kind of half-reset state that produced
	// "already registered" / StateRoot-divergence bugs throughout this
	// project's history.
	// FIX (Gesamtaudit 2026-06-28, P2-3): this used to TRUNCATE each table
	// individually via cs.db.Exec — every successful TRUNCATE auto-committed
	// immediately on its own. A failure partway through (table N+1 fails
	// after tables 1..N already truncated) left exactly the half-reset state
	// this function's own doc comment warns about: some tables wiped, others
	// not, with no way to undo the ones that already committed. Wrapping
	// every check+truncate in one real transaction means a failure anywhere
	// rolls back everything — this function's only two outcomes are now
	// "every table reset together" or "nothing changed at all", matching the
	// same all-or-nothing guarantee ResyncFromSnapshotURL already gives its
	// own writes.
	tx, err := cs.db.Begin()
	if err != nil {
		fmt.Printf("[DB-RESET] Refused: could not begin reset transaction: %v\n", err)
		return
	}
	var failed []string
	for _, table := range tables {
		var exists bool
		if err := tx.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, "public."+table).Scan(&exists); err != nil {
			fmt.Printf("[DB-RESET] Warning: could not check table %s: %v\n", table, err)
			failed = append(failed, table+" (existence check failed)")
			continue
		}
		if !exists {
			continue
		}
		if _, err := tx.Exec(fmt.Sprintf(`TRUNCATE TABLE %s RESTART IDENTITY CASCADE`, pq.QuoteIdentifier(table))); err != nil {
			fmt.Printf("[DB-RESET] Warning: could not truncate %s: %v\n", table, err)
			failed = append(failed, table)
		}
	}
	if len(failed) > 0 {
		if rbErr := tx.Rollback(); rbErr != nil {
			fmt.Printf("[DB-RESET] CRITICAL: rollback after failed reset also failed: %v\n", rbErr)
		}
		fmt.Printf("[ALERT] [DB-RESET] %d table(s) FAILED to truncate — rolled back, DB is UNCHANGED (not half-reset): %v. Investigate, then restart to retry.\n", len(failed), failed)
		return
	}
	if err := tx.Commit(); err != nil {
		fmt.Printf("[ALERT] [DB-RESET] commit failed — DB should be unchanged (Postgres rolls back a failed commit automatically): %v. Investigate, then restart to retry.\n", err)
		return
	}
	fmt.Println("[DB-RESET] Done")
	// FIX (audit 2026-06-28 recheck 5, P2-5): used to just log a warning and
	// let startup continue with RESET_DB_STATE still set — relying entirely
	// on the operator noticing and removing it before the next restart. Now
	// exits cleanly right here: the process won't finish starting (and
	// won't serve traffic) until the operator removes RESET_DB_STATE and
	// ALLOW_DESTRUCTIVE_MAINTENANCE and redeploys, turning "please remember
	// to remove this" into a forced step instead of a request.
	fmt.Println("[DB-RESET] ⚠ Remove RESET_DB_STATE and ALLOW_DESTRUCTIVE_MAINTENANCE from env vars now, then redeploy.")
	fmt.Println("[DB-RESET] Exiting so this node cannot start (and serve traffic) with the reset flags still set.")
	os.Exit(0)
}

// tableNameFromDelete extracts the table name from a "DELETE FROM <table>"
// or "DELETE FROM <table> WHERE ..." statement, returning "" for anything
// else (e.g. UPDATE statements, which should always run unconditionally
// since they only ever target tables initDB guarantees exist upfront).
func tableNameFromDelete(stmt string) string {
	const prefix = "DELETE FROM "
	if !strings.HasPrefix(stmt, prefix) {
		return ""
	}
	rest := stmt[len(prefix):]
	if idx := strings.IndexByte(rest, ' '); idx >= 0 {
		return rest[:idx]
	}
	return rest
}

// clearRegistrationsFromDB removes all human registration data without wiping
// the full DB. Triggered by CLEAR_REGISTRATIONS=true env var. Clears:
// nullifiers, bio_registrations, chain_accounts (is_human+balance), EVM
// storage slots for V7 (usedNullifiers/usedCommitments/isHuman), evm_nonces,
// evm_tx_receipts, and pending_txs. Safe to run on primary or secondary.
// Remove CLEAR_REGISTRATIONS=true after the first successful restart.
func (cs *ChainState) clearRegistrationsFromDB() {
	if cs.db == nil {
		return
	}
	if time.Since(processStartTime) > 5*time.Minute {
		fmt.Println("[CLEAR-REG] Refused: CLEAR_REGISTRATIONS=true but process started >5 minutes ago")
		return
	}
	// FIX (audit 2026-06-28 full recheck, P2-3): CLEAR_REGISTRATIONS=true on
	// its own is a single boolean a deploy tool, copy-paste from another
	// service's env file, or a typo'd "true" elsewhere could set by
	// accident — and once set, this wipes every human's registration data
	// on the very next restart with no further confirmation. Require a
	// second, explicit, impossible-to-fat-finger value alongside it.
	const clearConfirmPhrase = "I_UNDERSTAND_THIS_DELETES_ALL_REGISTRATIONS"
	if os.Getenv("CLEAR_REGISTRATIONS_CONFIRM") != clearConfirmPhrase {
		fmt.Printf("[CLEAR-REG] Refused: CLEAR_REGISTRATIONS=true requires CLEAR_REGISTRATIONS_CONFIRM=%s\n", clearConfirmPhrase)
		return
	}
	// FIX (audit 2026-06-28 recheck 5, P2-5): same additional gate as
	// resetDBStateForBootstrap — see its own comment.
	if os.Getenv("ALLOW_DESTRUCTIVE_MAINTENANCE") != "true" {
		fmt.Println("[CLEAR-REG] Refused: CLEAR_REGISTRATIONS=true also requires ALLOW_DESTRUCTIVE_MAINTENANCE=true")
		return
	}
	fmt.Println("[CLEAR-REG] Clearing all registration data from DB...")
	v7Addr := strings.ToLower(V7_CONTRACT_ADDR)
	stmts := []string{
		`DELETE FROM nullifiers`,
		`DELETE FROM bio_registrations`,
		// FIX: bio_hashes was never cleared here. CORRECTION (Monster Audit
		// follow-up, 2026-07-12): the comment used to claim "nothing on the
		// chain side reads this table for registration blocking" — false as
		// of register.go's GetWalletByStoredBioHash check (added after this
		// comment was written), which DOES reject a registration whose
		// bioHashKey/bioHash already has a row here — a real, active
		// secondary defense-in-depth dedup layer alongside the nullifier
		// check, not just write-only bookkeeping. Clearing it here is still
		// correct (a CLEAR_REGISTRATIONS wipe should reset every dedup layer
		// consistently, this one included), but for the actual reason —
		// leaving stale rows behind would keep blocking re-registration of a
		// biometric this same wipe just un-blocked everywhere else — not the
		// "nothing reads it" one. See register.go's own comment for the
		// current, accurate picture of what's write-only vs. actively
		// enforced.
		`DELETE FROM bio_hashes`,
		`UPDATE chain_accounts SET is_human = false, balance = 0, tusd_balance = 0, lp_shares = 0, last_activity_at = 0, faucet_claimed = false`,
		`DELETE FROM evm_storage WHERE lower(address) = '` + v7Addr + `'`,
		`DELETE FROM evm_nonces`,
		`DELETE FROM evm_tx_receipts`,
		`DELETE FROM pending_txs`,
		`DELETE FROM evm_contracts WHERE lower(address) = '` + v7Addr + `'`,
		// CRITICAL FIX: evm_upgrade_relationship_slots was never cleared here.
		// This table snapshots EVERY evm_storage row for V7 (see
		// SavePreUpgradeRelationshipSlots) before a contract-version upgrade
		// wipes evm_storage, then blindly restores all of it afterward via
		// RestorePreUpgradeRelationshipSlots — relying on MigrateEVMFromGoState
		// to overwrite the slots it knows how to re-derive (balanceOf, isHuman,
		// etc.) for every account that's still in chain_accounts. That
		// assumption breaks the moment CLEAR_REGISTRATIONS wipes chain_accounts
		// too: migration then has zero accounts to re-derive from, so EVERY
		// stale slot from the snapshot — including isHuman=true for wallets
		// this exact reset was supposed to un-register — gets faithfully
		// restored on the very next automatic V7 redeploy. Without this line,
		// a wallet that got its isHuman EVM slot stuck "true" (e.g. from the
		// concurrent-registration race fixed in 2dee74b) stayed stuck forever,
		// no matter how many times CLEAR_REGISTRATIONS was run — confirmed in
		// production via /api/admin/registration-debug and the
		// "[MIGRATE] Restored N guardian/escrow slots from pre-upgrade
		// snapshot" log line reappearing after every reset.
		`DELETE FROM evm_upgrade_relationship_slots WHERE address = '` + v7Addr + `'`,
		// CRITICAL FIX: liquidity_pool reserves were never reset here.
		// StateRoot() hashes cs.pool.ReserveAEQ/ReserveTUSD/TotalLPShares
		// directly (see state.go ~2261) — leaving stale pool reserves behind
		// while every other table gets wiped means two nodes that both ran
		// CLEAR_REGISTRATIONS at different points in their history (e.g. a
		// primary reset fresh, a secondary with leftover pool data from
		// before any reset ever touched it) compute permanently different
		// StateRoots for the IDENTICAL set of accounts/nullifiers — exactly
		// the "[DAG] StateRoot mismatch ... accepted (warn only)" /
		// "5+ consecutive StateRoot mismatches" pattern seen in production
		// on every single block between a freshly-reset primary and a
		// secondary whose liquidity_pool row was never touched.
		`UPDATE liquidity_pool SET reserve_aeq = 0, reserve_tusd = 0, total_lp_shares = 0 WHERE id = 1`,
	}
	// FIX: bio_hashes and evm_upgrade_relationship_slots are only ever
	// created lazily (by SaveBioHash / SavePreUpgradeRelationshipSlots) —
	// unlike nullifiers/bio_registrations/chain_accounts, which initDB
	// always creates upfront. On a node whose DB has never gone through a
	// registration or a contract-version upgrade, those two tables
	// genuinely don't exist yet, and DELETE FROM a nonexistent table prints
	// a scary "relation does not exist" warning that looks like a real
	// problem but is actually a harmless no-op. Skip cleanly instead.
	// FIX: track failures instead of printing an easy-to-miss "Warning" and
	// unconditionally claiming success at the end. This reset's entire job
	// is to guarantee no stale registration data survives it — a statement
	// that fails partway through (e.g. a transient DB hiccup) used to leave
	// some tables wiped and others not, while still printing "Done" with no
	// way to tell the two outcomes apart in the logs.
	// FIX (N1): wrap all DELETE/UPDATE statements in a single transaction so a
	// failure on statement N rolls back statements 1..N-1 too — identical to
	// the fix applied to resetDBStateForBootstrap. Previously, a mid-loop DB
	// error left some tables wiped and others not (half-reset state), which is
	// exactly the scenario the surrounding comment chain documents as catastrophic.
	clearTx, txErr := cs.db.Begin()
	if txErr != nil {
		fmt.Printf("[ALERT] [CLEAR-REG] Could not begin transaction: %v — no data changed.\n", txErr)
		return
	}
	for _, stmt := range stmts {
		tableName := tableNameFromDelete(stmt)
		if tableName != "" {
			var exists bool
			if err := cs.db.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, "public."+tableName).Scan(&exists); err == nil && !exists {
				continue
			}
		}
		if _, err := clearTx.Exec(stmt); err != nil {
			clearTx.Rollback()
			fmt.Printf("[ALERT] [CLEAR-REG] Statement FAILED — rolled back, no data changed: %v\n", err)
			fmt.Printf("[ALERT] [CLEAR-REG] Failed statement: %.100s\n", stmt)
			fmt.Println("[ALERT] [CLEAR-REG] Do not remove CLEAR_REGISTRATIONS yet — investigate and rerun.")
			return
		}
	}
	if err := clearTx.Commit(); err != nil {
		clearTx.Rollback()
		fmt.Printf("[ALERT] [CLEAR-REG] Transaction commit FAILED — no data changed: %v\n", err)
		return
	}
	fmt.Println("[CLEAR-REG] Done — all registrations cleared (transaction committed).")
	// FIX (audit 2026-06-28 recheck 5, P2-5): see resetDBStateForBootstrap's
	// matching comment — exits cleanly instead of relying on the operator
	// to remember to remove these vars before the node serves traffic again.
	fmt.Println("[CLEAR-REG] ⚠ Remove CLEAR_REGISTRATIONS, CLEAR_REGISTRATIONS_CONFIRM, and ALLOW_DESTRUCTIVE_MAINTENANCE from env vars now, then redeploy.")
	fmt.Println("[CLEAR-REG] Exiting so this node cannot start (and serve traffic) with the clear flags still set.")
	os.Exit(0)
}

// setConfigValue persists a key/value pair to chain_config (upsert) and
// returns an error if the write failed.
//
// FIX (audit recheck3, P0 #2): this used to call cs.db.Exec directly instead
// of cs.dbExec(), and returned nothing. Both mattered: last_ubi_at is
// StateRoot-relevant and written from inside runAtomicDistributionWithOutbox
// (via applyUBIFinalizeDeltaLocked) — calling cs.db.Exec there opened a
// SEPARATE auto-committing connection instead of joining cs.activeTx, so
// this write landed permanently the instant it ran, regardless of whether
// the surrounding distribution transaction later committed or rolled back.
// A rollback after this point reverted every account/pool change but left
// last_ubi_at changed anyway — a real, undetected gap in the atomic
// distribution work earlier this session. Now routes through cs.dbExec()
// like every other write in this file, and returns the error instead of
// only logging it, so callers that need to know (ResyncFromSnapshotURL,
// restoreFromRollback, applyUBIFinalizeDeltaLocked) actually can.
//
// PRECONDITION (audit 2026-06-28 recheck 4, P0-1): same as getConfigValue —
// caller must already hold cs.mu. Use setConfigValueDB outside any lock.
// setConfigValue is the context.Background()-calling wrapper kept for
// callers not yet migrated to thread ctx explicitly — see dbExecCtx's
// comment for the migration this is part of.
func (cs *ChainState) setConfigValue(key, value string) error {
	return cs.setConfigValueCtx(context.Background(), key, value)
}

func (cs *ChainState) setConfigValueCtx(ctx context.Context, key, value string) error {
	if cs.db == nil {
		return nil
	}
	if _, err := cs.dbExecCtx(ctx).Exec(`INSERT INTO chain_config (key, value) VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, key, value); err != nil {
		fmt.Printf("[DB] Warning: setConfigValue(%q) failed: %v\n", key, err)
		return fmt.Errorf("could not set config %q: %w", key, err)
	}
	return nil
}

// getConfigValue reads a key from chain_config, returning "" if missing.
// Uses cs.dbExec() so a read during an active transaction sees that
// transaction's own uncommitted writes instead of cs.db's separate
// connection, which wouldn't see them yet under Postgres MVCC.
//
// PRECONDITION (audit 2026-06-28 recheck 4, P0-1): the caller must already
// hold cs.mu for the duration of this call (read or write lock — cs.mu
// itself isn't touched here), or otherwise be certain no concurrent
// goroutine can be inside its own cs.mu-locked critical section right now.
// cs.activeTx, which cs.dbExec() reads, is ONLY synchronized by cs.mu — see
// activeTx's own field comment. Calling this without that lock held risks
// reading a DIFFERENT, concurrently-running atomic operation's in-flight
// transaction instead of either cs.db or your own transaction: a genuine
// data race on cs.activeTx itself, and a correctness bug (e.g. StateRoot
// observing another operation's uncommitted last_ubi_at). Callers outside
// any cs.mu hold (status endpoints, startup code, snapshot export) must use
// getConfigValueDB instead, which always reads cs.db directly and never
// touches cs.activeTx.
func (cs *ChainState) getConfigValue(key string) string {
	if cs.db == nil {
		return ""
	}
	var v string
	cs.dbExec().QueryRow(`SELECT value FROM chain_config WHERE key = $1`, key).Scan(&v)
	return v
}

// getConfigValueExists is getConfigValue plus whether the key actually has a
// row — needed by snapshotForRollback to distinguish "existed with empty
// value" / "didn't exist" from a plain "" return, so restoreFromRollback can
// tell a rollback "delete this key" instead of "nothing to restore". See
// configValueSnapshot. Same cs.mu-held precondition as getConfigValue.
func (cs *ChainState) getConfigValueExists(key string) (string, bool) {
	if cs.db == nil {
		return "", false
	}
	var v string
	err := cs.dbExec().QueryRow(`SELECT value FROM chain_config WHERE key = $1`, key).Scan(&v)
	if err != nil {
		return "", false
	}
	return v, true
}

// deleteConfigValue removes key from chain_config entirely and returns an
// error if the write failed. Used by restoreFromRollback to undo a block
// that set a StateRoot-relevant config key for the first time (setConfigValue
// alone can't represent "this key must not exist" — see configValueSnapshot).
// Routes through cs.dbExec() for the same reason as setConfigValue. Same
// cs.mu-held precondition as getConfigValue.
func (cs *ChainState) deleteConfigValue(key string) error {
	if cs.db == nil {
		return nil
	}
	if _, err := cs.dbExec().Exec(`DELETE FROM chain_config WHERE key = $1`, key); err != nil {
		fmt.Printf("[DB] Warning: deleteConfigValue(%q) failed: %v\n", key, err)
		return fmt.Errorf("could not delete config %q: %w", key, err)
	}
	return nil
}

// getConfigValueDB reads a key from chain_config via cs.db directly, NEVER
// via cs.activeTx — safe to call without holding cs.mu. Under Postgres's
// default read-committed isolation this only ever sees the last committed
// value, never another goroutine's in-flight transaction, which is exactly
// what a caller outside any atomic critical section wants (and the only
// thing it can safely use — see getConfigValue's precondition comment).
func (cs *ChainState) getConfigValueDB(key string) string {
	if cs.db == nil {
		return ""
	}
	var v string
	cs.db.QueryRow(`SELECT value FROM chain_config WHERE key = $1`, key).Scan(&v)
	return v
}

// getConfigValueExistsDB is getConfigValueDB plus whether the key actually
// has a row. See getConfigValue/getConfigValueExists for the existed-vs-empty
// distinction this preserves.
func (cs *ChainState) getConfigValueExistsDB(key string) (string, bool) {
	if cs.db == nil {
		return "", false
	}
	var v string
	if err := cs.db.QueryRow(`SELECT value FROM chain_config WHERE key = $1`, key).Scan(&v); err != nil {
		return "", false
	}
	return v, true
}

// setConfigValueDB writes a key to chain_config via cs.db directly, NEVER
// via cs.activeTx — safe to call without holding cs.mu. For callers that
// are not part of any atomic critical section (see getConfigValue's
// precondition comment for why joining a transaction without holding cs.mu
// would be unsafe).
func (cs *ChainState) setConfigValueDB(key, value string) error {
	if cs.db == nil {
		return nil
	}
	if _, err := cs.db.Exec(`INSERT INTO chain_config (key, value) VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, key, value); err != nil {
		fmt.Printf("[DB] Warning: setConfigValueDB(%q) failed: %v\n", key, err)
		return fmt.Errorf("could not set config %q: %w", key, err)
	}
	return nil
}

// GetLastUBIAt returns the Unix timestamp of the most recent UBI distribution,
// or 0 if it has never run.
func (cs *ChainState) GetLastUBIAt() int64 {
	// FIX (audit 2026-06-28 recheck 4, P0-1): no cs.mu held here — must use
	// the plain DB-only read, never cs.dbExec()/cs.activeTx.
	v := cs.getConfigValueDB("last_ubi_at")
	if v == "" {
		return 0
	}
	var t int64
	fmt.Sscan(v, &t)
	return t
}

// SecondsUntilNextUBI returns integer seconds until next UBI for the /api/status endpoint.
// P3-3: uses last_ubi_at from DB, not server uptime, so restarts don't give wrong countdowns.

// GetWealthCapInfo returns the current wealth cap parameters using the canonical
// formulas: bootstrapMultiplierLocked() for multiplier and 1000.0 for average.
// P2-2: ensures handleWealthCap shows the same values as enforceWealthCapLocked.
func (cs *ChainState) GetWealthCapInfo() (capAEQ float64, mult float64, avg float64, humans int) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	cs.accounts.Range(func(_ string, acc *AccountState) bool {
		if acc.IsHuman {
			humans++
		}
		return true
	})
	mult = cs.bootstrapMultiplierLocked()
	avg = cs.getAverageBalanceLocked()
	capAEQ = mult * avg
	return
}

// TryLockDistribution attempts to atomically claim the distribution slot.
// It uses a PostgreSQL compare-and-swap: updates last_ubi_at to now only if
// the current value is > 23 hours old (or missing). Returns true if this node
// won the lock — only then should it actually run the distribution.
// This replaces the IS_PRIMARY_NODE env-var, which any operator could set.
// TryLockDistribution claims the right to run THIS process's daily
// distribution attempt, returning false if this node already claimed it
// within the last ~24h (e.g. the goroutine somehow fired twice).
//
// FIX (audit recheck 2, P0 #3): this used to claim the lock by writing
// directly to chain_config's "last_ubi_at" key — the SAME key that feeds
// StateRoot (see StateRoot's read of last_ubi_at) and that
// ApplyUBIFinalizeDelta now sets as the actual, consensus-relevant
// distribution timestamp. A crash or any other interruption between this
// lock claim and the real distribution completing would leave
// last_ubi_at set to the LOCK's timestamp despite no distribution (and no
// explaining TX) having actually happened — a StateRoot field with no
// TX history behind it. Lock bookkeeping now lives in its own key,
// "distribution_lock_at", entirely separate from the value StateRoot
// reads.
func (cs *ChainState) TryLockDistribution() bool {
	if cs.db == nil {
		return true // no DB → single-node mode, always proceed
	}
	// Cross-node guard: if another node already distributed this round and
	// we've replayed that block (which sets last_ubi_at via
	// applyUBIFinalizeDeltaLocked), there's nothing left to do.
	if lastUBIAt := cs.GetLastUBIAt(); lastUBIAt > 0 &&
		time.Since(time.Unix(lastUBIAt, 0)) < 23*time.Hour+55*time.Minute {
		return false
	}
	threshold := fmt.Sprintf("%d", time.Now().Add(-23*time.Hour-55*time.Minute).Unix()) // F5-FIX: grace period, still < 24h
	now := fmt.Sprintf("%d", time.Now().Unix())
	// Insert if missing, update if older than threshold
	result, err := cs.db.Exec(
		`INSERT INTO chain_config (key, value) VALUES ('distribution_lock_at', $1)
ON CONFLICT (key) DO UPDATE SET value = $1
WHERE chain_config.value = '' OR COALESCE(NULLIF(regexp_replace(chain_config.value, '[^0-9]', '', 'g'), ''), '0')::BIGINT < $2`,
		now, threshold,
	)
	if err != nil {
		fmt.Printf("[POOLS] TryLockDistribution error: %v\n", err)
		return false
	}
	rows, _ := result.RowsAffected()
	return rows > 0
}

// SetNextUBIAt stores when the scheduler will next trigger pool distributions.
// Called by main.go immediately after calculating the next run time so the
// display timer is always in sync with the actual goroutine schedule.
func (cs *ChainState) SetNextUBIAt(unixTs int64) {
	// FIX (audit 2026-06-28 recheck 4, P0-1): no cs.mu held here — must use
	// the plain DB-only write, never cs.dbExec()/cs.activeTx.
	cs.setConfigValueDB("next_ubi_at", fmt.Sprintf("%d", unixTs))
}

// SecondsUntilNextUBI returns how many seconds until the next UBI distribution.
// Reads "next_ubi_at" which main.go writes every time it schedules a run,
// so the countdown is exact — not estimated from last_ubi_at + 24h.
func (cs *ChainState) SecondsUntilNextUBI() int64 {
	v := cs.getConfigValueDB("next_ubi_at")
	if v == "" {
		// Scheduler not yet started (non-primary node or fresh start before
		// first goroutine tick). Show no countdown rather than a wrong value.
		return 0
	}
	var nextAt int64
	fmt.Sscan(v, &nextAt)
	secs := nextAt - time.Now().Unix()
	if secs < 0 {
		return 0
	}
	return secs
}

// TimeUntilNextUBI returns how long until the next UBI distribution is due.
// Returns 0 if overdue.
func (cs *ChainState) TimeUntilNextUBI() time.Duration {
	last := cs.GetLastUBIAt()
	if last == 0 {
		return 5 * time.Second
	}
	next := time.Unix(last, 0).Add(24 * time.Hour)
	d := time.Until(next)
	if d < 0 {
		return 0
	}
	return d
}

// maxInMemAccounts caps how many chain_accounts rows are preloaded into memory
// at startup. Cold accounts above this threshold are fetched from the DB on
// demand via ensureAccountLoaded. At ~200 bytes per AccountState, 5M accounts
// ≈ 1 GB RAM — a safe default for a node with typical hardware.
const maxInMemAccounts = 5_000_000

// ensureAccountLoaded fetches addr from DB into cs.accounts if it isn't
// already there. Must be called while cs.mu (write lock) is held.
//
// Thin context.Background() wrapper around ensureAccountLoadedCtx — see
// dbExecCtx's own comment for what migration this is the first step of and
// why the wrapper exists (every one of this function's ~36 existing callers
// keeps compiling and behaving identically, untouched, until each is
// migrated to call the Ctx version directly).
func (cs *ChainState) ensureAccountLoaded(addr string) {
	cs.ensureAccountLoadedCtx(context.Background(), addr)
}

// ensureAccountLoadedCtx is ensureAccountLoaded's real implementation —
// see that function's comment.
func (cs *ChainState) ensureAccountLoadedCtx(ctx context.Context, addr string) {
	if _, ok := cs.accounts.Get(addr); ok {
		return
	}
	if cs.db == nil {
		return
	}
	acc := &AccountState{Address: addr}
	var bal, tusd, lp float64
	var version int64
	// FIX (deadlock, concurrency audit 2026-07-21): this used to always
	// query via cs.db (the shared connection pool) even when called from
	// inside an active runAtomicWithOutbox/runAtomicDistributionWithOutbox
	// transaction (cs.mu held, cs.activeTx set, one pool connection already
	// checked out for it). Under enough concurrent atomic operations to
	// saturate MaxOpenConns, every one of those already-open transactions
	// blocks waiting for cs.mu — held by whichever one is currently running
	// fn() — while THAT one's own cold-account load here waits for a 21st
	// pool connection that can only ever become free once one of the
	// cs.mu-blocked transactions completes, which can't happen until this
	// one releases cs.mu: a genuine self-deadlock, confirmed live via a
	// local concurrent-transfer benchmark (100 goroutines, MaxOpenConns=20,
	// zero transfers completed after 4+ minutes — full goroutine dump
	// showed every one of the 20 open *sql.Tx stuck in Tx.awaitDone with no
	// goroutine anywhere past cs.mu.Lock()). cs.dbExec() reuses the
	// already-held transaction's own connection when one is active (same
	// existing pattern every write in this file already uses), so this
	// query needs zero extra pool capacity instead of one more.
	err := cs.dbExecCtx(ctx).QueryRow(
		`SELECT balance, is_human, tusd_balance, lp_shares,
		        COALESCE(last_activity_at, 0), COALESCE(version, 1)
		 FROM chain_accounts WHERE lower(address) = $1`,
		addr,
	).Scan(&bal, &acc.IsHuman, &tusd, &lp, &acc.LastActivityAt, &version)
	if err != nil {
		// FIX (fresh Monster Audit 2026-07-12, P2): sql.ErrNoRows (genuinely
		// never registered) and a real transient DB error (connection drop,
		// pool exhaustion, timeout) used to be treated identically — silent
		// return, caller's !ok branch creates a fresh zero-balance account.
		// For ErrNoRows that's correct. For a real error on an address that
		// DOES have a real balance, it's the exact cold-cache bug class this
		// project has already hit and fixed at ~20 sites this session (see
		// the rollback/apply*DeltaLocked fixes), just triggered by DB flakiness
		// instead of a missing call. This function's callers (~36 sites
		// across money-movement, escrow, UBI, LP, guardian paths) can't be
		// safely switched to abort-on-error in one pass without touching
		// every one of them — too large a change to make well under time
		// pressure in this pass. Making the failure loud and observable
		// (instead of silently indistinguishable from "new account") is the
		// safe, scoped fix for now; a real error-propagating signature is
		// the correct follow-up, done deliberately with each call site
		// reviewed on its own.
		if err != sql.ErrNoRows {
			fmt.Printf("[STATE] ⚠ ensureAccountLoaded(%s): real DB error, not ErrNoRows — treating as cold/fresh account, which is WRONG if this address already has a balance: %v\n", addr, err)
		}
		return // not in DB (or DB unreachable) — caller's !ok branch will create a fresh account
	}
	acc.Balance = NewDecimal(bal)
	acc.TUsdBalance = NewDecimal(tusd)
	acc.LPShares = NewDecimal(lp)
	if version == 0 {
		version = 1
	}
	acc.Version = version
	// Cache this cold account's leaf so a later mutation can XOR its OLD
	// contribution out of accountSetXOR correctly. The account is already
	// folded into the accumulator (from the startup full scan), so we only set
	// the cache here — we must NOT XOR it in again.
	acc.leafHash = accountLeaf(acc)
	cs.accounts.Set(addr, acc)
}

// ensureAccountsLoaded is ensureAccountLoaded's batch counterpart: loads
// every addr in addrs that isn't already in cs.accounts via ONE
// WHERE address = ANY($1) query instead of one query per address (performance
// audit 2026-07-06 — distributeUBIPoolLocked used to call ensureAccountLoaded
// individually for every human, every daily distribution round). Caller must
// hold cs.mu. Addresses already cached, or genuinely absent from the DB
// (never registered), are silently skipped — the same contract
// ensureAccountLoaded's single-address version already has.
// ensureAccountsLoaded is the context.Background()-calling wrapper kept for
// callers not yet migrated to thread ctx explicitly — see dbExecCtx's
// comment for the migration this is part of.
func (cs *ChainState) ensureAccountsLoaded(addrs []string) {
	cs.ensureAccountsLoadedCtx(context.Background(), addrs)
}

func (cs *ChainState) ensureAccountsLoadedCtx(ctx context.Context, addrs []string) {
	if cs.db == nil {
		return
	}
	var missing []string
	for _, addr := range addrs {
		if _, ok := cs.accounts.Get(addr); !ok {
			missing = append(missing, addr)
		}
	}
	if len(missing) == 0 {
		return
	}
	// FIX (deadlock, same as ensureAccountLoaded's FIX comment): route
	// through cs.dbExecCtx(ctx) so this reuses an already-active
	// transaction's own connection instead of requesting a fresh one from
	// the shared pool — this is snapshotForRollbackLocked's own cold-load
	// call, reached from inside every runAtomicWithOutbox/
	// runAtomicDistributionWithOutbox critical section.
	rows, err := cs.dbExecCtx(ctx).Query(
		`SELECT address, balance, is_human, tusd_balance, lp_shares,
		        COALESCE(last_activity_at, 0), COALESCE(version, 1)
		 FROM chain_accounts WHERE lower(address) = ANY($1)`,
		pq.Array(missing),
	)
	if err != nil {
		// FIX (fresh Monster Audit 2026-07-12, P2): see ensureAccountLoaded's
		// identical-purpose comment. Here the blast radius is bigger than the
		// single-address version — one failed batch query means EVERY
		// address in `missing` (e.g. every human, on a daily UBI
		// distribution round) falls through to "treat as fresh account"
		// downstream, not just one. Loud logging so this is at least
		// observable; see the single-address version for why a full
		// abort-on-error signature change isn't done here in this pass.
		fmt.Printf("[STATE] ⚠ ensureAccountsLoaded: batch query failed for %d addresses — all will be treated as cold/fresh, which is WRONG for any that already have a balance: %v\n", len(missing), err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var addr string
		acc := &AccountState{}
		var bal, tusd, lp float64
		var version int64
		if err := rows.Scan(&addr, &bal, &acc.IsHuman, &tusd, &lp, &acc.LastActivityAt, &version); err != nil {
			fmt.Printf("[STATE] ⚠ ensureAccountsLoaded: row scan failed mid-batch — this address will be treated as cold/fresh, which is WRONG if it already has a balance: %v\n", err)
			continue
		}
		acc.Address = addr
		acc.Balance = NewDecimal(bal)
		acc.TUsdBalance = NewDecimal(tusd)
		acc.LPShares = NewDecimal(lp)
		if version == 0 {
			version = 1
		}
		acc.Version = version
		// See ensureAccountLoaded's identical comment: cache this cold
		// account's leaf so a later mutation can XOR its OLD contribution out
		// of accountSetXOR correctly — it's already folded into the
		// accumulator from the startup full scan, so only the cache is set.
		acc.leafHash = accountLeaf(acc)
		cs.accounts.Set(addr, acc)
	}
}

func (cs *ChainState) loadFromDB() {
	// FIX (2026-06-28, production incident): this used to give up silently
	// on the first error, leaving cs.accounts empty exactly as if the DB
	// genuinely had zero rows. main.go's only signal for "is this node
	// fresh, or does it have real history?" was TotalHumans()==0 — a
	// transient connection blip on this one query (the kind db.Ping()
	// succeeding moments earlier does not rule out: pool churn, a brief
	// network hiccup, a slow-starting Postgres proxy) made a node WITH a
	// full, intact chain_accounts table look indistinguishable from a
	// brand-new one, triggering an unwanted snapshot bootstrap/resync on
	// every restart that hit the hiccup — observed in production as a
	// node's block height repeatedly collapsing back toward genesis after
	// restarts that should have been routine. One retry after a short
	// delay absorbs the transient case for free; if it still fails,
	// accountsLoadFailed tells main.go this node's "fresh or not" status is
	// UNKNOWN, not "fresh", so it can refuse to bootstrap rather than guess.
	const baseQuery = "SELECT address, balance, is_human, tusd_balance, lp_shares, last_activity_at, demurrage_14_day_warning_shown, faucet_claimed, COALESCE(version,0) FROM chain_accounts"
	var totalAccounts int64
	cs.db.QueryRow(`SELECT COUNT(*) FROM chain_accounts`).Scan(&totalAccounts)
	query := baseQuery
	if totalAccounts > maxInMemAccounts {
		fmt.Printf("[SCALE] %d accounts in DB — preloading %d most-recent (cold accounts loaded on demand)\n",
			totalAccounts, maxInMemAccounts)
		query = baseQuery + fmt.Sprintf(" ORDER BY last_activity_at DESC LIMIT %d", maxInMemAccounts)
	}
	rows, err := cs.db.Query(query)
	if err != nil {
		fmt.Printf("⚠ Could not load from DB (attempt 1): %v — retrying once\n", err)
		time.Sleep(2 * time.Second)
		rows, err = cs.db.Query(query)
	}
	if err != nil {
		fmt.Printf("⚠ Could not load from DB after retry: %v — refusing to treat this node as fresh; bootstrap/resync will be skipped until a successful restart\n", err)
		cs.accountsLoadFailed = true
		return
	}
	defer rows.Close()
	count := 0
	mergedCount := 0
	for rows.Next() {
		acc := &AccountState{}
		var bal, tusd, lp float64
		if err := rows.Scan(&acc.Address, &bal, &acc.IsHuman, &tusd, &lp, &acc.LastActivityAt, &acc.Demurrage14DayWarningShown, &acc.FaucetClaimed, &acc.Version); err != nil {
			fmt.Printf("[DB] Scan error loading account: %v — skipping row\n", err)
			continue
		}
		acc.Balance = NewDecimal(bal)
		acc.TUsdBalance = NewDecimal(tusd)
		acc.LPShares = NewDecimal(lp)
		// Accounts loaded from DB must always use the conditional optimistic-lock
		// UPDATE path in saveAccountToDB. If the version column is NULL in an old
		// row, COALESCE returns 0, which would trigger the INSERT/unconditional
		// path and bypass the conflict check. Normalize to 1 — both in memory AND
		// in the DB row, so UPDATE … WHERE version = 1 actually finds the row.
		if acc.Version == 0 {
			acc.Version = 1
			cs.db.Exec(`UPDATE chain_accounts SET version = 1 WHERE lower(address) = $1 AND (version IS NULL OR version = 0)`,
				strings.ToLower(acc.Address))
		}
		count++

		// One-time migration: every state-mutating function (Transfer,
		// RegisterHuman, swapLocked, etc.) now consistently lowercases
		// addresses before using them as map keys — but rows written
		// BEFORE that fix could be stored under a mixed-case address (e.g.
		// MetaMask's checksum format) while later operations on the SAME
		// real wallet used lowercase, splitting one person's balance across
		// two separate accounts. This silently shrank what the UI showed
		// for that wallet without actually losing any AEQ — the rest was
		// just sitting under a differently-cased key. Merging here, once,
		// at load time, makes loadFromDB self-healing for any old data
		// without needing a separate manual SQL migration step.
		//
		// IMPORTANT: SQL row order is not guaranteed, so the mixed-case row
		// for a given wallet could arrive before OR after its lowercase
		// counterpart. We always check whether cs.accounts[normalized]
		// already exists — regardless of whether THIS row's own address
		// happened to already be lowercase — and merge into it rather than
		// assuming the first-seen row is "the real one".
		normalized := strings.ToLower(acc.Address)
		if existing, ok := cs.accounts.Get(normalized); ok {
			mergedCount++
			fmt.Printf("[MIGRATION] Merging duplicate-case account %s into %s (balance %.6f + %.6f, tusd %.6f + %.6f, lp %.6f + %.6f)\n",
				acc.Address, normalized, existing.Balance.Float(), acc.Balance.Float(), existing.TUsdBalance.Float(), acc.TUsdBalance.Float(), existing.LPShares.Float(), acc.LPShares.Float())
			existing.Balance = existing.Balance.Add(acc.Balance)
			existing.TUsdBalance = existing.TUsdBalance.Add(acc.TUsdBalance)
			existing.LPShares = existing.LPShares.Add(acc.LPShares)
			existing.IsHuman = existing.IsHuman || acc.IsHuman
			if acc.LastActivityAt > existing.LastActivityAt {
				existing.LastActivityAt = acc.LastActivityAt
				existing.Demurrage14DayWarningShown = acc.Demurrage14DayWarningShown
			}
			cs.saveAccountToDB(existing)
			if acc.Address != normalized {
				// Remove the old mixed-case row so it doesn't get re-merged
				// (harmlessly, but noisily) on every future restart.
				cs.db.Exec(`DELETE FROM chain_accounts WHERE address = $1`, acc.Address)
			}
			continue
		}
		acc.Address = normalized
		cs.accounts.Set(normalized, acc)
	}
	fmt.Printf("✓ Loaded %d accounts from PostgreSQL", count)
	if mergedCount > 0 {
		fmt.Printf(" (%d mixed-case duplicates merged)", mergedCount)
	}
	fmt.Println()

	// Load nullifiers into memory so IsNullifierUsed is O(1) without a DB hit.
	if nrows, nerr := cs.db.Query("SELECT nullifier, wallet_address FROM nullifiers"); nerr == nil {
		// defer replaced by explicit Close at end of block
		for nrows.Next() {
			var nul, wal string
			// P2-FIX: check scan error to skip malformed rows.
			if scanErr := nrows.Scan(&nul, &wal); scanErr != nil {
				fmt.Printf("[DB] Warning: nullifier scan error: %v\n", scanErr)
				continue
			}
			if nul == "" {
				continue
			}
			cs.nullifiers[nul] = wal
		}
		nrows.Close()
		fmt.Printf("✓ Loaded %d nullifiers from PostgreSQL\n", len(cs.nullifiers))
	}

	cs.loadOrInitPool()

	// Seed the incremental state-root accumulators from the full DB (see
	// ChainState.accountSetXOR). Runs once at startup, after accounts,
	// nullifiers and pool are loaded, so the very first StateRoot this node
	// computes already commits to every account and nullifier — including the
	// cold ones beyond maxInMemAccounts that will only ever be paged in on
	// demand. Startup is single-threaded, so no lock is needed here.
	cs.rebuildStateAccumulators()
	fmt.Printf("✓ State-root accumulators seeded (accountSetXOR=%x…, nullifierSetXOR=%x…)\n",
		cs.accountSetXOR[:4], cs.nullifierSetXOR[:4])
}

// loadOrInitPool reads the single liquidity_pool row, creating it (at
// 0/0/0) if it doesn't exist yet. The pool intentionally does NOT get
// auto-filled with any starting reserves: every AEQ in this system only
// ever exists because a real human registered for it ("money exists
// because people exist"), so a pool can't be seeded out of thin air
// without breaking that principle. Real liquidity has to come from
// someone actually depositing AEQ they earned via AddLiquidity below.
func (cs *ChainState) loadOrInitPool() {
	var reserveAEQ, reserveTUSD, totalShares float64
	err := cs.db.QueryRow("SELECT reserve_aeq, reserve_tusd, total_lp_shares FROM liquidity_pool WHERE id = 1").Scan(&reserveAEQ, &reserveTUSD, &totalShares)
	if err != nil {
		_, insertErr := cs.db.Exec(`INSERT INTO liquidity_pool (id, reserve_aeq, reserve_tusd, total_lp_shares) VALUES (1, 0, 0, 0)
ON CONFLICT (id) DO NOTHING`)
		if insertErr != nil {
			fmt.Printf("⚠ Could not create liquidity pool row: %v\n", insertErr)
		}
		cs.pool = &PoolState{ReserveAEQ: NewDecimal(0), ReserveTUSD: NewDecimal(0), TotalLPShares: NewDecimal(0)}
		fmt.Printf("✓ Liquidity pool created (empty — awaiting first deposit via AddLiquidity)\n")
		return
	}
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(reserveAEQ), ReserveTUSD: NewDecimal(reserveTUSD), TotalLPShares: NewDecimal(totalShares)}
	fmt.Printf("✓ Liquidity pool loaded: %.2f AEQ / %.2f tUSD / %.6f shares\n", reserveAEQ, reserveTUSD, totalShares)
}

func (cs *ChainState) loadFromFile(dataFile string) {
	data, err := os.ReadFile(dataFile)
	if err != nil {
		fmt.Println("✓ Starting with fresh chain state")
		return
	}
	var accounts map[string]*AccountState
	if err := json.Unmarshal(data, &accounts); err != nil {
		fmt.Println("⚠ Could not load state, starting fresh")
		return
	}
	fresh := newShardedAccounts()
	for addr, acc := range accounts {
		fresh.Set(addr, acc)
	}
	cs.accounts = fresh
	fmt.Printf("✓ Loaded chain state: %d accounts\n", len(accounts))
}

// save persists cs.accounts to the JSON state file in non-DB (file
// fallback) mode. Caller must already hold cs.mu (read or write) — every
// call site in this codebase is inside a "...Locked" function or an
// already cs.mu.Lock()'d block (DistributeUBIPool/LP/ValidatorsPool,
// transferLocked, registerHumanLocked, swapLocked, addLiquidityLocked,
// removeLiquidityLocked, claimTUsdFaucetLocked, ReleaseEscrowToUBI).
//
// FIX (deadlock): this used to take cs.mu.RLock() itself before
// marshaling. sync.RWMutex is not reentrant, so a caller that already
// holds cs.mu.Lock() (every real caller, per the list above) would
// deadlock forever the instant it reached this RLock — discovered via a
// unit test that actually exercised the non-DB code path (cs.useDB=false)
// for DistributeUBIPool with funds to distribute; "go test" caught it as
// a 10-minute timeout with all goroutines blocked on this exact RLock.
// In production this was silent because cs.useDB is true whenever
// Postgres is configured (see NewChainState), so save() returns above
// before ever reaching the lock — but any node that ever runs without a
// reachable Postgres (misconfigured DATABASE_URL, Postgres briefly down
// at startup) would freeze completely on the very first state-mutating
// call. Removing the internal lock here is correct, not just convenient:
// the caller's existing lock already guarantees a stable, exclusive view
// of cs.accounts for the marshal below.
func (cs *ChainState) save() {
	if cs.useDB {
		return // DB saves immediately in RegisterHuman/Transfer
	}
	data, _ := json.Marshal(cs.accounts)
	// D8-FIX: atomic write via temp-file + rename to prevent partial file
	// corruption if the process crashes mid-write.
	tmpPath := "/tmp/aequitas_state.json.tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		fmt.Printf("[STATE] Warning: failed to write state: %v\n", err)
		return
	}
	os.Rename(tmpPath, "/tmp/aequitas_state.json")
}

// errVersionConflict is returned internally by saveAccountToDBInner when an
// optimistic-lock UPDATE affects zero rows (another writer already advanced
// the row's version). See saveAccountToDB's loop for why this needs to be a
// distinguishable sentinel rather than a plain nil return.
var errVersionConflict = errors.New("optimistic lock version conflict")

// saveAccountToDB persists acc and returns an error if it could not be
// durably written after retries.
//
// FIX (audit3, P1 #4): this used to return nothing — a caller had no way
// to know a write silently failed (DB error) or hit a version conflict
// that exhausted all retries. runAtomicWithOutbox's fn() closures (and
// the "...Locked" functions they call: transferLocked, swapLocked,
// addLiquidityLocked, removeLiquidityLocked, claimTUsdFaucetLocked,
// registerHumanLocked) now check this error and abort instead of
// returning success while the underlying write never actually committed
// — see each of those functions' updated saveAccountToDB call sites.
// Call sites outside that atomic family (background EVM sync retries,
// snapshot import, etc.) may still choose to log-and-continue by
// discarding the returned error — Go allows ignoring it, and forcing
// every one of this function's ~50 call sites to handle failure
// identically would mean changing many call sites that were never
// claiming atomicity in the first place, for no behavioral change (they
// already only logged on failure).
// updateAccountLeafLocked folds acc's current leaf into cs.accountSetXOR,
// replacing whatever leaf was last counted for it (acc.leafHash — the zero
// hash for a brand-new account). XOR is self-inverse, so out-then-in keeps
// accountSetXOR equal to the XOR of every current account's leaf regardless
// of how many times this runs for the same account. Caller must hold cs.mu.
//
// Split out of saveAccountToDB/saveAccountsToDBBatch (see SCALING_
// ARCHITECTURE.md Phase 3) so the tokenomics-pool fast path can update the
// state-root accumulator eagerly, in-memory, at the moment a pool balance
// actually changes — independent of when (or whether yet) that balance has
// been durably flushed to Postgres. This keeps StateRoot correct and
// immediately deterministic across nodes (every node applies the same
// mutation at the same logical point, in the same order-independent XOR)
// even though each node's own Postgres flush timing is now purely a local
// durability concern, not a consensus one.
func (cs *ChainState) updateAccountLeafLocked(acc *AccountState) {
	newLeaf := accountLeaf(acc)
	// See accountSetXORMu's own field comment: uncontended (and therefore
	// free) for every cs.mu.Lock()-based caller, the actual synchronization
	// for the concurrent-transfer path (transfer_concurrent.go), which
	// only holds cs.mu.RLock().
	cs.accountSetXORMu.Lock()
	xorInto(&cs.accountSetXOR, acc.leafHash)
	xorInto(&cs.accountSetXOR, newLeaf)
	cs.accountSetXORMu.Unlock()
	acc.leafHash = newLeaf
}

func (cs *ChainState) saveAccountToDB(acc *AccountState) error {
	// FIX (audit 2026-06-28 full recheck, P0-3 — "saveAccountToDB
	// Konflikt-Retry kann die beabsichtigte Mutation verlieren"): this used
	// to retry up to 3 times on conflict by RELOADING the DB's current
	// absolute values into acc (balance, tusd, lp_shares, version, ...) and
	// then saving THAT back. That overwrites whatever delta the caller
	// computed in memory with whatever happened to be in the DB at conflict
	// time, then reports success on the next attempt — a textbook lost
	// update: "credit +10 AEQ" can silently become "re-save the current DB
	// balance unchanged, but bump the version", with no error anywhere.
	// generic persistence helper has no way to re-derive the caller's
	// intended delta from a freshly reloaded base — only the caller's own
	// business logic (e.g. transferLocked, ApplyTransferDelta) knows that.
	// So this no longer retries at all: a conflict is returned immediately
	// as a real error. Every call site that matters runs inside
	// runAtomicWithOutbox/runAtomicDistributionWithOutbox already (or holds
	// cs.mu for the whole replay — see replayTransactions), so this error
	// correctly aborts and rolls back the whole operation instead of
	// silently "succeeding" with the wrong data. acc.Version is still
	// resynced to the DB's value by saveAccountToDBInner before returning
	// errVersionConflict, so IF a caller chooses to retry the entire
	// business operation from scratch against fresh state, it has the
	// right base to do so — that retry decision belongs to the caller, not
	// to this generic helper.
	//
	// Note on why conflicts should be rare in practice: each node runs its
	// own independent Postgres (one writer process per DB, serialized by
	// cs.mu) — a conflict here would mean something outside this process's
	// normal control flow wrote to the same row (a stray unguarded
	// goroutine, manual SQL, or two instances briefly overlapping during a
	// deploy), not routine multi-node contention on a shared DB.
	return cs.saveAccountToDBCtx(context.Background(), acc)
}

// saveAccountToDBCtx is saveAccountToDB's real implementation — see
// dbExecCtx's comment for the migration this is part of.
func (cs *ChainState) saveAccountToDBCtx(ctx context.Context, acc *AccountState) error {
	err := cs.saveAccountToDBInnerCtx(ctx, acc)
	if err == nil {
		// Incremental state-root maintenance (see ChainState.accountSetXOR):
		// the row write succeeded, so replace this account's previously-counted
		// leaf with its new one. Every caller holds cs.mu for the whole
		// mutation (see this function's doc comment), so this shared-state
		// update needs no extra synchronization, and it participates in block
		// rollback via blockRollbackSnapshot.accountSetXOR.
		cs.updateAccountLeafLocked(acc)
		return nil
	}
	if errors.Is(err, errVersionConflict) {
		return fmt.Errorf("version conflict for account %s (resynced to DB version %d; caller must retry its business operation against fresh state, not this write alone): %w", acc.Address, acc.Version, err)
	}
	return err
}

// saveAccountToDBInnerCtx is saveAccountToDBInner's real implementation —
// see dbExecCtx's comment for the migration this is part of. Its only
// caller is saveAccountToDBCtx, so unlike the other shared low-level
// functions here, the original saveAccountToDBInner name is simply removed
// rather than kept as a wrapper -- nothing else in this codebase calls it.
func (cs *ChainState) saveAccountToDBInnerCtx(ctx context.Context, acc *AccountState) error {
	if !cs.useDB {
		acc.Version++ // no-DB mode: mark as saved
		return nil
	}
	var result sql.Result
	var err error
	// FIX (atomic outbox): use cs.dbExecCtx(ctx) instead of cs.db directly —
	// when the caller's operation has an active transaction, this write
	// becomes part of it (committed or rolled back together with the
	// pending_tx outbox insert) instead of always auto-committing on its
	// own connection.
	if acc.Version == 0 {
		// First write: caller has no cached baseline version, so there is
		// nothing to optimistically check against — INSERT for a genuinely
		// new address, or UPSERT-bump for one that (unexpectedly) already
		// has a row.
		//
		// FIX (found via TPS-benchmark investigation, 2026-07-23): this used
		// to blindly set acc.Version = 1 afterward (via the unconditional
		// acc.Version++ below, starting from the zero value) regardless of
		// what version the row ACTUALLY ended up at. That's only correct
		// when the row was genuinely absent before this call. Any caller
		// that constructs a fresh &AccountState{} (Version left at its Go
		// zero value) for an address that already has an existing row —
		// confirmed via a repeated-seed reproduction (TestDebugStaleVersionOverwrite)
		// mirroring exactly what re-running tps_bench_test.go's seed loop
		// against the same persistent bench DB does — silently desyncs
		// acc.Version from the DB's real version forever after, so the very
		// next optimistic-locked write for that address spuriously conflicts
		// (rows affected = 0, even though nothing else ever touched the
		// row). RETURNING version makes acc.Version reflect the row's ACTUAL
		// resulting version unconditionally, so this self-corrects
		// regardless of whether the row was new or already existed.
		if err = cs.dbExecCtx(ctx).QueryRow(`INSERT INTO chain_accounts (address, balance, is_human, tusd_balance, lp_shares, last_activity_at, demurrage_14_day_warning_shown, faucet_claimed, version) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1)
ON CONFLICT (address) DO UPDATE SET balance = $2, is_human = $3, tusd_balance = $4, lp_shares = $5, last_activity_at = $6, demurrage_14_day_warning_shown = $7, faucet_claimed = $8, version = COALESCE(chain_accounts.version,0) + 1
RETURNING version`,
			acc.Address, acc.Balance.Float(), acc.IsHuman, acc.TUsdBalance.Float(), acc.LPShares.Float(), acc.LastActivityAt, acc.Demurrage14DayWarningShown, acc.FaucetClaimed).Scan(&acc.Version); err != nil {
			fmt.Printf("[DB] Error saving account %s: %v\n", acc.Address, err)
			return fmt.Errorf("could not save account %s: %w", acc.Address, err)
		}
		// acc.Version was just set directly from the row's real, returned
		// value above — skip the shared acc.Version++ below (that's only
		// correct for the optimistic-lock branch, which increments from a
		// known-correct baseline it just confirmed still matched).
		return nil
	} else {
		// Optimistic locking: only update if version matches what we read.
		// If another node updated in parallel, rows affected = 0 → conflict detected.
		result, err = cs.dbExecCtx(ctx).Exec(`UPDATE chain_accounts SET balance = $2, is_human = $3, tusd_balance = $4, lp_shares = $5, last_activity_at = $6, demurrage_14_day_warning_shown = $7, faucet_claimed = $8, version = $9 + 1
WHERE address = $1 AND version = $9`,
			acc.Address, acc.Balance.Float(), acc.IsHuman, acc.TUsdBalance.Float(), acc.LPShares.Float(), acc.LastActivityAt, acc.Demurrage14DayWarningShown, acc.FaucetClaimed, acc.Version)
		if err == nil {
			if rows, _ := result.RowsAffected(); rows == 0 {
				// Conflict: another node wrote a newer version. Reload DB version
				// into memory so the next caller can retry with the correct base.
				// FIX (deadlock, same as ensureAccountLoaded's FIX comment):
				// cs.dbExecCtx(ctx) instead of cs.db — this runs inside the
				// same active transaction as the UPDATE just above.
				var dbVer int64
				cs.dbExecCtx(ctx).QueryRow(`SELECT version FROM chain_accounts WHERE lower(address) = $1`, acc.Address).Scan(&dbVer)
				acc.Version = dbVer // resync in-memory; do NOT increment
				fmt.Printf("[DB] Conflict: account %s modified by another node — local version reset to DB version %d\n", acc.Address, dbVer)
				// FIX (audit recheck2, P0 #2): used to return nil here. The
				// caller (saveAccountToDB) used "did Version change" as its
				// only success signal — and Version DOES change on a
				// conflict too (it's resynced to dbVer above), so a nil
				// return here made every first-attempt conflict look
				// identical to a successful write to that caller, which
				// then returned nil to ITS caller without ever retrying.
				// errVersionConflict lets saveAccountToDB tell the two
				// apart unambiguously.
				return errVersionConflict
			}
		}
	}
	if err != nil {
		fmt.Printf("[DB] Error saving account %s: %v\n", acc.Address, err)
		return fmt.Errorf("could not save account %s: %w", acc.Address, err)
	}
	// P0-1 fix: only increment version after a confirmed successful write
	acc.Version++
	return nil
}

// saveAccountsToDBBatch is saveAccountToDB's small-N batch counterpart:
// persists every account in accs via ONE round trip instead of one per
// account, while preserving BOTH guarantees saveAccountToDB gives a single
// account — per-row optimistic-version conflict detection (a stale
// in-memory account is never blindly overwritten) and the same
// accountSetXOR incremental state-root bookkeeping (see saveAccountToDB's
// own comment for why that exists) — applied per account here via the same
// accountLeaf/xorInto primitives, not reimplemented.
//
// Uses a writable-CTE upsert: an UPDATE matches every row whose current DB
// version equals what this process expects (the normal case, via a
// per-row WHERE ca.version = EXCLUDED-equivalent comparison against each
// row's own expected_version column — Postgres evaluates this separately
// per conflicting row in a multi-row VALUES set, so this is a genuine
// per-row check, not one condition applied uniformly to the whole batch).
// Any row NOT matched that way falls through to an INSERT ... ON CONFLICT
// DO UPDATE — for a genuinely new account (expected_version 0, no existing
// row) this is a clean insert; for a cold in-memory account whose real DB
// version this process never read (expected_version 0 but a row already
// exists) this still overwrites (same as saveAccountToDBInner's own
// Version==0 branch), but both CTEs RETURN the row's actual resulting
// version so the caller's acc.Version is set from that, not blindly
// incremented — see saveAccountToDBInnerCtx's matching fix (2026-07-23,
// same TPS-benchmark investigation) for why a blind increment silently
// desyncs acc.Version from the DB whenever the row already existed. A row
// with expected_version != 0 that the UPDATE didn't match (a genuine
// conflict) is deliberately excluded from the INSERT branch too, so it's
// neither updated nor inserted — the caller can tell it apart from a
// successful save by address, same as saveAccountToDBInner's
// RowsAffected==0 check does for a single account.
//
// Intended for small, fixed sets of accounts mutated together
// (distributeSwapFee's 4 pool addresses) — the query text grows linearly
// with len(accs), so this is not a substitute for saveAccountToDB at
// arbitrary N.
func (cs *ChainState) saveAccountsToDBBatch(accs []*AccountState) error {
	return cs.saveAccountsToDBBatchCtx(context.Background(), accs)
}

// writeDollarParam writes "$<n><suffix>" (e.g. "$7::boolean,") to sb —
// shared by saveAccountsToDBBatchCtx and savePendingTxsBatchExec's
// multi-row VALUES-list construction, in place of a per-placeholder
// fmt.Sprintf call (see saveAccountsToDBBatchCtx's own FIX comment for the
// measured allocation cost that replaced). Uses strconv.AppendInt into a
// stack-local scratch array rather than strconv.Itoa: Itoa still allocates
// one string per call (confirmed via a follow-up memory profile — it
// showed up as strconv.formatBits, 11% of this benchmark's total
// allocations, after the first pass replaced Sprintf+Join but kept Itoa),
// while AppendInt writes digits directly into a slice the caller already
// owns, with no allocation for n's range here (a handful of digits at
// most — batch sizes are bounded by transferBatchMaxSize).
func writeDollarParam(sb *strings.Builder, n int, suffix string) {
	var buf [20]byte
	sb.WriteByte('$')
	sb.Write(strconv.AppendInt(buf[:0], int64(n), 10))
	sb.WriteString(suffix)
}

// saveAccountsToDBBatchCtx is saveAccountsToDBBatch's real implementation —
// see dbExecCtx's comment for the migration this is part of.
func (cs *ChainState) saveAccountsToDBBatchCtx(ctx context.Context, accs []*AccountState) error {
	if len(accs) == 0 {
		return nil
	}
	if !cs.useDB {
		// Matches saveAccountToDB's own no-DB behavior exactly: accountSetXOR
		// bookkeeping still runs (it's driven by saveAccountToDBInner's success,
		// not by cs.useDB specifically — see saveAccountToDB's own comment),
		// only the SQL round trip is skipped.
		for _, acc := range accs {
			cs.updateAccountLeafLocked(acc)
			acc.Version++
		}
		return nil
	}
	// FIX (2026-07-23, 50k-TPS-goal investigation): this used to build a
	// VALUES(...) list whose TEXT SIZE grows linearly with len(accs) — up
	// to 1000 rows x 9 placeholders = 9000 individual $N parameters for a
	// full transferBatchMaxSize batch. Every call is a genuinely different
	// query string (batch size varies call to call), so Postgres/lib/pq
	// can never reuse a cached plan regardless — but the PARSE step itself
	// is also real work that scales with how much SQL text there is to
	// parse, independent of caching. A CPU profile of the parallel batch
	// path (processTransferBatchConcurrent) showed lib/pq's own
	// conn.prepareTo at ~18% of cumulative time. unnest() over 9 array
	// parameters makes the query TEXT constant regardless of batch size —
	// always exactly 9 placeholders — turning that parse cost from O(N)
	// into O(1). (A prior version of this comment's own FIX, before this
	// one, replaced fmt.Sprintf+strings.Join with a manual
	// strings.Builder-based VALUES-list construction for the SAME O(N)
	// growth — this supersedes that approach rather than layering on top
	// of it, since building 9 slices is simpler and the actual O(N) driver
	// remains one query with the same shape.)
	// FIX (2026-07-24, 50k-TPS-goal investigation): sort a LOCAL COPY of accs
	// by address before building the parallel arrays below -- unrelated to
	// correctness of what gets written (order-independent either way, the
	// UPDATE/INSERT below matches rows by address, not position), but
	// directly relevant to LOCK ORDER. Up to parallelBatchPoolSize (4)
	// batches from this same function, and separately the WAL flush
	// worker's own multi-row UPSERT (flushWALBatch, transfer_wal.go), can
	// all be touching overlapping chain_accounts rows concurrently --
	// without a shared, deterministic row-touch order, two such writers can
	// acquire the SAME two rows' locks in opposite order and deadlock
	// (confirmed live: "pq: deadlock detected (40P01)" under sustained load
	// in TestSustainedWAL_QueueConvergence once the flush worker stopped
	// blocking the whole fast path during its own writes -- see
	// SCALING_ARCHITECTURE.md's Runde-3 update). Sorting here (and
	// flushWALBatch sorting its own address list the same way, by the same
	// string ordering) makes every writer touching a given pair of rows
	// attempt them in the same relative order -- the standard fix for this
	// deadlock class. Not a 100% mathematically airtight guarantee against
	// every possible query plan Postgres might choose for the UPDATE...FROM
	// below (a sufficiently large batch could in principle still pick a
	// hash join that visits rows out of input order), but Postgres's own
	// deadlock detector remains as a last-resort safety net either way
	// (aborts one side cleanly, no corruption -- both this function's
	// caller and flushWALBatch already treat a failed write as a safe,
	// retryable no-op, never a partial/lost update).
	sorted := append([]*AccountState(nil), accs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Address < sorted[j].Address })

	addresses := make([]string, len(sorted))
	balances := make([]float64, len(sorted))
	isHumans := make([]bool, len(sorted))
	tusdBalances := make([]float64, len(sorted))
	lpShares := make([]float64, len(sorted))
	lastActivityAts := make([]int64, len(sorted))
	demurrageWarnings := make([]bool, len(sorted))
	faucetClaimeds := make([]bool, len(sorted))
	expectedVersions := make([]int64, len(sorted))
	for i, acc := range sorted {
		addresses[i] = acc.Address
		balances[i] = acc.Balance.Float()
		isHumans[i] = acc.IsHuman
		tusdBalances[i] = acc.TUsdBalance.Float()
		lpShares[i] = acc.LPShares.Float()
		lastActivityAts[i] = acc.LastActivityAt
		demurrageWarnings[i] = acc.Demurrage14DayWarningShown
		faucetClaimeds[i] = acc.FaucetClaimed
		expectedVersions[i] = acc.Version
	}
	args := []interface{}{
		pq.Array(addresses), pq.Array(balances), pq.Array(isHumans),
		pq.Array(tusdBalances), pq.Array(lpShares), pq.Array(lastActivityAts),
		pq.Array(demurrageWarnings), pq.Array(faucetClaimeds), pq.Array(expectedVersions),
	}
	query := `WITH updates(address, balance, is_human, tusd_balance, lp_shares, last_activity_at, demurrage_14_day_warning_shown, faucet_claimed, expected_version) AS (
	SELECT * FROM unnest($1::text[], $2::double precision[], $3::boolean[], $4::double precision[], $5::double precision[], $6::bigint[], $7::boolean[], $8::boolean[], $9::bigint[])
),
upd AS (
	UPDATE chain_accounts ca
	SET balance = u.balance, is_human = u.is_human, tusd_balance = u.tusd_balance,
	    lp_shares = u.lp_shares, last_activity_at = u.last_activity_at,
	    demurrage_14_day_warning_shown = u.demurrage_14_day_warning_shown,
	    faucet_claimed = u.faucet_claimed, version = u.expected_version + 1
	FROM updates u
	WHERE lower(ca.address) = lower(u.address) AND ca.version = u.expected_version
	RETURNING ca.address, ca.version
),
ins AS (
	INSERT INTO chain_accounts (address, balance, is_human, tusd_balance, lp_shares, last_activity_at, demurrage_14_day_warning_shown, faucet_claimed, version)
	SELECT address, balance, is_human, tusd_balance, lp_shares, last_activity_at, demurrage_14_day_warning_shown, faucet_claimed, 1
	FROM updates
	WHERE lower(address) NOT IN (SELECT lower(address) FROM upd) AND expected_version = 0
	ON CONFLICT (address) DO UPDATE SET
		balance = EXCLUDED.balance, is_human = EXCLUDED.is_human, tusd_balance = EXCLUDED.tusd_balance,
		lp_shares = EXCLUDED.lp_shares, last_activity_at = EXCLUDED.last_activity_at,
		demurrage_14_day_warning_shown = EXCLUDED.demurrage_14_day_warning_shown,
		faucet_claimed = EXCLUDED.faucet_claimed, version = COALESCE(chain_accounts.version, 0) + 1
	RETURNING address, version
)
SELECT address, version FROM upd UNION ALL SELECT address, version FROM ins`

	rows, err := cs.dbExecCtx(ctx).Query(query, args...)
	if err != nil {
		return fmt.Errorf("batch save failed: %w", err)
	}
	succeeded := make(map[string]int64, len(accs))
	for rows.Next() {
		var addr string
		var version int64
		if err := rows.Scan(&addr, &version); err == nil {
			succeeded[strings.ToLower(addr)] = version
		}
	}
	rows.Close()

	var conflicts []string
	for _, acc := range accs {
		newVersion, ok := succeeded[strings.ToLower(acc.Address)]
		if !ok {
			conflicts = append(conflicts, acc.Address)
			continue
		}
		// Same bookkeeping saveAccountToDB does on success (see its own
		// comment), applied per account here. acc.Version is set from the
		// row's actual RETURNING value, not blindly incremented — see this
		// function's own doc comment for why a blind increment is wrong
		// whenever expected_version was 0 but a row already existed.
		cs.updateAccountLeafLocked(acc)
		acc.Version = newVersion
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("version conflict for account(s) %v: %w", conflicts, errVersionConflict)
	}
	return nil
}

// Demurrage parameters. AEQ balances that haven't been touched (no
// transfer, swap, or liquidity action) for demurrageGracePeriodSeconds
// begin losing value continuously at demurrageMonthlyRate per month,
// compounding every second rather than in discrete daily/monthly steps —
// this avoids any visible "jump" at day/month boundaries. Touching the
// account in any AEQ-moving way resets the clock to zero, which is the
// entire point: money that's actively circulating never decays, only
// money sitting idle does. Modeled after real-world demurrage currencies
// (Wörgl's 1932 experiment used 1%/month; the long-running Chiemgauer
// uses roughly 2%/quarter ≈ 0.66%/month) — 0.5%/month here is a
// deliberately moderate starting point, slightly gentler than either.
// Lost AEQ is distributed across the same four tokenomics pools as the
// swap fee (40% validators / 30% LPs / 20% UBI / 10% treasury), not
// burned — it stays circulating in the system rather than vanishing
// from total supply. Only AEQ decays this way; tUSD (a simulated test
// currency, not the real UBI-grant currency) is unaffected.
const demurrageGracePeriodSeconds = 90 * 24 * 60 * 60 // 3 months
const demurrageMonthlyRate = 0.005                    // 0.5%/month

// wealthCapMultiplier defines the maximum AEQ a single account may hold,
// expressed as a multiple of the current average AEQ balance across all
// registered humans — not a fixed number. This makes the cap self-
// adapting: as the system grows and average wealth naturally rises
// through normal economic activity, the cap rises proportionally with
// it, rather than needing to be manually raised through discrete
// "phases" as the project matures. The cap only kicks in on incoming
// AEQ (registration grants, transfers, swap/liquidity payouts) — see
// enforceWealthCapLocked — never on a balance that's already there, so
// it can't retroactively punish someone for an average that later rose.
const wealthCapMultiplier = 25.0
const secondsPerMonth = 30 * 24 * 60 * 60 // approximation, consistent with the grace period's 30-day months

// touchActivity stamps address's LastActivityAt to now, resetting its
// demurrage clock. Called by every AEQ-moving action (Transfer, swaps,
// AddLiquidity/RemoveLiquidity, registration) — NOT by pure balance
// reads, since merely checking a balance isn't "using" the money. Caller
// must hold cs.mu (write lock).
func touchActivity(acc *AccountState) {
	acc.LastActivityAt = nowUnix()
	acc.Demurrage14DayWarningShown = false // new grace period — the 14-day notice can fire again when this one nears its end
}

// touchActivityAt is touchActivity for the REPLAY side, where "now" is the
// wrong instant to use.
//
// The producing node stamps nowUnix() while processing the transaction. Every
// other node sees that same transaction inside a block, possibly seconds later
// (normal operation) and possibly years later (a resync replaying historical
// blocks). Stamping the replaying node's own clock there is the exact pattern
// FromDemurrageLost and DistributionAt exist to avoid — see Transaction's own
// field comments: a value the primary decided must be replayed, never
// recomputed from local wall-clock time. block.Timestamp is that value here,
// already carried by every block and already part of its hash, so no new
// transaction field is needed.
//
// The stamp only ever moves FORWARD. A DAG merges blocks from competing tips,
// so replay order is not timestamp order; without this, replaying an older
// block after a newer one would drag an account's clock backwards and hand it
// demurrage it does not owe. Monotonic means the result does not depend on the
// order blocks happen to arrive in, which is the property the whole replay path
// is built on.
//
// at <= 0 falls back to nowUnix() for the handful of non-block callers (see
// ApplyTransferDelta), preserving their existing behaviour exactly.
//
// The stamp is also clamped to now, because block.Timestamp is chosen by the
// PROPOSER and nothing validates it: there is no future-drift check anywhere on
// the block-acceptance path (the only comparison against Timestamp in block.go
// is the fixed equivocationSlashingActivationUnix constant). Without the clamp,
// a validator could stamp LastActivityAt years into the future on any account
// it touches — and since demurrage and the 2.5-year escrow sweep are both
// measured from this field, that would exempt those accounts from decay and
// from inactivity recovery permanently. Clamping turns an unbounded, chain-wide
// exploit into at worst a per-node difference of a few seconds, in a field that
// is deliberately not part of accountLeaf and therefore not consensus-hashed.
// (The missing future-drift validation on blocks themselves is a separate,
// pre-existing gap — it is not created or worsened here.)
func touchActivityAt(acc *AccountState, at int64) {
	if at <= 0 {
		at = nowUnix()
	}
	if now := nowUnix(); at > now {
		at = now
	}
	if at <= acc.LastActivityAt {
		return
	}
	acc.LastActivityAt = at
	acc.Demurrage14DayWarningShown = false
}

// nowUnix exists as a single seam so demurrage timing could be mocked in
// tests later; right now it's just time.Now().Unix().
func nowUnix() int64 {
	return timeNowFunc().Unix()
}

// effectiveBalance computes what address's AEQ balance is RIGHT NOW,
// continuously decayed for any time past the grace period since
// LastActivityAt — without writing anything. This is what every balance
// read (GetBalance, /api/balance, /api/humans, etc.) should show, so the
// number displayed always reflects live decay even between the
// lazy-settlement points (see settleDemurrageLocked) where it actually
// gets written to the stored Balance field. Caller must hold at least a
// read lock.
func effectiveBalance(acc *AccountState) Decimal {
	if acc.LastActivityAt == 0 {
		return acc.Balance
	}
	idleSeconds := nowUnix() - acc.LastActivityAt
	if idleSeconds <= demurrageGracePeriodSeconds {
		return acc.Balance
	}
	decayingSeconds := float64(idleSeconds - demurrageGracePeriodSeconds)
	monthsDecaying := decayingSeconds / float64(secondsPerMonth)
	factor := math.Pow(1-demurrageMonthlyRate, monthsDecaying)
	return acc.Balance.MulFloat(factor)
}

// settleDemurrageLocked actually writes off the decay computed by
// effectiveBalance into acc.Balance, and distributes what was lost across
// the four tokenomics pools — same split as the swap fee. This is called
// right before any operation that's about to read-then-modify Balance
// (Transfer, swaps, liquidity actions), so those operations always work
// from an up-to-date, already-settled balance instead of accidentally
// granting someone pre-decay value just because they happened to act at
// that exact moment. Caller must hold cs.mu (write lock).
// Returns the amount that was decayed (0 if nothing was settled) so callers
// on the primary node can attach it to the queued Transaction — secondary
// nodes replay this exact figure via applyDemurrageLossLocked instead of
// recomputing it themselves (which would use their own wall-clock time and
// diverge from the primary's StateRoot; see ApplyTransferDelta etc.).
// Returns an error (audit fresh-pass finding, 2026-06-30 — same class as
// enforceWealthCapLocked's fix, see its comment) instead of just logging one:
// it used to deduct the decay from acc.Balance FIRST and only THEN try to
// credit the pools via distributeSwapFee, silently discarding the AEQ if
// that credit failed. Every call site already runs inside
// runAtomicWithOutbox (directly or via its caller), so returning the error
// here lets it reach that transaction's fn() and trigger a real rollback.
func (cs *ChainState) settleDemurrageLocked(acc *AccountState) (Decimal, error) {
	return cs.settleDemurrageLockedCtx(context.Background(), acc)
}

// settleDemurrageLockedCtx is settleDemurrageLocked's real implementation —
// see dbExecCtx's comment for the migration this is part of.
func (cs *ChainState) settleDemurrageLockedCtx(ctx context.Context, acc *AccountState) (Decimal, error) {
	// P0-FIX: pool addresses are tokenomics infrastructure — never apply
	// demurrage to them. Doing so would drain pool balances incorrectly.
	if isTokenomicsPoolAddress(acc.Address) {
		return 0, nil
	}
	current := effectiveBalance(acc)
	lost := acc.Balance.Sub(current)
	if lost <= 0 {
		return 0, nil
	}
	acc.Balance = current
	if err := cs.distributeSwapFeeCtx(ctx, lost.Float(), true); err != nil {
		return 0, fmt.Errorf("demurrage: could not persist pool credits for %s: %w", acc.Address, err)
	}
	fmt.Printf("[DEMURRAGE] %s: idle balance decayed by %.6f AEQ, redistributed to pools\n", acc.Address, lost.Float())
	return lost, nil
}

// applyDemurrageLossLocked applies a demurrage loss already decided by the
// primary node (lost, in AEQ) directly to acc's balance and redistributes it
// to the tokenomics pools, WITHOUT consulting effectiveBalance()/nowUnix().
// Used exclusively by secondary-node replay (the "Delta" functions below) so
// every node arrives at byte-identical state for a given block, regardless
// of how much wall-clock time has passed since the primary processed it
// (live replication has sub-second skew; a node resyncing from genesis can
// be replaying months-old transactions all at the "current" wall-clock
// instant, which would otherwise decay them by the wrong amount entirely).
// Caller must hold cs.mu (write lock).
// FIX (audit 2026-06-30 monster audit, P1-02): used to swallow
// distributeSwapFee's error as a warn-only log line — the same class of bug
// already fixed for settleDemurrageLocked and enforceWealthCapLocked (see
// ApplyTransferDelta's comment). A failure here means acc.Balance was
// already decremented in-memory but the matching pool credit never
// persisted: exactly the silent value-loss / StateRoot-divergence risk the
// audit flagged. Now returns the error so every Delta-replay call site can
// reject the block instead of continuing on divergent state.
// applyDemurrageLossLocked is the context.Background()-calling wrapper kept
// for callers not yet migrated to thread ctx explicitly — see dbExecCtx's
// comment for the migration this is part of.
func (cs *ChainState) applyDemurrageLossLocked(acc *AccountState, lost float64) error {
	return cs.applyDemurrageLossLockedCtx(context.Background(), acc, lost)
}

func (cs *ChainState) applyDemurrageLossLockedCtx(ctx context.Context, acc *AccountState, lost float64) error {
	if lost <= 0 || isTokenomicsPoolAddress(acc.Address) {
		return nil
	}
	acc.Balance = acc.Balance.Sub(NewDecimal(lost))
	if err := cs.distributeSwapFeeCtx(ctx, lost, true); err != nil {
		return fmt.Errorf("could not persist pool credits for %s demurrage delta: %w", acc.Address, err)
	}
	return nil
}

// readAccount runs a read-only fn against address under the READ lock
// whenever the account is already resident, escalating to the write lock only
// when it genuinely has to be loaded from Postgres first.
//
// FIX (P0 availability, 2026-07-25): every read-only account getter below
// took cs.mu.Lock() — the global chain-state WRITE lock — purely because
// ensureAccountLoaded may insert into cs.accounts on a cache miss. That made
// the miss path's cost the price of EVERY call, including the overwhelmingly
// common hit.
//
// The reach is larger than it looks: GetBalance alone backs eth_getBalance
// (evm_rpc.go), i.e. every wallet balance refresh from every connected
// MetaMask, plus four calls per /api/status hit before StatusMetrics
// stopped that. Go's RWMutex queues readers behind a waiting writer, so each
// such request both waited out whatever held cs.mu — block replay holds it
// for a whole block, and this chain still carries 50,000-transfer load-test
// blocks — and blocked every reader behind it. Measured on the live primary:
// /api/status at 11.0s while endpoints avoiding cs.mu answered in 0.22s, and
// /api/peers/register timing out often enough that peer challenges expired
// before the retry landed.
//
// ensureAccountLoaded already returns immediately when the account is
// resident, so the write lock was only ever NEEDED on a miss. Checking
// residency under RLock first is not a weakening: a hit performs exactly the
// same reads as before under a lock that admits other readers, and a miss
// takes the identical write-locked path, re-checking residency after
// acquiring it (another goroutine may have loaded it in between).
//
// fn MUST be pure. effectiveBalance, IsHuman, TUsdBalance and LPShares only
// read AccountState fields; settleDemurrageLocked — which actually writes
// decay off — is deliberately not reachable from here and keeps its
// documented write-lock contract.
func (cs *ChainState) readAccount(address string, fn func(*AccountState)) {
	address = strings.ToLower(address)
	cs.mu.RLock()
	if acc, ok := cs.accounts.Get(address); ok {
		fn(acc)
		cs.mu.RUnlock()
		return
	}
	cs.mu.RUnlock()

	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.ensureAccountLoaded(address)
	if acc, ok := cs.accounts.Get(address); ok {
		fn(acc)
	}
}

func (cs *ChainState) GetBalance(address string) float64 {
	var out float64
	cs.readAccount(address, func(acc *AccountState) {
		out = effectiveBalance(acc).Float()
	})
	return out
}

// DistributeUBIPool empties the UBI pool address's entire AEQ balance,
// splitting it equally across every currently-registered human, then
// calls cs.save()/persists each affected account. Intended to be called
// once a day by a background ticker (see main.go) — not on every block,
// since "the UBI pool" only makes sense as a daily payout, not a
// per-block trickle. The pool is fully drained each time rather than
// only partially distributed: any AEQ that flows into it between now
// and the next run (swap fees, demurrage, wealth-cap overflow) accrues
// fresh, so there's no need to hold a standing reserve.
// RegisterNode adds this node's operator wallet to the registered_nodes
// table so it participates in future validators pool distributions.
// BindValidatorSlot binds operatorWallet to signingAddress, overwriting
// any previous binding for that wallet. Called from handlePeerRegister
// right before AddAuthorizedValidator grants block-signing authority,
// and ONLY after the caller has verified an OperatorBindingSignature
// proving operatorWallet's private-key owner specifically authorized
// THIS signingAddress (see verifyPersonalSign and the "Aequitas:
// authorize validator <addr>" message built in handlePeerRegister).
//
// FIX (audit recheck 2 follow-up): an earlier version of this function
// (TryClaimValidatorSlot) bound on a first-come-first-served basis with
// no proof of operatorWallet ownership at all — IsHuman(operatorWallet)
// only confirms SOME registered human owns that address, not that the
// requester does. Anyone who controlled a validator signing key could
// have submitted any OTHER human's wallet as NODE_OPERATOR_WALLET,
// permanently squatting that human's validator slot before they ever
// got a chance to run their own node. Requiring a signature from
// operatorWallet itself closes that hole AND gives operators a
// self-service way to rebind to a new signing key (e.g. after losing
// the old one): sign a fresh message naming the new address, no
// biometric re-verification or admin intervention needed — the
// signature alone re-proves the same ownership the original bind relied
// on. Overwriting is therefore always safe to allow once the signature
// checks out; there is no "permanent lock-in" to defend against anymore.
func (cs *ChainState) BindValidatorSlot(operatorWallet, signingAddress, bindingSig string) error {
	if cs.db == nil {
		return nil // no DB → single-node mode, nothing to enforce
	}
	operatorWallet = strings.ToLower(operatorWallet)
	signingAddress = strings.ToLower(signingAddress)
	// FIX (P0, 2026-07-02): this used to re-run CREATE TABLE IF NOT EXISTS /
	// ALTER TABLE ADD COLUMN IF NOT EXISTS on every single call — cs.initDB
	// (state.go, called once at boot) already guarantees this exact table
	// and column exist, so these were pure redundant DDL on a hot path.
	// Confirmed live: a diverged peer re-authenticating via
	// /api/peers/register every ~20s was issuing this DDL just as often,
	// each one taking a Postgres table lock, correlating with Primary's
	// block cadence degrading from ~1s to ~4s and API p95/p99 latency
	// spiking to 12-25s while CPU stayed idle — the signature of contention
	// on a slow synchronous call, not request volume.
	_, err := cs.db.Exec(
		`INSERT INTO validator_slots (operator_wallet, signing_address, binding_signature, claimed_at) VALUES ($1, $2, $3, NOW())
ON CONFLICT (operator_wallet) DO UPDATE SET signing_address = EXCLUDED.signing_address, binding_signature = EXCLUDED.binding_signature, claimed_at = NOW()`,
		operatorWallet, signingAddress, bindingSig,
	)
	if err != nil {
		return fmt.Errorf("could not bind validator slot for %s: %w", operatorWallet, err)
	}
	// FIX (audit recheck2, P1 #8): registered_nodes.signing_address used to
	// only ever be set from the RELAYER_ADDRESS env var (RegisterNode, at
	// startup) — a value entirely unrelated to the verified signing address
	// this function just bound. IncrementBlockCount credits blocks to
	// whichever wallet/signing-address row matches the block's actual
	// proposer (the signing key) — so any operator using the wallet-bound
	// model this function exists for (operatorWallet != signingAddress,
	// the whole point of the Sybil-resistance redesign) had a
	// registered_nodes row whose signing_address never matched their real
	// proposer address, and whose wallet_address didn't either (that's the
	// OPERATOR's human wallet, not the signing key) — every block they
	// produced credited zero rows, so they earned no validator-pool reward
	// despite being correctly authorized to produce blocks. Updating the
	// SAME row this bind authorizes, with the SAME verified address, keeps
	// authorization and reward-eligibility from the one source instead of
	// two unrelated ones that can never agree by construction.
	//
	// CREATE TABLE here too (not just in RegisterNode) — BindValidatorSlot
	// can run on a node whose own NODE_OPERATOR_WALLET was never set (e.g.
	// the primary, authorizing a remote secondary's bind via
	// handlePeerRegister), meaning RegisterNode's own table creation may
	// never have run on this node at all.
	cs.db.Exec(`CREATE TABLE IF NOT EXISTS registered_nodes (
wallet_address TEXT PRIMARY KEY,
signing_address TEXT DEFAULT '',
registered_at TIMESTAMP DEFAULT NOW(),
blocks_produced BIGINT NOT NULL DEFAULT 0
)`)
	if _, err := cs.db.Exec(
		`INSERT INTO registered_nodes (wallet_address, signing_address) VALUES ($1, $2)
ON CONFLICT (wallet_address) DO UPDATE SET signing_address = EXCLUDED.signing_address`,
		operatorWallet, signingAddress,
	); err != nil {
		// Non-fatal: block-signing authorization (validator_slots, already
		// committed above) must not depend on reward bookkeeping succeeding.
		fmt.Printf("[NODE] Warning: bound validator slot for %s but could not sync registered_nodes.signing_address: %v\n", operatorWallet, err)
	}
	return nil
}

// Called once at startup if NODE_OPERATOR_WALLET env var is set.
// Safe to call multiple times — ON CONFLICT DO NOTHING.
func (cs *ChainState) RegisterNode(operatorWallet string) {
	if cs.db == nil || operatorWallet == "" {
		return
	}
	wallet := strings.ToLower(operatorWallet)
	cs.db.Exec(`CREATE TABLE IF NOT EXISTS registered_nodes (
wallet_address TEXT PRIMARY KEY,
signing_address TEXT DEFAULT '',
registered_at TIMESTAMP DEFAULT NOW(),
blocks_produced BIGINT NOT NULL DEFAULT 0
)`)
	cs.db.Exec(`ALTER TABLE registered_nodes ADD COLUMN IF NOT EXISTS blocks_produced BIGINT NOT NULL DEFAULT 0`)
	cs.db.Exec(`ALTER TABLE registered_nodes ADD COLUMN IF NOT EXISTS signing_address TEXT DEFAULT ''`)
	// FIX (setup-simplification audit): used to read RELAYER_ADDRESS
	// directly with no fallback, writing an EMPTY signing_address whenever
	// an operator set RELAYER_PRIVATE_KEY + NODE_OPERATOR_WALLET but not
	// the (now optional) RELAYER_ADDRESS — exactly the simplified setup
	// this audit pass aims for. An empty signing_address never matches this
	// node's real block proposer address, so IncrementBlockCount credits
	// zero rows for every block this operator produces: correctly
	// authorized to validate, silently never paid. relayerAddressFromEnv
	// (block.go) derives the same address from RELAYER_PRIVATE_KEY that
	// already signs this node's blocks, so the common single-key setup
	// works correctly with no separate RELAYER_ADDRESS at all.
	_, err := cs.db.Exec(
		`INSERT INTO registered_nodes (wallet_address, signing_address) VALUES ($1, $2) ON CONFLICT (wallet_address) DO UPDATE SET signing_address = EXCLUDED.signing_address`,
		wallet, relayerAddressFromEnv(),
	)
	if err != nil {
		fmt.Printf("[NODE] Warning: could not register node wallet %s: %v\n", wallet, err)
	} else {
		fmt.Printf("[NODE] ✓ Node operator wallet registered: %s\n", wallet)
	}
}

// GetRegisteredNodes returns all node operator wallets currently
// registered in the DB. Used by DistributeValidatorsPool.
func (cs *ChainState) GetRegisteredNodes() []string {
	if cs.db == nil {
		return nil
	}
	rows, err := cs.db.Query(`SELECT wallet_address FROM registered_nodes`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var wallets []string
	for rows.Next() {
		var w string
		rows.Scan(&w)
		wallets = append(wallets, w)
	}
	return wallets
}

// GetValidatorOrdinals returns a stable "Validator #N" label for every
// signing address THIS NODE'S OWN registered_nodes table knows about,
// numbered by THIS NODE'S local registration order (registered_at ASC) —
// 1-indexed, keyed by lower-cased signing_address (the same address that
// appears as a block's Proposer field). Used by the explorer to show a
// friendly label alongside a raw 0x address without hardcoding any
// specific node's identity — a deliberate design choice: this project
// explicitly invites any registered human to run a validator node, so
// baking in "Primary/Contabo 1/Contabo 2"-style names would need updating
// by hand every time a new node joins and would misrepresent the network
// as closed.
//
// Known limitation (launch-night finding, 2026-07-21): registered_nodes is
// populated ONLY by each node's own RegisterNode(NODE_OPERATOR_WALLET) call
// for ITSELF at startup — there is no cross-node sync for this table, unlike
// validator_keys/validator_slots. Two different nodes can therefore compute
// two different ordinals for the same address, purely from each one's own
// local registration history. handleValidatorLabels (api.go) layers the
// operator-configured VALIDATOR_LABELS override on top of this function's
// result specifically to fix that: setting the SAME override string
// identically on every node (the same trust model as
// KNIGHTDAG_ACTIVATION_HEIGHT) is what actually guarantees identical labels
// fleet-wide, including "Primary" for the deployment's trusted-seed node —
// this function alone cannot express that role at all, since IS_PRIMARY_NODE
// is per-process knowledge with no existing cross-node signal.
//
// FIX (2026-07-05 — website audit / UX finding): the block explorer only
// ever showed raw hex addresses for the proposer column, which is exactly
// correct but not very approachable for a non-technical visitor trying to
// understand "who produced this block" at a glance.
func (cs *ChainState) GetValidatorOrdinals() map[string]int {
	if cs.db == nil {
		return nil
	}
	rows, err := cs.db.Query(`SELECT lower(signing_address) FROM registered_nodes
	          WHERE signing_address != '' ORDER BY registered_at ASC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	ordinals := make(map[string]int)
	n := 0
	for rows.Next() {
		var addr string
		if err := rows.Scan(&addr); err != nil {
			continue
		}
		if _, exists := ordinals[addr]; exists {
			continue // a signing address can legitimately appear more than once (re-registration)
		}
		n++
		ordinals[addr] = n
	}
	return ordinals
}

// IncrementBlockCount records that the given proposer wallet produced a
// block. Used by distributeValidatorsPoolLocked to distribute rewards
// proportionally. Called for EVERY accepted block (own AND peer-produced —
// see block.go's two call sites) so this node's blocks_produced table
// reflects every validator's actual production, not just its own.
func (cs *ChainState) IncrementBlockCount(proposerAddr string) {
	if cs.db == nil || proposerAddr == "" {
		return
	}
	proposerAddr = strings.ToLower(proposerAddr)
	res, err := cs.db.Exec(`UPDATE registered_nodes SET blocks_produced = blocks_produced + 1 WHERE lower(signing_address) = lower($1)`, proposerAddr)
	if err != nil {
		fmt.Printf("[BLOCKCOUNT] Warning: could not increment block count for %s: %v\n", proposerAddr, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := cs.db.Exec(`UPDATE registered_nodes SET blocks_produced = blocks_produced + 1 WHERE lower(wallet_address) = lower($1)`, proposerAddr); err != nil {
			fmt.Printf("[BLOCKCOUNT] Warning: could not increment block count (wallet fallback) for %s: %v\n", proposerAddr, err)
		}
	}
}

// DistributionShare is one recipient's actual credited amount from a pool
// distribution (validator or LP rewards) — returned so the caller can
// build exactly-replayable TXs from the REAL result, instead of having
// secondaries try to recompute shares themselves from inputs (like
// registered_nodes.blocks_produced) that could differ slightly node to
// node and produce a different split.
type DistributionShare struct {
	Wallet        string
	Amount        float64
	DemurrageLost float64
	// LPSharesBurned/TUsdConverted are set only by checkAndMoveToEscrowLocked
	// when a wallet being swept into escrow held wealth as LP shares or tUSD
	// rather than liquid AEQ — see that function's comment for why these must
	// be liquidated into Amount before escrowing, and carried here so the
	// caller can replay the identical liquidation on secondary nodes.
	LPSharesBurned float64
	TUsdConverted  float64
}

// DistributeValidatorsPool credits registered node operators proportional
// to blocks produced and returns exactly what was credited to each — see
// DistributionShare's comment for why the caller must use these returned
// values (not recompute them) when building replay TXs.
//
// This public wrapper locks cs.mu itself and is kept for direct callers
// (currently only tests) outside the atomic distribution path — production
// distribution goes through RunDailyDistributionAtomic →
// distributeValidatorsPoolLocked, which assumes cs.mu is already held by
// the caller so it can run inside the SAME DB transaction as the rest of
// the round (see audit3, P0 #3).
func (cs *ChainState) DistributeValidatorsPool() []DistributionShare {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox/
	// runAtomicDistributionWithOutbox — see RegisterHuman's comment.
	shares, err := cs.distributeValidatorsPoolLocked(context.Background())
	if err != nil {
		fmt.Printf("[VALIDATORS] Error: %v\n", err)
		return nil
	}
	return shares
}

func (cs *ChainState) distributeValidatorsPoolLocked(ctx context.Context) ([]DistributionShare, error) {
	// GetRegisteredNodes/the blocks_produced query only read PostgreSQL, not
	// cs.accounts — safe to run while cs.mu is held (no deadlock risk; the
	// original "before acquiring cs.mu" ordering predates this function being
	// called from inside an already-locked scope and is no longer required
	// for correctness, only kept historically reachable via the public
	// DistributeValidatorsPool wrapper above where cs.mu is also already held
	// by the time this runs).
	nodes := cs.GetRegisteredNodes()
	if len(nodes) == 0 {
		fmt.Println("[VALIDATORS] No registered node operators — pool left untouched")
		return nil, nil
	}

	type nodeShare struct {
		wallet string
		blocks int64
	}
	var nodeShares []nodeShare
	var totalBlocks int64
	if cs.db != nil {
		rows, _ := cs.dbExecCtx(ctx).Query(`SELECT wallet_address, blocks_produced FROM registered_nodes WHERE wallet_address = ANY($1)`, pq.Array(nodes))
		if rows != nil {
			for rows.Next() {
				var w string
				var b int64
				rows.Scan(&w, &b)
				if b == 0 {
					b = 1
				} // minimum weight so new nodes still get something
				nodeShares = append(nodeShares, nodeShare{w, b})
				totalBlocks += b
			}
			rows.Close()
		}
	}
	if len(nodeShares) == 0 {
		for _, w := range nodes {
			nodeShares = append(nodeShares, nodeShare{w, 1})
			totalBlocks++
		}
	}

	// FIX (Monster Audit 2026-07-12, P1): a pool address that fell out of (or
	// never entered) the in-memory cache used to read as "not present" here,
	// which this function treated identically to "genuinely empty" — silently
	// skipping the ENTIRE day's distribution of a real, non-zero DB balance.
	// ensureAccountLoaded is a no-op once the address is already cached.
	cs.ensureAccountLoadedCtx(ctx, validatorsPoolAddr)
	poolAcc, ok := cs.accounts.Get(validatorsPoolAddr)
	if !ok || poolAcc.Balance <= 0 {
		fmt.Println("[VALIDATORS] Pool is empty — nothing to distribute today")
		return nil, nil
	}

	total := poolAcc.Balance.Float()
	// FIX (Monster Audit follow-up, 2026-07-12, P0): distributeLPPoolLocked/
	// distributeUBIPoolLocked (both fixed in an earlier 2026-07-05/06 audit
	// pass) warm every recipient via ensureAccountsLoaded before crediting —
	// this function warmed only the pool address itself (see the FIX comment
	// above) and never got the equivalent call for individual validator/
	// node-operator wallets, which can genuinely lack a chain_accounts row
	// touch independent of registration (RegisterNode/BindValidatorSlot write
	// straight into registered_nodes). A cold recipient wallet below used to
	// be blind-created as a fresh zero-balance AccountState, silently wiping
	// any real balance/tusd/lp/is_human it already had via saveAccountToDB's
	// Version==0 upsert.
	walletAddrs := make([]string, 0, len(nodeShares))
	for _, ns := range nodeShares {
		walletAddrs = append(walletAddrs, ns.wallet)
	}
	cs.ensureAccountsLoadedCtx(ctx, walletAddrs)
	// P0-2: credit recipients BEFORE zeroing the pool so a crash mid-loop
	// leaves money in the pool (re-distributable) rather than losing it.
	var totalDistributed float64
	var shares []DistributionShare
	for _, ns := range nodeShares {
		wallet := ns.wallet
		// P2-FIX: validate wallet address before crediting — a malformed
		// entry in registered_nodes would insert a garbage key into cs.accounts.
		if len(wallet) != 42 || wallet[:2] != "0x" {
			fmt.Printf("[VALIDATORS] Skipping invalid wallet address: %q\n", wallet)
			continue
		}
		share := round6(total * float64(ns.blocks) / float64(totalBlocks))
		if share <= 0 {
			continue
		} // E4-FIX: skip rounding-to-zero to preserve pool
		acc, ok := cs.accounts.Get(wallet)
		if !ok {
			acc = &AccountState{Address: wallet}
			cs.accounts.Set(wallet, acc)
		}
		lost, err := cs.settleDemurrageLockedCtx(ctx, acc)
		if err != nil {
			return nil, fmt.Errorf("could not settle demurrage for %s: %w", wallet, err)
		}
		acc.Balance = acc.Balance.Add(NewDecimal(share))
		touchActivity(acc)
		if err := cs.enforceWealthCapLockedCtx(ctx, acc); err != nil {
			return nil, fmt.Errorf("could not enforce wealth cap for %s: %w", wallet, err)
		}
		if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
			return nil, fmt.Errorf("could not save validator reward for %s: %w", wallet, err)
		}
		totalDistributed += share
		shares = append(shares, DistributionShare{Wallet: wallet, Amount: share, DemurrageLost: lost.Float()})
	}
	// Zero pool only after all recipients are successfully written,
	// and only if something was actually distributed (prevents destroying
	// pool balance when all shares rounded to zero).
	if totalDistributed > 0 {
		poolAcc.Balance = NewDecimal(0)
		if err := cs.saveAccountToDBCtx(ctx, poolAcc); err != nil {
			return nil, fmt.Errorf("could not zero validators pool: %w", err)
		}
	}
	cs.save()

	cs.syncBalanceLocked(V7_CONTRACT_ADDR, append(nodes, validatorsPoolAddr)...)
	fmt.Printf("[VALIDATORS] Distributed %.6f AEQ proportionally (%d nodes, block-weighted)\n", total, len(nodeShares))
	return shares, nil
}

// DistributeLPPool pays out the entire LP pool balance to liquidity
// providers, proportional to their LP share count. This mirrors how
// real AMMs (Uniswap v2, etc.) reward LPs — the more of the pool you
// provided, the larger your share of the fee income. Accounts with zero
// LP shares receive nothing. Returns exactly what was credited to each
// holder — see DistributionShare's comment for why the caller must use
// these returned values when building replay TXs.
//
// Public wrapper kept for direct callers (tests) outside the atomic
// distribution path — see DistributeValidatorsPool's comment for why
// production distribution uses distributeLPPoolLocked instead.
func (cs *ChainState) DistributeLPPool() []DistributionShare {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox/
	// runAtomicDistributionWithOutbox — see RegisterHuman's comment.
	shares, err := cs.distributeLPPoolLocked(context.Background())
	if err != nil {
		fmt.Printf("[LP] Error: %v\n", err)
		return nil
	}
	return shares
}

func (cs *ChainState) distributeLPPoolLocked(ctx context.Context) ([]DistributionShare, error) {
	// Collect all LP holders and their share counts BEFORE settling demurrage,
	// so we know who participates.
	//
	// FIX (P3, beta-launch audit 2026-07-05): this used to only iterate
	// cs.accounts (the in-memory cache) — an LP holder whose account had
	// been evicted (or never loaded) beyond maxInMemAccounts was silently
	// skipped, never receiving their share and permanently understating
	// totalShares. Query the DB directly instead, same fix already applied
	// to distributeUBIPoolLocked — see idx_chain_accounts_lp_shares.
	type lpHolder struct {
		addr   string
		shares float64
	}
	var holders []lpHolder
	totalShares := 0.0
	if cs.db != nil {
		// FIX (deadlock, same as ensureAccountLoaded's FIX comment):
		// distributeLPPoolLocked runs inside RunDailyDistributionAtomic's
		// runAtomicDistributionWithOutbox critical section (cs.mu held,
		// cs.activeTx set) — route through cs.dbExec() so this enumeration
		// reuses that transaction's own connection.
		rows, err := cs.dbExecCtx(ctx).Query(`SELECT lower(address) FROM chain_accounts WHERE lp_shares > 0`)
		if err != nil {
			return nil, fmt.Errorf("could not enumerate LP holders: %w", err)
		}
		var addrs []string
		for rows.Next() {
			var addr string
			rows.Scan(&addr)
			if addr != "" {
				addrs = append(addrs, addr)
			}
		}
		rows.Close()
		cs.ensureAccountsLoadedCtx(ctx, addrs) // page in cold accounts so LP distribution works beyond the in-memory cap
		for _, addr := range addrs {
			if acc, ok := cs.accounts.Get(addr); ok && acc.LPShares.Float() > 0 {
				holders = append(holders, lpHolder{addr, acc.LPShares.Float()})
				totalShares += acc.LPShares.Float()
			}
		}
	} else {
		// No DB (unit tests): fall back to in-memory iteration.
		cs.accounts.Range(func(addr string, acc *AccountState) bool {
			if acc.LPShares > 0 {
				holders = append(holders, lpHolder{addr, acc.LPShares.Float()})
				totalShares += acc.LPShares.Float()
			}
			return true
		})
	}
	if totalShares <= 0 || len(holders) == 0 {
		fmt.Println("[LP] No LP holders — pool left untouched")
		return nil, nil
	}

	// E3 fix: settle demurrage for ALL LP holders FIRST. settleDemurrageLocked
	// credits demurrage fees to pool addresses (including lpPoolAddr), so the
	// pool balance may increase during this loop. Reading poolAcc.Balance before
	// this loop would miss those newly-credited fees, and zeroing the pool at
	// the end would then destroy them.
	//
	// FIX (audit recheck 2, P0 #6): capture each holder's loss here so it can
	// be attached to their DistributionShare below — secondaries replaying
	// lp_distribution via ApplyLPRewardDelta need the EXACT same loss applied
	// (not recomputed, which could differ from a node whose LastActivityAt
	// view of this wallet has drifted) or their balance permanently diverges
	// from the primary's by however much demurrage each holder had accrued.
	demurrageLost := make(map[string]float64, len(holders))
	for _, h := range holders {
		acc, _ := cs.accounts.Get(h.addr)
		lost, err := cs.settleDemurrageLockedCtx(ctx, acc)
		if err != nil {
			return nil, fmt.Errorf("could not settle demurrage for %s: %w", h.addr, err)
		}
		demurrageLost[h.addr] = lost.Float()
	}
	// Re-check totalShares after demurrage settlement — shares could have gone to zero.
	if totalShares <= 0 {
		return nil, nil
	}

	// P2-FIX: second totalShares guard after the demurrage loop. Recompute from
	// live account LPShares values so any unexpected collapse is caught here,
	// preventing division by zero in the distribution loop below.
	totalShares = 0
	for _, h := range holders {
		if acc, ok := cs.accounts.Get(h.addr); ok {
			totalShares += acc.LPShares.Float()
		}
	}
	if totalShares <= 0 {
		fmt.Println("[LP] totalShares collapsed to zero after demurrage loop — pool left untouched")
		return nil, nil
	}

	// NOW read the pool balance — it includes any demurrage credits just added.
	// FIX (Monster Audit 2026-07-12, P1): see DistributeValidatorsPool's
	// comment — a cold pool address must not read as "empty".
	cs.ensureAccountLoadedCtx(ctx, lpPoolAddr)
	poolAcc, ok := cs.accounts.Get(lpPoolAddr)
	if !ok || poolAcc.Balance <= 0 {
		fmt.Println("[LP] Pool is empty — nothing to distribute today")
		return nil, nil
	}

	total := poolAcc.Balance.Float()
	// P0-2: credit holders BEFORE zeroing pool — crash-safe ordering.
	// E4 fix: track total distributed so we don't zero the pool if all shares
	// rounded to zero (which would destroy micro-AEQ silently).
	var totalDistributed float64
	var shares []DistributionShare
	for _, h := range holders {
		share := round6((h.shares / totalShares) * total)
		totalDistributed += share
		acc, _ := cs.accounts.Get(h.addr)
		acc.Balance = acc.Balance.Add(NewDecimal(share))
		touchActivity(acc)
		if err := cs.enforceWealthCapLockedCtx(ctx, acc); err != nil {
			return nil, fmt.Errorf("could not enforce wealth cap for %s: %w", h.addr, err)
		}
		if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
			return nil, fmt.Errorf("could not save LP reward for %s: %w", h.addr, err)
		}
		shares = append(shares, DistributionShare{Wallet: h.addr, Amount: share, DemurrageLost: demurrageLost[h.addr]})
	}
	if totalDistributed > 0 {
		poolAcc.Balance = NewDecimal(0)
		if err := cs.saveAccountToDBCtx(ctx, poolAcc); err != nil {
			return nil, fmt.Errorf("could not zero LP pool: %w", err)
		}
	} else {
		fmt.Printf("[LP] All shares rounded to zero (%.9f AEQ total) — pool preserved\n", total)
	}
	cs.save()

	holderAddrs := make([]string, len(holders))
	for i, h := range holders {
		holderAddrs[i] = h.addr
	}
	cs.syncBalanceLocked(V7_CONTRACT_ADDR, append(holderAddrs, lpPoolAddr)...)

	fmt.Printf("[LP] ✓ Distributed %.6f AEQ across %d LP holders (proportional to shares)\n", total, len(holders))
	return shares, nil
}

// DistributeUBIPool distributes the UBI pool equally across every
// registered human and returns exactly what was credited to each —
// including each human's individual demurrage loss, settled in the same
// pass — so the caller (main.go) can build per-human "ubi_distribution"
// TXs for secondaries to replay, rather than reading the pool balance
// separately beforehand or broadcasting a single flat amount.
//
// FIX (audit recheck 2, P0 #5): this used to return a flat
// (amountPerHuman, totalHumans) pair, broadcast as ONE TX that every
// secondary applied via ApplyUBIDelta — crediting amountPerHuman to every
// human, but never replaying the demurrage settlement below. On the
// primary, settleDemurrageLocked reduces each human's balance AND credits
// the pool BEFORE the equal split is computed; a human with zero accrued
// demurrage and one with significant accrued demurrage both received the
// exact same broadcast credit, but only the primary's own in-memory state
// reflected the (different, per-human) loss each of them took first. Any
// human with nonzero accrued demurrage at UBI time caused permanent
// StateRoot divergence. Now returns one DistributionShare per human with
// that human's own DemurrageLost, exactly like DistributeLPPool/
// DistributeValidatorsPool already did — main.go emits one TX per human
// instead of one flat broadcast TX.
// Public wrapper kept for direct callers (tests) outside the atomic
// distribution path — see DistributeValidatorsPool's comment for why
// production distribution uses distributeUBIPoolLocked instead.
func (cs *ChainState) DistributeUBIPool() []DistributionShare {
	cs.mu.Lock()
	acquired := time.Now()
	defer trackExclusiveHold(acquired, "UBI distribution")
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox/
	// runAtomicDistributionWithOutbox — see RegisterHuman's comment.
	shares, err := cs.distributeUBIPoolLocked(context.Background())
	if err != nil {
		fmt.Printf("[UBI] Error: %v\n", err)
		return nil
	}
	return shares
}

func (cs *ChainState) distributeUBIPoolLocked(ctx context.Context) ([]DistributionShare, error) {
	// FIX (Monster Audit 2026-07-12, P1): see DistributeValidatorsPool's
	// comment — a cold pool address must not read as "empty". Loaded once
	// here; the second read below (after the demurrage loop) reuses the same
	// now-cached map entry, so it needs no separate call.
	cs.ensureAccountLoadedCtx(ctx, ubiPoolAddr)
	poolAcc, ok := cs.accounts.Get(ubiPoolAddr)
	if !ok || poolAcc.Balance <= 0 {
		fmt.Println("[UBI] Pool is empty — nothing to distribute today")
		return nil, nil
	}

	// Query DB directly so UBI works correctly at 8B humans regardless of
	// whether their accounts are currently in the in-memory cache.
	// The partial index idx_chain_accounts_is_human makes this fast even at
	// very large account counts. Batch into memory in chunks to avoid a
	// single allocation of 8B addresses.
	var humanAddrs []string
	if cs.db != nil {
		// FIX (deadlock, same as ensureAccountLoaded's FIX comment):
		// distributeUBIPoolLocked runs inside RunDailyDistributionAtomic's
		// runAtomicDistributionWithOutbox critical section — route through
		// cs.dbExec() so this enumeration reuses that transaction's own
		// connection instead of requesting a fresh one from the pool.
		rows, err := cs.dbExecCtx(ctx).Query(`SELECT lower(address) FROM chain_accounts WHERE is_human = true`)
		if err != nil {
			return nil, fmt.Errorf("could not enumerate human accounts: %w", err)
		}
		for rows.Next() {
			var addr string
			rows.Scan(&addr)
			if addr != "" {
				humanAddrs = append(humanAddrs, addr)
			}
		}
		rows.Close()
	} else {
		// No DB (unit tests): fall back to in-memory iteration.
		cs.accounts.Range(func(addr string, acc *AccountState) bool {
			if acc.IsHuman {
				humanAddrs = append(humanAddrs, addr)
			}
			return true
		})
	}
	if len(humanAddrs) == 0 {
		fmt.Println("[UBI] No registered humans yet — pool left untouched")
		return nil, nil
	}
	// Ensure all human accounts are in the cache so the distribution loop
	// below can work on in-memory objects (reads + writes stay coherent).
	cs.ensureAccountsLoadedCtx(ctx, humanAddrs)

	// E3-FIX for UBI: settle demurrage for ALL humans FIRST. settleDemurrageLocked
	// credits 20% of each human's decay to ubiPoolAddr. Reading the pool balance
	// BEFORE this loop would miss those credits; zeroing AFTER distributes them.
	// Same fix applied to DistributeLPPool. Capture each human's own loss for
	// the returned DistributionShare — see the function comment above.
	demurrageLost := make(map[string]float64, len(humanAddrs))
	for _, addr := range humanAddrs {
		addrAcc, _ := cs.accounts.Get(addr)
		lost, err := cs.settleDemurrageLockedCtx(ctx, addrAcc)
		if err != nil {
			return nil, fmt.Errorf("could not settle demurrage for %s: %w", addr, err)
		}
		demurrageLost[addr] = lost.Float()
	}
	// NOW read pool balance — includes any demurrage credits just added.
	poolAcc, ok = cs.accounts.Get(ubiPoolAddr)
	if !ok || poolAcc.Balance <= 0 {
		fmt.Println("[UBI] Pool empty after demurrage settlement — nothing to distribute")
		return nil, nil
	}
	// P0-FIX: Do NOT call settleDemurrageLocked on the pool account itself —
	// pool addresses are tokenomics infrastructure and must never have demurrage applied.
	total := poolAcc.Balance.Float()
	share := total / float64(len(humanAddrs))
	// P0-5/P2-9: prevent funds vanishing via float rounding to 0
	if round6(share) == 0 {
		fmt.Printf("[UBI] Share %.10f rounds to zero — pool left intact for next distribution\n", share)
		return nil, nil
	}
	// P0-2 + P1-6: credit humans BEFORE zeroing pool AND before last_ubi_at.
	// Perf (scale roadmap 2026-07-21): mutate every account in memory first,
	// then persist the whole set via ONE saveAccountsToDBBatch call instead
	// of a per-account round trip — at 10k+ humans the per-row version held
	// cs.mu for minutes once a day. enforceWealthCapLocked only mutates the
	// account in memory and persists pool credits (never the account row
	// itself), so deferring the row writes to the batch changes nothing
	// semantically; the wrapping RunDailyDistributionAtomic transaction
	// commits or rolls back the batch and the outbox TXs together either way.
	shares := make([]DistributionShare, 0, len(humanAddrs))
	batch := make([]*AccountState, 0, len(humanAddrs))
	for _, addr := range humanAddrs {
		acc, _ := cs.accounts.Get(addr)
		acc.Balance = acc.Balance.Add(NewDecimal(share))
		touchActivity(acc)
		if err := cs.enforceWealthCapLockedCtx(ctx, acc); err != nil {
			return nil, fmt.Errorf("could not enforce wealth cap for %s: %w", addr, err)
		}
		batch = append(batch, acc)
		shares = append(shares, DistributionShare{Wallet: addr, Amount: round6(share), DemurrageLost: demurrageLost[addr]})
	}
	if err := cs.saveAccountsToDBBatchCtx(ctx, batch); err != nil {
		return nil, fmt.Errorf("could not save UBI rewards batch: %w", err)
	}
	poolAcc.Balance = NewDecimal(0)
	if err := cs.saveAccountToDBCtx(ctx, poolAcc); err != nil {
		return nil, fmt.Errorf("could not zero UBI pool: %w", err)
	}
	cs.save()
	// FIX (audit recheck 2, P0 #4): last_ubi_at used to be set HERE via
	// time.Now() — a different instant than whatever secondaries later
	// replayed (block.Timestamp, assigned whenever ProduceBlock's ticker
	// next fired). The caller (main.go) now finalizes via
	// ApplyUBIFinalizeDelta with a single explicit timestamp shared by the
	// primary's own state and the TX every secondary replays.
	cs.syncBalanceLocked(V7_CONTRACT_ADDR, append(humanAddrs, ubiPoolAddr)...)

	fmt.Printf("[UBI] ✓ Distributed %.6f AEQ across %d registered humans (%.6f AEQ each)\n",
		total, len(humanAddrs), share)
	capturedGini := cs.calcGiniLocked()
	capturedHumans := len(humanAddrs)
	SafeGoroutine("SaveGiniSnapshotValues", func() { cs.SaveGiniSnapshotValues(capturedGini, capturedHumans) })
	return shares, nil
}

// getAverageBalanceLocked computes the mean AEQ balance across every
// registered human (using each account's live, demurrage-adjusted
// balance, not the raw stored value, since that's the real current
// wealth distribution). Non-human accounts (the four fee-pool addresses,
// any unregistered address that merely received a transfer) are excluded
// — the cap is about wealth among the humans the system actually exists
// for, not diluted by infrastructure accounts. Caller must hold cs.mu.
func (cs *ChainState) getAverageBalanceLocked() float64 {
	// Use TotalSupply / humans (= 1000 AEQ always) rather than averaging
	// wallet balances. AEQ deposited into the AMM pool lives in cs.pool.ReserveAEQ
	// — NOT in any human's cs.accounts entry — so wallet-sum / humans gives a
	// misleadingly low number (e.g. 960 when 40 AEQ/human is in the pool).
	// The protocol invariant TotalSupply = humans × 1000 makes the fair-share
	// average exactly 1000 AEQ regardless of where those AEQ currently sit.
	// humanCountLocked (see its own comment) replaces the old cs.accounts
	// scan here too — same undercounts-at-scale bug accountSetXOR already
	// had fixed once.
	if cs.humanCountLocked() == 0 {
		return 0
	}
	return 1000.0 // TotalSupply / humans = humans×1000 / humans = 1000 AEQ
}

// enforceWealthCapLocked checks acc's balance against the current
// wealth cap (wealthCapMultiplier * average human balance) and, if it's
// over, skims the excess into the four tokenomics pools — the same
// 40/30/20/10 split used for swap fees and demurrage. This is called
// after AEQ arrives in an account (registration, receiving a transfer,
// a tusd->aeq swap, or removing liquidity), never on amounts already
// sitting in a balance from before the cap existed or before the
// average rose — so it can only ever trim genuinely NEW incoming AEQ
// down to the cap, not retroactively confiscate existing savings.
// Caller must hold cs.mu.
// isTokenomicsPoolAddress reports whether addr is one of the four
// official fee-recipient addresses (validators/LPs/UBI/treasury). These
// are deliberately exempt from the wealth cap — their entire purpose is
// to accumulate fees/demurrage/cap-overflow from everyone else, so
// capping them would be self-defeating. Every other address, registered
// human or not, is subject to the cap.
func isTokenomicsPoolAddress(addr string) bool {
	switch addr {
	case validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr:
		return true
	}
	return false
}

// bootstrapMultiplierLocked returns the effective wealth cap multiplier.
// During bootstrap (< 25 registered humans) the multiplier scales with the
// human count — max(5, min(N, 25)) — so early joiners cannot accumulate
// 25,000 AEQ before meaningful participation exists. At 25+ humans the
// full wealthCapMultiplier (25×) applies permanently. Caller must hold cs.mu.
//
// FIX (2026-07-23, 50k-TPS-goal investigation): used to count IsHuman
// accounts via its own cs.accounts.Range() scan instead of reusing
// humanCountLocked() -- the EXACT "undercounts at scale" bug class
// getAverageBalanceLocked's own comment (right above this function)
// already describes fixing for itself: a Range() scan only sees accounts
// currently WARM in memory, silently undercounting once the account set
// exceeds maxInMemAccounts and cold accounts stop being preloaded, while
// humanCountLocked (in cs.useDB mode) is the real, incrementally-maintained
// global count. This was also a real, measured hot-path cost: this
// function is called on the WAL fast path's every single transfer via
// wealthCapAmountLocked, and shardedAccounts.Range() unconditionally
// iterates every one of numAccountShards shards (locking and unlocking
// each), regardless of how many are actually populated -- confirmed live
// via CPU profiling to be 57% of total CPU time once numAccountShards was
// raised to 16384 for an unrelated fix (accumulated benchmark account
// count made the effect large enough to dominate). humanCountLocked is
// O(1) in cs.useDB mode (a cached counter), matching this function's
// sibling getAverageBalanceLocked exactly.
func (cs *ChainState) bootstrapMultiplierLocked() float64 {
	count := cs.humanCountLocked()
	if count >= 25 {
		return wealthCapMultiplier
	}
	m := float64(count)
	if m < 5.0 {
		m = 5.0
	}
	return m
}

// enforceWealthCapLocked caps acc's balance and redistributes the excess to
// the pools. Returns an error (audit fresh-pass finding, 2026-06-30) instead
// of just logging one: it used to deduct the excess from acc.Balance FIRST
// and only THEN try to credit the pools via distributeSwapFee — if that
// credit failed (e.g. an optimistic-locking version conflict on a pool
// address, which doesn't poison the enclosing DB transaction the way a hard
// SQL error would, so runAtomicWithOutbox's eventual Commit would still
// succeed), the deducted AEQ vanished: not in the user's balance, not in any
// pool, with only a printf to show for it. Every call site already runs
// inside runAtomicWithOutbox (directly or via its caller), so returning the
// error here lets it reach that transaction's fn() and trigger a real
// rollback — the same fix class already applied to ApplyUBIDelta,
// transferLocked, etc. (see their "audit recheck2, P0 #3" comments).
func (cs *ChainState) enforceWealthCapLocked(acc *AccountState) error {
	return cs.enforceWealthCapLockedCtx(context.Background(), acc)
}

// enforceWealthCapLockedCtx is enforceWealthCapLocked's real implementation
// — see dbExecCtx's comment for the migration this is part of.
func (cs *ChainState) enforceWealthCapLockedCtx(ctx context.Context, acc *AccountState) error {
	if isTokenomicsPoolAddress(acc.Address) {
		return nil
	}
	// Deliberately NOT gated on acc.IsHuman: capping only registered
	// humans would let someone bypass the entire mechanism just by
	// parking AEQ in any ordinary, unregistered address (a personal
	// "overflow wallet" they also control) — that address would have
	// accumulated unlimited AEQ with no cap ever applying to it. The cap
	// has to apply to anyone receiving AEQ, registered or not, for it to
	// mean anything.
	avg := cs.getAverageBalanceLocked()
	if avg <= 0 {
		return nil // no meaningful average yet (e.g. only one human registered so far)
	}
	multiplier := cs.bootstrapMultiplierLocked()
	wealthCapAmt := avg * multiplier
	if acc.Balance.Float() <= wealthCapAmt {
		return nil
	}
	excess := acc.Balance.Float() - wealthCapAmt
	acc.Balance = NewDecimal(wealthCapAmt)
	if err := cs.distributeSwapFeeCtx(ctx, excess, true); err != nil {
		return fmt.Errorf("wealth cap: could not persist pool credits for %s excess: %w", acc.Address, err)
	}
	fmt.Printf("[WEALTH CAP] %s exceeded %.2fx average (%.2f AEQ) — %.4f AEQ excess redistributed to pools\n",
		acc.Address, multiplier, wealthCapAmt, excess)
	return nil
}

// DemurrageStatus describes whether/when an idle account's AEQ will
// start (or has started) decaying, for surfacing to the user at login.
type DemurrageStatus struct {
	Active                bool    `json:"active"`                   // true if decay has already started
	DaysUntilStart        float64 `json:"days_until_start"`         // only meaningful if !Active; can be negative-free, always >= 0
	ShowFourteenDayNotice bool    `json:"show_fourteen_day_notice"` // one-time notice, true only on the call that first crosses into the 14-day window
	ShowSevenDayNotice    bool    `json:"show_seven_day_notice"`    // true on every check within the last 7 days before decay starts
}

// GetDemurrageStatus reports where address stands relative to the
// demurrage grace period, and — like settleDemurrageLocked — has a side
// effect: the first time this is called once the account has entered the
// 14-day warning window, it flips Demurrage14DayWarningShown so the
// one-time notice isn't repeated on every subsequent login within that
// same window. The 7-day notice has no such one-time flag; per Daniel's
// spec, that one is meant to repeat on every login during its window.
func (cs *ChainState) GetDemurrageStatus(address string) DemurrageStatus {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	address = strings.ToLower(address)
	cs.ensureAccountLoaded(address) // cold accounts must report their real demurrage status, not the grace-period default

	acc, ok := cs.accounts.Get(address)
	if !ok || acc.LastActivityAt == 0 {
		return DemurrageStatus{Active: false, DaysUntilStart: float64(demurrageGracePeriodSeconds) / 86400}
	}

	idleSeconds := nowUnix() - acc.LastActivityAt
	secondsUntilStart := demurrageGracePeriodSeconds - idleSeconds
	if secondsUntilStart <= 0 {
		return DemurrageStatus{Active: true}
	}

	daysUntilStart := float64(secondsUntilStart) / 86400
	status := DemurrageStatus{Active: false, DaysUntilStart: daysUntilStart}

	if daysUntilStart <= 7 {
		status.ShowSevenDayNotice = true
	} else if daysUntilStart <= 14 {
		if !acc.Demurrage14DayWarningShown {
			status.ShowFourteenDayNotice = true
			// P1-5: set in-memory flag SYNCHRONOUSLY to prevent duplicate notices on parallel requests.
			// DB write is async to avoid blocking the GET path.
			acc.Demurrage14DayWarningShown = true
			// FIX (audit 2026-06-29): this used to call cs.saveAccountToDB,
			// which internally calls cs.dbExec() — reading cs.activeTx. That
			// read is only synchronized by cs.mu (see activeTx's own field
			// comment), but this goroutine runs detached, after the caller's
			// deferred cs.mu.Unlock() has very likely already fired — a
			// genuine, unsynchronized data race against whatever atomic
			// operation (runAtomicWithOutbox/runAtomicDistributionWithOutbox)
			// happens to be setting/clearing cs.activeTx concurrently. Worse
			// than just a race: if it happened to observe a DIFFERENT,
			// unrelated in-flight transaction, this write could silently
			// become part of that operation and get rolled back with it (or
			// committed as a side effect of it) — breaking the isolation the
			// whole activeTx mechanism exists to provide. This field isn't
			// StateRoot-relevant (losing the write just means the notice can
			// show once more), so there's no need to participate in any
			// transaction at all: write straight to cs.db, never cs.activeTx.
			addr := acc.Address
			SafeGoroutine("persist-14day-demurrage-notice", func() {
				if cs.db == nil {
					return
				}
				if _, err := cs.db.Exec(`UPDATE chain_accounts SET demurrage_14_day_warning_shown = true WHERE address = $1`, addr); err != nil {
					fmt.Printf("[DB] Warning: could not persist 14-day demurrage notice flag for %s: %v\n", addr, err)
				}
			})
		}
	}

	return status
}

func (cs *ChainState) GetTUsdBalance(address string) float64 {
	var out float64
	cs.readAccount(address, func(acc *AccountState) {
		out = acc.TUsdBalance.Float()
	})
	return out
}

func (cs *ChainState) GetPoolReserves() (float64, float64) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.pool == nil {
		return 0, 0
	}
	return cs.pool.ReserveAEQ.Float(), cs.pool.ReserveTUSD.Float()
}

// GetPoolSnapshot returns reserveAEQ, reserveTUSD, and totalLPShares in a
// single lock acquisition — so a caller iterating many accounts can compute
// each account's withdrawable LP value without taking the pool lock per row.
func (cs *ChainState) GetPoolSnapshot() (reserveAEQ, reserveTUSD, totalLPShares float64) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.pool == nil {
		return 0, 0, 0
	}
	return cs.pool.ReserveAEQ.Float(), cs.pool.ReserveTUSD.Float(), cs.pool.TotalLPShares.Float()
}

func (cs *ChainState) IsHuman(address string) bool {
	var out bool
	cs.readAccount(address, func(acc *AccountState) {
		out = acc.IsHuman
	})
	return out
}

func (cs *ChainState) RegisterHuman(address string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox, so there is no
	// active transaction to join — context.Background() is correct, not a
	// placeholder. See dbExecCtx's comment for the migration this is part of.
	return cs.registerHumanLocked(context.Background(), address, 0)
}

// RegisterHumanAtomic behaves like RegisterHuman, except the state
// mutation, the nullifier claim, and the resulting outbox insert commit or
// roll back together as one DB transaction — see TransferAtomic's comment
// / runAtomicWithOutbox. pendingTxTemplate should have every field already
// set (RegisterHuman's result carries no extra fields the way Transfer's
// demurrage amounts do, so there's nothing to fill in after the fact
// here).
//
// FIX (audit recheck 2, P1 #7/#10): SaveNullifier used to be called by
// register.go as a separate, non-atomic step AFTER this function's
// transaction had already committed — see SaveNullifier's comment for the
// permanent-StateRoot-mismatch consequence that had. It's now called HERE,
// inside fn(), while cs.activeTx is set, so it participates in the exact
// same commit-or-rollback unit as the account mutation and the outbox
// insert.
//
// SHARD-LOCKED FAST PATH (SCALING_ARCHITECTURE.md Phase 8, register_concurrent.go
// — NOT staging-validated, see registerHumanConcurrent's own doc comment):
// tried first, ahead of the cs.mu.Lock()-based path below. When applied is
// true the fast path genuinely ran — its result (success or a real error,
// e.g. already registered) is final and returned as-is. Only when applied
// is false (would touch a pool address via the wealth cap, or a real DB
// error while checking cold-address state) does this fall through to the
// slow path, unchanged from before this fast path existed. registerHumanConcurrent
// self-gates on cs.db == nil, so this is a no-op for no-DB nodes.
func (cs *ChainState) RegisterHumanAtomic(address string, pendingTx Transaction) error {
	address = strings.ToLower(address)
	if applied, err := cs.registerHumanConcurrent(address, pendingTx); applied {
		return err
	}
	return cs.runAtomicWithOutbox([]string{address}, false, func(ctx context.Context) (Transaction, error) {
		if err := cs.registerHumanLocked(ctx, address, 0); err != nil {
			return Transaction{}, err
		}
		if pendingTx.Nullifier != "" {
			if err := cs.SaveNullifier(ctx, pendingTx.Nullifier, address); err != nil {
				return Transaction{}, err
			}
		}
		return pendingTx, nil
	})
}

// registerHumanLocked is RegisterHuman's implementation; caller must
// already hold cs.mu — see transferLocked's comment for why this split
// exists. ctx carries the caller's active transaction (if any) — see
// dbExecCtx's comment for the migration this is part of. Block replay
// (block.go) also calls this directly with context.Background(): it sets
// dag.state.activeTx itself before this runs, and dbExecCtx falls back to
// that field when ctx carries no transaction, so behavior there is
// unchanged.
// activityAt is the instant to start the 1,000 AEQ grant's demurrage grace
// period from. This function is shared by ingestion and replay, so callers
// that are applying a BLOCK pass its Timestamp (see touchActivityAt for why a
// replay handler must not read this node's wall clock — a resync replaying a
// years-old registration would otherwise hand it a brand-new grace period).
// Live callers pass 0, which keeps nowUnix().
func (cs *ChainState) registerHumanLocked(ctx context.Context, address string, activityAt int64) error {
	address = strings.ToLower(address)
	cs.ensureAccountLoadedCtx(ctx, address)

	acc, ok := cs.accounts.Get(address)
	if ok && acc.IsHuman {
		return fmt.Errorf("already registered")
	}
	if !ok {
		acc = &AccountState{Address: address}
		cs.accounts.Set(address, acc)
	}

	acc.IsHuman = true
	acc.Balance = acc.Balance.Add(NewDecimal(1000))
	touchActivityAt(acc, activityAt) // starts this 1,000 AEQ's own grace period fresh
	if err := cs.enforceWealthCapLockedCtx(ctx, acc); err != nil {
		return fmt.Errorf("could not enforce wealth cap: %w", err)
	}
	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		return fmt.Errorf("could not save account: %w", err)
	}
	// See humanCount's own field comment: this and registerHumanConcurrent
	// (register_concurrent.go) are the only live mutation paths that can
	// turn a non-human account human (confirmed — the only other
	// cs.accounts[x].IsHuman assignment anywhere in this codebase is
	// loadFromDB's duplicate-case merge, which runs entirely inside the
	// startup scan rebuildStateAccumulators already reseeds from fresh
	// afterward). Only counted after the durable write above succeeds,
	// same ordering accountSetXOR's own update already uses inside
	// saveAccountToDB — never count a registration that didn't persist.
	// humanCountMu: see that field's own comment.
	cs.humanCountMu.Lock()
	cs.humanCount++
	cs.humanCountMu.Unlock()
	cs.save()

	fmt.Printf("[STATE] ✓ Human registered: %s | Balance: %.2f AEQ\n",
		address, acc.Balance.Float())
	// P1-10: run EVM sync synchronously first, then retry in background.
	// Prevents permanent Go/EVM divergence if the first sync fails.
	cs.syncHumanRegistrationLocked(V7_CONTRACT_ADDR, address)
	addr := address
	SafeGoroutine("syncHumanRegistration-retries", func() {
		for attempt := 1; attempt <= 3; attempt++ {
			time.Sleep(time.Duration(attempt) * 3 * time.Second)
			cs.mu.RLock()
			cs.syncHumanRegistrationLocked(V7_CONTRACT_ADDR, addr)
			cs.mu.RUnlock()
			fmt.Printf("[STATE] EVM sync retry %d for %s\n", attempt, addr)
		}
	})
	return nil
}

// runAtomicWithOutbox executes fn (a "Locked" variant of a state-mutation
// function — assumes cs.mu is already held, and persists via
// cs.dbExec()-aware saveAccountToDB/savePoolToDB internally) and queues the
// Transaction it returns to the pending_tx outbox, as a single
// all-or-nothing unit: one DB transaction, with the in-memory mutation
// rolled back too if anything fails. fn returns the exact Transaction to
// enqueue (built from whatever it just computed — e.g. the demurrage-loss
// amounts a transfer settled — rather than a value the caller could have
// supplied upfront, since those fields aren't known until fn runs) together
// with its error; the Transaction is ignored if err != nil. touchedAddrs/
// fullSnapshot are passed straight to snapshotForRollback (see block.go's
// replayTransactions for the exact same pattern used for block-level
// atomicity — this reuses it at the single-operation level).
//
// This exists because, before it, every state-mutating RPC handler called
// its business-logic function (which commits its own DB writes immediately
// on success) and ONLY THEN called SavePendingTx — so a failure in the
// outbox write specifically (after the state mutation had already
// committed) left a permanent, silent divergence: this node's own state
// already reflected the change, but no other node would ever learn about
// it. Wrapping both in one transaction means a failure at either step undoes
// both.
func (cs *ChainState) runAtomicWithOutbox(touchedAddrs []string, fullSnapshot bool, fn func(ctx context.Context) (Transaction, error)) error {
	if cs.db == nil {
		// No DB configured — nothing to make atomic with. Every call site
		// of TransferAtomic/SwapAtomic/etc. treats a non-nil error here as
		// "the operation itself failed" (e.g. evm_rpc.go returns an RPC
		// error to the caller) — so this must NOT return an error just
		// because there's no outbox to use; the state mutation itself
		// already genuinely succeeded. This matches the pre-existing
		// no-DB-mode contract elsewhere in this file (e.g. saveAccountToDB
		// treats !cs.useDB as "mark as saved", not a failure).
		//
		// context.Background() is correct here, not a placeholder: this
		// branch never sets cs.activeTx, so there is no transaction for fn
		// to join regardless of what ctx carries.
		cs.mu.Lock()
		_, err := fn(context.Background())
		cs.mu.Unlock()
		return err
	}

	// FIX (audit recheck2, P0 #1): chainConfig must still be read before the
	// lock (blocking DB call — see snapshotForRollback's own comment), but
	// the accounts/pool snapshot itself now happens via the Locked variant
	// INSIDE the same critical section as fn(), not in a separate RLock that
	// fully releases before this Lock() is even acquired — see
	// snapshotForRollbackLocked's comment for the race this closes.
	//
	// FIX (audit 2026-06-28 recheck 4, P0-1): this read happens BEFORE
	// cs.mu.Lock() below, so it must use the plain DB-only variant — going
	// through cs.dbExec()/cs.activeTx here would risk reading a different,
	// concurrently-running atomic operation's in-flight transaction (a real
	// data race on cs.activeTx itself, since that operation's cs.mu hold
	// doesn't protect a read that never acquires cs.mu in the first place).
	//
	// FIX (deadlock, concurrency audit 2026-07-21): this read used to run
	// AFTER cs.db.Begin() had already checked out and was holding a pool
	// connection for `tx` — meaning every concurrent caller of this
	// function held ONE pool connection (its own idle, not-yet-used tx)
	// while ALSO needing a SECOND one for this loop's own
	// getConfigValueExistsDB call. Once MaxOpenConns concurrent callers all
	// reached that point (each having already grabbed a connection via
	// Begin()), every single one of them needed a 21st connection that
	// could never become free — none of them can finish (and release
	// theirs) until this very read succeeds. Confirmed live: 100 concurrent
	// TransferAtomic callers, MaxOpenConns=20, zero transfers completed
	// after minutes — full goroutine dump showed exactly 20 stuck holding
	// an open *sql.Tx from Begin() and the rest queued behind them, no
	// goroutine ever reaching cs.mu.Lock(). Moving Begin() to below this
	// loop (nothing between the old Begin() and cs.mu.Lock() ever used tx)
	// means a caller only ever holds one pool connection at a time, for the
	// shortest window that actually needs it.
	chainConfig := make(map[string]configValueSnapshot, len(stateRootRelevantConfigKeys))
	for _, key := range stateRootRelevantConfigKeys {
		value, existed := cs.getConfigValueExistsDB(key)
		chainConfig[key] = configValueSnapshot{value: value, existed: existed}
	}

	tx, err := cs.db.Begin()
	if err != nil {
		return fmt.Errorf("could not begin atomic transaction: %w", err)
	}

	// FIX (audit 2026-06-28 recheck 4, P0-2): cs.mu used to be released (and
	// cs.activeTx cleared) BEFORE tx.Commit()/tx.Rollback() ran below. In
	// that gap, the new in-memory state was already visible to every other
	// goroutine, and cs.activeTx==nil meant a concurrent caller would write
	// straight to cs.db — against a state that this transaction might still
	// fail to commit a moment later. If it did fail, restoreFromRollback
	// would revert memory out from under that concurrent write, silently
	// discarding it (or worse, leaving DB and memory permanently
	// disagreeing about whose write actually "won"). cs.mu is now held
	// continuously from before fn() runs through the final commit/rollback
	// decision — restoreFromRollbackLocked (not the public, self-locking
	// restoreFromRollback) is used so the lock is never released and
	// re-acquired in between.
	cs.mu.Lock()
	cs.setActiveTx(tx)
	snap := cs.snapshotForRollbackLocked(touchedAddrs, fullSnapshot, chainConfig)
	// See processTransferBatch's own (now-historical) comment for why
	// building ctx from cs.activeTx here, with cs.mu held throughout, is
	// safe — fn now receives it directly instead of every caller
	// reconstructing the same value from cs.activeTx itself.
	pendingTx, fnErr := fn(withTx(context.Background(), tx))
	var outboxErr error
	if fnErr == nil {
		outboxErr = savePendingTxExec(tx, pendingTx)
	}

	if fnErr != nil || outboxErr != nil {
		cs.setActiveTx(nil)
		tx.Rollback()
		if rbErr := cs.restoreFromRollbackLocked(snap); rbErr != nil {
			fmt.Printf("[ATOMIC] CRITICAL: rollback persistence failed after operation failure — memory/DB may now disagree: %v\n", rbErr)
		}
		cs.mu.Unlock()
		if fnErr != nil {
			return fnErr
		}
		return fmt.Errorf("outbox insert failed inside atomic transaction (state mutation rolled back): %w", outboxErr)
	}

	if err := tx.Commit(); err != nil {
		cs.setActiveTx(nil)
		if rbErr := cs.restoreFromRollbackLocked(snap); rbErr != nil {
			fmt.Printf("[ATOMIC] CRITICAL: rollback persistence failed after commit failure — memory/DB may now disagree: %v\n", rbErr)
		}
		cs.mu.Unlock()
		return fmt.Errorf("commit failed (state mutation rolled back): %w", err)
	}
	cs.setActiveTx(nil)
	cs.mu.Unlock()
	return nil
}

// runAtomicDistributionWithOutbox is runAtomicWithOutbox's counterpart for
// the daily distribution round: fn mutates state across several sub-steps
// (UBI, validators, LP, escrow) and returns EVERY Transaction those
// sub-steps produced; all of them are inserted into the pending_tx outbox
// inside the SAME DB transaction as every account/pool/config write fn made
// (via cs.activeTx — see dbExec), committed once at the end.
//
// FIX (audit3, P0 #3): distribution used to run each sub-step as its own
// immediately-committing operation (cs.mu.Lock/Unlock per Distribute* call),
// then separately call SavePendingTx per resulting TX afterward — main.go's
// WithBlockProductionPaused (added earlier this session) only serialized
// this against ProduceBlock's ticker, it never made the mutations and the
// outbox inserts one atomic unit. A crash or DB error between any mutation
// and its corresponding SavePendingTx call still produced state no other
// node could ever replay. There is also deliberately NO in-memory
// AddTransaction fallback here (unlike SavePendingTx's own retry-then-
// fallback contract used elsewhere) — for a consensus event the size of a
// full daily distribution, an outbox failure must roll back the whole
// round, not be "rescued" by a queue that doesn't survive a restart.
func (cs *ChainState) runAtomicDistributionWithOutbox(fn func(ctx context.Context) ([]Transaction, error)) error {
	if cs.db == nil {
		// context.Background() is correct — see runAtomicWithOutbox's
		// matching no-DB branch comment.
		cs.mu.Lock()
		_, err := fn(context.Background())
		cs.mu.Unlock()
		return err
	}

	// Full snapshot: distribution can touch any number of humans/validators/
	// LP holders/escrow wallets, none of which are known in advance — same
	// reasoning blockTouchedAddresses already uses for ubi_distribution.
	//
	// FIX (audit recheck2, P0 #1): see runAtomicWithOutbox's matching comment
	// — snapshot now taken via the Locked variant inside the same critical
	// section as fn(), not via a separate RLock that fully releases before
	// this Lock() is acquired.
	//
	// FIX (audit 2026-06-28 recheck 4, P0-1): plain DB-only read — see the
	// matching comment in runAtomicWithOutbox for why this must never go
	// through cs.dbExec()/cs.activeTx before cs.mu.Lock() is held.
	//
	// FIX (deadlock, concurrency audit 2026-07-21): moved below the old
	// cs.db.Begin() call — see runAtomicWithOutbox's matching FIX comment
	// for the full connection-pool self-deadlock this closes (identical
	// pattern, same fix: don't hold a pool connection via an idle tx while
	// this loop needs a second one of its own).
	chainConfig := make(map[string]configValueSnapshot, len(stateRootRelevantConfigKeys))
	for _, key := range stateRootRelevantConfigKeys {
		value, existed := cs.getConfigValueExistsDB(key)
		chainConfig[key] = configValueSnapshot{value: value, existed: existed}
	}

	tx, err := cs.db.Begin()
	if err != nil {
		return fmt.Errorf("could not begin atomic distribution transaction: %w", err)
	}

	// FIX (audit 2026-06-28 recheck 4, P0-2): same fix as runAtomicWithOutbox
	// above — cs.mu now stays held continuously through the final
	// commit/rollback decision instead of being released beforehand, so no
	// concurrent operation can observe the new memory state and write
	// against cs.db while this transaction's fate is still undecided.
	cs.mu.Lock()
	cs.setActiveTx(tx)
	snap := cs.snapshotForRollbackLocked(nil, true, chainConfig)
	txs, fnErr := fn(withTx(context.Background(), tx))
	var outboxErr error
	if fnErr == nil {
		for _, t := range txs {
			if outboxErr = savePendingTxExec(tx, t); outboxErr != nil {
				break
			}
		}
	}

	if fnErr != nil || outboxErr != nil {
		cs.setActiveTx(nil)
		tx.Rollback()
		if rbErr := cs.restoreFromRollbackLocked(snap); rbErr != nil {
			fmt.Printf("[ATOMIC] CRITICAL: distribution rollback persistence failed — memory/DB may now disagree: %v\n", rbErr)
		}
		cs.mu.Unlock()
		if fnErr != nil {
			return fnErr
		}
		return fmt.Errorf("outbox insert failed inside atomic distribution transaction (state mutation rolled back): %w", outboxErr)
	}

	if err := tx.Commit(); err != nil {
		cs.setActiveTx(nil)
		if rbErr := cs.restoreFromRollbackLocked(snap); rbErr != nil {
			fmt.Printf("[ATOMIC] CRITICAL: distribution rollback persistence failed after commit failure — memory/DB may now disagree: %v\n", rbErr)
		}
		cs.mu.Unlock()
		return fmt.Errorf("commit failed (state mutation rolled back): %w", err)
	}
	cs.setActiveTx(nil)
	cs.mu.Unlock()
	return nil
}

// RunDailyDistributionAtomic runs the complete daily distribution round —
// UBI, validator pool, LP pool, escrow move/release — as ONE all-or-nothing
// DB transaction together with every resulting outbox TX (see
// runAtomicDistributionWithOutbox). ubiAt is the single timestamp the
// caller (main.go) chose once for this round; it's used for both the
// primary's own immediate last_ubi_at write and the
// ubi_distribution_finalize TX every secondary replays — see
// ApplyUBIFinalizeDelta's comment for why that must be one shared value,
// not each side's own time.Now()/block.Timestamp.
func (cs *ChainState) RunDailyDistributionAtomic(ubiAt int64) error {
	return cs.runAtomicDistributionWithOutbox(func(ctx context.Context) ([]Transaction, error) {
		var txs []Transaction

		ubiShares, err := cs.distributeUBIPoolLocked(ctx)
		if err != nil {
			return nil, fmt.Errorf("UBI distribution failed: %w", err)
		}
		var ubiTotal float64
		for _, s := range ubiShares {
			txs = append(txs, Transaction{Type: "ubi_distribution", Wallet: s.Wallet, Amount: s.Amount, FromDemurrageLost: s.DemurrageLost})
			ubiTotal += s.Amount
		}
		if ubiTotal > 0 {
			if err := cs.applyUBIFinalizeDeltaLocked(ctx, ubiAt); err != nil {
				return nil, fmt.Errorf("UBI finalize failed: %w", err)
			}
			txs = append(txs, Transaction{Type: "ubi_distribution_finalize", DistributionAt: ubiAt})
		}

		validatorShares, err := cs.distributeValidatorsPoolLocked(ctx)
		if err != nil {
			return nil, fmt.Errorf("validator distribution failed: %w", err)
		}
		var validatorTotal float64
		for _, s := range validatorShares {
			txs = append(txs, Transaction{Type: "validator_distribution", Wallet: s.Wallet, Amount: s.Amount, FromDemurrageLost: s.DemurrageLost})
			validatorTotal += s.Amount
		}
		if validatorTotal > 0 {
			txs = append(txs, Transaction{Type: "validator_distribution_pool_zero"})
		}

		lpShares, err := cs.distributeLPPoolLocked(ctx)
		if err != nil {
			return nil, fmt.Errorf("LP distribution failed: %w", err)
		}
		var lpTotal float64
		for _, s := range lpShares {
			txs = append(txs, Transaction{Type: "lp_distribution", Wallet: s.Wallet, Amount: s.Amount, FromDemurrageLost: s.DemurrageLost})
			lpTotal += s.Amount
		}
		if lpTotal > 0 {
			txs = append(txs, Transaction{Type: "lp_distribution_pool_zero"})
		}

		moved, err := cs.checkAndMoveToEscrowLocked(ctx)
		if err != nil {
			return nil, fmt.Errorf("escrow move failed: %w", err)
		}
		for _, s := range moved {
			txs = append(txs, Transaction{
				Type:                "escrow_move",
				Wallet:              s.Wallet,
				Amount:              s.Amount,
				FromDemurrageLost:   s.DemurrageLost,
				LPShares:            s.LPSharesBurned,
				EscrowTUsdConverted: s.TUsdConverted,
			})
		}

		released, err := cs.releaseEscrowToUBILocked(ctx)
		if err != nil {
			return nil, fmt.Errorf("escrow release failed: %w", err)
		}
		for _, s := range released {
			txs = append(txs, Transaction{Type: "escrow_release", Wallet: s.Wallet, Amount: s.Amount})
		}

		return txs, nil
	})
}

// Transfer moves amount AEQ from->to on the primary node. Returns the AEQ
// amount demurrage-decayed off the sender and recipient respectively (0 if
// neither was idle long enough to decay) — callers must attach these to the
// queued Transaction so secondary nodes replay the exact same numbers
// instead of recomputing decay at their own wall-clock time (see
// applyDemurrageLossLocked).
func (cs *ChainState) Transfer(from, to string, amount float64) (float64, float64, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// context.Background() is correct here, not a placeholder: this path
	// does not go through runAtomicWithOutbox, so there has never been an
	// active transaction for it to join — every DB write below already fell
	// back to cs.db directly, exactly as dbExecCtx's own fallback preserves.
	return cs.transferLocked(context.Background(), from, to, amount)
}

// TransferAtomic behaves exactly like Transfer, except the state mutation
// and the resulting outbox insert commit or roll back together as one DB
// transaction (see runAtomicWithOutbox) instead of the outbox write being a
// separate, independently-failable step after this one has already
// committed. pendingTxTemplate should have Type/Wallet/To/Amount/TxHash
// set; FromDemurrageLost/ToDemurrageLost are filled in here from the
// transfer's actual result before it's queued — those aren't known until
// transferLocked runs, so the caller can't supply them upfront. Use this
// instead of calling Transfer + SavePendingTx separately whenever the
// caller will queue a pendingTx describing this transfer right afterward.
//
// THROUGHPUT (group commit, 2026-07-22): with cs.db set, this no longer
// opens its own transaction — it hands the request to runTransferBatcher,
// which coalesces concurrently-arriving TransferAtomic calls into shared DB
// transactions (see that function's own comment for why: the prior design,
// one Begin()/Commit() pair per transfer, meant every transfer paid its own
// fsync and serialized fully behind cs.mu regardless — a real deadlock was
// found and fixed in that design under concurrent load, see this file's git
// history; the throughput ceiling after that fix was ~186 TPS single-node,
// dominated by per-commit fsync cost). Behavior for a single isolated
// caller is unchanged; the one observable difference under concurrency is
// that a batch is all-or-nothing — see runTransferBatcher's own comment for
// why, and processTransferBatch's for what a caller gets back when a batch
// mate (not this call) is what failed. Falls back to the direct,
// unbatched path when cs.db is nil (no-DB / test mode) — batching only
// helps when there's a real fsync to amortize.
//
// SHARD-LOCKED FAST PATH (SCALING_ARCHITECTURE.md Phase 5, not
// staging-validated — see transferConcurrent's own doc comment): tried
// first, ahead of the batcher. When applied is true the fast path genuinely
// ran — its result (success or a real error, e.g. insufficient balance) is
// final and returned as-is, never retried through the batcher. Only when
// applied is false (cold account, would settle demurrage, would overflow
// the wealth cap, or no DB) does this fall through to the batched path
// below, unchanged from before this fast path existed.
//
// WAL-DURABLE FAST PATH (SCALING_ARCHITECTURE.md Phase 7, transfer_wal.go —
// NOT staging-validated, changes real durability semantics when enabled,
// see that file's own warning): when cs.wal is set (AEQUITAS_WAL_ENABLED=1),
// transferConcurrentWAL is tried INSTEAD of transferConcurrent, not in
// addition to it — both share the exact same eligibility scope, so trying
// both would just repeat the same checks twice for nothing. cs.wal is nil
// by default, so this branch does not change behavior for any node that
// hasn't explicitly opted in.
func (cs *ChainState) TransferAtomic(from, to string, amount float64, pendingTxTemplate Transaction) (fromLost, toLost float64, err error) {
	// Time the whole call. Throughput has sat near 1,264/s while the node used
	// 244% of 600% available CPU with no lock contention, no connection waits
	// and only 6% of samples in database syscalls — so transfers are spending
	// their time waiting, and until now nothing measured on what. Dividing
	// throughput by the load generator's 72 concurrent senders implies roughly
	// 57ms per transfer, which is the number this either confirms or refutes.
	transferStart := time.Now()
	defer func() { recordTransferLatency(time.Since(transferStart)) }()
	from = strings.ToLower(from)
	to = strings.ToLower(to)
	if cs.db == nil {
		return cs.transferAtomicDirect(from, to, amount, pendingTxTemplate)
	}
	if cs.wal != nil {
		if fLost, tLost, applied, werr := cs.transferConcurrentWAL(from, to, amount, pendingTxTemplate); applied {
			transferFastPathApplied.Add(1)
			return fLost, tLost, werr
		}
	} else if fLost, tLost, applied, cerr := cs.transferConcurrent(from, to, amount, pendingTxTemplate); applied {
		transferFastPathApplied.Add(1)
		return fLost, tLost, cerr
	}
	transferFastPathFallback.Add(1)
	cs.ensureTransferBatcherStarted()
	req := &transferBatchRequest{
		from: from, to: to, amount: amount,
		pendingTxTemplate: pendingTxTemplate,
		result:            make(chan transferBatchResult, 1),
	}
	cs.transferBatchCh <- req
	res := <-req.result
	return res.fromLost, res.toLost, res.err
}

// transferFastPathApplied/transferFastPathFallback count, process-wide, how
// often TransferAtomic's shard-locked/WAL fast path actually ran versus
// fell through to the batcher (cold account, contended shard, wealth-cap/
// demurrage edge case, or WAL/DB unavailable -- see transferConcurrent/
// transferConcurrentWAL's own eligibility checks for the exact conditions).
// Cheap (single atomic add on the already-hot transfer path) and exists so
// this ratio can be measured directly instead of inferred from throughput
// alone -- see TestSimulateMaxTPS_WarmSteadyState, which reports it.
var (
	transferFastPathApplied  atomic.Int64
	transferFastPathFallback atomic.Int64
)

// TransferFastPathStats returns the process-wide applied/fallback counts
// since the last ResetTransferFastPathStats call (or process start).
// Exported for benchmarks/diagnostics only -- not read anywhere in the
// normal transfer path itself.
func TransferFastPathStats() (applied, fallback int64) {
	return transferFastPathApplied.Load(), transferFastPathFallback.Load()
}

// ResetTransferFastPathStats zeroes both counters, e.g. so a benchmark can
// exclude an untimed warm-up pass from the ratio it reports.
func ResetTransferFastPathStats() {
	transferFastPathApplied.Store(0)
	transferFastPathFallback.Store(0)
}

// transferAtomicDirect is TransferAtomic's original, unbatched
// implementation — one Begin()/Commit() per call via runAtomicWithOutbox.
// Used directly when there's no real DB (batching has nothing to amortize),
// and by the batcher's own no-DB code path is unreachable there since
// TransferAtomic already short-circuits before ever enqueuing.
func (cs *ChainState) transferAtomicDirect(from, to string, amount float64, pendingTxTemplate Transaction) (fromLost, toLost float64, err error) {
	err = cs.runAtomicWithOutbox([]string{from, to, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr}, false, func(ctx context.Context) (Transaction, error) {
		// ctx is correct to pass straight through here, not a placeholder:
		// this function only ever runs via runAtomicWithOutbox's no-DB
		// branch (see this function's own doc comment — "Used directly when
		// there is no real DB"), which always calls fn(context.Background()),
		// so ctx carries no transaction to lose regardless.
		fromLost, toLost, err = cs.transferLocked(ctx, from, to, amount)
		if err != nil {
			return Transaction{}, err
		}
		pendingTxTemplate.FromDemurrageLost = fromLost
		pendingTxTemplate.ToDemurrageLost = toLost
		return pendingTxTemplate, nil
	})
	return fromLost, toLost, err
}

// transferBatchRequest is one caller's pending TransferAtomic call, queued
// for runTransferBatcher to fold into a shared DB transaction.
type transferBatchRequest struct {
	from, to          string
	amount            float64
	pendingTxTemplate Transaction
	result            chan transferBatchResult
}

type transferBatchResult struct {
	fromLost, toLost float64
	err              error
}

// transferBatchChSize bounds how many pending batch requests can queue
// before TransferAtomic callers block feeding the channel — generous
// enough that a real burst doesn't stall producers, bounded so a stuck
// batcher (should never happen — see its own panic-recovery) can't grow
// this without limit.
const transferBatchChSize = 4096

// transferBatchMaxSize caps how many transfers one physical DB transaction
// bundles: bounds how long cs.mu is held for a single batch (every member
// still does real work — demurrage settlement, wealth-cap enforcement, EVM
// mirror sync) and keeps a single COMMIT's WAL record a bounded size.
//
// FIX (2026-07-23, TPS-benchmark investigation): raised from 200 to 1000
// after processTransferBatch stopped paying one DB round trip per member
// (previous commit) — with that fixed, a LOW cap became the new binding
// constraint at high concurrency: 200 in-flight requests is a small slice
// of what a real burst can look like, so hitting the cap mid-burst forces
// several sequential batches (each still paying its own fixed group-commit
// overhead: the wait window plus one fsync-backed commit) to drain what a
// single bigger batch could have absorbed in one round trip. Measured on
// the shared-recipient benchmark at 2000-5000 concurrent senders (well
// beyond this test suite's normal 100, run manually to find where this cap
// started to matter): raising it 200->1000 roughly doubled sustained TPS
// (7.6k->13.3k at 5000 senders); 1000->2500+ kept climbing but with
// shrinking returns (~15k, essentially the concurrency ceiling of the
// probe itself, not of the constant). Chose 1000 over chasing that last
// ~15%: it keeps a single batch's worst-case cs.mu hold time in the same
// low-hundreds-of-ms range this codebase already accepts elsewhere
// (walFlushInterval=500ms, poolFlushInterval=2s) rather than the
// multi-hundred-ms-to-low-second hold a 2500-6000 cap could produce under
// a genuinely large burst, trading a small amount of peak throughput for
// bounded latency on any isolated request unlucky enough to arrive while a
// giant batch is mid-commit. At normal (non-bursty) concurrency this has
// no effect at all — actual batch size is bounded by how many requests are
// really in flight, not by this cap, exactly as before.
const transferBatchMaxSize = 1000

// transferBatchMaxWait is the group-commit window: how long the batcher
// waits for more requests before committing whatever it already has. Short
// enough that one isolated request still gets a fast response (worst case:
// this plus one commit); long enough that a real concurrent burst
// coalesces into one commit instead of each paying its own fsync.
//
// FIX (2026-07-23, TPS-benchmark investigation): lowered from 3ms to 1ms —
// same root cause and same fix as wal.MaxBatchWait (x/humanity/wal/wal.go,
// see its own FIX comment): every caller of TransferAtomic blocks on
// req.result until its batch's commit returns, so this constant isn't just
// "how long a batch waits to grow" — under closed-loop load (every real
// client also waits for its previous transfer's result before sending the
// next one, exactly like this file's own TestSimulateMaxTPS_Ingestion
// benchmark) it directly throttles how fast new requests can arrive to
// join a batch at all. A longer wait doesn't reliably produce a bigger,
// more-amortized batch under that load shape — it mostly just adds latency
// every cycle pays before the pipe can refill. Measured on the
// shared-recipient benchmark (single hot recipient, so this constant is
// the only lever — see that test's own comment on why it never takes the
// shard-locked fast path): 3 runs each at 3ms/1ms/500us/200us/100us gave
// averages of 858.6/889.6/858.4/838.6/815.2 TPS respectively. 1ms was both
// the highest average AND by far the lowest-variance (871-899 across all 3
// runs, vs. e.g. 100us's 699.7-914.3 -- sub-millisecond windows start
// producing batches too small to amortize the fsync reliably, the same
// tradeoff wal.MaxBatchWait's own comment describes). Chose 1ms over
// wal.go's exact reasoning rather than chasing the single best individual
// run, for the same reason: a config whose worst case is competitive
// matters more here than a config whose best case is highest.
const transferBatchMaxWait = 1 * time.Millisecond

// ensureTransferBatcherStarted lazily starts the one background goroutine
// that drains transferBatchCh — lazy (not started in NewChainState) so a
// node that never processes a single transfer never pays for an idle
// goroutine, and so this works uniformly whether TransferAtomic's first
// call happens to be during tests or during normal operation.
func (cs *ChainState) ensureTransferBatcherStarted() {
	cs.transferBatchOnce.Do(func() {
		cs.transferBatchCh = make(chan *transferBatchRequest, transferBatchChSize)
		cs.parallelBatchSem = make(chan struct{}, parallelBatchPoolSize)
		SafeGoroutine("transferBatcher", cs.runTransferBatcher)
	})
}

// runTransferBatcher is the group-commit collection loop: block for the
// first request, then greedily collect more (up to transferBatchMaxSize)
// for up to transferBatchMaxWait, exactly as before.
//
// THROUGHPUT (2026-07-23, 50k-TPS-goal investigation): each collected
// batch is now dispatched to its own goroutine (bounded by
// parallelBatchSem, so at most parallelBatchPoolSize batches ever have
// their own DB transaction open at once) instead of being processed
// synchronously right here — this collector loop no longer waits for one
// batch's DB round trip to finish before it can start collecting the
// next. Each dispatch first tries processTransferBatchConcurrent
// (transfer_batch_concurrent.go): a shard-locked path (the same
// TryLockAddrs mechanism transferConcurrent already uses, generalized
// from 2 addresses to the whole batch) that lets batches with genuinely
// disjoint touched-address sets commit to Postgres truly in parallel,
// instead of always serializing behind one global lock regardless of
// whether their addresses even overlap. It bails (returns false) for
// every case it isn't safely eligible for — falling back to the
// existing, unchanged, always-correct processTransferBatch, still fully
// serialized via cs.mu.Lock(), for those. See processTransferBatchConcurrent's
// own comment for the deadlock-safety argument this relies on, and why
// it doesn't repeat an earlier (reverted) attempt's mistake.
func (cs *ChainState) runTransferBatcher() {
	for first := range cs.transferBatchCh {
		batch := []*transferBatchRequest{first}
		timer := time.NewTimer(transferBatchMaxWait)
	collect:
		for len(batch) < transferBatchMaxSize {
			select {
			case req := <-cs.transferBatchCh:
				batch = append(batch, req)
			case <-timer.C:
				break collect
			}
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		cs.parallelBatchSem <- struct{}{}
		go func(b []*transferBatchRequest) {
			defer func() { <-cs.parallelBatchSem }()
			SafeCall("transferBatchParallelDispatch", func() {
				if !cs.processTransferBatchConcurrent(b) {
					cs.processTransferBatch(b)
				}
			})
		}(batch)
	}
}

// processTransferBatch commits an entire batch as ONE runAtomicWithOutbox
// call — reusing that function's existing, already-proven snapshot/
// rollback/commit machinery completely unchanged (see its own comment) —
// rather than inventing new per-request SAVEPOINT-based partial-rollback
// logic. The tradeoff this makes deliberately: the batch is all-or-nothing.
// If ANY member's transferLocked call fails (insufficient balance, self-
// transfer, etc.), the WHOLE batch rolls back and every member in it —
// including ones whose own transfer would have succeeded alone — gets an
// error naming which member actually failed, safe to retry individually.
// This is a real cost under concurrent load with a high per-request
// failure rate, but SAVEPOINT-based per-request isolation would need its
// own new snapshot/restore reasoning for the in-memory side (this
// codebase's rollback snapshots restore to "state right before this
// specific attempt", which composes correctly only when restores also
// unwind in reverse chronological order for addresses — like the 4
// tokenomics pools — every request in a batch shares) — exactly the class
// of subtlety that has caused real production incidents in this file
// before (see runAtomicWithOutbox's and ghostdagIsAncestor's own FIX
// comments). All-or-nothing reuses machinery already proven correct;
// per-request isolation is a valid future step once real telemetry shows
// batch-abort rate actually matters, not before.
func (cs *ChainState) processTransferBatch(batch []*transferBatchRequest) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[PANIC RECOVERED] processTransferBatch: %v\n%s\n", r, debug.Stack())
			for _, req := range batch {
				select {
				case req.result <- transferBatchResult{err: fmt.Errorf("internal error processing transfer batch: %v", r)}:
				default:
				}
			}
		}
	}()

	touchedSet := map[string]bool{
		validatorsPoolAddr: true, lpPoolAddr: true, ubiPoolAddr: true, treasuryPoolAddr: true,
	}
	for _, req := range batch {
		touchedSet[req.from] = true
		touchedSet[req.to] = true
	}
	touched := make([]string, 0, len(touchedSet))
	for addr := range touchedSet {
		touched = append(touched, addr)
	}

	results := make([]transferBatchResult, len(batch))
	err := cs.runAtomicWithOutbox(touched, false, func(ctx context.Context) (Transaction, error) {
		// FIX (2026-07-23, TPS-benchmark investigation): this used to call
		// transferLocked per member, which does its OWN saveAccountToDBCtx
		// round trip for both the sender and recipient — up to 2 individual
		// Exec calls per member, plus one savePendingTxExec, all sequential
		// round trips inside this one already-open transaction. Profiled on
		// the shared-recipient TPS benchmark: internal/poll.(*FD).Write and
		// friends (real network round trips to Postgres) accounted for
		// ~85%+ of CPU time even after transferBatchMaxWait's own fix, with
		// measured batch sizes averaging ~48 members — i.e. up to ~144
		// individual statements per commit. transferMutateLocked applies
		// every member's mutation in memory ONLY (no DB call), and every
		// touched account is then persisted ONCE, after the whole batch's
		// mutations are done, via ONE multi-row saveAccountsToDBBatchCtx
		// call (same technique already proven for the WAL flush path —
		// see flushWALBatch's own FIX comment — and already used elsewhere
		// in this file for distributeSwapFee's 4 pool addresses). Every
		// member's outbox row is similarly collected into ONE multi-row
		// INSERT instead of one savePendingTxExec call each. This does not
		// change the all-or-nothing contract described above: the first
		// member whose mutation fails aborts the whole closure exactly as
		// before, and every account/outbox write still lives inside this
		// same runAtomicWithOutbox transaction, still rolled back as one
		// unit on any later failure.
		touchedAccs := make(map[string]*AccountState, len(batch)*2)
		pendingTxs := make([]Transaction, 0, len(batch))
		var last Transaction
		for i, req := range batch {
			fromLost, toLost, fromAcc, toAcc, mErr := cs.transferMutateLocked(ctx, req.from, req.to, req.amount)
			if mErr != nil {
				return Transaction{}, fmt.Errorf("batch member %d/%d (%s -> %s) failed: %w", i+1, len(batch), req.from, req.to, mErr)
			}
			touchedAccs[fromAcc.Address] = fromAcc
			touchedAccs[toAcc.Address] = toAcc

			pendingTx := req.pendingTxTemplate
			pendingTx.FromDemurrageLost = fromLost.Float()
			pendingTx.ToDemurrageLost = toLost.Float()
			results[i] = transferBatchResult{fromLost: fromLost.Float(), toLost: toLost.Float()}
			if i == len(batch)-1 {
				// Last member's outbox row is still inserted by
				// runAtomicWithOutbox itself (its normal single-Transaction
				// contract, unchanged) — every earlier member's row is
				// inserted explicitly below, as one multi-row statement.
				last = pendingTx
				continue
			}
			pendingTxs = append(pendingTxs, pendingTx)
		}

		accsToSave := make([]*AccountState, 0, len(touchedAccs))
		for _, acc := range touchedAccs {
			accsToSave = append(accsToSave, acc)
		}
		if err := cs.saveAccountsToDBBatchCtx(ctx, accsToSave); err != nil {
			return Transaction{}, fmt.Errorf("could not batch-save %d account(s): %w", len(accsToSave), err)
		}
		if err := savePendingTxsBatchExec(cs.dbExecCtx(ctx), pendingTxs); err != nil {
			return Transaction{}, fmt.Errorf("could not batch-insert %d outbox row(s): %w", len(pendingTxs), err)
		}

		touchedAddrs := make([]string, 0, len(touchedAccs)+4)
		for addr := range touchedAccs {
			touchedAddrs = append(touchedAddrs, addr)
		}
		// Every transferLocked call unconditionally refreshed all 4 pool
		// addresses' EVM mirrors too (cheap — see syncBalanceLocked's own
		// comment, this only marks them dirty for the async flush worker) —
		// preserved here so consolidating to one call per batch doesn't
		// change how often the pools' own mirror gets refreshed.
		touchedAddrs = append(touchedAddrs, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr)
		cs.syncBalanceLocked(V7_CONTRACT_ADDR, touchedAddrs...)
		cs.save()
		// FIX (2026-07-23, TPS-benchmark investigation): this used to print
		// one "[STATE] ✓ Transfer" line per member (transferLocked's own
		// line, moved here when its persistence was batched — previous
		// commit). Profiled at 2000 concurrent senders: fmt.(*pp).doPrintf
		// alone was ~10% of total CPU time -- at real 50k TPS, 50,000
		// individual stdout writes/sec would be its own operational
		// problem regardless of anything else in this file. Every other
		// batched background writer in this codebase (flushWALBatch,
		// flushPoolAccountsIfDirty) already logs once per flush, not once
		// per item it flushed -- this now matches that convention instead
		// of being the one exception.
		fmt.Printf("[STATE] ✓ Batch committed: %d transfer(s), %d account(s) updated\n", len(batch), len(accsToSave))
		return last, nil
	})
	if err != nil {
		// All-or-nothing: runAtomicWithOutbox already rolled back every
		// mutation (DB and in-memory) this batch made, regardless of which
		// member actually failed — every result computed above is stale.
		for i := range results {
			results[i] = transferBatchResult{err: err}
		}
	}
	for i, req := range batch {
		req.result <- results[i]
	}
}

// transferLocked is Transfer's actual implementation; caller must already
// hold cs.mu. Split out so TransferAtomic can run it under the SAME lock
// acquisition it uses to set/clear cs.activeTx (see runAtomicWithOutbox) —
// Transfer() itself locks cs.mu, so calling it from inside an already-locked
// context would deadlock on Go's non-reentrant sync.Mutex.
// transferLocked takes ctx explicitly (see dbExecCtx's own comment) rather
// than through the old/new wrapper split most other shared functions here
// use: unlike those, it has exactly three callers (TransferAtomic's no-DB
// fallback, transferAtomicDirect, processTransferBatch), all migrated to
// pass ctx explicitly in this same change, so no compatibility wrapper is
// needed.
func (cs *ChainState) transferLocked(ctx context.Context, from, to string, amount float64) (float64, float64, error) {
	fromLost, toLost, fromAcc, toAcc, err := cs.transferMutateLocked(ctx, from, to, amount)
	if err != nil {
		return 0, 0, err
	}
	// FIX (audit3, P1 #4): saveAccountToDB now returns an error — checked here
	// so a DB failure aborts the transfer (causing runAtomicWithOutbox to roll
	// back) instead of returning success while the debit was never persisted.
	if err := cs.saveAccountToDBCtx(ctx, fromAcc); err != nil {
		return 0, 0, fmt.Errorf("could not save sender account: %w", err)
	}
	if err := cs.saveAccountToDBCtx(ctx, toAcc); err != nil {
		return 0, 0, fmt.Errorf("could not save recipient account: %w", err)
	}
	cs.save()

	fmt.Printf("[STATE] ✓ Transfer %.2f AEQ: %s → %s\n", amount, fromAcc.Address, toAcc.Address)
	cs.syncBalanceLocked(V7_CONTRACT_ADDR, fromAcc.Address, toAcc.Address, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr)
	return fromLost.Float(), toLost.Float(), nil
}

// transferMutateLocked is transferLocked's actual balance-mutation logic,
// split out so processTransferBatch can apply every batch member's mutation
// in memory without ALSO paying transferLocked's own two saveAccountToDBCtx
// round trips per member — see processTransferBatch's own comment for why
// that matters (up to 2N individual Exec calls per batch, one N-times-
// smaller multi-row UPSERT instead). Does not persist either account, call
// cs.save(), print the "[STATE] ✓ Transfer" line, or mark the EVM mirror
// dirty — every one of those is still transferLocked's job for its own
// (single-transfer) callers, and processTransferBatch's job (once per whole
// batch, not once per member) for the batched path.
func (cs *ChainState) transferMutateLocked(ctx context.Context, from, to string, amount float64) (fromLost, toLost Decimal, fromAcc, toAcc *AccountState, err error) {
	from = strings.ToLower(from)
	to = strings.ToLower(to)
	// P1-FIX: reject NaN/Inf amounts — these would corrupt balances via
	// NewDecimal which uses math.Round (NaN/Inf propagate silently).
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, 0, nil, nil, fmt.Errorf("invalid transfer amount: %v", amount)
	}
	// P2-5: reject self-transfers; mirrors AequitasV7.sol behaviour and
	// prevents double-demurrage settlement on the same account object.
	if from == to {
		return 0, 0, nil, nil, fmt.Errorf("self-transfer not allowed")
	}

	cs.ensureAccountLoadedCtx(ctx, from)
	cs.ensureAccountLoadedCtx(ctx, to)
	fromAcc, ok := cs.accounts.Get(from)
	if !ok {
		return 0, 0, nil, nil, fmt.Errorf("insufficient balance")
	}
	fromLost, err = cs.settleDemurrageLockedCtx(ctx, fromAcc) // make sure we're checking against the real, decayed balance
	if err != nil {
		return 0, 0, nil, nil, fmt.Errorf("could not settle demurrage for sender: %w", err)
	}
	if fromAcc.Balance.Float() < amount {
		return 0, 0, nil, nil, fmt.Errorf("insufficient balance")
	}

	fromAcc.Balance = fromAcc.Balance.Sub(NewDecimal(amount))
	touchActivity(fromAcc) // sending counts as "using" the money — resets its decay clock

	toAcc, ok = cs.accounts.Get(to)
	if !ok {
		toAcc = &AccountState{Address: to}
		cs.accounts.Set(to, toAcc)
	}
	toLost, err = cs.settleDemurrageLockedCtx(ctx, toAcc)
	if err != nil {
		return 0, 0, nil, nil, fmt.Errorf("could not settle demurrage for recipient: %w", err)
	}
	toAcc.Balance = toAcc.Balance.Add(NewDecimal(amount))
	touchActivity(toAcc) // receiving also resets the clock on the recipient's whole balance
	if err := cs.enforceWealthCapLockedCtx(ctx, toAcc); err != nil {
		return 0, 0, nil, nil, fmt.Errorf("could not enforce wealth cap for recipient: %w", err)
	}
	return fromLost, toLost, fromAcc, toAcc, nil
}

// TransferWithV7Fee is used by the RPC layer when intercepting V7 ERC-20
// transfer() calls (selector a9059cbb) — this IS the fee path a real user's
// MetaMask "send AEQ" transaction actually goes through.
//
// FIX (P2-10, beta-launch audit 2026-07-05): this comment used to claim it
// "mirrors V7's _calcFee(): TX_FEE_BPS = 10 (0.1% base fee)" — that's wrong
// on both counts. calcV7Fee below independently hardcodes its own 0.1% base
// + tiered concentration surcharge (0/0.1%/0.5%/1%) rather than reading
// AequitasV7.sol's TX_FEE_BPS constant at all. These are two genuinely
// different fee schedules for the same nominal transfer() call — not a bug
// in the sense of "should be kept identical and drifted," since the
// contract's own transfer()/_calcFee() logic is never actually executed for
// a real user transfer (the RPC layer intercepts the selector before the
// EVM call would reach it — see evm_engine.go's checkPersistedCallAllowed).
// The Go-computed fee below is the one that actually applies to real value
// movements; the contract's TX_FEE_BPS is inert, display-only bytecode a
// raw eth_call against the deployed contract would report but that never
// fires for any live transfer — set to 0 (P2-e, audit 2026-07-06; was
// previously 700/7%, which read as a real, active fee to anyone inspecting
// the contract directly with no reason to know about this RPC-layer
// intercept). Whatever documentation describes "the transfer fee" to users
// should describe THIS fee, not the contract's.
//
//	base = 0.1% of amount
//	Concentration surcharge if sender holds ≥1/5/10% of total supply
//	20% of fee → UBI pool, 80% burned (removed from supply)
//
// Returns (netAmountCredited, fromDemurrageLost, toDemurrageLost, err) — the
// two demurrage figures must be attached to the queued Transaction so
// secondary nodes replay the exact decay instead of recomputing it (see
// applyDemurrageLossLocked).
func (cs *ChainState) TransferWithV7Fee(from, to string, amount float64) (float64, float64, float64, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox — see
	// RegisterHuman's comment.
	return cs.transferWithV7FeeLocked(context.Background(), from, to, amount)
}

// TransferWithV7FeeAtomic behaves like TransferWithV7Fee but commits or
// rolls back together with pendingTx's outbox insert as one DB transaction
// — see runAtomicWithOutbox / TransferAtomic's comment.
// pendingTxTemplate should have Type/Wallet/To/TxHash set; Amount is set
// here to the actual net amount credited (not the raw pre-fee amount), and
// FromDemurrageLost/ToDemurrageLost from the transfer's result — none of
// which are known until transferWithV7FeeLocked runs. See TransferAtomic.
func (cs *ChainState) TransferWithV7FeeAtomic(from, to string, amount float64, pendingTxTemplate Transaction) (netAmount, fromLost, toLost float64, err error) {
	from = strings.ToLower(from)
	to = strings.ToLower(to)
	err = cs.runAtomicWithOutbox([]string{from, to, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr}, false, func(ctx context.Context) (Transaction, error) {
		netAmount, fromLost, toLost, err = cs.transferWithV7FeeLocked(ctx, from, to, amount)
		if err != nil {
			return Transaction{}, err
		}
		pendingTxTemplate.Amount = netAmount
		pendingTxTemplate.FromDemurrageLost = fromLost
		pendingTxTemplate.ToDemurrageLost = toLost
		return pendingTxTemplate, nil
	})
	return netAmount, fromLost, toLost, err
}

// transferWithV7FeeLocked is TransferWithV7Fee's implementation; caller
// must already hold cs.mu — see transferLocked's comment for why this split
// exists.
func (cs *ChainState) transferWithV7FeeLocked(ctx context.Context, from, to string, amount float64) (float64, float64, float64, error) {
	from = strings.ToLower(from)
	to = strings.ToLower(to)

	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, 0, 0, fmt.Errorf("invalid transfer amount: %v", amount)
	}

	// Page both parties in from the DB if they're cold (beyond maxInMemAccounts):
	// without this a returning sender hits "insufficient balance" despite a real
	// DB balance, and a cold RECIPIENT would be recreated blank below and have
	// its real balance overwritten on save. Matches transferLocked.
	cs.ensureAccountLoadedCtx(ctx, from)
	cs.ensureAccountLoadedCtx(ctx, to)
	fromAcc, ok := cs.accounts.Get(from)
	if !ok {
		return 0, 0, 0, fmt.Errorf("insufficient balance")
	}
	fromLost, err := cs.settleDemurrageLockedCtx(ctx, fromAcc)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("could not settle demurrage for sender: %w", err)
	}
	if fromAcc.Balance.Float() < amount {
		return 0, 0, 0, fmt.Errorf("insufficient balance")
	}

	// FIX (performance audit 2026-07-06): this used to scan the entire
	// cs.accounts map on every fee-liable transfer just to rederive
	// TotalSupply = humans*1000, an already-documented invariant — see
	// humanCountLocked's own comment for why it's now O(1) here without
	// reintroducing the in-memory-cache-undercounts-at-scale risk that was
	// already found and fixed once for accountSetXOR.
	totalSupply := float64(cs.humanCountLocked()) * 1000.0
	fee := calcV7Fee(fromAcc.Balance.Float(), amount, totalSupply)
	// E1-FIX: In the Go-state ledger, AEQ cannot be burned (supply is tied
	// to humans * 1000). Redirect 100% of fee to UBI pool instead of the
	// V7-contract's 20%/80% split — this preserves the supply invariant
	// and ensures all fees benefit the community rather than disappearing.
	// E-FIX: compute net first, derive ubi as remainder - preserves supply invariant
	netToRecipient := round6(amount - fee)
	ubiContrib := amount - netToRecipient

	fromAcc.Balance = fromAcc.Balance.Sub(NewDecimal(amount))
	touchActivity(fromAcc)
	if err := cs.saveAccountToDBCtx(ctx, fromAcc); err != nil {
		return 0, 0, 0, fmt.Errorf("could not save sender account: %w", err)
	}

	toAcc, ok := cs.accounts.Get(to)
	if !ok {
		toAcc = &AccountState{Address: to}
		cs.accounts.Set(to, toAcc)
	}
	toLost, err := cs.settleDemurrageLockedCtx(ctx, toAcc)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("could not settle demurrage for recipient: %w", err)
	}
	toAcc.Balance = toAcc.Balance.Add(NewDecimal(netToRecipient))
	touchActivity(toAcc)
	if err := cs.enforceWealthCapLockedCtx(ctx, toAcc); err != nil {
		return 0, 0, 0, fmt.Errorf("could not enforce wealth cap for recipient: %w", err)
	}
	if err := cs.saveAccountToDBCtx(ctx, toAcc); err != nil {
		return 0, 0, 0, fmt.Errorf("could not save recipient account: %w", err)
	}

	if ubiContrib > 0 {
		// FIX (Monster Audit 2026-07-12, P1): without this, a cold ubiPoolAddr
		// got recreated as a blank AccountState{} here and saved with
		// Version==0, which saveAccountToDB's Version==0 branch treats as
		// "brand new row" and blindly overwrites any existing DB balance —
		// silently erasing real, previously-accumulated pool funds.
		cs.ensureAccountLoadedCtx(ctx, ubiPoolAddr)
		ubiAcc, ok := cs.accounts.Get(ubiPoolAddr)
		if !ok {
			ubiAcc = &AccountState{Address: ubiPoolAddr}
			cs.accounts.Set(ubiPoolAddr, ubiAcc)
		}
		ubiAcc.Balance = ubiAcc.Balance.Add(NewDecimal(ubiContrib))
		if err := cs.saveAccountToDBCtx(ctx, ubiAcc); err != nil {
			return 0, 0, 0, fmt.Errorf("could not save UBI pool: %w", err)
		}
	}
	cs.save()

	fmt.Printf("[STATE] ✓ TransferV7 %.6f AEQ (fee=%.6f → UBI): %s → %s\n",
		amount, fee, from, to)
	cs.syncBalanceLocked(V7_CONTRACT_ADDR, from, to, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr)
	return netToRecipient, fromLost.Float(), toLost.Float(), nil
}

// calcV7Fee is the Go ledger's own fee schedule for a real user transfer —
// see TransferWithV7Fee's comment for why this does NOT actually mirror
// AequitasV7.sol's _calcFee()/TX_FEE_BPS despite this function's name.
// base = 0.1% of amount, plus a concentration surcharge based on the
// sender's share of total supply.
func calcV7Fee(senderBalance, amount, totalSupply float64) float64 {
	base := amount * 10.0 / 10_000.0
	if totalSupply <= 0 {
		return round6(base)
	}
	shareBPS := (senderBalance * 10_000.0) / totalSupply
	var extra float64
	switch {
	case shareBPS >= 1000:
		extra = amount * 100.0 / 10_000.0
	case shareBPS >= 500:
		extra = amount * 50.0 / 10_000.0
	case shareBPS >= 100:
		extra = amount * 10.0 / 10_000.0
	}
	return round6(base + extra)
}

// Fee recipient addresses for the four tokenomics pools, per the original
// design (40% validators / 30% LPs / 20% UBI / 10% treasury). These are
// real wallet addresses Daniel controls — provided explicitly so swap
// fees are credited somewhere actually accessible, rather than to
// addresses with no corresponding private key.
const (
	validatorsPoolAddr = "0x78c1c143e395b181f13bcb6868ff53aa86c3d2ba"
	lpPoolAddr         = "0xc181c3a4d09444b99089ae0f56c1e7f4c20d01eb"
	ubiPoolAddr        = "0x4a9b8f99f0d8cff0e510fef502100571203b054a"
	treasuryPoolAddr   = "0x2273894fb781978d54e767f9fba2dcb33d93eb15"
)

// swapFeeBps is the fee taken from every swap's input amount, in basis
// points (10 = 0.1%). This ONLY applies to swaps through the AEQ<->tUSD
// pool.
//
// FIX (P2-10, beta-launch audit 2026-07-05): this comment used to also
// claim "ordinary AEQ-to-AEQ transfers via Transfer() above remain
// completely free" — true only for Transfer()/transferLocked, the
// internal, system-only path used for crediting UBI/validator/LP
// distributions. It does NOT describe what a real user's MetaMask "send
// AEQ" actually costs: that goes through TransferWithV7Fee/calcV7Fee (see
// their own comments), which charges the same 0.1% base plus a
// concentration surcharge. There is no fee-less way for one human to send
// AEQ to another through this chain's actual UI/RPC surface today.
const swapFeeBps = 10

// SwapAEQForTUSD swaps `amountIn` AEQ from `address` into tUSD, using the
// constant-product formula (reserveAEQ * reserveTUSD = k) for pricing. A
// 0.1% fee is deducted from amountIn before the swap math runs, and is
// distributed across the four tokenomics pools (see DistributeSwapFee)
// rather than added to the liquidity pool's reserves — so the pool's own
// k grows only from genesis seeding, not from accumulated fees, keeping
// the fee-distribution logic in one place instead of split between the
// pool and the four-way split.
func (cs *ChainState) SwapAEQForTUSD(address string, amountIn, minAmountOut float64) (float64, float64, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// context.Background() is correct here, not a placeholder: this path
	// does not go through runAtomicWithOutbox, so there is no active
	// transaction to join, same reasoning as Transfer's own comment.
	return cs.swapLocked(context.Background(), address, amountIn, true, minAmountOut)
}

// SwapTUSDForAEQ swaps `amountIn` tUSD from `address` into AEQ. Same
// constant-product pricing and fee handling as SwapAEQForTUSD, just with
// the two reserves' roles reversed.
func (cs *ChainState) SwapTUSDForAEQ(address string, amountIn, minAmountOut float64) (float64, float64, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// context.Background() is correct here, not a placeholder — see
	// SwapAEQForTUSD's own comment.
	return cs.swapLocked(context.Background(), address, amountIn, false, minAmountOut)
}

// SwapAtomic behaves like SwapAEQForTUSD/SwapTUSDForAEQ, except the state
// mutation and the resulting outbox insert commit or roll back together as
// one DB transaction — see TransferAtomic's comment. pendingTxTemplate
// should have Type/Wallet/Amount set; AmountOut and FromDemurrageLost are
// filled in here from the swap's actual result.
func (cs *ChainState) SwapAtomic(address string, amountIn float64, aeqToTusd bool, minAmountOut float64, pendingTxTemplate Transaction) (amountOut, demurrageLost float64, err error) {
	address = strings.ToLower(address)
	err = cs.runAtomicWithOutbox([]string{address, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr}, false, func(ctx context.Context) (Transaction, error) {
		amountOut, demurrageLost, err = cs.swapLocked(ctx, address, amountIn, aeqToTusd, minAmountOut)
		if err != nil {
			return Transaction{}, err
		}
		pendingTxTemplate.AmountOut = amountOut
		pendingTxTemplate.FromDemurrageLost = demurrageLost
		return pendingTxTemplate, nil
	})
	return amountOut, demurrageLost, err
}

// swapLocked implements both swap directions. aeqToTusd=true means AEQ is
// the input side and tUSD is the output side; false is the reverse.
// minAmountOut, if > 0, rejects the swap before any state is mutated when
// the computed output would fall below it (slippage protection) — 0 means
// no protection requested.
// Caller must hold cs.mu. Returns (amountOut, demurrageLost, err) — lost
// must be attached to the queued Transaction so secondary nodes replay the
// exact decay via ApplySwapDelta instead of recomputing it themselves.
// swapLocked takes ctx explicitly rather than through the old/new wrapper
// split (see dbExecCtx's own comment) — like transferLocked, it has few
// enough callers (SwapAEQForTUSD, SwapTUSDForAEQ, SwapAtomic) to migrate
// together in this same change.
func (cs *ChainState) swapLocked(ctx context.Context, address string, amountIn float64, aeqToTusd bool, minAmountOut float64) (float64, float64, error) {
	// P2-7: reload pool from DB before swap to avoid stale-memory AMM invariant violation
	cs.reloadPoolFromDB()
	address = strings.ToLower(address)
	if amountIn <= 0 {
		return 0, 0, fmt.Errorf("amount must be positive")
	}
	if cs.pool == nil {
		return 0, 0, fmt.Errorf("liquidity pool not initialized")
	}

	cs.ensureAccountLoadedCtx(ctx, address) // page in cold accounts so swaps work beyond the in-memory cap
	acc, ok := cs.accounts.Get(address)
	if !ok {
		return 0, 0, fmt.Errorf("account not found")
	}
	lost, err := cs.settleDemurrageLockedCtx(ctx, acc) // settle decay before checking/using the AEQ balance below
	if err != nil {
		return 0, 0, fmt.Errorf("could not settle demurrage: %w", err)
	}

	if aeqToTusd {
		if acc.Balance.Float() < amountIn {
			return 0, 0, fmt.Errorf("insufficient AEQ balance")
		}
	} else {
		if acc.TUsdBalance.Float() < amountIn {
			return 0, 0, fmt.Errorf("insufficient tUSD balance")
		}
	}

	// Fee is taken off the top of the input amount; only the remainder
	// participates in the constant-product swap.
	fee := amountIn * float64(swapFeeBps) / 10000.0
	amountInAfterFee := amountIn - fee

	var amountOut float64
	if aeqToTusd {
		// x*y=k: reserveAEQ * reserveTUSD = (reserveAEQ + amountInAfterFee) * (reserveTUSD - amountOut)
		amountOut = AMMSwapOut(cs.pool.ReserveAEQ, cs.pool.ReserveTUSD, NewDecimal(amountInAfterFee)).Float()
		if amountOut >= cs.pool.ReserveTUSD.Float() {
			return 0, 0, fmt.Errorf("swap too large for pool liquidity")
		}
		if minAmountOut > 0 && amountOut < minAmountOut {
			return 0, 0, fmt.Errorf("slippage: output %.6f tUSD below requested minimum %.6f", amountOut, minAmountOut)
		}
		cs.pool.ReserveAEQ = cs.pool.ReserveAEQ.Add(NewDecimal(amountInAfterFee))
		cs.pool.ReserveTUSD = cs.pool.ReserveTUSD.Sub(NewDecimal(amountOut)).AtLeastZero()
		acc.Balance = acc.Balance.Sub(NewDecimal(amountIn))
		acc.TUsdBalance = acc.TUsdBalance.Add(NewDecimal(amountOut))
	} else {
		amountOut = AMMSwapOut(cs.pool.ReserveTUSD, cs.pool.ReserveAEQ, NewDecimal(amountInAfterFee)).Float()
		if amountOut >= cs.pool.ReserveAEQ.Float() {
			return 0, 0, fmt.Errorf("swap too large for pool liquidity")
		}
		if minAmountOut > 0 && amountOut < minAmountOut {
			return 0, 0, fmt.Errorf("slippage: output %.6f AEQ below requested minimum %.6f", amountOut, minAmountOut)
		}
		cs.pool.ReserveTUSD = cs.pool.ReserveTUSD.Add(NewDecimal(amountInAfterFee))
		cs.pool.ReserveAEQ = cs.pool.ReserveAEQ.Sub(NewDecimal(amountOut)).AtLeastZero()
		acc.TUsdBalance = acc.TUsdBalance.Sub(NewDecimal(amountIn))
		acc.Balance = acc.Balance.Add(NewDecimal(amountOut))
	}
	touchActivity(acc) // swapping (either direction) counts as using the AEQ side
	if !aeqToTusd {
		if err := cs.enforceWealthCapLockedCtx(ctx, acc); err != nil { // AEQ just arrived via this swap direction — check the cap
			return 0, 0, fmt.Errorf("could not enforce wealth cap: %w", err)
		}
	}

	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		return 0, 0, fmt.Errorf("could not save account: %w", err)
	}
	if err := cs.savePoolToDBCtx(ctx); err != nil {
		return 0, 0, fmt.Errorf("could not save pool: %w", err)
	}
	if err := cs.distributeSwapFeeCtx(ctx, fee, aeqToTusd); err != nil {
		return 0, 0, fmt.Errorf("could not persist swap fee distribution: %w", err)
	}
	cs.save()

	fmt.Printf("[SWAP] %s: %.4f %s → %.4f %s (fee %.4f)\n",
		address, amountIn, sideLabel(aeqToTusd, true), amountOut, sideLabel(aeqToTusd, false), fee)

	cs.syncBalanceLocked(V7_CONTRACT_ADDR, address, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr)
	SafeGoroutine("SavePriceSnapshot", cs.SavePriceSnapshot)
	return amountOut, lost.Float(), nil
}

func sideLabel(aeqToTusd, isInput bool) string {
	if aeqToTusd == isInput {
		return "AEQ"
	}
	return "tUSD"
}

// savePoolToDB persists cs.pool and returns an error on failure — see
// saveAccountToDB's comment (audit3, P1 #4) for why this now returns
// error and which callers are expected to actually check it.
func (cs *ChainState) savePoolToDB() error {
	return cs.savePoolToDBCtx(context.Background())
}

// savePoolToDBCtx is savePoolToDB's real implementation — see dbExecCtx's
// comment for the migration this is part of.
func (cs *ChainState) savePoolToDBCtx(ctx context.Context) error {
	if !cs.useDB || cs.pool == nil {
		return nil
	}
	// FIX (atomic outbox): if the current operation has an active
	// transaction open, use it directly instead of starting a SEPARATE one
	// via cs.db.Begin() — that would open an independent connection-level
	// transaction with no relationship to it, defeating the point (this
	// write needs to commit/rollback together with the rest of the
	// operation, not on its own), and risking a self-deadlock if both ever
	// needed the same row lock concurrently. The outer transaction already
	// provides the serialization the SELECT FOR UPDATE below exists for, so
	// skip that dance entirely here.
	if tx := cs.activeTxCtx(ctx); tx != nil {
		if _, err := tx.Exec(`UPDATE liquidity_pool SET reserve_aeq = $1, reserve_tusd = $2, total_lp_shares = $3 WHERE id = 1`,
			cs.pool.ReserveAEQ.Float(), cs.pool.ReserveTUSD.Float(), cs.pool.TotalLPShares.Float()); err != nil {
			fmt.Printf("[DB] Error saving pool inside active transaction: %v\n", err)
			return fmt.Errorf("could not save pool inside active transaction: %w", err)
		}
		return nil
	}
	// Use a transaction so concurrent pool writes are serialized at the DB level.
	// This prevents two nodes from simultaneously distributing UBI or running swaps
	// with stale pool reserves. The WHERE id = 1 ensures we update the single pool row.
	tx, err := cs.db.Begin()
	if err != nil {
		fmt.Printf("[DB] Error starting pool tx: %v\n", err)
		return fmt.Errorf("could not start pool tx: %w", err)
	}
	// Lock the pool row for this transaction (other writers block until we commit)
	var dummy int
	tx.QueryRow(`SELECT id FROM liquidity_pool WHERE id = 1 FOR UPDATE`).Scan(&dummy)
	_, err = tx.Exec(`UPDATE liquidity_pool SET reserve_aeq = $1, reserve_tusd = $2, total_lp_shares = $3 WHERE id = 1`,
		cs.pool.ReserveAEQ.Float(), cs.pool.ReserveTUSD.Float(), cs.pool.TotalLPShares.Float())
	if err != nil {
		tx.Rollback()
		fmt.Printf("[DB] Error saving pool: %v\n", err)
		return fmt.Errorf("could not save pool: %w", err)
	}
	if err := tx.Commit(); err != nil {
		fmt.Printf("[DB] Error committing pool: %v\n", err)
		return fmt.Errorf("could not commit pool tx: %w", err)
	}
	return nil
}

// reloadPoolFromDB loads the current pool state from PostgreSQL so swap
// operations always start from the authoritative DB state, not stale memory.
//
// FIX (P2-3, beta-launch audit 2026-07-05): this comment used to claim a
// "SELECT FOR UPDATE" row lock, but the query below has never had one — a
// row lock only holds meaning inside a transaction anyway, and this call
// goes through cs.db directly, never cs.dbExec()/an active transaction.
// What actually prevents an AMM invariant violation is cs.mu: each node
// runs its own separate PostgreSQL instance (no shared DB across nodes —
// see this repo's architecture notes), so there is exactly one writer
// process per database, serialized by the Go-level mutex, not a DB-level
// lock. A future change that relaxed cs.mu around a swap without adding a
// real transactional FOR UPDATE here would silently reopen the exact race
// this comment used to claim was already closed.
func (cs *ChainState) reloadPoolFromDB() {
	if cs.db == nil || cs.pool == nil {
		return
	}
	var aeq, tusd, lp float64
	err := cs.db.QueryRow(`SELECT reserve_aeq, reserve_tusd, total_lp_shares FROM liquidity_pool WHERE id = 1`).
		Scan(&aeq, &tusd, &lp)
	if err == nil {
		cs.pool.ReserveAEQ = NewDecimal(aeq)
		cs.pool.ReserveTUSD = NewDecimal(tusd)
		cs.pool.TotalLPShares = NewDecimal(lp)
	}
}

// convertTUsdFeeToAEQLocked swaps a tUSD-denominated swap fee into AEQ through
// the liquidity pool, so the four tokenomics pools only ever accumulate the AEQ
// they actually pay out (see distributeSwapFee). It mirrors a tUSD->AEQ swap:
// the fee's tUSD enters the pool's tUSD reserve and the corresponding AEQ leaves
// its AEQ reserve. Fee-less — the user's swap already paid the 0.1%. Returns the
// AEQ amount and true on success, or (0,false) if the pool can't price it (nil
// or empty pool, or an output that would drain the AEQ reserve) so the caller
// can fall back to crediting the raw tUSD. Deterministic given the reserves,
// which are in the identical post-swap state on the primary (swapLocked) and on
// secondaries (applySwapDeltaLocked) at the point this runs — so it needs no
// separate replay transaction. Caller holds cs.mu and persists the pool after.
func (cs *ChainState) convertTUsdFeeToAEQLocked(feeTUsd float64) (float64, bool) {
	if cs.pool == nil || feeTUsd <= 0 {
		return 0, false
	}
	rT := cs.pool.ReserveTUSD.Float()
	rA := cs.pool.ReserveAEQ.Float()
	if rT <= 0 || rA <= 0 {
		return 0, false
	}
	aeqOut := AMMSwapOut(cs.pool.ReserveTUSD, cs.pool.ReserveAEQ, NewDecimal(feeTUsd)).Float()
	if aeqOut <= 0 || aeqOut >= rA {
		return 0, false // can't price it, or it would drain the pool — fall back to tUSD
	}
	cs.pool.ReserveTUSD = cs.pool.ReserveTUSD.Add(NewDecimal(feeTUsd))
	cs.pool.ReserveAEQ = cs.pool.ReserveAEQ.Sub(NewDecimal(aeqOut)).AtLeastZero()
	return round6(aeqOut), true
}

// liquidateLPSharesForEscrowLocked burns up to sharesToBurn LP shares from
// acc into Balance+TUsdBalance proportional to the pool's current reserves
// — the same math as removeLiquidityLocked's normal-case branch, EXCEPT it
// deliberately does not call touchActivity: that call exists in
// removeLiquidityLocked because it's a voluntary user action, but this
// helper exists specifically for checkAndMoveToEscrowLocked's involuntary,
// inactivity-triggered sweep — resetting LastActivityAt there would erase
// the very inactivity the sweep exists to act on.
//
// Shared by both checkAndMoveToEscrowLocked (primary, burning everything an
// inactive human holds) and applyEscrowMoveDeltaLocked (secondary replay,
// burning the exact amount — an input, like RemoveLiquidityDelta's
// sharesToBurn — the primary already computed) specifically so the two can
// never independently drift into computing a different result from what
// should be the same starting pool state; a shared implementation makes
// that structurally impossible instead of relying on two hand-written copies
// staying in sync. Caller holds cs.mu and mustn't call this outside that.
//
// Returns the actual shares burned (clamped to TotalLPShares, 0 if the pool
// has no shares or sharesToBurn<=0) and the AEQ/tUSD credited. Persists the
// pool via savePoolToDB before returning.
func (cs *ChainState) liquidateLPSharesForEscrowLocked(ctx context.Context, acc *AccountState, sharesToBurn float64) (burned, outAEQ, outTUSD float64, err error) {
	if sharesToBurn <= 0 || cs.pool == nil || cs.pool.TotalLPShares.Float() <= 0 {
		return 0, 0, 0, nil
	}
	burned = sharesToBurn
	if burned > cs.pool.TotalLPShares.Float() {
		burned = cs.pool.TotalLPShares.Float()
	}
	fraction := burned / cs.pool.TotalLPShares.Float()
	outAEQ = cs.pool.ReserveAEQ.Float() * fraction
	outTUSD = cs.pool.ReserveTUSD.Float() * fraction
	if outAEQ > cs.pool.ReserveAEQ.Float() {
		outAEQ = cs.pool.ReserveAEQ.Float()
	}
	if outTUSD > cs.pool.ReserveTUSD.Float() {
		outTUSD = cs.pool.ReserveTUSD.Float()
	}
	acc.LPShares = acc.LPShares.Sub(NewDecimal(burned)).AtLeastZero()
	acc.Balance = acc.Balance.Add(NewDecimal(outAEQ))
	acc.TUsdBalance = acc.TUsdBalance.Add(NewDecimal(outTUSD))
	cs.pool.ReserveAEQ = cs.pool.ReserveAEQ.Sub(NewDecimal(outAEQ)).AtLeastZero()
	cs.pool.ReserveTUSD = cs.pool.ReserveTUSD.Sub(NewDecimal(outTUSD)).AtLeastZero()
	cs.pool.TotalLPShares = cs.pool.TotalLPShares.Sub(NewDecimal(burned)).AtLeastZero()
	if err := cs.savePoolToDBCtx(ctx); err != nil {
		return burned, outAEQ, outTUSD, fmt.Errorf("could not save pool after LP liquidation: %w", err)
	}
	return burned, outAEQ, outTUSD, nil
}

// convertTUsdForEscrowLocked converts up to tusdAmount of acc's TUsdBalance
// into AEQ via the pool's standard AMM swap math (paying the normal
// swapFeeBps fee, distributed the normal way via distributeSwapFee) — shared
// by checkAndMoveToEscrowLocked and applyEscrowMoveDeltaLocked for the same
// determinism reason as liquidateLPSharesForEscrowLocked above.
//
// Returns (0, false, nil) without changing anything if tusdAmount<=0, the
// pool lacks either reserve, or converting would drain the AEQ reserve —
// callers decide what a false ok means for them: the primary simply doesn't
// convert this cycle (retried on the next daily sweep), while a secondary
// replaying a primary-reported nonzero conversion must treat false as a hard
// pool-state-divergence error, not a silent skip. Caller holds cs.mu.
func (cs *ChainState) convertTUsdForEscrowLocked(ctx context.Context, acc *AccountState, tusdAmount float64) (outAEQ float64, ok bool, err error) {
	if tusdAmount <= 0 || cs.pool == nil || cs.pool.ReserveAEQ.Float() <= 0 || cs.pool.ReserveTUSD.Float() <= 0 {
		return 0, false, nil
	}
	fee := tusdAmount * float64(swapFeeBps) / 10000.0
	amountInAfterFee := tusdAmount - fee
	outAEQ = AMMSwapOut(cs.pool.ReserveTUSD, cs.pool.ReserveAEQ, NewDecimal(amountInAfterFee)).Float()
	if outAEQ >= cs.pool.ReserveAEQ.Float() {
		return 0, false, nil
	}
	cs.pool.ReserveTUSD = cs.pool.ReserveTUSD.Add(NewDecimal(amountInAfterFee))
	cs.pool.ReserveAEQ = cs.pool.ReserveAEQ.Sub(NewDecimal(outAEQ)).AtLeastZero()
	acc.TUsdBalance = acc.TUsdBalance.Sub(NewDecimal(tusdAmount)).AtLeastZero()
	acc.Balance = acc.Balance.Add(NewDecimal(outAEQ))
	if err := cs.savePoolToDBCtx(ctx); err != nil {
		return outAEQ, true, fmt.Errorf("could not save pool after tUSD conversion: %w", err)
	}
	if err := cs.distributeSwapFeeCtx(ctx, fee, false); err != nil {
		return outAEQ, true, fmt.Errorf("could not distribute swap fee: %w", err)
	}
	return outAEQ, true, nil
}

// distributeSwapFee splits the fee collected from a swap across the four
// tokenomics pools from the original design: 40% validators, 30% liquidity
// providers, 20% UBI, 10% treasury — crediting each of the four real addresses
// above. feeInAEQ is true when the fee was collected in AEQ (an AEQ->tUSD swap).
// A fee collected in tUSD (feeInAEQ=false, a tUSD->AEQ buy) is first converted
// to AEQ via convertTUsdFeeToAEQLocked so the pools only ever hold the AEQ they
// distribute — without this, tUSD fees pile up in pool addresses that
// DistributeUBIPool/Validators/LP never read (they look only at the AEQ
// balance), so the money is stranded and the pools look permanently empty
// (2026-07-02: confirmed live, UBI pool held 0.2 tUSD / 0 AEQ). The conversion
// runs identically on the primary and on replaying secondaries (identical
// post-swap reserves), so it needs no separate transaction; if the pool can't
// price the fee it falls back to crediting the raw tUSD, also deterministic.
// Caller must hold cs.mu.
func (cs *ChainState) distributeSwapFee(fee float64, feeInAEQ bool) error {
	return cs.distributeSwapFeeCtx(context.Background(), fee, feeInAEQ)
}

// distributeSwapFeeCtx is distributeSwapFee's real implementation — see
// dbExecCtx's comment for the migration this is part of.
func (cs *ChainState) distributeSwapFeeCtx(ctx context.Context, fee float64, feeInAEQ bool) error {
	if fee <= 0 {
		return nil
	}
	if !feeInAEQ {
		if aeqFee, ok := cs.convertTUsdFeeToAEQLocked(fee); ok {
			fee = aeqFee
			feeInAEQ = true
			if err := cs.savePoolToDBCtx(ctx); err != nil {
				return fmt.Errorf("distributeSwapFee: could not persist fee conversion: %w", err)
			}
		}
	}
	shares := [4]struct {
		addr   string
		amount float64
	}{
		{validatorsPoolAddr, fee * 0.40},
		{lpPoolAddr, fee * 0.30},
		{ubiPoolAddr, fee * 0.20},
		{treasuryPoolAddr, fee * 0.10},
	}
	accs := make([]*AccountState, len(shares))
	for i, s := range shares {
		// FIX (Monster Audit 2026-07-12, P1): see applyTransferDeltaLocked's
		// ubiContrib comment above — a cold pool address must be loaded from
		// the DB before it's touched, or a fresh Version==0 AccountState here
		// blindly overwrites its real, previously-accumulated DB balance.
		cs.ensureAccountLoaded(s.addr)
		sAcc, ok := cs.accounts.Get(s.addr)
		if !ok {
			sAcc = &AccountState{Address: s.addr}
			cs.accounts.Set(s.addr, sAcc)
		}
		if feeInAEQ {
			sAcc.Balance = sAcc.Balance.Add(NewDecimal(s.amount))
		} else {
			sAcc.TUsdBalance = sAcc.TUsdBalance.Add(NewDecimal(s.amount))
		}
		// Eager, in-memory-only state-root update — see the Phase 3 comment
		// below for why this must happen here regardless of whether the DB
		// write itself is synchronous (below) or deferred. Redundant-but-
		// harmless (a self-canceling no-op) on the no-DB path, where
		// saveAccountsToDBBatch's own !cs.useDB branch also calls this.
		cs.updateAccountLeafLocked(sAcc)
		accs[i] = sAcc
	}
	// SCALING_ARCHITECTURE.md Phase 3: with a real DB, defer the actual
	// Postgres write for these 4 pool rows to a periodic background flush
	// instead of paying a synchronous round trip on every single fee event
	// (this used to be the second DB write of nearly every transfer that
	// had any demurrage to settle). StateRoot correctness does NOT depend
	// on this write happening synchronously — updateAccountLeafLocked just
	// above already folded each pool's new balance into cs.accountSetXOR
	// eagerly, in-memory, at the exact same logical point every node
	// applies this same mutation, so every node's StateRoot stays
	// consistent regardless of each node's own local flush timing. Only
	// the DURABILITY of the pool balance to Postgres is now "eventually
	// consistent" (bounded by poolFlushInterval) -- see pool_flush.go.
	// Skipped in no-DB mode: saveAccountsToDBBatch's own !cs.useDB branch
	// is already a cheap in-memory-only no-op there, so deferring it would
	// add complexity for zero benefit and would change no-DB unit tests'
	// long-established "pool balance visible immediately" assertions.
	if cs.useDB {
		cs.markPoolAccountsDirtyLocked()
	} else if err := cs.saveAccountsToDBBatchCtx(ctx, accs); err != nil {
		return fmt.Errorf("distributeSwapFee: could not persist pool credits: %w", err)
	}
	currency := "tUSD"
	if feeInAEQ {
		currency = "AEQ"
	}
	fmt.Printf("[FEE] Swap fee %.6f %s distributed across validators/lps/ubi/treasury\n", fee, currency)
	return nil
}

// MigrateStrandedPoolTUsdFeesV1 is a one-time cleanup for tUSD that piled up
// in the four fee-distribution pool addresses from swaps that predate
// distributeSwapFee's tUSD->AEQ conversion step (convertTUsdFeeToAEQLocked)
// above: a tUSD->AEQ swap's fee used to credit the pool address directly in
// tUSD, with no conversion, leaving balances that DistributeUBIPool/
// Validators/LP never read (they only look at the AEQ balance) — see
// distributeSwapFee's own comment. Confirmed live 2026-07-03: validators
// 0.4, LP 0.3, UBI 0.2, treasury 0.1 tUSD, 0 AEQ each — the exact 40/30/20/10
// split, confirming this is old stranded swap-fee residue, not user funds.
//
// Safe to call on every startup: each pool's CURRENT TUsdBalance is read
// fresh and converted via the same AMM math distributeSwapFee already uses
// for ongoing fees, then zeroed — so a pool with nothing stranded (already
// converted, or never had any) is a no-op, and there is no way to
// double-convert even if this runs more than once (e.g. the chain_config
// flag write below fails after a real conversion already succeeded).
// Produces identical AEQ amounts on every node with the same reserves at
// call time (same reasoning as convertTUsdFeeToAEQLocked's own comment), so
// it needs no separate replicated transaction — each node converges to the
// same result independently.
func (cs *ChainState) MigrateStrandedPoolTUsdFeesV1() {
	const flagKey = "migrated_stranded_pool_tusd_fees_v1"
	if cs.getConfigValueDB(flagKey) == "1" {
		return
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.reloadPoolFromDB()
	converted := 0
	for _, addr := range []string{validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr} {
		cs.ensureAccountLoaded(addr)
		acc, ok := cs.accounts.Get(addr)
		if !ok {
			continue
		}
		stranded := acc.TUsdBalance.Float()
		if stranded <= 0 {
			continue
		}
		aeqOut, ok := cs.convertTUsdFeeToAEQLocked(stranded)
		if !ok {
			fmt.Printf("[MIGRATE] ✗ Could not convert %.6f stranded tUSD for %s (pool too shallow to price it) — left as-is, will retry next restart\n", stranded, addr)
			continue
		}
		acc.TUsdBalance = acc.TUsdBalance.Sub(NewDecimal(stranded))
		acc.Balance = acc.Balance.Add(NewDecimal(aeqOut))
		if err := cs.saveAccountToDB(acc); err != nil {
			fmt.Printf("[MIGRATE] ✗ Could not persist converted balance for %s: %v\n", addr, err)
			continue
		}
		converted++
		fmt.Printf("[MIGRATE] ✓ Converted %.6f stranded tUSD -> %.6f AEQ for %s\n", stranded, aeqOut, addr)
	}
	if err := cs.savePoolToDB(); err != nil {
		fmt.Printf("[MIGRATE] ✗ Could not persist pool after stranded-tUSD conversion: %v — will retry next restart\n", err)
		return
	}
	if converted > 0 {
		cs.save()
	}
	if err := cs.setConfigValueDB(flagKey, "1"); err != nil {
		fmt.Printf("[MIGRATE] Could not set migration flag (converted %d/4 pools; harmless to retry, already-converted pools have TUsdBalance=0 so nothing double-converts): %v\n", converted, err)
	}
}

// AddLiquidity lets a real account deposit AEQ and tUSD into the pool in
// proportion to the pool's current ratio (or, if the pool is currently
// empty, at whatever ratio the depositor chooses — that first deposit
// sets the initial price). This is the ONLY way reserves enter the pool;
// there is no admin/genesis fill, since every AEQ here has to trace back
// to a real human's registration grant, consistent with "money exists
// because people exist."
//
// FIX (Monster Audit 2026-07-12, P3): this comment used to say LP shares
// weren't minted/tracked and a depositor had no on-chain claim to withdraw
// their share — stale; that gap was closed since. Deposits mint LP shares
// (proportional to the pool's current ratio, or math.Sqrt(amountAEQ*amountTUSD)
// for the first deposit into an empty pool — see addLiquidityLocked below),
// and RemoveLiquidityAtomic burns them back out proportionally.
// Returns the AEQ amount demurrage-decayed off address before the deposit
// (0 if none) — callers must attach this to the queued Transaction so
// secondary nodes replay the exact decay via AddLiquidityDelta instead of
// recomputing it themselves.
func (cs *ChainState) AddLiquidity(address string, amountAEQ, amountTUSD float64) (float64, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// context.Background() is correct here, not a placeholder — this path
	// does not go through runAtomicWithOutbox, same reasoning as Transfer's
	// own comment.
	return cs.addLiquidityLocked(context.Background(), address, amountAEQ, amountTUSD)
}

// AddLiquidityAtomic behaves like AddLiquidity, except the state mutation
// and the resulting outbox insert commit or roll back together as one DB
// transaction — see TransferAtomic's comment. pendingTxTemplate should
// have Type/Wallet/Amount(AEQ)/AmountOut(tUSD) set; LPShares and
// FromDemurrageLost are filled in here from the operation's actual result.
func (cs *ChainState) AddLiquidityAtomic(address string, amountAEQ, amountTUSD float64, pendingTxTemplate Transaction) (demurrageLost float64, err error) {
	address = strings.ToLower(address)
	err = cs.runAtomicWithOutbox([]string{address, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr}, false, func(ctx context.Context) (Transaction, error) {
		sharesBefore := 0.0
		if acc, ok := cs.accounts.Get(address); ok {
			sharesBefore = acc.LPShares.Float()
		}
		demurrageLost, err = cs.addLiquidityLocked(ctx, address, amountAEQ, amountTUSD)
		if err != nil {
			return Transaction{}, err
		}
		sharesAfterAcc, _ := cs.accounts.Get(address)
		sharesAfter := sharesAfterAcc.LPShares.Float()
		pendingTxTemplate.LPShares = sharesAfter - sharesBefore
		pendingTxTemplate.FromDemurrageLost = demurrageLost
		return pendingTxTemplate, nil
	})
	return demurrageLost, err
}

// addLiquidityLocked is AddLiquidity's implementation; caller must already
// hold cs.mu — see transferLocked's comment for why this split exists.
// Takes ctx explicitly (see dbExecCtx's own comment) rather than through
// the old/new wrapper split — few enough callers to migrate together.
func (cs *ChainState) addLiquidityLocked(ctx context.Context, address string, amountAEQ, amountTUSD float64) (float64, error) {
	address = strings.ToLower(address)

	if amountAEQ <= 0 || amountTUSD <= 0 {
		return 0, fmt.Errorf("both amounts must be positive")
	}
	if math.IsNaN(amountAEQ) || math.IsInf(amountAEQ, 0) || math.IsNaN(amountTUSD) || math.IsInf(amountTUSD, 0) {
		return 0, fmt.Errorf("invalid liquidity amounts")
	}
	if cs.pool == nil {
		return 0, fmt.Errorf("liquidity pool not initialized")
	}

	cs.ensureAccountLoadedCtx(ctx, address) // page in cold accounts so add-liquidity works beyond the in-memory cap
	acc, ok := cs.accounts.Get(address)
	if !ok {
		return 0, fmt.Errorf("account not found")
	}
	lost, err := cs.settleDemurrageLockedCtx(ctx, acc) // settle decay before checking/using the AEQ balance below
	if err != nil {
		return 0, fmt.Errorf("could not settle demurrage: %w", err)
	}
	if acc.Balance.Float() < amountAEQ {
		return 0, fmt.Errorf("insufficient AEQ balance")
	}
	if acc.TUsdBalance.Float() < amountTUSD {
		return 0, fmt.Errorf("insufficient tUSD balance")
	}

	// If the pool already has liquidity, require the deposit to roughly
	// match the existing ratio — an unbalanced deposit would otherwise
	// instantly shift the price, which is the same rule real AMMs enforce.
	var mintedShares float64
	if cs.pool.ReserveAEQ > 0 && cs.pool.ReserveTUSD > 0 {
		expectedTUSD := amountAEQ * (cs.pool.ReserveTUSD.Float() / cs.pool.ReserveAEQ.Float())
		tolerance := expectedTUSD * 0.003 // 0.3% slack — tighter than 1% to prevent price manipulation
		if amountTUSD < expectedTUSD-tolerance || amountTUSD > expectedTUSD+tolerance {
			return 0, fmt.Errorf("deposit ratio does not match pool ratio (expected ~%.4f tUSD for %.4f AEQ)", expectedTUSD, amountAEQ)
		}
		if cs.pool.TotalLPShares > 0 {
			// Proportional to the pool's existing size — same fraction of the
			// AEQ reserve as the fraction of total shares being minted, so an
			// LP's claim accurately tracks how much of the pool they actually
			// own (including any fees the pool has accumulated since genesis).
			mintedShares = (amountAEQ / cs.pool.ReserveAEQ.Float()) * cs.pool.TotalLPShares.Float()
		} else {
			// Pool has reserves but zero LP shares — legacy state from before
			// share-tracking was introduced. Only mint shares for the NEW
			// deposit via geometric mean; do NOT credit pre-existing reserves
			// to the depositor. Doing so would let anyone with a tiny deposit
			// claim practically the entire pool (a drain attack).
			mintedShares = math.Sqrt(amountAEQ * amountTUSD)
			fmt.Printf("[POOL] Pool had %.4f AEQ / %.4f tUSD with no LP shares recorded — minting %.6f shares for new deposit only\n",
				cs.pool.ReserveAEQ.Float(), cs.pool.ReserveTUSD.Float(), mintedShares)
		}
	} else {
		// First-ever deposit: shares = geometric mean of the two amounts
		// (standard Uniswap v2 bootstrap formula). Using sqrt(x*y) instead
		// of, say, just amountAEQ means the first depositor can't mint an
		// outsized number of shares simply by picking a lopsided ratio.
		mintedShares = math.Sqrt(amountAEQ * amountTUSD)
	}

	acc.Balance = acc.Balance.Sub(NewDecimal(amountAEQ))
	acc.TUsdBalance = acc.TUsdBalance.Sub(NewDecimal(amountTUSD))
	acc.LPShares = acc.LPShares.Add(NewDecimal(mintedShares))
	touchActivity(acc) // depositing into the pool counts as using the AEQ
	cs.pool.ReserveAEQ = cs.pool.ReserveAEQ.Add(NewDecimal(amountAEQ))
	cs.pool.ReserveTUSD = cs.pool.ReserveTUSD.Add(NewDecimal(amountTUSD))
	cs.pool.TotalLPShares = cs.pool.TotalLPShares.Add(NewDecimal(mintedShares))

	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		return 0, fmt.Errorf("could not save account: %w", err)
	}
	if err := cs.savePoolToDBCtx(ctx); err != nil {
		return 0, fmt.Errorf("could not save pool: %w", err)
	}
	cs.save()

	cs.syncBalanceLocked(V7_CONTRACT_ADDR, address)
	SafeGoroutine("SavePriceSnapshot", cs.SavePriceSnapshot)

	fmt.Printf("[POOL] ✓ %s added liquidity: %.4f AEQ + %.4f tUSD → %.6f LP shares\n", address, amountAEQ, amountTUSD, mintedShares)
	return lost.Float(), nil
}

// RemoveLiquidity burns sharesToBurn of address's LP shares and returns
// the corresponding proportional amount of both reserves to their
// balances. sharesToBurn must not exceed the account's own LPShares —
// an account can only withdraw its own claim, never another LP's.
// Returns (outAEQ, outTUSD, demurrageLost, err) — demurrageLost must be
// attached to the queued Transaction so secondary nodes replay the exact
// decay via RemoveLiquidityDelta instead of recomputing it themselves.
func (cs *ChainState) RemoveLiquidity(address string, sharesToBurn float64) (float64, float64, float64, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// context.Background() is correct here, not a placeholder — this path
	// does not go through runAtomicWithOutbox, same reasoning as Transfer's
	// own comment.
	return cs.removeLiquidityLocked(context.Background(), address, sharesToBurn)
}

// RemoveLiquidityAtomic behaves like RemoveLiquidity, except the state
// mutation and the resulting outbox insert commit or roll back together as
// one DB transaction — see TransferAtomic's comment. pendingTxTemplate
// should have Type/Wallet/Amount(=sharesToBurn) set; FromDemurrageLost is
// filled in here from the operation's actual result (RemoveLiquidityDelta,
// the replay-side counterpart, re-derives outAEQ/outTUSD from the
// secondary's own current pool state rather than replaying exact amounts,
// so those aren't part of the queued Transaction either today).
func (cs *ChainState) RemoveLiquidityAtomic(address string, sharesToBurn float64, pendingTxTemplate Transaction) (outAEQ, outTUSD, demurrageLost float64, err error) {
	address = strings.ToLower(address)
	err = cs.runAtomicWithOutbox([]string{address, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr}, false, func(ctx context.Context) (Transaction, error) {
		outAEQ, outTUSD, demurrageLost, err = cs.removeLiquidityLocked(ctx, address, sharesToBurn)
		if err != nil {
			return Transaction{}, err
		}
		pendingTxTemplate.FromDemurrageLost = demurrageLost
		return pendingTxTemplate, nil
	})
	return outAEQ, outTUSD, demurrageLost, err
}

// removeLiquidityLocked is RemoveLiquidity's implementation; caller must
// already hold cs.mu — see transferLocked's comment for why this split
// exists. Takes ctx explicitly (see dbExecCtx's own comment) rather than
// through the old/new wrapper split — few enough callers to migrate
// together.
func (cs *ChainState) removeLiquidityLocked(ctx context.Context, address string, sharesToBurn float64) (float64, float64, float64, error) {
	address = strings.ToLower(address)

	if sharesToBurn <= 0 {
		return 0, 0, 0, fmt.Errorf("shares must be positive")
	}
	if cs.pool == nil {
		return 0, 0, 0, fmt.Errorf("liquidity pool not initialized")
	}

	cs.ensureAccountLoadedCtx(ctx, address) // page in cold accounts so remove-liquidity works beyond the in-memory cap
	acc, ok := cs.accounts.Get(address)
	if !ok {
		return 0, 0, 0, fmt.Errorf("account not found")
	}
	// FIX: RemoveLiquidity previously never settled demurrage on the
	// withdrawing account, unlike AddLiquidity/Transfer/swapLocked — an idle
	// wealthy account could dodge decay indefinitely by periodically
	// removing/re-adding trivial liquidity amounts (touchActivity() below
	// resets the decay clock without the decay ever having been applied).
	lost, err := cs.settleDemurrageLockedCtx(ctx, acc)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("could not settle demurrage: %w", err)
	}

	// F17-BOUNDARY: If TotalLPShares rounds to 0 but the user still has LP shares
	// (dust rounding edge case), allow them to drain the entire pool — they are
	// the last LP and the pool is effectively theirs.
	if cs.pool.TotalLPShares <= 0 {
		if acc.LPShares.Float() > 0 {
			outAEQ := cs.pool.ReserveAEQ.Float()
			outTUSD := cs.pool.ReserveTUSD.Float()
			acc.LPShares = NewDecimal(0)
			acc.Balance = acc.Balance.Add(NewDecimal(outAEQ))
			acc.TUsdBalance = acc.TUsdBalance.Add(NewDecimal(outTUSD))
			touchActivity(acc)
			if err := cs.enforceWealthCapLockedCtx(ctx, acc); err != nil {
				return 0, 0, 0, fmt.Errorf("could not enforce wealth cap: %w", err)
			}
			cs.pool.ReserveAEQ = NewDecimal(0)
			cs.pool.ReserveTUSD = NewDecimal(0)
			cs.pool.TotalLPShares = NewDecimal(0)
			if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
				return 0, 0, 0, fmt.Errorf("could not save account: %w", err)
			}
			if err := cs.savePoolToDBCtx(ctx); err != nil {
				return 0, 0, 0, fmt.Errorf("could not save pool: %w", err)
			}
			cs.save()
			cs.syncBalanceLocked(V7_CONTRACT_ADDR, address)
			SafeGoroutine("SavePriceSnapshot", cs.SavePriceSnapshot)
			fmt.Printf("[POOL] ✓ %s drained final dust position → %.4f AEQ + %.4f tUSD\n", address, outAEQ, outTUSD)
			return outAEQ, outTUSD, lost.Float(), nil
		}
		return 0, 0, 0, fmt.Errorf("liquidity pool is empty")
	}

	if acc.LPShares.Float() < sharesToBurn {
		return 0, 0, 0, fmt.Errorf("insufficient LP shares (have %.6f, requested %.6f)", acc.LPShares.Float(), sharesToBurn)
	}
	// F17-FIX: guard against TotalLPShares corruption (< actual shares).
	// Capping fraction to 1.0 above prevents over-withdrawal from reserves,
	// but TotalLPShares -= sharesToBurn would go negative. Clamp sharesToBurn.
	if sharesToBurn > cs.pool.TotalLPShares.Float() {
		sharesToBurn = cs.pool.TotalLPShares.Float()
		if sharesToBurn <= 0 {
			return 0, 0, 0, fmt.Errorf("pool total LP shares is zero or negative")
		}
		// Zeroing acc.LPShares prevents phantom shares when the clamped
		// sharesToBurn is less than the user's recorded LPShares.
		acc.LPShares = NewDecimal(0)
		// P0-FIX: return immediately after the zero-out so we do NOT fall
		// through to the normal "acc.LPShares -= sharesToBurn" path below,
		// which would compute 0 - sharesToBurn = negative LP shares.
		fraction17 := sharesToBurn / cs.pool.TotalLPShares.Float()
		if fraction17 > 1.0 {
			fraction17 = 1.0
		}
		outAEQ17 := cs.pool.ReserveAEQ.Float() * fraction17
		outTUSD17 := cs.pool.ReserveTUSD.Float() * fraction17
		if outAEQ17 > cs.pool.ReserveAEQ.Float() {
			outAEQ17 = cs.pool.ReserveAEQ.Float()
		}
		if outTUSD17 > cs.pool.ReserveTUSD.Float() {
			outTUSD17 = cs.pool.ReserveTUSD.Float()
		}
		acc.Balance = acc.Balance.Add(NewDecimal(outAEQ17))
		acc.TUsdBalance = acc.TUsdBalance.Add(NewDecimal(outTUSD17))
		touchActivity(acc)
		if err := cs.enforceWealthCapLockedCtx(ctx, acc); err != nil {
			return 0, 0, 0, fmt.Errorf("could not enforce wealth cap: %w", err)
		}
		newResAEQ17 := round6(cs.pool.ReserveAEQ.Float() - outAEQ17)
		newResTUSD17 := round6(cs.pool.ReserveTUSD.Float() - outTUSD17)
		if newResAEQ17 < 0 {
			newResAEQ17 = 0
		}
		if newResTUSD17 < 0 {
			newResTUSD17 = 0
		}
		cs.pool.ReserveAEQ = NewDecimal(newResAEQ17)
		cs.pool.ReserveTUSD = NewDecimal(newResTUSD17)
		cs.pool.TotalLPShares = cs.pool.TotalLPShares.Sub(NewDecimal(sharesToBurn))
		if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
			return 0, 0, 0, fmt.Errorf("could not save account: %w", err)
		}
		if err := cs.savePoolToDBCtx(ctx); err != nil {
			return 0, 0, 0, fmt.Errorf("could not save pool: %w", err)
		}
		cs.save()
		cs.syncBalanceLocked(V7_CONTRACT_ADDR, address)
		SafeGoroutine("SavePriceSnapshot", cs.SavePriceSnapshot)
		fmt.Printf("[POOL] ✓ %s removed liquidity (F17 clamp): %.6f shares → %.4f AEQ + %.4f tUSD\n", address, sharesToBurn, outAEQ17, outTUSD17)
		return outAEQ17, outTUSD17, lost.Float(), nil
	}

	fraction := sharesToBurn / cs.pool.TotalLPShares.Float()
	if fraction > 1.0 {
		fraction = 1.0
	} // cap: TotalLPShares corruption guard
	outAEQ := cs.pool.ReserveAEQ.Float() * fraction
	outTUSD := cs.pool.ReserveTUSD.Float() * fraction
	if outAEQ > cs.pool.ReserveAEQ.Float() {
		outAEQ = cs.pool.ReserveAEQ.Float()
	}
	if outTUSD > cs.pool.ReserveTUSD.Float() {
		outTUSD = cs.pool.ReserveTUSD.Float()
	}

	acc.LPShares = acc.LPShares.Sub(NewDecimal(sharesToBurn))
	acc.Balance = acc.Balance.Add(NewDecimal(outAEQ))
	acc.TUsdBalance = acc.TUsdBalance.Add(NewDecimal(outTUSD))
	touchActivity(acc) // receiving AEQ back from the pool counts as using it
	if err := cs.enforceWealthCapLockedCtx(ctx, acc); err != nil {
		return 0, 0, 0, fmt.Errorf("could not enforce wealth cap: %w", err)
	}
	newReserveAEQ := round6(cs.pool.ReserveAEQ.Float() - outAEQ)
	newReserveTUSD := round6(cs.pool.ReserveTUSD.Float() - outTUSD)
	if newReserveAEQ < 0 {
		newReserveAEQ = 0
	}
	if newReserveTUSD < 0 {
		newReserveTUSD = 0
	}
	cs.pool.ReserveAEQ = NewDecimal(newReserveAEQ)
	cs.pool.ReserveTUSD = NewDecimal(newReserveTUSD)
	cs.pool.TotalLPShares = cs.pool.TotalLPShares.Sub(NewDecimal(sharesToBurn))

	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		return 0, 0, 0, fmt.Errorf("could not save account: %w", err)
	}
	if err := cs.savePoolToDBCtx(ctx); err != nil {
		return 0, 0, 0, fmt.Errorf("could not save pool: %w", err)
	}
	cs.save()

	cs.syncBalanceLocked(V7_CONTRACT_ADDR, address)
	SafeGoroutine("SavePriceSnapshot", cs.SavePriceSnapshot)

	fmt.Printf("[POOL] ✓ %s removed liquidity: %.6f shares → %.4f AEQ + %.4f tUSD\n", address, sharesToBurn, outAEQ, outTUSD)
	return outAEQ, outTUSD, lost.Float(), nil
}

// GetLPShares returns address's current LP share balance, and the pool's
// total shares — callers can compute the account's ownership fraction
// (and therefore its withdrawable amounts) from these two numbers.
func (cs *ChainState) GetLPShares(address string) (float64, float64) {
	var mine float64
	cs.readAccount(address, func(acc *AccountState) {
		mine = acc.LPShares.Float()
	})
	// cs.pool is guarded by the same lock; read it separately rather than
	// widening readAccount's contract to cover non-account state.
	cs.mu.RLock()
	total := 0.0
	if cs.pool != nil {
		total = cs.pool.TotalLPShares.Float()
	}
	cs.mu.RUnlock()
	return mine, total
}

func (cs *ChainState) TotalSupply() float64 {
	// Total supply is always exactly Humans × 1,000 AEQ by protocol design.
	// Each registered human receives exactly 1,000 AEQ upon registration —
	// no more, no less. Floating-point drift from swap fees and demurrage
	// calculations means the sum of all account balances + pool reserves
	// diverges slightly from this over time, so we compute it directly
	// from the human count instead of summing balances.
	//
	// FIX (performance audit 2026-07-06): this used to scan cs.accounts —
	// capped at maxInMemAccounts, so it would undercount at scale exactly
	// like accountSetXOR did before it was fixed the same way.
	// humanCountLocked (see its own comment) is seeded from a full DB scan
	// and kept current incrementally, so it's correct regardless of cache
	// size, with a live-scan fallback for DB-free deployments/tests.
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return float64(cs.humanCountLocked()) * 1000.0
}

// humanCountLocked returns the current registered-human count. Caller must
// hold cs.mu (read or write lock).
//
// With a real DB (cs.useDB), returns the maintained cs.humanCount field —
// see its own comment for why it's kept current incrementally rather than
// scanned on every call (the scan pattern was already found and fixed once
// for accountSetXOR: cs.accounts is capped at maxInMemAccounts, so counting
// only what's resident undercounts at scale).
//
// Without a DB (unit tests, or a genuinely DB-free deployment), falls back
// to a live scan of cs.accounts instead of trusting cs.humanCount. This
// isn't the scale workaround above — in DB-free mode nothing ever gets
// evicted from cs.accounts, so a live scan is always both cheap and exactly
// correct. It exists because cs.humanCount is only maintained through
// registerHumanLocked and the startup/resync reseed paths — a test that
// constructs an AccountState{IsHuman: true} directly (there are over a
// dozen in this package) bypasses both, and cs.humanCount would silently
// stay 0 with no error, exactly the class of drift bug this whole
// consolidation is meant to avoid. The live-scan fallback makes that
// bypass harmless instead of a trap for the next test author.
func (cs *ChainState) humanCountLocked() int64 {
	if cs.useDB {
		// humanCountMu: see that field's own comment.
		cs.humanCountMu.Lock()
		defer cs.humanCountMu.Unlock()
		return cs.humanCount
	}
	var count int64
	cs.accounts.Range(func(_ string, acc *AccountState) bool {
		if acc.IsHuman {
			count++
		}
		return true
	})
	return count
}

// TotalHumans returns the current registered-human count.
func (cs *ChainState) TotalHumans() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return int(cs.humanCountLocked())
}

// GetAllAccounts returns a COPY of each account, with Balance set to its
// live, demurrage-adjusted value (see effectiveBalance) — not the raw
// stored value, and not a pointer to the real account. Copies matter
// here: callers (the explorer's /api/humans, etc.) must never be able to
// mutate the actual stored balance just by displaying it, and showing
// the raw stored value would make the UI lag behind real decay until
// that specific account next did something that triggered
// settleDemurrageLocked.
func (cs *ChainState) GetAllAccounts() []*AccountState {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	result := make([]*AccountState, 0, cs.accounts.Len())
	cs.accounts.Range(func(_ string, acc *AccountState) bool {
		displayCopy := *acc
		displayCopy.Balance = effectiveBalance(acc)
		result = append(result, &displayCopy)
		return true
	})
	return result
}

// GetAccountsForAddresses is GetAllAccounts' targeted counterpart: returns
// the current (demurrage-adjusted) state for exactly the given addresses,
// paging in any cold account via ensureAccountLoaded rather than silently
// skipping it. Addresses not found anywhere (never registered, never
// received anything) are simply omitted from the result — the caller
// treats a missing entry as a zero balance, matching GetAllAccounts' own
// implicit behavior for an address absent from cs.accounts.
//
// FIX (G5, beta-launch audit 2026-07-05): added specifically so
// EVMEngine.newStateDB can load only the handful of accounts a given
// eth_call/transaction actually touches instead of every registered human
// via GetAllAccounts — see newStateDB's own comment for the full
// reasoning. A useful side effect: unlike GetAllAccounts (which only ever
// sees whatever happens to already be in the in-memory cache, with no DB
// fallback for a cold/evicted account), this one explicitly pages in each
// requested address, so it's strictly MORE correct for the specific
// addresses it's asked about, not just faster.
func (cs *ChainState) GetAccountsForAddresses(addrs []string) []*AccountState {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.getAccountsForAddressesLocked(addrs)
}

// GetAccountsForAddressesLocked is GetAccountsForAddresses' lock-free
// sibling — same "...Locked assumes cs.mu already held" convention used
// elsewhere in this file (see snapshotForRollbackLocked et al.).
//
// FIX (self-deadlock, found live on Contabo1/Contabo2 2026-07-11):
// replayTransactions holds cs.mu.Lock() for the ENTIRE replay (see that
// function's own comment on why), including its call to verifyZKProof for
// register_human transactions. verifyZKProof calls EVMEngine.CallContract
// to invoke the on-chain BioVerifier, which calls newStateDB, which used to
// call the public, self-locking GetAccountsForAddresses — the SAME
// goroutine trying to Lock() a mutex it already holds, deadlocking against
// itself forever. Every subsequent peer block merge, orphan resolution, and
// even /api/health request (anything needing cs.mu) then piles up behind
// it, since Go's sync.RWMutex blocks new readers once a writer is
// waiting/held. CallContractLocked/newStateDBLocked route through this
// method instead, closing the gap without touching any of the other
// (correctly unlocked) CallContract call sites in api.go/evm_rpc.go/
// register.go.
func (cs *ChainState) getAccountsForAddressesLocked(addrs []string) []*AccountState {
	result := make([]*AccountState, 0, len(addrs))
	seen := make(map[string]bool, len(addrs))
	unique := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		addr = strings.ToLower(addr)
		if seen[addr] {
			continue
		}
		seen[addr] = true
		unique = append(unique, addr)
	}
	cs.ensureAccountsLoaded(unique)
	for _, addr := range unique {
		acc, ok := cs.accounts.Get(addr)
		if !ok {
			continue
		}
		displayCopy := *acc
		displayCopy.Balance = effectiveBalance(acc)
		result = append(result, &displayCopy)
	}
	return result
}

// tusdFaucetAmount is how much test-tUSD ClaimTUsdFaucet grants per
// account, once. tUSD is a simulated currency with no real-world value —
// unlike AEQ (which only ever exists because a real human registered for
// it), there's no "money exists because people exist" principle being
// violated by handing test-tUSD out directly. This exists purely so a
// registered human has something to pair with their real AEQ the first
// time they call AddLiquidity, since otherwise nobody could ever provide
// the tUSD side of the very first deposit.
const tusdFaucetAmount = 1000.0

// ClaimTUsdFaucet grants tusdFaucetAmount of test-tUSD to address, once.
// Returns an error if the account isn't registered, or already claimed.
func (cs *ChainState) ClaimTUsdFaucet(address string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox — see
	// RegisterHuman's comment.
	return cs.claimTUsdFaucetLocked(context.Background(), address)
}

// ClaimTUsdFaucetAtomic behaves like ClaimTUsdFaucet, except the state
// mutation and the resulting outbox insert commit or roll back together as
// one DB transaction — see TransferAtomic's comment.
func (cs *ChainState) ClaimTUsdFaucetAtomic(address string, pendingTx Transaction) error {
	address = strings.ToLower(address)
	return cs.runAtomicWithOutbox([]string{address}, false, func(ctx context.Context) (Transaction, error) {
		if err := cs.claimTUsdFaucetLocked(ctx, address); err != nil {
			return Transaction{}, err
		}
		return pendingTx, nil
	})
}

// claimTUsdFaucetLocked is ClaimTUsdFaucet's implementation; caller must
// already hold cs.mu — see transferLocked's comment for why this split
// exists.
func (cs *ChainState) claimTUsdFaucetLocked(ctx context.Context, address string) error {
	address = strings.ToLower(address)
	cs.ensureAccountLoadedCtx(ctx, address) // a cold registered human must still be recognised as human here

	acc, ok := cs.accounts.Get(address)
	if !ok || !acc.IsHuman {
		return fmt.Errorf("only registered humans can claim the test-tUSD faucet")
	}
	if acc.FaucetClaimed {
		return fmt.Errorf("faucet already claimed")
	}

	acc.FaucetClaimed = true
	// P2-AUDIT: Add to existing balance instead of overwriting — a user who had
	// received tUSD via another path (pool payout, migration) before claiming the
	// faucet would have had their entire tUSD balance zeroed by the old Set.
	acc.TUsdBalance = acc.TUsdBalance.Add(NewDecimal(tusdFaucetAmount))
	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		return fmt.Errorf("could not save account: %w", err)
	}
	cs.save()

	fmt.Printf("[FAUCET] ✓ %s claimed %.2f test-tUSD\n", address, tusdFaucetAmount)
	return nil
}

// StateRoot computes a deterministic hash of ALL economically meaningful state:
// human AEQ balances, tUSD balances, LP shares, pool reserves, and nullifiers.
// Two states with the same root are guaranteed to be economically identical.
// Previously only human AEQ balances were included; this allowed different
// economic states to hash identically, defeating state-root verification.
func (cs *ChainState) StateRoot() string {
	// P1-1: read last_ubi_at from DB BEFORE acquiring the mutex to avoid
	// holding RLock across a blocking DB query (deadlock / latency risk).
	//
	// FIX (audit 2026-06-28 recheck 4, P0-1): this is exactly the call site
	// the audit flagged by name — getConfigValue's cs.dbExec() routing
	// meant this could read a DIFFERENT, concurrently-running atomic
	// operation's in-flight transaction (cs.activeTx, set/cleared only
	// under THAT operation's own cs.mu hold, which this read never
	// acquires). StateRoot is consensus-relevant: observing another
	// operation's uncommitted last_ubi_at here could produce a StateRoot
	// no replay could ever reproduce. getConfigValueDB always reads cs.db
	// directly, so under Postgres's read-committed isolation it only ever
	// sees the last value that was actually committed — never a
	// concurrent transaction's in-flight write, and never races on
	// cs.activeTx itself.
	lastUBIAt := cs.getConfigValueDB("last_ubi_at")
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.stateRootLocked(lastUBIAt)
}

// stateRootLocked is StateRoot's body, for callers that already hold cs.mu
// (audit recheck3, P0/P1 — replayTransactions holds cs.mu continuously
// across an entire block's snapshot/deltas/StateRoot-comparison instead of
// taking its own separate RLock here, which would deadlock against an
// exclusive Lock already held by the same goroutine). lastUBIAt is passed
// in rather than read here because getConfigValue's DB call should still
// happen before any lock is taken when called from the public StateRoot()
// — replayTransactions, which already holds cs.mu for unrelated writes by
// the time it needs this, accepts that one extra blocking DB read during
// its critical section, matching the same tradeoff every atomic operation
// in this file already makes (see runAtomicWithOutbox).
func (cs *ChainState) stateRootLocked(lastUBIAt string) string {
	// O(1) in the account/nullifier count: both sets are summarized by their
	// incremental XOR accumulators (see ChainState.accountSetXOR). The OLD
	// implementation iterated cs.accounts and cs.nullifiers here — correct
	// only while every account fits in memory, but cs.accounts is capped at
	// maxInMemAccounts (5M). Past that cap two honest nodes loaded different
	// subsets and hashed different roots for identical chain state, so the
	// root silently stopped being a consensus invariant exactly when the
	// network grew large enough to need one. accountSetXOR/nullifierSetXOR
	// commit to the FULL sets regardless of residency, and cost nothing to
	// read here.
	//
	// This is a DIFFERENT hash construction from the pre-accumulator nodes,
	// so during a mixed-version rollout upgraded and non-upgraded nodes will
	// log StateRoot mismatches against each other. That is safe: a mismatch
	// is a warning, never a block rejection (see AddPeerBlock) — the chain
	// keeps advancing on individually-verified TXs until every node runs this
	// code, at which point they agree again.
	var sb strings.Builder
	sb.WriteString("acctXOR:")
	sb.WriteString(hex.EncodeToString(cs.accountSetXOR[:]))
	// Include pool state: reserves and total LP shares (P2-10: integer atoms)
	if cs.pool != nil {
		fmt.Fprintf(&sb, "|pool:%d:%d:%d",
			cs.pool.ReserveAEQ.Micro(),
			cs.pool.ReserveTUSD.Micro(),
			cs.pool.TotalLPShares.Micro())
	}
	// Nullifier set commitment (keys only, never wallet addresses — privacy).
	sb.WriteString("|nullXOR:")
	sb.WriteString(hex.EncodeToString(cs.nullifierSetXOR[:]))
	// Include last UBI distribution timestamp (pre-fetched before RLock — P1-1).
	fmt.Fprintf(&sb, "|ubi:%s", lastUBIAt)
	hash := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(hash[:])
}

// StateRootComponents is the per-component breakdown of exactly what
// stateRootLocked hashes, in the same order it concatenates them. Purely
// diagnostic: it lets an operator compare two nodes that disagree on the
// StateRoot and see WHICH of the four inputs actually differs, instead of
// only that the final SHA256 does.
//
// Added 2026-07-25 for the persistent-divergence incident (see
// ANALYSE_STATEROOT_DIVERGENZ.md): all economically visible aggregates
// (total_supply, gini, every pool) matched bit-for-bit across all three
// nodes while the roots disagreed on every single block, which narrows the
// cause to a component with no aggregate of its own — but "narrows" is not
// "identifies", and the primary runs on Railway with no SSH/psql access, so
// the only way to read its side of the comparison is over HTTP.
//
// Deliberately exposes no wallet addresses and no per-account data: the two
// accumulators are already-public set commitments (every block header
// carries the root derived from them), the pool reserves are already in
// /api/status, and last_ubi_at is a timestamp.
type StateRootComponents struct {
	AccountSetXOR   string `json:"account_set_xor"`
	PoolReserveAEQ  int64  `json:"pool_reserve_aeq_micro"`
	PoolReserveTUSD int64  `json:"pool_reserve_tusd_micro"`
	PoolLPShares    int64  `json:"pool_lp_shares_micro"`
	NullifierSetXOR string `json:"nullifier_set_xor"`
	LastUBIAt       string `json:"last_ubi_at"`
	StateRoot       string `json:"state_root"`
}

// StateRootComponentBreakdown returns the same inputs stateRootLocked hashes,
// plus the resulting root, so two nodes can be diffed component by component.
// Reads last_ubi_at before taking the lock for exactly the reason StateRoot's
// own P1-1 comment describes.
func (cs *ChainState) StateRootComponentBreakdown() StateRootComponents {
	lastUBIAt := cs.getConfigValueDB("last_ubi_at")
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	out := StateRootComponents{
		AccountSetXOR:   hex.EncodeToString(cs.accountSetXOR[:]),
		NullifierSetXOR: hex.EncodeToString(cs.nullifierSetXOR[:]),
		LastUBIAt:       lastUBIAt,
		StateRoot:       cs.stateRootLocked(lastUBIAt),
	}
	if cs.pool != nil {
		out.PoolReserveAEQ = cs.pool.ReserveAEQ.Micro()
		out.PoolReserveTUSD = cs.pool.ReserveTUSD.Micro()
		out.PoolLPShares = cs.pool.TotalLPShares.Micro()
	}
	return out
}

// xorInto XORs src into dst in place — the combine step of both set accumulators.
func xorInto(dst *[32]byte, src [32]byte) {
	for i := 0; i < 32; i++ {
		dst[i] ^= src[i]
	}
}

// accountLeaf returns this account's contribution to accountSetXOR: a SHA256
// over exactly the fields the pre-accumulator StateRoot committed per account
// (lowercased address, micro-unit balances, human/faucet flags), or the zero
// hash when the account is excluded from the root (non-human with every
// balance zero). Because XOR is its own inverse, XORing an account's leaf in
// when it changes and its previous leaf out first keeps accountSetXOR equal to
// the XOR of every current account's leaf — no rescan needed.
//
// The inclusion rule matches the old code's "!=0" superset-of-">0" choice: a
// balance that should never go negative but somehow did is still committed,
// so such a bug stays visible to consensus rather than hashing identically to
// an absent account.
// FIX (2026-07-23, TPS-benchmark investigation): CPU profiling of the
// WAL-enabled ingestion path (see transfer_wal.go) found accountLeaf as one
// of the single hottest functions -- it runs twice per transfer (once per
// side) on every account mutation. The cost was almost entirely
// fmt.Sprintf's own overhead (reflection-based formatting, an intermediate
// string allocation before the final []byte(s) conversion for hashing), not
// the sha256 computation itself. Rewritten to build the identical byte
// sequence directly via strconv.AppendInt into a stack-local scratch array
// (same technique as writeDollarParam elsewhere in this file), skipping the
// intermediate string entirely.
//
// CONSENSUS-CRITICAL: the exact byte sequence fed to sha256.Sum256 below
// MUST remain byte-for-byte identical to the old Sprintf-built string, or
// every node computes a different accountLeaf for the same account state --
// a silent StateRoot fork, not just a local bug. TestAccountLeaf_GoldenValues
// (state_test.go) pins accountLeaf's output for representative inputs,
// captured from the old Sprintf implementation before this rewrite, and
// must keep passing unchanged.
func accountLeaf(acc *AccountState) [32]byte {
	if !(acc.IsHuman || acc.Balance != 0 || acc.TUsdBalance != 0 || acc.LPShares != 0) {
		return [32]byte{}
	}
	addr := strings.ToLower(acc.Address)
	var scratch [160]byte // generous for "acct:" + address + 3 int64s + ":h=false:fc=false"; append grows to heap if ever exceeded, still correct
	b := scratch[:0]
	b = append(b, "acct:"...)
	b = append(b, addr...)
	b = append(b, ':')
	b = strconv.AppendInt(b, acc.Balance.Micro(), 10)
	b = append(b, ':')
	b = strconv.AppendInt(b, acc.TUsdBalance.Micro(), 10)
	b = append(b, ':')
	b = strconv.AppendInt(b, acc.LPShares.Micro(), 10)
	b = append(b, ":h="...)
	if acc.IsHuman {
		b = append(b, "true"...)
	} else {
		b = append(b, "false"...)
	}
	b = append(b, ":fc="...)
	if acc.FaucetClaimed {
		b = append(b, "true"...)
	} else {
		b = append(b, "false"...)
	}
	return sha256.Sum256(b)
}

// nullifierLeaf returns a nullifier key's contribution to nullifierSetXOR.
// Keys are unique (PRIMARY KEY), so no two distinct nullifiers can cancel.
func nullifierLeaf(key string) [32]byte {
	return sha256.Sum256([]byte("null:" + key))
}

// rebuildStateAccumulators recomputes accountSetXOR and nullifierSetXOR from
// scratch and refreshes the cached leafHash on every resident account. Call it
// after any BULK change that bypasses the incremental hooks in saveAccountToDB
// and the nullifier helpers: initial DB load, snapshot import, resync, or a
// full reset. Caller must hold cs.mu (write).
//
// Accounts are summed from a full chain_accounts scan (NOT just the in-memory
// map) so the accumulator stays complete beyond maxInMemAccounts; leafHash is
// cached only for rows that happen to be resident. With no DB (unit tests) it
// falls back to the in-memory map, which is authoritative in that mode.
func (cs *ChainState) rebuildStateAccumulators() {
	var acc [32]byte
	var humans int64
	scanned := false
	if cs.db != nil {
		rows, err := cs.db.Query(`SELECT address, balance, is_human, tusd_balance, lp_shares, faucet_claimed FROM chain_accounts`)
		if err == nil {
			for rows.Next() {
				var addr string
				var bal, tusd, lp float64
				var human, faucet bool
				if scanErr := rows.Scan(&addr, &bal, &human, &tusd, &lp, &faucet); scanErr != nil {
					continue
				}
				lower := strings.ToLower(addr)
				tmp := &AccountState{Address: lower, Balance: NewDecimal(bal), IsHuman: human, TUsdBalance: NewDecimal(tusd), LPShares: NewDecimal(lp), FaucetClaimed: faucet}
				leaf := accountLeaf(tmp)
				xorInto(&acc, leaf)
				if human {
					humans++
				}
				if resident, ok := cs.accounts.Get(lower); ok {
					resident.leafHash = leaf
				}
			}
			rows.Close()
			scanned = true
		}
	}
	if !scanned {
		humans = 0
		cs.accounts.Range(func(_ string, a *AccountState) bool {
			leaf := accountLeaf(a)
			a.leafHash = leaf
			xorInto(&acc, leaf)
			if a.IsHuman {
				humans++
			}
			return true
		})
	}
	cs.accountSetXOR = acc
	// See humanCount's own field comment (TotalSupply's cheap source of
	// truth) — reseeded here in the exact same full-scan pass as
	// accountSetXOR, for the exact same reason: cs.accounts alone is capped
	// at maxInMemAccounts, so counting only what's resident would undercount
	// at scale exactly like the pre-fix accountSetXOR did.
	cs.humanCount = humans

	// Nullifiers: scan the whole table, not cs.nullifiers — that map is capped
	// at maxInMemNullifiers, so iterating it would miss keys beyond the cap and
	// leave the accumulator (hence the root) short exactly at scale. Fall back
	// to the in-memory map only when there is no DB (unit tests).
	var nul [32]byte
	nscanned := false
	if cs.db != nil {
		nrows, nerr := cs.db.Query(`SELECT nullifier FROM nullifiers`)
		if nerr == nil {
			for nrows.Next() {
				var k string
				if err := nrows.Scan(&k); err != nil || k == "" {
					continue
				}
				xorInto(&nul, nullifierLeaf(k))
			}
			nrows.Close()
			nscanned = true
		}
	}
	if !nscanned {
		for k := range cs.nullifiers {
			xorInto(&nul, nullifierLeaf(k))
		}
	}
	cs.nullifierSetXOR = nul
}

// calcGiniLocked computes the Gini coefficient without acquiring cs.mu.
// Must only be called while cs.mu is already held (read or write).
// calcGiniFromBalances is the single shared implementation used by both
// calcGiniLocked (inside lock) and CalcGini (acquires lock). P2-1: uses
// sort.Float64s O(n log n) instead of the old O(n^2) bubble sort.
func calcGiniFromBalances(balances []float64) float64 {
	n := len(balances)
	if n < 2 {
		return 0.0
	}
	sort.Float64s(balances)
	var sum, numerator float64
	for i, x := range balances {
		sum += x
		numerator += float64(2*i+1-n) * x
	}
	if sum == 0 {
		return 0.0
	}
	gini := numerator / (float64(n) * sum)
	if gini < 0 {
		gini = -gini
	}
	// This is the POPULATION Gini, deliberately without the ×n/(n-1) sample
	// correction that used to be applied here.
	//
	// That correction estimates the Gini of a large unknown population from a
	// small sample of it. This chain is not sampling anything: it knows every
	// registered human, so the balances above ARE the whole population. Applying
	// the correction inflated our own published figure by n/(n-1) — 7.1% at the
	// 15 humans live on 2026-08-15, 20% at n=6, and a full 2× at n=2, exactly
	// when the network is smallest and most scrutinised.
	//
	// Two things made the inflation actively wrong rather than merely debatable:
	//
	//   1. The landing page and explorer put this number directly beside World
	//      Bank / OECD country figures (Scandinavia 0.27, Germany 0.31, USA
	//      0.41, Bitcoin 0.85) and call it "the international standard, adopted
	//      by the World Bank, OECD and UN". Those are population Ginis. Comparing
	//      a sample-corrected number against them measured a different quantity
	//      and made Aequitas look less equal than it is.
	//
	//   2. The Lorenz curve is the population Gini stated geometrically. With the
	//      correction the curve and the coefficient printed next to it could never
	//      agree — measured live: curve 0.12361229 vs published 0.13244174, a
	//      ratio of exactly 15/14. humanAEQWealthLocked's comment records that
	//      this same class of curve-vs-coefficient divergence had already been
	//      chased down once from the other side.
	//
	// The old ×n/(n-1) also needed a >1.0 clamp, because it could push the value
	// past 1 for small n — a maximum that a Gini cannot exceed by definition. No
	// clamp is needed now: the population Gini is bounded by (n-1)/n < 1.
	return gini
}

// humanAEQWealthLocked is a human's total AEQ-denominated wealth: their liquid
// (demurrage-adjusted) balance PLUS the AEQ value of their liquidity-pool
// position (their share of the pool's AEQ reserve). AEQ deposited as liquidity
// still belongs to the human — they can withdraw it — so every wealth view must
// count it. This single definition is the source of truth for the Gini, the
// Aequitas Index, the Lorenz curve and the humans list; /api/humans exposes the
// same number as total_value_aeq so the frontend Lorenz matches the server Gini
// exactly (they used to disagree — 0.72 vs 0.15 — because the Gini silently
// dropped the two LP wallets, whose liquid balance is 0). Caller holds cs.mu.
func (cs *ChainState) humanAEQWealthLocked(acc *AccountState) float64 {
	wealth := effectiveBalance(acc).Float()
	if cs.pool != nil {
		if total := cs.pool.TotalLPShares.Float(); total > 0 {
			wealth += acc.LPShares.Float() / total * cs.pool.ReserveAEQ.Float()
		}
	}
	return wealth
}

func (cs *ChainState) calcGiniLocked() float64 {
	var balances []float64
	cs.accounts.Range(func(_ string, acc *AccountState) bool {
		// Every human counts, and each counts their FULL AEQ wealth (liquid +
		// LP). Filtering on liquid Balance>0 used to exclude anyone who parked
		// all their AEQ as liquidity, understating inequality and — worse —
		// producing a Gini that disagreed with the Lorenz curve, which included
		// them at 0.
		if acc.IsHuman {
			balances = append(balances, cs.humanAEQWealthLocked(acc))
		}
		return true
	})
	return calcGiniFromBalances(balances)
}

func (cs *ChainState) CalcGini() float64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	// P2-1: deduplicated — delegates to calcGiniLocked (which now uses sort.Float64s).
	return cs.calcGiniLocked()
}

func (cs *ChainState) CalcAequitasIndex() float64 {
	gini := cs.CalcGini()
	return aequitasIndexFromGini(gini)
}

// aequitasIndexFromGini is CalcAequitasIndex's arithmetic without the lock,
// so StatusMetrics can derive the index from a gini it already computed.
func aequitasIndexFromGini(gini float64) float64 {
	index := gini * 100.0
	return float64(int(index*10)) / 10.0
}

// StatusMetrics bundles everything /api/status needs out of ChainState.
//
// FIX (P0 availability, 2026-07-25 — measured on the live primary, which was
// answering /api/status in ELEVEN SECONDS while /api/health/combined and
// /api/blocks answered in 0.22s):
//
// handleStatus used to assemble its response from nine separate accessor
// calls, each taking cs.mu independently:
//
//	CalcAequitasIndex -> CalcGini   RLock
//	CalcGini                        RLock   (same value, computed again)
//	CalcPhase -> CalcGini           RLock   (and again)
//	TotalHumans                     RLock   x2
//	GetBalance(4 pool addresses)    LOCK    x4  <- WRITE lock, not read
//
// GetBalance takes the WRITE lock because it may lazily load a cold account.
// So every hit on a read-only display endpoint — the one the explorer polls
// on a timer, from every open browser tab — acquired the global state write
// lock four times. Go's RWMutex blocks incoming readers as soon as a writer
// queues, so those four acquisitions each had to wait out whatever held the
// lock, and in turn stalled every reader behind them.
//
// The 11s itself is not this function's arithmetic (gini over a few hundred
// accounts is microseconds) — it is time spent WAITING, because a block
// replay holds cs.mu for the duration of a block, and this chain still has
// 50,000-transfer load-test blocks in its history. The knock-on effect was
// the real damage: /api/peers/register contends for the same lock, so
// Contabo1's registration POST timed out, its challenge expired before the
// retry landed, and the primary logged "invalid/expired challenge signature"
// in a loop while the retries tripped its own rate limiter.
//
// This computes the whole set under ONE read lock, and reads the four pool
// balances from Postgres OUTSIDE the lock entirely — pool addresses are
// tokenomics infrastructure that never has demurrage applied (see
// distributeUBIPoolLocked's P0-FIX), so the stored balance IS the effective
// balance and no settlement is needed to report it.
type StatusMetrics struct {
	Humans         int
	Supply         float64
	Gini           float64
	Index          float64
	Phase          int
	PoolValidators float64
	PoolLP         float64
	PoolUBI        float64
	PoolTreasury   float64
}

func (cs *ChainState) StatusMetrics() StatusMetrics {
	// Pool balances first, without cs.mu: one DB round trip for all four.
	pools := map[string]float64{}
	if cs.db != nil {
		rows, err := cs.db.Query(
			`SELECT lower(address), balance FROM chain_accounts WHERE lower(address) = ANY($1)`,
			pq.Array([]string{validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr}),
		)
		if err == nil {
			for rows.Next() {
				var addr string
				var bal float64
				if rows.Scan(&addr, &bal) == nil {
					pools[addr] = bal
				}
			}
			rows.Close()
		}
	}

	cs.mu.RLock()
	humans := int(cs.humanCountLocked())
	gini := cs.calcGiniLocked()
	// No DB (unit tests) — fall back to the in-memory map while we already
	// hold the read lock, rather than reaching for the write lock later.
	if cs.db == nil {
		for _, addr := range []string{validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr} {
			if acc, ok := cs.accounts.Get(addr); ok {
				pools[addr] = acc.Balance.Float()
			}
		}
	}
	cs.mu.RUnlock()

	// Supply is humans x 1000 by protocol design (see TotalSupply) — derived
	// here from the count we just read instead of taking the lock again.
	supply := float64(humans) * 1000.0

	phase := 0
	switch {
	case humans >= 1000000 && gini < 0.3:
		phase = 3
	case humans >= 10000 || supply >= 10000000:
		phase = 2
	case humans >= 100:
		phase = 1
	}

	return StatusMetrics{
		Humans:         humans,
		Supply:         supply,
		Gini:           gini,
		Index:          aequitasIndexFromGini(gini),
		Phase:          phase,
		PoolValidators: pools[validatorsPoolAddr],
		PoolLP:         pools[lpPoolAddr],
		PoolUBI:        pools[ubiPoolAddr],
		PoolTreasury:   pools[treasuryPoolAddr],
	}
}

func (cs *ChainState) CalcPhase() int {
	humans := cs.TotalHumans()
	supply := cs.TotalSupply()
	gini := cs.CalcGini()
	switch {
	case humans >= 1000000 && gini < 0.3:
		return 3
	case humans >= 10000 || supply >= 10000000:
		return 2
	case humans >= 100:
		return 1
	default:
		return 0
	}
}

// FIX (audit 2026-06-29): SetBalance removed — confirmed zero remaining
// callers after deleting its only caller, EVMEngine.syncBalancesFromDB
// (evm_engine.go), which P0-3 had already (correctly) stopped invoking but
// never actually deleted. This was a real landmine left in place: an
// unconditional acc.Balance = NewDecimal(amount) that bypassed
// settleDemurrageLocked, enforceWealthCapLocked, touchActivity, AND —
// critically — the pending_tx outbox every other balance mutation in this
// file goes through. Had anything ever called this again, the change
// would never have been recorded as a Transaction, so secondary nodes
// would have had no way to replay it: an instant, permanent StateRoot
// divergence between the node that called it and every other node on the
// network. Any future legitimate need to set a balance directly (tests,
// admin tooling) should go through the same Apply*Delta/
// runAtomicWithOutbox pattern every other mutation in this file uses, not
// a shortcut that skips all of it.

// -- SECONDARY-NODE REPLAY DELTA METHODS -----------------------------------
// These methods are called exclusively by replayTransactions on secondary nodes.
// They apply pre-computed amounts directly, without re-running business logic,
// to avoid pool-state divergence and floating-point ordering differences.

// ─── BLOCK-LEVEL REPLAY ROLLBACK ─────────────────────────────────────────────
//
// A block can carry more than one transaction. Each individual Apply*Delta
// call above is internally "fail-clean" (mutates nothing if it returns an
// error — see the FIX comments on each), but that alone doesn't make a
// MULTI-transaction block atomic: TX1 in a block could succeed (mutate +
// persist) while TX2 in the SAME block then genuinely fails (real
// insufficient-balance / missing-account divergence, not an expected
// idempotent skip like "already registered"). Without rolling TX1 back too,
// the block ends up partially applied — this node's state reflects less
// than what the producer's StateRoot was computed against.
//
// snapshotForRollback/restoreFromRollback let block.go's replayTransactions
// capture exactly the accounts and pool state a block's transactions can
// touch BEFORE processing it, and restore that snapshot if any transaction
// in the block hits a genuine failure — so a failed block changes nothing
// at all, rather than partially changing things. Scoped to what StateRoot()
// actually hashes (accounts, pool, nullifier keys) — bio_registrations/
// bio_hashes are deliberately out of scope, they're non-consensus side
// bookkeeping that doesn't affect StateRoot.

type accountSnapshot struct {
	address string
	existed bool
	state   AccountState
}

type blockRollbackSnapshot struct {
	accounts []accountSnapshot
	pool     *PoolState // nil if cs.pool was nil
	// chainConfig captures StateRoot-relevant chain_config keys (currently
	// just last_ubi_at — see StateRoot's getConfigValue("last_ubi_at") call)
	// before a block's transactions are replayed.
	//
	// FIX (audit3, P0 #2): this used to be entirely absent. ApplyUBIFinalizeDelta
	// writes last_ubi_at directly via setConfigValue — bypassing
	// cs.accounts/cs.pool entirely, so it was invisible to this rollback
	// mechanism. If a block contains ubi_distribution_finalize AND a LATER
	// transaction in that same block then genuinely hard-fails (or the
	// post-replay StateRoot check itself fails), restoreFromRollback reverted
	// accounts/pool but left last_ubi_at changed — a rejected block could
	// permanently mutate a StateRoot-relevant value anyway, a real consensus
	// bug independent of whether any TX actually committed.
	//
	// FIX (audit recheck2, P0 #4): map[string]string couldn't distinguish
	// "key existed with this value" from "key didn't exist" (getConfigValue
	// returns "" for both). restoreFromRollback used "" as "skip restoring
	// this key" — so a block that set last_ubi_at for the FIRST TIME and was
	// then rolled back left that brand-new DB row in place forever; there
	// was no way to tell restore "this key must be deleted, not skipped".
	// configValueSnapshot's existed field makes that distinction explicit.
	chainConfig map[string]configValueSnapshot
	// accountSetXOR / nullifierSetXOR capture the two incremental state-root
	// accumulators (see ChainState) at snapshot time. The per-account leafHash
	// caches are already covered by the account-state copies above (leafHash is
	// a field of AccountState, so *acc snapshots it), but the two GLOBAL
	// accumulators live on ChainState, not on any account, so a rollback must
	// restore them explicitly — otherwise a hard-failed block's XOR mutations
	// would survive its own rejection and permanently skew every later root.
	accountSetXOR   [32]byte
	nullifierSetXOR [32]byte
}

type configValueSnapshot struct {
	value   string
	existed bool
}

// stateRootRelevantConfigKeys lists every chain_config key StateRoot()
// reads. Kept as a single list so snapshotForRollback/restoreFromRollback
// can't drift out of sync with StateRoot as new keys are added there.
var stateRootRelevantConfigKeys = []string{"last_ubi_at"}

// blockTouchedAddresses returns the wallets a block's transactions can
// mutate, and whether a full-account snapshot is needed instead. A
// ubi_distribution TX credits EVERY registered human (see ApplyUBIDelta) —
// none of which appear in that TX's own Wallet/To fields (Wallet is the
// zero address) — so a block containing one needs every account snapshotted,
// not just the ones named in its transactions, for rollback to be complete
// if some OTHER TX in the same block later hard-fails.
func blockTouchedAddresses(block *Block) (addrs []string, needsFullSnapshot bool) {
	seen := make(map[string]bool)
	add := func(a string) {
		a = strings.ToLower(strings.TrimSpace(a))
		if a != "" && !seen[a] {
			seen[a] = true
			addrs = append(addrs, a)
		}
	}
	for _, tx := range block.Transactions {
		if tx.Type == "ubi_distribution" {
			needsFullSnapshot = true
		}
		add(tx.Wallet)
		add(tx.To)
	}
	add(validatorsPoolAddr)
	add(lpPoolAddr)
	add(ubiPoolAddr)
	add(treasuryPoolAddr)
	return addrs, needsFullSnapshot
}

// snapshotForRollback captures the current state of the given addresses
// plus the liquidity pool, before a block's transactions are replayed.
func (cs *ChainState) snapshotForRollback(addrs []string, full bool) *blockRollbackSnapshot {
	// Read StateRoot-relevant config BEFORE acquiring cs.mu — getConfigValue
	// does a blocking DB query, and StateRoot() itself already established
	// the pattern of never holding cs.mu across one (see its own
	// last_ubi_at read and P1-1 comment).
	//
	// FIX (audit 2026-06-28 recheck 4, P0-1): plain DB-only read, for the
	// same reason as StateRoot()'s and runAtomicWithOutbox's matching
	// fixes — this runs before cs.mu.RLock() below, so it must never touch
	// cs.activeTx.
	chainConfig := make(map[string]configValueSnapshot, len(stateRootRelevantConfigKeys))
	for _, key := range stateRootRelevantConfigKeys {
		value, existed := cs.getConfigValueExistsDB(key)
		chainConfig[key] = configValueSnapshot{value: value, existed: existed}
	}
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.snapshotForRollbackLocked(addrs, full, chainConfig)
}

// snapshotForRollbackLocked is snapshotForRollback's body, for callers that
// already hold cs.mu (in either mode) and have already pre-fetched
// chainConfig themselves.
//
// FIX (audit recheck2, P0 #1): runAtomicWithOutbox/runAtomicDistributionWithOutbox
// used to call the lock-acquiring snapshotForRollback BEFORE taking their own
// cs.mu.Lock() for fn() — snapshotForRollback's internal RLock/RUnlock meant
// the snapshot was released and the lock fully dropped before the caller's
// own Lock() even started, leaving a window where a concurrent operation
// (different goroutine, e.g. a transfer on another account) could acquire
// cs.mu, mutate and commit its own change, and complete — all before THIS
// operation's Lock() finally went through. If THIS operation then failed and
// rolled back using the now-stale snapshot, restoreFromRollback would revert
// every account in that snapshot to its pre-snapshot value, silently undoing
// the other goroutine's already-committed, unrelated, successful mutation.
// Calling this Locked variant from inside the SAME critical section as fn()
// (snapshot and mutation under one unbroken cs.mu.Lock()) closes that gap —
// nothing else can touch cs.accounts/cs.pool between the two.
func (cs *ChainState) snapshotForRollbackLocked(addrs []string, full bool, chainConfig map[string]configValueSnapshot) *blockRollbackSnapshot {
	// FIX (Monster Audit follow-up, 2026-07-12, P0): both branches below used
	// to read cs.accounts[a] directly with no DB warm-up first. addrs always
	// includes all 4 pool addresses (see blockTouchedAddresses) — exactly the
	// addresses already known (from this same day's earlier fix) to go cold
	// after a cache eviction or restart. A cold-but-real address was recorded
	// as existed:false; if the block then hard-failed for ANY reason (or even
	// just a DB commit error at the end of a successful replay), the
	// restoreFromRollbackLocked call below would DELETE FROM chain_accounts
	// that address's real row — permanently destroying its balance,
	// tusd_balance, lp_shares, and is_human, as collateral damage from a
	// failure that had nothing to do with it. Warming here, before either
	// branch reads cs.accounts, closes the gap for both: the non-full branch
	// directly, and the full branch too (its own first loop iterates
	// cs.accounts, so anything warmed here is captured as existed:true with
	// real state instead of falling into the addrs-only "doesn't exist yet"
	// fallback).
	cs.ensureAccountsLoaded(addrs)
	snap := &blockRollbackSnapshot{}
	if full {
		// ubi_distribution touches every human's account (see ApplyUBIDelta) —
		// snapshot all of them rather than trying to enumerate which wallets
		// are currently human (that set is itself part of what we're
		// snapshotting, and could race against ApplyUBIDelta's own enumeration).
		existing := make(map[string]bool, cs.accounts.Len())
		snap.accounts = make([]accountSnapshot, 0, cs.accounts.Len()+len(addrs))
		cs.accounts.Range(func(a string, acc *AccountState) bool {
			existing[a] = true
			snap.accounts = append(snap.accounts, accountSnapshot{address: a, existed: true, state: *acc})
			return true
		})
		// addrs (the block's OTHER, non-ubi TXs' wallets) may name an
		// account that doesn't exist yet but could be CREATED during this
		// block's replay (e.g. a transfer to a brand-new wallet). Without
		// also tracking those as existed:false, a rollback wouldn't know to
		// remove them — they're absent from the existing-accounts loop above
		// precisely because they don't exist yet.
		for _, a := range addrs {
			if !existing[a] {
				snap.accounts = append(snap.accounts, accountSnapshot{address: a, existed: false})
				existing[a] = true
			}
		}
	} else {
		snap.accounts = make([]accountSnapshot, 0, len(addrs))
		for _, a := range addrs {
			if acc, ok := cs.accounts.Get(a); ok {
				snap.accounts = append(snap.accounts, accountSnapshot{address: a, existed: true, state: *acc})
			} else {
				snap.accounts = append(snap.accounts, accountSnapshot{address: a, existed: false})
			}
		}
	}
	if cs.pool != nil {
		poolCopy := *cs.pool
		snap.pool = &poolCopy
	}
	snap.chainConfig = chainConfig
	snap.accountSetXOR = cs.accountSetXOR
	snap.nullifierSetXOR = cs.nullifierSetXOR
	return snap
}

// restoreFromRollback reverts cs.accounts/cs.pool to a previously captured
// snapshot and persists the reverted values, undoing whatever a failed
// block's transactions had already mutated and saved. Accounts that didn't
// exist before the block are removed from memory and the DB so a failed
// block can't leave behind a fresh, empty-but-present row either.
//
// FIX (audit recheck2, P1 #7): used to return nothing — saveAccountToDB/
// savePoolToDB errors during the restore itself were silently dropped, so a
// rollback could succeed in memory while failing to persist, then look
// "restored" again after a restart (DB wins over memory on reload). Returns
// the first persistence error encountered, after still attempting every
// other write (best-effort, matching the existing DELETE query's behavior
// just below) — callers surface this loudly since it means the in-memory
// and DB states are now known to disagree, which a later replay/StateRoot
// check may not otherwise explain. This does not (yet) force the node into
// a halt/resync state, which is the audit's suggested deeper fix — that
// needs the authoritative-resync mode tracked separately (task #65); for
// now, making the failure visible is the honest, scoped improvement.
func (cs *ChainState) restoreFromRollback(snap *blockRollbackSnapshot) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.restoreFromRollbackLocked(snap)
}

// restoreFromRollbackLocked is restoreFromRollback's body, for callers that
// already hold cs.mu for the ENTIRE surrounding operation (audit recheck3,
// P0/P1 — replayTransactions). Unlike the public restoreFromRollback, this
// does NOT release cs.mu before its DB writes: replayTransactions needs the
// lock held continuously from its snapshot through to either a successful
// StateRoot match or this rollback, so no concurrent API/distribution
// operation can mutate the same accounts in the gap and then get its own
// already-committed change silently reverted by a rollback using a
// snapshot taken before that change happened — exactly the race the public
// restoreFromRollback's per-call locking still leaves open for replay
// specifically (every other caller of the public version already releases
// cs.mu beforehand for unrelated reasons, e.g. runAtomicWithOutbox, so this
// duplication is intentional, not copy-paste: the two functions hold the
// lock for genuinely different durations on purpose).
func (cs *ChainState) restoreFromRollbackLocked(snap *blockRollbackSnapshot) error {
	var toDelete []string
	for _, s := range snap.accounts {
		if s.existed {
			restored := s.state
			cs.accounts.Set(s.address, &restored)
		} else {
			cs.accounts.Delete(s.address)
			toDelete = append(toDelete, s.address)
		}
	}
	if snap.pool != nil {
		poolCopy := *snap.pool
		cs.pool = &poolCopy
	}
	// Unlike the public restoreFromRollback, cs.mu stays held through the DB
	// writes below — see this function's own doc comment for why.
	var toSave []*AccountState
	for _, s := range snap.accounts {
		if s.existed {
			acc, _ := cs.accounts.Get(s.address)
			toSave = append(toSave, acc)
		}
	}
	poolToSave := cs.pool

	var firstErr error
	for _, acc := range toSave {
		if err := cs.saveAccountToDB(acc); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("rollback: could not persist restored account %s: %w", acc.Address, err)
		}
	}
	if poolToSave != nil {
		if err := cs.savePoolToDB(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("rollback: could not persist restored pool: %w", err)
		}
	}
	if cs.db != nil {
		// FIX (deadlock, concurrency audit 2026-07-21): cs.dbExec() instead
		// of cs.db — see ensureAccountLoaded's FIX comment. Reached on the
		// rollback path with cs.mu+cs.activeTx already held (see this
		// function's own doc comment on why the lock stays held through
		// these DB writes).
		for _, addr := range toDelete {
			if _, err := cs.dbExec().Exec(`DELETE FROM chain_accounts WHERE lower(address) = $1`, addr); err != nil {
				fmt.Printf("[ROLLBACK] Warning: could not delete rolled-back account %s: %v\n", addr, err)
				if firstErr == nil {
					firstErr = fmt.Errorf("rollback: could not delete rolled-back account %s: %w", addr, err)
				}
			}
		}
	}
	// FIX (audit3, P0 #2; audit recheck2, P0 #4): restore StateRoot-relevant
	// chain_config too — see blockRollbackSnapshot.chainConfig's comment.
	// setConfigValue/deleteConfigValue do their own blocking DB I/O, now run
	// with cs.mu still held (see this function's doc comment for why that's
	// now intentional, not the latency bug it would have been before audit
	// recheck3). A key that didn't exist before this block must be DELETED,
	// not skipped — skipping it
	// left a block's first-ever write to that key permanently in place even
	// after a full rollback (the original bug: an empty string was treated
	// as "nothing to restore", indistinguishable from "key never existed").
	for key, cv := range snap.chainConfig {
		if !cv.existed {
			if err := cs.deleteConfigValue(key); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("rollback: could not delete config %q: %w", key, err)
			}
			continue
		}
		if err := cs.setConfigValue(key, cv.value); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("rollback: could not restore config %q: %w", key, err)
		}
	}
	// Restore the two global state-root accumulators to their pre-block values.
	// The per-account saveAccountToDB calls above are net-zero for accountSetXOR
	// (each restored account's leafHash already equals its recomputed leaf), and
	// accounts deleted by this rollback had their leaf folded in during the
	// failed block — so overwriting with the snapshot is the authoritative
	// revert. Done LAST so it wins over any incremental XOR the restore path
	// itself performed. nullifierSetXOR is reverted here too, since the nullifier
	// map/DB rollback happens via the surrounding transaction, not this function.
	cs.accountSetXOR = snap.accountSetXOR
	cs.nullifierSetXOR = snap.nullifierSetXOR
	return firstErr
}

// ApplyTransferDelta directly adjusts AEQ balances by the net amount that
// reached the recipient (after any fee that was applied on the primary).
// Used by secondary nodes replaying transfer TXs from blocks. fromLost/toLost
// are the exact demurrage amounts the primary decayed off each side (see
// Transfer()/TransferWithV7Fee()) — applied directly via
// applyDemurrageLossLocked rather than recomputed, since recomputing via
// effectiveBalance()/nowUnix() at replay time would use this node's own
// wall-clock time and diverge from what the primary actually settled.
func (cs *ChainState) ApplyTransferDelta(from, to string, netAmount, fromLost, toLost float64) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox — see
	// RegisterHuman's comment.
	//
	// activityAt 0: this wrapper has no block to read a timestamp from, so
	// touchActivityAt falls back to nowUnix() — see its comment. Every
	// block-replay caller goes through applyTransferDeltaLocked directly and
	// passes block.Timestamp.
	return cs.applyTransferDeltaLocked(context.Background(), from, to, netAmount, fromLost, toLost, 0)
}

// applyTransferDeltaLocked is ApplyTransferDelta's body, for callers that
// already hold cs.mu (audit recheck3, P0/P1 — replayTransactions holds
// cs.mu continuously across an entire block's snapshot/deltas/StateRoot
// check instead of releasing and reacquiring it once per TX; see
// replayTransactions' own comment for the isolation race this closes).
// Block replay (block.go) also calls this directly with
// context.Background(): it sets dag.state.activeTx itself before this
// runs, and dbExecCtx falls back to that field when ctx carries no
// transaction, so behavior there is unchanged — see registerHumanLocked's
// comment for the same reasoning.
//
// activityAt is the instant to stamp on both parties' demurrage clock — the
// replayed block's own Timestamp. See touchActivityAt for why replay must not
// read its own wall clock, and why this used to reset no clock at all.
func (cs *ChainState) applyTransferDeltaLocked(ctx context.Context, from, to string, netAmount, fromLost, toLost float64, activityAt int64) error {
	from = strings.ToLower(from)
	to = strings.ToLower(to)
	// FIX (Monster Audit follow-up, 2026-07-12, P0): same cold-cache pattern
	// already fixed today at the 11 pool-address sites, unaddressed here for
	// ordinary wallet addresses. A cold `from` used to fail with "account not
	// found" even though it has a real DB balance — on a secondary replaying
	// a block the primary already accepted, that's a hard-fail this function
	// returns, which the caller (replayTransactions) treats as block-invalid
	// and rejects, diverging this node from consensus. A cold `to` was worse:
	// blind-created as a fresh zero-balance AccountState below, which
	// silently overwrites (via saveAccountToDB's Version==0 upsert) any real
	// balance `to` already had in the DB — including if `to` happens to be a
	// pool address reached via an ordinary transfer rather than the
	// distribution paths already fixed today.
	cs.ensureAccountLoadedCtx(ctx, from)
	cs.ensureAccountLoadedCtx(ctx, to)
	fromAcc, ok := cs.accounts.Get(from)
	if !ok {
		return fmt.Errorf("from account not found: %s", from)
	}
	// FIX: applyDemurrageLossLocked mutates fromAcc.Balance AND credits the
	// tokenomics pools (via distributeSwapFee, persisted to DB immediately)
	// as a side effect — it used to run BEFORE this sufficiency check, so a
	// transfer that turned out insufficient AFTER decay still left the pools
	// permanently credited in the DB while the sender's matching decay was
	// only ever applied in-memory, never persisted (since the early return
	// skipped the saveAccountToDB call below). Check against the
	// post-decay balance FIRST, without mutating anything, so a failing
	// transfer truly changes nothing.
	if fromAcc.Balance.Float()-fromLost < netAmount {
		return fmt.Errorf("insufficient balance (have %.6f after demurrage, need %.6f)", fromAcc.Balance.Float()-fromLost, netAmount)
	}
	if err := cs.applyDemurrageLossLockedCtx(ctx, fromAcc, fromLost); err != nil {
		return fmt.Errorf("transfer: could not settle sender %s demurrage: %w", from, err)
	}
	fromAcc.Balance = fromAcc.Balance.Sub(NewDecimal(netAmount))
	// FIX (audit 2026-08-15): the ingestion path this mirrors (transferLocked)
	// calls touchActivity on BOTH sides — "sending counts as using the money",
	// per its own comment — and every other apply*Delta counterpart in this
	// file was given the matching call in the 2026-07-04 audit. Plain
	// transfers, by far the most common transaction on the chain, were the one
	// case left out: a node that merely REPLAYED a transfer left both wallets'
	// demurrage clocks untouched, so on every node except the one that produced
	// the block the two parties kept ageing as if idle. That decides real
	// money — settleDemurrageLocked charges decay from this field, and
	// checkAndMoveToEscrowLocked sweeps a wallet to escrow after 2.5 years of
	// it — and the daily distribution round runs on whichever node wins
	// TryLockDistribution, so which numbers the chain committed depended on
	// which node happened to run it. Stamped from the block, not from this
	// node's clock: see touchActivityAt.
	touchActivityAt(fromAcc, activityAt)
	// FIX (audit recheck2, P0 #3): this and every other saveAccountToDB/
	// savePoolToDB call in this function used to discard the returned error
	// — replayTransactions's caller checks THIS function's own return value
	// and rolls back on error, but with the error swallowed here it always
	// saw nil regardless of whether the DB write actually durably
	// committed. A block could be accepted (in-memory state mutated, block
	// inserted into the DAG) while the underlying account row never made it
	// to disk — exactly the kind of divergence that only surfaces after a
	// restart or bootstrap, when DB wins over memory.
	if err := cs.saveAccountToDBCtx(ctx, fromAcc); err != nil {
		return fmt.Errorf("transfer: could not save sender %s: %w", from, err)
	}

	if _, ok := cs.accounts.Get(to); !ok {
		cs.accounts.Set(to, &AccountState{Address: to})
	}
	toAcc, _ := cs.accounts.Get(to)
	if err := cs.applyDemurrageLossLockedCtx(ctx, toAcc, toLost); err != nil {
		return fmt.Errorf("transfer: could not settle recipient %s demurrage: %w", to, err)
	}
	toAcc.Balance = toAcc.Balance.Add(NewDecimal(netAmount))
	touchActivityAt(toAcc, activityAt) // see the sender's own call above; transferLocked touches both sides
	if err := cs.enforceWealthCapLockedCtx(ctx, toAcc); err != nil {
		return fmt.Errorf("transfer: could not enforce wealth cap for recipient %s: %w", to, err)
	}
	if err := cs.saveAccountToDBCtx(ctx, toAcc); err != nil {
		return fmt.Errorf("transfer: could not save recipient %s: %w", to, err)
	}
	return nil
}

// ApplySwapDelta adjusts balances after a swap, using the exact amountIn/amountOut
// stored in the block TX. aeqToTusd=true: wallet loses amountIn AEQ, gains amountOut tUSD.
// aeqToTusd=false: wallet loses amountIn tUSD, gains amountOut AEQ.
// Also updates pool reserves to mirror what swapLocked() did on the primary.
// demurrageLost is the exact amount swapLocked() decayed off wallet on the
// primary — applied directly (applyDemurrageLossLocked) rather than
// recomputed, for the same reason as in ApplyTransferDelta.
func (cs *ChainState) ApplySwapDelta(wallet string, amountIn, amountOut float64, aeqToTusd bool, demurrageLost float64) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox — see
	// RegisterHuman's comment.
	// activityAt 0 — no block here, so touchActivityAt falls back to nowUnix();
	// see ApplyTransferDelta's identical note.
	return cs.applySwapDeltaLocked(context.Background(), wallet, amountIn, amountOut, aeqToTusd, demurrageLost, 0)
}

// applySwapDeltaLocked is ApplySwapDelta's body — see
// applyTransferDeltaLocked's comment (both for the cs.mu contract and for
// why block replay's own context.Background() call sites are correct).
// activityAt is the replayed block's own Timestamp — see touchActivityAt for
// why a replay handler must not read this node's wall clock.
func (cs *ChainState) applySwapDeltaLocked(ctx context.Context, wallet string, amountIn, amountOut float64, aeqToTusd bool, demurrageLost float64, activityAt int64) error {
	wallet = strings.ToLower(wallet)
	cs.ensureAccountLoadedCtx(ctx, wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok {
		return fmt.Errorf("account not found: %s", wallet)
	}
	// FIX: same class of bug as ApplyTransferDelta — applyDemurrageLossLocked
	// has DB-persisted side effects (pool credits) and used to run before
	// the sufficiency check below, leaving those side effects committed even
	// when the swap itself then failed. Check against the post-decay
	// balance first.
	if aeqToTusd {
		if acc.Balance.Float()-demurrageLost < amountIn {
			return fmt.Errorf("insufficient AEQ balance")
		}
	} else {
		if acc.TUsdBalance.Float() < amountIn {
			// tUSD balance is unaffected by AEQ demurrage, no projection needed.
			return fmt.Errorf("insufficient tUSD balance")
		}
	}
	if err := cs.applyDemurrageLossLockedCtx(ctx, acc, demurrageLost); err != nil {
		return fmt.Errorf("swap: could not settle %s demurrage: %w", wallet, err)
	}
	if aeqToTusd {
		acc.Balance = acc.Balance.Sub(NewDecimal(amountIn))
		acc.TUsdBalance = acc.TUsdBalance.Add(NewDecimal(amountOut))
	} else {
		acc.TUsdBalance = acc.TUsdBalance.Sub(NewDecimal(amountIn))
		acc.Balance = acc.Balance.Add(NewDecimal(amountOut))
	}
	// FIX (P0, 2026-07-04 brutal audit): swapLocked (primary path) calls
	// touchActivity unconditionally right here, then enforceWealthCapLocked
	// when !aeqToTusd (AEQ just arrived via this swap direction) — this
	// replay counterpart never mirrored either. A wallet near the cap
	// swapping tUSD->AEQ would end up with a HIGHER, uncapped Balance on
	// secondaries than the primary's capped, excess-redistributed value:
	// AccountSetXOR and pool state diverge, surfacing as a StateRoot
	// mismatch that looks unrelated to swaps at all. Mirroring exactly what
	// the primary does, in the same order, before saving.
	touchActivityAt(acc, activityAt)
	if !aeqToTusd {
		if err := cs.enforceWealthCapLockedCtx(ctx, acc); err != nil {
			return fmt.Errorf("swap: could not enforce wealth cap for %s: %w", wallet, err)
		}
	}
	// FIX (audit recheck2, P0 #3): see ApplyTransferDelta's comment — every
	// saveAccountToDB/savePoolToDB call in this function used to discard its
	// returned error.
	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		return fmt.Errorf("swap: could not save account %s: %w", wallet, err)
	}

	// Update pool reserves to match what swapLocked() did on primary.
	// fee is swapFeeBps (0.1%); amountInAfterFee is what enters the pool.
	if cs.pool != nil {
		fee := amountIn * float64(swapFeeBps) / 10000.0
		amountInAfterFee := amountIn - fee
		if aeqToTusd {
			// Sender put in AEQ, got tUSD: reserveAEQ grows, reserveTUSD shrinks.
			cs.pool.ReserveAEQ = cs.pool.ReserveAEQ.Add(NewDecimal(amountInAfterFee))
			cs.pool.ReserveTUSD = cs.pool.ReserveTUSD.Sub(NewDecimal(amountOut)).AtLeastZero()
		} else {
			// Sender put in tUSD, got AEQ: reserveTUSD grows, reserveAEQ shrinks.
			cs.pool.ReserveTUSD = cs.pool.ReserveTUSD.Add(NewDecimal(amountInAfterFee))
			cs.pool.ReserveAEQ = cs.pool.ReserveAEQ.Sub(NewDecimal(amountOut)).AtLeastZero()
		}
		if err := cs.savePoolToDBCtx(ctx); err != nil {
			return fmt.Errorf("swap: could not save pool: %w", err)
		}
		// Distribute swap fee to the 4 tokenomics pools (40% validators /
		// 30% LP / 20% UBI / 10% treasury) — mirrors swapLocked() on primary.
		// Without this the fee-pool addresses stay at 0 on secondaries,
		// causing StateRoot divergence (pool addresses are included in the hash).
		if err := cs.distributeSwapFeeCtx(ctx, fee, aeqToTusd); err != nil {
			return fmt.Errorf("could not persist swap fee distribution: %w", err)
		}
	}
	return nil
}

// AddLiquidityDelta applies an add-liquidity operation on secondary nodes using
// the exact stored amounts. lpShares is the number of LP shares minted on the
// primary node; if > 0 it is used directly instead of recomputing, eliminating
// pool-state drift between nodes. Reloads pool from DB first for consistency.
// demurrageLost is the exact amount AddLiquidity() decayed off wallet on the
// primary — applied directly rather than recomputed, for the same reason as
// in ApplyTransferDelta.
func (cs *ChainState) AddLiquidityDelta(wallet string, aeqAmount, tusdAmount, lpShares, demurrageLost float64) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox — see
	// RegisterHuman's comment.
	// activityAt 0 — no block here, so touchActivityAt falls back to nowUnix();
	// see ApplyTransferDelta's identical note.
	return cs.addLiquidityDeltaLocked(context.Background(), wallet, aeqAmount, tusdAmount, lpShares, demurrageLost, 0)
}

// addLiquidityDeltaLocked is AddLiquidityDelta's body — see
// applyTransferDeltaLocked's comment.
// activityAt is the replayed block's own Timestamp — see touchActivityAt for
// why a replay handler must not read this node's wall clock.
func (cs *ChainState) addLiquidityDeltaLocked(ctx context.Context, wallet string, aeqAmount, tusdAmount, lpShares, demurrageLost float64, activityAt int64) error {
	wallet = strings.ToLower(wallet)
	cs.ensureAccountLoadedCtx(ctx, wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok {
		return fmt.Errorf("account not found: %s", wallet)
	}
	cs.reloadPoolFromDB()
	// FIX: same class of bug as ApplyTransferDelta — check against the
	// post-decay balance before calling applyDemurrageLossLocked (which has
	// DB-persisted side effects via distributeSwapFee), so a failing
	// add-liquidity truly changes nothing instead of leaving a phantom pool
	// credit committed.
	if acc.Balance.Float()-demurrageLost < aeqAmount {
		return fmt.Errorf("insufficient AEQ balance")
	}
	if acc.TUsdBalance.Float() < tusdAmount {
		return fmt.Errorf("insufficient tUSD balance")
	}
	if err := cs.applyDemurrageLossLockedCtx(ctx, acc, demurrageLost); err != nil {
		return fmt.Errorf("add_liquidity: could not settle %s demurrage: %w", wallet, err)
	}

	// Use the stored LP shares from the primary node when available.
	// Fall back to recomputing (from pool state or geometric mean) for
	// blocks produced by old nodes that don't include the lp_shares field.
	var mintedShares float64
	if lpShares > 0 {
		mintedShares = lpShares
	} else if cs.pool != nil && cs.pool.ReserveAEQ.Float() > 0 && cs.pool.TotalLPShares.Float() > 0 {
		mintedShares = (aeqAmount / cs.pool.ReserveAEQ.Float()) * cs.pool.TotalLPShares.Float()
	} else {
		mintedShares = math.Sqrt(aeqAmount * tusdAmount)
	}

	acc.Balance = acc.Balance.Sub(NewDecimal(aeqAmount))
	acc.TUsdBalance = acc.TUsdBalance.Sub(NewDecimal(tusdAmount))
	acc.LPShares = acc.LPShares.Add(NewDecimal(mintedShares))
	// FIX (audit 2026-08-15): missing entirely, the same gap as the transfer
	// path's — addLiquidityLocked (the ingestion side this mirrors) calls
	// touchActivity here, "depositing into the pool counts as using the AEQ"
	// per its own comment, and this replay counterpart never did. The wallet
	// therefore kept ageing toward demurrage and the 2.5-year escrow sweep on
	// every node except the one that produced the block.
	touchActivityAt(acc, activityAt)
	// FIX (audit recheck2, P0 #3): see ApplyTransferDelta's comment.
	if cs.pool != nil {
		cs.pool.ReserveAEQ = cs.pool.ReserveAEQ.Add(NewDecimal(aeqAmount))
		cs.pool.ReserveTUSD = cs.pool.ReserveTUSD.Add(NewDecimal(tusdAmount))
		cs.pool.TotalLPShares = cs.pool.TotalLPShares.Add(NewDecimal(mintedShares))
		if err := cs.savePoolToDBCtx(ctx); err != nil {
			return fmt.Errorf("add_liquidity: could not save pool: %w", err)
		}
	}
	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		return fmt.Errorf("add_liquidity: could not save account %s: %w", wallet, err)
	}
	return nil
}

// RemoveLiquidityDelta burns sharesToBurn LP shares and returns proportional
// pool reserves to the wallet, using the secondary's current pool state.
// demurrageLost is the exact amount RemoveLiquidity() decayed off wallet on
// the primary — applied directly rather than recomputed, for the same
// reason as in ApplyTransferDelta.
func (cs *ChainState) RemoveLiquidityDelta(wallet string, sharesToBurn, demurrageLost float64) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox — see
	// RegisterHuman's comment.
	// activityAt 0 — no block here, so touchActivityAt falls back to nowUnix();
	// see ApplyTransferDelta's identical note.
	return cs.removeLiquidityDeltaLocked(context.Background(), wallet, sharesToBurn, demurrageLost, 0)
}

// removeLiquidityDeltaLocked is RemoveLiquidityDelta's body — see
// applyTransferDeltaLocked's comment.
// activityAt is the replayed block's own Timestamp — see touchActivityAt for
// why a replay handler must not read this node's wall clock.
func (cs *ChainState) removeLiquidityDeltaLocked(ctx context.Context, wallet string, sharesToBurn, demurrageLost float64, activityAt int64) error {
	wallet = strings.ToLower(wallet)
	cs.ensureAccountLoadedCtx(ctx, wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok {
		return fmt.Errorf("account not found: %s", wallet)
	}
	cs.reloadPoolFromDB()
	// FIX: same class of bug as ApplyTransferDelta — these two checks don't
	// even depend on AEQ demurrage (they're about LP shares, not Balance),
	// so there was never a reason for applyDemurrageLossLocked's DB-persisted
	// side effects (pool credits via distributeSwapFee) to run before them.
	// Moved below the checks so a failing remove-liquidity changes nothing.
	if cs.pool == nil || cs.pool.TotalLPShares.Float() <= 0 {
		return fmt.Errorf("liquidity pool is empty")
	}
	if acc.LPShares.Float() < sharesToBurn {
		return fmt.Errorf("insufficient LP shares")
	}
	if err := cs.applyDemurrageLossLockedCtx(ctx, acc, demurrageLost); err != nil {
		return fmt.Errorf("remove_liquidity: could not settle %s demurrage: %w", wallet, err)
	}
	// Mirror F17 + F18 caps from primary RemoveLiquidity
	if sharesToBurn > cs.pool.TotalLPShares.Float() {
		sharesToBurn = cs.pool.TotalLPShares.Float()
		if sharesToBurn <= 0 {
			return fmt.Errorf("pool total LP shares is zero")
		}
	}
	fraction := sharesToBurn / cs.pool.TotalLPShares.Float()
	if fraction > 1.0 {
		fraction = 1.0
	}
	outAEQ := round6(cs.pool.ReserveAEQ.Float() * fraction)
	outTUSD := round6(cs.pool.ReserveTUSD.Float() * fraction)
	if outAEQ > cs.pool.ReserveAEQ.Float() {
		outAEQ = cs.pool.ReserveAEQ.Float()
	}
	if outTUSD > cs.pool.ReserveTUSD.Float() {
		outTUSD = cs.pool.ReserveTUSD.Float()
	}

	acc.LPShares = acc.LPShares.Sub(NewDecimal(sharesToBurn))
	acc.Balance = acc.Balance.Add(NewDecimal(outAEQ))
	acc.TUsdBalance = acc.TUsdBalance.Add(NewDecimal(outTUSD))
	// FIX (P0, 2026-07-04 brutal audit): removeLiquidityLocked (primary path)
	// calls touchActivity + enforceWealthCapLocked right here, in all three
	// of its branches, immediately after crediting the AEQ received back
	// from the pool — this replay counterpart never mirrored either. A
	// wallet near the cap removing liquidity would end up with a HIGHER,
	// uncapped Balance on secondaries than the primary's capped, excess-
	// redistributed value: AccountSetXOR and pool state diverge, which
	// surfaces as a StateRoot mismatch that looks unrelated to liquidity at
	// all. Mirroring exactly what the primary does, in the same order,
	// before saving.
	touchActivityAt(acc, activityAt)
	if err := cs.enforceWealthCapLockedCtx(ctx, acc); err != nil {
		return fmt.Errorf("remove_liquidity: could not enforce wealth cap for %s: %w", wallet, err)
	}
	newReserveAEQ := round6(cs.pool.ReserveAEQ.Float() - outAEQ)
	newReserveTUSD := round6(cs.pool.ReserveTUSD.Float() - outTUSD)
	if newReserveAEQ < 0 {
		newReserveAEQ = 0
	}
	if newReserveTUSD < 0 {
		newReserveTUSD = 0
	}
	cs.pool.ReserveAEQ = NewDecimal(newReserveAEQ)
	cs.pool.ReserveTUSD = NewDecimal(newReserveTUSD)
	cs.pool.TotalLPShares = cs.pool.TotalLPShares.Sub(NewDecimal(sharesToBurn))
	// FIX (audit recheck2, P0 #3): see ApplyTransferDelta's comment.
	if err := cs.savePoolToDBCtx(ctx); err != nil {
		return fmt.Errorf("remove_liquidity: could not save pool: %w", err)
	}
	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		return fmt.Errorf("remove_liquidity: could not save account %s: %w", wallet, err)
	}
	return nil
}

// ApplyUBIDelta credits amountPerHuman AEQ to every registered human on this node.
// Used by secondary nodes replaying ubi_distribution TXs from blocks.
//
// FIX (StateRoot divergence): ubiAt must be the timestamp the PRIMARY used
// when it ran DistributeUBIPool (i.e. the block's Timestamp), not this
// node's own wall clock. last_ubi_at feeds directly into StateRoot(), so
// every secondary independently calling time.Now() here wrote a different
// value than the primary and than every OTHER secondary — guaranteeing a
// StateRoot mismatch on every single UBI distribution. Pass 0 to fall back
// to time.Now() only for callers that have no block context (none should,
// post-fix, but this keeps the function safe to call directly).
// ApplyUBIDelta is the LEGACY flat-broadcast replay path: credits the same
// amountPerHuman to every current human and finalizes (pool zero +
// last_ubi_at) in one call. Kept only so historical blocks already on the
// chain (produced before the per-human fix below) still replay correctly —
// main.go no longer emits this TX shape. It can never replay each human's
// individual demurrage loss (see ApplyUBIRewardDelta's comment), which is
// exactly the gap audit recheck 2 (P0 #5) flagged.
// FIX (audit recheck2, P0 #3): used to return nothing, so a DB write failure
// for any human's account mid-loop was invisible to the caller — see
// ApplyTransferDelta's comment for the general class of bug. Now returns on
// the first save failure, leaving the remaining humans in this round
// uncredited in cs.accounts too; the caller (replayTransactions) marks the
// block a hardFailure and restoreFromRollback reverts everything this round
// already touched, rather than leaving a partially-applied, partially-
// persisted UBI round.
func (cs *ChainState) ApplyUBIDelta(amountPerHuman float64, ubiAt int64) error {
	if amountPerHuman <= 0 {
		return nil
	}
	if ubiAt <= 0 {
		ubiAt = time.Now().Unix()
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox — see
	// RegisterHuman's comment.
	return cs.applyUBIDeltaLocked(context.Background(), amountPerHuman, ubiAt)
}

// applyUBIDeltaLocked is ApplyUBIDelta's body — see applyTransferDeltaLocked's
// comment.
func (cs *ChainState) applyUBIDeltaLocked(ctx context.Context, amountPerHuman float64, ubiAt int64) error {
	var rangeErr error
	cs.accounts.Range(func(addr string, acc *AccountState) bool {
		if !acc.IsHuman {
			return true
		}
		acc.Balance = acc.Balance.Add(NewDecimal(amountPerHuman))
		touchActivityAt(acc, ubiAt)
		if err := cs.enforceWealthCapLockedCtx(ctx, acc); err != nil {
			rangeErr = fmt.Errorf("ubi (legacy flat): could not enforce wealth cap for %s: %w", addr, err)
			return false
		}
		if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
			rangeErr = fmt.Errorf("ubi (legacy flat): could not save account %s: %w", addr, err)
			return false
		}
		return true
	})
	if rangeErr != nil {
		return rangeErr
	}
	// Zero the UBI pool on secondary (it was zeroed on primary after distribution)
	// FIX (Monster Audit 2026-07-12, P1): a cold pool address used to read as
	// "not present" and this silently skipped zeroing it — leaving this
	// secondary's pool balance (and therefore its StateRoot) diverged from
	// every node that DID have it cached at the time.
	cs.ensureAccountLoadedCtx(ctx, ubiPoolAddr)
	if ubiAcc, ok := cs.accounts.Get(ubiPoolAddr); ok {
		ubiAcc.Balance = NewDecimal(0)
		if err := cs.saveAccountToDBCtx(ctx, ubiAcc); err != nil {
			return fmt.Errorf("ubi (legacy flat): could not save pool account: %w", err)
		}
	}
	// Write last_ubi_at to secondary's chain_config so StateRoot matches primary.
	if err := cs.setConfigValueCtx(ctx, "last_ubi_at", fmt.Sprintf("%d", ubiAt)); err != nil {
		return fmt.Errorf("ubi (legacy flat): could not save last_ubi_at: %w", err)
	}
	return nil
}

// ApplyUBIRewardDelta credits a single human's UBI share, settling the
// EXACT demurrage loss the primary already computed for that human in its
// pre-pass over ALL humans (before the pool total was read) — see
// DistributeUBIPool's comment. Used by secondary nodes replaying
// "ubi_distribution" TXs (the per-human shape; see ApplyUBIDelta's comment
// for the legacy flat shape this replaced). The pool itself is zeroed and
// last_ubi_at finalized by a separate "ubi_distribution_finalize" TX (see
// ApplyUBIFinalizeDelta) emitted once per distribution round, not by this
// per-human delta — mirrors DistributeUBIPool's own structure (settle+
// credit loop, then a single unconditional pool zero-out).
func (cs *ChainState) ApplyUBIRewardDelta(wallet string, amount, demurrageLost float64) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox — see
	// RegisterHuman's comment.
	// activityAt 0 — no block here, so touchActivityAt falls back to nowUnix();
	// see ApplyTransferDelta's identical note.
	return cs.applyUBIRewardDeltaLocked(context.Background(), wallet, amount, demurrageLost, 0)
}

// applyUBIRewardDeltaLocked is ApplyUBIRewardDelta's body — see
// applyTransferDeltaLocked's comment.
// activityAt is the replayed block's own Timestamp — see touchActivityAt for
// why a replay handler must not read this node's wall clock.
func (cs *ChainState) applyUBIRewardDeltaLocked(ctx context.Context, wallet string, amount, demurrageLost float64, activityAt int64) error {
	wallet = strings.ToLower(wallet)
	// FIX (Monster Audit follow-up, 2026-07-12, P0): see applyTransferDeltaLocked's
	// comment — same cold-cache pattern. Here a cold wallet fails as
	// "not found" and this secondary rejects a block its primary already
	// accepted, diverging from consensus.
	cs.ensureAccountLoadedCtx(ctx, wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok {
		return fmt.Errorf("ubi reward: account not found: %s", wallet)
	}
	if err := cs.applyDemurrageLossLockedCtx(ctx, acc, demurrageLost); err != nil {
		return fmt.Errorf("ubi reward: could not settle %s demurrage: %w", wallet, err)
	}
	acc.Balance = acc.Balance.Add(NewDecimal(amount))
	touchActivityAt(acc, activityAt)
	if err := cs.enforceWealthCapLockedCtx(ctx, acc); err != nil {
		return fmt.Errorf("ubi reward: could not enforce wealth cap for %s: %w", wallet, err)
	}
	// FIX (audit recheck2, P0 #3): see ApplyTransferDelta's comment.
	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		return fmt.Errorf("ubi reward: could not save account %s: %w", wallet, err)
	}
	return nil
}

// ApplyUBIFinalizeDelta zeroes the UBI pool and records last_ubi_at,
// mirroring the unconditional finalization DistributeUBIPool performs on
// the primary after crediting every human. ubiAt must be the SAME value
// for every node — main.go passes the producing block's Timestamp, never
// time.Now(), so primary and secondaries agree exactly (see the audit
// recheck 2, P0 #4 finding this addresses: the primary used to call
// time.Now() directly inside DistributeUBIPool while secondaries replayed
// block.Timestamp, two different instants).
func (cs *ChainState) ApplyUBIFinalizeDelta(ubiAt int64) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox/
	// runAtomicDistributionWithOutbox — see RegisterHuman's comment.
	return cs.applyUBIFinalizeDeltaLocked(context.Background(), ubiAt)
}

// applyUBIFinalizeDeltaLocked is ApplyUBIFinalizeDelta's body, callable from
// inside RunDailyDistributionAtomic where cs.mu is already held — see
// DistributeValidatorsPool's comment for the same pattern. Block replay
// (block.go) also calls this directly with context.Background(): it sets
// dag.state.activeTx itself before this runs, and dbExecCtx falls back to
// that field when ctx carries no transaction, so behavior there is
// unchanged — see registerHumanLocked's comment for the same reasoning.
//
// FIX (audit recheck2, P0 #3): used to return nothing, discarding
// saveAccountToDB's error — see ApplyTransferDelta's comment.
func (cs *ChainState) applyUBIFinalizeDeltaLocked(ctx context.Context, ubiAt int64) error {
	// FIX (Monster Audit 2026-07-12, P1): see applyUBIDeltaLocked's comment on
	// the same pattern — a cold pool address must not silently skip zeroing.
	cs.ensureAccountLoadedCtx(ctx, ubiPoolAddr)
	if ubiAcc, ok := cs.accounts.Get(ubiPoolAddr); ok {
		ubiAcc.Balance = NewDecimal(0)
		if err := cs.saveAccountToDBCtx(ctx, ubiAcc); err != nil {
			return fmt.Errorf("ubi finalize: could not save pool account: %w", err)
		}
	}
	if err := cs.setConfigValueCtx(ctx, "last_ubi_at", fmt.Sprintf("%d", ubiAt)); err != nil {
		return fmt.Errorf("ubi finalize: could not save last_ubi_at: %w", err)
	}
	return nil
}

// ApplyValidatorRewardDelta credits a single validator-pool reward to
// wallet, settling the EXACT demurrage loss the primary already computed
// for that wallet (so secondaries don't need to — and can't — recompute
// it independently). Used by secondary nodes replaying
// "validator_distribution" TXs. The validators pool itself is zeroed by a
// separate "validator_distribution_pool_zero" TX (see
// ApplyValidatorPoolZeroDelta) emitted once per distribution round, not by
// this per-recipient delta — mirrors DistributeValidatorsPool's own
// structure (credit loop, then a single unconditional pool zero-out).
func (cs *ChainState) ApplyValidatorRewardDelta(wallet string, amount, demurrageLost float64) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox — see
	// RegisterHuman's comment.
	// activityAt 0 — no block here, so touchActivityAt falls back to nowUnix();
	// see ApplyTransferDelta's identical note.
	return cs.applyValidatorRewardDeltaLocked(context.Background(), wallet, amount, demurrageLost, 0)
}

// applyValidatorRewardDeltaLocked is ApplyValidatorRewardDelta's body — see
// applyTransferDeltaLocked's comment.
// activityAt is the replayed block's own Timestamp — see touchActivityAt for
// why a replay handler must not read this node's wall clock.
func (cs *ChainState) applyValidatorRewardDeltaLocked(ctx context.Context, wallet string, amount, demurrageLost float64, activityAt int64) error {
	wallet = strings.ToLower(wallet)
	// FIX (Monster Audit follow-up, 2026-07-12, P0): see applyTransferDeltaLocked's
	// comment — same cold-cache pattern. Here a cold wallet was blind-created
	// as a fresh zero-balance AccountState below, silently wiping any real
	// balance/tusd/lp/is_human it already had via saveAccountToDB's
	// Version==0 upsert.
	cs.ensureAccountLoadedCtx(ctx, wallet)
	if _, ok := cs.accounts.Get(wallet); !ok {
		cs.accounts.Set(wallet, &AccountState{Address: wallet})
	}
	acc, _ := cs.accounts.Get(wallet)
	if err := cs.applyDemurrageLossLockedCtx(ctx, acc, demurrageLost); err != nil {
		return fmt.Errorf("validator reward: could not settle %s demurrage: %w", wallet, err)
	}
	acc.Balance = acc.Balance.Add(NewDecimal(amount))
	touchActivityAt(acc, activityAt)
	if err := cs.enforceWealthCapLockedCtx(ctx, acc); err != nil {
		return fmt.Errorf("validator reward: could not enforce wealth cap for %s: %w", wallet, err)
	}
	// FIX (audit recheck2, P0 #3): see ApplyTransferDelta's comment.
	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		return fmt.Errorf("validator reward: could not save account %s: %w", wallet, err)
	}
	return nil
}

// ApplyValidatorPoolZeroDelta zeroes the validators pool, mirroring the
// unconditional zero-out DistributeValidatorsPool performs on the primary
// after crediting every recipient.
//
// FIX (audit recheck2, P0 #3): used to return nothing — see
// ApplyTransferDelta's comment.
func (cs *ChainState) ApplyValidatorPoolZeroDelta() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox — see
	// RegisterHuman's comment.
	return cs.applyValidatorPoolZeroDeltaLocked(context.Background())
}

// applyValidatorPoolZeroDeltaLocked is ApplyValidatorPoolZeroDelta's body —
// see applyTransferDeltaLocked's comment.
func (cs *ChainState) applyValidatorPoolZeroDeltaLocked(ctx context.Context) error {
	// FIX (Monster Audit 2026-07-12, P1): see applyUBIDeltaLocked's comment on
	// the same pattern — a cold pool address must not silently skip zeroing.
	cs.ensureAccountLoadedCtx(ctx, validatorsPoolAddr)
	if acc, ok := cs.accounts.Get(validatorsPoolAddr); ok {
		acc.Balance = NewDecimal(0)
		if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
			return fmt.Errorf("validator pool zero: could not save pool account: %w", err)
		}
	}
	return nil
}

// ApplyLPRewardDelta credits a single LP-pool reward to wallet, settling
// the EXACT demurrage loss the primary already computed for that wallet
// in its pre-pass over ALL holders (before the pool total was read). Used
// by secondary nodes replaying "lp_distribution" TXs.
//
// FIX (audit recheck 2, P0 #6): this used to only credit the reward amount,
// on the theory that demurrage was "already settled" on the primary before
// the pool was read — true for the PRIMARY's own in-memory state, but that
// settlement (a balance reduction + pool credit) was never replayed on
// secondaries at all, since DistributionShare didn't carry it. Any LP
// holder with accrued demurrage caused permanent StateRoot divergence on
// every single LP distribution. DistributeLPPool now returns DemurrageLost
// per holder; this applies it via applyDemurrageLossLocked exactly like
// ApplyValidatorRewardDelta already did for validator rewards.
// The LP pool itself is zeroed by a separate "lp_distribution_pool_zero"
// TX (see ApplyLPPoolZeroDelta).
func (cs *ChainState) ApplyLPRewardDelta(wallet string, amount, demurrageLost float64) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox — see
	// RegisterHuman's comment.
	// activityAt 0 — no block here, so touchActivityAt falls back to nowUnix();
	// see ApplyTransferDelta's identical note.
	return cs.applyLPRewardDeltaLocked(context.Background(), wallet, amount, demurrageLost, 0)
}

// applyLPRewardDeltaLocked is ApplyLPRewardDelta's body — see
// applyTransferDeltaLocked's comment.
// activityAt is the replayed block's own Timestamp — see touchActivityAt for
// why a replay handler must not read this node's wall clock.
func (cs *ChainState) applyLPRewardDeltaLocked(ctx context.Context, wallet string, amount, demurrageLost float64, activityAt int64) error {
	wallet = strings.ToLower(wallet)
	// FIX (Monster Audit follow-up, 2026-07-12, P0): see applyValidatorRewardDeltaLocked's
	// comment — same cold-cache blind-create/silent-wipe pattern.
	cs.ensureAccountLoadedCtx(ctx, wallet)
	if _, ok := cs.accounts.Get(wallet); !ok {
		cs.accounts.Set(wallet, &AccountState{Address: wallet})
	}
	acc, _ := cs.accounts.Get(wallet)
	if err := cs.applyDemurrageLossLockedCtx(ctx, acc, demurrageLost); err != nil {
		return fmt.Errorf("lp reward: could not settle %s demurrage: %w", wallet, err)
	}
	acc.Balance = acc.Balance.Add(NewDecimal(amount))
	touchActivityAt(acc, activityAt)
	if err := cs.enforceWealthCapLockedCtx(ctx, acc); err != nil {
		return fmt.Errorf("lp reward: could not enforce wealth cap for %s: %w", wallet, err)
	}
	// FIX (audit recheck2, P0 #3): see ApplyTransferDelta's comment.
	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		return fmt.Errorf("lp reward: could not save account %s: %w", wallet, err)
	}
	return nil
}

// ApplyLPPoolZeroDelta zeroes the LP pool, mirroring the unconditional
// zero-out DistributeLPPool performs on the primary after crediting every
// holder.
//
// FIX (audit recheck2, P0 #3): used to return nothing — see
// ApplyTransferDelta's comment.
func (cs *ChainState) ApplyLPPoolZeroDelta() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox — see
	// RegisterHuman's comment.
	return cs.applyLPPoolZeroDeltaLocked(context.Background())
}

// applyLPPoolZeroDeltaLocked is ApplyLPPoolZeroDelta's body — see
// applyTransferDeltaLocked's comment.
func (cs *ChainState) applyLPPoolZeroDeltaLocked(ctx context.Context) error {
	// FIX (Monster Audit 2026-07-12, P1): see applyUBIDeltaLocked's comment on
	// the same pattern — a cold pool address must not silently skip zeroing.
	cs.ensureAccountLoadedCtx(ctx, lpPoolAddr)
	if acc, ok := cs.accounts.Get(lpPoolAddr); ok {
		acc.Balance = NewDecimal(0)
		if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
			return fmt.Errorf("lp pool zero: could not save pool account: %w", err)
		}
	}
	return nil
}

// ApplyEscrowMoveDelta zeroes wallet's balance after settling the EXACT
// demurrage loss the primary already computed, mirroring
// CheckAndMoveToEscrow's effect on a single wallet. Used by secondary nodes
// replaying "escrow_move" TXs. Secondaries don't maintain an
// escrow_accounts row at all — only the balance zeroing affects StateRoot,
// and secondaries never independently decide who to escrow (see
// main.go's primary-only gate).
//
// lpSharesBurned/tusdConverted (beta-launch audit 2026-07-05) carry the exact
// LP-shares-burned input the primary computed in checkAndMoveToEscrowLocked
// when a wallet's wealth wasn't (only) liquid AEQ — see that function's
// comment. Re-derived here against THIS node's own pool state, the same way
// removeLiquidityDeltaLocked re-derives output from a primary-supplied input
// rather than trusting a primary-supplied output.
func (cs *ChainState) ApplyEscrowMoveDelta(wallet string, demurrageLost, lpSharesBurned, tusdConverted float64) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox/
	// runAtomicDistributionWithOutbox — see RegisterHuman's comment.
	return cs.applyEscrowMoveDeltaLocked(context.Background(), wallet, demurrageLost, lpSharesBurned, tusdConverted)
}

// applyEscrowMoveDeltaLocked is ApplyEscrowMoveDelta's body — see
// applyTransferDeltaLocked's comment. Block replay (block.go) also calls
// this directly with context.Background(): it sets dag.state.activeTx
// itself before this runs, and dbExecCtx falls back to that field when ctx
// carries no transaction, so behavior there is unchanged — see
// registerHumanLocked's comment for the same reasoning.
func (cs *ChainState) applyEscrowMoveDeltaLocked(ctx context.Context, wallet string, demurrageLost, lpSharesBurned, tusdConverted float64) error {
	wallet = strings.ToLower(wallet)
	// FIX (Monster Audit follow-up, 2026-07-12, P0): see applyTransferDeltaLocked's
	// comment — same cold-cache pattern. Here a cold wallet fails as
	// "not found" and this secondary rejects a block its primary already
	// accepted, diverging from consensus.
	cs.ensureAccountLoadedCtx(ctx, wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok {
		return fmt.Errorf("escrow move: account not found: %s", wallet)
	}
	// FIX (2026-07-07): mirror addLiquidityDeltaLocked/removeLiquidityDeltaLocked,
	// which both reload cs.pool from the DB before using it for AMM math (see
	// reloadPoolFromDB's own comment on the stale-in-memory-pool class of bug
	// this guards against) — liquidateLPSharesForEscrowLocked/
	// convertTUsdForEscrowLocked below touch cs.pool the same way those do,
	// so this replay path should start from the same authoritative state.
	cs.reloadPoolFromDB()
	if err := cs.applyDemurrageLossLockedCtx(ctx, acc, demurrageLost); err != nil {
		return fmt.Errorf("escrow move: could not settle %s demurrage: %w", wallet, err)
	}

	// Replay the primary's LP liquidation / tUSD conversion via the exact
	// same shared math checkAndMoveToEscrowLocked used (guardian.go) — see
	// liquidateLPSharesForEscrowLocked/convertTUsdForEscrowLocked's own
	// comments for why a shared implementation, not a hand-written copy, is
	// what makes this structurally guaranteed to match the primary.
	if lpSharesBurned > 0 {
		burned, _, _, err := cs.liquidateLPSharesForEscrowLocked(ctx, acc, lpSharesBurned)
		if err != nil {
			return fmt.Errorf("escrow move: could not replay LP liquidation for %s: %w", wallet, err)
		}
		if burned <= 0 {
			return fmt.Errorf("escrow move: primary reported %.6f LP shares burned for %s but this node's pool has none — pool state has diverged", lpSharesBurned, wallet)
		}
	}

	if tusdConverted > 0 {
		_, ok, err := cs.convertTUsdForEscrowLocked(ctx, acc, tusdConverted)
		if err != nil {
			return fmt.Errorf("escrow move: could not replay tUSD conversion for %s: %w", wallet, err)
		}
		if !ok {
			return fmt.Errorf("escrow move: primary reported a %.6f tUSD conversion for %s that this node's pool cannot replay — pool state has diverged", tusdConverted, wallet)
		}
	}

	acc.Balance = NewDecimal(0)
	acc.TUsdBalance = NewDecimal(0)
	acc.LPShares = NewDecimal(0)
	// FIX (audit recheck2, P0 #3): see ApplyTransferDelta's comment.
	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		return fmt.Errorf("escrow move: could not save account %s: %w", wallet, err)
	}
	return nil
}

// ApplyEscrowReleaseDelta credits amount to the UBI pool, mirroring
// ReleaseEscrowToUBI's effect for a single released wallet. Used by
// secondary nodes replaying "escrow_release" TXs.
func (cs *ChainState) ApplyEscrowReleaseDelta(amount float64) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox — see
	// RegisterHuman's comment.
	return cs.applyEscrowReleaseDeltaLocked(context.Background(), amount)
}

// applyEscrowReleaseDeltaLocked is ApplyEscrowReleaseDelta's body — see
// applyTransferDeltaLocked's comment.
func (cs *ChainState) applyEscrowReleaseDeltaLocked(ctx context.Context, amount float64) error {
	// FIX (Monster Audit 2026-07-12, P1): see distributeSwapFee's comment on
	// the same pattern — a cold pool address must be loaded before a blank
	// Version==0 AccountState is created for it, or the real DB balance gets
	// silently overwritten.
	cs.ensureAccountLoadedCtx(ctx, ubiPoolAddr)
	if _, ok := cs.accounts.Get(ubiPoolAddr); !ok {
		cs.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr})
	}
	ubiPoolAcc, _ := cs.accounts.Get(ubiPoolAddr)
	ubiPoolAcc.Balance = ubiPoolAcc.Balance.Add(NewDecimal(amount))
	// FIX (audit recheck2, P0 #3): see ApplyTransferDelta's comment.
	if err := cs.saveAccountToDBCtx(ctx, ubiPoolAcc); err != nil {
		return fmt.Errorf("escrow release: could not save pool account: %w", err)
	}
	return nil
}

// ApplyFaucetDelta credits faucetAmount tUSD to wallet and marks FaucetClaimed.
// Used by secondary nodes replaying faucet TXs from blocks.
func (cs *ChainState) ApplyFaucetDelta(wallet string, faucetAmount float64) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox — see
	// RegisterHuman's comment.
	return cs.applyFaucetDeltaLocked(context.Background(), wallet, faucetAmount)
}

// applyFaucetDeltaLocked is ApplyFaucetDelta's body — see
// applyTransferDeltaLocked's comment.
func (cs *ChainState) applyFaucetDeltaLocked(ctx context.Context, wallet string, faucetAmount float64) error {
	wallet = strings.ToLower(wallet)
	// FIX (Monster Audit follow-up, 2026-07-12, P0): see applyValidatorRewardDeltaLocked's
	// comment — same cold-cache blind-create/silent-wipe pattern. Also matters
	// for FaucetClaimed specifically: a blind-created account always starts
	// with FaucetClaimed=false, which would have silently let a cold wallet
	// that already claimed the faucet claim it again on replay.
	cs.ensureAccountLoadedCtx(ctx, wallet)
	if _, ok := cs.accounts.Get(wallet); !ok {
		cs.accounts.Set(wallet, &AccountState{Address: wallet})
	}
	acc, _ := cs.accounts.Get(wallet)
	if acc.FaucetClaimed {
		return nil // idempotent: already applied
	}
	acc.FaucetClaimed = true
	acc.TUsdBalance = acc.TUsdBalance.Add(NewDecimal(faucetAmount))
	// FIX (audit recheck2, P0 #3): see ApplyTransferDelta's comment.
	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		return fmt.Errorf("faucet: could not save account %s: %w", wallet, err)
	}
	return nil
}

// V6 Contract State Mirror - persists EVM contract state to PostgreSQL
func (cs *ChainState) InitV6StateTables() {
	if cs.db == nil {
		return
	}
	cs.db.Exec(`CREATE TABLE IF NOT EXISTS v6_state (
key TEXT PRIMARY KEY,
value TEXT NOT NULL,
updated_at TIMESTAMP DEFAULT NOW()
)`)
	cs.db.Exec(`CREATE TABLE IF NOT EXISTS v6_humans (
address TEXT PRIMARY KEY,
commitment TEXT,
is_human BOOLEAN DEFAULT true,
is_inactive BOOLEAN DEFAULT false,
registered_at TIMESTAMP DEFAULT NOW(),
last_activity TIMESTAMP DEFAULT NOW()
)`)
	cs.db.Exec(`CREATE TABLE IF NOT EXISTS v6_balances (
address TEXT PRIMARY KEY,
balance_wei TEXT NOT NULL,
updated_at TIMESTAMP DEFAULT NOW()
)`)
	cs.db.Exec(`CREATE TABLE IF NOT EXISTS v6_commitments (
commitment TEXT PRIMARY KEY,
wallet TEXT NOT NULL,
used_at TIMESTAMP DEFAULT NOW()
)`)
	fmt.Println("[V6] State tables initialized")
}

func (cs *ChainState) SaveV6State(key, value string) {
	if cs.db == nil {
		return
	}
	cs.db.Exec(
		`INSERT INTO v6_state (key, value) VALUES ($1, $2)
 ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = NOW()`,
		key, value,
	)
}

func (cs *ChainState) LoadV6State(key string) string {
	if cs.db == nil {
		return ""
	}
	var value string
	cs.db.QueryRow(`SELECT value FROM v6_state WHERE key = $1`, key).Scan(&value)
	return value
}

func (cs *ChainState) SaveV6Balance(address, balanceWei string) {
	if cs.db == nil {
		return
	}
	cs.db.Exec(
		`INSERT INTO v6_balances (address, balance_wei) VALUES ($1, $2)
 ON CONFLICT (address) DO UPDATE SET balance_wei = $2, updated_at = NOW()`,
		address, balanceWei,
	)
}

func (cs *ChainState) LoadV6Balance(address string) string {
	if cs.db == nil {
		return "0"
	}
	var balanceWei string
	cs.db.QueryRow(`SELECT balance_wei FROM v6_balances WHERE address = $1`, address).Scan(&balanceWei)
	if balanceWei == "" {
		return "0"
	}
	return balanceWei
}

func (cs *ChainState) SaveV6Human(address, commitment string) {
	if cs.db == nil {
		return
	}
	cs.db.Exec(
		`INSERT INTO v6_humans (address, commitment) VALUES ($1, $2)
 ON CONFLICT (address) DO UPDATE SET commitment = $2, last_activity = NOW()`,
		address, commitment,
	)
}

func (cs *ChainState) SaveV6Commitment(commitment, wallet string) {
	if cs.db == nil {
		return
	}
	cs.db.Exec(
		`INSERT INTO v6_commitments (commitment, wallet) VALUES ($1, $2)
 ON CONFLICT (commitment) DO NOTHING`,
		commitment, wallet,
	)
}

func (cs *ChainState) GetAllV6Humans() []map[string]string {
	if cs.db == nil {
		return nil
	}
	rows, err := cs.db.Query(
		`SELECT address, commitment FROM v6_humans WHERE is_human = true AND is_inactive = false`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var humans []map[string]string
	for rows.Next() {
		var addr, commitment string
		rows.Scan(&addr, &commitment)
		humans = append(humans, map[string]string{
			"address":    addr,
			"commitment": commitment,
		})
	}
	return humans
}

func (cs *ChainState) GetAllV6Balances() []map[string]string {
	if cs.db == nil {
		return nil
	}
	rows, err := cs.db.Query(`SELECT address, balance_wei FROM v6_balances`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var balances []map[string]string
	for rows.Next() {
		var addr, bal string
		rows.Scan(&addr, &bal)
		balances = append(balances, map[string]string{
			"address":     addr,
			"balance_wei": bal,
		})
	}
	return balances
}
