package keeper

import "testing"

// initCleanSyncStreak initializes the maps recordCleanSyncCycle/
// resetCleanSyncStreak/hasCaughtUpWithAllPeers need, matching the pattern
// newGhostdagTestDAG's other helpers already use for this minimal test DAG.
func initCleanSyncStreak(dag *BlockDAG) {
	dag.cleanSyncStreak = make(map[string]int)
}

// TestHasCaughtUpWithAllPeers_RequiresThresholdConsecutiveCleanCycles is the
// regression guard for the 2026-07-10 fix: Contabo1 forked within its first
// 30-45 blocks of every RESYNC_FROM_SNAPSHOT boot because every earlier
// height-derived gate (dag.height vs a target, then peerSyncHeight vs a
// target) could read "caught up" while real blocks from a peer were still
// being fetched but rejected as orphans — the live signature of an active
// fork in progress, indistinguishable from genuine catch-up by height alone.
// hasCaughtUpWithAllPeers must require cleanSyncStreakThreshold CONSECUTIVE
// clean cycles per seed, not just one.
func TestHasCaughtUpWithAllPeers_RequiresThresholdConsecutiveCleanCycles(t *testing.T) {
	dag := newGhostdagTestDAG()
	initCleanSyncStreak(dag)
	seeds := []string{"https://primary.example"}

	if dag.hasCaughtUpWithAllPeers(seeds) {
		t.Fatal("must not report caught up before any sync cycle has run")
	}
	for i := 0; i < cleanSyncStreakThreshold-1; i++ {
		dag.recordCleanSyncCycle("https://primary.example")
	}
	if dag.hasCaughtUpWithAllPeers(seeds) {
		t.Fatalf("must not report caught up with only %d of %d required consecutive clean cycles", cleanSyncStreakThreshold-1, cleanSyncStreakThreshold)
	}
	dag.recordCleanSyncCycle("https://primary.example")
	if !dag.hasCaughtUpWithAllPeers(seeds) {
		t.Fatalf("must report caught up once %d consecutive clean cycles are recorded", cleanSyncStreakThreshold)
	}
}

// TestHasCaughtUpWithAllPeers_UnmergedBlockResetsStreak is the core
// regression guard: a page that contained a genuinely new block this node
// could NOT attach (the exact live signature of an active fork — real data
// exists on the peer that this node cannot currently place) must reset the
// streak to zero, even after it had already climbed close to the threshold.
func TestHasCaughtUpWithAllPeers_UnmergedBlockResetsStreak(t *testing.T) {
	dag := newGhostdagTestDAG()
	initCleanSyncStreak(dag)
	seeds := []string{"https://primary.example"}

	for i := 0; i < cleanSyncStreakThreshold; i++ {
		dag.recordCleanSyncCycle("https://primary.example")
	}
	if !dag.hasCaughtUpWithAllPeers(seeds) {
		t.Fatal("setup failed: expected caught up after threshold clean cycles")
	}

	dag.resetCleanSyncStreak("https://primary.example") // doSyncOnce's own call when sawUnmergedBlocks==true
	if dag.hasCaughtUpWithAllPeers(seeds) {
		t.Fatal("a reset (unmerged block seen) must immediately un-set caught-up status, not just decrement it")
	}
}

// TestHasCaughtUpWithAllPeers_RequiresEverySeed guards against a multi-seed
// node (PRIMARY_NODE_URL plus PRIMARY_NODE_URLS) considering itself caught
// up just because ONE seed's streak reached the threshold while another —
// e.g. one that was temporarily unreachable, so its streak never advanced
// at all — has not.
func TestHasCaughtUpWithAllPeers_RequiresEverySeed(t *testing.T) {
	dag := newGhostdagTestDAG()
	initCleanSyncStreak(dag)
	seeds := []string{"https://primary.example", "https://secondary.example"}

	for i := 0; i < cleanSyncStreakThreshold; i++ {
		dag.recordCleanSyncCycle("https://primary.example")
	}
	if dag.hasCaughtUpWithAllPeers(seeds) {
		t.Fatal("must not report caught up when a second configured seed has zero clean cycles")
	}
	for i := 0; i < cleanSyncStreakThreshold; i++ {
		dag.recordCleanSyncCycle("https://secondary.example")
	}
	if !dag.hasCaughtUpWithAllPeers(seeds) {
		t.Fatal("must report caught up once every configured seed independently reaches the threshold")
	}
}

// TestHasCaughtUpWithAllPeers_EmptySeedListIsNotCaughtUp guards the vacuous-
// truth trap: no configured/known seeds must never read as "caught up" —
// it means discovery hasn't run yet, not that there's nothing to catch up
// with.
func TestHasCaughtUpWithAllPeers_EmptySeedListIsNotCaughtUp(t *testing.T) {
	dag := newGhostdagTestDAG()
	initCleanSyncStreak(dag)
	if dag.hasCaughtUpWithAllPeers(nil) {
		t.Fatal("an empty seed list must never report caught up")
	}
}
