package keeper

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
)

// These tests exercise shardedAccounts entirely in isolation -- no
// ChainState, no DB, no production code path. Phase 1 of
// SCALING_ARCHITECTURE.md: prove the primitive itself is correct before
// anything is migrated to use it.

func TestShardedAccounts_GetSetDelete(t *testing.T) {
	sa := newShardedAccounts()

	if _, ok := sa.Get("0xabc"); ok {
		t.Fatal("expected miss on empty store")
	}

	acc := &AccountState{Address: "0xabc", Balance: NewDecimal(100)}
	sa.Set("0xabc", acc)

	got, ok := sa.Get("0xabc")
	if !ok {
		t.Fatal("expected hit after Set")
	}
	if got != acc {
		t.Fatal("Get must return the exact same pointer that was Set -- callers rely on in-place mutation via the returned pointer (e.g. acc.Balance = ...), same as the native map of pointers it replaces")
	}
	if got.Balance.Float() != 100 {
		t.Fatalf("balance = %v, want 100", got.Balance.Float())
	}

	sa.Delete("0xabc")
	if _, ok := sa.Get("0xabc"); ok {
		t.Fatal("expected miss after Delete")
	}
}

// TestShardedAccounts_InPlaceMutationVisibleViaGet pins the exact
// semantic every one of the 190 existing cs.accounts[addr].Field = ...
// call sites depends on: since AccountState is stored as a pointer,
// mutating the struct a Get() returned must be visible to the NEXT Get()
// for the same address, without a matching Set() call -- exactly how the
// native map of pointers behaves today.
func TestShardedAccounts_InPlaceMutationVisibleViaGet(t *testing.T) {
	sa := newShardedAccounts()
	sa.Set("0xabc", &AccountState{Address: "0xabc", Balance: NewDecimal(100)})

	acc, _ := sa.Get("0xabc")
	acc.Balance = acc.Balance.Add(NewDecimal(50))

	got, _ := sa.Get("0xabc")
	if got.Balance.Float() != 150 {
		t.Fatalf("balance after in-place mutation = %v, want 150", got.Balance.Float())
	}
}

func TestShardedAccounts_LenAndRange(t *testing.T) {
	sa := newShardedAccounts()
	want := map[string]float64{}
	for i := 0; i < 500; i++ {
		addr := fmt.Sprintf("0xaddr%04d", i)
		sa.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(float64(i))})
		want[addr] = float64(i)
	}

	if got := sa.Len(); got != 500 {
		t.Fatalf("Len() = %d, want 500", got)
	}

	seen := map[string]float64{}
	sa.Range(func(addr string, acc *AccountState) bool {
		seen[addr] = acc.Balance.Float()
		return true
	})
	if len(seen) != 500 {
		t.Fatalf("Range visited %d accounts, want 500", len(seen))
	}
	for addr, bal := range want {
		if seen[addr] != bal {
			t.Errorf("addr %s: Range saw balance %v, want %v", addr, seen[addr], bal)
		}
	}
}

// TestShardedAccounts_RangeEarlyStop pins the `for !fn() { return false }`
// contract -- Range must stop as soon as fn returns false, not visit every
// remaining account in the current shard or later shards.
func TestShardedAccounts_RangeEarlyStop(t *testing.T) {
	sa := newShardedAccounts()
	for i := 0; i < numAccountShards*3; i++ {
		addr := fmt.Sprintf("0xstop%04d", i)
		sa.Set(addr, &AccountState{Address: addr})
	}
	visited := 0
	sa.Range(func(addr string, acc *AccountState) bool {
		visited++
		return visited < 5
	})
	if visited != 5 {
		t.Fatalf("Range visited %d accounts before stopping, want exactly 5", visited)
	}
}

// TestShardedAccounts_MarshalJSON pins cs.save()'s no-DB fallback
// contract: json.Marshal(sa) must produce the same shape as
// json.Marshal(the equivalent plain map) would have.
func TestShardedAccounts_MarshalJSON(t *testing.T) {
	sa := newShardedAccounts()
	sa.Set("0xabc", &AccountState{Address: "0xabc", Balance: NewDecimal(42)})
	sa.Set("0xdef", &AccountState{Address: "0xdef", Balance: NewDecimal(7)})

	data, err := json.Marshal(sa)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}
	var decoded map[string]*AccountState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("could not decode marshaled output: %v", err)
	}
	if len(decoded) != 2 {
		t.Fatalf("decoded %d accounts, want 2", len(decoded))
	}
	if decoded["0xabc"].Balance.Float() != 42 {
		t.Errorf("0xabc balance = %v, want 42", decoded["0xabc"].Balance.Float())
	}
	if decoded["0xdef"].Balance.Float() != 7 {
		t.Errorf("0xdef balance = %v, want 7", decoded["0xdef"].Balance.Float())
	}
}

// TestShardedAccounts_ShardIndexDeterministic pins the routing contract
// every future cross-shard-locking phase depends on: the same address
// must always hash to the same shard, and the range must stay in bounds.
func TestShardedAccounts_ShardIndexDeterministic(t *testing.T) {
	addrs := []string{"0x0", "0xabc123", "0xValidatorsPool", "", "0x" + string(make([]byte, 100))}
	for _, a := range addrs {
		first := shardIndexFor(a)
		if first < 0 || first >= numAccountShards {
			t.Fatalf("shardIndexFor(%q) = %d, out of range [0,%d)", a, first, numAccountShards)
		}
		for i := 0; i < 10; i++ {
			if got := shardIndexFor(a); got != first {
				t.Fatalf("shardIndexFor(%q) not deterministic: got %d and %d", a, first, got)
			}
		}
	}
}

// TestShardedAccounts_ConcurrentDifferentAddresses is the actual point of
// sharding: many goroutines hammering DIFFERENT addresses concurrently
// must never corrupt the store or race (run with -race), and every write
// must be durably visible afterward -- no lost updates.
func TestShardedAccounts_ConcurrentDifferentAddresses(t *testing.T) {
	sa := newShardedAccounts()
	const n = 2000
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			addr := fmt.Sprintf("0xconc%05d", idx)
			sa.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(float64(idx))})
		}(i)
	}
	wg.Wait()

	if got := sa.Len(); got != n {
		t.Fatalf("Len() = %d, want %d", got, n)
	}
	for i := 0; i < n; i++ {
		addr := fmt.Sprintf("0xconc%05d", i)
		acc, ok := sa.Get(addr)
		if !ok {
			t.Fatalf("addr %s missing after concurrent Set", addr)
		}
		if acc.Balance.Float() != float64(i) {
			t.Errorf("addr %s: balance = %v, want %v", addr, acc.Balance.Float(), i)
		}
	}
}

// TestShardedAccounts_ConcurrentSameAddress proves per-shard locking
// actually serializes writes to the SAME address (the case sharding does
// NOT parallelize, by design) without corruption -- every increment must
// land, none lost to a lost-update race.
func TestShardedAccounts_ConcurrentSameAddress(t *testing.T) {
	sa := newShardedAccounts()
	sa.Set("0xshared", &AccountState{Address: "0xshared", Balance: NewDecimal(0)})

	const n = 1000
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := sa.shardFor("0xshared")
			s.mu.Lock()
			acc := s.data["0xshared"]
			acc.Balance = acc.Balance.Add(NewDecimal(1))
			s.mu.Unlock()
		}()
	}
	wg.Wait()

	acc, _ := sa.Get("0xshared")
	if acc.Balance.Float() != float64(n) {
		t.Fatalf("balance after %d concurrent +1 increments = %v, want %v (lost update if lower)", n, acc.Balance.Float(), n)
	}
}

// The tests below exercise XorLeaf/CombinedXOR -- the Phase 4 per-shard
// state-root accumulator primitive. Like every other method here, entirely
// isolated: no ChainState, no cs.accountSetXOR, nothing wired into any
// production code path yet (see XorLeaf's own comment for why that wiring
// is deliberately deferred to a later phase).

// TestShardedAccounts_CombinedXOR_MatchesFullRecompute is CombinedXOR's
// core correctness proof, mirroring state_accumulator_test.go's
// referenceAccountXOR pattern for cs.accountSetXOR: after a sequence of
// XorLeaf calls (add, mutate, remove-by-zeroing), CombinedXOR() must always
// equal a full recompute (XOR of accountLeaf(acc) over every stored
// account), never just "probably close."
func TestShardedAccounts_CombinedXOR_MatchesFullRecompute(t *testing.T) {
	sa := newShardedAccounts()
	assertMatch := func(label string) {
		t.Helper()
		var full [32]byte
		sa.Range(func(_ string, acc *AccountState) bool {
			xorInto(&full, accountLeaf(acc))
			return true
		})
		if sa.CombinedXOR() != full {
			t.Fatalf("%s: CombinedXOR() diverged from a full recompute", label)
		}
	}
	assertMatch("empty")

	// Add an account: its leaf swaps in from the zero hash.
	a := &AccountState{Address: "0xaaa", Balance: NewDecimal(1000), IsHuman: true}
	sa.Set("0xaaa", a)
	leaf := accountLeaf(a)
	sa.XorLeaf("0xaaa", [32]byte{}, leaf)
	a.leafHash = leaf
	assertMatch("add 0xaaa")

	// Mutate its balance repeatedly -- each XorLeaf call must swap the OLD
	// leaf out and the NEW leaf in, exactly like updateAccountLeafLocked
	// does for the single global accumulator.
	for _, bal := range []float64{500, 750, 12.345678, 1000} {
		oldLeaf := a.leafHash
		a.Balance = NewDecimal(bal)
		newLeaf := accountLeaf(a)
		sa.XorLeaf("0xaaa", oldLeaf, newLeaf)
		a.leafHash = newLeaf
		assertMatch("mutate 0xaaa balance")
	}

	// A second account, landing in a different shard virtually always (64
	// shards) -- proves cross-shard combination, not just one shard's math.
	b := &AccountState{Address: "0xbbb", TUsdBalance: NewDecimal(10), LPShares: NewDecimal(5)}
	sa.Set("0xbbb", b)
	bLeaf := accountLeaf(b)
	sa.XorLeaf("0xbbb", [32]byte{}, bLeaf)
	b.leafHash = bLeaf
	assertMatch("add 0xbbb (likely different shard)")

	// Toggle 0xaaa OUT of the accumulator the same way the real system does
	// (see accountLeaf's own zero-leaf condition): zero every field that
	// keeps it "included", so accountLeaf(a) itself now naturally returns
	// the zero hash -- not just an out-of-band claim that it should.
	oldLeaf := a.leafHash
	a.IsHuman = false
	a.Balance = NewDecimal(0)
	newLeaf := accountLeaf(a)
	if newLeaf != ([32]byte{}) {
		t.Fatalf("test setup error: accountLeaf(a) should be the zero hash once every included field is zeroed, got %x", newLeaf)
	}
	sa.XorLeaf("0xaaa", oldLeaf, newLeaf)
	a.leafHash = newLeaf
	assertMatch("zero out 0xaaa's leaf")
}

// TestShardedAccounts_CombinedXOR_OrderIndependence proves two stores that
// reach the same account set by different insertion orders (and therefore
// different XorLeaf call orders, since shard assignment doesn't change but
// timing does) produce the same CombinedXOR -- the property real concurrent
// use depends on, since concurrent goroutines touching different shards can
// finish in any order.
func TestShardedAccounts_CombinedXOR_OrderIndependence(t *testing.T) {
	build := func(order []string) *shardedAccounts {
		sa := newShardedAccounts()
		for _, addr := range order {
			acc := &AccountState{Address: addr, Balance: NewDecimal(100), IsHuman: true}
			sa.Set(addr, acc)
			leaf := accountLeaf(acc)
			sa.XorLeaf(addr, [32]byte{}, leaf)
			acc.leafHash = leaf
		}
		return sa
	}
	a := build([]string{"0x01", "0x02", "0x03"})
	b := build([]string{"0x03", "0x01", "0x02"})
	if a.CombinedXOR() != b.CombinedXOR() {
		t.Fatal("CombinedXOR depends on insertion order")
	}
}

// TestShardedAccounts_XorLeaf_ConcurrentDifferentShards is the actual point
// of Phase 4: many goroutines XOR-updating accounts in DIFFERENT shards
// concurrently must never race (run with -race) or lose an update -- the
// per-shard partialXOR must end up bit-identical to a sequential
// application of the exact same operations in any fixed order (XOR being
// commutative/associative, the final combined value doesn't depend on
// which goroutine's update landed in its shard first).
func TestShardedAccounts_XorLeaf_ConcurrentDifferentShards(t *testing.T) {
	sa := newShardedAccounts()
	const n = 2000
	accs := make([]*AccountState, n)
	for i := 0; i < n; i++ {
		addr := fmt.Sprintf("0xxor%05d", i)
		accs[i] = &AccountState{Address: addr, Balance: NewDecimal(float64(i))}
		sa.Set(addr, accs[i])
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			acc := accs[idx]
			leaf := accountLeaf(acc)
			sa.XorLeaf(acc.Address, [32]byte{}, leaf)
		}(i)
	}
	wg.Wait()
	for _, acc := range accs {
		acc.leafHash = accountLeaf(acc)
	}

	var full [32]byte
	for _, acc := range accs {
		xorInto(&full, accountLeaf(acc))
	}
	if sa.CombinedXOR() != full {
		t.Fatal("CombinedXOR() diverged from a full recompute after concurrent XorLeaf calls across many shards -- lost or corrupted update")
	}
}

// TestShardedAccounts_Clone_DoesNotCarryOverPartialXOR pins Clone's
// documented contract: it copies accounts via Set only, never XorLeaf, so a
// fresh clone's CombinedXOR() starts at zero regardless of the source's
// accumulator state -- a caller that needs the clone's own accumulator to
// be meaningful must rebuild it explicitly. This is a deliberate design
// choice (see Clone's own comment), not an oversight -- this test exists so
// a future change can't silently flip that behavior unnoticed.
func TestShardedAccounts_Clone_DoesNotCarryOverPartialXOR(t *testing.T) {
	sa := newShardedAccounts()
	acc := &AccountState{Address: "0xaaa", Balance: NewDecimal(1000), IsHuman: true}
	sa.Set("0xaaa", acc)
	leaf := accountLeaf(acc)
	sa.XorLeaf("0xaaa", [32]byte{}, leaf)

	if sa.CombinedXOR() == ([32]byte{}) {
		t.Fatal("test setup error: source CombinedXOR() should be non-zero before cloning")
	}

	clone := sa.Clone()
	if clone.CombinedXOR() != ([32]byte{}) {
		t.Fatal("Clone() unexpectedly carried over partialXOR state -- either fix this test to match a deliberate behavior change, or fix Clone()")
	}
	// The clone's accounts themselves ARE correctly copied -- only the
	// accumulator is independent of Set.
	got, ok := clone.Get("0xaaa")
	if !ok || got.Balance.Float() != 1000 {
		t.Fatal("Clone() did not correctly copy the account itself")
	}
}
