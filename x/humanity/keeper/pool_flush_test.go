package keeper

import (
	"sync"
	"testing"
	"time"
)

// referenceAccountXORFromRange is referenceAccountXOR's twin for a real-DB
// ChainState: cs.accounts is fully warm for every address this test touches
// (no eviction at this scale), so ranging it gives the same full recompute
// referenceAccountXOR gives for the no-DB test state.
func referenceAccountXORFromRange(cs *ChainState) [32]byte {
	var x [32]byte
	cs.accounts.Range(func(_ string, a *AccountState) bool {
		xorInto(&x, accountLeaf(a))
		return true
	})
	return x
}

// TestDistributeSwapFee_StateRootCorrectBeforeAnyFlush is the core
// correctness proof for SCALING_ARCHITECTURE.md Phase 3: accountSetXOR (and
// therefore StateRoot) must reflect a pool credit the INSTANT
// distributeSwapFee applies it in memory, never waiting on the deferred
// Postgres flush -- consensus depends on this being true on every node
// regardless of that node's own local flush timing. Also confirms the
// Postgres write really IS deferred (not accidentally still synchronous),
// by reading the DB row directly and finding it unchanged immediately after
// the credit.
func TestDistributeSwapFee_StateRootCorrectBeforeAnyFlush(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	cs := NewChainState("unused-pool-flush-test-1.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}

	cs.mu.Lock()
	before := readPoolBalanceFromDBLocked(t, cs, validatorsPoolAddr)
	if err := cs.distributeSwapFee(10, true); err != nil {
		cs.mu.Unlock()
		t.Fatalf("distributeSwapFee: %v", err)
	}
	// The DB row must NOT reflect the credit yet -- this is the entire
	// point of deferring it. Read while still holding cs.mu so nothing else
	// can race a flush in between (the flush worker itself needs cs.mu).
	afterDBStillOld := readPoolBalanceFromDBLocked(t, cs, validatorsPoolAddr)
	if afterDBStillOld != before {
		t.Errorf("expected the Postgres row to be unchanged immediately after distributeSwapFee (deferred write), got %v -> %v", before, afterDBStillOld)
	}
	// But accountSetXOR must ALREADY be correct -- a full recompute over
	// cs.accounts (which DOES have the new in-memory balance) must match
	// the live accumulator exactly, with zero dependency on any flush.
	full := referenceAccountXORFromRange(cs)
	if cs.accountSetXOR != full {
		t.Fatal("accountSetXOR diverged from a full recompute immediately after distributeSwapFee, before any DB flush -- StateRoot would be wrong on any node that computes it before its own flush timer fires")
	}
	cs.mu.Unlock()

	// Clean up: flush for real so this test doesn't leave a dangling dirty
	// flag/goroutine state that could confuse a later test in the same run.
	cs.FlushPoolAccountsNow()
}

// readPoolBalanceFromDBLocked reads a pool address's CURRENT balance
// directly from Postgres, bypassing cs.accounts entirely -- the only way to
// observe whether a credit has actually been flushed yet, as opposed to
// just applied in-memory (see pool_flush.go's whole point). Callers must
// already hold cs.mu (the DB read itself needs no lock -- cs.db is a
// connection pool safe for concurrent use -- but cs.mu must stay held around
// it here so no concurrent flush can run between a credit and this read).
func readPoolBalanceFromDBLocked(t *testing.T, cs *ChainState, addr string) float64 {
	t.Helper()
	var bal float64
	err := cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE address = $1`, addr).Scan(&bal)
	if err != nil {
		return 0
	}
	return bal
}

// TestPoolFlush_PersistsAccumulatedCreditsToDB proves the deferred write
// eventually lands, and that several credits between flushes correctly
// collapse into ONE persisted value equal to their sum -- not just the last
// one, and not lost entirely.
func TestPoolFlush_PersistsAccumulatedCreditsToDB(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	cs := NewChainState("unused-pool-flush-test-2.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}

	cs.mu.Lock()
	before := readPoolBalanceFromDBLocked(t, cs, ubiPoolAddr)
	cs.mu.Unlock()

	const numCredits = 5
	const feePerCredit = 7.0
	for i := 0; i < numCredits; i++ {
		cs.mu.Lock()
		if err := cs.distributeSwapFee(feePerCredit, true); err != nil {
			cs.mu.Unlock()
			t.Fatalf("distributeSwapFee %d: %v", i, err)
		}
		cs.mu.Unlock()
	}

	cs.FlushPoolAccountsNow()

	cs.mu.Lock()
	after := readPoolBalanceFromDBLocked(t, cs, ubiPoolAddr)
	cs.mu.Unlock()

	wantDelta := numCredits * feePerCredit * 0.20 // ubiPoolAddr's 20% share
	gotDelta := after - before
	if diff := gotDelta - wantDelta; diff < -1e-6 || diff > 1e-6 {
		t.Errorf("ubi pool DB balance after flush: want delta %.6f (5 credits x 7 x 20%%), got %.6f (before=%v after=%v)", wantDelta, gotDelta, before, after)
	}
}

// TestPoolFlush_NoOpWhenNotDirty proves flushing with nothing pending is
// always safe -- both before the flush worker has ever been started (a
// fresh ChainState that has never seen a fee event) and after a flush has
// already cleared the dirty flag.
func TestPoolFlush_NoOpWhenNotDirty(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	cs := NewChainState("unused-pool-flush-test-3.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}

	// Never touched distributeSwapFee at all -- must not panic, must not
	// start a worker, must not error.
	cs.FlushPoolAccountsNow()

	cs.mu.Lock()
	if err := cs.distributeSwapFee(3, true); err != nil {
		cs.mu.Unlock()
		t.Fatalf("distributeSwapFee: %v", err)
	}
	cs.mu.Unlock()
	cs.FlushPoolAccountsNow() // clears dirty
	if cs.poolFlushDirty.Load() {
		t.Fatal("expected dirty flag cleared after a successful flush")
	}
	cs.FlushPoolAccountsNow() // second call: nothing pending, must still be safe
}

// TestPoolFlush_ConcurrentCreditsNoLostUpdates is the -race regression guard
// for the new mechanism: many goroutines credit the pools concurrently
// (each holding cs.mu for its own distributeSwapFee call, exactly like real
// concurrent transfers would), then one final flush -- the persisted DB
// balance must equal the EXACT sum of every credit, proving the deferred
// in-memory accumulation drops nothing under concurrency.
func TestPoolFlush_ConcurrentCreditsNoLostUpdates(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	cs := NewChainState("unused-pool-flush-test-4.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}

	cs.mu.Lock()
	before := readPoolBalanceFromDBLocked(t, cs, treasuryPoolAddr)
	cs.mu.Unlock()

	const numGoroutines = 50
	const feePerCredit = 2.0
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			cs.mu.Lock()
			if err := cs.distributeSwapFee(feePerCredit, true); err != nil {
				t.Errorf("distributeSwapFee: %v", err)
			}
			cs.mu.Unlock()
		}()
	}
	wg.Wait()

	cs.FlushPoolAccountsNow()

	cs.mu.Lock()
	after := readPoolBalanceFromDBLocked(t, cs, treasuryPoolAddr)
	full := referenceAccountXORFromRange(cs)
	xorMatches := cs.accountSetXOR == full
	cs.mu.Unlock()

	if !xorMatches {
		t.Error("accountSetXOR diverged from a full recompute after concurrent credits + flush")
	}
	wantDelta := numGoroutines * feePerCredit * 0.10 // treasuryPoolAddr's 10% share
	gotDelta := after - before
	if diff := gotDelta - wantDelta; diff < -1e-6 || diff > 1e-6 {
		t.Errorf("treasury pool DB balance: want delta %.6f (%d x %v x 10%%), got %.6f -- a lost or duplicated update under concurrency", wantDelta, numGoroutines, feePerCredit, gotDelta)
	}
}

// TestPoolFlush_ThroughputComparisonVsSynchronous directly isolates the one
// thing SCALING_ARCHITECTURE.md Phase 3 changes: N pool-credit events used
// to each pay their own synchronous Postgres round trip (saveAccountsToDBBatch
// called once per event); now they share ONE round trip via the deferred
// flush. The end-to-end TPS benchmark (TestSimulateMaxTPS_Ingestion) does
// NOT exercise this at all -- its fresh, continuously-active accounts never
// accrue demurrage within the test's ~30s runtime (demurrageGracePeriodSeconds
// is 90 days), so it cannot honestly be cited as evidence for this specific
// change. This test measures the mechanism directly instead: N sequential
// "credit + synchronous batch save" calls (reproducing exactly what
// distributeSwapFee did before Phase 3) versus N sequential distributeSwapFee
// calls (which internally defer) plus one trailing flush, both against the
// same real Postgres connection, same account set, same total work.
func TestPoolFlush_ThroughputComparisonVsSynchronous(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	cs := NewChainState("unused-pool-flush-test-5.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}

	const n = 200
	const feePerCredit = 1.0

	// Warm the 4 pool accounts once so both phases below start from an
	// identical, already-loaded state (ensureAccountLoaded's one-time cold
	// DB read shouldn't count against either side).
	cs.mu.Lock()
	if err := cs.distributeSwapFee(0.000001, true); err != nil {
		cs.mu.Unlock()
		t.Fatalf("warmup distributeSwapFee: %v", err)
	}
	cs.mu.Unlock()
	cs.FlushPoolAccountsNow()

	pools := []struct {
		addr string
		frac float64
	}{
		{validatorsPoolAddr, 0.40}, {lpPoolAddr, 0.30}, {ubiPoolAddr, 0.20}, {treasuryPoolAddr, 0.10},
	}

	// Phase: N synchronous saves, reproducing pre-Phase-3 distributeSwapFee
	// exactly (mutate + saveAccountsToDBBatch on every single call).
	syncStart := time.Now()
	cs.mu.Lock()
	for i := 0; i < n; i++ {
		accs := make([]*AccountState, 0, 4)
		for _, p := range pools {
			acc, _ := cs.accounts.Get(p.addr)
			acc.Balance = acc.Balance.Add(NewDecimal(feePerCredit * p.frac))
			accs = append(accs, acc)
		}
		if err := cs.saveAccountsToDBBatch(accs); err != nil {
			cs.mu.Unlock()
			t.Fatalf("synchronous saveAccountsToDBBatch %d: %v", i, err)
		}
	}
	cs.mu.Unlock()
	syncElapsed := time.Since(syncStart)

	// Phase: N deferred credits via the real Phase 3 path, one trailing
	// flush -- exactly how live traffic calls distributeSwapFee today.
	deferredStart := time.Now()
	cs.mu.Lock()
	for i := 0; i < n; i++ {
		if err := cs.distributeSwapFee(feePerCredit, true); err != nil {
			cs.mu.Unlock()
			t.Fatalf("distributeSwapFee %d: %v", i, err)
		}
	}
	cs.mu.Unlock()
	cs.FlushPoolAccountsNow()
	deferredElapsed := time.Since(deferredStart)

	t.Logf("%d pool-credit events: synchronous (pre-Phase-3 pattern) = %v (%v/event); deferred (Phase 3) = %v (%v/event)",
		n, syncElapsed, syncElapsed/n, deferredElapsed, deferredElapsed/n)

	if deferredElapsed >= syncElapsed {
		t.Errorf("expected the deferred path to be faster than the synchronous path (fewer Postgres round trips for the same %d credits), got deferred=%v >= synchronous=%v", n, deferredElapsed, syncElapsed)
	}
}
