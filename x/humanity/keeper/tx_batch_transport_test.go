package keeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTxBatchTestDAG builds the minimal DAG the by-reference transport needs:
// a ChainState to store and look up bodies, plus the peer set that doubles as
// the fetch allowlist.
func newTxBatchTestDAG(peers ...string) *BlockDAG {
	dag := newGhostdagTestDAG()
	dag.state = newTestState()
	dag.state.txBatches = newTxBatchCache()
	dag.activeSyncPeers = make(map[string]bool, len(peers))
	for _, p := range peers {
		dag.activeSyncPeers[p] = true
	}
	return dag
}

// useLocalTxBatchClient points body fetches at a plain client for the
// duration of one test. The production client deliberately refuses loopback
// addresses, so an httptest server is unreachable through it — which is the
// correct behaviour, pinned separately by
// TestFetchTxBatch_ProductionClientRefusesLoopbackAddresses.
func useLocalTxBatchClient(t *testing.T) func() {
	t.Helper()
	previous := txBatchClient
	txBatchClient = &http.Client{Timeout: 5 * time.Second}
	return func() { txBatchClient = previous }
}

// The anti-SSRF property the transport inherits from httpSyncClient: even if
// a peer URL somehow named an internal address, the connection itself is
// refused. This is a second, independent layer under
// resolveTxBatchSources' allowlist, and it must not be weakened by the
// indirection that lets tests substitute a local client.
func TestFetchTxBatch_ProductionClientRefusesLoopbackAddresses(t *testing.T) {
	if txBatchClient != httpSyncClient {
		t.Fatal("body fetches must go through httpSyncClient by default — it pins DNS and refuses private, loopback and link-local addresses")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the guarded client must never actually reach a loopback address")
	}))
	defer srv.Close()

	dag := newTxBatchTestDAG(srv.URL)
	if _, err := dag.FetchTxBatch(txBatchRoot(manyTxs(3)), srv.URL); err == nil {
		t.Fatal("fetching from a loopback address must be refused by the client itself")
	}
}

func manyTxs(n int) []Transaction {
	txs := make([]Transaction, n)
	for i := range txs {
		txs[i] = Transaction{Type: "transfer", Wallet: "0xa", To: "0xb", Amount: 1, TxHash: fmt.Sprintf("0x%d", i)}
	}
	return txs
}

// The property the whole scheme rests on: what actually goes over the wire
// carries no transactions, yet a receiver decoding it computes the same hash
// the producer signed. If this ever stops holding, stripped blocks fail
// AddPeerBlock's hash check and the push path silently stops merging.
func TestStrippedBlockPayload_TravelsWithoutBodyAndHashesIdentically(t *testing.T) {
	txs := manyTxs(txBatchMinTxsToStrip)
	block := &Block{Height: 12, Timestamp: 5, ParentHashes: []string{"p"}, Proposer: "0xp", Transactions: txs}
	block.TxRoot = txBatchRoot(txs)
	signedHash := calculateBlockHash(block)

	payload, ok := strippedBlockPayload(block)
	if !ok {
		t.Fatal("a block with a full body must be strippable")
	}

	var received Block
	if err := json.Unmarshal(payload, &received); err != nil {
		t.Fatalf("stripped payload must still decode as a block: %v", err)
	}
	if len(received.Transactions) != 0 {
		t.Fatalf("the payload must carry no transactions, got %d", len(received.Transactions))
	}
	if received.TxRoot != block.TxRoot {
		t.Fatal("the payload must carry tx_root, or the receiver cannot know which body to ask for")
	}
	if got := calculateBlockHash(&received); got != signedHash {
		t.Fatalf("a stripped block must hash to the value the producer signed —\n  signed:   %s\n  received: %s\nOtherwise every stripped block fails AddPeerBlock's hash check.", signedHash, got)
	}
	if len(payload) >= 2000 {
		t.Fatalf("the whole point is a small payload that fits inside the 3 s push timeout; got %d bytes", len(payload))
	}

	// The original must be untouched — it is still broadcast in full to peers
	// that do not support the scheme, and it is this node's own stored block.
	if len(block.Transactions) != txBatchMinTxsToStrip {
		t.Fatal("stripping must not mutate the caller's block")
	}
}

// Stripping a small block would make propagation slower, not faster: the
// extra round trip costs more than the bytes saved.
func TestStrippedBlockPayload_LeavesSmallAndBodylessBlocksAlone(t *testing.T) {
	small := manyTxs(txBatchMinTxsToStrip - 1)
	if _, ok := strippedBlockPayload(&Block{Height: 1, Transactions: small, TxRoot: txBatchRoot(small)}); ok {
		t.Fatal("a block below the break-even size must be sent inline")
	}
	if _, ok := strippedBlockPayload(&Block{Height: 1}); ok {
		t.Fatal("an empty block has nothing to strip")
	}
	// A block from before tx_root existed has no digest to reference, so its
	// body cannot be requested and must never be removed.
	if _, ok := strippedBlockPayload(&Block{Height: 1, Transactions: manyTxs(100)}); ok {
		t.Fatal("a block carrying no tx_root must never be stripped — nothing could ask for its body")
	}
}

// The safe default. An unknown peer gets complete blocks, exactly as before
// this existed, so rolling this out cannot break a peer that has not been
// upgraded yet.
func TestTxBatchCapability_UnknownPeerIsNeverStrippedTo(t *testing.T) {
	const peer = "https://unknown-peer.example"
	txBatchPeerCap.Delete(peer)
	if txBatchPeerSupports(peer) {
		t.Fatal("a peer we know nothing about must be assumed NOT to support bodies by reference")
	}
}

// A peer redeployed onto older code stops advertising the capability. Because
// it is re-learned from every response, the sender must go back to complete
// blocks immediately rather than waiting for a timeout.
func TestTxBatchCapability_DowngradeIsLearnedFromTheNextResponse(t *testing.T) {
	const peer = "https://downgrading-peer.example"
	t.Cleanup(func() { txBatchPeerCap.Delete(peer) })

	recordTxBatchCapability(peer, true)
	if !txBatchPeerSupports(peer) {
		t.Fatal("a peer advertising the capability must be recorded as supporting it")
	}
	recordTxBatchCapability(peer, false) // its next response carries no token
	if txBatchPeerSupports(peer) {
		t.Fatal("a peer that stopped advertising the capability must immediately go back to receiving complete blocks")
	}
}

// SECURITY. The sender names itself in a header so the receiver knows which
// peer to ask. That header is attacker-controlled, so it must only ever be
// able to REORDER peers this node already contacts — never add one. Without
// this, a peer could point the fetch at a cloud metadata endpoint or any
// other internal address and turn every pushed block into an SSRF primitive.
func TestResolveTxBatchSources_HeaderCanReorderButNeverAddADestination(t *testing.T) {
	known := []string{"https://a.example", "https://b.example"}
	dag := newTxBatchTestDAG(known...)

	for _, hostile := range []string{
		"http://169.254.169.254", // cloud instance metadata
		"http://127.0.0.1:5432",  // the node's own database
		"http://10.0.0.5",
		"https://attacker.example",
	} {
		got := dag.resolveTxBatchSources(hostile)
		if len(got) != len(known) {
			t.Fatalf("a hostile source hint must not change the candidate set: %q yielded %v", hostile, got)
		}
		for _, g := range got {
			if !dag.activeSyncPeers[g] {
				t.Fatalf("resolveTxBatchSources returned %q, which is not a peer this node already talks to — hint %q created a new outbound destination", g, hostile)
			}
		}
	}

	// A legitimate hint is honoured: the peer that pushed the block is the one
	// most likely to have the body, so it is tried first.
	got := dag.resolveTxBatchSources("https://b.example/")
	if len(got) != 2 || got[0] != "https://b.example" {
		t.Fatalf("a hint naming a known peer must put it first, got %v", got)
	}
}

// With no peers registered there is nowhere legitimate to fetch from, and the
// answer must be a clean error rather than any kind of fallback guess.
func TestFetchTxBatch_WithNoKnownPeersFailsInsteadOfGuessing(t *testing.T) {
	dag := newTxBatchTestDAG()
	if _, err := dag.FetchTxBatch(txBatchRoot(manyTxs(3)), "https://somewhere.example"); err == nil {
		t.Fatal("with no known sync peers the fetch must fail, not reach out to the hinted URL")
	}
}

// A malformed root must never reach the network at all.
func TestFetchTxBatch_RejectsMalformedRootBeforeAnyRequest(t *testing.T) {
	dag := newTxBatchTestDAG("https://a.example")
	for _, bad := range []string{"", "not-a-digest", "../../etc/passwd", "ABCDEF"} {
		if _, err := dag.FetchTxBatch(bad, ""); err == nil {
			t.Fatalf("a malformed root %q must be rejected before any request is made", bad)
		}
	}
}

// A peer serving a body that does not hash to the requested root is serving
// transactions the producer never signed. It must be refused, not returned to
// the caller and certainly not attached to the block.
func TestFetchTxBatch_RefusesABodyThatDoesNotMatchTheRequestedRoot(t *testing.T) {
	genuine := manyTxs(4)
	root := txBatchRoot(genuine)

	defer useLocalTxBatchClient(t)()

	hostile := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"root": r.URL.Query().Get("root"), // echoes the request, but the body is not it
			"txs":  []Transaction{{Type: "transfer", Wallet: "0xvictim", To: "0xattacker", Amount: 1e9, TxHash: "0x0"}},
		})
	}))
	defer hostile.Close()

	dag := newTxBatchTestDAG(hostile.URL)
	if txs, err := dag.FetchTxBatch(root, hostile.URL); err == nil {
		t.Fatalf("a body that does not hash to the requested root must be refused, got %d transactions", len(txs))
	}
}

// The happy path, end to end over real HTTP: a peer stores a body, and
// another node fetches it by digest and gets exactly those transactions back.
func TestFetchTxBatch_RetrievesAVerifiedBodyOverHTTP(t *testing.T) {
	defer useLocalTxBatchClient(t)()

	genuine := manyTxs(5)
	root := txBatchRoot(genuine)

	server := newTxBatchTestDAG()
	if err := server.state.SaveTxBatch(root, genuine); err != nil {
		t.Fatalf("storing the body must succeed: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc((&APIServer{blockchain: server}).handleTxBatch))
	defer srv.Close()

	client := newTxBatchTestDAG(srv.URL)
	txs, err := client.FetchTxBatch(root, srv.URL)
	if err != nil {
		t.Fatalf("fetching a stored body must succeed: %v", err)
	}
	if txBatchRoot(txs) != root {
		t.Fatal("the fetched body must hash to the requested root")
	}
	if len(txs) != len(genuine) {
		t.Fatalf("expected %d transactions, got %d", len(genuine), len(txs))
	}
}

// The capability handshake. A node running older code has no route here, and
// its catch-all serves the explorer page with a 200 — so support is proven by
// the token in the body, never by the status code.
func TestHandleTxBatch_AdvertisesCapabilityWhenAskedWithoutARoot(t *testing.T) {
	dag := newTxBatchTestDAG()
	rec := httptest.NewRecorder()
	(&APIServer{blockchain: dag}).handleTxBatch(rec, httptest.NewRequest(http.MethodGet, "/api/txbatch", nil))

	var resp struct {
		Capability string `json:"capability"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("the capability response must be JSON: %v", err)
	}
	if resp.Capability != txBatchCapabilityToken {
		t.Fatalf("expected capability %q, got %q", txBatchCapabilityToken, resp.Capability)
	}
}

// An unknown digest must 404 rather than answer with an empty body: an empty
// answer that passed verification would let a block replay as if it had no
// transactions, silently dropping every transfer it contains.
func TestHandleTxBatch_UnknownDigestIsNotFoundRatherThanEmpty(t *testing.T) {
	dag := newTxBatchTestDAG()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/txbatch?root="+txBatchRoot(manyTxs(2)), nil)
	(&APIServer{blockchain: dag}).handleTxBatch(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("an unknown digest must return 404, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// THE FAIL-SOFT GUARANTEE, through the real push handler.
//
// A block arrives stripped and the body cannot be obtained. The one thing
// that must never happen is accepting it anyway: replaying a block without
// its transactions would apply it as empty and silently drop every transfer
// it contains. Instead the block is rejected and the sender is told to resend
// it complete, which HTTPBroadcastBlock does immediately.
func TestHandleBlockPush_UnreachableBodyAsksForAResendInsteadOfAcceptingAnEmptyBlock(t *testing.T) {
	txs := manyTxs(64)
	// No peers registered, so there is nowhere to fetch the body from.
	dag := newTxBatchTestDAG()

	stripped := &Block{Height: 77, Timestamp: 3, ParentHashes: []string{"p"}, Proposer: "0xp", TxRoot: txBatchRoot(txs)}
	stripped.Hash = calculateBlockHash(stripped)
	payload, err := json.Marshal(stripped)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/blocks/push", bytes.NewReader(payload))
	req.Header.Set(txBatchSourceHeader, "https://not-a-known-peer.example")
	(&APIServer{blockchain: dag}).handleBlockPush(rec, req)

	var resp struct {
		OK     bool   `json:"ok"`
		Action string `json:"action"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("push response must be JSON, got %q: %v", rec.Body.String(), err)
	}
	if resp.OK {
		t.Fatal("a block whose body could not be obtained must NEVER be accepted — it would replay as empty and drop every transfer it carries")
	}
	if resp.Action != "resend_full" {
		t.Fatalf("the sender must be told to resend the complete block, got action %q", resp.Action)
	}
	if _, known := dag.blocks[stripped.Hash]; known {
		t.Fatal("the block must not have entered the DAG without its body")
	}
}

// Every push response must advertise the capability, so the sender learns it
// from traffic it is already exchanging and needs no probe request.
func TestHandleBlockPush_EveryResponseCarriesTheCapabilityToken(t *testing.T) {
	dag := newTxBatchTestDAG()
	block := &Block{Height: 5, Timestamp: 1, ParentHashes: []string{"p"}, Proposer: "0xp"}
	block.Hash = calculateBlockHash(block)
	dag.blocks[block.Hash] = block // already known: the cheap idempotent path

	payload, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rec := httptest.NewRecorder()
	(&APIServer{blockchain: dag}).handleBlockPush(rec, httptest.NewRequest(http.MethodPost, "/api/blocks/push", bytes.NewReader(payload)))

	var resp blockPushResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("push response must be JSON, got %q: %v", rec.Body.String(), err)
	}
	if resp.TxBatch != txBatchCapabilityToken {
		t.Fatalf("every push response must carry the capability token, got %q", resp.TxBatch)
	}
}

// Only a well-formed digest may reach the lookup.
func TestHandleTxBatch_RejectsAMalformedRoot(t *testing.T) {
	dag := newTxBatchTestDAG()
	rec := httptest.NewRecorder()
	(&APIServer{blockchain: dag}).handleTxBatch(rec, httptest.NewRequest(http.MethodGet, "/api/txbatch?root=%27+OR+1%3D1", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a malformed root must be rejected with 400, got %d", rec.Code)
	}
}
