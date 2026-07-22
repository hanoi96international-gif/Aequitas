package keeper

import (
	"encoding/json"
	"hash/fnv"
	"sync"
)

// numAccountShards partitions account storage into this many independent
// locks/maps. See SCALING_ARCHITECTURE.md for the full design this is
// Phase 1 of: today (Phase 1), shardedAccounts is used exactly like the
// map it replaces, always under cs.mu -- no new concurrency behavior yet.
// Its own per-shard mutexes exist for forward-compatibility with later
// phases (letting operations that only need specific shards eventually
// skip cs.mu entirely) and make the type safe to use completely on its
// own, which is what lets it be tested here in isolation, unconnected to
// any production code path.
//
// Picked as a power of two for a cheap, well-distributed modulo via
// bitmask; 64 is a starting point (more than typical core counts, so
// contention from two unrelated hot addresses colliding in the same
// shard stays rare) -- not tuned against real workload data yet, that's
// a later-phase question once this is actually wired into ChainState.
const numAccountShards = 64

// accountShard is one partition: its own map, its own mutex. Concurrent
// access to DIFFERENT shards never contends on the same lock -- the
// entire point of sharding. Native Go maps are not safe for ANY
// concurrent access from multiple goroutines, not even to different keys
// (a write can trigger a rehash that touches the whole internal
// structure) -- so this MUST be N separate map instances, never one
// shared map with striped locks around it.
type accountShard struct {
	mu   sync.Mutex
	data map[string]*AccountState
	// partialXOR is this shard's own contribution to the state-root
	// account accumulator (see SCALING_ARCHITECTURE.md Phase 4 / the
	// XorLeaf/CombinedXOR methods below) -- guarded by this shard's own mu,
	// not cs.mu, by design: the whole point is that two goroutines updating
	// accounts in DIFFERENT shards can update their respective partialXOR
	// without contending on any shared lock.
	partialXOR [32]byte
}

// shardedAccounts is a map[string]*AccountState replacement, safe for
// concurrent use on its own (every method takes and releases its own
// shard lock(s) internally -- callers do not need to hold any external
// lock to call these methods safely). Deliberately mirrors Go's native
// map semantics (comma-ok Get, Range instead of `for range`, explicit
// Delete/Len) so migrating existing `cs.accounts[...]` call sites is a
// mechanical, low-risk transformation -- see SCALING_ARCHITECTURE.md's
// Phase 2 for that migration, NOT done by this file.
type shardedAccounts struct {
	shards [numAccountShards]*accountShard
}

func newShardedAccounts() *shardedAccounts {
	sa := &shardedAccounts{}
	for i := range sa.shards {
		sa.shards[i] = &accountShard{data: make(map[string]*AccountState)}
	}
	return sa
}

// shardIndexFor is the routing function every operation on a given
// address must agree on -- used both to pick a single shard (Get/Set/
// Delete) and, in a later phase, to decide cross-shard lock ordering for
// operations touching two addresses. FNV-1a: fast, well-distributed for
// short string keys, no cryptographic properties needed here (this is
// purely a load-balancing hash, not security-relevant).
func shardIndexFor(addr string) int {
	h := fnv.New32a()
	h.Write([]byte(addr))
	return int(h.Sum32() % uint32(numAccountShards))
}

func (sa *shardedAccounts) shardFor(addr string) *accountShard {
	return sa.shards[shardIndexFor(addr)]
}

// Get mirrors `acc, ok := m[addr]`, including on a nil receiver: reading a
// nil native map never panics (returns the zero value, ok=false), and
// several existing tests construct a bare &ChainState{} whose accounts
// field is left as a nil *shardedAccounts -- those tests only ever read
// (e.g. via snapshotForRollbackLocked), so this nil check preserves their
// pre-migration behavior exactly instead of turning a safe no-op into a
// nil-pointer panic.
func (sa *shardedAccounts) Get(addr string) (*AccountState, bool) {
	if sa == nil {
		return nil, false
	}
	s := sa.shardFor(addr)
	s.mu.Lock()
	defer s.mu.Unlock()
	acc, ok := s.data[addr]
	return acc, ok
}

// Set mirrors `m[addr] = acc` -- including panicking on a nil receiver, the
// same as assigning into a nil native map. Every real construction path
// (NewChainState, newTestState, etc.) always calls newShardedAccounts(), so
// this can only fire for a test that both zero-value-constructs ChainState
// AND then tries to mutate cs.accounts, which would have been an equally
// invalid nil-map write before this migration.
func (sa *shardedAccounts) Set(addr string, acc *AccountState) {
	s := sa.shardFor(addr)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[addr] = acc
}

// Delete mirrors `delete(m, addr)`, including on a nil receiver: deleting
// from a nil native map is always a safe no-op in Go, never a panic -- see
// Get's comment for why that nil-safety matters here too.
func (sa *shardedAccounts) Delete(addr string) {
	if sa == nil {
		return
	}
	s := sa.shardFor(addr)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, addr)
}

// XorLeaf folds addr's leaf change into its shard's own partialXOR,
// swapping oldLeaf out and newLeaf in -- the same self-inverse XOR-out/
// XOR-in technique ChainState.updateAccountLeafLocked uses for the single
// global cs.accountSetXOR (state.go), just scoped to one shard's own lock
// instead of cs.mu.
//
// SCALING_ARCHITECTURE.md Phase 4: this and CombinedXOR below are a
// complete, independently-tested primitive for a per-shard state-root
// accumulator, built the same way shardedAccounts itself was in Phase 1 --
// NOT YET wired into cs.accountSetXOR/StateRoot(), which remain the single
// global accumulator described in state.go and stay the actual source of
// truth for now. The real benefit of routing leaf updates through here
// instead of cs.accountSetXOR only materializes once a LATER phase (5)
// moves account mutation off cs.mu onto these same per-shard locks --
// before that, cs.mu already serializes every leaf update anyway, so
// switching StateRoot's source of truth over would add a second lock
// acquisition for zero present benefit. Kept here, tested, and ready for
// that wiring rather than built later under time pressure.
//
// Nil-safe no-op, consistent with every other method here.
func (sa *shardedAccounts) XorLeaf(addr string, oldLeaf, newLeaf [32]byte) {
	if sa == nil {
		return
	}
	s := sa.shardFor(addr)
	s.mu.Lock()
	defer s.mu.Unlock()
	xorInto(&s.partialXOR, oldLeaf)
	xorInto(&s.partialXOR, newLeaf)
}

// CombinedXOR returns the XOR of every shard's partialXOR -- equal to the
// XOR of every stored account's current leaf, by the same associative/
// commutative XOR-accumulator reasoning cs.accountSetXOR already relies on
// (see state.go), just computed as a combination of N independent partial
// sums instead of one running total. O(numAccountShards): acceptable for
// the same reason Len() is -- this is a read-time combination step, not a
// per-mutation cost.
func (sa *shardedAccounts) CombinedXOR() [32]byte {
	if sa == nil {
		return [32]byte{}
	}
	var total [32]byte
	for _, s := range sa.shards {
		s.mu.Lock()
		xorInto(&total, s.partialXOR)
		s.mu.Unlock()
	}
	return total
}

// Len mirrors `len(m)`, including on a nil receiver (len(nilMap) == 0 in
// Go, never a panic -- see Get's comment). O(numAccountShards), not O(1) --
// acceptable here since every existing len(cs.accounts) call site is
// cold-path bookkeeping (snapshot sizing, stats), never a per-transfer hot
// path.
func (sa *shardedAccounts) Len() int {
	if sa == nil {
		return 0
	}
	total := 0
	for _, s := range sa.shards {
		s.mu.Lock()
		total += len(s.data)
		s.mu.Unlock()
	}
	return total
}

// Range calls fn for every account across all shards, stopping early if
// fn returns false. Mirrors `for addr, acc := range m`. Iteration order
// is not guaranteed (varies by shard and by Go's own native map
// iteration order within a shard) and must not be relied on for
// determinism -- exactly the same non-guarantee Go's native map range
// already has, so no existing caller can have relied on order.
//
// fn must NOT call back into sa for the same shard (Get/Set/Delete/Range)
// -- that would deadlock on the shard mutex Range is already holding.
// Existing cs.accounts range-loop bodies in this codebase call other
// ChainState methods, not cs.accounts itself, from inside the loop, so
// this restriction matches how the map is actually used today.
//
// Nil-safe (zero iterations), mirroring `range` over a nil native map --
// see Get's comment.
func (sa *shardedAccounts) Range(fn func(addr string, acc *AccountState) bool) {
	if sa == nil {
		return
	}
	for _, s := range sa.shards {
		s.mu.Lock()
		for addr, acc := range s.data {
			if !fn(addr, acc) {
				s.mu.Unlock()
				return
			}
		}
		s.mu.Unlock()
	}
}

// Clone returns a new, independent shardedAccounts holding a deep copy of
// every account currently stored (each *AccountState copied by value, not
// shared by pointer -- mutating the original after Clone() never affects
// the clone, mirroring the `accCopy := *acc` pattern every existing
// backup-before-mutating call site already used against the plain map).
// Used by callers that need "snapshot now, restore verbatim later if
// something fails" (see ResyncFromSnapshotURL) -- restoring is then just
// reassigning cs.accounts to the clone, since cs.accounts is a pointer
// field.
//
// Does NOT carry over the source's per-shard partialXOR state (see
// XorLeaf/CombinedXOR): Clone only ever calls Set, and Set/XorLeaf are
// deliberately independent operations here, the same way mutating an
// AccountState's fields and calling updateAccountLeafLocked are two
// separate steps in state.go today -- nothing silently keeps them in sync.
// A caller that needs the clone's own CombinedXOR() to be meaningful must
// rebuild it explicitly (Range + XorLeaf per account) after cloning.
func (sa *shardedAccounts) Clone() *shardedAccounts {
	clone := newShardedAccounts()
	sa.Range(func(addr string, acc *AccountState) bool {
		cp := *acc
		clone.Set(addr, &cp)
		return true
	})
	return clone
}

// MarshalJSON lets `json.Marshal(sa)` behave like `json.Marshal(m)` did
// for the plain map -- needed for cs.save()'s no-DB fallback path, the
// one production call site that serializes the whole account map
// directly rather than through individual Get/Set calls.
func (sa *shardedAccounts) MarshalJSON() ([]byte, error) {
	combined := make(map[string]*AccountState, sa.Len())
	sa.Range(func(addr string, acc *AccountState) bool {
		combined[addr] = acc
		return true
	})
	return json.Marshal(combined)
}
