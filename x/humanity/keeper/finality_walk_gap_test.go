package keeper

import (
	"testing"
	"time"
)

func initOrphanTracking(dag *BlockDAG) {
	dag.orphans = make(map[string][]*Block)
	dag.orphanFirstSeen = make(map[string]time.Time)
	dag.finalityWalkGaps = make(map[string]bool)
	dag.produceStuckGaps = make(map[string]bool)
	dag.orphanAttempts = make(map[string]int)
}

// TestRegisterFinalityWalkGap_AddsHashToPendingFetch is the regression guard
// for the live 2026-07-10 incident: finishCheckpointWalkFromDB used to log
// "genuinely lost block" and return when its DB lookup came up empty,
// without ever registering the hash anywhere fetchMissingAncestors
// (sync_blocks.go, running on a ~1s ticker per sync peer) actually reads
// from — so the exact same lookup failed identically forever. This verifies
// the fix's core mechanism: after registerFinalityWalkGap(hash), that hash
// must appear in PendingFetchHashes() — the slice fetchMissingAncestors
// actually iterates every tick to decide what to fetch.
func TestRegisterFinalityWalkGap_AddsHashToPendingFetch(t *testing.T) {
	dag := newGhostdagTestDAG()
	initOrphanTracking(dag)
	const hash = "deadbeef00000000000000000000000000000000000000000000000000000"

	dag.registerFinalityWalkGap(hash)

	found := false
	for _, h := range dag.PendingFetchHashes() {
		if h == hash {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("registerFinalityWalkGap(%q) did not add the hash to PendingFetchHashes() — fetchMissingAncestors would never attempt to fetch it", hash)
	}
}

// TestRegisterFinalityWalkGap_DoesNotFeedDeepScanTrigger is the regression
// guard for the SECOND live incident this fix's own first version caused,
// found the same day: that version wrote into dag.orphans, the exact map
// MissingParentHashes() exposes to doSyncOnce's wantDeepScan trigger
// (sync_blocks.go: wantDeepScan := len(dag.MissingParentHashes()) > 0).
// A checkpoint walk finds a fresh gap on essentially every call as the
// target keeps sliding forward with the tip, so wantDeepScan stayed
// permanently true — confirmed live: a node stuck re-scanning its entire
// history from height 0 in a loop, re-importing thousands of ancient
// disconnected block fragments as new dag.tips every pass (5000+ tips,
// block production halted) instead of ever reaching steady-state real-time
// sync. A finality-walk gap must never appear in MissingParentHashes().
func TestRegisterFinalityWalkGap_DoesNotFeedDeepScanTrigger(t *testing.T) {
	dag := newGhostdagTestDAG()
	initOrphanTracking(dag)
	const hash = "1eaf00000000000000000000000000000000000000000000000000000000"

	dag.registerFinalityWalkGap(hash)

	for _, h := range dag.MissingParentHashes() {
		if h == hash {
			t.Fatalf("registerFinalityWalkGap(%q) leaked into MissingParentHashes() — this would keep doSyncOnce's wantDeepScan permanently true", hash)
		}
	}
	if len(dag.MissingParentHashes()) != 0 {
		t.Fatalf("MissingParentHashes() = %v, want empty — a finality-walk-only gap must never trigger deepScan", dag.MissingParentHashes())
	}
}

// TestRegisterFinalityWalkGap_DoesNotClobberExistingWaiters verifies that
// registering a hash the normal orphan machinery already knows about (a
// real block IS waiting on it) never touches that separate waiter list —
// orphans and finalityWalkGaps are independent maps.
func TestRegisterFinalityWalkGap_DoesNotClobberExistingWaiters(t *testing.T) {
	dag := newGhostdagTestDAG()
	initOrphanTracking(dag)
	const hash = "cafebabe00000000000000000000000000000000000000000000000000000"
	waiter := &Block{Hash: "waiter", Height: 5}

	dag.orphansMu.Lock()
	dag.orphans[hash] = []*Block{waiter}
	dag.orphansMu.Unlock()

	dag.registerFinalityWalkGap(hash)

	dag.orphansMu.Lock()
	got := dag.orphans[hash]
	dag.orphansMu.Unlock()

	if len(got) != 1 || got[0] != waiter {
		t.Fatalf("registerFinalityWalkGap touched the unrelated orphans waiter list for %q: got %+v", hash, got)
	}
}

// TestRegisterFinalityWalkGap_SetsFirstSeenForNewHash verifies a freshly
// registered gap gets a first-seen timestamp — without it, orphanAbandonAfter
// bookkeeping (block.go) would never age this entry out if it truly never
// resolves (e.g. a hash no peer has either).
func TestRegisterFinalityWalkGap_SetsFirstSeenForNewHash(t *testing.T) {
	dag := newGhostdagTestDAG()
	initOrphanTracking(dag)
	const hash = "0badf00d00000000000000000000000000000000000000000000000000000"

	before := time.Now()
	dag.registerFinalityWalkGap(hash)
	after := time.Now()

	dag.orphansMu.Lock()
	seen, ok := dag.orphanFirstSeen[hash]
	dag.orphansMu.Unlock()

	if !ok {
		t.Fatalf("registerFinalityWalkGap(%q) did not set orphanFirstSeen", hash)
	}
	if seen.Before(before) || seen.After(after) {
		t.Fatalf("orphanFirstSeen[%q] = %v, want between %v and %v", hash, seen, before, after)
	}
}

// TestClearFinalityWalkGap_RemovesEntry verifies the cleanup path used once
// PendingFetchHashes' caller (fetchMissingAncestors) finds the hash resolved.
func TestClearFinalityWalkGap_RemovesEntry(t *testing.T) {
	dag := newGhostdagTestDAG()
	initOrphanTracking(dag)
	const hash = "5ca1ab1e0000000000000000000000000000000000000000000000000000"

	dag.registerFinalityWalkGap(hash)
	dag.clearFinalityWalkGap(hash)

	for _, h := range dag.PendingFetchHashes() {
		if h == hash {
			t.Fatalf("clearFinalityWalkGap(%q) did not remove the entry: still in PendingFetchHashes()", hash)
		}
	}
}
