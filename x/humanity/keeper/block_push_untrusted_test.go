package keeper

import "testing"

// TestAddPeerBlock_UnauthorizedProposerRejectedWithoutFromSyncBypass is the
// regression guard for the 2026-07-04 brutal-audit P0 finding: handleBlockPush
// (api.go) used to set block.FromSync = true unconditionally for EVERY
// incoming HTTP push, before ever checking who sent it. /api/blocks/push is
// publicly reachable — unlike a trusted seed's URL, which an operator
// explicitly configures via PRIMARY_NODE_URL/PEER_NODES — so any HTTP client
// could get an unregistered proposer's block accepted, since FromSync skips
// the authorized-validator check entirely (see AddPeerBlock's own gate).
// handleBlockPush now sets FromSync = false; this test proves what that
// buys: a genuinely valid, well-formed, correctly signed block from a
// proposer that was never registered as an authorized validator must still
// be rejected when FromSync is false, exactly as a push-path block now is.
func TestAddPeerBlock_UnauthorizedProposerRejectedWithoutFromSyncBypass(t *testing.T) {
	dag := newOrphanTestDAG()
	dag.state = &ChainState{}
	dag.bootHeight = 0
	dag.authorizedValidators = map[string]bool{} // deliberately empty — proposer is NOT registered
	dag.warnedUnknownProposers = map[string]bool{}
	blk := signTestBlockWithParent(t, 1, "deadbeef")
	blk.FromSync = false // what handleBlockPush now sets, instead of true

	if dag.AddPeerBlock(blk) {
		t.Fatal("an unauthorized proposer's block must be rejected when FromSync is false — this is exactly the protection the old unconditional FromSync=true on the push path bypassed")
	}
}

// TestAddPeerBlock_FromSyncTrueWouldHaveBypassedAuthorization documents the
// OLD bug's mechanism directly: with FromSync=true (what handleBlockPush used
// to set for every push, regardless of sender), the SAME unauthorized
// proposer's block WOULD have been accepted. This isn't asserting desired
// behavior — it's proof of what the vulnerability actually let through,
// kept as a permanent record of why the fix in api.go (block.FromSync =
// false) matters.
func TestAddPeerBlock_FromSyncTrueWouldHaveBypassedAuthorization(t *testing.T) {
	dag := newOrphanTestDAG()
	dag.state = &ChainState{}
	dag.bootHeight = 0
	dag.authorizedValidators = map[string]bool{} // deliberately empty — proposer is NOT registered
	dag.warnedUnknownProposers = map[string]bool{}
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}
	dag.replayedBlocks = map[string]bool{}
	dag.equivocationIndex = map[string]string{}
	// FIX (2026-07-10): "deadbeef" must actually exist in dag.blocks now.
	// It previously worked unresolved purely because Integrity check 3 skips
	// parent-existence verification for height==1, and computeGHOSTDAGState
	// used to silently tolerate an unresolvable SelectedParent candidate
	// instead of deferring — a real production block always builds on a
	// real, already-known ancestor (genesis or a checkpoint stub), so a
	// genuinely-dangling parent hash was never a case this test needed to
	// exercise; it only ever cared about the authorization gate below.
	dag.blocks["deadbeef"] = &Block{Hash: "deadbeef", Height: 0, IsGenesis: true}
	blk := signTestBlockWithParent(t, 1, "deadbeef")
	blk.FromSync = true // the OLD, vulnerable handleBlockPush behavior

	if !dag.AddPeerBlock(blk) {
		t.Fatal("expected FromSync=true to bypass the authorized-validator check (confirming the mechanism the api.go fix removes) — if this now fails, some other gate started rejecting unauthorized FromSync blocks and this test's premise should be revisited")
	}
}
