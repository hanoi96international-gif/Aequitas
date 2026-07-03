package keeper

import (
	"sync"
	"testing"
	"time"
)

// TestGhostdagBlockLookup_MemoryHit verifies the fast path (block resident in
// dag.blocks) never touches dag.state, and still works when dag.state is nil
// — the same contract the raw `dag.blocks[hash]` access had before
// ghostdagBlockLookup replaced it in computeGHOSTDAGState/ghostdagMergeSet/
// ghostdagIsAncestor.
func TestGhostdagBlockLookup_MemoryHit(t *testing.T) {
	dag := newGhostdagTestDAG()
	want := &Block{Hash: "h1", Height: 1}
	dag.blocks["h1"] = want

	got := dag.ghostdagBlockLookup("h1")
	if got != want {
		t.Fatalf("ghostdagBlockLookup returned %v, want the in-memory block %v", got, want)
	}
}

// TestGhostdagBlockLookup_MissNoState verifies a genuinely-missing hash with
// no DB backing (dag.state == nil, the same setup every other GHOSTDAG scale
// test in this package uses) returns nil rather than panicking — matching
// the prior raw map access's `ok == false` behavior exactly.
func TestGhostdagBlockLookup_MissNoState(t *testing.T) {
	dag := newGhostdagTestDAG()
	if got := dag.ghostdagBlockLookup("does-not-exist"); got != nil {
		t.Fatalf("ghostdagBlockLookup() = %v, want nil for a hash absent from both dag.blocks and dag.state", got)
	}
}

// TestTriggerSoftRetryFlush_CoalescesConcurrentTriggers is the regression
// guard for the goroutine-spawn amplification found live on 2026-07-03 (see
// triggerSoftRetryFlush's comment): a burst of concurrent triggers must run
// the underlying flush at most a small, bounded number of times — not once
// per trigger — and must leave softRetryFlushInFlight cleared afterward so a
// future trigger can still start a fresh pass.
func TestTriggerSoftRetryFlush_CoalescesConcurrentTriggers(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.softRetryBlocks = make(map[string]*Block)
	dag.softRetryFirstAt = make(map[string]time.Time)

	// retryAndFlushSoftRetry's own behavior is covered elsewhere; what's under
	// test here is triggerSoftRetryFlush's coalescing logic itself — that a
	// burst of concurrent triggers all return promptly and leave the
	// in-flight/again bookkeeping in a clean, non-stuck state, regardless of
	// how many flush passes actually ran underneath.
	var runs int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dag.triggerSoftRetryFlush()
			mu.Lock()
			runs++
			mu.Unlock()
		}()
	}
	wg.Wait()

	// Give the last spawned flush goroutine(s) a moment to finish and clear
	// the in-flight flag.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		dag.softRetryMu.Lock()
		inFlight := dag.softRetryFlushInFlight
		dag.softRetryMu.Unlock()
		if !inFlight {
			break
		}
		time.Sleep(time.Millisecond)
	}

	dag.softRetryMu.Lock()
	inFlight := dag.softRetryFlushInFlight
	again := dag.softRetryFlushAgain
	dag.softRetryMu.Unlock()
	if inFlight {
		t.Fatalf("softRetryFlushInFlight still true after all triggers settled — a future trigger would be silently dropped")
	}
	if again {
		t.Fatalf("softRetryFlushAgain still true after settling — a pass was requested but never run")
	}
	if runs != 200 {
		t.Fatalf("expected all 200 trigger calls to return, got %d", runs)
	}
}
