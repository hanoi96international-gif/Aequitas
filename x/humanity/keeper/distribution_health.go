package keeper

import (
	"fmt"
	"sync"
	"time"
)

// Is the daily round — UBI, validator rewards, LP rewards, escrow sweeps —
// actually happening?
//
// That question was surprisingly hard to answer, and answering it wrongly is
// easy. last_ubi_at looks like the obvious signal and is not one:
// RunDailyDistributionAtomic only stamps it `if ubiTotal > 0` (state.go), so a
// round that runs correctly and finds the pools empty deliberately leaves the
// timestamp alone. On 2026-08-15 last_ubi_at was nineteen days old, which looks
// alarming and means only that there had been no fees to distribute since —
// the four pools held 0.0000 / 0.0000 / 0.0000 / 0.2227 AEQ, because a chain
// with fifteen wallets and no trading generates almost no fees.
//
// The signal that does mean something is what the SCHEDULER decided, and until
// now that existed only as a log line:
//
//	[POOLS] ✗ Skipping distribution this round: peers are configured but this
//	        node has never successfully synced a block from any of them
//
// which scrolls past among a thousand blocks a minute. So this records every
// decision the scheduler makes and publishes it, and it deliberately does NOT
// treat "nothing was paid out" as a fault.
//
// The gate itself is not changed here: refusing to distribute while this node's
// view of the chain may be incomplete is correct — validator rewards are
// weighted by blocks_produced read from local state, so distributing on a
// partial view pays the wrong people. What was missing is that the refusal was
// invisible.

type distributionOutcome struct {
	mu       sync.Mutex
	at       time.Time
	outcome  string // "ran" | "ran_elsewhere" | "skipped" | "failed"; empty before the first attempt
	reason   string
	attempts int
	lastRan  time.Time // last attempt that actually executed a round
}

var lastDistribution distributionOutcome

// RecordDistributionOutcome is called by the scheduler for every round it
// considers, including the ones it decides not to run.
func RecordDistributionOutcome(outcome, reason string) {
	lastDistribution.mu.Lock()
	defer lastDistribution.mu.Unlock()
	lastDistribution.at = time.Now()
	lastDistribution.outcome = outcome
	lastDistribution.reason = reason
	lastDistribution.attempts++
	// "ran_elsewhere" counts as a round that happened: the node stood down
	// because the distributed lock showed another validator had already done
	// it, which is the mechanism working, not a fault. Treating it as a
	// decline made this metric raise the exact kind of false alarm it exists
	// to replace — observed on Contabo1 within an hour of shipping it.
	if outcome == "ran" || outcome == "ran_elsewhere" {
		lastDistribution.lastRan = time.Now()
	}
}

// distributionStaleAfter is when "the scheduler has not completed a round"
// stops being normal. The schedule is daily; an hour of slack covers a restart
// landing either side of the slot.
const distributionStaleAfter = 25 * time.Hour

// DistributionHealth reports whether the daily round is running — as distinct
// from whether it had anything to pay.
func (cs *ChainState) DistributionHealth() map[string]interface{} {
	out := map[string]interface{}{}

	// Payout history. Reported as fact, NOT as a fault: an empty pool is the
	// normal state of a chain with little activity.
	if lastUBI := cs.GetLastUBIAt(); lastUBI > 0 {
		out["last_payout_at"] = lastUBI
		out["last_payout_age_hours"] = fmt.Sprintf("%.1f", time.Since(time.Unix(lastUBI, 0)).Hours())
	} else {
		out["last_payout_at"] = 0
	}
	out["last_payout_note"] = "only stamped when a round actually distributed something; " +
		"an old value means no fees accumulated, not a missed round"

	lastDistribution.mu.Lock()
	defer lastDistribution.mu.Unlock()

	if lastDistribution.outcome == "" {
		out["last_attempt"] = nil
		out["healthy"] = true // nothing has gone wrong; this node simply has not reached a slot yet
		out["note"] = "this node has not reached a scheduled round since it started"
		return out
	}

	out["last_attempt"] = map[string]interface{}{
		"at":                   lastDistribution.at.Unix(),
		"seconds_ago":          int64(time.Since(lastDistribution.at).Seconds()),
		"outcome":              lastDistribution.outcome,
		"reason":               lastDistribution.reason,
		"attempts_this_uptime": lastDistribution.attempts,
	}

	// The real fault condition: rounds are being reached but none of them has
	// executed within a day. That is the shape a silently-blocked scheduler
	// has, and the shape the peer-sync gate produced.
	stale := !lastDistribution.lastRan.IsZero() && time.Since(lastDistribution.lastRan) > distributionStaleAfter
	neverRan := lastDistribution.lastRan.IsZero() && lastDistribution.attempts > 0
	out["healthy"] = !(stale || neverRan)
	if stale {
		out["problem"] = fmt.Sprintf("no round has executed in %.1f hours despite %d attempt(s) — last outcome %q: %s",
			time.Since(lastDistribution.lastRan).Hours(), lastDistribution.attempts, lastDistribution.outcome, lastDistribution.reason)
	}
	if neverRan {
		out["problem"] = fmt.Sprintf("every round since startup was declined (%d attempt(s)) — last outcome %q: %s",
			lastDistribution.attempts, lastDistribution.outcome, lastDistribution.reason)
	}
	return out
}
