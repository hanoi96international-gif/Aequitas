package keeper

import (
	"fmt"
	"testing"
	"time"
)

// TestBoundedBreaker_TrackedKeyCountIsCapped is the unit-level guard for
// boundedBreaker's own MaxTracked bound — the shared mechanism
// TestProposerBreaker_TrackedProposerCountIsCapped already exercises through
// the proposer breaker, and TestBlockPushBreaker_TrackedIPCountIsCapped
// exercises through the blockPush breaker (which never had this protection
// at all before this consolidation — see breaker.go's own comment).
func TestBoundedBreaker_TrackedKeyCountIsCapped(t *testing.T) {
	b := newBoundedBreaker(5, time.Second, 1, 3)
	for i := 0; i < 3; i++ {
		b.RecordOutcome(fmt.Sprintf("key%d", i), false)
	}
	if len(b.failRun) != 3 {
		t.Fatalf("expected exactly 3 tracked keys, got %d", len(b.failRun))
	}
	b.RecordOutcome("oneTooMany", false)
	if _, admitted := b.failRun["oneTooMany"]; admitted || len(b.failRun) != 3 {
		t.Fatalf("a new key must not be admitted once at MaxTracked (tracked=%d)", len(b.failRun))
	}
	// An already-tracked key must still update normally past the cap.
	for i := 0; i < 5; i++ {
		b.RecordOutcome("key0", false)
	}
	if !b.ShouldDrop("key0") {
		t.Fatal("an already-tracked key must still be able to trip the breaker even while at MaxTracked")
	}
}

// TestBlockPushBreaker_TrackedIPCountIsCapped verifies the NEW protection
// this consolidation gives blockPushBreaker: before extracting boundedBreaker
// (performance audit 2026-07-06), blockPushFailRun/blockPushBreakerUntil had
// NO cap at all — an unbounded, unauthenticated-source-IP-keyed map, exactly
// the P2-c class of memory-exhaustion DoS the proposer breaker was already
// fixed against. This proves blockPushBreaker now has the identical bound.
func TestBlockPushBreaker_TrackedIPCountIsCapped(t *testing.T) {
	resetBlockPushBreaker()
	for i := 0; i < maxTrackedPushIPs; i++ {
		blockPushRecordOutcome(fmt.Sprintf("203.0.%d.%d", i/256, i%256), false)
	}
	blockPushBreaker.mu.Lock()
	tracked := len(blockPushBreaker.failRun)
	blockPushBreaker.mu.Unlock()
	if tracked != maxTrackedPushIPs {
		t.Fatalf("expected exactly %d tracked IPs, got %d", maxTrackedPushIPs, tracked)
	}

	blockPushRecordOutcome("9.9.9.9", false)
	blockPushBreaker.mu.Lock()
	_, admitted := blockPushBreaker.failRun["9.9.9.9"]
	trackedAfter := len(blockPushBreaker.failRun)
	blockPushBreaker.mu.Unlock()
	if admitted || trackedAfter != maxTrackedPushIPs {
		t.Fatalf("a new IP must not be admitted once at maxTrackedPushIPs (tracked=%d, admitted=%v)", trackedAfter, admitted)
	}
}

// TestBoundedBreaker_ClearWipesStateAndReportsOpenCount verifies Clear's
// contract: every tracked key is wiped, and it returns how many had an OPEN
// (still-cooling-down) breaker at the time — the count PerformResync's
// ClearProposerCircuitBreakers logs to the operator.
func TestBoundedBreaker_ClearWipesStateAndReportsOpenCount(t *testing.T) {
	b := newBoundedBreaker(2, time.Second, 1, 10)
	b.RecordOutcome("a", false)
	b.RecordOutcome("a", false) // trips -> open
	b.RecordOutcome("b", false) // one failure, not tripped

	n := b.Clear()
	if n != 1 {
		t.Fatalf("expected Clear to report 1 open breaker, got %d", n)
	}
	if len(b.failRun) != 0 || len(b.breakerUntil) != 0 {
		t.Fatal("Clear must wipe both failRun and breakerUntil entirely")
	}
	if b.ShouldDrop("a") {
		t.Fatal("a cleared key must not still be dropped")
	}
}

// TestBoundedBreaker_NilSafe verifies every method is safe to call on a nil
// *boundedBreaker — the fallback for any BlockDAG built as a raw struct
// literal without going through NewBlockchain (every lightweight test
// helper in this package does exactly that unless it explicitly needs
// breaker behavior, per newGhostdagTestDAG/newOrphanTestDAG's own comments).
func TestBoundedBreaker_NilSafe(t *testing.T) {
	var b *boundedBreaker
	if b.ShouldDrop("x") {
		t.Fatal("a nil breaker must never report ShouldDrop")
	}
	b.RecordOutcome("x", false) // must not panic
	if n := b.Clear(); n != 0 {
		t.Fatalf("a nil breaker's Clear must report 0, got %d", n)
	}
}
