package keeper

import (
	"fmt"
	"sync"
	"testing"
)

// registerHumanConcurrent/RegisterHumanAtomic's fast path require a real
// Postgres connection, so every test in this file is opt-in real-DB,
// matching this project's existing *_db_test.go convention. Reuses
// newConcurrentTransferTestState (transfer_concurrent_test.go) — it's
// generic (truncate + NewChainState), nothing transfer-specific about it.

func TestRegisterHumanConcurrent_EligibleColdRegistrationSucceeds(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	addr := distTestAddr(1001) // never seeded -- genuinely cold, the common real-world case

	applied, err := cs.registerHumanConcurrent(addr, Transaction{Type: "register_human", Wallet: addr, TxHash: "0xregcold1"})
	if !applied {
		t.Fatal("want applied=true for a cold, never-registered, cap-safe address")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cs.mu.RLock()
	acc, ok := cs.accounts.Get(addr)
	cs.mu.RUnlock()
	if !ok {
		t.Fatal("account not present in cs.accounts after registration")
	}
	if !acc.IsHuman {
		t.Error("want IsHuman=true")
	}
	if acc.Balance.Float() != 1000 {
		t.Errorf("balance = %v, want 1000", acc.Balance.Float())
	}

	var dbBalance float64
	var dbIsHuman bool
	if err := cs.db.QueryRow(`SELECT balance, is_human FROM chain_accounts WHERE lower(address) = $1`, addr).Scan(&dbBalance, &dbIsHuman); err != nil {
		t.Fatalf("row not found in Postgres: %v", err)
	}
	if dbBalance != 1000 || !dbIsHuman {
		t.Errorf("Postgres row = (balance=%v, is_human=%v), want (1000, true)", dbBalance, dbIsHuman)
	}
}

func TestRegisterHumanConcurrent_EligibleWarmRegistrationSucceeds(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	addr := distTestAddr(1002)
	// Pre-fund via a normal (non-human) account -- the "received AEQ before
	// ever registering" case, already warm in cs.accounts. Deliberately NOT
	// using seedConcurrentTestAccount here: that helper always sets
	// IsHuman=true (fine for transfer tests, wrong for this one).
	cs.mu.Lock()
	acc := &AccountState{Address: addr, IsHuman: false, Balance: NewDecimal(50)}
	if err := cs.saveAccountToDB(acc); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cs.accounts.Set(addr, acc)
	cs.mu.Unlock()

	applied, err := cs.registerHumanConcurrent(addr, Transaction{Type: "register_human", Wallet: addr, TxHash: "0xregwarm1"})
	if !applied || err != nil {
		t.Fatalf("applied=%v err=%v", applied, err)
	}

	cs.mu.RLock()
	acc, _ = cs.accounts.Get(addr)
	cs.mu.RUnlock()
	if acc.Balance.Float() != 1050 {
		t.Errorf("balance = %v, want 1050 (50 pre-funded + 1000 grant)", acc.Balance.Float())
	}
}

func TestRegisterHumanConcurrent_AlreadyRegisteredWarmErrors(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	addr := distTestAddr(1003)
	acc := seedConcurrentTestAccount(t, cs, addr, 1000, 0)
	acc.IsHuman = true

	applied, err := cs.registerHumanConcurrent(addr, Transaction{Type: "register_human", Wallet: addr})
	if !applied {
		t.Fatal("want applied=true (a real, decided outcome) for an already-registered address")
	}
	if err == nil {
		t.Fatal("want a non-nil error")
	}
}

func TestRegisterHumanConcurrent_AlreadyRegisteredColdInDBErrors(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	addr := distTestAddr(1004)
	acc := &AccountState{Address: addr, IsHuman: true, Balance: NewDecimal(1000)}
	if err := cs.saveAccountToDB(acc); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Deliberately NOT added to cs.accounts -- exercises the cold-address
	// Postgres existence check, not the warm in-memory one.

	applied, err := cs.registerHumanConcurrent(addr, Transaction{Type: "register_human", Wallet: addr})
	if !applied {
		t.Fatal("want applied=true for a cold address already registered in Postgres")
	}
	if err == nil {
		t.Fatal("want a non-nil error")
	}
}

// TestRegisterHumanConcurrent_NullifierReuseByDifferentWalletFails is the
// security-critical test: SaveNullifier's DB-level UNIQUE constraint (via
// INSERT ... ON CONFLICT DO NOTHING + RowsAffected check) is what actually
// prevents the same biometric nullifier from minting two 1,000 AEQ grants —
// this proves that guarantee survives running the check from the
// concurrent, RLock()-based fast path, not just the old cs.mu.Lock() one.
// The losing side's registration must be a true no-op: rolled back
// balance, humanCount, and accountSetXOR.
func TestRegisterHumanConcurrent_NullifierReuseByDifferentWalletFails(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	addrA := distTestAddr(1005)
	addrB := distTestAddr(1006)
	// FIX (pre-launch audit 2026-08-16): this used to be the descriptive
	// string "0xsharednullifier1005", which is not a hexadecimal integer.
	// Since nullifier validation was added, registerHumanConcurrent rejects it
	// outright — so this test failed at its FIRST registration and never
	// reached the reuse it exists to check. It is skipped without Postgres, so
	// nothing surfaced it: the project's own security-critical test for "one
	// biometric cannot mint two grants" had quietly stopped testing that.
	// Real nullifiers are 32-byte hex values; the fixture now is one.
	const nullifier = "0x00000000000000000000000000000000000000000000000000000000000003ed"

	appliedA, errA := cs.registerHumanConcurrent(addrA, Transaction{Type: "register_human", Wallet: addrA, Nullifier: nullifier})
	if !appliedA || errA != nil {
		t.Fatalf("first registration: applied=%v err=%v", appliedA, errA)
	}

	humanCountBefore := cs.humanCountLocked()

	appliedB, errB := cs.registerHumanConcurrent(addrB, Transaction{Type: "register_human", Wallet: addrB, Nullifier: nullifier})
	if !appliedB {
		t.Fatal("want applied=true (a real, decided outcome) for a nullifier reused by a different wallet")
	}
	if errB == nil {
		t.Fatal("want a non-nil error -- the whole point of this test")
	}

	// True no-op for the loser: no account created, humanCount unchanged.
	cs.mu.RLock()
	_, exists := cs.accounts.Get(addrB)
	cs.mu.RUnlock()
	if exists {
		t.Error("addrB must not have been created -- the failed registration must be a true no-op")
	}
	if got := cs.humanCountLocked(); got != humanCountBefore {
		t.Errorf("humanCount = %d after a failed registration, want unchanged %d", got, humanCountBefore)
	}

	cs.mu.RLock()
	gotXOR := cs.accountSetXOR
	wantXOR := referenceAccountXOR(cs)
	cs.mu.RUnlock()
	if gotXOR != wantXOR {
		t.Error("accountSetXOR diverged after a failed (rolled-back) registration")
	}
}

// TestRegisterHumanConcurrent_ConcurrentDistinctRegistrationsAllSucceed is
// the property-based concurrency proof: many goroutines registering
// distinct addresses (own nullifier each) concurrently, -race clean, must
// never deadlock, and afterward humanCount/accountSetXOR/nullifierSetXOR
// must all match a full recompute -- the same properties
// accountSetXORMu/humanCountMu/nullifiersMu exist to guarantee under
// genuine concurrent RLock()-based writers.
//
// Drives this through RegisterHumanAtomic, not the raw registerHumanConcurrent
// primitive directly -- with 40 concurrent goroutines and only 64 shards,
// some pairs of DIFFERENT addresses land on the same shard by the birthday
// paradox, so TryLockAddrs's non-blocking design correctly bails
// (applied=false, no error, nothing attempted -- see its own comment) for
// whichever one loses that transient race. RegisterHumanAtomic is what
// actually guarantees "every registration eventually succeeds" in
// production: it falls back to the proven, cs.mu.Lock()-based slow path
// exactly when the fast path bails, the same pattern already proven for
// transfers (TestTransferConcurrent_ViaTransferAtomic_RingConservesBalance).
func TestRegisterHumanConcurrent_ConcurrentDistinctRegistrationsAllSucceed(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	const n = 40
	addrs := make([]string, n)
	for i := range addrs {
		addrs[i] = distTestAddr(1100 + i)
	}
	humanCountBefore := cs.humanCountLocked()

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := cs.RegisterHumanAtomic(addrs[i], Transaction{
				// Hex, for the same reason as the fixture above: a
				// non-hex nullifier is rejected before the concurrency this
				// test exists to exercise is ever reached.
				Type: "register_human", Wallet: addrs[i], Nullifier: fmt.Sprintf("0x%064x", 0xC0FFEE0000+i), TxHash: fmt.Sprintf("0xregconc%04d", i),
			})
			if err != nil {
				t.Errorf("goroutine %d: RegisterHumanAtomic failed: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	cs.mu.RLock()
	gotHumans := cs.humanCountLocked()
	gotAcctXOR := cs.accountSetXOR
	wantAcctXOR := referenceAccountXOR(cs)
	gotNullXOR := cs.nullifierSetXOR
	wantNullXOR := referenceNullifierXOR(cs)
	cs.mu.RUnlock()

	if gotHumans != humanCountBefore+n {
		t.Errorf("humanCount = %d, want %d (%d before + %d new registrations)", gotHumans, humanCountBefore+n, humanCountBefore, n)
	}
	if gotAcctXOR != wantAcctXOR {
		t.Error("accountSetXOR diverged from a full recompute after concurrent registrations")
	}
	if gotNullXOR != wantNullXOR {
		t.Error("nullifierSetXOR diverged from a full recompute after concurrent registrations")
	}

	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, addr := range addrs {
		acc, ok := cs.accounts.Get(addr)
		if !ok {
			t.Errorf("account %s: not found, want present after RegisterHumanAtomic", addr)
			continue
		}
		if !acc.IsHuman || acc.Balance.Float() != 1000 {
			t.Errorf("account %s: IsHuman=%v Balance=%v, want true/1000", addr, acc.IsHuman, acc.Balance.Float())
		}
	}
}

// TestRegisterHumanConcurrent_ViaRegisterHumanAtomic is the end-to-end
// check through the real production entry point.
func TestRegisterHumanConcurrent_ViaRegisterHumanAtomic(t *testing.T) {
	cs := newConcurrentTransferTestState(t)
	addr := distTestAddr(1200)

	if err := cs.RegisterHumanAtomic(addr, Transaction{Type: "register_human", Wallet: addr, Nullifier: "0x00000000000000000000000000000000000000000000000000000000000004b0", TxHash: "0xregatomic1200"}); err != nil {
		t.Fatalf("RegisterHumanAtomic: %v", err)
	}

	cs.mu.RLock()
	acc, ok := cs.accounts.Get(addr)
	cs.mu.RUnlock()
	if !ok || !acc.IsHuman || acc.Balance.Float() != 1000 {
		t.Fatalf("account after RegisterHumanAtomic: ok=%v acc=%+v", ok, acc)
	}

	var queued int
	if err := cs.db.QueryRow(`SELECT count(*) FROM pending_txs WHERE tx_json::json->>'tx_hash' = $1`, "0xregatomic1200").Scan(&queued); err != nil {
		t.Fatalf("checking outbox: %v", err)
	}
	if queued != 1 {
		t.Errorf("want exactly 1 outbox row, got %d", queued)
	}

	// Calling it again for the SAME address must fail (already registered),
	// through the exact same real entry point.
	if err := cs.RegisterHumanAtomic(addr, Transaction{Type: "register_human", Wallet: addr}); err == nil {
		t.Fatal("want an error re-registering the same address via RegisterHumanAtomic")
	}
}
