package keeper

import "testing"

// TestClaimDeepScanSlot_PerPeerIndependence is the regression guard for the
// 2026-07-04 incident where the deepScan cooldown was a single dag-wide
// timestamp shared across every peer's syncWithNode goroutine. One peer
// claiming the slot used to lock every OTHER peer out of deepScan for the
// same 30s window — confirmed live: a still-isolated secondary peer kept
// winning the shared slot, so Primary (the peer whose bulk catch-up
// actually mattered) never got its own deepScan turn at all. Each peer must
// get its own independent cooldown.
func TestClaimDeepScanSlot_PerPeerIndependence(t *testing.T) {
	dag := &BlockDAG{lastDeepScanAt: make(map[string]int64)}

	if !dag.claimDeepScanSlot("https://primary.example") {
		t.Fatal("a peer with no prior deepScan should be able to claim the slot immediately")
	}
	// Immediately re-claiming for the SAME peer must fail (within cooldown).
	if dag.claimDeepScanSlot("https://primary.example") {
		t.Fatal("the same peer must not claim a second deepScan slot within the cooldown window")
	}
	// A DIFFERENT peer must still be able to claim its own slot right away —
	// this is the exact bug: the old shared timestamp would have blocked
	// this too, purely because ANOTHER peer claimed a moment earlier.
	if !dag.claimDeepScanSlot("https://secondary.example") {
		t.Fatal("a different peer must get its own independent deepScan slot, not be blocked by another peer's claim")
	}
}
