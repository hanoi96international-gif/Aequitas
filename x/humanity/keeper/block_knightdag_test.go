package keeper

import (
	"fmt"
	"testing"
)

// TestKnightdagConcCache_SymmetricAndConsistent pins the cache's contract:
// concurrent(x,y) must equal concurrent(y,x), a block is never concurrent
// with itself, ancestor/descendant pairs are not concurrent, and true
// siblings are. The whole KnightDAG inference rests on this relation being
// a pure, symmetric function of DAG structure.
func TestKnightdagConcCache_SymmetricAndConsistent(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.blocks["genesis"] = &Block{Hash: "genesis", Height: 0, IsGenesis: true}
	dag.blocks["a"] = &Block{Hash: "a", Height: 1, ParentHashes: []string{"genesis"}}
	dag.blocks["b"] = &Block{Hash: "b", Height: 2, ParentHashes: []string{"a"}}
	dag.blocks["c"] = &Block{Hash: "c", Height: 1, ParentHashes: []string{"genesis"}}

	cc := dag.newKnightdagConcCache(nil)
	if !cc.concurrent("a", "c") || !cc.concurrent("c", "a") {
		t.Fatal("true siblings a/c must be concurrent in both argument orders")
	}
	if cc.concurrent("a", "b") || cc.concurrent("b", "a") {
		t.Fatal("ancestor/descendant a/b must not be concurrent")
	}
	if cc.concurrent("a", "a") {
		t.Fatal("a block is never concurrent with itself")
	}
	// Cached answer must be stable across repeated queries.
	if !cc.concurrent("a", "c") {
		t.Fatal("cached concurrent(a,c) flipped on repeat query")
	}
}

// TestKnightDAG_EmptyMergeSet verifies the trivial-chain fast path: with
// nothing to classify, K_eff is 0 and the blue set empty — no ceiling
// fallback for a merge set that never existed.
func TestKnightDAG_EmptyMergeSet(t *testing.T) {
	dag := newGhostdagTestDAG()
	cc := dag.newKnightdagConcCache(nil)
	kEff, blues := dag.knightdagInferK(nil, cc)
	if kEff != 0 || blues != nil {
		t.Fatalf("knightdagInferK(empty) = (%d, %v), want (0, nil)", kEff, blues)
	}
}

// buildSiblingBurstDAG creates genesis, n concurrent siblings s0..s(n-1)
// (each with genesis as sole parent, full GHOSTDAG state computed), and
// returns a child block referencing all n siblings as parents, NOT yet
// computed — so tests can inspect classification of the child's merge set
// (the n-1 non-SelectedParent siblings, all mutually concurrent).
func buildSiblingBurstDAG(t *testing.T, dag *BlockDAG, n int) *Block {
	t.Helper()
	dag.blocks["genesis"] = &Block{Hash: "genesis", Height: 0, IsGenesis: true}
	parents := make([]string, 0, n)
	for i := 0; i < n; i++ {
		h := fmt.Sprintf("s%02d", i)
		b := &Block{Hash: h, Height: 1, ParentHashes: []string{"genesis"}}
		dag.blocks[h] = b
		if _, ok := dag.computeGHOSTDAGState(b); !ok {
			t.Fatalf("sibling %s did not compute", h)
		}
		parents = append(parents, h)
	}
	return &Block{Hash: "child", Height: 2, ParentHashes: parents}
}

// TestKnightDAG_AdaptiveKReducesBlues is the core behavioral test: with 4
// mutually concurrent siblings in the merge set (5 siblings total, one
// becomes SelectedParent... here: child of 5 siblings → merge set = 4),
// classic GHOSTDAG at the epoch ceiling K=18 would classify ALL 4 blue.
// KnightDAG must instead find the smallest majority k: at k=0 one blue
// (2·1>4 fails), k=1 two blues (2·2>4 fails), k=2 three blues (2·3>4
// holds) → K_eff=2, three blue / one red, BlueScore = 1+1+3 = 5 instead of
// the fixed-K value 6.
func TestKnightDAG_AdaptiveKReducesBlues(t *testing.T) {
	dag := newGhostdagTestDAG()
	child := buildSiblingBurstDAG(t, dag, 5)
	dag.blocks[child.Hash] = child
	if _, ok := dag.computeGHOSTDAGState(child); !ok {
		t.Fatal("child did not compute")
	}
	if child.SelectedParent != "s00" {
		t.Fatalf("SelectedParent = %q, want s00 (first of tied-score parents)", child.SelectedParent)
	}
	if len(child.Blues) != 3 {
		t.Fatalf("len(Blues) = %d, want 3 (K_eff=2 majority classification, not all-blue fixed-K)", len(child.Blues))
	}
	if child.BlueScore != 5 {
		t.Fatalf("BlueScore = %d, want 5 (SP score 1 + 1 + 3 blues)", child.BlueScore)
	}
}

// TestKnightDAG_SmallMergeSetEquivalentToClassic pins the no-divergence
// case: a 2-sibling merge (merge set of one block, no concurrency visible)
// must produce the identical Blues/BlueScore as classic fixed-K GHOSTDAG,
// with K_eff = 0.
func TestKnightDAG_SmallMergeSetEquivalentToClassic(t *testing.T) {
	dag := newGhostdagTestDAG()
	child := buildSiblingBurstDAG(t, dag, 2)
	dag.blocks[child.Hash] = child
	if _, ok := dag.computeGHOSTDAGState(child); !ok {
		t.Fatal("child did not compute")
	}
	if len(child.Blues) != 1 || child.BlueScore != 3 {
		t.Fatalf("Blues=%v BlueScore=%d, want 1 blue and score 3 — must match classic GHOSTDAG exactly", child.Blues, child.BlueScore)
	}
	// The single merge-set block has an empty blue-anticone, so the
	// smallest majority k is 0.
	mergeSet, missing := dag.ghostdagMergeSet(child, child.SelectedParent, nil)
	if missing != "" {
		t.Fatalf("unexpected missing ancestor %q", missing)
	}
	sorted := ghostdagTopoSort(mergeSet, dag.blocks)
	kEff, _ := dag.knightdagInferK(sorted, dag.newKnightdagConcCache(nil))
	if kEff != 0 {
		t.Fatalf("kEff = %d, want 0 for a concurrency-free merge set", kEff)
	}
}

// TestKnightDAG_FallbackCeilingMatchesClassic is the safety guarantee: when
// no k below the epoch ceiling reaches a merge-set majority (here: 39
// mutually concurrent merge-set blocks, majority needs 2·(k+1) > 39 → k ≥
// 19 > ceiling 18), classification must fall back to the ceiling and
// produce bit-for-bit the classic fixed-K result — first K+1 = 19 blocks
// blue, the rest red.
func TestKnightDAG_FallbackCeilingMatchesClassic(t *testing.T) {
	dag := newGhostdagTestDAG()
	child := buildSiblingBurstDAG(t, dag, 40)
	dag.blocks[child.Hash] = child
	if _, ok := dag.computeGHOSTDAGState(child); !ok {
		t.Fatal("child did not compute")
	}
	wantBlues := dag.k() + 1 // classic greedy: m-th sibling blue iff m-1 ≤ K
	if len(child.Blues) != wantBlues {
		t.Fatalf("len(Blues) = %d, want %d (ceiling-K fallback must equal classic GHOSTDAG)", len(child.Blues), wantBlues)
	}
	if child.BlueScore != int64(1+1+wantBlues) {
		t.Fatalf("BlueScore = %d, want %d", child.BlueScore, 1+1+wantBlues)
	}
	mergeSet, missing := dag.ghostdagMergeSet(child, child.SelectedParent, nil)
	if missing != "" {
		t.Fatalf("unexpected missing ancestor %q", missing)
	}
	sorted := ghostdagTopoSort(mergeSet, dag.blocks)
	kEff, _ := dag.knightdagInferK(sorted, dag.newKnightdagConcCache(nil))
	if kEff != dag.k() {
		t.Fatalf("kEff = %d, want the ceiling %d when no smaller k reaches a majority", kEff, dag.k())
	}
}

// TestKnightDAG_MinimalityInvariant is the refactor guard for the linear
// scan (e.g. against a future binary-search "optimization" that assumes
// monotonicity the greedy k-cluster rule does not formally have): the
// returned kEff must be MINIMAL — every smaller k must fail the majority —
// and the returned blue set must be exactly knightdagClassify at kEff.
func TestKnightDAG_MinimalityInvariant(t *testing.T) {
	for _, siblings := range []int{3, 5, 9, 14, 25} {
		dag := newGhostdagTestDAG()
		child := buildSiblingBurstDAG(t, dag, siblings)
		dag.blocks[child.Hash] = child
		if _, ok := dag.computeGHOSTDAGState(child); !ok {
			t.Fatalf("siblings=%d: child did not compute", siblings)
		}
		mergeSet, missing := dag.ghostdagMergeSet(child, child.SelectedParent, nil)
		if missing != "" {
			t.Fatalf("siblings=%d: unexpected missing ancestor %q", siblings, missing)
		}
		sorted := ghostdagTopoSort(mergeSet, dag.blocks)
		cc := dag.newKnightdagConcCache(nil)
		kEff, blues := dag.knightdagInferK(sorted, cc)
		if kEff > dag.k() {
			t.Fatalf("siblings=%d: kEff %d exceeds ceiling %d", siblings, kEff, dag.k())
		}
		if got := knightdagClassify(sorted, kEff, cc); len(got) != len(blues) {
			t.Fatalf("siblings=%d: returned blues (%d) diverge from classify at kEff (%d)", siblings, len(blues), len(got))
		}
		for k := 0; k < kEff; k++ {
			if b := knightdagClassify(sorted, k, cc); 2*len(b) > len(sorted) {
				t.Fatalf("siblings=%d: k=%d already reaches majority — kEff=%d is not minimal", siblings, k, kEff)
			}
		}
		if kEff < dag.k() && 2*len(blues) <= len(sorted) {
			t.Fatalf("siblings=%d: kEff=%d below ceiling but blues %d/%d is no majority", siblings, kEff, len(blues), len(sorted))
		}
	}
}
