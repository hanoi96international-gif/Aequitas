package wal

import (
	"os"
	"path/filepath"
	"testing"
)

// Audit 2026-08-16 (transfer/WAL/concurrency pass), finding WAL-SEQ.
//
// writeBatch states the invariant this file tests, verbatim: "sequence numbers
// are only ever a monotonic ordering label, never reused" (wal.go:235-238). Two
// consumers in the keeper depend on that holding across process restarts, and
// both persist sequence numbers OUTSIDE this file:
//
//   - chain_accounts.wal_seq, written by flushWALBatch's UPSERT under the guard
//     `WHERE chain_accounts.wal_seq < EXCLUDED.wal_seq` (keeper/transfer_wal.go:832).
//     A reused, therefore lower, sequence number makes that predicate false, so
//     Postgres silently updates zero rows. Exec returns no error for a
//     zero-row UPDATE, so the flush reports success and the balance never
//     reaches the database. On the next restart loadFromDB reads the stale row.
//
//   - chain_config.wal_recovery_floor_seq, written by
//     markWALSupersededByStateReplacement (keeper/transfer_wal.go:975) and read
//     by recoverFromWAL, which skips every record with `entry.Seq <= floor`
//     (keeper/transfer_wal.go:1041). A floor recorded at an old, high head
//     silently discards every record of a restarted, low-numbered sequence —
//     i.e. crash recovery drops exactly the unflushed transfers it exists to
//     recover.
//
// Neither consumer can detect the reuse: both treat the number as globally
// monotonic, which is what the invariant above promises.

// TestWAL_SeqNotReusedAfterFullCompaction was INTENTIONALLY RED; it is the
// regression guard for the fix that followed (Open now takes the higher of the
// scanned last sequence and a high-water mark that TruncateBefore persists
// before the compacted file goes live — see wal.go's own FIX comments).
//
// TruncateBefore is this package's own compaction primitive and is the
// documented next step before any real deployment (see transfer_wal.go's
// top-of-file scope note: compaction "is the single largest gap before this
// could ever be considered for staging"). Compacting past the head empties the
// file; Open then re-derives nextSeq by scanning that empty file and restarts
// at 1, handing out sequence numbers this log has already issued.
//
// The demand this test makes is fair and in-package: TruncateBefore is a WAL
// operation, the WAL remains present and openable after it, and the "never
// reused" guarantee is this package's own.
func TestWAL_SeqNotReusedAfterFullCompaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compact.wal")

	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var highest uint64
	for i := 0; i < 5; i++ {
		seq, err := w.Append([]byte("record"))
		if err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
		highest = seq
	}
	if highest != 5 {
		t.Fatalf("premise broken: expected the fifth Append to return seq 5, got %d", highest)
	}

	// Everything is durably reconciled elsewhere — the exact precondition
	// TruncateBefore documents for dropping records.
	if err := w.TruncateBefore(highest + 1); err != nil {
		t.Fatalf("TruncateBefore: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Restart.
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { reopened.Close() })

	next, err := reopened.Append([]byte("after restart"))
	if err != nil {
		t.Fatalf("Append after reopen: %v", err)
	}
	if next <= highest {
		t.Errorf("sequence number %d was reused: this log already issued up to %d before compaction.\n"+
			"  wal.go:235-238 states sequence numbers are \"never reused\", and two persistent keeper\n"+
			"  consumers rely on that across restarts — chain_accounts.wal_seq's monotonic UPSERT guard\n"+
			"  (keeper/transfer_wal.go:832), which silently updates zero rows for a stale seq, and\n"+
			"  chain_config.wal_recovery_floor_seq (keeper/transfer_wal.go:1041), which silently skips\n"+
			"  every record at or below a floor recorded from the pre-compaction head.\n"+
			"  Open re-derives nextSeq from the file alone (wal.go:128-145), so a fully compacted log\n"+
			"  restarts at 1.", next, highest)
	}
}

// TestWAL_SeqRestartsAtOneWhenFileIsLost PINS current behaviour rather than
// demanding a different one: with the sequence counter living only in the file,
// losing the file necessarily restarts numbering, and no in-package fix is
// possible. PASS is expected.
//
// It is here because it is the fact that makes the finding above reachable
// without compaction ever being enabled. The WAL lives on a host volume chosen
// specifically to survive container recreation (see
// markWALSupersededByStateReplacement's incident note), so an operator deleting
// or rotating aequitas_transfers.wal — a routine response to a full disk — puts
// the node in exactly this state while chain_accounts.wal_seq and
// chain_config.wal_recovery_floor_seq keep their old, higher values in Postgres.
// The fix belongs in the keeper: the floor and the per-account marker must be
// invalidated whenever the WAL's identity changes, not just when account state
// is replaced.
func TestWAL_SeqRestartsAtOneWhenFileIsLost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lost.wal")

	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := w.Append([]byte("record")); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}
	if head := w.HeadSeq(); head != 3 {
		t.Fatalf("premise broken: HeadSeq = %d, want 3", head)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// The operator (or a wiped volume) removes the file.
	if err := os.Remove(path); err != nil {
		t.Fatalf("removing WAL file: %v", err)
	}

	fresh, err := Open(path)
	if err != nil {
		t.Fatalf("Open after loss: %v", err)
	}
	t.Cleanup(func() { fresh.Close() })

	seq, err := fresh.Append([]byte("first after loss"))
	if err != nil {
		t.Fatalf("Append after loss: %v", err)
	}
	if seq != 1 {
		t.Fatalf("expected numbering to restart at 1 after the file was lost, got %d — "+
			"if this changed, the keeper-side hazard this test documents may no longer apply", seq)
	}
	t.Logf("confirmed: after WAL file loss the next sequence number is %d, while "+
		"chain_accounts.wal_seq and chain_config.wal_recovery_floor_seq still hold values up to 3 "+
		"in Postgres — every subsequent flush is silently dropped by the monotonic UPSERT guard, and "+
		"every subsequent crash-recovery record is silently skipped by the recovery floor.", seq)
}
