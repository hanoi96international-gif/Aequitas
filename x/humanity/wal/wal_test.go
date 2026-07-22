package wal

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func tempWALPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "test.wal")
}

func TestAppendReplay_RoundTrip(t *testing.T) {
	path := tempWALPath(t)
	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	var seqs []uint64
	for i := 0; i < 100; i++ {
		seq, err := w.Append([]byte(fmt.Sprintf("payload-%d", i)))
		if err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
		seqs = append(seqs, seq)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Sequence numbers must be strictly increasing, starting at 1.
	for i, seq := range seqs {
		if seq != uint64(i+1) {
			t.Fatalf("seq[%d] = %d, want %d", i, seq, i+1)
		}
	}

	var got []Entry
	count, truncated, err := ReplayFile(path, func(e Entry) error {
		got = append(got, e)
		return nil
	})
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	if truncated {
		t.Fatal("expected a clean (non-truncated) file after a clean Close")
	}
	if count != 100 {
		t.Fatalf("replayed %d entries, want 100", count)
	}
	for i, e := range got {
		want := fmt.Sprintf("payload-%d", i)
		if string(e.Payload) != want {
			t.Errorf("entry %d payload = %q, want %q", i, e.Payload, want)
		}
		if e.Seq != uint64(i+1) {
			t.Errorf("entry %d seq = %d, want %d", i, e.Seq, i+1)
		}
	}
}

func TestOpen_ReopenContinuesSequenceNumbering(t *testing.T) {
	path := tempWALPath(t)
	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := w.Append([]byte("first-session")); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	w2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	seq, err := w2.Append([]byte("second-session"))
	if err != nil {
		t.Fatalf("Append after reopen: %v", err)
	}
	if seq != 11 {
		t.Fatalf("first seq after reopen = %d, want 11 (continuing from the 10 already-written records)", seq)
	}
	if err := w2.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	count, truncated, err := ReplayFile(path, func(Entry) error { return nil })
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	if truncated {
		t.Fatal("expected a clean file")
	}
	if count != 11 {
		t.Fatalf("replayed %d entries across both sessions, want 11", count)
	}
}

// TestCrashRecovery_TruncatedTailRecordIsDropped is the core crash-safety
// proof: simulate a process dying mid-write by chopping bytes off the end
// of a valid WAL file (a truncated last record — the exact shape a crash
// mid-fsync produces, since a partial write can never include a complete,
// checksummed record). ReplayFile must recover every record BEFORE the
// truncation point and silently stop there — not error out, not skip
// records inside the still-valid prefix, not fabricate the missing tail.
func TestCrashRecovery_TruncatedTailRecordIsDropped(t *testing.T) {
	path := tempWALPath(t)
	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	const n = 20
	for i := 0; i < n; i++ {
		if _, err := w.Append([]byte(fmt.Sprintf("entry-%d", i))); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	full, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// Chop off the last 5 bytes -- guaranteed to land inside the final
	// record's checksum or payload for any non-trivial payload, simulating
	// a crash after the header was written but before the record fully
	// landed (or before fsync even completed for the whole thing).
	truncatedContent := full[:len(full)-5]
	if err := os.WriteFile(path, truncatedContent, 0o600); err != nil {
		t.Fatalf("WriteFile (simulating truncation): %v", err)
	}

	var recovered []Entry
	count, truncated, err := ReplayFile(path, func(e Entry) error {
		recovered = append(recovered, e)
		return nil
	})
	if err != nil {
		t.Fatalf("ReplayFile on truncated file: %v", err)
	}
	if !truncated {
		t.Fatal("expected ReplayFile to report the tail as truncated")
	}
	if count != n-1 {
		t.Fatalf("recovered %d entries, want exactly %d (every record except the truncated last one)", count, n-1)
	}
	for i, e := range recovered {
		want := fmt.Sprintf("entry-%d", i)
		if string(e.Payload) != want {
			t.Errorf("recovered entry %d = %q, want %q", i, e.Payload, want)
		}
	}
}

// TestCrashRecovery_OpenTruncatesCorruptTailAndResumesCleanly proves the
// OTHER half of crash recovery: not just reading around a corrupt tail
// (ReplayFile), but Open() actively cleaning it up so the writer can safely
// resume appending afterward -- without this, a future Append would land
// its valid new record AFTER leftover garbage bytes, and the next replay
// would incorrectly treat that garbage as a corrupt records in the MIDDLE
// of the log instead of a clean, ignorable pre-recovery artifact.
func TestCrashRecovery_OpenTruncatesCorruptTailAndResumesCleanly(t *testing.T) {
	path := tempWALPath(t)
	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := w.Append([]byte(fmt.Sprintf("pre-crash-%d", i))); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	full, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := os.WriteFile(path, append(full, []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x01, 0x02}...), 0o600); err != nil {
		t.Fatalf("WriteFile (appending garbage): %v", err)
	}

	w2, err := Open(path) // must truncate the garbage internally
	if err != nil {
		t.Fatalf("Open after simulated crash: %v", err)
	}
	seq, err := w2.Append([]byte("post-crash-0"))
	if err != nil {
		t.Fatalf("Append after recovery: %v", err)
	}
	if seq != 6 {
		t.Fatalf("post-recovery seq = %d, want 6 (continuing from the 5 pre-crash records, garbage discarded)", seq)
	}
	if err := w2.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	var got []string
	count, truncated, err := ReplayFile(path, func(e Entry) error {
		got = append(got, string(e.Payload))
		return nil
	})
	if err != nil {
		t.Fatalf("final ReplayFile: %v", err)
	}
	if truncated {
		t.Fatal("expected a clean file after Open's own recovery + a clean Close")
	}
	if count != 6 {
		t.Fatalf("replayed %d entries, want 6 (5 pre-crash + 1 post-recovery)", count)
	}
	for i := 0; i < 5; i++ {
		want := fmt.Sprintf("pre-crash-%d", i)
		if got[i] != want {
			t.Errorf("entry %d = %q, want %q", i, got[i], want)
		}
	}
	if got[5] != "post-crash-0" {
		t.Errorf("entry 5 = %q, want %q", got[5], "post-crash-0")
	}
}

// TestChecksumMismatch_CorruptedMiddleRecordStopsReplayThere proves silent
// bit-rot/corruption in the MIDDLE of an otherwise-complete record (not
// just a truncated tail) is detected via the checksum, not silently
// accepted as valid data -- replay must stop at the first record whose
// checksum doesn't match its payload, exactly as if that were the true end
// of durable history (the safe, conservative choice: never replay data
// that might not be what was actually written).
func TestChecksumMismatch_CorruptedMiddleRecordStopsReplayThere(t *testing.T) {
	path := tempWALPath(t)
	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := w.Append([]byte(fmt.Sprintf("entry-%d", i))); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// Flip a byte inside the THIRD record's payload region. Each record
	// here is headerSize(12) + len("entry-N")(7) + checksumSize(4) = 23
	// bytes; record index 2 (0-based) starts at offset 2*23=46, payload
	// begins right after the 12-byte header.
	const recordSize = headerSize + len("entry-0") + checksumSize
	corruptOffset := 2*recordSize + headerSize + 1
	mutated := append([]byte(nil), content...)
	mutated[corruptOffset] ^= 0xFF
	if err := os.WriteFile(path, mutated, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var recovered []Entry
	count, truncated, err := ReplayFile(path, func(e Entry) error {
		recovered = append(recovered, e)
		return nil
	})
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	if !truncated {
		t.Fatal("expected the corrupted record to be reported as a truncated/invalid tail")
	}
	if count != 2 {
		t.Fatalf("recovered %d entries, want exactly 2 (records before the corrupted one)", count)
	}
	for i, e := range recovered {
		want := fmt.Sprintf("entry-%d", i)
		if string(e.Payload) != want {
			t.Errorf("entry %d = %q, want %q", i, e.Payload, want)
		}
	}
}

func TestTruncateBefore_DropsOnlyOlderRecords(t *testing.T) {
	path := tempWALPath(t)
	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := w.Append([]byte(fmt.Sprintf("entry-%d", i))); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	if err := w.TruncateBefore(6); err != nil {
		t.Fatalf("TruncateBefore: %v", err)
	}

	// More appends after compaction must continue the SAME sequence
	// numbering (compaction must not reset or renumber anything).
	seq, err := w.Append([]byte("entry-after-compaction"))
	if err != nil {
		t.Fatalf("Append after compaction: %v", err)
	}
	if seq != 11 {
		t.Fatalf("seq after compaction = %d, want 11", seq)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	var got []Entry
	count, truncated, err := ReplayFile(path, func(e Entry) error {
		got = append(got, e)
		return nil
	})
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	if truncated {
		t.Fatal("expected a clean file")
	}
	// Records with seq 1-5 (entry-0..entry-4) dropped; seq 6-10 (entry-5..entry-9)
	// plus the new seq-11 record remain = 6 records.
	if count != 6 {
		t.Fatalf("replayed %d entries after compaction, want 6", count)
	}
	if got[0].Seq != 6 || string(got[0].Payload) != "entry-5" {
		t.Errorf("first surviving entry = seq %d %q, want seq 6 \"entry-5\"", got[0].Seq, got[0].Payload)
	}
	if got[len(got)-1].Seq != 11 || string(got[len(got)-1].Payload) != "entry-after-compaction" {
		t.Errorf("last entry = seq %d %q, want seq 11 \"entry-after-compaction\"", got[len(got)-1].Seq, got[len(got)-1].Payload)
	}
}

// TestConcurrentAppend_NoLostOrDuplicateSequenceNumbers is the group-commit
// correctness proof under -race: many goroutines appending concurrently
// must all get a UNIQUE sequence number (no two callers ever receive the
// same seq, whether or not their Append calls landed in the same
// group-commit batch), and every single payload must be durably present on
// replay -- proving the shared-buffer batching in writeBatch doesn't lose,
// duplicate, or misattribute any concurrent caller's record.
func TestConcurrentAppend_NoLostOrDuplicateSequenceNumbers(t *testing.T) {
	path := tempWALPath(t)
	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	const n = 2000
	var wg sync.WaitGroup
	seqs := make([]uint64, n)
	errsCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			seq, err := w.Append([]byte(fmt.Sprintf("concurrent-%d", idx)))
			if err != nil {
				errsCh <- err
				return
			}
			seqs[idx] = seq
		}(i)
	}
	wg.Wait()
	close(errsCh)
	for err := range errsCh {
		t.Fatalf("Append error: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	seen := make(map[uint64]bool, n)
	for i, seq := range seqs {
		if seq == 0 {
			t.Fatalf("goroutine %d never received a sequence number", i)
		}
		if seen[seq] {
			t.Fatalf("duplicate sequence number %d assigned to more than one Append call", seq)
		}
		seen[seq] = true
	}

	count, truncated, err := ReplayFile(path, func(Entry) error { return nil })
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	if truncated {
		t.Fatal("expected a clean file")
	}
	if count != n {
		t.Fatalf("replayed %d entries, want %d -- a concurrent Append was lost", count, n)
	}
}

func TestReplayFile_NonexistentFileIsEmptyNotError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.wal")
	count, truncated, err := ReplayFile(path, func(Entry) error { return nil })
	if err != nil {
		t.Fatalf("ReplayFile on nonexistent file: %v", err)
	}
	if truncated {
		t.Fatal("nonexistent file should not report truncated")
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

func TestOpen_EmptyFileStartsAtSeqOne(t *testing.T) {
	path := tempWALPath(t)
	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	seq, err := w.Append([]byte("first"))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if seq != 1 {
		t.Fatalf("first seq on an empty WAL = %d, want 1", seq)
	}
	w.Close()
}

func TestClose_DoubleCloseReturnsErrorNotPanic(t *testing.T) {
	path := tempWALPath(t)
	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := w.Close(); err == nil {
		t.Fatal("expected an error on double Close, got nil")
	}
}

func TestAppend_AfterCloseReturnsError(t *testing.T) {
	path := tempWALPath(t)
	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := w.Append([]byte("too late")); err == nil {
		t.Fatal("expected Append after Close to return an error")
	}
}

// TestReplayFile_CallbackErrorStopsAndPropagates proves a failure in the
// CALLER's own apply logic (fn) is reported distinctly from file corruption
// -- both stop replay, but only file corruption should be silently treated
// as "that's just where crash-recovery ends"; a callback error is a real
// failure the caller needs to see.
func TestReplayFile_CallbackErrorStopsAndPropagates(t *testing.T) {
	path := tempWALPath(t)
	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := w.Append([]byte(fmt.Sprintf("entry-%d", i))); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	wantErr := fmt.Errorf("simulated apply failure")
	seen := 0
	_, _, err = ReplayFile(path, func(e Entry) error {
		seen++
		if seen == 3 {
			return wantErr
		}
		return nil
	})
	if err == nil {
		t.Fatal("expected ReplayFile to propagate the callback's error")
	}
	if seen != 3 {
		t.Fatalf("callback invoked %d times, want exactly 3 (stops immediately on error)", seen)
	}
}
