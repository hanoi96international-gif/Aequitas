package keeper

import (
	"testing"
	"time"
)

// This file is a regression guard for the reentrant-Range deadlock fixed in
// applyUBIDeltaLocked (state.go, "FIX (audit 2026-08-16)" comment, ~line
// 7440) as part of a broader money-movement audit (2026-08-16) that also
// checked every OTHER production cs.accounts.Range call site in this
// codebase (transfer_concurrent.go, transfer_batch_concurrent.go,
// evm_storage.go x3, snapshot.go x2, and every remaining Range call in
// state.go) for the same pattern. applyUBIDeltaLocked was the only
// violation found; every other Range callback either never calls back into
// cs.accounts at all, or only calls functions (saveAccountToDBCtx,
// SaveStorageSlot, effectiveBalance, accountLeaf, humanAEQWealthLocked) that
// themselves never touch cs.accounts. This test cannot re-verify a static
// analysis result, but it CAN keep the one real, previously-broken code path
// honest going forward.
//
// THE MECHANISM: sharded_accounts.go's Range doc comment states the rule
// directly — "fn must NOT call back into sa for the same shard (Get/Set/
// Delete/Range) -- that would deadlock on the shard mutex Range is already
// holding." humanCountLocked (state.go) calls cs.accounts.Range() itself
// whenever cs.useDB is false (the file-backed fallback a node uses when
// Postgres is unavailable — and the mode every test in this package that
// doesn't opt into AEQUITAS_TPS_BENCH runs under, including this one).
// enforceWealthCapLockedCtx -> getAverageBalanceLocked -> humanCountLocked,
// and separately enforceWealthCapLockedCtx -> bootstrapMultiplierLocked ->
// humanCountLocked, so calling enforceWealthCapLockedCtx from INSIDE a
// cs.accounts.Range callback re-enters the same shard's mutex. Go's
// sync.Mutex is not reentrant, so the goroutine (and, since
// applyUBIDeltaLocked runs under cs.mu.Lock(), every other caller of cs.mu
// on this node) hangs permanently.
//
// The current code snapshots humans into a plain slice first (a pure Range
// with no nested calls), then does the wealth-cap/save work in a SECOND,
// ordinary for-loop outside Range — this test proves that holds by actually
// calling ApplyUBIDelta under the exact condition (cs.useDB == false, at
// least one human, an amount that reaches enforceWealthCapLockedCtx for
// every one of them) that reintroduces the hang if the fix is ever reverted
// or a similar Range-callback change is made elsewhere in this function.
//
// This is a regression guard, not an attack test: PASS is the expected,
// correct outcome given the code as it stands today. A hang (reported here
// as a test failure after a bounded wait, never as an indefinite hang of the
// whole test binary) is what reintroducing the bug looks like.
func TestApplyUBIDelta_NoDBMode_DoesNotDeadlockOnReentrantRange(t *testing.T) {
	cs := newTestState()
	const humans = 10
	addrs := make([]string, humans)
	for i := 0; i < humans; i++ {
		addrs[i] = distTestAddr(9000 + i)
		addHuman(cs, addrs[i], 1000)
	}

	// Run off the test goroutine so a genuine deadlock is reported as a
	// bounded test failure instead of hanging the whole `go test` invocation
	// for its full default timeout.
	done := make(chan error, 1)
	go func() {
		done <- cs.ApplyUBIDelta(50, time.Now().Unix())
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ApplyUBIDelta returned an error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ApplyUBIDelta deadlocked — humanCountLocked's cs.accounts.Range call (reached via enforceWealthCapLockedCtx -> getAverageBalanceLocked/bootstrapMultiplierLocked) re-entered a shard mutex already held by an outer cs.accounts.Range. This is the exact bug fixed in applyUBIDeltaLocked (state.go, 'FIX (audit 2026-08-16)') — if this test hangs, that fix has been undone or a similar reentrant call was added elsewhere in the function.")
	}

	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for i, addr := range addrs {
		acc, ok := cs.accounts.Get(addr)
		if !ok {
			t.Fatalf("account %d (%s) vanished", i, addr)
		}
		if acc.Balance.Float() != 1050 {
			t.Errorf("account %d (%s) balance = %v, want 1050 (1000 + 50 UBI)", i, addr, acc.Balance.Float())
		}
	}
}
