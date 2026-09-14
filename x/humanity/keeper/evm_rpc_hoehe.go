package keeper

import (
	"encoding/json"
	"strings"
)

// EINE Sicht je Hoehe fuer EVM-Werkzeuge.
//
// WAS AM 14.09.2026 PASSIERTE. Eine Ueberweisung ueber MetaMask (30 AEQ, Nonce
// 5) kam an, die Quittung sagte status 0x1 -- und MetaMask zeigte "Senden
// fehlgeschlagen". Die Quittung nannte blockNumber 0x6440d9 und blockHash
// 0xeefcb1..., eth_getBlockByNumber(0x6440d9) lieferte aber den Block
// 0x135eea... mit LEERER Transaktionsliste. Beide Bloecke sind echt: an
// dieser Hoehe haben beide Validatoren produziert (GHOSTDAG, Geschwister),
// die Ueberweisung stand im Geschwisterblock, "nach Nummer" antwortet der
// ausgewaehlte. Fuer eine Wallet, die Ethereum erwartet -- eine Hoehe, ein
// Block, eine Liste --, ist das ein Widerspruch: die Quittung zeigt auf einen
// Block, den es "nach Nummer" nicht gibt.
//
// DIE ABBILDUNG. Nach aussen ist "Block H" der ausgewaehlte Block an Hoehe H,
// und seine Transaktionsliste ist die VEREINIGUNG aller Bloecke dieser Hoehe
// (der ausgewaehlte plus seine Geschwister, die der DAG ohnehin mergt).
// Quittungen und Transaktionen nennen als blockHash den ausgewaehlten Block
// ihrer Hoehe. Damit gilt fuer jede Transaktion:
//   eth_getTransactionReceipt(h).blockHash == eth_getBlockByNumber(H).hash
//   und h steht in eth_getBlockByNumber(H).transactions.
// Reine Darstellung fuer /rpc -- nichts davon beruehrt Konsens, Replay oder
// die Kettenansicht unter /api.
//
// GRENZEN. Faellt die Hoehe aus dem Kanonik-Lauf (maxCanonicalWalkHops) und
// aus dem Speicherfenster, antwortet die Datenbank -- fuer "nach Nummer" und
// fuer die Quittung ueber dieselbe Funktion, also weiterhin konsistent.

// kanonischerBlockHash liefert den Hash des nach aussen sichtbaren Blocks an
// dieser Hoehe -- oder den enthaltenden Block, wenn keiner bestimmbar ist.
func (s *EVMRPCServer) kanonischerBlockHash(height int64, enthaltend string) string {
	if s.dag == nil {
		return enthaltend
	}
	if b := s.dag.GetBlockByHeight(height); b != nil && b.Hash != "" {
		return b.Hash
	}
	return enthaltend
}

// bloeckeAnHoehe: alle bekannten Bloecke dieser Hoehe (Speicher + Datenbank),
// ohne synthetische, nach Hash dedupliziert, Ruempfe geladen.
func (s *EVMRPCServer) bloeckeAnHoehe(height int64) []*Block {
	gesehen := map[string]bool{}
	var out []*Block
	if s.dag != nil {
		s.dag.mu.Lock()
		for _, b := range s.dag.blocks {
			if b != nil && b.Height == height && b.Proposer != "synthetic-checkpoint" && !gesehen[b.Hash] {
				gesehen[b.Hash] = true
				out = append(out, b)
			}
		}
		s.dag.mu.Unlock()
		out = s.dag.hydratisiertAlle(out)
	}
	if s.state != nil && s.state.db != nil {
		for _, b := range s.state.LoadBlocksAtHeightFromDB(height) {
			if !gesehen[b.Hash] {
				gesehen[b.Hash] = true
				out = append(out, b)
			}
		}
	}
	return out
}

// transaktionenDerHoehe: die Transaktionen von "Block H" nach aussen -- der
// ausgewaehlte Block zuerst, dann die Geschwister nach Hash, jede Transaktion
// einmal.
func (s *EVMRPCServer) transaktionenDerHoehe(kanonisch *Block) []Transaction {
	alle := s.bloeckeAnHoehe(kanonisch.Height)
	if len(alle) <= 1 {
		return kanonisch.Transactions
	}
	// Geschwister deterministisch ordnen: ausgewaehlter zuerst, Rest nach Hash.
	var geschwister []*Block
	for _, b := range alle {
		if b.Hash != kanonisch.Hash {
			geschwister = append(geschwister, b)
		}
	}
	for i := 1; i < len(geschwister); i++ {
		for j := i; j > 0 && geschwister[j].Hash < geschwister[j-1].Hash; j-- {
			geschwister[j], geschwister[j-1] = geschwister[j-1], geschwister[j]
		}
	}
	gesehen := map[string]bool{}
	out := make([]Transaction, 0, len(kanonisch.Transactions))
	anhaengen := func(txs []Transaction) {
		for _, t := range txs {
			k := strings.ToLower(t.TxHash)
			if k != "" && gesehen[k] {
				continue
			}
			if k != "" {
				gesehen[k] = true
			}
			out = append(out, t)
		}
	}
	anhaengen(kanonisch.Transactions)
	for _, b := range geschwister {
		anhaengen(b.Transactions)
	}
	return out
}

// istKanonischAnSeinerHoehe: ist dieser Block der nach aussen sichtbare
// seiner Hoehe? Nur dann bekommt er die vereinigte Liste; ein Geschwister,
// per Hash abgefragt, zeigt seine eigenen Transaktionen.
func (s *EVMRPCServer) istKanonischAnSeinerHoehe(b *Block) bool {
	if s.dag == nil || b == nil {
		return false
	}
	k := s.dag.GetBlockByHeight(b.Height)
	return k != nil && k.Hash == b.Hash
}

// LoadBlocksAtHeightFromDB: alle Bloecke einer Hoehe aus chain_blocks (ohne
// synthetische), Ruempfe entpackt. Fuer die EVM-Sicht; die Kettenlogik nutzt
// LoadBlockFromDBByHeight (einer, der beste).
func (cs *ChainState) LoadBlocksAtHeightFromDB(height int64) []*Block {
	if cs.db == nil {
		return nil
	}
	cs.ensureGHOSTDAGColumns()
	rows, err := cs.db.Query(`SELECT hash, height, parent_hashes, proposer, timestamp, humans, state_root,
	                 signature, transactions, COALESCE(transactions_z, ''::bytea),
	                 COALESCE(selected_parent,''), COALESCE(blue_score,0), COALESCE(blues,'[]')
	          FROM chain_blocks WHERE height = $1`, height)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Block
	for rows.Next() {
		var b Block
		var parentHashesRaw, txsRaw, bluesRaw string
		var txsZ []byte
		if err := rows.Scan(
			&b.Hash, &b.Height, &parentHashesRaw, &b.Proposer, &b.Timestamp,
			&b.Humans, &b.StateRoot, &b.Signature, &txsRaw, &txsZ,
			&b.SelectedParent, &b.BlueScore, &bluesRaw,
		); err != nil {
			continue
		}
		if b.Proposer == "synthetic-checkpoint" {
			continue
		}
		_ = json.Unmarshal([]byte(parentHashesRaw), &b.ParentHashes)
		txs, decErr := decodeBlockPayload(txsRaw, txsZ)
		if decErr != nil {
			continue
		}
		b.Transactions = txs
		if bluesRaw != "" && bluesRaw != "[]" && bluesRaw != "null" {
			_ = json.Unmarshal([]byte(bluesRaw), &b.Blues)
		}
		bCopy := b
		out = append(out, &bCopy)
	}
	return out
}
