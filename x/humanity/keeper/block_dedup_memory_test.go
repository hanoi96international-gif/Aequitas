package keeper

import "testing"

// TestHasBlockInMemory_KnownAndUnknown is the regression guard for the
// 2026-07-05 redundant-delivery cleanup: p2p.go's handleBlockStream and
// api.go's handleBlockPush both called AddPeerBlock unconditionally for
// EVERY delivery of a block, even when the exact same block had already
// arrived moments earlier via a different channel (P2P direct + gossip
// relay + HTTP push all deliver the same live block independently).
// Confirmed live via recordRawArrivalLatency: raw arrival counts ran 2-4x
// the real block-production rate. hasBlockInMemory is the cheap check that
// lets those two entry points skip the redundant call entirely.
func TestHasBlockInMemory_KnownAndUnknown(t *testing.T) {
	dag := newOrphanTestDAG()
	dag.blocks["known-hash"] = &Block{Hash: "known-hash", Height: 5}

	if !dag.hasBlockInMemory("known-hash") {
		t.Fatal("hasBlockInMemory(known-hash) = false, want true")
	}
	if dag.hasBlockInMemory("never-seen-hash") {
		t.Fatal("hasBlockInMemory(never-seen-hash) = true, want false")
	}
}
