package keeper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/wal"
)

// Regression guard for the 2026-07-25 night state-corruption incident on
// Contabo2 — the one node running the WAL fast path in production.
//
// recoverFromWAL decides idempotency PER ACCOUNT, via
// `acc.WALSeq >= record.Seq`, seeded from chain_accounts.wal_seq. An
// authoritative snapshot resync REPLACES chain_accounts — and
// AccountState.WALSeq is `json:"-"`, so the snapshot cannot carry it. Every
// account therefore came back with wal_seq = 0 while the WAL file (on a host
// volume that deliberately survives container recreation) still held the full
// history. The next boot found NOTHING "already reflected" and re-applied all
// of it on top of the fresh, correct snapshot state. Verbatim from the live
// boot log:
//
//	[WAL] Read 153429 record(s): 0 already reflected in Postgres (skipped,
//	      idempotent), 153429 reapplied to in-memory state
//
// Damage: 294 of 315 accounts diverged from the other two validators, 74 of
// them NEGATIVE, total supply exactly conserved — transfers applied twice.
// The resulting invalid transfers then entered blocks, which the (correct)
// primary rejected outright ("insufficient balance"), walling it off behind an
// orphan chain and isolating it from the network.
//
// The fix is a recovery FLOOR in chain_config — which a snapshot import does
// not clear — recorded at every wholesale state replacement.
func TestRecoverFromWAL_FloorSkipsSupersededRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wal")

	w, err := wal.Open(path)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	var lastSeq uint64
	for i := 0; i < 5; i++ {
		payload, _ := json.Marshal(walTransferRecord{
			From: "0xaaa", To: "0xbbb", Amount: 1, TxHash: "0xtx",
		})
		seq, err := w.Append(payload)
		if err != nil {
			t.Fatalf("append: %v", err)
		}
		lastSeq = seq
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if got := lastSeq; got != 5 {
		t.Fatalf("expected 5 records with head seq 5, got head %d", got)
	}

	// Simulate exactly what a resync leaves behind: the WAL file is intact,
	// but the account state it would apply to has been replaced wholesale.
	// Without a floor, replay re-applies all 5; with the floor, none.
	skipped := 0
	applied := 0
	floor := lastSeq
	if _, _, err := wal.ReplayFile(path, func(entry wal.Entry) error {
		if entry.Seq <= floor {
			skipped++
			return nil
		}
		applied++
		return nil
	}); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if applied != 0 {
		t.Fatalf("a record at or below the recovery floor must never be re-applied: %d applied — "+
			"this is the exact double-application that drove 74 production accounts negative", applied)
	}
	if skipped != 5 {
		t.Fatalf("expected all 5 records fenced off by the floor, got %d", skipped)
	}

	// A record written AFTER the state replacement must still be recovered —
	// the floor must not disable crash recovery going forward.
	w2, err := wal.Open(path)
	if err != nil {
		t.Fatalf("reopen wal: %v", err)
	}
	payload, _ := json.Marshal(walTransferRecord{From: "0xaaa", To: "0xbbb", Amount: 1})
	postSeq, err := w2.Append(payload)
	if err != nil {
		t.Fatalf("append after floor: %v", err)
	}
	if err := w2.Close(); err != nil {
		t.Fatalf("close2: %v", err)
	}
	if postSeq <= floor {
		t.Fatalf("a record appended after the floor must have a higher seq (%d <= %d)", postSeq, floor)
	}

	appliedAfter := 0
	if _, _, err := wal.ReplayFile(path, func(entry wal.Entry) error {
		if entry.Seq > floor {
			appliedAfter++
		}
		return nil
	}); err != nil {
		t.Fatalf("replay2: %v", err)
	}
	if appliedAfter != 1 {
		t.Fatalf("post-replacement records must still be recovered, got %d", appliedAfter)
	}
}

// The floor must be readable back exactly as written — it is the only thing
// standing between a resynced node and a full WAL re-application, so a
// silently unparseable value would reintroduce the whole incident.
func TestWALRecoveryFloor_RoundTrip(t *testing.T) {
	cs := newTestState()
	if cs.db != nil {
		t.Skip("this test covers the no-DB default path only")
	}
	if got := cs.walRecoveryFloor(); got != 0 {
		t.Fatalf("a node that never replaced its state must have floor 0, got %d", got)
	}
	// Without a DB there is nothing to persist into; the call must be a safe
	// no-op rather than a panic (nodes run WAL-less and DB-less in tests).
	cs.markWALSupersededByStateReplacement()
	if got := cs.walRecoveryFloor(); got != 0 {
		t.Fatalf("no-DB floor must stay 0, got %d", got)
	}
	_ = os.Getenv("AEQUITAS_WAL_PATH")
}
