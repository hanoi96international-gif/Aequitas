package keeper

import "testing"

// Regression tests for the FIX (audit 2026-07-21) described on
// ghostdagIsAncestor: a hash genuinely unresolvable right now (absent from
// both dag.blocks and the DB — simulated here by simply never adding it to
// dag.blocks, with dag.state left nil so there is no DB fallback either)
// must be reported to the caller as "missing", not silently treated as a
// dead end that coerces the answer to false. See that function's own FIX
// comment for the two real production forks (2026-07-04, 2026-07-10) this
// mirrors the fix for, in the sibling function ghostdagMergeSet.

// TestGhostdagIsAncestor_MissingAncestorPropagates pins the fix at its
// lowest level: walking from "mid" back through its only parent hits
// "phantom", which is nowhere to be found — the walk must report that hash
// instead of concluding "not an ancestor".
func TestGhostdagIsAncestor_MissingAncestorPropagates(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.blocks["genesis"] = &Block{Hash: "genesis", Height: 0, IsGenesis: true}
	dag.blocks["mid"] = &Block{Hash: "mid", Height: 1, ParentHashes: []string{"phantom"}}

	isAnc, missing := dag.ghostdagIsAncestor("genesis", "mid", nil)
	if missing != "phantom" {
		t.Fatalf("missing = %q, want %q — an unresolvable intermediate hash must be reported, not silently treated as a dead end", missing, "phantom")
	}
	if isAnc {
		t.Fatal("isAncestor must not be reported true off an incomplete walk")
	}
}

// TestGhostdagIsAncestor_ResolvedAncestryStillWorks is the non-regression
// half: an ordinary, fully-resolvable ancestor chain must still answer
// correctly and report no missing hash — the fix must not turn every
// query into a false "missing".
func TestGhostdagIsAncestor_ResolvedAncestryStillWorks(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.blocks["genesis"] = &Block{Hash: "genesis", Height: 0, IsGenesis: true}
	dag.blocks["mid"] = &Block{Hash: "mid", Height: 1, ParentHashes: []string{"genesis"}}
	dag.blocks["tip"] = &Block{Hash: "tip", Height: 2, ParentHashes: []string{"mid"}}

	isAnc, missing := dag.ghostdagIsAncestor("genesis", "tip", nil)
	if missing != "" {
		t.Fatalf("unexpected missing ancestor %q for a fully-resolvable chain", missing)
	}
	if !isAnc {
		t.Fatal("genesis must be reported as an ancestor of tip")
	}

	isAnc, missing = dag.ghostdagIsAncestor("tip", "genesis", nil)
	if missing != "" {
		t.Fatalf("unexpected missing ancestor %q", missing)
	}
	if isAnc {
		t.Fatal("tip must not be reported as an ancestor of genesis")
	}
}

// TestKnightdagConcCache_MissingAncestorNotCached ensures concurrent()
// propagates the missing hash up (instead of coercing it into a concurrent/
// not-concurrent verdict) and — critically — never caches that non-answer.
// A cached wrong verdict from a block that was still in flight would never
// self-correct once the block actually arrived; see concurrent()'s own
// comment for why this matters.
func TestKnightdagConcCache_MissingAncestorNotCached(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.blocks["genesis"] = &Block{Hash: "genesis", Height: 0, IsGenesis: true}
	dag.blocks["mid"] = &Block{Hash: "mid", Height: 1, ParentHashes: []string{"phantom"}}

	cc := dag.newKnightdagConcCache(nil)
	_, missing := cc.concurrent("genesis", "mid")
	if missing == "" {
		t.Fatal("expected the missing ancestor to propagate through concurrent()")
	}
	key := [2]string{"genesis", "mid"}
	if _, cached := cc.res[key]; cached {
		t.Fatal("a missing-ancestor result must not be cached — it is not a real answer")
	}
}

// TestComputeGHOSTDAGState_FailureLeavesBlockUntouched pins the documented
// contract (computeGHOSTDAGState's own header comment): on ok==false, the
// block's SelectedParent/Blues/BlueScore must stay exactly as they were,
// not partially computed. This guards the SelectedParent-assignment timing
// change made alongside the ghostdagIsAncestor fix (it now happens only
// after classification fully succeeds, since classification itself can now
// also fail on a genuinely-unresolvable ancestor).
func TestComputeGHOSTDAGState_FailureLeavesBlockUntouched(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.blocks["genesis"] = &Block{Hash: "genesis", Height: 0, IsGenesis: true}
	dag.blocks["s0"] = &Block{Hash: "s0", Height: 1, ParentHashes: []string{"genesis"}, BlueScore: 5}
	dag.blocks["s1"] = &Block{Hash: "s1", Height: 1, ParentHashes: []string{"phantom"}, BlueScore: 1}
	child := &Block{Hash: "child", Height: 2, ParentHashes: []string{"s0", "s1"}}
	dag.blocks["child"] = child

	missing, ok := dag.computeGHOSTDAGState(child)
	if ok {
		t.Fatal("expected computeGHOSTDAGState to fail on an unresolvable deep ancestor")
	}
	if missing != "phantom" {
		t.Fatalf("missing = %q, want %q", missing, "phantom")
	}
	if child.SelectedParent != "" || child.Blues != nil || child.BlueScore != 0 || child.KEff != nil {
		t.Fatalf("block fields must stay untouched on failure, got SelectedParent=%q Blues=%v BlueScore=%d KEff=%v",
			child.SelectedParent, child.Blues, child.BlueScore, child.KEff)
	}
}
