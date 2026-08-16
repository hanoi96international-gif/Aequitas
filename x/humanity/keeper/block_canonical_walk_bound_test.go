package keeper

import (
	"fmt"
	"testing"
	"time"
)

// buildLinearChain seeds dag with a single SelectedParent chain from height 0
// up to tipHeight and marks the top block as the only tip. Every block is a
// real (non-stub) block with a strictly increasing BlueScore, so
// canonicalBlockAtHeightLocked's tip selection and walk both behave exactly
// as they do on a healthy production chain.
func buildLinearChain(dag *BlockDAG, tipHeight int64) {
	dag.tips = make(map[string]bool)
	prev := ""
	for h := int64(0); h <= tipHeight; h++ {
		hash := fmt.Sprintf("blk-%d", h)
		b := &Block{
			Hash:           hash,
			Height:         h,
			Proposer:       "0xvalidator",
			SelectedParent: prev,
			BlueScore:      h,
		}
		if prev != "" {
			b.ParentHashes = []string{prev}
		} else {
			b.IsGenesis = true
		}
		dag.blocks[hash] = b
		prev = hash
	}
	dag.tips[prev] = true
}

// TestCanonicalBlockAtHeight_DeepLookupIsBounded is the regression guard for
// the P0 unauthenticated remote-DoS found on 2026-08-14 (reproduced live: a
// single GET /api/block?height=1520000 against Contabo1 at height ~3.73M
// stopped the node answering HTTP entirely, while it kept accepting TCP).
//
// canonicalBlockAtHeightLocked runs a LINEAR SelectedParent walk while
// holding the global dag.mu WRITE lock, and the DB-lookup budget it passed
// to ghostdagBlockLookup stopped gating anything on 2026-07-10 (that
// function's "budget is now advisory only" change, correct for its merge-set
// callers, which self-limit via maxMergeVisits — this walk has no such cap).
// A deep height therefore walked millions of hops, most of them synchronous
// Postgres round trips, freezing block production and every API read behind
// the lock, and caching every fetched block into dag.blocks on the way (OOM).
//
// The bound must reject deep lookups WITHOUT walking, handing GetBlockByHeight
// its existing LoadBlockFromDBByHeight fallback — one indexed query using the
// same canonical selection rule.
func TestCanonicalBlockAtHeight_DeepLookupIsBounded(t *testing.T) {
	dag := newGhostdagTestDAG()
	tipHeight := int64(maxCanonicalWalkHops + 500)
	buildLinearChain(dag, tipHeight)

	// Height 0 sits maxCanonicalWalkHops+500 below the tip — beyond the
	// bound, so this must refuse to walk and return nil for the DB fallback.
	// dag.state is nil in this harness, so a walk that DID run would still
	// terminate; the point of the timing assertion is that it must not walk
	// the chain at all. A full walk of this chain is orders of magnitude
	// slower than the O(1) precheck even fully in memory.
	start := time.Now()
	got := dag.canonicalBlockAtHeightLocked(0)
	elapsed := time.Since(start)

	if got != nil {
		t.Fatalf("canonicalBlockAtHeightLocked(0) = %s at height %d, want nil — a lookup %d hops below the tip must fall through to the DB path, not walk",
			got.Hash, got.Height, tipHeight)
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("deep lookup took %s — expected an O(1) rejection via the distance precheck, not a full %d-hop walk", elapsed, tipHeight)
	}
}

// TestCanonicalBlockAtHeight_NearTipStillExact verifies the bound did not
// break the case that actually matters for correctness: near-tip lookups —
// the explorer's /api/blocks/canonical window and autoheal's finalized-
// checkpoint hash comparison — must still take the exact SelectedParent walk
// and return the precise canonical block, not fall back to the DB heuristic.
func TestCanonicalBlockAtHeight_NearTipStillExact(t *testing.T) {
	dag := newGhostdagTestDAG()
	tipHeight := int64(maxCanonicalWalkHops + 500)
	buildLinearChain(dag, tipHeight)

	for _, depth := range []int64{0, 1, 200, maxCanonicalWalkHops - 1} {
		target := tipHeight - depth
		got := dag.canonicalBlockAtHeightLocked(target)
		if got == nil {
			t.Fatalf("canonicalBlockAtHeightLocked(%d) = nil at depth %d below the tip, want the real block — the bound must not reject lookups within maxCanonicalWalkHops (%d)",
				target, depth, maxCanonicalWalkHops)
		}
		if got.Height != target {
			t.Fatalf("canonicalBlockAtHeightLocked(%d) returned height %d, want %d", target, got.Height, target)
		}
		if want := fmt.Sprintf("blk-%d", target); got.Hash != want {
			t.Fatalf("canonicalBlockAtHeightLocked(%d) = %s, want %s", target, got.Hash, want)
		}
	}
}

// TestCanonicalBlockAtHeight_CycleTerminates covers the reason the walk keeps
// an explicit hop counter in addition to the distance precheck. The precheck
// alone relies on every SelectedParent hop strictly decreasing Height — true
// for well-formed data, but this walk reads whatever is in dag.blocks, and a
// corrupt/adversarial SelectedParent link whose parent does NOT have a lower
// height turns the loop into an infinite one that never terminates while
// holding the global write lock. Two blocks at the SAME height pointing at
// each other reproduce exactly that; the hop counter must stop it.
func TestCanonicalBlockAtHeight_CycleTerminates(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.tips = make(map[string]bool)

	// Both at height 100, each naming the other as SelectedParent. The
	// requested height (50) is only 50 below the tip, so the distance
	// precheck deliberately does NOT fire here — only the hop counter can
	// break this.
	a := &Block{Hash: "cyc-a", Height: 100, Proposer: "0xa", SelectedParent: "cyc-b", BlueScore: 100}
	b := &Block{Hash: "cyc-b", Height: 100, Proposer: "0xb", SelectedParent: "cyc-a", BlueScore: 99}
	dag.blocks[a.Hash] = a
	dag.blocks[b.Hash] = b
	dag.tips[a.Hash] = true

	done := make(chan *Block, 1)
	go func() { done <- dag.canonicalBlockAtHeightLocked(50) }()

	select {
	case got := <-done:
		if got != nil {
			t.Fatalf("canonicalBlockAtHeightLocked(50) = %s, want nil — a cyclic SelectedParent chain cannot resolve to a real block", got.Hash)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canonicalBlockAtHeightLocked did not terminate on a cyclic SelectedParent chain — the hop counter is not bounding the walk")
	}
}
