package keeper

import (
	"testing"
	"time"
)

// Get BLOCKS on a held shard; TryLockAddrs does not. Mixing them in one path
// throws away the whole point of the non-blocking one.
//
// This is not hypothetical. transferConcurrentWAL's eligibility checks called
// shardedAccounts.Get on both addresses immediately before TryLockAddrs, and
// flushWALBatch holds those same shard mutexes across its entire Postgres
// transaction -- 37ms on average, over 371 addresses. The measured phase split
// of a transfer:
//
//	precheck    46.41ms   <- the two Gets
//	wal_append  20.62ms
//	lock         0.003ms  <- TryLockAddrs, uncontended by then
//	apply        0.014ms
//	enqueue      0.46ms
//
// 69% of a transfer spent waiting for a lock the next line is specifically
// designed not to wait for. Worse, it hid itself: fast_path_pct read 99.82%,
// which looks like almost no contention, when it actually meant transfers
// waited for the shard and then found it free.
//
// The lesson generalises past the one call site: any blocking shard access on
// a path that ends in TryLockAddrs silently converts "bail to the batcher" into
// "queue behind whatever holds the shard".

func TestGetBlocksOnAHeldShardButTryLockDoesNot(t *testing.T) {
	sa := newShardedAccounts()
	const addr = "0xabc0000000000000000000000000000000000001"
	sa.Set(addr, &AccountState{})

	// Stand in for a flush: hold the shard the way flushWALBatch does.
	const held = 250 * time.Millisecond
	unlock := sa.LockAddrs(addr)
	released := make(chan struct{})
	go func() {
		time.Sleep(held)
		unlock()
		close(released)
	}()

	// TryLockAddrs must give up immediately -- that is its entire purpose.
	start := time.Now()
	gotUnlock, ok := sa.TryLockAddrs(addr)
	tryElapsed := time.Since(start)
	if ok {
		gotUnlock()
		t.Fatal("TryLockAddrs succeeded on a shard that is held — it must fail instantly instead, " +
			"so a contended transfer falls through to the batcher")
	}
	if tryElapsed > 50*time.Millisecond {
		t.Errorf("TryLockAddrs took %v on a held shard; it must not wait at all", tryElapsed)
	}

	// Get, by contrast, waits for the whole hold. This is the property that
	// made the precheck cost 46ms.
	start = time.Now()
	if _, found := sa.Get(addr); !found {
		t.Fatal("account vanished")
	}
	getElapsed := time.Since(start)
	<-released

	if getElapsed < held/2 {
		t.Fatalf("Get returned after %v while the shard was held for %v.\n"+
			"  If Get has become non-blocking, this test's premise is gone and the comment in "+
			"transferConcurrentWAL explaining why its prechecks were removed needs revisiting.",
			getElapsed, held)
	}

	// The contrast is the point: the two differ by orders of magnitude, so
	// putting a Get in front of a TryLockAddrs costs the entire hold.
	if getElapsed <= tryElapsed {
		t.Errorf("Get took %v and TryLockAddrs took %v — the blocking one should be far slower; "+
			"if they now behave alike, the reasoning behind the precheck removal no longer holds",
			getElapsed, tryElapsed)
	}
}
