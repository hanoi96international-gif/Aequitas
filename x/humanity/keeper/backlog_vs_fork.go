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

// backlogHeightSlack is how close to a peer's height this node must be for its
// unresolved set to be read as a backlog at all. A genuinely forked node is not
// merely a few blocks off: it is on a different chain, and its height diverges
// and keeps diverging. A handful of blocks is ordinary DAG latency.
const backlogHeightSlack = 50

var (
	prevUnresolvedMu sync.Mutex
	prevUnresolved   = map[string]int{}
)

// noteUnresolvedDeferrals records this cycle's unresolved count for a peer and
// reports whether it grew since the previous cycle.
//
// The first observation counts as growth: with nothing to compare against,
// assuming the safe answer keeps a node that has just started from producing on
// a single lucky reading.
func noteUnresolvedDeferrals(peerURL string, unresolved int) (grew bool) {
	prevUnresolvedMu.Lock()
	defer prevUnresolvedMu.Unlock()
	prev, seen := prevUnresolved[peerURL]
	prevUnresolved[peerURL] = unresolved
	if !seen {
		return true
	}
	return unresolved > prev
}

// isShrinkingBacklog reports whether an unresolved set should be read as a
// backlog this node can work off rather than as a fork it must not build on.
//
// Every condition has to hold: the operator asked for it, this node is level
// with the peer, and the unresolved set is not growing. Any one of them missing
// falls back to treating the cycle as unclean, exactly as before.
func isShrinkingBacklog(peerURL string, unresolved int, ownHeight, peerHeight int64) bool {
	if !backlogEscapeEnabled() || unresolved <= 0 {
		return false
	}
	grew := noteUnresolvedDeferrals(peerURL, unresolved)
	if grew {
		return false
	}
	// Level with the peer, or ahead of it. Behind by more than the slack is
	// the case the gate was built for and is left untouched.
	return peerHeight-ownHeight <= backlogHeightSlack
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
