package keeper

import (
	"errors"
	"math"
	"testing"
)

// Anlauf nach einer neuen Genesis: ein Schluessel der geschlossenen Liste
// erzeugt ohne Bindung -- ab der Genesis, hoechstens erzeugerAnlaufDauer,
// und nur bis seine erste Bindung wirkt. Danach allein das Register.
func TestErzeugerAnlauf_FensterUndGrenzen(t *testing.T) {
	g := int64(1_794_758_400)
	ende := g + erzeugerAnlaufDauer
	bindung := g + 3*3600 // C1 bindet drei Stunden nach der Genesis
	st := standAus(z("mc1", "0xc1", bindung))
	m := anlaufFensterErgaenzen(st.fenster, []string{"0xc1", "0xb"}, g, nil)
	if len(st.fenster["0xb"]) != 0 {
		t.Fatal("Eingabe veraendert")
	}
	a := &erzeugerStand{fenster: m}
	for name, f := range map[string]struct {
		addr string
		t    int64
		want string
	}{
		"vor der Genesis":                    {"0xb", g - 1, ""},
		"ab der Genesis":                     {"0xb", g, anlaufBetreiber},
		"letzte Sekunde des Anlaufs":         {"0xb", ende - 1, anlaufBetreiber},
		"nach dem Anlauf, nie gebunden":      {"0xb", ende, ""},
		"C1 vor der Bindung":                 {"0xc1", bindung - 1, anlaufBetreiber},
		"C1 in der Frist seiner Bindung":     {"0xc1", bindung + erzeugerFrist - 1, anlaufBetreiber},
		"C1 ab der Wirkung seiner Bindung":   {"0xc1", bindung + erzeugerFrist, "mc1"},
		"C1 nach dem Anlauf, gebunden":       {"0xc1", ende + 1, "mc1"},
		"fremder Schluessel, nicht in Liste": {"0xf", g + 60, ""},
	} {
		if got := a.erzeugerFenster(f.addr, f.t); got != f.want {
			t.Errorf("%s: %q, erwartet %q", name, got, f.want)
		}
	}
}

// Missbrauch: das Register nimmt nach dem Anlauf weiter weg. Ein Schluessel,
// dessen Betreiber ihn gebunden und wieder an einen anderen Schluessel
// abgegeben hat, bekommt keinen Anlauf zurueck; ohne Liste (offen) gibt es
// gar keinen; ein Lesefehler schliesst weiter ab.
func TestErzeugerAnlauf_NimmtNichtsZurueckUndOffenNie(t *testing.T) {
	g := int64(1_794_758_400)
	erzeugerSchnittOverride.Store(1)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(math.MaxInt64) })

	// 0xa bindet bei g+1h, wechselt bei g+5h auf 0xa2: 0xa darf ab
	// g+5h+Frist nicht mehr -- auch nicht im Anlaufzeitraum.
	st := standAus(z("ma", "0xa", g+3600), z("ma", "0xa2", g+5*3600))
	st.fenster = anlaufFensterErgaenzen(st.fenster, []string{"0xa", "0xb"}, g, nil)
	dag := erzeugerTestDAG(st)
	spaeter := g + 5*3600 + erzeugerFrist + 60
	if dag.erzeugerErlaubt("0xa", spaeter) {
		t.Fatal("abgegebener Schluessel erzeugt im Anlauf weiter")
	}
	if !dag.erzeugerErlaubt("0xa", g+60) {
		t.Fatal("0xa vor seiner Bindung nicht im Anlauf")
	}
	if !dag.erzeugerErlaubt("0xb", spaeter) {
		t.Fatal("0xb (Liste, nie gebunden) im Anlauf abgewiesen")
	}
	// Eigener Schluessel ausserhalb der Liste: kein Anlauf, ueber die Liste.
	dag.state.erzeugerAnlauf.Store(&[]string{"0xa", "0xb"})
	if got := dag.erzeugerAnlaufListe(); len(got) != 2 || got[0] != "0xa" || got[1] != "0xb" {
		t.Fatalf("Anlaufliste %v -- nur AUTHORIZED_VALIDATORS, ohne den eigenen Schluessel", got)
	}
	if dag.erzeugerErlaubt("0xs", g+60) {
		t.Fatal("eigener Schluessel ausserhalb der Liste erzeugt im Anlauf")
	}
	// Offen: keine Liste, kein Anlauf.
	dag.produzentenFest = nil
	if l := dag.erzeugerAnlaufListe(); l != nil {
		t.Fatalf("offen mit Anlaufliste %v", l)
	}
	leer := &erzeugerStand{fenster: anlaufFensterErgaenzen(map[string][]zeitfenster{}, dag.erzeugerAnlaufListe(), g, nil)}
	dag.state.erzeugerRegister.Store(leer)
	if dag.erzeugerErlaubt("0xb", g+60) {
		t.Fatal("offen: Anlauf ohne Liste")
	}
	// Lesefehler: zu, auch im Anlauf.
	dag.produzentenFest = map[string]bool{"0xa": true, "0xb": true}
	fehler := &erzeugerStand{fenster: st.fenster, fehler: errors.New("db weg")}
	dag.state.erzeugerRegister.Store(fehler)
	if dag.erzeugerErlaubt("0xb", g+60) {
		t.Fatal("Lesefehler: Anlauf offen")
	}
}

// Der Anlauf ist keine Bindung: /api/status fuehrt den Schluessel weiter
// unter erzeuger_ohne_bindung, und das Komitee nimmt ihn auf.
func TestErzeugerAnlauf_StatusUndKomitee(t *testing.T) {
	g := nowUnix() - 600
	st := standAus()
	st.fenster = anlaufFensterErgaenzen(map[string][]zeitfenster{}, []string{"0xa", "0xb"}, g, nil)
	st.komiteeRang = komiteeVorberechnen(st, nowUnix())
	dag := erzeugerTestDAG(st)
	if o := dag.ErzeugerOhneBindung(); len(o) != 2 {
		t.Fatalf("erzeuger_ohne_bindung %v -- der Anlauf darf nicht als Bindung zaehlen", o)
	}
	erzeugerSchnittOverride.Store(1)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(math.MaxInt64) })
	ec := dag.komiteeAusRegister(nowUnix())
	if !ec.Members["0xa"] || !ec.Members["0xb"] || ec.Members["0xs"] {
		t.Fatalf("Komitee im Anlauf: %v", ec.Members)
	}
}

// Sicherheitsdurchgang #524: ist der Betreiber einer Bindung heute kein
// Mensch mehr, faellt sein Fenster weg (fensterAusIntervallen) -- der Anlauf
// darf deshalb nicht zurueckkommen. Er endet an der ersten Zeile im Verlauf,
// gleich von wem.
func TestErzeugerAnlauf_EndetAnDerErstenZeileAuchOhneMenschen(t *testing.T) {
	g := int64(1_794_758_400)
	zeilen := []bindungsZeile{z("nichtmehrmensch", "0xa", g+3600), z("mb", "0xb", g+7200)}
	// 0xa: Betreiber ist kein Mensch mehr -> kein echtes Fenster
	fenster := fensterAusIntervallen(bindungsIntervalle(zeilen), map[string]bool{"mb": true}, nil)
	if len(fenster["0xa"]) != 0 {
		t.Fatal("Testaufbau: 0xa haette kein Fenster haben duerfen")
	}
	erste := ersteBindungen(zeilen)
	if erste["0xa"] != g+3600 || erste["0xb"] != g+7200 {
		t.Fatalf("erste Bindungen: %v", erste)
	}
	st := &erzeugerStand{fenster: anlaufFensterErgaenzen(fenster, []string{"0xa", "0xb"}, g, erste)}
	if got := st.erzeugerFenster("0xa", g+3600+erzeugerFrist-1); got != anlaufBetreiber {
		t.Fatalf("0xa vor dem Ende seines Anlaufs: %q", got)
	}
	for _, zeit := range []int64{g + 3600 + erzeugerFrist, g + 86400, g + erzeugerAnlaufDauer - 1} {
		if got := st.erzeugerFenster("0xa", zeit); got != "" {
			t.Fatalf("0xa zur Zeit %d: %q -- Anlauf zurueckgekommen", zeit, got)
		}
	}
	if got := st.erzeugerFenster("0xb", g+7200+erzeugerFrist); got != "mb" {
		t.Fatalf("0xb mit wirksamer Bindung: %q", got)
	}
}
