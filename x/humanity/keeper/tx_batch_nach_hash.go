package keeper

import "sync"

// Rumpf nach Hash: tx_root -> Blockhash, damit jeder Block gestrippt werden
// kann, dessen Rumpf in chain_blocks liegt.
//
// GEMESSEN 15.09.2026, Lauf 7, C2 an C1 in 9 Minuten: 1,92 GB ueber
// /api/blocks -- 4.014 Bloecke gestrippt, 7.492 mit Rumpf. Die gestrippten
// waren C2s eigene Bloecke (SaveTxBatch bei der Produktion); die mit Rumpf
// waren C1s Bloecke, die C2 per Push mit Inline-Rumpf bekommen hatte. Fuer
// die legte niemand einen Batch an, HasTxBatch sagte nein, und C2 schickte
// C1 die Ruempfe seiner EIGENEN Bloecke zurueck: 5 MB je Seite, alle zwei
// Sekunden, mitten im Lastlauf.
//
// Jeden Inline-Rumpf zusaetzlich in chain_tx_batches zu schreiben waere die
// naheliegende Loesung -- und Schreibverstaerkung auf genau dem Knoten,
// dessen Platte der Engpass ist (chain_blocks.transactions_z hat den Rumpf
// laengst). Stattdessen merkt sich der Speicher-DAG bei jedem angenommenen
// oder produzierten Block tx_root -> hash; HasTxBatch und LoadTxBatch
// fallen darauf zurueck und lesen den Rumpf bei Bedarf aus chain_blocks.
// Kein zusaetzlicher Schreibzugriff, ein Ring von txRootIndexMax Eintraegen
// (~1 MB), der die letzten gut 60 Minuten bei zwei Bloecken je Sekunde
// abdeckt -- weit mehr als der Ueberlapp von 20 Hoehen je Sync-Zyklus.

const txRootIndexMax = 8192

type txRootIndex struct {
	mu   sync.Mutex
	hash map[string]string
	ring []string
	pos  int
}

func (ix *txRootIndex) merke(root, hash string) {
	if root == "" || hash == "" {
		return
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.hash == nil {
		ix.hash = make(map[string]string, txRootIndexMax)
		ix.ring = make([]string, txRootIndexMax)
	}
	if _, da := ix.hash[root]; da {
		return
	}
	if alt := ix.ring[ix.pos]; alt != "" {
		delete(ix.hash, alt)
	}
	ix.ring[ix.pos] = root
	ix.pos = (ix.pos + 1) % txRootIndexMax
	ix.hash[root] = hash
}

func (ix *txRootIndex) hashFuer(root string) (string, bool) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	h, ok := ix.hash[root]
	return h, ok
}

// merkeTxRoot: beim Einhaengen eines Blocks mit Rumpf in den Speicher-DAG.
func (cs *ChainState) merkeTxRoot(b *Block) {
	if cs == nil || b == nil || b.TxRoot == "" || len(b.Transactions) == 0 {
		return
	}
	cs.txRootIdx.merke(b.TxRoot, b.Hash)
}

// rumpfAusChainBlocks: der Rumpf zu einem tx_root ueber den Blockhash aus
// chain_blocks -- der Weg fuer Bloecke, die ohne Batch angekommen sind.
func (cs *ChainState) rumpfAusChainBlocks(root string) ([]Transaction, bool) {
	if cs.db == nil {
		return nil, false
	}
	hash, ok := cs.txRootIdx.hashFuer(root)
	if !ok {
		return nil, false
	}
	b := cs.LoadBlockFromDBByHash(hash)
	if b == nil || len(b.Transactions) == 0 {
		return nil, false
	}
	// Nie ungeprueft ausliefern: der Digest muss zum Root passen, sonst
	// wuerde ein Peer einen Rumpf bekommen, den AttachTxBatch verwirft.
	if txBatchRoot(b.Transactions) != root {
		return nil, false
	}
	return b.Transactions, true
}

// hatRumpfInChainBlocks: nur die Existenzfrage, ohne den Rumpf zu laden.
func (cs *ChainState) hatRumpfInChainBlocks(root string) bool {
	if cs.db == nil {
		return false
	}
	hash, ok := cs.txRootIdx.hashFuer(root)
	return ok && cs.BlockExistsInDB(hash)
}
