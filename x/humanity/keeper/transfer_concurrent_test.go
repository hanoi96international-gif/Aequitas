package keeper

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/lib/pq"
)

// transferConcurrent requires a real Postgres connection (it bails
// applied=false immediately when cs.db == nil — there's no DB to make
// atomic with), so every test in this file is opt-in real-DB, matching
// this project's existing *_db_test.go convention.

func newConcurrentTransferTestState(t *testing.T) *ChainState {
	t.Helper()
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL must point at a disposable local Postgres database")
	}
	truncateDistTestTables(t)
	cs := NewChainState("unused-transfer-concurrent-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection (cs.useDB == false) — check DATABASE_URL")
	}
	return cs
}

func seedConcurrentTestAccount(t *testing.T, cs *ChainState, addr string, balance float64, lastActivityAt int64) *AccountState {
	t.Helper()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	acc := &AccountState{Address: addr, IsHuman: true, Balance: NewDecimal(balance), LastActivityAt: lastActivityAt}
	if err := cs.saveAccountToDB(acc); err != nil {
		t.Fatalf("seed %s: %v", addr, err)
	}
	cs.accounts.Set(addr, acc)
	return acc
}

// TestTransferConcurrent_EligibleTransferSucceeds is the basic happy path:
// two warm, cap-safe, decay-free accounts — applied=true, err=nil, both
// balances correct in memory AND in the real Postgres row (the ctx-routed
// DB write this whole fast path exists to prove out), and accountSetXOR
// matches a full recompute afterward.
func TestTransferConcurrent_EligibleTransferSucceeds(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	from := distTestAddr(401)
	to := distTestAddr(402)
	seedConcurrentTestAccount(t, cs, from, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, cs, to, 0, time.Now().Unix())

	fromLost, toLost, applied, err := cs.transferConcurrent(from, to, 30, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 30, TxHash: "0xtest401"})
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
	gotXOR := cs.accountSetXOR
	cs.mu.RUnlock()
	if fromAcc.Balance.Float() != 70 {
		t.Errorf("sender balance = %v, want 70", fromAcc.Balance.Float())
	}
	if toAcc.Balance.Float() != 30 {
		t.Errorf("recipient balance = %v, want 30", toAcc.Balance.Float())
	}

	var dbFrom, dbTo float64
	if err := cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, from).Scan(&dbFrom); err != nil {
		t.Fatalf("sender row not found in Postgres: %v", err)
	}
	if err := cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, to).Scan(&dbTo); err != nil {
		t.Fatalf("recipient row not found in Postgres: %v", err)
	}
	if dbFrom != 70 || dbTo != 30 {
		t.Errorf("Postgres balances = (%v, %v), want (70, 30)", dbFrom, dbTo)
	}

	cs.mu.RLock()
	want := referenceAccountXOR(cs)
	cs.mu.RUnlock()
	if gotXOR != want {
		t.Fatal("accountSetXOR diverged from a full recompute after a successful transferConcurrent call")
	}

	var queued int
	if err := cs.db.QueryRow(`SELECT count(*) FROM pending_txs WHERE tx_json::json->>'tx_hash' = $1`, "0xtest401").Scan(&queued); err != nil {
		t.Fatalf("checking outbox: %v", err)
	}
	if queued != 1 {
		t.Errorf("want exactly 1 outbox row for this transfer, got %d", queued)
	}
}

// TestTransferConcurrent_ColdAccountFallsBack proves an account not
// already warm in cs.accounts (never loaded) makes this path ineligible —
// applied=false, no error, nothing touched — so the caller can safely fall
// back to the slow path's ensureAccountLoadedCtx warming.
func TestTransferConcurrent_ColdAccountFallsBack(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	from := distTestAddr(403)
	to := distTestAddr(404) // never seeded into cs.accounts, never saved to DB

	seedConcurrentTestAccount(t, cs, from, 100, time.Now().Unix())

	_, _, applied, err := cs.transferConcurrent(from, to, 10, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 10})
	if applied {
		t.Fatal("want applied=false for a cold recipient")
	}
	if err != nil {
		t.Errorf("want nil error when falling back, got %v", err)
	}
}

// TestTransferConcurrent_DemurrageEligibleFallsBack proves an account
// whose idle balance would actually decay (LastActivityAt far enough in
// the past) makes this path ineligible — settling that decay would credit
// a tokenomics pool address, outside the two shard locks this fast path
// holds.
func TestTransferConcurrent_DemurrageEligibleFallsBack(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	from := distTestAddr(405)
	to := distTestAddr(406)
	longInactive := time.Now().Unix() - demurrageGracePeriodSeconds - 3600*24*60 // well past the grace period
	seedConcurrentTestAccount(t, cs, from, 1000, longInactive)
	seedConcurrentTestAccount(t, cs, to, 0, time.Now().Unix())

	_, _, applied, err := cs.transferConcurrent(from, to, 10, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 10})
	if applied {
		t.Fatal("want applied=false when the sender's idle balance would actually decay")
	}
	if err != nil {
		t.Errorf("want nil error when falling back, got %v", err)
	}

	// Confirm nothing was touched — the fallback must be a true no-op.
	cs.mu.RLock()
	fromAcc, _ := cs.accounts.Get(from)
	cs.mu.RUnlock()
	if fromAcc.Balance.Float() != 1000 {
		t.Errorf("sender balance changed despite applied=false: got %v, want 1000", fromAcc.Balance.Float())
	}
}

// TestTransferConcurrent_InsufficientBalanceErrors proves this is treated
// as a real, applied error (not an eligibility fallback) — the caller must
// surface it, not silently retry via the slow path (which would just
// reject it identically anyway, but with duplicated work).
func TestTransferConcurrent_InsufficientBalanceErrors(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	from := distTestAddr(407)
	to := distTestAddr(408)
	seedConcurrentTestAccount(t, cs, from, 5, time.Now().Unix())
	seedConcurrentTestAccount(t, cs, to, 0, time.Now().Unix())

	_, _, applied, err := cs.transferConcurrent(from, to, 100, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 100})
	if !applied {
		t.Fatal("want applied=true for an insufficient-balance transfer (a real, decided outcome)")
	}
	if err == nil {
		t.Fatal("want a non-nil error for insufficient balance")
	}
}

// TestTransferConcurrent_SelfTransferErrors mirrors transferLocked's own
// self-transfer rejection (P2-5's comment: prevents double-demurrage
// settlement on the same account object).
func TestTransferConcurrent_SelfTransferErrors(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	addr := distTestAddr(409)
	seedConcurrentTestAccount(t, cs, addr, 100, time.Now().Unix())

	_, _, applied, err := cs.transferConcurrent(addr, addr, 10, Transaction{Type: "transfer", Wallet: addr, To: addr, Amount: 10})
	if !applied || err == nil {
		t.Fatalf("want applied=true, err!=nil for a self-transfer, got applied=%v err=%v", applied, err)
	}
}

// TestTransferConcurrent_ConcurrentRingTransfersConserveBalance is the
// core property-based concurrency proof for the raw transferConcurrent
// primitive: many goroutines running it in a ring topology (i sends to
// i+1) so no two goroutines ever touch the exact same {from,to} PAIR —
// though adjacent goroutines DO share one address each (i's recipient is
// i+1's sender), which is exactly the kind of genuine, expected shard
// contention TryLockAddrs is built to bail out of rather than block on
// (see transferConcurrent's and TryLockAddrs's own comments). This test
// calls transferConcurrent directly, with no fallback available (unlike
// TransferAtomic, which falls back to the batcher — see
// TestTransferConcurrent_ViaTransferAtomic_RingConservesBalance below for
// that end-to-end guarantee), so some fraction of these calls are expected
// to return applied=false here. What must hold regardless of how many
// apply: -race clean, never deadlock, exact balance conservation across
// however many DID apply (no lost updates, no double-spends, no
// corruption from a bailed attempt), and accountSetXOR must still match a
// full recompute afterward.
func TestTransferConcurrent_ConcurrentRingTransfersConserveBalance(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	const n = 60
	const amount = 25
	addrs := make([]string, n)
	for i := 0; i < n; i++ {
		addrs[i] = distTestAddr(500 + i)
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
			_, _, ok, err := cs.transferConcurrent(from, to, amount, Transaction{
				Type: "transfer", Wallet: from, To: to, Amount: amount, TxHash: fmt.Sprintf("0xring%03d", i),
			})
			if err != nil {
				t.Errorf("goroutine %d: unexpected error: %v", i, err)
				return
			}
			applied[i] = ok
		}(i)
	}
	wg.Wait()

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
		t.Fatalf("total balance after %d concurrent ring transfer attempts = %v, want %v (lost update or double-spend if different)", n, total, float64(n)*1000)
	}
	if gotXOR != wantXOR {
		t.Fatal("accountSetXOR diverged from a full recompute after concurrent transferConcurrent calls — accountSetXORMu did not correctly serialize the global accumulator")
	}

	// Reconstruct each account's expected balance from exactly which
	// attempts actually applied (a stronger check than the aggregate total
	// above, which alone would not catch e.g. money silently moving from
	// account A to the wrong account C without B losing any).
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

// TestTransferConcurrent_ViaTransferAtomic_RingConservesBalance is the
// end-to-end counterpart to the raw-primitive test above: the same ring
// topology (same genuine per-address contention between neighbors), but
// driven through TransferAtomic — the real production entry point, which
// tries transferConcurrent first and falls back to the proven
// batcher-based slow path whenever the fast path bails (applied=false),
// including a TryLockAddrs contention bail. This is the guarantee that
// actually matters: every one of the n transfers must succeed (via
// whichever path), and the ledger must end up exactly conserved, whether
// each individual transfer happened to take the fast path or the slow one.
func TestTransferConcurrent_ViaTransferAtomic_RingConservesBalance(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	const n = 60
	const amount = 25
	addrs := make([]string, n)
	for i := 0; i < n; i++ {
		addrs[i] = distTestAddr(700 + i)
		seedConcurrentTestAccount(t, cs, addrs[i], 1000, time.Now().Unix())
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			from := addrs[i]
			to := addrs[(i+1)%n]
			tmpl := Transaction{Type: "transfer", Wallet: from, To: to, Amount: amount, TxHash: fmt.Sprintf("0xatomicring%03d", i)}
			if _, _, err := cs.TransferAtomic(from, to, amount, tmpl); err != nil {
				t.Errorf("goroutine %d: TransferAtomic failed: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	cs.mu.RLock()
	defer cs.mu.RUnlock()
	var total float64
	cs.accounts.Range(func(_ string, acc *AccountState) bool {
		total += acc.Balance.Float()
		return true
	})
	if total != float64(n)*1000 {
		t.Fatalf("total balance after %d concurrent TransferAtomic ring transfers = %v, want %v", n, total, float64(n)*1000)
	}
	// Every account moved by exactly (+25 in, -25 out) = net 0.
	for _, addr := range addrs {
		acc, _ := cs.accounts.Get(addr)
		if acc.Balance.Float() != 1000 {
			t.Errorf("account %s balance = %v, want 1000 (ring net-zero)", addr, acc.Balance.Float())
		}
	}
}

// TestTransferConcurrent_ConcurrentOverlappingTransfersNoDeadlockNoCorruption
// is the deadlock-freedom + correctness proof under REALISTIC contention:
// a small pool of accounts (heavy shard/address overlap, unlike the
// disjoint ring above) hit by many concurrent transfers with randomly
// paired, non-monotonically-ordered {from,to} — proves LockAddrs's sorted
// lock ordering actually prevents cross-transfer deadlock in the real
// transferConcurrent path (not just the lower-level shardedAccounts
// primitive test), and that balance conservation holds exactly even when
// many transfers fail on insufficient balance along the way (a failed
// attempt must have exactly zero effect).
func TestTransferConcurrent_ConcurrentOverlappingTransfersNoDeadlockNoCorruption(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	const numAddrs = 12
	addrs := make([]string, numAddrs)
	for i := range addrs {
		addrs[i] = distTestAddr(600 + i)
		seedConcurrentTestAccount(t, cs, addrs[i], 500, time.Now().Unix())
	}

	const rounds = 400
	var wg sync.WaitGroup
	done := make(chan struct{})
	go func() {
		for r := 0; r < rounds; r++ {
			wg.Add(1)
			go func(r int) {
				defer wg.Done()
				from := addrs[r%numAddrs]
				to := addrs[(r*7+3)%numAddrs]
				if from == to {
					to = addrs[(r*7+5)%numAddrs]
				}
				_, _, _, err := cs.transferConcurrent(from, to, 3, Transaction{
					Type: "transfer", Wallet: from, To: to, Amount: 3, TxHash: fmt.Sprintf("0xhot%04d", r),
				})
				// Insufficient-balance errors are expected and fine here —
				// only a non-insufficient-balance error is a real failure.
				if err != nil && err.Error() != "insufficient balance" {
					t.Errorf("round %d: unexpected error: %v", r, err)
				}
			}(r)
		}
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("transferConcurrent deadlocked under overlapping-address contention")
	}

	cs.mu.RLock()
	var total float64
	cs.accounts.Range(func(_ string, acc *AccountState) bool {
		total += acc.Balance.Float()
		return true
	})
	gotXOR := cs.accountSetXOR
	wantXOR := referenceAccountXOR(cs)
	cs.mu.RUnlock()

	if total != float64(numAddrs)*500 {
		t.Fatalf("total balance after %d concurrent overlapping transfers = %v, want %v", rounds, total, float64(numAddrs)*500)
	}
	if gotXOR != wantXOR {
		t.Fatal("accountSetXOR diverged from a full recompute under overlapping concurrent contention")
	}

	// Cross-check against Postgres directly -- proves the DB rows are not
	// just internally consistent with cs.accounts by coincidence, but
	// actually reflect every committed mutation.
	var dbTotal float64
	rows, err := cs.db.Query(`SELECT balance FROM chain_accounts WHERE lower(address) = ANY($1)`, pq.Array(addrs))
	if err != nil {
		t.Fatalf("querying Postgres balances: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var b float64
		if err := rows.Scan(&b); err != nil {
			t.Fatalf("scanning balance: %v", err)
		}
		dbTotal += b
	}
	if dbTotal != float64(numAddrs)*500 {
		t.Fatalf("total balance in Postgres = %v, want %v", dbTotal, float64(numAddrs)*500)
	}
}
