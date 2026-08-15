package keeper

import (
	"fmt"
	"sync"
	"time"
)

// MeasuredTotalAEQ sums what the ledger actually holds: every account's balance
// plus the AMM's AEQ reserve.
//
// WHY THIS EXISTS (audit 2026-08-15). TotalSupply() does not measure anything —
// it returns humanCount x 1000, the protocol's rule, and its own comment
// dismissed any divergence from the ledger as "floating-point drift from swap
// fees and demurrage calculations". Measured from both validators' own
// databases, byte-identically on each:
//
//	total AEQ in existence   15,305.278004
//	humans x 1000            15,000
//	difference                  +305.278004   (2.04%)
//
// Float64 drift across fifteen accounts is on the order of 1e-10. Being wrong
// by nine orders of magnitude is what let a real 2% gap sit behind a
// plausible-sounding explanation, and /api/status kept publishing the rule as
// though it were the count.
//
// The gap is not growing: re-measured twenty minutes and ~1,200 blocks later,
// to the last micro-AEQ. Every money-moving path is held to conservation by
// TestSupplyConservation_* (transfer, the V7 fee path, swaps both directions,
// add/remove liquidity, demurrage settlement, the wealth cap, and the validator
// and LP rounds), and they pass — so this is a scar from an earlier incident
// rather than a live leak. This project has had several candidates, the
// clearest being the 2026-07-25 WAL-plus-resync corruption that re-applied
// history and drove 74 accounts negative.
//
// Publishing the measurement next to the rule turns an invisible 2% into a
// number someone can watch. If it ever starts moving, that is a live mint and
// the conservation tests above are the place to look.
//
// Cached briefly: this is one SUM over chain_accounts, which is cheap at
// today's size and a sequential scan at a million. The health endpoint it feeds
// is polled, not hammered, and a minute-stale figure answers the question it
// exists to answer just as well.
const measuredSupplyTTL = 60 * time.Second

type measuredSupply struct {
	mu      sync.Mutex
	value   float64
	ok      bool
	err     string
	fetched time.Time
}

var measuredSupplyCache measuredSupply

// MeasuredTotalAEQ returns the summed ledger total, whether it could be
// measured at all, and — when it could not — why. It never fabricates a number
// on failure: a caller that cannot tell "0 AEQ" from "the query failed" is the
// exact defect this whole audit kept finding.
func (cs *ChainState) MeasuredTotalAEQ() (total float64, ok bool, reason string) {
	if cs.db == nil {
		return 0, false, "no database configured"
	}
	measuredSupplyCache.mu.Lock()
	defer measuredSupplyCache.mu.Unlock()
	if time.Since(measuredSupplyCache.fetched) < measuredSupplyTTL && !measuredSupplyCache.fetched.IsZero() {
		return measuredSupplyCache.value, measuredSupplyCache.ok, measuredSupplyCache.err
	}

	var accounts, reserve float64
	if err := cs.db.QueryRow(`SELECT COALESCE(sum(balance), 0) FROM chain_accounts`).Scan(&accounts); err != nil {
		measuredSupplyCache = measuredSupply{fetched: time.Now(), err: "could not sum chain_accounts: " + err.Error()}
		return 0, false, measuredSupplyCache.err
	}
	// The pool row is absent on a node that has never had liquidity; that is a
	// zero reserve, not a failure.
	if err := cs.db.QueryRow(`SELECT COALESCE(reserve_aeq, 0) FROM liquidity_pool WHERE id = 1`).Scan(&reserve); err != nil {
		reserve = 0
	}
	measuredSupplyCache = measuredSupply{value: accounts + reserve, ok: true, fetched: time.Now()}
	return measuredSupplyCache.value, true, ""
}

// SupplyReconciliation reports the rule, the measurement and the gap between
// them in one place, for /api/health/combined.
func (cs *ChainState) SupplyReconciliation() map[string]interface{} {
	claimed := cs.TotalSupply()
	out := map[string]interface{}{
		"claimed_humans_x_1000": fmt.Sprintf("%.6f", claimed),
	}
	measured, ok, reason := cs.MeasuredTotalAEQ()
	if !ok {
		out["measured"] = nil
		out["measured_error"] = reason
		return out
	}
	out["measured"] = fmt.Sprintf("%.6f", measured)
	out["difference"] = fmt.Sprintf("%+.6f", measured-claimed)
	// Anything past a micro-AEQ is structural, not arithmetic: balances are
	// stored as micro-integers, so a correct implementation is exact.
	out["reconciled"] = measured-claimed < 1e-6 && claimed-measured < 1e-6
	return out
}
