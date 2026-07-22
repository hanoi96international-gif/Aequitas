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

// Get mirrors `acc, ok := m[addr]`.
func (sa *shardedAccounts) Get(addr string) (*AccountState, bool) {
	s := sa.shardFor(addr)
	s.mu.Lock()
	defer s.mu.Unlock()
	acc, ok := s.data[addr]
	return acc, ok
}

// Set mirrors `m[addr] = acc`.
func (sa *shardedAccounts) Set(addr string, acc *AccountState) {
	s := sa.shardFor(addr)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[addr] = acc
}

// Delete mirrors `delete(m, addr)`.
func (sa *shardedAccounts) Delete(addr string) {
	s := sa.shardFor(addr)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, addr)
}

// Len mirrors `len(m)`. O(numAccountShards), not O(1) -- acceptable here
// since every existing len(cs.accounts) call site is cold-path bookkeeping
// (snapshot sizing, stats), never a per-transfer hot path.
func (sa *shardedAccounts) Len() int {
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
func (sa *shardedAccounts) Range(fn func(addr string, acc *AccountState) bool) {
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
