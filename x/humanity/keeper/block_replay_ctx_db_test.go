package keeper

import (
	"os"
	"testing"
)

// TestReplayTransactions_TransferRealDB is replayTransactions' first
// real-DB test. Every existing replayTransactions test (via AddPeerBlock)
// constructs dag.state as a bare &ChainState{} (cs.db == nil), so they only
// ever exercise the no-DB branch of replayTransactions — dag.state.activeTx
// is never actually set to a real *sql.Tx there. The ctx-threading
// migration (see dbExecCtx's comment) specifically touches what happens
// when it IS set: every apply*DeltaLocked call in the replay switch now
// receives context.Background() and must fall back to dag.state.activeTx
// correctly to land inside the SAME real DB transaction the rest of the
// replay uses. This test is the one place that path gets exercised end to
// end, for the most common case (a "transfer" TX).
//
// Deliberately calls replayTransactions directly rather than going through
// the full AddPeerBlock (signature/authorization/GHOSTDAG) gauntlet — that
// machinery is orthogonal to what this test checks, and the DAG rejects an
// improperly-signed/authorized test block long before replayTransactions
// would ever run. block.StateRoot is left empty so the StateRoot
// comparison at the end of replayTransactions (a warning-only diagnostic
// for non-empty claims) is skipped entirely.
//
// Opt-in (like every other *_bench_test.go/_db_test.go in this project):
// set AEQUITAS_TPS_BENCH=1 and DATABASE_URL to a disposable local Postgres.
func TestReplayTransactions_TransferRealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL must point at a disposable local Postgres database")
	}

	truncateDistTestTables(t)
	cs := NewChainState("unused-replay-ctx-db-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection (cs.useDB == false) — check DATABASE_URL")
	}

	sender := distTestAddr(301)
	recipient := distTestAddr(302)
	cs.mu.Lock()
	senderAcc := &AccountState{Address: sender, Balance: NewDecimal(100)}
	if err := cs.saveAccountToDB(senderAcc); err != nil {
		cs.mu.Unlock()
		t.Fatalf("seed sender: %v", err)
	}
	cs.mu.Unlock()

	dag := newOrphanTestDAG()
	dag.state = cs
	dag.bootHeight = 0
	dag.replayedBlocks = make(map[string]bool)
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}

	block := &Block{
		Height: 1,
		Hash:   "replay-ctx-test-block-1",
		Transactions: []Transaction{
			{Type: "transfer", Wallet: sender, To: recipient, Amount: 10},
		},
		// StateRoot deliberately left empty — see this test's doc comment.
	}

	if ok := dag.replayTransactions(block, false); !ok {
		t.Fatal("replayTransactions returned false (hard failure) for a well-formed transfer")
	}

	cs.mu.Lock()
	senderAfter, ok1 := cs.accounts.Get(sender)
	recipientAfter, ok2 := cs.accounts.Get(recipient)
	cs.mu.Unlock()
	if !ok1 || !ok2 {
		t.Fatalf("want both accounts present after replay, got ok1=%v ok2=%v", ok1, ok2)
	}
	if senderAfter.Balance.Float() != 90 {
		t.Errorf("want sender debited to 90, got %v", senderAfter.Balance.Float())
	}
	if recipientAfter.Balance.Float() != 10 {
		t.Errorf("want recipient credited 10, got %v", recipientAfter.Balance.Float())
	}

	// Confirm it actually landed in Postgres, not just cs.accounts —
	// exactly the real-DB path this test exists to exercise (a nil-vs-set
	// dag.state.activeTx ctx-threading bug would still pass the in-memory
	// assertions above but leave the DB row stale or untouched).
	var dbBalance float64
	if err := cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, recipient).Scan(&dbBalance); err != nil {
		t.Fatalf("recipient row not found in Postgres after replay: %v", err)
	}
	if dbBalance != 10 {
		t.Errorf("want recipient's real Postgres row credited 10, got %v", dbBalance)
	}
}
