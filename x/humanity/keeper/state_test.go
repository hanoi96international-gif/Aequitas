package keeper

import (
	"fmt"
	"math"
	"testing"
)

// newTestState creates an in-memory ChainState with no file/DB backend.
func newTestState() *ChainState {
	return &ChainState{
		accounts: make(map[string]*AccountState),
		pool:     &PoolState{},
		useDB:    false,
	}
}

// addHuman inserts a registered human account directly into state for testing.
// LastActivityAt=0 means effectiveBalance returns Balance with no demurrage.
func addHuman(cs *ChainState, addr string, balance float64) {
	cs.accounts[addr] = &AccountState{
		Address: addr,
		Balance: NewDecimal(balance),
		IsHuman: true,
	}
}

// --- CalcGini ---

func TestCalcGini_TwoEqualHumans(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0x01", 1000)
	addHuman(cs, "0x02", 1000)
	g := cs.CalcGini()
	if g != 0.0 {
		t.Errorf("equal balances: want Gini=0, got %v", g)
	}
}

func TestCalcGini_TwoHumansMaxConcentration(t *testing.T) {
	// One human has nearly everything. With n=2, biased Gini would cap at 0.5;
	// the unbiased estimator (×n/(n-1)) must push it close to 1.
	cs := newTestState()
	addHuman(cs, "0x01", 1)
	addHuman(cs, "0x02", 9999)
	g := cs.CalcGini()
	// biased = 9998/(2*10000) ≈ 0.4999; unbiased ≈ 0.9998 — must be > 0.99
	if g < 0.99 {
		t.Errorf("near-total concentration: want Gini>0.99, got %v", g)
	}
}

func TestCalcGini_ThreeEqualHumans(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0x01", 500)
	addHuman(cs, "0x02", 500)
	addHuman(cs, "0x03", 500)
	g := cs.CalcGini()
	if g != 0.0 {
		t.Errorf("equal balances (3): want Gini=0, got %v", g)
	}
}

func TestCalcGini_ThreeHumansKnownValue(t *testing.T) {
	// balances=[1,2,6], n=3, sum=9
	// biased Gini: numerator = (-2)*1 + 0*2 + 2*6 = 10; gini = 10/27 ≈ 0.3704
	// unbiased = 0.3704 * (3/2) ≈ 0.5556
	cs := newTestState()
	addHuman(cs, "0x01", 1)
	addHuman(cs, "0x02", 2)
	addHuman(cs, "0x03", 6)
	g := cs.CalcGini()
	want := 10.0 / 27.0 * 3.0 / 2.0 // exact unbiased value
	if math.Abs(g-want) > 1e-9 {
		t.Errorf("three humans [1,2,6]: want %v, got %v", want, g)
	}
}

func TestCalcGini_SingleHuman_ReturnsZero(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0x01", 1000)
	g := cs.CalcGini()
	if g != 0.0 {
		t.Errorf("single human: want 0, got %v", g)
	}
}

func TestCalcGini_NonHumanBalancesIgnored(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0x01", 100)
	addHuman(cs, "0x02", 100)
	// Non-human with huge balance — must not skew Gini
	cs.accounts["0x99"] = &AccountState{Address: "0x99", Balance: NewDecimal(1_000_000), IsHuman: false}
	g := cs.CalcGini()
	if g != 0.0 {
		t.Errorf("non-human balance must be ignored: want Gini=0, got %v", g)
	}
}

func TestCalcGini_CountsLPWealth(t *testing.T) {
	// A human who parked all their AEQ as liquidity has liquid Balance 0 but is
	// not broke — their wealth is their share of the pool's AEQ reserve. The Gini
	// must count that; the old code filtered on Balance>0 and dropped them,
	// which both understated inequality AND disagreed with the Lorenz curve.
	cs := newTestState()
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(1000), TotalLPShares: NewDecimal(1000)}
	addHuman(cs, "0x01", 100) // 100 AEQ liquid, no LP
	// 0 liquid, owns 900 of 1000 LP shares → 900 AEQ of pool wealth.
	cs.accounts["0x02"] = &AccountState{Address: "0x02", IsHuman: true, Balance: NewDecimal(0), LPShares: NewDecimal(900)}
	g := cs.CalcGini()
	// Wealth is [100, 900] → highly unequal. The old code returned ~0 here
	// (0x02 dropped, lone human left), the exact "LP wallet shows 0" bug.
	if g < 0.7 {
		t.Errorf("LP wealth must count toward Gini: want >0.7 for wealth [100,900], got %v", g)
	}
}

func TestCalcGini_LPAndLiquidEqual_IsZero(t *testing.T) {
	// Two humans holding the same TOTAL wealth by different means (one liquid,
	// one all-LP) are perfectly equal → Gini 0.
	cs := newTestState()
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(500), TotalLPShares: NewDecimal(500)}
	addHuman(cs, "0x01", 500) // 500 liquid
	cs.accounts["0x02"] = &AccountState{Address: "0x02", IsHuman: true, Balance: NewDecimal(0), LPShares: NewDecimal(500)} // 500 via LP
	if g := cs.CalcGini(); g != 0.0 {
		t.Errorf("equal total wealth (liquid vs LP): want Gini=0, got %v", g)
	}
}

// --- CalcAequitasIndex ---

func TestCalcAequitasIndex_EqualHumans(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0x01", 1000)
	addHuman(cs, "0x02", 1000)
	idx := cs.CalcAequitasIndex()
	if idx != 0.0 {
		t.Errorf("equal balances: want Index=0, got %v", idx)
	}
}

func TestCalcAequitasIndex_IsGiniTimes100Truncated(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0x01", 1)
	addHuman(cs, "0x02", 2)
	addHuman(cs, "0x03", 6)
	gini := cs.CalcGini()
	idx := cs.CalcAequitasIndex()
	// Index = floor(gini*1000) / 10  (truncates to 1 decimal place)
	want := math.Trunc(gini*1000) / 10
	if idx != want {
		t.Errorf("index=%v, want gini*100 truncated to 1dp=%v (gini=%v)", idx, want, gini)
	}
}

// --- bootstrapMultiplierLocked ---

func TestBootstrapMultiplier_ZeroHumans(t *testing.T) {
	cs := newTestState()
	cs.mu.RLock()
	m := cs.bootstrapMultiplierLocked()
	cs.mu.RUnlock()
	if m != 5.0 {
		t.Errorf("0 humans: want 5.0, got %v", m)
	}
}

func TestBootstrapMultiplier_OneHuman(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0x01", 0)
	cs.mu.RLock()
	m := cs.bootstrapMultiplierLocked()
	cs.mu.RUnlock()
	if m != 5.0 {
		t.Errorf("1 human: want 5.0, got %v", m)
	}
}

func TestBootstrapMultiplier_FiveHumans(t *testing.T) {
	cs := newTestState()
	for i := 0; i < 5; i++ {
		addHuman(cs, fmt.Sprintf("0xF%d", i), 0)
	}
	cs.mu.RLock()
	m := cs.bootstrapMultiplierLocked()
	cs.mu.RUnlock()
	if m != 5.0 {
		t.Errorf("5 humans: want 5.0, got %v", m)
	}
}

func TestBootstrapMultiplier_TenHumans(t *testing.T) {
	cs := newTestState()
	for i := 0; i < 10; i++ {
		addHuman(cs, fmt.Sprintf("0xA%d", i), 0)
	}
	cs.mu.RLock()
	m := cs.bootstrapMultiplierLocked()
	cs.mu.RUnlock()
	if m != 10.0 {
		t.Errorf("10 humans: want 10.0, got %v", m)
	}
}

func TestBootstrapMultiplier_TwentyFourHumans(t *testing.T) {
	cs := newTestState()
	for i := 0; i < 24; i++ {
		addHuman(cs, fmt.Sprintf("0xB%d", i), 0)
	}
	cs.mu.RLock()
	m := cs.bootstrapMultiplierLocked()
	cs.mu.RUnlock()
	if m != 24.0 {
		t.Errorf("24 humans: want 24.0, got %v", m)
	}
}

func TestBootstrapMultiplier_TwentyFiveHumans(t *testing.T) {
	cs := newTestState()
	for i := 0; i < 25; i++ {
		addHuman(cs, fmt.Sprintf("0xC%d", i), 0)
	}
	cs.mu.RLock()
	m := cs.bootstrapMultiplierLocked()
	cs.mu.RUnlock()
	if m != 25.0 {
		t.Errorf("25 humans: want 25.0 (full cap), got %v", m)
	}
}

func TestBootstrapMultiplier_ThirtyHumans(t *testing.T) {
	cs := newTestState()
	for i := 0; i < 30; i++ {
		addHuman(cs, fmt.Sprintf("0xD%d", i), 0)
	}
	cs.mu.RLock()
	m := cs.bootstrapMultiplierLocked()
	cs.mu.RUnlock()
	if m != 25.0 {
		t.Errorf("30 humans: want 25.0 (capped at wealthCapMultiplier), got %v", m)
	}
}

// --- enforceWealthCapLocked ---

func TestEnforceWealthCap_BelowCap_NoChange(t *testing.T) {
	cs := newTestState()
	// 2 humans at 500 each: avg=500, multiplier=5 (floor), cap=2500 > 500 → no cap
	addHuman(cs, "0x01", 500)
	addHuman(cs, "0x02", 500)
	acc := cs.accounts["0x01"]
	cs.mu.Lock()
	cs.enforceWealthCapLocked(acc)
	cs.mu.Unlock()
	if acc.Balance.Float() != 500 {
		t.Errorf("under cap: balance should stay 500, got %v", acc.Balance.Float())
	}
}

func TestEnforceWealthCap_AboveCap_ExcessRedistributed(t *testing.T) {
	// The cap bites only when N > multiplier (otherwise the rich account
	// pulls the average up and is never capped by its own contribution).
	// With 26 humans (multiplier=25) and 25 poor at 10 AEQ + 1 rich at 100_000:
	//   avg = (100_000 + 25*10) / 26  ≈ 3855.77
	//   cap = 25 * avg                ≈ 96394.23
	//   excess = 100_000 - cap        ≈ 3605.77
	cs := newTestState()
	for i := 0; i < 25; i++ {
		addHuman(cs, fmt.Sprintf("0xpoor%02d", i), 10)
	}
	addHuman(cs, "0xrich", 100_000)
	acc := cs.accounts["0xrich"]
	// Current production logic intentionally uses the fixed fair-share
	// invariant of 1000 AEQ per human.
	expectedCap := 25_000.0
	cs.mu.Lock()
	cs.enforceWealthCapLocked(acc)
	cs.mu.Unlock()
	if math.Abs(acc.Balance.Float()-expectedCap) > 1e-6 {
		t.Errorf("after cap: want balance=%.6f, got %.6f", expectedCap, acc.Balance.Float())
	}
	poolTotal := cs.accounts[validatorsPoolAddr].Balance.Float() +
		cs.accounts[lpPoolAddr].Balance.Float() +
		cs.accounts[ubiPoolAddr].Balance.Float() +
		cs.accounts[treasuryPoolAddr].Balance.Float()
	expectedExcess := 75_000.0
	if math.Abs(poolTotal-expectedExcess) > 1e-6 {
		t.Errorf("pool total: want %.6f excess redistributed, got %.6f", expectedExcess, poolTotal)
	}
}

func TestEnforceWealthCap_PoolAddresses_Exempt(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0x01", 100)
	// Pool address with huge balance — must be exempt from cap
	cs.accounts[validatorsPoolAddr] = &AccountState{Address: validatorsPoolAddr, Balance: NewDecimal(1_000_000)}
	acc := cs.accounts[validatorsPoolAddr]
	cs.mu.Lock()
	cs.enforceWealthCapLocked(acc)
	cs.mu.Unlock()
	if acc.Balance.Float() != 1_000_000 {
		t.Errorf("pool address must be exempt from cap, balance changed to %v", acc.Balance.Float())
	}
}

// --- Delta functions: fail-clean on insufficient balance (no partial mutation) ---

func TestApplyTransferDelta_InsufficientAfterDemurrage_NoMutation(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0xfrom", 100)
	addHuman(cs, "0xto", 0)
	// Demurrage decay of 95 leaves only 5 available — insufficient for a 10 transfer.
	err := cs.ApplyTransferDelta("0xfrom", "0xto", 10, 95, 0)
	if err == nil {
		t.Fatal("expected insufficient-balance error, got nil")
	}
	if cs.accounts["0xfrom"].Balance.Float() != 100 {
		t.Errorf("sender balance must be unchanged on failure, got %v", cs.accounts["0xfrom"].Balance.Float())
	}
	if cs.accounts["0xto"].Balance.Float() != 0 {
		t.Errorf("recipient balance must be unchanged on failure, got %v", cs.accounts["0xto"].Balance.Float())
	}
	for _, addr := range []string{validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr} {
		if acc, ok := cs.accounts[addr]; ok && acc.Balance.Float() != 0 {
			t.Errorf("pool %s must not be credited when the transfer fails, got %v", addr, acc.Balance.Float())
		}
	}
}

func TestApplySwapDelta_InsufficientAfterDemurrage_NoMutation(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0xswapper", 100)
	err := cs.ApplySwapDelta("0xswapper", 10, 5, true, 95)
	if err == nil {
		t.Fatal("expected insufficient-balance error, got nil")
	}
	if cs.accounts["0xswapper"].Balance.Float() != 100 {
		t.Errorf("balance must be unchanged on failure, got %v", cs.accounts["0xswapper"].Balance.Float())
	}
	if cs.accounts["0xswapper"].TUsdBalance.Float() != 0 {
		t.Errorf("tUSD balance must be unchanged on failure, got %v", cs.accounts["0xswapper"].TUsdBalance.Float())
	}
}

func TestAddLiquidityDelta_InsufficientAfterDemurrage_NoMutation(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0xlp", 100)
	err := cs.AddLiquidityDelta("0xlp", 10, 10, 0, 95)
	if err == nil {
		t.Fatal("expected insufficient-balance error, got nil")
	}
	if cs.accounts["0xlp"].Balance.Float() != 100 {
		t.Errorf("balance must be unchanged on failure, got %v", cs.accounts["0xlp"].Balance.Float())
	}
	if cs.pool.ReserveAEQ.Float() != 0 {
		t.Errorf("pool reserves must be unchanged on failure, got %v", cs.pool.ReserveAEQ.Float())
	}
}

// --- Block-level rollback snapshot/restore ---

func TestSnapshotRestoreRollback_RevertsExistingAccountAndPool(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0xa", 1000)
	cs.pool.ReserveAEQ = NewDecimal(500)
	cs.pool.ReserveTUSD = NewDecimal(500)

	snap := cs.snapshotForRollback([]string{"0xa"}, false)

	// Mutate as if a TX had partially applied.
	cs.accounts["0xa"].Balance = NewDecimal(1)
	cs.pool.ReserveAEQ = NewDecimal(999999)

	cs.restoreFromRollback(snap)

	if cs.accounts["0xa"].Balance.Float() != 1000 {
		t.Errorf("account balance not restored: got %v, want 1000", cs.accounts["0xa"].Balance.Float())
	}
	if cs.pool.ReserveAEQ.Float() != 500 {
		t.Errorf("pool reserve not restored: got %v, want 500", cs.pool.ReserveAEQ.Float())
	}
}

func TestSnapshotRestoreRollback_RemovesAccountCreatedDuringBlock(t *testing.T) {
	cs := newTestState()
	// "0xnew" does not exist yet at snapshot time.
	snap := cs.snapshotForRollback([]string{"0xnew"}, false)

	// Simulate a transfer creating the recipient mid-block.
	cs.accounts["0xnew"] = &AccountState{Address: "0xnew", Balance: NewDecimal(50)}

	cs.restoreFromRollback(snap)

	if _, exists := cs.accounts["0xnew"]; exists {
		t.Error("account created during a rolled-back block must be removed, but still exists")
	}
}

func TestSnapshotRestoreRollback_FullSnapshotCoversAllAccounts(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0x01", 1000)
	addHuman(cs, "0x02", 1000)

	// full=true (the ubi_distribution case) must capture every account,
	// not just ones explicitly named.
	snap := cs.snapshotForRollback(nil, true)

	cs.accounts["0x01"].Balance = NewDecimal(5000)
	cs.accounts["0x02"].Balance = NewDecimal(5000)

	cs.restoreFromRollback(snap)

	if cs.accounts["0x01"].Balance.Float() != 1000 {
		t.Errorf("0x01 not restored under full snapshot: got %v", cs.accounts["0x01"].Balance.Float())
	}
	if cs.accounts["0x02"].Balance.Float() != 1000 {
		t.Errorf("0x02 not restored under full snapshot: got %v", cs.accounts["0x02"].Balance.Float())
	}
}

// --- Daily distribution: primary computes real amounts, secondaries replay them exactly ---

func TestDistributeUBIPool_ReturnsAmountActuallyCredited(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0x01", 0)
	addHuman(cs, "0x02", 0)
	cs.accounts[ubiPoolAddr] = &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(100)}

	shares := cs.DistributeUBIPool()

	if len(shares) != 2 {
		t.Fatalf("want 2 shares, got %d", len(shares))
	}
	got := map[string]float64{}
	for _, s := range shares {
		got[s.Wallet] = s.Amount
	}
	if got["0x01"] != 50 || got["0x02"] != 50 {
		t.Errorf("want 0x01=50 0x02=50, got %v", got)
	}
	// The returned values must match what was ACTUALLY credited — this is
	// the exact bug the audit flagged: main.go used to compute this number
	// independently (reading the pool balance before calling this
	// function), which could differ from what got applied here.
	if cs.accounts["0x01"].Balance.Float() != got["0x01"] {
		t.Errorf("returned amount (%v) doesn't match actual credit (%v)", got["0x01"], cs.accounts["0x01"].Balance.Float())
	}
	if cs.accounts[ubiPoolAddr].Balance.Float() != 0 {
		t.Errorf("pool must be zeroed after distribution, got %v", cs.accounts[ubiPoolAddr].Balance.Float())
	}
}

func TestApplyUBIRewardDelta_SettlesDemurrageThenCredits(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0xhuman", 100)

	if err := cs.ApplyUBIRewardDelta("0xhuman", 50, 8); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cs.accounts["0xhuman"].Balance.Float() != 142 {
		t.Errorf("want balance=142 (100-8+50), got %v", cs.accounts["0xhuman"].Balance.Float())
	}
}

func TestApplyUBIRewardDelta_UnknownWallet_Errors(t *testing.T) {
	cs := newTestState()
	if err := cs.ApplyUBIRewardDelta("0xdoesnotexist", 50, 0); err == nil {
		t.Fatal("expected error for unknown wallet, got nil")
	}
}

func TestApplyUBIFinalizeDelta_ZeroesPool(t *testing.T) {
	cs := newTestState()
	cs.accounts[ubiPoolAddr] = &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(33)}

	// setConfigValue/getConfigValue are no-ops without cs.db (newTestState
	// has none) — this test only verifies the pool-zeroing half; last_ubi_at
	// persistence is exercised against a real DB in production.
	cs.ApplyUBIFinalizeDelta(123456789)

	if cs.accounts[ubiPoolAddr].Balance.Float() != 0 {
		t.Errorf("want pool zeroed, got %v", cs.accounts[ubiPoolAddr].Balance.Float())
	}
}

func TestDistributeLPPool_ReturnsSharesMatchingActualCredits(t *testing.T) {
	cs := newTestState()
	cs.accounts["0x01"] = &AccountState{Address: "0x01", LPShares: NewDecimal(3)}
	cs.accounts["0x02"] = &AccountState{Address: "0x02", LPShares: NewDecimal(1)}
	cs.accounts[lpPoolAddr] = &AccountState{Address: lpPoolAddr, Balance: NewDecimal(40)}

	shares := cs.DistributeLPPool()

	if len(shares) != 2 {
		t.Fatalf("want 2 shares, got %d", len(shares))
	}
	got := map[string]float64{}
	for _, s := range shares {
		got[s.Wallet] = s.Amount
	}
	if got["0x01"] != 30 || got["0x02"] != 10 {
		t.Errorf("want 0x01=30 0x02=10 (3:1 split of 40), got %v", got)
	}
	// Returned shares must equal the wallet's actual post-distribution balance.
	if cs.accounts["0x01"].Balance.Float() != got["0x01"] {
		t.Errorf("returned share for 0x01 (%v) doesn't match actual balance (%v)", got["0x01"], cs.accounts["0x01"].Balance.Float())
	}
	if cs.accounts[lpPoolAddr].Balance.Float() != 0 {
		t.Errorf("pool must be zeroed after distribution, got %v", cs.accounts[lpPoolAddr].Balance.Float())
	}
}

func TestApplyLPRewardDelta_SettlesDemurrageThenCredits(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0xholder", 100)

	// Primary settled 12 AEQ of demurrage loss before crediting a 10 AEQ reward.
	if err := cs.ApplyLPRewardDelta("0xholder", 10, 12); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cs.accounts["0xholder"].Balance.Float() != 98 {
		t.Errorf("want balance=98 (100-12+10), got %v", cs.accounts["0xholder"].Balance.Float())
	}
}

func TestApplyValidatorRewardDelta_SettlesDemurrageThenCredits(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0xvalidator", 100)

	// Primary settled 20 AEQ of demurrage loss before crediting a 5 AEQ reward.
	if err := cs.ApplyValidatorRewardDelta("0xvalidator", 5, 20); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cs.accounts["0xvalidator"].Balance.Float() != 85 {
		t.Errorf("want balance=85 (100-20+5), got %v", cs.accounts["0xvalidator"].Balance.Float())
	}
}

func TestApplyValidatorPoolZeroDelta_ZeroesPool(t *testing.T) {
	cs := newTestState()
	cs.accounts[validatorsPoolAddr] = &AccountState{Address: validatorsPoolAddr, Balance: NewDecimal(50)}
	cs.ApplyValidatorPoolZeroDelta()
	if cs.accounts[validatorsPoolAddr].Balance.Float() != 0 {
		t.Errorf("want pool zeroed, got %v", cs.accounts[validatorsPoolAddr].Balance.Float())
	}
}

func TestApplyEscrowMoveDelta_SettlesDemurrageThenZeroesBalance(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0xinactive", 100)

	if err := cs.ApplyEscrowMoveDelta("0xinactive", 30); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cs.accounts["0xinactive"].Balance.Float() != 0 {
		t.Errorf("want balance=0 after escrow move, got %v", cs.accounts["0xinactive"].Balance.Float())
	}
}

func TestApplyEscrowMoveDelta_UnknownWallet_Errors(t *testing.T) {
	cs := newTestState()
	if err := cs.ApplyEscrowMoveDelta("0xdoesnotexist", 0); err == nil {
		t.Fatal("expected error for unknown wallet, got nil")
	}
}

func TestApplyEscrowReleaseDelta_CreditsUBIPool(t *testing.T) {
	cs := newTestState()
	cs.accounts[ubiPoolAddr] = &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(10)}

	if err := cs.ApplyEscrowReleaseDelta(25); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cs.accounts[ubiPoolAddr].Balance.Float() != 35 {
		t.Errorf("want UBI pool balance=35 (10+25), got %v", cs.accounts[ubiPoolAddr].Balance.Float())
	}
}

func TestRunDailyDistributionAtomic_CreditsUBIAndValidators(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0x01", 0)
	addHuman(cs, "0x02", 0)
	cs.accounts[ubiPoolAddr] = &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(100)}

	if err := cs.RunDailyDistributionAtomic(123456789); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cs.accounts["0x01"].Balance.Float() != 50 || cs.accounts["0x02"].Balance.Float() != 50 {
		t.Errorf("want both humans credited 50, got 0x01=%v 0x02=%v",
			cs.accounts["0x01"].Balance.Float(), cs.accounts["0x02"].Balance.Float())
	}
	if cs.accounts[ubiPoolAddr].Balance.Float() != 0 {
		t.Errorf("want UBI pool zeroed, got %v", cs.accounts[ubiPoolAddr].Balance.Float())
	}
}

func TestRunDailyDistributionAtomic_NoOpWhenPoolsEmpty(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0x01", 1000)

	if err := cs.RunDailyDistributionAtomic(123456789); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cs.accounts["0x01"].Balance.Float() != 1000 {
		t.Errorf("want unchanged balance 1000, got %v", cs.accounts["0x01"].Balance.Float())
	}
}
