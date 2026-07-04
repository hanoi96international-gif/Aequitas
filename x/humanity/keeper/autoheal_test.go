package keeper

import (
	"testing"
	"time"
)

// TestStartChainDivergenceCheck_NoopWithoutPrimaryURL verifies the check
// never touches dag.state (which is nil in this minimal test DAG) when
// PRIMARY_NODE_URL isn't configured — it must return before starting the
// goroutine, not panic or spin up a ticker that dereferences a nil state.
func TestStartChainDivergenceCheck_NoopWithoutPrimaryURL(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.startChainDivergenceCheck("")
}

// TestRunChainDivergenceCheckOnce_UnsettledSkipsBeforeAnyNetworkCall is the
// regression guard for the 2026-07-04 refactor (extracting the ticker
// loop's body into runChainDivergenceCheckOnce so it can also run once,
// immediately, at startup — see startChainDivergenceCheck's own comment
// for the self-heal blind spot this closes). Verifies the unsettled-state
// bookkeeping (unsettledSince) still behaves exactly as it did as a
// loop-local variable: zero -> set on first unsettled observation -> reset
// to zero once settled again. Uses an unreachable primaryURL so if the
// unsettled guard ever failed to short-circuit, the test would hang/fail on
// a real network attempt instead of silently passing.
func TestRunChainDivergenceCheckOnce_UnsettledSkipsBeforeAnyNetworkCall(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.tips = make(map[string]bool)
	const unreachable = "http://127.0.0.1:1" // pinningDialer rejects loopback instantly

	// Force "catching up": bootHeight far ahead of dag.height.
	dag.bootHeight = 100000
	dag.height = 0

	var unsettledSince time.Time
	dag.runChainDivergenceCheckOnce(unreachable, &unsettledSince)
	if unsettledSince.IsZero() {
		t.Fatal("unsettledSince should be set after the first unsettled observation")
	}
	firstMark := unsettledSince

	dag.runChainDivergenceCheckOnce(unreachable, &unsettledSince)
	if unsettledSince != firstMark {
		t.Fatal("unsettledSince should not be reset on a second consecutive unsettled observation")
	}

	// Settle: bring dag.height within the catch-up buffer of bootHeight.
	dag.height = 99995
	dag.runChainDivergenceCheckOnce(unreachable, &unsettledSince)
	if !unsettledSince.IsZero() {
		t.Fatal("unsettledSince should reset to zero once the node is settled")
	}
}
