package keeper

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// This file guards the 2026-07-24 fix for the fork that survived every
// resync: PerformResync rolls dag.height back to a trusted checkpoint that
// necessarily TRAILS the primary's tip, but every per-peer "how far am I
// with this peer" marker used to keep its pre-rollback, post-fork value. All
// three catch-up gates therefore read "fully caught up" on a node that was
// genuinely hundreds of blocks behind, and it resumed producing on a fresh
// fork within seconds — at exactly the checkpoint-lag offset, which is why
// both Contabos sat a FROZEN, constant distance behind the primary after
// every single resync.

// newResyncGateTestDAG builds the minimal DAG the per-peer sync bookkeeping
// needs, matching newGhostdagTestDAG's pattern for these focused tests.
func newResyncGateTestDAG() *BlockDAG {
	dag := newGhostdagTestDAG()
	dag.peerSyncHeight = make(map[string]int64)
	dag.cleanSyncStreak = make(map[string]int)
	dag.deepScanResumeHeight = make(map[string]int64)
	dag.deepScanFloorOverride = make(map[string]int64)
	dag.lastDeepScanAt = make(map[string]int64)
	return dag
}

// TestResetPeerSyncProgress_ClearsEveryCatchUpMarker is the core regression
// guard: after an in-process resync, none of the markers that make a gate
// read "caught up" may survive from before the rollback.
func TestResetPeerSyncProgress_ClearsEveryCatchUpMarker(t *testing.T) {
	dag := newResyncGateTestDAG()
	const seed = "https://primary.example"
	seeds := []string{seed}

	// Pre-resync state: this node had genuinely caught up with the seed on
	// what later turned out to be its own fork.
	dag.advancePeerSyncHeight(seed, 1771238)
	for i := 0; i < cleanSyncStreakThreshold; i++ {
		dag.recordCleanSyncCycle(seed)
	}
	dag.setDeepScanResumeHeight(seed, 1770500)
	dag.lowerDeepScanFloor(seed, 1770400)
	if !dag.hasCaughtUpWithAllPeers(seeds) {
		t.Fatal("setup failed: expected caught-up before the resync")
	}

	// The resync rolls the chain back to a checkpoint ~800 blocks below the
	// seed's tip. Without this call, everything above stays as it is.
	dag.resetPeerSyncProgress()

	if got := dag.getPeerSyncHeight(seed); got != 0 {
		t.Errorf("peerSyncHeight must be cleared, got %d — doSyncOnce derives its minHeight from it and would resume ABOVE the gap the resync just created", got)
	}
	if dag.hasCaughtUpWithAllPeers(seeds) {
		t.Error("cleanSyncStreak must be cleared: a streak earned before the rollback must not report the post-rollback node as caught up")
	}
	if got := dag.getDeepScanResumeHeight(seed); got != 0 {
		t.Errorf("deepScanResumeHeight must be cleared, got %d — resuming a sweep mid-walk skips the fresh gap", got)
	}
	if got := dag.effectiveDeepScanFloor(seed); got != dag.deepScanFloor() {
		t.Errorf("deepScanFloorOverride must be cleared: effective floor %d != base floor %d", got, dag.deepScanFloor())
	}
}

// TestResetPeerSyncProgress_SendsCatchUpBackThroughTheDeepScanFloor encodes
// the consequence that actually closes the gap. doSyncOnce derives its
// starting height as `getPeerSyncHeight(peer) - syncOverlap` and only falls
// back to effectiveDeepScanFloor (the checkpoint) when that is negative. A
// stale cursor therefore made the ordered catch-up start ABOVE the gap and
// never request it; a cleared one sends it through the floor instead, so it
// re-walks from the checkpoint forward.
func TestResetPeerSyncProgress_SendsCatchUpBackThroughTheDeepScanFloor(t *testing.T) {
	dag := newResyncGateTestDAG()
	const seed = "https://primary.example"
	const syncOverlap = 20 // doSyncOnce's own local constant

	dag.advancePeerSyncHeight(seed, 1771238)
	if got := dag.getPeerSyncHeight(seed) - syncOverlap; got < 0 {
		t.Fatalf("setup failed: expected a stale cursor well above zero, got %d", got)
	}

	dag.resetPeerSyncProgress()

	if got := dag.getPeerSyncHeight(seed) - syncOverlap; got >= 0 {
		t.Fatalf("after the reset doSyncOnce's minHeight must be negative (%d) so it falls through to effectiveDeepScanFloor and re-walks from the checkpoint", got)
	}
}

// TestSeedIsAbsorbed_StaleCursorSuppressesTheGate exercises the single
// comparison fetchAndSetSyncTarget uses to decide whether the initial-sync
// gate engages. It is driven through resetPeerSyncProgress rather than with
// literals so the test fails if the reset ever stops clearing the cursor
// this comparison reads. (fetchAndSetSyncTarget itself cannot be driven from
// a test: httpSyncClient's pinningDialer rejects loopback addresses by
// design, which is why the comparison is extracted.)
func TestSeedIsAbsorbed_StaleCursorSuppressesTheGate(t *testing.T) {
	const seedTip = 1770900     // the primary's real tip
	const localHeight = 1770009 // the checkpoint this node was just seeded from
	const staleCursor = 1771238 // pulled from the primary BEFORE the rollback

	dag := newResyncGateTestDAG()
	dag.height = localHeight
	const seed = "https://primary.example"
	dag.advancePeerSyncHeight(seed, staleCursor)

	if !seedIsAbsorbed(seedTip, dag.getPeerSyncHeight(seed)) {
		t.Fatal("setup assumption broken: a pre-rollback cursor above the seed's tip must read as 'absorbed' — that is the fail-open being fixed")
	}

	dag.resetPeerSyncProgress()

	if seedIsAbsorbed(seedTip, dag.getPeerSyncHeight(seed)) {
		t.Fatalf("after the reset a seed at %d must NOT read as absorbed by a node at %d — the gate would stay open and production would resume %d blocks behind it, forking there",
			seedTip, localHeight, seedTip-localHeight)
	}
}

// TestSeedIsAbsorbed_LevelWithSeedStillCountsAsAbsorbed pins the boundary in
// the other direction, guarding against the deadlock class the 2026-07-24
// sync-gate floor produced: a node exactly level with a seed has nothing to
// catch up to and must not gate itself.
func TestSeedIsAbsorbed_LevelWithSeedStillCountsAsAbsorbed(t *testing.T) {
	if !seedIsAbsorbed(1770900, 1770900) {
		t.Fatal("a node level with the seed must count as absorbed — gating there is a deadlock, not a safety margin")
	}
}

// TestArmInitialSyncGate_NoSeedsIsANoOp guards the re-arm path against
// touching the gate on a node that has no seeds to gate against at all (the
// primary: no PRIMARY_NODE_URL, so syncGateSeeds is empty). Arming a target
// there would silence the one node nothing else can replace.
func TestArmInitialSyncGate_NoSeedsIsANoOp(t *testing.T) {
	dag := newResyncGateTestDAG()
	dag.height = 1770009

	dag.armInitialSyncGate(true)

	if got := dag.syncTargetHeight.Load(); got != 0 {
		t.Fatalf("a node with no configured seeds must never arm the production gate, got target %d", got)
	}
}

// TestArmInitialSyncGate_UsesSeedsNotStaticPeers pins the distinction the
// syncGateSeeds field exists for: trustedSeeds additionally holds PEER_NODES
// static peers, whose height was never meant to gate this node's production.
// Re-arming off trustedSeeds would quietly widen the gate to them.
func TestArmInitialSyncGate_UsesSeedsNotStaticPeers(t *testing.T) {
	var staticPeerQueried bool
	staticPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		staticPeerQueried = true
		fmt.Fprint(w, `{"chain":{"height":9999999}}`)
	}))
	defer staticPeer.Close()

	dag := newResyncGateTestDAG()
	dag.height = 100
	dag.trustedSeeds = map[string]bool{staticPeer.URL: true}
	// syncGateSeeds deliberately left empty: this peer came from PEER_NODES.

	dag.armInitialSyncGate(true)

	if staticPeerQueried {
		t.Error("a PEER_NODES static peer must not be queried for the production gate's target")
	}
	if got := dag.syncTargetHeight.Load(); got != 0 {
		t.Errorf("a static peer's height must not arm the gate, got target %d", got)
	}
}
