package keeper

import "testing"

// TestSeedCheckpointParentStubsLocked_SeedsMissingParent is the regression
// guard for the 2026-07-11 fix: a permanent post-resync production deadlock
// confirmed live on Contabo1 and Contabo2. A freshly-seeded trusted
// checkpoint block still carries its own real (pre-resync) ParentHashes,
// which can never resolve through ordinary sync once chain_blocks has been
// wiped — and the runtime self-heal bridge that exists for exactly this gap
// can never fire either, because it gates on !isCatchingUpLocked(), which
// stays true forever while dag.height is stuck exactly at this gap. Seeding
// the stub proactively at checkpoint-seed time sidesteps the deadlock
// entirely.
func TestSeedCheckpointParentStubsLocked_SeedsMissingParent(t *testing.T) {
	dag := newOrphanTestDAG()
	cp := &Block{Hash: "checkpoint-hash", Height: 743026, BlueScore: 3690821, Proposer: "0xhonest", ParentHashes: []string{"missing-parent-hash"}}
	dag.blocks[cp.Hash] = cp
	dag.tips = map[string]bool{cp.Hash: true}

	dag.seedCheckpointParentStubsLocked(cp)

	stub, ok := dag.blocks["missing-parent-hash"]
	if !ok {
		t.Fatal("expected a synthetic-checkpoint stub to be seeded for the checkpoint's own missing parent")
	}
	if stub.Height != cp.Height-1 {
		t.Errorf("stub height = %d, want %d (one below the checkpoint)", stub.Height, cp.Height-1)
	}
	if stub.Proposer != "synthetic-checkpoint" {
		t.Errorf("stub proposer = %q, want %q", stub.Proposer, "synthetic-checkpoint")
	}
	if stub.BlueScore != cp.BlueScore-1 {
		t.Errorf("stub BlueScore = %d, want %d (checkpoint's own BlueScore - 1, via safeStubBlueScoreLocked)", stub.BlueScore, cp.BlueScore-1)
	}
	if len(stub.ParentHashes) != 0 {
		t.Errorf("stub must have no parents of its own (chain terminates here), got %v", stub.ParentHashes)
	}
}

// TestSeedCheckpointParentStubsLocked_SkipsAlreadyPresentParent verifies a
// parent that's already in dag.blocks (e.g. a genuine block resolved by
// some other path, or the checkpoint's own sibling machinery) is never
// clobbered with a synthetic stub.
func TestSeedCheckpointParentStubsLocked_SkipsAlreadyPresentParent(t *testing.T) {
	dag := newOrphanTestDAG()
	cp := &Block{Hash: "checkpoint-hash", Height: 100, BlueScore: 500, Proposer: "0xhonest", ParentHashes: []string{"real-parent-hash"}}
	realParent := &Block{Hash: "real-parent-hash", Height: 99, BlueScore: 499, Proposer: "0xhonest"}
	dag.blocks[cp.Hash] = cp
	dag.blocks[realParent.Hash] = realParent
	dag.tips = map[string]bool{cp.Hash: true}

	dag.seedCheckpointParentStubsLocked(cp)

	if got := dag.blocks["real-parent-hash"]; got != realParent {
		t.Fatal("an already-present parent block must not be replaced with a synthetic stub")
	}
}

// TestSeedCheckpointParentStubsLocked_NoParentIsNoOp verifies a checkpoint
// with no recorded parent (e.g. genesis itself) does nothing — there is
// nothing to seed a stub for.
func TestSeedCheckpointParentStubsLocked_NoParentIsNoOp(t *testing.T) {
	dag := newOrphanTestDAG()
	cp := &Block{Hash: "checkpoint-hash", Height: 0, BlueScore: 0, Proposer: "0xhonest"}
	dag.blocks[cp.Hash] = cp
	dag.tips = map[string]bool{cp.Hash: true}

	dag.seedCheckpointParentStubsLocked(cp)

	if len(dag.blocks) != 1 {
		t.Fatalf("expected no new blocks to be seeded, dag.blocks has %d entries", len(dag.blocks))
	}
}
