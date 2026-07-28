package keeper

import "testing"

func txBatchTestState() *ChainState {
	// No DB: LoadTxBatch then answers purely from the in-memory batch cache,
	// which is all these cases need and keeps them free of a live Postgres.
	return &ChainState{txBatches: newTxBatchCache()}
}

// GetBlocksSince hands back pointers into the live DAG. Stripping must copy,
// because writing Transactions = nil through those pointers would delete the
// bodies from this node's OWN memory -- it would serve a peer correctly once
// and quietly destroy its own chain state doing it.
func TestStripBlocksForPeer_DoesNotMutateTheDAGsOwnBlocks(t *testing.T) {
	txs := []Transaction{{Type: "transfer", Wallet: "0xaaa", To: "0xbbb", Amount: 5}}
	root := txBatchRoot(txs)
	cs := txBatchTestState()
	cs.txBatches.put(root, txs)

	original := &Block{Hash: "0xblock", Height: 7, TxRoot: root, Transactions: txs}
	a := &APIServer{state: cs}

	out := a.stripBlocksForPeer([]*Block{original})

	if len(original.Transactions) != 1 {
		t.Fatalf("the DAG's own block lost its transactions (%d left) -- stripping mutated shared state",
			len(original.Transactions))
	}
	if out[0] == original {
		t.Fatal("stripped block is the same pointer as the DAG's block; any change to it is a change to live state")
	}
	if len(out[0].Transactions) != 0 {
		t.Fatalf("block was not stripped: %d transactions still attached", len(out[0].Transactions))
	}
	if out[0].TxRoot != root {
		t.Fatalf("stripped block carries TxRoot %q, want %q -- without it the peer cannot fetch the body", out[0].TxRoot, root)
	}
}

// A body this node cannot serve must not be stripped. A peer receiving such a
// header could never complete the block, and because calculateBlockHash falls
// back to the carried TxRoot when the transaction list is empty, the block
// would still hash correctly -- so the peer would apply it as though it had no
// transactions rather than failing loudly.
func TestStripBlocksForPeer_KeepsBodiesItCannotServe(t *testing.T) {
	txs := []Transaction{{Type: "transfer", Wallet: "0xaaa", To: "0xbbb", Amount: 5}}
	// Deliberately NOT stored in the batch cache.
	original := &Block{Hash: "0xblock", Height: 7, TxRoot: txBatchRoot(txs), Transactions: txs}
	a := &APIServer{state: txBatchTestState()}

	out := a.stripBlocksForPeer([]*Block{original})

	if len(out[0].Transactions) != 1 {
		t.Fatal("block was stripped even though its body is not retrievable from this node; the peer would be stranded")
	}
}

// A block with no TxRoot predates the commitment scheme. There is nothing for a
// peer to fetch the body by, so it has to travel whole.
func TestStripBlocksForPeer_KeepsBlocksWithoutATxRoot(t *testing.T) {
	original := &Block{
		Hash:         "0xblock",
		Height:       7,
		Transactions: []Transaction{{Type: "transfer", Wallet: "0xaaa", To: "0xbbb", Amount: 5}},
	}
	a := &APIServer{state: txBatchTestState()}

	out := a.stripBlocksForPeer([]*Block{original})

	if len(out[0].Transactions) != 1 {
		t.Fatal("a block with no TxRoot was stripped; nothing could ever reattach its body")
	}
}

// Empty blocks and nils must survive the pass untouched rather than panicking
// it -- this runs on every page a syncing peer requests.
func TestStripBlocksForPeer_HandlesEmptyAndNilBlocks(t *testing.T) {
	a := &APIServer{state: txBatchTestState()}
	out := a.stripBlocksForPeer([]*Block{nil, {Hash: "0xempty", Height: 1}})
	if len(out) != 2 {
		t.Fatalf("page length changed from 2 to %d", len(out))
	}
	if out[0] != nil {
		t.Fatal("a nil entry was replaced")
	}
}

// The push path learns a peer's capability only from a push RESPONSE, and a
// push that times out has none. That produced a bootstrap deadlock measured on
// the live network: pushes time out because blocks are large, blocks stay large
// because stripping is off, and stripping stays off because it needs a
// successful push. Ten minutes of production traffic showed zero stripped
// pushes and a steady trickle of push timeouts to both peers.
//
// Serving a stripped block is independent evidence, so the pull path records it.
func TestServedStrippedBlockProvesPushCapability(t *testing.T) {
	const peer = "http://peer.example:8080"
	txBatchPeerCap.Delete(peer)
	if txBatchPeerSupports(peer) {
		t.Fatal("precondition: an unknown peer must not be considered capable")
	}
	recordTxBatchCapability(peer, true)
	if !txBatchPeerSupports(peer) {
		t.Fatal("a peer that served a stripped block must become eligible for stripped pushes, or the deadlock persists")
	}
	// And the retraction path still works, so a peer rolled back to older code
	// stops being stripped to.
	recordTxBatchCapability(peer, false)
	if txBatchPeerSupports(peer) {
		t.Fatal("a peer that stopped advertising support must stop being stripped to")
	}
	txBatchPeerCap.Delete(peer)
}
