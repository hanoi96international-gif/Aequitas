package keeper

import (
	"testing"
	"time"
)

// TestMinProductionDelayAfterSnapshotBoot_GateArithmetic is the regression
// guard for the 2026-07-10 fix closing Contabo1's repeated fork-within-
// first-30-45-blocks-of-every-RESYNC_FROM_SNAPSHOT-boot incident. Three
// height-derived gates (bootHeight, syncTargetHeight compared to raw
// dag.height, then to peerSyncHeight) were each confirmed live to still let
// self-production resume before doSyncOnce's bulk catch-up had genuinely
// finished, because dag.height (which the height-based gates all ultimately
// compare against) is the same field this node's own self-production
// advances — once production starts even once, it and any continuously-
// refreshed target climb in lockstep forever after. ProduceBlock now also
// requires secondsSinceStartup() >= minProductionDelayAfterSnapshotBoot for
// a snapshot-seeded boot (dag.bootHeight > 0), a wall-clock floor immune to
// every height-tracking hazard above. This test pins down the arithmetic
// that gate's condition (block.go) relies on.
func TestMinProductionDelayAfterSnapshotBoot_GateArithmetic(t *testing.T) {
	dag := newGhostdagTestDAG()

	dag.startupTime = time.Now().Unix()
	if elapsed := dag.secondsSinceStartup(); elapsed >= minProductionDelayAfterSnapshotBoot {
		t.Fatalf("immediately after startup, secondsSinceStartup() = %d, want < %d (settle window must still be active)",
			elapsed, minProductionDelayAfterSnapshotBoot)
	}

	dag.startupTime = time.Now().Unix() - minProductionDelayAfterSnapshotBoot - 5
	if elapsed := dag.secondsSinceStartup(); elapsed < minProductionDelayAfterSnapshotBoot {
		t.Fatalf("well past the settle window, secondsSinceStartup() = %d, want >= %d (gate must release)",
			elapsed, minProductionDelayAfterSnapshotBoot)
	}
}

// TestMinProductionDelayAfterSnapshotBoot_BelowSyncStallTimeout guards the
// two constants' relative ordering: the settle window must stay comfortably
// under syncStallTimeout, or a genuinely-down seed (whose gate the stall
// timeout exists to eventually release production from) would sit blocked
// even longer than intended, and — the specific incident this fix closes —
// comfortably above the ~30-45s window forks were confirmed live to form
// in without it.
func TestMinProductionDelayAfterSnapshotBoot_BelowSyncStallTimeout(t *testing.T) {
	if minProductionDelayAfterSnapshotBoot >= syncStallTimeout {
		t.Fatalf("minProductionDelayAfterSnapshotBoot (%d) must stay below syncStallTimeout (%d)",
			minProductionDelayAfterSnapshotBoot, syncStallTimeout)
	}
	if minProductionDelayAfterSnapshotBoot < 45 {
		t.Fatalf("minProductionDelayAfterSnapshotBoot (%d) must stay comfortably above the ~30-45s window live forks formed in without it",
			minProductionDelayAfterSnapshotBoot)
	}
}
