package keeper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Transport half of roadmap step 4 — see tx_batch.go for the commitment
// scheme and why it needs no activation height.
//
// WHAT THIS ACTUALLY BUYS, in this codebase's own numbers:
//
// HTTPBroadcastBlock (sync_blocks.go) gives each push a THREE SECOND context
// timeout, and the push path is the primary relay whenever port 4001 is
// firewalled — which is the normal case on Railway. A full block at
// maxTxsPerBlock is megabytes of JSON (SCALING_ARCHITECTURE.md measured
// 11.59 MB at 50,000 transactions). Under load that payload does not reliably
// finish inside 3 s, the push fails, and the peer never adopts the tip — so
// its next ProduceBlock cannot reference the block as a parent and no merge
// happens. That is the shape of the merge failures seen repeatedly under
// load, and it gets WORSE exactly as throughput rises, which is the opposite
// of what the 50k TPS goal needs.
//
// A stripped block is a few hundred bytes no matter how many transactions it
// carries. The push always fits in 3 s, the tip is adopted immediately, and
// the body is fetched on a separate request with its own, far more generous
// timeout — off the critical path that merging depends on.
//
// THREE PROPERTIES MAKE THIS SAFE TO RUN AGAINST A LIVE NETWORK:
//
//  1. Never strip toward a peer that has not proven it understands the
//     scheme. Capability is learned from the push responses this node is
//     already exchanging (no probe traffic), and it is re-learned on EVERY
//     response, so a peer redeployed onto older code stops being stripped to
//     after a single block.
//
//  2. Fail soft, always. If a receiver cannot obtain the body, it rejects the
//     block with action:"resend_full" and the sender immediately re-pushes
//     the complete block. A block is never accepted without its body, and a
//     failed fetch costs one extra round trip, never a lost transaction.
//
//  3. Never introduce a new outbound destination. Bodies are fetched ONLY
//     from URLs this node already polls on its own sync loop (activeSyncPeers).
//     The sender-supplied header can pick among those; it can never add one.
//     That closes the SSRF surface a peer-supplied URL would otherwise open —
//     see resolveTxBatchSources.

// txBatchCapabilityToken is echoed by /api/txbatch and in push responses. An
// older node has no such route, but its mux serves the explorer page for any
// unmatched path with a 200, so the presence of THIS token — not the status
// code — is what proves support.
const txBatchCapabilityToken = "tx-batch-v1"

// txBatchSourceHeader lets a pushing node name itself so the receiver knows
// which peer to ask for the body. Purely a hint: resolveTxBatchSources
// accepts it only if it is already a known sync peer.
const txBatchSourceHeader = "X-Aequitas-Node-URL"

// txBatchFetchTimeout is deliberately far longer than HTTPBroadcastBlock's 3 s
// push timeout. Moving the large payload OFF the latency-critical push and
// onto a request that can afford to take its time is the entire point.
const txBatchFetchTimeout = 25 * time.Second

// txBatchRootPattern keeps this endpoint a pure primary-key lookup: a
// well-formed sha256 hex digest or nothing. Nothing user-shaped ever reaches
// the query.
var txBatchRootPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// txBatchPeerCap remembers, per peer URL, whether that peer proved it
// understands bodies by reference. Absent means "assume not" — the safe
// default is always to send the complete block, exactly as before this
// existed.
var txBatchPeerCap sync.Map // peerURL -> bool

func txBatchPeerSupports(peerURL string) bool {
	v, ok := txBatchPeerCap.Load(peerURL)
	if !ok {
		return false
	}
	supports, _ := v.(bool)
	return supports
}

// recordTxBatchCapability updates what we believe about a peer. Called on
// EVERY push response, so a peer rolled back to older code (whose responses
// carry no token) reverts to full bodies after one block rather than needing
// a timeout to expire.
func recordTxBatchCapability(peerURL string, supports bool) {
	prev, existed := txBatchPeerCap.Load(peerURL)
	txBatchPeerCap.Store(peerURL, supports)
	if had, _ := prev.(bool); !existed || had != supports {
		if supports {
			fmt.Printf("[TX-BATCH] ✓ %s understands block bodies by reference — its pushes will carry the header only\n", peerURL)
		} else if existed {
			fmt.Printf("[TX-BATCH] ↩ %s no longer advertises body-by-reference — sending complete blocks to it again\n", peerURL)
		}
	}
}

// strippedBlockPayload returns the block serialised WITHOUT its transactions,
// and whether stripping was worth doing at all.
//
// The copy is by value and marshalled immediately, so it cannot observe any
// later mutation of the original. Transactions carries `omitempty`, so
// clearing it removes the key entirely; TxRoot stays, and calculateBlockHash
// reproduces the identical hash from it because no body is attached (see that
// function's comment for why an attached body must always win instead).
//
// chain_blocks deliberately has no tx_root column, so a block RELOADED from
// the database comes back with the field empty and is simply sent complete,
// as it always was. That is a safe degradation rather than an oversight: the
// root is derivable from the stored body at any time, every hash still
// verifies either way (see calculateBlockHash), and avoiding a schema
// migration keeps this change deployable to a live network one node at a
// time. Only freshly produced blocks — the ones actually being broadcast,
// which is the entire hot path — carry it.
func strippedBlockPayload(block *Block) ([]byte, bool) {
	if block == nil || len(block.Transactions) == 0 || block.TxRoot == "" {
		return nil, false
	}
	// A body this small is cheaper to send inline than to fetch in a second
	// round trip, so stripping it would make propagation slower, not faster.
	if len(block.Transactions) < txBatchMinTxsToStrip {
		return nil, false
	}
	stripped := *block
	stripped.Transactions = nil
	data, err := json.Marshal(&stripped)
	if err != nil {
		return nil, false
	}
	return data, true
}

// txBatchMinTxsToStrip is the break-even point. Below it the second round
// trip costs more than the bytes it saves.
const txBatchMinTxsToStrip = 32

// resolveTxBatchSources returns the peer URLs this node may ask for a body,
// most likely first.
//
// SECURITY: the result is always a SUBSET of activeSyncPeers — the peers this
// node already issues outbound HTTP requests to on its ordinary sync loop.
// The sender-supplied header only reorders that set. It cannot extend it, so
// a hostile or misconfigured peer cannot steer this node into fetching from
// an address it would not otherwise contact (cloud metadata endpoints being
// the classic target). httpSyncClient additionally refuses to follow
// redirects, so a peer cannot bounce the request elsewhere either.
func (dag *BlockDAG) resolveTxBatchSources(preferred string) []string {
	dag.syncPeerMu.Lock()
	peers := make([]string, 0, len(dag.activeSyncPeers))
	for p := range dag.activeSyncPeers {
		peers = append(peers, p)
	}
	dag.syncPeerMu.Unlock()

	preferred = strings.TrimRight(strings.TrimSpace(preferred), "/")
	if preferred == "" {
		return peers
	}
	ordered := make([]string, 0, len(peers))
	for _, p := range peers {
		if strings.TrimRight(p, "/") == preferred {
			ordered = append(ordered, p)
		}
	}
	for _, p := range peers {
		if strings.TrimRight(p, "/") != preferred {
			ordered = append(ordered, p)
		}
	}
	return ordered
}

// FetchTxBatch retrieves a block body by digest from a peer and verifies it.
//
// The returned transactions are guaranteed to hash to root, so the caller may
// treat them as exactly what the producer signed — a peer serving anything
// else is detected here and simply not used.
func (dag *BlockDAG) FetchTxBatch(root, preferred string) ([]Transaction, error) {
	if !txBatchRootPattern.MatchString(root) {
		return nil, fmt.Errorf("refusing to fetch a malformed tx batch root %q", root)
	}
	if txs, ok := dag.state.LoadTxBatch(root); ok {
		return txs, nil
	}
	sources := dag.resolveTxBatchSources(preferred)
	if len(sources) == 0 {
		return nil, fmt.Errorf("no sync peer available to fetch tx batch %s from", root[:16])
	}
	var lastErr error
	for _, peerURL := range sources {
		txs, err := fetchTxBatchFrom(peerURL, root)
		if err != nil {
			lastErr = err
			continue
		}
		return txs, nil
	}
	return nil, fmt.Errorf("no peer could serve tx batch %s: %w", root[:16], lastErr)
}

// txBatchClient is the client body fetches go through. It is httpSyncClient —
// which pins DNS and refuses private, loopback and link-local addresses
// outright (see pinningDialer, sync_blocks.go) — so a body fetch inherits the
// same anti-SSRF and anti-DNS-rebinding protection as every other peer
// request, independently of resolveTxBatchSources' allowlist.
//
// Indirected through a variable solely so tests can point it at a local
// httptest server, which the real client correctly refuses to connect to.
// Production never reassigns it; TestFetchTxBatch_RealClientRefusesLoopback
// pins that the default is the guarded client.
var txBatchClient = httpSyncClient

func fetchTxBatchFrom(peerURL, root string) ([]Transaction, error) {
	ctx, cancel := context.WithTimeout(context.Background(), txBatchFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(peerURL, "/")+"/api/txbatch?root="+root, nil)
	if err != nil {
		return nil, err
	}
	resp, err := txBatchClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", peerURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", peerURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBlockStreamBytes))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", peerURL, err)
	}
	var payload struct {
		Root string        `json:"root"`
		Txs  []Transaction `json:"txs"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%s: malformed tx batch response: %w", peerURL, err)
	}
	// Verify here as well as in AttachTxBatch. This one catches a peer serving
	// the wrong body before it can be handed anywhere else, and it is the only
	// check that runs when a caller fetches a batch outside block acceptance.
	if got := txBatchRoot(payload.Txs); got != root {
		return nil, fmt.Errorf("%s: served a body hashing to %s, not the requested %s", peerURL, got[:16], root[:16])
	}
	return payload.Txs, nil
}

// handleTxBatch serves a block body by its digest, and doubles as this node's
// capability advertisement when called without a root.
//
// Serving is unconditional and unauthenticated by design: a body is only ever
// retrievable by its own sha256 digest, every one of these transactions is
// already public in the block that commits to it, and any peer that has the
// block header can compute the digest anyway. There is nothing here that
// /api/blocks does not already expose.
func (a *APIServer) handleTxBatch(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodGet {
		jsonError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	root := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("root")))
	if root == "" {
		// The capability handshake. A node running older code has no route
		// here at all, and its catch-all serves the explorer page with a 200 —
		// so the token in this body, never the status code, is what a peer
		// checks before it strips a body toward us.
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":         true,
			"capability": txBatchCapabilityToken,
		})
		return
	}
	// Keeping this a strict digest lookup means the query can never be shaped
	// by the caller into anything but a primary-key hit.
	if !txBatchRootPattern.MatchString(root) {
		jsonError(w, "root must be a 64-character hex sha256 digest", http.StatusBadRequest)
		return
	}
	txs, ok := a.blockchain.state.LoadTxBatch(root)
	if !ok {
		jsonError(w, "unknown tx batch", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"root": root,
		"txs":  txs,
	})
}

// ensureBlockBody attaches a by-reference block's transactions before the
// block is handed to AddPeerBlock, fetching them from a peer if needed.
//
// Returns false when the body could not be obtained, and the caller MUST then
// reject the block and ask the sender to resend it complete. Accepting a
// block whose body is missing would replay it as empty and silently drop
// every transfer it contains, so there is no tolerable middle ground here.
func (a *APIServer) ensureBlockBody(block *Block, preferred string) bool {
	if !a.blockchain.state.NeedsTxBatch(block) {
		return true
	}
	txs, err := a.blockchain.FetchTxBatch(block.TxRoot, preferred)
	if err != nil {
		fmt.Printf("[TX-BATCH] ✗ Could not obtain the body of block #%d (root %s...): %v — asking the sender to resend the block complete\n",
			block.Height, block.TxRoot[:min(16, len(block.TxRoot))], err)
		return false
	}
	// AttachTxBatch re-verifies the digest against the root the SIGNED block
	// committed to, so this is checked independently of fetchTxBatchFrom's own
	// check — and AddPeerBlock's hash check then verifies it a third time,
	// now that a body is attached.
	if err := a.blockchain.state.AttachTxBatch(block, txs); err != nil {
		fmt.Printf("[TX-BATCH] ✗ %v — asking the sender to resend the block complete\n", err)
		return false
	}
	fmt.Printf("[TX-BATCH] ✓ Fetched %d transactions for block #%d by reference (header arrived stripped)\n", len(txs), block.Height)
	return true
}
