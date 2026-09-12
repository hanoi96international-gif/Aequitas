package keeper

import (
	"fmt"
	"testing"
)

func baueDAGMitRuempfen(n, txJeBlock int) (*BlockDAG, *ChainState) {
	cs := &ChainState{txBatches: newTxBatchCache()}
	dag := &BlockDAG{
		blocks:         map[string]*Block{},
		tips:           map[string]bool{},
		replayedBlocks: map[string]bool{},
		state:          cs,
	}
	for i := 0; i < n; i++ {
		txs := make([]Transaction, txJeBlock)
		for j := range txs {
			txs[j] = Transaction{Type: "transfer", TxHash: fmt.Sprintf("0x%06d%03d", i, j), Amount: 1}
		}
		root := fmt.Sprintf("root%06d", i)
		b := &Block{Height: int64(i), Hash: fmt.Sprintf("h%06d", i), TxRoot: root, Transactions: txs}
		dag.blocks[b.Hash] = b
		cs.txBatches.put(root, txs)
		dag.height = int64(i)
	}
	return dag, cs
}

// Ausgeduennt wird nur, was die Datenbank bestaetigt, und nur unterhalb der
// Schutzzone -- die juengsten 60 Hoehen behalten ihren Rumpf.
func TestAusduennen_NurBestaetigteUndNurAlte(t *testing.T) {
	dag, _ := baueDAGMitRuempfen(200, 5)
	alt := ausduennBestaetigung
	t.Cleanup(func() { ausduennBestaetigung = alt })
	// Die "Datenbank" kennt alle ausser h000010.
	ausduennBestaetigung = func(_ *BlockDAG, kandidaten []string) (map[string]bool, bool) {
		out := map[string]bool{}
		for _, h := range kandidaten {
			if h != "h000010" {
				out[h] = true
			}
		}
		return out, true
	}
	dag.ausduennenEinmal()

	for i := 0; i < 200; i++ {
		b := dag.blocks[fmt.Sprintf("h%06d", i)]
		juenger := int64(i) >= dag.height-ausduennAbHoehen
		switch {
		case i == 10:
			if b.ausgeduennt || len(b.Transactions) == 0 {
				t.Errorf("h000010 ist nicht in der Datenbank und wurde trotzdem ausgeduennt -- Rumpf verloren")
			}
		case juenger:
			if b.ausgeduennt || len(b.Transactions) == 0 {
				t.Errorf("Hoehe %d liegt in der Schutzzone unter der Spitze und wurde ausgeduennt", i)
			}
		default:
			if !b.ausgeduennt || b.Transactions != nil {
				t.Errorf("Hoehe %d ist alt und bestaetigt, wurde aber nicht ausgeduennt", i)
			}
			if b.TxRoot == "" {
				t.Errorf("Hoehe %d hat nach dem Ausduennen keinen TxRoot mehr -- der Hash waere nicht mehr pruefbar", i)
			}
		}
	}
}

// Jeder Getter muss einen vollstaendigen Block liefern -- als KOPIE. Der
// DAG-Block bleibt ausgeduennt, sonst waere der Speichergewinn beim ersten
// Explorer-Aufruf wieder weg.
func TestAusduennen_GetterLadenNachOhneDenDAGZuFuellen(t *testing.T) {
	dag, _ := baueDAGMitRuempfen(200, 5)
	alt := ausduennBestaetigung
	t.Cleanup(func() { ausduennBestaetigung = alt })
	ausduennBestaetigung = func(_ *BlockDAG, k []string) (map[string]bool, bool) {
		out := map[string]bool{}
		for _, h := range k {
			out[h] = true
		}
		return out, true
	}
	dag.ausduennenEinmal()

	b := dag.GetBlockByHash("h000005")
	if b == nil || len(b.Transactions) != 5 {
		t.Fatalf("GetBlockByHash liefert %v Transaktionen, erwartet 5 -- der Rumpf wurde nicht nachgeladen", b)
	}
	if b.Transactions[2].TxHash != "0x000005002" {
		t.Errorf("nachgeladener Rumpf hat falschen Inhalt: %s", b.Transactions[2].TxHash)
	}
	if orig := dag.blocks["h000005"]; !orig.ausgeduennt || orig.Transactions != nil {
		t.Error("das Nachladen hat den DAG-Block selbst gefuellt -- der Speichergewinn ist beim ersten Aufruf weg")
	}
	if b == dag.blocks["h000005"] {
		t.Error("der Getter gab den DAG-Block selbst zurueck statt einer Kopie")
	}
	liste := dag.GetBlocksByHashesForPeer([]string{"h000003", "h000004"})
	for _, x := range liste {
		if len(x.Transactions) != 5 {
			t.Errorf("GetBlocksByHashesForPeer: Block %s ohne Rumpf", x.Hash)
		}
	}
	// Ein Block ohne Ausduennung kommt unveraendert zurueck (kein Umweg).
	frisch := dag.GetBlockByHash("h000199")
	if frisch != dag.blocks["h000199"] {
		t.Error("ein nicht ausgeduennter Block wurde kopiert -- unnoetige Arbeit auf dem heissen Pfad")
	}
	if AusduennStand()["nachlade_fehler"].(int64) != 0 {
		t.Error("Nachladefehler gezaehlt, obwohl alle Ruempfe im Cache lagen")
	}
}

// Der Hash eines ausgeduennten Blocks muss derselbe bleiben: calculateBlockHash
// hasht ueber TxRoot, wenn Transactions leer ist.
func TestAusduennen_HashBleibtGleich(t *testing.T) {
	txs := []Transaction{{Type: "transfer", TxHash: "0xa", Amount: 1}, {Type: "transfer", TxHash: "0xb", Amount: 2}}
	voll := &Block{Height: 7, ParentHashes: []string{"p"}, Proposer: "0xv", Timestamp: 1, Transactions: txs}
	voll.TxRoot = txBatchRoot(txs)
	vollHash := calculateBlockHash(voll)
	duenn := *voll
	duenn.Transactions = nil
	duenn.ausgeduennt = true
	if calculateBlockHash(&duenn) != vollHash {
		t.Error("ein ausgeduennter Block hasht anders als der volle -- jede Hashpruefung gegen den Peer schluege fehl")
	}
}
