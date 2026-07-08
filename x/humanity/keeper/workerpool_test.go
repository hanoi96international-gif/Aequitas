package keeper

import (
	"sync"
	"testing"
)

// TestWorkerPool_RunsSubmittedJobs verifies jobs actually execute and their
// side effects (via closures) are visible after WaitGroup completion —
// ProduceBlock's exact usage pattern (result vars captured by closure,
// awaited via sync.WaitGroup, not the pool's own return value).
func TestWorkerPool_RunsSubmittedJobs(t *testing.T) {
	p := newWorkerPool(2)
	var wg sync.WaitGroup
	var a, b int
	wg.Add(2)
	p.submit(func() { defer wg.Done(); a = 1 })
	p.submit(func() { defer wg.Done(); b = 2 })
	wg.Wait()
	if a != 1 || b != 2 {
		t.Fatalf("expected both jobs to run (a=%d b=%d)", a, b)
	}
}

// TestWorkerPool_ReusedAcrossManyRounds verifies the pool keeps working
// correctly across repeated submit/wait cycles — ProduceBlock's real usage
// is exactly this, once per block for the node's whole lifetime.
func TestWorkerPool_ReusedAcrossManyRounds(t *testing.T) {
	p := newWorkerPool(2)
	for round := 0; round < 50; round++ {
		var wg sync.WaitGroup
		sum := 0
		var mu sync.Mutex
		wg.Add(2)
		p.submit(func() { defer wg.Done(); mu.Lock(); sum += 1; mu.Unlock() })
		p.submit(func() { defer wg.Done(); mu.Lock(); sum += 2; mu.Unlock() })
		wg.Wait()
		if sum != 3 {
			t.Fatalf("round %d: expected sum=3, got %d", round, sum)
		}
	}
}
