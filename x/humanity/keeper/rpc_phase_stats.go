package keeper

import (
	"sync/atomic"
	"time"
)

// Where does a transfer spend the time that TransferAtomic does not account
// for?
//
// THE ARITHMETIC THAT MADE THIS NECESSARY. The load generator holds ~380
// concurrent sender pairs. Each JSON-RPC batch carries transfers from ONE
// sender, and consecutive nonces force the dispatch loop to apply them in
// order, so the concurrency the node ever sees equals the number of senders.
// Throughput is therefore senders divided by per-transfer latency:
//
//	380 senders / 0.062s measured in TransferAtomic  =  ~6,100 TPS expected
//	                                        observed =   ~3,400 TPS
//
// Inverting the observed number gives 380/3,400 = 112ms of real per-transfer
// latency against 62ms measured inside TransferAtomic. About 50ms per
// transfer — nearly half of it — falls outside every instrument in place.
//
// That gap is why "add more senders" is the wrong next move: the harness
// ceiling is ~6,100 and only 3,400 is being reached, so the harness is not
// what binds.
//
// WHAT WAS ELIMINATED BY READING, BEFORE MEASURING. The two obvious suspects
// sitting either side of TransferAtomic in the native-transfer path are both
// already deferred and cost nothing on the request path:
//
//	SaveTxReceipt      buffers into pendingReceipt, flushed in the background
//	SyncBalancesToEVM  only marks the EVM mirror dirty; a worker drains it
//
// So the remaining candidates are the nonce reservation, the HTTP and JSON
// layers around dispatch, or the client. This splits the request into phases
// that add up to the whole, so the answer is a subtraction rather than a
// guess.
//
// HOW TO READ IT. rpc_total_per_item_ms is the handler measured from first
// byte to encoded response, divided by the number of batch items — directly
// comparable with TransferPathStats' avg_latency_ms. Then:
//
//	total ≈ transfer + nonce + rest   ->  the node owns the gap, and the
//	                                      phase carrying it is named
//	total ≈ transfer, well under 112  ->  the missing time is client-side,
//	                                      and the harness needs the fix
//
// Both outcomes are actionable, which is the point: this measurement cannot
// come back inconclusive.

var (
	rpcHandlerNanos atomic.Int64 // whole handler, arrival to response written
	rpcHandlerCount atomic.Int64 // requests measured
	rpcHandlerItems atomic.Int64 // batch items inside those requests

	rpcDecodeNanos atomic.Int64 // parallel decode + ecrecover pre-pass
	rpcEncodeNanos atomic.Int64 // response marshalling

	rpcNonceNanos atomic.Int64 // nonce lock, LoadNonce, ReserveNonce
	rpcNonceCount atomic.Int64

	rpcSendNanos atomic.Int64 // whole sendRawTransaction, per transaction
	rpcSendCount atomic.Int64
)

func noteRPCHandler(d time.Duration, items int) {
	rpcHandlerNanos.Add(int64(d))
	rpcHandlerCount.Add(1)
	rpcHandlerItems.Add(int64(items))
}

func noteRPCDecode(d time.Duration) { rpcDecodeNanos.Add(int64(d)) }
func noteRPCEncode(d time.Duration) { rpcEncodeNanos.Add(int64(d)) }

func noteRPCNonce(d time.Duration) {
	rpcNonceNanos.Add(int64(d))
	rpcNonceCount.Add(1)
}

func noteRPCSend(d time.Duration) {
	rpcSendNanos.Add(int64(d))
	rpcSendCount.Add(1)
}

func msPer(total, count int64) float64 {
	if count == 0 {
		return 0
	}
	return float64(total) / float64(count) / float64(time.Millisecond)
}

// RPCPhaseStats reports the request split so the unaccounted time can be
// subtracted out rather than guessed at.
//
// unaccounted_ms is the number this exists to produce: the part of a
// transaction's time in sendRawTransaction that is neither the nonce
// reservation nor TransferAtomic. If it is near zero, the node is not where
// the missing 50ms lives and the load generator is.
func RPCPhaseStats() map[string]interface{} {
	reqs := rpcHandlerCount.Load()
	items := rpcHandlerItems.Load()
	sends := rpcSendCount.Load()

	sendMs := msPer(rpcSendNanos.Load(), sends)
	nonceMs := msPer(rpcNonceNanos.Load(), rpcNonceCount.Load())

	// TransferAtomic's own average, the figure this is being reconciled
	// against. Read from the same counters TransferPathStats reports.
	transferMs := msPer(transferLatencyNanos.Load(), transferLatencyCount.Load())

	unaccounted := sendMs - nonceMs - transferMs
	if unaccounted < 0 {
		// Not all sends are transfers, so the averages are over different
		// populations and a small negative is expected rather than a bug.
		unaccounted = 0
	}

	return map[string]interface{}{
		"requests":               reqs,
		"batch_items":            items,
		"avg_items_per_request":  msPerItems(items, reqs),
		"rpc_total_per_item_ms":  msPer(rpcHandlerNanos.Load(), items),
		"decode_per_request_ms":  msPer(rpcDecodeNanos.Load(), reqs),
		"encode_per_request_ms":  msPer(rpcEncodeNanos.Load(), reqs),
		"send_tx_ms":             sendMs,
		"nonce_ms":               nonceMs,
		"transfer_ms":            transferMs,
		"unaccounted_in_send_ms": unaccounted,
	}
}

func msPerItems(items, reqs int64) float64 {
	if reqs == 0 {
		return 0
	}
	return float64(items) / float64(reqs)
}
