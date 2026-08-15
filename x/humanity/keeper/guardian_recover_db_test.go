package keeper

import (
	"os"
	"testing"
)

// TestRecoverFromEscrow_RealDB is RecoverFromEscrow's only regression test
// (it had none before): the function requires a live cs.db (returns an
// error immediately otherwise), so it can only ever be exercised against a
// real Postgres connection — exactly the ctx-threaded cs.dbExecCtx(ctx)
// branch this migration touched (DELETE...RETURNING joining cs.activeTx).
//
// Opt-in (like every other *_bench_test.go/_db_test.go in this project):
// set AEQUITAS_TPS_BENCH=1 and DATABASE_URL to a disposable local Postgres.
func TestRecoverFromEscrow_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL must point at a disposable local Postgres database")
	}

	truncateDistTestTables(t)
	cs := NewChainState("unused-recover-escrow-db-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection (cs.useDB == false) — check DATABASE_URL")
	}

	wallet := distTestAddr(201)
	cs.mu.Lock()
	acc := &AccountState{Address: wallet, IsHuman: true}
	if err := cs.saveAccountToDB(acc); err != nil {
		cs.mu.Unlock()
		t.Fatalf("seed account: %v", err)
	}
	cs.mu.Unlock()

	if _, err := cs.db.Exec(
		`INSERT INTO escrow_accounts (wallet_address, amount, moved_at) VALUES ($1, $2, $3)`,
		wallet, 12.5, 0,
	); err != nil {
		t.Fatalf("seed escrow row: %v", err)
	}

	if err := cs.RecoverFromEscrow(wallet); err != nil {
		t.Fatalf("RecoverFromEscrow: %v", err)
	}

	cs.mu.Lock()
	recovered, ok := cs.accounts.Get(wallet)
	if !ok {
		cs.mu.Unlock()
		t.Fatal("want account present after recovery")
	}
	balance := recovered.Balance.Float()
	cs.mu.Unlock()
	if balance != 12.5 {
		t.Errorf("want balance credited 12.5 AEQ via real-DB escrow recovery, got %v", balance)
	}

	var rowCount int
	if err := cs.db.QueryRow(`SELECT count(*) FROM escrow_accounts WHERE wallet_address = $1`, wallet).Scan(&rowCount); err != nil {
		t.Fatalf("checking escrow row: %v", err)
	}
	if rowCount != 0 {
		t.Error("want the escrow row DELETEd via real-DB recovery")
	}

	// Recovering again must fail — the row is gone.
	if err := cs.RecoverFromEscrow(wallet); err == nil {
		t.Error("want a second recovery attempt to fail (no escrow left)")
	}
}

// TestConfirmAlive_ColdAccount_RealDB pins the cold-cache fix in ConfirmAlive.
// Same opt-in harness and reasoning as TestRecoverFromEscrow_RealDB above: the
// bug only exists when the account has a real row in Postgres but is not
// resident in memory, which cannot be reproduced without a live cs.db.
//
// The wallet is seeded, then explicitly evicted from cs.accounts to reproduce
// what loadFromDB's maxInMemAccounts preload does to a long-inactive account —
// the exact population this endpoint serves.
func TestConfirmAlive_ColdAccount_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL must point at a disposable local Postgres database")
	}

	truncateDistTestTables(t)
	cs := NewChainState("unused-confirm-alive-db-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection (cs.useDB == false) — check DATABASE_URL")
	}

	wallet := distTestAddr(202)
	guardian := distTestAddr(203)
	cs.mu.Lock()
	acc := &AccountState{Address: wallet, IsHuman: true, Balance: NewDecimal(500)}
	acc.LastActivityAt = 1 // long inactive, the state that makes this account cold
	if err := cs.saveAccountToDB(acc); err != nil {
		cs.mu.Unlock()
		t.Fatalf("seed account: %v", err)
	}
	cs.mu.Unlock()

	if _, err := cs.db.Exec(
		`INSERT INTO guardians (wallet_address, guardian_address) VALUES ($1, $2)
		 ON CONFLICT (wallet_address) DO UPDATE SET guardian_address = EXCLUDED.guardian_address`,
		wallet, guardian,
	); err != nil {
		t.Fatalf("seed guardian row: %v", err)
	}

	// Evict: the account now exists in Postgres and nowhere else, exactly as
	// after a restart whose preload did not reach this far down the list.
	cs.mu.Lock()
	cs.accounts.Delete(wallet)
	cs.mu.Unlock()

	if err := cs.ConfirmAlive(wallet, guardian); err != nil {
		t.Fatalf("ConfirmAlive on a cold (non-resident) account: %v\n"+
			"the guardian's proof-of-life must work for exactly the long-inactive "+
			"wallets that are least likely to be resident", err)
	}

	cs.mu.Lock()
	after, ok := cs.accounts.Get(wallet)
	cs.mu.Unlock()
	if !ok {
		t.Fatal("want the account warmed and resident after ConfirmAlive")
	}
	if after.LastActivityAt <= 1 {
		t.Errorf("want the inactivity clock reset, got LastActivityAt = %d", after.LastActivityAt)
	}
	if after.Balance.Float() != 500 {
		t.Errorf("want the real balance preserved (500 AEQ), got %v — a blind create would read 0",
			after.Balance.Float())
	}
}
