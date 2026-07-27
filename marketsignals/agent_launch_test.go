package marketsignals

import (
	"strings"
	"testing"
)

// cleanLaunch is a token that passes every disqualifying check. Tests below
// start from it and break one thing at a time, which is the only way to be
// sure a rejection came from the reason under test.
func cleanLaunch() Launch {
	return Launch{
		Symbol:                "CLEAN",
		Chain:                 "base",
		TokenAddress:          "0xclean",
		AgeMinutes:            720,
		LiquidityUSD:          250_000,
		LiquidityLockedPct:    100,
		LockRemainingDays:     365,
		LPBurnedPct:           0,
		MintAuthorityActive:   false,
		FreezeAuthorityActive: false,
		UpgradeableProxy:      false,
		ContractVerified:      true,
		TopHolderPct:          4,
		Top10Pct:              18,
		HolderCount:           2400,
		SniperPct:             6,
		SellSimulationPassed:  true,
		BuyTaxPct:             0,
		SellTaxPct:            0,
		VolumeUSD1h:           400_000,
		TxCount1h:             1800,
		UniqueBuyers1h:        900,
		DeployerPriorRugs:     0,
		DeployerPriorTokens:   3,
		Checks:                AllChecksPerformed(),
	}
}

func TestLaunch_AcceptsACleanDeployment(t *testing.T) {
	got := NewLaunchAgent().Screen(cleanLaunch())
	if got.Verdict != Accept {
		t.Fatalf("verdict %s for a launch that fails nothing:\n%s", got.Verdict, got.Summary())
	}
	if len(got.HardFails) != 0 {
		t.Fatalf("unexpected disqualifications: %v", got.HardFails)
	}
}

// TestLaunch_VetoesOutrankScore is the single most important property of this
// screener. A rug is not a token with bad numbers; it is a token with
// excellent numbers and one fatal property. A weighted model that lets deep
// liquidity and a wide holder base outvote a live mint authority will buy
// exactly the tokens that are engineered to look good.
func TestLaunch_VetoesOutrankScore(t *testing.T) {
	vetoes := []struct {
		name   string
		mutate func(*Launch)
		expect string
	}{
		{"honeypot", func(l *Launch) { l.SellSimulationPassed = false }, "honeypot"},
		{"mint authority", func(l *Launch) { l.MintAuthorityActive = true }, "mint authority"},
		{"freeze authority", func(l *Launch) { l.FreezeAuthorityActive = true }, "freeze authority"},
		{"upgradeable proxy", func(l *Launch) { l.UpgradeableProxy = true }, "upgradeable proxy"},
		{"unverified source", func(l *Launch) { l.ContractVerified = false }, "unverified source"},
		{"punitive sell tax", func(l *Launch) { l.SellTaxPct = 25 }, "sell tax"},
		{"unlocked LP", func(l *Launch) { l.LiquidityLockedPct = 10 }, "locked or burned"},
		{"expiring lock", func(l *Launch) { l.LockRemainingDays = 2 }, "lock expires"},
		{"deployer rug history", func(l *Launch) { l.DeployerPriorRugs = 2 }, "prior rug"},
		{"mixer-funded deployer", func(l *Launch) { l.DeployerFundedByMixer = true }, "mixer"},
		{"whale holder", func(l *Launch) { l.TopHolderPct = 40 }, "top holder"},
		{"sniper capture", func(l *Launch) { l.SniperPct = 60 }, "snipers"},
		{"too young", func(l *Launch) { l.AgeMinutes = 4 }, "minutes old"},
	}

	a := NewLaunchAgent()
	for _, tc := range vetoes {
		t.Run(tc.name, func(t *testing.T) {
			// Everything else is not merely fine but outstanding, so only the
			// veto can produce a rejection.
			l := cleanLaunch()
			l.LiquidityUSD = 10_000_000
			l.HolderCount = 100_000
			l.UniqueBuyers1h = 20_000
			l.TxCount1h = 30_000
			l.Top10Pct = 5
			tc.mutate(&l)

			got := a.Screen(l)
			if got.Verdict != Reject {
				t.Fatalf("verdict %s despite %s:\n%s", got.Verdict, tc.name, got.Summary())
			}
			if got.Score != 0 {
				t.Fatalf("a disqualified launch scored %.0f — a score on a rejected token "+
					"invites someone to trade it anyway", got.Score)
			}
			if !strings.Contains(strings.Join(got.HardFails, " "), tc.expect) {
				t.Fatalf("hard fails %v do not mention %q", got.HardFails, tc.expect)
			}
		})
	}
}

// TestLaunch_CatchesWashTrading: volume is free to fabricate, distinct
// counterparties are not. A token with enormous volume across very few
// wallets is a wallet trading with itself to appear on a volume leaderboard.
func TestLaunch_CatchesWashTrading(t *testing.T) {
	l := cleanLaunch()
	l.VolumeUSD1h = 5_000_000
	l.TxCount1h = 4000
	l.UniqueBuyers1h = 120 // 0.03 buyers per trade

	got := NewLaunchAgent().Screen(l)
	if got.Verdict != Reject {
		t.Fatalf("verdict %s for volume concentrated in a handful of wallets:\n%s",
			got.Verdict, got.Summary())
	}
	if !strings.Contains(strings.Join(got.HardFails, " "), "wash trading") {
		t.Fatalf("hard fails %v should name wash trading", got.HardFails)
	}
}

func TestLaunch_BurnedLPNeedsNoLockDuration(t *testing.T) {
	l := cleanLaunch()
	l.LiquidityLockedPct = 0
	l.LPBurnedPct = 100
	l.LockRemainingDays = 0 // irrelevant: burned liquidity cannot be withdrawn

	if got := NewLaunchAgent().Screen(l); got.Verdict == Reject {
		t.Fatalf("burned LP was rejected for lock duration:\n%s", got.Summary())
	}
}

func TestLaunch_WatchesAThinButHonestLaunch(t *testing.T) {
	l := cleanLaunch()
	// Nothing disqualifying, but only just past every minimum: not a scam,
	// not yet evidence of anything either.
	l.LiquidityUSD = 30_000
	l.HolderCount = 260
	l.UniqueBuyers1h = 110
	l.TxCount1h = 400
	l.AgeMinutes = 45
	l.Top10Pct = 33

	got := NewLaunchAgent().Screen(l)
	if got.Verdict != Watch {
		t.Fatalf("verdict %s, want watch for a marginal but honest launch:\n%s",
			got.Verdict, got.Summary())
	}
}

func TestLaunch_FlagsAnAnonymousDeployerAsAConcern(t *testing.T) {
	l := cleanLaunch()
	l.DeployerPriorTokens = 0

	got := NewLaunchAgent().Screen(l)
	if strings.Contains(strings.Join(got.Concerns, " "), "no prior tokens") {
		return
	}
	t.Fatalf("a deployer with no history should be recorded as an unknown, not read as "+
		"a clean record; concerns were %v", got.Concerns)
}

// TestLaunch_UncheckedIsNotClean is the veto that a real data source forces
// into existence. Dexscreener and its peers publish prices, not authorities,
// so a Launch built from one has MintAuthorityActive == false purely because
// nobody looked — and false is the value that PASSES. The screener must
// reject for the absence of the check, not accept for the absence of a
// finding.
func TestLaunch_UncheckedIsNotClean(t *testing.T) {
	// Everything about this token is excellent, and nothing about it was
	// verified. This is exactly the shape of a record assembled from a price
	// aggregator.
	l := cleanLaunch()
	l.LiquidityUSD = 5_000_000
	l.HolderCount = 40_000
	l.Checks = LaunchChecks{}

	got := NewLaunchAgent().Screen(l)
	if got.Verdict != Reject {
		t.Fatalf("verdict %s for a token nobody inspected:\n%s", got.Verdict, got.Summary())
	}
	joined := strings.Join(got.HardFails, " ")
	for _, want := range []string{
		"no sell was simulated",
		"authorities were never inspected",
		"LP lock or burn was never verified",
		"deployer was never traced",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("hard fails do not mention %q: %v", want, got.HardFails)
		}
	}

	// One missing check is enough. Partial diligence is still diligence that
	// did not cover the thing it missed.
	partial := cleanLaunch()
	partial.Checks.Authorities = false
	if got := NewLaunchAgent().Screen(partial); got.Verdict != Reject {
		t.Fatalf("verdict %s with only the authority check skipped:\n%s",
			got.Verdict, got.Summary())
	}
}

// TestLaunch_MostRealisticLaunchesAreRejected states the base rate plainly.
// If this screener ever starts approving most of what it sees, it has been
// tuned toward the thing it exists to prevent.
func TestLaunch_MostRealisticLaunchesAreRejected(t *testing.T) {
	population := []Launch{
		func() Launch { l := cleanLaunch(); l.MintAuthorityActive = true; return l }(),
		func() Launch { l := cleanLaunch(); l.SellSimulationPassed = false; return l }(),
		func() Launch { l := cleanLaunch(); l.LiquidityLockedPct = 0; return l }(),
		func() Launch { l := cleanLaunch(); l.DeployerPriorRugs = 1; return l }(),
		func() Launch { l := cleanLaunch(); l.SniperPct = 55; return l }(),
		func() Launch { l := cleanLaunch(); l.AgeMinutes = 3; return l }(),
		func() Launch { l := cleanLaunch(); l.HolderCount = 40; return l }(),
		func() Launch { l := cleanLaunch(); l.SellTaxPct = 12; return l }(),
		cleanLaunch(),
	}

	a := NewLaunchAgent()
	rejected := 0
	for _, l := range population {
		if a.Screen(l).Verdict == Reject {
			rejected++
		}
	}
	if rejected != len(population)-1 {
		t.Fatalf("rejected %d of %d; every launch here but the clean one is disqualified",
			rejected, len(population))
	}
}
