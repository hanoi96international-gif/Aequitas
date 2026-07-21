package keeper

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSimulateMaxTPS_Ingestion is an opt-in local throughput simulation
// (skipped unless AEQUITAS_TPS_BENCH=1 is set) — NOT part of the normal
// test suite, and NOT a measurement of the live production network (this
// sandbox has no network route to it). It drives the REAL code path every
// incoming transfer actually takes in production: evm_rpc.go's
// eth_sendRawTransaction handler applies a native AEQ transfer immediately,
// synchronously, via ChainState.TransferAtomic — NOT via the
// pendingTxs/ProduceBlock/replayTransactions path, which exists for
// RELAYING an already-applied transaction to other nodes, not for applying
// it locally. TransferAtomic runs entirely under cs.mu.Lock() (a single
// process-wide exclusive lock) plus one real Postgres transaction
// (runAtomicWithOutbox's cs.db.Begin()/commit), so every transfer this
// node accepts is fully serialized against every other one — this
// benchmark measures exactly that ceiling: how many individual transfer
// requests one validator's own ChainState can absorb per second against a
// local, disposable Postgres database.
//
// Run with:
//
//	sudo -u postgres psql -c "CREATE DATABASE aequitas_bench;"
//	AEQUITAS_TPS_BENCH=1 DATABASE_URL="postgres://postgres@localhost/aequitas_bench?sslmode=disable" \
//	  go test ./x/humanity/keeper/ -run TestSimulateMaxTPS_Ingestion -v -timeout 300s
func TestSimulateMaxTPS_Ingestion(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL must point at a disposable local Postgres database")
	}

	state := NewChainState("unused-tps-bench-ingestion.json")
	if !state.useDB {
		t.Fatal("expected a live PostgreSQL connection (state.useDB == false) — check DATABASE_URL")
	}

	const numSenders = 100     // simulates 100 distinct wallets submitting concurrently
	const txsPerSender = 100   // 10,000 transfers total
	const recipient = "0x00000000000000000000000000000000000bee"

	senderAddrs := make([]string, numSenders)
	state.mu.Lock()
	for i := 0; i < numSenders; i++ {
		addr := fmt.Sprintf("0xbe0000000000000000000000000000000%05x", i)
		senderAddrs[i] = addr
		acc := &AccountState{Address: addr, Balance: NewDecimal(1e9), LastActivityAt: time.Now().Unix()}
		if err := state.saveAccountToDB(acc); err != nil {
			state.mu.Unlock()
			t.Fatalf("seeding sender %s: %v", addr, err)
		}
	}
	state.mu.Unlock()

	var succeeded, failed int64
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < numSenders; i++ {
		wg.Add(1)
		go func(senderIdx int) {
			defer wg.Done()
			addr := senderAddrs[senderIdx]
			for j := 0; j < txsPerSender; j++ {
				txHash := fmt.Sprintf("0xbench-%d-%d", senderIdx, j)
				tmpl := Transaction{Type: "transfer", Wallet: addr, To: recipient, Amount: 0.0001, TxHash: txHash}
				if _, _, err := state.TransferAtomic(addr, recipient, 0.0001, tmpl); err != nil {
					atomic.AddInt64(&failed, 1)
					if failed <= 5 {
						t.Logf("transfer %d/%d for sender %d failed: %v", j, txsPerSender, senderIdx, err)
					}
					continue
				}
				atomic.AddInt64(&succeeded, 1)
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	total := succeeded + failed
	tps := float64(succeeded) / elapsed.Seconds()

	t.Logf("=== Max TPS simulation: ingestion (TransferAtomic, %d concurrent senders) ===", numSenders)
	t.Logf("attempted: %d  succeeded: %d  failed: %d", total, succeeded, failed)
	t.Logf("wall clock: %s", elapsed)
	t.Logf("sustained TPS (single local node, real Postgres, no network latency): %.1f", tps)
	if failed > 0 {
		t.Errorf("%d/%d transfers failed unexpectedly (pre-funded balances should never run out)", failed, total)
	}
}
