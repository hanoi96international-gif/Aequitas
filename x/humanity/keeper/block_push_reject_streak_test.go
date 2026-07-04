package keeper

import "testing"

// TestRecordPushRejection_BenignOrphanNeverCounts is the regression guard
// for the 2026-07-04 brutal-audit finding "sender ignores almost all push
// rejections": HTTPBroadcastBlock used to react ONLY to action:
// "resync_required", silently discarding every other response. This test
// verifies the specific carve-out that must stay silent: "orphaned, within
// grace period" is the expected shape of ordinary cross-network
// propagation lag (see proposerBreakerOrphanGrace's identical reasoning on
// the receiving side) and must never advance the rejection streak, no
// matter how many times it happens.
func TestRecordPushRejection_BenignOrphanNeverCounts(t *testing.T) {
	dag := &BlockDAG{}
	for i := 0; i < pushRejectStreakThreshold*3; i++ {
		dag.recordPushRejection("http://peer.example", false, "orphaned, within grace period")
	}
	dag.pushRejectStreakMu.Lock()
	streak := dag.pushRejectStreak["http://peer.example"]
	dag.pushRejectStreakMu.Unlock()
	if streak != 0 {
		t.Fatalf("benign within-grace orphan responses must never count toward the rejection streak, got streak=%d", streak)
	}
}

// TestRecordPushRejection_NonBenignRejectionAccumulates verifies a genuine
// (non-benign) rejection reason DOES advance the per-peer streak — the
// exact signal the old code discarded entirely.
func TestRecordPushRejection_NonBenignRejectionAccumulates(t *testing.T) {
	dag := &BlockDAG{}
	dag.recordPushRejection("http://peer.example", false, "rejected or already known")
	dag.pushRejectStreakMu.Lock()
	streak := dag.pushRejectStreak["http://peer.example"]
	dag.pushRejectStreakMu.Unlock()
	if streak != 1 {
		t.Fatalf("a non-benign rejection must advance the streak, got streak=%d, want 1", streak)
	}
}

// TestRecordPushRejection_AcceptedClearsStreak verifies a subsequent
// accepted push (ok:true) resets a peer's streak — a peer that was
// rejecting us and then starts accepting again is healthy, not still
// under suspicion.
func TestRecordPushRejection_AcceptedClearsStreak(t *testing.T) {
	dag := &BlockDAG{}
	dag.recordPushRejection("http://peer.example", false, "rejected or already known")
	dag.recordPushRejection("http://peer.example", false, "rejected or already known")
	dag.recordPushRejection("http://peer.example", true, "")
	dag.pushRejectStreakMu.Lock()
	_, tracked := dag.pushRejectStreak["http://peer.example"]
	dag.pushRejectStreakMu.Unlock()
	if tracked {
		t.Fatal("an accepted push must clear the peer's rejection streak entirely")
	}
}

// TestRecordPushRejection_StreakResetsAfterThreshold verifies the streak is
// reset once it crosses pushRejectStreakThreshold (whether or not the
// downstream auto-heal gate actually fires — that depends on env vars this
// test doesn't set), so a peer that keeps rejecting doesn't re-fire the
// reaction on every single subsequent push once past the threshold.
func TestRecordPushRejection_StreakResetsAfterThreshold(t *testing.T) {
	dag := &BlockDAG{}
	for i := 0; i < pushRejectStreakThreshold; i++ {
		dag.recordPushRejection("http://peer.example", false, "rejected or already known")
	}
	dag.pushRejectStreakMu.Lock()
	streak := dag.pushRejectStreak["http://peer.example"]
	dag.pushRejectStreakMu.Unlock()
	if streak != 0 {
		t.Fatalf("streak must reset to 0 once the threshold is crossed and acted on, got %d", streak)
	}
}
