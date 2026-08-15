package keeper

import (
	"fmt"
	"math"
	"sort"
	"testing"
)

// Regression tests for the double-demurrage bug found live on 2026-08-15.
//
// GetAllAccounts returns a COPY whose Balance has already been replaced with
// effectiveBalance(acc). handleHumans then called effectiveBalance() a second
// time on that copy, applying the decay factor twice over the same idle
// period. Every account past the three-month grace period was therefore
// published lighter than it actually is, and the Lorenz curve the frontend
// draws from those numbers disagreed with the Gini in /api/status — which is
// computed from real wealth via calcGiniLocked.
//
// Measured against production before the fix: the curve's basis gave
// 0.12361229 while the chain reported 0.13244174, reproducibly, to the last
// digit. humanAEQWealthLocked's own comment records that this exact class of
// divergence (0.72 vs 0.15) had already happened once from the other side, so
// these tests pin BOTH properties: the published balance is decayed exactly
// once, and the Gini implied by the payload equals the chain's own.

// giniOfPayload mirrors calcGiniFromBalances so the test computes the Lorenz
// basis the same way a frontend would, rather than reusing chain internals
// (which would make the comparison circular).
func giniOfPayload(v []float64) float64 {
	sort.Float64s(v)
	n := len(v)
	if n == 0 {
		return 0
	}
	var total, cum float64
	for _, x := range v {
		total += x
	}
	if total == 0 {
		return 0
	}
	for i, x := range v {
		cum += float64(i+1) * x
	}
	return (2*cum)/(float64(n)*total) - float64(n+1)/float64(n)
}

// idleHumansState builds a state whose humans are all well past the demurrage
// grace period — the only regime in which a second effectiveBalance() call
// changes anything, and so the only regime that can catch this bug.
func idleHumansState(balances []float64) (*ChainState, []string) {
	cs := newTestState()
	longIdle := nowUnix() - demurrageGracePeriodSeconds - 6*secondsPerMonth
	addrs := make([]string, 0, len(balances))
	for i, bal := range balances {
		addr := fmt.Sprintf("0x%040x", i+1)
		addHuman(cs, addr, bal)
		acc, _ := cs.accounts.Get(addr)
		acc.LastActivityAt = longIdle
		addrs = append(addrs, addr)
	}
	return cs, addrs
}

func TestHandleHumans_BalanceIsDemurragedExactlyOnce(t *testing.T) {
	cs, addrs := idleHumansState([]float64{100, 250, 500, 1000, 5000})
	a := &APIServer{state: cs}
	out := doHumansRequest(t, a, "203.0.113.10:1", "")

	published := map[string]float64{}
	for _, h := range out["humans"].([]interface{}) {
		m := h.(map[string]interface{})
		published[m["address"].(string)] = m["balance"].(float64)
	}

	for _, addr := range addrs {
		acc, ok := cs.accounts.Get(addr)
		if !ok {
			t.Fatalf("account %s vanished from state", addr)
		}
		raw := acc.Balance.Float()
		want := effectiveBalance(acc).Float() // decayed once — the truth
		factor := want / raw                  // the decay factor for this idle period
		twice := want * factor                // what the bug published
		got := published[addr]

		// Guard the fixture first: if no decay applies, both the correct and
		// the buggy value are identical and the assertion below proves nothing.
		if math.Abs(want-raw) < 1e-9 {
			t.Fatalf("%s: no decay applied (raw=%.10f effective=%.10f) — fixture is "+
				"not past the grace period, so this test cannot detect double application",
				addr, raw, want)
		}
		if math.Abs(got-want) > 1e-9 {
			extra := ""
			if math.Abs(got-twice) < 1e-9 {
				extra = " — this is exactly the doubly-decayed figure"
			}
			t.Errorf("%s: published balance %.10f, want %.10f (decayed once)%s",
				addr, got, want, extra)
		}
	}
}

func TestHandleHumans_PayloadGiniEqualsChainGini(t *testing.T) {
	cs, _ := idleHumansState([]float64{100, 250, 500, 1000, 5000, 12000})
	a := &APIServer{state: cs}
	out := doHumansRequest(t, a, "203.0.113.11:1", "")

	var vals []float64
	for _, h := range out["humans"].([]interface{}) {
		m := h.(map[string]interface{})
		vals = append(vals, m["total_value_aeq"].(float64))
	}
	if len(vals) != 6 {
		t.Fatalf("got %d humans, want 6", len(vals))
	}

	fromPayload := giniOfPayload(vals)
	fromChain := cs.CalcGini()
	if math.Abs(fromPayload-fromChain) > 1e-9 {
		t.Fatalf("Lorenz basis disagrees with published Gini:\n"+
			"  from /api/humans total_value_aeq: %.10f\n"+
			"  from CalcGini (the number shown): %.10f\n"+
			"  difference:                       %.10f\n"+
			"The curve and the coefficient beside it must come from the same wealth.",
			fromPayload, fromChain, fromPayload-fromChain)
	}
}

// The LP half of humanAEQWealthLocked: AEQ parked as liquidity still belongs
// to the human, so total_value_aeq must include it — and must still reproduce
// the chain's Gini once it does.
func TestHandleHumans_PayloadGiniEqualsChainGini_WithLiquidity(t *testing.T) {
	cs, addrs := idleHumansState([]float64{100, 250, 500, 1000, 5000, 12000})
	cs.pool = &PoolState{
		ReserveAEQ:    NewDecimal(9000),
		ReserveTUSD:   NewDecimal(9000),
		TotalLPShares: NewDecimal(300),
	}
	// Two humans hold the pool; one of them is the poorest, so including LP
	// value genuinely changes the ordering the Lorenz curve depends on.
	for addr, shares := range map[string]float64{addrs[0]: 200, addrs[3]: 100} {
		acc, _ := cs.accounts.Get(addr)
		acc.LPShares = NewDecimal(shares)
	}

	a := &APIServer{state: cs}
	out := doHumansRequest(t, a, "203.0.113.12:1", "")

	var vals []float64
	sawLP := false
	for _, h := range out["humans"].([]interface{}) {
		m := h.(map[string]interface{})
		vals = append(vals, m["total_value_aeq"].(float64))
		if m["lp_value_aeq"].(float64) > 0 {
			sawLP = true
		}
	}
	if !sawLP {
		t.Fatal("no account reported LP value — fixture did not exercise the LP path")
	}

	fromPayload := giniOfPayload(vals)
	fromChain := cs.CalcGini()
	if math.Abs(fromPayload-fromChain) > 1e-9 {
		t.Fatalf("with liquidity, Lorenz basis %.10f != chain Gini %.10f (diff %.10f)",
			fromPayload, fromChain, fromPayload-fromChain)
	}
}
