package keeper

import (
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

// Do not accept work this node cannot turn into blocks.
//
// THE TRAP, measured on 2026-08-21 under sustained load.
//
// The node kept applying transfers at 2,000-4,000/s (peak 6,787/s) while its
// block height did not move at all -- frozen for over 15 seconds, which is what
// the load generator's own safety net aborts on. Its log, once a second:
//
//	[BLOCK] ⏳ Not yet 3 consecutive clean sync cycles with every trusted seed
//	        — skipping block production regardless of height-based gates
//
// backlog_vs_fork.go already describes where that leads: "A validator that
// falls out of production once cannot get back in on its own, because merging
// its backlog requires producing. That is why a resync has so often been the
// only remedy in this project's history: not because data was corrupt, but
// because the state is a trap."
//
// WHY NOT FIX IT AT THE GATE. The gate is correct and must stay. Unresolved
// deferrals are the one signal that separates "behind" from "forked", and
// reconcileDeferrals already refuses to count a deferral until it has outlived
// the propagation grace, so a healthy node converges to zero and produces.
// AEQUITAS_PRODUCE_WHEN_BACKLOG_SHRINKING already lets a node produce while a
// backlog SHRINKS. Under sustained overload the backlog grows, so it correctly
// declines -- the gate is doing its job.
//
// WHAT IS ACTUALLY MISSING is upstream: nothing stops the node from accepting
// more transactions while it cannot produce. It has a per-IP rate limiter and
// no admission control at all, so under load it keeps saying yes to work that
// can only pile up. Every accepted transfer makes the backlog it must drain
// larger, which keeps the gate shut, which stops it draining anything.
//
// THE INVARIANT. A node that is not producing blocks cannot include your
// transaction, so it should not take it. Refusing is not a degradation, it is
// the honest answer -- and it is retryable, so a client moves to another
// validator or comes back, instead of having its transfer absorbed into a
// backlog that will not clear.
//
// This is deliberately NOT a mempool depth limit. Depth says how much is
// queued; it does not say whether the queue is draining. Time since the last
// produced block says exactly that, costs one atomic load, and cannot be
// fooled by a queue that happens to be briefly short.

var lastBlockProducedUnix atomic.Int64

// noteBlockProduced records that this node produced a block. Called from the
// one place ProduceBlock returns one.
func noteBlockProduced() { lastBlockProducedUnix.Store(time.Now().Unix()) }

// admissionStallSeconds is how long production may be stalled before this node
// stops accepting transfers.
//
// 30s is many block times at BLOCK_TIME=1s, so ordinary jitter, a slow tick or
// one gated cycle never trips it. It only fires on the sustained case the trap
// needs, and errs towards accepting: a false refusal costs a client one retry,
// a false acceptance costs the network a validator.
const admissionStallSeconds = 30

func admissionStallLimit() int64 {
	if raw := os.Getenv("AEQUITAS_ADMIT_STALL_SECONDS"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v > 0 {
			return v
		}
		// A typo must not disable the protection -- same rule as
		// rpcRateLimitMax's own override.
	}
	return admissionStallSeconds
}

// processStartUnix is when this process came up, used as the stall origin
// until the node produces its first block.
//
// THE HOLE THIS CLOSES, observed live on 2026-08-22. Contabo2 was restarted,
// took a sustained load run, and froze at exactly its starting height while
// still applying 4,349 transfers a second. Its admission stats read:
//
//	last_block_produced_unix: 0
//	refusing:                 false
//	stalled_seconds:          0
//
// It had never produced a block since the restart, so "zero before the first
// one" made it PERMANENTLY exempt from the check written to protect it. It
// accepted 104,060 transfers it could never include, and drifted 135 blocks
// behind its peer.
//
// The exemption was meant to cover the seconds between process start and the
// first block. Anchoring to process start covers exactly that and no more: a
// node still gets the full stall limit to produce its first block, and after
// that a node that has never produced is treated like any other node that
// cannot produce -- because from a client's point of view it is the same
// thing.
var processStartUnix = time.Now().Unix()

// productionStalledFor reports how long it has been since this node produced a
// block, or since the process started if it never has.
//
// It deliberately does NOT return zero in the never-produced case. See
// processStartUnix: that exemption was unbounded, and an unbounded exemption
// turns the one node that most needs the check into the one node that never
// gets it.
func productionStalledFor() time.Duration {
	since := lastBlockProducedUnix.Load()
	if since == 0 {
		since = processStartUnix
	}
	d := time.Now().Unix() - since
	if d < 0 {
		return 0
	}
	return time.Duration(d) * time.Second
}

// admissionRefusalReason returns a non-empty, client-facing reason when this
// node should not accept new transfers, and "" when it should.
func admissionRefusalReason() string {
	limit := admissionStallLimit()
	stalled := productionStalledFor()
	if stalled < time.Duration(limit)*time.Second {
		return ""
	}
	if lastBlockProducedUnix.Load() == 0 {
		// Distinguished because the two cases need different operator action: a
		// node that has never produced since starting is not draining a
		// backlog, it has not entered production at all.
		return fmt.Sprintf(
			"this validator has not produced a block since starting %ds ago and cannot include "+
				"new transactions; retry shortly or send to another validator", int64(stalled.Seconds()))
	}
	return fmt.Sprintf(
		"this validator has not produced a block for %ds and cannot include new transactions; "+
			"retry shortly or send to another validator", int64(stalled.Seconds()))
}

// AdmissionStats exposes the signal for an operator endpoint, so a refusal can
// be told apart from a rate limit without reading logs.
func AdmissionStats() map[string]interface{} {
	last := lastBlockProducedUnix.Load()
	return map[string]interface{}{
		"last_block_produced_unix": last,
		"stalled_seconds":          int64(productionStalledFor().Seconds()),
		"stall_limit_seconds":      admissionStallLimit(),
		"refusing":                 admissionRefusalReason() != "",
		// True while the node has not produced a single block since starting.
		// Read it together with stalled_seconds: that pair is what showed the
		// unbounded exemption on Contabo2, where stalled_seconds sat at 0 while
		// the node was frozen.
		"never_produced":     last == 0,
		"process_start_unix": processStartUnix,
	}
}
