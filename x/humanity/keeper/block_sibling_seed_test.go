package keeper

import "testing"

// TestFilterAndVerifySiblings_KeepsValidSiblingsAtHeight is the regression
// guard for the 2026-07-04 true-root-cause finding: SeedTrustedCheckpoint
// only ever seeded the SINGLE canonical block at the checkpoint height, so
// a later block that legitimately referenced a DIFFERENT sibling at that
// same height as a GHOSTDAG merge parent orphaned forever — confirmed live
// via exact hash reproduction (checkpoint seeded 84cf31da1a85a5cb at height
// 182624, but block #182625 needed sibling 03a902c59bac8227, never fetched).
// This proves the fix's core filtering keeps every distinct, validly signed
// block at exactly the requested height.
func TestFilterAndVerifySiblings_KeepsValidSiblingsAtHeight(t *testing.T) {
	a := signTestBlock(t, 182624)
	b := signTestBlock(t, 182624)
	got := filterAndVerifySiblings([]*Block{a, b}, 182624)
	if len(got) != 2 {
		t.Fatalf("filterAndVerifySiblings() returned %d blocks, want 2 distinct valid siblings at the requested height", len(got))
	}
}

// TestFilterAndVerifySiblings_SkipsWrongHeight guards against a peer
// response that (legitimately, per /api/blocks?min_height= semantics, or
// maliciously) includes blocks above/below the requested height — only
// exact matches may be treated as checkpoint-height siblings.
func TestFilterAndVerifySiblings_SkipsWrongHeight(t *testing.T) {
	wrongHeight := signTestBlock(t, 182623)
	got := filterAndVerifySiblings([]*Block{wrongHeight}, 182624)
	if len(got) != 0 {
		t.Fatalf("filterAndVerifySiblings() = %d blocks, want 0 for a block at the wrong height", len(got))
	}
}

// TestFilterAndVerifySiblings_SkipsGenesis guards against the genesis block
// (which never carries a normal signature) being misidentified as a
// checkpoint-height sibling if a height ever coincidentally matched.
func TestFilterAndVerifySiblings_SkipsGenesis(t *testing.T) {
	genesis := signTestBlock(t, 0)
	genesis.IsGenesis = true
	got := filterAndVerifySiblings([]*Block{genesis}, 0)
	if len(got) != 0 {
		t.Fatalf("filterAndVerifySiblings() = %d blocks, want 0 for the genesis block", len(got))
	}
}

// TestFilterAndVerifySiblings_SkipsTamperedSibling guards the same trust
// boundary as TestVerifyFetchedBlock_TamperedField: a sibling from a
// misbehaving or compromised peer that fails verifyFetchedBlock must be
// dropped silently, not returned as if it were trustworthy — but must not
// prevent OTHER, valid siblings in the same response from being kept.
func TestFilterAndVerifySiblings_SkipsTamperedSibling(t *testing.T) {
	good := signTestBlock(t, 182624)
	tampered := signTestBlock(t, 182624)
	tampered.Humans = 999999 // tamper after signing, hash no longer matches
	got := filterAndVerifySiblings([]*Block{good, tampered}, 182624)
	if len(got) != 1 {
		t.Fatalf("filterAndVerifySiblings() returned %d blocks, want exactly the 1 untampered sibling", len(got))
	}
	if got[0].Hash != good.Hash {
		t.Fatalf("filterAndVerifySiblings() kept the wrong block: got hash %s, want %s", got[0].Hash, good.Hash)
	}
}

// TestMergeSiblingsIntoBlocks_AddsNewSiblingAtCheckpointHeight is the
// regression guard for RefreshBootHeightAfterSnapshotImport's checkpoint-
// seeding branch: a sibling at exactly checkpointHeight, distinct from the
// canonical block already seeded, must land in dag.blocks (making it
// resolvable as a merge parent for later blocks) — but critically NOT in
// dag.tips, which must stay pinned to the single canonical block so the
// sequential resync still has one unambiguous starting frontier.
func TestMergeSiblingsIntoBlocks_AddsNewSiblingAtCheckpointHeight(t *testing.T) {
	canonical := &Block{Hash: "canonical-hash", Height: 182624}
	sibling := &Block{Hash: "sibling-hash", Height: 182624}
	blocks := map[string]*Block{canonical.Hash: canonical}

	added := mergeSiblingsIntoBlocks(blocks, []*Block{sibling}, 182624, canonical.Hash)

	if added != 1 {
		t.Fatalf("mergeSiblingsIntoBlocks() returned added=%d, want 1", added)
	}
	if _, ok := blocks[sibling.Hash]; !ok {
		t.Fatal("mergeSiblingsIntoBlocks() did not add the sibling to dag.blocks")
	}
}

// TestMergeSiblingsIntoBlocks_SkipsCanonicalDuplicate guards against
// double-counting: LoadBlocksSinceFromDB's own result set includes the
// canonical block itself (it was just saved to chain_blocks moments
// earlier by the same resync), which must not be re-added or counted.
func TestMergeSiblingsIntoBlocks_SkipsCanonicalDuplicate(t *testing.T) {
	canonical := &Block{Hash: "canonical-hash", Height: 182624}
	blocks := map[string]*Block{canonical.Hash: canonical}

	added := mergeSiblingsIntoBlocks(blocks, []*Block{canonical}, 182624, canonical.Hash)

	if added != 0 {
		t.Fatalf("mergeSiblingsIntoBlocks() returned added=%d, want 0 for the canonical block appearing in its own sibling list", added)
	}
	if len(blocks) != 1 {
		t.Fatalf("mergeSiblingsIntoBlocks() left %d entries in blocks, want 1 (no duplicate)", len(blocks))
	}
}

// TestMergeSiblingsIntoBlocks_SkipsWrongHeight guards against
// LoadBlocksSinceFromDB's minHeight-1 range query (used so the >= boundary
// includes checkpointHeight itself) leaking an off-by-one neighbor height
// into dag.blocks under the wrong assumption.
func TestMergeSiblingsIntoBlocks_SkipsWrongHeight(t *testing.T) {
	canonical := &Block{Hash: "canonical-hash", Height: 182624}
	neighbor := &Block{Hash: "neighbor-hash", Height: 182623}
	blocks := map[string]*Block{canonical.Hash: canonical}

	added := mergeSiblingsIntoBlocks(blocks, []*Block{neighbor}, 182624, canonical.Hash)

	if added != 0 {
		t.Fatalf("mergeSiblingsIntoBlocks() returned added=%d, want 0 for a neighboring-height block", added)
	}
	if _, ok := blocks[neighbor.Hash]; ok {
		t.Fatal("mergeSiblingsIntoBlocks() must not add a block at the wrong height")
	}
}

// TestMergeSiblingsIntoBlocks_DoesNotOverwriteExisting guards against a
// sibling entry clobbering a block already present under the same hash —
// should never happen in practice (hashes are content-addressed) but the
// exists-check is the safety net if it ever did.
func TestMergeSiblingsIntoBlocks_DoesNotOverwriteExisting(t *testing.T) {
	original := &Block{Hash: "sibling-hash", Height: 182624, Humans: 4}
	replacement := &Block{Hash: "sibling-hash", Height: 182624, Humans: 999}
	blocks := map[string]*Block{"canonical-hash": {Hash: "canonical-hash", Height: 182624}, original.Hash: original}

	added := mergeSiblingsIntoBlocks(blocks, []*Block{replacement}, 182624, "canonical-hash")

	if added != 0 {
		t.Fatalf("mergeSiblingsIntoBlocks() returned added=%d, want 0 for an already-present hash", added)
	}
	if blocks[original.Hash].Humans != 4 {
		t.Fatal("mergeSiblingsIntoBlocks() overwrote an already-present block entry")
	}
}
