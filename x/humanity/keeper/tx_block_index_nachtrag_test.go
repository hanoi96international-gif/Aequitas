package keeper

import "testing"

// Ein verworfener Block landet auf der Nachtragsliste statt im Nichts.
func TestTxIndex_VerworfenerBlockWirdGemerkt(t *testing.T) {
	txIndexNachtragMu.Lock()
	vorher := txIndexNachtrag
	txIndexNachtrag = nil
	txIndexNachtragMu.Unlock()
	t.Cleanup(func() {
		txIndexNachtragMu.Lock()
		txIndexNachtrag = vorher
		txIndexNachtragMu.Unlock()
	})

	txIndexNachtragen(4711, "0xabc")
	txIndexNachtragen(4712, "0xdef")
	st := TxIndexStats()
	if st["nachtrag_offen"] != 2 {
		t.Fatalf("zwei verworfene Bloecke erwartet, Stand meldet %v", st["nachtrag_offen"])
	}
	txIndexNachtragMu.Lock()
	e := txIndexNachtrag[0]
	txIndexNachtragMu.Unlock()
	if e.height != 4711 || e.blockHash != "0xabc" {
		t.Fatalf("falscher Eintrag: %+v", e)
	}
}

// Die Liste ist gedeckelt -- ein Knoten unter Dauerlast darf nicht mit ihr
// wachsen.
func TestTxIndex_NachtragslisteIstGedeckelt(t *testing.T) {
	txIndexNachtragMu.Lock()
	vorher := txIndexNachtrag
	txIndexNachtrag = make([]txIndexNachtragEintrag, txIndexNachtragMax)
	txIndexNachtragMu.Unlock()
	t.Cleanup(func() {
		txIndexNachtragMu.Lock()
		txIndexNachtrag = vorher
		txIndexNachtragMu.Unlock()
	})
	txIndexNachtragen(1, "0x1")
	txIndexNachtragMu.Lock()
	n := len(txIndexNachtrag)
	txIndexNachtragMu.Unlock()
	if n != txIndexNachtragMax {
		t.Fatalf("Deckel %d ueberschritten: %d", txIndexNachtragMax, n)
	}
}
