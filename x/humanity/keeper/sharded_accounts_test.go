package keeper

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
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

// The tests below exercise LockAddrs/GetLocked/SetLocked -- the Phase 5
// multi-shard locking primitive. Entirely isolated: no ChainState, no
// cs.mu, no cs.accountSetXOR, nothing wired into any production code path
// yet (see LockAddrs's own comment for the concurrent-transfer path this
// is built for, added separately).

// TestShardedAccounts_LockAddrs_BasicReadModifyWrite proves the intended
// usage pattern works: lock two addresses, read+mutate+write both while
// held, unlock -- the exact sequence a caller needs for a safe
// cross-account transfer.
func TestShardedAccounts_LockAddrs_BasicReadModifyWrite(t *testing.T) {
	sa := newShardedAccounts()
	sa.Set("0xfrom", &AccountState{Address: "0xfrom", Balance: NewDecimal(100)})
	sa.Set("0xto", &AccountState{Address: "0xto", Balance: NewDecimal(0)})

	unlock := sa.LockAddrs("0xfrom", "0xto")
	from, ok := sa.GetLocked("0xfrom")
	if !ok {
		t.Fatal("GetLocked(0xfrom) miss")
	}
	to, ok := sa.GetLocked("0xto")
	if !ok {
		t.Fatal("GetLocked(0xto) miss")
	}
	from.Balance = from.Balance.Sub(NewDecimal(30))
	to.Balance = to.Balance.Add(NewDecimal(30))
	unlock()

	gotFrom, _ := sa.Get("0xfrom")
	gotTo, _ := sa.Get("0xto")
	if gotFrom.Balance.Float() != 70 {
		t.Errorf("from balance = %v, want 70", gotFrom.Balance.Float())
	}
	if gotTo.Balance.Float() != 30 {
		t.Errorf("to balance = %v, want 30", gotTo.Balance.Float())
	}
}

// TestShardedAccounts_LockAddrs_SetLockedInsertsNewAccount proves
// SetLocked can insert a brand-new address while the lock is held (the
// "recipient didn't exist yet" case), visible via a plain Get after
// unlock.
func TestShardedAccounts_LockAddrs_SetLockedInsertsNewAccount(t *testing.T) {
	sa := newShardedAccounts()
	unlock := sa.LockAddrs("0xnew")
	sa.SetLocked("0xnew", &AccountState{Address: "0xnew", Balance: NewDecimal(42)})
	unlock()

	got, ok := sa.Get("0xnew")
	if !ok || got.Balance.Float() != 42 {
		t.Fatal("SetLocked did not durably insert the new account")
	}
}

// TestShardedAccounts_LockAddrs_DedupesSameShard proves LockAddrs does not
// deadlock or double-lock when multiple addresses land in the same shard
// (guaranteed to happen at least once among enough addresses with only 64
// shards) -- and when the exact same address is passed twice.
func TestShardedAccounts_LockAddrs_DedupesSameShard(t *testing.T) {
	sa := newShardedAccounts()
	// Find two distinct addresses that hash to the same shard.
	var a, b string
	seen := map[int]string{}
	for i := 0; i < 100000; i++ {
		addr := fmt.Sprintf("0xdup%06d", i)
		idx := shardIndexFor(addr)
		if prior, ok := seen[idx]; ok {
			a, b = prior, addr
			break
		}
		seen[idx] = addr
	}
	if a == "" {
		t.Fatal("test setup: could not find two addresses sharing a shard within 100000 tries")
	}
	sa.Set(a, &AccountState{Address: a, Balance: NewDecimal(1)})
	sa.Set(b, &AccountState{Address: b, Balance: NewDecimal(2)})

	done := make(chan struct{})
	go func() {
		defer close(done)
		unlock := sa.LockAddrs(a, b, a, b) // same shard, and each address repeated
		unlock()
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("LockAddrs deadlocked on same-shard/duplicate addresses")
	}
}

// TestShardedAccounts_LockAddrs_ConcurrentTransfersDifferentShards is
// LockAddrs's actual point: many goroutines each locking their OWN
// disjoint {from,to} pair concurrently must genuinely run in parallel
// (never corrupt state, -race clean) and conserve the total balance
// exactly -- no lost updates, no double-spends.
func TestShardedAccounts_LockAddrs_ConcurrentTransfersDifferentShards(t *testing.T) {
	sa := newShardedAccounts()
	const n = 500
	for i := 0; i < n; i++ {
		addr := fmt.Sprintf("0xpar%05d", i)
		sa.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(1000)})
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			from := fmt.Sprintf("0xpar%05d", i)
			to := fmt.Sprintf("0xpar%05d", (i+1)%n)
			unlock := sa.LockAddrs(from, to)
			defer unlock()
			f, _ := sa.GetLocked(from)
			t, _ := sa.GetLocked(to)
			f.Balance = f.Balance.Sub(NewDecimal(10))
			t.Balance = t.Balance.Add(NewDecimal(10))
		}(i)
	}
	wg.Wait()

	var total float64
	sa.Range(func(_ string, acc *AccountState) bool {
		total += acc.Balance.Float()
		return true
	})
	if total != float64(n)*1000 {
		t.Fatalf("total balance after %d concurrent ring transfers = %v, want %v (lost update or corruption if different)", n, total, float64(n)*1000)
	}
}

// TestShardedAccounts_LockAddrs_ConcurrentOverlappingAddressSets is the
// deadlock-freedom proof: many goroutines locking OVERLAPPING,
// differently-ordered address pairs from a small shared pool concurrently
// must never deadlock (proves the sorted-shard-index lock ordering
// actually works, not just the disjoint-sets happy path above) and must
// still conserve the total balance exactly.
func TestShardedAccounts_LockAddrs_ConcurrentOverlappingAddressSets(t *testing.T) {
	sa := newShardedAccounts()
	const numAddrs = 20 // small pool -> heavy overlap, heavy contention
	addrs := make([]string, numAddrs)
	for i := range addrs {
		addrs[i] = fmt.Sprintf("0xhot%03d", i)
		sa.Set(addrs[i], &AccountState{Address: addrs[i], Balance: NewDecimal(10000)})
	}

	const rounds = 3000
	var wg sync.WaitGroup
	done := make(chan struct{})
	go func() {
		for r := 0; r < rounds; r++ {
			wg.Add(1)
			go func(r int) {
				defer wg.Done()
				from := addrs[r%numAddrs]
				to := addrs[(r*7+3)%numAddrs] // different, deliberately non-monotonic pairing
				if from == to {
					to = addrs[(r*7+5)%numAddrs]
				}
				unlock := sa.LockAddrs(from, to)
				defer unlock()
				f, _ := sa.GetLocked(from)
				t, _ := sa.GetLocked(to)
				if f.Balance.Float() >= 1 {
					f.Balance = f.Balance.Sub(NewDecimal(1))
					t.Balance = t.Balance.Add(NewDecimal(1))
				}
			}(r)
		}
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("LockAddrs deadlocked under heavy overlapping-address-set contention")
	}

	var total float64
	sa.Range(func(_ string, acc *AccountState) bool {
		total += acc.Balance.Float()
		return true
	})
	if total != float64(numAddrs)*10000 {
		t.Fatalf("total balance after %d concurrent overlapping transfers = %v, want %v", rounds, total, float64(numAddrs)*10000)
	}
}

// TestShardedAccounts_TryLockAddrs_SucceedsWhenUncontended proves the
// trivial case: nothing else holds either shard, so TryLockAddrs must
// succeed exactly like LockAddrs would, and GetLocked/SetLocked must work
// normally while held.
func TestShardedAccounts_TryLockAddrs_SucceedsWhenUncontended(t *testing.T) {
	sa := newShardedAccounts()
	a, b := "0xtla01", "0xtla02"
	sa.Set(a, &AccountState{Address: a, Balance: NewDecimal(100)})
	sa.Set(b, &AccountState{Address: b, Balance: NewDecimal(0)})

	unlock, ok := sa.TryLockAddrs(a, b)
	if !ok {
		t.Fatal("want ok=true when neither shard is contended")
	}
	defer unlock()

	accA, _ := sa.GetLocked(a)
	accB, _ := sa.GetLocked(b)
	accA.Balance = accA.Balance.Sub(NewDecimal(40))
	accB.Balance = accB.Balance.Add(NewDecimal(40))
	if accA.Balance.Float() != 60 || accB.Balance.Float() != 40 {
		t.Fatalf("got balances (%v, %v), want (60, 40)", accA.Balance.Float(), accB.Balance.Float())
	}
}

// TestShardedAccounts_TryLockAddrs_FailsWithoutBlockingWhenContended is the
// core property this primitive exists for: if another goroutine already
// holds one of the needed shards, TryLockAddrs must return ok=false
// IMMEDIATELY (not after waiting for the holder to release) — the whole
// point is a caller that can't get the lock right now falls back to a
// different code path instead of queuing.
func TestShardedAccounts_TryLockAddrs_FailsWithoutBlockingWhenContended(t *testing.T) {
	sa := newShardedAccounts()
	a, b := "0xtlb01", "0xtlb02"
	sa.Set(a, &AccountState{Address: a})
	sa.Set(b, &AccountState{Address: b})

	holderUnlock := sa.LockAddrs(a) // hold a's shard indefinitely
	defer holderUnlock()

	done := make(chan struct{})
	var unlock func()
	var ok bool
	go func() {
		unlock, ok = sa.TryLockAddrs(a, b)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("TryLockAddrs blocked instead of returning immediately")
	}
	if ok {
		unlock()
		t.Fatal("want ok=false while a's shard is held by another goroutine")
	}
}

// TestShardedAccounts_TryLockAddrs_ReleasesPartialAcquisitionOnFailure
// proves the rollback path: when the FIRST shard (in ascending index
// order) is free but a LATER one is contended, TryLockAddrs must not leak
// the first shard's lock — a bug here would deadlock every future caller
// of that shard, including the contending holder's own eventual retry.
func TestShardedAccounts_TryLockAddrs_ReleasesPartialAcquisitionOnFailure(t *testing.T) {
	sa := newShardedAccounts()
	// Find two addresses whose shard indices land in a known order, and a
	// third that collides with the second — shardIndexFor is deterministic,
	// so scanning a small range of synthetic addresses reliably finds this
	// shape without hardcoding hash values.
	var lowAddr, highAddr, collideAddr string
	seen := map[int]string{}
	for i := 0; i < 10000 && (lowAddr == "" || collideAddr == ""); i++ {
		addr := fmt.Sprintf("0xseek%05d", i)
		idx := shardIndexFor(addr)
		if lowAddr == "" {
			lowAddr, highAddr = addr, addr
			seen[idx] = addr
			continue
		}
		lowIdx := shardIndexFor(lowAddr)
		if idx < lowIdx {
			lowAddr = addr
		}
		if existing, ok := seen[idx]; ok && existing != addr && highAddr != "" && idx != shardIndexFor(lowAddr) {
			collideAddr = existing
			highAddr = addr
		}
		seen[idx] = addr
	}
	if lowAddr == "" || highAddr == "" || collideAddr == "" {
		t.Skip("could not synthesize a shard collision in the scan budget")
	}

	sa.Set(lowAddr, &AccountState{Address: lowAddr})
	sa.Set(highAddr, &AccountState{Address: highAddr})

	holderUnlock := sa.LockAddrs(highAddr) // holds the shard `collideAddr` also maps to
	defer holderUnlock()

	_, ok := sa.TryLockAddrs(lowAddr, highAddr)
	if ok {
		t.Fatal("want ok=false: highAddr's shard is already held")
	}

	// lowAddr's shard must have been released, not leaked -- prove it by
	// successfully locking it fresh right now.
	unlockLow, okLow := sa.TryLockAddrs(lowAddr)
	if !okLow {
		t.Fatal("lowAddr's shard was not released after the failed TryLockAddrs -- partial acquisition leaked")
	}
	unlockLow()
}

// TestShardedAccounts_TryLockAddrs_ConcurrentContentionNeverDeadlocks
// stress-tests TryLockAddrs the same way
// TestShardedAccounts_LockAddrs_ConcurrentOverlappingAddressSets stresses
// the blocking primitive: heavy overlap, many goroutines, non-monotonic
// pairing. Every attempt either fully succeeds or fully fails (ok=false,
// nothing held) -- balance conservation only needs to hold across the
// successful attempts, since a failed one is defined to be a no-op.
func TestShardedAccounts_TryLockAddrs_ConcurrentContentionNeverDeadlocks(t *testing.T) {
	sa := newShardedAccounts()
	const numAddrs = 20
	addrs := make([]string, numAddrs)
	for i := range addrs {
		addrs[i] = fmt.Sprintf("0xtry%03d", i)
		sa.Set(addrs[i], &AccountState{Address: addrs[i], Balance: NewDecimal(10000)})
	}

	const rounds = 3000
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
				unlock, ok := sa.TryLockAddrs(from, to)
				if !ok {
					return // contended -- defined as a no-op, same as a caller falling back
				}
				defer unlock()
				f, _ := sa.GetLocked(from)
				t, _ := sa.GetLocked(to)
				if f.Balance.Float() >= 1 {
					f.Balance = f.Balance.Sub(NewDecimal(1))
					t.Balance = t.Balance.Add(NewDecimal(1))
				}
			}(r)
		}
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("TryLockAddrs deadlocked (or hung) under heavy overlapping-address-set contention")
	}

	var total float64
	sa.Range(func(_ string, acc *AccountState) bool {
		total += acc.Balance.Float()
		return true
	})
	if total != float64(numAddrs)*10000 {
		t.Fatalf("total balance after %d concurrent TryLockAddrs attempts = %v, want %v (a successful attempt must still move exactly 1 unit)", rounds, total, float64(numAddrs)*10000)
	}
}
