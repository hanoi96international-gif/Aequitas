package keeper

import (
	"fmt"
	"testing"
)

// Audit 2026-08-16 (transfer/WAL/concurrency pass), transfer-path equivalence.
//
// applyTransferBatchParallel (replay_parallel.go) must be a pure speed-up: for
// any block it accepts, the resulting state must be byte-identical to what the
// serial path (applyTransferDeltaLocked, state.go:7067) would have produced.
// The 2026-08-15 fix added the wealth-cap decline that makes that true for a
// clear cap crossing, and replay_parallel_wealthcap_test.go /
// replay_parallel_wealthcap_multi_test.go pin one and two crossings.
//
// Neither covers the two edges where the two implementations express the cap
// rule in DIFFERENT code and could therefore disagree by one micro-AEQ:
//
//	parallel  it.to.Balance.Add(NewDecimal(it.amount)).Float() > capAmt  -> decline
//	serial    acc.Balance.Float() <= wealthCapAmt                        -> no cap
//
// and the tokenomics-pool exemption, which the parallel path re-implements as
// !isTokenomicsPoolAddress(it.toKey) (replay_parallel.go:205) against the serial
// path's early return inside enforceWealthCapLockedCtx (state.go:3700).
//
// These are regression guards, not attack tests: PASS is the expected outcome
// for the code as it stands, and each pins a boundary that a future edit to
// either expression could silently move in only one of the two places.

// replayBatchAgainstSerial replays txs two ways — all in one block, so the
// parallel batch path engages, and one per block, so it never can — and returns
// the two ChainStates for comparison. seed runs against each fresh state.
func replayBatchAgainstSerial(t *testing.T, tag string, seed func(*ChainState), txs []Transaction) (parallel, serial *ChainState) {
	t.Helper()

	parDAG, parCS := newDeterminismTestDAG()
	seed(parCS)
	if batch, _ := collectDisjointTransferBatch(txs, 0); len(batch) < parallelReplayMinBatch {
		t.Fatalf("%s: test set does not reach the parallel batch threshold: %d < %d",
			tag, len(batch), parallelReplayMinBatch)
	}
	if ok := parDAG.replayTransactions(&Block{Height: 1, Hash: "0xpar-" + tag, Transactions: txs}, true); !ok {
		t.Fatalf("%s: parallel replay rejected a well-formed block", tag)
	}

	serDAG, serCS := newDeterminismTestDAG()
	seed(serCS)
	for i, tx := range txs {
		b := &Block{Height: int64(i + 1), Hash: fmt.Sprintf("0xser-%s-%d", tag, i), Transactions: []Transaction{tx}}
		if ok := serDAG.replayTransactions(b, true); !ok {
			t.Fatalf("%s: serial replay rejected tx %d", tag, i)
		}
	}
	return parCS, serCS
}

func balanceOf(cs *ChainState, addr string) float64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	acc, ok := cs.accounts.Get(addr)
	if !ok {
		return 0
	}
	return acc.Balance.Float()
}

// assertParallelMatchesSerial compares everything a block can move: the named
// account, all four tokenomics pools, and the consensus root itself.
func assertParallelMatchesSerial(t *testing.T, tag string, par, ser *ChainState, watch string) {
	t.Helper()
	if got, want := balanceOf(par, watch), balanceOf(ser, watch); got != want {
		t.Errorf("%s: recipient %s — parallel %.6f, serial %.6f", tag, watch, got, want)
	}
	for _, pool := range []string{validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr} {
		if got, want := balanceOf(par, pool), balanceOf(ser, pool); got != want {
			t.Errorf("%s: pool %s — parallel %.6f, serial %.6f", tag, pool, got, want)
		}
	}
	if parRoot, serRoot := par.StateRoot(), ser.StateRoot(); parRoot != serRoot {
		t.Errorf("%s: StateRoot divergence — this forks the network\n  parallel: %s\n  serial:   %s",
			tag, parRoot, serRoot)
	}
}

// batchWithLeadTransfer builds a pairwise-disjoint, demurrage-free run long
// enough to reach parallelReplayMinBatch, led by one transfer under test.
func batchWithLeadTransfer(addrs []string, from, to string, amount float64) []Transaction {
	txs := []Transaction{{Type: "transfer", Wallet: from, To: to, Amount: amount}}
	for i := 0; len(txs) < parallelReplayMinBatch+2; i++ {
		txs = append(txs, Transaction{
			Type: "transfer", Wallet: addrs[2*i], To: addrs[2*i+1], Amount: 1,
		})
	}
	return txs
}

// With 40 humans, humanCountLocked() >= 25 so bootstrapMultiplierLocked returns
// the full 25x, and getAverageBalanceLocked is a flat 1000 by protocol
// invariant — the cap is exactly 25,000 AEQ.
const capBoundaryHumans = 40
const capBoundaryAmount = 25000.0

func seedCapBoundaryState(sender, recipient string, senderBalance float64) func(*ChainState) []string {
	return func(cs *ChainState) []string {
		addrs := seedHumanAccounts(cs, capBoundaryHumans, 1000)
		cs.mu.Lock()
		cs.accounts.Set(sender, &AccountState{Address: sender, Balance: NewDecimal(senderBalance)})
		if _, ok := cs.accounts.Get(recipient); !ok {
			cs.accounts.Set(recipient, &AccountState{Address: recipient})
		}
		cs.mu.Unlock()
		return addrs
	}
}

// TestParallelReplay_CapBoundary_ExactlyAtCapIsNotTrimmed pins the boundary the
// two implementations express differently: landing EXACTLY on the cap must be
// left alone by both. The parallel path declines on `> capAmt`; the serial path
// caps on `> wealthCapAmt` (i.e. returns early on `<=`). If either ever became
// `>=`, only one of them would move and the block would fork.
func TestParallelReplay_CapBoundary_ExactlyAtCapIsNotTrimmed(t *testing.T) {
	const sender, recipient = "0xwhaleexact", "0xrecipientexact"
	var addrs []string
	seedFn := seedCapBoundaryState(sender, recipient, capBoundaryAmount*2)
	seed := func(cs *ChainState) { addrs = seedFn(cs) }

	// Seed once up front so the transaction list can reference the filler
	// addresses; the per-state seeding inside replayBatchAgainstSerial rebuilds
	// them identically on each fresh state.
	probe := newTestState()
	addrs = seedFn(probe)
	txs := batchWithLeadTransfer(addrs, sender, recipient, capBoundaryAmount)

	par, ser := replayBatchAgainstSerial(t, "exactcap", seed, txs)

	// Premise: the serial path really did leave it untrimmed at exactly the cap.
	if got := balanceOf(ser, recipient); got != capBoundaryAmount {
		t.Fatalf("premise broken: serial path left %s at %.6f, want exactly the cap %.6f",
			recipient, got, capBoundaryAmount)
	}
	assertParallelMatchesSerial(t, "exactcap", par, ser, recipient)
}

// TestParallelReplay_CapBoundary_OneMicroOverIsTrimmedIdentically is the other
// side of the same boundary: one micro-AEQ past the cap must be trimmed, and
// the excess must reach the pools. The parallel path cannot trim, so it must
// decline the whole batch and let the serial path reproduce the trim, the four
// pool credits and the resulting root exactly.
func TestParallelReplay_CapBoundary_OneMicroOverIsTrimmedIdentically(t *testing.T) {
	const sender, recipient = "0xwhalemicro", "0xrecipientmicro"
	const overCap = capBoundaryAmount + 0.000001

	var addrs []string
	seedFn := seedCapBoundaryState(sender, recipient, capBoundaryAmount*2)
	seed := func(cs *ChainState) { addrs = seedFn(cs) }

	probe := newTestState()
	addrs = seedFn(probe)
	txs := batchWithLeadTransfer(addrs, sender, recipient, overCap)

	par, ser := replayBatchAgainstSerial(t, "microover", seed, txs)

	if got := balanceOf(ser, recipient); got != capBoundaryAmount {
		t.Fatalf("premise broken: serial path left %s at %.6f, want it trimmed to the cap %.6f",
			recipient, got, capBoundaryAmount)
	}
	assertParallelMatchesSerial(t, "microover", par, ser, recipient)
}

// TestParallelReplay_PoolRecipientIsExemptIdentically pins the tokenomics-pool
// exemption, which the two paths implement in different places: an ordinary
// transfer INTO a pool address (reachable — applyTransferDeltaLocked's own
// comment, state.go:7078-7081, names exactly this case) must be exempt from the
// cap on both. If the parallel path's !isTokenomicsPoolAddress guard and the
// serial path's early return in enforceWealthCapLockedCtx ever disagree, one
// side trims a pool balance the other leaves whole.
func TestParallelReplay_PoolRecipientIsExemptIdentically(t *testing.T) {
	const sender = "0xwhalepool"
	recipient := ubiPoolAddr
	// Far past the cap, so an accidentally-applied cap would be unmistakable.
	const amount = capBoundaryAmount * 3

	var addrs []string
	seedFn := seedCapBoundaryState(sender, recipient, amount*2)
	seed := func(cs *ChainState) { addrs = seedFn(cs) }

	probe := newTestState()
	addrs = seedFn(probe)
	txs := batchWithLeadTransfer(addrs, sender, recipient, amount)

	par, ser := replayBatchAgainstSerial(t, "poolrecipient", seed, txs)

	// Premise: the pool really was left uncapped by the serial path.
	if got := balanceOf(ser, recipient); got != amount {
		t.Fatalf("premise broken: serial path left pool %s at %.6f, want the full %.6f "+
			"(pools are exempt from the wealth cap)", recipient, got, amount)
	}
	assertParallelMatchesSerial(t, "poolrecipient", par, ser, recipient)
}
