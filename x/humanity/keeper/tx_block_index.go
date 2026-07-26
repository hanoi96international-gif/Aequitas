package keeper

import (
	"fmt"
	"strings"
)

// tx → including-block index.
//
// THE BUG THIS FIXES (reported from a live wallet, 2026-07-26): a transfer of
// 150 AEQ landed correctly in block #1918326, and MetaMask showed it as
// "Senden fehlgeschlagen". The money had moved; the wallet said it failed.
//
// The cause is that nothing recorded WHICH block a transaction went into, so
// both JSON-RPC methods a wallet uses to follow a transaction answered with
// placeholders:
//
//	eth_getTransactionByHash   blockNumber 0x1, blockHash 0x00…01
//	                           (hardcoded, the real block was 0x1D4576)
//	eth_getTransactionReceipt  blockNumber = dag.LatestBlock()
//
// The receipt one is the worse of the two: it reports the CURRENT CHAIN HEAD
// as the transaction's block, so the answer changes on every call — measured
// 0x1d5102, then 0x1d511a seconds later. A wallet counts confirmations as
// (head - receipt.blockNumber), which under that behaviour is permanently
// zero, while getTransactionByHash simultaneously claims block 1. No wallet
// can reconcile that, so MetaMask eventually gives up on the transaction and
// marks it failed — even though the receipt's own status field says 0x1.
//
// evm_tx_receipts cannot carry this: it is written at RPC time and therefore
// only exists on the node that ACCEPTED the transaction. A wallet talks to
// whichever node serves aequitas.digital, which is frequently not that node.
// This index is written by EVERY node when it accepts a block, so any node
// can answer correctly regardless of where the transaction originated.

// ensureTxBlockIndexTable creates the index. Keyed by tx_hash because that is
// the only thing a wallet has to ask with.
func (cs *ChainState) ensureTxBlockIndexTable() {
	if cs.db == nil {
		return
	}
	cs.txBlockIndexOnce.Do(func() {
		cs.db.Exec(`CREATE TABLE IF NOT EXISTS chain_tx_block_index (
			tx_hash      TEXT PRIMARY KEY,
			block_height BIGINT NOT NULL,
			block_hash   TEXT NOT NULL,
			tx_index     INT NOT NULL DEFAULT 0
		)`)
	})
}

// IndexBlockTransactions records which block each of a block's transactions
// went into. Called once per accepted block, by producer and replayer alike.
//
// A no-op for the overwhelmingly common empty block, so steady-state
// operation pays nothing. ON CONFLICT DO NOTHING keeps the FIRST block that
// contained a transaction authoritative: the same transaction can legitimately
// appear in more than one block of a DAG, and a wallet must be given a stable
// answer rather than one that flips as later blocks merge.
func (cs *ChainState) IndexBlockTransactions(height int64, blockHash string, txs []Transaction) error {
	if cs.db == nil || len(txs) == 0 || blockHash == "" {
		return nil
	}
	cs.ensureTxBlockIndexTable()

	var sb strings.Builder
	args := make([]interface{}, 0, len(txs)*4)
	n := 0
	for i, tx := range txs {
		h := strings.ToLower(strings.TrimSpace(tx.TxHash))
		if h == "" {
			continue // not every transaction type carries an EVM hash
		}
		if n > 0 {
			sb.WriteByte(',')
		}
		p := n * 4
		fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d)", p+1, p+2, p+3, p+4)
		args = append(args, h, height, blockHash, i)
		n++
	}
	if n == 0 {
		return nil
	}
	// cs.db directly, never dbExec(): a standalone write that must not join
	// another goroutine's transaction — see SaveBlockToDB's own comment for
	// the wire-protocol corruption that caused in production.
	_, err := cs.db.Exec(
		`INSERT INTO chain_tx_block_index (tx_hash, block_height, block_hash, tx_index)
		 VALUES `+sb.String()+` ON CONFLICT (tx_hash) DO NOTHING`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("index %d transaction(s) of block #%d: %w", n, height, err)
	}
	return nil
}

// LookupTxBlock returns the block a transaction was included in.
func (cs *ChainState) LookupTxBlock(txHash string) (height int64, blockHash string, txIndex int, found bool) {
	if cs.db == nil || txHash == "" {
		return 0, "", 0, false
	}
	cs.ensureTxBlockIndexTable()
	row := cs.db.QueryRow(
		`SELECT block_height, block_hash, tx_index FROM chain_tx_block_index WHERE tx_hash = $1`,
		strings.ToLower(strings.TrimSpace(txHash)),
	)
	if err := row.Scan(&height, &blockHash, &txIndex); err != nil {
		return 0, "", 0, false
	}
	return height, blockHash, txIndex, true
}
