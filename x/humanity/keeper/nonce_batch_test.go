package keeper

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// The nonce reservation is the replay guard on the payment path: a caller that
// never hears back would hang forever holding its sender's shard lock, and a
// caller told "reserved" when the database never saw the write could spend the
// same nonce twice. Everything below is about those two properties.

// Every request must be answered, whatever happens. A batch that drops one
// reply deadlocks that sender permanently.
func TestNonceBatcher_AnswersEveryRequest(t *testing.T) {
	cs := newTestState()
	const n = 200
	var wg sync.WaitGroup
	answered := make([]bool, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			done := make(chan struct{})
			go func() {
				cs.reserveNonceBatched(fmt.Sprintf("0x%040x", i), uint64(i), uint64(i+1))
				close(done)
			}()
			select {
			case <-done:
				answered[i] = true
			case <-time.After(10 * time.Second):
			}
		}(i)
	}
	wg.Wait()
	for i, ok := range answered {
		if !ok {
			t.Fatalf("request %d never got an answer — that sender would hold its nonce shard lock forever", i)
		}
	}
}

// Without a database there is nothing to compare against, and the single-row
// version returned true. The batched one must agree, or every no-DB test in
// this package would start rejecting transactions.
func TestNonceBatcher_NoDatabaseReservesOptimistically(t *testing.T) {
	cs := newTestState()
	reserved, err := cs.reserveNonceBatched("0xabc", 5, 6)
	if err != nil {
		t.Fatalf("no-DB reservation must not error, got %v", err)
	}
	if !reserved {
		t.Fatal("with no database the reservation must succeed, matching the single-row behaviour it replaces")
	}
}

// processNonceBatch must reply to a request whose result channel nobody is
// reading, rather than blocking the batcher and stalling every later
// reservation on the node.
func TestNonceBatcher_AbandonedCallerDoesNotStallTheBatcher(t *testing.T) {
	cs := newTestState()
	abandoned := &nonceReserveRequest{
		address: "0xdead", expected: 1, next: 2,
		result: make(chan nonceReserveResult), // unbuffered, nobody receiving
	}
	live := &nonceReserveRequest{
		address: "0xbeef", expected: 1, next: 2,
		result: make(chan nonceReserveResult, 1),
	}
	done := make(chan struct{})
	go func() { cs.processNonceBatch([]*nonceReserveRequest{abandoned, live}); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("processNonceBatch blocked on an abandoned caller — one dropped request would stall every reservation behind it")
	}
	select {
	case <-live.result:
	default:
		t.Fatal("the live caller must still have been answered")
	}
}

// The batcher is started lazily; starting it twice must not launch a second
// collector reading from the same channel, which would split batches
// unpredictably.
func TestNonceBatcher_StartsOnlyOnce(t *testing.T) {
	cs := newTestState()
	cs.ensureNonceBatcherStarted()
	first := cs.nonceBatchCh
	cs.ensureNonceBatcherStarted()
	if cs.nonceBatchCh != first {
		t.Fatal("a second start replaced the channel — two collectors would then race over the same requests")
	}
}
