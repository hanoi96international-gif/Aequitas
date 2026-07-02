package keeper

import (
	"testing"
	"time"
)

// resetBlockPushBreaker clears the package-level breaker state so each test
// starts clean (the maps are process-global).
func resetBlockPushBreaker() {
	blockPushBreakerMu.Lock()
	blockPushFailRun = map[string]int{}
	blockPushBreakerUntil = map[string]int64{}
	blockPushBreakerMu.Unlock()
}

// TestBlockPushBreaker_TripsAfterThreshold verifies a peer whose pushes never
// attach trips exactly at the threshold and is then dropped pre-parse.
func TestBlockPushBreaker_TripsAfterThreshold(t *testing.T) {
	resetBlockPushBreaker()
	const ip = "203.0.113.7"

	// One below the threshold: not yet tripped.
	for i := 0; i < blockPushBreakerThreshold-1; i++ {
		blockPushRecordOutcome(ip, false)
		if blockPushShouldDrop(ip) {
			t.Fatalf("breaker tripped early after %d failures (threshold %d)", i+1, blockPushBreakerThreshold)
		}
	}
	// The threshold-th failure trips it.
	blockPushRecordOutcome(ip, false)
	if !blockPushShouldDrop(ip) {
		t.Fatalf("breaker did not trip after %d consecutive failures", blockPushBreakerThreshold)
	}
}

// TestBlockPushBreaker_SuccessResetsRun verifies a single attaching block clears
// the failure run so a healthy peer never trips.
func TestBlockPushBreaker_SuccessResetsRun(t *testing.T) {
	resetBlockPushBreaker()
	const ip = "203.0.113.8"

	for cycle := 0; cycle < 5; cycle++ {
		for i := 0; i < blockPushBreakerThreshold-1; i++ {
			blockPushRecordOutcome(ip, false)
		}
		blockPushRecordOutcome(ip, true) // attaches — resets the run
		if blockPushShouldDrop(ip) {
			t.Fatalf("healthy peer tripped: a success must reset the failure run (cycle %d)", cycle)
		}
	}
}

// TestBlockPushBreaker_CooldownExpiryAllowsProbe verifies that once the cooldown
// elapses the IP is allowed through again (one probe) and its run is cleared.
func TestBlockPushBreaker_CooldownExpiryAllowsProbe(t *testing.T) {
	resetBlockPushBreaker()
	const ip = "203.0.113.9"

	for i := 0; i < blockPushBreakerThreshold; i++ {
		blockPushRecordOutcome(ip, false)
	}
	if !blockPushShouldDrop(ip) {
		t.Fatalf("breaker should be open immediately after tripping")
	}
	// Force the cooldown to have already expired.
	blockPushBreakerMu.Lock()
	blockPushBreakerUntil[ip] = time.Now().Add(-time.Second).UnixNano()
	blockPushBreakerMu.Unlock()

	if blockPushShouldDrop(ip) {
		t.Fatalf("breaker should let a probe through after the cooldown expires")
	}
	// Single real probe (P0 fix, 2026-07-02 liveness audit): cooldown expiry
	// clears the until-map but seeds the run one short of the threshold, not
	// at 0 — the next single outcome decides, not another full run of fresh
	// failures.
	blockPushBreakerMu.Lock()
	run := blockPushFailRun[ip]
	_, stillOpen := blockPushBreakerUntil[ip]
	blockPushBreakerMu.Unlock()
	if run != blockPushBreakerThreshold-1 || stillOpen {
		t.Fatalf("cooldown expiry must clear until and seed run at threshold-1 (run=%d want=%d open=%v)",
			run, blockPushBreakerThreshold-1, stillOpen)
	}
}

// TestBlockPushBreaker_ProbeFailureRetripsImmediately verifies the cooldown
// reopen is a single real probe, not a full reopen: one failing push right
// after cooldown expiry must re-trip the breaker immediately, not require
// another full run of blockPushBreakerThreshold fresh failures. Without the
// P0 fix this test fails — the old code let up to threshold-1 more full-cost
// pushes through every cooldown cycle before re-tripping.
func TestBlockPushBreaker_ProbeFailureRetripsImmediately(t *testing.T) {
	resetBlockPushBreaker()
	const ip = "203.0.113.10"

	for i := 0; i < blockPushBreakerThreshold; i++ {
		blockPushRecordOutcome(ip, false)
	}
	blockPushBreakerMu.Lock()
	blockPushBreakerUntil[ip] = time.Now().Add(-time.Second).UnixNano()
	blockPushBreakerMu.Unlock()

	if blockPushShouldDrop(ip) {
		t.Fatalf("precondition: probe should be let through after cooldown expiry")
	}
	blockPushRecordOutcome(ip, false) // the single probe fails
	if !blockPushShouldDrop(ip) {
		t.Fatalf("a single failing probe must re-trip the breaker immediately, not require %d more failures", blockPushBreakerThreshold-1)
	}
}

// TestBlockPushBreaker_ProbeSuccessClearsRun verifies the cooldown reopen
// still lets a genuinely-healed peer back in on a single success.
func TestBlockPushBreaker_ProbeSuccessClearsRun(t *testing.T) {
	resetBlockPushBreaker()
	const ip = "203.0.113.11"

	for i := 0; i < blockPushBreakerThreshold; i++ {
		blockPushRecordOutcome(ip, false)
	}
	blockPushBreakerMu.Lock()
	blockPushBreakerUntil[ip] = time.Now().Add(-time.Second).UnixNano()
	blockPushBreakerMu.Unlock()

	if blockPushShouldDrop(ip) {
		t.Fatalf("precondition: probe should be let through after cooldown expiry")
	}
	blockPushRecordOutcome(ip, true) // the single probe attaches
	if blockPushShouldDrop(ip) {
		t.Fatalf("a single successful probe must clear the run, not leave the IP blocked")
	}
}

// TestBlockPushBreaker_DenylistAlwaysDrops verifies the static env denylist
// blocks an IP regardless of breaker state.
func TestBlockPushBreaker_DenylistAlwaysDrops(t *testing.T) {
	resetBlockPushBreaker()
	const ip = "198.51.100.4"

	if blockPushShouldDrop(ip) {
		t.Fatalf("clean IP must not be dropped before denylisting")
	}
	// Simulate PEER_PUSH_DENYLIST containing this IP (the var is parsed once at
	// init from the env, so inject directly for the test).
	blockPushIPDenylist[ip] = true
	defer delete(blockPushIPDenylist, ip)

	if !blockPushShouldDrop(ip) {
		t.Fatalf("denylisted IP must always be dropped")
	}
}
