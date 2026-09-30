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

// FromSync entbindet seit dem Stichtag (30.09.2026, Audit H-1) nur noch
// Bloecke der alten Geschichte von der Produzentenpruefung. Bis dahin hielt
// dieser Test fest, dass FromSync=true einen nicht zugelassenen Produzenten
// durchliess -- genau das, was der Seed-Abruf ueber HTTP fuer jeden auf dem
// Weg offen liess. Jetzt: ein aktueller Block wird trotz FromSync
// abgewiesen, ein Block von vor dem Stichtag weiter angenommen.
func TestAddPeerBlock_FromSyncNurFuerAlteGeschichte(t *testing.T) {
	neu := func() *BlockDAG {
		dag := newOrphanTestDAG()
		dag.state = &ChainState{}
		dag.bootHeight = 0
		dag.authorizedValidators = map[string]bool{} // Produzent NICHT zugelassen
		dag.warnedUnknownProposers = map[string]bool{}
		dag.stateRootMismatches = map[string]int{}
		dag.stateRootMismatchLastAt = map[string]int64{}
		dag.replayedBlocks = map[string]bool{}
		dag.equivocationIndex = map[string]string{}
		dag.blocks["deadbeef"] = &Block{Hash: "deadbeef", Height: 0, IsGenesis: true}
		return dag
	}

	aktuell := signTestBlockWithParent(t, 1, "deadbeef")
	aktuell.FromSync = true
	if neu().AddPeerBlock(aktuell) {
		t.Fatal("ein aktueller Block eines nicht zugelassenen Produzenten darf auch mit FromSync nicht durchkommen")
	}

	alt := signTestBlockWithZeit(t, 1, "deadbeef", zeitstempelPruefungAbUnix-86400)
	alt.FromSync = true
	if !neu().AddPeerBlock(alt) {
		t.Fatal("ein Block von vor dem Stichtag muss vom Seed weiter nachladbar sein")
	}
}
