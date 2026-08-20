package keeper

import (
	"fmt"
	"math"
	"testing"
)

// Review pass over the LP-aware demurrage and wealth cap, 2026-08-20.
//
// The change reduces real balances, so it was checked rather than reasoned
// about. Three of these passed first time; the fourth failed and found a real
// regression, which is why it is kept rather than deleted once green.

// TestVerify_DemurrageWithLPConserves: releasing LP to pay the decay moves AEQ
// out of the reserve and into a balance before taking it, so the total must be
// unchanged.
func TestVerify_DemurrageWithLPConserves(t *testing.T) {
	cs := newTestState()
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(5000), ReserveTUSD: NewDecimal(5000), TotalLPShares: NewDecimal(1000)}
	idle := nowUnix() - 400*24*3600
	acc := &AccountState{Address: "0xlp", Balance: NewDecimal(100), IsHuman: true, LPShares: NewDecimal(1000), LastActivityAt: idle}
	cs.accounts.Set(acc.Address, acc)
	cs.humanCount = 1

	before := totalAEQ(cs)
	cs.mu.Lock()
	_, err := cs.settleDemurrageLockedCtx(t.Context(), acc)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	after := totalAEQ(cs)
	if math.Abs(after-before) > 1e-9 {
		t.Errorf("LP-aware demurrage changed total supply by %+.9f (before %.6f after %.6f)", after-before, before, after)
	}
}

// TestVerify_WealthCapWithLPConserves: the same, for the cap.
func TestVerify_WealthCapWithLPConserves(t *testing.T) {
	cs := newTestState()
	for i := 0; i < 4; i++ {
		a := fmt.Sprintf("0xh%d", i)
		cs.accounts.Set(a, &AccountState{Address: a, Balance: NewDecimal(1000), IsHuman: true})
		cs.humanCount++
	}
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(400000), ReserveTUSD: NewDecimal(400000), TotalLPShares: NewDecimal(1000)}
	acc := &AccountState{Address: "0xhoard", Balance: NewDecimal(10), IsHuman: true, LPShares: NewDecimal(1000)}
	cs.accounts.Set(acc.Address, acc)
	cs.humanCount++

	before := totalAEQ(cs)
	cs.mu.Lock()
	err := cs.enforceWealthCapLockedCtx(t.Context(), acc)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	after := totalAEQ(cs)
	if math.Abs(after-before) > 1e-9 {
		t.Errorf("LP-aware wealth cap changed total supply by %+.9f (before %.6f after %.6f)", after-before, before, after)
	}
}

// TestVerify_IdleProviderCanStillWithdrawEverything is the regression this
// review found.
//
// removeLiquidityLocked settles demurrage before validating the requested
// shares. Once demurrage could burn LP shares, settling consumed part of the
// very position being withdrawn, and the check then reported "insufficient LP
// shares (have 959.617838, requested 1000)" — blaming the person for asking
// about the position they held when they clicked. It now clamps to what
// remains, and only errors when the request exceeded the pre-settlement
// position.
func TestVerify_IdleProviderCanStillWithdrawEverything(t *testing.T) {
	cs := newTestState()
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(5000), ReserveTUSD: NewDecimal(5000), TotalLPShares: NewDecimal(1000)}
	idle := nowUnix() - 400*24*3600
	acc := &AccountState{Address: "0xlp", Balance: NewDecimal(0), IsHuman: true, LPShares: NewDecimal(1000), LastActivityAt: idle}
	cs.accounts.Set(acc.Address, acc)
	cs.humanCount = 1

	cs.mu.Lock()
	_, _, _, err := cs.removeLiquidityLocked(t.Context(), "0xlp", 1000)
	cs.mu.Unlock()
	if err != nil {
		t.Errorf("an idle provider asking to withdraw their whole position got: %v\n"+
			"  demurrage now settles LP shares BEFORE the share check, so the position it "+
			"validates against is smaller than the one the user asked about", err)
	}
}

// TestVerify_FairShareFloorStillApplies: demurrage applies only above the fair
// share, and routing LP value through the same formula must not quietly change
// that — otherwise the smallest holders would start decaying.
func TestVerify_FairShareFloorStillApplies(t *testing.T) {
	cs := newTestState()
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(500), ReserveTUSD: NewDecimal(500), TotalLPShares: NewDecimal(1000)}
	idle := nowUnix() - 3650*24*3600 // ten years
	acc := &AccountState{Address: "0xsmall", Balance: NewDecimal(0), IsHuman: true, LPShares: NewDecimal(1000), LastActivityAt: idle}
	cs.accounts.Set(acc.Address, acc)
	cs.humanCount = 1

	cs.mu.Lock()
	lost, err := cs.settleDemurrageLockedCtx(t.Context(), acc)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if lost.Float() != 0 {
		t.Errorf("a holding of 500 AEQ (below the 1000 fair share) decayed by %v after ten "+
			"years idle. Demurrage only applies above fair share", lost.Float())
	}
}
