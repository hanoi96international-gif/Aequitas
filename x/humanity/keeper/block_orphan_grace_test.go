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

// TestAddPeerBlock_BelowBootHeightAcceptedWithoutOrphanQueue is the
// regression guard for the 2026-07-04 fix found live right after a fresh
// checkpoint-seeded resync: stale gossip/relay of a proposer's own
// PRE-resync blocks (height at or below the just-seeded checkpoint) kept
// arriving and being queued as orphans, wasting the exact resolution
// machinery that should focus entirely on catching up to the current tip.
// A block at or below BootHeight is already fully accounted for by the
// checkpoint and must be reported as accepted without ever reaching the
// orphan queue — but ONLY when BootHeight is actually checkpoint-backed
// (see bootHeightCheckpointBacked's own comment and the next test below for
// the unsafe case this same skip must NOT apply to).
func TestAddPeerBlock_BelowBootHeightAcceptedWithoutOrphanQueue(t *testing.T) {
	dag := newOrphanTestDAG()
	dag.state = &ChainState{}
	dag.bootHeight = 100
	dag.bootHeightCheckpointBacked = true
	// Height 50 (below bootHeight) references a parent this node was never
	// going to have (it was wiped by the resync) -- must still be accepted,
	// not queued as an orphan.
	stale := &Block{Hash: "stale-child", Height: 50, ParentHashes: []string{"long-gone-parent"}, Proposer: "0xhonest"}
	if !dag.AddPeerBlock(stale) {
		t.Fatal("a block at or below a checkpoint-backed BootHeight must be reported as accepted")
	}
	if _, tracked := dag.orphanAge("long-gone-parent"); tracked {
		t.Fatal("a block at or below a checkpoint-backed BootHeight must never reach the orphan queue")
	}
}

// TestAddPeerBlock_BelowBootHeightNotCheckpointBackedFallsThrough is the
// regression guard for the 2026-07-04 permanent-isolation-after-plain-restart
// incident: after a PLAIN restart (no resync), BootHeight gets ratcheted up
// to a bare persisted max_block_height NUMBER with no matching block ever
// stored in dag.blocks/dag.tips at that height. Confirmed live on both
// Contabo nodes: the old unconditional skip waved through every real
// historical block up to that height without ever storing them, so every
// later block referencing one of them as a parent orphaned permanently —
// the DAG never actually merged, just kept re-reporting the same "added"
// blocks that never stuck. Without checkpoint backing, the skip must NOT
// fire — a genuinely valid, properly signed and authorized block must fall
// through to normal processing (which for a still-missing parent means the
// ordinary orphan queue, not a silent, unstored "accepted").
func TestAddPeerBlock_BelowBootHeightNotCheckpointBackedFallsThrough(t *testing.T) {
	dag := newOrphanTestDAG()
	dag.state = &ChainState{}
	dag.bootHeight = 100
	dag.bootHeightCheckpointBacked = false // plain-restart case, not a checkpoint-seeded resync
	blk := signTestBlockWithParent(t, 50, "long-gone-parent")
	dag.authorizedValidators = map[string]bool{blk.Proposer: true}
	if dag.AddPeerBlock(blk) {
		t.Fatal("without checkpoint backing, a block below BootHeight must not be silently accepted")
	}
	if _, tracked := dag.orphanAge("long-gone-parent"); !tracked {
		t.Fatal("without checkpoint backing, a block with a genuinely missing parent must reach the normal orphan queue, not be silently waved through")
	}
	if _, exists := dag.blocks[blk.Hash]; exists {
		t.Fatal("a block waved through without checkpoint backing must never be silently stored as if verified")
	}
}

// TestAddPeerBlock_SelfFetchedGenuinelyAwaitedFallsThrough is the regression
// guard for the third layer of the 2026-07-04 incident, found live even
// WITH checkpoint backing genuinely correct: fetchMissingAncestors
// deliberately, individually fetches ONE specific missing-parent hash
// because some already-orphaned child genuinely needs it — that is exactly
// why it's SelfFetched. Confirmed live: it kept resolving hashes that
// happened to land at or below a correctly checkpoint-backed BootHeight (a
// second isolated peer's own historical chain, walked backward one hash at
// a time), got the free pass, and was reported "accepted" WITHOUT ever
// being stored — so the orphaned child waiting on that exact hash could
// never resolve no matter how many times the fetch "succeeded".
//
// fetchMissingAncestors fetches BY HASH, so the delivered block's OWN hash
// IS the previously-missing-parent hash something is waiting on — this
// test's setup mirrors that shape (queues a child waiting on "ancestor-
// hash", then delivers a SelfFetched block whose Hash IS "ancestor-hash"),
// unlike the deepScan/paging shape covered by the stale-no-longer-awaited
// test below. A SelfFetched block that's still genuinely awaited must fall
// through to normal processing and be genuinely stored, even when it also
// happens to be at or below a checkpoint-backed BootHeight.
func TestAddPeerBlock_SelfFetchedGenuinelyAwaitedFallsThrough(t *testing.T) {
	dag := newOrphanTestDAG()
	dag.state = &ChainState{}
	dag.bootHeight = 100
	dag.bootHeightCheckpointBacked = true // genuinely checkpoint-backed this time
	ancestor := signTestBlockWithParent(t, 50, "even-older-parent")
	waitingChild := &Block{Hash: "waiting-child", Height: 51, ParentHashes: []string{ancestor.Hash}, Proposer: "0xhonest"}
	dag.queueOrphan(ancestor.Hash, waitingChild) // something is genuinely, actively waiting on this exact hash

	ancestor.SelfFetched = true
	dag.authorizedValidators = map[string]bool{ancestor.Proposer: true}
	if dag.AddPeerBlock(ancestor) {
		t.Fatal("a SelfFetched, genuinely-awaited block whose own parent is also missing should queue as an orphan, not report accepted")
	}
	if _, exists := dag.blocks[ancestor.Hash]; exists {
		t.Fatal("a block whose own parent is missing must not be stored in dag.blocks")
	}
	if _, tracked := dag.orphanAge("even-older-parent"); !tracked {
		t.Fatal("a SelfFetched block that is still genuinely awaited must fall through to normal processing (reaching the orphan queue for ITS OWN missing parent), not be silently skipped")
	}
}

// TestAddPeerBlock_SelfFetchedStaleNoLongerAwaitedIsSkipped is the
// regression guard for the 2026-07-05 fix (fourth layer of the same
// incident): a SelfFetched delivery only proves a fetch was deliberately
// issued for a hash something needed WHEN THE REQUEST WENT OUT, not that
// the need still exists when the response arrives. Confirmed live,
// repeating on every single resync: fetchMissingAncestors/doSyncOnce
// requests already in flight when a resync clears dag.orphans land moments
// later still marked SelfFetched, with nothing left waiting on them —
// scattered ancient-height blocks re-entering AddPeerBlock and queueing as
// brand-new orphans nothing needs, burning real-time catch-up attention.
// A SelfFetched block below a checkpoint-backed BootHeight that NOTHING is
// currently awaiting must be silently accepted without storage, exactly
// like a non-SelfFetched one.
func TestAddPeerBlock_SelfFetchedStaleNoLongerAwaitedIsSkipped(t *testing.T) {
	dag := newOrphanTestDAG()
	dag.state = &ChainState{}
	dag.bootHeight = 100
	dag.bootHeightCheckpointBacked = true
	stale := &Block{Hash: "stale-ancestor-hash", Height: 50, ParentHashes: []string{"long-gone-parent"}, Proposer: "0xhonest"}
	stale.SelfFetched = true
	// Deliberately do NOT queue anything waiting on "stale-ancestor-hash" —
	// simulates a resync clearing the orphan queue while this fetch was
	// already in flight.
	if !dag.AddPeerBlock(stale) {
		t.Fatal("a SelfFetched block below a checkpoint-backed BootHeight that nothing is awaiting must be reported as accepted")
	}
	if _, exists := dag.blocks["stale-ancestor-hash"]; exists {
		t.Fatal("a stale, no-longer-awaited SelfFetched block must never be stored")
	}
	if _, tracked := dag.orphanAge("long-gone-parent"); tracked {
		t.Fatal("a stale, no-longer-awaited SelfFetched block must never reach the orphan queue either")
	}
}
