package keeper

import "testing"

// Chunking the mirror flush is only correct if it still covers every dirty
// address exactly once. The dangerous failure is not a long hold -- it is an
// address silently skipped, whose EVM-mirror balance then stays wrong until
// something else happens to touch it.

// chunkRanges reproduces the loop's slicing so the boundary arithmetic can be
// checked without a ChainState, a database or a lock.
func chunkRanges(n, chunk int) [][2]int {
	var out [][2]int
	for start := 0; start < n; start += chunk {
		stop := start + chunk
		if stop > n {
			stop = n
		}
		out = append(out, [2]int{start, stop})
	}
	return out
}

func TestChunkingCoversEveryAddressExactlyOnce(t *testing.T) {
	// The awkward sizes are the point: exactly one chunk, one short of a
	// chunk, one over, and the size measured under load.
	for _, n := range []int{0, 1, 63, 64, 65, 128, 371, 4000} {
		seen := make([]int, n)
		for _, r := range chunkRanges(n, evmMirrorFlushChunk) {
			if r[0] < 0 || r[1] > n || r[0] >= r[1] {
				t.Fatalf("n=%d produced an invalid range %v", n, r)
			}
			for i := r[0]; i < r[1]; i++ {
				seen[i]++
			}
		}
		for i, c := range seen {
			if c != 1 {
				t.Fatalf("n=%d: address %d covered %d times, want exactly 1.\n"+
					"  A skipped address keeps a stale mirror balance until something else "+
					"happens to touch it; a repeated one writes the same row twice under the "+
					"node's global write lock, which is the cost this change exists to cut.",
					n, i, c)
			}
		}
	}
}

// An empty set must take no lock at all. The flush runs every 2 seconds
// whether or not anything happened.
func TestEmptySetTakesNoChunk(t *testing.T) {
	if got := len(chunkRanges(0, evmMirrorFlushChunk)); got != 0 {
		t.Errorf("an empty dirty set produced %d chunk(s); it must acquire the exclusive "+
			"lock zero times, since this runs on a ticker regardless of activity", got)
	}
}

// The measured case: ~371 addresses used to be one hold. It must now be
// several, or nothing was actually gained.
func TestMeasuredLoadIsSplitIntoSeveralHolds(t *testing.T) {
	const measuredAddrsPerFlush = 371
	n := len(chunkRanges(measuredAddrsPerFlush, evmMirrorFlushChunk))
	if n < 4 {
		t.Errorf("%d addresses split into only %d hold(s) at chunk size %d.\n"+
			"  That set was one single hold when exclusive_busy_pct measured 50.26%% with a "+
			"worst hold of 8,221ms; splitting it is the entire point.",
			measuredAddrsPerFlush, n, evmMirrorFlushChunk)
	}
	// And not so fine that lock acquisitions dominate the work they guard.
	if n > 32 {
		t.Errorf("%d addresses split into %d holds; past a point this trades one long stall "+
			"for so many short ones that the acquisitions cost more than the writes", measuredAddrsPerFlush, n)
	}
}
