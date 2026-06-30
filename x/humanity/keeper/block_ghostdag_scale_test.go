package keeper

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// newGhostdagTestDAG builds a minimal BlockDAG sufficient for exercising
// computeGHOSTDAGState in isolation (no DB, no signing key, no ChainState —
// computeGHOSTDAGState only ever reads dag.blocks).
func newGhostdagTestDAG() *BlockDAG {
	return &BlockDAG{blocks: make(map[string]*Block)}
}

// simulateConcurrentRound creates one block per validator, all sharing the
// SAME parent set (the tips snapshotted before any of them "saw" each
// other's block) — the worst realistic case for a BlockDAG: every validator
// in the network produces within the same propagation window and only
// merges on the next round. This is intentionally adversarial: real traffic
// will usually merge in twos and threes, but a burst (e.g. after a network
// partition heals) can look like this.
func simulateConcurrentRound(height int64, validators int, parentTips []string) []*Block {
	blocks := make([]*Block, validators)
	for v := 0; v < validators; v++ {
		parents := make([]string, len(parentTips))
		copy(parents, parentTips)
		blocks[v] = &Block{
			Height:       height,
			ParentHashes: parents,
			Hash:         fmt.Sprintf("h%d-v%d", height, v),
			Proposer:     fmt.Sprintf("validator-%d", v),
		}
	}
	return blocks
}

// buildMultiValidatorDAG constructs `rounds` rounds of `validators` fully
// concurrent blocks each (every block in a round merges ALL tips from the
// previous round), computing real GHOSTDAG state for every block as it is
// inserted — exactly what AddPeerBlock/ProduceBlock do one block at a time.
// processOrder, if non-nil, is called to permute each round's blocks before
// insertion (used by the determinism test below); pass nil for natural order.
func buildMultiValidatorDAG(t testing.TB, validators, rounds int, shuffle bool) (*BlockDAG, time.Duration) {
	t.Helper()
	dag := newGhostdagTestDAG()

	genesis := &Block{Height: 0, Hash: "genesis", IsGenesis: true}
	dag.blocks["genesis"] = genesis
	tips := []string{"genesis"}

	rng := rand.New(rand.NewSource(42))
	start := time.Now()
	maxMergeSet := 0

	for r := int64(1); r <= int64(rounds); r++ {
		round := simulateConcurrentRound(r, validators, tips)
		if shuffle {
			rng.Shuffle(len(round), func(i, j int) { round[i], round[j] = round[j], round[i] })
		}
		newTips := make([]string, 0, validators)
		for _, b := range round {
			dag.blocks[b.Hash] = b
			dag.computeGHOSTDAGState(b)
			if len(b.Blues) > maxMergeSet {
				maxMergeSet = len(b.Blues)
			}
			newTips = append(newTips, b.Hash)
		}
		tips = newTips
	}
	elapsed := time.Since(start)
	t.Logf("validators=%d rounds=%d total_blocks=%d elapsed=%s max_blue_merge_set=%d",
		validators, rounds, validators*rounds+1, elapsed, maxMergeSet)
	return dag, elapsed
}

// TestGHOSTDAG_ManyValidators_NoHangNoPanic is the basic scale smoke test:
// the project's own comment on ghostdagK (block.go) states the algorithm was
// only ever validated "for our <=3-validator network". This proves (or
// disproves) that it still completes in bounded time and without panicking
// well beyond that, at a scale representative of a 100-node deployment.
func TestGHOSTDAG_ManyValidators_NoHangNoPanic(t *testing.T) {
	const validators = 100
	const rounds = 30 // 3000 blocks, worst-case full-merge every round

	done := make(chan time.Duration, 1)
	go func() {
		_, elapsed := buildMultiValidatorDAG(t, validators, rounds, false)
		done <- elapsed
	}()

	select {
	case elapsed := <-done:
		// Generous bound: this must stay well within one block interval
		// (6s) per block on average for the chain to keep up with
		// production, but for a one-off test run we allow much more
		// slack and just guard against a multi-minute hang/blowup.
		if elapsed > 60*time.Second {
			t.Errorf("computeGHOSTDAGState across %d validators x %d rounds took %s — too slow for a node producing a block every 6s at this validator count", validators, rounds, elapsed)
		}
	case <-time.After(120 * time.Second):
		t.Fatal("computeGHOSTDAGState did not complete within 120s — likely unbounded ancestor walk under high concurrency (see ghostdagIsAncestor)")
	}
}

// buildFixedBlockSet generates ONE canonical, immutable set of blocks (fixed
// hashes, fixed ParentHashes content per block — exactly what a real network
// agrees on, since ParentHashes is baked into a block at production time and
// never changes in transit) without computing any GHOSTDAG state. Use this
// shared block set as input to two separate BlockDAG instances processed in
// different (but each individually valid — parents-before-children) orders,
// to test true delivery-order independence without the test harness itself
// accidentally varying block content (see git history of this test: an
// earlier version regenerated each round's ParentHashes from the *previous*
// run's own shuffle, which varied block content rather than delivery order
// and produced a misleading divergence signal).
func buildFixedBlockSet(validators, rounds int) (genesis *Block, byHeight [][]*Block) {
	genesis = &Block{Height: 0, Hash: "genesis", IsGenesis: true}
	tips := []string{"genesis"}
	byHeight = make([][]*Block, 0, rounds)
	for r := int64(1); r <= int64(rounds); r++ {
		round := simulateConcurrentRound(r, validators, tips)
		byHeight = append(byHeight, round)
		newTips := make([]string, len(round))
		for i, b := range round {
			newTips[i] = b.Hash
		}
		tips = newTips
	}
	return genesis, byHeight
}

// insertFixedBlockSet feeds a pre-built (genesis, byHeight) block set into
// dag, processing each height level in the given per-level order (parents —
// the previous height level — are always fully inserted+computed first, so
// this only varies SIBLING processing order, exactly like real delivery-order
// variance between nodes).
func insertFixedBlockSet(dag *BlockDAG, genesis *Block, byHeight [][]*Block, permute func([]*Block)) {
	dag.blocks[genesis.Hash] = genesis
	for _, round := range byHeight {
		level := append([]*Block(nil), round...) // copy so permute doesn't affect the shared source
		if permute != nil {
			permute(level)
		}
		for _, b := range level {
			dag.blocks[b.Hash] = b
			dag.computeGHOSTDAGState(b)
		}
	}
}

// TestGHOSTDAG_Determinism_OrderIndependent is the correctness property the
// whole replay/StateRoot model depends on: two nodes that receive the exact
// same set of blocks (identical hashes, identical ParentHashes content) but
// process same-height siblings in a different order MUST compute identical
// BlueScore/SelectedParent/Blues for every block. This is what makes
// canonical replay order (height ASC, blueScore DESC, hash ASC) reproducible
// network-wide regardless of P2P/HTTP-sync delivery order.
func TestGHOSTDAG_Determinism_OrderIndependent(t *testing.T) {
	const validators = 12
	const rounds = 8

	genesis, byHeight := buildFixedBlockSet(validators, rounds)

	dagA := newGhostdagTestDAG()
	insertFixedBlockSet(dagA, genesis, byHeight, nil) // natural order

	rng := rand.New(rand.NewSource(7))
	dagB := newGhostdagTestDAG()
	insertFixedBlockSet(dagB, genesis, byHeight, func(level []*Block) {
		rng.Shuffle(len(level), func(i, j int) { level[i], level[j] = level[j], level[i] })
	})

	if len(dagA.blocks) != len(dagB.blocks) {
		t.Fatalf("block count mismatch: %d vs %d", len(dagA.blocks), len(dagB.blocks))
	}
	mismatches := 0
	for hash, a := range dagA.blocks {
		b, ok := dagB.blocks[hash]
		if !ok {
			t.Fatalf("block %s present in run A but missing in run B", hash)
		}
		if a.BlueScore != b.BlueScore {
			t.Errorf("block %s: BlueScore diverged across sibling processing order: A=%d B=%d", hash, a.BlueScore, b.BlueScore)
			mismatches++
		}
		if a.SelectedParent != b.SelectedParent {
			t.Errorf("block %s: SelectedParent diverged across sibling processing order: A=%q B=%q", hash, a.SelectedParent, b.SelectedParent)
			mismatches++
		}
		if len(a.Blues) != len(b.Blues) {
			t.Errorf("block %s: Blues set size diverged across sibling processing order: A=%d B=%d", hash, len(a.Blues), len(b.Blues))
			mismatches++
		}
		if mismatches > 20 {
			t.Fatal("too many mismatches, stopping early")
		}
	}
}

// TestGHOSTDAG_DeepChain_AncestorWalkBounded specifically targets
// ghostdagIsAncestor: a long single-parent chain (simulating a node that has
// been running for a long time — production was already observed at height
// ~45000+) followed by a burst of concurrent blocks at the tip. If
// ghostdagIsAncestor's BFS is unbounded, this round's anticone checks walk
// back toward genesis on every miss, and cost scales with total chain
// height, not with the (bounded) merge-set size — exactly the kind of
// landmine that is invisible at 3 validators / a few thousand blocks but
// fatal at height 100k+ with real concurrency.
func TestGHOSTDAG_DeepChain_AncestorWalkBounded(t *testing.T) {
	dag := newGhostdagTestDAG()
	genesis := &Block{Height: 0, Hash: "genesis", IsGenesis: true}
	dag.blocks["genesis"] = genesis

	const chainDepth = 20000
	parent := "genesis"
	for h := int64(1); h <= chainDepth; h++ {
		hash := fmt.Sprintf("chain-%d", h)
		b := &Block{Height: h, Hash: hash, ParentHashes: []string{parent}}
		dag.blocks[hash] = b
		dag.computeGHOSTDAGState(b)
		parent = hash
	}

	// Now simulate a burst of 50 validators all merging on the deep tip.
	start := time.Now()
	round := simulateConcurrentRound(chainDepth+1, 50, []string{parent})
	for _, b := range round {
		dag.blocks[b.Hash] = b
		dag.computeGHOSTDAGState(b)
	}
	elapsed := time.Since(start)
	t.Logf("burst of 50 concurrent blocks at chain depth %d took %s", chainDepth, elapsed)
	if elapsed > 5*time.Second {
		t.Errorf("burst at depth %d took %s — ancestor checks appear to scale with total chain depth, not merge-set size (see ghostdagIsAncestor's unbounded BFS)", chainDepth, elapsed)
	}
}
