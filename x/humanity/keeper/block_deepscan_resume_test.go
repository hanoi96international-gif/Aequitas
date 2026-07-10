package keeper

import "testing"

// TestDeepScanResumeHeight_DefaultsToZero is the baseline: a peer that has
// never had a bounded deepScan pass run against it must report 0 (no resume
// in progress — doSyncOnce falls back to deepScanFloor()), matching a fresh
// map's zero-value read exactly like lastDeepScanAt's existing pattern.
func TestDeepScanResumeHeight_DefaultsToZero(t *testing.T) {
	dag := &BlockDAG{deepScanResumeHeight: make(map[string]int64)}
	if got := dag.getDeepScanResumeHeight("https://peer.example"); got != 0 {
		t.Fatalf("getDeepScanResumeHeight() for an unseen peer = %d, want 0", got)
	}
}

// TestDeepScanResumeHeight_RoundTrips is the regression guard for the
// 2026-07-10 fix (see deepScanResumeHeight's own struct comment, block.go):
// a bounded deepScan pass that exhausts its page budget mid-walk must
// persist exactly where it stopped, so the NEXT call resumes forward
// instead of restarting the full historical walk from the floor — the
// mechanism that closes the "Primary never merges a single live block from
// a peer with a large historical gap" incident (confirmed live: 174-407s
// raw arrival latency from one unbounded deepScan pass blocking the entire
// per-peer sync loop).
func TestDeepScanResumeHeight_RoundTrips(t *testing.T) {
	dag := &BlockDAG{deepScanResumeHeight: make(map[string]int64)}
	dag.setDeepScanResumeHeight("https://peer.example", 680000)
	if got := dag.getDeepScanResumeHeight("https://peer.example"); got != 680000 {
		t.Fatalf("getDeepScanResumeHeight() after set(680000) = %d, want 680000", got)
	}
	// A later call advancing further must overwrite, not accumulate.
	dag.setDeepScanResumeHeight("https://peer.example", 685000)
	if got := dag.getDeepScanResumeHeight("https://peer.example"); got != 685000 {
		t.Fatalf("getDeepScanResumeHeight() after set(685000) = %d, want 685000 (overwrite, not accumulate)", got)
	}
}

// TestDeepScanResumeHeight_ResetToZeroOnCompletedSweep guards the "full
// sweep actually finished" path: once a deepScan pass reaches the peer's
// real tip (a genuine empty page, not just a short one), doSyncOnce resets
// the resume height back to 0 — not to wherever the walk happened to stop —
// so a LATER deepScan triggered by a fresh, unrelated orphan starts a
// genuine full sweep from the true floor again instead of silently
// resuming from "already at the tip" and skipping over the new gap it was
// actually triggered to find.
func TestDeepScanResumeHeight_ResetToZeroOnCompletedSweep(t *testing.T) {
	dag := &BlockDAG{deepScanResumeHeight: make(map[string]int64)}
	dag.setDeepScanResumeHeight("https://peer.example", 687000) // mid-walk
	dag.setDeepScanResumeHeight("https://peer.example", 0)      // sweep completed
	if got := dag.getDeepScanResumeHeight("https://peer.example"); got != 0 {
		t.Fatalf("getDeepScanResumeHeight() after a completed sweep = %d, want 0 (reset, so a future deepScan starts a genuine full sweep)", got)
	}
}

// TestDeepScanResumeHeight_PerPeerIndependence mirrors
// TestClaimDeepScanSlot_PerPeerIndependence's exact rationale (the
// 2026-07-04 incident where a SHARED timestamp let one peer's state starve
// another's): each peer's resume progress must be tracked independently, so
// one peer being deep in a bounded walk never affects another peer's own
// (possibly already-complete, possibly not-yet-started) progress.
func TestDeepScanResumeHeight_PerPeerIndependence(t *testing.T) {
	dag := &BlockDAG{deepScanResumeHeight: make(map[string]int64)}
	dag.setDeepScanResumeHeight("https://primary.example", 680000)
	if got := dag.getDeepScanResumeHeight("https://secondary.example"); got != 0 {
		t.Fatalf("a different peer's resume height = %d, want 0 (unaffected by primary.example's progress)", got)
	}
	if got := dag.getDeepScanResumeHeight("https://primary.example"); got != 680000 {
		t.Fatalf("primary.example's own resume height = %d, want 680000 (unaffected by reading a different peer)", got)
	}
}
