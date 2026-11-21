package keeper

import (
	"context"
	"strings"
	"testing"
)

// Eindeutiges Urteil (quanten_w_urteil.go, R10) gegen die echten Regeln, mit
// annehmendem und nachspielendem Knoten im Gleichschritt (qwWelt).

func qwuAktiv(t *testing.T) {
	t.Helper()
	quantenWUrteilAbOverride.Store(1)
	t.Cleanup(func() { quantenWUrteilAbOverride.Store(0) })
}

func (w *qsWelt) einloesen(z int64) {
	w.t.Helper()
	w.annehmen(w.anmeldenTx(w.stelle[0], 0, w.probe[0], w.vorlage(0, w.keys[w.anna])), z)
	if g := w.offenlegenAlle(0, z+10); g.Urteil != "echt" {
		w.t.Fatalf("Einloesen: %+v", g)
	}
}

func (w *qsWelt) totMachen(z int64) {
	w.t.Helper()
	for i := 0; i < quantenWFehlMax; i++ {
		zi := z + 100*int64(i)
		w.annehmen(w.anmeldenTx(w.stelle[1], 1, w.probe[1], w.vorlage(1, w.keys[w.anna])), zi)
		if g := w.offenlegenAlle(1, zi+5); g.Urteil != "abgewiesen" {
			w.t.Fatalf("Doppelvorlage %d: %+v", i, g)
		}
	}
	if w.token(w.cs) != nil {
		w.t.Fatal("nach quantenWFehlMax Fehlversuchen nicht tot")
	}
}

func (w *qsWelt) ansicht() map[string]interface{} {
	return w.cs.quantenWNoteAnsicht(context.Background(), w.p.K)
}

// Schlafend: wie bisher -- eingeloest und tot sehen gleich aus, das Register
// traegt kein Urteil (StateRoot unveraendert), lebende Token sagen nichts.
func TestQuantenWUrteil_SchlafendWieBisher(t *testing.T) {
	for _, tot := range []bool{false, true} {
		w := neueQsWelt(t, 128, 80)
		z := qwT0
		w.bereit(z)
		if w.ansicht()["urteil_eindeutig"] != nil {
			t.Fatal("schlafend: urteil_eindeutig gesetzt")
		}
		if tot {
			w.totMachen(z + 50)
		} else {
			w.einloesen(z + 50)
		}
		v := w.ansicht()
		if v["tot"] != true || v["urteil"] != nil || v["ende"] != nil {
			t.Fatalf("schlafend (tot=%v): %v", tot, v)
		}
		reg := w.cs.quantenWJetzt()
		if len(reg.Enden) != 0 || strings.Contains(reg.kodiert(), "urteil") || strings.Contains(reg.kodiert(), "ende") {
			t.Fatalf("schlafend: Urteil im Register: %s", reg.kodiert())
		}
	}
}

// Aktiv: eingeloest und tot sind unterscheidbar, auf beiden Knoten gleich.
func TestQuantenWUrteil_EingeloestUndTot(t *testing.T) {
	qwuAktiv(t)
	for _, f := range []struct {
		tot  bool
		soll string
	}{{false, quantenWUrteilEingeloest}, {true, quantenWUrteilTot}} {
		w := neueQsWelt(t, 128, 80)
		z := qwT0
		w.bereit(z)
		if w.ansicht()["urteil_eindeutig"] != true {
			t.Fatal("aktiv: lebender Token ohne urteil_eindeutig")
		}
		if f.tot {
			w.totMachen(z + 50)
		} else {
			w.einloesen(z + 50)
		}
		v := w.ansicht()
		if v["tot"] != true || v["urteil"] != f.soll || v["ende"] == nil {
			t.Fatalf("aktiv (%s): %v", f.soll, v)
		}
		for _, cs := range []*ChainState{w.cs, w.nach} {
			if en := cs.quantenWJetzt().Enden[w.p.K]; en.Urteil != f.soll || en.Ende <= z {
				t.Fatalf("%s: Register %+v", f.soll, en)
			}
		}
		if w.cs.quantenWJetzt().kodiert() != w.nach.quantenWJetzt().kodiert() {
			t.Fatal("annehmender und nachspielender Knoten weichen ab")
		}
	}
}

// Spaetes Ende (laenger als quantenWToteBehalten nach der Ausgabe): schlafend
// spurlos (wie nie ausgegeben), aktiv mit Urteil -- und erst
// quantenWToteBehalten nach dem ENDE aufgeraeumt.
func TestQuantenWUrteil_SpaetesEnde(t *testing.T) {
	for _, aktiv := range []bool{false, true} {
		if aktiv {
			qwuAktiv(t)
		}
		w := neueQsWelt(t, 128, 80)
		z := qwT0
		w.bereit(z)
		spaet := z + quantenWToteBehalten + 1000
		w.einloesen(spaet)
		v := w.ansicht()
		if !aktiv {
			if v != nil {
				t.Fatalf("schlafend, spaetes Ende: %v", v)
			}
			continue
		}
		if v["urteil"] != quantenWUrteilEingeloest {
			t.Fatalf("aktiv, spaetes Ende: %v", v)
		}
		reg := w.cs.quantenWJetzt().kopie()
		ende := reg.Enden[w.p.K].Ende
		if !quantenWToteAufraeumen(reg, ende+quantenWToteBehalten-1) || reg.Tote[w.p.K] == 0 {
			t.Fatal("vor Ablauf nach dem Ende aufgeraeumt")
		}
		quantenWToteAufraeumen(reg, ende+quantenWToteBehalten+1)
		if _, da := reg.Tote[w.p.K]; da {
			t.Fatal("nach Ablauf nicht aufgeraeumt")
		}
		if _, da := reg.Enden[w.p.K]; da {
			t.Fatal("Urteil ohne toten Eintrag stehen geblieben")
		}
	}
}

// Lesen: nur eingeloest/tot mit Zeit des Endes, oder beides leer; Rundweg
// ueber die Kodierung byte-gleich.
func TestQuantenWUrteil_RegisterLesen(t *testing.T) {
	K := strings.Repeat("ab", 32)
	gut := `{"tote":[{"k":"` + K + `","zeit":5,"ende":9,"urteil":"tot"}]}`
	r, err := quantenWRegisterLesen(gut)
	if err != nil || r.Enden[K].Urteil != "tot" || r.Enden[K].Ende != 9 || r.kodiert() != gut {
		t.Fatalf("Gutfall: %v %+v %s", err, r, r.kodiert())
	}
	if r, err := quantenWRegisterLesen(`{"tote":[{"k":"` + K + `","zeit":5}]}`); err != nil || len(r.Enden) != 0 {
		t.Fatalf("ohne Urteil: %v", err)
	}
	for _, schlecht := range []string{
		`"ende":9,"urteil":"vielleicht"`, `"urteil":"tot"`, `"ende":9`, `"ende":-1,"urteil":"tot"`,
		`"ende":9223372036854775807,"urteil":"eingeloest"`, `"ende":9,"urteil":"Tot"`,
	} {
		if _, err := quantenWRegisterLesen(`{"tote":[{"k":"` + K + `","zeit":5,` + schlecht + `}]}`); err == nil {
			t.Errorf("angenommen: %s", schlecht)
		}
	}
}

// Werkzeug: nur das Urteil der Kette zaehlt; ohne Urteil beim letzten
// Versuch kein behaupteter Erfolg.
func TestQuantenSUrteilLesen(t *testing.T) {
	for _, f := range []struct {
		nach    map[string]interface{}
		fehl    int
		letzter bool
		soll    string
		fehler  bool
	}{
		{nil, 0, false, "eingeloest", false},
		{map[string]interface{}{"tot": true}, 1, false, "eingeloest", false},
		{map[string]interface{}{"tot": true, "urteil": "eingeloest"}, 2, true, "eingeloest", false},
		{map[string]interface{}{"tot": true, "urteil": "tot"}, 2, true, "tot", false},
		{map[string]interface{}{"tot": true, "urteil": "tot"}, 0, false, "tot", false},
		{map[string]interface{}{"tot": true}, 2, true, "", true},
		{nil, 2, true, "", true},
		{map[string]interface{}{"tot": true, "urteil": "echt"}, 2, true, "", true},
		{map[string]interface{}{"fehl": float64(1)}, 0, false, "abgewiesen", false},
		{map[string]interface{}{"fehl": float64(0)}, 0, false, "frist", false},
	} {
		u, err := quantenSUrteilLesen(f.nach, f.fehl, f.letzter)
		if (err != nil) != f.fehler || u != f.soll {
			t.Errorf("%v fehl=%d letzter=%v: %q %v", f.nach, f.fehl, f.letzter, u, err)
		}
	}
}

// Variante W: eine eingeloeste Note steht ab dem Stichtag mit Urteil
// "eingeloest" im Register, auf beiden Knoten gleich.
func TestQuantenWUrteil_WNoteEingeloest(t *testing.T) {
	qwuAktiv(t)
	w := neueQwWelt(t, 200)
	z := qwT0
	w.annehmen(w.praegenTx(z, 10_000_000), z+1)
	w.annehmen(w.anfrageTx(w.bert, 0, qwLK, w.bert, z+100), z+101)
	w.annehmen(w.zufallTx(0, 0, 1, 2, 3, 4), z+102)
	a := w.anfrage(w.cs)
	w.annehmen(w.antwortTx(w.bert, 0, w.p.ehrlich(a.Stellen, a.Basen)), z+103)
	if g := w.annehmen(w.offenTx(0, a.Stellen), z+104).QuantenW.gerechnet(); g.Urteil != "echt" || g.WertAn != w.bert {
		t.Fatalf("Einloesen: %+v", g)
	}
	for _, cs := range []*ChainState{w.cs, w.nach} {
		if en := cs.quantenWJetzt().Enden[w.p.K]; en.Urteil != quantenWUrteilEingeloest || en.Ende != z+104 {
			t.Fatalf("Register: %+v", en)
		}
	}
	if v := w.cs.quantenWNoteAnsicht(context.Background(), w.p.K); v["urteil"] != quantenWUrteilEingeloest {
		t.Fatalf("Ansicht: %v", v)
	}
}
