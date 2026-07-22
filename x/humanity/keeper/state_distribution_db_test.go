package keeper

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// distTestAddr builds a well-formed 42-char (0x + 40 hex digits) test
// address — distributeValidatorsPoolLocked rejects anything shorter as
// malformed, which a hand-written "0xdist...N" string can silently violate.
func distTestAddr(n int) string {
	return fmt.Sprintf("0x%040x", n)
}

// truncateDistTestTables clears every table these real-DB distribution/
// escrow/recovery tests read or write a global (not test-address-scoped)
// view of — chain_accounts.is_human and chain_accounts.lp_shares in
// particular are read as "every human"/"every LP holder" by
// distributeUBIPoolLocked/distributeLPPoolLocked, so a human or LP-share
// account left behind by an unrelated test running earlier in the same
// `go test` invocation silently skews their math (confirmed: running these
// tests together without a truncate at all produced a real 3-way split
// where 2 was expected, from a single leftover human seeded by
// TestRecoverFromEscrow_RealDB).
//
// MUST run BEFORE NewChainState, not after: NewChainState's constructor
// calls loadFromDB and caches every existing row into cs.accounts
// in-memory, versions included. Truncating only the DB afterward leaves
// those stale, non-zero Version values cached in memory while the actual
// rows are gone — the next saveAccountToDB[Ctx] call for one of those
// addresses (e.g. a pool address touched by fee distribution) then loses
// the optimistic-lock race against a row that no longer exists, surfacing
// as a spurious "version conflict" error that has nothing to do with real
// concurrent writers. Confirmed: this exact failure mode reproduced when
// the truncate briefly lived after NewChainState during this test's own
// development.
func truncateDistTestTables(t *testing.T) {
	t.Helper()
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("truncateDistTestTables: open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`TRUNCATE chain_accounts, chain_config, nullifiers, liquidity_pool, escrow_accounts, registered_nodes CASCADE`); err != nil {
		t.Fatalf("truncateDistTestTables: %v", err)
	}
}

// TestRunDailyDistributionAtomic_RealDB is the DB-backed counterpart to
// TestRunDailyDistributionAtomic_CreditsUBIAndValidators — that test only
// ever exercises the cs.db==nil branches of distributeUBIPoolLocked/
// distributeValidatorsPoolLocked/distributeLPPoolLocked/
// checkAndMoveToEscrowLocked/releaseEscrowToUBILocked, which return before
// ever calling cs.dbExecCtx(ctx). The ctx-threading migration (see
// dbExecCtx's own comment) specifically touches the cs.db!=nil query
// branches these functions take when a real Postgres connection is set —
// exactly the branches this test forces by requiring DATABASE_URL, so a
// mistake in how RunDailyDistributionAtomic's closure builds and passes ctx
// down through all five sub-steps would show up here even though it's
// invisible to every no-DB unit test.
//
// Opt-in (like every other *_bench_test.go/_db_test.go in this project):
// set AEQUITAS_TPS_BENCH=1 and DATABASE_URL to a disposable local Postgres.
func TestRunDailyDistributionAtomic_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL must point at a disposable local Postgres database")
	}

	truncateDistTestTables(t)
	cs := NewChainState("unused-distribution-db-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection (cs.useDB == false) — check DATABASE_URL")
	}

	cs.mu.Lock()
	human1 := &AccountState{Address: distTestAddr(1), IsHuman: true, LastActivityAt: time.Now().Unix()}
	human2 := &AccountState{Address: distTestAddr(2), IsHuman: true, LastActivityAt: time.Now().Unix()}
	lpHolder := &AccountState{Address: distTestAddr(3), LPShares: NewDecimal(10), LastActivityAt: time.Now().Unix()}
	if err := cs.saveAccountToDB(human1); err != nil {
		cs.mu.Unlock()
		t.Fatalf("seed human1: %v", err)
	}
	if err := cs.saveAccountToDB(human2); err != nil {
		cs.mu.Unlock()
		t.Fatalf("seed human2: %v", err)
	}
	if err := cs.saveAccountToDB(lpHolder); err != nil {
		cs.mu.Unlock()
		t.Fatalf("seed lpHolder: %v", err)
	}
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(100000), ReserveTUSD: NewDecimal(100000), TotalLPShares: NewDecimal(10)}
	if err := cs.savePoolToDB(); err != nil {
		cs.mu.Unlock()
		t.Fatalf("seed pool: %v", err)
	}
	// Pool addresses don't pre-exist in a freshly truncated DB —
	// ensureAccountLoaded is a no-op for a row that isn't there yet, so
	// these must be created directly (same as human1/human2/lpHolder
	// above), not fetched-then-mutated.
	ubiAcc := &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(100)}
	if err := cs.saveAccountToDB(ubiAcc); err != nil {
		cs.mu.Unlock()
		t.Fatalf("fund UBI pool: %v", err)
	}
	cs.accounts.Set(ubiPoolAddr, ubiAcc)
	validatorsAcc := &AccountState{Address: validatorsPoolAddr, Balance: NewDecimal(50)}
	if err := cs.saveAccountToDB(validatorsAcc); err != nil {
		cs.mu.Unlock()
		t.Fatalf("fund validators pool: %v", err)
	}
	cs.accounts.Set(validatorsPoolAddr, validatorsAcc)
	lpAcc := &AccountState{Address: lpPoolAddr, Balance: NewDecimal(30)}
	if err := cs.saveAccountToDB(lpAcc); err != nil {
		cs.mu.Unlock()
		t.Fatalf("fund LP pool: %v", err)
	}
	cs.accounts.Set(lpPoolAddr, lpAcc)
	cs.mu.Unlock()

	cs.RegisterNode(distTestAddr(9))

	ubiAt := time.Now().Unix()
	if err := cs.RunDailyDistributionAtomic(ubiAt); err != nil {
		t.Fatalf("RunDailyDistributionAtomic: %v", err)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()
	h1, ok1 := cs.accounts.Get(human1.Address)
	h2, ok2 := cs.accounts.Get(human2.Address)
	if !ok1 || !ok2 {
		t.Fatalf("want both human accounts present after distribution, got ok1=%v ok2=%v", ok1, ok2)
	}
	if h1.Balance.Float() != 50 || h2.Balance.Float() != 50 {
		t.Errorf("want both humans credited 50 AEQ via real-DB UBI distribution, got %v and %v", h1.Balance.Float(), h2.Balance.Float())
	}
	if ubiPoolAcc, ok := cs.accounts.Get(ubiPoolAddr); !ok {
		t.Error("want UBI pool account still present after real-DB finalize")
	} else if ubiPoolAcc.Balance.Float() != 0 {
		t.Errorf("want UBI pool zeroed via real-DB finalize, got %v", ubiPoolAcc.Balance.Float())
	}
	if validator, ok := cs.accounts.Get(distTestAddr(9)); !ok {
		t.Error("want registered node account present after real-DB validator distribution")
	} else if validator.Balance.Float() != 50 {
		t.Errorf("want registered node credited 50 AEQ via real-DB validator distribution, got %v", validator.Balance.Float())
	}
	if lp, ok := cs.accounts.Get(lpHolder.Address); !ok {
		t.Error("want LP holder account present after real-DB LP distribution")
	} else if lp.Balance.Float() != 30 {
		t.Errorf("want LP holder credited 30 AEQ via real-DB LP distribution, got %v", lp.Balance.Float())
	}

	lastUBIAt := cs.getConfigValueDB("last_ubi_at")
	if lastUBIAt == "" {
		t.Error("want last_ubi_at persisted via real-DB UBI finalize")
	}
}

// TestRunDailyDistributionAtomic_EscrowRealDB is the real-DB counterpart
// covering checkAndMoveToEscrowLocked/releaseEscrowToUBILocked and, via the
// inactive wallet's LP shares + tUSD, liquidateLPSharesForEscrowLocked/
// convertTUsdForEscrowLocked — none of which TestRunDailyDistributionAtomic_
// RealDB exercises (no inactive wallet there). Same ctx-threading risk as
// that test's own comment: these functions' cs.db!=nil query branches are
// exactly what this migration touched.
func TestRunDailyDistributionAtomic_EscrowRealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL must point at a disposable local Postgres database")
	}

	truncateDistTestTables(t)
	cs := NewChainState("unused-distribution-escrow-db-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection (cs.useDB == false) — check DATABASE_URL")
	}

	longInactive := time.Now().Unix() - inactivityEscrowSeconds - 3600
	cs.mu.Lock()
	// Deliberately NO LPShares here: distributeLPPoolLocked's holder loop
	// (runs earlier in the same round) settles demurrage — and refreshes
	// LastActivityAt — for every account with lp_shares>0 in the DB, which
	// would un-inactivate this wallet before checkAndMoveToEscrowLocked
	// even runs. TUsdBalance alone still exercises the real-DB
	// convertTUsdForEscrowLocked path without that cross-step interaction.
	inactive := &AccountState{
		Address:        distTestAddr(101),
		IsHuman:        true,
		Balance:        NewDecimal(10),
		TUsdBalance:    NewDecimal(20),
		LastActivityAt: longInactive,
	}
	if err := cs.saveAccountToDB(inactive); err != nil {
		cs.mu.Unlock()
		t.Fatalf("seed inactive account: %v", err)
	}
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(100000), ReserveTUSD: NewDecimal(100000), TotalLPShares: NewDecimal(1)}
	if err := cs.savePoolToDB(); err != nil {
		cs.mu.Unlock()
		t.Fatalf("seed pool: %v", err)
	}
	cs.mu.Unlock()

	// Pre-seed an already-old escrow_accounts row for a second, unrelated
	// wallet so releaseEscrowToUBILocked's real-DB DELETE...RETURNING query
	// has something to actually release in this same round (the wallet
	// above only enters escrow just now, moved_at=now, far too young to be
	// released in the same pass).
	releaseWallet := distTestAddr(102)
	oldMovedAt := time.Now().Unix() - escrowToUBISeconds - 3600
	if _, err := cs.db.Exec(
		`INSERT INTO escrow_accounts (wallet_address, amount, moved_at) VALUES ($1, $2, $3)`,
		releaseWallet, 7.5, oldMovedAt,
	); err != nil {
		t.Fatalf("seed old escrow row: %v", err)
	}

	if err := cs.RunDailyDistributionAtomic(time.Now().Unix()); err != nil {
		t.Fatalf("RunDailyDistributionAtomic: %v", err)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	movedAcc, ok := cs.accounts.Get(inactive.Address)
	if !ok {
		t.Fatal("want inactive account still present after escrow move")
	}
	if movedAcc.Balance.Float() != 0 || movedAcc.TUsdBalance.Float() != 0 {
		t.Errorf("want inactive account fully zeroed (tUSD liquidated into escrow) via real-DB move, got balance=%v tusd=%v",
			movedAcc.Balance.Float(), movedAcc.TUsdBalance.Float())
	}
	// Not asserting the exact escrowed amount: it depends on the AMM
	// conversion rate at test time, already covered precisely by the no-DB
	// unit test TestConvertTUsdForEscrowLocked_ConvertsAtAMMRate. This
	// test's job is confirming the real-DB ctx-threaded path completes and
	// actually persists a nonzero escrow row at all.
	var escrowedAmount float64
	if err := cs.db.QueryRow(`SELECT amount FROM escrow_accounts WHERE wallet_address = $1`, inactive.Address).Scan(&escrowedAmount); err != nil {
		t.Errorf("want an escrow_accounts row for the moved wallet via real-DB INSERT, got: %v", err)
	} else if escrowedAmount <= 0 {
		t.Errorf("want a positive escrowed amount (Balance + tUSD-converted-to-AEQ), got %v", escrowedAmount)
	}

	var releasedRowCount int
	if err := cs.db.QueryRow(`SELECT count(*) FROM escrow_accounts WHERE wallet_address = $1`, releaseWallet).Scan(&releasedRowCount); err != nil {
		t.Fatalf("checking release row: %v", err)
	}
	if releasedRowCount != 0 {
		t.Error("want the old escrow row DELETEd via real-DB release")
	}
	if ubiAcc, ok := cs.accounts.Get(ubiPoolAddr); !ok || ubiAcc.Balance.Float() < 7.5 {
		t.Errorf("want UBI pool credited with the released 7.5 AEQ via real-DB release, got %v", ubiAcc)
	}
}
