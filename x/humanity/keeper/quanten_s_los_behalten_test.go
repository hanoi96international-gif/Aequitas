package keeper

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
)

// P2a, Los je Token behalten (quanten_s_los_behalten.go): gegen die echten
// Regeln, mit annehmendem und nachspielendem Knoten im Gleichschritt (bWelt).

func losAktiv(t *testing.T) {
	t.Helper()
	quantenSLosBehaltenAbOverride.Store(qwT0 - 30*86400)
	t.Cleanup(func() { quantenSLosBehaltenAbOverride.Store(0) })
}

func losSchlafend(t *testing.T) {
	t.Helper()
	quantenSLosBehaltenAbOverride.Store(math.MaxInt64)
	t.Cleanup(func() { quantenSLosBehaltenAbOverride.Store(0) })
}

// bWeltAusgegeben: bestellt, gelost, gegen das Los ausgegeben, gebunden, gewaehlt.
func bWeltAusgegeben(t *testing.T) (*bWelt, []string) {
	t.Helper()
	w := neueBWelt(t)
	los := slices.Clone(w.bestelltUndGelost())
	w.annehmen(w.ausgabeMit(los, w.z+900), w.z+900)
	w.annehmen(w.bindenTx(w.anna), w.z+902)
	w.annehmen(w.waehlenTx(w.anna, string(rune('0'+w.c))), w.z+903)
	return w, los
}

// Die Nachweis-Frist (P4) muss ganz in der Zeit liegen, in der das Register
// den toten Eintrag -- und damit das Los -- behaelt.
func TestQuantenSLosBehalten_FristImNachlauf(t *testing.T) {
	if quantenSNachweisFrist >= quantenWToteBehalten {
		t.Fatalf("Frist %d s, Tote nur %d s behalten", quantenSNachweisFrist, quantenWToteBehalten)
	}
}

// Schlafend: wie bisher -- kein Los im Register, keine Zeile, keine Ansicht,
// kein Feld im Status; die Bytes sind die des Registers ohne P2a.
func TestQuantenSLosBehalten_Schlafend(t *testing.T) {
	losSchlafend(t)
	qwuAktiv(t)
	w, _ := bWeltAusgegeben(t)
	for _, cs := range []*ChainState{w.cs, w.nach} {
		reg := cs.quantenWJetzt()
		if len(reg.Lose) != 0 || strings.Contains(reg.kodiert(), `"lose"`) {
			t.Fatalf("schlafend: Los im Register: %v", reg.Lose)
		}
		for z := range reg.form().zeilen() {
			if z.art == quantenWArtLos {
				t.Fatal("schlafend: Zeile der Art los")
			}
		}
	}
	if v := w.ansicht(); v["los"] != nil {
		t.Fatalf("schlafend: Los in der Ansicht: %v", v)
	}
	w.einloesen(w.z + 1000)
	if v := w.ansicht(); v["tot"] != true || v["los"] != nil {
		t.Fatalf("schlafend, nach dem Ende: %v", v)
	}
	if _, da := quantenWStatus()["los_ab"]; da {
		t.Fatal("schlafend: los_ab im Status")
	}
}

// Gutfall: das Los steht ab der Ausgabe im Register (beide Knoten gleich),
// in Losreihenfolge, in der Ansicht, in einer eigenen Zeile; es bleibt nach
// dem Ende (eingeloest wie tot) und geht erst mit dem toten Eintrag.
func TestQuantenSLosBehalten_Gutfall(t *testing.T) {
	losAktiv(t)
	qwuAktiv(t)
	for _, tot := range []bool{false, true} {
		w, los := bWeltAusgegeben(t)
		for _, cs := range []*ChainState{w.cs, w.nach} {
			if got := cs.quantenWJetzt().Lose[w.p.K]; !slices.Equal(got, los) {
				t.Fatalf("Los im Register %v, gelost %v", got, los)
			}
		}
		reg := w.cs.quantenWJetzt()
		if r, err := quantenWRegisterLesen(reg.kodiert()); err != nil || r.kodiert() != reg.kodiert() {
			t.Fatalf("Rundweg: %v", err)
		}
		if reg.form().zeilen()[quantenWZeile{quantenWArtLos, w.p.K}] == "" {
			t.Fatal("keine Zeile der Art los")
		}
		if v := w.ansicht(); !slices.Equal(v["los"].([]string), los) {
			t.Fatalf("Ansicht des lebenden Tokens: %v", v["los"])
		}
		if tot {
			w.totMachen(w.z + 1000)
		} else {
			w.einloesen(w.z + 1000)
		}
		v := w.ansicht()
		if v["tot"] != true || !slices.Equal(v["los"].([]string), los) {
			t.Fatalf("nach dem Ende (tot=%v): %v", tot, v)
		}
		reg = w.cs.quantenWJetzt()
		if r, err := quantenWRegisterLesen(reg.kodiert()); err != nil || !slices.Equal(r.Lose[w.p.K], los) {
			t.Fatalf("nach dem Ende gelesen: %v", err)
		}
		// Aufraeumen: bis zum Ablauf nach dem Ende bleibt das Los (Frist),
		// danach geht es mit dem toten Eintrag.
		ende := reg.Enden[w.p.K].Ende
		k := reg.kopie()
		quantenWToteAufraeumen(k, ende+quantenSNachweisFrist)
		if !slices.Equal(k.Lose[w.p.K], los) {
			t.Fatal("Los vor dem Ende der Frist aufgeraeumt")
		}
		quantenWToteAufraeumen(k, ende+quantenWToteBehalten+1)
		if _, da := k.Lose[w.p.K]; da {
			t.Fatal("Los ohne toten Eintrag stehen geblieben")
		}
		if _, err := quantenWRegisterLesen(k.kodiert()); err != nil {
			t.Fatalf("aufgeraeumtes Register: %v", err)
		}
	}
	if ab, _ := quantenWStatus()["los_ab"].(int64); ab != qwT0-30*86400 {
		t.Fatalf("Status los_ab: %v", quantenWStatus()["los_ab"])
	}
}

// Weg A (gebundene Validatoren, nicht bestellt): kein Los, nichts gemerkt.
func TestQuantenSLosBehalten_WegAOhneLos(t *testing.T) {
	losAktiv(t)
	w := neueQsWelt(t, 128, 80)
	w.bereit(qwT0)
	if reg := w.cs.quantenWJetzt(); len(reg.Lose) != 0 {
		t.Fatalf("Weg A: Los gemerkt: %v", reg.Lose)
	}
}

// Spaetes Ende ohne Urteil (Stichtag des Urteils schlafend): toteMerken
// merkt den Token nicht -- dann darf auch kein Los ohne Eintrag stehen
// bleiben, sonst liest kein Knoten das Register mehr.
func TestQuantenSLosBehalten_SpaetesEndeOhneTot(t *testing.T) {
	losAktiv(t)
	quantenWUrteilAbOverride.Store(math.MaxInt64)
	t.Cleanup(func() { quantenWUrteilAbOverride.Store(0) })
	w, _ := bWeltAusgegeben(t)
	w.einloesen(w.z + 900 + quantenWToteBehalten + 1000)
	reg := w.cs.quantenWJetzt()
	if _, tot := reg.Tote[w.p.K]; tot {
		t.Fatal("Vorbedingung: spaetes Ende ohne Urteil wurde doch gemerkt")
	}
	if len(reg.Lose) != 0 {
		t.Fatalf("Los ohne Token und ohne toten Eintrag: %v", reg.Lose)
	}
	if _, err := quantenWRegisterLesen(reg.kodiert()); err != nil {
		t.Fatalf("Register nicht mehr lesbar: %v", err)
	}
}

// Missbrauch beim Lesen (Snapshot, Tabelle, Start): alles, was so nie
// entstehen konnte, wird abgewiesen -- fail-closed.
func TestQuantenSLosBehalten_RegisterLesen(t *testing.T) {
	losAktiv(t)
	qwuAktiv(t)
	w, los := bWeltAusgegeben(t)
	gut := w.cs.quantenWJetzt().form()
	if len(gut.Lose) != 1 {
		t.Fatalf("Vorbedingung: %d Lose", len(gut.Lose))
	}
	_, fremd := neuerSchluessel(t)
	kaputt := map[string]func(f *quantenSLosForm){
		"K ohne Token":        func(f *quantenSLosForm) { f.K = strings.Repeat("e", 64) },
		"K kein Hash":         func(f *quantenSLosForm) { f.K = "abc" },
		"K gross":             func(f *quantenSLosForm) { f.K = strings.ToUpper(f.K) },
		"leer":                func(f *quantenSLosForm) { f.Los = nil },
		"zu kurz":             func(f *quantenSLosForm) { f.Los = f.Los[:quantenSLosVMin-1] },
		"v passt nicht":       func(f *quantenSLosForm) { f.Los = f.Los[:len(f.Los)-1] },
		"doppelt":             func(f *quantenSLosForm) { f.Los[1] = f.Los[0] },
		"nicht kanonisch":     func(f *quantenSLosForm) { f.Los[0] = strings.ToUpper(f.Los[0][2:]) },
		"Systemadresse":       func(f *quantenSLosForm) { f.Los[0] = ubiPoolAddr },
		"kein Konto":          func(f *quantenSLosForm) { f.Los[0] = "0x1234" },
		"zu lang":             func(f *quantenSLosForm) { f.Los = losMit(t, quantenSLosVMax+1) },
		"fremdes Konto passt": func(f *quantenSLosForm) { f.Los[0] = fremd }, // Form ok: v passt, Konto gueltig
	}
	for name, aendern := range kaputt {
		f := gut
		l := quantenSLosForm{K: gut.Lose[0].K, Los: slices.Clone(los)}
		aendern(&l)
		f.Lose = []quantenSLosForm{l}
		_, err := quantenWRegisterLesen(f.kodiert())
		if name == "fremdes Konto passt" {
			// Das Los selbst prueft der Faelschungsnachweis gegen die Kette
			// nicht nach (es gibt die Bestellung nicht mehr); ein Snapshot
			// kommt aber nur von einem Knoten, dessen StateRoot die Mehrheit
			// traegt -- ein falsches Los aendert die Wurzel.
			if err != nil {
				t.Errorf("%s: abgewiesen (%v)", name, err)
			}
			if f.kodiert() == gut.kodiert() {
				t.Error("fremdes Los aendert die Bytes nicht")
			}
			continue
		}
		if err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
	// Ordnung: zwei Eintraege verkehrt herum, doppelt.
	f := gut
	f.Lose = []quantenSLosForm{gut.Lose[0], gut.Lose[0]}
	if _, err := quantenWRegisterLesen(f.kodiert()); err == nil {
		t.Error("doppelter Eintrag angenommen")
	}
	// Grenze: mehr Eintraege als je Token und tote K moeglich.
	f = gut
	f.Lose = make([]quantenSLosForm, quantenSLoseHoechstens+1)
	for i := range f.Lose {
		f.Lose[i] = quantenSLosForm{K: fmt.Sprintf("%064x", i+1), Los: los}
	}
	if _, err := quantenWRegisterLesen(f.kodiert()); err == nil || !strings.Contains(err.Error(), "hoechstens") {
		t.Errorf("ueber der Grenze: %v", err)
	}
	// Nur ein Los, sonst nichts im Register: kein Token, kein toter Eintrag.
	f = quantenWRegisterForm{Lose: gut.Lose}
	if _, err := quantenWRegisterLesen(f.kodiert()); err == nil {
		t.Error("Los ohne Register angenommen")
	}
	// Unbekanntes Feld in der Kodierung.
	roh := strings.Replace(gut.kodiert(), `"lose":[{"k"`, `"lose":[{"x":1,"k"`, 1)
	if roh == gut.kodiert() {
		t.Fatal("Vorbedingung: Feld nicht ersetzt")
	}
	if _, err := quantenWRegisterLesen(roh); err == nil {
		t.Error("unbekanntes Feld angenommen")
	}
	// Zeile der Tabelle: dieselben Bytes wie im Register, streng gelesen.
	z := gut.zeilen()[quantenWZeile{quantenWArtLos, gut.Lose[0].K}]
	var l quantenSLosForm
	if err := quantenWZeileLesen(z, &l); err != nil || l.K != gut.Lose[0].K || !slices.Equal(l.Los, los) {
		t.Fatalf("Zeile: %v", err)
	}
	if err := quantenWZeileLesen(z+`{}`, &l); err == nil {
		t.Error("Daten nach der Zeile angenommen")
	}
	// Nach dem Ende gibt es kein v des Tokens mehr: dann tragen allein die
	// Grenzen des Loses (quantenSLosVMin..quantenSLosVMax).
	w.einloesen(w.z + 1000)
	totF := w.cs.quantenWJetzt().form()
	if len(totF.Lose) != 1 || len(totF.Token) != 0 {
		t.Fatalf("Vorbedingung tot: %d Lose, %d Token", len(totF.Lose), len(totF.Token))
	}
	for name, n := range map[string]int{"tot, zu kurz": quantenSLosVMin - 1, "tot, zu lang": quantenSLosVMax + 1} {
		f := totF
		f.Lose = []quantenSLosForm{{K: totF.Lose[0].K, Los: losMit(t, n)}}
		if _, err := quantenWRegisterLesen(f.kodiert()); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
	for _, n := range []int{quantenSLosVMin, quantenSLosVMax} {
		f := totF
		f.Lose = []quantenSLosForm{{K: totF.Lose[0].K, Los: losMit(t, n)}}
		if _, err := quantenWRegisterLesen(f.kodiert()); err != nil {
			t.Errorf("tot, v = %d an der Grenze abgewiesen: %v", n, err)
		}
	}
}

// losMit: n verschiedene kanonische Konten.
func losMit(t *testing.T, n int) []string {
	t.Helper()
	var out []string
	for i := 0; i < n; i++ {
		_, a := neuerSchluessel(t)
		out = append(out, a)
	}
	return out
}

// Kopie: das Register im Speicher bleibt unberuehrt, wenn eine Kopie das
// Los ersetzt oder loescht.
func TestQuantenSLosBehalten_Kopie(t *testing.T) {
	r := neuesQuantenWRegister()
	r.Lose["a"] = []string{"x"}
	k := r.kopie()
	delete(k.Lose, "a")
	k.Lose["b"] = []string{"y"}
	if len(r.Lose) != 1 || r.Lose["a"] == nil || r.leer() {
		t.Fatalf("Original veraendert: %v", r.Lose)
	}
}

// Tabelle gegen eine echte Datenbank: das Los als eigene Zeile, Neustart
// liest es zurueck (dieselbe Wurzel), Aufraeumen loescht die Zeile.
func TestQuantenSLosBehalten_Tabelle_RealDB(t *testing.T) {
	losAktiv(t)
	qwuAktiv(t)
	w, los := bWeltAusgegeben(t)
	lebend := w.cs.quantenWJetzt()
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-quanten-s-los-realdb-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	ctx := context.Background()
	if err := qwtSetzen(t, cs, ctx, lebend); err != nil {
		t.Fatal(err)
	}
	if z := qwtZeilen(t, cs)[quantenWArtLos+"|"+w.p.K]; z.daten == "" {
		t.Fatal("keine Zeile der Art los")
	}
	neu := testKnoten(t, "unused-quanten-s-los-realdb-test-2.json")
	if neu.quantenWWurzel() != cs.quantenWWurzel() || !slices.Equal(neu.quantenWJetzt().Lose[w.p.K], los) {
		t.Fatal("nach dem Neustart ein anderes Register")
	}
	// Manipulierte Zeile (Los um ein Konto gekuerzt): Laden scheitert.
	l := quantenSLosForm{K: w.p.K, Los: los[:len(los)-1]}
	roh := quantenWRegisterForm{Lose: []quantenSLosForm{l}}.zeilen()[quantenWZeile{quantenWArtLos, w.p.K}]
	if _, err := cs.db.Exec(`UPDATE `+quantenWTabelle+` SET daten = $1 WHERE art = $2 AND k = $3`, roh, quantenWArtLos, w.p.K); err != nil {
		t.Fatal(err)
	}
	if err := qwtLaden(cs); err == nil {
		t.Error("manipulierte Zeile geladen")
	}
}

// Der echte Stichtag (Betreiber, 26.11.2026): auf die Sekunde, nach Weg B,
// Status; die Pfaender schlafen weiter.
func TestQuantenSLosBehalten_Stichtag(t *testing.T) {
	const ab int64 = 1795762800
	if quantenSLosBehaltenAbUnix != ab {
		t.Fatalf("Stichtag Los behalten %d, erwartet %d (27.11.2026 07:00 UTC)", quantenSLosBehaltenAbUnix, ab)
	}
	if quantenSLosBehaltenAbOverride.Load() != 0 || quantenSWegBAbOverride.Load() != 0 || quantenSPfaenderAbOverride.Load() != 0 {
		t.Fatal("Override gesetzt")
	}
	if quantenSWegBAbUnix > ab {
		t.Fatalf("Weg B (%d) erst nach Los behalten (%d) aktiv", quantenSWegBAbUnix, ab)
	}
	if quantenSLosBehaltenAktiv(ab-1) || !quantenSLosBehaltenAktiv(ab) {
		t.Fatal("Stichtag wirkt nicht auf die Sekunde")
	}
	if v, da := quantenWStatus()["los_ab"]; !da || v != ab {
		t.Fatalf("Status nennt den Stichtag nicht: %v", v)
	}
	if quantenSPfaenderAktiv(ab + 365*86400) {
		t.Fatal("Pfaender mitgeschaltet")
	}
	if _, da := quantenWStatus()["pfaender_ab"]; da {
		t.Fatal("pfaender_ab im Status")
	}
}
