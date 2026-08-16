package keeper

import (
	"math"
	"testing"
)

// Demurrage is defined against the FAIR SHARE, not against the whole balance.
// Four independent descriptions of the rule agree on this:
//
//	WHITEPAPER.md            "(Balance - fairShare) x 0.5%/month"
//	README.md                same formula
//	landing.go's mechanism   "only on the portion above your fair share"
//	AequitasV7.sol:313       if (bal <= fs) return; excess = bal - fs
//
// The Go chain — the one that actually holds everyone's balance — decayed
// acc.Balance in full instead, so a person holding exactly their 1,000 AEQ
// registration grant was charged demurrage after three idle months while
// being shown a promise that they would not be. These tests pin the corrected
// behaviour so that outlier cannot come back.
//
// fairShare on this chain is exactly registrationGrant: TotalSupply() is
// defined as humanCount x 1000, so TotalSupply/humanCount is identically 1000
// for any non-zero human count (see registrationGrant's own comment).

// The case the whole fix exists for: the poorest possible participant, holding
// nothing but their grant, idle well past the grace period. They must lose
// nothing at all.
func TestDemurrage_GrantOnlyHolderNeverDecays(t *testing.T) {
	acc := &AccountState{
		Address:        "0xpoorest",
		Balance:        NewDecimal(registrationGrant),
		IsHuman:        true,
		LastActivityAt: nowUnix() - 400*24*3600, // ~13 months idle
	}
	got := effectiveBalance(acc)
	if got != acc.Balance {
		t.Errorf("a holder of exactly one fair share lost %.6f AEQ to demurrage after 13 idle months; "+
			"the fair share is a floor and must never decay (want %.6f, got %.6f)",
			acc.Balance.Sub(got).Float(), acc.Balance.Float(), got.Float())
	}
}

// Below the fair share there is even less to take. This is the same rule, but
// it is worth pinning separately: an account can land under fair share through
// transfers, and the `<=` boundary is exactly where an off-by-one would hide.
func TestDemurrage_BelowFairShareNeverDecays(t *testing.T) {
	for _, bal := range []float64{0, 0.000001, 1, 250, registrationGrant - 0.000001} {
		acc := &AccountState{
			Address:        "0xbelow",
			Balance:        NewDecimal(bal),
			IsHuman:        true,
			LastActivityAt: nowUnix() - 400*24*3600,
		}
		if got := effectiveBalance(acc); got != acc.Balance {
			t.Errorf("balance %.6f (below the %.0f fair share) decayed to %.6f; nothing under the floor may decay",
				bal, registrationGrant, got.Float())
		}
	}
}

// Above the fair share, only the excess decays — and the floor holds no matter
// how long the account sits idle. Decaying the whole balance would drive a
// holder below their fair share given enough time; decaying only the excess
// converges ON the fair share and never past it.
func TestDemurrage_OnlyTheExcessDecays(t *testing.T) {
	const excess = 4000.0
	acc := &AccountState{
		Address:        "0xwhale",
		Balance:        NewDecimal(registrationGrant + excess),
		IsHuman:        true,
		LastActivityAt: nowUnix() - (demurrageGracePeriodSeconds + 6*secondsPerMonth),
	}

	got := effectiveBalance(acc).Float()

	// Six months of decay past the grace period, applied to the excess only.
	wantExcess := excess * math.Pow(1-demurrageMonthlyRate, 6)
	want := registrationGrant + wantExcess
	if math.Abs(got-want) > 1e-4 {
		t.Errorf("effectiveBalance = %.6f, want %.6f (fair share %.0f + decayed excess %.6f)",
			got, want, registrationGrant, wantExcess)
	}

	// The old whole-balance behaviour would have produced this instead. Named
	// explicitly so a regression reports the two numbers side by side rather
	// than just "wrong".
	oldBehaviour := (registrationGrant + excess) * math.Pow(1-demurrageMonthlyRate, 6)
	if math.Abs(got-oldBehaviour) < 1e-4 {
		t.Errorf("effectiveBalance = %.6f, which is the pre-fix whole-balance decay — the fair-share floor is not being applied", got)
	}
}

// The floor is asymptotic: no amount of idle time may push a balance below one
// fair share. A century is well past any horizon the protocol cares about, and
// is the cheapest way to state "never".
func TestDemurrage_FloorHoldsAcrossAnyIdleDuration(t *testing.T) {
	for _, years := range []int64{1, 5, 20, 100} {
		acc := &AccountState{
			Address:        "0xancient",
			Balance:        NewDecimal(registrationGrant + 10000),
			IsHuman:        true,
			LastActivityAt: nowUnix() - years*365*24*3600,
		}
		got := effectiveBalance(acc).Float()
		if got < registrationGrant-1e-6 {
			t.Errorf("after %d idle years the balance decayed to %.6f, below the %.0f fair-share floor",
				years, got, registrationGrant)
		}
	}
}

// The grace period is a SEPARATE protection from the floor, and adding the
// floor must not have disturbed it. Inside the grace period nothing decays at
// all, including the excess.
func TestDemurrage_GracePeriodStillSuppressesDecayOfTheExcess(t *testing.T) {
	acc := &AccountState{
		Address:        "0xrecent",
		Balance:        NewDecimal(registrationGrant + 5000),
		IsHuman:        true,
		LastActivityAt: nowUnix() - (demurrageGracePeriodSeconds - 3600), // one hour short of the grace period
	}
	if got := effectiveBalance(acc); got != acc.Balance {
		t.Errorf("decay was applied inside the grace period: want %.6f, got %.6f", acc.Balance.Float(), got.Float())
	}
}

// Settlement must agree with the read. settleDemurrageLockedCtx writes off
// exactly what effectiveBalance reports and hands the difference to the pools;
// if the two ever disagree, money is created or destroyed at settlement time.
func TestDemurrage_SettlementMatchesTheReportedBalance(t *testing.T) {
	cs := newTestState()
	cs.pool = &PoolState{}
	acc := &AccountState{
		Address:        "0xsettler",
		Balance:        NewDecimal(registrationGrant + 4000),
		IsHuman:        true,
		LastActivityAt: nowUnix() - (demurrageGracePeriodSeconds + 6*secondsPerMonth),
	}
	cs.accounts.Set(acc.Address, acc)
	cs.humanCount = 1

	expected := effectiveBalance(acc)

	cs.mu.Lock()
	_, err := cs.settleDemurrageLockedCtx(t.Context(), acc)
	cs.mu.Unlock()
	if err != nil {
		t.Fatalf("settle demurrage: %v", err)
	}

	after, _ := cs.accounts.Get("0xsettler")
	if math.Abs(after.Balance.Float()-expected.Float()) > 1e-6 {
		t.Errorf("settled balance %.6f does not match the balance effectiveBalance reported (%.6f)",
			after.Balance.Float(), expected.Float())
	}
	if after.Balance.Float() < registrationGrant-1e-6 {
		t.Errorf("settlement drove the balance to %.6f, below the %.0f fair-share floor",
			after.Balance.Float(), registrationGrant)
	}
}
