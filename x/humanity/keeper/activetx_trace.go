package keeper

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// activetx_trace.go answers, by measurement rather than by reading, the one
// question that blocks finishing the cs.activeTx migration (Roadmap step 5,
// SCALING_ARCHITECTURE.md Phase 5/7): WHICH call paths still reach a DB
// write without carrying the current operation's transaction in their ctx,
// and therefore still depend on the implicit cs.activeTx field?
//
// The production [DB-GUARD] log (see dbExecCtx) only ever fires for the
// subset of those paths that additionally run on a FOREIGN goroutine — the
// actively dangerous ones. It is silent about the much larger set that
// merely falls back on the OWNING goroutine, which is exactly the set that
// has to shrink to zero before cs.activeTx can be deleted and two atomic
// operations can run at the same time. Waiting for production logs would
// therefore have answered the wrong question, and answered it slowly.
//
// So instead: with AEQUITAS_ACTIVETX_TRACE=1 every such fallback records
// its stack, deduplicated by call site. Running the full test suite under
// that flag enumerates the real, reachable set — which is what
// activetx_trace_test.go then pins as a regression gate, so a newly added
// unmigrated write cannot silently reappear once the set has been driven
// to its intended contents.
//
// The trace is OFF unless the env var is set, and the check is a single
// preloaded bool, so this costs a predictable-branch read on the hot path
// and nothing else.

var (
	activeTxTraceOn    bool
	activeTxTraceOnce  sync.Once
	activeTxTraceMu    sync.Mutex
	activeTxTraceSeen  = map[string][]string{}
	activeTxTraceOut   *os.File
	activeTxTraceOutOK bool
	// activeTxTraceForce lets a test turn tracing on for its own duration
	// without the env var, so the "zero fallbacks" property can be asserted
	// by an ordinary `go test ./...` run instead of only by a human
	// remembering to set AEQUITAS_ACTIVETX_TRACE=1.
	activeTxTraceForce atomic.Bool
)

// activeTxTraceEnabled reports whether fallback tracing is on. Read once
// per process; toggling the env var mid-run has no effect by design (the
// trace is a measurement harness, not a runtime feature).
func activeTxTraceEnabled() bool {
	if activeTxTraceForce.Load() {
		return true
	}
	activeTxTraceOnce.Do(func() {
		activeTxTraceOn = os.Getenv("AEQUITAS_ACTIVETX_TRACE") == "1"
		if !activeTxTraceOn {
			return
		}
		path := os.Getenv("AEQUITAS_ACTIVETX_TRACE_FILE")
		if path == "" {
			return
		}
		// O_APPEND so several test binaries (each its own process) can
		// write into one shared file without clobbering each other; every
		// record is written with a single Write call, which is atomic for
		// appends below PIPE_BUF-scale sizes on Linux.
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Printf("[ACTIVETX-TRACE] could not open %s: %v — tracing to stdout instead\n", path, err)
			return
		}
		activeTxTraceOut, activeTxTraceOutOK = f, true
	})
	return activeTxTraceOn || activeTxTraceForce.Load()
}

// recordActiveTxFallback notes one implicit cs.activeTx pickup: a DB write
// that got the current operation's transaction from the ChainState field
// instead of from its own ctx. kind distinguishes the two fallback sites
// (dbExecCtx / activeTxCtx).
//
// Deduplication is by *call site* (the trimmed stack), not by occurrence —
// a hot path that falls back a million times is one entry, because the work
// it implies is one migration either way.
func recordActiveTxFallback(kind string) {
	site, stack := activeTxCallSite()
	key := kind + " " + site

	activeTxTraceMu.Lock()
	_, dup := activeTxTraceSeen[key]
	if !dup {
		activeTxTraceSeen[key] = stack
	}
	activeTxTraceMu.Unlock()
	if dup {
		return
	}

	rec := fmt.Sprintf("=== ACTIVETX-FALLBACK %s\nsite: %s\n%s\n", kind, site, strings.Join(stack, "\n"))
	if activeTxTraceOutOK {
		activeTxTraceOut.Write([]byte(rec))
		return
	}
	fmt.Print(rec)
}

// activeTxCallSite returns a stable identifier for the call path that fell
// back, plus the trimmed frames behind it.
//
// "Stable" matters: the key has to be identical across runs so the
// regression gate in activetx_trace_test.go can compare sets. So this uses
// function names and files only — never line numbers (which move with every
// edit to an unrelated part of the same file) and never addresses.
//
// The site itself is the first frame OUTSIDE this package's plumbing
// (dbExecCtx/dbExec/activeTxCtx/recordActiveTxFallback and the low-level
// save helpers are all just conduits) — i.e. the first frame that a person
// would actually have to migrate. The remaining frames are kept for
// context, capped so a deeply recursive replay stack cannot bloat the file.
func activeTxCallSite() (string, []string) {
	var pcs [48]uintptr
	n := runtime.Callers(3, pcs[:]) // skip runtime.Callers, this fn, recordActiveTxFallback
	frames := runtime.CallersFrames(pcs[:n])

	var (
		site  string
		trace []string
	)
	for {
		f, more := frames.Next()
		name := f.Function
		if name == "" {
			break
		}
		short := shortFuncName(name)
		if site == "" && !isActiveTxPlumbing(short) {
			site = short
		}
		if site != "" {
			trace = append(trace, "    "+short+" ("+shortFile(f.File)+")")
			if len(trace) >= 12 {
				break
			}
		}
		if !more {
			break
		}
	}
	if site == "" {
		site = "unknown"
	}
	return site, trace
}

// isActiveTxPlumbing reports whether fn is one of the pass-through helpers
// between a real call site and the fallback itself. These are never the
// thing to migrate — they are what every migrated caller keeps calling,
// just with a ctx that carries the transaction.
func isActiveTxPlumbing(fn string) bool {
	switch fn {
	case "ChainState.dbExec", "ChainState.dbExecCtx", "ChainState.activeTxCtx":
		return true
	}
	return false
}

func shortFuncName(full string) string {
	// "github.com/x/y/x/humanity/keeper.(*ChainState).saveAccountToDB" →
	// "ChainState.saveAccountToDB"
	if i := strings.LastIndex(full, "/"); i >= 0 {
		full = full[i+1:]
	}
	if i := strings.Index(full, "."); i >= 0 {
		full = full[i+1:]
	}
	return strings.NewReplacer("(*", "", ")", "").Replace(full)
}

func shortFile(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// ActiveTxFallbackSites returns the sorted set of distinct call sites that
// have fallen back to cs.activeTx in this process so far. Exported for
// activetx_trace_test.go, which uses it as the regression gate described at
// the top of this file.
func ActiveTxFallbackSites() []string {
	activeTxTraceMu.Lock()
	defer activeTxTraceMu.Unlock()
	out := make([]string, 0, len(activeTxTraceSeen))
	for k := range activeTxTraceSeen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ResetActiveTxFallbackSites clears the recorded set. Test-only.
func ResetActiveTxFallbackSites() {
	activeTxTraceMu.Lock()
	activeTxTraceSeen = map[string][]string{}
	activeTxTraceMu.Unlock()
}

// SetActiveTxTraceForced turns tracing on/off for the caller's duration.
// Test-only; returns the previous value so a test can restore it.
func SetActiveTxTraceForced(on bool) bool { return activeTxTraceForce.Swap(on) }
