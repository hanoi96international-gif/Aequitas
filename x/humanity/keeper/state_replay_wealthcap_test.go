package keeper

import (
	"context"
	"testing"
)

// newWealthCapTestPool seeds n poor humans plus one "target" account,
// mirroring TestEnforceWealthCap_AboveCap_ExcessRedistributed's setup, so a
// swap or remove-liquidity operation that credits the target with enough AEQ
// pushes it over the wealth cap (25x the population average, floor 25 with
// this many humans) and gives both the primary and replay paths something
// real to cap.
func newWealthCapTestState(targetBalance float64) *ChainState {
	cs := newTestState()
	for i := 0; i < 25; i++ {
		addHuman(cs, humanAddr(i), 10)
	}
	addHuman(cs, "0xtarget", targetBalance)
	return cs
}

func humanAddr(i int) string {
	return "0xpoor" + string(rune('a'+i))
}

// TestApplySwapDeltaLocked_MirrorsPrimaryWealthCap is the regression guard
// for the 2026-07-04 brutal-audit P0 finding: applySwapDeltaLocked (the
// replay path secondaries use) never enforced the wealth cap that
// swapLocked (the primary path) applies after AEQ arrives via a tUSD->AEQ
// swap. Confirmed by direct comparison: run the SAME swap through both
// paths, starting from identical state, and assert they end at the
// identical (capped) balance — before this fix, the primary would cap and
// redistribute the excess while the replay path left the account
// uncapped, diverging AccountSetXOR and pool state between nodes.
func TestApplySwapDeltaLocked_MirrorsPrimaryWealthCap(t *testing.T) {
	const startBalance = 20_000.0 // comfortably above the 25x-average cap once AEQ arrives
	const startTUsd = 20_000.0    // funds the tUSD->AEQ swap below
	primary := newWealthCapTestState(startBalance)
	acct(primary, "0xtarget").TUsdBalance = NewDecimal(startTUsd)
	primary.pool = &PoolState{ReserveAEQ: NewDecimal(1_000_000), ReserveTUSD: NewDecimal(1_000_000), TotalLPShares: NewDecimal(1)}
	secondary := newWealthCapTestState(startBalance)
	acct(secondary, "0xtarget").TUsdBalance = NewDecimal(startTUsd)
	secondary.pool = &PoolState{ReserveAEQ: NewDecimal(1_000_000), ReserveTUSD: NewDecimal(1_000_000), TotalLPShares: NewDecimal(1)}

	const amountIn = 15_000.0 // tUSD in
	amountOut, _, err := primary.swapLocked(context.Background(), "0xtarget", amountIn, false, 0)
	if err != nil {
		t.Fatalf("primary swapLocked: %v", err)
	}

	if err := secondary.applySwapDeltaLocked("0xtarget", amountIn, amountOut, false, 0); err != nil {
		t.Fatalf("secondary applySwapDeltaLocked: %v", err)
	}

	primaryBal := acct(primary, "0xtarget").Balance.Float()
	secondaryBal := acct(secondary, "0xtarget").Balance.Float()
	if primaryBal != secondaryBal {
		t.Fatalf("primary and replay diverged: primary=%.6f secondary=%.6f (replay must mirror the primary's wealth-cap enforcement)", primaryBal, secondaryBal)
	}
	// Sanity: the cap must have actually bitten (otherwise this test proves
	// nothing) — the account should hold less than start+amountOut.
	if primaryBal >= startBalance+amountOut {
		t.Fatalf("test setup did not actually exceed the wealth cap — primary balance %.6f was never capped, adjust startBalance/amountIn", primaryBal)
	}
}

// TestRemoveLiquidityDeltaLocked_MirrorsPrimaryWealthCap is the equivalent
// regression guard for removeLiquidityDeltaLocked: removeLiquidityLocked
// (primary) calls touchActivity + enforceWealthCapLocked immediately after
// crediting the AEQ a wallet gets back from the pool; the replay path never
// mirrored either. Direct comparison as above.
func TestRemoveLiquidityDeltaLocked_MirrorsPrimaryWealthCap(t *testing.T) {
	const startBalance = 20_000.0
	const lpShares = 100.0
	primary := newWealthCapTestState(startBalance)
	acct(primary, "0xtarget").LPShares = NewDecimal(lpShares)
	primary.pool = &PoolState{ReserveAEQ: NewDecimal(1_000_000), ReserveTUSD: NewDecimal(1_000_000), TotalLPShares: NewDecimal(lpShares)}
	secondary := newWealthCapTestState(startBalance)
	acct(secondary, "0xtarget").LPShares = NewDecimal(lpShares)
	secondary.pool = &PoolState{ReserveAEQ: NewDecimal(1_000_000), ReserveTUSD: NewDecimal(1_000_000), TotalLPShares: NewDecimal(lpShares)}

	const sharesToBurn = 50.0
	_, _, _, err := primary.removeLiquidityLocked("0xtarget", sharesToBurn)
	if err != nil {
		t.Fatalf("primary removeLiquidityLocked: %v", err)
	}

	if err := secondary.removeLiquidityDeltaLocked("0xtarget", sharesToBurn, 0); err != nil {
		t.Fatalf("secondary removeLiquidityDeltaLocked: %v", err)
	}

	primaryBal := acct(primary, "0xtarget").Balance.Float()
	secondaryBal := acct(secondary, "0xtarget").Balance.Float()
	if primaryBal != secondaryBal {
		t.Fatalf("primary and replay diverged: primary=%.6f secondary=%.6f (replay must mirror the primary's wealth-cap enforcement)", primaryBal, secondaryBal)
	}
	if primaryBal >= startBalance+250_000 { // 50/100 shares of a 1,000,000 AEQ reserve = 500,000 AEQ, must have been capped down hard
		t.Fatalf("test setup did not actually exceed the wealth cap — primary balance %.6f was never capped", primaryBal)
	}
}
