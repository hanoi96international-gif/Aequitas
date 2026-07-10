package keeper

import (
	"os"
	"testing"
	"time"
)

// TestQueueOrphan_SkipsStubBridgeWhileCatchingUp is the regression guard for
// the live 2026-07-10 Contabo1 incident: this node's own initial catch-up
// (dag.height far below dag.bootHeight, i.e. isCatchingUpLocked()==true) was
// itself slow enough — under the combined lock contention of concurrent
// ProduceBlock, push handling, and deep-scan catch-up — that plenty of
// genuinely-available parents missed the 15-minute/3-attempt orphanAbandonAfter
// window. Each miss got bridged into a permanent synthetic-checkpoint stub,
// which made every subsequent GHOSTDAG merge-set computation heavier and
// catch-up slower still — a self-reinforcing spiral confirmed live to have
// fabricated 469,599 stub events while a healthy sibling node bootstrapped
// from the identical snapshot at the identical time finished clean. A block
// missing during THIS node's own catch-up must stay queued, not get replaced
// with fake history.
func TestQueueOrphan_SkipsStubBridgeWhileCatchingUp(t *testing.T) {
	os.Setenv("ALLOW_RUNTIME_ORPHAN_BRIDGE", "true")
	defer os.Unsetenv("ALLOW_RUNTIME_ORPHAN_BRIDGE")

	dag := newGhostdagTestDAG()
	initOrphanTracking(dag)
	dag.orphanAttempts = make(map[string]int)
	dag.height = 100      // far below bootHeight — still catching up
	dag.bootHeight = 5000 // dag.height+10 < bootHeight -> isCatchingUpLocked() == true

	const missingParent = "beefbeef00000000000000000000000000000000000000000000000000000"
	waiter := &Block{Hash: "waiter-1", Height: 200}

	dag.orphansMu.Lock()
	dag.orphans[missingParent] = []*Block{waiter}
	dag.orphanFirstSeen[missingParent] = time.Now().Add(-orphanAbandonAfter - time.Minute)
	dag.orphanAttempts[missingParent] = minOrphanAttemptsBeforeAbandon
	dag.orphansMu.Unlock()

	triggerBlock := &Block{Hash: "waiter-2", Height: 201}
	dag.queueOrphan(missingParent, triggerBlock)

	dag.mu.RLock()
	_, stubExists := dag.blocks[missingParent]
	dag.mu.RUnlock()
	if stubExists {
		t.Fatalf("queueOrphan bridged %q into a synthetic-checkpoint stub while this node is still catching up — the real parent is still fetchable, this just fabricates permanent fake history", missingParent)
	}

	dag.orphansMu.Lock()
	waiting := dag.orphans[missingParent]
	dag.orphansMu.Unlock()
	if len(waiting) != 2 {
		t.Fatalf("both blocks waiting on %q must remain queued (not abandoned) while catching up, got %d", missingParent, len(waiting))
	}
}

// TestQueueOrphan_StillBridgesWhenNotCatchingUp confirms the pre-existing,
// deliberate ALLOW_RUNTIME_ORPHAN_BRIDGE behavior is unchanged for a node
// that is NOT in its own catch-up phase — the catchingUp gate must only
// suppress the bridge during startup catch-up, never once a node is settled
// and a parent is genuinely, permanently gone.
func TestQueueOrphan_StillBridgesWhenNotCatchingUp(t *testing.T) {
	os.Setenv("ALLOW_RUNTIME_ORPHAN_BRIDGE", "true")
	defer os.Unsetenv("ALLOW_RUNTIME_ORPHAN_BRIDGE")

	dag := newGhostdagTestDAG()
	initOrphanTracking(dag)
	dag.orphanAttempts = make(map[string]int)
	dag.unverifiedStubHeights = make(map[string]int64)
	dag.height = 5000
	dag.bootHeight = 5000 // caught up: isCatchingUpLocked() == false

	const missingParent = "deadfeed00000000000000000000000000000000000000000000000000000"
	waiter := &Block{Hash: "waiter-3", Height: 5100}

	dag.orphansMu.Lock()
	dag.orphans[missingParent] = []*Block{waiter}
	dag.orphanFirstSeen[missingParent] = time.Now().Add(-orphanAbandonAfter - time.Minute)
	dag.orphanAttempts[missingParent] = minOrphanAttemptsBeforeAbandon
	dag.orphansMu.Unlock()

	triggerBlock := &Block{Hash: "waiter-4", Height: 5101}
	dag.queueOrphan(missingParent, triggerBlock)

	dag.mu.RLock()
	_, stubExists := dag.blocks[missingParent]
	dag.mu.RUnlock()
	if !stubExists {
		t.Fatalf("queueOrphan must still bridge a permanently-unresolvable parent into a synthetic-checkpoint stub once this node is caught up, per the existing ALLOW_RUNTIME_ORPHAN_BRIDGE contract")
	}
}
