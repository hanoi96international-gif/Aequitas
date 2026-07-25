package keeper

import (
	"fmt"
	"math/rand"
	"testing"
)

// This file is SCALING_ARCHITECTURE.md's 2026-07-25 deep-dive roadmap item 7:
// the determinism/equivalence safety net that must exist and pass BEFORE
// replayTransactions' sequential per-tx loop is ever changed to run disjoint
// transfers concurrently (roadmap item 1 — explicitly flagged there as the
// riskiest single step in the whole plan, the same bug class that caused
// Solana's Feb 2026 outage). It does not itself parallelize anything —
// replayTransactions is untouched — it locks in the ONE mathematical
// property any future parallel implementation would rely on: that a batch
// of transfers with pairwise-disjoint touched addresses produces an
// IDENTICAL final state no matter what order they're applied in. If this
// ever fails, the "disjoint => safely reorderable/parallelizable"
// assumption is wrong and no parallel replay implementation should be
// attempted until whatever broke it is understood.

// newDeterminismTestDAG builds a fresh, no-DB (cs.db == nil) BlockDAG+
// ChainState pair — same minimal-construction pattern block_replay_ctx_db_test.go
// established for exercising replayTransactions directly without a real
// Postgres instance.
func newDeterminismTestDAG() (*BlockDAG, *ChainState) {
	cs := newTestState()
	dag := newOrphanTestDAG()
	dag.state = cs
	dag.bootHeight = 0
	dag.replayedBlocks = make(map[string]bool)
	dag.replayFailures = make(map[string]replayFailureState)
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}
	return dag, cs
}

// seedAccounts creates n accounts (addresses "0xacct-0000".."0xacct-000N")
// each with startBalance, mirroring block_replay_ctx_db_test.go's seeding
// style (Balance set, LastActivityAt left at its zero value so
// effectiveBalance reports no pending demurrage — see that test's own
// comment for why this is a clean, decay-free starting point).
func seedAccounts(cs *ChainState, n int, startBalance float64) []string {
	addrs := make([]string, n)
	cs.mu.Lock()
	for i := 0; i < n; i++ {
		addr := fmt.Sprintf("0xacct-%04d", i)
		addrs[i] = addr
		cs.accounts.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(startBalance)})
	}
	cs.mu.Unlock()
	return addrs
}

// disjointTransferSet builds count transfers over 2*count of the given
// addresses, each address used by at most one transfer in the whole
// returned set (the "safe to run in any order / concurrently" case a future
// parallel replay implementation would target) — random but
// balance-respecting amounts, seeded by rng for reproducibility.
func disjointTransferSet(rng *rand.Rand, addrs []string, count int, maxAmount float64) []Transaction {
	if count*2 > len(addrs) {
		panic("disjointTransferSet: not enough addresses for a disjoint set of this size")
	}
	perm := rng.Perm(len(addrs))
	txs := make([]Transaction, count)
	for i := 0; i < count; i++ {
		from := addrs[perm[2*i]]
		to := addrs[perm[2*i+1]]
		amount := 1 + rng.Float64()*(maxAmount-1)
		txs[i] = Transaction{Type: "transfer", Wallet: from, To: to, Amount: amount}
	}
	return txs
}

// runReplayWithOrder seeds a fresh ChainState with n accounts at
// startBalance, replays a single block containing txs (in the given order,
// under the given block hash so replayedBlocks dedup can't short-circuit a
// second run), and returns every seeded account's final balance plus the
// resulting StateRoot.
func runReplayWithOrder(t *testing.T, n int, startBalance float64, txs []Transaction, blockHash string) (balances map[string]float64, stateRoot string) {
	t.Helper()
	dag, cs := newDeterminismTestDAG()
	addrs := seedAccounts(cs, n, startBalance)

	block := &Block{
		Height:       1,
		Hash:         blockHash,
		Transactions: txs,
	}
	if ok := dag.replayTransactions(block, true); !ok {
		t.Fatalf("replayTransactions returned false (hard failure) for a well-formed disjoint-transfer batch")
	}

	balances = make(map[string]float64, n)
	cs.mu.Lock()
	for _, a := range addrs {
		acc, ok := cs.accounts.Get(a)
		if !ok {
			t.Fatalf("account %s missing after replay", a)
		}
		balances[a] = acc.Balance.Float()
	}
	cs.mu.Unlock()
	return balances, cs.StateRoot()
}

// TestReplayTransactions_DisjointTransfers_OrderIndependent is the core
// determinism check: the exact same set of pairwise-disjoint transfers,
// applied in two different orders (original vs. reversed), must produce
// byte-identical final balances and StateRoot. replayTransactions itself
// stays fully sequential here — this only proves the order in which
// disjoint transfers are sequentially applied never affects the outcome,
// which is the precondition for ever letting them run concurrently instead.
func TestReplayTransactions_DisjointTransfers_OrderIndependent(t *testing.T) {
	const n = 16
	const startBalance = 1000.0
	rng := rand.New(rand.NewSource(42))
	addrsForGen := make([]string, n)
	for i := range addrsForGen {
		addrsForGen[i] = fmt.Sprintf("0xacct-%04d", i)
	}
	txs := disjointTransferSet(rng, addrsForGen, n/2, 100)

	reversed := make([]Transaction, len(txs))
	for i, tx := range txs {
		reversed[len(txs)-1-i] = tx
	}

	balancesA, rootA := runReplayWithOrder(t, n, startBalance, txs, "det-test-original-order")
	balancesB, rootB := runReplayWithOrder(t, n, startBalance, reversed, "det-test-reversed-order")

	if rootA != rootB {
		t.Fatalf("StateRoot depends on tx order for a disjoint transfer batch: original=%s reversed=%s — the disjoint=>reorderable assumption is FALSE, do not parallelize replayTransactions until this is understood", rootA, rootB)
	}
	for addr, balA := range balancesA {
		balB, ok := balancesB[addr]
		if !ok {
			t.Fatalf("account %s present in original-order run but missing in reversed-order run", addr)
		}
		if balA != balB {
			t.Errorf("account %s balance depends on tx order: original=%v reversed=%v", addr, balA, balB)
		}
	}
}

// TestReplayTransactions_DisjointTransfers_OrderIndependent_Fuzz repeats the
// same order-independence check across many random disjoint-transfer sets
// and random shuffles (not just a single reversal), per SCALING_ARCHITECTURE.md
// roadmap item 7's call for fuzzing, not a single example. Deliberately
// modest in size/iterations to stay fast enough for the normal test suite —
// increase via manual local runs (go test -run OrderIndependent_Fuzz -count=N)
// for deeper confidence before ever acting on this test's green result.
func TestReplayTransactions_DisjointTransfers_OrderIndependent_Fuzz(t *testing.T) {
	const n = 20
	const startBalance = 1000.0
	const iterations = 25

	addrsForGen := make([]string, n)
	for i := range addrsForGen {
		addrsForGen[i] = fmt.Sprintf("0xacct-%04d", i)
	}

	for iter := 0; iter < iterations; iter++ {
		rng := rand.New(rand.NewSource(int64(1000 + iter)))
		txCount := 1 + rng.Intn(n/2)
		txs := disjointTransferSet(rng, addrsForGen, txCount, 250)

		shuffled := make([]Transaction, len(txs))
		copy(shuffled, txs)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

		hashA := fmt.Sprintf("det-fuzz-%d-orig", iter)
		hashB := fmt.Sprintf("det-fuzz-%d-shuffled", iter)
		balancesA, rootA := runReplayWithOrder(t, n, startBalance, txs, hashA)
		balancesB, rootB := runReplayWithOrder(t, n, startBalance, shuffled, hashB)

		if rootA != rootB {
			t.Fatalf("iter %d: StateRoot depends on tx order (txCount=%d): orig=%s shuffled=%s", iter, txCount, rootA, rootB)
		}
		for addr, balA := range balancesA {
			if balB := balancesB[addr]; balA != balB {
				t.Fatalf("iter %d: account %s balance depends on tx order: orig=%v shuffled=%v", iter, addr, balA, balB)
			}
		}
	}
}
