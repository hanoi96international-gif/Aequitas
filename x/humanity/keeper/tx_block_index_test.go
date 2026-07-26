package keeper

import "testing"

// Regression guards for the wallet report of 2026-07-26: a 150 AEQ transfer
// landed in block #1918326 and MetaMask showed "Senden fehlgeschlagen".
//
// The cause was that nothing recorded which block a transaction went into, so
// eth_getTransactionByHash answered blockNumber 0x1 and eth_getTransactionReceipt
// answered with the CURRENT CHAIN HEAD — an answer that changed on every call
// (measured 0x1d5102, then 0x1d511a seconds later). Confirmations are computed
// as head - receipt.blockNumber, so that is permanently zero and no wallet can
// ever conclude the transaction succeeded.

// A transaction with no EVM hash (internal types like ubi_distribution) must
// be skipped rather than indexed under an empty key — a single empty-string
// primary key would otherwise collide across every such transaction.
func TestIndexBlockTransactions_SkipsHashlessTransactions(t *testing.T) {
	cs := newTestState()
	if cs.db != nil {
		t.Skip("covers the no-DB path")
	}
	err := cs.IndexBlockTransactions(42, "abc", []Transaction{
		{Type: "ubi_distribution", Wallet: "0xa"},
		{Type: "transfer", Wallet: "0xa", To: "0xb", Amount: 1}, // no TxHash
	})
	if err != nil {
		t.Fatalf("indexing without a DB must be a silent no-op, got %v", err)
	}
}

// An empty block must cost nothing: blocks in normal operation carry no
// transactions at all, so this runs on every single accepted block.
func TestIndexBlockTransactions_EmptyBlockIsNoOp(t *testing.T) {
	cs := newTestState()
	if err := cs.IndexBlockTransactions(1, "hash", nil); err != nil {
		t.Fatalf("empty block must be a no-op, got %v", err)
	}
	if err := cs.IndexBlockTransactions(1, "hash", []Transaction{}); err != nil {
		t.Fatalf("empty slice must be a no-op, got %v", err)
	}
}

// A lookup that finds nothing must report found=false rather than a zero
// block — reporting block 0 would resurrect exactly the bug this fixes, since
// a wallet would treat it as "mined, 1.9M confirmations".
func TestLookupTxBlock_MissingReportsNotFound(t *testing.T) {
	cs := newTestState()
	h, bh, idx, found := cs.LookupTxBlock("0xdoesnotexist")
	if found {
		t.Fatalf("unknown hash must not be reported as found (got height %d, block %q, index %d)", h, bh, idx)
	}
	if h != 0 || bh != "" {
		t.Fatalf("a miss must return zero values, got height %d block %q", h, bh)
	}
	// An empty hash must not be looked up at all.
	if _, _, _, found := cs.LookupTxBlock(""); found {
		t.Fatal("empty tx hash must never resolve to a block")
	}
}

// The body-by-reference digest must match calculateBlockHash's own tx_root
// computation exactly, including the nil-to-empty normalisation. If these
// ever drift, blocks would hash differently at producer and receiver — the
// precise failure the normalisation comment in calculateBlockHash describes.
func TestTxBatchRoot_MatchesBlockHashCommitment(t *testing.T) {
	txs := []Transaction{
		{Type: "transfer", Wallet: "0xa", To: "0xb", Amount: 1.5, TxHash: "0x1"},
		{Type: "transfer", Wallet: "0xc", To: "0xd", Amount: 2.5, TxHash: "0x2"},
	}
	withBody := &Block{Height: 7, Timestamp: 1, ParentHashes: []string{"p"}, Proposer: "0xp", Transactions: txs}
	byReference := &Block{Height: 7, Timestamp: 1, ParentHashes: []string{"p"}, Proposer: "0xp", TxRoot: txBatchRoot(txs)}

	if a, b := calculateBlockHash(withBody), calculateBlockHash(byReference); a != b {
		t.Fatalf("a block carrying its body and the same block carrying only tx_root must hash identically —\n  with body:     %s\n  by reference:  %s\nAnything else is a consensus break.", a, b)
	}

	// nil and empty must produce the same root, or a body stripped by
	// `omitempty` in transport would be seen as a different transaction set.
	if txBatchRoot(nil) != txBatchRoot([]Transaction{}) {
		t.Fatal("nil and empty transaction lists must produce the same root")
	}
}

// A body served by a peer must be rejected unless it hashes to exactly the
// root the signed block committed to. This is the security boundary of the
// whole by-reference scheme.
func TestAttachTxBatch_RejectsSubstitutedBody(t *testing.T) {
	cs := newTestState()
	real := []Transaction{{Type: "transfer", Wallet: "0xa", To: "0xb", Amount: 1, TxHash: "0x1"}}
	block := &Block{Height: 3, TxRoot: txBatchRoot(real)}

	tampered := []Transaction{{Type: "transfer", Wallet: "0xa", To: "0xattacker", Amount: 999, TxHash: "0x1"}}
	if err := cs.AttachTxBatch(block, tampered); err == nil {
		t.Fatal("a body that does not match the committed tx_root must be rejected")
	}
	if len(block.Transactions) != 0 {
		t.Fatal("a rejected body must not be attached to the block")
	}

	if err := cs.AttachTxBatch(block, real); err != nil {
		t.Fatalf("the genuine body must be accepted, got %v", err)
	}
	if len(block.Transactions) != 1 {
		t.Fatal("the genuine body must be attached")
	}
}

// A block whose committed root is the empty-list root is not missing its body
// — it genuinely has none. Treating that as "body missing" would defer every
// ordinary empty block, which is nearly all of them.
func TestNeedsTxBatch_EmptyBlockIsNotMissing(t *testing.T) {
	cs := newTestState()
	if cs.NeedsTxBatch(&Block{Height: 1, TxRoot: txBatchRoot(nil)}) {
		t.Fatal("a block committing to an empty transaction list must never be treated as missing its body")
	}
	if cs.NeedsTxBatch(&Block{Height: 1}) {
		t.Fatal("a block with no tx_root at all (pre-upgrade format) must not be treated as missing its body")
	}
	withRoot := &Block{Height: 1, TxRoot: txBatchRoot([]Transaction{{Type: "transfer", TxHash: "0x1"}})}
	if !cs.NeedsTxBatch(withRoot) {
		t.Fatal("a block committing to a non-empty body it does not carry must report the body as missing")
	}
}
