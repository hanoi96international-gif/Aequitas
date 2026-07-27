package keeper

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
)

// Contention profiling, off unless explicitly switched on.
//
// WHY THIS EXISTS. This node has 133 exclusive `cs.mu.Lock()` call sites and
// exactly four of them were instrumented by hand (see exclusive_lock_stats.go).
// That instrumentation reported the block-replay lock holding 3.16% of wall
// time and was read as "the exclusive lock is not the problem" — a conclusion
// drawn from 3% of the lock sites. At the same moment a goroutine dump showed
// 132 goroutines parked in sync.RWMutex.RLock, which those four counters
// cannot explain and cannot locate.
//
// Hand-instrumenting the remaining 129 sites would be a large, error-prone
// change to the consensus path for a diagnostic. Go already answers this
// question properly: the mutex and block profiles attribute blocked time to
// the exact stack that caused it, with no code change at any lock site.
// net/http/pprof is already served (startPprofServer), but both profiles are
// governed by a sampling rate that defaults to zero — so those endpoints have
// been returning empty profiles all along, which is why the contention never
// showed up in anything measured so far.
//
// WHY IT IS OFF BY DEFAULT. Both profiles cost real time on every contended
// lock and every blocking operation, on the payment path. The primary must not
// be made slower or less predictable by a diagnostic; it must never crash
// (that requirement has its own history in this project). So this stays dark
// unless an operator turns it on for a specific investigation, on a specific
// box.
//
// HOW TO USE IT. Set AEQUITAS_CONTENTION_PROFILE=1 for the default sampling
// rates, or to an integer N for a custom mutex fraction (1 = record every
// contention event, higher = sample 1/N of them). Then, from inside the
// container:
//
//	curl -s localhost:6061/debug/pprof/mutex > mutex.pprof
//	curl -s localhost:6061/debug/pprof/block > block.pprof
//
// and read them off-box with `go tool pprof -top`.

// defaultMutexProfileFraction records one in every N contention events. 100
// keeps the added cost negligible while still ranking lock sites reliably:
// the ordering of contended locks is what this is for, not an exact total.
const defaultMutexProfileFraction = 100

// defaultBlockProfileRate samples one blocking event per this many nanoseconds
// of blocking. 10ms is coarse on purpose — the waits being hunted here are the
// long ones (a transfer averaging 10.65ms, individual holds reaching 432ms),
// and a finer rate would sample far more aggressively than the question needs.
const defaultBlockProfileRate = 10_000_000

// StartContentionProfiling enables the mutex and block profiles when
// AEQUITAS_CONTENTION_PROFILE is set, and does nothing otherwise.
//
// Returns whether profiling was enabled, so the caller can say so in the
// startup log — a node running with a diagnostic active should announce it
// rather than leave someone to wonder why it is slower than its peers.
func StartContentionProfiling() bool {
	raw := os.Getenv("AEQUITAS_CONTENTION_PROFILE")
	if raw == "" || raw == "0" || raw == "false" {
		return false
	}

	fraction := defaultMutexProfileFraction
	// A value other than the plain on/off forms is read as an explicit mutex
	// fraction. An unparseable or non-positive value keeps the default rather
	// than disabling the profile the operator just asked for, or (worse)
	// passing 0 to SetMutexProfileFraction, which means "off".
	if raw != "1" && raw != "true" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			fraction = n
		}
	}

	runtime.SetMutexProfileFraction(fraction)
	runtime.SetBlockProfileRate(defaultBlockProfileRate)

	fmt.Printf("[PROF] contention profiling ON (mutex 1/%d, block %dms) — this costs throughput; unset AEQUITAS_CONTENTION_PROFILE to disable\n",
		fraction, defaultBlockProfileRate/1_000_000)
	return true
}
