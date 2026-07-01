package keeper

import (
	"fmt"
)

// ── HARD FINALITY CHECKPOINTS ───────────────────────────────────────────────
//
// GHOSTDAG natively provides probabilistic finality (like Kaspa): after K+1
// blue blocks build on top of a block B, the probability that B will ever be
// reorged out is negligible. For K=18 that is ~19 blocks ≈ 2 minutes.
//
// This file adds HARD checkpoint finality on top: once a block's blue_score
// is more than finalityBlueScoreDepth below the current tip, it is declared
// a "finalized checkpoint" and AddPeerBlock refuses any block that would only
// be relevant for a reorg below that depth.
//
// finalityBlueScoreDepth = 50 ≫ 2*K=36: well inside the probabilistic window,
// giving extra safety margin against network jitter.
// finalityHeightSlack     = 50: the height-space equivalent used for the fast
// gate in AddPeerBlock (height and blue_score track each other closely in
// practice; using height avoids the need to compute GHOSTDAG for new blocks
// before the gate fires).
//
// The finalized checkpoint is stored in chain_config and advanced
// monotonically (never retreated). It only affects which new, previously
// unseen blocks the node will accept — already-known blocks in dag.blocks are
// never evicted, and gap-fill blocks for spans WITHIN the finality window are
// still accepted normally.
const (
	finalityBlueScoreDepth = 50
	finalityHeightSlack    = 50
)

// GetFinalizedCheckpoint returns the current finalized height and blue_score
// from chain_config. Returns (0, 0) on a fresh node before any checkpoint
// has been advanced. Safe to call without any lock (uses getConfigValueDB
// which bypasses activeTx).
func (cs *ChainState) GetFinalizedCheckpoint() (height int64, blueScore int64) {
	var h, s int64
	fmt.Sscan(cs.getConfigValueDB("finalized_height"), &h)
	fmt.Sscan(cs.getConfigValueDB("finalized_blue_score"), &s)
	return h, s
}

// SetFinalizedCheckpoint persists the new finalized checkpoint. Caller must
// ensure this only advances (never retreats) the checkpoint.
func (cs *ChainState) SetFinalizedCheckpoint(hash string, height, blueScore int64) {
	cs.setConfigValueDB("finalized_height", fmt.Sprintf("%d", height))
	cs.setConfigValueDB("finalized_blue_score", fmt.Sprintf("%d", blueScore))
	cs.setConfigValueDB("finalized_hash", hash)
}

// isFinalityViolation returns true when block is so far below the current
// finalized checkpoint that accepting it would only matter for a deep reorg —
// which the finality guarantee explicitly forbids. The check uses block.Height
// (known before GHOSTDAG computation) with a generous finalityHeightSlack so
// that legitimate gap-fills within the finality window are never blocked.
// Must be called with dag.mu held (reads dag.state but not dag.blocks).
func (dag *BlockDAG) isFinalityViolation(block *Block) bool {
	if block.IsGenesis || block.FromSync {
		return false // never a finality violation for genesis or HTTP-SYNC canonical replay
	}
	finalizedHeight, _ := dag.state.GetFinalizedCheckpoint()
	if finalizedHeight == 0 {
		return false // checkpoint not yet established
	}
	return block.Height < finalizedHeight-finalityHeightSlack
}

// maybeAdvanceFinalizedCheckpoint walks back along the selected-parent chain
// from newBlock to find the block finalityBlueScoreDepth blue-score units
// below it, then advances the persisted checkpoint to that block if it is
// strictly higher than the current checkpoint. Must be called with dag.mu
// held (reads dag.blocks). Designed to be fast (O(finalityBlueScoreDepth)
// pointer-hops) since it is called on every accepted peer block.
func (dag *BlockDAG) maybeAdvanceFinalizedCheckpoint(newBlock *Block) {
	if newBlock == nil || newBlock.IsGenesis || newBlock.BlueScore < finalityBlueScoreDepth {
		return
	}
	target := newBlock.BlueScore - finalityBlueScoreDepth

	// Walk selected-parent chain toward genesis until we drop to target depth.
	b := newBlock
	for b != nil && b.BlueScore > target {
		if b.SelectedParent == "" {
			return // chain not yet fully resolved (e.g. GHOSTDAG migration pending)
		}
		parent, ok := dag.blocks[b.SelectedParent]
		if !ok {
			return // gap in DAG — can't walk further; try again on next block
		}
		b = parent
	}
	if b == nil || b.IsGenesis {
		return
	}

	curHeight, _ := dag.state.GetFinalizedCheckpoint()
	if b.Height <= curHeight {
		return // checkpoint already at or beyond this block
	}

	dag.state.SetFinalizedCheckpoint(b.Hash, b.Height, b.BlueScore)
	fmt.Printf("[FINALITY] ✓ Checkpoint advanced → height %d blue_score %d (%s...)\n",
		b.Height, b.BlueScore, b.Hash[:min(16, len(b.Hash))])
}
