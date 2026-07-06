package keeper

import "testing"

// Regression tests for peerSyncHeight (definitive root cause of the
// 2026-07-06 3-node non-merging incident): doSyncOnce's normal windowed sync
// used to anchor minHeight on dag.Height() (highest height from ANY source,
// including this node's own continuous self-production), which races ahead
// of real per-peer catch-up progress. peerSyncHeight tracks progress against
// one specific peer, immune to this node's own production rate.
func newPeerSyncTestDAG() *BlockDAG {
	return &BlockDAG{peerSyncHeight: make(map[string]int64)}
}

func TestPeerSyncHeight_StartsAtZeroForUnknownPeer(t *testing.T) {
	dag := newPeerSyncTestDAG()
	if got := dag.getPeerSyncHeight("https://peer.example"); got != 0 {
		t.Fatalf("expected 0 for a never-synced peer, got %d", got)
	}
}

func TestPeerSyncHeight_AdvancesOnHigherHeight(t *testing.T) {
	dag := newPeerSyncTestDAG()
	dag.advancePeerSyncHeight("https://peer.example", 100)
	if got := dag.getPeerSyncHeight("https://peer.example"); got != 100 {
		t.Fatalf("expected 100, got %d", got)
	}
	dag.advancePeerSyncHeight("https://peer.example", 250)
	if got := dag.getPeerSyncHeight("https://peer.example"); got != 250 {
		t.Fatalf("expected 250 after advancing, got %d", got)
	}
}

func TestPeerSyncHeight_NeverRegresses(t *testing.T) {
	dag := newPeerSyncTestDAG()
	dag.advancePeerSyncHeight("https://peer.example", 500)
	dag.advancePeerSyncHeight("https://peer.example", 300) // a smaller/older page arriving later
	if got := dag.getPeerSyncHeight("https://peer.example"); got != 500 {
		t.Fatalf("peerSyncHeight must never regress, expected 500, got %d", got)
	}
}

func TestPeerSyncHeight_IndependentPerPeer(t *testing.T) {
	dag := newPeerSyncTestDAG()
	dag.advancePeerSyncHeight("https://primary.example", 10000)
	dag.advancePeerSyncHeight("https://secondary.example", 50)
	if got := dag.getPeerSyncHeight("https://primary.example"); got != 10000 {
		t.Fatalf("primary's cursor must be unaffected by secondary's, got %d", got)
	}
	if got := dag.getPeerSyncHeight("https://secondary.example"); got != 50 {
		t.Fatalf("secondary's cursor must be independent, got %d", got)
	}
}
