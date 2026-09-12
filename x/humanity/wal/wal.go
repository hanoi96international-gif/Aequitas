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
	// mu guards the WRITER's state: file, nextSeq, writeOff, allocEnd. It is
	// held by writeBatch for the whole write+sync, and by TruncateBefore for
	// the whole compaction.
	mu      sync.Mutex
	path    string
	file    *os.File
	nextSeq uint64

	// closeMu guards `closed` AND the send on appendCh. Deliberately NOT mu.
	//
	// Append used to take mu just to read `closed` -- the same mutex writeBatch
	// holds across its write and fsync. So while the writer was syncing, no
	// arriving Append could even ENQUEUE: they piled up on the mutex instead of
	// joining the batch being formed. That makes batch size equal to
	// arrival-rate times hold-time, and therefore
	//
	//	throughput = batch / hold = arrival rate
	//
	// self-consistent at ANY sync speed. It is why avg_batch sat at ~48 no
	// matter what was tuned, and why halving the fsync (preallocation plus
	// fdatasync, measured 14.4ms -> 7.1ms) moved throughput by 13% instead of
	// doubling it. Everything upstream was measured and innocent: CPU at one
	// core of six, GC at 0.15% of wall time, and seven other hypotheses
	// rejected.
	//
	// A read lock also makes Close correct rather than merely lucky: the send
	// happens under RLock and Close takes the write lock before closing the
	// channel, so a sender can no longer be mid-send when the channel closes --
	// which was a live panic window, not a new one.
	closeMu sync.RWMutex
	closed  bool

	// writeOff is where the next record goes; allocEnd is how far the file is
	// preallocated. Records are written AT AN OFFSET inside an already-sized
	// file rather than appended, so the file size does not change per batch and
	// fdatasync has no metadata to persist. See datasync_linux.go for the
	// measurement that made this worth doing (4x the sync rate, a seventh of
	// the p99) and why neither half of it works alone.
	//
	// Both are guarded by mu, like file and nextSeq, and only the single writer
	// goroutine advances writeOff.
	writeOff int64
	allocEnd int64

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

	// FIX (audit 2026-08-16, finding WAL-SEQ): the file alone is not a
	// sufficient source for the next sequence number. TruncateBefore can
	// legitimately remove EVERY record, and scan then reports lastSeq 0, so
	// numbering would restart at 1 and hand out values this log already
	// issued — breaking the "never reused" guarantee writeBatch states and two
	// persistent keeper consumers rely on across restarts
	// (chain_accounts.wal_seq's monotonic UPSERT guard, which silently updates
	// zero rows for a stale seq, and chain_config.wal_recovery_floor_seq,
	// which silently skips every record at or below the floor). The
	// high-water mark written by TruncateBefore survives compaction and is
	// consulted here; whichever source is higher wins.
	//
	// Losing the WAL file itself still restarts numbering — no in-package fix
	// exists for that, and it is pinned deliberately by
	// TestWAL_SeqRestartsAtOneWhenFileIsLost. That case must be handled
	// keeper-side by invalidating the floor and the per-account marker when
	// the WAL's identity changes.
	hwm, err := readSeqHighWaterMark(path)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("wal: could not read sequence high-water mark for %s: %w", path, err)
	}
	nextSeq := lastSeq + 1
	if hwm+1 > nextSeq {
		nextSeq = hwm + 1
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
		path:    path,
		file:    f,
		nextSeq: nextSeq,
		// Both start at the end of the valid data Open just truncated to.
		// Preallocation happens lazily on the first write, so opening a WAL
		// costs nothing extra and a node that never writes never grows a file.
		writeOff:   validEnd,
		allocEnd:   validEnd,
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
	// Send while holding the READ lock, so many callers enqueue concurrently
	// and Close cannot close the channel underneath one of them. Crucially
	// this does not touch mu, so a batch being written no longer blocks the
	// next batch from forming -- see closeMu's comment for why that was the
	// throughput ceiling.
	w.closeMu.RLock()
	if w.closed {
		w.closeMu.RUnlock()
		return 0, errors.New("wal: append on closed WAL")
	}
	w.appendCh <- req
	w.closeMu.RUnlock()

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
		// Ueber die Umgebung einstellbar, Vorgabe unveraendert -- siehe
		// batch_tuning.go fuer die Messung, die das noetig macht.
		timer := time.NewTimer(batchWait())
	collect:
		for len(batch) < batchSize() {
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

	// WriteAt into an already-sized file, then fdatasync. Appending would grow
	// the file on every batch, and a size change forces the kernel to journal
	// the inode -- which measured four times more expensive than this on the
	// production filesystem. See datasync_linux.go.
	writeStart := time.Now()
	writeErr := w.ensureCapacity(int64(len(buf)))
	if writeErr == nil {
		_, writeErr = w.file.WriteAt(buf, w.writeOff)
	}
	writeDur := time.Since(writeStart)
	var syncErr error
	var syncDur time.Duration
	if writeErr == nil {
		syncStart := time.Now()
		syncErr = datasync(w.file)
		syncDur = time.Since(syncStart)
	}
	// Recorded for every batch, successful or not: a batch that failed still
	// cost the time, and hiding failures would make the average look better
	// than the disk is. See writer_stats.go for why avg_batch is the number
	// that decides where the ceiling is.
	noteWriterBatch(len(batch), writeDur, syncDur)
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
	// Only now, and only on success. A batch that failed leaves writeOff where
	// it was, so the next one overwrites whatever partial bytes reached the
	// file -- which is right: those records are not durable, scan stops at the
	// first one that does not parse, and their sequence numbers stay burned
	// exactly as the failure path above describes.
	w.writeOff += int64(len(buf))

	for i, req := range batch {
		req.result <- appendResult{seq: assigned[i]}
	}
}

// preallocChunk is how much space one extension reserves.
//
// 64 MB is roughly 260,000 records at the ~250 bytes a transfer record takes,
// so the one full fsync an extension costs is amortised to nothing. Smaller
// would reintroduce the metadata write this exists to avoid; larger would make
// a fresh WAL claim more disk than a quiet node ever uses.
const preallocChunk = 64 << 20

// ensureCapacity grows the preallocated region so that n more bytes fit
// without the file's size changing.
//
// Caller holds w.mu.
func (w *WAL) ensureCapacity(n int64) error {
	if w.writeOff+n <= w.allocEnd {
		return nil
	}
	end := w.allocEnd
	if end < w.writeOff {
		// Defensive: a WAL opened on a file shorter than its own write offset
		// should extend from the offset, never leave a hole.
		end = w.writeOff
	}
	grow := int64(preallocChunk)
	if need := w.writeOff + n - end; need > grow {
		grow = need
	}
	if err := preallocate(w.file, end, grow); err != nil {
		return fmt.Errorf("wal: preallocate %d bytes at offset %d: %w", grow, end, err)
	}
	// A FULL fsync here, once per chunk rather than once per batch: this is
	// the only moment the file's size changes, so it is the only moment the
	// metadata has to be made durable. Every write until the next extension
	// then needs data only.
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("wal: sync after preallocating to %d: %w", end+grow, err)
	}
	w.allocEnd = end + grow
	return nil
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
	// Taking the WRITE lock waits for every in-flight Append to finish its
	// send, and once closed is set no new one can start. Only then is closing
	// the channel safe -- the previous shape released the lock before the
	// senders had sent, leaving a real window where close() raced a send and
	// panicked.
	w.closeMu.Lock()
	if w.closed {
		w.closeMu.Unlock()
		return errors.New("wal: already closed")
	}
	w.closed = true
	w.closeMu.Unlock()

	close(w.appendCh)
	<-w.writerDone

	// Trim the preallocated tail so a cleanly closed WAL is exactly its data.
	// Open would truncate it anyway on the next start, but leaving 64 MB of
	// zeros behind makes every external reader -- ls, du, a backup, a human --
	// see a file that is mostly padding, and makes ReplayFile depend on the
	// seq-0 guard rather than simply not encountering one.
	//
	// Failure here is not worth failing Close over: the padding is harmless
	// and self-correcting, so the close still proceeds.
	if err := w.file.Truncate(w.writeOff); err != nil {
		fmt.Printf("[WAL] could not trim the preallocated tail on close: %v (harmless, the next Open truncates it)\n", err)
	}
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
// TruncateBefore verwirft alle Datensaetze mit Seq < before.
//
// OHNE DIE SPERRE ZU HALTEN, WAEHREND KOPIERT WIRD.
//
// Die erste Fassung nahm w.mu am Anfang und gab sie am Ende zurueck -- und
// dazwischen las sie die ganze Datei, schrieb die Ueberlebenden in eine neue
// und synchronisierte sie. Bei der Ausloeseschwelle von 512 MB sind das
// Sekunden. w.mu ist aber dieselbe Sperre, die writeBatch braucht: solange
// die Kompaktierung lief, kam KEIN Append durch, und jede Ueberweisung des
// Knotens wartete -- unter Last alle paar Minuten ein Annahmestillstand von
// Sekunden, am 12.09.2026 als sync_max von 746 ms und als Ausreisser in den
// Append-Zeiten sichtbar. Ein Einfrieren, das nur die Hoehe nicht anhaelt.
//
// Jetzt in drei Schritten:
//
//  1. Unter w.mu nur MERKEN, wo die gueltigen Daten gerade enden (cutOff).
//  2. Ohne w.mu alles bis cutOff kopieren. Appends laufen derweil weiter und
//     landen bei Offsets >= cutOff; die Kopie liest nur darunter. WriteAt und
//     ein SectionReader beruehren den gemeinsamen Dateizeiger nicht.
//  3. Unter w.mu nachholen, was seit cutOff dazukam (Sekunden mal Schreibrate,
//     also Megabyte, nicht Hunderte), dann tauschen.
//
// Die Sperre wird damit fuer das Nachholen und den Tausch gehalten -- zehn
// Millisekunden statt Sekunden. Kompaktiert wird dieselbe Menge wie vorher.
// KompaktierungsPhasen haelt die Dauer der drei Schritte der letzten
// Kompaktierung fest -- der dritte ist der einzige, der Appends aufhaelt.
var KompaktierungsPhasen struct {
	mu                       sync.Mutex
	Kopie, Nachholen, Tausch time.Duration
	NachgeholtBytes          int64
}

func (w *WAL) TruncateBefore(before uint64) error {
	// Schritt 1: Schnitt merken.
	w.mu.Lock()
	w.closeMu.RLock()
	closed := w.closed
	w.closeMu.RUnlock()
	if closed {
		w.mu.Unlock()
		return errors.New("wal: TruncateBefore on closed WAL")
	}
	cutOff := w.writeOff
	datei := w.file
	w.mu.Unlock()

	// Schritt 2: kopieren, ohne jemanden aufzuhalten.
	tKopie := time.Now()
	tmpPath := w.path + ".compact.tmp"
	tmp, err := os.OpenFile(tmpPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("wal: could not create compaction temp file: %w", err)
	}
	verwerfen := func(err error) error {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	datenEnde, err := kopiereUeberlebende(tmp, 0, io.NewSectionReader(datei, 0, cutOff), before)
	if err != nil {
		return verwerfen(err)
	}
	if err := tmp.Sync(); err != nil {
		return verwerfen(fmt.Errorf("wal: could not sync compacted file: %w", err))
	}
	// Die neue Datei HIER vorbelegen, nicht im ersten Batch nach dem Tausch:
	// dort liefe es unter w.mu, und auf Systemen ohne fallocate ist eine
	// Vorbelegung ein Schreiben von 64 MB Nullen -- im Test 900 ms, in denen
	// jeder Append wartete. Das Nachholen in Schritt 3 schreibt in die
	// vorbelegte Region hinein; datenEnde bleibt der einzige Zeiger auf das
	// Ende der Daten, die Dateigroesse sagt ab jetzt nichts mehr darueber.
	allocEnde := datenEnde + int64(preallocChunk)
	if err := preallocate(tmp, datenEnde, int64(preallocChunk)); err != nil {
		return verwerfen(fmt.Errorf("wal: could not preallocate compacted file: %w", err))
	}
	if err := tmp.Sync(); err != nil {
		return verwerfen(fmt.Errorf("wal: could not sync preallocation: %w", err))
	}

	dKopie := time.Since(tKopie)

	// Schritt 3: nachholen und tauschen -- jetzt unter der Sperre.
	w.mu.Lock()
	defer w.mu.Unlock()
	tNachholen := time.Now()
	nachgeholt := int64(0)
	defer func() {
		KompaktierungsPhasen.mu.Lock()
		KompaktierungsPhasen.Kopie = dKopie
		KompaktierungsPhasen.Nachholen = time.Since(tNachholen)
		KompaktierungsPhasen.NachgeholtBytes = nachgeholt
		KompaktierungsPhasen.mu.Unlock()
	}()
	w.closeMu.RLock()
	closed = w.closed
	w.closeMu.RUnlock()
	if closed {
		return verwerfen(errors.New("wal: WAL closed during compaction"))
	}
	if w.writeOff > cutOff {
		nachgeholt = w.writeOff - cutOff
		datenEnde, err = kopiereUeberlebende(tmp, datenEnde, io.NewSectionReader(w.file, cutOff, w.writeOff-cutOff), before)
		if err != nil {
			return verwerfen(err)
		}
		if err := tmp.Sync(); err != nil {
			return verwerfen(fmt.Errorf("wal: could not sync catch-up records: %w", err))
		}
	}
	if datenEnde > allocEnde {
		allocEnde = datenEnde // das Nachholen hat die Vorbelegung ueberschritten
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("wal: could not close compacted file: %w", err)
	}
	if err := writeSeqHighWaterMark(w.path, w.nextSeq-1); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("wal: could not persist sequence high-water mark: %w", err)
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
	w.file = f
	// NICHT Seek(End): die Datei ist hinter den Daten vorbelegt, ihr Ende
	// sind Nullen. writeOff muss auf das Ende der DATEN zeigen -- sonst
	// schriebe der naechste Batch hinter 64 MB Nullen, und ein Replay, das am
	// ersten Datensatz mit Seq 0 endet, saehe nichts mehr davon. Der Fehler
	// waere still: keine Meldung, nur ein WAL, das ab hier leer scheint.
	w.writeOff = datenEnde
	w.allocEnd = allocEnde
	return nil
}

// kopiereUeberlebende schreibt alle Datensaetze aus r mit Seq >= before nach
// tmp. Endet am Ende von r oder am ersten Datensatz mit Seq 0 (Nullfuellung
// hinter den gueltigen Daten).
//
// IN STUECKEN SYNCHRONISIERT, NICHT EINMAL AM ENDE. Ein einzelner Sync ueber
// hunderte Megabyte haelt die Platte fuer Sekunden -- und mit ihr jeden
// anderen fsync auf demselben Datentraeger, also auch die Gruppen-Commits des
// Schreibers, obwohl der gar keine Sperre mehr wartet. Gemessen im Test: ohne
// dieses Stueckeln wartete der laengste Append 766 von 788 ms Kompaktierung,
// mit Sperre oder ohne. Kleine Syncs lassen die Commits dazwischen.
func kopiereUeberlebende(tmp *os.File, ab int64, r io.Reader, before uint64) (int64, error) {
	const syncStueck = 4 << 20
	br := bufio.NewReader(r)
	seitLetztemSync := 0
	off := ab
	for {
		entry, ok, err := readRecord(br)
		if err != nil {
			return off, fmt.Errorf("wal: could not read record during compaction: %w", err)
		}
		if !ok || entry.Seq == 0 {
			return off, nil
		}
		if entry.Seq < before {
			continue
		}
		// WriteAt an einem eigenen Offset, nicht Write am Dateizeiger: die
		// Datei ist hinter den Daten vorbelegt (siehe TruncateBefore), und
		// nur der Offset sagt, wo die Daten enden -- die Dateigroesse nicht.
		n, err := tmp.WriteAt(appendRecord(nil, entry.Seq, entry.Payload), off)
		if err != nil {
			return off, fmt.Errorf("wal: could not write compacted record: %w", err)
		}
		off += int64(n)
		seitLetztemSync += n
		if seitLetztemSync >= syncStueck {
			if err := datasync(tmp); err != nil {
				return off, fmt.Errorf("wal: could not sync compacted chunk: %w", err)
			}
			seitLetztemSync = 0
		}
	}
}

func seqHighWaterMarkPath(path string) string { return path + ".seqhwm" }

// readSeqHighWaterMark returns the persisted high-water mark for path, or 0 if
// no mark has ever been written (the normal case — the mark only appears once
// TruncateBefore has run).
//
// A mark that exists but cannot be parsed is an ERROR, not a 0: falling back to
// "start from whatever the file says" is precisely the sequence reuse this
// mechanism exists to prevent, and doing it silently would hide the one
// condition worth shouting about. Refusing to open is the safe direction — the
// node fails to start instead of quietly corrupting two Postgres-side
// invariants that no consumer can detect being violated.
func readSeqHighWaterMark(path string) (uint64, error) {
	data, err := os.ReadFile(seqHighWaterMarkPath(path))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var v uint64
	if _, err := fmt.Sscanf(string(data), "%d", &v); err != nil {
		return 0, fmt.Errorf("mark file %s is present but unreadable (%q): refusing to guess a "+
			"starting sequence number, because guessing low silently reuses numbers already issued",
			seqHighWaterMarkPath(path), string(data))
	}
	return v, nil
}

// writeSeqHighWaterMark durably records seq as the highest sequence number ever
// issued for the log at path. Written to a temp file, fsynced, then renamed
// over the mark: a crash can leave the old mark or the new one, never a
// half-written number.
//
// Like TruncateBefore's own rename, this does not fsync the containing
// directory, so a power loss immediately after the rename could in principle
// lose the rename itself — matching the durability level the rest of this file
// already commits to rather than silently claiming a stronger one here.
func writeSeqHighWaterMark(path string, seq uint64) error {
	markPath := seqHighWaterMarkPath(path)
	tmpPath := markPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write([]byte(fmt.Sprintf("%d", seq))); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, markPath); err != nil {
		os.Remove(tmpPath)
		return err
	}
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

		// A sequence number that does not advance ends the log.
		//
		// Sequence numbers are assigned strictly increasing and are never
		// reused (writeBatch burns them rather than rolling back on a failed
		// write), so a record whose seq does not move forward is not data.
		//
		// It is specifically what PREALLOCATED PADDING looks like: zero bytes
		// read as seq=0, length=0, and crc32 of an empty payload is 0, so the
		// checksum MATCHES. Without this the scan would walk the entire
		// preallocated tail as an endless run of valid empty records and
		// return lastSeq=0 -- reporting the high-water mark of a live log as
		// zero, which is the worst possible answer.
		//
		// It also guards the failure class that already hit production once:
		// commit 8a49c38, "compacting the write-ahead log handed out sequence
		// numbers it had already used".
		if entry.Seq == 0 || (count > 0 && entry.Seq <= lastSeq) {
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
	var lastSeq uint64
	for {
		entry, ok, recErr := readRecord(r)
		if recErr != nil {
			return count, true, nil
		}
		if !ok {
			return count, false, nil
		}
		// Preallocated padding is the normal tail of a WAL whose process died
		// before Close could trim it: zero bytes parse as a valid record with
		// seq 0 and an empty payload, and crc32 of an empty payload IS zero,
		// so the checksum matches. Sequence numbers start at 1, so seq 0 can
		// only be padding -- and it is the clean end of the log, not damage.
		if entry.Seq == 0 {
			return count, false, nil
		}
		// A non-zero sequence that does not advance is something else, and
		// worth reporting: sequence numbers are strictly increasing and never
		// reused. Commit 8a49c38 records what it looked like when compaction
		// broke that.
		if count > 0 && entry.Seq <= lastSeq {
			return count, true, nil
		}
		lastSeq = entry.Seq
		if err := fn(entry); err != nil {
			return count, false, fmt.Errorf("wal: replay callback failed at seq %d: %w", entry.Seq, err)
		}
		count++
	}
}

// Path meldet den Dateipfad dieses WAL. Die Kompaktierung braucht ihn, um zu
// entscheiden, ob sich das Neuschreiben ueberhaupt lohnt -- siehe
// keeper/wal_kompaktierung.go.
func (w *WAL) Path() string { return w.path }
