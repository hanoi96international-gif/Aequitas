package keeper

import (
	"fmt"
	"testing"
	"time"
)

// TestRecordResyncSignal_SinglePeerNeverTriggers verifies one peer alone can
// never force the threshold, no matter how many times it re-signals.
func TestRecordResyncSignal_SinglePeerNeverTriggers(t *testing.T) {
	dag := newGhostdagTestDAG()
	for i := 0; i < 5; i++ {
		if dag.recordResyncSignal("http://peer-a") {
			t.Fatalf("a single peer must never trigger the threshold alone (attempt %d)", i+1)
		}
	}
}

// TestRecordResyncSignal_TwoDistinctPeersTrigger verifies the threshold fires
// once resyncSignalThreshold distinct peers have signaled.
func TestRecordResyncSignal_TwoDistinctPeersTrigger(t *testing.T) {
	dag := newGhostdagTestDAG()
	if dag.recordResyncSignal("http://peer-a") {
		t.Fatalf("first distinct peer must not trigger alone")
	}
	if !dag.recordResyncSignal("http://peer-b") {
		t.Fatalf("second distinct peer must trigger the threshold")
	}
}

// TestRecordResyncSignal_StaleSignalDoesNotCount verifies a signal outside
// resyncSignalWindow no longer counts toward the threshold.
func TestRecordResyncSignal_StaleSignalDoesNotCount(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.resyncSignalFrom = map[string]int64{
		"http://peer-a": time.Now().Add(-(resyncSignalWindow + time.Second)).Unix(),
	}
	if dag.recordResyncSignal("http://peer-b") {
		t.Fatalf("a stale signal outside the window must not count toward the threshold")
	}
}

// withRegisteredPeers registers count fake active peers into
// GlobalPeerRegistry (a package-level global — see its own comment) for the
// duration of the test, and restores the registry to empty afterward so
// this doesn't leak state into other tests sharing the same process.
func withRegisteredPeers(t *testing.T, count int) {
	t.Helper()
	GlobalPeerRegistry.mu.Lock()
	saved := GlobalPeerRegistry.peers
	GlobalPeerRegistry.peers = make(map[string]time.Time, count)
	for i := 0; i < count; i++ {
		GlobalPeerRegistry.peers[fmt.Sprintf("http://peer-%d", i)] = time.Now()
	}
	GlobalPeerRegistry.mu.Unlock()
	t.Cleanup(func() {
		GlobalPeerRegistry.mu.Lock()
		GlobalPeerRegistry.peers = saved
		GlobalPeerRegistry.mu.Unlock()
	})
}

// TestResyncSignalThresholdFor_FloorAtSmallPeerCount verifies the exact
// today's-scale case (2 known peers) resolves to the same threshold (2) the
// fixed constant this replaces already had — no behavior change at current
// network size.
func TestResyncSignalThresholdFor_FloorAtSmallPeerCount(t *testing.T) {
	dag := newGhostdagTestDAG()
	withRegisteredPeers(t, 2)
	if got := dag.resyncSignalThresholdFor(""); got != 2 {
		t.Fatalf("2 known peers: want threshold 2 (matches the old fixed constant), got %d", got)
	}
}

// TestResyncSignalThresholdFor_NeverBelowFloor verifies a genuinely solo or
// near-solo node (0-1 known peers) still requires minResyncSignalThreshold,
// not fewer — the formula alone (known/2+1) would otherwise drop below 2.
func TestResyncSignalThresholdFor_NeverBelowFloor(t *testing.T) {
	dag := newGhostdagTestDAG()
	withRegisteredPeers(t, 0)
	if got := dag.resyncSignalThresholdFor(""); got != minResyncSignalThreshold {
		t.Fatalf("0 known peers: want the floor %d, got %d", minResyncSignalThreshold, got)
	}
}

// TestResyncSignalThresholdFor_ScalesToMajorityAtLargePeerCount verifies the
// actual audit item this fixes: at a much larger peer target, the threshold
// scales to a real majority instead of staying pinned at a small absolute
// count a tiny minority of peers could reach.
func TestResyncSignalThresholdFor_ScalesToMajorityAtLargePeerCount(t *testing.T) {
	dag := newGhostdagTestDAG()
	withRegisteredPeers(t, 100)
	want := 100/2 + 1
	if got := dag.resyncSignalThresholdFor(""); got != want {
		t.Fatalf("100 known peers: want majority threshold %d, got %d", want, got)
	}
}
