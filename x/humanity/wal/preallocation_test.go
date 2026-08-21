package wal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Preallocation is what makes fdatasync cheap, and it is only safe because
// every reader stops at the padding.
//
// MEASURED on the production filesystem, four shapes back to back:
//
//	fsync,     appending      352/s   p50 1970us  p99 14696us
//	fdatasync, appending      355/s   p50 1990us  p99 13321us
//	fsync,     preallocated   470/s   p50 1803us  p99  5571us
//	fdatasync, preallocated  1429/s   p50  559us  p99  2060us
//
// Four times the sync rate. Neither half helps alone, which is why the pair
// has to stay together.
//
// THE HAZARD IT CREATES. Padding is zero bytes, and zero bytes parse as a
// perfectly valid record: seq 0, length 0, and crc32 of an empty payload is
// zero, so the CHECKSUM MATCHES. Without a guard, every reader would walk the
// entire preallocated tail as an endless run of valid empty records --
// reporting the high-water mark of a live log as 0, replaying thousands of
// empty entries, or copying megabytes of them into a compacted file.
//
// Sequence numbers start at 1, so seq 0 can only ever be padding. These pin
// that every reader honours it.

func writeSome(t *testing.T, path string, n int) {
	t.Helper()
	w, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for i := 0; i < n; i++ {
		if _, err := w.Append([]byte(fmt.Sprintf(`{"i":%d}`, i))); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestCleanCloseLeavesNoPadding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.wal")
	writeSome(t, path, 20)

	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// 20 small records are a few hundred bytes. If the padding survived, this
	// would be at least preallocChunk.
	if st.Size() > 1<<20 {
		t.Fatalf("file is %d bytes after a clean close — the preallocated tail was not trimmed. "+
			"Every external reader (ls, du, a backup) then sees a file that is almost entirely "+
			"padding.", st.Size())
	}

	count, truncated, err := ReplayFile(path, func(Entry) error { return nil })
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if truncated {
		t.Error("a cleanly closed WAL was reported as truncated")
	}
	if count != 20 {
		t.Fatalf("replayed %d records, wrote 20", count)
	}
}

// The case a clean Close does NOT cover: the process died, so the padding is
// still there. Every reader has to treat it as the end of the log.
func TestPaddingLeftByACrashIsNotReadAsData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "w.wal")
	writeSome(t, path, 10)

	// Simulate the crash: append padding that a dead process would have left
	// behind, exactly as preallocate would.
	f, err := os.OpenFile(path, os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	end, _ := f.Seek(0, os.SEEK_END)
	if err := preallocate(f, end, 1<<20); err != nil {
		t.Fatalf("preallocate: %v", err)
	}
	f.Close()

	count, truncated, err := ReplayFile(path, func(e Entry) error {
		if e.Seq == 0 {
			t.Error("a padding record reached the replay callback — zero bytes pass the checksum, " +
				"so nothing downstream can tell them from data")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if truncated {
		t.Error("padding was reported as a truncated tail. It is the normal end of a preallocated " +
			"log, not damage, and reporting it as damage would cry wolf on every crash recovery")
	}
	if count != 10 {
		t.Fatalf("replayed %d records, want the 10 real ones (the rest is padding)", count)
	}
}

// Reopening after a crash must find the real high-water mark, not zero.
func TestOpenAfterCrashPaddingKeepsSequenceNumbering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.wal")
	writeSome(t, path, 5)

	f, err := os.OpenFile(path, os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	end, _ := f.Seek(0, os.SEEK_END)
	if err := preallocate(f, end, 1<<20); err != nil {
		t.Fatal(err)
	}
	f.Close()

	w, err := Open(path)
	if err != nil {
		t.Fatalf("open over padding: %v", err)
	}
	defer w.Close()
	seq, err := w.Append([]byte(`{"after":"crash"}`))
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if seq != 6 {
		t.Fatalf("next sequence is %d, want 6.\n"+
			"  Reading padding as data would leave the high-water mark at 0 and hand out "+
			"sequence numbers that already exist — the failure class commit 8a49c38 records.", seq)
	}
}

// Compaction reads the live file, so it meets the padding too.
func TestCompactionStopsAtPadding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.wal")
	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		if _, err := w.Append([]byte(fmt.Sprintf(`{"i":%d}`, i))); err != nil {
			t.Fatal(err)
		}
	}
	// The live file now carries a preallocated tail.
	if err := w.TruncateBefore(20); err != nil {
		t.Fatalf("compaction: %v", err)
	}
	// Writing must continue correctly against the recompacted file.
	seq, err := w.Append([]byte(`{"after":"compaction"}`))
	if err != nil {
		t.Fatalf("append after compaction: %v", err)
	}
	if seq != 31 {
		t.Errorf("next sequence after compaction is %d, want 31", seq)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	count, truncated, err := ReplayFile(path, func(e Entry) error {
		if e.Seq == 0 {
			t.Error("compaction copied padding into the compacted file")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("replay after compaction: %v", err)
	}
	if truncated {
		t.Error("the compacted file was reported as truncated")
	}
	// Sequence numbers run 1..30, so keeping seq >= 20 keeps 20..30 — eleven
	// records — plus the one appended after compaction.
	if count != 12 {
		t.Fatalf("compacted file holds %d records, want 12 (seq 20-30 plus the new one)", count)
	}
}
