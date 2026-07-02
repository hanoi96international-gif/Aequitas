package keeper

import "testing"

var feePoolAddrs = []string{validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr}

// TestSwapFee_TUsdConvertedToAEQ verifies a tUSD-denominated swap fee is
// converted to AEQ through the pool before crediting the four tokenomics pools,
// so the pools hold the AEQ they actually distribute (not stranded tUSD).
func TestSwapFee_TUsdConvertedToAEQ(t *testing.T) {
	cs := newTestState()
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(1000), ReserveTUSD: NewDecimal(1000), TotalLPShares: NewDecimal(0)}

	if err := cs.distributeSwapFee(10, false); err != nil { // 10 tUSD fee (a tUSD->AEQ buy)
		t.Fatalf("distributeSwapFee: %v", err)
	}

	var totalAEQ float64
	for _, addr := range feePoolAddrs {
		acc := cs.accounts[addr]
		if acc == nil {
			t.Fatalf("pool %s was not credited at all", addr)
		}
		if acc.TUsdBalance.Float() != 0 {
			t.Errorf("pool %s got tUSD %v — the fee should have been converted to AEQ", addr, acc.TUsdBalance.Float())
		}
		totalAEQ += acc.Balance.Float()
	}
	if totalAEQ <= 0 {
		t.Fatalf("pools received no AEQ — conversion did not happen")
	}
	// The fee's tUSD entered the pool's tUSD reserve; the AEQ it bought left the
	// AEQ reserve.
	if got := cs.pool.ReserveTUSD.Float(); got != 1010 {
		t.Errorf("reserveTUSD: want 1010 (1000 + 10 fee), got %v", got)
	}
	if cs.pool.ReserveAEQ.Float() >= 1000 {
		t.Errorf("reserveAEQ must shrink as AEQ leaves for the pools, got %v", cs.pool.ReserveAEQ.Float())
	}
	// Conservation: AEQ credited to the pools equals AEQ removed from the reserve
	// (within split-rounding dust).
	aeqRemoved := 1000 - cs.pool.ReserveAEQ.Float()
	if diff := totalAEQ - aeqRemoved; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("distributed AEQ (%v) must equal AEQ removed from reserve (%v)", totalAEQ, aeqRemoved)
	}
	// 40/30/20/10 split (approximate — each share is rounded to 6 decimals).
	checkShare := func(addr string, frac float64) {
		got := cs.accounts[addr].Balance.Float()
		want := totalAEQ * frac
		if diff := got - want; diff < -0.001 || diff > 0.001 {
			t.Errorf("pool %s: want ~%v (%.0f%%), got %v", addr, want, frac*100, got)
		}
	}
	checkShare(validatorsPoolAddr, 0.40)
	checkShare(lpPoolAddr, 0.30)
	checkShare(ubiPoolAddr, 0.20)
	checkShare(treasuryPoolAddr, 0.10)
}

// TestSwapFee_ConversionDeterministic proves the conversion reaches bit-
// identical state from identical inputs — the property that keeps the primary
// (swapLocked) and a replaying secondary (applySwapDeltaLocked) in consensus,
// since both call distributeSwapFee with the pool reserves in the same state.
func TestSwapFee_ConversionDeterministic(t *testing.T) {
	mk := func() *ChainState {
		cs := newTestState()
		cs.pool = &PoolState{ReserveAEQ: NewDecimal(1234), ReserveTUSD: NewDecimal(5678), TotalLPShares: NewDecimal(0)}
		return cs
	}
	a, b := mk(), mk()
	if err := a.distributeSwapFee(7, false); err != nil {
		t.Fatalf("a: %v", err)
	}
	if err := b.distributeSwapFee(7, false); err != nil {
		t.Fatalf("b: %v", err)
	}
	if a.pool.ReserveAEQ != b.pool.ReserveAEQ || a.pool.ReserveTUSD != b.pool.ReserveTUSD {
		t.Fatalf("reserves diverged: a=(%v,%v) b=(%v,%v)",
			a.pool.ReserveAEQ.Float(), a.pool.ReserveTUSD.Float(), b.pool.ReserveAEQ.Float(), b.pool.ReserveTUSD.Float())
	}
	for _, addr := range feePoolAddrs {
		if a.accounts[addr].Balance != b.accounts[addr].Balance {
			t.Errorf("pool %s balance diverged: a=%v b=%v", addr, a.accounts[addr].Balance.Float(), b.accounts[addr].Balance.Float())
		}
	}
}

// TestSwapFee_EmptyPoolFallsBackToTUsd verifies that if the pool can't price the
// fee (no liquidity), the fee is still credited — as tUSD — rather than lost.
// This branch is also deterministic (reserves are identical on all nodes).
func TestSwapFee_EmptyPoolFallsBackToTUsd(t *testing.T) {
	cs := newTestState()
	cs.pool = &PoolState{} // zero reserves — cannot price a swap

	if err := cs.distributeSwapFee(10, false); err != nil {
		t.Fatalf("distributeSwapFee: %v", err)
	}
	var totalTUsd, totalAEQ float64
	for _, addr := range feePoolAddrs {
		totalTUsd += cs.accounts[addr].TUsdBalance.Float()
		totalAEQ += cs.accounts[addr].Balance.Float()
	}
	if totalTUsd <= 0 {
		t.Fatalf("fee was lost when the pool is empty; it must fall back to a tUSD credit")
	}
	if totalAEQ != 0 {
		t.Errorf("no AEQ should be credited when the pool can't price the fee, got %v", totalAEQ)
	}
	if cs.pool.ReserveAEQ.Float() != 0 || cs.pool.ReserveTUSD.Float() != 0 {
		t.Errorf("empty pool reserves must be untouched, got AEQ=%v tUSD=%v", cs.pool.ReserveAEQ.Float(), cs.pool.ReserveTUSD.Float())
	}
}

// TestSwapFee_AEQFeeCreditedDirectly verifies an AEQ-denominated fee (an
// AEQ->tUSD sell, or a demurrage/wealth-cap credit) is still credited directly
// as AEQ with no conversion and no reserve change.
func TestSwapFee_AEQFeeCreditedDirectly(t *testing.T) {
	cs := newTestState()
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(1000), ReserveTUSD: NewDecimal(1000), TotalLPShares: NewDecimal(0)}

	if err := cs.distributeSwapFee(10, true); err != nil { // AEQ fee
		t.Fatalf("distributeSwapFee: %v", err)
	}
	if cs.pool.ReserveAEQ.Float() != 1000 || cs.pool.ReserveTUSD.Float() != 1000 {
		t.Errorf("an AEQ fee must not touch reserves, got AEQ=%v tUSD=%v", cs.pool.ReserveAEQ.Float(), cs.pool.ReserveTUSD.Float())
	}
	if got := cs.accounts[ubiPoolAddr].Balance.Float(); got != 2 { // 20% of 10
		t.Errorf("ubi pool: want 2 AEQ (20%% of 10), got %v", got)
	}
	if got := cs.accounts[ubiPoolAddr].TUsdBalance.Float(); got != 0 {
		t.Errorf("ubi pool must hold no tUSD for an AEQ fee, got %v", got)
	}
}
