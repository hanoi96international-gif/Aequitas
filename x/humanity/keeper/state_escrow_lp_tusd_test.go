package keeper

import "testing"

// Regression tests for the beta-launch audit (2026-07-05) fix: an inactive
// human whose wealth lived in LP shares or tUSD (liquid AEQ Balance == 0)
// used to be silently skipped by checkAndMoveToEscrowLocked forever — never
// escrowed, never recycled to the UBI pool. Fixed by liquidating LP shares
// and converting tUSD into AEQ before the existing (AEQ-only) escrow
// capture, via liquidateLPSharesForEscrowLocked/convertTUsdForEscrowLocked —
// shared between the primary (guardian.go's checkAndMoveToEscrowLocked) and
// the secondary replay path (applyEscrowMoveDeltaLocked) so the two can
// never independently compute a different result.

func TestLiquidateLPSharesForEscrowLocked_BurnsProportionalShare(t *testing.T) {
	cs := newTestState()
	acc := &AccountState{Address: "0xlp", IsHuman: true, LPShares: NewDecimal(50)}
	cs.accounts.Set(acc.Address, acc)
	cs.pool = &PoolState{
		ReserveAEQ:    NewDecimal(1000),
		ReserveTUSD:   NewDecimal(2000),
		TotalLPShares: NewDecimal(100),
	}

	burned, outAEQ, outTUSD, err := cs.liquidateLPSharesForEscrowLocked(acc, acc.LPShares.Float())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if burned != 50 {
		t.Errorf("want burned=50, got %v", burned)
	}
	// 50/100 = 50% of reserves.
	if outAEQ != 500 || outTUSD != 1000 {
		t.Errorf("want outAEQ=500 outTUSD=1000, got %v %v", outAEQ, outTUSD)
	}
	if acc.LPShares.Float() != 0 {
		t.Errorf("want acc.LPShares=0, got %v", acc.LPShares.Float())
	}
	if acc.Balance.Float() != 500 || acc.TUsdBalance.Float() != 1000 {
		t.Errorf("want acc credited 500 AEQ + 1000 tUSD, got %v AEQ + %v tUSD", acc.Balance.Float(), acc.TUsdBalance.Float())
	}
	if cs.pool.ReserveAEQ.Float() != 500 || cs.pool.ReserveTUSD.Float() != 1000 || cs.pool.TotalLPShares.Float() != 50 {
		t.Errorf("want pool halved, got reserveAEQ=%v reserveTUSD=%v totalShares=%v",
			cs.pool.ReserveAEQ.Float(), cs.pool.ReserveTUSD.Float(), cs.pool.TotalLPShares.Float())
	}
}

func TestLiquidateLPSharesForEscrowLocked_NoOpWhenPoolEmpty(t *testing.T) {
	cs := newTestState()
	acc := &AccountState{Address: "0xlp", IsHuman: true, LPShares: NewDecimal(10)}
	cs.accounts.Set(acc.Address, acc)
	cs.pool = &PoolState{}

	burned, outAEQ, outTUSD, err := cs.liquidateLPSharesForEscrowLocked(acc, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if burned != 0 || outAEQ != 0 || outTUSD != 0 {
		t.Errorf("want no-op (0,0,0), got %v %v %v", burned, outAEQ, outTUSD)
	}
	if acc.LPShares.Float() != 10 {
		t.Errorf("want acc.LPShares untouched at 10, got %v", acc.LPShares.Float())
	}
}

func TestConvertTUsdForEscrowLocked_ConvertsAtAMMRate(t *testing.T) {
	cs := newTestState()
	acc := &AccountState{Address: "0xtusd", IsHuman: true, TUsdBalance: NewDecimal(100)}
	cs.accounts.Set(acc.Address, acc)
	cs.pool = &PoolState{
		ReserveAEQ:  NewDecimal(10000),
		ReserveTUSD: NewDecimal(10000),
	}

	outAEQ, ok, err := cs.convertTUsdForEscrowLocked(acc, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for a well-liquidified pool")
	}
	if outAEQ <= 0 {
		t.Errorf("want positive AEQ output, got %v", outAEQ)
	}
	if acc.TUsdBalance.Float() != 0 {
		t.Errorf("want acc.TUsdBalance=0 after full conversion, got %v", acc.TUsdBalance.Float())
	}
	if acc.Balance.Float() != round6(outAEQ) {
		t.Errorf("want acc.Balance credited with outAEQ=%v, got %v", outAEQ, acc.Balance.Float())
	}
	// Pool reserves should reflect the trade (tUSD in, AEQ out).
	if cs.pool.ReserveTUSD.Float() <= 10000 {
		t.Errorf("want ReserveTUSD to increase, got %v", cs.pool.ReserveTUSD.Float())
	}
	if cs.pool.ReserveAEQ.Float() >= 10000 {
		t.Errorf("want ReserveAEQ to decrease, got %v", cs.pool.ReserveAEQ.Float())
	}
	// Swap fee (40/30/20/10 split) lands in AEQ here: distributeSwapFee
	// converts a tUSD-denominated fee to AEQ via convertTUsdFeeToAEQLocked
	// whenever the pool can price it, which this well-liquidified pool can.
	if vAcc, ok := cs.accounts.Get(validatorsPoolAddr); !ok || vAcc.Balance.Float() <= 0 {
		t.Error("want a nonzero AEQ fee share credited to validatorsPoolAddr")
	}
}

func TestConvertTUsdForEscrowLocked_RefusesWhenAEQReserveIsEmpty(t *testing.T) {
	cs := newTestState()
	acc := &AccountState{Address: "0xtusd", IsHuman: true, TUsdBalance: NewDecimal(50)}
	cs.accounts.Set(acc.Address, acc)
	// An AEQ reserve of exactly 0 can't back ANY conversion, however small.
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(0), ReserveTUSD: NewDecimal(100)}

	outAEQ, ok, err := cs.convertTUsdForEscrowLocked(acc, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when the pool's AEQ reserve is empty")
	}
	if outAEQ != 0 {
		t.Errorf("want outAEQ=0 on refusal, got %v", outAEQ)
	}
	if acc.TUsdBalance.Float() != 50 {
		t.Errorf("want acc.TUsdBalance untouched, got %v", acc.TUsdBalance.Float())
	}
	if cs.pool.ReserveAEQ.Float() != 0 || cs.pool.ReserveTUSD.Float() != 100 {
		t.Errorf("want pool untouched, got reserveAEQ=%v reserveTUSD=%v", cs.pool.ReserveAEQ.Float(), cs.pool.ReserveTUSD.Float())
	}
}

func TestApplyEscrowMoveDelta_LiquidatesLPSharesThenZeroes(t *testing.T) {
	cs := newTestState()
	acc := &AccountState{Address: "0xward", IsHuman: true, LPShares: NewDecimal(20)}
	cs.accounts.Set(acc.Address, acc)
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(1000), ReserveTUSD: NewDecimal(1000), TotalLPShares: NewDecimal(200)}

	if err := cs.ApplyEscrowMoveDelta("0xward", 0, 20, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acc.Balance.Float() != 0 || acc.TUsdBalance.Float() != 0 || acc.LPShares.Float() != 0 {
		t.Errorf("want fully zeroed account after escrow, got balance=%v tusd=%v lp=%v",
			acc.Balance.Float(), acc.TUsdBalance.Float(), acc.LPShares.Float())
	}
	// 20/200 = 10% of the pool should have been removed.
	if cs.pool.TotalLPShares.Float() != 180 {
		t.Errorf("want TotalLPShares reduced to 180, got %v", cs.pool.TotalLPShares.Float())
	}
}

func TestApplyEscrowMoveDelta_ConvertsTUsdThenZeroes(t *testing.T) {
	cs := newTestState()
	acc := &AccountState{Address: "0xward", IsHuman: true, TUsdBalance: NewDecimal(50)}
	cs.accounts.Set(acc.Address, acc)
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(1000), ReserveTUSD: NewDecimal(1000)}

	if err := cs.ApplyEscrowMoveDelta("0xward", 0, 0, 50); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acc.Balance.Float() != 0 || acc.TUsdBalance.Float() != 0 {
		t.Errorf("want fully zeroed account, got balance=%v tusd=%v", acc.Balance.Float(), acc.TUsdBalance.Float())
	}
}

func TestApplyEscrowMoveDelta_PoolStateDivergence_ErrorsInsteadOfSilentlySkipping(t *testing.T) {
	cs := newTestState()
	acc := &AccountState{Address: "0xward", IsHuman: true}
	cs.accounts.Set(acc.Address, acc)
	// This node's pool has NO LP shares at all, unlike the primary that
	// reported burning 20 — simulates a diverged/inconsistent pool state.
	cs.pool = &PoolState{}

	if err := cs.ApplyEscrowMoveDelta("0xward", 0, 20, 0); err == nil {
		t.Fatal("expected an error when replaying a nonzero LP liquidation against an empty local pool, got nil")
	}
}

// TestLiquidateAndConvert_Deterministic proves the two call sites (primary
// and secondary replay) that both invoke these shared helpers can never
// diverge: running the exact same sequence against two independently
// constructed but identical starting states must produce identical results.
func TestLiquidateAndConvertForEscrow_Deterministic(t *testing.T) {
	newScenario := func() (*ChainState, *AccountState) {
		cs := newTestState()
		acc := &AccountState{Address: "0xward", IsHuman: true, LPShares: NewDecimal(30), TUsdBalance: NewDecimal(10)}
		cs.accounts.Set(acc.Address, acc)
		cs.pool = &PoolState{ReserveAEQ: NewDecimal(500), ReserveTUSD: NewDecimal(700), TotalLPShares: NewDecimal(150)}
		return cs, acc
	}

	csA, accA := newScenario()
	burnedA, outAEQ_A, outTUSD_A, err := csA.liquidateLPSharesForEscrowLocked(accA, accA.LPShares.Float())
	if err != nil {
		t.Fatalf("scenario A LP liquidation error: %v", err)
	}
	tusdInA := accA.TUsdBalance.Float()
	convAEQ_A, okA, err := csA.convertTUsdForEscrowLocked(accA, tusdInA)
	if err != nil || !okA {
		t.Fatalf("scenario A tUSD conversion error: %v ok=%v", err, okA)
	}

	csB, accB := newScenario()
	burnedB, outAEQ_B, outTUSD_B, err := csB.liquidateLPSharesForEscrowLocked(accB, accB.LPShares.Float())
	if err != nil {
		t.Fatalf("scenario B LP liquidation error: %v", err)
	}
	tusdInB := accB.TUsdBalance.Float()
	convAEQ_B, okB, err := csB.convertTUsdForEscrowLocked(accB, tusdInB)
	if err != nil || !okB {
		t.Fatalf("scenario B tUSD conversion error: %v ok=%v", err, okB)
	}

	if burnedA != burnedB || outAEQ_A != outAEQ_B || outTUSD_A != outTUSD_B {
		t.Fatalf("LP liquidation diverged: A=(%v,%v,%v) B=(%v,%v,%v)", burnedA, outAEQ_A, outTUSD_A, burnedB, outAEQ_B, outTUSD_B)
	}
	if convAEQ_A != convAEQ_B {
		t.Fatalf("tUSD conversion diverged: A=%v B=%v", convAEQ_A, convAEQ_B)
	}
	if accA.Balance.Float() != accB.Balance.Float() {
		t.Fatalf("final balances diverged: A=%v B=%v", accA.Balance.Float(), accB.Balance.Float())
	}
	if csA.pool.ReserveAEQ.Float() != csB.pool.ReserveAEQ.Float() || csA.pool.ReserveTUSD.Float() != csB.pool.ReserveTUSD.Float() || csA.pool.TotalLPShares.Float() != csB.pool.TotalLPShares.Float() {
		t.Fatalf("pool state diverged: A=%+v B=%+v", csA.pool, csB.pool)
	}
}
