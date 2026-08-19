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
	// 1e-9, not 1e-6. The old bound tolerated a full micro-AEQ, which is
	// exactly the size of the drift these rounds actually produce — so this
	// helper could never have caught the minting the daily distributions were
	// doing (see floor6's comment). At 1e-9 it catches a single micro-AEQ while
	// staying far above float64 noise on the magnitudes involved here.
	if math.Abs(after-before) > 1e-9 {
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

	// CORRECTION (Audit 2026-08-18): this block used to pin "it can only ever
	// BURN, never mint" as measured behaviour, and that was false. The comment
	// said the per-head amount was "rounded down"; the code used round6 and
	// NewDecimal, both math.Round, i.e. half away from zero. With a remainder
	// at or above half a micro-AEQ every share rounded UP, the credits summed
	// to more than the pool, and the pool was zeroed anyway — a mint. Measured:
	// a 0.000007 AEQ pool over two humans created one micro-AEQ, and the LP
	// round did it on every value probed. This fixture (100.000001 over 7)
	// happens to land on the rounding-down side, which is why it never showed.
	//
	// The distributions floor now (see floor6), so the claim below is finally
	// the truth rather than an assumption: the shares can never sum above the
	// pool, and the finalize that zeroes it can only destroy the remainder.
	// The burn bound is unchanged at 1e-6 x recipients.
	//
	// The old block's CONCLUSION still holds — at n=15 this drift would need
	// ~43 million rounds to reach the +305 AEQ measured live, so this path
	// never explained that gap. Only its reasoning was wrong.
	delta := after - before
	if delta > 1e-9 {
		t.Errorf("the UBI round CREATED %+.6f AEQ — a distribution may only move money", delta)
	}
	if maxBurn := 1e-6 * float64(len(shares)); -delta > maxBurn {
		t.Errorf("the UBI round destroyed %.9f AEQ, more than the %.9f rounding bound "+
			"(1 micro-AEQ per recipient) — that is no longer rounding", -delta, maxBurn)
	}
}

// The fixture above lands on the rounding-DOWN side by luck, which is how a
// minting bug sat behind a test asserting the opposite for weeks. These are the
// pool/recipient combinations where total_micro / n has a fractional part at or
// above one half — the ones that rounded UP and created money. Each is checked
// for exact non-creation, not merely for a small delta.
func TestSupplyConservation_UBIDistribution_RemaindersThatUsedToMint(t *testing.T) {
	for _, tc := range []struct {
		poolAEQ float64
		humans  int
	}{
		{0.000005, 2}, // 5 micro / 2 = 2.5 -> rounded to 3 each -> 6 > 5
		{0.000007, 2}, // 3.5 -> 4 each -> 8 > 7
		{0.000011, 2}, // 5.5 -> 6 each -> 12 > 11
		{0.000003, 2}, // 1.5 -> 2 each -> 4 > 3
		{0.000025, 10},
		{1.0000005, 2},
	} {
		cs := newTestState()
		for i := 0; i < tc.humans; i++ {
			addr := fmt.Sprintf("0xr%02d", i)
			cs.accounts.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(1000), IsHuman: true})
			cs.humanCount++
		}
		cs.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(tc.poolAEQ)})
		cs.pool = &PoolState{}

		before := totalAEQ(cs)
		cs.mu.Lock()
		_, err := cs.distributeUBIPoolLocked(t.Context())
		cs.mu.Unlock()
		if err != nil {
			t.Fatalf("pool %.6f over %d: %v", tc.poolAEQ, tc.humans, err)
		}
		if delta := totalAEQ(cs) - before; delta > 1e-9 {
			t.Errorf("pool %.6f AEQ over %d humans CREATED %+.9f AEQ — the shares summed "+
				"above the pool and the pool was zeroed anyway", tc.poolAEQ, tc.humans, delta)
		}
	}
}

// Same shape for the LP round, which minted on every value probed before the
// distributions were changed to floor.
func TestSupplyConservation_LPDistribution_RemaindersThatUsedToMint(t *testing.T) {
	for _, poolAEQ := range []float64{0.000003, 0.000005, 0.000007, 0.000011} {
		cs := newTestState()
		for i := 0; i < 2; i++ {
			addr := fmt.Sprintf("0xl%02d", i)
			acc := &AccountState{Address: addr, Balance: NewDecimal(1000), IsHuman: true, LPShares: NewDecimal(100)}
			cs.accounts.Set(addr, acc)
			cs.humanCount++
		}
		cs.accounts.Set(lpPoolAddr, &AccountState{Address: lpPoolAddr, Balance: NewDecimal(poolAEQ)})
		cs.pool = &PoolState{TotalLPShares: NewDecimal(200)}

		before := totalAEQ(cs)
		cs.mu.Lock()
		_, err := cs.distributeLPPoolLocked(t.Context())
		cs.mu.Unlock()
		if err != nil {
			t.Fatalf("LP pool %.6f: %v", poolAEQ, err)
		}
		if delta := totalAEQ(cs) - before; delta > 1e-9 {
			t.Errorf("LP pool %.6f AEQ over 2 holders CREATED %+.9f AEQ", poolAEQ, delta)
		}
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

// The paths a survey on 2026-08-19 found had no conservation test of their own.
//
// The existing tests cover transfer, registration, swap, liquidity, demurrage,
// the wealth cap and the three daily distributions. Mapping every site in
// state.go that increases an AEQ balance turned up four more that no test held
// to the invariant: the two escrow helpers, the swap-fee distribution, and the
// one-time stranded-fee migration. A path with no test is exactly where the
// last two minting bugs were found.

// TestSupplyConservation_TUsdFeeConversion covers the conversion the stranded-
// fee migration and every ongoing swap fee run through.
//
// Written while chasing a suspected rounding asymmetry: the function debits the
// reserve by aeqOut and returns round6(aeqOut), which looks like it could
// credit more than the pool gave up. It cannot. aeqOut comes from AMMSwapOut,
// which returns a Decimal — an int64 of micro-units — so the value is always
// exactly on the micro-grid and round6 is a no-op there.
//
// Kept anyway, because the reasoning only holds while that stays true. If
// anyone ever makes AMMSwapOut return a raw float, or routes an off-grid amount
// through here, the two sides stop matching and this test says so.
func TestSupplyConservation_TUsdFeeConversion(t *testing.T) {
	// Values chosen so the AMM output lands off a micro-AEQ boundary, which is
	// where the asymmetry showed.
	for _, fee := range []float64{0.1, 0.333333, 1.0, 0.000007, 2.5} {
		cs := newTestState()
		cs.pool = &PoolState{
			ReserveAEQ:  NewDecimal(10000),
			ReserveTUSD: NewDecimal(3000),
		}
		acc := &AccountState{Address: "0xfee", Balance: NewDecimal(0)}
		cs.accounts.Set(acc.Address, acc)

		before := totalAEQ(cs)
		cs.mu.Lock()
		out, ok := cs.convertTUsdFeeToAEQLocked(fee)
		cs.mu.Unlock()
		if !ok {
			continue
		}
		acc.Balance = acc.Balance.Add(NewDecimal(out))

		if delta := totalAEQ(cs) - before; math.Abs(delta) > 1e-9 {
			t.Errorf("converting %.6f tUSD moved the AEQ supply by %+.9f — the pool must give up "+
				"exactly what the account is credited", fee, delta)
		}
	}
}

// TestSupplyConservation_EscrowLiquidation: the inactivity sweep burns LP
// shares into a balance. It moves value out of the pool, so it must not create
// any on the way.
func TestSupplyConservation_EscrowLiquidation(t *testing.T) {
	cs := newTestState()
	cs.pool = &PoolState{
		ReserveAEQ:    NewDecimal(5000),
		ReserveTUSD:   NewDecimal(5000),
		TotalLPShares: NewDecimal(1000),
	}
	acc := &AccountState{
		Address:  "0xescrow",
		Balance:  NewDecimal(100),
		IsHuman:  true,
		LPShares: NewDecimal(400),
	}
	cs.accounts.Set(acc.Address, acc)
	cs.humanCount++

	assertConserved(t, cs, "escrow LP liquidation", func() {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		if _, _, _, err := cs.liquidateLPSharesForEscrowLocked(t.Context(), acc, 400); err != nil {
			t.Fatalf("liquidate: %v", err)
		}
	})
}

// TestSupplyConservation_SwapFeeDistribution: the fee is split four ways. Every
// share must come out of the fee, not out of nowhere — the daily distributions
// got exactly this wrong by rounding each share up and zeroing the pool anyway.
func TestSupplyConservation_SwapFeeDistribution(t *testing.T) {
	for _, fee := range []float64{0.000003, 0.000007, 0.1, 1.0} {
		cs := newTestState()
		cs.pool = &PoolState{ReserveAEQ: NewDecimal(10000), ReserveTUSD: NewDecimal(10000)}

		payer := &AccountState{Address: "0xpayer", Balance: NewDecimal(1000), IsHuman: true}
		cs.accounts.Set(payer.Address, payer)
		cs.humanCount++

		before := totalAEQ(cs)
		// The fee has already been taken from the payer by the caller; model
		// that, then distribute it.
		payer.Balance = payer.Balance.Sub(NewDecimal(fee))
		cs.mu.Lock()
		err := cs.distributeSwapFeeCtx(t.Context(), fee, true)
		cs.mu.Unlock()
		if err != nil {
			t.Fatalf("fee %.6f: %v", fee, err)
		}
		if delta := totalAEQ(cs) - before; delta > 1e-9 {
			t.Errorf("distributing a %.6f AEQ fee CREATED %+.9f AEQ — the four shares summed "+
				"above the fee", fee, delta)
		}
	}
}

// TestSupplyBreakdownIsRefusedWithoutADatabase: the breakdown reads the ledger,
// so with no database it must say so rather than report zeros.
//
// Zeros would be actively misleading here — "non_humans: 0.000000" reads like a
// measurement that rules out an explanation, when nothing was measured at all.
func TestSupplyBreakdownIsRefusedWithoutADatabase(t *testing.T) {
	cs := &ChainState{}
	if _, err := cs.supplyBreakdown(); err == nil {
		t.Error("a breakdown was produced with no database — zeros would read as evidence")
	}

	out := cs.SupplyReconciliation()
	if _, hasBreakdown := out["breakdown"]; hasBreakdown {
		t.Error("SupplyReconciliation published a breakdown it could not measure")
	}
}
