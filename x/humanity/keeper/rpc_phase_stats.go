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

	metaMs := msPer(rpcMetaNanos.Load(), rpcMetaCount.Load())
	quittungMs := msPer(rpcQuittungNanos.Load(), rpcQuittungCount.Load())
	spiegelMs := msPer(rpcSpiegelNanos.Load(), rpcSpiegelCount.Load())
	vorlaufMs := msPer(rpcVorlaufNanos.Load(), rpcVorlaufCount.Load())
	// Der Vorlauf enthaelt die Nonce bereits -- sonst zoege man sie zweimal ab.
	unaccounted := sendMs - transferMs - metaMs - quittungMs - spiegelMs - vorlaufMs
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
		"meta_shard_ms":          msPer(rpcMetaNanos.Load(), rpcMetaCount.Load()),
		"quittung_ms":            msPer(rpcQuittungNanos.Load(), rpcQuittungCount.Load()),
		"spiegel_ms":             msPer(rpcSpiegelNanos.Load(), rpcSpiegelCount.Load()),
		"vorlauf_ms":             msPer(rpcVorlaufNanos.Load(), rpcVorlaufCount.Load()),
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

// Die zwei letzten unvermessenen Posten in sendRawTransaction.
//
// unaccounted_in_send_ms stand am 06.09.2026 bei 55,55 ms -- 30 % der Zeit
// in dieser Funktion -- und keiner der benannten Posten erklaerte sie: die
// Nonce kostet 0,016 ms, das Dekodieren 0,8 ms je Ueberweisung, die
// Signaturpruefung ist vorberechnet. Uebrig bleiben die Schreibvorgaenge auf
// den Metadaten-Shards und die Quittung. Beide sind fuer sich billig -- aber
// beide nehmen eine Sperre, und unter 1.600 gleichzeitigen Goroutinen ist
// genau das die Groesse, die von "billig" zu "teuer" kippen kann, ohne dass
// sich am Code etwas aendert.
//
// Dieselbe Trennung wie ueberall sonst hier: die Phasen summieren sich zum
// Ganzen, also ist die Antwort eine Subtraktion. Bleibt unaccounted auch
// nach diesen beiden gross, liegt die Zeit im HTTP- oder JSON-Mantel und
// nicht mehr in dieser Funktion.
var (
	rpcMetaNanos     atomic.Int64
	rpcMetaCount     atomic.Int64
	rpcQuittungNanos atomic.Int64
	rpcQuittungCount atomic.Int64
	rpcSpiegelNanos  atomic.Int64
	rpcSpiegelCount  atomic.Int64
)

func merkeRPCMeta(d time.Duration) {
	rpcMetaNanos.Add(int64(d))
	rpcMetaCount.Add(1)
}

func merkeRPCQuittung(d time.Duration) {
	rpcQuittungNanos.Add(int64(d))
	rpcQuittungCount.Add(1)
}

func merkeRPCSpiegel(d time.Duration) {
	rpcSpiegelNanos.Add(int64(d))
	rpcSpiegelCount.Add(1)
}

// Der Vorlauf: vom Eintritt in sendRawTransaction bis zu den Metadaten.
//
// Nach META (0,05 ms), Quittung (0,29 ms) und Spiegel (0,28 ms) waren die
// 59 ms unaccounted unveraendert -- alle drei Verdaechtigen INNERHALB der
// Funktion sind damit ausgeschlossen. Uebrig bleibt genau ein Abschnitt, den
// noch keine Uhr beruehrt: der Anfang. Dort liegen die Zulassungspruefung,
// das Auspacken des JSON-Parameters, die Auswertung der vorberechneten
// Signatur (oder deren Nachholen), der Transaktionshash und die
// Nonce-Reservierung.
//
// Ist der Vorlauf klein, liegt die Zeit ausserhalb dieser Funktion -- im
// HTTP- oder JSON-Mantel, im Batch-Dispatch oder beim Client. Auch das waere
// eine Antwort, und danach ist diese Funktion vollstaendig zerlegt: die
// Phasen summieren sich dann zum Ganzen und unaccounted geht gegen null.
var (
	rpcVorlaufNanos atomic.Int64
	rpcVorlaufCount atomic.Int64
)

func merkeRPCVorlauf(d time.Duration) {
	rpcVorlaufNanos.Add(int64(d))
	rpcVorlaufCount.Add(1)
}
