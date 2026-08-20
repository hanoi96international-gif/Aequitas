package keeper

import (
	"testing"
)

// The demurrage clock records what the HOLDER did with their money, never what
// happened to them. See touchActivity's comment for why that distinction is the
// difference between the mechanism working and being decorative.

func idleAcc(addr string, bal float64, idleDays int64) *AccountState {
	return &AccountState{
		Address: addr, Balance: NewDecimal(bal), IsHuman: true,
		LastActivityAt: nowUnix() - idleDays*24*60*60,
	}
}

// TestUBICreditDoesNotResetTheDemurrageClock is the fix for a mechanism that
// had never fired. Every human is credited daily and was touched by it, and the
// grace period is 90 days, so the clock never elapsed for anybody.
func TestUBICreditDoesNotResetTheDemurrageClock(t *testing.T) {
	cs := newTestState()
	cs.pool = &PoolState{}
	rich := idleAcc("0xrich", 50000, 200) // long idle, far above fair share
	cs.accounts.Set(rich.Address, rich)
	cs.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(10)})
	cs.humanCount = 1

	before := rich.LastActivityAt
	cs.mu.Lock()
	_, err := cs.distributeUBIPoolLocked(t.Context())
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if rich.LastActivityAt != before {
		t.Errorf("the UBI credit moved LastActivityAt from %d to %d. Crediting someone is not "+
			"them using their money, and resetting on it is what kept demurrage from ever "+
			"firing for anyone", before, rich.LastActivityAt)
	}
}

// TestReceivingDoesNotResetTheClock closes an evasion that cost nothing: a
// micro-AEQ from a second wallet every 89 days reset the clock on an entire
// balance. Anyone who knew it was exempt; anyone who did not, was not.
func TestReceivingDoesNotResetTheClock(t *testing.T) {
	cs := newTestState()
	cs.pool = &PoolState{}
	hoard := idleAcc("0xhoard", 50000, 89)
	sender := &AccountState{Address: "0xsender", Balance: NewDecimal(10), IsHuman: true}
	cs.accounts.Set(hoard.Address, hoard)
	cs.accounts.Set(sender.Address, sender)
	cs.humanCount = 2

	before := hoard.LastActivityAt
	cs.mu.Lock()
	_, _, _, _, err := cs.transferMutateLocked(t.Context(), sender.Address, hoard.Address, 0.000001)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if hoard.LastActivityAt != before {
		t.Errorf("a 0.000001 AEQ transfer reset the recipient's clock (%d -> %d). The whole "+
			"mechanism would be avoidable for a millionth of an AEQ every 89 days",
			before, hoard.LastActivityAt)
	}
	if sender.LastActivityAt == 0 {
		t.Error("the SENDER's clock was not reset — sending is the holder using their money " +
			"and must still count")
	}
}

// TestReceivingStartsTheClockOnAFreshAccount: effectiveBalance exempts accounts
// with LastActivityAt == 0 entirely, so an address only ever paid into would
// otherwise be the shelter that removing the reset was meant to close.
func TestReceivingStartsTheClockOnAFreshAccount(t *testing.T) {
	cs := newTestState()
	cs.pool = &PoolState{}
	fresh := &AccountState{Address: "0xfresh", Balance: NewDecimal(0), IsHuman: true}
	sender := &AccountState{Address: "0xsender", Balance: NewDecimal(100), IsHuman: true}
	cs.accounts.Set(fresh.Address, fresh)
	cs.accounts.Set(sender.Address, sender)
	cs.humanCount = 2

	cs.mu.Lock()
	_, _, _, _, err := cs.transferMutateLocked(t.Context(), sender.Address, fresh.Address, 50)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if fresh.LastActivityAt == 0 {
		t.Error("a fresh account's clock never started, so it would be exempt from demurrage " +
			"forever — an address that is only ever paid into would hold wealth untouched")
	}
}

// TestFairShareFloorProtectsSmallHolders is the fairness core, and the reason
// making demurrage fire is not a tax on the poor.
func TestFairShareFloorProtectsSmallHolders(t *testing.T) {
	cs := newTestState()
	cs.pool = &PoolState{}
	for _, bal := range []float64{1, 100, 500, 999} {
		acc := idleAcc("0xsmall", bal, 3650) // ten years idle
		cs.accounts.Set(acc.Address, acc)
		cs.humanCount = 1

		cs.mu.Lock()
		lost, err := cs.settleDemurrageLockedCtx(t.Context(), acc)
		cs.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		if lost.Float() != 0 {
			t.Errorf("a holding of %.0f AEQ — at or below the fair share — decayed by %v after "+
				"ten years. Demurrage applies only ABOVE an equal share; if it reaches below it, "+
				"it has become a tax on the people the protocol exists for", bal, lost.Float())
		}
	}
}

// TestDemurrageMagnitudeIsGentle measures rather than asserts a feeling: this
// change makes a dormant mechanism live, and how hard it bites decides whether
// that is fair or punitive.
func TestDemurrageMagnitudeIsGentle(t *testing.T) {
	for _, tc := range []struct {
		balance  float64
		idleDays int64
	}{
		{2000, 365}, {5000, 365}, {50000, 365}, {5000, 120},
	} {
		cs := newTestState()
		cs.pool = &PoolState{}
		acc := idleAcc("0xh", tc.balance, tc.idleDays)
		cs.accounts.Set(acc.Address, acc)
		cs.humanCount = 1

		cs.mu.Lock()
		lost, err := cs.settleDemurrageLockedCtx(t.Context(), acc)
		cs.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		pct := 100 * lost.Float() / tc.balance
		t.Logf("%8.0f AEQ idle %3d days -> %9.4f AEQ decayed (%.2f%% of the holding)",
			tc.balance, tc.idleDays, lost.Float(), pct)

		if pct > 15 {
			t.Errorf("a holding of %.0f lost %.2f%% in %d days. That is punitive rather than a "+
				"gentle pressure, and would drive people off the currency", tc.balance, pct, tc.idleDays)
		}
	}
}
