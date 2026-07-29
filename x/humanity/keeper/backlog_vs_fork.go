package keeper

import (
	"os"
	"sync"
)

// Telling a shrinking backlog apart from a fork.
//
// THE TRAP THIS ADDRESSES. Contabo1 was measured producing exactly zero blocks
// over 300 heights while the other two validators produced 177 and 123. It was
// not behind (2,114,057 against the primary's 2,114,063), it merged peer blocks
// normally, and POST /api/blocks/by-hash answered correctly on every node. It
// was simply never allowed to produce:
//
//	[BLOCK] ⏳ Not yet 3 consecutive clean sync cycles with every trusted seed
//	        — skipping block production regardless of height-based gates
//
// Its own log explains why: `Tips: 602`. A node that does not produce has
// nothing that merges its accumulated tips, its peers build only on their own,
// so those 602 tips stay unconnected forever, the deferrals behind them never
// resolve, cleanSyncStreak never reaches its threshold, and it never produces.
//
// A validator that falls out of production once cannot get back in on its own,
// because merging its backlog requires producing. That is why a resync has so
// often been the only remedy in this project's history: not because data was
// corrupt, but because the state is a trap.
//
// WHY NOT SIMPLY LOOSEN THE GATE. The gate is correct. It exists because
// dag.height, syncTargetHeight and peerSyncHeight can all read "caught up"
// while a fork is actively in progress, and unresolved deferrals are the one
// signal that distinguishes them. Weakening it would trade a working fork
// detector for a symptom.
//
// THE DISTINCTION IT IS MISSING. A fork GROWS: the diverged peer keeps
// producing, and every new block it makes is one this node can never attach, so
// the unresolved set climbs cycle after cycle. A backlog SHRINKS: the blocks
// are attachable, they are merely behind, and each cycle resolves some. Both
// look identical to "unresolvedDeferrals > 0", which is all the gate looks at.
//
// So this adds the derivative, not a bypass: a cycle still counts as unclean
// whenever the unresolved set grew, and only a set that is holding steady or
// falling — while the node is level with the peer — is treated as a backlog.
//
// OFF BY DEFAULT. This changes when a node is willing to produce blocks, which
// is the most consequential decision it makes. It stays dark until an operator
// turns it on for a specific node, and the default path is byte-for-byte the
// behaviour that shipped.

// backlogEscapeEnabled reports whether an operator asked for this.
func backlogEscapeEnabled() bool {
	v := os.Getenv("AEQUITAS_PRODUCE_WHEN_BACKLOG_SHRINKING")
	return v != "" && v != "0" && v != "false"
}

// backlogHeightSlack separates "level with the peer" from "genuinely behind".
// Within it, an unresolved set that merely holds steady is enough. Beyond it,
// the set has to be actively SHRINKING — see isShrinkingBacklog.
const backlogHeightSlack = 50

var (
	prevUnresolvedMu sync.Mutex
	prevUnresolved   = map[string]int{}
)

// isShrinkingBacklog reports whether an unresolved set should be read as a
// backlog this node can work off rather than as a fork it must not build on.
//
// THE HEIGHT GUARD USED TO MAKE THIS UNREACHABLE. The first version required
// the node to be within backlogHeightSlack of the peer before it would treat
// anything as a backlog. That is precisely the condition a trapped node cannot
// meet: not producing means not merging its own tips, so it falls further
// behind every cycle. Contabo2 sat 780 blocks back and climbing, and the escape
// declined every time — the same shape as the two bootstrap deadlocks found the
// same day, where a mechanism could not engage under the condition it existed
// for. It only ever worked on Contabo1 because that node happened to be six
// blocks behind.
//
// Height distance is the wrong discriminator anyway: a node that was offline for
// an hour is far behind without being forked. The trend is the real signal. A
// fork GROWS — the diverged peer keeps producing blocks this node can never
// attach. A backlog SHRINKS as it is worked off.
//
// So the height slack now only chooses HOW STRICT the trend test is:
//
//   - level with the peer  → holding steady is enough (ordinary DAG latency
//     produces a stable trickle of deferrals that resolve and reappear)
//   - genuinely behind     → the set must be strictly falling, i.e. this node
//     is demonstrably making progress rather than merely not losing ground
//
// A stalled fork cannot satisfy the second: its unresolved set does not fall,
// because nothing in it is attachable.
func isShrinkingBacklog(peerURL string, unresolved int, ownHeight, peerHeight int64) bool {
	if !backlogEscapeEnabled() || unresolved <= 0 {
		return false
	}
	prevUnresolvedMu.Lock()
	prev, seen := prevUnresolved[peerURL]
	prevUnresolved[peerURL] = unresolved
	prevUnresolvedMu.Unlock()

	// Nothing to compare against yet. Assuming the safe answer keeps a node that
	// has just started from producing on a single lucky reading.
	if !seen {
		return false
	}
	if peerHeight-ownHeight <= backlogHeightSlack {
		return unresolved <= prev
	}
	return unresolved < prev
}

// isShrinkingBacklogFor is the BlockDAG-side entry point, so the decision site
// in doSyncOnce reads as one condition rather than three lookups.
//
// peerSyncHeight is read under syncPeerMu, the same lock that guards it
// everywhere else.
func (dag *BlockDAG) isShrinkingBacklogFor(peerURL string, unresolved int) bool {
	if !backlogEscapeEnabled() || unresolved <= 0 {
		return false
	}
	dag.syncPeerMu.Lock()
	peerHeight := dag.peerSyncHeight[peerURL]
	dag.syncPeerMu.Unlock()
	return isShrinkingBacklog(peerURL, unresolved, dag.Height(), peerHeight)
}

// IsOrphanRejection reports whether a block failed only because a parent is
// missing, regardless of how long it has been missing.
//
// IsWithinOrphanGrace answers a narrower question and conflates two very
// different situations in its false case: a block that is not an orphan at all
// (bad signature, unauthorized proposer, finality violation, far-ahead fork)
// and a block that IS an orphan whose parent has simply been missing longer
// than the grace window. Both took the immediate-reset path, and on Contabo2
// the second was the entire cause — counted at 39 resets against zero from
// deferrals, while the node processed blocks a thousand heights back whose
// parents it had never obtained.
//
// That is why the backlog escape did nothing there: it only ever saw
// unresolvedDeferrals, and those were zero. The blocks it was meant to help
// with were leaving through the other branch.
func (dag *BlockDAG) IsOrphanRejection(block *Block) bool {
	if block == nil {
		return false
	}
	dag.mu.Lock()
	defer dag.mu.Unlock()
	for _, ph := range block.ParentHashes {
		if dag.ghostdagBlockLookup(ph, nil) == nil {
			return true
		}
	}
	return false
}
