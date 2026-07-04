package main

import (
	"testing"
	"time"
)

func berlinLoc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("LoadLocation(Europe/Berlin): %v", err)
	}
	return loc
}

// TestNextDaily20_SameDayBeforeSlot verifies a time before today's 20:00
// resolves to today's 20:00, not tomorrow's.
func TestNextDaily20_SameDayBeforeSlot(t *testing.T) {
	berlin := berlinLoc(t)
	now := time.Date(2026, 7, 5, 14, 0, 0, 0, berlin)
	got := nextDaily20(now, berlin)
	want := time.Date(2026, 7, 5, 20, 0, 0, 0, berlin)
	if !got.Equal(want) {
		t.Fatalf("nextDaily20(%v) = %v, want %v", now, got, want)
	}
}

// TestNextDaily20_AfterSlotRollsToTomorrow verifies a time already past
// today's 20:00 resolves to tomorrow's 20:00.
func TestNextDaily20_AfterSlotRollsToTomorrow(t *testing.T) {
	berlin := berlinLoc(t)
	now := time.Date(2026, 7, 5, 23, 17, 0, 0, berlin)
	got := nextDaily20(now, berlin)
	want := time.Date(2026, 7, 6, 20, 0, 0, 0, berlin)
	if !got.Equal(want) {
		t.Fatalf("nextDaily20(%v) = %v, want %v", now, got, want)
	}
}

// TestComputeFirstUBITarget_FreshDBGuardsAgainstImmediateFire is the
// regression guard for FIX 10: a brand-new node (lastAt == 0) must not fire
// distribution immediately, even if "now" happens to be right at 20:00.
func TestComputeFirstUBITarget_FreshDBGuardsAgainstImmediateFire(t *testing.T) {
	berlin := berlinLoc(t)
	now := time.Date(2026, 7, 5, 20, 0, 0, 0, berlin)
	got := computeFirstUBITarget(0, now, berlin)
	if !got.After(now.Add(time.Hour - time.Second)) {
		t.Fatalf("computeFirstUBITarget(0, %v) = %v, want at least 1h out (fresh-DB guard)", now, got)
	}
}

// TestComputeFirstUBITarget_OverdueCatchesUpImmediately is the regression
// guard for the 2026-07-03 fix: once a full 24h has passed since the last
// distribution, the next attempt must be almost immediate, not deferred to
// the next calendar 20:00 slot.
func TestComputeFirstUBITarget_OverdueCatchesUpImmediately(t *testing.T) {
	berlin := berlinLoc(t)
	now := time.Date(2026, 7, 5, 23, 0, 0, 0, berlin)
	lastAt := now.Add(-25 * time.Hour).Unix()
	got := computeFirstUBITarget(lastAt, now, berlin)
	if got.After(now.Add(5 * time.Minute)) {
		t.Fatalf("computeFirstUBITarget() = %v, want an almost-immediate catch-up (within a few minutes of %v) for an overdue (>24h) last distribution", got, now)
	}
}

// TestComputeFirstUBITarget_RestartRightAfterMissedSlotCatchesUpToday is the
// regression guard for the 2026-07-05 fix: a restart landing shortly AFTER
// today's 20:00 slot, whose last real distribution predates that slot (but
// is under 24h old — e.g. only 23h50m), must catch up today, not silently
// defer to tomorrow's slot the way the bare `nextDaily20(now)` fallback did.
func TestComputeFirstUBITarget_RestartRightAfterMissedSlotCatchesUpToday(t *testing.T) {
	berlin := berlinLoc(t)
	// Last distribution was yesterday at 20:05 — 23h55m before "now".
	now := time.Date(2026, 7, 5, 20, 0, 0, 0, berlin)
	lastAt := time.Date(2026, 7, 4, 20, 5, 0, 0, berlin).Unix()
	got := computeFirstUBITarget(lastAt, now, berlin)
	if got.After(now.Add(5 * time.Minute)) {
		t.Fatalf("computeFirstUBITarget() = %v, want an almost-immediate catch-up for today's missed slot (last distribution %v predates today's 20:00, now is %v)", got, time.Unix(lastAt, 0).In(berlin), now)
	}
}

// TestComputeFirstUBITarget_NormalDayWaitsForTodaysSlot verifies the
// ordinary, non-overdue, non-restart-race case still waits for today's
// scheduled slot rather than firing early — this fix must not make the
// distribution MORE eager than intended in the common case.
func TestComputeFirstUBITarget_NormalDayWaitsForTodaysSlot(t *testing.T) {
	berlin := berlinLoc(t)
	now := time.Date(2026, 7, 5, 14, 0, 0, 0, berlin)
	lastAt := time.Date(2026, 7, 4, 20, 0, 0, 0, berlin).Unix() // 18h ago
	got := computeFirstUBITarget(lastAt, now, berlin)
	want := time.Date(2026, 7, 5, 20, 0, 0, 0, berlin)
	if !got.Equal(want) {
		t.Fatalf("computeFirstUBITarget() = %v, want today's normal 20:00 slot %v", got, want)
	}
}

// TestComputeFirstUBITarget_AlreadyRanTodayWaitsForTomorrow verifies that
// once today's distribution has already run (lastAt is after today's
// 20:00), a restart later the same evening must NOT re-catch-up — it
// should wait for tomorrow's slot like normal.
func TestComputeFirstUBITarget_AlreadyRanTodayWaitsForTomorrow(t *testing.T) {
	berlin := berlinLoc(t)
	now := time.Date(2026, 7, 5, 23, 0, 0, 0, berlin)
	lastAt := time.Date(2026, 7, 5, 20, 0, 30, 0, berlin).Unix() // ran today, just after the slot
	got := computeFirstUBITarget(lastAt, now, berlin)
	want := time.Date(2026, 7, 6, 20, 0, 0, 0, berlin)
	if !got.Equal(want) {
		t.Fatalf("computeFirstUBITarget() = %v, want tomorrow's slot %v since today's distribution already ran at %v", got, want, time.Unix(lastAt, 0).In(berlin))
	}
}
