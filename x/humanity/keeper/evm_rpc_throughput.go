package keeper

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// This file exists because of one measurement. On 2026-07-25 a load run against
// Contabo2 offered 21,600 transfers over 64s and the generator scored zero:
// every request hit its 20s client timeout, and 216 attempts produced 216
// timeouts and not one response. Yet the node's own log for that same window
// was full of "[RPC] ✓ Transfer" lines, block production never left its budget
// ("no slow-tick lines"), and /api/status answered normally before and after.
//
// So the transfers were being APPLIED. What never came back was the answer. And
// the generator, which can only see its own socket, reported that as 0.0 TPS —
// a number that says nothing about the chain and everything about where the
// measurement was taken.
//
// Two things follow, and this file is both.

// ─── 1. MEASURE THROUGHPUT WHERE IT ACTUALLY HAPPENS ─────────────────────────

// recordAppliedTransfer counts one successfully applied transfer and, once per
// second, emits a single line with the rate. That line is the honest throughput
// number: it counts what the node DID, not what a client managed to hear about,
// so it stays correct when responses are slow, timing out, or dropped entirely.
//
// One line per second is not a hot path — the counter is a single atomic add,
// and the reporter is one goroutine started lazily on the first transfer, so a
// node that never serves a transfer never pays for it.
//
// Silent when idle: a second with no transfers prints nothing, so this cannot
// turn a quiet node's log into a heartbeat.
func recordAppliedTransfer() {
	appliedTransfers.Add(1)
	throughputReporterOnce.Do(startThroughputReporter)
}

var (
	appliedTransfers       atomic.Int64
	throughputReporterOnce sync.Once
)

// AppliedTransfers exposes the running total for tests and diagnostics.
func AppliedTransfers() int64 { return appliedTransfers.Load() }

func startThroughputReporter() {
	SafeGoroutine("rpcThroughputReporter", func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		var last int64
		var peak int64
		for range ticker.C {
			now := appliedTransfers.Load()
			delta := now - last
			last = now
			if delta <= 0 {
				continue
			}
			if delta > peak {
				peak = delta
			}
			fmt.Printf("[RPC] ⚡ %d transfers applied in the last second (peak %d/s, total %d)\n",
				delta, peak, now)
		}
	})
}

// ─── 2. STOP PAYING TWO SYSCALLS PER TRANSFER ────────────────────────────────

// rpcQuietTx suppresses the two per-transaction log lines in
// sendRawTransaction: the "eth_sendRawTransaction hash=..." line on entry and
// the "✓ Transfer ..." line on success.
//
// Those are two unbuffered fmt.Printf calls per transaction, each a write
// syscall that Docker's log driver then serialises to disk. At the 50,000 TPS
// this chain is tuned for (maxTxsPerBlock=50000 at BLOCK_TIME=1s) that is
// 100,000 log lines per second, which no log driver will absorb — the writes
// become the throughput ceiling regardless of how fast consensus and storage
// are. They are genuinely useful at ordinary volume and unaffordable at target
// volume, which is exactly what a per-box switch is for.
//
// DEFAULT UNCHANGED: unset or anything other than "1" keeps today's behaviour
// line for line. This is the same class of explicit operator decision as
// AEQUITAS_RPC_RATE_LIMIT_MAX or AEQUITAS_WAL_ENABLED, and it follows the same
// rule those do — an operator who sets nothing inherits nothing.
//
// Setting it does not blind the node: recordAppliedTransfer above still counts
// every applied transfer and still reports the rate once per second, so
// throughput stays visible while the per-transaction detail does not.
var rpcQuietTx = rpcQuietTxFromEnv()

func rpcQuietTxFromEnv() bool {
	raw := strings.TrimSpace(os.Getenv("AEQUITAS_RPC_QUIET_TX"))
	if raw != "1" {
		return false
	}
	fmt.Println("[RPC] ⚠ AEQUITAS_RPC_QUIET_TX=1 — per-transaction log lines suppressed (two fmt.Printf/tx, unaffordable at 50k TPS). Applied-transfer throughput is still reported once per second.")
	return true
}
