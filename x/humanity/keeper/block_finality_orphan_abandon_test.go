package keeper

import "testing"

// TestAddPeerBlock_FinalityViolationAbandonsWaitingOrphans is the regression
// guard for the other half of the 2026-07-10 Primary/Contabo1 fix: a block
// rejected by the finality gate is permanently unrecoverable (finalizedHeight
// only ever advances — see isFinalityViolation's own comment), exactly the
// precondition abandonOrphansWaitingFor requires, and exactly the same
// reasoning already applied to the unauthorized-proposer gate a few lines
// above it in AddPeerBlock. Without this, an orphan waiting on a
// finality-rejected hash as its missing parent stayed queued forever:
// fetchMissingAncestors kept re-fetching it from a peer that genuinely has
// it, AddPeerBlock kept rejecting it for the same reason every time,
// RecordOrphanAttempt never fired for it (only recorded when a peer does NOT
// have the hash), so queueOrphan's own TTL-based abandon check never
// re-triggered either — MissingParentHashes() (and therefore wantDeepScan)
// stayed permanently non-empty for a gap that could never close.
func TestAddPeerBlock_FinalityViolationAbandonsWaitingOrphans(t *testing.T) {
	dag := newOrphanTestDAG()
	cs := newTestState()
	cs.SetFinalizedCheckpoint("deadbeef", 1000, 5000)
	dag.state = cs

	// Height 500 is far below 1000-finalityHeightSlack(50) — a genuine,
	// permanent finality violation, not a borderline case.
	ancestor := signTestBlockWithParent(t, 500, "even-older-parent")
	dag.authorizedValidators = map[string]bool{ancestor.Proposer: true}

	waitingChild := &Block{Hash: "waiting-child", Height: 501, ParentHashes: []string{ancestor.Hash}, Proposer: "0xhonest"}
	dag.queueOrphan(ancestor.Hash, waitingChild)
	if _, tracked := dag.orphanAge(ancestor.Hash); !tracked {
		t.Fatal("setup: waitingChild must be queued waiting on ancestor.Hash before AddPeerBlock runs")
	}

	if dag.AddPeerBlock(ancestor) {
		t.Fatal("a block below the finalized checkpoint must still be rejected — this fix only changes orphan cleanup, not the finality verdict itself")
	}
	if _, tracked := dag.orphanAge(ancestor.Hash); tracked {
		t.Fatal("a finality-violating block's orphan entry must be abandoned — otherwise it stays queued forever, keeping wantDeepScan permanently true for a gap that can never close")
	}
}

// TestAddPeerBlock_FinalityViolationWithNoWaitingOrphanIsHarmless verifies
// the new abandon call is safe for the overwhelmingly common case: a
// finality-violating block that nothing happens to be waiting on (e.g. an
// ordinary stale gossip replay, not part of an active orphan chain).
// abandonOrphansWaitingFor on a hash with no queued entry must be a no-op,
// not a panic or spurious log line.
func TestAddPeerBlock_FinalityViolationWithNoWaitingOrphanIsHarmless(t *testing.T) {
	dag := newOrphanTestDAG()
	cs := newTestState()
	cs.SetFinalizedCheckpoint("deadbeef", 1000, 5000)
	dag.state = cs

	stale := signTestBlockWithParent(t, 500, "some-parent")
	dag.authorizedValidators = map[string]bool{stale.Proposer: true}

	if dag.AddPeerBlock(stale) {
		t.Fatal("a block below the finalized checkpoint must still be rejected")
	}
	if _, exists := dag.blocks[stale.Hash]; exists {
		t.Fatal("a finality-violating block must never be stored")
	}
}
