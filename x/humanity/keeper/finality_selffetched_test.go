package keeper

import "testing"

// Regression test for the definitive root cause of the 2026-07-06 3-node
// non-merging incident: isFinalityViolation exempted FromSync blocks but not
// SelfFetched ones, even though both the circuit breaker (block.go) and the
// suspension gate (block.go) already treat them identically — a block this
// node deliberately fetched (via fetchMissingAncestors, to resolve a real
// orphan) can never be irrelevant old history, by definition something else
// needs it as a direct ancestor right now. Without this exemption, a
// genuinely-needed ancestor fetched from a non-seed peer (FromSync only
// applies to a configured seed) was rejected forever once the local
// finalized checkpoint climbed past it — which happens quickly under
// continuous self-production — permanently orphaning every descendant block
// and preventing the whole network from merging.
//
// FIX (same day, second half of the same incident): SelfFetched alone turned
// out to be too broad an exemption — see hasAwaitingOrphan's own comment for
// why "a fetch was once issued for this hash" (past tense) isn't the same
// claim as "something is still waiting on it now" (present tense). Without
// requiring hasAwaitingOrphan too, a peer relaying a large backlog of its
// own long-past self-produced blocks cascaded through as an ever-deepening,
// thousands-deep orphan chain instead of being cleanly rejected as stale.
func TestIsFinalityViolation_SelfFetchedAncestorExempt(t *testing.T) {
	cs := newTestState()
	cs.SetFinalizedCheckpoint("deadbeef", 1000, 5000)
	dag := &BlockDAG{state: cs, orphans: make(map[string][]*Block)}

	oldBlock := &Block{Height: 500, Hash: "old-ancestor"} // far below 1000-50

	if !dag.isFinalityViolation(oldBlock) {
		t.Fatal("an ordinary old block (not FromSync, not SelfFetched) must still be a finality violation")
	}

	oldBlock.SelfFetched = true
	if !dag.isFinalityViolation(oldBlock) {
		t.Fatal("a SelfFetched block nothing is currently waiting on must NOT be exempt — it's stale history, not a genuinely-needed ancestor")
	}

	// Now something is genuinely, actively waiting on this exact hash.
	dag.orphans[oldBlock.Hash] = []*Block{{Height: 501, Hash: "waiting-child"}}
	if dag.isFinalityViolation(oldBlock) {
		t.Fatal("a SelfFetched block with a genuinely-waiting child must be exempt from the finality gate, regardless of how far below the checkpoint it is")
	}

	oldBlock.SelfFetched = false
	oldBlock.FromSync = true
	if dag.isFinalityViolation(oldBlock) {
		t.Fatal("a FromSync (trusted seed) block must remain unconditionally exempt, as before")
	}
}

// Regression test for the 2026-07-19 fix: both Contabo1 and Contabo2 stuck
// permanently unable to advance their finality checkpoint (and, downstream,
// permanently unable to resume block production) on a distinct missing
// self-produced block each — present and fetchable from Primary the whole
// time, rejected by isFinalityViolation on every single retry.
// registerFinalityWalkGap/registerProduceStuckGap deliberately write into
// their own maps instead of dag.orphans (see each one's own comment — the
// wantDeepScan-avoidance fix this preserves), but hasAwaitingOrphan used to
// check ONLY dag.orphans, so a SelfFetched block genuinely needed to close
// one of those two gap types was never recognized as "awaited" and never
// passed the finality-violation exemption.
func TestIsFinalityViolation_FinalityWalkGapAncestorExempt(t *testing.T) {
	cs := newTestState()
	cs.SetFinalizedCheckpoint("deadbeef", 1000, 5000)
	dag := &BlockDAG{state: cs, orphans: make(map[string][]*Block), finalityWalkGaps: make(map[string]bool)}

	oldBlock := &Block{Height: 500, Hash: "old-ancestor", SelfFetched: true}
	if !dag.isFinalityViolation(oldBlock) {
		t.Fatal("a SelfFetched block nothing has registered a gap for must still be a finality violation")
	}

	dag.finalityWalkGaps[oldBlock.Hash] = true
	if dag.isFinalityViolation(oldBlock) {
		t.Fatal("a SelfFetched block genuinely needed to close a registered finality-checkpoint-walk gap must be exempt, regardless of how far below the checkpoint it is")
	}
}

func TestIsFinalityViolation_ProduceStuckGapAncestorExempt(t *testing.T) {
	cs := newTestState()
	cs.SetFinalizedCheckpoint("deadbeef", 1000, 5000)
	dag := &BlockDAG{state: cs, orphans: make(map[string][]*Block), produceStuckGaps: make(map[string]bool)}

	oldBlock := &Block{Height: 500, Hash: "old-ancestor", SelfFetched: true}
	if !dag.isFinalityViolation(oldBlock) {
		t.Fatal("a SelfFetched block nothing has registered a produce-stuck gap for must still be a finality violation")
	}

	dag.produceStuckGaps[oldBlock.Hash] = true
	if dag.isFinalityViolation(oldBlock) {
		t.Fatal("a SelfFetched block genuinely needed to close a registered produce-stuck gap must be exempt, regardless of how far below the checkpoint it is")
	}
}

func TestIsFinalityViolation_RecentBlockNeverAViolation(t *testing.T) {
	cs := newTestState()
	cs.SetFinalizedCheckpoint("deadbeef", 1000, 5000)
	dag := &BlockDAG{state: cs}

	recentBlock := &Block{Height: 990, Hash: "recent"} // within finalityHeightSlack of 1000
	if dag.isFinalityViolation(recentBlock) {
		t.Fatal("a block within finalityHeightSlack of the checkpoint must never be a violation, self-fetched or not")
	}
}
