package keeper

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// readBalanceOfSlotFromDB reads addr's balanceOf mirror slot (slot 4, the
// same one doSyncBalanceLocked writes) directly from Postgres, bypassing
// cs.accounts entirely -- the only way to observe whether a deferred write
// has actually landed yet. Returns "" if no row exists (never synced). Safe
// to call whether or not the caller holds cs.mu (cs.db is a connection pool
// safe for concurrent use) -- callers that need to rule out a race against
// a concurrent flush should keep cs.mu held around the call themselves.
func readBalanceOfSlotFromDB(t *testing.T, cs *ChainState, contractAddr, addr string) string {
	t.Helper()
	contractAddr = strings.ToLower(contractAddr) // doSyncBalanceLocked stores it lowercased
	slot := mappingSlot(common.HexToAddress(addr).Bytes(), 4).Hex()
	var value string
	err := cs.db.QueryRow(`SELECT value FROM evm_storage WHERE address = $1 AND slot = $2`, contractAddr, slot).Scan(&value)
	if err != nil {
		return ""
	}
	return value
}

// deleteBalanceOfSlotFromDB removes addr's balanceOf mirror slot row, if
// any -- used to give a test a clean starting point against the shared
// bench DB, which otherwise accumulates rows across separate test runs.
func deleteBalanceOfSlotFromDB(t *testing.T, cs *ChainState, contractAddr, addr string) {
	t.Helper()
	contractAddr = strings.ToLower(contractAddr)
	slot := mappingSlot(common.HexToAddress(addr).Bytes(), 4).Hex()
	if _, err := cs.db.Exec(`DELETE FROM evm_storage WHERE address = $1 AND slot = $2`, contractAddr, slot); err != nil {
		t.Fatalf("could not clear pre-existing evm_storage row: %v", err)
	}
}

// TestSyncBalanceLocked_DefersEVMMirrorWrite is the core correctness proof
// for SCALING_ARCHITECTURE.md Phase 6: with a real DB, syncBalanceLocked
// must NOT write to evm_storage synchronously -- it only marks the address
// dirty. The row must remain absent/unchanged until FlushEVMMirrorNow (or
// the ticker) actually runs.
func TestSyncBalanceLocked_DefersEVMMirrorWrite(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	cs := NewChainState("unused-evm-mirror-flush-test-1.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}

	addr := "0xbf00000000000000000000000000000000e001"
	// This is a shared, disposable bench DB across test runs -- clear any
	// leftover row from a PREVIOUS run of this exact test first, or the
	// "must not exist yet" assertion below would be fooled by stale state
	// rather than actually proving the write was deferred THIS run.
	deleteBalanceOfSlotFromDB(t, cs, V7_CONTRACT_ADDR, addr)

	cs.mu.Lock()
	cs.accounts.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(250), IsHuman: true})
	cs.syncBalanceLocked(V7_CONTRACT_ADDR, addr)
	deferredValue := readBalanceOfSlotFromDB(t, cs, V7_CONTRACT_ADDR, addr)
	cs.mu.Unlock()

	if deferredValue != "" {
		t.Errorf("expected no evm_storage row immediately after syncBalanceLocked (deferred write), got %q", deferredValue)
	}

	cs.FlushEVMMirrorNow()

	got := readBalanceOfSlotFromDB(t, cs, V7_CONTRACT_ADDR, addr)
	if got == "" {
		t.Fatal("expected evm_storage row to exist after FlushEVMMirrorNow")
	}
	wantWei := aeqToWei(250)
	wantHex := common.BigToHash(wantWei).Hex()
	if got != wantHex {
		t.Errorf("balanceOf slot after flush: got %s, want %s (250 AEQ in wei)", got, wantHex)
	}
}

// TestEVMMirrorFlush_BatchesMultipleDirtyAddresses proves several addresses
// marked dirty across multiple syncBalanceLocked calls before a flush all
// get correctly synced by ONE flush -- not just the most recently marked
// one, and none silently dropped.
func TestEVMMirrorFlush_BatchesMultipleDirtyAddresses(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	cs := NewChainState("unused-evm-mirror-flush-test-2.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}

	addrs := []string{
		"0xbf00000000000000000000000000000000e011",
		"0xbf00000000000000000000000000000000e012",
		"0xbf00000000000000000000000000000000e013",
	}
	cs.mu.Lock()
	for i, addr := range addrs {
		cs.accounts.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(float64(10 * (i + 1))), IsHuman: true})
		cs.syncBalanceLocked(V7_CONTRACT_ADDR, addr)
	}
	cs.mu.Unlock()

	cs.FlushEVMMirrorNow()

	for i, addr := range addrs {
		got := readBalanceOfSlotFromDB(t, cs, V7_CONTRACT_ADDR, addr)
		want := common.BigToHash(aeqToWei(float64(10 * (i + 1)))).Hex()
		if got != want {
			t.Errorf("addr %s: balanceOf slot = %q, want %q", addr, got, want)
		}
	}
}

// TestEVMMirrorFlush_NoOpWhenNotDirty proves flushing with nothing pending
// is always safe, both before the worker has ever started and after a
// flush already drained everything.
func TestEVMMirrorFlush_NoOpWhenNotDirty(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	cs := NewChainState("unused-evm-mirror-flush-test-3.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}

	cs.FlushEVMMirrorNow() // never touched syncBalanceLocked -- must not panic

	addr := "0xbf00000000000000000000000000000000e021"
	cs.mu.Lock()
	cs.accounts.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(5), IsHuman: true})
	cs.syncBalanceLocked(V7_CONTRACT_ADDR, addr)
	cs.mu.Unlock()

	cs.FlushEVMMirrorNow() // drains it
	cs.evmMirrorDirtyMu.Lock()
	remaining := len(cs.evmMirrorDirty)
	cs.evmMirrorDirtyMu.Unlock()
	if remaining != 0 {
		t.Fatalf("expected dirty set empty after flush, got %d entries", remaining)
	}
	cs.FlushEVMMirrorNow() // second call: nothing pending, must still be safe
}

// TestEVMMirrorFlush_ConcurrentMarksNoLostUpdates is the -race regression
// guard: many goroutines each mark a DIFFERENT address dirty concurrently
// (each holding cs.mu for its own syncBalanceLocked call, exactly like real
// concurrent transfers would), then one flush -- every single address must
// end up with the correct persisted balanceOf slot, none lost.
func TestEVMMirrorFlush_ConcurrentMarksNoLostUpdates(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	cs := NewChainState("unused-evm-mirror-flush-test-4.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}

	const n = 60
	addrs := make([]string, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		addr := fmt.Sprintf("0xbf000000000000000000000000000000%06x", i)
		addrs[i] = addr
		go func(addr string, bal float64) {
			defer wg.Done()
			cs.mu.Lock()
			cs.accounts.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(bal), IsHuman: true})
			cs.syncBalanceLocked(V7_CONTRACT_ADDR, addr)
			cs.mu.Unlock()
		}(addr, float64(i+1))
	}
	wg.Wait()

	cs.FlushEVMMirrorNow()

	for i, addr := range addrs {
		got := readBalanceOfSlotFromDB(t, cs, V7_CONTRACT_ADDR, addr)
		want := common.BigToHash(aeqToWei(float64(i + 1))).Hex()
		if got != want {
			t.Errorf("addr %s: balanceOf slot = %q, want %q -- a lost update under concurrency", addr, got, want)
		}
	}
}
