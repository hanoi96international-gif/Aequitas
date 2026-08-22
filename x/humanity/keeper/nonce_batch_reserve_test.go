package keeper

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// The batch nonce pre-reservation replaces up to 100 database round trips with
// one, which is 26% of a transfer's measured time. It is only safe because it
// reserves EXACTLY the nonces the per-item path would have reserved, and
// leaves everything else alone.
//
// These pin that boundary. The dangerous failure is not "no speedup" -- it is
// reserving a nonce for a transaction that was never going to run, which would
// silently skip it for the sender.

func txWithNonce(n uint64) *types.Transaction {
	to := common.HexToAddress("0x000000000000000000000000000000000000dEaD")
	return types.NewTx(&types.LegacyTx{
		Nonce:    n,
		To:       &to,
		Value:    big.NewInt(1),
		Gas:      21000,
		GasPrice: big.NewInt(0),
	})
}

func items(sender string, nonces ...uint64) []*precomputedSendTx {
	out := make([]*precomputedSendTx, 0, len(nonces))
	for _, n := range nonces {
		out = append(out, &precomputedSendTx{tx: txWithNonce(n), sender: sender})
	}
	return out
}

// newReserveTestServer builds a server whose ChainState has no database, so
// ReserveNonce succeeds without one and LoadNonce reports 0. That isolates the
// run-detection logic, which is the part that can be wrong.
func newReserveTestServer() *EVMRPCServer {
	s := &EVMRPCServer{state: &ChainState{}}
	s.initNonceShards()
	return s
}

func allIndexes(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func TestConsecutiveRunIsReservedOnce(t *testing.T) {
	s := newReserveTestServer()
	sender := "0x1111111111111111111111111111111111111111"
	batch := items(sender, 0, 1, 2, 3, 4)

	before := batchNonceRuns.Load()
	s.preReserveBatchNonces(batch, allIndexes(len(batch)))

	for i, p := range batch {
		if !p.nonceReserved {
			t.Errorf("item %d (nonce %d) was not covered — every transaction in a "+
				"consecutive run should be, or the round trips are not actually saved",
				i, p.tx.Nonce())
		}
	}
	if got := batchNonceRuns.Load() - before; got != 1 {
		t.Errorf("performed %d range reservations for one consecutive run, want exactly 1 — "+
			"the entire point is replacing %d round trips with one", got, len(batch))
	}
	if got := s.nonceShardFor(sender).nonces[sender]; got != 5 {
		t.Errorf("stored nonce is %d, want 5 (0..4 consumed)", got)
	}
}

// The run must stop at the first nonce that is not the next one. Reserving
// past a gap would consume nonces for transactions that will be refused.
func TestRunStopsAtAGap(t *testing.T) {
	s := newReserveTestServer()
	sender := "0x2222222222222222222222222222222222222222"
	// 0,1 are consecutive; 7 is not; 8 only follows 7.
	batch := items(sender, 0, 1, 7, 8)

	s.preReserveBatchNonces(batch, allIndexes(len(batch)))

	if !batch[0].nonceReserved || !batch[1].nonceReserved {
		t.Error("the consecutive prefix 0,1 was not reserved")
	}
	if batch[2].nonceReserved || batch[3].nonceReserved {
		t.Fatal("a nonce PAST a gap was reserved.\n" +
			"  Those transactions are answered 'nonce too high' and never run, so " +
			"consuming their nonces silently skips them for the sender — the one " +
			"failure this optimisation must not introduce.")
	}
	if got := s.nonceShardFor(sender).nonces[sender]; got != 2 {
		t.Errorf("stored nonce is %d, want 2 — only the prefix may be consumed", got)
	}
}

// Each sender gets its own run; one sender's gap must not affect another.
func TestTwoSendersEachGetTheirOwnRun(t *testing.T) {
	s := newReserveTestServer()
	a := "0x3333333333333333333333333333333333333333"
	b := "0x4444444444444444444444444444444444444444"

	batch := append(items(a, 0, 1, 2), items(b, 0, 1)...)
	s.preReserveBatchNonces(batch, allIndexes(len(batch)))

	for i, p := range batch {
		if !p.nonceReserved {
			t.Errorf("item %d (%s nonce %d) not covered", i, p.sender[:6], p.tx.Nonce())
		}
	}
	if got := s.nonceShardFor(a).nonces[a]; got != 3 {
		t.Errorf("sender A stored nonce %d, want 3", got)
	}
	if got := s.nonceShardFor(b).nonces[b]; got != 2 {
		t.Errorf("sender B stored nonce %d, want 2", got)
	}
}

// Interleaved senders still form runs: grouping is by sender, and order within
// each sender is what has to be consecutive.
func TestInterleavedSendersStillFormRuns(t *testing.T) {
	s := newReserveTestServer()
	a := "0x5555555555555555555555555555555555555555"
	b := "0x6666666666666666666666666666666666666666"

	batch := []*precomputedSendTx{
		{tx: txWithNonce(0), sender: a},
		{tx: txWithNonce(0), sender: b},
		{tx: txWithNonce(1), sender: a},
		{tx: txWithNonce(1), sender: b},
	}
	s.preReserveBatchNonces(batch, allIndexes(len(batch)))

	for i, p := range batch {
		if !p.nonceReserved {
			t.Errorf("item %d not covered — interleaving two senders must not break "+
				"either one's run, since batches are grouped by sender", i)
		}
	}
}

// A single transaction gains nothing and must not be marked: the per-item path
// would make exactly the same one round trip, and marking it here would move
// the reservation away from the code that reports its errors.
func TestSingleTransactionIsLeftToThePerItemPath(t *testing.T) {
	s := newReserveTestServer()
	batch := items("0x7777777777777777777777777777777777777777", 0)

	s.preReserveBatchNonces(batch, allIndexes(len(batch)))

	if batch[0].nonceReserved {
		t.Error("a single-transaction batch was range-reserved")
	}
}

// Items that failed to decode carry no transaction. They must be skipped
// without disturbing the rest.
func TestFailedDecodesAreSkipped(t *testing.T) {
	s := newReserveTestServer()
	sender := "0x8888888888888888888888888888888888888888"
	batch := []*precomputedSendTx{
		{tx: txWithNonce(0), sender: sender},
		{err: errDecodeForTest},
		{tx: txWithNonce(1), sender: sender},
	}

	s.preReserveBatchNonces(batch, allIndexes(len(batch)))

	if !batch[0].nonceReserved || !batch[2].nonceReserved {
		t.Error("a failed decode in the middle prevented the surviving " +
			"transactions from forming a run; they are still consecutive for the sender")
	}
	if batch[1].nonceReserved {
		t.Error("an item that never decoded was marked as having a reserved nonce")
	}
}

var errDecodeForTest = errForTest("decode failed")

type errForTest string

func (e errForTest) Error() string { return string(e) }
