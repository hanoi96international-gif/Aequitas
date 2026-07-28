package keeper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
	txs := txsForStrip(40)
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

// A body this node cannot serve must not be stripped. A peer receiving such a
// header could never complete the block, and because calculateBlockHash falls
// back to the carried TxRoot when the transaction list is empty, the block
// would still hash correctly -- so the peer would apply it as though it had no
// transactions rather than failing loudly.
func TestStripBlocksForPeer_KeepsBodiesItCannotServe(t *testing.T) {
	txs := txsForStrip(40)
	// Deliberately NOT stored in the batch cache.
	original := &Block{Hash: "0xblock", Height: 7, TxRoot: txBatchRoot(txs), Transactions: txs}
	a := &APIServer{state: txBatchTestState()}

	out := a.stripBlocksForPeer([]*Block{original})

	if len(out[0].Transactions) != 40 {
		t.Fatal("block was stripped even though its body is not retrievable from this node; the peer would be stranded")
	}
}

// A block with no TxRoot predates the commitment scheme. There is nothing for a
// peer to fetch the body by, so it has to travel whole.
func TestStripBlocksForPeer_KeepsBlocksWithoutATxRoot(t *testing.T) {
	original := &Block{
		Hash:         "0xblock",
		Height:       7,
		Transactions: txsForStrip(40),
	}
	a := &APIServer{state: txBatchTestState()}

	out := a.stripBlocksForPeer([]*Block{original})

	if len(out[0].Transactions) != 40 {
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

// The serving path must never write. An earlier version stored the body here so
// that more blocks became strippable, which put a write proportional to the
// served payload onto a read path: chain_tx_batches grew by 851MB in minutes and
// the node stopped advancing entirely (+0 blocks in 100s) until it was reverted.
//
// Asserted by serving a block whose body is NOT in the store: it must go out
// whole and the store must still not contain it afterwards.
func TestStripBlocksForPeer_NeverWritesWhileServing(t *testing.T) {
	txs := txsForStrip(40)
	root := txBatchRoot(txs)
	cs := txBatchTestState()
	original := &Block{Hash: "0xblock", Height: 7, TxRoot: root, Transactions: txs}
	a := &APIServer{state: cs}

	out := a.stripBlocksForPeer([]*Block{original})

	if len(out[0].Transactions) != 40 {
		t.Fatal("a block whose body is not stored was stripped; the peer could never complete it")
	}
	if _, ok := cs.LoadTxBatch(root); ok {
		t.Fatal("serving stored the body — this is the write-on-a-read-path that took the node down")
	}
}

// Below the break-even a second round trip costs more than the bytes it saves,
// so small blocks travel whole -- the same threshold the push path applies.
func TestStripBlocksForPeer_KeepsBlocksBelowTheBreakEven(t *testing.T) {
	txs := txsForStrip(txBatchMinTxsToStrip - 1)
	cs := txBatchTestState()
	root := txBatchRoot(txs)
	cs.txBatches.put(root, txs)
	original := &Block{Hash: "0xblock", Height: 7, TxRoot: root, Transactions: txs}
	a := &APIServer{state: cs}

	out := a.stripBlocksForPeer([]*Block{original})

	if len(out[0].Transactions) != txBatchMinTxsToStrip-1 {
		t.Fatal("a block below the break-even was stripped; the extra round trip costs more than it saves")
	}
}

func txsForStrip(n int) []Transaction {
	txs := make([]Transaction, n)
	for i := range txs {
		txs[i] = Transaction{Type: "transfer", Wallet: "0xaaa", To: "0xbbb", Amount: float64(i + 1)}
	}
	return txs
}

// queueTxBatchStore runs while the caller holds dag.mu, so it must never block
// and never touch the database itself. A full queue must drop rather than wait:
// the cost of dropping is one block travelling whole, which is exactly the
// behaviour that existed before any of this.
func TestQueueTxBatchStore_NeverBlocksWhenTheQueueIsFull(t *testing.T) {
	cs := txBatchTestState()
	txs := txsForStrip(40)
	block := &Block{Hash: "0xb", Height: 1, TxRoot: txBatchRoot(txs), Transactions: txs}

	before := txBatchStoreDropped.Load()
	// Far more than the queue can hold, with no worker draining it fast enough.
	done := make(chan struct{})
	go func() {
		for i := 0; i < cap(txBatchStoreQueue)*3; i++ {
			queueTxBatchStore(cs, block)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("queueTxBatchStore blocked; a caller holding dag.mu would stall the whole node")
	}
	if txBatchStoreDropped.Load() == before {
		t.Skip("worker drained everything; the non-blocking path was not exercised")
	}
}

// Below the break-even there is nothing to gain from storing the body, so the
// queue must not carry it -- otherwise every tiny block costs a database write
// for a saving that would never be taken.
func TestQueueTxBatchStore_IgnoresBlocksBelowTheBreakEven(t *testing.T) {
	cs := txBatchTestState()
	txs := txsForStrip(txBatchMinTxsToStrip - 1)
	block := &Block{Hash: "0xb", Height: 1, TxRoot: txBatchRoot(txs), Transactions: txs}

	before := len(txBatchStoreQueue)
	queueTxBatchStore(cs, block)
	if len(txBatchStoreQueue) != before {
		t.Fatal("a block below the break-even was queued for storage; it can never be stripped, so the write is pure cost")
	}
}
