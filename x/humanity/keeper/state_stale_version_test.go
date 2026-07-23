package keeper

import (
	"testing"
	"time"
)

// TestSaveAccountToDB_FreshStructForExistingRowDoesNotDesyncVersion
// reproduces the root cause behind a real TPS-benchmark failure (2026-07-23
// investigation): a caller that constructs a fresh &AccountState{} (Version
// left at its Go zero value) for an address that ALREADY has a row in
// Postgres -- exactly what happens on every repeated invocation of
// tps_bench_test.go's seed loop against the same persistent bench DB, and
// more generally whenever any code seeds/re-seeds an address without first
// loading its existing row (ensureAccountLoadedCtx correctly avoids this for
// every real production code path, but nothing stopped a fresh struct from
// silently corrupting the version otherwise).
//
// Before the fix, saveAccountToDBInnerCtx's Version==0 branch blindly set
// acc.Version = 1 (via the shared acc.Version++, starting from zero)
// regardless of what version the row's INSERT ... ON CONFLICT DO UPDATE
// actually landed on -- permanently desyncing acc.Version from the DB's
// real value, so the very next optimistic-locked write for that address
// spuriously failed with "optimistic lock version conflict" even though
// nothing else had ever touched the row concurrently. Fixed by using
// RETURNING version so acc.Version always reflects the row's actual
// resulting value.
func TestSaveAccountToDB_FreshStructForExistingRowDoesNotDesyncVersion(t *testing.T) {
	truncateDistTestTables(t)
	cs := NewChainState("unused-stale-version-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection (cs.useDB == false) -- check DATABASE_URL")
	}
	addr := "0xstaleversionregressiontest0000000000001"

	// Simulate repeated prior seeding (e.g. several earlier process
	// invocations against the same persistent DB) -- several fresh structs
	// saved in a row, each bumping the DB's real version further.
	for i := 0; i < 5; i++ {
		acc := &AccountState{Address: addr, Balance: NewDecimal(1000), LastActivityAt: time.Now().Unix()}
		if err := cs.saveAccountToDB(acc); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	var dbVersionBefore int64
	if err := cs.db.QueryRow(`SELECT version FROM chain_accounts WHERE lower(address) = $1`, addr).Scan(&dbVersionBefore); err != nil {
		t.Fatalf("query db version: %v", err)
	}
	if dbVersionBefore != 5 {
		t.Fatalf("sanity check: expected DB version 5 after 5 seed writes, got %d", dbVersionBefore)
	}

	// One more fresh struct for the SAME, already-existing address --
	// the exact scenario that desynced acc.Version before the fix.
	final := &AccountState{Address: addr, Balance: NewDecimal(2000), LastActivityAt: time.Now().Unix()}
	if err := cs.saveAccountToDB(final); err != nil {
		t.Fatalf("final seed: %v", err)
	}

	var dbVersionAfter int64
	cs.db.QueryRow(`SELECT version FROM chain_accounts WHERE lower(address) = $1`, addr).Scan(&dbVersionAfter)
	if final.Version != dbVersionAfter {
		t.Fatalf("in-memory Version (%d) != actual DB version (%d) after saving a fresh struct for an existing row", final.Version, dbVersionAfter)
	}

	// The real-world consequence: a subsequent normal update must not
	// spuriously conflict.
	final.Balance = NewDecimal(3000)
	if err := cs.saveAccountToDB(final); err != nil {
		t.Errorf("subsequent normal update spuriously failed after a fresh-struct save: %v", err)
	}
}

// TestSaveAccountsToDBBatch_FreshStructForExistingRowDoesNotDesyncVersion is
// the saveAccountsToDBBatch counterpart of the test above -- that function's
// own doc comment documented the identical Version==0-blind-increment defect
// as intentional ("exactly matching saveAccountToDBInner's own Version==0
// branch"); fixed the same way (RETURNING version instead of a blind
// increment) in the same change.
func TestSaveAccountsToDBBatch_FreshStructForExistingRowDoesNotDesyncVersion(t *testing.T) {
	truncateDistTestTables(t)
	cs := NewChainState("unused-stale-version-batch-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection (cs.useDB == false) -- check DATABASE_URL")
	}
	addr := "0xstaleversionbatchregressiontest000001"

	for i := 0; i < 5; i++ {
		acc := &AccountState{Address: addr, Balance: NewDecimal(1000), LastActivityAt: time.Now().Unix()}
		if err := cs.saveAccountToDB(acc); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	final := &AccountState{Address: addr, Balance: NewDecimal(2000), LastActivityAt: time.Now().Unix()}
	if err := cs.saveAccountsToDBBatch([]*AccountState{final}); err != nil {
		t.Fatalf("batch seed: %v", err)
	}

	var dbVersionAfter int64
	cs.db.QueryRow(`SELECT version FROM chain_accounts WHERE lower(address) = $1`, addr).Scan(&dbVersionAfter)
	if final.Version != dbVersionAfter {
		t.Fatalf("in-memory Version (%d) != actual DB version (%d) after batch-saving a fresh struct for an existing row", final.Version, dbVersionAfter)
	}

	final.Balance = NewDecimal(3000)
	if err := cs.saveAccountsToDBBatch([]*AccountState{final}); err != nil {
		t.Errorf("subsequent normal batch update spuriously failed after a fresh-struct save: %v", err)
	}
}
