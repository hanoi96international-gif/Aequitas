package keeper

import (
	"fmt"
	"os"
	"testing"
)

// TestReplayTransactions_ParallelTransfers_RealDB is the real-Postgres
// counterpart to TestReplayTransactions_DisjointTransfers_OrderIndependent(_Fuzz)
// (replay_determinism_fuzz_test.go, in-memory only): it drives enough
// disjoint transfers through ONE block (comfortably more than runtime.NumCPU())
// that replayTransactions' new parallel phase (block.go, 2026-07-25) actually
// dispatches multiple goroutines concurrently calling applyTransferDeltaLocked
// against a REAL dag.state.activeTx (*sql.Tx) -- the one part of the
// concurrency-safety argument that can't be verified against an in-memory
// ChainState (cs.db == nil short-circuits every DB write). Confirms every
// account's balance lands correctly in Postgres itself, not just in
// cs.accounts.
//
// Opt-in (like every other *_bench_test.go/_db_test.go in this project): set
// AEQUITAS_TPS_BENCH=1 and DATABASE_URL to a disposable local Postgres.
func TestReplayTransactions_ParallelTransfers_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL must point at a disposable local Postgres database")
	}

	truncateDistTestTables(t)
	cs := NewChainState("unused-replay-parallel-realdb-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection (cs.useDB == false) — check DATABASE_URL")
	}

	const pairs = 24 // comfortably more than any realistic runtime.NumCPU(), so every worker gets real concurrent work
	type acctPair struct{ sender, recipient string }
	pairsList := make([]acctPair, pairs)
	txs := make([]Transaction, pairs)

	cs.mu.Lock()
	for i := 0; i < pairs; i++ {
		sender := distTestAddr(400 + 2*i)
		recipient := distTestAddr(401 + 2*i)
		pairsList[i] = acctPair{sender, recipient}
		senderAcc := &AccountState{Address: sender, Balance: NewDecimal(1000)}
		if err := cs.saveAccountToDB(senderAcc); err != nil {
			cs.mu.Unlock()
			t.Fatalf("seed sender %d: %v", i, err)
		}
		txs[i] = Transaction{Type: "transfer", Wallet: sender, To: recipient, Amount: 37.5}
	}
	cs.mu.Unlock()

	dag := newOrphanTestDAG()
	dag.state = cs
	dag.bootHeight = 0
	dag.replayedBlocks = make(map[string]bool)
	dag.replayFailures = make(map[string]replayFailureState)
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}

	block := &Block{
		Height:       1,
		Hash:         "replay-parallel-realdb-test-block-1",
		Transactions: txs,
	}

	if ok := dag.replayTransactions(block, false); !ok {
		t.Fatal("replayTransactions returned false (hard failure) for a well-formed disjoint-transfer batch")
	}

	for i, p := range pairsList {
		cs.mu.Lock()
		senderAfter, ok1 := cs.accounts.Get(p.sender)
		recipientAfter, ok2 := cs.accounts.Get(p.recipient)
		cs.mu.Unlock()
		if !ok1 || !ok2 {
			t.Fatalf("pair %d: want both accounts present after replay, got ok1=%v ok2=%v", i, ok1, ok2)
		}
		if senderAfter.Balance.Float() != 1000-37.5 {
			t.Errorf("pair %d: want sender debited to %v, got %v", i, 1000-37.5, senderAfter.Balance.Float())
		}
		if recipientAfter.Balance.Float() != 37.5 {
			t.Errorf("pair %d: want recipient credited 37.5, got %v", i, recipientAfter.Balance.Float())
		}

		// Confirm it actually landed in Postgres itself, not just cs.accounts
		// -- exactly the concurrent dbTx.Exec path this test exists to
		// exercise (see this test's own doc comment).
		var dbBalance float64
		if err := cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, p.recipient).Scan(&dbBalance); err != nil {
			t.Fatalf("pair %d: recipient row not found in Postgres after replay: %v", i, err)
		}
		if dbBalance != 37.5 {
			t.Errorf("pair %d: want recipient's real Postgres row credited 37.5, got %v", i, dbBalance)
		}
	}

	fmt.Printf("TestReplayTransactions_ParallelTransfers_RealDB: %d disjoint transfers replayed and verified in Postgres\n", pairs)
}
