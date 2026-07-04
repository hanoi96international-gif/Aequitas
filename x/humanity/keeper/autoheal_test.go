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

// TestProduceBlock_SkipsWhileResyncInProgress and
// TestAddPeerBlock_SkipsWhileResyncInProgress are the regression guard for
// the 2026-07-04 in-process-resync fix (PerformResync/triggerAutoResync):
// both functions must bail out before touching any lock or dag.state field
// while a resync is atomically swapping account/DAG state, so a minimal
// test DAG (state == nil, no locks ever initialized beyond zero value) must
// not panic just from the gate being checked first.
func TestProduceBlock_SkipsWhileResyncInProgress(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.resyncInProgress.Store(true)
	if b := dag.ProduceBlock(); b != nil {
		t.Fatalf("expected nil block while resyncInProgress, got %+v", b)
	}
}

func TestAddPeerBlock_SkipsWhileResyncInProgress(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.resyncInProgress.Store(true)
	if ok := dag.AddPeerBlock(&Block{Hash: "0xdeadbeef", Height: 1}); ok {
		t.Fatal("expected AddPeerBlock to reject while resyncInProgress")
	}
}

// TestPerformResync_ClearsInProgressFlagOnFailure verifies the resyncInProgress
// gate always gets cleared, even when the resync itself fails outright — a
// stuck-true gate would permanently halt ProduceBlock/AddPeerBlock, a much
// worse outcome than the divergence this was trying to fix. Uses an
// unreachable bootstrapURL (127.0.0.1:1, rejected instantly by the pinning
// dialer) so ResyncFromSnapshotURL fails fast, before ever touching
// dag.state (nil in this minimal test DAG) — the failure path returns from
// inside fetchAndValidateSnapshot, ahead of any cs.db/cs.mu access.
func TestPerformResync_ClearsInProgressFlagOnFailure(t *testing.T) {
	dag := newGhostdagTestDAG()
	const unreachable = "http://127.0.0.1:1"
	err := dag.PerformResync(unreachable, "0x0000000000000000000000000000000000000001", "")
	if err == nil {
		t.Fatal("expected an error from an unreachable bootstrap URL")
	}
	if dag.resyncInProgress.Load() {
		t.Fatal("resyncInProgress must be cleared even after a failed resync")
	}
}
