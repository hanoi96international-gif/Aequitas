package keeper

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// api_debug_goroutines.go unblocks the cs.mu-contention investigation.
//
// The investigation has a specific, mechanical blocker: diagnosing which
// handlers are stuck waiting on which lock needs a goroutine dump from the
// primary, and there is no way to get one. net/http/pprof is deliberately
// bound to localhost only, and the primary runs on a platform with no
// `docker exec` — so the standard route to a dump does not exist there.
// Without it, every theory about the 11-16 second response times stayed a
// theory: two hypotheses were excluded by elimination, and the actual cause
// was never found.
//
// This serves the dump over the ordinary API, behind the same
// SNAPSHOT_TOKEN bearer check the other operator-only endpoints already use,
// and — critically — WITHOUT taking cs.mu, dag.mu, or any other application
// lock. That is the whole point: the endpoint has to answer precisely when
// something else is holding a lock and not letting go. runtime.Stack reads
// the scheduler's own view of every goroutine, so a fully wedged
// ChainState changes nothing about its ability to reply.
//
// Two views are offered, because they answer different questions:
//
//	/api/debug/goroutines          full dump, exactly what pprof would give
//	/api/debug/goroutines?summary  goroutines grouped by what they are
//	                               blocked on, counted, slowest wait first
//
// The summary is the one to read first. "142 goroutines blocked in
// sync.(*RWMutex).Lock under ChainState.TransferAtomic" is a diagnosis;
// eight megabytes of raw stacks is homework.

// goroutineDumpMaxBytes bounds the buffer used for a full dump. A node with
// tens of thousands of goroutines can produce a very large dump, and this
// endpoint must never be the thing that pushes a struggling process into an
// OOM. runtime.Stack truncates rather than failing if the buffer is too
// small, and the response says so explicitly when that happens.
const goroutineDumpMaxBytes = 32 << 20 // 32 MiB

// goroutineDumpMinInterval rate-limits full dumps. Collecting one stops the
// world briefly (proportional to goroutine count), so an operator — or a
// monitoring system pointed at this URL by mistake — must not be able to
// turn the diagnostic into the outage.
const goroutineDumpMinInterval = 2 * time.Second

var (
	goroutineDumpMu     sync.Mutex
	goroutineDumpLastAt time.Time
)

// handleGoroutineDump serves GET /api/debug/goroutines.
func (a *APIServer) handleGoroutineDump(w http.ResponseWriter, r *http.Request) {
	token := os.Getenv("SNAPSHOT_TOKEN")
	if token == "" {
		http.Error(w, `{"error":"SNAPSHOT_TOKEN not configured on this node"}`, http.StatusForbidden)
		return
	}
	auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(auth), []byte(token)) != 1 {
		http.Error(w, `{"error":"unauthorized — set Authorization: Bearer <SNAPSHOT_TOKEN>"}`, http.StatusUnauthorized)
		return
	}

	goroutineDumpMu.Lock()
	since := time.Since(goroutineDumpLastAt)
	if since < goroutineDumpMinInterval {
		goroutineDumpMu.Unlock()
		w.Header().Set("Retry-After", "2")
		http.Error(w, `{"error":"rate limited — one dump every 2s; collecting a dump briefly stops the world"}`, http.StatusTooManyRequests)
		return
	}
	goroutineDumpLastAt = time.Now()
	goroutineDumpMu.Unlock()

	// NOTE: no application lock is taken anywhere in this handler. See this
	// file's comment — the endpoint exists for the case where one is stuck.
	dump, truncated := captureGoroutineDump()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Goroutine-Count", strconv.Itoa(runtime.NumGoroutine()))
	if truncated {
		w.Header().Set("X-Dump-Truncated", "true")
	}

	if _, wantSummary := r.URL.Query()["summary"]; wantSummary {
		fmt.Fprint(w, summarizeGoroutineDump(dump, truncated))
		return
	}
	if truncated {
		fmt.Fprintf(w, "!! DUMP TRUNCATED at %d bytes — %d goroutines were live; use ?summary for the grouped view\n\n",
			goroutineDumpMaxBytes, runtime.NumGoroutine())
	}
	w.Write(dump)
}

// captureGoroutineDump returns all goroutine stacks, growing the buffer up
// to goroutineDumpMaxBytes. The bool reports whether the dump was cut off.
func captureGoroutineDump() ([]byte, bool) {
	size := 1 << 20
	for {
		buf := make([]byte, size)
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return buf[:n], false
		}
		if size >= goroutineDumpMaxBytes {
			return buf[:n], true
		}
		size *= 2
		if size > goroutineDumpMaxBytes {
			size = goroutineDumpMaxBytes
		}
	}
}

// blockedGroup is one "these N goroutines are all stuck the same way" entry.
type blockedGroup struct {
	State      string // scheduler state, e.g. "semacquire", "IO wait"
	WaitingOn  string // the frame that is actually blocking, e.g. "sync.(*RWMutex).Lock"
	Origin     string // the outermost project frame — who got us here
	Count      int
	MaxWaitMin int // longest "N minutes" the scheduler reported for this group
}

// summarizeGoroutineDump groups a raw dump by (state, blocking frame,
// originating project frame) and renders it counted, longest wait first.
//
// The grouping keys are chosen for one purpose: making lock contention
// legible. The scheduler state says whether a goroutine is blocked at all;
// the blocking frame says on WHAT (a mutex, a channel, a syscall); and the
// originating frame — the outermost frame belonging to this project rather
// than the runtime or a dependency — says which subsystem is responsible.
// Together those three answer "who is waiting on which lock, and how many
// of them", which is exactly the question the cs.mu investigation stalled on.
func summarizeGoroutineDump(dump []byte, truncated bool) string {
	groups := map[string]*blockedGroup{}
	total := 0

	for _, block := range strings.Split(string(dump), "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		if !strings.HasPrefix(lines[0], "goroutine ") {
			continue
		}
		total++

		state, waitMin := parseGoroutineHeader(lines[0])
		waitingOn, origin := classifyFrames(lines[1:])

		key := state + "|" + waitingOn + "|" + origin
		g := groups[key]
		if g == nil {
			g = &blockedGroup{State: state, WaitingOn: waitingOn, Origin: origin}
			groups[key] = g
		}
		g.Count++
		if waitMin > g.MaxWaitMin {
			g.MaxWaitMin = waitMin
		}
	}

	out := make([]*blockedGroup, 0, len(groups))
	for _, g := range groups {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MaxWaitMin != out[j].MaxWaitMin {
			return out[i].MaxWaitMin > out[j].MaxWaitMin
		}
		return out[i].Count > out[j].Count
	})

	var b strings.Builder
	fmt.Fprintf(&b, "goroutines: %d (parsed %d stacks)\n", runtime.NumGoroutine(), total)
	if truncated {
		fmt.Fprintf(&b, "!! dump truncated at %d bytes — counts below are a LOWER BOUND\n", goroutineDumpMaxBytes)
	}
	b.WriteString("\ngrouped by (state, blocking frame, originating frame), longest wait first:\n")
	b.WriteString("  the top line is usually the cause; everything under it is usually the symptom\n\n")
	for _, g := range out {
		fmt.Fprintf(&b, "%6d  %-14s", g.Count, g.State)
		if g.MaxWaitMin > 0 {
			fmt.Fprintf(&b, " waited >= %dmin", g.MaxWaitMin)
		}
		b.WriteString("\n")
		fmt.Fprintf(&b, "        blocked in: %s\n", g.WaitingOn)
		fmt.Fprintf(&b, "        entered at: %s\n\n", g.Origin)
	}
	return b.String()
}

// parseGoroutineHeader reads "goroutine 42 [semacquire, 13 minutes]:" into
// its state and the reported wait in minutes (0 when none is reported).
func parseGoroutineHeader(line string) (state string, waitMin int) {
	open := strings.Index(line, "[")
	close := strings.LastIndex(line, "]")
	if open < 0 || close <= open {
		return "unknown", 0
	}
	inner := line[open+1 : close]
	parts := strings.SplitN(inner, ",", 2)
	state = strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		if n, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[1]), " minutes"))); err == nil {
			waitMin = n
		}
	}
	return state, waitMin
}

// classifyFrames picks the blocking frame and the originating project frame
// out of one goroutine's stack.
//
// Frame lines alternate: a function line, then an indented file:line. The
// blocking frame is the first function that is not pure runtime scheduling
// plumbing — for a contended mutex that is sync.(*RWMutex).Lock, which is
// the useful answer. The origin is the LAST project frame, i.e. the
// outermost one, which names the subsystem rather than the leaf helper.
func classifyFrames(lines []string) (waitingOn, origin string) {
	const projectPrefix = "github.com/hanoi96international-gif/aequitas-chain/"
	waitingOn = "unknown"
	origin = "unknown"

	for _, l := range lines {
		if strings.HasPrefix(l, "\t") || strings.HasPrefix(l, " ") {
			continue // the file:line half of the pair
		}
		fn := strings.TrimSpace(l)
		if fn == "" || strings.HasPrefix(fn, "created by ") {
			continue
		}
		fn = stripCallArgs(fn)
		if waitingOn == "unknown" && !isSchedulerPlumbing(fn) {
			waitingOn = fn
		}
		if strings.HasPrefix(fn, projectPrefix) {
			origin = strings.TrimPrefix(fn, projectPrefix)
		}
	}
	return waitingOn, origin
}

// stripCallArgs turns a stack frame's function line into a bare function
// name by removing the trailing argument list.
//
// Naive "cut at the first (" is wrong for Go's method frames: in
// `sync.(*RWMutex).RLock(...)` the first paren belongs to the receiver type,
// and cutting there yields "sync." — which silently collapses every mutex
// method into one meaningless group. The argument list is the LAST
// parenthesised group, so this scans from the end and matches it.
func stripCallArgs(fn string) string {
	fn = strings.TrimSpace(fn)
	if !strings.HasSuffix(fn, ")") {
		return fn
	}
	depth := 0
	for i := len(fn) - 1; i >= 0; i-- {
		switch fn[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return fn[:i]
			}
		}
	}
	return fn
}

// isSchedulerPlumbing reports whether fn is a frame that every blocked
// goroutine has and that therefore says nothing about WHY this one is
// blocked.
//
// sync.runtime_* matters as much as runtime.* here: those are the runtime
// bridge functions that sit BELOW the mutex method in every contended-lock
// stack. Treating them as the blocking frame would report
// "sync.runtime_SemacquireRWMutexR" for every lock in the process, which is
// true and useless — the frame an operator needs is the one above it,
// sync.(*RWMutex).RLock, because that one says which kind of wait it is.
func isSchedulerPlumbing(fn string) bool {
	switch fn {
	case "runtime.gopark", "runtime.goparkunlock", "runtime.semacquire1",
		"runtime.semacquire", "runtime.notesleep", "runtime.selectgo",
		"runtime.chanrecv", "runtime.chansend", "runtime.block":
		return true
	}
	return strings.HasPrefix(fn, "runtime.") ||
		strings.HasPrefix(fn, "sync.runtime_") ||
		strings.HasPrefix(fn, "internal/poll.runtime_") ||
		strings.HasPrefix(fn, "sync/atomic.")
}
