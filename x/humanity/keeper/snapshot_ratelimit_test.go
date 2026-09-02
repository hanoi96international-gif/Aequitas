package keeper

import (
	"errors"
	"testing"
	"time"
)

// A 429 from the peer is the one snapshot failure worth waiting out, and it was
// being treated as terminal.
//
// WHAT THAT COST, measured live on 2026-08-22: the primary sat 1,866 blocks
// behind for 33 minutes with all four self-heal monitors armed and firing
// correctly. Every recovery attempt died on the status check, because
// handleSnapshot throttles its public tier to one request per 30 seconds per IP
// and a recovering validator is polling that same peer for blocks continuously.
// The in-process heal fell back to a restart, and the restart hit the same
// window again at boot.
//
// The distinction pinned here is the whole fix: 429 means "ask again", every
// other failure means "this peer is wrong", and a recovering node must not sit
// in a retry loop against a peer that is wrong.
//
// These drive retryWhileRateLimited rather than the network path: the real
// fetcher refuses loopback addresses (SSRF guard), so an httptest server cannot
// reach it, and the decision is the part that was broken anyway.

func TestRateLimitIsRetriedUntilThePeerRelents(t *testing.T) {
	calls := 0
	_, err := retryWhileRateLimited(4, time.Millisecond, func() (*StateSnapshot, error) {
		calls++
		if calls < 3 {
			return nil, errSnapshotRateLimited
		}
		return &StateSnapshot{Height: 42}, nil
	})
	if err != nil {
		t.Fatalf("a peer that relented on the third attempt still failed: %v", err)
	}
	if calls != 3 {
		t.Errorf("fetched %d times, want 3 — a 429 must be followed by another attempt", calls)
	}
}

// The failure that matters most: everything that is NOT a rate limit has to
// fail on the first attempt.
func TestNonRateLimitFailuresAreNotRetried(t *testing.T) {
	sentinel := errors.New("snapshot signed by 0xbad, expected 0xgood")
	calls := 0
	_, err := retryWhileRateLimited(4, time.Millisecond, func() (*StateSnapshot, error) {
		calls++
		return nil, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("got %v, want the original error unchanged", err)
	}
	if calls != 1 {
		t.Errorf("fetched %d times for a bad signature.\n"+
			"  A failure that cannot improve by asking again must be reported immediately — "+
			"retrying it leaves a node sitting still while it could be naming the real problem.", calls)
	}
}

// A peer that never relents must give up, and say that is what happened.
func TestPersistentRateLimitGivesUpAndSaysSo(t *testing.T) {
	calls := 0
	_, err := retryWhileRateLimited(3, time.Millisecond, func() (*StateSnapshot, error) {
		calls++
		return nil, errSnapshotRateLimited
	})
	if err == nil {
		t.Fatal("a peer that always rate limits produced no error")
	}
	if calls != 3 {
		t.Errorf("fetched %d times, want exactly the 3 attempts allowed", calls)
	}
	if !errors.Is(err, errSnapshotRateLimited) {
		t.Errorf("the final error %v no longer identifies the cause; an operator needs to see "+
			"that the peer was throttling, not a generic failure", err)
	}
}

// A success on the first attempt must not wait at all -- the common case.
func TestFirstAttemptSuccessDoesNotWait(t *testing.T) {
	start := time.Now()
	snap, err := retryWhileRateLimited(4, 10*time.Second, func() (*StateSnapshot, error) {
		return &StateSnapshot{Height: 7}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Height != 7 {
		t.Errorf("got height %d, want 7", snap.Height)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("took %v for a first-attempt success; the wait must only happen between "+
			"attempts, never after the last one or before the first", elapsed)
	}
}

// The wait is sized against the peer's own window. If handleSnapshot's throttle
// changes, this is the line that has to change with it.
func TestRetryWaitClearsThePeersThrottleWindow(t *testing.T) {
	const peerWindow = 30 * time.Second // handleSnapshot's "snapshot-public:" throttle
	if snapshotRetryWait <= peerWindow {
		t.Fatalf("retry wait is %s against a %s peer window: a retry that lands inside the "+
			"window is refused again, so the node burns all its attempts without ever "+
			"reaching a fetch that could succeed", snapshotRetryWait, peerWindow)
	}
	if total := time.Duration(snapshotRetryAttempts-1) * snapshotRetryWait; total > 3*time.Minute {
		t.Errorf("waiting out a throttling peer can take %s; that is long enough that the "+
			"height-stall monitor would fire first and restart the node mid-recovery", total)
	}
}
