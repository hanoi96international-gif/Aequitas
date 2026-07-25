package keeper

import (
	"fmt"
	"os"
	"runtime/pprof"
	"strconv"
	"testing"
	"time"
)

// replay_throughput_bench_test.go measures the thing Roadmap step 6 is about:
// how long it takes ONE node to replay a block full of transfers.
//
// This is the number that decides whether 50k TPS is reachable, and it had
// never been measured directly. Every existing benchmark here measures a
// neighbouring cost — TestBlockCostAtScale measures hashing and JSON of a
// block's wire form, TestSimulateMaxTPS_Ingestion measures the INGESTION
// path (TransferAtomic, the primary's side) — but the replay path is what
// every secondary runs for every block, and it is strictly serial today:
// one applyTransferDeltaLocked at a time, each with its own DB round trips.
//
// Opt-in: AEQUITAS_REPLAY_BENCH=1 plus DATABASE_URL pointing at a disposable
// local Postgres. Block size defaults to 2000 transfers and is overridable
// with AEQUITAS_REPLAY_BENCH_TXS.
func TestReplayThroughput_DisjointTransfers(t *testing.T) {
	if os.Getenv("AEQUITAS_REPLAY_BENCH") != "1" || os.Getenv("DATABASE_URL") == "" {
		t.Skip("opt-in only: set AEQUITAS_REPLAY_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	// Create the schema before truncating: truncateDistTestTables assumes the
	// tables already exist, which is true for a DB some other test already
	// touched but not for the fresh, disposable one this benchmark wants
	// (measuring against a table another test left thousands of rows in
	// would report that test's row count as this one's cost).
	if cs0 := NewChainState(""); cs0.db != nil {
		cs0.db.Close()
	}
	truncateDistTestTables(t)

	n := 2000
	if v := os.Getenv("AEQUITAS_REPLAY_BENCH_TXS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			n = parsed
		}
	}

	cs := NewChainState("")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection — check DATABASE_URL")
	}
	defer cs.db.Close()

	// Seed n disjoint sender/recipient pairs. Seeding itself goes through
	// the batch writer so it doesn't dominate the run.
	txs := make([]Transaction, n)
	seed := make([]*AccountState, 0, n)
	cs.mu.Lock()
	for i := 0; i < n; i++ {
		sender := distTestAddr(1_000_000 + 2*i)
		recipient := distTestAddr(1_000_001 + 2*i)
		acc := &AccountState{Address: sender, Balance: NewDecimal(1000)}
		cs.accounts.Set(sender, acc)
		seed = append(seed, acc)
		txs[i] = Transaction{
			Type: "transfer", Wallet: sender, To: recipient, Amount: 1,
			TxHash: fmt.Sprintf("0xreplaybench-%d", i),
		}
	}
	if err := cs.saveAccountsToDBBatch(seed); err != nil {
		cs.mu.Unlock()
		t.Fatalf("seed: %v", err)
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
		Hash:         "0xreplay-throughput-bench",
		Timestamp:    nowUnix(),
		Transactions: txs,
	}

	if path := os.Getenv("AEQUITAS_REPLAY_BENCH_CPUPROFILE"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("cpuprofile: %v", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			t.Fatalf("cpuprofile: %v", err)
		}
		defer pprof.StopCPUProfile()
	}

	rtBefore := DBRoundTrips()
	start := time.Now()
	ok := dag.replayTransactions(block, false)
	elapsed := time.Since(start)
	roundTrips := DBRoundTrips() - rtBefore
	if !ok {
		t.Fatal("replayTransactions rejected the benchmark block")
	}

	perTx := elapsed / time.Duration(n)
	tps := float64(n) / elapsed.Seconds()
	t.Logf("REPLAY THROUGHPUT: %d disjoint transfers in %v — %v/tx, %.0f tx/s replayed",
		n, elapsed.Round(time.Millisecond), perTx.Round(time.Microsecond), tps)
	t.Logf("  SQL statements: %d total, %.1f per transfer", roundTrips, float64(roundTrips)/float64(n))
	t.Logf("  (50,000 TPS needs a 1s block of 50,000 transfers replayed in under 1s, i.e. <20µs/tx)")

	// Correctness alongside the number: a benchmark that silently stopped
	// applying anything would otherwise look like a spectacular win.
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for i := 0; i < n; i += max(1, n/20) {
		sender := distTestAddr(1_000_000 + 2*i)
		recipient := distTestAddr(1_000_001 + 2*i)
		sAcc, okS := cs.accounts.Get(sender)
		rAcc, okR := cs.accounts.Get(recipient)
		if !okS || !okR {
			t.Fatalf("pair %d: accounts missing after replay (sender=%v recipient=%v)", i, okS, okR)
		}
		if got := sAcc.Balance.Float(); got != 999 {
			t.Fatalf("pair %d: sender balance %v, want 999", i, got)
		}
		if got := rAcc.Balance.Float(); got != 1 {
			t.Fatalf("pair %d: recipient balance %v, want 1", i, got)
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
