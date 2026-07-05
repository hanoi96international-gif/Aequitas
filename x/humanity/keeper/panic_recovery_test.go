package keeper

import (
	"sync"
	"testing"
	"time"
)

// Regression tests for the beta-launch audit (2026-07-05) P0-3/P1-2 fix:
// every `go func(){...}()` in this codebase used to have no recover() at
// all, so a panic anywhere in a background goroutine crashed the entire
// process. These tests prove SafeGoroutine/SafeCall actually survive a
// panic — the test process itself not dying IS the proof; a regression
// here would take the whole `go test` run down with it, not just fail an
// assertion.

func TestSafeGoroutine_RecoversPanicWithoutCrashing(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	ranAfterPanic := false
	SafeGoroutine("test-panic", func() {
		defer wg.Done()
		panic("deliberate test panic")
	})
	wg.Wait()
	// If SafeGoroutine's recover() didn't work, the panic would have already
	// crashed the whole test binary before reaching this line.
	ranAfterPanic = true
	if !ranAfterPanic {
		t.Fatal("unreachable if the process survived")
	}
}

func TestSafeGoroutine_RunsFnNormallyWhenNoPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	ran := false
	SafeGoroutine("test-normal", func() {
		defer wg.Done()
		ran = true
	})
	wg.Wait()
	if !ran {
		t.Fatal("expected fn to run normally when it doesn't panic")
	}
}

func TestSafeCall_RecoversPanicAndReturnsToCaller(t *testing.T) {
	SafeCall("test-panic", func() {
		panic("deliberate test panic")
	})
	// Reaching this line at all is the proof — an unrecovered panic would
	// have unwound past this point (and crashed the test binary).
}

func TestSafeCall_LoopSurvivesOnePanickingIteration(t *testing.T) {
	// Proves the actual pattern used throughout this codebase: a ticker
	// loop that recovers per-iteration keeps running after one bad
	// iteration, instead of a single panic silently ending the loop forever.
	completedIterations := 0
	for i := 0; i < 5; i++ {
		SafeCall("test-loop-tick", func() {
			completedIterations++
			if i == 2 {
				panic("deliberate mid-loop panic")
			}
		})
	}
	if completedIterations != 5 {
		t.Fatalf("expected all 5 iterations to run despite iteration 2 panicking, got %d", completedIterations)
	}
}

func TestSafeGoroutine_ConcurrentPanicsDoNotCrashProcess(t *testing.T) {
	// Fires several goroutines that all panic concurrently — if any one of
	// them weren't properly recovered, the whole test binary would go down
	// with it (Go: an unrecovered panic in ANY goroutine kills the process).
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		SafeGoroutine("test-concurrent-panic", func() {
			defer wg.Done()
			panic("deliberate concurrent test panic")
		})
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("goroutines did not complete — recover() may not be working, or a real deadlock")
	}
}
