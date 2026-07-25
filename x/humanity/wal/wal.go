// Package wal implements a local, sequential, append-only write-ahead log
// with group commit and crash-safe replay — the primitive
// SCALING_ARCHITECTURE.md Phase 7 (state primarily in RAM, Postgres becomes
// an asynchronous durability/reporting log instead of the synchronous write
// path) is built on.
//
// This package is deliberately standalone and business-logic-agnostic: it
// knows nothing about ChainState, AccountState, or Transaction. It stores
// and replays opaque byte payloads in order, durably. Wiring it in as
// ChainState's actual source of truth (thread a per-operation durability
// point through dbExec/saveAccountToDB, replace cs.activeTx's single-
// transaction model, decide the exact payload encoding for a state
// mutation, and integrate crash recovery with cs.accounts/cs.pool) is a
// deliberately SEPARATE, much larger step — see this package's own doc
// comment in SCALING_ARCHITECTURE.md's Phase 7 section for why that
// integration needs its own dedicated project phase with staging
// validation, not a same-session follow-on to this primitive.
package wal

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sync"
	"time"
)

// Record framing, one entry on disk:
//
//	[8 bytes big-endian seq][4 bytes big-endian length N][N bytes payload][4 bytes big-endian crc32(payload)]
//
// The length prefix lets a reader know exactly how many payload bytes to
// expect; the trailing checksum lets a reader detect a record whose bytes
// were corrupted or only partially written (e.g. the process crashed mid-
// fsync) — see Replay's own comment for exactly how that's used to
// establish where "durable" history actually ends.
const (
	seqSize      = 8
	lengthSize   = 4
	checksumSize = 4
	headerSize   = seqSize + lengthSize
)

// Entry is one successfully replayed WAL record.
type Entry struct {
	Seq     uint64
	Payload []byte
}

// appendRequest is one caller's pending Append call, queued for the
// background writer goroutine to fold into a shared fsync — the exact same
// group-commit shape as this codebase's transferBatchCh/runTransferBatcher
// (x/humanity/keeper/state.go), one level lower: a local append-only file
// instead of a full relational transaction.
type appendRequest struct {
	payload []byte
	result  chan appendResult
}

type appendResult struct {
	seq uint64
	err error
}

// WAL is a single append-only log file plus the group-commit machinery that
// batches concurrent Append calls into shared fsyncs. Safe for concurrent
// use from multiple goroutines.
type WAL struct {
	mu      sync.Mutex // guards file/nextSeq/closed against Append's own writer goroutine and TruncateBefore
	path    string
	file    *os.File
	nextSeq uint64
	closed  bool

	appendCh   chan *appendRequest
	writerDone chan struct{}
}

// MaxBatchSize caps how many pending Append calls one fsync can bundle —
// same rationale as transferBatchMaxSize (state.go): bounds how long any
// single fsync's batch grows, keeping worst-case latency for an unlucky
// caller predictable.
const MaxBatchSize = 500

// MaxBatchWait is the group-commit window: how long the writer goroutine
// waits for more Append calls to arrive before fsyncing whatever it already
// has. Same tradeoff as transferBatchMaxWait (state.go): short enough that
// one isolated Append still returns fast, long enough that a real
// concurrent burst shares one fsync instead of paying for one each.
//
// FIX (2026-07-23, 50k-TPS-goal investigation): lowered from 3ms after
// measuring the actual effect on a 100-concurrent-caller benchmark. Every
// Append call blocks its caller until its batch's fsync returns -- callers
// here are themselves sequential per-goroutine loops (each one issues its
// next Append only after the previous one returns), so a LONGER wait
// doesn't just risk bigger batches, it directly adds to every caller's
// per-call latency, which throttles how fast NEW calls can even arrive to
// join a batch in the first place. Measured live: 3ms -> ~3100-3700 TPS,
// 8ms -> ~2000-2200 TPS (worse -- confirms the added-latency cost
// dominates for this workload), 1ms -> ~5500-5900 TPS. 300-500us measured
// similarly to 1ms (within run-to-run noise in this sandbox); 1ms kept as
// a less aggressive, still-real improvement rather than chasing sandbox-
// specific noise down to the microsecond. Re-measure if real staging
// hardware's fsync latency turns out meaningfully different from this
// sandbox's.
const MaxBatchWait = 1 * time.Millisecond

// Open opens path for appending, creating it if it does not exist, and
// scans any existing content to determine the next sequence number —
// reopening a WAL that already has entries (e.g. after a restart) continues
// numbering from where it left off rather than restarting at 1. If the
// existing file has a corrupt or truncated tail record (see Replay's
// comment), Open truncates the file to the last valid record boundary
// before resuming appends — the crash-safe recovery behavior a real
// restart needs: a partially-written record was never acknowledged as
// durable to any caller, so it must not linger as garbage in the middle of
// future reads.
func Open(path string) (*WAL, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("wal: could not open %s: %w", path, err)
	}

	lastSeq, validEnd, _, err := scan(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("wal: could not scan %s: %w", path, err)
	}
	if err := f.Truncate(validEnd); err != nil {
		f.Close()
		return nil, fmt.Errorf("wal: could not truncate %s to last valid record: %w", path, err)
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		f.Close()
		return nil, fmt.Errorf("wal: could not seek %s to end: %w", path, err)
	}

	w := &WAL{
		path:       path,
		file:       f,
		nextSeq:    lastSeq + 1,
		appendCh:   make(chan *appendRequest, MaxBatchSize*8),
		writerDone: make(chan struct{}),
	}
	go w.runWriter()
	return w, nil
}

// Append durably appends payload and returns its sequence number. Blocks
// until the record (and every other record in the same group-commit batch)
// has been written and fsynced — the payload is guaranteed durable (will
// survive a crash immediately after this call returns) if and only if err
// is nil.
func (w *WAL) Append(payload []byte) (uint64, error) {
	req := &appendRequest{payload: payload, result: make(chan appendResult, 1)}
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return 0, errors.New("wal: append on closed WAL")
	}
	w.mu.Unlock()
	w.appendCh <- req
	res := <-req.result
	return res.seq, res.err
}

// runWriter is the group-commit loop: block for the first pending Append,
// then greedily collect more (up to MaxBatchSize) for up to MaxBatchWait
// before writing the whole batch and fsyncing once. Mirrors
// runTransferBatcher's exact structure (state.go).
func (w *WAL) runWriter() {
	defer close(w.writerDone)
	for first := range w.appendCh {
		batch := []*appendRequest{first}
		timer := time.NewTimer(MaxBatchWait)
	collect:
		for len(batch) < MaxBatchSize {
			select {
			case req, ok := <-w.appendCh:
				if !ok {
					break collect
				}
				batch = append(batch, req)
			case <-timer.C:
				break collect
			}
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		w.writeBatch(batch)
	}
}

// writeBatch performs the actual file I/O for one group-commit batch: every
// record is written, then Sync() is called exactly once for the whole
// batch, then every request is notified. If the write or sync fails
// partway, every request in the batch reports the same error — group
// commit's fundamental tradeoff (shared fate for a shared fsync) applies
// here exactly as it does for the DB-transaction group commit this mirrors.
func (w *WAL) writeBatch(batch []*appendRequest) {
	w.mu.Lock()
	defer w.mu.Unlock()

	assigned := make([]uint64, len(batch))
	buf := make([]byte, 0, 256*len(batch))
	for i, req := range batch {
		seq := w.nextSeq
		w.nextSeq++
		assigned[i] = seq
		buf = appendRecord(buf, seq, req.payload)
	}

	_, writeErr := w.file.Write(buf)
	var syncErr error
	if writeErr == nil {
		syncErr = w.file.Sync()
	}
	err := writeErr
	if err == nil {
		err = syncErr
	}
	if err != nil {
		// FIX-equivalent to this codebase's own "never report success for a
		// write that didn't durably land" rule (see saveAccountToDB's own
		// comment in keeper/state.go for the exact same principle applied
		// to Postgres writes): nextSeq is NOT rolled back on failure —
		// sequence numbers are only ever a monotonic ordering label, never
		// reused, so a failed batch simply burns some seq values rather
		// than risk a future successful Append silently colliding with one
		// of THIS batch's un-persisted, already-assigned numbers.
		err = fmt.Errorf("wal: batch write/sync failed: %w", err)
		for _, req := range batch {
			req.result <- appendResult{err: err}
		}
		return
	}
	for i, req := range batch {
		req.result <- appendResult{seq: assigned[i]}
	}
}

// HeadSeq returns the highest sequence number this WAL has assigned so far
// (0 on a brand-new, empty log). Used by the keeper to record a recovery
// FLOOR when the account state this WAL reconciles into is replaced wholesale
// from a trusted snapshot — see ChainState.markWALSupersededByStateReplacement
// for the live corruption incident that made this necessary.
func (w *WAL) HeadSeq() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.nextSeq == 0 {
		return 0
	}
	return w.nextSeq - 1
}

// Close stops accepting new Append calls, waits for any in-flight batch to
// finish, and closes the underlying file. Safe to call once; a second call
// returns an error rather than panicking on a double-close.
func (w *WAL) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return errors.New("wal: already closed")
	}
	w.closed = true
	w.mu.Unlock()

	close(w.appendCh)
	<-w.writerDone
	return w.file.Close()
}

// TruncateBefore rewrites the WAL to drop every record with Seq < before —
// compaction, for once the caller (during real integration) has confirmed
// everything up to that point is durably reflected elsewhere (e.g.
// Postgres) and no longer needs replaying on a future crash recovery. Safe,
// simple, correctness-first implementation: reads every still-wanted
// record, writes them to a fresh temp file, fsyncs it, then atomically
// renames it over the original — never leaves the log in a half-rewritten
// state even if the process dies mid-compaction (the temp file is either
// never renamed, in which case the original is untouched, or fully renamed
// after its own fsync, in which case it's fully valid). Not yet segment-
// based (an O(1)-per-compaction design a high-frequency real deployment
// would eventually want) — this priority (crash-safety proven first, raw
// throughput tuned later once real integration exposes real numbers to
// tune against) matches how every other phase in this project was built.
func (w *WAL) TruncateBefore(before uint64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("wal: TruncateBefore on closed WAL")
	}

	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("wal: could not seek for compaction: %w", err)
	}
	tmpPath := w.path + ".compact.tmp"
	tmp, err := os.OpenFile(tmpPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("wal: could not create compaction temp file: %w", err)
	}
	r := bufio.NewReader(w.file)
	for {
		entry, ok, err := readRecord(r)
		if err != nil {
			tmp.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("wal: could not read record during compaction: %w", err)
		}
		if !ok {
			break
		}
		if entry.Seq < before {
			continue
		}
		if _, err := tmp.Write(appendRecord(nil, entry.Seq, entry.Payload)); err != nil {
			tmp.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("wal: could not write compacted record: %w", err)
		}
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("wal: could not sync compacted file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("wal: could not close compacted file: %w", err)
	}
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("wal: could not close original file before rename: %w", err)
	}
	if err := os.Rename(tmpPath, w.path); err != nil {
		return fmt.Errorf("wal: could not rename compacted file into place: %w", err)
	}
	f, err := os.OpenFile(w.path, os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("wal: could not reopen compacted file: %w", err)
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		f.Close()
		return fmt.Errorf("wal: could not seek reopened file to end: %w", err)
	}
	w.file = f
	return nil
}

// appendRecord appends one framed record (see the framing comment above)
// to dst and returns the extended slice.
func appendRecord(dst []byte, seq uint64, payload []byte) []byte {
	var header [headerSize]byte
	binary.BigEndian.PutUint64(header[:seqSize], seq)
	binary.BigEndian.PutUint32(header[seqSize:], uint32(len(payload)))
	dst = append(dst, header[:]...)
	dst = append(dst, payload...)
	var crc [checksumSize]byte
	binary.BigEndian.PutUint32(crc[:], crc32.ChecksumIEEE(payload))
	dst = append(dst, crc[:]...)
	return dst
}

// readRecord reads exactly one record from r. ok=false with a nil error
// means a clean EOF at a record boundary (nothing corrupt, just the end of
// the log). A non-nil error means the record header/payload/checksum was
// short-read or the checksum didn't match — i.e. a corrupt or partially-
// written (crashed mid-write) final record.
func readRecord(r *bufio.Reader) (Entry, bool, error) {
	var header [headerSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		if errors.Is(err, io.EOF) {
			return Entry{}, false, nil
		}
		return Entry{}, false, fmt.Errorf("short read on record header: %w", err)
	}
	seq := binary.BigEndian.Uint64(header[:seqSize])
	length := binary.BigEndian.Uint32(header[seqSize:])

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return Entry{}, false, fmt.Errorf("short read on %d-byte payload for seq %d: %w", length, seq, err)
	}
	var crcBytes [checksumSize]byte
	if _, err := io.ReadFull(r, crcBytes[:]); err != nil {
		return Entry{}, false, fmt.Errorf("short read on checksum for seq %d: %w", seq, err)
	}
	want := binary.BigEndian.Uint32(crcBytes[:])
	got := crc32.ChecksumIEEE(payload)
	if want != got {
		return Entry{}, false, fmt.Errorf("checksum mismatch for seq %d: file has %x, computed %x", seq, want, got)
	}
	return Entry{Seq: seq, Payload: payload}, true, nil
}

// scan reads f from the start and returns the highest sequence number seen,
// the byte offset immediately after the last fully-valid record (safe to
// truncate to), and how many valid records were found. Never returns a
// non-nil error for a corrupt/truncated tail — that is the exact, expected
// shape of "the process crashed mid-append" and is handled by simply
// stopping there, not treated as a fatal error opening the file.
func scan(f *os.File) (lastSeq uint64, validEnd int64, count int, err error) {
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return 0, 0, 0, err
	}
	r := bufio.NewReader(f)
	var offset int64
	for {
		entry, ok, recErr := readRecord(r)
		if recErr != nil {
			// Corrupt/truncated tail: everything up to `offset` is valid
			// and durable; stop here, as if the file ended cleanly at this
			// point (see Open's comment — the caller truncates to here).
			break
		}
		if !ok {
			break
		}
		offset += int64(headerSize) + int64(len(entry.Payload)) + int64(checksumSize)
		lastSeq = entry.Seq
		count++
	}
	return lastSeq, offset, count, nil
}

// ReplayFile sequentially reads every valid record in path and calls fn for
// each, in order, stopping at the first corrupt/truncated record (if any)
// exactly like scan does — everything before that point is durable history;
// nothing after it was ever successfully acknowledged to a caller. Intended
// for startup recovery, BEFORE calling Open for the writer: replay first to
// rebuild in-memory state, then Open to resume appending (Open's own scan
// truncates any corrupt tail so future appends never land after garbage).
//
// Returns the count of valid records replayed and whether the file's tail
// was corrupt/truncated (true is the normal, expected shape of "the
// process crashed after the last confirmed record" — not itself an error).
// fn returning an error stops replay immediately and that error is
// returned, distinguishing "the WAL itself is fine but the caller's own
// apply logic failed" from "the file was corrupt."
func ReplayFile(path string, fn func(Entry) error) (count int, truncated bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("wal: could not open %s for replay: %w", path, err)
	}
	defer f.Close()

	r := bufio.NewReader(f)
	for {
		entry, ok, recErr := readRecord(r)
		if recErr != nil {
			return count, true, nil
		}
		if !ok {
			return count, false, nil
		}
		if err := fn(entry); err != nil {
			return count, false, fmt.Errorf("wal: replay callback failed at seq %d: %w", entry.Seq, err)
		}
		count++
	}
}
