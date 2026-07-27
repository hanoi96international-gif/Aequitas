package keeper

import (
	"testing"
	"time"
)

// busy_pct is the number the whole measurement exists for: an upper bound on
// how much of the time this node can accept transfers at all. If it were
// silently zero or absent, the measurement would look healthy precisely when
// it matters most.
func TestExclusiveLockStats_ReportsTheShareReadersAreLockedOut(t *testing.T) {
	resetExclusiveStats()
	trackExclusiveHold(time.Now().Add(-40*time.Millisecond), "test")
	trackExclusiveHold(time.Now().Add(-10*time.Millisecond), "test")

	st := ExclusiveLockStats()
	if got, _ := st["exclusive_holds"].(int64); got != 2 {
		t.Fatalf("expected 2 holds, got %d", got)
	}
	if got, _ := st["exclusive_max_ms"].(int64); got < 35 {
		t.Fatalf("max hold reported as %d ms; the 40 ms hold must be the maximum", got)
	}
	pct, ok := st["exclusive_busy_pct"].(float64)
	if !ok || pct <= 0 {
		t.Fatalf("busy_pct is %v — without it the stats cannot answer whether exclusive mode caps throughput", st["exclusive_busy_pct"])
	}
}

// A node that has never taken the lock exclusively must report zeroes rather
// than dividing by an unset start time.
func TestExclusiveLockStats_EmptyIsSafe(t *testing.T) {
	resetExclusiveStats()
	st := ExclusiveLockStats()
	if got, _ := st["exclusive_holds"].(int64); got != 0 {
		t.Fatalf("expected no holds, got %d", got)
	}
	if pct, _ := st["exclusive_busy_pct"].(float64); pct != 0 {
		t.Fatalf("busy_pct must be 0 with no measurements, got %v", pct)
	}
}

func resetExclusiveStats() {
	exclusiveHoldNanos.Store(0)
	exclusiveHoldCount.Store(0)
	exclusiveHoldMax.Store(0)
	exclusiveStatsFrom.Store(0)
}
