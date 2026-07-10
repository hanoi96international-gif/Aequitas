package keeper

import (
	"os"
	"testing"
	"time"
)

// TestRegisterProduceStuckGap_AddsHashToPendingFetch is the regression guard
// for the live 2026-07-10 incident (620-783 synthetic-checkpoint stubs
// accumulated on the two secondary production nodes): ProduceBlock's own
// stuck-ancestor escape hatch used to track a missing merge-set ancestor
// with nothing but a raw local tick counter, never once registering the
// hash anywhere fetchMissingAncestors (sync_blocks.go, running on a ~1s
// ticker per sync peer) actually reads from. It passively hoped ordinary
// gossip would deliver the ancestor within 5 ticks (~5s at BLOCK_TIME=1s)
// and fabricated a permanent stub the instant that race was lost — even
// though the real block was often one HTTP call away on a peer the whole
// time. This verifies the fix's core mechanism: after
// registerProduceStuckGap(hash), that hash must appear in
// PendingFetchHashes(), the slice fetchMissingAncestors actually iterates
// every tick to decide what to fetch.
func TestRegisterProduceStuckGap_AddsHashToPendingFetch(t *testing.T) {
	dag := newGhostdagTestDAG()
	initOrphanTracking(dag)
	const hash = "feedface00000000000000000000000000000000000000000000000000000"

	dag.registerProduceStuckGap(hash)

	found := false
	for _, h := range dag.PendingFetchHashes() {
		if h == hash {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("registerProduceStuckGap(%q) did not add the hash to PendingFetchHashes() — fetchMissingAncestors would never attempt to fetch it, reproducing the passive-wait bug", hash)
	}
}

// TestRegisterProduceStuckGap_DoesNotFeedDeepScanTrigger mirrors
// registerFinalityWalkGap's identical guard: produceStuckGaps must stay out
// of MissingParentHashes() (doSyncOnce's wantDeepScan trigger, sync_blocks.go)
// for the same reason a finality-walk gap must — ProduceBlock can find a
// fresh stuck hash on essentially every tick while genuinely waiting on
// ordinary propagation, and reusing dag.orphans for that would risk the
// exact permanent-deepScan spiral already fixed once for finality gaps.
func TestRegisterProduceStuckGap_DoesNotFeedDeepScanTrigger(t *testing.T) {
	dag := newGhostdagTestDAG()
	initOrphanTracking(dag)
	const hash = "0ff1ce0000000000000000000000000000000000000000000000000000000"

	dag.registerProduceStuckGap(hash)

	if len(dag.MissingParentHashes()) != 0 {
		t.Fatalf("MissingParentHashes() = %v, want empty — a ProduceBlock stuck-gap must never trigger deepScan", dag.MissingParentHashes())
	}
}

// TestRegisterProduceStuckGap_SetsFirstSeenForNewHash verifies a freshly
// registered gap gets a first-seen timestamp, shared with the same
// orphanFirstSeen map queueOrphan/registerFinalityWalkGap already write —
// without it, produceStuckGapReady's orphanAbandonAfter bookkeeping could
// never age this entry in.
func TestRegisterProduceStuckGap_SetsFirstSeenForNewHash(t *testing.T) {
	dag := newGhostdagTestDAG()
	initOrphanTracking(dag)
	const hash = "b16b00b500000000000000000000000000000000000000000000000000000"

	before := time.Now()
	dag.registerProduceStuckGap(hash)
	after := time.Now()

	dag.orphansMu.Lock()
	seen, ok := dag.orphanFirstSeen[hash]
	dag.orphansMu.Unlock()

	if !ok {
		t.Fatalf("registerProduceStuckGap(%q) did not set orphanFirstSeen", hash)
	}
	if seen.Before(before) || seen.After(after) {
		t.Fatalf("orphanFirstSeen[%q] = %v, want between %v and %v", hash, seen, before, after)
	}
}

// TestRegisterProduceStuckGap_DoesNotResetFirstSeenOnRepeatedTicks verifies
// that calling registerProduceStuckGap again for the same hash (exactly what
// happens every ProduceBlock tick while still stuck) never resets the clock
// — otherwise a hash that reappears every single tick could never age past
// orphanAbandonAfter, since its "first seen" timestamp would keep sliding
// forward forever.
func TestRegisterProduceStuckGap_DoesNotResetFirstSeenOnRepeatedTicks(t *testing.T) {
	dag := newGhostdagTestDAG()
	initOrphanTracking(dag)
	const hash = "5eed00000000000000000000000000000000000000000000000000000000"

	dag.registerProduceStuckGap(hash)
	dag.orphansMu.Lock()
	original := dag.orphanFirstSeen[hash]
	dag.orphansMu.Unlock()

	dag.registerProduceStuckGap(hash)

	dag.orphansMu.Lock()
	got := dag.orphanFirstSeen[hash]
	dag.orphansMu.Unlock()

	if !got.Equal(original) {
		t.Fatalf("registerProduceStuckGap reset orphanFirstSeen[%q] from %v to %v — a hash stuck across many ticks would never reach orphanAbandonAfter", hash, original, got)
	}
}

// TestClearProduceStuckGap_RemovesEntry verifies the cleanup path used once
// PendingFetchHashes' caller (fetchMissingAncestors) finds the hash resolved,
// or once ProduceBlock's own bridge fires for it.
func TestClearProduceStuckGap_RemovesEntry(t *testing.T) {
	dag := newGhostdagTestDAG()
	initOrphanTracking(dag)
	const hash = "decade0000000000000000000000000000000000000000000000000000000"

	dag.registerProduceStuckGap(hash)
	dag.clearProduceStuckGap(hash)

	for _, h := range dag.PendingFetchHashes() {
		if h == hash {
			t.Fatalf("clearProduceStuckGap(%q) did not remove the entry: still in PendingFetchHashes()", hash)
		}
	}
}

// TestProduceStuckGapReady_FalseImmediatelyAfterRegistering is the core
// regression guard for the incident: a hash just seen for the first time
// must NOT be ready to bridge, no matter how the old 5-tick counter would
// have judged it — it must wait for the same orphanAbandonAfter/
// minOrphanAttemptsBeforeAbandon standard every other stub site already
// uses.
func TestProduceStuckGapReady_FalseImmediatelyAfterRegistering(t *testing.T) {
	os.Setenv("ALLOW_RUNTIME_ORPHAN_BRIDGE", "true")
	defer os.Unsetenv("ALLOW_RUNTIME_ORPHAN_BRIDGE")

	dag := newGhostdagTestDAG()
	initOrphanTracking(dag)
	const hash = "a11a00000000000000000000000000000000000000000000000000000000"

	dag.registerProduceStuckGap(hash)

	if dag.produceStuckGapReady(hash) {
		t.Fatalf("produceStuckGapReady(%q) = true immediately after first registration — must wait orphanAbandonAfter with real fetch attempts first", hash)
	}
}

// TestProduceStuckGapReady_FalseWithoutEnoughAttemptsEvenIfOld verifies time
// alone is not sufficient — see orphanAbandonAfter's own comment for the
// live incident (a hash sitting in a queueing backlog aging out without ever
// actually being tried) this guards against.
func TestProduceStuckGapReady_FalseWithoutEnoughAttemptsEvenIfOld(t *testing.T) {
	os.Setenv("ALLOW_RUNTIME_ORPHAN_BRIDGE", "true")
	defer os.Unsetenv("ALLOW_RUNTIME_ORPHAN_BRIDGE")

	dag := newGhostdagTestDAG()
	initOrphanTracking(dag)
	const hash = "01d0000000000000000000000000000000000000000000000000000000000"

	dag.orphansMu.Lock()
	dag.orphanFirstSeen[hash] = time.Now().Add(-orphanAbandonAfter - time.Minute)
	dag.orphanAttempts[hash] = minOrphanAttemptsBeforeAbandon - 1
	dag.orphansMu.Unlock()

	if dag.produceStuckGapReady(hash) {
		t.Fatalf("produceStuckGapReady(%q) = true with only %d fetch attempt(s) (need %d) — elapsed time alone must not be enough", hash, minOrphanAttemptsBeforeAbandon-1, minOrphanAttemptsBeforeAbandon)
	}
}

// TestProduceStuckGapReady_FalseWhileCatchingUp is the regression guard for
// the exact class of incident queueOrphan's own catchingUp carve-out fixed
// (see TestQueueOrphan_SkipsStubBridgeWhileCatchingUp): a node still
// performing its own initial catch-up can be too slow/loaded to reach a
// perfectly reachable peer in time, which must never be mistaken for the
// ancestor being genuinely gone.
func TestProduceStuckGapReady_FalseWhileCatchingUp(t *testing.T) {
	os.Setenv("ALLOW_RUNTIME_ORPHAN_BRIDGE", "true")
	defer os.Unsetenv("ALLOW_RUNTIME_ORPHAN_BRIDGE")

	dag := newGhostdagTestDAG()
	initOrphanTracking(dag)
	dag.height = 100
	dag.bootHeight = 5000 // isCatchingUpLocked() == true
	const hash = "ca7c40000000000000000000000000000000000000000000000000000000"

	dag.orphansMu.Lock()
	dag.orphanFirstSeen[hash] = time.Now().Add(-orphanAbandonAfter - time.Minute)
	dag.orphanAttempts[hash] = minOrphanAttemptsBeforeAbandon
	dag.orphansMu.Unlock()

	if dag.produceStuckGapReady(hash) {
		t.Fatalf("produceStuckGapReady(%q) = true while this node is still catching up — must not bridge fake history just because catch-up is slow", hash)
	}
}

// TestProduceStuckGapReady_FalseWithoutOptInFlag verifies bridging stays an
// explicit, deliberate operator opt-in (ALLOW_RUNTIME_ORPHAN_BRIDGE) for
// ProduceBlock's own bridge, exactly as it already is for queueOrphan's —
// the old 5-tick version bridged unconditionally regardless of this flag,
// which this closes.
func TestProduceStuckGapReady_FalseWithoutOptInFlag(t *testing.T) {
	os.Unsetenv("ALLOW_RUNTIME_ORPHAN_BRIDGE")

	dag := newGhostdagTestDAG()
	initOrphanTracking(dag)
	const hash = "defec7000000000000000000000000000000000000000000000000000000"

	dag.orphansMu.Lock()
	dag.orphanFirstSeen[hash] = time.Now().Add(-orphanAbandonAfter - time.Minute)
	dag.orphanAttempts[hash] = minOrphanAttemptsBeforeAbandon
	dag.orphansMu.Unlock()

	if dag.produceStuckGapReady(hash) {
		t.Fatalf("produceStuckGapReady(%q) = true without ALLOW_RUNTIME_ORPHAN_BRIDGE set — bridging must require explicit operator opt-in, not fire by default", hash)
	}
}

// TestProduceStuckGapReady_TrueOnceAllThresholdsMet confirms the bridge can
// still fire once a hash has genuinely, patiently exhausted every real
// standard — this is not a regression guard against bridging ever happening,
// only against it happening too eagerly.
func TestProduceStuckGapReady_TrueOnceAllThresholdsMet(t *testing.T) {
	os.Setenv("ALLOW_RUNTIME_ORPHAN_BRIDGE", "true")
	defer os.Unsetenv("ALLOW_RUNTIME_ORPHAN_BRIDGE")

	dag := newGhostdagTestDAG()
	initOrphanTracking(dag)
	dag.height = 5000
	dag.bootHeight = 5000 // caught up
	const hash = "9000d0000000000000000000000000000000000000000000000000000000"

	dag.orphansMu.Lock()
	dag.orphanFirstSeen[hash] = time.Now().Add(-orphanAbandonAfter - time.Minute)
	dag.orphanAttempts[hash] = minOrphanAttemptsBeforeAbandon
	dag.orphansMu.Unlock()

	if !dag.produceStuckGapReady(hash) {
		t.Fatalf("produceStuckGapReady(%q) = false with all thresholds satisfied (old, enough attempts, not catching up, opt-in set) — the bridge would never fire, reintroducing the original 1000+-tick permanent-halt incident this escape hatch exists for", hash)
	}
}
