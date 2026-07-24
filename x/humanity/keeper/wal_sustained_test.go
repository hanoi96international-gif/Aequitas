package keeper

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSustainedWAL_QueueConvergence is the longer-duration follow-up
// SCALING_ARCHITECTURE.md's walFlushInterval investigation (2026-07-23)
// called for: TestSimulateMaxTPS_WarmSteadyState runs only 8-20 seconds,
// which turned out to reward NOT reconciling to Postgres promptly (near-
// zero cs.mu.Lock() contention within that short window) over the WAL
// design's own actual goal (bounded, timely reconciliation) -- three
// tuning attempts each measured "worse" on that short benchmark while
// each was, more plausibly, actually fixing the real problem. This test
// instead runs each candidate config for a genuinely sustained duration
// and watches cs.WALFlushQueueDepth() over time: a config whose queue
// stabilizes (rises, then plateaus/oscillates around a steady depth) is
// keeping pace with real load; one that keeps climbing for the whole run
// is not, regardless of what its average TPS number looks like.
//
// Opt-in via its own env var (not AEQUITAS_TPS_BENCH) because this is
// deliberately much slower than the other benchmarks in this package —
// several configs x tens of seconds each.
//
// Run with:
//
//	sudo -u postgres psql -c "CREATE DATABASE aequitas_sustained;"
//	AEQUITAS_WAL_SUSTAINED_BENCH=1 DATABASE_URL="postgres://postgres@localhost/aequitas_sustained?sslmode=disable" \
//	  go test ./x/humanity/keeper/ -run TestSustainedWAL_QueueConvergence -v -timeout 600s
func TestSustainedWAL_QueueConvergence(t *testing.T) {
	if os.Getenv("AEQUITAS_WAL_SUSTAINED_BENCH") != "1" {
		t.Skip("opt-in only, slow: set AEQUITAS_WAL_SUSTAINED_BENCH=1 and DATABASE_URL to run")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL must point at a disposable local Postgres database")
	}

	type config struct {
		name     string
		interval time.Duration
		maxBatch int
	}
	configs := []config{
		{"original-500ms-500", 500 * time.Millisecond, 500},
		{"100ms-2000", 100 * time.Millisecond, 2000},
		{"20ms-1000", 20 * time.Millisecond, 1000},
	}

	const numPairs = 100
	const runDuration = 20 * time.Second
	const sampleEvery = 500 * time.Millisecond

	for _, cfg := range configs {
		t.Run(cfg.name, func(t *testing.T) {
			origInterval, origBatch := walFlushInterval, walFlushMaxBatch
			walFlushInterval, walFlushMaxBatch = cfg.interval, cfg.maxBatch
			defer func() { walFlushInterval, walFlushMaxBatch = origInterval, origBatch }()

			walPath := fmt.Sprintf("/tmp/aequitas-sustained-%s.wal", cfg.name)
			os.Remove(walPath)
			t.Setenv("AEQUITAS_WAL_ENABLED", "1")
			t.Setenv("AEQUITAS_WAL_PATH", walPath)

			state := NewChainState(fmt.Sprintf("unused-sustained-%s.json", cfg.name))
			if !state.useDB {
				t.Fatal("expected a live PostgreSQL connection (state.useDB == false) — check DATABASE_URL")
			}
			if state.wal == nil {
				t.Fatal("expected cs.wal to be set (AEQUITAS_WAL_ENABLED=1) — WAL fast path did not activate")
			}
			defer state.stopWALFlushWorkerForTest()
			defer os.Remove(walPath)

			senderAddrs := make([]string, numPairs)
			recipientAddrs := make([]string, numPairs)
			state.mu.Lock()
			for i := 0; i < numPairs; i++ {
				sAddr := fmt.Sprintf("0xc0000000000000000000000000000000%s%04x", cfg.name[:2], i)
				rAddr := fmt.Sprintf("0xd0000000000000000000000000000000%s%04x", cfg.name[:2], i)
				senderAddrs[i] = sAddr
				recipientAddrs[i] = rAddr
				state.accounts.Delete(sAddr)
				state.accounts.Delete(rAddr)
				sAcc := &AccountState{Address: sAddr, Balance: NewDecimal(1e12), LastActivityAt: time.Now().Unix()}
				rAcc := &AccountState{Address: rAddr, Balance: NewDecimal(1e12), LastActivityAt: time.Now().Unix()}
				if err := state.saveAccountToDB(sAcc); err != nil {
					state.mu.Unlock()
					t.Fatalf("seeding sender %s: %v", sAddr, err)
				}
				if err := state.saveAccountToDB(rAcc); err != nil {
					state.mu.Unlock()
					t.Fatalf("seeding recipient %s: %v", rAddr, err)
				}
			}
			state.mu.Unlock()

			// Untimed warm-up, same reasoning as TestSimulateMaxTPS_WarmSteadyState.
			for i := 0; i < numPairs; i++ {
				from, to := senderAddrs[i], recipientAddrs[i]
				tmpl := Transaction{Type: "transfer", Wallet: from, To: to, Amount: 0.0001, TxHash: fmt.Sprintf("0xwarmup-%s-%d", cfg.name, i)}
				if _, _, err := state.TransferAtomic(from, to, 0.0001, tmpl); err != nil {
					t.Fatalf("warm-up transfer %d failed: %v", i, err)
				}
			}
			ResetTransferFastPathStats()

			var succeeded, failed int64
			var txSeq int64
			stopCh := make(chan struct{})
			var wg sync.WaitGroup
			for i := 0; i < numPairs; i++ {
				wg.Add(1)
				go func(pairIdx int) {
					defer wg.Done()
					from, to := senderAddrs[pairIdx], recipientAddrs[pairIdx]
					for {
						select {
						case <-stopCh:
							return
						default:
						}
						seq := atomic.AddInt64(&txSeq, 1)
						txHash := fmt.Sprintf("0xsustained-%s-%d-%d", cfg.name, pairIdx, seq)
						tmpl := Transaction{Type: "transfer", Wallet: from, To: to, Amount: 0.0001, TxHash: txHash}
						if _, _, err := state.TransferAtomic(from, to, 0.0001, tmpl); err != nil {
							atomic.AddInt64(&failed, 1)
							continue
						}
						atomic.AddInt64(&succeeded, 1)
					}
				}(i)
			}

			// Sample queue depth on a fixed cadence for the whole run.
			var depths []int
			var depthsMu sync.Mutex
			sampleDone := make(chan struct{})
			go func() {
				defer close(sampleDone)
				ticker := time.NewTicker(sampleEvery)
				defer ticker.Stop()
				for {
					select {
					case <-stopCh:
						return
					case <-ticker.C:
						d := state.WALFlushQueueDepth()
						depthsMu.Lock()
						depths = append(depths, d)
						depthsMu.Unlock()
					}
				}
			}()

			start := time.Now()
			time.Sleep(runDuration)
			close(stopCh)
			wg.Wait()
			<-sampleDone
			elapsed := time.Since(start)

			tps := float64(atomic.LoadInt64(&succeeded)) / elapsed.Seconds()
			fastApplied, fastFallback := TransferFastPathStats()
			fastTotal := fastApplied + fastFallback
			var fastPathPct float64
			if fastTotal > 0 {
				fastPathPct = 100 * float64(fastApplied) / float64(fastTotal)
			}

			// Convergence signal: compare the queue depth in the first third of
			// samples against the last third. Rising a lot then flat = healthy
			// (absorbed the initial burst, keeping pace since). Still climbing at
			// a similar rate throughout = not keeping pace.
			depthsMu.Lock()
			n := len(depths)
			var firstThirdAvg, lastThirdAvg float64
			if n >= 6 {
				third := n / 3
				var sum1, sum2 int
				for i := 0; i < third; i++ {
					sum1 += depths[i]
				}
				for i := n - third; i < n; i++ {
					sum2 += depths[i]
				}
				firstThirdAvg = float64(sum1) / float64(third)
				lastThirdAvg = float64(sum2) / float64(third)
			}
			maxDepth := 0
			for _, d := range depths {
				if d > maxDepth {
					maxDepth = d
				}
			}
			finalDepth := 0
			if n > 0 {
				finalDepth = depths[n-1]
			}
			depthsMu.Unlock()

			t.Logf("=== Sustained WAL config %q: interval=%s maxBatch=%d, %d pairs, %s ===", cfg.name, cfg.interval, cfg.maxBatch, numPairs, runDuration)
			t.Logf("succeeded: %d  failed: %d  elapsed: %s  TPS: %.1f", succeeded, failed, elapsed, tps)
			t.Logf("fast path applied: %d  batcher fallback: %d  (%.1f%% fast path)", fastApplied, fastFallback, fastPathPct)
			t.Logf("queue depth samples: %d, max: %d, final: %d, first-third avg: %.0f, last-third avg: %.0f", n, maxDepth, finalDepth, firstThirdAvg, lastThirdAvg)
			if lastThirdAvg > firstThirdAvg*1.5 && lastThirdAvg > 100 {
				t.Logf("SIGNAL: queue depth still climbing in the last third of the run (%.0f -> %.0f) -- likely NOT keeping pace at this config", firstThirdAvg, lastThirdAvg)
			} else {
				t.Logf("SIGNAL: queue depth stabilized (first-third %.0f vs last-third %.0f) -- likely keeping pace at this config", firstThirdAvg, lastThirdAvg)
			}
			if failed > 0 {
				t.Errorf("%d transfers failed unexpectedly", failed)
			}
		})
	}
}
