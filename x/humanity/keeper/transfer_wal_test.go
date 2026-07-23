package keeper

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// transferConcurrentWAL/recoverFromWAL require a real Postgres connection
// (the DB is this design's async reconciliation target — see
// initWALIfEnabled's own comment), so every test in this file is opt-in
// real-DB, matching this project's existing *_db_test.go convention.

// newWALTestState opens a fresh, WAL-enabled ChainState against walPath
// (created if it doesn't exist) and the shared bench DB. t.Setenv resets
// AEQUITAS_WAL_ENABLED/AEQUITAS_WAL_PATH automatically at test end.
//
// Registers t.Cleanup to stop the background flush worker (see
// stopWALFlushWorkerForTest's own comment) — without this, EVERY instance
// this helper creates leaks a goroutine that keeps ticking (and, if its
// queue is non-empty, keeps writing to the shared bench DB) after the test
// function returns, at an unpredictable wall-clock moment relative to
// whichever LATER test happens to be running. Confirmed to actually happen
// in this file's own development: the crash-recovery test's deliberately-
// abandoned csA leaked exactly this way and corrupted a later concurrency
// test's expected starting balance. A test that deliberately "crashes" an
// instance mid-test (not just at the very end) must ALSO call
// stopWALFlushWorkerForTest() explicitly at that point — t.Cleanup alone
// only protects against leakage into OTHER tests, not against the
// abandoned instance's worker firing during the REST of the same test
// function (see the crash-recovery tests below for that explicit call).
func newWALTestState(t *testing.T, walPath string) *ChainState {
	t.Helper()
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL must point at a disposable local Postgres database")
	}
	t.Setenv("AEQUITAS_WAL_ENABLED", "1")
	t.Setenv("AEQUITAS_WAL_PATH", walPath)
	cs := NewChainState("unused-wal-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection (cs.useDB == false) — check DATABASE_URL")
	}
	if cs.wal == nil {
		t.Fatal("expected cs.wal to be set (AEQUITAS_WAL_ENABLED=1) — WAL fast path did not activate, check the [WAL] log lines above")
	}
	// Stop the flush worker before closing the DB pool (order matters: a
	// worker tick racing an already-closed *sql.DB would just fail and log
	// noise, harmless but avoidable). Closing cs.db itself matters for THIS
	// file specifically, unlike most other DB-backed tests in this package:
	// the concurrent WAL tests below genuinely open many real connections
	// under load (up to MaxOpenConns=20 each), and those stay open for the
	// rest of the test BINARY's lifetime (ConnMaxLifetime=30min) unless
	// explicitly closed — confirmed to exhaust Postgres's max_connections
	// (100) when the full package suite runs in one process.
	t.Cleanup(func() {
		cs.stopWALFlushWorkerForTest()
		cs.db.Close()
	})
	return cs
}

func TestTransferConcurrentWAL_EligibleTransferSucceeds(t *testing.T) {
	dir := t.TempDir()
	truncateDistTestTables(t)
	cs := newWALTestState(t, filepath.Join(dir, "test.wal"))
	from := distTestAddr(801)
	to := distTestAddr(802)
	seedConcurrentTestAccount(t, cs, from, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, cs, to, 0, time.Now().Unix())

	fromLost, toLost, applied, err := cs.transferConcurrentWAL(from, to, 30, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 30, TxHash: "0xwaltest801"})
	if !applied {
		t.Fatal("want applied=true for a warm, cap-safe, decay-free transfer")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fromLost != 0 || toLost != 0 {
		t.Errorf("want zero demurrage loss, got fromLost=%v toLost=%v", fromLost, toLost)
	}

	cs.mu.RLock()
	fromAcc, _ := cs.accounts.Get(from)
	toAcc, _ := cs.accounts.Get(to)
	cs.mu.RUnlock()
	if fromAcc.Balance.Float() != 70 {
		t.Errorf("sender balance = %v, want 70", fromAcc.Balance.Float())
	}
	if toAcc.Balance.Float() != 30 {
		t.Errorf("recipient balance = %v, want 30", toAcc.Balance.Float())
	}
	if fromAcc.WALSeq == 0 || toAcc.WALSeq == 0 {
		t.Error("want both accounts' WALSeq advanced past zero after a WAL-durable transfer")
	}

	// Before any flush: Postgres must NOT yet reflect this transfer — that
	// lag is the explicit, documented consequence of this design.
	var dbBalance float64
	if err := cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, from).Scan(&dbBalance); err != nil {
		t.Fatalf("sender row not found in Postgres: %v", err)
	}
	if dbBalance != 100 {
		t.Errorf("Postgres balance BEFORE flush = %v, want unchanged 100 (async reconciliation hasn't run yet)", dbBalance)
	}

	cs.FlushWALNow()
	if err := cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, from).Scan(&dbBalance); err != nil {
		t.Fatalf("sender row not found in Postgres after flush: %v", err)
	}
	if dbBalance != 70 {
		t.Errorf("Postgres balance AFTER flush = %v, want 70", dbBalance)
	}
	var dbTo float64
	if err := cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, to).Scan(&dbTo); err != nil {
		t.Fatalf("recipient row not found in Postgres after flush: %v", err)
	}
	if dbTo != 30 {
		t.Errorf("Postgres recipient balance AFTER flush = %v, want 30", dbTo)
	}
	var queued int
	if err := cs.db.QueryRow(`SELECT count(*) FROM pending_txs WHERE tx_json::json->>'tx_hash' = $1`, "0xwaltest801").Scan(&queued); err != nil {
		t.Fatalf("checking outbox: %v", err)
	}
	if queued != 1 {
		t.Errorf("want exactly 1 outbox row after flush, got %d", queued)
	}
}

func TestTransferConcurrentWAL_ColdAccountFallsBack(t *testing.T) {
	dir := t.TempDir()
	truncateDistTestTables(t)
	cs := newWALTestState(t, filepath.Join(dir, "test.wal"))
	from := distTestAddr(803)
	to := distTestAddr(804) // never seeded

	seedConcurrentTestAccount(t, cs, from, 100, time.Now().Unix())

	_, _, applied, err := cs.transferConcurrentWAL(from, to, 10, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 10})
	if applied {
		t.Fatal("want applied=false for a cold recipient")
	}
	if err != nil {
		t.Errorf("want nil error when falling back, got %v", err)
	}
}

func TestTransferConcurrentWAL_InsufficientBalanceErrors(t *testing.T) {
	dir := t.TempDir()
	truncateDistTestTables(t)
	cs := newWALTestState(t, filepath.Join(dir, "test.wal"))
	from := distTestAddr(805)
	to := distTestAddr(806)
	seedConcurrentTestAccount(t, cs, from, 5, time.Now().Unix())
	seedConcurrentTestAccount(t, cs, to, 0, time.Now().Unix())

	_, _, applied, err := cs.transferConcurrentWAL(from, to, 100, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 100})
	if !applied {
		t.Fatal("want applied=true for an insufficient-balance transfer (a real, decided outcome)")
	}
	if err == nil {
		t.Fatal("want a non-nil error for insufficient balance")
	}
	// Must be a true no-op: an error after the eligibility checks but before
	// any WAL append must never happen here (insufficient balance is
	// rejected BEFORE wal.Append), so nothing should have moved.
	cs.mu.RLock()
	fromAcc, _ := cs.accounts.Get(from)
	cs.mu.RUnlock()
	if fromAcc.Balance.Float() != 5 {
		t.Errorf("sender balance changed despite an insufficient-balance error: got %v, want 5", fromAcc.Balance.Float())
	}
}

func TestTransferConcurrentWAL_NotEnabledReturnsFalse(t *testing.T) {
	// No t.Setenv here — cs.wal must be nil by default, proving this whole
	// path is inert unless explicitly opted into.
	cs := &ChainState{accounts: newShardedAccounts()}
	_, _, applied, err := cs.transferConcurrentWAL(distTestAddr(1), distTestAddr(2), 10, Transaction{})
	if applied {
		t.Fatal("want applied=false when cs.wal is nil (AEQUITAS_WAL_ENABLED unset)")
	}
	if err != nil {
		t.Errorf("want nil error, got %v", err)
	}
}

// TestTransferConcurrentWAL_CrashRecovery_UnflushedTransfersReconstructed is
// this file's core safety proof: transfers applied via the WAL fast path,
// durable in the WAL file but NEVER reconciled to Postgres (simulating a
// restart before the async flush worker caught up), must be fully and
// correctly reconstructed by a FRESH ChainState opening the same WAL path —
// both in memory immediately, and in Postgres after that fresh instance's
// own next flush.
func TestTransferConcurrentWAL_CrashRecovery_UnflushedTransfersReconstructed(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "test.wal")
	truncateDistTestTables(t)

	csA := newWALTestState(t, walPath)
	from := distTestAddr(810)
	to := distTestAddr(811)
	seedConcurrentTestAccount(t, csA, from, 1000, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, to, 0, time.Now().Unix())

	// Two WAL-durable transfers, deliberately NEVER flushed to Postgres —
	// this is the "process restarted before the async worker's next tick"
	// scenario.
	if _, _, applied, err := csA.transferConcurrentWAL(from, to, 100, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 100, TxHash: "0xcrash1"}); !applied || err != nil {
		t.Fatalf("first transfer: applied=%v err=%v", applied, err)
	}
	if _, _, applied, err := csA.transferConcurrentWAL(from, to, 50, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 50, TxHash: "0xcrash2"}); !applied || err != nil {
		t.Fatalf("second transfer: applied=%v err=%v", applied, err)
	}

	// Confirm Postgres genuinely does NOT have these yet (proves the test
	// is exercising the real gap, not a no-op).
	var dbBalanceBefore float64
	if err := csA.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, from).Scan(&dbBalanceBefore); err != nil {
		t.Fatalf("sender row not found: %v", err)
	}
	if dbBalanceBefore != 1000 {
		t.Fatalf("test setup bug: Postgres already reflects the transfer (dbBalanceBefore=%v) — recovery test would be meaningless", dbBalanceBefore)
	}

	// "Crash": close csA's WAL cleanly (releases the file for a fresh
	// Open), WITHOUT ever flushing. csA itself is simply abandoned — real
	// production code never keeps using a ChainState after this point,
	// matching an actual process restart. stopWALFlushWorkerForTest here
	// (not just via t.Cleanup) is required, not optional: csA's queue
	// still has 2 unflushed items by design, and its background worker
	// would otherwise flush them into the shared bench DB at an
	// unpredictable moment during the REST of this test — see
	// newWALTestState's own comment for the leak this fixes (confirmed to
	// actually happen during this file's development).
	csA.stopWALFlushWorkerForTest()
	if err := csA.wal.Close(); err != nil {
		t.Fatalf("closing WAL: %v", err)
	}

	csB := newWALTestState(t, walPath)
	// Recovery already ran inside NewChainState (via initWALIfEnabled) by
	// the time newWALTestState returns — assert the reconstructed state.
	csB.mu.RLock()
	fromAcc, _ := csB.accounts.Get(from)
	toAcc, _ := csB.accounts.Get(to)
	csB.mu.RUnlock()
	if fromAcc.Balance.Float() != 850 { // 1000 - 100 - 50
		t.Errorf("recovered sender balance = %v, want 850", fromAcc.Balance.Float())
	}
	if toAcc.Balance.Float() != 150 { // 0 + 100 + 50
		t.Errorf("recovered recipient balance = %v, want 150", toAcc.Balance.Float())
	}

	// Recovery must also have re-queued both transfers for reconciliation —
	// flush now and confirm Postgres catches up to the correct final state.
	csB.FlushWALNow()
	var dbFrom, dbTo float64
	if err := csB.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, from).Scan(&dbFrom); err != nil {
		t.Fatalf("sender row not found after recovery flush: %v", err)
	}
	if err := csB.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, to).Scan(&dbTo); err != nil {
		t.Fatalf("recipient row not found after recovery flush: %v", err)
	}
	if dbFrom != 850 || dbTo != 150 {
		t.Errorf("Postgres after recovery+flush = (%v, %v), want (850, 150)", dbFrom, dbTo)
	}
	var queuedCount int
	if err := csB.db.QueryRow(`SELECT count(*) FROM pending_txs WHERE tx_json::json->>'tx_hash' IN ('0xcrash1','0xcrash2')`).Scan(&queuedCount); err != nil {
		t.Fatalf("checking outbox: %v", err)
	}
	if queuedCount != 2 {
		t.Errorf("want exactly 2 outbox rows after recovery+flush, got %d", queuedCount)
	}
}

// TestTransferConcurrentWAL_CrashRecovery_AutoFlushesWithoutManualTrigger is
// the regression test for a real bug found via a local end-to-end crash
// drill (real aequitasd process, real Postgres, kill -9, restart — not just
// this package's own unit tests): recoverFromWAL used to append reapplied,
// still-unflushed items straight to cs.walFlushQueue instead of going
// through enqueueWALFlushLocked, which ALSO calls
// ensureWALFlushWorkerStarted(). The sibling test above
// (UnflushedTransfersReconstructed) calls csB.FlushWALNow() manually right
// after recovery — which masked this exact gap, since a manual flush works
// regardless of whether the periodic background worker ever started. A real
// restarted node never calls FlushWALNow() itself; it relies entirely on
// that background ticker. Confirmed live in the local drill: Postgres
// stayed stale indefinitely (checked repeatedly over 15+ seconds, no other
// transfer touching either address) until the fix (recoverFromWAL now calls
// enqueueWALFlushLocked, same as the live transfer path) made the worker
// start automatically. This test therefore deliberately does NOT call
// FlushWALNow() — it only waits (polling, well beyond walFlushInterval) to
// prove the worker resumes on its own after a recovery with nothing else
// triggering it.
func TestTransferConcurrentWAL_CrashRecovery_AutoFlushesWithoutManualTrigger(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "test.wal")
	truncateDistTestTables(t)

	csA := newWALTestState(t, walPath)
	from := distTestAddr(820)
	to := distTestAddr(821)
	seedConcurrentTestAccount(t, csA, from, 1000, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, to, 0, time.Now().Unix())

	if _, _, applied, err := csA.transferConcurrentWAL(from, to, 42, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 42, TxHash: "0xautoflush1"}); !applied || err != nil {
		t.Fatalf("transfer: applied=%v err=%v", applied, err)
	}

	// "Crash": abandon csA without ever flushing, exactly like the sibling
	// test above.
	csA.stopWALFlushWorkerForTest()
	if err := csA.wal.Close(); err != nil {
		t.Fatalf("closing WAL: %v", err)
	}

	csB := newWALTestState(t, walPath)
	// Deliberately NO csB.FlushWALNow() call here -- that is the entire
	// point of this test. Only the background worker (started as a side
	// effect of recovery, if the fix is in place) should make this happen.
	deadline := time.Now().Add(3 * time.Second)
	var dbFrom float64
	for {
		if err := csB.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, from).Scan(&dbFrom); err != nil {
			t.Fatalf("sender row not found: %v", err)
		}
		if dbFrom == 958 { // 1000 - 42
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Postgres balance = %v after %s of waiting with no manual flush trigger — recovery did not restart the background flush worker (want 958)", dbFrom, 3*time.Second)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestTransferConcurrentWAL_CrashRecovery_IdempotentOnRepeatedReplay proves
// the actual safety property this whole design hinges on: replaying WAL
// records that are ALREADY reflected in Postgres (via WALSeq) must never
// double-apply them. Without this, EVERY restart of a WAL-enabled node
// would double-debit/credit every transfer that happened to be durable but
// unflushed at any prior restart — this is the single most important
// invariant in this file.
func TestTransferConcurrentWAL_CrashRecovery_IdempotentOnRepeatedReplay(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "test.wal")
	truncateDistTestTables(t)

	csA := newWALTestState(t, walPath)
	from := distTestAddr(820)
	to := distTestAddr(821)
	seedConcurrentTestAccount(t, csA, from, 1000, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, to, 0, time.Now().Unix())

	if _, _, applied, err := csA.transferConcurrentWAL(from, to, 100, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 100, TxHash: "0xidem1"}); !applied || err != nil {
		t.Fatalf("transfer: applied=%v err=%v", applied, err)
	}
	// This time DO flush before "crashing" — Postgres now genuinely
	// reflects this transfer, unlike the previous test. Stopping the worker
	// explicitly here too (belt and suspenders, see newWALTestState's own
	// comment) even though the queue is already empty at this point.
	csA.FlushWALNow()
	csA.stopWALFlushWorkerForTest()
	if err := csA.wal.Close(); err != nil {
		t.Fatalf("closing WAL: %v", err)
	}

	var dbBalanceAfterFlush float64
	if err := csA.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, from).Scan(&dbBalanceAfterFlush); err != nil {
		t.Fatalf("sender row not found: %v", err)
	}
	if dbBalanceAfterFlush != 900 {
		t.Fatalf("test setup bug: expected Postgres to already reflect the transfer (900), got %v", dbBalanceAfterFlush)
	}

	// First "restart": replays a WAL file whose only record is ALREADY
	// fully reflected in Postgres (via wal_seq). Must be a pure no-op.
	csB := newWALTestState(t, walPath)
	csB.mu.RLock()
	fromAcc, _ := csB.accounts.Get(from)
	toAcc, _ := csB.accounts.Get(to)
	csB.mu.RUnlock()
	if fromAcc.Balance.Float() != 900 || toAcc.Balance.Float() != 100 {
		t.Fatalf("after first restart: balances = (%v, %v), want (900, 100) — replay must be a no-op for an already-reconciled record", fromAcc.Balance.Float(), toAcc.Balance.Float())
	}
	csB.stopWALFlushWorkerForTest()
	if err := csB.wal.Close(); err != nil {
		t.Fatalf("closing WAL: %v", err)
	}

	// Second "restart": same file, same already-reconciled record, one more
	// time — must STILL be a no-op (proves idempotency isn't a one-shot
	// accident of csB's particular in-memory state, but a real property of
	// WALSeq read fresh from Postgres each time).
	csC := newWALTestState(t, walPath)
	csC.mu.RLock()
	fromAcc, _ = csC.accounts.Get(from)
	toAcc, _ = csC.accounts.Get(to)
	csC.mu.RUnlock()
	if fromAcc.Balance.Float() != 900 || toAcc.Balance.Float() != 100 {
		t.Fatalf("after second restart: balances = (%v, %v), want (900, 100) — double-application bug", fromAcc.Balance.Float(), toAcc.Balance.Float())
	}
	var dbFrom float64
	if err := csC.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, from).Scan(&dbFrom); err != nil {
		t.Fatalf("sender row not found: %v", err)
	}
	if dbFrom != 900 {
		t.Errorf("Postgres balance after two idempotent restarts = %v, want 900 (unchanged)", dbFrom)
	}
	var queuedCount int
	if err := csC.db.QueryRow(`SELECT count(*) FROM pending_txs WHERE tx_json::json->>'tx_hash' = $1`, "0xidem1").Scan(&queuedCount); err != nil {
		t.Fatalf("checking outbox: %v", err)
	}
	if queuedCount != 1 {
		t.Errorf("want exactly 1 outbox row total across all restarts, got %d — idempotency must also prevent a duplicate outbox insert", queuedCount)
	}
}

// TestTransferConcurrentWAL_ConcurrentRingTransfersConserveBalance is the
// property-based concurrency proof for the WAL-durable path, mirroring
// TestTransferConcurrent_ConcurrentRingTransfersConserveBalance: many
// goroutines, ring topology (adjacent goroutines share one address, real
// TryLockAddrs contention), -race clean, exact balance conservation across
// whichever attempts actually applied.
func TestTransferConcurrentWAL_ConcurrentRingTransfersConserveBalance(t *testing.T) {
	dir := t.TempDir()
	truncateDistTestTables(t)
	cs := newWALTestState(t, filepath.Join(dir, "test.wal"))
	const n = 40
	const amount = 25
	addrs := make([]string, n)
	for i := 0; i < n; i++ {
		addrs[i] = distTestAddr(900 + i)
		seedConcurrentTestAccount(t, cs, addrs[i], 1000, time.Now().Unix())
	}

	var wg sync.WaitGroup
	applied := make([]bool, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			from := addrs[i]
			to := addrs[(i+1)%n]
			_, _, ok, err := cs.transferConcurrentWAL(from, to, amount, Transaction{
				Type: "transfer", Wallet: from, To: to, Amount: amount, TxHash: fmt.Sprintf("0xwalring%03d", i),
			})
			if err != nil {
				t.Errorf("goroutine %d: unexpected error: %v", i, err)
				return
			}
			applied[i] = ok
		}(i)
	}
	wg.Wait()
	cs.FlushWALNow()

	cs.mu.RLock()
	var total float64
	cs.accounts.Range(func(_ string, acc *AccountState) bool {
		total += acc.Balance.Float()
		return true
	})
	gotXOR := cs.accountSetXOR
	wantXOR := referenceAccountXOR(cs)
	cs.mu.RUnlock()

	if total != float64(n)*1000 {
		t.Fatalf("total balance after %d concurrent WAL ring transfer attempts = %v, want %v", n, total, float64(n)*1000)
	}
	if gotXOR != wantXOR {
		t.Fatal("accountSetXOR diverged from a full recompute after concurrent transferConcurrentWAL calls")
	}

	want := make([]float64, n)
	for i := range want {
		want[i] = 1000
	}
	for i := 0; i < n; i++ {
		if applied[i] {
			want[i] -= amount
			want[(i+1)%n] += amount
		}
	}
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for i, addr := range addrs {
		acc, _ := cs.accounts.Get(addr)
		if acc.Balance.Float() != want[i] {
			t.Errorf("account %s balance = %v, want %v given which ring attempts actually applied", addr, acc.Balance.Float(), want[i])
		}
	}
}

// TestTransferConcurrentWAL_ViaTransferAtomic_RingConservesBalance is the
// end-to-end counterpart: drives the same ring topology through
// TransferAtomic (the real production entry point) with WAL enabled,
// proving TransferAtomic's cs.wal-branch correctly routes every eligible
// transfer through the WAL path and conserves balance exactly.
func TestTransferConcurrentWAL_ViaTransferAtomic_RingConservesBalance(t *testing.T) {
	dir := t.TempDir()
	truncateDistTestTables(t)
	cs := newWALTestState(t, filepath.Join(dir, "test.wal"))
	const n = 30
	const amount = 25
	addrs := make([]string, n)
	for i := 0; i < n; i++ {
		addrs[i] = distTestAddr(950 + i)
		seedConcurrentTestAccount(t, cs, addrs[i], 1000, time.Now().Unix())
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			from := addrs[i]
			to := addrs[(i+1)%n]
			tmpl := Transaction{Type: "transfer", Wallet: from, To: to, Amount: amount, TxHash: fmt.Sprintf("0xwalatomicring%03d", i)}
			if _, _, err := cs.TransferAtomic(from, to, amount, tmpl); err != nil {
				t.Errorf("goroutine %d: TransferAtomic failed: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	cs.FlushWALNow()

	cs.mu.RLock()
	defer cs.mu.RUnlock()
	var total float64
	cs.accounts.Range(func(_ string, acc *AccountState) bool {
		total += acc.Balance.Float()
		return true
	})
	if total != float64(n)*1000 {
		t.Fatalf("total balance after %d concurrent TransferAtomic (WAL-enabled) ring transfers = %v, want %v", n, total, float64(n)*1000)
	}
	for _, addr := range addrs {
		acc, _ := cs.accounts.Get(addr)
		if acc.Balance.Float() != 1000 {
			t.Errorf("account %s balance = %v, want 1000 (ring net-zero)", addr, acc.Balance.Float())
		}
	}
}

// TestFlushWALBatch_FailureRequeuesForRetry proves a failed flush (e.g. a
// transient Postgres error) never silently drops queued items — they must
// remain queued for the next tick, matching flushEVMMirrorDirty's own
// "leftover state is picked up later" precedent. Simulated by closing
// cs.db out from under an already-populated queue, which makes the flush's
// own Begin() fail deterministically.
func TestFlushWALBatch_FailureRequeuesForRetry(t *testing.T) {
	dir := t.TempDir()
	truncateDistTestTables(t)
	cs := newWALTestState(t, filepath.Join(dir, "test.wal"))
	from := distTestAddr(830)
	to := distTestAddr(831)
	seedConcurrentTestAccount(t, cs, from, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, cs, to, 0, time.Now().Unix())

	if _, _, applied, err := cs.transferConcurrentWAL(from, to, 10, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 10, TxHash: "0xretry1"}); !applied || err != nil {
		t.Fatalf("transfer: applied=%v err=%v", applied, err)
	}

	cs.walFlushMu.Lock()
	queueLenBefore := len(cs.walFlushQueue)
	cs.walFlushMu.Unlock()
	if queueLenBefore != 1 {
		t.Fatalf("test setup bug: want 1 queued item, got %d", queueLenBefore)
	}

	badDB := cs.db
	if err := badDB.Close(); err != nil {
		t.Fatalf("closing db for failure simulation: %v", err)
	}
	cs.flushWALQueue() // must fail (db closed) and requeue, not panic or drop

	cs.walFlushMu.Lock()
	queueLenAfter := len(cs.walFlushQueue)
	cs.walFlushMu.Unlock()
	if queueLenAfter != 1 {
		t.Fatalf("want the failed batch requeued (len=1), got len=%d — a failed flush must never lose an item", queueLenAfter)
	}
}

// TestTransferConcurrentWAL_ConcurrentMixedWithSlowPath_PostgresStaysConsistent
// is the regression test for a real lost-update bug found and fixed during
// this file's own development: flushWALBatch originally snapshotted account
// balances under cs.mu.RLock(), released it, then wrote to Postgres
// separately — leaving a window where a concurrent SLOW-PATH write (guarded
// by the unrelated optimistic-lock `version` column, which the WAL flush's
// `wal_seq` guard knows nothing about) could land BETWEEN the snapshot and
// the flush's own write, and then get silently clobbered back to the
// flush's stale snapshot value. The fix holds cs.mu.Lock() for the flush's
// entire snapshot-then-write sequence, making it atomic relative to every
// cs.mu-based writer.
//
// This drives real concurrent load through BOTH paths on an OVERLAPPING
// address pool — transferConcurrentWAL (fast, WAL-durable) and cs.Transfer
// (slow, synchronous, unconditional — bypasses eligibility checks entirely
// so it reliably exercises the cs.mu.Lock()-based path regardless of
// demurrage/cap state) — interleaved with periodic flushes, then asserts
// Postgres matches the in-memory ledger EXACTLY for every account. This is
// a stronger check than balance conservation: conservation alone would not
// catch "correct total, but this one row's DB value is stale" the way a
// per-account Postgres-vs-memory equality check does.
func TestTransferConcurrentWAL_ConcurrentMixedWithSlowPath_PostgresStaysConsistent(t *testing.T) {
	dir := t.TempDir()
	truncateDistTestTables(t)
	cs := newWALTestState(t, filepath.Join(dir, "test.wal"))
	const numAddrs = 10
	addrs := make([]string, numAddrs)
	for i := range addrs {
		addrs[i] = distTestAddr(840 + i)
		seedConcurrentTestAccount(t, cs, addrs[i], 1000, time.Now().Unix())
	}

	const rounds = 200
	var wg sync.WaitGroup
	for r := 0; r < rounds; r++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			from := addrs[r%numAddrs]
			to := addrs[(r*7+3)%numAddrs]
			if from == to {
				to = addrs[(r*7+5)%numAddrs]
			}
			if r%2 == 0 {
				// Fast, WAL-durable path.
				_, _, _, err := cs.transferConcurrentWAL(from, to, 1, Transaction{
					Type: "transfer", Wallet: from, To: to, Amount: 1, TxHash: fmt.Sprintf("0xmixedwal%04d", r),
				})
				if err != nil && err.Error() != "insufficient balance" {
					t.Errorf("round %d (WAL path): unexpected error: %v", r, err)
				}
			} else {
				// Slow, synchronous, cs.mu.Lock()-based path -- unconditional,
				// deliberately bypassing eligibility checks so this reliably
				// exercises the exact code path the fix above targets.
				if _, _, err := cs.Transfer(from, to, 1); err != nil && err.Error() != "insufficient balance" {
					t.Errorf("round %d (slow path): unexpected error: %v", r, err)
				}
			}
			// Occasionally flush concurrently with still-running transfers --
			// exactly the interleaving the fixed bug depended on.
			if r%17 == 0 {
				cs.FlushWALNow()
			}
		}(r)
	}
	wg.Wait()

	// Drain any remaining queued items (retries included) before comparing.
	for i := 0; i < 10; i++ {
		cs.walFlushMu.Lock()
		empty := len(cs.walFlushQueue) == 0
		cs.walFlushMu.Unlock()
		if empty {
			break
		}
		cs.FlushWALNow()
	}

	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, addr := range addrs {
		acc, _ := cs.accounts.Get(addr)
		var dbBalance float64
		if err := cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, addr).Scan(&dbBalance); err != nil {
			t.Fatalf("account %s: row not found in Postgres: %v", addr, err)
		}
		if dbBalance != acc.Balance.Float() {
			t.Errorf("account %s: Postgres balance = %v, in-memory balance = %v -- DIVERGED (the exact lost-update class this test guards against)", addr, dbBalance, acc.Balance.Float())
		}
	}
}
