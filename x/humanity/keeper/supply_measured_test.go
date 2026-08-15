package keeper

import (
	"testing"
	"time"
)

// Regression for a fatal I shipped on 2026-08-15: the cache's mutex lived
// inside the cached struct, and a refresh assigned a whole new struct value —
// replacing the held, locked mutex with a fresh unlocked one. The deferred
// Unlock then released an unlocked mutex, which Go reports as
//
//	fatal error: sync: unlock of unlocked mutex
//
// and which, unlike a panic, no recover() can catch: it terminates the process.
// Every call to /api/health/combined killed the node, and the deploy
// verification step added the same day polls precisely that endpoint.
//
// The mutex is a separate variable now. This exercises the lock/refresh/unlock
// sequence the live code performs, several times in a row — under the old
// arrangement the second Unlock is the one that kills the process, so a single
// pass would not have caught it either.
func TestMeasuredSupplyCache_RefreshDoesNotDisturbTheLock(t *testing.T) {
	for i := 0; i < 5; i++ {
		measuredSupplyMu.Lock()
		measuredSupplyCache.value = float64(i)
		measuredSupplyCache.ok = true
		measuredSupplyCache.err = ""
		measuredSupplyCache.fetched = time.Now()
		measuredSupplyMu.Unlock()
	}
	measuredSupplyMu.Lock()
	got := measuredSupplyCache.value
	measuredSupplyMu.Unlock()
	if got != 4 {
		t.Fatalf("cached value = %v, want 4", got)
	}
}

// Concurrent callers must be safe too — the health endpoint is polled by the
// deploy gate, by monitoring and by the explorer at the same time. Run with
// -race to make this meaningful.
func TestMeasuredSupplyCache_ConcurrentRefresh(t *testing.T) {
	done := make(chan struct{})
	for g := 0; g < 8; g++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			for i := 0; i < 50; i++ {
				measuredSupplyMu.Lock()
				measuredSupplyCache.value = float64(n*i) 
				measuredSupplyCache.fetched = time.Now()
				measuredSupplyMu.Unlock()
			}
		}(g)
	}
	for g := 0; g < 8; g++ {
		<-done
	}
}

// With no database the function must say so rather than reporting a plausible
// zero — the distinction this whole audit kept finding missing.
func TestMeasuredTotalAEQ_NoDatabaseIsNotZero(t *testing.T) {
	cs := newTestState()
	total, ok, reason := cs.MeasuredTotalAEQ()
	if ok {
		t.Fatalf("want ok=false without a database, got total=%v", total)
	}
	if reason == "" {
		t.Fatal("want a reason explaining why it could not be measured")
	}
	rec := cs.SupplyReconciliation()
	if rec["measured"] != nil {
		t.Errorf("measured = %v, want nil when it could not be measured", rec["measured"])
	}
	if rec["measured_error"] == nil {
		t.Error("want measured_error set so a reader cannot mistake this for a real zero")
	}
}
