package keeper

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// TestComputeGHOSTDAGState_IncompleteOnUnresolvableDirectParent is the
// regression guard for the 2026-07-10 architectural fix closing the
// remaining BlueScore-drift class: a live incident showed Primary and a
// freshly-caught-up secondary computing DIFFERENT BlueScore for the
// IDENTICAL, hash-matching block, because a merge-set ancestor genuinely
// wasn't yet locally resolvable (still in flight over the network) at the
// exact instant one node computed it — and that wrong value, once
// committed, was never revisited. computeGHOSTDAGState must report
// incomplete rather than silently excluding an unresolvable DIRECT parent
// from the SelectedParent comparison, and must leave every field untouched.
func TestComputeGHOSTDAGState_IncompleteOnUnresolvableDirectParent(t *testing.T) {
	dag := newGhostdagTestDAG() // dag.state == nil: any unknown hash is unresolvable

	block := &Block{
		Hash:         "child",
		Height:       1,
		ParentHashes: []string{"missing-parent"},
	}
	missing, ok := dag.computeGHOSTDAGState(block)
	if ok {
		t.Fatalf("expected incomplete result for an unresolvable direct parent, got ok=true")
	}
	if missing != "missing-parent" {
		t.Fatalf("missingAncestor = %q, want %q", missing, "missing-parent")
	}
	if block.SelectedParent != "" || block.Blues != nil || block.BlueScore != 0 {
		t.Fatalf("fields must stay untouched on failure, got SelectedParent=%q Blues=%v BlueScore=%d",
			block.SelectedParent, block.Blues, block.BlueScore)
	}
}

// TestComputeGHOSTDAGState_IncompleteOnUnresolvableMergeSetAncestor covers
// the deeper case: the DIRECT parents all resolve (so Integrity check 3 in
// AddPeerBlock would have let this block through), but the merge-set BFS
// walking back from a non-SelectedParent parent hits a hash it cannot
// resolve. This is the actual live scenario: a validator's block
// legitimately lists a concurrent sibling as a parent, but that sibling's
// OWN parent (needed to compute the merge set correctly) hasn't arrived yet.
func TestComputeGHOSTDAGState_IncompleteOnUnresolvableMergeSetAncestor(t *testing.T) {
	dag := newGhostdagTestDAG()

	sp := &Block{Hash: "sp", Height: 1, BlueScore: 5}
	sibling := &Block{Hash: "sibling", Height: 1, BlueScore: 3, ParentHashes: []string{"sibling-parent-not-present"}}
	dag.blocks["sp"] = sp
	dag.blocks["sibling"] = sibling

	block := &Block{
		Hash:         "child",
		Height:       2,
		ParentHashes: []string{"sp", "sibling"},
	}
	missing, ok := dag.computeGHOSTDAGState(block)
	if ok {
		t.Fatalf("expected incomplete result — sibling's own parent is genuinely unresolvable, got ok=true (BlueScore=%d)", block.BlueScore)
	}
	if missing != "sibling-parent-not-present" {
		t.Fatalf("missingAncestor = %q, want %q", missing, "sibling-parent-not-present")
	}
	if block.SelectedParent != "" || block.Blues != nil || block.BlueScore != 0 {
		t.Fatalf("fields must stay untouched on failure, got SelectedParent=%q Blues=%v BlueScore=%d",
			block.SelectedParent, block.Blues, block.BlueScore)
	}
}

// TestComputeGHOSTDAGState_CompleteWhenEverythingResolves is the sibling
// guard: once every ancestor the BFS needs is genuinely present, the
// computation must succeed exactly as before this fix (no false positives
// from the new completeness check).
func TestComputeGHOSTDAGState_CompleteWhenEverythingResolves(t *testing.T) {
	dag := newGhostdagTestDAG()

	sp := &Block{Hash: "sp", Height: 1, BlueScore: 5}
	sibling := &Block{Hash: "sibling", Height: 1, BlueScore: 3, ParentHashes: []string{"genesis"}}
	genesis := &Block{Hash: "genesis", Height: 0, IsGenesis: true}
	dag.blocks["sp"] = sp
	dag.blocks["sibling"] = sibling
	dag.blocks["genesis"] = genesis

	block := &Block{
		Hash:         "child",
		Height:       2,
		ParentHashes: []string{"sp", "sibling"},
	}
	missing, ok := dag.computeGHOSTDAGState(block)
	if !ok {
		t.Fatalf("expected complete result when every ancestor resolves, got missing=%q", missing)
	}
	if block.SelectedParent != "sp" {
		t.Fatalf("SelectedParent = %q, want %q (higher BlueScore)", block.SelectedParent, "sp")
	}
	if block.BlueScore != 8 { // sp.BlueScore(5) + 1 + len(blues: sibling, genesis)=2 — genesis isn't reachable from SP either, so it's also in the merge set
		t.Fatalf("BlueScore = %d, want 8", block.BlueScore)
	}
}

// TestAddPeerBlock_QueuesOrphanWhenGHOSTDAGIncomplete is the end-to-end
// regression guard: a peer block whose DIRECT parents all exist (passing
// Integrity check 3) but whose merge-set BFS hits an unresolvable DEEPER
// ancestor must NOT be attached to dag.blocks/dag.tips — it must be queued
// as an orphan on that specific missing hash, exactly like a missing direct
// parent, so it is retried once the hash arrives instead of permanently
// committing a BlueScore computed from incomplete data.
func TestAddPeerBlock_QueuesOrphanWhenGHOSTDAGIncomplete(t *testing.T) {
	dag := newOrphanTestDAG()
	dag.state = &ChainState{}
	dag.bootHeight = 0
	dag.authorizedValidators = map[string]bool{}
	dag.warnedUnknownProposers = map[string]bool{}
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}
	dag.replayedBlocks = map[string]bool{}
	dag.equivocationIndex = map[string]string{}

	genesis := &Block{Hash: "genesis", Height: 0, IsGenesis: true}
	// "sibling" is a real, present direct-parent candidate whose OWN parent
	// is NOT present — this is what the merge-set BFS cannot resolve.
	sibling := &Block{Hash: "sibling", Height: 1, ParentHashes: []string{"sibling-ancestor-not-present"}}
	dag.blocks["genesis"] = genesis
	dag.blocks["sibling"] = sibling

	// signTestBlockWithParent only supports a single parent; this test needs
	// two, so sign inline (same sequence: build, hash, sign) rather than
	// mutating ParentHashes after signing — that would desync the signed
	// hash from the block's actual content and fail hash verification
	// before ever reaching the code path this test exercises.
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	addr := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	blk := &Block{
		Height:       2,
		Timestamp:    time.Now().Unix(),
		ParentHashes: []string{"genesis", "sibling"},
		Proposer:     addr,
		Humans:       4,
		StateRoot:    "some-state-root",
	}
	blk.Hash = calculateBlockHash(blk)
	hashBytes, err := hex.DecodeString(blk.Hash)
	if err != nil {
		t.Fatalf("decode hash: %v", err)
	}
	sig, err := crypto.Sign(hashBytes, key)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	blk.Signature = hex.EncodeToString(sig)
	blk.FromSync = true // bypass authorization for this test's purpose

	if dag.AddPeerBlock(blk) {
		t.Fatal("block must not attach while a merge-set ancestor is genuinely unresolvable")
	}
	dag.orphansMu.Lock()
	waiting := dag.orphans["sibling-ancestor-not-present"]
	dag.orphansMu.Unlock()
	if len(waiting) != 1 || waiting[0].Hash != blk.Hash {
		t.Fatalf("expected block queued as orphan on the missing merge-set ancestor, got %+v", waiting)
	}
	if _, exists := dag.blocks[blk.Hash]; exists {
		t.Fatal("block must not be inserted into dag.blocks while GHOSTDAG-incomplete")
	}
}
