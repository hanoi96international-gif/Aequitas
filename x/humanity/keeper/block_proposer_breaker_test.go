package keeper

import (
	"testing"
	"time"
)

// TestProposerBreaker_TripsAfterThreshold verifies a proposer whose blocks never
// attach trips exactly at the threshold and is then dropped lock-free.
func TestProposerBreaker_TripsAfterThreshold(t *testing.T) {
	dag := newGhostdagTestDAG()
	const p = "0xbadf00d"

	for i := 0; i < proposerBreakerFailThreshold-1; i++ {
		dag.recordProposerOutcome(p, false)
		if dag.proposerBlockBlocked(p) {
			t.Fatalf("breaker tripped early after %d failures (threshold %d)", i+1, proposerBreakerFailThreshold)
		}
	}
	dag.recordProposerOutcome(p, false) // the threshold-th failure trips it
	if !dag.proposerBlockBlocked(p) {
		t.Fatalf("breaker did not trip after %d consecutive failures", proposerBreakerFailThreshold)
	}
	// Case-insensitive: the same identity in a different case is still blocked.
	if !dag.proposerBlockBlocked("0xBADF00D") {
		t.Fatalf("breaker must key on the lowercased proposer")
	}
}

// TestProposerBreaker_SuccessResetsRun verifies a single attaching block clears
// the failure run so a healthy proposer never trips.
func TestProposerBreaker_SuccessResetsRun(t *testing.T) {
	dag := newGhostdagTestDAG()
	const p = "0xhealthy"

	for cycle := 0; cycle < 5; cycle++ {
		for i := 0; i < proposerBreakerFailThreshold-1; i++ {
			dag.recordProposerOutcome(p, false)
		}
		dag.recordProposerOutcome(p, true) // attaches — resets the run
		if dag.proposerBlockBlocked(p) {
			t.Fatalf("healthy proposer tripped: a success must reset the run (cycle %d)", cycle)
		}
	}
}

// TestProposerBreaker_CooldownExpiryAllowsProbe verifies that once the cooldown
// elapses the proposer is allowed through again and its state is cleared.
func TestProposerBreaker_CooldownExpiryAllowsProbe(t *testing.T) {
	dag := newGhostdagTestDAG()
	const p = "0xdiverged"

	for i := 0; i < proposerBreakerFailThreshold; i++ {
		dag.recordProposerOutcome(p, false)
	}
	if !dag.proposerBlockBlocked(p) {
		t.Fatalf("breaker should be open immediately after tripping")
	}
	dag.proposerBreakerMu.Lock()
	dag.proposerBreakerUntil[p] = time.Now().Add(-time.Second).UnixNano() // force cooldown expired
	dag.proposerBreakerMu.Unlock()

	if dag.proposerBlockBlocked(p) {
		t.Fatalf("breaker should let a probe through after the cooldown expires")
	}
	dag.proposerBreakerMu.Lock()
	run := dag.proposerFailRun[p]
	_, stillOpen := dag.proposerBreakerUntil[p]
	dag.proposerBreakerMu.Unlock()
	if run != 0 || stillOpen {
		t.Fatalf("cooldown expiry must clear run and until (run=%d open=%v)", run, stillOpen)
	}
}

// TestProposerBreaker_EmptyProposerNeverBlocks guards the nil/empty case so an
// unsigned or malformed block can never trip or be gated by the breaker (it is
// rejected by the signature/parent checks instead).
func TestProposerBreaker_EmptyProposerNeverBlocks(t *testing.T) {
	dag := newGhostdagTestDAG()
	for i := 0; i < proposerBreakerFailThreshold*2; i++ {
		dag.recordProposerOutcome("", false)
	}
	if dag.proposerBlockBlocked("") {
		t.Fatalf("empty proposer must never be blocked")
	}
}

// TestProposerBreaker_AddPeerBlockDropsTrippedProposer verifies the end-to-end
// wiring: once a proposer is tripped, AddPeerBlock drops its block at the
// lock-free top (returns false) rather than processing it.
func TestProposerBreaker_AddPeerBlockDropsTrippedProposer(t *testing.T) {
	dag := newGhostdagTestDAG()
	const p = "0xflooder"
	for i := 0; i < proposerBreakerFailThreshold; i++ {
		dag.recordProposerOutcome(p, false)
	}
	if !dag.proposerBlockBlocked(p) {
		t.Fatalf("precondition: proposer should be tripped")
	}
	// A block from the tripped proposer is dropped. (It would be rejected later
	// anyway for lacking a valid signature, but the breaker returns first — the
	// value here is that it does not panic and the wiring is exercised.)
	blk := &Block{Hash: "x", Height: 2, ParentHashes: []string{"genesis"}, Proposer: p}
	if dag.AddPeerBlock(blk) {
		t.Fatalf("AddPeerBlock must drop a block from a tripped proposer")
	}
}
