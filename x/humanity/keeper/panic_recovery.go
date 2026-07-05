package keeper

import (
	"fmt"
	"runtime/debug"
)

// SafeGoroutine runs fn in a new goroutine with panic recovery, logging the
// panic (with a label for diagnosis) instead of letting it crash the whole
// process. Exported so cmd/aequitasd/main.go's own background goroutines
// (the block-production ticker chief among them) can use the exact same
// helper instead of a second, potentially-drifting copy of the same logic.
//
// FIX (P0-3, beta-launch audit 2026-07-05): every `go func(){...}()` in this
// codebase used to have no recover() at all. A panic in a Go goroutine can
// only be recovered by a deferred recover() call in THAT SAME goroutine's
// own call stack — a recover() elsewhere (even in the goroutine that
// launched it) cannot catch it, and an unrecovered panic anywhere takes
// down the entire process, not just the one operation. The public
// /api/peers/register endpoint puts a caller (any peer authorized to
// register a URL — a lower bar than full validator status, and also
// reachable by a legitimate but simply broken peer) in direct control of
// one of these call chains via startSyncForPeer → syncWithNode →
// doSyncOnce → fetchBlocksSince/syncValidatorsFromPeer, any panic in which
// (a nil dereference on an unexpected response shape, for example) used to
// crash the whole node.
//
// Every `go func(){...}()` in this package should go through this (or, for
// the handful of call sites that pass an explicit parameter into the
// goroutine func literal, an equivalent inline recover() — see those call
// sites for why a parameterized version isn't used everywhere: preserving
// each site's exact existing closure-capture semantics mattered more than
// forcing one shared signature).
func SafeGoroutine(label string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("[PANIC RECOVERED] goroutine %q: %v\n%s\n", label, r, debug.Stack())
			}
		}()
		fn()
	}()
}

// SafeCall runs fn with panic recovery and returns to the caller either way
// — for use INSIDE a long-lived loop (a ticker, a `for { time.Sleep(...);
// ... }` retry loop) where a single iteration panicking must not silently
// end the whole loop forever. SafeGoroutine's own recover happens once, at
// the top of the goroutine function — fine for a one-shot background task,
// but wrapping only the goroutine and not each loop iteration would still
// let one bad iteration kill every future one with no crash to notice.
func SafeCall(label string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[PANIC RECOVERED] %q: %v\n%s\n", label, r, debug.Stack())
		}
	}()
	fn()
}
