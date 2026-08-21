package keeper

import (
	"os"
	"strings"
	"testing"
)

// A resync must not be defeated by how long the node has been running.
//
// On 2026-08-21 Contabo1 diverged and could not get back. Every resync attempt
// -- the boot-time one and the in-process auto-heal alike -- died at the same
// step, and the only place it surfaced was degraded_reason:
//
//	resync failed: resync: could not clear chain_blocks:
//	pq: canceling statement due to statement timeout (57014)
//
// `DELETE FROM chain_blocks` walks every row and writes an equal volume of WAL.
// At 4.38 million blocks it cannot finish inside statement_timeout, so the
// transaction rolled back and the node stayed diverged -- while answering
// /api/status and looking healthy the whole time.
//
// The property that matters is not "TRUNCATE is used" for its own sake: it is
// that clearing these tables costs the same whether the node is a day old or a
// year old. A row-by-row DELETE does not have that property and a fresh
// database will never reveal it, which is why this reads the source instead of
// exercising the path -- reproducing it needs millions of rows and a real
// Postgres, and by the time it reproduces, a validator is already down.
func TestResyncClearsBlocksInConstantTime(t *testing.T) {
	src, err := os.ReadFile("snapshot.go")
	if err != nil {
		t.Fatalf("could not read snapshot.go: %v", err)
	}
	content := strings.ReplaceAll(string(src), "\r\n", "\n")

	start := strings.Index(content, "func (cs *ChainState) ResyncFromSnapshotURL")
	if start < 0 {
		start = 0
	}
	body := content[start:]

	for _, table := range []string{"chain_blocks", "chain_accounts", "nullifiers", "bio_registrations"} {
		// Match the EXECUTED statement, not the word. This file and
		// snapshot.go both explain the old DELETE in prose, and an earlier
		// version of this check flagged its own explanation.
		if strings.Contains(body, "Exec(`DELETE FROM "+table) {
			t.Errorf("the resync path still clears %s with DELETE.\n"+
				"  That is O(rows) and cannot finish inside statement_timeout on a node with "+
				"millions of blocks -- the transaction rolls back, the node stays diverged, and "+
				"it keeps reporting healthy while being unable to rejoin the chain.\n"+
				"  Use TRUNCATE, which unlinks the file in constant time.", table)
		}
	}

	if !strings.Contains(body, "TRUNCATE chain_blocks") {
		t.Error("the resync no longer TRUNCATEs chain_blocks. If this moved somewhere else, " +
			"re-point this test rather than deleting it: the failure it guards against is " +
			"invisible until a long-running validator has to resync, which is exactly when " +
			"nobody has time to diagnose it.")
	}

	if !strings.Contains(body, "SET LOCAL statement_timeout = 0") {
		t.Error("the resync transaction no longer lifts statement_timeout. TRUNCATE alone is " +
			"fast, but the rest of this transaction re-inserts the whole snapshot, and the " +
			"timeout that killed the DELETE applies to those statements too. SET LOCAL reverts " +
			"at the end of the transaction, so nothing else on the connection is affected.")
	}
}
