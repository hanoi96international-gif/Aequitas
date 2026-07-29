package keeper

import "testing"

func withBacklogEscape(t *testing.T) {
	t.Helper()
	t.Setenv("AEQUITAS_PRODUCE_WHEN_BACKLOG_SHRINKING", "1")
	prevUnresolvedMu.Lock()
	prevUnresolved = map[string]int{}
	prevUnresolvedMu.Unlock()
}

// The whole point: a set that keeps growing is a fork, and must keep resetting
// the streak no matter how level the heights look. A diverged peer goes on
// producing, so every cycle brings blocks this node can never attach — that is
// what growth means here, and it is exactly the case the gate exists for.
func TestShrinkingBacklog_GrowingSetIsStillTreatedAsFork(t *testing.T) {
	withBacklogEscape(t)
	const peer = "http://peer:8080"

	if isShrinkingBacklog(peer, 10, 1000, 1000) {
		t.Fatal("the first observation must not count as a backlog; with nothing to compare against the safe answer is 'not caught up'")
	}
	if isShrinkingBacklog(peer, 25, 1000, 1000) {
		t.Fatal("unresolved set grew 10 -> 25 and was still read as a backlog; a fork looks exactly like this")
	}
	if isShrinkingBacklog(peer, 40, 1000, 1000) {
		t.Fatal("unresolved set grew 25 -> 40 and was still read as a backlog")
	}
}

// A set that holds steady or falls, on a node that is level with its peer, is a
// backlog it can work off — and working it off requires producing, which is the
// trap this opens. Contabo1 sat at 602 tips and zero blocks produced for
// exactly this reason.
func TestShrinkingBacklog_FallingSetOnALevelNodeIsABacklog(t *testing.T) {
	withBacklogEscape(t)
	const peer = "http://peer:8080"

	isShrinkingBacklog(peer, 60, 1000, 1000) // first observation, seeds the baseline
	if !isShrinkingBacklog(peer, 45, 1000, 1002) {
		t.Fatal("a falling unresolved set on a node level with its peer must be readable as a backlog")
	}
	if !isShrinkingBacklog(peer, 45, 1000, 1000) {
		t.Fatal("a steady unresolved set must also count; it is not growing, so it is not a fork")
	}
}

// A node far behind must still be able to escape, provided it is demonstrably
// making progress. The first version required the node to be within
// backlogHeightSlack before treating anything as a backlog — which is exactly
// the condition a trapped node cannot meet, since not producing means falling
// further behind every cycle. Contabo2 sat 780 blocks back and climbing and the
// escape declined every single time.
func TestShrinkingBacklog_FarBehindButShrinkingStillEscapes(t *testing.T) {
	withBacklogEscape(t)
	const peer = "http://peer.example:8080"
	far := int64(1000 + backlogHeightSlack + 500)

	isShrinkingBacklog(peer, 400, 1000, far) // Grundlinie
	if !isShrinkingBacklog(peer, 350, 1000, far) {
		t.Fatal("a node far behind whose unresolved set is falling must be able to escape; otherwise the trap is permanent")
	}
}

// But far behind and NOT falling stays gated: merely holding steady is not
// evidence of progress, and a stalled fork looks exactly like that.
func TestShrinkingBacklog_FarBehindAndFlatStaysGated(t *testing.T) {
	withBacklogEscape(t)
	const peer = "http://peer.example:8080"
	far := int64(1000 + backlogHeightSlack + 500)

	isShrinkingBacklog(peer, 400, 1000, far)
	if isShrinkingBacklog(peer, 400, 1000, far) {
		t.Fatal("far behind with a flat unresolved set was treated as a backlog; that is indistinguishable from a stalled fork")
	}
}

// Default OFF. Without the switch the behaviour must be byte-for-byte what
// shipped, because this changes when a node is willing to produce blocks.
func TestShrinkingBacklog_DisabledUnlessOperatorAsks(t *testing.T) {
	t.Setenv("AEQUITAS_PRODUCE_WHEN_BACKLOG_SHRINKING", "")
	prevUnresolvedMu.Lock()
	prevUnresolved = map[string]int{}
	prevUnresolvedMu.Unlock()

	const peer = "http://peer:8080"
	isShrinkingBacklog(peer, 60, 1000, 1000)
	if isShrinkingBacklog(peer, 10, 1000, 1000) {
		t.Fatal("the escape fired without being switched on")
	}
}

// Two peers must not share a baseline, or one peer's progress would excuse
// another's divergence.
func TestShrinkingBacklog_TracksPeersSeparately(t *testing.T) {
	withBacklogEscape(t)
	const a, b = "http://a:8080", "http://b:8080"

	isShrinkingBacklog(a, 50, 1000, 1000)
	isShrinkingBacklog(b, 5, 1000, 1000)

	if isShrinkingBacklog(b, 20, 1000, 1000) {
		t.Fatal("peer b's set grew 5 -> 20 but was excused; baselines are leaking between peers")
	}
	if !isShrinkingBacklog(a, 30, 1000, 1000) {
		t.Fatal("peer a's set fell 50 -> 30 and should read as a backlog")
	}
}
