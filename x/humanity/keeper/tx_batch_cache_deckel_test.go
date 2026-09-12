package keeper

import (
	"fmt"
	"testing"
)

// Heap-Profil C2, 12.09.2026: 512 gecachte Ruempfe zu je 7.000 Transaktionen
// = 0,9 GB, nachdem die DAG-Ausduennung die Ruempfe aus den Bloecken laengst
// entfernt hatte. Der Cache muss nach Transaktionen deckeln, nicht nach
// Eintraegen.
func TestTxBatchCache_DeckeltNachTransaktionen(t *testing.T) {
	c := newTxBatchCache()
	gross := make([]Transaction, 7000)
	for i := 0; i < 40; i++ {
		c.put(fmt.Sprintf("root-%d", i), gross)
	}
	e, n := c.stand()
	if n > txBatchCacheMaxTxs {
		t.Fatalf("Cache haelt %d Transaktionen, Deckel ist %d", n, txBatchCacheMaxTxs)
	}
	if e != txBatchCacheMaxTxs/7000 {
		t.Fatalf("erwartet %d Eintraege zu 7.000, bekommen %d", txBatchCacheMaxTxs/7000, e)
	}
	if _, ok := c.get("root-39"); !ok {
		t.Fatalf("der juengste Rumpf muss im Cache bleiben")
	}
	if _, ok := c.get("root-0"); ok {
		t.Fatalf("der aelteste Rumpf muss verdraengt sein")
	}
}

// Ein einzelner Rumpf ueber dem Deckel bleibt trotzdem drin -- sonst waere
// der Cache fuer genau den Block nutzlos, dessen Rumpf der Partner gleich holt.
func TestTxBatchCache_EinzelnerGrosserRumpfBleibt(t *testing.T) {
	c := newTxBatchCache()
	c.put("riese", make([]Transaction, txBatchCacheMaxTxs+1))
	if _, ok := c.get("riese"); !ok {
		t.Fatalf("der juengste Eintrag darf nie verdraengt werden")
	}
	c.put("klein", make([]Transaction, 10))
	if _, ok := c.get("riese"); ok {
		t.Fatalf("sobald ein juengerer da ist, muss der Riese weichen")
	}
	if e, n := c.stand(); e != 1 || n != 10 {
		t.Fatalf("Stand nach Verdraengung: %d Eintraege / %d txs", e, n)
	}
}

// Kleine Ruempfe: der Eintragsdeckel greift weiterhin.
func TestTxBatchCache_EintragsdeckelBleibt(t *testing.T) {
	c := newTxBatchCache()
	for i := 0; i < txBatchCacheMax+50; i++ {
		c.put(fmt.Sprintf("r%d", i), make([]Transaction, 1))
	}
	if e, _ := c.stand(); e != txBatchCacheMax {
		t.Fatalf("Eintraege %d, Deckel %d", e, txBatchCacheMax)
	}
}
