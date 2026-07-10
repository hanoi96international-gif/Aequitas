package keeper

import "testing"

// TestEffectiveDeepScanFloor_DefaultsToNormalFloor is the baseline: a peer
// that has never triggered lowerDeepScanFloor must see deepScanFloor()'s own
// value unchanged — matching every peer's behavior before the 2026-07-10
// progressive-floor fix.
func TestEffectiveDeepScanFloor_DefaultsToNormalFloor(t *testing.T) {
	dag := &BlockDAG{bootHeight: 690000, bootHeightCheckpointBacked: true, deepScanFloorOverride: make(map[string]int64)}
	if got := dag.effectiveDeepScanFloor("https://peer.example"); got != 690000 {
		t.Fatalf("effectiveDeepScanFloor() with no override = %d, want deepScanFloor() = 690000", got)
	}
}

// TestEffectiveDeepScanFloor_UsesOverrideWhenLower is the core regression
// guard for the Primary/Contabo1 permanent-partial-merge incident: once
// lowerDeepScanFloor has recorded a lower floor for this specific peer, the
// next sweep must actually start from there instead of deepScanFloor()'s own
// (too-high, for this peer) value.
func TestEffectiveDeepScanFloor_UsesOverrideWhenLower(t *testing.T) {
	dag := &BlockDAG{bootHeight: 690000, bootHeightCheckpointBacked: true, deepScanFloorOverride: make(map[string]int64)}
	dag.deepScanFloorOverride["https://peer.example"] = 345000
	if got := dag.effectiveDeepScanFloor("https://peer.example"); got != 345000 {
		t.Fatalf("effectiveDeepScanFloor() with a lower override = %d, want 345000", got)
	}
}

// TestEffectiveDeepScanFloor_IgnoresOverrideWhenHigherThanFloor guards
// against an override computed against a stale (lower) deepScanFloor()
// somehow ending up ABOVE the current one (e.g. a fresh resync raised
// BootHeight past where the override was last set) — effectiveDeepScanFloor
// must never let the peer's floor become less safe than deepScanFloor()
// itself, only ever more searching.
func TestEffectiveDeepScanFloor_IgnoresOverrideWhenHigherThanFloor(t *testing.T) {
	dag := &BlockDAG{bootHeight: 100, bootHeightCheckpointBacked: true, deepScanFloorOverride: make(map[string]int64)}
	dag.deepScanFloorOverride["https://peer.example"] = 5000 // stale, from before a resync dropped BootHeight
	if got := dag.effectiveDeepScanFloor("https://peer.example"); got != 100 {
		t.Fatalf("effectiveDeepScanFloor() with a stale override above the current floor = %d, want deepScanFloor() = 100", got)
	}
}

// TestEffectiveDeepScanFloor_PerPeerIndependence mirrors
// TestDeepScanResumeHeight_PerPeerIndependence's exact rationale: one peer's
// lowered floor must never leak into a different peer's sweep.
func TestEffectiveDeepScanFloor_PerPeerIndependence(t *testing.T) {
	dag := &BlockDAG{bootHeight: 690000, bootHeightCheckpointBacked: true, deepScanFloorOverride: make(map[string]int64)}
	dag.deepScanFloorOverride["https://primary.example"] = 345000
	if got := dag.effectiveDeepScanFloor("https://secondary.example"); got != 690000 {
		t.Fatalf("a different peer's floor = %d, want deepScanFloor() = 690000 (unaffected by primary.example's override)", got)
	}
}

// TestClearDeepScanFloorOverride_ResetsToNormalFloor verifies a cleared
// override falls back to deepScanFloor() — the "fully resolved" path
// doSyncOnce takes once a lowered-floor sweep finally converges cleanly.
func TestClearDeepScanFloorOverride_ResetsToNormalFloor(t *testing.T) {
	dag := &BlockDAG{bootHeight: 690000, bootHeightCheckpointBacked: true, deepScanFloorOverride: make(map[string]int64)}
	dag.deepScanFloorOverride["https://peer.example"] = 345000
	dag.clearDeepScanFloorOverride("https://peer.example")
	if got := dag.effectiveDeepScanFloor("https://peer.example"); got != 690000 {
		t.Fatalf("effectiveDeepScanFloor() after clear = %d, want deepScanFloor() = 690000", got)
	}
}

// TestFinalityFloorLimit_ZeroBeforeAnyCheckpoint verifies a fresh node (or
// one with dag.state == nil) reports no limit beyond deepScan's own genesis
// floor — lowerDeepScanFloor must still be able to search all the way to 0.
func TestFinalityFloorLimit_ZeroBeforeAnyCheckpoint(t *testing.T) {
	dag := &BlockDAG{state: newTestState()}
	if got := dag.finalityFloorLimit(); got != 0 {
		t.Fatalf("finalityFloorLimit() before any checkpoint = %d, want 0", got)
	}
	dag2 := &BlockDAG{}
	if got := dag2.finalityFloorLimit(); got != 0 {
		t.Fatalf("finalityFloorLimit() with nil state = %d, want 0", got)
	}
}

// TestFinalityFloorLimit_TracksFinalizedCheckpoint verifies the limit is
// derived from the real finalized checkpoint (minus finalityHeightSlack),
// not deepScanFloor()/BootHeight — the two are independent concepts that
// happen to both bound how far back a sweep may go.
func TestFinalityFloorLimit_TracksFinalizedCheckpoint(t *testing.T) {
	cs := newTestState()
	cs.SetFinalizedCheckpoint("deadbeef", 1000, 5000)
	dag := &BlockDAG{state: cs}
	want := int64(1000 - finalityHeightSlack)
	if got := dag.finalityFloorLimit(); got != want {
		t.Fatalf("finalityFloorLimit() with finalized checkpoint 1000 = %d, want %d", got, want)
	}
}

// TestLowerDeepScanFloor_HalvesTowardTheLimit is the core regression guard
// for the Primary/Contabo1 incident: a full deepScan sweep that reached the
// peer's tip but still left blocks unmerged must narrow the floor, not leave
// it stuck at a value already proven too high.
func TestLowerDeepScanFloor_HalvesTowardTheLimit(t *testing.T) {
	// bootHeight/checkpointBacked must be consistent with the sweptFrom value
	// below: effectiveDeepScanFloor never trusts an override ABOVE
	// deepScanFloor() itself (see that guard's own comment), so a real sweep
	// starting at 100000 can only happen when deepScanFloor() is >= 100000.
	dag := &BlockDAG{bootHeight: 100000, bootHeightCheckpointBacked: true, deepScanFloorOverride: make(map[string]int64)}
	dag.state = newTestState()
	dag.lowerDeepScanFloor("https://peer.example", 100000)
	if got := dag.effectiveDeepScanFloor("https://peer.example"); got != 50000 {
		t.Fatalf("floor after one lowerDeepScanFloor(100000) with no finality limit = %d, want 50000 (halved)", got)
	}
}

// TestLowerDeepScanFloor_NeverGoesAtOrBelowFinalityLimit guards the safety
// property that makes this fix sound: no matter how many times a peer fails
// to converge, the floor must never search past the point where
// isFinalityViolation would reject the recovered block anyway — searching
// there can never help and only wastes a full sweep.
func TestLowerDeepScanFloor_NeverGoesAtOrBelowFinalityLimit(t *testing.T) {
	cs := newTestState()
	cs.SetFinalizedCheckpoint("deadbeef", 60000, 5000) // limit = 60000-50 = 59950
	// bootHeight must stay >= every sweptFrom value used below so
	// effectiveDeepScanFloor's "never above deepScanFloor()" guard (see its
	// own comment) doesn't mask what this test is actually verifying.
	dag := &BlockDAG{state: cs, bootHeight: 100000, bootHeightCheckpointBacked: true, deepScanFloorOverride: make(map[string]int64)}

	sweptFrom := int64(100000)
	for i := 0; i < 50; i++ {
		dag.lowerDeepScanFloor("https://peer.example", sweptFrom)
		got := dag.effectiveDeepScanFloor("https://peer.example")
		if got < 59950 {
			t.Fatalf("iteration %d: floor = %d, must never go below finalityFloorLimit() = 59950", i, got)
		}
		sweptFrom = got // simulate the next sweep starting from wherever this one landed
	}
	if got := dag.effectiveDeepScanFloor("https://peer.example"); got != 59950 {
		t.Fatalf("floor after repeated non-convergence = %d, want it to have converged exactly to the finality limit 59950", got)
	}
}

// TestLowerDeepScanFloor_NoOpAtOrBelowLimit verifies calling it when the
// swept range has already reached (or started below) the finality limit is
// a no-op — nothing further to try, so it must not set a spurious override.
func TestLowerDeepScanFloor_NoOpAtOrBelowLimit(t *testing.T) {
	cs := newTestState()
	cs.SetFinalizedCheckpoint("deadbeef", 1000, 5000) // limit = 950
	dag := &BlockDAG{state: cs, deepScanFloorOverride: make(map[string]int64)}
	dag.lowerDeepScanFloor("https://peer.example", 950)
	if _, ok := dag.deepScanFloorOverride["https://peer.example"]; ok {
		t.Fatal("lowerDeepScanFloor at exactly the finality limit must not set an override — there is nothing lower left worth trying")
	}
}

// TestLowerDeepScanFloor_PerPeerIndependence mirrors the other per-peer
// independence tests: one peer failing to converge must never narrow a
// different peer's floor.
func TestLowerDeepScanFloor_PerPeerIndependence(t *testing.T) {
	dag := &BlockDAG{state: newTestState(), deepScanFloorOverride: make(map[string]int64)}
	dag.lowerDeepScanFloor("https://primary.example", 100000)
	if got := dag.effectiveDeepScanFloor("https://secondary.example"); got != 0 {
		t.Fatalf("a different peer's floor = %d, want 0 (unaffected by primary.example's lowered floor)", got)
	}
}
