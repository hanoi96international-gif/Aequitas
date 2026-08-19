package keeper

import (
	"math"
	"testing"
)

// The four-way pool split is the busiest money path in the system: every swap
// fee, every demurrage settlement and every wealth-cap overflow goes through
// distributeSwapFee. It used to compute {fee*0.40, fee*0.30, fee*0.20,
// fee*0.10} and round each independently, so the four credits did not have to
// sum back to the fee that had already been debited. When they summed higher,
// the difference was money that never existed.
//
// It hid behind assertConserved's 1e-6 tolerance — the same order of magnitude
// as the drift itself — and only surfaced when that bound was tightened while
// fixing the daily distribution rounding.
//
// The property is exact, so it is tested exactly: the four shares must sum to
// the fee in micro-AEQ, with no epsilon anywhere.
func TestFeeSplit_SumsToExactlyTheFee(t *testing.T) {
	// Values chosen so that 40/30/20/10 percent each land on a different side
	// of a micro-AEQ boundary — the shapes that used to drift.
	fees := []float64{
		0.000001, 0.000003, 0.000007, 0.000011, 0.000013,
		0.0000001, // below one micro entirely
		0.123457, 1.000001, 33.333333, 65000.000001,
		7.7, 0.999999, 12.345679,
	}
	for _, fee := range fees {
		cs := newTestState()
		cs.pool = &PoolState{}

		cs.mu.Lock()
		err := cs.distributeSwapFeeCtx(t.Context(), fee, true)
		cs.mu.Unlock()
		if err != nil {
			t.Fatalf("fee %.6f: %v", fee, err)
		}

		// Summed in micro-units, not float64. The ledger is exact int64; adding
		// four .Float() values back up reintroduces representation error that
		// has nothing to do with the property under test.
		var creditedMicro int64
		for _, addr := range []string{validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr} {
			if acc, ok := cs.accounts.Get(addr); ok {
				creditedMicro += acc.Balance.Micro()
			}
		}
		wantMicro := NewDecimal(fee).Micro()
		if creditedMicro != wantMicro {
			t.Errorf("fee %.6f: the four pools received %d micro-AEQ, the payer was debited %d "+
				"— a %+d micro-AEQ discrepancy that is created or destroyed money",
				fee, creditedMicro, wantMicro, creditedMicro-wantMicro)
		}
	}
}

// The same identity must hold on the tUSD side, which uses the same helper.
func TestFeeSplit_SumsToExactlyTheFee_TUsd(t *testing.T) {
	for _, fee := range []float64{0.000001, 0.000003, 0.000007, 0.000011, 1.000001, 33.333333} {
		cs := newTestState()
		cs.pool = &PoolState{} // no reserves → no tUSD→AEQ conversion, stays in tUSD
		cs.mu.Lock()
		err := cs.distributeSwapFeeCtx(t.Context(), fee, false)
		cs.mu.Unlock()
		if err != nil {
			t.Fatalf("fee %.6f: %v", fee, err)
		}
		var creditedMicro int64
		for _, addr := range []string{validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr} {
			if acc, ok := cs.accounts.Get(addr); ok {
				creditedMicro += acc.TUsdBalance.Micro()
			}
		}
		if wantMicro := NewDecimal(fee).Micro(); creditedMicro != wantMicro {
			t.Errorf("tUSD fee %.6f: pools received %d micro, payer debited %d (%+d)",
				fee, creditedMicro, wantMicro, creditedMicro-wantMicro)
		}
	}
}

// The split must stay recognisably 40/30/20/10 — exactness must not have been
// bought by dumping everything into one pool. The treasury absorbs the integer
// remainder, so it may exceed its nominal share by at most three micro-AEQ.
func TestFeeSplit_KeepsTheIntendedProportions(t *testing.T) {
	const fee = 1000.0
	cs := newTestState()
	cs.pool = &PoolState{}
	cs.mu.Lock()
	if err := cs.distributeSwapFeeCtx(t.Context(), fee, true); err != nil {
		cs.mu.Unlock()
		t.Fatalf("distribute: %v", err)
	}
	cs.mu.Unlock()

	for _, want := range []struct {
		addr  string
		share float64
	}{
		{validatorsPoolAddr, 400},
		{lpPoolAddr, 300},
		{ubiPoolAddr, 200},
		{treasuryPoolAddr, 100},
	} {
		acc, ok := cs.accounts.Get(want.addr)
		if !ok {
			t.Fatalf("%s was never credited", want.addr)
		}
		if got := acc.Balance.Float(); math.Abs(got-want.share) > 3e-6 {
			t.Errorf("%s received %.6f AEQ of a %.0f fee, expected about %.0f",
				want.addr, got, fee, want.share)
		}
	}
}
