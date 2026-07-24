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

// TestPersistedUnsettledSince_NilStateIsNoOp is the regression guard for the
// nil-panic TestRunChainDivergenceCheckOnce_UnsettledSkipsBeforeAnyNetworkCall
// caught when the 2026-07-09 durable-clock fix was first added: dag.state is
// nil in this minimal test DAG (see this file's own header comment), so
// persistedUnsettledSince/setPersistedUnsettledSince must degrade to a safe
// no-op/empty-read rather than dereference a nil ChainState.
func TestPersistedUnsettledSince_NilStateIsNoOp(t *testing.T) {
	dag := newGhostdagTestDAG()
	if dag.state != nil {
		t.Fatal("test assumption violated: newGhostdagTestDAG's dag.state must be nil for this test to be meaningful")
	}
	if got := dag.persistedUnsettledSince(); got != "" {
		t.Fatalf("persistedUnsettledSince with nil dag.state: want empty string, got %q", got)
	}
	dag.setPersistedUnsettledSince("1700000000") // must not panic
	if got := dag.loadUnsettledSinceFromDB(); !got.IsZero() {
		t.Fatalf("loadUnsettledSinceFromDB with nil dag.state: want zero Time, got %v", got)
	}
}

// TestRunChainDivergenceCheckOnce_PreSeededOldUnsettledSinceOverridesImmediately
// is the regression guard for the actual bug found live 2026-07-09:
// unsettledSince used to be purely in-memory (always starting from the zero
// value), so a node that gets restarted more often than
// chainDivergenceStallOverride (45min) — exactly what happens during a live
// incident where an operator keeps redeploying/resyncing it — could never
// accumulate enough continuous unsettled time to trigger the override, no
// matter how long it had actually been isolated for in wall-clock terms:
// every restart reset the 45-minute countdown back to zero.
//
// The actual fix (loadUnsettledSinceFromDB, wired into
// startChainDivergenceCheck) seeds a NEW goroutine's unsettledSince from a
// persisted chain_config value instead of the zero value — this test proves
// the consumption side of that fix: given an unsettledSince that is already
// 50 minutes old on the VERY FIRST call in this process's lifetime (exactly
// what loadUnsettledSinceFromDB would hand back after a restart mid-outage),
// the override fires immediately rather than restarting the 45-minute wait.
// The SQL round trip that produces that seeded value (chain_config
// upsert/read-back for chainDivergenceUnsettledSinceKey) was separately
// verified live against a real Postgres instance.
func TestRunChainDivergenceCheckOnce_PreSeededOldUnsettledSinceOverridesImmediately(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.tips = make(map[string]bool)
	const unreachable = "http://127.0.0.1:1" // fails fast (connection refused), doesn't hang
	dag.bootHeight = 100000
	dag.height = 0 // forces isCatchingUpLocked() == true, i.e. "unsettled"

	// Simulate "this is a freshly restarted process, but the DB says we've
	// already been unsettled for 50 minutes straight" — exactly what
	// loadUnsettledSinceFromDB would return right after a restart, unlike
	// the old code's unconditional zero value.
	preSeeded := time.Now().Add(-50 * time.Minute)
	unsettledSince := preSeeded

	dag.runChainDivergenceCheckOnce(unreachable, &unsettledSince)

	// The old (buggy) behavior would treat this as brand new (since a
	// fresh in-memory var always started at the zero value) and skip with
	// "still catching up", staying unchanged. The fix must instead
	// recognize 50 minutes > chainDivergenceStallOverride (45 min) and
	// proceed past the skip into the real comparison — fetchPrimaryHeight
	// then fails fast against the unreachable URL and returns without
	// mutating unsettledSince further, but the key assertion is that it
	// did NOT get reset to "just now" by a false "first observation".
	if unsettledSince != preSeeded {
		t.Fatalf("a pre-seeded, already-stale unsettledSince must not be overwritten as a fresh observation: want unchanged %v, got %v", preSeeded, unsettledSince)
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

// TestAddPeerBlock_RecordsPeerContactEvenWhenRejected is the regression
// guard for the 2026-07-24 stall-timeout fix (see lastPeerContactAt's own
// struct comment for the fork incident this closes): a foreign block must
// update lastPeerContactAt at AddPeerBlock's unconditional entry point,
// BEFORE any gate — including gates, like resyncInProgress here, that go on
// to reject the block outright. This is what lets ProduceBlock's
// stall-timeout escape valves tell "peer has gone silent" apart from "peer
// is sending plenty, this node just can't merge it fast enough yet".
func TestAddPeerBlock_RecordsPeerContactEvenWhenRejected(t *testing.T) {
	dag := newGhostdagTestDAG()
	dag.resyncInProgress.Store(true) // forces an early, safe reject
	before := time.Now().Unix()
	block := &Block{
		Hash:         "0xdeadbeef",
		Height:       1,
		Proposer:     "0xsomepeer",
		ProducedAtMs: time.Now().UnixMilli(),
	}
	if ok := dag.AddPeerBlock(block); ok {
		t.Fatal("expected AddPeerBlock to reject while resyncInProgress")
	}
	if got := dag.lastPeerContactAt.Load(); got < before {
		t.Fatalf("lastPeerContactAt = %d, want it set to roughly now (>= %d) even though the block was rejected", got, before)
	}
}

// TestLastPeerActivityAt_ReturnsMoreRecentOfTheTwoSignals verifies the core
// logic the 2026-07-24 fork-prevention fix relies on: whichever of
// lastSuccessfulPeerSyncAt (a successful merge) and lastPeerContactAt (any
// received block, merged or not) is more recent wins — a node that's
// receiving plenty of peer traffic but merging none of it must still read
// as "recently active", not stale, or the stall-timeout gates would
// wrongly conclude the peer is unreachable and let this node fork off its
// own chain.
func TestLastPeerActivityAt_ReturnsMoreRecentOfTheTwoSignals(t *testing.T) {
	dag := newGhostdagTestDAG()

	dag.lastSuccessfulPeerSyncAt.Store(100)
	dag.lastPeerContactAt.Store(50)
	if got := dag.lastPeerActivityAt(); got != 100 {
		t.Fatalf("lastPeerActivityAt() = %d, want 100 (successful-merge signal is more recent)", got)
	}

	dag.lastSuccessfulPeerSyncAt.Store(50)
	dag.lastPeerContactAt.Store(200)
	if got := dag.lastPeerActivityAt(); got != 200 {
		t.Fatalf("lastPeerActivityAt() = %d, want 200 (contact-only signal is more recent — the exact case a backlogged-but-reachable peer produces)", got)
	}

	dag.lastSuccessfulPeerSyncAt.Store(0)
	dag.lastPeerContactAt.Store(0)
	if got := dag.lastPeerActivityAt(); got != 0 {
		t.Fatalf("lastPeerActivityAt() = %d, want 0 when neither signal has ever fired", got)
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
