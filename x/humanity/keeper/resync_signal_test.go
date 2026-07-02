package keeper

import (
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
