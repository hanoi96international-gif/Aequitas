package keeper

import (
	"fmt"
	"testing"
)

// The indexed (bitset) classification path added 2026-07-25 exists purely to
// remove the O(K·n²) string-map bookkeeping around KnightDAG's per-block
// classification — see knightdagConcCache.prepare's comment. It must be an
// OPTIMIZATION ONLY: for every merge set, every k, the blue set it produces
// has to be byte-for-byte what the original map-based loop produced.
//
// These tests pin exactly that, by running both paths over the same DAG and
// comparing. classifyViaMap forces the map path by handing knightdagClassify
// a cache that was never prepare()d, which is the same code the pre-2026-07-25
// build ran.

func classifyViaMap(t *testing.T, dag *BlockDAG, sorted []string, k int) []string {
	t.Helper()
	cc := dag.newKnightdagConcCache(nil) // not prepared → map path
	blues, missing := knightdagClassify(sorted, k, cc)
	if missing != "" {
		t.Fatalf("map path reported missing ancestor %q", missing)
	}
	return blues
}

func classifyViaBitset(t *testing.T, dag *BlockDAG, sorted []string, k int) []string {
	t.Helper()
	cc := dag.newKnightdagConcCache(nil)
	cc.prepare(sorted)
	if cc.n != len(sorted) {
		t.Fatalf("prepare did not index the merge set: n=%d, want %d", cc.n, len(sorted))
	}
	blues, missing := knightdagClassify(sorted, k, cc)
	if missing != "" {
		t.Fatalf("bitset path reported missing ancestor %q", missing)
	}
	return blues
}

func sameOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestKnightdagBitset_MatchesMapPathForEveryK is the central equivalence
// proof: over a merge set of mutually concurrent siblings, both paths must
// agree for EVERY k from 0 up to and past the epoch ceiling — including the
// small-k cases where knightdagClassify's antiCnt>k early break fires most
// often, which is exactly where an implementation could silently diverge by
// querying pairs in a different order.
func TestKnightdagBitset_MatchesMapPathForEveryK(t *testing.T) {
	for _, siblings := range []int{2, 3, 5, 9, 16} {
		dag := newGhostdagTestDAG()
		child := buildSiblingBurstDAG(t, dag, siblings)
		// The merge set a real classification would see: every parent except
		// the SelectedParent. buildSiblingBurstDAG sorts parents s00..sNN, and
		// s00 wins the tie-break, so drop it.
		sorted := append([]string{}, child.ParentHashes[1:]...)

		for k := 0; k <= ghostdagKBase+2; k++ {
			viaMap := classifyViaMap(t, dag, sorted, k)
			viaBits := classifyViaBitset(t, dag, sorted, k)
			if !sameOrder(viaMap, viaBits) {
				t.Fatalf("siblings=%d k=%d: bitset path diverged from map path\n  map:    %v\n  bitset: %v",
					siblings, k, viaMap, viaBits)
			}
		}
	}
}

// A chain (every block an ancestor of the next) is the opposite structural
// extreme from the sibling burst: nothing is concurrent, so no candidate is
// ever rejected and the early break never fires. Both paths must still agree.
func TestKnightdagBitset_MatchesMapPathOnChain(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.blocks["genesis"] = &Block{Hash: "genesis", Height: 0, IsGenesis: true}
	prev := "genesis"
	sorted := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		h := fmt.Sprintf("c%02d", i)
		b := &Block{Hash: h, Height: int64(i + 1), ParentHashes: []string{prev}}
		dag.blocks[h] = b
		if _, ok := dag.computeGHOSTDAGState(b); !ok {
			t.Fatalf("chain block %s did not compute", h)
		}
		sorted = append(sorted, h)
		prev = h
	}
	for k := 0; k <= 4; k++ {
		if viaMap, viaBits := classifyViaMap(t, dag, sorted, k), classifyViaBitset(t, dag, sorted, k); !sameOrder(viaMap, viaBits) {
			t.Fatalf("chain k=%d: map %v != bitset %v", k, viaMap, viaBits)
		}
	}
}

// The inferred K_eff — the single number KnightDAG adds on top of GHOSTDAG,
// and the one that feeds BlueScore and therefore consensus — must be
// identical on both paths. A divergence here would be a network-wide fork,
// so it gets its own explicit assertion rather than riding on the
// classification comparison above.
func TestKnightdagBitset_InferKIdenticalToMapPath(t *testing.T) {
	withKnightdagActive(t)
	for _, siblings := range []int{3, 5, 9, 16} {
		dag := newGhostdagTestDAG()
		child := buildSiblingBurstDAG(t, dag, siblings)
		sorted := append([]string{}, child.ParentHashes[1:]...)

		ccMap := dag.newKnightdagConcCache(nil) // map path
		kMap, bluesMap, missMap := dag.knightdagInferK(sorted, ccMap)

		ccBits := dag.newKnightdagConcCache(nil)
		ccBits.prepare(sorted)
		kBits, bluesBits, missBits := dag.knightdagInferK(sorted, ccBits)

		if missMap != "" || missBits != "" {
			t.Fatalf("siblings=%d: unexpected missing ancestor (map %q / bitset %q)", siblings, missMap, missBits)
		}
		if kMap != kBits {
			t.Fatalf("siblings=%d: K_eff diverged — map %d, bitset %d. This would fork the network.",
				siblings, kMap, kBits)
		}
		if !sameOrder(bluesMap, bluesBits) {
			t.Fatalf("siblings=%d: blue set diverged at K_eff=%d\n  map:    %v\n  bitset: %v",
				siblings, kMap, bluesMap, bluesBits)
		}
	}
}

// prepare() must tolerate the degenerate inputs the classification path can
// legitimately hand it, and must leave the cache on the map path when it
// cannot index (so knightdagClassify's guard picks the correct branch).
func TestKnightdagBitset_PrepareDegenerateInputs(t *testing.T) {
	dag := newGhostdagTestDAG()

	cc := dag.newKnightdagConcCache(nil)
	cc.prepare(nil)
	if cc.n != 0 {
		t.Fatalf("prepare(nil) must leave the cache unindexed, got n=%d", cc.n)
	}
	blues, missing := knightdagClassify(nil, 3, cc)
	if missing != "" || len(blues) != 0 {
		t.Fatalf("classify over an empty merge set = (%v, %q), want (empty, \"\")", blues, missing)
	}

	dag.blocks["genesis"] = &Block{Hash: "genesis", Height: 0, IsGenesis: true}
	dag.blocks["solo"] = &Block{Hash: "solo", Height: 1, ParentHashes: []string{"genesis"}}
	single := []string{"solo"}
	cc2 := dag.newKnightdagConcCache(nil)
	cc2.prepare(single)
	got, miss := knightdagClassify(single, 0, cc2)
	if miss != "" || len(got) != 1 || got[0] != "solo" {
		t.Fatalf("single-member merge set = (%v, %q), want ([solo], \"\")", got, miss)
	}
}
