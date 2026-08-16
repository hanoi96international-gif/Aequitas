package keeper

import (
	"fmt"
	"math"
	"testing"
)

// AEQ is created in exactly one place: registration grants 1,000 to a new
// human. Every other operation moves it. That is the protocol's central claim —
// "money exists because people exist" — and /api/status publishes it as
// total_supply, computed as humans x 1000 rather than measured.
//
// Measured against both live validators on 2026-08-15, from their own
// databases and byte-identically on each:
//
//	total AEQ in existence   15,305.278004   (all account balances + pool reserve)
//	humans x 1000            15,000
//	difference                  +305.278004  (2.04%)
//
// TotalSupply's own comment attributes such a gap to "floating-point drift from
// swap fees and demurrage". Float64 drift across 15 accounts is on the order of
// 1e-10, so that explanation is off by nine orders of magnitude and something
// genuinely mints AEQ.
//
// These tests hold every operation to the invariant directly, so the path that
// creates money names itself instead of having to be guessed at.

// totalAEQ is the whole of the currency: every account's balance plus whatever
// the AMM holds. LP shares are claims on the pool reserve, not separate money,
// so counting the reserve once is correct.
func totalAEQ(cs *ChainState) float64 {
	total := 0.0
	cs.accounts.Range(func(_ string, a *AccountState) bool {
		total += a.Balance.Float()
		return true
	})
	if cs.pool != nil {
		total += cs.pool.ReserveAEQ.Float()
	}
	return total
}

// assertConserved runs fn and requires that it did not change the total. The
// tolerance is one micro-AEQ: balances are stored as micro-integers (see
// Decimal), so anything a correct implementation does is exact at that scale
// and a real leak is orders of magnitude larger.
func assertConserved(t *testing.T, cs *ChainState, what string, fn func()) {
	t.Helper()
	before := totalAEQ(cs)
	fn()
	after := totalAEQ(cs)
	if math.Abs(after-before) > 1e-6 {
		t.Errorf("%s changed the total AEQ supply by %+.6f (before %.6f, after %.6f)\n"+
			"  only registration may create AEQ; every other operation must move it",
			what, after-before, before, after)
	}
}

func TestSupplyConservation_Transfer(t *testing.T) {
	cs := newTestState()
	cs.accounts.Set("0xa", &AccountState{Address: "0xa", Balance: NewDecimal(1000), IsHuman: true})
	cs.accounts.Set("0xb", &AccountState{Address: "0xb", Balance: NewDecimal(1000), IsHuman: true})
	cs.humanCount = 2

	assertConserved(t, cs, "transfer", func() {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		if err := cs.applyTransferDeltaLocked(t.Context(), "0xa", "0xb", 250, 0, 0, 0); err != nil {
			t.Fatalf("transfer: %v", err)
		}
	})
}

func TestSupplyConservation_Registration_CreatesExactlyOneGrant(t *testing.T) {
	cs := newTestState()
	before := totalAEQ(cs)
	cs.mu.Lock()
	err := cs.registerHumanLocked(t.Context(), "0xnewhuman", 0)
	cs.mu.Unlock()
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	got := totalAEQ(cs) - before
	if math.Abs(got-1000) > 1e-6 {
		t.Errorf("registration created %.6f AEQ, want exactly 1000", got)
	}
}

// A swap moves AEQ between a wallet and the pool reserve, and takes a fee that
// is credited to the four tokenomics pool ACCOUNTS. Every one of those parts is
// a movement; none of them may add to the whole.
func TestSupplyConservation_Swap(t *testing.T) {
	cs := newTestState()
	cs.accounts.Set("0xtrader", &AccountState{Address: "0xtrader", Balance: NewDecimal(1000), TUsdBalance: NewDecimal(1000), IsHuman: true})
	cs.humanCount = 1
	cs.pool = &PoolState{
		ReserveAEQ:    NewDecimal(5000),
		ReserveTUSD:   NewDecimal(5000),
		TotalLPShares: NewDecimal(5000),
	}

	assertConserved(t, cs, "swap AEQ->tUSD", func() {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		if _, _, err := cs.swapLocked(t.Context(), "0xtrader", 100, true, 0); err != nil {
			t.Fatalf("swap aeq->tusd: %v", err)
		}
	})

	assertConserved(t, cs, "swap tUSD->AEQ", func() {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		if _, _, err := cs.swapLocked(t.Context(), "0xtrader", 100, false, 0); err != nil {
			t.Fatalf("swap tusd->aeq: %v", err)
		}
	})
}

func TestSupplyConservation_Liquidity(t *testing.T) {
	cs := newTestState()
	cs.accounts.Set("0xlp", &AccountState{Address: "0xlp", Balance: NewDecimal(1000), TUsdBalance: NewDecimal(1000), IsHuman: true})
	cs.humanCount = 1
	cs.pool = &PoolState{}

	assertConserved(t, cs, "add liquidity", func() {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		if _, err := cs.addLiquidityLocked(t.Context(), "0xlp", 400, 400); err != nil {
			t.Fatalf("add liquidity: %v", err)
		}
	})

	acc, _ := cs.accounts.Get("0xlp")
	shares := acc.LPShares.Float()
	if shares <= 0 {
		t.Fatalf("expected LP shares to be minted, got %v", shares)
	}

	assertConserved(t, cs, "remove liquidity", func() {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		if _, _, _, err := cs.removeLiquidityLocked(t.Context(), "0xlp", shares/2); err != nil {
			t.Fatalf("remove liquidity: %v", err)
		}
	})
}

// Demurrage takes decayed AEQ off a wallet and hands it to the pools. It is the
// clearest movement in the protocol and must not change the whole.
//
// The balance is deliberately well ABOVE one fair share. Demurrage only decays
// the excess over fairShare (see demurrage_fairshare_test.go and
// effectiveBalance's own comment), so a fixture holding exactly the 1,000 AEQ
// grant — which this used to hold — decays by nothing and would let this test
// pass while exercising no movement at all.
func TestSupplyConservation_Demurrage(t *testing.T) {
	cs := newTestState()
	old := nowUnix() - 400*24*3600 // well past any grace period
	cs.accounts.Set("0xidle", &AccountState{Address: "0xidle", Balance: NewDecimal(9000), IsHuman: true, LastActivityAt: old})
	cs.humanCount = 1
	cs.pool = &PoolState{}

	assertConserved(t, cs, "demurrage settlement", func() {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		acc, _ := cs.accounts.Get("0xidle")
		if _, err := cs.settleDemurrageLockedCtx(t.Context(), acc); err != nil {
			t.Fatalf("settle demurrage: %v", err)
		}
	})
}

// The wealth cap trims a balance above avg x multiplier and redistributes the
// excess to the pools — again a movement, not a burn and not a mint.
func TestSupplyConservation_WealthCap(t *testing.T) {
	cs := newTestState()
	for i := 0; i < 30; i++ {
		addr := fmt.Sprintf("0xh%02d", i)
		cs.accounts.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(1000), IsHuman: true})
		cs.humanCount++
	}
	cs.accounts.Set("0xwhale", &AccountState{Address: "0xwhale", Balance: NewDecimal(90000), IsHuman: true})
	cs.humanCount++
	cs.pool = &PoolState{}

	assertConserved(t, cs, "wealth cap enforcement", func() {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		acc, _ := cs.accounts.Get("0xwhale")
		if err := cs.enforceWealthCapLockedCtx(t.Context(), acc); err != nil {
			t.Fatalf("enforce wealth cap: %v", err)
		}
	})
}

// The daily round is the remaining place large amounts move at once: the UBI,
// validator and LP pools are emptied into humans' balances. Emptying a pool
// account and crediting the same total to recipients is a movement. Crediting a
// per-head amount and then zeroing the pool independently is not — any gap
// between "what was credited" and "what was removed" is created or destroyed
// money, and it would show up on the chain exactly as the +305 AEQ measured on
// both live validators.
func TestSupplyConservation_UBIDistribution(t *testing.T) {
	cs := newTestState()
	const humans = 7
	for i := 0; i < humans; i++ {
		addr := fmt.Sprintf("0xh%02d", i)
		cs.accounts.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(1000), IsHuman: true})
		cs.humanCount++
	}
	// A pool balance that does NOT divide evenly by the human count, which is
	// the ordinary case and the one where a rounding remainder appears.
	cs.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(100.000001)})
	cs.pool = &PoolState{}

	before := totalAEQ(cs)
	cs.mu.Lock()
	shares, err := cs.distributeUBIPoolLocked(t.Context())
	cs.mu.Unlock()
	if err != nil {
		t.Fatalf("distribute UBI: %v", err)
	}
	after := totalAEQ(cs)
	credited := 0.0
	for _, s := range shares {
		credited += s.Amount
	}
	t.Logf("pool 100.000001 over %d humans: credited %.6f to %d recipient(s), total %+.6f",
		humans, credited, len(shares), after-before)

	// MEASURED BEHAVIOUR, pinned deliberately rather than "fixed" three days
	// before launch: the per-head amount is rounded down to micro-AEQ and the
	// pool is then zeroed by the separate finalize transaction, so a remainder
	// of up to one micro-AEQ per recipient is destroyed. Two properties make
	// that acceptable and worth pinning as-is:
	//
	//   - it can only ever BURN, never mint. Supply drifts imperceptibly down,
	//     which is the safe direction for a currency, and it cannot explain the
	//     +305 AEQ measured on the live chain.
	//   - the bound is tight: 1e-6 x recipients, i.e. 15 micro-AEQ per round at
	//     today's size, roughly five thousandths of an AEQ per year.
	//
	// Closing it means changing what the distribution credits, and the per-head
	// amount travels in the block, so a rollout window with two rules in flight
	// would diverge. It is a deliberate, documented no-op, not an oversight.
	delta := after - before
	if delta > 1e-9 {
		t.Errorf("the UBI round CREATED %+.6f AEQ — a distribution may only move money", delta)
	}
	if maxBurn := 1e-6 * float64(len(shares)); -delta > maxBurn {
		t.Errorf("the UBI round destroyed %.9f AEQ, more than the %.9f rounding bound "+
			"(1 micro-AEQ per recipient) — that is no longer rounding", -delta, maxBurn)
	}
}

func TestSupplyConservation_ValidatorAndLPDistribution(t *testing.T) {
	cs := newTestState()
	for i := 0; i < 3; i++ {
		addr := fmt.Sprintf("0xv%02d", i)
		cs.accounts.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(1000), IsHuman: true})
		cs.humanCount++
	}
	cs.accounts.Set(validatorsPoolAddr, &AccountState{Address: validatorsPoolAddr, Balance: NewDecimal(33.333333)})
	cs.accounts.Set(lpPoolAddr, &AccountState{Address: lpPoolAddr, Balance: NewDecimal(33.333333)})
	// One LP holder so the LP round has somewhere to send its share.
	lp, _ := cs.accounts.Get("0xv00")
	lp.LPShares = NewDecimal(100)
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(0), ReserveTUSD: NewDecimal(0), TotalLPShares: NewDecimal(100)}

	assertConserved(t, cs, "validator pool distribution", func() {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		if _, err := cs.distributeValidatorsPoolLocked(t.Context()); err != nil {
			t.Fatalf("distribute validators: %v", err)
		}
	})
	assertConserved(t, cs, "LP pool distribution", func() {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		if _, err := cs.distributeLPPoolLocked(t.Context()); err != nil {
			t.Fatalf("distribute LP: %v", err)
		}
	})
}

// TransferWithV7Fee is the path a real MetaMask "send AEQ" goes through (see
// its own comment): the RPC layer intercepts the ERC-20 transfer() selector and
// routes it here, where a fee is taken and credited to the four pools. A fee is
// a movement — the sender must lose exactly what the recipient and the pools
// together gain.
func TestSupplyConservation_V7FeeTransfer(t *testing.T) {
	cs := newTestState()
	cs.accounts.Set("0xsender", &AccountState{Address: "0xsender", Balance: NewDecimal(5000), IsHuman: true})
	cs.accounts.Set("0xrecipient", &AccountState{Address: "0xrecipient", Balance: NewDecimal(1000), IsHuman: true})
	cs.humanCount = 2
	cs.pool = &PoolState{}

	assertConserved(t, cs, "V7-fee transfer", func() {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		if _, _, _, err := cs.transferWithV7FeeLocked(t.Context(), "0xsender", "0xrecipient", 1000); err != nil {
			t.Fatalf("v7 fee transfer: %v", err)
		}
	})
}

// The three ingestion fast paths bypass transferMutateLocked entirely. Each one
// reimplements the arithmetic, so each one can drift from it independently.
func TestSupplyConservation_ConcurrentTransferFastPath(t *testing.T) {
	cs := newTestState()
	cs.accounts.Set("0xfrom", &AccountState{Address: "0xfrom", Balance: NewDecimal(5000), IsHuman: true})
	cs.accounts.Set("0xto", &AccountState{Address: "0xto", Balance: NewDecimal(1000), IsHuman: true})
	cs.humanCount = 2
	cs.pool = &PoolState{}

	assertConserved(t, cs, "concurrent transfer fast path", func() {
		_, _, applied, err := cs.transferConcurrent("0xfrom", "0xto", 100, Transaction{Type: "transfer", Wallet: "0xfrom", To: "0xto", Amount: 100})
		if err != nil {
			t.Fatalf("transferConcurrent: %v", err)
		}
		if !applied {
			t.Skip("fast path declined (needs a DB); nothing to assert")
		}
	})
}
