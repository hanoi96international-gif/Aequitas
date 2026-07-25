package keeper

import (
	"database/sql"
	"testing"
)

// Regression guard for the 2026-07-25 profiling finding: SyncBalancesToEVM must
// not write to Postgres on the request path.
//
// It used to loop over its addresses and call SaveStorageSlot synchronously for
// each one. sendRawTransaction calls it with two addresses per transfer, so
// every transfer paid two extra DB round trips plus two cs.mu.RLock
// acquisitions before it could answer. Its sibling syncBalanceLocked was moved
// to the deferred path in SCALING_ARCHITECTURE.md Phase 6 for precisely this
// reason; this one was missed.
//
// These tests pin the ROUTING, not the SQL: the deferred path's own write
// (doSyncBalanceLocked) is unchanged, so the only thing that can silently
// regress is this function going back to writing inline.

// newMirrorRoutingTestState builds the minimal ChainState these tests need.
//
// db must be non-nil to get past SyncBalancesToEVM's own guard, but is never
// queried: marking the mirror dirty is in-memory, and the flush worker that
// markEVMMirrorDirtyLocked lazily starts returns before touching the database
// whenever the dirty set is empty. The cleanup empties it for exactly that
// reason, so the worker started by this test can never reach a fake handle.
func newMirrorRoutingTestState(t *testing.T) *ChainState {
	t.Helper()
	cs := &ChainState{db: &sql.DB{}}
	t.Cleanup(func() {
		cs.evmMirrorDirtyMu.Lock()
		cs.evmMirrorDirty = nil
		cs.evmMirrorDirtyMu.Unlock()
	})
	return cs
}

// The addresses must land in the dirty set — that is what proves the work was
// handed to the flush worker rather than performed inline.
func TestSyncBalancesToEVM_DefersInsteadOfWriting(t *testing.T) {
	cs := newMirrorRoutingTestState(t)

	cs.SyncBalancesToEVM("0xCONTRACT", "0xAAA", "0xBBB")

	cs.evmMirrorDirtyMu.Lock()
	defer cs.evmMirrorDirtyMu.Unlock()
	if len(cs.evmMirrorDirty) != 2 {
		t.Fatalf("expected both addresses queued for the deferred flush, got %d entries", len(cs.evmMirrorDirty))
	}
	for _, want := range []evmMirrorDirtyKey{
		{addr: "0xaaa", contractAddr: "0xcontract"},
		{addr: "0xbbb", contractAddr: "0xcontract"},
	} {
		if _, ok := cs.evmMirrorDirty[want]; !ok {
			t.Fatalf("missing dirty entry %+v — that address was not queued", want)
		}
	}
}

// Mixed case must collapse onto one key rather than queueing the same account
// twice, since flushEVMMirrorDirty groups by this key.
func TestSyncBalancesToEVM_NormalisesCase(t *testing.T) {
	cs := newMirrorRoutingTestState(t)

	cs.SyncBalancesToEVM("0xCoNtRaCt", "0xAbC", "0xaBc")

	cs.evmMirrorDirtyMu.Lock()
	defer cs.evmMirrorDirtyMu.Unlock()
	if len(cs.evmMirrorDirty) != 1 {
		t.Fatalf("the same address in two cases must produce one dirty entry, got %d", len(cs.evmMirrorDirty))
	}
}

// A nil db must remain a no-op. Nodes and tests run without one, and queueing
// work for a flush worker that can never write it would grow the set forever.
func TestSyncBalancesToEVM_NilDBQueuesNothing(t *testing.T) {
	cs := &ChainState{}

	cs.SyncBalancesToEVM("0xCONTRACT", "0xAAA")

	cs.evmMirrorDirtyMu.Lock()
	defer cs.evmMirrorDirtyMu.Unlock()
	if len(cs.evmMirrorDirty) != 0 {
		t.Fatalf("a nil db must queue nothing, got %d entries", len(cs.evmMirrorDirty))
	}
}
