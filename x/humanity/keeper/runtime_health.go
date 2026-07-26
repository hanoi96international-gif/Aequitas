package keeper

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"time"
)

// Memory telemetry, because the primary has restarted under load repeatedly
// and the cause has stayed unknown every single time.
//
// The node reported nothing about its own memory: no heap size, no goroutine
// count, no GC statistics anywhere in the codebase. Railway's own logs would
// say whether a container was OOM-killed, but reading them needs an API token
// this repository does not have. So each restart could only ever be observed,
// never explained — and "a restart fixed it" has been the recurring shape of
// several incidents in this project's history.
//
// These numbers turn that into an answerable question. A sample taken while a
// node is healthy, compared against one taken as it degrades, distinguishes a
// climbing heap (a leak or an unbounded queue) from a flat one (killed for an
// unrelated reason). Those call for opposite investigations, and guessing
// between them has cost real time here.

// runtimeSnapshot reports the process's current memory and scheduler state.
//
// ReadMemStats stops the world briefly, so callers must keep this on operator
// endpoints only — never anywhere the explorer polls on a timer.
func runtimeSnapshot() map[string]interface{} {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	const mb = 1024 * 1024
	return map[string]interface{}{
		// HeapAlloc is live heap; HeapSys is what the process has taken from
		// the OS and is the number a container memory limit actually kills on.
		"heap_alloc_mb": m.HeapAlloc / mb,
		"heap_sys_mb":   m.HeapSys / mb,
		"heap_objects":  m.HeapObjects,
		"stack_sys_mb":  m.StackSys / mb,
		// Total memory the runtime obtained from the OS — the closest thing to
		// what an OOM killer sees.
		"total_sys_mb": m.Sys / mb,
		"num_gc":       m.NumGC,
		// Cumulative stop-the-world pause. Rising fast means GC pressure;
		// SCALING_ARCHITECTURE.md records this as measured-and-fine at 15.8x
		// the target transaction rate, so a large value here would be new.
		"gc_pause_total_ms": m.PauseTotalNs / uint64(time.Millisecond),
		"num_goroutine":     runtime.NumGoroutine(),
	}
}

// heapWarnThresholdMB is where the watcher starts complaining. Railway's
// smaller instances sit around 512 MB-1 GB, and the Contabo boxes have 12 GB,
// so 768 MB is comfortably above anything a healthy node was ever observed to
// need while still leaving room to react before a container limit is reached.
const heapWarnThresholdMB = 768

// heapWatchInterval is slow on purpose: this exists to catch a heap that grows
// over minutes, not to sample fine-grained behaviour, and every tick pays a
// stop-the-world pause.
const heapWatchInterval = 60 * time.Second

var lastHeapWarnMB atomic.Uint64

// StartHeapWatcher logs a warning when the heap crosses heapWarnThresholdMB,
// and again on every further 256 MB.
//
// The point is to leave a mark in the logs BEFORE a restart rather than after
// it. A node that was OOM-killed leaves nothing behind by definition — the
// process is gone — so the only evidence that can exist is what it wrote while
// it was still alive. Every previous restart here produced no such evidence,
// which is exactly why the cause was never found.
//
// Safe to call more than once; only the first call starts a goroutine.
var heapWatcherStarted atomic.Bool

func StartHeapWatcher() {
	if !heapWatcherStarted.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("[PANIC RECOVERED] heap watcher: %v\n", r)
			}
		}()
		ticker := time.NewTicker(heapWatchInterval)
		defer ticker.Stop()
		for range ticker.C {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			heapMB := m.HeapAlloc / (1024 * 1024)
			sysMB := m.Sys / (1024 * 1024)
			if heapMB < heapWarnThresholdMB {
				// Recovered — allow the next crossing to warn again rather
				// than staying silent because an earlier peak was higher.
				lastHeapWarnMB.Store(0)
				continue
			}
			// Re-warn only every 256 MB, so a node sitting just above the
			// threshold does not fill the log with the same line every minute.
			if prev := lastHeapWarnMB.Load(); prev != 0 && heapMB < prev+256 {
				continue
			}
			lastHeapWarnMB.Store(heapMB)
			fmt.Printf("[MEM] ⚠ Heap at %d MB (process total %d MB from the OS), %d goroutines, %d GC cycles. "+
				"Above the %d MB watch threshold — if this node restarts shortly after this line, it was almost certainly killed for memory, and this is the evidence that would otherwise not exist.\n",
				heapMB, sysMB, runtime.NumGoroutine(), m.NumGC, heapWarnThresholdMB)
		}
	}()
}
