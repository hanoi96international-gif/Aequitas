package keeper

import (
	"testing"
	"time"
)

// Regression tests for the beta-launch audit (2026-07-05) fix: /rpc had no
// rate limiting at all, unlike every other expensive/mutating endpoint in
// this codebase — every request (including a read-only eth_call) forces
// EVMEngine.newStateDB() to fully reload every account and contract storage
// slot from Postgres.

func TestRpcRateLimited_AllowsUpToMax(t *testing.T) {
	ip := "1.2.3.4:rate-test-allow"
	for i := 0; i < rpcRateLimitMax; i++ {
		if rpcRateLimited(ip) {
			t.Fatalf("request %d unexpectedly rate limited (max is %d)", i+1, rpcRateLimitMax)
		}
	}
}

func TestRpcRateLimited_BlocksOverMax(t *testing.T) {
	ip := "1.2.3.4:rate-test-block"
	for i := 0; i < rpcRateLimitMax; i++ {
		rpcRateLimited(ip)
	}
	if !rpcRateLimited(ip) {
		t.Fatal("expected request past the max to be rate limited")
	}
}

func TestRpcRateLimited_IndependentPerIP(t *testing.T) {
	ipA := "1.2.3.4:rate-test-a"
	ipB := "5.6.7.8:rate-test-b"
	for i := 0; i < rpcRateLimitMax; i++ {
		rpcRateLimited(ipA)
	}
	if rpcRateLimited(ipB) {
		t.Fatal("a different IP's own quota must not be affected by another IP's usage")
	}
}

func TestRpcRateLimited_ResetsAfterWindow(t *testing.T) {
	ip := "1.2.3.4:rate-test-reset"
	v, _ := rpcRateLimit.LoadOrStore(ip, &rpcRateLimitEntry{windowStart: time.Now()})
	entry := v.(*rpcRateLimitEntry)
	entry.mu.Lock()
	entry.count = rpcRateLimitMax
	entry.windowStart = time.Now().Add(-rpcRateLimitWindow - time.Second) // force the window to have elapsed
	entry.mu.Unlock()

	if rpcRateLimited(ip) {
		t.Fatal("expected the counter to reset once the window elapsed")
	}
}

// TestRpcRateLimited_BatchItemsShareOneIPBudget is a regression test for the
// P1 batch-bypass finding (security audit 2026-07-21): handleRPC used to
// call rpcRateLimited exactly once per HTTP request, before the body was
// even parsed, regardless of whether that request carried a single call or
// a maxBatchSize (100, a local const in handleRPC not reachable from here)
// JSON-RPC batch — so one HTTP request could drive up to maxBatchSize
// dispatches (each capable of a full EVMEngine.newStateDB() reload) past the
// documented per-IP budget. The fix moved the check inside handleRPC's
// batch-processing loop so every batch item consumes its own unit of the
// same per-IP budget, exactly like a standalone request already did.
//
// This exercises rpcRateLimited the same way handleRPC's batch loop now
// does — once per item — across what would be three separate HTTP requests
// each carrying a full 100-item batch, and confirms the per-IP budget is
// shared across all of them instead of resetting per "request" (the old,
// buggy call site would never have been able to observe this: it only ever
// spent 1 unit per HTTP request no matter the batch size).
func TestRpcRateLimited_BatchItemsShareOneIPBudget(t *testing.T) {
	ip := "1.2.3.4:rate-test-batch-budget"
	const simulatedBatchSize = 100 // mirrors handleRPC's own maxBatchSize

	countLimited := func() int {
		limited := 0
		for i := 0; i < simulatedBatchSize; i++ {
			if rpcRateLimited(ip) {
				limited++
			}
		}
		return limited
	}

	// First simulated batch (items 1-100 of this IP's window): well within
	// rpcRateLimitMax (200), nothing should be limited.
	if n := countLimited(); n != 0 {
		t.Fatalf("first 100-item batch: expected 0 items rate limited, got %d", n)
	}
	// Second simulated batch (items 101-200): still exactly at the budget,
	// nothing should be limited yet.
	if n := countLimited(); n != 0 {
		t.Fatalf("second 100-item batch: expected 0 items rate limited, got %d", n)
	}
	// Third simulated batch (items 201-300): the shared per-IP budget is
	// already exhausted from the first two batches, so every item in this
	// one must be rejected — proving the budget spans batches/requests
	// rather than resetting per HTTP call.
	if n := countLimited(); n != simulatedBatchSize {
		t.Fatalf("third 100-item batch: expected all %d items rate limited once the shared budget (%d) was exhausted, got %d",
			simulatedBatchSize, rpcRateLimitMax, n)
	}
}
