package keeper

// maxExtraBlocksPerTick bounds how many ADDITIONAL blocks
// ProduceBlocksForTick will produce within a single BLOCK_TIME window,
// beyond the first (normal, wall-clock-aligned) one -- see that function's
// own doc comment for why this exists and why it is deliberately small.
const maxExtraBlocksPerTick = 4

// MaxExtraBlocksPerTick exposes maxExtraBlocksPerTick to main.go, purely
// for its own startup log line — keeps the constant itself unexported
// (nothing outside this package needs to depend on the specific number).
func MaxExtraBlocksPerTick() int { return maxExtraBlocksPerTick }

// ProduceBlocksForTick is SCALING_ARCHITECTURE.md Phase 9's cadence
// follow-up: instead of the block-production ticker (main.go) calling
// ProduceBlock() exactly once per BLOCK_TIME tick unconditionally, it
// calls this instead -- which calls ProduceBlock() once (identical
// behavior to before this existed, for the common case), and ONLY when
// that block came back completely full (its transaction count hit
// maxTxsPerBlock -- evm_storage.go's own strong, cheap-to-check signal
// that more work is genuinely still queued) keeps calling ProduceBlock()
// again, immediately, up to maxExtraBlocksPerTick additional times,
// stopping the moment a block comes back NOT full (backlog drained) or
// nil (some other gate -- catch-up, degraded, resync, etc. -- kicked in).
//
// Deliberately does NOT touch ProduceBlock's own ~700-line body at all --
// every extra call is a completely ordinary, unmodified ProduceBlock()
// invocation, with its own full dag.mu/replayMu acquisition and every
// existing gate/check intact. This function only adds OUTER SCHEDULING:
// "call it again, right now, instead of waiting for the next tick" -- the
// minimum possible change surface for real throughput headroom without
// touching consensus-critical internals, and, unlike shortening BLOCK_TIME
// itself, a purely PER-VALIDATOR decision: a validator running this
// produces more blocks when genuinely backlogged; every other validator
// (including ones still calling ProduceBlock() once per tick, the
// pre-this-change behavior) simply receives and merges however many
// blocks arrive via the existing P2P gossip/GHOSTDAG multi-parent design
// -- no network-wide coordinated upgrade required, unlike a BLOCK_TIME
// change (which affects every node's own tick alignment/breaker/finality
// tuning simultaneously -- the reason that option was set aside as the
// higher-risk of the two cadence approaches considered for this work).
//
// The FIRST call's timing is unaffected: it still happens exactly on the
// existing wall-clock-aligned tick, preserving the "every validator
// produces within ~50ms of every other" property that makes concurrent
// GHOSTDAG merging work well (see the ticker's own alignment comment,
// main.go). Only the EXTRA blocks (if any) happen immediately after,
// inside the same tick, faster than BLOCK_TIME would otherwise allow.
//
// NOT STAGING-VALIDATED: this changes how many blocks a validator can
// produce within one BLOCK_TIME window -- a genuinely different risk
// class from every other change in this session (multi-node consensus
// timing can't be fully validated by single-process testing the way
// storage-layer correctness can). maxExtraBlocksPerTick is deliberately
// conservative for exactly that reason, and this is disabled by default
// (see ENABLE_MULTI_BLOCK_TICK in main.go) -- enabling it on a live node
// is an explicit operator decision, the same class as BLOCK_TIME itself.
func (dag *BlockDAG) ProduceBlocksForTick() []*Block {
	return produceBlocksForTick(dag.ProduceBlock)
}

// produceBlocksForTick holds ProduceBlocksForTick's actual looping logic,
// parameterized over produce (normally dag.ProduceBlock) instead of calling
// it directly. Building a real BlockDAG that successfully produces a block
// end-to-end needs substantial setup (genesis, a signing key, every one of
// ProduceBlock's own ~700-line body's gates satisfied) that no existing
// test in this package sets up even once — this split lets the LOOP
// behavior (the only thing genuinely new here; ProduceBlock's own body is
// untouched and separately already relied upon in production) be tested
// directly with a fake producer instead, at real unit-test speed and
// without inventing that missing integration harness just for this.
func produceBlocksForTick(produce func() *Block) []*Block {
	var produced []*Block
	block := produce()
	if block == nil {
		return produced
	}
	produced = append(produced, block)

	for extra := 0; extra < maxExtraBlocksPerTick; extra++ {
		if len(block.Transactions) < maxTxsPerBlock {
			break // backlog drained (or was never that deep) -- done for this tick
		}
		block = produce()
		if block == nil {
			break // some other gate kicked in (catch-up, degraded, etc.) -- stop, don't fight it
		}
		produced = append(produced, block)
	}
	return produced
}
