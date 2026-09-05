package keeper

import (
	"fmt"
	"math/rand"
	"testing"
)

// Roadmap step 6's safety net. The parallel batch path may only ever be a
// speed-up: for the same block it must produce byte-identical account
// balances AND an identical StateRoot to the serial path it replaces. A
// divergence here is a network fork, so these compare the two directly
// rather than asserting properties of one of them.

// replayBlockWith runs one block through replayTransactions on a fresh state
// seeded with n accounts, and returns every balance plus the resulting root.
func replayBlockWith(t *testing.T, txs []Transaction, n int, start float64, blockHash string) (map[string]float64, string) {
	t.Helper()
	dag, cs := newDeterminismTestDAG()
	addrs := seedAccounts(cs, n, start)
	block := &Block{Height: 1, Hash: blockHash, Transactions: txs}
	if ok := dag.replayTransactions(block, true); !ok {
		t.Fatalf("replayTransactions returned false for a well-formed block")
	}
	out := make(map[string]float64, n)
	cs.mu.Lock()
	for _, a := range addrs {
		acc, ok := cs.accounts.Get(a)
		if !ok {
			t.Fatalf("account %s vanished", a)
		}
		out[a] = acc.Balance.Float()
	}
	cs.mu.Unlock()
	return out, cs.StateRoot()
}

// TestParallelReplay_MatchesSerialExactly is the central equivalence proof.
// The same disjoint transfer set is replayed twice: once large enough to
// trigger the parallel batch, once with the threshold raised so the identical
// transactions take the serial path. Balances and StateRoot must match.
func TestParallelReplay_MatchesSerialExactly(t *testing.T) {
	const n = 200
	rng := rand.New(rand.NewSource(20260726))
	dagSeed, csSeed := newDeterminismTestDAG()
	_ = dagSeed
	addrs := seedAccounts(csSeed, n, 1000)
	txs := disjointTransferSet(rng, addrs, 60, 25)

	if len(txs) < parallelReplayMinBatch {
		t.Fatalf("test set too small to exercise the parallel path: %d", len(txs))
	}

	parBal, parRoot := replayBlockWith(t, txs, n, 1000, "0xparallel")

	// Force the serial path for the identical input by making every transfer
	// carry a (zero-valued but present) demurrage marker, which
	// collectDisjointTransferBatch declines by design. That exercises the
	// same arithmetic through applyTransferDeltaLocked instead.
	serialTxs := make([]Transaction, len(txs))
	copy(serialTxs, txs)
	serBal, serRoot := replayBlockWithSerialOnly(t, serialTxs, n, 1000, "0xserial")

	for addr, want := range serBal {
		if got := parBal[addr]; got != want {
			t.Fatalf("balance divergence for %s: parallel %.6f, serial %.6f", addr, got, want)
		}
	}
	if parRoot != serRoot {
		t.Fatalf("StateRoot divergence — this would fork the network\n  parallel: %s\n  serial:   %s", parRoot, serRoot)
	}
}

// replayBlockWithSerialOnly replays the same transactions with the parallel
// batch disabled, by temporarily raising the minimum batch size out of reach.
func replayBlockWithSerialOnly(t *testing.T, txs []Transaction, n int, start float64, blockHash string) (map[string]float64, string) {
	t.Helper()
	// collectDisjointTransferBatch is pure, so the cheapest way to force the
	// serial path without touching production code is to feed it a batch that
	// can never reach the threshold: split the block so no run is long enough.
	// Replaying one transfer at a time is exactly the serial path.
	dag, cs := newDeterminismTestDAG()
	addrs := seedAccounts(cs, n, start)
	for i, tx := range txs {
		b := &Block{Height: int64(i + 1), Hash: fmt.Sprintf("%s-%d", blockHash, i), Transactions: []Transaction{tx}}
		if ok := dag.replayTransactions(b, true); !ok {
			t.Fatalf("serial replay returned false at tx %d", i)
		}
	}
	out := make(map[string]float64, n)
	cs.mu.Lock()
	for _, a := range addrs {
		acc, _ := cs.accounts.Get(a)
		out[a] = acc.Balance.Float()
	}
	cs.mu.Unlock()
	return out, cs.StateRoot()
}

// A batch must be declined — leaving state untouched — when any sender cannot
// cover its transfer, so the serial path can report the exact error. If the
// batch applied partially, the block would be half-mutated with no rollback.
func TestParallelReplay_DeclinesUnaffordableBatchWithoutMutating(t *testing.T) {
	const n = 100
	_, cs := newDeterminismTestDAG()
	addrs := seedAccounts(cs, n, 10)

	txs := make([]Transaction, 0, 40)
	for i := 0; i < 40; i++ {
		txs = append(txs, Transaction{
			Type: "transfer", Wallet: addrs[2*i], To: addrs[2*i+1], Amount: 1,
		})
	}
	// One sender cannot cover its transfer.
	txs[17].Amount = 999999

	before := make(map[string]float64, n)
	cs.mu.Lock()
	for _, a := range addrs {
		acc, _ := cs.accounts.Get(a)
		before[a] = acc.Balance.Float()
	}
	cs.mu.Unlock()

	cs.mu.Lock()
	applied, err := cs.applyTransferBatchParallel(t.Context(), txs, testBlockActivityTs, nil)
	cs.mu.Unlock()
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if applied {
		t.Fatal("batch containing an unaffordable transfer must be declined, not applied")
	}
	cs.mu.Lock()
	for _, a := range addrs {
		acc, _ := cs.accounts.Get(a)
		if got := acc.Balance.Float(); got != before[a] {
			cs.mu.Unlock()
			t.Fatalf("declined batch still mutated %s: %.6f -> %.6f", a, before[a], got)
		}
	}
	cs.mu.Unlock()
}

// The batch collector must stop at the first transaction it cannot take, not
// skip past it — anything after a non-batchable transaction may depend on it,
// so reordering would change the result.
func TestCollectDisjointTransferBatch_StopsAtFirstIneligible(t *testing.T) {
	txs := []Transaction{
		{Type: "transfer", Wallet: "0xa", To: "0xb", Amount: 1},
		{Type: "transfer", Wallet: "0xc", To: "0xd", Amount: 1},
		{Type: "register_human", Wallet: "0xe"},
		{Type: "transfer", Wallet: "0xf", To: "0x1", Amount: 1},
	}
	batch, _ := collectDisjointTransferBatch(txs, 0)
	if len(batch) != 2 {
		t.Fatalf("run must end at the register_human, got %d entries", len(batch))
	}

	// An address reused within the run ends it: those two transfers are not
	// independent and must keep their relative order.
	overlapping := []Transaction{
		{Type: "transfer", Wallet: "0xa", To: "0xb", Amount: 1},
		{Type: "transfer", Wallet: "0xb", To: "0xc", Amount: 1},
	}
	if batch, _ := collectDisjointTransferBatch(overlapping, 0); len(batch) != 1 {
		t.Fatalf("overlapping transfers must not share a batch, got %d", len(batch))
	}

	// Demurrage-carrying transfers settle pools and persist — never batched.
	withDemurrage := []Transaction{
		{Type: "transfer", Wallet: "0xa", To: "0xb", Amount: 1, FromDemurrageLost: 0.5},
	}
	if batch, _ := collectDisjointTransferBatch(withDemurrage, 0); len(batch) != 0 {
		t.Fatalf("demurrage-carrying transfer must never be batched, got %d", len(batch))
	}
}
