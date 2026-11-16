package keeper

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"testing"
)

// Variante W, Stufe W2a (quanten_w.go, quanten_w_register.go): der Knoten
// rechnet genau wie die Spezifikation (forschung/quantengeld/wiesner_kette.py,
// Pruefvektor testdata/quanten_w_vektor.json), und die Uebergaenge weisen
// jeden Missbrauch ab.

type qwVektor struct {
	K            string     `json:"K"`
	N            int        `json:"n"`
	T            int        `json:"t"`
	Validatoren  []string   `json:"validatoren"`
	Werte        []string   `json:"werte"`
	Pruefsummen  []string   `json:"pruefsummen"`
	Wurzel       string     `json:"wurzel"`
	Pfade        [][]string `json:"pfade"`
	Anteile      [][]string `json:"anteile"`
	Blockhash    string     `json:"blockhash"`
	AnfrageID    string     `json:"anfrage_id"`
	Stellen      []int      `json:"stellen"`
	Aufforderung []int      `json:"aufforderung"`
	Antwort      []int      `json:"antwort"`
	Treffer      int        `json:"treffer"`
	Fehler       int        `json:"fehler"`
	Urteil       *bool      `json:"urteil"`
	Bestaetigung string     `json:"bestaetigung"`
	F2Auf        []int      `json:"fall2_aufforderung"`
	F2Ein        []int      `json:"fall2_antwort_1fehler"`
	F2Zwei       []int      `json:"fall2_antwort_2fehler"`
}

func qwLaden(t *testing.T) qwVektor {
	t.Helper()
	roh, err := os.ReadFile("testdata/quanten_w_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v qwVektor
	if err := json.Unmarshal(roh, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func qwH32(t *testing.T, s string) [32]byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		t.Fatalf("hex %q", s)
	}
	var out [32]byte
	copy(out[:], b)
	return out
}

func qwBig(s string) *big.Int {
	v, _ := new(big.Int).SetString(s, 10)
	return v
}

// qwOffen: die Offenlegung der Validatoren mit Index (1-basiert) in wer.
func qwOffen(t *testing.T, v qwVektor, stellen []int, wer ...int) []quantenWOffenStelle {
	t.Helper()
	var out []quantenWOffenStelle
	for _, i := range stellen {
		var o quantenWOffenStelle
		for _, x := range wer {
			o.Anteile = append(o.Anteile, quantenWAnteil{X: x, Y: qwBig(v.Anteile[i][x-1])})
		}
		for _, p := range v.Pfade[i] {
			o.Pfad = append(o.Pfad, qwH32(t, p))
		}
		out = append(out, o)
	}
	return out
}

func TestQuantenW_GleichWieSpezifikation(t *testing.T) {
	v := qwLaden(t)
	var blaetter [][32]byte
	for i, w := range v.Werte {
		c := quantenWPruefsumme(v.K, i, qwBig(w))
		if hex.EncodeToString(c[:]) != v.Pruefsummen[i] {
			t.Fatalf("Pruefsumme %d: %x, Spezifikation %s", i, c, v.Pruefsummen[i])
		}
		blaetter = append(blaetter, c)
	}
	wurzel := quantenWMerkleWurzel(blaetter)
	if hex.EncodeToString(wurzel[:]) != v.Wurzel {
		t.Fatalf("Wurzel %x, Spezifikation %s", wurzel, v.Wurzel)
	}
	for i := range blaetter {
		pfad := quantenWMerklePfad(blaetter, i)
		if len(pfad) != len(v.Pfade[i]) || !quantenWMerklePruefen(wurzel, blaetter[i], i, pfad, v.N) {
			t.Fatalf("Pfad %d", i)
		}
		for j := range pfad {
			if hex.EncodeToString(pfad[j][:]) != v.Pfade[i][j] {
				t.Fatalf("Pfad %d/%d anders als die Spezifikation", i, j)
			}
		}
	}
	// Jede Wahl von t Validatoren setzt denselben Wert zusammen.
	for i, w := range v.Werte {
		for _, wer := range [][]int{{1, 2, 3}, {2, 4, 5}, {5, 1, 3}} {
			var a []quantenWAnteil
			for _, x := range wer {
				a = append(a, quantenWAnteil{X: x, Y: qwBig(v.Anteile[i][x-1])})
			}
			if quantenWZusammensetzen(a).Cmp(qwBig(w)) != 0 {
				t.Fatalf("Stelle %d, Validatoren %v: falsch zusammengesetzt", i, wer)
			}
		}
	}
	// Aufforderung aus dem Blockhash: dieselben Stellen und Basen.
	bh, _ := hex.DecodeString(v.Blockhash)
	frei := make([]int, v.N)
	for i := range frei {
		frei[i] = i
	}
	st, ba := quantenWAufforderung(v.K, v.AnfrageID, bh, frei, len(v.Stellen))
	for i := range st {
		if st[i] != v.Stellen[i] || ba[i] != v.Aufforderung[i] {
			t.Fatalf("Aufforderung %v %v, Spezifikation %v %v", st, ba, v.Stellen, v.Aufforderung)
		}
	}
	// Auswerten und Urteil wie die Spezifikation.
	tr, fe, err := quantenWAuswerten(v.K, v.N, v.T, len(v.Validatoren), wurzel, v.Stellen, v.Aufforderung, v.Antwort, qwOffen(t, v, v.Stellen, 1, 2, 3))
	if err != nil || tr != v.Treffer || fe != v.Fehler {
		t.Fatalf("Auswerten %d/%d (%v), Spezifikation %d/%d", tr, fe, err, v.Treffer, v.Fehler)
	}
	u := quantenWUrteil(tr, fe)
	if (v.Urteil == nil) != (u == quantenWUnentschieden) || (v.Urteil != nil && *v.Urteil != (u == quantenWEcht)) {
		t.Fatalf("Urteil %v, Spezifikation %v", u, v.Urteil)
	}
	// Fall 2: alle Stellen, 10 Treffer -- 1 Fehler echt, 2 Fehler abgewiesen.
	alle := frei
	for _, c := range []struct {
		antwort []int
		soll    quantenWErgebnis
	}{{v.F2Ein, quantenWEcht}, {v.F2Zwei, quantenWAbgewiesen}} {
		tr, fe, err := quantenWAuswerten(v.K, v.N, v.T, 5, wurzel, alle, v.F2Auf, c.antwort, qwOffen(t, v, alle, 2, 3, 5))
		if err != nil || quantenWUrteil(tr, fe) != c.soll {
			t.Fatalf("Fall 2: %d Treffer, %d Fehler -> %v, soll %v (%v)", tr, fe, quantenWUrteil(tr, fe), c.soll, err)
		}
	}
	// Bestaetigungssatz der Validatoren wie die Spezifikation.
	if quantenWBestaetigungNachricht(v.K, wurzel, v.N, v.T, v.Validatoren) != v.Bestaetigung {
		t.Fatalf("Bestaetigung %q", quantenWBestaetigungNachricht(v.K, wurzel, v.N, v.T, v.Validatoren))
	}
}

// Missbrauch der Offenlegung: falscher Anteil, zu wenige, doppeltes oder
// fremdes x, falscher Pfad, fehlende Stelle, Wert ausserhalb des Koerpers.
func TestQuantenW_OffenlegenMissbrauch(t *testing.T) {
	v := qwLaden(t)
	w := qwH32(t, v.Wurzel)
	gut := qwOffen(t, v, v.Stellen, 1, 2, 3)
	auswerten := func(o []quantenWOffenStelle) error {
		_, _, err := quantenWAuswerten(v.K, v.N, v.T, 5, w, v.Stellen, v.Aufforderung, v.Antwort, o)
		return err
	}
	if err := auswerten(gut); err != nil {
		t.Fatalf("gute Offenlegung abgewiesen: %v", err)
	}
	kopie := func() []quantenWOffenStelle { return qwOffen(t, v, v.Stellen, 1, 2, 3) }
	faelle := map[string]func() []quantenWOffenStelle{
		"falscher Anteil": func() []quantenWOffenStelle {
			o := kopie()
			o[0].Anteile[0].Y = new(big.Int).Add(o[0].Anteile[0].Y, big.NewInt(1))
			return o
		},
		"zu wenige": func() []quantenWOffenStelle { o := kopie(); o[0].Anteile = o[0].Anteile[:2]; return o },
		"doppeltes x": func() []quantenWOffenStelle {
			o := kopie()
			o[0].Anteile[1] = o[0].Anteile[0]
			return o
		},
		"fremdes x": func() []quantenWOffenStelle { o := kopie(); o[0].Anteile[0].X = 9; return o },
		"x null":    func() []quantenWOffenStelle { o := kopie(); o[0].Anteile[0].X = 0; return o },
		"falscher Pfad": func() []quantenWOffenStelle {
			o := kopie()
			o[1].Pfad[0], o[1].Pfad[1] = o[1].Pfad[1], o[1].Pfad[0]
			return o
		},
		"kurzer Pfad":   func() []quantenWOffenStelle { o := kopie(); o[0].Pfad = o[0].Pfad[1:]; return o },
		"Stelle fehlt":  func() []quantenWOffenStelle { return kopie()[1:] },
		"Wert zu gross": func() []quantenWOffenStelle { o := kopie(); o[0].Anteile[0].Y = new(big.Int).Set(quantenWP); return o },
		"Wert negativ":  func() []quantenWOffenStelle { o := kopie(); o[0].Anteile[0].Y = big.NewInt(-1); return o },
		"Anteil fehlt":  func() []quantenWOffenStelle { o := kopie(); o[0].Anteile[0].Y = nil; return o },
	}
	for name, f := range faelle {
		if err := auswerten(f()); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
}

// Die Uebergaenge wie KetteW in der Spezifikation.
func TestQuantenW_Uebergaenge(t *testing.T) {
	v := qwLaden(t)
	neu := func() *quantenWNote {
		return &quantenWNote{N: v.N, Wurzel: qwH32(t, v.Wurzel), Wert: 5_000_000, Validatoren: v.Validatoren, T: v.T}
	}
	bh, _ := hex.DecodeString(v.Blockhash)

	// Praegen: Form.
	if err := quantenWPraegenPruefen(false, v.K, v.N, v.T, v.Validatoren, 3); err != nil {
		t.Fatalf("gute Note abgewiesen: %v", err)
	}
	for name, f := range map[string]func() error{
		"doppelt":             func() error { return quantenWPraegenPruefen(true, v.K, v.N, v.T, v.Validatoren, 3) },
		"K gross":             func() error { return quantenWPraegenPruefen(false, "AB"+v.K[2:], v.N, v.T, v.Validatoren, 3) },
		"zu gross":            func() error { return quantenWPraegenPruefen(false, v.K, quantenWNMax+1, v.T, v.Validatoren, 3) },
		"zu klein":            func() error { return quantenWPraegenPruefen(false, v.K, quantenWKMin-1, v.T, v.Validatoren, 3) },
		"t ueber v":           func() error { return quantenWPraegenPruefen(false, v.K, v.N, 6, v.Validatoren, 6) },
		"Validator doppelt":   func() error { return quantenWPraegenPruefen(false, v.K, v.N, 3, []string{"a", "a", "b"}, 3) },
		"zu wenig bestaetigt": func() error { return quantenWPraegenPruefen(false, v.K, v.N, v.T, v.Validatoren, 2) },
	} {
		if f() == nil {
			t.Errorf("Praegen %s: angenommen", name)
		}
	}
	if quantenWPraegenPruefen(false, v.K, quantenWKMin, v.T, v.Validatoren, 3) != nil {
		t.Error("Gegenprobe: kleinste pruefbare Note abgewiesen")
	}

	// Gutfall wie im Vektor: anfragen in Block 10 (noch keine Stellen),
	// ziehen in Block 11 aus dem Hash des Anfrage-Blocks (dann verbraucht),
	// antworten, offenlegen -> wie die Spezifikation, Pfand zurueck.
	e := neu()
	if err := e.anfragen(v.K, "anna", len(v.Stellen), "", 10); err != nil {
		t.Fatal(err)
	}
	if len(e.benutztListe()) != 0 || e.Anfrage.Stellen != nil {
		t.Fatal("Stellen schon beim Anfragen festgelegt")
	}
	if e.anfragen(v.K, "anna", 8, "", 10) == nil {
		t.Error("zweite offene Anfrage angenommen")
	}
	if e.antworten("anna", v.Antwort, 11) == nil || e.antworten("anna", []int{}, 11) == nil {
		t.Error("Antwort vor dem Ziehen angenommen")
	}
	if e.ziehen(v.K, 10, bh) == nil || e.Anfrage.Stellen != nil {
		t.Error("im Block der Anfrage gezogen")
	}
	if err := e.ziehen(v.K, 11, bh); err != nil {
		t.Fatal(err)
	}
	if got := e.benutztListe(); len(got) != len(v.Stellen) || e.Anfrage.Stellen[0] != v.Stellen[0] || e.Anfrage.Basen[0] != v.Aufforderung[0] {
		t.Fatalf("gezogen/verbraucht %v, Spezifikation %v", got, v.Stellen)
	}
	vorher := append([]int(nil), e.Anfrage.Stellen...)
	if err := e.ziehen(v.K, 12, []byte("anderer Hash")); err != nil || e.Anfrage.Stellen[0] != vorher[0] {
		t.Error("zweites Ziehen aendert die Aufforderung")
	}
	if e.antworten("anna", v.Antwort, 10) == nil {
		t.Error("Antwort im selben Block angenommen")
	}
	if e.antworten("fremd", v.Antwort, 11) == nil {
		t.Error("fremde Antwort angenommen")
	}
	if e.antworten("anna", append([]int{2}, v.Antwort[1:]...), 11) == nil {
		t.Error("Bit 2 angenommen")
	}
	if e.antworten("anna", v.Antwort[1:], 11) == nil {
		t.Error("kurze Antwort angenommen")
	}
	if e.antworten("anna", v.Antwort, 10+quantenWFristAntwort+1) == nil {
		t.Error("Antwort nach der Frist angenommen (ohne frist())")
	}
	if _, _, err := e.offenlegen(v.K, qwOffen(t, v, v.Stellen, 1, 2, 3)); err == nil {
		t.Error("Offenlegen vor der Antwort angenommen")
	}
	if err := e.antworten("anna", v.Antwort, 11); err != nil {
		t.Fatal(err)
	}
	if e.antworten("anna", v.Antwort, 12) == nil {
		t.Error("Antwort wiederholt")
	}
	u, b, err := e.offenlegen(v.K, qwOffen(t, v, v.Stellen, 1, 2, 3))
	soll := quantenWUrteil(v.Treffer, v.Fehler)
	if err != nil || u != soll || e.Fehl != 0 || e.Anfrage != nil {
		t.Fatalf("Urteil: %v (soll %v) %+v %v fehl=%d", u, soll, b, err, e.Fehl)
	}
	if soll != quantenWAbgewiesen && b.PfandZurueckAn != "anna" {
		t.Fatalf("Pfand nicht zurueck: %+v", b)
	}

	// Frist: keine Antwort -> Pfand verfaellt, Fehlversuch; nach 3 gesperrt;
	// gesperrt kann noch eingeloest werden; Einloesen ohne Antwort -> tot.
	e = &quantenWNote{N: 64, Wurzel: qwH32(t, v.Wurzel), Wert: 5_000_000, Validatoren: v.Validatoren, T: v.T}
	for j := 0; j < quantenWFehlMax; j++ {
		h := int64(100 * j)
		if err := e.anfragen(v.K, "fremd", 8, "", h); err != nil {
			t.Fatal(err)
		}
		if err := e.ziehen(v.K, h+1, bh); err != nil {
			t.Fatal(err)
		}
		if ab, _ := e.frist(h + quantenWFristAntwort); ab {
			t.Fatal("Frist zu frueh")
		}
		ab, b := e.frist(h + quantenWFristAntwort + 1)
		if !ab || !b.PfandVerfaellt {
			t.Fatalf("Frist %d: %v %+v", j, ab, b)
		}
	}
	if len(e.benutztListe()) != 8*quantenWFehlMax {
		t.Fatalf("verfallene Anfragen haben %d Stellen verbraucht", len(e.benutztListe()))
	}
	if !e.Gesperrt || e.anfragen(v.K, "fremd", 8, "", 1000) == nil {
		t.Fatal("nach 3 Fehlversuchen nicht gesperrt")
	}
	if err := e.anfragen(v.K, "anna", 8, "anna", 1000); err != nil {
		t.Fatalf("gesperrte Note nicht einloesbar: %v", err)
	}
	if err := e.ziehen(v.K, 1001, bh); err != nil {
		t.Fatal(err)
	}
	ab, b2 := e.frist(1000 + quantenWFristAntwort + 1)
	if !ab || !b2.WertVerfaellt || !e.Tot {
		t.Fatalf("Einloesen ohne Antwort: %+v tot=%v", b2, e.Tot)
	}
	if e.anfragen(v.K, "anna", 8, "anna", 2000) == nil {
		t.Error("tote Note angefragt")
	}
	// Mehr Stellen als frei, zu wenige Stellen.
	e = &quantenWNote{N: 10, Wurzel: qwH32(t, v.Wurzel), Validatoren: v.Validatoren, T: v.T}
	if e.anfragen(v.K, "anna", 11, "", 1) == nil || e.anfragen(v.K, "anna", 7, "", 1) == nil {
		t.Error("k ausserhalb der Grenzen angenommen")
	}
	if st, ba := quantenWAufforderung(v.K, "x", bh, []int{1, 2}, 3); st != nil || ba != nil {
		t.Error("Aufforderung mit k > frei")
	}
	// Nie gezogen (Fehler der Einbindung): Pfand zurueck, kein Fehlversuch.
	e = neu()
	if err := e.anfragen(v.K, "anna", 8, "", 1); err != nil {
		t.Fatal(err)
	}
	if ab, b := e.frist(1 + quantenWFristAntwort + 1); !ab || b.PfandZurueckAn != "anna" || b.PfandVerfaellt || e.Fehl != 0 || len(e.benutztListe()) != 0 {
		t.Fatalf("nie gezogen: %v %+v fehl=%d", ab, b, e.Fehl)
	}
	// Validatoren legen nicht offen: Pfand zurueck, kein Fehlversuch.
	e = neu()
	if err := e.anfragen(v.K, "anna", 8, "", 1); err != nil {
		t.Fatal(err)
	}
	if err := e.ziehen(v.K, 2, bh); err != nil {
		t.Fatal(err)
	}
	if err := e.antworten("anna", make([]int, 8), 2); err != nil {
		t.Fatal(err)
	}
	if ab, _ := e.frist(2 + quantenWFristOffen); ab {
		t.Fatal("Offenlegungsfrist zu frueh")
	}
	if ab, b := e.frist(2 + quantenWFristOffen + 1); !ab || b.PfandZurueckAn != "anna" || e.Fehl != 0 {
		t.Fatalf("Offenlegungsfrist: %v %+v", ab, b)
	}
}
