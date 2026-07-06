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
	"sort"
	"strings"
	"sync"
	"time"

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
	mu         sync.RWMutex
	accounts   map[string]*AccountState
	pool       *PoolState
	db         *sql.DB
	useDB      bool
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
	nullifiers map[string]string // nullifier hex → wallet address (in-memory cache)
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
	// activeTx, when non-nil, is the transaction every DB write inside the
	// CURRENT cs.mu-locked operation must use instead of cs.db directly —
	// see dbExec() and runAtomicWithOutbox. Only ever set/cleared while
	// cs.mu is held (write-locked), so reading it without separate
	// synchronization is safe: at most one goroutine can be inside a
	// cs.mu-locked region at a time, and that goroutine is the only one
	// that could have set it.
	activeTx *sql.Tx

	// degradedMu guards bootstrapDegradedReason. Set by main.go when
	// snapshot bootstrap/resync's EVM mirror migration step fails (see
	// ImportSnapshotFromURL/ResyncFromSnapshotURL) — Go-state itself is
	// fine (the source of truth), but eth_call/V7 contract storage reads
	// may be stale until the next successful migration. Surfaced via
	// /api/health/combined so this doesn't silently sit unnoticed (audit
	// 2026-06-28 recheck 5, P1-3: "Startup/Bootstrap sollte ... mindestens
	// einen Health-Status degraded setzen").
	degradedMu             sync.RWMutex
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
func (cs *ChainState) dbExec() sqlExecutor {
	if cs.activeTx != nil {
		return cs.activeTx
	}
	return cs.db
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
		accounts:   make(map[string]*AccountState),
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
			db.SetMaxOpenConns(20)
			db.SetMaxIdleConns(20)
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
		"pending_txs",      // prevent stale TXs from polluting post-reset state
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
		// FIX: bio_hashes was never cleared here. Nothing on the chain side
		// reads this table for registration blocking today (it's write-only,
		// populated by SaveBioHash/snapshot import), but leaving stale rows
		// behind after every other registration table is wiped is an
		// inconsistent half-reset, and it's the SAME table name as the one
		// the separate proof-server service uses for its own (much more
		// consequential) duplicate-biometric check — clearing it here keeps
		// the chain's own copy honest regardless of what reads it later.
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
func (cs *ChainState) setConfigValue(key, value string) error {
	if cs.db == nil {
		return nil
	}
	if _, err := cs.dbExec().Exec(`INSERT INTO chain_config (key, value) VALUES ($1, $2)
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
	for _, acc := range cs.accounts {
		if acc.IsHuman {
			humans++
		}
	}
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
func (cs *ChainState) ensureAccountLoaded(addr string) {
	if _, ok := cs.accounts[addr]; ok {
		return
	}
	if cs.db == nil {
		return
	}
	acc := &AccountState{Address: addr}
	var bal, tusd, lp float64
	var version int64
	err := cs.db.QueryRow(
		`SELECT balance, is_human, tusd_balance, lp_shares,
		        COALESCE(last_activity_at, 0), COALESCE(version, 1)
		 FROM chain_accounts WHERE lower(address) = $1`,
		addr,
	).Scan(&bal, &acc.IsHuman, &tusd, &lp, &acc.LastActivityAt, &version)
	if err != nil {
		return // not in DB — caller's !ok branch will create a fresh account
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
	cs.accounts[addr] = acc
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
		if existing, ok := cs.accounts[normalized]; ok {
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
		cs.accounts[normalized] = acc
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
	cs.accounts = accounts
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
	err := cs.saveAccountToDBInner(acc)
	if err == nil {
		// Incremental state-root maintenance (see ChainState.accountSetXOR):
		// the row write succeeded, so replace this account's previously-counted
		// leaf with its new one. acc.leafHash is whatever was last folded into
		// the accumulator for this account (the zero hash for a brand-new one);
		// XOR is self-inverse, so out-then-in keeps accountSetXOR equal to the
		// XOR of every current account's leaf. Every caller holds cs.mu for the
		// whole mutation (see this function's doc comment), so this shared-state
		// update needs no extra synchronization, and it participates in block
		// rollback via blockRollbackSnapshot.accountSetXOR.
		newLeaf := accountLeaf(acc)
		xorInto(&cs.accountSetXOR, acc.leafHash)
		xorInto(&cs.accountSetXOR, newLeaf)
		acc.leafHash = newLeaf
		return nil
	}
	if errors.Is(err, errVersionConflict) {
		return fmt.Errorf("version conflict for account %s (resynced to DB version %d; caller must retry its business operation against fresh state, not this write alone): %w", acc.Address, acc.Version, err)
	}
	return err
}

func (cs *ChainState) saveAccountToDBInner(acc *AccountState) error {
	if !cs.useDB {
		acc.Version++ // no-DB mode: mark as saved
		return nil
	}
	var result sql.Result
	var err error
	// FIX (atomic outbox): use cs.dbExec() instead of cs.db directly — when
	// runAtomicWithOutbox has an active transaction open for the current
	// operation, this write becomes part of it (committed or rolled back
	// together with the pending_tx outbox insert) instead of always
	// auto-committing on its own connection.
	if acc.Version == 0 {
		// First write: INSERT with version=1, or update if exists without version conflict check
		result, err = cs.dbExec().Exec(`INSERT INTO chain_accounts (address, balance, is_human, tusd_balance, lp_shares, last_activity_at, demurrage_14_day_warning_shown, faucet_claimed, version) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1)
ON CONFLICT (address) DO UPDATE SET balance = $2, is_human = $3, tusd_balance = $4, lp_shares = $5, last_activity_at = $6, demurrage_14_day_warning_shown = $7, faucet_claimed = $8, version = COALESCE(chain_accounts.version,0) + 1`,
			acc.Address, acc.Balance.Float(), acc.IsHuman, acc.TUsdBalance.Float(), acc.LPShares.Float(), acc.LastActivityAt, acc.Demurrage14DayWarningShown, acc.FaucetClaimed)
	} else {
		// Optimistic locking: only update if version matches what we read.
		// If another node updated in parallel, rows affected = 0 → conflict detected.
		result, err = cs.dbExec().Exec(`UPDATE chain_accounts SET balance = $2, is_human = $3, tusd_balance = $4, lp_shares = $5, last_activity_at = $6, demurrage_14_day_warning_shown = $7, faucet_claimed = $8, version = $9 + 1
WHERE address = $1 AND version = $9`,
			acc.Address, acc.Balance.Float(), acc.IsHuman, acc.TUsdBalance.Float(), acc.LPShares.Float(), acc.LastActivityAt, acc.Demurrage14DayWarningShown, acc.FaucetClaimed, acc.Version)
		if err == nil {
			if rows, _ := result.RowsAffected(); rows == 0 {
				// Conflict: another node wrote a newer version. Reload DB version
				// into memory so the next caller can retry with the correct base.
				var dbVer int64
				cs.db.QueryRow(`SELECT version FROM chain_accounts WHERE lower(address) = $1`, acc.Address).Scan(&dbVer)
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
	if err := cs.distributeSwapFee(lost.Float(), true); err != nil {
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
func (cs *ChainState) applyDemurrageLossLocked(acc *AccountState, lost float64) error {
	if lost <= 0 || isTokenomicsPoolAddress(acc.Address) {
		return nil
	}
	acc.Balance = NewDecimal(round6(acc.Balance.Float() - lost))
	if err := cs.distributeSwapFee(lost, true); err != nil {
		return fmt.Errorf("could not persist pool credits for %s demurrage delta: %w", acc.Address, err)
	}
	return nil
}

func (cs *ChainState) GetBalance(address string) float64 {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	address = strings.ToLower(address)
	cs.ensureAccountLoaded(address)
	if acc, ok := cs.accounts[address]; ok {
		return effectiveBalance(acc).Float()
	}
	return 0
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
// signing address that has ever registered, numbered by registration
// order (registered_at ASC) — 1-indexed, keyed by lower-cased
// signing_address (the same address that appears as a block's Proposer
// field). Used by the explorer to show a friendly label alongside a raw
// 0x address without hardcoding any specific node's identity — a
// deliberate design choice: this project explicitly invites any
// registered human to run a validator node, so baking in "Primary/
// Contabo 1/Contabo 2"-style names would need updating by hand every time
// a new node joins and would misrepresent the network as closed. The
// ordinal is purely registration order, learned the same way for every
// node without any operator ever hand-editing this list.
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
	shares, err := cs.distributeValidatorsPoolLocked()
	if err != nil {
		fmt.Printf("[VALIDATORS] Error: %v\n", err)
		return nil
	}
	return shares
}

func (cs *ChainState) distributeValidatorsPoolLocked() ([]DistributionShare, error) {
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
		rows, _ := cs.dbExec().Query(`SELECT wallet_address, blocks_produced FROM registered_nodes WHERE wallet_address = ANY($1)`, pq.Array(nodes))
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

	poolAcc, ok := cs.accounts[validatorsPoolAddr]
	if !ok || poolAcc.Balance <= 0 {
		fmt.Println("[VALIDATORS] Pool is empty — nothing to distribute today")
		return nil, nil
	}

	total := poolAcc.Balance.Float()
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
		if _, ok := cs.accounts[wallet]; !ok {
			cs.accounts[wallet] = &AccountState{Address: wallet}
		}
		acc := cs.accounts[wallet]
		lost, err := cs.settleDemurrageLocked(acc)
		if err != nil {
			return nil, fmt.Errorf("could not settle demurrage for %s: %w", wallet, err)
		}
		acc.Balance = NewDecimal(round6(acc.Balance.Float() + share))
		touchActivity(acc)
		if err := cs.enforceWealthCapLocked(acc); err != nil {
			return nil, fmt.Errorf("could not enforce wealth cap for %s: %w", wallet, err)
		}
		if err := cs.saveAccountToDB(acc); err != nil {
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
		if err := cs.saveAccountToDB(poolAcc); err != nil {
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
	shares, err := cs.distributeLPPoolLocked()
	if err != nil {
		fmt.Printf("[LP] Error: %v\n", err)
		return nil
	}
	return shares
}

func (cs *ChainState) distributeLPPoolLocked() ([]DistributionShare, error) {
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
		rows, err := cs.db.Query(`SELECT lower(address) FROM chain_accounts WHERE lp_shares > 0`)
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
		for _, addr := range addrs {
			cs.ensureAccountLoaded(addr) // page in cold accounts so LP distribution works beyond the in-memory cap
			if acc, ok := cs.accounts[addr]; ok && acc.LPShares.Float() > 0 {
				holders = append(holders, lpHolder{addr, acc.LPShares.Float()})
				totalShares += acc.LPShares.Float()
			}
		}
	} else {
		// No DB (unit tests): fall back to in-memory iteration.
		for addr, acc := range cs.accounts {
			if acc.LPShares > 0 {
				holders = append(holders, lpHolder{addr, acc.LPShares.Float()})
				totalShares += acc.LPShares.Float()
			}
		}
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
		acc := cs.accounts[h.addr]
		lost, err := cs.settleDemurrageLocked(acc)
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
		if acc, ok := cs.accounts[h.addr]; ok {
			totalShares += acc.LPShares.Float()
		}
	}
	if totalShares <= 0 {
		fmt.Println("[LP] totalShares collapsed to zero after demurrage loop — pool left untouched")
		return nil, nil
	}

	// NOW read the pool balance — it includes any demurrage credits just added.
	poolAcc, ok := cs.accounts[lpPoolAddr]
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
		acc := cs.accounts[h.addr]
		acc.Balance = NewDecimal(round6(acc.Balance.Float() + share))
		touchActivity(acc)
		if err := cs.enforceWealthCapLocked(acc); err != nil {
			return nil, fmt.Errorf("could not enforce wealth cap for %s: %w", h.addr, err)
		}
		if err := cs.saveAccountToDB(acc); err != nil {
			return nil, fmt.Errorf("could not save LP reward for %s: %w", h.addr, err)
		}
		shares = append(shares, DistributionShare{Wallet: h.addr, Amount: share, DemurrageLost: demurrageLost[h.addr]})
	}
	if totalDistributed > 0 {
		poolAcc.Balance = NewDecimal(0)
		if err := cs.saveAccountToDB(poolAcc); err != nil {
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
	defer cs.mu.Unlock()
	shares, err := cs.distributeUBIPoolLocked()
	if err != nil {
		fmt.Printf("[UBI] Error: %v\n", err)
		return nil
	}
	return shares
}

func (cs *ChainState) distributeUBIPoolLocked() ([]DistributionShare, error) {
	poolAcc, ok := cs.accounts[ubiPoolAddr]
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
		rows, err := cs.db.Query(`SELECT lower(address) FROM chain_accounts WHERE is_human = true`)
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
		for addr, acc := range cs.accounts {
			if acc.IsHuman {
				humanAddrs = append(humanAddrs, addr)
			}
		}
	}
	if len(humanAddrs) == 0 {
		fmt.Println("[UBI] No registered humans yet — pool left untouched")
		return nil, nil
	}
	// Ensure all human accounts are in the cache so the distribution loop
	// below can work on in-memory objects (reads + writes stay coherent).
	for _, addr := range humanAddrs {
		cs.ensureAccountLoaded(addr)
	}

	// E3-FIX for UBI: settle demurrage for ALL humans FIRST. settleDemurrageLocked
	// credits 20% of each human's decay to ubiPoolAddr. Reading the pool balance
	// BEFORE this loop would miss those credits; zeroing AFTER distributes them.
	// Same fix applied to DistributeLPPool. Capture each human's own loss for
	// the returned DistributionShare — see the function comment above.
	demurrageLost := make(map[string]float64, len(humanAddrs))
	for _, addr := range humanAddrs {
		lost, err := cs.settleDemurrageLocked(cs.accounts[addr])
		if err != nil {
			return nil, fmt.Errorf("could not settle demurrage for %s: %w", addr, err)
		}
		demurrageLost[addr] = lost.Float()
	}
	// NOW read pool balance — includes any demurrage credits just added.
	poolAcc, ok = cs.accounts[ubiPoolAddr]
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
	shares := make([]DistributionShare, 0, len(humanAddrs))
	for _, addr := range humanAddrs {
		acc := cs.accounts[addr]
		acc.Balance = NewDecimal(round6(acc.Balance.Float() + share))
		touchActivity(acc)
		if err := cs.enforceWealthCapLocked(acc); err != nil {
			return nil, fmt.Errorf("could not enforce wealth cap for %s: %w", addr, err)
		}
		if err := cs.saveAccountToDB(acc); err != nil {
			return nil, fmt.Errorf("could not save UBI reward for %s: %w", addr, err)
		}
		shares = append(shares, DistributionShare{Wallet: addr, Amount: round6(share), DemurrageLost: demurrageLost[addr]})
	}
	poolAcc.Balance = NewDecimal(0)
	if err := cs.saveAccountToDB(poolAcc); err != nil {
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
	humans := 0
	for _, acc := range cs.accounts {
		if acc.IsHuman {
			humans++
		}
	}
	if humans == 0 {
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
func (cs *ChainState) bootstrapMultiplierLocked() float64 {
	count := 0
	for _, acc := range cs.accounts {
		if acc.IsHuman {
			count++
		}
	}
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
	if err := cs.distributeSwapFee(excess, true); err != nil {
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

	acc, ok := cs.accounts[address]
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
	cs.mu.Lock()
	defer cs.mu.Unlock()
	address = strings.ToLower(address)
	cs.ensureAccountLoaded(address)
	if acc, ok := cs.accounts[address]; ok {
		return acc.TUsdBalance.Float()
	}
	return 0
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
	cs.mu.Lock()
	defer cs.mu.Unlock()
	address = strings.ToLower(address)
	cs.ensureAccountLoaded(address)
	if acc, ok := cs.accounts[address]; ok {
		return acc.IsHuman
	}
	return false
}

func (cs *ChainState) RegisterHuman(address string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.registerHumanLocked(address)
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
func (cs *ChainState) RegisterHumanAtomic(address string, pendingTx Transaction) error {
	address = strings.ToLower(address)
	return cs.runAtomicWithOutbox([]string{address}, false, func() (Transaction, error) {
		if err := cs.registerHumanLocked(address); err != nil {
			return Transaction{}, err
		}
		if pendingTx.Nullifier != "" {
			if err := cs.SaveNullifier(pendingTx.Nullifier, address); err != nil {
				return Transaction{}, err
			}
		}
		return pendingTx, nil
	})
}

// registerHumanLocked is RegisterHuman's implementation; caller must
// already hold cs.mu — see transferLocked's comment for why this split
// exists.
func (cs *ChainState) registerHumanLocked(address string) error {
	address = strings.ToLower(address)
	cs.ensureAccountLoaded(address)

	if acc, ok := cs.accounts[address]; ok && acc.IsHuman {
		return fmt.Errorf("already registered")
	}

	if _, ok := cs.accounts[address]; !ok {
		cs.accounts[address] = &AccountState{Address: address}
	}

	cs.accounts[address].IsHuman = true
	cs.accounts[address].Balance = cs.accounts[address].Balance.Add(NewDecimal(1000))
	touchActivity(cs.accounts[address]) // starts this 1,000 AEQ's own grace period fresh
	if err := cs.enforceWealthCapLocked(cs.accounts[address]); err != nil {
		return fmt.Errorf("could not enforce wealth cap: %w", err)
	}
	if err := cs.saveAccountToDB(cs.accounts[address]); err != nil {
		return fmt.Errorf("could not save account: %w", err)
	}
	cs.save()

	fmt.Printf("[STATE] ✓ Human registered: %s | Balance: %.2f AEQ\n",
		address, cs.accounts[address].Balance.Float())
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
func (cs *ChainState) runAtomicWithOutbox(touchedAddrs []string, fullSnapshot bool, fn func() (Transaction, error)) error {
	if cs.db == nil {
		// No DB configured — nothing to make atomic with. Every call site
		// of TransferAtomic/SwapAtomic/etc. treats a non-nil error here as
		// "the operation itself failed" (e.g. evm_rpc.go returns an RPC
		// error to the caller) — so this must NOT return an error just
		// because there's no outbox to use; the state mutation itself
		// already genuinely succeeded. This matches the pre-existing
		// no-DB-mode contract elsewhere in this file (e.g. saveAccountToDB
		// treats !cs.useDB as "mark as saved", not a failure).
		cs.mu.Lock()
		_, err := fn()
		cs.mu.Unlock()
		return err
	}

	tx, err := cs.db.Begin()
	if err != nil {
		return fmt.Errorf("could not begin atomic transaction: %w", err)
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
	chainConfig := make(map[string]configValueSnapshot, len(stateRootRelevantConfigKeys))
	for _, key := range stateRootRelevantConfigKeys {
		value, existed := cs.getConfigValueExistsDB(key)
		chainConfig[key] = configValueSnapshot{value: value, existed: existed}
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
	cs.activeTx = tx
	snap := cs.snapshotForRollbackLocked(touchedAddrs, fullSnapshot, chainConfig)
	pendingTx, fnErr := fn()
	var outboxErr error
	if fnErr == nil {
		outboxErr = savePendingTxExec(tx, pendingTx)
	}

	if fnErr != nil || outboxErr != nil {
		cs.activeTx = nil
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
		cs.activeTx = nil
		if rbErr := cs.restoreFromRollbackLocked(snap); rbErr != nil {
			fmt.Printf("[ATOMIC] CRITICAL: rollback persistence failed after commit failure — memory/DB may now disagree: %v\n", rbErr)
		}
		cs.mu.Unlock()
		return fmt.Errorf("commit failed (state mutation rolled back): %w", err)
	}
	cs.activeTx = nil
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
func (cs *ChainState) runAtomicDistributionWithOutbox(fn func() ([]Transaction, error)) error {
	if cs.db == nil {
		cs.mu.Lock()
		_, err := fn()
		cs.mu.Unlock()
		return err
	}

	tx, err := cs.db.Begin()
	if err != nil {
		return fmt.Errorf("could not begin atomic distribution transaction: %w", err)
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
	chainConfig := make(map[string]configValueSnapshot, len(stateRootRelevantConfigKeys))
	for _, key := range stateRootRelevantConfigKeys {
		value, existed := cs.getConfigValueExistsDB(key)
		chainConfig[key] = configValueSnapshot{value: value, existed: existed}
	}

	// FIX (audit 2026-06-28 recheck 4, P0-2): same fix as runAtomicWithOutbox
	// above — cs.mu now stays held continuously through the final
	// commit/rollback decision instead of being released beforehand, so no
	// concurrent operation can observe the new memory state and write
	// against cs.db while this transaction's fate is still undecided.
	cs.mu.Lock()
	cs.activeTx = tx
	snap := cs.snapshotForRollbackLocked(nil, true, chainConfig)
	txs, fnErr := fn()
	var outboxErr error
	if fnErr == nil {
		for _, t := range txs {
			if outboxErr = savePendingTxExec(tx, t); outboxErr != nil {
				break
			}
		}
	}

	if fnErr != nil || outboxErr != nil {
		cs.activeTx = nil
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
		cs.activeTx = nil
		if rbErr := cs.restoreFromRollbackLocked(snap); rbErr != nil {
			fmt.Printf("[ATOMIC] CRITICAL: distribution rollback persistence failed after commit failure — memory/DB may now disagree: %v\n", rbErr)
		}
		cs.mu.Unlock()
		return fmt.Errorf("commit failed (state mutation rolled back): %w", err)
	}
	cs.activeTx = nil
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
	return cs.runAtomicDistributionWithOutbox(func() ([]Transaction, error) {
		var txs []Transaction

		ubiShares, err := cs.distributeUBIPoolLocked()
		if err != nil {
			return nil, fmt.Errorf("UBI distribution failed: %w", err)
		}
		var ubiTotal float64
		for _, s := range ubiShares {
			txs = append(txs, Transaction{Type: "ubi_distribution", Wallet: s.Wallet, Amount: s.Amount, FromDemurrageLost: s.DemurrageLost})
			ubiTotal += s.Amount
		}
		if ubiTotal > 0 {
			if err := cs.applyUBIFinalizeDeltaLocked(ubiAt); err != nil {
				return nil, fmt.Errorf("UBI finalize failed: %w", err)
			}
			txs = append(txs, Transaction{Type: "ubi_distribution_finalize", DistributionAt: ubiAt})
		}

		validatorShares, err := cs.distributeValidatorsPoolLocked()
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

		lpShares, err := cs.distributeLPPoolLocked()
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

		moved, err := cs.checkAndMoveToEscrowLocked()
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

		released, err := cs.releaseEscrowToUBILocked()
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
	return cs.transferLocked(from, to, amount)
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
func (cs *ChainState) TransferAtomic(from, to string, amount float64, pendingTxTemplate Transaction) (fromLost, toLost float64, err error) {
	from = strings.ToLower(from)
	to = strings.ToLower(to)
	err = cs.runAtomicWithOutbox([]string{from, to, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr}, false, func() (Transaction, error) {
		fromLost, toLost, err = cs.transferLocked(from, to, amount)
		if err != nil {
			return Transaction{}, err
		}
		pendingTxTemplate.FromDemurrageLost = fromLost
		pendingTxTemplate.ToDemurrageLost = toLost
		return pendingTxTemplate, nil
	})
	return fromLost, toLost, err
}

// transferLocked is Transfer's actual implementation; caller must already
// hold cs.mu. Split out so TransferAtomic can run it under the SAME lock
// acquisition it uses to set/clear cs.activeTx (see runAtomicWithOutbox) —
// Transfer() itself locks cs.mu, so calling it from inside an already-locked
// context would deadlock on Go's non-reentrant sync.Mutex.
func (cs *ChainState) transferLocked(from, to string, amount float64) (float64, float64, error) {
	from = strings.ToLower(from)
	to = strings.ToLower(to)
	// P1-FIX: reject NaN/Inf amounts — these would corrupt balances via
	// NewDecimal which uses math.Round (NaN/Inf propagate silently).
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, 0, fmt.Errorf("invalid transfer amount: %v", amount)
	}
	// P2-5: reject self-transfers; mirrors AequitasV7.sol behaviour and
	// prevents double-demurrage settlement on the same account object.
	if from == to {
		return 0, 0, fmt.Errorf("self-transfer not allowed")
	}

	cs.ensureAccountLoaded(from)
	cs.ensureAccountLoaded(to)
	fromAcc, ok := cs.accounts[from]
	if !ok {
		return 0, 0, fmt.Errorf("insufficient balance")
	}
	fromLost, err := cs.settleDemurrageLocked(fromAcc) // make sure we're checking against the real, decayed balance
	if err != nil {
		return 0, 0, fmt.Errorf("could not settle demurrage for sender: %w", err)
	}
	if fromAcc.Balance.Float() < amount {
		return 0, 0, fmt.Errorf("insufficient balance")
	}

	fromAcc.Balance = NewDecimal(round6(fromAcc.Balance.Float() - amount))
	touchActivity(fromAcc) // sending counts as "using" the money — resets its decay clock
	// FIX (audit3, P1 #4): saveAccountToDB now returns an error — checked here
	// so a DB failure aborts the transfer (causing runAtomicWithOutbox to roll
	// back) instead of returning success while the debit was never persisted.
	if err := cs.saveAccountToDB(fromAcc); err != nil {
		return 0, 0, fmt.Errorf("could not save sender account: %w", err)
	}

	if _, ok := cs.accounts[to]; !ok {
		cs.accounts[to] = &AccountState{Address: to}
	}
	toLost, err := cs.settleDemurrageLocked(cs.accounts[to])
	if err != nil {
		return 0, 0, fmt.Errorf("could not settle demurrage for recipient: %w", err)
	}
	cs.accounts[to].Balance = NewDecimal(round6(cs.accounts[to].Balance.Float() + amount))
	touchActivity(cs.accounts[to]) // receiving also resets the clock on the recipient's whole balance
	if err := cs.enforceWealthCapLocked(cs.accounts[to]); err != nil {
		return 0, 0, fmt.Errorf("could not enforce wealth cap for recipient: %w", err)
	}
	if err := cs.saveAccountToDB(cs.accounts[to]); err != nil {
		return 0, 0, fmt.Errorf("could not save recipient account: %w", err)
	}
	cs.save()

	fmt.Printf("[STATE] ✓ Transfer %.2f AEQ: %s → %s\n", amount, from, to)
	cs.syncBalanceLocked(V7_CONTRACT_ADDR, from, to, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr)
	return fromLost.Float(), toLost.Float(), nil
}

// TransferWithV7Fee is used by the RPC layer when intercepting V7 ERC-20
// transfer() calls (selector a9059cbb) — this IS the fee path a real user's
// MetaMask "send AEQ" transaction actually goes through.
//
// FIX (P2-10, beta-launch audit 2026-07-05): this comment used to claim it
// "mirrors V7's _calcFee(): TX_FEE_BPS = 10 (0.1% base fee)" — that's wrong
// on both counts. AequitasV7.sol's own TX_FEE_BPS constant is 700 (7%), not
// 10, and calcV7Fee below independently hardcodes its own 0.1% base +
// tiered concentration surcharge (0/0.1%/0.5%/1%) rather than reading the
// contract's constant at all. These are two genuinely different fee
// schedules for the same nominal transfer() call — not a bug in the sense
// of "should be kept identical and drifted," since the contract's own
// transfer()/_calcFee() logic is never actually executed for a real user
// transfer (the RPC layer intercepts the selector before the EVM call would
// reach it — see evm_engine.go's checkPersistedCallAllowed). The Go-computed
// fee below is the one that actually applies to real value movements; the
// contract's 7% is inert, display-only bytecode a raw eth_call against the
// deployed contract would report but that never fires for any live
// transfer. Whatever documentation describes "the transfer fee" to users
// should describe THIS fee, not the contract's.
//   base = 0.1% of amount
//   Concentration surcharge if sender holds ≥1/5/10% of total supply
//   20% of fee → UBI pool, 80% burned (removed from supply)
// Returns (netAmountCredited, fromDemurrageLost, toDemurrageLost, err) — the
// two demurrage figures must be attached to the queued Transaction so
// secondary nodes replay the exact decay instead of recomputing it (see
// applyDemurrageLossLocked).
func (cs *ChainState) TransferWithV7Fee(from, to string, amount float64) (float64, float64, float64, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.transferWithV7FeeLocked(from, to, amount)
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
	err = cs.runAtomicWithOutbox([]string{from, to, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr}, false, func() (Transaction, error) {
		netAmount, fromLost, toLost, err = cs.transferWithV7FeeLocked(from, to, amount)
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
func (cs *ChainState) transferWithV7FeeLocked(from, to string, amount float64) (float64, float64, float64, error) {
	from = strings.ToLower(from)
	to = strings.ToLower(to)

	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, 0, 0, fmt.Errorf("invalid transfer amount: %v", amount)
	}

	// Page both parties in from the DB if they're cold (beyond maxInMemAccounts):
	// without this a returning sender hits "insufficient balance" despite a real
	// DB balance, and a cold RECIPIENT would be recreated blank below and have
	// its real balance overwritten on save. Matches transferLocked.
	cs.ensureAccountLoaded(from)
	cs.ensureAccountLoaded(to)
	fromAcc, ok := cs.accounts[from]
	if !ok {
		return 0, 0, 0, fmt.Errorf("insufficient balance")
	}
	fromLost, err := cs.settleDemurrageLocked(fromAcc)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("could not settle demurrage for sender: %w", err)
	}
	if fromAcc.Balance.Float() < amount {
		return 0, 0, 0, fmt.Errorf("insufficient balance")
	}

	// Compute total supply inline to avoid re-entering the mutex.
	humans := 0
	for _, acc := range cs.accounts {
		if acc.IsHuman {
			humans++
		}
	}
	totalSupply := float64(humans) * 1000.0
	fee := calcV7Fee(fromAcc.Balance.Float(), amount, totalSupply)
	// E1-FIX: In the Go-state ledger, AEQ cannot be burned (supply is tied
	// to humans * 1000). Redirect 100% of fee to UBI pool instead of the
	// V7-contract's 20%/80% split — this preserves the supply invariant
	// and ensures all fees benefit the community rather than disappearing.
	// E-FIX: compute net first, derive ubi as remainder - preserves supply invariant
	netToRecipient := round6(amount - fee)
	ubiContrib := amount - netToRecipient

	fromAcc.Balance = NewDecimal(round6(fromAcc.Balance.Float() - amount))
	touchActivity(fromAcc)
	if err := cs.saveAccountToDB(fromAcc); err != nil {
		return 0, 0, 0, fmt.Errorf("could not save sender account: %w", err)
	}

	if _, ok := cs.accounts[to]; !ok {
		cs.accounts[to] = &AccountState{Address: to}
	}
	toLost, err := cs.settleDemurrageLocked(cs.accounts[to])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("could not settle demurrage for recipient: %w", err)
	}
	cs.accounts[to].Balance = NewDecimal(round6(cs.accounts[to].Balance.Float() + netToRecipient))
	touchActivity(cs.accounts[to])
	if err := cs.enforceWealthCapLocked(cs.accounts[to]); err != nil {
		return 0, 0, 0, fmt.Errorf("could not enforce wealth cap for recipient: %w", err)
	}
	if err := cs.saveAccountToDB(cs.accounts[to]); err != nil {
		return 0, 0, 0, fmt.Errorf("could not save recipient account: %w", err)
	}

	if ubiContrib > 0 {
		if _, ok := cs.accounts[ubiPoolAddr]; !ok {
			cs.accounts[ubiPoolAddr] = &AccountState{Address: ubiPoolAddr}
		}
		cs.accounts[ubiPoolAddr].Balance = cs.accounts[ubiPoolAddr].Balance.Add(NewDecimal(ubiContrib))
		if err := cs.saveAccountToDB(cs.accounts[ubiPoolAddr]); err != nil {
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
	return cs.swapLocked(address, amountIn, true, minAmountOut)
}

// SwapTUSDForAEQ swaps `amountIn` tUSD from `address` into AEQ. Same
// constant-product pricing and fee handling as SwapAEQForTUSD, just with
// the two reserves' roles reversed.
func (cs *ChainState) SwapTUSDForAEQ(address string, amountIn, minAmountOut float64) (float64, float64, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.swapLocked(address, amountIn, false, minAmountOut)
}

// SwapAtomic behaves like SwapAEQForTUSD/SwapTUSDForAEQ, except the state
// mutation and the resulting outbox insert commit or roll back together as
// one DB transaction — see TransferAtomic's comment. pendingTxTemplate
// should have Type/Wallet/Amount set; AmountOut and FromDemurrageLost are
// filled in here from the swap's actual result.
func (cs *ChainState) SwapAtomic(address string, amountIn float64, aeqToTusd bool, minAmountOut float64, pendingTxTemplate Transaction) (amountOut, demurrageLost float64, err error) {
	address = strings.ToLower(address)
	err = cs.runAtomicWithOutbox([]string{address, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr}, false, func() (Transaction, error) {
		amountOut, demurrageLost, err = cs.swapLocked(address, amountIn, aeqToTusd, minAmountOut)
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
func (cs *ChainState) swapLocked(address string, amountIn float64, aeqToTusd bool, minAmountOut float64) (float64, float64, error) {
	// P2-7: reload pool from DB before swap to avoid stale-memory AMM invariant violation
	cs.reloadPoolFromDB()
	address = strings.ToLower(address)
	if amountIn <= 0 {
		return 0, 0, fmt.Errorf("amount must be positive")
	}
	if cs.pool == nil {
		return 0, 0, fmt.Errorf("liquidity pool not initialized")
	}

	cs.ensureAccountLoaded(address) // page in cold accounts so swaps work beyond the in-memory cap
	acc, ok := cs.accounts[address]
	if !ok {
		return 0, 0, fmt.Errorf("account not found")
	}
	lost, err := cs.settleDemurrageLocked(acc) // settle decay before checking/using the AEQ balance below
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
		cs.pool.ReserveAEQ = NewDecimal(round6(cs.pool.ReserveAEQ.Float() + amountInAfterFee))
		cs.pool.ReserveTUSD = NewDecimal(max(0.0, round6(cs.pool.ReserveTUSD.Float()-amountOut)))
		acc.Balance = NewDecimal(round6(acc.Balance.Float() - amountIn))
		acc.TUsdBalance = NewDecimal(round6(acc.TUsdBalance.Float() + amountOut))
	} else {
		amountOut = AMMSwapOut(cs.pool.ReserveTUSD, cs.pool.ReserveAEQ, NewDecimal(amountInAfterFee)).Float()
		if amountOut >= cs.pool.ReserveAEQ.Float() {
			return 0, 0, fmt.Errorf("swap too large for pool liquidity")
		}
		if minAmountOut > 0 && amountOut < minAmountOut {
			return 0, 0, fmt.Errorf("slippage: output %.6f AEQ below requested minimum %.6f", amountOut, minAmountOut)
		}
		cs.pool.ReserveTUSD = NewDecimal(round6(cs.pool.ReserveTUSD.Float() + amountInAfterFee))
		cs.pool.ReserveAEQ = NewDecimal(max(0.0, round6(cs.pool.ReserveAEQ.Float()-amountOut)))
		acc.TUsdBalance = NewDecimal(round6(acc.TUsdBalance.Float() - amountIn))
		acc.Balance = NewDecimal(round6(acc.Balance.Float() + amountOut))
	}
	touchActivity(acc) // swapping (either direction) counts as using the AEQ side
	if !aeqToTusd {
		if err := cs.enforceWealthCapLocked(acc); err != nil { // AEQ just arrived via this swap direction — check the cap
			return 0, 0, fmt.Errorf("could not enforce wealth cap: %w", err)
		}
	}

	if err := cs.saveAccountToDB(acc); err != nil {
		return 0, 0, fmt.Errorf("could not save account: %w", err)
	}
	if err := cs.savePoolToDB(); err != nil {
		return 0, 0, fmt.Errorf("could not save pool: %w", err)
	}
	if err := cs.distributeSwapFee(fee, aeqToTusd); err != nil {
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
	if !cs.useDB || cs.pool == nil {
		return nil
	}
	// FIX (atomic outbox): if runAtomicWithOutbox has an active transaction
	// open for the current operation, use it directly instead of starting a
	// SEPARATE one via cs.db.Begin() — that would open an independent
	// connection-level transaction with no relationship to cs.activeTx,
	// defeating the point (this write needs to commit/rollback together
	// with the rest of the operation, not on its own), and risking a
	// self-deadlock if both ever needed the same row lock concurrently.
	// The outer transaction already provides the serialization the
	// SELECT FOR UPDATE below exists for, so skip that dance entirely here.
	if cs.activeTx != nil {
		if _, err := cs.activeTx.Exec(`UPDATE liquidity_pool SET reserve_aeq = $1, reserve_tusd = $2, total_lp_shares = $3 WHERE id = 1`,
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
	cs.pool.ReserveTUSD = NewDecimal(round6(rT + feeTUsd))
	cs.pool.ReserveAEQ = NewDecimal(max(0.0, round6(rA-aeqOut)))
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
func (cs *ChainState) liquidateLPSharesForEscrowLocked(acc *AccountState, sharesToBurn float64) (burned, outAEQ, outTUSD float64, err error) {
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
	acc.LPShares = NewDecimal(max(0.0, round6(acc.LPShares.Float()-burned)))
	acc.Balance = NewDecimal(round6(acc.Balance.Float() + outAEQ))
	acc.TUsdBalance = NewDecimal(round6(acc.TUsdBalance.Float() + outTUSD))
	cs.pool.ReserveAEQ = NewDecimal(max(0.0, round6(cs.pool.ReserveAEQ.Float()-outAEQ)))
	cs.pool.ReserveTUSD = NewDecimal(max(0.0, round6(cs.pool.ReserveTUSD.Float()-outTUSD)))
	cs.pool.TotalLPShares = NewDecimal(max(0.0, round6(cs.pool.TotalLPShares.Float()-burned)))
	if err := cs.savePoolToDB(); err != nil {
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
func (cs *ChainState) convertTUsdForEscrowLocked(acc *AccountState, tusdAmount float64) (outAEQ float64, ok bool, err error) {
	if tusdAmount <= 0 || cs.pool == nil || cs.pool.ReserveAEQ.Float() <= 0 || cs.pool.ReserveTUSD.Float() <= 0 {
		return 0, false, nil
	}
	fee := tusdAmount * float64(swapFeeBps) / 10000.0
	amountInAfterFee := tusdAmount - fee
	outAEQ = AMMSwapOut(cs.pool.ReserveTUSD, cs.pool.ReserveAEQ, NewDecimal(amountInAfterFee)).Float()
	if outAEQ >= cs.pool.ReserveAEQ.Float() {
		return 0, false, nil
	}
	cs.pool.ReserveTUSD = NewDecimal(round6(cs.pool.ReserveTUSD.Float() + amountInAfterFee))
	cs.pool.ReserveAEQ = NewDecimal(max(0.0, round6(cs.pool.ReserveAEQ.Float()-outAEQ)))
	acc.TUsdBalance = NewDecimal(max(0.0, round6(acc.TUsdBalance.Float()-tusdAmount)))
	acc.Balance = NewDecimal(round6(acc.Balance.Float() + outAEQ))
	if err := cs.savePoolToDB(); err != nil {
		return outAEQ, true, fmt.Errorf("could not save pool after tUSD conversion: %w", err)
	}
	if err := cs.distributeSwapFee(fee, false); err != nil {
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
	if fee <= 0 {
		return nil
	}
	if !feeInAEQ {
		if aeqFee, ok := cs.convertTUsdFeeToAEQLocked(fee); ok {
			fee = aeqFee
			feeInAEQ = true
			if err := cs.savePoolToDB(); err != nil {
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
	for _, s := range shares {
		if _, ok := cs.accounts[s.addr]; !ok {
			cs.accounts[s.addr] = &AccountState{Address: s.addr}
		}
		if feeInAEQ {
			cs.accounts[s.addr].Balance = cs.accounts[s.addr].Balance.Add(NewDecimal(s.amount))
		} else {
			cs.accounts[s.addr].TUsdBalance = cs.accounts[s.addr].TUsdBalance.Add(NewDecimal(s.amount))
		}
		if err := cs.saveAccountToDB(cs.accounts[s.addr]); err != nil {
			return fmt.Errorf("distributeSwapFee: could not persist %s pool credit: %w", s.addr, err)
		}
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
		acc, ok := cs.accounts[addr]
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
		acc.TUsdBalance = NewDecimal(round6(acc.TUsdBalance.Float() - stranded))
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
// NOTE: this does not yet mint or track LP shares/tokens — it only moves
// balances into the pool. A depositor currently has no on-chain claim to
// withdraw their share back out. Tracking proportional LP ownership (so
// deposits are genuinely reversible) is a deliberate follow-up, not
// included in this first pass.
// Returns the AEQ amount demurrage-decayed off address before the deposit
// (0 if none) — callers must attach this to the queued Transaction so
// secondary nodes replay the exact decay via AddLiquidityDelta instead of
// recomputing it themselves.
func (cs *ChainState) AddLiquidity(address string, amountAEQ, amountTUSD float64) (float64, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.addLiquidityLocked(address, amountAEQ, amountTUSD)
}

// AddLiquidityAtomic behaves like AddLiquidity, except the state mutation
// and the resulting outbox insert commit or roll back together as one DB
// transaction — see TransferAtomic's comment. pendingTxTemplate should
// have Type/Wallet/Amount(AEQ)/AmountOut(tUSD) set; LPShares and
// FromDemurrageLost are filled in here from the operation's actual result.
func (cs *ChainState) AddLiquidityAtomic(address string, amountAEQ, amountTUSD float64, pendingTxTemplate Transaction) (demurrageLost float64, err error) {
	address = strings.ToLower(address)
	err = cs.runAtomicWithOutbox([]string{address, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr}, false, func() (Transaction, error) {
		sharesBefore := 0.0
		if acc, ok := cs.accounts[address]; ok {
			sharesBefore = acc.LPShares.Float()
		}
		demurrageLost, err = cs.addLiquidityLocked(address, amountAEQ, amountTUSD)
		if err != nil {
			return Transaction{}, err
		}
		sharesAfter := cs.accounts[address].LPShares.Float()
		pendingTxTemplate.LPShares = sharesAfter - sharesBefore
		pendingTxTemplate.FromDemurrageLost = demurrageLost
		return pendingTxTemplate, nil
	})
	return demurrageLost, err
}

// addLiquidityLocked is AddLiquidity's implementation; caller must already
// hold cs.mu — see transferLocked's comment for why this split exists.
func (cs *ChainState) addLiquidityLocked(address string, amountAEQ, amountTUSD float64) (float64, error) {
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

	cs.ensureAccountLoaded(address) // page in cold accounts so add-liquidity works beyond the in-memory cap
	acc, ok := cs.accounts[address]
	if !ok {
		return 0, fmt.Errorf("account not found")
	}
	lost, err := cs.settleDemurrageLocked(acc) // settle decay before checking/using the AEQ balance below
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

	acc.Balance = NewDecimal(round6(acc.Balance.Float() - amountAEQ))
	acc.TUsdBalance = NewDecimal(round6(acc.TUsdBalance.Float() - amountTUSD))
	acc.LPShares = NewDecimal(round6(acc.LPShares.Float() + mintedShares))
	touchActivity(acc) // depositing into the pool counts as using the AEQ
	cs.pool.ReserveAEQ = NewDecimal(round6(cs.pool.ReserveAEQ.Float() + amountAEQ))
	cs.pool.ReserveTUSD = NewDecimal(round6(cs.pool.ReserveTUSD.Float() + amountTUSD))
	cs.pool.TotalLPShares = NewDecimal(round6(cs.pool.TotalLPShares.Float() + mintedShares))

	if err := cs.saveAccountToDB(acc); err != nil {
		return 0, fmt.Errorf("could not save account: %w", err)
	}
	if err := cs.savePoolToDB(); err != nil {
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
	return cs.removeLiquidityLocked(address, sharesToBurn)
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
	err = cs.runAtomicWithOutbox([]string{address, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr}, false, func() (Transaction, error) {
		outAEQ, outTUSD, demurrageLost, err = cs.removeLiquidityLocked(address, sharesToBurn)
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
// exists.
func (cs *ChainState) removeLiquidityLocked(address string, sharesToBurn float64) (float64, float64, float64, error) {
	address = strings.ToLower(address)

	if sharesToBurn <= 0 {
		return 0, 0, 0, fmt.Errorf("shares must be positive")
	}
	if cs.pool == nil {
		return 0, 0, 0, fmt.Errorf("liquidity pool not initialized")
	}

	cs.ensureAccountLoaded(address) // page in cold accounts so remove-liquidity works beyond the in-memory cap
	acc, ok := cs.accounts[address]
	if !ok {
		return 0, 0, 0, fmt.Errorf("account not found")
	}
	// FIX: RemoveLiquidity previously never settled demurrage on the
	// withdrawing account, unlike AddLiquidity/Transfer/swapLocked — an idle
	// wealthy account could dodge decay indefinitely by periodically
	// removing/re-adding trivial liquidity amounts (touchActivity() below
	// resets the decay clock without the decay ever having been applied).
	lost, err := cs.settleDemurrageLocked(acc)
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
			acc.Balance = NewDecimal(round6(acc.Balance.Float() + outAEQ))
			acc.TUsdBalance = NewDecimal(round6(acc.TUsdBalance.Float() + outTUSD))
			touchActivity(acc)
			if err := cs.enforceWealthCapLocked(acc); err != nil {
				return 0, 0, 0, fmt.Errorf("could not enforce wealth cap: %w", err)
			}
			cs.pool.ReserveAEQ = NewDecimal(0)
			cs.pool.ReserveTUSD = NewDecimal(0)
			cs.pool.TotalLPShares = NewDecimal(0)
			if err := cs.saveAccountToDB(acc); err != nil {
				return 0, 0, 0, fmt.Errorf("could not save account: %w", err)
			}
			if err := cs.savePoolToDB(); err != nil {
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
		acc.Balance = NewDecimal(round6(acc.Balance.Float() + outAEQ17))
		acc.TUsdBalance = NewDecimal(round6(acc.TUsdBalance.Float() + outTUSD17))
		touchActivity(acc)
		if err := cs.enforceWealthCapLocked(acc); err != nil {
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
		cs.pool.TotalLPShares = NewDecimal(round6(cs.pool.TotalLPShares.Float() - sharesToBurn))
		if err := cs.saveAccountToDB(acc); err != nil {
			return 0, 0, 0, fmt.Errorf("could not save account: %w", err)
		}
		if err := cs.savePoolToDB(); err != nil {
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

	acc.LPShares = NewDecimal(round6(acc.LPShares.Float() - sharesToBurn))
	acc.Balance = NewDecimal(round6(acc.Balance.Float() + outAEQ))
	acc.TUsdBalance = NewDecimal(round6(acc.TUsdBalance.Float() + outTUSD))
	touchActivity(acc) // receiving AEQ back from the pool counts as using it
	if err := cs.enforceWealthCapLocked(acc); err != nil {
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
	cs.pool.TotalLPShares = NewDecimal(round6(cs.pool.TotalLPShares.Float() - sharesToBurn))

	if err := cs.saveAccountToDB(acc); err != nil {
		return 0, 0, 0, fmt.Errorf("could not save account: %w", err)
	}
	if err := cs.savePoolToDB(); err != nil {
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
	cs.mu.Lock()
	defer cs.mu.Unlock()
	address = strings.ToLower(address)
	cs.ensureAccountLoaded(address)
	var mine float64
	if acc, ok := cs.accounts[address]; ok {
		mine = acc.LPShares.Float()
	}
	total := 0.0
	if cs.pool != nil {
		total = cs.pool.TotalLPShares.Float()
	}
	return mine, total
}

func (cs *ChainState) TotalSupply() float64 {
	// Total supply is always exactly Humans × 1,000 AEQ by protocol design.
	// Each registered human receives exactly 1,000 AEQ upon registration —
	// no more, no less. Floating-point drift from swap fees and demurrage
	// calculations means the sum of all account balances + pool reserves
	// diverges slightly from this over time, so we compute it directly
	// from the human count instead of summing balances.
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	humans := 0
	for _, acc := range cs.accounts {
		if acc.IsHuman {
			humans++
		}
	}
	return float64(humans) * 1000.0
}

func (cs *ChainState) TotalHumans() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	count := 0
	for _, acc := range cs.accounts {
		if acc.IsHuman {
			count++
		}
	}
	return count
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
	result := make([]*AccountState, 0, len(cs.accounts))
	for _, acc := range cs.accounts {
		displayCopy := *acc
		displayCopy.Balance = effectiveBalance(acc)
		result = append(result, &displayCopy)
	}
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
	result := make([]*AccountState, 0, len(addrs))
	seen := make(map[string]bool, len(addrs))
	for _, addr := range addrs {
		addr = strings.ToLower(addr)
		if seen[addr] {
			continue
		}
		seen[addr] = true
		cs.ensureAccountLoaded(addr)
		acc, ok := cs.accounts[addr]
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
	return cs.claimTUsdFaucetLocked(address)
}

// ClaimTUsdFaucetAtomic behaves like ClaimTUsdFaucet, except the state
// mutation and the resulting outbox insert commit or roll back together as
// one DB transaction — see TransferAtomic's comment.
func (cs *ChainState) ClaimTUsdFaucetAtomic(address string, pendingTx Transaction) error {
	address = strings.ToLower(address)
	return cs.runAtomicWithOutbox([]string{address}, false, func() (Transaction, error) {
		if err := cs.claimTUsdFaucetLocked(address); err != nil {
			return Transaction{}, err
		}
		return pendingTx, nil
	})
}

// claimTUsdFaucetLocked is ClaimTUsdFaucet's implementation; caller must
// already hold cs.mu — see transferLocked's comment for why this split
// exists.
func (cs *ChainState) claimTUsdFaucetLocked(address string) error {
	address = strings.ToLower(address)
	cs.ensureAccountLoaded(address) // a cold registered human must still be recognised as human here

	acc, ok := cs.accounts[address]
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
	if err := cs.saveAccountToDB(acc); err != nil {
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
func accountLeaf(acc *AccountState) [32]byte {
	if !(acc.IsHuman || acc.Balance != 0 || acc.TUsdBalance != 0 || acc.LPShares != 0) {
		return [32]byte{}
	}
	s := fmt.Sprintf("acct:%s:%d:%d:%d:h=%v:fc=%v",
		strings.ToLower(acc.Address),
		acc.Balance.Micro(),
		acc.TUsdBalance.Micro(),
		acc.LPShares.Micro(),
		acc.IsHuman,
		acc.FaucetClaimed)
	return sha256.Sum256([]byte(s))
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
				if resident, ok := cs.accounts[lower]; ok {
					resident.leafHash = leaf
				}
			}
			rows.Close()
			scanned = true
		}
	}
	if !scanned {
		for _, a := range cs.accounts {
			leaf := accountLeaf(a)
			a.leafHash = leaf
			xorInto(&acc, leaf)
		}
	}
	cs.accountSetXOR = acc

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
	if n > 1 {
		gini = gini * float64(n) / float64(n-1)
	}
	if gini > 1.0 {
		gini = 1.0
	}
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
	for _, acc := range cs.accounts {
		// Every human counts, and each counts their FULL AEQ wealth (liquid +
		// LP). Filtering on liquid Balance>0 used to exclude anyone who parked
		// all their AEQ as liquidity, understating inequality and — worse —
		// producing a Gini that disagreed with the Lorenz curve, which included
		// them at 0.
		if acc.IsHuman {
			balances = append(balances, cs.humanAEQWealthLocked(acc))
		}
	}
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
	index := gini * 100.0
	return float64(int(index*10)) / 10.0
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
	snap := &blockRollbackSnapshot{}
	if full {
		// ubi_distribution touches every human's account (see ApplyUBIDelta) —
		// snapshot all of them rather than trying to enumerate which wallets
		// are currently human (that set is itself part of what we're
		// snapshotting, and could race against ApplyUBIDelta's own enumeration).
		existing := make(map[string]bool, len(cs.accounts))
		snap.accounts = make([]accountSnapshot, 0, len(cs.accounts)+len(addrs))
		for a, acc := range cs.accounts {
			existing[a] = true
			snap.accounts = append(snap.accounts, accountSnapshot{address: a, existed: true, state: *acc})
		}
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
			if acc, ok := cs.accounts[a]; ok {
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
			cs.accounts[s.address] = &restored
		} else {
			delete(cs.accounts, s.address)
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
			toSave = append(toSave, cs.accounts[s.address])
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
		for _, addr := range toDelete {
			if _, err := cs.db.Exec(`DELETE FROM chain_accounts WHERE lower(address) = $1`, addr); err != nil {
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
	return cs.applyTransferDeltaLocked(from, to, netAmount, fromLost, toLost)
}

// applyTransferDeltaLocked is ApplyTransferDelta's body, for callers that
// already hold cs.mu (audit recheck3, P0/P1 — replayTransactions holds
// cs.mu continuously across an entire block's snapshot/deltas/StateRoot
// check instead of releasing and reacquiring it once per TX; see
// replayTransactions' own comment for the isolation race this closes).
func (cs *ChainState) applyTransferDeltaLocked(from, to string, netAmount, fromLost, toLost float64) error {
	from = strings.ToLower(from)
	to = strings.ToLower(to)
	fromAcc, ok := cs.accounts[from]
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
	if err := cs.applyDemurrageLossLocked(fromAcc, fromLost); err != nil {
		return fmt.Errorf("transfer: could not settle sender %s demurrage: %w", from, err)
	}
	fromAcc.Balance = NewDecimal(round6(fromAcc.Balance.Float() - netAmount))
	// FIX (audit recheck2, P0 #3): this and every other saveAccountToDB/
	// savePoolToDB call in this function used to discard the returned error
	// — replayTransactions's caller checks THIS function's own return value
	// and rolls back on error, but with the error swallowed here it always
	// saw nil regardless of whether the DB write actually durably
	// committed. A block could be accepted (in-memory state mutated, block
	// inserted into the DAG) while the underlying account row never made it
	// to disk — exactly the kind of divergence that only surfaces after a
	// restart or bootstrap, when DB wins over memory.
	if err := cs.saveAccountToDB(fromAcc); err != nil {
		return fmt.Errorf("transfer: could not save sender %s: %w", from, err)
	}

	if _, ok := cs.accounts[to]; !ok {
		cs.accounts[to] = &AccountState{Address: to}
	}
	toAcc := cs.accounts[to]
	if err := cs.applyDemurrageLossLocked(toAcc, toLost); err != nil {
		return fmt.Errorf("transfer: could not settle recipient %s demurrage: %w", to, err)
	}
	toAcc.Balance = NewDecimal(round6(toAcc.Balance.Float() + netAmount))
	if err := cs.enforceWealthCapLocked(toAcc); err != nil {
		return fmt.Errorf("transfer: could not enforce wealth cap for recipient %s: %w", to, err)
	}
	if err := cs.saveAccountToDB(toAcc); err != nil {
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
	return cs.applySwapDeltaLocked(wallet, amountIn, amountOut, aeqToTusd, demurrageLost)
}

// applySwapDeltaLocked is ApplySwapDelta's body — see applyTransferDeltaLocked's comment.
func (cs *ChainState) applySwapDeltaLocked(wallet string, amountIn, amountOut float64, aeqToTusd bool, demurrageLost float64) error {
	wallet = strings.ToLower(wallet)
	cs.ensureAccountLoaded(wallet)
	acc, ok := cs.accounts[wallet]
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
	if err := cs.applyDemurrageLossLocked(acc, demurrageLost); err != nil {
		return fmt.Errorf("swap: could not settle %s demurrage: %w", wallet, err)
	}
	if aeqToTusd {
		acc.Balance = NewDecimal(round6(acc.Balance.Float() - amountIn))
		acc.TUsdBalance = NewDecimal(round6(acc.TUsdBalance.Float() + amountOut))
	} else {
		acc.TUsdBalance = NewDecimal(round6(acc.TUsdBalance.Float() - amountIn))
		acc.Balance = NewDecimal(round6(acc.Balance.Float() + amountOut))
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
	touchActivity(acc)
	if !aeqToTusd {
		if err := cs.enforceWealthCapLocked(acc); err != nil {
			return fmt.Errorf("swap: could not enforce wealth cap for %s: %w", wallet, err)
		}
	}
	// FIX (audit recheck2, P0 #3): see ApplyTransferDelta's comment — every
	// saveAccountToDB/savePoolToDB call in this function used to discard its
	// returned error.
	if err := cs.saveAccountToDB(acc); err != nil {
		return fmt.Errorf("swap: could not save account %s: %w", wallet, err)
	}

	// Update pool reserves to match what swapLocked() did on primary.
	// fee is swapFeeBps (0.1%); amountInAfterFee is what enters the pool.
	if cs.pool != nil {
		fee := amountIn * float64(swapFeeBps) / 10000.0
		amountInAfterFee := amountIn - fee
		if aeqToTusd {
			// Sender put in AEQ, got tUSD: reserveAEQ grows, reserveTUSD shrinks.
			cs.pool.ReserveAEQ = NewDecimal(round6(cs.pool.ReserveAEQ.Float() + amountInAfterFee))
			cs.pool.ReserveTUSD = NewDecimal(max(0.0, round6(cs.pool.ReserveTUSD.Float()-amountOut)))
		} else {
			// Sender put in tUSD, got AEQ: reserveTUSD grows, reserveAEQ shrinks.
			cs.pool.ReserveTUSD = NewDecimal(round6(cs.pool.ReserveTUSD.Float() + amountInAfterFee))
			cs.pool.ReserveAEQ = NewDecimal(max(0.0, round6(cs.pool.ReserveAEQ.Float()-amountOut)))
		}
		if err := cs.savePoolToDB(); err != nil {
			return fmt.Errorf("swap: could not save pool: %w", err)
		}
		// Distribute swap fee to the 4 tokenomics pools (40% validators /
		// 30% LP / 20% UBI / 10% treasury) — mirrors swapLocked() on primary.
		// Without this the fee-pool addresses stay at 0 on secondaries,
		// causing StateRoot divergence (pool addresses are included in the hash).
		if err := cs.distributeSwapFee(fee, aeqToTusd); err != nil {
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
	return cs.addLiquidityDeltaLocked(wallet, aeqAmount, tusdAmount, lpShares, demurrageLost)
}

// addLiquidityDeltaLocked is AddLiquidityDelta's body — see applyTransferDeltaLocked's comment.
func (cs *ChainState) addLiquidityDeltaLocked(wallet string, aeqAmount, tusdAmount, lpShares, demurrageLost float64) error {
	wallet = strings.ToLower(wallet)
	cs.ensureAccountLoaded(wallet)
	acc, ok := cs.accounts[wallet]
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
	if err := cs.applyDemurrageLossLocked(acc, demurrageLost); err != nil {
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

	acc.Balance = NewDecimal(round6(acc.Balance.Float() - aeqAmount))
	acc.TUsdBalance = NewDecimal(round6(acc.TUsdBalance.Float() - tusdAmount))
	acc.LPShares = NewDecimal(round6(acc.LPShares.Float() + mintedShares))
	// FIX (audit recheck2, P0 #3): see ApplyTransferDelta's comment.
	if cs.pool != nil {
		cs.pool.ReserveAEQ = NewDecimal(round6(cs.pool.ReserveAEQ.Float() + aeqAmount))
		cs.pool.ReserveTUSD = NewDecimal(round6(cs.pool.ReserveTUSD.Float() + tusdAmount))
		cs.pool.TotalLPShares = NewDecimal(round6(cs.pool.TotalLPShares.Float() + mintedShares))
		if err := cs.savePoolToDB(); err != nil {
			return fmt.Errorf("add_liquidity: could not save pool: %w", err)
		}
	}
	if err := cs.saveAccountToDB(acc); err != nil {
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
	return cs.removeLiquidityDeltaLocked(wallet, sharesToBurn, demurrageLost)
}

// removeLiquidityDeltaLocked is RemoveLiquidityDelta's body — see applyTransferDeltaLocked's comment.
func (cs *ChainState) removeLiquidityDeltaLocked(wallet string, sharesToBurn, demurrageLost float64) error {
	wallet = strings.ToLower(wallet)
	cs.ensureAccountLoaded(wallet)
	acc, ok := cs.accounts[wallet]
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
	if err := cs.applyDemurrageLossLocked(acc, demurrageLost); err != nil {
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

	acc.LPShares = NewDecimal(round6(acc.LPShares.Float() - sharesToBurn))
	acc.Balance = NewDecimal(round6(acc.Balance.Float() + outAEQ))
	acc.TUsdBalance = NewDecimal(round6(acc.TUsdBalance.Float() + outTUSD))
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
	touchActivity(acc)
	if err := cs.enforceWealthCapLocked(acc); err != nil {
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
	cs.pool.TotalLPShares = NewDecimal(round6(cs.pool.TotalLPShares.Float() - sharesToBurn))
	// FIX (audit recheck2, P0 #3): see ApplyTransferDelta's comment.
	if err := cs.savePoolToDB(); err != nil {
		return fmt.Errorf("remove_liquidity: could not save pool: %w", err)
	}
	if err := cs.saveAccountToDB(acc); err != nil {
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
	return cs.applyUBIDeltaLocked(amountPerHuman, ubiAt)
}

// applyUBIDeltaLocked is ApplyUBIDelta's body — see applyTransferDeltaLocked's comment.
func (cs *ChainState) applyUBIDeltaLocked(amountPerHuman float64, ubiAt int64) error {
	for addr, acc := range cs.accounts {
		if !acc.IsHuman {
			continue
		}
		acc.Balance = NewDecimal(round6(acc.Balance.Float() + amountPerHuman))
		touchActivity(acc)
		if err := cs.enforceWealthCapLocked(acc); err != nil {
			return fmt.Errorf("ubi (legacy flat): could not enforce wealth cap for %s: %w", addr, err)
		}
		if err := cs.saveAccountToDB(acc); err != nil {
			return fmt.Errorf("ubi (legacy flat): could not save account %s: %w", addr, err)
		}
	}
	// Zero the UBI pool on secondary (it was zeroed on primary after distribution)
	if ubiAcc, ok := cs.accounts[ubiPoolAddr]; ok {
		ubiAcc.Balance = NewDecimal(0)
		if err := cs.saveAccountToDB(ubiAcc); err != nil {
			return fmt.Errorf("ubi (legacy flat): could not save pool account: %w", err)
		}
	}
	// Write last_ubi_at to secondary's chain_config so StateRoot matches primary.
	if err := cs.setConfigValue("last_ubi_at", fmt.Sprintf("%d", ubiAt)); err != nil {
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
	return cs.applyUBIRewardDeltaLocked(wallet, amount, demurrageLost)
}

// applyUBIRewardDeltaLocked is ApplyUBIRewardDelta's body — see applyTransferDeltaLocked's comment.
func (cs *ChainState) applyUBIRewardDeltaLocked(wallet string, amount, demurrageLost float64) error {
	wallet = strings.ToLower(wallet)
	acc, ok := cs.accounts[wallet]
	if !ok {
		return fmt.Errorf("ubi reward: account not found: %s", wallet)
	}
	if err := cs.applyDemurrageLossLocked(acc, demurrageLost); err != nil {
		return fmt.Errorf("ubi reward: could not settle %s demurrage: %w", wallet, err)
	}
	acc.Balance = NewDecimal(round6(acc.Balance.Float() + amount))
	touchActivity(acc)
	if err := cs.enforceWealthCapLocked(acc); err != nil {
		return fmt.Errorf("ubi reward: could not enforce wealth cap for %s: %w", wallet, err)
	}
	// FIX (audit recheck2, P0 #3): see ApplyTransferDelta's comment.
	if err := cs.saveAccountToDB(acc); err != nil {
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
	return cs.applyUBIFinalizeDeltaLocked(ubiAt)
}

// applyUBIFinalizeDeltaLocked is ApplyUBIFinalizeDelta's body, callable from
// inside RunDailyDistributionAtomic where cs.mu is already held — see
// DistributeValidatorsPool's comment for the same pattern.
//
// FIX (audit recheck2, P0 #3): used to return nothing, discarding
// saveAccountToDB's error — see ApplyTransferDelta's comment.
func (cs *ChainState) applyUBIFinalizeDeltaLocked(ubiAt int64) error {
	if ubiAcc, ok := cs.accounts[ubiPoolAddr]; ok {
		ubiAcc.Balance = NewDecimal(0)
		if err := cs.saveAccountToDB(ubiAcc); err != nil {
			return fmt.Errorf("ubi finalize: could not save pool account: %w", err)
		}
	}
	if err := cs.setConfigValue("last_ubi_at", fmt.Sprintf("%d", ubiAt)); err != nil {
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
	return cs.applyValidatorRewardDeltaLocked(wallet, amount, demurrageLost)
}

// applyValidatorRewardDeltaLocked is ApplyValidatorRewardDelta's body — see applyTransferDeltaLocked's comment.
func (cs *ChainState) applyValidatorRewardDeltaLocked(wallet string, amount, demurrageLost float64) error {
	wallet = strings.ToLower(wallet)
	if _, ok := cs.accounts[wallet]; !ok {
		cs.accounts[wallet] = &AccountState{Address: wallet}
	}
	acc := cs.accounts[wallet]
	if err := cs.applyDemurrageLossLocked(acc, demurrageLost); err != nil {
		return fmt.Errorf("validator reward: could not settle %s demurrage: %w", wallet, err)
	}
	acc.Balance = NewDecimal(round6(acc.Balance.Float() + amount))
	touchActivity(acc)
	if err := cs.enforceWealthCapLocked(acc); err != nil {
		return fmt.Errorf("validator reward: could not enforce wealth cap for %s: %w", wallet, err)
	}
	// FIX (audit recheck2, P0 #3): see ApplyTransferDelta's comment.
	if err := cs.saveAccountToDB(acc); err != nil {
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
	return cs.applyValidatorPoolZeroDeltaLocked()
}

// applyValidatorPoolZeroDeltaLocked is ApplyValidatorPoolZeroDelta's body — see applyTransferDeltaLocked's comment.
func (cs *ChainState) applyValidatorPoolZeroDeltaLocked() error {
	if acc, ok := cs.accounts[validatorsPoolAddr]; ok {
		acc.Balance = NewDecimal(0)
		if err := cs.saveAccountToDB(acc); err != nil {
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
	return cs.applyLPRewardDeltaLocked(wallet, amount, demurrageLost)
}

// applyLPRewardDeltaLocked is ApplyLPRewardDelta's body — see applyTransferDeltaLocked's comment.
func (cs *ChainState) applyLPRewardDeltaLocked(wallet string, amount, demurrageLost float64) error {
	wallet = strings.ToLower(wallet)
	if _, ok := cs.accounts[wallet]; !ok {
		cs.accounts[wallet] = &AccountState{Address: wallet}
	}
	acc := cs.accounts[wallet]
	if err := cs.applyDemurrageLossLocked(acc, demurrageLost); err != nil {
		return fmt.Errorf("lp reward: could not settle %s demurrage: %w", wallet, err)
	}
	acc.Balance = NewDecimal(round6(acc.Balance.Float() + amount))
	touchActivity(acc)
	if err := cs.enforceWealthCapLocked(acc); err != nil {
		return fmt.Errorf("lp reward: could not enforce wealth cap for %s: %w", wallet, err)
	}
	// FIX (audit recheck2, P0 #3): see ApplyTransferDelta's comment.
	if err := cs.saveAccountToDB(acc); err != nil {
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
	return cs.applyLPPoolZeroDeltaLocked()
}

// applyLPPoolZeroDeltaLocked is ApplyLPPoolZeroDelta's body — see applyTransferDeltaLocked's comment.
func (cs *ChainState) applyLPPoolZeroDeltaLocked() error {
	if acc, ok := cs.accounts[lpPoolAddr]; ok {
		acc.Balance = NewDecimal(0)
		if err := cs.saveAccountToDB(acc); err != nil {
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
	return cs.applyEscrowMoveDeltaLocked(wallet, demurrageLost, lpSharesBurned, tusdConverted)
}

// applyEscrowMoveDeltaLocked is ApplyEscrowMoveDelta's body — see applyTransferDeltaLocked's comment.
func (cs *ChainState) applyEscrowMoveDeltaLocked(wallet string, demurrageLost, lpSharesBurned, tusdConverted float64) error {
	wallet = strings.ToLower(wallet)
	acc, ok := cs.accounts[wallet]
	if !ok {
		return fmt.Errorf("escrow move: account not found: %s", wallet)
	}
	if err := cs.applyDemurrageLossLocked(acc, demurrageLost); err != nil {
		return fmt.Errorf("escrow move: could not settle %s demurrage: %w", wallet, err)
	}

	// Replay the primary's LP liquidation / tUSD conversion via the exact
	// same shared math checkAndMoveToEscrowLocked used (guardian.go) — see
	// liquidateLPSharesForEscrowLocked/convertTUsdForEscrowLocked's own
	// comments for why a shared implementation, not a hand-written copy, is
	// what makes this structurally guaranteed to match the primary.
	if lpSharesBurned > 0 {
		burned, _, _, err := cs.liquidateLPSharesForEscrowLocked(acc, lpSharesBurned)
		if err != nil {
			return fmt.Errorf("escrow move: could not replay LP liquidation for %s: %w", wallet, err)
		}
		if burned <= 0 {
			return fmt.Errorf("escrow move: primary reported %.6f LP shares burned for %s but this node's pool has none — pool state has diverged", lpSharesBurned, wallet)
		}
	}

	if tusdConverted > 0 {
		_, ok, err := cs.convertTUsdForEscrowLocked(acc, tusdConverted)
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
	if err := cs.saveAccountToDB(acc); err != nil {
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
	return cs.applyEscrowReleaseDeltaLocked(amount)
}

// applyEscrowReleaseDeltaLocked is ApplyEscrowReleaseDelta's body — see applyTransferDeltaLocked's comment.
func (cs *ChainState) applyEscrowReleaseDeltaLocked(amount float64) error {
	if _, ok := cs.accounts[ubiPoolAddr]; !ok {
		cs.accounts[ubiPoolAddr] = &AccountState{Address: ubiPoolAddr}
	}
	cs.accounts[ubiPoolAddr].Balance = cs.accounts[ubiPoolAddr].Balance.Add(NewDecimal(round6(amount)))
	// FIX (audit recheck2, P0 #3): see ApplyTransferDelta's comment.
	if err := cs.saveAccountToDB(cs.accounts[ubiPoolAddr]); err != nil {
		return fmt.Errorf("escrow release: could not save pool account: %w", err)
	}
	return nil
}

// ApplyFaucetDelta credits faucetAmount tUSD to wallet and marks FaucetClaimed.
// Used by secondary nodes replaying faucet TXs from blocks.
func (cs *ChainState) ApplyFaucetDelta(wallet string, faucetAmount float64) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.applyFaucetDeltaLocked(wallet, faucetAmount)
}

// applyFaucetDeltaLocked is ApplyFaucetDelta's body — see applyTransferDeltaLocked's comment.
func (cs *ChainState) applyFaucetDeltaLocked(wallet string, faucetAmount float64) error {
	wallet = strings.ToLower(wallet)
	if _, ok := cs.accounts[wallet]; !ok {
		cs.accounts[wallet] = &AccountState{Address: wallet}
	}
	acc := cs.accounts[wallet]
	if acc.FaucetClaimed {
		return nil // idempotent: already applied
	}
	acc.FaucetClaimed = true
	acc.TUsdBalance = acc.TUsdBalance.Add(NewDecimal(faucetAmount))
	// FIX (audit recheck2, P0 #3): see ApplyTransferDelta's comment.
	if err := cs.saveAccountToDB(acc); err != nil {
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
