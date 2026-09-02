package keeper

import (
	"fmt"
	"testing"
)

// The address cap bounds what a flush freezes. Its dangerous failure is not a
// hold that stays long -- it is a cap so tight that the batch comes back empty
// and the queue never drains at all.

func flushItems(pairs ...[2]string) []walFlushItem {
	out := make([]walFlushItem, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, walFlushItem{from: p[0], to: p[1]})
	}
	return out
}

func TestAddrCapStopsAtTheLimit(t *testing.T) {
	// Six items, two fresh addresses each: 12 distinct addresses in total.
	var q []walFlushItem
	for i := 0; i < 6; i++ {
		q = append(q, walFlushItem{
			from: fmt.Sprintf("0x%040x", 2*i),
			to:   fmt.Sprintf("0x%040x", 2*i+1),
		})
	}

	// A cap of 6 addresses admits exactly three items.
	if got := limitBatchByAddrs(q, len(q), 6); got != 3 {
		t.Errorf("took %d items under a 6-address cap, want 3 (two new addresses per item)", got)
	}
	// A cap at or above the total admits everything.
	if got := limitBatchByAddrs(q, len(q), 12); got != 6 {
		t.Errorf("took %d items under a 12-address cap, want all 6", got)
	}
}

// Repeated addresses cost nothing after the first sighting, so a batch between
// the same two accounts is never split.
func TestRepeatedAddressesDoNotConsumeBudget(t *testing.T) {
	q := flushItems(
		[2]string{"0xaa", "0xbb"},
		[2]string{"0xaa", "0xbb"},
		[2]string{"0xbb", "0xaa"},
		[2]string{"0xaa", "0xbb"},
	)
	if got := limitBatchByAddrs(q, len(q), 2); got != 4 {
		t.Errorf("took %d of 4 items that touch only 2 addresses under a 2-address cap, want 4 — "+
			"the cap counts DISTINCT addresses, and a hot pair must not be split for no benefit", got)
	}
}

// The failure that matters: a cap smaller than one item's address count must
// still make progress, or the queue stalls forever and every transfer
// eventually hits the backpressure bail.
func TestCapNeverReturnsAnEmptyBatch(t *testing.T) {
	q := flushItems([2]string{"0xaa", "0xbb"}, [2]string{"0xcc", "0xdd"})
	for _, cap := range []int{1, 2} {
		if got := limitBatchByAddrs(q, len(q), cap); got < 1 {
			t.Fatalf("a cap of %d produced a batch of %d items.\n"+
				"  A flush that takes nothing never drains the queue, so the depth grows until "+
				"transferConcurrentWAL's backpressure check bails every transfer to the batcher — "+
				"strictly worse than any hold this cap was meant to shorten.", cap, got)
		}
	}
}

// Zero and negative mean "no cap", which must be exactly the shipped behaviour.
func TestZeroMeansNoCap(t *testing.T) {
	var q []walFlushItem
	for i := 0; i < 50; i++ {
		q = append(q, walFlushItem{
			from: fmt.Sprintf("0x%040x", 2*i),
			to:   fmt.Sprintf("0x%040x", 2*i+1),
		})
	}
	for _, cap := range []int{0, -1} {
		if got := limitBatchByAddrs(q, len(q), cap); got != len(q) {
			t.Errorf("cap %d took %d of %d items; it must be a no-op", cap, got, len(q))
		}
	}
}

// An unusable environment value must disable the cap rather than pick some
// arbitrary limit -- the same rule admissionStallLimit follows.
func TestBadEnvValueDisablesTheCap(t *testing.T) {
	for _, bad := range []string{"abc", "-5", "12.5", ""} {
		t.Setenv(walFlushMaxAddrsEnv, bad)
		if got := walFlushMaxAddrs(); got != 0 {
			t.Errorf("value %q gave a cap of %d, want 0 (no cap) — an unreadable setting must "+
				"fall back to shipped behaviour, never to a guess", bad, got)
		}
	}
	t.Setenv(walFlushMaxAddrsEnv, "64")
	if got := walFlushMaxAddrs(); got != 64 {
		t.Errorf("a valid value gave %d, want 64", got)
	}
}
