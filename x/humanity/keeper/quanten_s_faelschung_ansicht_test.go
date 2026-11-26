package keeper

import "testing"

// Die Ansicht des Nachweises (P5c, App): Fehlversuche (die Stelle
// unterschreibt sie beim Anmelden mit) und der Fortschritt einer offenen
// Anmeldung (ein unterbrochener Nachweis setzt dort fort).
func TestQuantenSFaelschung_AnsichtFehlUndOffen(t *testing.T) {
	w, _ := neueFwWelt(t, true)
	zahl := func(k string) (int, bool) {
		v, ok := w.ansicht()[k]
		if !ok {
			return 0, false
		}
		i, ok := v.(int)
		if !ok {
			t.Fatalf("%s: %T", k, v)
		}
		return i, true
	}
	if f, ok := zahl("nachweis_fehl"); !ok || f != 0 {
		t.Fatalf("vor dem Anmelden: nachweis_fehl %d %v", f, ok)
	}
	if _, ok := zahl("nachweis_offen"); ok {
		t.Fatal("nachweis_offen ohne Anmeldung")
	}
	z := w.fwEnde() + 100
	// Ehrliche Vorlage: besteht B's Stichprobe nicht -- ein Fehlversuch.
	w.annehmen(w.fwAnmeldenTx(w.stelle[1], 1, w.x, w.anna), z)
	if o, ok := zahl("nachweis_offen"); !ok || o != 0 {
		t.Fatalf("nach dem Anmelden: nachweis_offen %d %v", o, ok)
	}
	p := w.probe[1]
	if len(p) <= quantenWOffenTeil {
		t.Fatalf("Testaufbau: Stichprobe %d, mehr als ein Teil noetig", len(p))
	}
	w.annehmen(w.fwWerteTx(0, p[:quantenWOffenTeil]), z+5)
	if o, _ := zahl("nachweis_offen"); o != quantenWOffenTeil {
		t.Fatalf("nach einem Teil: nachweis_offen %d", o)
	}
	for ab := quantenWOffenTeil; ab < len(p); ab += quantenWOffenTeil {
		w.annehmen(w.fwWerteTx(ab, p[ab:min(ab+quantenWOffenTeil, len(p))]), z+6)
	}
	if f, _ := zahl("nachweis_fehl"); f != 1 {
		t.Fatalf("nach dem Fehlversuch: nachweis_fehl %d", f)
	}
	if _, ok := zahl("nachweis_offen"); ok {
		t.Fatal("nachweis_offen nach dem Urteil")
	}
	// Mit dem alten Zaehler (0) unterschrieben: abgewiesen -- darum braucht
	// die App den Wert aus der Ansicht.
	alt := w.fwAnmeldenTx(w.stelle[1], 1, w.p.bits, w.anna)
	q := *alt.QuantenW
	q.WalletSig = personalSign(w.t, w.keys[w.stelle[1]], quantenSFaelschungAnmeldenNachricht(w.stelle[1], w.p.K, q.B, q.X, q.Probe, 0))
	alt.QuantenW = &q
	if r := w.praepRegel(alt, z+10); r != "quanten_w_unterschrift" {
		t.Fatalf("Anmelden mit veraltetem Zaehler: %q", r)
	}
}
