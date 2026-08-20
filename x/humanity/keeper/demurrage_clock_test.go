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

// TestReplayMirrorsTheClockRule is the check the suite was missing.
//
// The rule was applied to the primary paths only. The replay paths kept calling
// touchActivityAt on UBI, on the three reward credits and on transfer
// recipients — so every node that was not producing went on resetting the clock
// exactly as before.
//
// Nothing broke visibly: LastActivityAt is deliberately outside accountLeaf, so
// consensus was unaffected, and demurrage amounts travel in the delta rather
// than being recomputed on replay. The failure was silent and deferred — the
// moment another node became primary, every account there looked freshly
// active and demurrage stopped firing again. The fix would have undone itself
// at exactly the moment nobody was watching.
func TestReplayMirrorsTheClockRule(t *testing.T) {
	// In the PAST on purpose. touchActivityAt clamps a stamp to now, because
	// block.Timestamp is chosen by the proposer and nothing validates it — an
	// unclamped future stamp would exempt any account it touched from decay
	// permanently. A future timestamp here would test the clamp, not the rule.
	blockTs := nowUnix() - 3600

	t.Run("a UBI credit on replay does not reset the clock", func(t *testing.T) {
		cs := newTestState()
		cs.pool = &PoolState{}
		acc := idleAcc("0xreplay", 50000, 200)
		cs.accounts.Set(acc.Address, acc)
		cs.humanCount = 1
		before := acc.LastActivityAt

		cs.mu.Lock()
		err := cs.applyUBIDeltaLocked(t.Context(), 1.0, blockTs)
		cs.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		if acc.LastActivityAt != before {
			t.Errorf("replaying a UBI credit moved LastActivityAt from %d to %d. The primary no "+
				"longer does this, so a node that only replays would disagree — and the "+
				"disagreement surfaces as demurrage silently switching off the moment that node "+
				"starts producing", before, acc.LastActivityAt)
		}
	})

	t.Run("receiving on replay starts the clock without resetting it", func(t *testing.T) {
		cs := newTestState()
		cs.pool = &PoolState{}
		sender := &AccountState{Address: "0xs", Balance: NewDecimal(1000), IsHuman: true, LastActivityAt: nowUnix()}
		idle := idleAcc("0xr", 50000, 200)
		fresh := &AccountState{Address: "0xf", Balance: NewDecimal(0), IsHuman: true}
		for _, a := range []*AccountState{sender, idle, fresh} {
			cs.accounts.Set(a.Address, a)
		}
		cs.humanCount = 3

		idleBefore := idle.LastActivityAt
		cs.mu.Lock()
		err1 := cs.applyTransferDeltaLocked(t.Context(), sender.Address, idle.Address, 1, 0, 0, blockTs)
		err2 := cs.applyTransferDeltaLocked(t.Context(), sender.Address, fresh.Address, 1, 0, 0, blockTs)
		cs.mu.Unlock()
		if err1 != nil || err2 != nil {
			t.Fatalf("replay transfer: %v / %v", err1, err2)
		}

		if idle.LastActivityAt != idleBefore {
			t.Errorf("a replayed transfer reset an idle recipient's clock (%d -> %d) — the "+
				"micro-transfer evasion would still work on every replaying node",
				idleBefore, idle.LastActivityAt)
		}
		if fresh.LastActivityAt != blockTs {
			t.Errorf("a fresh recipient's clock started at %d, want the block time %d. Using this "+
				"node's clock instead would stamp a different moment on every node",
				fresh.LastActivityAt, blockTs)
		}
	})
}
