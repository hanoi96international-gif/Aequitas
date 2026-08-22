package keeper

import (
	"os"
	"strconv"
)

// Bound a flush by how much of the ADDRESS SPACE it freezes, not just by how
// many items it carries.
//
// WHY ITEM COUNT IS THE WRONG UNIT. flushWALBatch holds cs.accounts.LockAddrs
// over every address in its batch for the whole Postgres transaction, and that
// hold is what transfers collide with. The existing cap is on items
// (walFlushMaxBatch, 4000), which does not bound the thing that costs:
// measured under load, a flush carried 393 items and locked 371 distinct
// addresses for 37ms. Against a test that only has ~1,246 live addresses, a
// single flush therefore freezes roughly a third of the address space at once,
// and 32 concurrent flushes freeze it several times over.
//
// That also explains a result that looked paradoxical: raising
// AEQUITAS_WAL_FLUSH_CONCURRENCY from 32 to 64 made throughput WORSE
// (3,779 -> 3,476) while addrs_per_flush stayed at ~371. More workers did not
// make each flush smaller, so all it bought was more of the address space
// frozen simultaneously.
//
// WHY EARLIER SWEEPS DO NOT SETTLE THIS. The batch-size, flush-interval and
// concurrency sweeps were all run while the RPC rate limiter was capping the
// node at 1,000 transfers/s (see the limiter finding of 2026-08-22). At a fifth
// of the real load the flush queue barely fills, so none of those sweeps
// exercised the regime they were meant to tune.
//
// DEFAULT IS OFF. Zero means "no address cap", which is byte-for-byte the
// behaviour that shipped. The cap is enabled per box via
// AEQUITAS_WAL_FLUSH_MAX_ADDRS so it can be swept against a live load without
// a code change, and so this file cannot alter production until someone
// deliberately turns it on.
//
// A cap never takes zero items: if the very first item already exceeds it, that
// item is still taken. Otherwise a low cap would stall the queue forever, which
// is a far worse failure than a long hold.

const walFlushMaxAddrsEnv = "AEQUITAS_WAL_FLUSH_MAX_ADDRS"

// walFlushMaxAddrs reports the per-flush distinct-address cap, or 0 for none.
//
// Parsed per call rather than cached: this is read once per flush, not per
// transfer, so the cost is irrelevant next to being able to change it on a live
// node. An unusable value yields 0 (no cap) — the shipped behaviour — for the
// same reason admissionStallLimit refuses to weaken itself on a typo.
func walFlushMaxAddrs() int {
	raw := os.Getenv(walFlushMaxAddrsEnv)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// limitBatchByAddrs returns how many of the first n queued items may be taken
// without the batch touching more than maxAddrs distinct addresses.
//
// Items are consumed as a prefix so the queue stays FIFO — reordering to pack
// more items per address set would delay whatever it skipped, and the flush is
// already the lagging half of this design.
func limitBatchByAddrs(queue []walFlushItem, n, maxAddrs int) int {
	if maxAddrs <= 0 || n <= 0 {
		return n
	}
	seen := make(map[string]struct{}, maxAddrs*2)
	taken := 0
	for taken < n {
		it := queue[taken]
		added := 0
		if _, ok := seen[it.from]; !ok {
			added++
		}
		if _, ok := seen[it.to]; !ok && it.to != it.from {
			added++
		}
		// Always take at least one item, whatever the cap: a batch of zero
		// makes no progress and the queue would never drain.
		if taken > 0 && len(seen)+added > maxAddrs {
			break
		}
		seen[it.from] = struct{}{}
		seen[it.to] = struct{}{}
		taken++
	}
	return taken
}
