package keeper

import (
	"fmt"
	"testing"
)

// LastActivityAt is every account's demurrage clock: settleDemurrageLocked and
// effectiveBalance both measure decay from it, and checkAndMoveToEscrowLocked
// decides the 2.5-year inactivity sweep by it. The INGESTION path
// (transferLocked) resets it on both parties of a transfer — "sending counts as
// using the money", per its own comment.
//
// Replay must therefore reset it too, and must arrive at the SAME instant on
// every node. Two things make that non-obvious, and both were wrong:
//
//   - the serial replay path (applyTransferDeltaLocked) never touched the clock
//     at all, so on every node that merely replayed a transfer — i.e. all of
//     them except the one that produced the block — both wallets kept ageing as
//     if idle;
//   - the parallel batch path (replay_parallel.go) did touch it, but with the
//     replaying node's own wall clock, which is the exact pattern
//     FromDemurrageLost and DistributionAt exist to avoid (see Transaction's own
//     field comments): a node replaying historical blocks during a resync would
//     stamp "now" onto a transfer that happened years ago.
//
// A block carries the instant already: block.Timestamp. These pin that both
// paths use it, and that they agree — a batch of 16+ disjoint transfers takes
// the parallel path, anything shorter takes the serial one, so the two must not
// be distinguishable in the state they leave behind.

const testBlockActivityTs = int64(1_700_000_000)

// replayAndReadClocks replays one block of txs (with an explicit Timestamp) and
// returns each seeded account's LastActivityAt.
func replayAndReadClocks(t *testing.T, txs []Transaction, n int, blockHash string) map[string]int64 {
	t.Helper()
	dag, cs := newDeterminismTestDAG()
	addrs := seedAccounts(cs, n, 1000)
	block := &Block{Height: 1, Hash: blockHash, Timestamp: testBlockActivityTs, Transactions: txs}
	if ok := dag.replayTransactions(block, true); !ok {
		t.Fatalf("replayTransactions returned false for a well-formed block")
	}
	out := make(map[string]int64, n)
	cs.mu.Lock()
	for _, a := range addrs {
		acc, ok := cs.accounts.Get(a)
		if !ok {
			t.Fatalf("account %s vanished", a)
		}
		out[a] = acc.LastActivityAt
	}
	cs.mu.Unlock()
	return out
}

// oneToOneTransfers builds count transfers over 2*count distinct addresses, so
// the run is pairwise disjoint and demurrage-free — i.e. batchable, and
// order-independent by the property the determinism tests already pin.
func oneToOneTransfers(addrs []string, count int) []Transaction {
	txs := make([]Transaction, count)
	for i := 0; i < count; i++ {
		txs[i] = Transaction{Type: "transfer", Wallet: addrs[2*i], To: addrs[2*i+1], Amount: 10}
	}
	return txs
}

func TestReplay_StampsActivityClockFromTheBlock(t *testing.T) {
	// Long enough to reach the parallel path.
	const count = parallelReplayMinBatch + 4
	const n = count * 2
	addrs := make([]string, n)
	for i := range addrs {
		addrs[i] = fmt.Sprintf("0xacct-%04d", i)
	}
	txs := oneToOneTransfers(addrs, count)

	clocks := replayAndReadClocks(t, txs, n, "0xparallel-clock")
	for i := 0; i < count; i++ {
		from, to := addrs[2*i], addrs[2*i+1]
		if got := clocks[from]; got != testBlockActivityTs {
			t.Fatalf("sender %s: LastActivityAt = %d, want the block's own timestamp %d\n"+
				"a replaying node must reset the demurrage clock exactly as the producing node did",
				from, got, testBlockActivityTs)
		}
		if got := clocks[to]; got != testBlockActivityTs {
			t.Fatalf("recipient %s: LastActivityAt = %d, want the block's own timestamp %d",
				to, got, testBlockActivityTs)
		}
	}
}

func TestReplay_ActivityClockIdenticalSerialAndParallel(t *testing.T) {
	const count = parallelReplayMinBatch + 4
	const n = count * 2
	addrs := make([]string, n)
	for i := range addrs {
		addrs[i] = fmt.Sprintf("0xacct-%04d", i)
	}
	txs := oneToOneTransfers(addrs, count)

	parallel := replayAndReadClocks(t, txs, n, "0xpar-clock")

	// Same transfers, one per block: every run is below parallelReplayMinBatch,
	// so each takes the serial path — the identical trick
	// replayBlockWithSerialOnly uses, without touching production code.
	dag, cs := newDeterminismTestDAG()
	seedAccounts(cs, n, 1000)
	for i, tx := range txs {
		b := &Block{
			Height:       int64(i + 1),
			Hash:         fmt.Sprintf("0xser-clock-%d", i),
			Timestamp:    testBlockActivityTs,
			Transactions: []Transaction{tx},
		}
		if ok := dag.replayTransactions(b, true); !ok {
			t.Fatalf("serial replay returned false at tx %d", i)
		}
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()
	for _, a := range addrs {
		acc, ok := cs.accounts.Get(a)
		if !ok {
			t.Fatalf("account %s vanished", a)
		}
		if acc.LastActivityAt != parallel[a] {
			t.Fatalf("demurrage-clock divergence for %s: serial %d, parallel %d\n"+
				"whether a node batches depends only on how many transfers happen to sit\n"+
				"consecutively in one block, which must not change the state it produces",
				a, acc.LastActivityAt, parallel[a])
		}
	}
}

// Nothing on the block-acceptance path validates block.Timestamp against the
// local clock — the only comparison against it in block.go is a fixed
// activation constant. Since replay now stamps the demurrage clock FROM that
// field, a proposer could otherwise push any account it touches years into the
// future and exempt it from both demurrage and the 2.5-year escrow sweep.
func TestReplay_ActivityClockNeverStampsTheFuture(t *testing.T) {
	const count = parallelReplayMinBatch + 4
	const n = count * 2
	addrs := make([]string, n)
	for i := range addrs {
		addrs[i] = fmt.Sprintf("0xacct-%04d", i)
	}
	txs := oneToOneTransfers(addrs, count)

	future := nowUnix() + 100*365*24*3600 // a century ahead
	dag, cs := newDeterminismTestDAG()
	seedAccounts(cs, n, 1000)
	block := &Block{Height: 1, Hash: "0xfuture-clock", Timestamp: future, Transactions: txs}
	if ok := dag.replayTransactions(block, true); !ok {
		t.Fatal("replayTransactions returned false for a well-formed block")
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()
	for _, a := range addrs {
		acc, ok := cs.accounts.Get(a)
		if !ok {
			t.Fatalf("account %s vanished", a)
		}
		if acc.LastActivityAt > nowUnix() {
			t.Fatalf("%s: LastActivityAt = %d is in the future (block claimed %d)\n"+
				"a proposer-supplied timestamp must never exempt an account from demurrage",
				a, acc.LastActivityAt, future)
		}
	}
}
