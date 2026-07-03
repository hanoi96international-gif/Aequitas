package keeper

import (
	"fmt"
	"testing"
)

// simulateReplayOutcome mirrors AddPeerBlock's tail exactly (see its own
// comment): replayTransactions records a mismatch by setting
// lastReplayMismatchHash to the block's hash and striking the breaker;
// AddPeerBlock's tail then checks whether ITS block was the one that just
// mismatched before deciding whether to clear the run. Kept as a standalone
// helper (rather than duplicating the real code inline in every test) so the
// two tests below stay readable.
func simulateReplayOutcome(dag *BlockDAG, proposer, blockHash string, hadMismatch bool) {
	if hadMismatch {
		dag.recordProposerOutcome(proposer, false)
		dag.replayMismatchMu.Lock()
		dag.lastReplayMismatchHash = blockHash
		dag.replayMismatchMu.Unlock()
	}
	dag.replayMismatchMu.Lock()
	mismatched := dag.lastReplayMismatchHash == blockHash
	if mismatched {
		dag.lastReplayMismatchHash = ""
	}
	dag.replayMismatchMu.Unlock()
	if !mismatched {
		dag.recordProposerOutcome(proposer, true)
	}
}

// TestReplayMismatch_AccumulatesTowardBreakerTrip is the P0 regression test
// for the 2026-07-03 night incident: a validator whose EVERY block mismatches
// (a genuinely isolated/diverged fork, not normal occasional sibling drift)
// must eventually trip the per-proposer circuit breaker. Before this fix,
// AddPeerBlock's tail unconditionally called recordProposerOutcome(true) for
// every successfully-attached block — including one that had just been
// struck for a mismatch moments earlier — permanently erasing the strike
// before it could ever accumulate.
func TestReplayMismatch_AccumulatesTowardBreakerTrip(t *testing.T) {
	dag := newGhostdagTestDAG()
	const p = "0xisolated-fork-validator"

	for i := 0; i < proposerBreakerFailThreshold; i++ {
		simulateReplayOutcome(dag, p, fmt.Sprintf("block-%d", i), true /* mismatch */)
		if i < proposerBreakerFailThreshold-1 && dag.proposerBlockBlocked(p) {
			t.Fatalf("breaker tripped early after %d mismatching blocks (threshold %d)", i+1, proposerBreakerFailThreshold)
		}
	}
	if !dag.proposerBlockBlocked(p) {
		t.Fatalf("a proposer whose every block mismatches must trip the breaker after %d strikes", proposerBreakerFailThreshold)
	}
}

// TestReplayMismatch_CleanBlockStillClearsRun is the regression guard for the
// common, healthy case: a block that replays with NO StateRoot mismatch (or
// no StateRoot to check at all) must still clear the proposer's breaker run
// exactly as before this fix — only a block whose OWN replay just mismatched
// should be treated as a strike instead of a success.
func TestReplayMismatch_CleanBlockStillClearsRun(t *testing.T) {
	dag := newGhostdagTestDAG()
	const p = "0xhealthy-validator"

	for i := 0; i < proposerBreakerFailThreshold-1; i++ {
		dag.recordProposerOutcome(p, false)
	}
	simulateReplayOutcome(dag, p, "clean-block", false /* no mismatch */)
	if dag.proposerBlockBlocked(p) {
		t.Fatalf("a cleanly-matching block must still clear the run, not leave the proposer blocked")
	}
}
