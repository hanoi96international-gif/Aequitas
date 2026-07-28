package keeper

import "testing"

// Lokale Fixtures: die gleichnamigen Helfer lagen auf einem Branch, der wegen
// eines Fehlalarms zurueckgerollt wurde.
func pruneTestState() *ChainState {
	return &ChainState{txBatches: newTxBatchCache()}
}

func pruneTestTxs(n int) []Transaction {
	txs := make([]Transaction, n)
	for i := range txs {
		txs[i] = Transaction{Type: "transfer", Wallet: "0xaaa", To: "0xbbb", Amount: float64(i + 1)}
	}
	return txs
}

// A pruned body must never break anything: stripBlocksForPeer checks
// LoadTxBatch and sends the block WHOLE when the body is gone, which is exactly
// the behaviour that existed before bodies-by-reference. This pins that
// contract, because it is the entire safety argument for pruning at all.
func TestPrunedBatchMeansTheBlockTravelsWhole(t *testing.T) {
	txs := pruneTestTxs(40)
	root := txBatchRoot(txs)
	cs := pruneTestState()
	cs.txBatches.put(root, txs)

	block := &Block{Hash: "0xb", Height: 7, TxRoot: root, Transactions: txs}
	a := &APIServer{state: cs}

	// With the body present it is stripped.
	if out := a.stripBlocksForPeer([]*Block{block}); len(out[0].Transactions) != 0 {
		t.Fatal("block with an available body was not stripped")
	}
	// Simulate the pruner having removed it.
	cs.txBatches = newTxBatchCache()
	out := a.stripBlocksForPeer([]*Block{block})
	if len(out[0].Transactions) != 40 {
		t.Fatal("after the body was pruned the block must travel whole; otherwise pruning would strand peers")
	}
}

// A malformed override must not silently disable the bound — an unbounded table
// is what this exists to prevent.
func TestTxBatchKeep_RejectsUnusableOverride(t *testing.T) {
	t.Setenv("AEQUITAS_TX_BATCH_MAX_BYTES", "nonsense")
	if got := txBatchKeep(); got != txBatchMaxBytes {
		t.Fatalf("keep=%d for an unparseable override; the default %d must stand", got, txBatchMaxBytes)
	}
	t.Setenv("AEQUITAS_TX_BATCH_MAX_BYTES", "-5")
	if got := txBatchKeep(); got != txBatchMaxBytes {
		t.Fatalf("keep=%d for a negative override; the default %d must stand", got, txBatchMaxBytes)
	}
	t.Setenv("AEQUITAS_TX_BATCH_MAX_BYTES", "250")
	if got := txBatchKeep(); got != int64(250) {
		t.Fatalf("keep=%d, want the operator's 250", got)
	}
}

// Pruning with no database must be a no-op rather than a panic: the same code
// runs in tests and in nodes configured without Postgres.
func TestPruneTxBatches_NoDatabaseIsHarmless(t *testing.T) {
	cs := pruneTestState()
	cs.pruneTxBatches()
	cs.StartTxBatchPruner()
}
