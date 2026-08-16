package keeper

import (
	"fmt"
	"testing"
)

// This file extends replay_parallel_wealthcap_test.go's single-recipient
// wealth-cap regression test to the multi-recipient case, as part of an
// independent verification (2026-08-16 money-movement audit) of the
// wealth-cap fix in applyTransferBatchParallel (replay_parallel.go, "FIX
// (audit 2026-08-15)" comment). The single-recipient test already proves the
// fix works and was confirmed passing during this audit; this test checks a
// case the original didn't: TWO separate over-cap crossings landing in the
// SAME disjoint parallel batch, verifying the phase-1 check loop
// (applyTransferBatchParallel, "for _, it := range items") correctly catches
// every crossing in the batch, not just the first one it happens to see.

// multiCapCrossingBatch builds a run of pairwise-disjoint, demurrage-free
// transfers long enough to reach parallelReplayMinBatch, in which TWO
// distinct recipients are each independently pushed past the wealth cap.
func multiCapCrossingBatch(addrs []string, overCapAmount float64) []Transaction {
	txs := make([]Transaction, 0, parallelReplayMinBatch+6)
	// Both cap crossings go first so they are unambiguously inside the run.
	txs = append(txs, Transaction{
		Type: "transfer", Wallet: "0xwhale1", To: "0xrecipient1", Amount: overCapAmount,
	})
	txs = append(txs, Transaction{
		Type: "transfer", Wallet: "0xwhale2", To: "0xrecipient2", Amount: overCapAmount,
	})
	for i := 0; len(txs) < parallelReplayMinBatch+3; i++ {
		txs = append(txs, Transaction{
			Type: "transfer", Wallet: addrs[2*i], To: addrs[2*i+1], Amount: 1,
		})
	}
	return txs
}

func TestParallelReplay_EnforcesWealthCapForMultipleRecipientsInSameBatch(t *testing.T) {
	// 40 humans => humanCountLocked() >= 25 => full 25x multiplier, and
	// getAverageBalanceLocked is a flat 1000 by protocol invariant, so the
	// cap is exactly 25,000 AEQ for both recipients.
	const humans = 40
	const overCap = 26000.0
	const wantCapped = 25000.0

	seed := func(cs *ChainState) []string {
		addrs := seedHumanAccounts(cs, humans, 1000)
		cs.mu.Lock()
		cs.accounts.Set("0xwhale1", &AccountState{Address: "0xwhale1", Balance: NewDecimal(overCap)})
		cs.accounts.Set("0xrecipient1", &AccountState{Address: "0xrecipient1"})
		cs.accounts.Set("0xwhale2", &AccountState{Address: "0xwhale2", Balance: NewDecimal(overCap)})
		cs.accounts.Set("0xrecipient2", &AccountState{Address: "0xrecipient2"})
		cs.mu.Unlock()
		return addrs
	}

	// ---- parallel: the whole run in ONE block, so the batch path engages ----
	parDAG, parCS := newDeterminismTestDAG()
	addrs := seed(parCS)
	txs := multiCapCrossingBatch(addrs, overCap)
	if batch, _ := collectDisjointTransferBatch(txs, 0); len(batch) < parallelReplayMinBatch {
		t.Fatalf("test set does not reach the parallel batch threshold: %d < %d",
			len(batch), parallelReplayMinBatch)
	}
	if ok := parDAG.replayTransactions(&Block{Height: 1, Hash: "0xparmulticap", Transactions: txs}, true); !ok {
		t.Fatal("parallel replay rejected a well-formed block")
	}

	// ---- serial: identical transactions, one per block, batch never forms ----
	serDAG, serCS := newDeterminismTestDAG()
	seed(serCS)
	for i, tx := range txs {
		b := &Block{Height: int64(i + 1), Hash: fmt.Sprintf("0xsermulticap-%d", i), Transactions: []Transaction{tx}}
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

	for _, recipient := range []string{"0xrecipient1", "0xrecipient2"} {
		serRecipient := read(serCS, recipient)
		parRecipient := read(parCS, recipient)

		// Sanity: the serial path really did cap this recipient. If this
		// ever fails the test's premise is wrong, not the parallel path.
		if serRecipient != wantCapped {
			t.Fatalf("premise broken: serial path did not cap %s (got %.6f, want %.6f)",
				recipient, serRecipient, wantCapped)
		}
		if parRecipient != serRecipient {
			t.Errorf("wealth cap not enforced by the parallel batch path for %s (one of TWO simultaneous crossings in the same batch): parallel %.6f, serial %.6f",
				recipient, parRecipient, serRecipient)
		}
	}
	for _, pool := range []string{validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr} {
		if got, want := read(parCS, pool), read(serCS, pool); got != want {
			t.Errorf("pool %s: parallel %.6f, serial %.6f — cap overflow from two simultaneous crossings was not redistributed identically", pool, got, want)
		}
	}
	if parRoot, serRoot := parCS.StateRoot(), serCS.StateRoot(); parRoot != serRoot {
		t.Errorf("StateRoot divergence — this forks the network\n  parallel: %s\n  serial:   %s", parRoot, serRoot)
	}
}
