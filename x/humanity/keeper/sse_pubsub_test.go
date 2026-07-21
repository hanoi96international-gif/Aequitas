package keeper

import (
	"sync"
	"testing"
	"time"
)

// TestSSEPubSub_SubscribeNotifyUnsubscribe exercises SubscribeNewBlocks/
// notifyNewBlock/unsubscribe under concurrent load — the exact pattern
// handleBlockEvents (api.go) and ProduceBlock/AddPeerBlock (block.go) use.
// Guards against: a subscriber not receiving a notification while
// subscribed, notifyNewBlock blocking on a full/unsubscribed channel, and a
// data race between concurrent subscribe/notify/unsubscribe (run with -race
// in CI via `go test ./...`).
func TestSSEPubSub_SubscribeNotifyUnsubscribe(t *testing.T) {
	dag := &BlockDAG{newBlockSubs: make(map[chan struct{}]struct{})}

	ch, unsubscribe := dag.SubscribeNewBlocks()
	defer unsubscribe()

	dag.notifyNewBlock(&Block{Hash: "0xabc"})
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber did not receive notification")
	}

	// A second notify before the first is drained must not block (buffered
	// size 1, non-blocking send) — this is the "slow reader" case
	// notifyNewBlock's own comment describes.
	done := make(chan struct{})
	go func() {
		dag.notifyNewBlock(&Block{Hash: "0xdef"})
		dag.notifyNewBlock(&Block{Hash: "0xghi"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("notifyNewBlock blocked on a full subscriber channel")
	}

	// Concurrent subscribe/notify/unsubscribe from many goroutines must not
	// race or deadlock.
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, unsub := dag.SubscribeNewBlocks()
			defer unsub()
			dag.notifyNewBlock(&Block{Hash: "0xrace"})
			select {
			case <-c:
			case <-time.After(2 * time.Second):
			}
		}()
	}
	waitDone := make(chan struct{})
	go func() { wg.Wait(); close(waitDone) }()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent subscribe/notify/unsubscribe deadlocked")
	}

	// After unsubscribe, the subscriber must be gone from the map — notify
	// must not panic or leak against a channel nobody reads anymore.
	ch2, unsubscribe2 := dag.SubscribeNewBlocks()
	unsubscribe2()
	dag.notifyNewBlock(&Block{Hash: "0xafter-unsub"})
	select {
	case <-ch2:
		t.Fatal("unsubscribed channel still received a notification")
	case <-time.After(100 * time.Millisecond):
	}
}
