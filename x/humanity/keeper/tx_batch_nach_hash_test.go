package keeper

import (
	"fmt"
	"testing"
)

// 15.09.2026: C2 schickte C1 die Ruempfe von C1s EIGENEN Bloecken zurueck
// (1,92 GB in 9 Minuten), weil ein per Push mit Inline-Rumpf angekommener
// Block keinen Batch hat. Der Index tx_root -> hash macht ihn strippbar,
// ohne den Rumpf ein zweites Mal zu schreiben.
func TestTxRootIndex_MerktUndVerdraengtImRing(t *testing.T) {
	var ix txRootIndex
	ix.merke("", "0xabc") // ohne Root: nichts
	ix.merke("r0", "")    // ohne Hash: nichts
	if _, ok := ix.hashFuer("r0"); ok {
		t.Fatal("ohne Hash darf nichts gemerkt werden")
	}
	for i := 0; i < txRootIndexMax+5; i++ {
		ix.merke(fmt.Sprintf("r%d", i), fmt.Sprintf("h%d", i))
	}
	if _, ok := ix.hashFuer("r0"); ok {
		t.Fatal("der aelteste Eintrag muss dem Ring zum Opfer gefallen sein")
	}
	if h, ok := ix.hashFuer(fmt.Sprintf("r%d", txRootIndexMax+4)); !ok || h != fmt.Sprintf("h%d", txRootIndexMax+4) {
		t.Fatal("der juengste Eintrag muss auffindbar sein")
	}
	if len(ix.hash) != txRootIndexMax {
		t.Fatalf("der Index darf nicht ueber %d Eintraege wachsen: %d", txRootIndexMax, len(ix.hash))
	}
	// Doppelt merken aendert nichts.
	ix.merke("r5", "anders")
	if h, _ := ix.hashFuer("r5"); h != "h5" {
		t.Fatal("ein bekannter Root behaelt seinen Hash")
	}
}

func TestMerkeTxRoot_NurBloeckeMitRumpfUndRoot(t *testing.T) {
	cs := &ChainState{}
	cs.merkeTxRoot(&Block{Hash: "h", TxRoot: "r"}) // leer: nichts
	cs.merkeTxRoot(&Block{Hash: "h", Transactions: []Transaction{{}}})
	if _, ok := cs.txRootIdx.hashFuer("r"); ok {
		t.Fatal("ohne Rumpf oder ohne Root wird nichts gemerkt")
	}
	cs.merkeTxRoot(&Block{Hash: "h", TxRoot: "r", Transactions: []Transaction{{}}})
	if h, ok := cs.txRootIdx.hashFuer("r"); !ok || h != "h" {
		t.Fatal("Block mit Rumpf und Root muss gemerkt werden")
	}
	if cs.hatRumpfInChainBlocks("r") {
		t.Fatal("ohne Datenbank kann kein Rumpf zugesichert werden")
	}
	var nilCS *ChainState
	nilCS.merkeTxRoot(&Block{Hash: "h", TxRoot: "r", Transactions: []Transaction{{}}}) // darf nicht panicen
}
