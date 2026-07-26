package keeper

import (
	"fmt"
	"sync"
	"testing"
)

// These tests exercise the nonce shards in isolation -- no ChainState, no DB,
// no RPC. They exist because sharding the nonce lock is only safe if the MAP
// is sharded with it, and that invariant is invisible at the call sites: every
// one of them looks correctly locked either way.
//
// Run with -race. The failure these guard against ("fatal error: concurrent
// map writes") aborts the process rather than failing a goroutine, so without
// -race a broken build can still pass by luck on a quiet machine.

// TestNonceShardsOwnTheirMaps is the direct statement of the invariant: two
// addresses that hash to different shards must not be able to reach the same
// map. When the lock was sharded but a single s.nonces map was kept, this held
// for the lock and not for the map -- which is what made concurrent senders
// crash the node.
func TestNonceShardsOwnTheirMaps(t *testing.T) {
	s := &EVMRPCServer{}
	s.initNonceShards()

	// Find two addresses in different shards, then prove a write through one
	// is invisible through the other. With a shared map it would be visible.
	var a, b string
	for i := 0; a == "" || b == ""; i++ {
		addr := fmt.Sprintf("0x%040x", i)
		if s.nonceShardFor(addr) == &s.nonceShards[0] {
			a = addr
		} else if s.nonceShardFor(addr) == &s.nonceShards[1] {
			b = addr
		}
		if i > 100000 {
			t.Fatal("could not find addresses for shards 0 and 1")
		}
	}

	sa, sb := s.nonceShardFor(a), s.nonceShardFor(b)
	if sa == sb {
		t.Fatalf("addresses %s and %s resolved to the same shard", a, b)
	}
	sa.nonces[a] = 42
	if _, leaked := sb.nonces[a]; leaked {
		t.Fatalf("shard for %s can see a key written through the shard for %s -- the map is shared", b, a)
	}
	if got := sa.nonces[a]; got != 42 {
		t.Fatalf("write through own shard lost: got %d, want 42", got)
	}
}

// TestNonceShardsConcurrentDistinctSenders reproduces the production shape:
// many different senders writing their nonces at the same time, each holding
// only its own shard's lock. This is the exact pattern the load generator
// produces with 72 concurrent pairs, and the pattern that aborted the node.
func TestNonceShardsConcurrentDistinctSenders(t *testing.T) {
	s := &EVMRPCServer{}
	s.initNonceShards()

	const senders = 256
	const writesPerSender = 200

	var wg sync.WaitGroup
	for i := 0; i < senders; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			addr := fmt.Sprintf("0x%040x", i)
			shard := s.nonceShardFor(addr)
			for n := 0; n < writesPerSender; n++ {
				shard.mu.Lock()
				shard.nonces[addr] = uint64(n)
				_ = shard.nonces[addr]
				shard.mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	for i := 0; i < senders; i++ {
		addr := fmt.Sprintf("0x%040x", i)
		shard := s.nonceShardFor(addr)
		if got := shard.nonces[addr]; got != writesPerSender-1 {
			t.Fatalf("sender %s: got nonce %d, want %d", addr, got, writesPerSender-1)
		}
	}
}

// TestNonceShardForIsStable guards the property every call site depends on:
// the same address always resolves to the same shard. If it did not, two
// requests from one sender could reserve the same nonce under two different
// locks -- the replay the sharding was explicitly required to keep preventing.
func TestNonceShardForIsStable(t *testing.T) {
	s := &EVMRPCServer{}
	s.initNonceShards()

	for i := 0; i < 1000; i++ {
		addr := fmt.Sprintf("0x%040x", i)
		first := s.nonceShardFor(addr)
		for r := 0; r < 5; r++ {
			if got := s.nonceShardFor(addr); got != first {
				t.Fatalf("%s resolved to two different shards", addr)
			}
		}
	}
}
