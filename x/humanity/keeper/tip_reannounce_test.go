package keeper

import (
	"testing"
	"time"
)

// dagWithTips builds the minimum a re-announce pass reads: a tip set and the
// blocks behind it.
func dagWithTips(hashes ...string) *BlockDAG {
	dag := &BlockDAG{
		tips:   make(map[string]bool, len(hashes)),
		blocks: make(map[string]*Block, len(hashes)),
	}
	for _, h := range hashes {
		dag.tips[h] = true
		dag.blocks[h] = &Block{Hash: h}
	}
	return dag
}

func TestHealthyNodeReannouncesNothing(t *testing.T) {
	dag := dagWithTips("0xaa", "0xbb")
	sent := dag.reannounceTips(0, func(*Block) {
		t.Error("a node that is producing normally re-broadcast a tip — this must only ever " +
			"run while stuck, or every healthy node adds traffic for nothing")
	})
	if sent != 0 {
		t.Fatalf("sent %d tips while not stalled", sent)
	}
}

func TestBriefStallReannouncesNothing(t *testing.T) {
	// Just under the threshold. A node between blocks is not a node in the
	// trap, and treating it as one would broadcast constantly on a busy chain.
	dag := dagWithTips("0xaa")
	if sent := dag.reannounceTips(tipReannounceAfter-time.Second, func(*Block) {}); sent != 0 {
		t.Fatalf("sent %d tips after a stall shorter than the threshold", sent)
	}
}

func TestSustainedStallReoffersItsTips(t *testing.T) {
	dag := dagWithTips("0xaa", "0xbb")
	var got []string
	sent := dag.reannounceTips(tipReannounceAfter+time.Second, func(b *Block) {
		got = append(got, b.Hash)
	})
	if sent != 2 || len(got) != 2 {
		t.Fatalf("offered %d tips (%v), want both.\n"+
			"  Without this the node waits for a peer that cannot know the tip exists: the peer "+
			"pages by height near its OWN frontier, and fetches missing parents only for blocks "+
			"it already holds. Nothing ever asks for ours.", sent, got)
	}
}

func TestReannounceIsCappedAgainstAForkStorm(t *testing.T) {
	hashes := make([]string, maxTipsToReannounce*3)
	for i := range hashes {
		hashes[i] = string(rune('a'+i%26)) + string(rune('0'+i/26))
	}
	dag := dagWithTips(hashes...)
	sent := dag.reannounceTips(tipReannounceAfter+time.Second, func(*Block) {})
	if sent > maxTipsToReannounce {
		t.Fatalf("offered %d tips, cap is %d.\n"+
			"  Hundreds of tips is a fork storm, and spraying them at a peer is exactly the "+
			"traffic /api/blocks/push's flood shield drops — which would get this node's real "+
			"tips dropped along with the rest.", sent, maxTipsToReannounce)
	}
	if sent == 0 {
		t.Error("offered nothing at all; the cap must limit the pass, not cancel it — the first " +
			"few tips are still what breaks the deadlock")
	}
}

func TestPrunedTipIsSkippedNotFatal(t *testing.T) {
	dag := dagWithTips("0xaa", "0xbb")
	delete(dag.blocks, "0xaa") // in the tip set, no longer in memory
	sent := dag.reannounceTips(tipReannounceAfter+time.Second, func(b *Block) {
		if b == nil {
			t.Fatal("broadcast called with a nil block")
		}
	})
	if sent != 1 {
		t.Fatalf("offered %d, want 1 — a tip whose block has been pruned has nothing to send, "+
			"and must not stop the others being offered", sent)
	}
}

func TestNoTipsIsNotAnError(t *testing.T) {
	dag := &BlockDAG{tips: map[string]bool{}, blocks: map[string]*Block{}}
	if sent := dag.reannounceTips(tipReannounceAfter+time.Second, func(*Block) {}); sent != 0 {
		t.Fatalf("sent %d tips from an empty DAG", sent)
	}
}
