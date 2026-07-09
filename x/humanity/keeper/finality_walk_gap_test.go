package keeper

import (
	"testing"
	"time"
)

// TestRegisterFinalityWalkGap_AddsHashToOrphanTracking is the regression
// guard for the live 2026-07-10 incident: finishCheckpointWalkFromDB used
// to log "genuinely lost block" and return when its DB lookup came up
// empty, without ever registering the hash anywhere fetchMissingAncestors
// (sync_blocks.go, running on a ~1s ticker per sync peer) actually reads
// from — so the exact same lookup failed identically forever, confirmed
// live as the same hash repeating 10+ times in a row. This verifies the
// fix's core mechanism in isolation: after registerFinalityWalkGap(hash),
// that hash must appear in MissingParentHashes() — the same slice
// fetchMissingAncestors iterates every tick to decide what to fetch.
func TestRegisterFinalityWalkGap_AddsHashToOrphanTracking(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.orphans = make(map[string][]*Block)
	dag.orphanFirstSeen = make(map[string]time.Time)
	const hash = "deadbeef00000000000000000000000000000000000000000000000000000"

	dag.registerFinalityWalkGap(hash)

	found := false
	for _, h := range dag.MissingParentHashes() {
		if h == hash {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("registerFinalityWalkGap(%q) did not add the hash to MissingParentHashes() — fetchMissingAncestors would never attempt to fetch it", hash)
	}
}

// TestRegisterFinalityWalkGap_DoesNotClobberExistingWaiters verifies that
// registering a hash the normal orphan machinery already knows about (a
// real block IS waiting on it) never wipes out that waiter list — only a
// genuinely-new entry should be added as nil.
func TestRegisterFinalityWalkGap_DoesNotClobberExistingWaiters(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.orphans = make(map[string][]*Block)
	dag.orphanFirstSeen = make(map[string]time.Time)
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
		t.Fatalf("registerFinalityWalkGap clobbered an existing waiter list for %q: got %+v", hash, got)
	}
}

// TestRegisterFinalityWalkGap_SetsFirstSeenForNewHash verifies a freshly
// registered gap gets a first-seen timestamp — without it, orphanAbandonAfter
// bookkeeping (block.go) would never age this entry out if it truly never
// resolves (e.g. a hash no peer has either).
func TestRegisterFinalityWalkGap_SetsFirstSeenForNewHash(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.orphans = make(map[string][]*Block)
	dag.orphanFirstSeen = make(map[string]time.Time)
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
