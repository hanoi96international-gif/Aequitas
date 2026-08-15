package keeper

import (
	"fmt"
	"testing"
)

// The parallel batch path (replay_parallel.go) may only ever be a SPEED-UP of
// the serial path (applyTransferDeltaLocked). TestParallelReplay_MatchesSerialExactly
// pins that for the ordinary case, but it seeds every account with an equal,
// far-below-cap balance and no IsHuman flag — so getAverageBalanceLocked
// returns 0, enforceWealthCapLockedCtx exits on its first line, and the cap is
// never actually exercised by that test at all.
//
// This one puts a real cap crossing inside a batchable run: the serial path
// trims the recipient to the cap and credits the excess to the four tokenomics
// pools, so if the parallel path skips that step the two produce different
// balances AND a different StateRoot for the identical block — a network fork.

// seedHumanAccounts is seedAccounts with IsHuman set (and cs.humanCount kept in
// step, exactly as addHuman does), so getAverageBalanceLocked/
// bootstrapMultiplierLocked report a real cap instead of "no meaningful
// average yet".
func seedHumanAccounts(cs *ChainState, n int, startBalance float64) []string {
	addrs := make([]string, n)
	cs.mu.Lock()
	for i := 0; i < n; i++ {
		addr := fmt.Sprintf("0xacct-%04d", i)
		addrs[i] = addr
		cs.accounts.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(startBalance), IsHuman: true})
		cs.humanCount++
	}
	cs.mu.Unlock()
	return addrs
}

// capCrossingBatch builds a run of pairwise-disjoint, demurrage-free transfers
// long enough to reach parallelReplayMinBatch, in which exactly one recipient
// is pushed past the wealth cap.
func capCrossingBatch(addrs []string, overCapAmount float64) []Transaction {
	txs := make([]Transaction, 0, parallelReplayMinBatch+4)
	// The cap crossing goes first so it is unambiguously inside the run.
	txs = append(txs, Transaction{
		Type: "transfer", Wallet: "0xwhale", To: "0xrecipient", Amount: overCapAmount,
	})
	for i := 0; len(txs) < parallelReplayMinBatch+2; i++ {
		txs = append(txs, Transaction{
			Type: "transfer", Wallet: addrs[2*i], To: addrs[2*i+1], Amount: 1,
		})
	}
	return txs
}

func TestParallelReplay_EnforcesWealthCapLikeSerial(t *testing.T) {
	// 40 humans => humanCountLocked() >= 25 => full 25x multiplier, and
	// getAverageBalanceLocked is a flat 1000 by protocol invariant, so the cap
	// is exactly 25,000 AEQ.
	const humans = 40
	const overCap = 26000.0
	const wantCapped = 25000.0

	seed := func(cs *ChainState) []string {
		addrs := seedHumanAccounts(cs, humans, 1000)
		cs.mu.Lock()
		cs.accounts.Set("0xwhale", &AccountState{Address: "0xwhale", Balance: NewDecimal(overCap)})
		cs.accounts.Set("0xrecipient", &AccountState{Address: "0xrecipient"})
		cs.mu.Unlock()
		return addrs
	}

	// ---- parallel: the whole run in ONE block, so the batch path engages ----
	parDAG, parCS := newDeterminismTestDAG()
	addrs := seed(parCS)
	txs := capCrossingBatch(addrs, overCap)
	if batch, _ := collectDisjointTransferBatch(txs, 0); len(batch) < parallelReplayMinBatch {
		t.Fatalf("test set does not reach the parallel batch threshold: %d < %d",
			len(batch), parallelReplayMinBatch)
	}
	if ok := parDAG.replayTransactions(&Block{Height: 1, Hash: "0xpar", Transactions: txs}, true); !ok {
		t.Fatal("parallel replay rejected a well-formed block")
	}

	// ---- serial: identical transactions, one per block, batch never forms ----
	serDAG, serCS := newDeterminismTestDAG()
	seed(serCS)
	for i, tx := range txs {
		b := &Block{Height: int64(i + 1), Hash: fmt.Sprintf("0xser-%d", i), Transactions: []Transaction{tx}}
		if ok := serDAG.replayTransactions(b, true); !ok {
			t.Fatalf("serial replay rejected tx %d", i)
		}
	}

	read := func(cs *ChainState, addr string) float64 {
		cs.mu.RLock()
		defer cs.mu.RUnlock()
		acc, ok := cs.accounts.Get(addr)
		if !ok {
			return 0
		}
		return acc.Balance.Float()
	}

	serRecipient := read(serCS, "0xrecipient")
	parRecipient := read(parCS, "0xrecipient")

	// Sanity: the serial path really did cap. If this ever fails the test's
	// premise is wrong, not the parallel path.
	if serRecipient != wantCapped {
		t.Fatalf("premise broken: serial path did not cap the recipient (got %.6f, want %.6f)",
			serRecipient, wantCapped)
	}

	if parRecipient != serRecipient {
		t.Errorf("wealth cap not enforced by the parallel batch path: recipient holds %.6f after parallel replay, %.6f after serial replay of the identical transactions",
			parRecipient, serRecipient)
	}
	for _, pool := range []string{validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr} {
		if got, want := read(parCS, pool), read(serCS, pool); got != want {
			t.Errorf("pool %s: parallel %.6f, serial %.6f — cap overflow was not redistributed", pool, got, want)
		}
	}
	if parRoot, serRoot := parCS.StateRoot(), serCS.StateRoot(); parRoot != serRoot {
		t.Errorf("StateRoot divergence — this forks the network\n  parallel: %s\n  serial:   %s", parRoot, serRoot)
	}
}
