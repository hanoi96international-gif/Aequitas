package keeper

import (
	"testing"
	"time"
)

// newOrphanTestDAG builds a minimal BlockDAG with every map queueOrphan
// touches initialized (nil-map writes panic in Go), for exercising the
// orphan-queue / circuit-breaker-grace mechanism directly without needing
// to satisfy AddPeerBlock's full signature/authorization gauntlet.
func newOrphanTestDAG() *BlockDAG {
	return &BlockDAG{
		blocks:          make(map[string]*Block),
		tips:            make(map[string]bool),
		orphans:         make(map[string][]*Block),
		orphanFirstSeen: make(map[string]time.Time),
		orphanAttempts:  make(map[string]int),
	}
}

// TestOrphanAge_UntrackedReturnsFalse verifies a missing-parent hash that has
// never been queued reports "not tracked" rather than a zero-but-tracked age
// — AddPeerBlock's grace check treats these differently (untracked also
// skips the breaker, but for a distinct reason: nothing to compare against).
func TestOrphanAge_UntrackedReturnsFalse(t *testing.T) {
	dag := newOrphanTestDAG()
	if age, tracked := dag.orphanAge("never-seen"); tracked || age != 0 {
		t.Fatalf("orphanAge(never-seen) = (%v, %v), want (0, false)", age, tracked)
	}
}

// TestOrphanAge_ReflectsElapsedTime is the core regression guard for the
// 2026-07-04 fix (proposerBreakerOrphanGrace): confirmed live, two fully
// healthy, fully-synced Contabo nodes tripped their circuit breakers against
// EACH OTHER during ordinary steady-state operation — a short-lived
// propagation gap (entirely normal between independently-hosted validators)
// was counted as a proposer failure the instant it was first observed, with
// no chance to resolve on its own first. This verifies the underlying age
// tracking a gate on that failure-recording depends on: a freshly-queued
// orphan reports an age well under the grace period, and one manually
// backdated past it reports an age at or beyond it.
func TestOrphanAge_ReflectsElapsedTime(t *testing.T) {
	dag := newOrphanTestDAG()
	blk := &Block{Hash: "child-1", Height: 2, ParentHashes: []string{"missing-parent"}, Proposer: "0xhonest"}
	dag.queueOrphan("missing-parent", blk)

	age, tracked := dag.orphanAge("missing-parent")
	if !tracked {
		t.Fatal("orphanAge should track a hash right after queueOrphan first saw it")
	}
	if age >= proposerBreakerOrphanGrace {
		t.Fatalf("a just-queued orphan reported age %v, want well under the grace period %v", age, proposerBreakerOrphanGrace)
	}

	// Simulate the same gap persisting past the grace period — manually
	// backdate orphanFirstSeen the way real elapsed wall-clock time would.
	dag.orphansMu.Lock()
	dag.orphanFirstSeen["missing-parent"] = time.Now().Add(-2 * proposerBreakerOrphanGrace)
	dag.orphansMu.Unlock()

	age, tracked = dag.orphanAge("missing-parent")
	if !tracked {
		t.Fatal("orphanAge should still track the backdated hash")
	}
	if age < proposerBreakerOrphanGrace {
		t.Fatalf("a backdated orphan reported age %v, want at least the grace period %v", age, proposerBreakerOrphanGrace)
	}
}

// TestOrphanAge_DuplicateBlockForSameGapDoesNotResetClock verifies a second,
// distinct block waiting on the SAME missing parent does not restart the
// age clock — otherwise a proposer producing a steady stream of blocks on
// top of one genuinely stuck gap would keep re-arming its own grace period
// forever, defeating the whole point of the fix.
func TestOrphanAge_DuplicateBlockForSameGapDoesNotResetClock(t *testing.T) {
	dag := newOrphanTestDAG()
	first := &Block{Hash: "child-1", Height: 2, ParentHashes: []string{"missing-parent"}, Proposer: "0xhonest"}
	dag.queueOrphan("missing-parent", first)

	dag.orphansMu.Lock()
	backdated := time.Now().Add(-2 * proposerBreakerOrphanGrace)
	dag.orphanFirstSeen["missing-parent"] = backdated
	dag.orphansMu.Unlock()

	second := &Block{Hash: "child-2", Height: 3, ParentHashes: []string{"missing-parent"}, Proposer: "0xhonest"}
	dag.queueOrphan("missing-parent", second)

	age, tracked := dag.orphanAge("missing-parent")
	if !tracked {
		t.Fatal("orphanAge should still track the hash after a second block queues on it")
	}
	if age < proposerBreakerOrphanGrace {
		t.Fatalf("a second block on the same already-old gap reset the clock: age = %v, want >= %v", age, proposerBreakerOrphanGrace)
	}
}

// TestIsWithinOrphanGrace_FreshOrphanIsForgiven is the regression guard for
// the per-IP push-breaker half of the 2026-07-04 mutual-lockout fix: a block
// whose parent is missing but was JUST first observed (age well under the
// grace period) must be reported as within grace, so handleBlockPush
// (api.go) doesn't feed it to the per-IP breaker either.
func TestIsWithinOrphanGrace_FreshOrphanIsForgiven(t *testing.T) {
	dag := newOrphanTestDAG()
	blk := &Block{Hash: "child-1", Height: 2, ParentHashes: []string{"missing-parent"}, Proposer: "0xhonest"}
	dag.queueOrphan("missing-parent", blk)

	if !dag.IsWithinOrphanGrace(blk) {
		t.Fatal("a freshly-queued orphan should be within grace")
	}
}

// TestIsWithinOrphanGrace_OldOrphanIsNotForgiven verifies a gap that has
// persisted past the grace period is no longer forgiven — a genuinely
// diverged fork's blocks must still trip the breaker, just not instantly.
func TestIsWithinOrphanGrace_OldOrphanIsNotForgiven(t *testing.T) {
	dag := newOrphanTestDAG()
	blk := &Block{Hash: "child-1", Height: 2, ParentHashes: []string{"missing-parent"}, Proposer: "0xhonest"}
	dag.queueOrphan("missing-parent", blk)
	dag.orphansMu.Lock()
	dag.orphanFirstSeen["missing-parent"] = time.Now().Add(-2 * proposerBreakerOrphanGrace)
	dag.orphansMu.Unlock()

	if dag.IsWithinOrphanGrace(blk) {
		t.Fatal("a gap past the grace period must no longer be forgiven")
	}
}

// TestIsWithinOrphanGrace_NonOrphanRejectionNeverForgiven verifies a block
// whose parent IS known (so any rejection is for some other reason, e.g. a
// bad signature) is never reported as within grace — only a genuine missing-
// parent gap gets the benefit of the doubt.
func TestIsWithinOrphanGrace_NonOrphanRejectionNeverForgiven(t *testing.T) {
	dag := newOrphanTestDAG()
	dag.blocks["known-parent"] = &Block{Hash: "known-parent", Height: 1}
	blk := &Block{Hash: "child-1", Height: 2, ParentHashes: []string{"known-parent"}, Proposer: "0xbadsig"}

	if dag.IsWithinOrphanGrace(blk) {
		t.Fatal("a block whose parent is known must never be reported as within orphan grace")
	}
}
