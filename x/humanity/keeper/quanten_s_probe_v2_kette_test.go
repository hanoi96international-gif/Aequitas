package keeper

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"sort"
	"strings"
	"testing"
)

// Kettenregel der Stichprobe v2 (quanten_w_s_kette.go, quanten_s.go):
// schlafend bis zum Stichtag -- jeder Satz mit probe_art oder salze bekommt
// einen Befund --, danach Ausgabe mit der Art in der Unterschrift und
// Anmelden mit den Salzen je Abschnitt.

// stellenV2Laden: nach Bindung und Wahl laedt jede Stelle ihre Stichprobe und
// die Salze bei allen Validatoren.
func (a *qsaWelt) stellenV2Laden(t *testing.T) (probe [2][]int, salze [2][]string) {
	t.Helper()
	w, s := a.w, a.s
	v := len(w.val)
	for b := 0; b < 2; b++ {
		alle := map[int]*big.Int{}
		for x := 1; x <= v; x++ {
			c, err := a.d[x-1].oeffnungFuerStelle(a.K, b)
			if err != nil {
				t.Fatalf("Validator %d, Stelle %d: %v", x, b, err)
			}
			von, bis, _ := quantenSAbschnitt(a.n, v, x)
			m, salz, err := quantenSOeffnungEntschluesselnV2(w.keys[s.stelle[b]], c, a.K, b, x, von, bis)
			if err != nil {
				t.Fatal(err)
			}
			for i, val := range m {
				alle[i] = val
			}
			salze[b] = append(salze[b], hex.EncodeToString(salz[:]))
		}
		for i := range alle {
			probe[b] = append(probe[b], i)
		}
		sort.Ints(probe[b])
	}
	return probe, salze
}

func (a *qsaWelt) anmeldenV2Tx(b int, probe []int, salze []string) Transaction {
	s := a.s
	tx := s.anmeldenTx(s.stelle[b], b, probe, s.vorlage(b, a.w.keys[a.w.anna]))
	tx.QuantenW.Salze = salze
	return tx
}

func qsp2Regel(t *testing.T, a *qsaWelt, tx Transaction, zeit int64) string {
	t.Helper()
	return a.w.bescheid(a.w.cs, tx, zeit).regel
}

func TestQuantenSProbeV2Kette_Schlafend(t *testing.T) {
	if quantenSProbeV2Aktiv(4_000_000_000) {
		t.Fatal("Stichprobe v2 ist ohne Freigabe des Betreibers scharf")
	}
	// Der Deploy-Waechter liest den Stichtag aus /api/status: schlafend = kein Feld.
	if _, da := quantenWStatus()["probe_v2_ab"]; da {
		t.Fatal("Status meldet einen Stichtag der Stichprobe v2, obwohl sie schlaeft")
	}
	quantenSProbeV2AbOverride.Store(1_900_000_000)
	if quantenWStatus()["probe_v2_ab"] != int64(1_900_000_000) {
		t.Fatalf("Status: %v", quantenWStatus())
	}
	quantenSProbeV2AbOverride.Store(0)
	a := neueQsaWelt(t, 140)
	a.praeparierenV2(t)
	proben := a.probenV2(t)
	// Ausgabe mit probe_art 2: Befund, auch richtig unterschrieben.
	if r := qsp2Regel(t, a, a.ausgebenV2Tx(t, proben, 2), a.zeit+1); r != "quanten_w_s_probe_v2" {
		t.Fatalf("Ausgabe v2 vor dem Stichtag: %q", r)
	}
	// Ausgabe wie bisher, aber mit salze: Befund (schon die Form -- salze
	// gehoert nicht zur Ausgabe).
	tx := a.ausgebenV2Tx(t, proben, 0)
	tx.QuantenW.Salze = []string{strings.Repeat("ab", 32)}
	if r := qsp2Regel(t, a, tx, a.zeit+1); r == "" {
		t.Fatal("Ausgabe mit salze vor dem Stichtag angenommen")
	}
	// Ein leeres salze ist wie ein fehlendes (ein aelterer Knoten verwirft es,
	// und omitempty laesst es beim Hashen weg): derselbe Bescheid, derselbe Hash.
	ohne := a.ausgebenV2Tx(t, proben, 0)
	leer := a.ausgebenV2Tx(t, proben, 0)
	leer.QuantenW.Salze = []string{}
	if r1, r2 := qsp2Regel(t, a, ohne, a.zeit+1), qsp2Regel(t, a, leer, a.zeit+1); r1 != r2 || r1 != "" {
		t.Fatalf("leeres salze: %q, ohne: %q", r2, r1)
	}
	if quantenWTxHash(&ohne) != quantenWTxHash(&leer) {
		t.Fatal("leeres salze aendert den Hash")
	}
	j1, _ := json.Marshal(ohne)
	j2, _ := json.Marshal(leer)
	if string(j1) != string(j2) || strings.Contains(string(j1), "probe_art") || strings.Contains(string(j1), "salze") {
		t.Fatalf("Block-Kodierung: %s / %s", j1, j2)
	}
	// Bisheriger Weg unveraendert: ausgeben, binden, waehlen, anmelden ohne salze.
	a.w.annehmen(ohne, a.zeit+1)
	a.w.annehmen(a.s.bindenTx(a.w.anna), a.zeit+2)
	a.w.annehmen(a.s.waehlenTx(a.w.anna, "1"), a.zeit+3)
	// Anmelden mit salze vor dem Stichtag: Befund.
	an := a.anmeldenV2Tx(1, []int{1, 2, 3}, []string{strings.Repeat("cd", 32)})
	if r := qsp2Regel(t, a, an, a.zeit+4); r != "quanten_w_s_probe_v2" {
		t.Fatalf("Anmelden mit salze vor dem Stichtag: %q", r)
	}
	// probe_art in einer anderen Art: abgewiesen (Form).
	bi := a.s.bindenTx(a.w.anna)
	bi.QuantenW.ProbeArt = 2
	if r := qsp2Regel(t, a, bi, a.zeit+4); r == "" {
		t.Fatal("binden mit probe_art angenommen")
	}
}

func TestQuantenSProbeV2Kette_Aktiv(t *testing.T) {
	quantenSProbeV2AbOverride.Store(1)
	defer quantenSProbeV2AbOverride.Store(0)
	a := neueQsaWelt(t, 140)
	w, s := a.w, a.s
	a.praeparierenV2(t)
	proben := a.probenV2(t)
	// Missbrauch der Ausgabe: andere Art; Art ohne/mit falschem Wortlaut unterschrieben.
	for _, art := range []int{1, 3} {
		if r := qsp2Regel(t, a, a.ausgebenV2Tx(t, proben, art), a.zeit+1); r == "" {
			t.Fatalf("Ausgabe mit probe_art %d angenommen", art)
		}
	}
	falsch := a.ausgebenV2Tx(t, proben, 0)
	falsch.QuantenW.ProbeArt = 2 // unterschrieben ohne "|probe:2"
	if r := qsp2Regel(t, a, falsch, a.zeit+1); r != "quanten_w_unterschrift" {
		t.Fatalf("Art ohne Unterschrift: %q", r)
	}
	falsch2 := a.ausgebenV2Tx(t, proben, 2)
	falsch2.QuantenW.ProbeArt = 0 // unterschrieben mit "|probe:2"
	if r := qsp2Regel(t, a, falsch2, a.zeit+1); r != "quanten_w_unterschrift" {
		t.Fatalf("Art entfernt: %q", r)
	}
	a.ausgebenV2Art(t, proben, 2)
	if tok := w.cs.quantenWJetzt().Token[a.K]; tok == nil || tok.token.ProbeArt != 2 {
		t.Fatal("Token nicht als Stichprobe v2 ausgegeben")
	}
	w.annehmen(s.waehlenTx(w.anna, "1"), a.zeit+3)
	probe, salze := a.stellenV2Laden(t)
	// Missbrauch des Anmeldens.
	falschesSalz := append([]string(nil), salze[1]...)
	falschesSalz[0] = strings.Repeat("00", 32)
	for name, tx := range map[string]Transaction{
		"ohne Salze":                    a.anmeldenV2Tx(1, probe[1], nil),
		"falsches Salz":                 a.anmeldenV2Tx(1, probe[1], falschesSalz),
		"Salz zu wenig":                 a.anmeldenV2Tx(1, probe[1], salze[1][:len(salze[1])-1]),
		"Salze der anderen Stelle":      a.anmeldenV2Tx(1, probe[1], salze[0]),
		"Teilmenge":                     a.anmeldenV2Tx(1, probe[1][1:], salze[1]),
		"Stichprobe der anderen Stelle": a.anmeldenV2Tx(1, probe[0], salze[0]),
		"Salz kein Hex":                 a.anmeldenV2Tx(1, probe[1], append([]string{"zz"}, salze[1][1:]...)),
	} {
		if r := qsp2Regel(t, a, tx, a.zeit+4); r == "" {
			t.Fatalf("Anmelden %s angenommen", name)
		}
	}
	// Doppelvorlage an Stelle 1 (dort prueft die Kette die Gegenbasis): mit den
	// Salzen angemeldet, von den Validatoren offengelegt, abgewiesen.
	a.qsDienste.zeit = a.zeit + 10
	w.annehmen(a.anmeldenV2Tx(1, probe[1], salze[1]), a.qsDienste.zeit)
	if tok := w.cs.quantenWJetzt().Token[a.K]; tok.token.Anmeldung == nil || len(tok.token.Anmeldung.Salze) != len(w.val) {
		t.Fatal("Anmeldung ohne Salze")
	}
	a.qsDienste.zeit += 5
	a.takt()
	a.takt()
	if tok := w.cs.quantenWJetzt().Token[a.K]; tok == nil || tok.token.Fehl != 1 || tok.token.Anmeldung != nil {
		t.Fatal("Doppelvorlage v2 nicht abgewiesen")
	}
	// Ehrliche Vorlage an Stelle 0: eingeloest, die Deckung geht an die Stelle.
	w.annehmen(a.anmeldenV2Tx(0, probe[0], salze[0]), a.qsDienste.zeit)
	tok := w.cs.quantenWJetzt().Token[a.K]
	if tok.token.Anmeldung == nil || len(tok.token.Anmeldung.Salze) != len(w.val) {
		t.Fatal("Anmeldung ohne Salze")
	}
	// Register: Form -> JSON -> Lesen ergibt denselben Token.
	f := tok.form(a.K)
	roh, _ := json.Marshal(f)
	var f2 quantenWSTokenForm
	if err := json.Unmarshal(roh, &f2); err != nil {
		t.Fatal(err)
	}
	gelesen, err := quantenWSTokenLesen(f2)
	if err != nil {
		t.Fatalf("Register lesen: %v", err)
	}
	if g, _ := json.Marshal(gelesen.form(a.K)); string(g) != string(roh) {
		t.Fatalf("Register rund: %s / %s", g, roh)
	}
	a.qsDienste.zeit += 5
	a.takt()
	a.takt()
	if s.token(w.cs) != nil || w.stand(w.cs, s.stelle[0]) != 10_000_000 {
		t.Fatalf("v2-Token nicht eingeloest: Stelle 0 hat %d", w.stand(w.cs, s.stelle[0]))
	}
	// Fail-closed beim Lesen: Salze weg, Art unbekannt, Salze bei Art 0.
	for name, mach := range map[string]func(*quantenWSTokenForm){
		"Salze weg":    func(x *quantenWSTokenForm) { x.Anmeldung.Salze = nil },
		"Salz falsch":  func(x *quantenWSTokenForm) { x.Anmeldung.Salze[0] = strings.Repeat("00", 32) },
		"Art 3":        func(x *quantenWSTokenForm) { x.ProbeArt = 3 },
		"Art 0 mit v2": func(x *quantenWSTokenForm) { x.ProbeArt = 0 },
	} {
		var g quantenWSTokenForm
		_ = json.Unmarshal(roh, &g)
		mach(&g)
		if _, err := quantenWSTokenLesen(g); err == nil {
			t.Fatalf("Register mit %s gelesen", name)
		}
	}
	// Auch ohne Anmeldung: eine unbekannte Art liest das Register nicht.
	var ohneAnm quantenWSTokenForm
	_ = json.Unmarshal(roh, &ohneAnm)
	ohneAnm.Anmeldung = nil
	if _, err := quantenWSTokenLesen(ohneAnm); err != nil {
		t.Fatalf("v2-Token ohne Anmeldung: %v", err)
	}
	ohneAnm.ProbeArt = 3
	if _, err := quantenWSTokenLesen(ohneAnm); err == nil {
		t.Fatal("Register mit Art 3 (ohne Anmeldung) gelesen")
	}
}

// Auch ein W-Satz (Praegen) mit leerem salze ist wie einer ohne: derselbe
// Bescheid und derselbe Hash -- ein aelterer Knoten sieht das Feld nicht.
func TestQuantenSProbeV2Kette_LeeresSalzeBeiW(t *testing.T) {
	w := neueQwWelt(t, 64)
	z := qwT0
	ohne := w.praegenTx(z, 10_000_000)
	leer := w.praegenTx(z, 10_000_000)
	leer.QuantenW.Salze = []string{}
	r1, r2 := w.bescheid(w.cs, ohne, z).regel, w.bescheid(w.cs, leer, z).regel
	if r1 != r2 || r1 != "" {
		t.Fatalf("Praegen mit leerem salze: %q, ohne: %q", r2, r1)
	}
	if quantenWTxHash(&ohne) != quantenWTxHash(&leer) {
		t.Fatal("leeres salze aendert den Hash")
	}
	// Mit einem Salz: Befund (gehoert nicht zum Praegen).
	voll := w.praegenTx(z, 10_000_000)
	voll.QuantenW.Salze = []string{strings.Repeat("ab", 32)}
	if w.bescheid(w.cs, voll, z).regel == "" {
		t.Fatal("Praegen mit salze angenommen")
	}
}

// Bisherige Token (Art 0) behalten ihr Register Wort fuer Wort: weder
// probe_art noch salze in der Form (StateRoot unveraendert), und nach dem
// Stichtag weisen sie salze beim Anmelden ab.
func TestQuantenSProbeV2Kette_BisherigeToken(t *testing.T) {
	quantenSProbeV2AbOverride.Store(1)
	defer quantenSProbeV2AbOverride.Store(0)
	a := neueQsaWelt(t, 140)
	a.ausgebenAbschnitte(t) // bisheriger Weg (Stichproben vor der Ausgabe)
	tok := a.w.cs.quantenWJetzt().Token[a.K]
	roh, _ := json.Marshal(tok.form(a.K))
	if strings.Contains(string(roh), "probe_art") || strings.Contains(string(roh), "salze") {
		t.Fatalf("bisheriges Register traegt neue Felder: %s", roh)
	}
	an := a.s.anmeldenTx(a.s.stelle[1], 1, a.s.probe[1], a.s.vorlage(1, a.w.keys[a.w.anna]))
	an.QuantenW.Salze = []string{strings.Repeat("ab", 32)}
	if r := qsp2Regel(t, a, an, a.zeit+4); r == "" {
		t.Fatal("bisheriger Token nimmt salze an")
	}
	an.QuantenW.Salze = nil
	a.w.annehmen(an, a.zeit+4)
	roh2, _ := json.Marshal(a.w.cs.quantenWJetzt().Token[a.K].form(a.K))
	if strings.Contains(string(roh2), "salze") {
		t.Fatalf("Anmeldung eines bisherigen Tokens traegt salze: %s", roh2)
	}
	// Im Register: eine Anmeldung eines bisherigen Tokens mit Salzen wird
	// nicht gelesen (fail-closed); ohne liest sie sich wie zuvor.
	var f quantenWSTokenForm
	_ = json.Unmarshal(roh2, &f)
	if _, err := quantenWSTokenLesen(f); err != nil {
		t.Fatalf("bisheriges Register: %v", err)
	}
	f.Anmeldung.Salze = []string{strings.Repeat("ab", 32)}
	if _, err := quantenWSTokenLesen(f); err == nil {
		t.Fatal("bisheriges Register mit Salzen gelesen")
	}
}
