package keeper

import (
	"testing"
	"time"
)

// Regression guards for the 2026-07-25 fix in doSyncOnce: a block that
// AddPeerBlock DEFERRED behind a still-in-flight parent must not reset
// cleanSyncStreak, while a block it genuinely REFUSED still must.
//
// Measured live before the fix, from each container's own log:
//
//   Contabo1  uptime 21969s   "Not yet 3 consecutive clean sync cycles" 15736
//   Contabo2  uptime 21869s   "Not yet 3 consecutive clean sync cycles"  1656
//   both      "Catch-up in progress" (the height gate)                      0
//
// Contabo1 spent 72% of its uptime unable to produce a block — up, syncing,
// serving /api/status, and silently producing nothing — and the height-based
// gate was never once responsible. Recovery after a restart measured 792s and
// 1108s respectively, which is the "es dauert ewig bis die Contabos nach dem
// Redeploy wieder laufen" the operator reported.
//
// The distinction is IsWithinOrphanGrace, which doSyncOnce now consults on
// AddPeerBlock's false path. These tests pin its three outcomes, because that
// function is what decides whether a node may produce at all.

// newOrphanGraceTestDAG builds the minimal DAG IsWithinOrphanGrace needs.
// state stays nil on purpose: ghostdagBlockLookup returns nil outright for a
// nil state, so "parent not in dag.blocks" means "missing" with no DB in the
// picture, which is exactly the condition under test.
func newOrphanGraceTestDAG() *BlockDAG {
	return &BlockDAG{
		blocks:          make(map[string]*Block),
		orphanFirstSeen: make(map[string]time.Time),
		cleanSyncStreak: make(map[string]int),
	}
}

// A block whose parents are ALL present locally did not fail on a gap — it
// was refused for a real reason (bad signature, unauthorized proposer,
// finality violation, far-ahead fork). That is fork evidence and must still
// reset the streak, which is what the pre-fix behaviour got right and the fix
// must not lose.
func TestIsWithinOrphanGrace_FalseWhenNoParentIsMissing(t *testing.T) {
	dag := newOrphanGraceTestDAG()
	dag.blocks["parent-a"] = &Block{Hash: "parent-a", Height: 10}
	dag.blocks["parent-b"] = &Block{Hash: "parent-b", Height: 10}

	block := &Block{Hash: "child", Height: 11, ParentHashes: []string{"parent-a", "parent-b"}}

	if dag.IsWithinOrphanGrace(block) {
		t.Fatal("a block with every parent present locally must NOT be treated as deferred — " +
			"AddPeerBlock refused it for some other reason, which is genuine fork evidence")
	}
}

// The common case during ordered paged catch-up: page N ends between a block
// and its parent, which arrives on page N+1 or via fetchMissingAncestors
// seconds later. Nothing is tracked in orphanFirstSeen yet because this is the
// first sighting. Counting this as fork evidence is precisely what pinned
// cleanSyncStreak at 0 for as long as a backlog took to drain.
func TestIsWithinOrphanGrace_TrueForAFirstSightingMissingParent(t *testing.T) {
	dag := newOrphanGraceTestDAG()
	block := &Block{Hash: "child", Height: 11, ParentHashes: []string{"parent-not-here-yet"}}

	if !dag.IsWithinOrphanGrace(block) {
		t.Fatal("a block whose missing parent has never been seen before must count as deferred, not refused")
	}
}

// Still deferred while the gap is young: the parent is being fetched and the
// grace has not run out.
func TestIsWithinOrphanGrace_TrueWhileTheGapIsYoungerThanTheGrace(t *testing.T) {
	dag := newOrphanGraceTestDAG()
	dag.orphanFirstSeen["parent-in-flight"] = time.Now().Add(-proposerBreakerOrphanGrace / 2)

	block := &Block{Hash: "child", Height: 11, ParentHashes: []string{"parent-in-flight"}}

	if !dag.IsWithinOrphanGrace(block) {
		t.Fatalf("a gap younger than proposerBreakerOrphanGrace (%s) must count as deferred", proposerBreakerOrphanGrace)
	}
}

// The property that keeps the fix from weakening what the gate exists for. A
// genuinely diverged peer serves blocks whose parents this node will never
// receive. Those parents age past the grace within one window, after which
// every further cycle resets the streak again and production stays blocked
// exactly as before. The exception forgives only gaps that actually close.
func TestIsWithinOrphanGrace_FalseOnceTheGapOutlivesTheGrace(t *testing.T) {
	dag := newOrphanGraceTestDAG()
	dag.orphanFirstSeen["parent-that-never-arrives"] = time.Now().Add(-2 * proposerBreakerOrphanGrace)

	block := &Block{Hash: "child", Height: 11, ParentHashes: []string{"parent-that-never-arrives"}}

	if dag.IsWithinOrphanGrace(block) {
		t.Fatalf("a gap older than proposerBreakerOrphanGrace (%s) must count as fork evidence, "+
			"otherwise a permanently diverged peer would be forgiven forever", proposerBreakerOrphanGrace)
	}
}

// A nil block is never "deferred" — it cannot be classified, so it must fall
// through to the strict side.
func TestIsWithinOrphanGrace_FalseForNil(t *testing.T) {
	dag := newOrphanGraceTestDAG()
	if dag.IsWithinOrphanGrace(nil) {
		t.Fatal("nil must never be reported as within orphan grace")
	}
}

// End-to-end on the gate itself, expressed the way doSyncOnce now drives it:
// cycles that only ever deferred blocks accumulate a streak and open
// production; the first genuinely refused block closes it again.
func TestCleanSyncStreak_DeferralsAccumulate_RefusalResets(t *testing.T) {
	dag := newOrphanGraceTestDAG()
	const seed = "https://primary.example"
	seeds := []string{seed}

	deferred := &Block{Hash: "deferred-child", Height: 11, ParentHashes: []string{"parent-in-flight"}}
	dag.blocks["parent-present"] = &Block{Hash: "parent-present", Height: 10}
	refused := &Block{Hash: "refused-child", Height: 11, ParentHashes: []string{"parent-present"}}

	// doSyncOnce's decision, verbatim: sawUnmergedBlocks is set only when the
	// failure is NOT within orphan grace.
	runCycle := func(unmerged *Block) {
		if unmerged != nil && !dag.IsWithinOrphanGrace(unmerged) {
			dag.resetCleanSyncStreak(seed)
			return
		}
		dag.recordCleanSyncCycle(seed)
	}

	for i := 0; i < cleanSyncStreakThreshold; i++ {
		runCycle(deferred)
	}
	if !dag.hasCaughtUpWithAllPeers(seeds) {
		t.Fatalf("%d cycles whose only unmerged blocks were deferred behind in-flight parents must reach the threshold — "+
			"this is the case that held Contabo1 for 72%% of its uptime", cleanSyncStreakThreshold)
	}

	runCycle(refused)
	if dag.hasCaughtUpWithAllPeers(seeds) {
		t.Fatal("a genuinely refused block must still reset the streak and re-close the production gate")
	}
}
