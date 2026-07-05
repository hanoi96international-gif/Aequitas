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
