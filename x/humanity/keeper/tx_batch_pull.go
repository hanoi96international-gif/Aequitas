package keeper

import (
	"net/http"
	"sync/atomic"
)

// Stripped blocks on the PULL sync path.
//
// tx_batch_transport.go already does this for the push path, where the payoff
// was fitting a block inside HTTPBroadcastBlock's 3-second timeout. The pull
// path — GET /api/blocks, which is how a peer catches up — still sent every
// transaction body, and that turned out to be the single largest cost this node
// pays under load.
//
// MEASURED on Contabo2, 60 seconds of 597-sender load, with per-endpoint
// counters (endpoint_stats.go) added specifically because three earlier guesses
// at where the CPU was going had all been wrong:
//
//	/api/blocks          203 calls   1,027,041,504 bytes   (1.03 GB, COMPRESSED)
//	/api/blocks/by-hash  315 calls     407,448,579 bytes
//	callers: 173.249.37.118, 152.55.176.167  — two real peers
//
// 1.4 GB compressed in one minute, about 5 MB per response and roughly five
// times that before compression. A CPU profile in the same window put 41.59% of
// the node in handleBlocks, against 15.32% for the entire signature-recovery
// path — the node spent more effort shipping blocks to two peers than verifying
// every signature it processed.
//
// That is a feedback loop, not a plateau: higher throughput makes fatter
// blocks, fatter blocks make peers fall behind, peers that fall behind pull
// larger ranges, and serving them takes CPU away from producing. It gets worse
// exactly as throughput rises, which is the opposite of what a 50k TPS goal
// needs.
//
// WHY THIS IS SAFE AGAINST A LIVE NETWORK.
//
//  1. The REQUESTER opts in, with ?stripped=1. A peer running older code never
//     sends it and therefore cannot receive a stripped block. No capability
//     negotiation, no version handshake, no way for this to reach a node that
//     would not understand it.
//
//  2. A block is only stripped if this node can actually serve its body. If
//     LoadTxBatch cannot produce the batch for that TxRoot, the block goes out
//     in full. Stripping a body the peer could never obtain would strand it,
//     which is the one failure this must not have.
//
//  3. The DAG's own blocks are never mutated. GetBlocksSince returns pointers
//     into live state, so stripping copies the header and leaves the original
//     alone — writing Transactions = nil through those pointers would erase the
//     bodies from this node's own memory.
//
//  4. The requester fails soft: if it cannot obtain a body it re-fetches the
//     same page without the parameter. See fetchBlocksSince.

var (
	strippedBlocksServed atomic.Int64
	strippedBlocksFull   atomic.Int64
)

// stripBlocksForPeer returns the page with transaction bodies removed from
// every block whose body this node can still serve on request.
func (a *APIServer) stripBlocksForPeer(blocks []*Block) []*Block {
	if a.state == nil {
		return blocks
	}
	out := make([]*Block, len(blocks))
	for i, b := range blocks {
		// Nothing to strip, nothing to strip it back to, or too little to be
		// worth a second round trip. The last is the same break-even the push
		// path applies (txBatchMinTxsToStrip), for the same reason: below it the
		// extra request costs more than the bytes it saves.
		//
		// This stays strictly READ-ONLY. An earlier attempt also stored the body
		// here so that more blocks became strippable, which put a write
		// proportional to the served payload onto a read path: it wrote 851MB in
		// minutes and starved this node's own sync until it stopped advancing at
		// all. The population of the batch store belongs at produce/accept time,
		// never here.
		if b == nil || len(b.Transactions) < txBatchMinTxsToStrip || b.TxRoot == "" {
			out[i] = b
			strippedBlocksFull.Add(1)
			continue
		}
		if _, ok := a.state.LoadTxBatch(b.TxRoot); !ok {
			// The body is not retrievable from here, so sending a header would
			// leave the peer unable to complete the block. Send it whole.
			out[i] = b
			strippedBlocksFull.Add(1)
			continue
		}
		// Shallow copy: the original belongs to the DAG and must keep its
		// transactions.
		stripped := *b
		stripped.Transactions = nil
		out[i] = &stripped
		strippedBlocksServed.Add(1)
	}
	return out
}

// StrippedPullStats reports how much of the pull path is actually being
// stripped. A high full count means bodies are not retrievable and the saving
// is not being realised — worth seeing rather than assuming.
func StrippedPullStats() map[string]interface{} {
	return map[string]interface{}{
		"blocks_stripped":   strippedBlocksServed.Load(),
		"blocks_sent_whole": strippedBlocksFull.Load(),
	}
}

// pushError answers a rejected block push, and carries this node's
// body-by-reference capability while doing so.
//
// WHY THE TOKEN BELONGS ON AN ERROR TOO. The sender learns capability from
// whatever the push response contains, and four of handleBlockPush's response
// paths used to omit it. One of them closes a loop that cannot open itself:
//
//	jsonError(w, "request body too large", 413)
//
// Under load a COMPLETE block push is megabytes and exceeds the body limit, so
// the receiver answers 413 with no token, the sender concludes the peer does
// not understand bodies by reference and stops stripping toward it, and the
// next push is another complete, oversized block. The stripped push would have
// been a few hundred bytes and would have fit — the error response disables
// precisely the mechanism that prevents the error.
//
// The token asserts what this build can do, which is true regardless of
// whether one particular request was accepted. Withholding it on an error says
// something the node does not mean.
//
// Observed live: Contabo2 logged "↩ https://aequitas.digital no longer
// advertises body-by-reference" without any accompanying "could not fetch the
// body" line, which is what pointed here rather than at the resend_full path.
func pushError(w http.ResponseWriter, status int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(`{"ok":false,"reason":"` + reason + `","tx_batch":"` + txBatchCapabilityToken + `"}`))
}
