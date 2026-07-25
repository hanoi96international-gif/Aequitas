package keeper

import (
	"testing"
	"time"
)

// TestReplayBackoff_SkipDoesNotResetClock pins the fix for the 2026-07-25
// night incident: replayTransactions' backoff guard used to fall through the
// failure-bookkeeping defer, so every SKIPPED attempt was recorded as a fresh
// failure — count++ and lastTriedAt=now. With fetchMissingAncestors re-driving
// an awaited block every few seconds, the backoff window was pushed forward
// faster than it could ever elapse: after ONE transient DB error the block was
// never genuinely retried again, and the whole chain above it deferred forever
// (Contabo2, stuck at #1856714 and again at #1857181 the same evening).
//
// A skip must leave the failure record EXACTLY as it was, so the backoff can
// expire and the next arrival runs a real retry.
func TestReplayBackoff_SkipDoesNotResetClock(t *testing.T) {
	dag, _ := newDeterminismTestDAG()
	block := &Block{Hash: "0xlivelock", Height: 42}

	// One real failure happened 1s ago: backoff for count=1 is 5s, so the
	// guard must skip this attempt.
	firstTried := time.Now().Add(-1 * time.Second)
	dag.replayedMu.Lock()
	dag.replayFailures[block.Hash] = replayFailureState{count: 1, lastTriedAt: firstTried}
	dag.replayedMu.Unlock()

	if ok := dag.replayTransactions(block, false); ok {
		t.Fatalf("attempt within the backoff window must be skipped (return false)")
	}

	dag.replayedMu.Lock()
	got := dag.replayFailures[block.Hash]
	dag.replayedMu.Unlock()
	if got.count != 1 {
		t.Fatalf("a skipped attempt must not count as a failure: count = %d, want 1 — "+
			"counting skips is exactly the livelock that permanently stranded Contabo2", got.count)
	}
	if !got.lastTriedAt.Equal(firstTried) {
		t.Fatalf("a skipped attempt must not touch lastTriedAt (got %v, want %v) — "+
			"resetting it on every skip means the backoff window can never elapse", got.lastTriedAt, firstTried)
	}
}

// TestReplayBackoff_ExpiredWindowAllowsRealRetry complements the test above:
// once the backoff HAS elapsed, the attempt must actually proceed past the
// guard — and, as a real attempt, it must CHANGE the failure record (an
// empty block replays cleanly here, so success deletes the record). An
// untouched record would mean the guard swallowed the attempt forever.
func TestReplayBackoff_ExpiredWindowAllowsRealRetry(t *testing.T) {
	dag, _ := newDeterminismTestDAG()
	block := &Block{Hash: "0xretry", Height: 43}

	// Last real failure comfortably outside the 5s count=1 backoff.
	stale := time.Now().Add(-10 * time.Minute)
	dag.replayedMu.Lock()
	dag.replayFailures[block.Hash] = replayFailureState{count: 1, lastTriedAt: stale}
	dag.replayedMu.Unlock()

	dag.replayTransactions(block, false)

	dag.replayedMu.Lock()
	got, exists := dag.replayFailures[block.Hash]
	dag.replayedMu.Unlock()
	if exists && got.count == 1 && got.lastTriedAt.Equal(stale) {
		t.Fatalf("an attempt after the backoff elapsed must be a REAL attempt that updates " +
			"the failure record (success deletes it, failure re-stamps it) — an untouched " +
			"record means the guard skipped it forever")
	}
}
