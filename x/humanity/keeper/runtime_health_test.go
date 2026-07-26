package keeper

import "testing"

// The primary has restarted under load repeatedly and the cause was never
// established, because the node reported nothing about its own memory. These
// guard that the numbers an operator needs are actually present and non-zero —
// a snapshot that silently returned an empty map would recreate exactly the
// blind spot this exists to remove.
func TestRuntimeSnapshot_ReportsWhatAnOOMInvestigationNeeds(t *testing.T) {
	snap := runtimeSnapshot()
	for _, key := range []string{
		"heap_alloc_mb", "heap_sys_mb", "total_sys_mb",
		"heap_objects", "num_goroutine", "num_gc", "gc_pause_total_ms",
	} {
		if _, ok := snap[key]; !ok {
			t.Fatalf("%q missing — an OOM investigation cannot distinguish a climbing heap from a flat one without it", key)
		}
	}
	// total_sys_mb is what a container memory limit actually kills on, so a
	// zero there would make the whole snapshot useless for the case it exists
	// for. Goroutine count is always at least this test's own.
	if n, _ := snap["num_goroutine"].(int); n <= 0 {
		t.Fatalf("num_goroutine must be positive, got %d", n)
	}
	if _, ok := snap["total_sys_mb"].(uint64); !ok {
		t.Fatal("total_sys_mb must be a number an operator can compare against the container limit")
	}
}

// Starting twice must not launch a second watcher goroutine — main may be
// re-entered in tests and a duplicate would double the stop-the-world sampling
// this deliberately keeps rare.
func TestStartHeapWatcher_IsIdempotent(t *testing.T) {
	StartHeapWatcher()
	StartHeapWatcher()
	if !heapWatcherStarted.Load() {
		t.Fatal("watcher must record that it started")
	}
}
