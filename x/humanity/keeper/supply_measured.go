package keeper

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
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

// FIX (2026-08-15, same day, my own regression): the mutex used to live INSIDE
// this struct, and the code below refreshed the cache by assigning a whole new
// struct value — which overwrote the held, locked mutex with a fresh, unlocked
// one. The deferred Unlock then released a mutex that was no longer locked, and
// Go answers that with `fatal error: sync: unlock of unlocked mutex`, which is
// NOT recoverable: it terminates the process. Every single call to
// /api/health/combined therefore killed the node, and the deploy verification
// step added earlier today polls exactly that endpoint.
//
// The lock is a separate variable now, so the whole-struct assignment that
// caused this cannot reach it. Guarding a value with a mutex stored beside the
// value it guards is only safe if the value is never replaced wholesale — and
// the safe arrangement is the one where the unsafe version does not compile
// into something plausible.
var (
	measuredSupplyMu    sync.Mutex
	measuredSupplyCache struct {
		value   float64
		ok      bool
		err     string
		fetched time.Time
	}
)

// MeasuredTotalAEQ returns the summed ledger total, whether it could be
// measured at all, and — when it could not — why. It never fabricates a number
// on failure: a caller that cannot tell "0 AEQ" from "the query failed" is the
// exact defect this whole audit kept finding.
func (cs *ChainState) MeasuredTotalAEQ() (total float64, ok bool, reason string) {
	if cs.db == nil {
		return 0, false, "no database configured"
	}
	measuredSupplyMu.Lock()
	defer measuredSupplyMu.Unlock()
	if time.Since(measuredSupplyCache.fetched) < measuredSupplyTTL && !measuredSupplyCache.fetched.IsZero() {
		return measuredSupplyCache.value, measuredSupplyCache.ok, measuredSupplyCache.err
	}

	var accounts, reserve float64
	if err := cs.db.QueryRow(`SELECT COALESCE(sum(balance), 0) FROM chain_accounts`).Scan(&accounts); err != nil {
		// Field-wise, never a whole-struct assignment — see the var block above.
		measuredSupplyCache.value = 0
		measuredSupplyCache.ok = false
		measuredSupplyCache.err = "could not sum chain_accounts: " + err.Error()
		measuredSupplyCache.fetched = time.Now()
		return 0, false, measuredSupplyCache.err
	}
	// The pool row is absent on a node that has never had liquidity; that is a
	// zero reserve, not a failure.
	if err := cs.db.QueryRow(`SELECT COALESCE(reserve_aeq, 0) FROM liquidity_pool WHERE id = 1`).Scan(&reserve); err != nil {
		reserve = 0
	}
	measuredSupplyCache.value = accounts + reserve
	measuredSupplyCache.ok = true
	measuredSupplyCache.err = ""
	measuredSupplyCache.fetched = time.Now()
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
	difference := measured - claimed
	out["difference"] = fmt.Sprintf("%+.6f", difference)
	// Anything past a micro-AEQ is structural, not arithmetic: balances are
	// stored as micro-integers, so a correct implementation is exact.
	out["reconciled"] = difference < 1e-6 && -difference < 1e-6

	// Tell the KNOWN gap apart from a NEW one.
	//
	// Without this the field above reads "+305.278" for as long as the old
	// excess sits in the AMM reserve, and it would go on reading roughly that
	// if a fresh bug minted another five AEQ tomorrow. A number that is always
	// wrong stops being read -- and this is the one number that says whether
	// the currency's central promise still holds.
	//
	// The baseline is the excess measured on 2026-08-20, created before the
	// save-ordering fixes of that day (see applySwapDeltaLocked,
	// AddLiquidityDelta and RemoveLiquidityAtomic, which now all persist the
	// debit before the credit so a failure between the two writes destroys
	// supply rather than creating it). It is deliberately a floor to compare
	// against, never an excuse: burning it is still open.
	//
	// Set SUPPLY_GAP_BASELINE_AEQ after the old excess is actually removed --
	// lowering it is what turns the alarm back on at the new level, and it
	// takes no rebuild.
	baseline := knownSupplyGapAEQ()
	beyond := difference - baseline
	out["known_gap_baseline"] = fmt.Sprintf("%.6f", baseline)
	out["beyond_known_gap"] = fmt.Sprintf("%+.6f", beyond)
	alarm, reason := supplyAlarm(difference, baseline)
	out["supply_alarm"] = alarm
	if alarm {
		out["supply_alarm_reason"] = reason
	}

	// The breakdown that tells the two explanations apart.
	//
	// A positive difference means either AEQ was created, or fewer humans are
	// counted than were granted 1,000. Those need opposite responses, and the
	// numbers below separate them without anyone needing shell access to the
	// database — which is what has kept the live +305.278 unexplained since
	// 2026-08-15.
	//
	//   humans ~= claimed        the humans hold what they were granted, so a
	//                            gap lives somewhere else
	//   non_humans ~= difference the AEQ sits in accounts no longer counted as
	//                            people: deregistered, or never marked human
	//   pools carry it           it is fee revenue in the tokenomics pools,
	//                            which is granted money that merely moved
	//   none of the above        something created it
	if parts, err := cs.supplyBreakdown(); err == nil {
		out["breakdown"] = parts
	} else {
		out["breakdown_error"] = err.Error()
	}
	return out
}

// supplyBreakdown splits the measured total by where the AEQ actually sits.
func (cs *ChainState) supplyBreakdown() (map[string]string, error) {
	if cs.db == nil {
		return nil, fmt.Errorf("no database configured")
	}
	var humans, nonHumans, pools, reserve float64
	var nonHumanAccounts, humanAccounts int

	// The human COUNT straight from the ledger, next to the counter the rule is
	// computed from.
	//
	// TotalSupply is humanCountLocked x 1,000, and that counter is seeded by a
	// full scan once and then maintained incrementally. An incremental counter
	// that drifts low makes the claimed supply too small, and the gap looks
	// exactly like minted money while nothing was minted at all. Comparing it
	// against COUNT(*) is one query and rules that out — or finds it.
	if err := cs.db.QueryRow(
		`SELECT COALESCE(sum(balance),0), count(*) FROM chain_accounts WHERE is_human = true`,
	).Scan(&humans, &humanAccounts); err != nil {
		return nil, err
	}
	if err := cs.db.QueryRow(
		`SELECT COALESCE(sum(balance),0), count(*) FROM chain_accounts
		 WHERE is_human = false AND balance > 0`).Scan(&nonHumans, &nonHumanAccounts); err != nil {
		return nil, err
	}
	if err := cs.db.QueryRow(
		`SELECT COALESCE(sum(balance),0) FROM chain_accounts WHERE lower(address) = ANY($1)`,
		pq.Array([]string{ubiPoolAddr, lpPoolAddr, validatorsPoolAddr, treasuryPoolAddr}),
	).Scan(&pools); err != nil {
		return nil, err
	}
	if err := cs.db.QueryRow(
		`SELECT COALESCE(reserve_aeq,0) FROM liquidity_pool WHERE id = 1`).Scan(&reserve); err != nil {
		reserve = 0
	}

	// What the rule would claim if it counted the ledger instead of the
	// incremental counter. If this differs from claimed_humans_x_1000, the
	// counter has drifted and the "excess" is arithmetic, not creation.
	claimedFromLedger := float64(humanAccounts) * 1000

	return map[string]string{
		"human_accounts":     fmt.Sprintf("%d", humanAccounts),
		"claimed_if_counted": fmt.Sprintf("%.6f", claimedFromLedger),
		"humans":             fmt.Sprintf("%.6f", humans),
		"non_humans":         fmt.Sprintf("%.6f", nonHumans),
		"non_human_accounts": fmt.Sprintf("%d", nonHumanAccounts),
		"tokenomics_pools":   fmt.Sprintf("%.6f", pools),
		"amm_reserve":        fmt.Sprintf("%.6f", reserve),
	}, nil
}

// knownSupplyGapAEQ is the excess that already existed when the supply audit
// of 2026-08-20 measured it, in AEQ.
//
// It is NOT a tolerance. Every path that could create money was made
// debit-before-credit on that day, so nothing should ever be added to this
// figure; it exists only so that a NEW gap is visible next to an old one that
// has not been cleaned up yet. Removing the old excess means moving AEQ on a
// live chain, which is a decision about real balances and not a code change.
func knownSupplyGapAEQ() float64 {
	if raw := strings.TrimSpace(os.Getenv("SUPPLY_GAP_BASELINE_AEQ")); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil && v >= 0 {
			return v
		}
		// A malformed override must not silently widen the alarm's blind spot.
		fmt.Printf("[SUPPLY] SUPPLY_GAP_BASELINE_AEQ=%q is not a number, using the built-in baseline\n", raw)
	}
	return 305.277988
}

// supplyAlarm decides whether the gap grew past what was already known.
//
// One-sided on purpose. A gap that SHRANK is the old excess being cleaned up,
// which is the outcome we want and not something to wake anyone for. Only
// growth means money appeared that no human's registration accounts for.
func supplyAlarm(difference, baseline float64) (bool, string) {
	beyond := difference - baseline
	if beyond <= 1e-6 {
		return false, ""
	}
	return true, fmt.Sprintf(
		"%.6f AEQ beyond the gap known on 2026-08-20 -- this is newly created "+
			"money, not the old excess", beyond)
}
