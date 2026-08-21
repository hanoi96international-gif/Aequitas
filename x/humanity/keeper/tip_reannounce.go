package keeper

import (
	"fmt"
	"time"
)

// Offer our own tips again when we cannot produce.
//
// THE DEADLOCK THIS BREAKS, observed on every restart on 2026-08-21.
//
// A node restarts, falls a few hundred blocks behind, and ends up holding TWO
// DAG tips: the peer's chain, and its own last block. Merging the two requires
// a block that names both as parents -- which only this node can make, and it
// cannot, because the production gate is shut. The gate is shut because the
// unmerged tip keeps sync cycles from ever being clean. backlog_vs_fork.go
// names it: "the state is a trap... a resync has so often been the only remedy
// in this project's history: not because data was corrupt, but because the
// state is a trap."
//
// WHY THE PEER NEVER FIXES IT EITHER. doSyncOnce pages by height near the
// PEER'S OWN frontier. Our stale tip sits far below that, so it never falls in
// the window. fetchMissingAncestors would fetch it, but only as a missing
// PARENT of something the peer already has -- and the peer has nothing that
// names our tip as a parent, because it never received it. The block is not
// contested or rejected. It is invisible.
//
// THE WAY OUT is the one the codebase already uses for a freshly produced
// block: push it. HTTPBroadcastBlock's own comment states the mechanism --
// "the peer never adopts the tip, so its next ProduceBlock cannot use this
// block as a parent and the merge this broadcast exists to cause simply does
// not happen". After a restart that broadcast never happened at all, because
// the block was produced before the process died.
//
// So: while this node has not produced for a while, re-offer its tips. The
// peer can produce, adopts the tip as a parent, and the merge that unsticks us
// comes from the other side. Nothing here loosens the gate, invents a new
// consensus path, or writes state -- it re-sends blocks we already produced,
// through the same validated push path, and lets the peer decide.
//
// DELIBERATELY CONSERVATIVE. It only runs while production is stalled, so a
// healthy node never sends anything extra. It is capped, so a genuine fork
// storm cannot turn this into the flood the push shield exists to stop. And it
// is slow: one pass every 30 s is many orders of magnitude below any flood
// threshold, while being far faster than the alternative, which is an operator
// noticing and dispatching a resync.

const (
	// tipReannounceInterval is how often a stalled node re-offers its tips.
	// Slow on purpose: the peer needs one block to adopt the tip, so sending
	// more often cannot help and only adds traffic to a node already behind.
	tipReannounceInterval = 30 * time.Second

	// tipReannounceAfter is how long production must have been stalled first.
	// Longer than admissionStallSeconds, so a node that is merely between
	// blocks never re-broadcasts: by the time this fires, the node has been
	// refusing transactions for a while and something is genuinely wrong.
	tipReannounceAfter = 60 * time.Second

	// maxTipsToReannounce bounds one pass. A healthy DAG carries a handful of
	// tips; hundreds means a fork storm, and spraying those at a peer is the
	// exact traffic /api/blocks/push's flood shield exists to drop. Sending
	// the first few still breaks the two-tip deadlock this targets.
	maxTipsToReannounce = 8
)

// StartTipReannouncer runs the loop. Safe to call once at startup.
func (dag *BlockDAG) StartTipReannouncer() {
	SafeGoroutine("tipReannouncer", func() {
		ticker := time.NewTicker(tipReannounceInterval)
		defer ticker.Stop()
		for range ticker.C {
			dag.reannounceTipsIfStalled()
		}
	})
}

// reannounceTipsIfStalled is one pass with the real clock and transport.
func (dag *BlockDAG) reannounceTipsIfStalled() {
	dag.reannounceTips(productionStalledFor(), dag.HTTPBroadcastBlock)
}

// reannounceTips takes the stall duration and the send function as parameters
// so the decision can be tested without a clock or a network. Every branch
// here is about WHETHER to send, which is the part worth pinning. Returns how
// many tips were offered.
func (dag *BlockDAG) reannounceTips(stalled time.Duration, broadcast func(*Block)) int {
	if stalled < tipReannounceAfter {
		return 0
	}
	tips := dag.GetTips()
	if len(tips) == 0 {
		return 0
	}
	if len(tips) > maxTipsToReannounce {
		tips = tips[:maxTipsToReannounce]
	}
	sent := 0
	for _, hash := range tips {
		block := dag.GetBlockByHash(hash)
		if block == nil {
			continue // pruned from memory; nothing to offer
		}
		broadcast(block)
		sent++
	}
	if sent > 0 {
		fmt.Printf("[TIP-REANNOUNCE] no block produced for %s — re-offered %d tip(s) so a peer "+
			"can merge them; this node cannot merge them itself while the production gate is shut\n",
			stalled.Round(time.Second), sent)
	}
	return sent
}
