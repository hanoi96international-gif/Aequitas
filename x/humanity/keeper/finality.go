package keeper

import (
	"fmt"
	"time"
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

// isolatedFinalityPauseWindow bounds how long a node that KNOWS about other
// authorized validators may keep hardening its own finality checkpoint from
// self-produced blocks alone, without having successfully merged any of
// those other validators' blocks in.
//
// ROOT CAUSE (2026-07-04, Contabo 2 permanent-isolation incident): a node
// that falls out of real-time sync with its peers — for ANY reason: a
// registration hiccup, a network blip, a slow restart — keeps producing and
// self-finalizing its own one-parent chain the entire time it's isolated,
// because maybeAdvanceFinalizedCheckpoint previously advanced unconditionally
// on every self-produced block (see that ProduceBlock call site's own P0
// 2026-07-03 fix — added for the opposite failure, a solo validator whose
// checkpoint never advanced at all). Once the isolation gap exceeds
// finalityBlueScoreDepth/finalityHeightSlack worth of blocks (~50, a few
// minutes at BLOCK_TIME), isFinalityViolation permanently walls off the
// real peer chain at those heights — SelfFetched's circuit-breaker bypass,
// deepScan, fetchMissingAncestors, none of it can ever cross a HARD finality
// gate by design. Confirmed live: Contabo 2 re-diverged and re-locked itself
// within under an hour of a fresh checkpoint-seeded resync, because nothing
// locally distinguished "healthy, merging fine" from "isolated, hardening a
// dead-end fork" — both looked identical from inside ProduceBlock.
//
// The fix: only let self-produced blocks harden the checkpoint when either
// (a) this node doesn't know of any OTHER authorized validator yet (a
// genuinely solo network — the original 2026-07-03 fix's exact case, left
// fully intact), or (b) a foreign block was merged within this window. A
// node that's actually diverged just pauses its own hardening — its
// checkpoint stops advancing (never retreats) until real merging resumes,
// at which point the very next accepted peer block (AddPeerBlock's own
// unconditional call site) unfreezes it immediately. This can never
// reproduce the pre-2026-07-03 "frozen forever" bug: that only happened for
// a node with NO peers at all, which case (a) above still exempts.
//
// Generous relative to finalityBlueScoreDepth's own ~5-minute real-time
// equivalent (50 blocks * BLOCK_TIME) so ordinary propagation jitter across
// independently-hosted, cross-provider validators never falsely pauses
// hardening — only a gap already large enough to risk a real finality lock
// trips this.
const isolatedFinalityPauseWindow = 10 * time.Minute

// recordForeignMerge marks now as the last time this node successfully
// attached another authorized validator's block — called from AddPeerBlock's
// commit path. Must be called without dag.mu held (atomic only).
func (dag *BlockDAG) recordForeignMerge() {
	dag.lastForeignMergeAt.Store(time.Now().Unix())
}

// selfProducedFinalityAllowed reports whether ProduceBlock's own
// self-produced block may advance the hard finality checkpoint right now —
// see isolatedFinalityPauseWindow's comment for the full rationale. Must be
// called with dag.mu held (reads dag.authorizedValidators, dag.selfProposer).
func (dag *BlockDAG) selfProducedFinalityAllowed() bool {
	hasOtherValidator := false
	for addr := range dag.authorizedValidators {
		if addr != dag.selfProposer {
			hasOtherValidator = true
			break
		}
	}
	if !hasOtherValidator {
		return true // genuinely solo network — advance freely (2026-07-03 fix's exact case)
	}
	last := dag.lastForeignMergeAt.Load()
	return last != 0 && time.Since(time.Unix(last, 0)) <= isolatedFinalityPauseWindow
}

// GetFinalizedCheckpoint returns the current finalized height and blue_score.
// Returns (0, 0) on a fresh node before any checkpoint has been advanced.
// Safe to call without dag.mu/cs.mu held.
//
// FIX (P0, cadence audit 2026-07-03): this used to hit chain_config via
// getConfigValueDB on every call — two synchronous Postgres round trips —
// and is called at least twice per accepted peer block (isFinalityViolation,
// maybeAdvanceFinalizedCheckpoint), both while dag.mu is write-locked. Over
// the primary's remote DB proxy that reintroduced exactly the class of
// cadence bug the 2026-07-02 commits eliminated elsewhere (see ProduceBlock's
// post-commit bookkeeping comment), just in a path those commits didn't
// touch. The checkpoint is only ever written by this process — through
// SetFinalizedCheckpoint/ResetFinalizedCheckpoint below, both of which keep
// this cache in sync — so a cache populated once at first use is always
// current for the rest of the process's lifetime; no separate invalidation
// path is needed.
func (cs *ChainState) GetFinalizedCheckpoint() (height int64, blueScore int64) {
	cs.finalizedMu.RLock()
	if cs.finalizedCacheLoaded {
		h, s := cs.finalizedHeightCache, cs.finalizedBlueScoreCache
		cs.finalizedMu.RUnlock()
		return h, s
	}
	cs.finalizedMu.RUnlock()

	// First call this process: load once from durable storage.
	cs.finalizedMu.Lock()
	defer cs.finalizedMu.Unlock()
	if cs.finalizedCacheLoaded { // another goroutine won the race to load it
		return cs.finalizedHeightCache, cs.finalizedBlueScoreCache
	}
	var h, s int64
	fmt.Sscan(cs.getConfigValueDB("finalized_height"), &h)
	fmt.Sscan(cs.getConfigValueDB("finalized_blue_score"), &s)
	cs.finalizedHeightCache, cs.finalizedBlueScoreCache = h, s
	cs.finalizedCacheLoaded = true
	return h, s
}

// SetFinalizedCheckpoint advances the in-memory checkpoint immediately —
// every caller in this process sees the new value right away, even before
// the DB write below finishes — and persists it in the background. Caller
// must ensure this only advances (never retreats) the checkpoint.
//
// FIX (P0, cadence audit 2026-07-03): the 3 chain_config writes here used to
// run synchronously, 3 more round trips on top of GetFinalizedCheckpoint's,
// on every block that advances the checkpoint — the common case once past
// the initial finalityBlueScoreDepth blocks. Same reasoning as the
// ProduceBlock post-commit fix: this value is monotonic and
// self-re-advancing, so losing the very last write on an ill-timed crash
// only leaves the hard-finality gate briefly less strict than it could be
// after restart — never less safe than not having advanced yet at all —
// while GHOSTDAG's own probabilistic finality is unaffected either way.
func (cs *ChainState) SetFinalizedCheckpoint(hash string, height, blueScore int64) {
	cs.finalizedMu.Lock()
	cs.finalizedHeightCache = height
	cs.finalizedBlueScoreCache = blueScore
	cs.finalizedHashCache = hash
	cs.finalizedCacheLoaded = true
	cs.finalizedMu.Unlock()

	go func() {
		cs.setConfigValueDB("finalized_height", fmt.Sprintf("%d", height))
		cs.setConfigValueDB("finalized_blue_score", fmt.Sprintf("%d", blueScore))
		cs.setConfigValueDB("finalized_hash", hash)
	}()
}

// ResetFinalizedCheckpoint clears the finalized checkpoint back to its
// zero state (chain_config value "0" for all three keys, matching how a
// fresh node reads before any checkpoint has ever been set). Used only by
// ResyncFromSnapshotURL when chain_blocks is wiped for a clean re-sync — a
// stale finalized_height surviving the wipe deadlocks catch-up (see that
// call site's comment). Unlike SetFinalizedCheckpoint, this persists
// synchronously and returns any error: it runs during an already-slow,
// rare bootstrap path where correctness and error visibility matter more
// than shaving a few round trips. Caller must hold cs.mu (same precondition
// as the setConfigValue calls inside).
func (cs *ChainState) ResetFinalizedCheckpoint() error {
	cs.finalizedMu.Lock()
	cs.finalizedHeightCache = 0
	cs.finalizedBlueScoreCache = 0
	cs.finalizedHashCache = "0"
	cs.finalizedCacheLoaded = true
	cs.finalizedMu.Unlock()

	for _, fk := range []string{"finalized_height", "finalized_blue_score", "finalized_hash"} {
		if err := cs.setConfigValue(fk, "0"); err != nil {
			return fmt.Errorf("could not reset %s: %w", fk, err)
		}
	}
	return nil
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

// releaseFinalitySealedStubs stops synthetic-checkpoint stubs from gating
// block production once the finalized checkpoint has moved more than
// finalityHeightSlack past them. Must be called with dag.mu held (writes
// dag.unverifiedStubHeights).
//
// Rationale: the production gate on UnverifiedSyntheticCheckpointCount
// (ProduceBlock, audit P1-05) exists so a node doesn't mint new blocks on
// top of bridged-but-unverified ancestry "until real history syncs in behind
// it". But once the finalized checkpoint is finalityHeightSlack above a
// stub's height, isFinalityViolation rejects the real block from every
// non-seed peer — the node's own hard-finality rule makes the heal the gate
// is waiting for impossible, so the gate degenerates from "wait until
// verified" into "never produce again". At that point the bridged region is
// below the hard-finality line (immutable by protocol), already reflected in
// this node's account state, and continuously cross-checked against peers
// via StateRoot comparison + divergence auto-heal — releasing the stub
// trades no real verification away. Confirmed live on Contabo 2026-07-03: a
// runtime-orphan-bridge stub over genuinely-lost blocks (gone from every
// node's DB, unhealable forever) silently turned a 2-validator network into
// a 1-validator network. A stub still WITHIN the finality window keeps
// gating production exactly as before — a fresh gap is genuinely suspect
// until the network finalizes past it.
func (dag *BlockDAG) releaseFinalitySealedStubs() {
	if dag.unverifiedSyntheticCheckpointCount.Load() == 0 || dag.state == nil {
		return
	}
	finalizedHeight, _ := dag.state.GetFinalizedCheckpoint()
	if finalizedHeight == 0 {
		return
	}
	for hash, h := range dag.unverifiedStubHeights {
		if h < finalizedHeight-finalityHeightSlack {
			delete(dag.unverifiedStubHeights, hash)
			dag.unverifiedSyntheticCheckpointCount.Add(-1)
			fmt.Printf("[FINALITY] ✓ Synthetic checkpoint at height %d (%s...) sealed by finality (checkpoint %d) — released from the production gate, %d unverified stub(s) remain\n",
				h, hash[:min(16, len(hash))], finalizedHeight, dag.unverifiedSyntheticCheckpointCount.Load())
		}
	}
}

// maybeAdvanceFinalizedCheckpoint walks back along the selected-parent chain
// from newBlock to find the block finalityBlueScoreDepth blue-score units
// below it, then advances the persisted checkpoint to that block if it is
// strictly higher than the current checkpoint. Must be called with dag.mu
// held (reads dag.blocks). Designed to be fast (O(finalityBlueScoreDepth)
// pointer-hops) since it is called on every accepted peer block.
func (dag *BlockDAG) maybeAdvanceFinalizedCheckpoint(newBlock *Block) {
	// Piggyback the stub-release sweep here: runs under dag.mu on every
	// accepted peer block, and no-ops via one atomic read when there are no
	// unverified stubs (the overwhelmingly common case).
	dag.releaseFinalitySealedStubs()
	if newBlock == nil || newBlock.IsGenesis || newBlock.BlueScore < finalityBlueScoreDepth {
		return
	}
	target := newBlock.BlueScore - finalityBlueScoreDepth

	// Walk selected-parent chain toward genesis until we drop to target depth.
	// Fast path only: pure in-memory pointer-hops, no DB access — this runs
	// while the caller holds dag.mu (see AddPeerBlock), so it must stay cheap.
	b := newBlock
	for b != nil && b.BlueScore > target {
		if b.SelectedParent == "" {
			return // chain not yet fully resolved (e.g. GHOSTDAG migration pending)
		}
		parent, ok := dag.blocks[b.SelectedParent]
		if !ok {
			// FIX (P0, merge-reliability audit 2026-07-03): pruneOldDAGBlocks
			// evicts in-memory blocks below (finalizedHeight - pruneBuffer()), a
			// HEIGHT-based cutoff — but this walk's target is BLUE-SCORE-based.
			// On a heavily-forked DAG (many non-selected/"red" blocks per
			// blue-score point — confirmed live: blue_score 1694 at height
			// 80095, a ~47:1 ratio) a finalityBlueScoreDepth-deep walk can need
			// to reach much further back in HEIGHT than the height-based buffer
			// guarantees, hitting an evicted (but NOT deleted — chain_blocks
			// retains it, see pruneOldDAGBlocks' own comment) parent. Silently
			// giving up here used to freeze finalized_height forever: every
			// later block hits the exact same gap on the exact same
			// selected-parent chain, which ALSO freezes pruneOldDAGBlocks' own
			// cutoff (it's finalizedHeight-relative) — a permanent,
			// self-reinforcing deadlock confirmed live on Contabo
			// (finalized_height stuck at 80095 while the tip passed 130,000,
			// which in turn left the isolated-fork auto-heal check in
			// autoheal.go permanently comparing the same stale, still-matching
			// height instead of anything recent enough to catch the actual
			// divergence). Finish the walk asynchronously against the DB
			// (which always still has it) instead of leaving the checkpoint
			// stuck — see finishCheckpointWalkFromDB.
			dag.finishCheckpointWalkFromDB(b.SelectedParent, target)
			return
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

// finishCheckpointWalkFromDB continues the selected-parent walk started by
// maybeAdvanceFinalizedCheckpoint once it hits a block evicted from the
// in-memory DAG (dag.blocks), resuming from startHash and stopping once
// blue_score drops to target or below (or a genesis/unresolved parent is
// hit). Runs in its own goroutine, off dag.mu — safe because it only reads
// already-committed, immutable block headers (chain_blocks rows are never
// modified after insert by SaveBlockToDB's ON CONFLICT DO NOTHING) and its
// result feeds SetFinalizedCheckpoint, which has its own independent lock
// (finalizedMu — see GetFinalizedCheckpoint's comment) rather than dag.mu.
func (dag *BlockDAG) finishCheckpointWalkFromDB(startHash string, target int64) {
	go func() {
		if dag.state == nil {
			return
		}
		hash := startHash
		var b *Block
		for {
			b = dag.state.LoadBlockFromDBByHash(hash)
			if b == nil {
				fmt.Printf("[FINALITY] ✗ Could not advance checkpoint: %s… is missing from both the in-memory DAG and the DB — a genuinely lost block, not just a pruned one\n",
					hash[:min(16, len(hash))])
				return
			}
			if b.BlueScore <= target || b.SelectedParent == "" {
				break
			}
			hash = b.SelectedParent
		}
		if b == nil {
			return
		}
		curHeight, _ := dag.state.GetFinalizedCheckpoint()
		if b.Height <= curHeight {
			return // another call already advanced past this point
		}
		dag.state.SetFinalizedCheckpoint(b.Hash, b.Height, b.BlueScore)
		fmt.Printf("[FINALITY] ✓ Checkpoint advanced (via DB fallback, in-memory DAG had pruned the path) → height %d blue_score %d (%s...)\n",
			b.Height, b.BlueScore, b.Hash[:min(16, len(b.Hash))])
	}()
}
