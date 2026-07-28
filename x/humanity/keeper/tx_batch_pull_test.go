package keeper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func txBatchTestState() *ChainState {
	// No DB: LoadTxBatch then answers purely from the in-memory batch cache,
	// which is all these cases need and keeps them free of a live Postgres.
	return &ChainState{txBatches: newTxBatchCache()}
}

// txsFor builds a batch large enough to be worth stripping (see
// txBatchMinTxsToStrip): below that break-even a block is deliberately sent
// whole, so a smaller fixture would test the wrong branch.
func txsFor(n int) []Transaction {
	txs := make([]Transaction, n)
	for i := range txs {
		txs[i] = Transaction{Type: "transfer", Wallet: "0xaaa", To: "0xbbb", Amount: float64(i + 1)}
	}
	return txs
}

// GetBlocksSince hands back pointers into the live DAG. Stripping must copy,
// because writing Transactions = nil through those pointers would delete the
// bodies from this node's OWN memory -- it would serve a peer correctly once
// and quietly destroy its own chain state doing it.
func TestStripBlocksForPeer_DoesNotMutateTheDAGsOwnBlocks(t *testing.T) {
	txs := txsFor(40)
	root := txBatchRoot(txs)
	cs := txBatchTestState()
	cs.txBatches.put(root, txs)

	original := &Block{Hash: "0xblock", Height: 7, TxRoot: root, Transactions: txs}
	a := &APIServer{state: cs}

	out := a.stripBlocksForPeer([]*Block{original})

	if len(original.Transactions) != 40 {
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

// The case that made the whole mechanism inert in production. chain_blocks has
// no tx_root column, so a block reloaded from the database arrives with the
// field empty -- and every block this endpoint serves is a reloaded one,
// because 100% of GetBlocksSince goes through LoadBlocksSinceFromDB. Measured
// live: blocks_stripped 0 against blocks_sent_whole 11,400.
func TestStripBlocksForPeer_DerivesTheRootForDatabaseLoadedBlocks(t *testing.T) {
	txs := txsFor(40)
	cs := txBatchTestState()
	// No TxRoot, and nothing in the batch store -- exactly a DB-loaded block.
	original := &Block{Hash: "0xblock", Height: 7, Transactions: txs}
	a := &APIServer{state: cs}

	out := a.stripBlocksForPeer([]*Block{original})

	if len(out[0].Transactions) != 0 {
		t.Fatal("a database-loaded block was sent whole; this is the case that made stripping a no-op")
	}
	want := txBatchRoot(txs)
	if out[0].TxRoot != want {
		t.Fatalf("derived TxRoot %q, want %q", out[0].TxRoot, want)
	}
	// And the body must now be retrievable, or the peer is stranded.
	got, ok := cs.LoadTxBatch(want)
	if !ok {
		t.Fatal("the body was not stored, so the peer could never complete the block it was just handed a header for")
	}
	if len(got) != 40 {
		t.Fatalf("stored body has %d transactions, want 40", len(got))
	}
}

// Below the break-even a second round trip costs more than the bytes it saves,
// so small blocks travel whole -- the same threshold the push path applies.
func TestStripBlocksForPeer_KeepsBlocksBelowTheBreakEven(t *testing.T) {
	original := &Block{Hash: "0xblock", Height: 7, Transactions: txsFor(txBatchMinTxsToStrip - 1)}
	a := &APIServer{state: txBatchTestState()}

	out := a.stripBlocksForPeer([]*Block{original})

	if len(out[0].Transactions) != txBatchMinTxsToStrip-1 {
		t.Fatal("a block below the break-even was stripped; the extra round trip costs more than it saves")
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

// Every response path of handleBlockPush must carry the capability token,
// including the failures. One of them closes a loop that cannot open itself: a
// COMPLETE block push under load is megabytes and draws "request body too
// large", the untokenised 413 demotes the peer, the demotion stops stripping,
// and the next push is another oversized complete block -- while the stripped
// push would have been a few hundred bytes and would have fit.
func TestPushErrorCarriesTheCapabilityToken(t *testing.T) {
	rec := httptest.NewRecorder()
	pushError(rec, http.StatusRequestEntityTooLarge, "request body too large")

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413", rec.Code)
	}
	var resp struct {
		OK      bool   `json:"ok"`
		Reason  string `json:"reason"`
		TxBatch string `json:"tx_batch"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("error response is not valid JSON (%q): %v", rec.Body.String(), err)
	}
	if resp.TxBatch != txBatchCapabilityToken {
		t.Fatalf("error response omits the capability token (tx_batch=%q); the sender would read that as "+
			"'peer does not understand bodies by reference' and stop stripping toward it", resp.TxBatch)
	}
	if resp.OK {
		t.Fatal("a rejection must not report ok:true")
	}
	if resp.Reason != "request body too large" {
		t.Fatalf("reason was lost: %q", resp.Reason)
	}
}
