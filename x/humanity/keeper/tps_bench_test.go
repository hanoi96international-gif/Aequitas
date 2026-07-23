package keeper

import (
	"fmt"
	"os"
	"runtime/pprof"
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
		// FIX (2026-07-23, TPS-benchmark investigation): this benchmark
		// reuses the same persistent bench DB across repeated manual runs
		// (deterministic addresses, never truncated in between), so a
		// SENDER address may already be warm in state.accounts (loaded by
		// NewChainState's own startup loadFromDB scan, correctly, from an
		// EARLIER run's final version) when this seed write runs. Evicting
		// first forces the account to start genuinely cold for THIS run, so
		// its first TransferAtomic call warms it via ensureAccountLoadedCtx
		// against the value THIS seed write just committed — otherwise the
		// stale warm entry's cached Version permanently disagrees with what
		// this seed write just bumped the DB to (this write's own version
		// increment never reaches an already-warm cs.accounts entry, since
		// saveAccountToDB here writes straight to Postgres, deliberately
		// bypassing cs.accounts — see saveAccountToDBInnerCtx's own doc
		// comment), so every later optimistic-locked write for that address
		// spuriously conflicts even though nothing else ever touched it
		// concurrently. Confirmed as the actual root cause via repeated
		// same-address benchmark runs; state.go's Version==0 handling itself
		// was ALSO found and fixed in the same investigation (a real,
		// separate defect — see saveAccountToDBInnerCtx's own FIX comment —
		// but insufficient alone to fix this benchmark's own stale-warm-
		// cache-entry issue).
		state.accounts.Delete(addr)
		acc := &AccountState{Address: addr, Balance: NewDecimal(1e9), LastActivityAt: time.Now().Unix()}
		if err := state.saveAccountToDB(acc); err != nil {
			state.mu.Unlock()
			t.Fatalf("seeding sender %s: %v", addr, err)
		}
	}
	// The shared recipient is never explicitly seeded (see below — it's
	// created on demand by the first transfer that touches it), but a prior
	// run against this same persistent DB may have already warmed it with a
	// real balance/version — evict for the same reason as the senders above,
	// so this run starts from a clean, consistent baseline regardless of
	// what earlier runs left behind.
	state.accounts.Delete(recipient)
	state.mu.Unlock()

	if path := os.Getenv("AEQUITAS_TPS_CPUPROFILE"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("could not create cpu profile: %v", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			t.Fatalf("could not start cpu profile: %v", err)
		}
		defer pprof.StopCPUProfile()
	}

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

// TestSimulateMaxTPS_IngestionDisjointRecipients is
// TestSimulateMaxTPS_Ingestion's counterpart for the OTHER realistic
// shape of concurrent load: every sender has its own distinct recipient
// (ring topology, i -> i+1), so no two concurrent transfers ever touch the
// same account. This is deliberately the best case for
// SCALING_ARCHITECTURE.md Phase 5's shard-locked transferConcurrent fast
// path (see TransferAtomic's own comment) — every pair of concurrent
// transfers lands on different shards essentially all the time (64 shards,
// 100 addresses), so they never contend on the same shard lock, unlike
// TestSimulateMaxTPS_Ingestion's single shared recipient (every transfer
// contends on ONE shard there, by construction).
//
// Compare this test's TPS against TestSimulateMaxTPS_Ingestion's on the
// same code: the gap between them IS the honest characterization of when
// shard-locking helps (disjoint, P2P-style transfers) versus when it can't
// (many senders converging on one hot address, e.g. a faucet, merchant, or
// staking pool) — that shared-recipient case funnels every transfer's
// whole DB round trip through one shard mutex with its own per-transfer
// Begin/Commit, forgoing the batcher's cross-transfer commit amortization
// entirely, which is why it measured SLOWER with the fast path wired in
// than the batcher alone (see this file's git history for both numbers).
func TestSimulateMaxTPS_IngestionDisjointRecipients(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL must point at a disposable local Postgres database")
	}

	state := NewChainState("unused-tps-bench-ingestion-disjoint.json")
	if !state.useDB {
		t.Fatal("expected a live PostgreSQL connection (state.useDB == false) — check DATABASE_URL")
	}

	const numSenders = 100
	const txsPerSender = 100 // 10,000 transfers total

	addrs := make([]string, numSenders)
	state.mu.Lock()
	for i := 0; i < numSenders; i++ {
		addr := fmt.Sprintf("0xdee0000000000000000000000000000000%04x", i)
		addrs[i] = addr
		// See TestSimulateMaxTPS_Ingestion's identical seeding fix (same
		// FIX comment there, 2026-07-23) for why evicting first is required
		// when this benchmark reuses a persistent, cross-run bench DB.
		state.accounts.Delete(addr)
		acc := &AccountState{Address: addr, Balance: NewDecimal(1e9), LastActivityAt: time.Now().Unix()}
		if err := state.saveAccountToDB(acc); err != nil {
			state.mu.Unlock()
			t.Fatalf("seeding account %s: %v", addr, err)
		}
	}
	state.mu.Unlock()

	if path := os.Getenv("AEQUITAS_TPS_CPUPROFILE"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("could not create cpu profile: %v", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			t.Fatalf("could not start cpu profile: %v", err)
		}
		defer pprof.StopCPUProfile()
	}

	var succeeded, failed int64
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < numSenders; i++ {
		wg.Add(1)
		go func(senderIdx int) {
			defer wg.Done()
			from := addrs[senderIdx]
			to := addrs[(senderIdx+1)%numSenders]
			for j := 0; j < txsPerSender; j++ {
				txHash := fmt.Sprintf("0xbench-disjoint-%d-%d", senderIdx, j)
				tmpl := Transaction{Type: "transfer", Wallet: from, To: to, Amount: 0.0001, TxHash: txHash}
				if _, _, err := state.TransferAtomic(from, to, 0.0001, tmpl); err != nil {
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

	t.Logf("=== Max TPS simulation: ingestion, disjoint recipients (TransferAtomic, %d concurrent senders, ring topology) ===", numSenders)
	t.Logf("attempted: %d  succeeded: %d  failed: %d", total, succeeded, failed)
	t.Logf("wall clock: %s", elapsed)
	t.Logf("sustained TPS (single local node, real Postgres, no network latency): %.1f", tps)
	if failed > 0 {
		t.Errorf("%d/%d transfers failed unexpectedly (pre-funded balances should never run out)", failed, total)
	}
}
