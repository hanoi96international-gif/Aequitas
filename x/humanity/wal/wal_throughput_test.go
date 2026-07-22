package wal

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

// TestWALThroughput measures this environment's real, achievable Append
// throughput under concurrent load -- SCALING_ARCHITECTURE.md names "WAL-
// Fsync-Durchsatz der tatsächlich genutzten Platte" as one of exactly three
// still-unmeasured limits standing between Phase 1-6 and a genuine 50,000
// TPS claim (the other two: Go GC pause behavior at high allocation rates,
// and shard count vs CPU cores -- neither addressed by this test). This is
// the closest thing to a direct, disk-real answer for that first unknown
// this project has produced so far.
//
// Opt-in (like every other *_bench_test.go in this project) -- prints real
// measurements, does not assert a pass/fail threshold; there is no
// predetermined "right" number, only what this specific disk can do.
func TestWALThroughput(t *testing.T) {
	if os.Getenv("AEQUITAS_WAL_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_WAL_BENCH=1 to run (SCALING_ARCHITECTURE.md Phase 7 fsync-throughput measurement)")
	}

	concurrencyLevels := []int{1, 10, 50, 200, 1000}
	for _, concurrency := range concurrencyLevels {
		t.Run(fmt.Sprintf("concurrency=%d", concurrency), func(t *testing.T) {
			path := tempWALPath(t)
			w, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer w.Close()

			const perGoroutine = 200
			payload := make([]byte, 128) // roughly transfer-Transaction-JSON-sized
			for i := range payload {
				payload[i] = byte(i)
			}

			var wg sync.WaitGroup
			start := time.Now()
			for g := 0; g < concurrency; g++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < perGoroutine; i++ {
						if _, err := w.Append(payload); err != nil {
							t.Errorf("Append: %v", err)
							return
						}
					}
				}()
			}
			wg.Wait()
			elapsed := time.Since(start)

			total := concurrency * perGoroutine
			tps := float64(total) / elapsed.Seconds()
			t.Logf("concurrency=%d: %d appends in %v = %.1f appends/sec", concurrency, total, elapsed, tps)
		})
	}
}
