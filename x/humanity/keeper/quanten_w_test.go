package keeper

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"math/big"
	"math/rand"
	"os"
	"reflect"
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
	Anker        []string   `json:"anker"`
	Zufall0      []string   `json:"zufall0"`
	Zufall3      []string   `json:"zufall3"`
	Saat         string     `json:"saat"`
	AnfrageID    string     `json:"anfrage_id"`
	Stellen      []int      `json:"stellen"`
	Aufforderung []int      `json:"aufforderung"`
	Antwort      []int      `json:"antwort"`
	Treffer      int        `json:"treffer"`
	Fehler       int        `json:"fehler"`
	Urteil       *bool      `json:"urteil"`
	Bestaetigung string     `json:"bestaetigung2"`
	Pfand        [][4]int64 `json:"pfand"`
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
	// Hash-Ketten: jeder Wert fuehrt zum Anker, Anfrage 3 zu Anfrage 0; die
	// Saat aus den Werten aller Validatoren wie die Spezifikation.
	var werte0 [][32]byte
	for x := range v.Anker {
		a, z0, z3 := qwH32(t, v.Anker[x]), qwH32(t, v.Zufall0[x]), qwH32(t, v.Zufall3[x])
		if quantenWKetteVor(z0, 1) != a || quantenWKetteVor(z3, 4) != a || quantenWKetteVor(z3, 3) != z0 || quantenWKetteVor(z3, 2) == z0 {
			t.Fatalf("Hash-Kette von Validator %d anders als die Spezifikation", x+1)
		}
		werte0 = append(werte0, z0)
	}
	saat := quantenWZufallSaat(v.K, 0, werte0)
	if hex.EncodeToString(saat[:]) != v.Saat {
		t.Fatalf("Saat %x, Spezifikation %s", saat, v.Saat)
	}
	// Aufforderung aus der Saat: dieselben Stellen und Basen.
	frei := make([]int, v.N)
	for i := range frei {
		frei[i] = i
	}
	st, ba := quantenWAufforderung(v.K, v.AnfrageID, saat[:], frei, len(v.Stellen))
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
	// Bestaetigungssatz von Validator 2 (mit seinem Anker) wie die Spezifikation.
	if b := quantenWBestaetigungNachricht(v.K, wurzel, v.N, v.T, v.Validatoren, 2, qwH32(t, v.Anker[1])); b != v.Bestaetigung {
		t.Fatalf("Bestaetigung %q, Spezifikation %q", b, v.Bestaetigung)
	}
	// Pfand wie die Spezifikation (auch nahe am Ueberlauf).
	for _, p := range v.Pfand {
		if got := quantenWPfandFuer(p[0], int(p[1]), int(p[2])); got != p[3] {
			t.Fatalf("Pfand(%d, %d, %d) = %d, Spezifikation %d", p[0], p[1], p[2], got, p[3])
		}
	}
	if quantenWPfandFuer(math.MaxInt64, 64, 8) != math.MaxInt64 || quantenWPfandFuer(0, 64, 8) != quantenWPfand {
		t.Fatal("Pfand am Rand")
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

// ------------------------------------------------------------ Uebergaenge
//
// Wie KetteW in der Spezifikation, mit Noten, die der Test selbst praegt
// (Werte, Anteile, Merkle, Hash-Ketten) -- der Vektor hat nur 10 Stellen.

type qwPraegung struct {
	K           string
	n, t, v     int
	basen, bits []int
	blaetter    [][32]byte
	wurzel      [32]byte
	anteile     [][]*big.Int // [Stelle][Validator]
	geheim      [][32]byte
	anker       [][32]byte
}

func qwPraegen(n, t, v int, seed int64) *qwPraegung {
	r := rand.New(rand.NewSource(seed))
	p := &qwPraegung{n: n, t: t, v: v}
	k := sha256.Sum256([]byte{byte(seed), byte(seed >> 8), byte(n), byte(n >> 8)})
	p.K = hex.EncodeToString(k[:])
	salzGrenze := new(big.Int).Lsh(big.NewInt(1), quantenWSalzBits)
	for i := 0; i < n; i++ {
		b, x := r.Intn(2), r.Intn(2)
		p.basen, p.bits = append(p.basen, b), append(p.bits, x)
		w := new(big.Int).Rand(r, salzGrenze)
		w.Lsh(w, 2).Add(w, big.NewInt(int64(b+2*x)))
		p.blaetter = append(p.blaetter, quantenWPruefsumme(p.K, i, w))
		koeff := []*big.Int{w}
		for j := 1; j < t; j++ {
			koeff = append(koeff, new(big.Int).Rand(r, quantenWP))
		}
		var je []*big.Int
		for x := 1; x <= v; x++ {
			y := new(big.Int)
			for j := len(koeff) - 1; j >= 0; j-- {
				y.Mul(y, big.NewInt(int64(x))).Add(y, koeff[j]).Mod(y, quantenWP)
			}
			je = append(je, y)
		}
		p.anteile = append(p.anteile, je)
	}
	p.wurzel = quantenWMerkleWurzel(p.blaetter)
	for x := 0; x < v; x++ {
		var g [32]byte
		r.Read(g[:])
		p.geheim = append(p.geheim, g)
		p.anker = append(p.anker, quantenWKetteVor(g, quantenWZufallL))
	}
	return p
}

func (p *qwPraegung) validatoren() []string {
	var out []string
	for x := 1; x <= p.v; x++ {
		out = append(out, "v"+string(rune('0'+x)))
	}
	return out
}

func (p *qwPraegung) note() *quantenWNote {
	return neueQuantenWNote(p.n, p.t, p.wurzel, p.validatoren(), p.anker)
}

// zufall: die Werte fuer Anfrage j (Validatoren 0-basiert; ohne Angabe alle).
func (p *qwPraegung) zufall(j int, wer ...int) map[int][32]byte {
	if len(wer) == 0 {
		for x := 0; x < p.v; x++ {
			wer = append(wer, x)
		}
	}
	out := map[int][32]byte{}
	for _, x := range wer {
		out[x] = quantenWKetteVor(p.geheim[x], quantenWZufallL-1-j)
	}
	return out
}

func (p *qwPraegung) saat(j int) [32]byte {
	var w [][32]byte
	for x := 0; x < p.v; x++ {
		w = append(w, quantenWKetteVor(p.geheim[x], quantenWZufallL-1-j))
	}
	return quantenWZufallSaat(p.K, j, w)
}

// offen: die Validatoren wer (1-basiert) legen ihre Anteile offen.
func (p *qwPraegung) offen(stellen []int, wer ...int) []quantenWOffenStelle {
	var out []quantenWOffenStelle
	for _, i := range stellen {
		o := quantenWOffenStelle{Pfad: quantenWMerklePfad(p.blaetter, i)}
		for _, x := range wer {
			o.Anteile = append(o.Anteile, quantenWAnteil{X: x, Y: p.anteile[i][x-1]})
		}
		out = append(out, o)
	}
	return out
}

func (p *qwPraegung) ehrlich(stellen, basen []int) []int {
	out := make([]int, len(stellen))
	for j, i := range stellen {
		if basen[j] == p.basen[i] {
			out[j] = p.bits[i]
		}
	}
	return out
}

func qwRaten(r *rand.Rand, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = r.Intn(2)
	}
	return out
}

// qwRunde: anfragen, Zufall, antworten, offenlegen. Gibt Urteil und alle
// Buchungen der Runde zurueck.
func qwRunde(t *testing.T, p *qwPraegung, e *quantenWNote, von string, k int, einloesenAn string, antwort func(st, ba []int) []int, zeit int64) (quantenWErgebnis, quantenWBuchung) {
	t.Helper()
	if einloesenAn != "" {
		k = min(quantenWKMax, e.N-len(e.Benutzt))
	}
	if err := e.anfragen(von, k, einloesenAn, quantenWPfandFuer(5_000_000, k, e.N), zeit); err != nil {
		t.Fatalf("anfragen: %v", err)
	}
	j := e.Anfrage.Index
	if g, _, err := e.zufall(p.K, j, p.zufall(j), zeit+5); err != nil || !g {
		t.Fatalf("zufall: %v %v", g, err)
	}
	a := e.Anfrage
	if err := e.antworten(von, antwort(a.Stellen, a.Basen), zeit+10); err != nil {
		t.Fatalf("antworten: %v", err)
	}
	u, b, err := e.offenlegen(p.K, p.offen(a.Stellen, 1, 2, 3))
	if err != nil {
		t.Fatalf("offenlegen: %v", err)
	}
	return u, b
}

func TestQuantenW_PraegenForm(t *testing.T) {
	val := []string{"a", "b", "c", "d", "e"}
	K := "ab" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcd"
	if err := quantenWPraegenPruefen(false, K, 200, 3, val, 5, 5); err != nil {
		t.Fatalf("gute Note abgewiesen: %v", err)
	}
	for name, f := range map[string]func() error{
		"doppelt":                  func() error { return quantenWPraegenPruefen(true, K, 200, 3, val, 5, 5) },
		"K gross":                  func() error { return quantenWPraegenPruefen(false, "AB"+K[2:], 200, 3, val, 5, 5) },
		"zu gross":                 func() error { return quantenWPraegenPruefen(false, K, quantenWNMax+1, 3, val, 5, 5) },
		"zu klein":                 func() error { return quantenWPraegenPruefen(false, K, quantenWKMin-1, 3, val, 5, 5) },
		"t ueber v":                func() error { return quantenWPraegenPruefen(false, K, 200, 6, val, 5, 5) },
		"Validator doppelt":        func() error { return quantenWPraegenPruefen(false, K, 200, 2, []string{"a", "a", "b"}, 3, 3) },
		"Validator leer":           func() error { return quantenWPraegenPruefen(false, K, 200, 2, []string{"a", "", "b"}, 3, 3) },
		"ein Validator fehlt":      func() error { return quantenWPraegenPruefen(false, K, 200, 3, val, 5, 4) },
		"Anker fehlt":              func() error { return quantenWPraegenPruefen(false, K, 200, 3, val, 4, 5) },
		"t erfuellt, nicht alle v": func() error { return quantenWPraegenPruefen(false, K, 200, 3, val, 5, 3) },
	} {
		if f() == nil {
			t.Errorf("Praegen %s: angenommen", name)
		}
	}
	if quantenWPraegenPruefen(false, K, quantenWKMin, 3, val, 5, 5) != nil {
		t.Error("Gegenprobe: kleinste pruefbare Note abgewiesen")
	}
}

// Gutfall: pruefen (Pfand zurueck), einloesen mit fester Pruefgroesse (Wert
// an das Ziel, Note tot), danach nichts mehr.
func TestQuantenW_Gutfall(t *testing.T) {
	p := qwPraegen(200, 3, 5, 1)
	e := p.note()
	u, b := qwRunde(t, p, e, "bert", 16, "", p.ehrlich, 100)
	if u != quantenWEcht || b != (quantenWBuchung{PfandZurueckAn: "bert"}) || e.Tot || len(e.Benutzt) != 16 {
		t.Fatalf("Pruefung: %v %+v", u, b)
	}
	u, b = qwRunde(t, p, e, "bert", 0, "carl", p.ehrlich, 200)
	if u != quantenWEcht || b != (quantenWBuchung{PfandZurueckAn: "bert", WertAn: "carl"}) || !e.Tot || len(e.Benutzt) != 16+quantenWKMax {
		t.Fatalf("Einloesen: %v %+v", u, b)
	}
	if e.anfragen("bert", quantenWKMax, "bert", quantenWPfand, 300) == nil {
		t.Fatal("zweimal eingeloest")
	}
}

// Zufall: zu frueh, andere Anfrage, fremder oder falscher Wert, ein falscher
// unter richtigen (nichts geaendert), Teil-Zufall zieht nicht, Wiederholung,
// Antwort vor dem Zufall; die Aufforderung entsteht genau aus der Saat aller.
func TestQuantenW_ZufallMissbrauch(t *testing.T) {
	p := qwPraegen(200, 3, 5, 2)
	e := p.note()
	if _, _, err := e.zufall(p.K, 0, p.zufall(0), 1); err == nil {
		t.Fatal("Zufall vor der Anfrage angenommen")
	}
	if err := e.anfragen("anna", 16, "", quantenWPfand, 10); err != nil {
		t.Fatal(err)
	}
	falsch := func(x int, w [32]byte) map[int][32]byte { return map[int][32]byte{x: w} }
	for name, w := range map[string]map[int][32]byte{
		"andere Anfrage":          p.zufall(1),
		"Wert eines anderen":      falsch(0, p.zufall(0)[1]),
		"falsche Stelle":          falsch(0, p.zufall(1)[0]),
		"fremder Validator":       falsch(5, p.zufall(0)[0]),
		"negativer Validator":     falsch(-1, p.zufall(0)[0]),
		"der Anker selbst":        falsch(0, p.anker[0]),
		"leer":                    {},
		"einer falsch unter vier": {0: p.zufall(0)[0], 1: p.zufall(0)[1], 2: p.zufall(0)[2], 3: [32]byte{7}},
	} {
		j := 0
		if name == "andere Anfrage" {
			j = 1
		}
		if _, _, err := e.zufall(p.K, j, w, 11); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
	// Alles oder nichts: die Reihenfolge einer Map ist zufaellig, darum oft
	// genug, dass der falsche Wert auch nach den richtigen geprueft wird.
	for i := 0; i < 64; i++ {
		w := p.zufall(0, 0, 1, 2)
		w[3] = [32]byte{byte(i)}
		if _, _, err := e.zufall(p.K, 0, w, 11); err == nil {
			t.Fatal("einer falsch unter vier: angenommen")
		}
	}
	if !reflect.DeepEqual(e.ZIndex, []int{-1, -1, -1, -1, -1}) {
		t.Fatalf("abgewiesener Zufall hat etwas geaendert: %v", e.ZIndex)
	}
	if g, _, err := e.zufall(p.K, 0, p.zufall(0, 0, 1, 2, 3), 12); err != nil || g || e.Anfrage.Basen != nil || len(e.Benutzt) != 0 {
		t.Fatalf("Teil-Zufall: %v %v", g, err)
	}
	if _, _, err := e.zufall(p.K, 0, p.zufall(0, 0), 13); err == nil {
		t.Fatal("Zufall wiederholt angenommen")
	}
	if e.antworten("anna", make([]int, 16), 13) == nil {
		t.Fatal("Antwort vor dem Zufall angenommen")
	}
	if g, _, err := e.zufall(p.K, 0, p.zufall(0, 4), 14); err != nil || !g {
		t.Fatalf("voller Zufall zieht nicht: %v", err)
	}
	frei := make([]int, 200)
	for i := range frei {
		frei[i] = i
	}
	saat := p.saat(0)
	st, ba := quantenWAufforderung(p.K, p.K+":0", saat[:], frei, 16)
	if !reflect.DeepEqual(st, e.Anfrage.Stellen) || !reflect.DeepEqual(ba, e.Anfrage.Basen) || !reflect.DeepEqual(e.benutztListe(), st) {
		t.Fatal("Aufforderung nicht aus den Hash-Ketten aller Validatoren")
	}
	if _, _, err := e.zufall(p.K, 0, p.zufall(0), 15); err == nil {
		t.Fatal("Zufall nach dem Ziehen angenommen")
	}
	// Antwort: fremd, Form, Wiederholung.
	gut := p.ehrlich(e.Anfrage.Stellen, e.Anfrage.Basen)
	if e.antworten("fremd", gut, 16) == nil || e.antworten("anna", gut[1:], 16) == nil || e.antworten("anna", append([]int{2}, gut[1:]...), 16) == nil {
		t.Fatal("falsche Antwort angenommen")
	}
	if _, _, err := e.offenlegen(p.K, p.offen(st, 1, 2, 3)); err == nil {
		t.Fatal("Offenlegen vor der Antwort angenommen")
	}
	if err := e.antworten("anna", gut, 16); err != nil {
		t.Fatal(err)
	}
	if e.antworten("anna", gut, 17) == nil {
		t.Fatal("Antwort wiederholt angenommen")
	}
	if u, b, err := e.offenlegen(p.K, p.offen(st, 2, 4, 5)); err != nil || u != quantenWEcht || b.PfandZurueckAn != "anna" {
		t.Fatalf("Offenlegen: %v %+v %v", u, b, err)
	}
}

// Einloesen mit fester Pruefgroesse; Pruefen laesst die Reserve stehen; zu
// kleines Pfand; Ende der Hash-Ketten.
func TestQuantenW_PruefgroesseFest(t *testing.T) {
	p := qwPraegen(200, 3, 5, 3)
	e := p.note()
	for name, f := range map[string]func() error{
		"Einloesen k = 8":  func() error { return e.anfragen("fremd", 8, "fremd", quantenWPfand, 1) },
		"Einloesen k = 65": func() error { return e.anfragen("fremd", 65, "fremd", quantenWPfand*2, 1) },
		"Pruefung k = 65":  func() error { return e.anfragen("bert", 65, "", quantenWPfand*2, 1) },
		"Pruefung k = 7":   func() error { return e.anfragen("bert", 7, "", quantenWPfand, 1) },
		"Pruefung frisst die Reserve": func() error {
			e.Benutzt = map[int]bool{}
			return qwVerbraucht(e, 130).anfragen("bert", 8, "", quantenWPfand, 1)
		},
		"Pfand zu klein": func() error { e.Benutzt = nil; return e.anfragen("bert", 8, "", quantenWPfand-1, 1) },
		"Hash-Ketten verbraucht": func() error {
			e.Benutzt = nil
			e.Zaehl = quantenWZufallL
			return e.anfragen("bert", 8, "", quantenWPfand, 1)
		},
	} {
		if err := f(); err == nil {
			t.Errorf("%s: angenommen", name)
		}
		e.Anfrage, e.Zaehl = nil, 0
	}
	// Gegenprobe am Rand: genau die Reserve bleibt.
	e.Benutzt = nil
	if err := qwVerbraucht(e, 128).anfragen("bert", 8, "", quantenWPfand, 1); err != nil {
		t.Fatalf("Pruefung bis an die Reserve abgewiesen: %v", err)
	}
	e.Anfrage = nil
	if err := e.anfragen("bert", quantenWKMax, "bert", quantenWPfand, 1); err != nil {
		t.Fatalf("Einloesen mit k = K_MAX abgewiesen: %v", err)
	}
}

func qwVerbraucht(e *quantenWNote, m int) *quantenWNote {
	for i := 0; i < m; i++ {
		e.verbrauchen([]int{i})
	}
	return e
}

// Ein Faelscher, der einloesen will, toetet die Note nicht: Pfand weg,
// Fehlversuch, der echte Halter loest danach ein. Drei Fehlversuche sperren
// Pruefungen, nicht das Einloesen.
func TestQuantenW_FaelscherToetetNicht(t *testing.T) {
	r := rand.New(rand.NewSource(9))
	raten := func(st, _ []int) []int { return qwRaten(r, len(st)) }
	p := qwPraegen(400, 3, 5, 4)
	e := p.note()
	u, b := qwRunde(t, p, e, "fremd", 0, "fremd", raten, 10)
	if u != quantenWAbgewiesen || b != (quantenWBuchung{PfandVerfaellt: true}) || e.Tot || e.Fehl != 1 {
		t.Fatalf("Faelscher-Einloesen: %v %+v tot=%v", u, b, e.Tot)
	}
	for i := 0; i < 2; i++ {
		if u, _ := qwRunde(t, p, e, "fremd", quantenWKMax, "", raten, int64(100+100*i)); u != quantenWAbgewiesen {
			t.Fatalf("geratene Pruefung %d: %v", i, u)
		}
	}
	if !e.Gesperrt || e.anfragen("bert", 8, "", quantenWPfand, 400) == nil {
		t.Fatal("nach drei Fehlversuchen nicht gesperrt")
	}
	u, b = qwRunde(t, p, e, "anna", 0, "anna", p.ehrlich, 500)
	if u != quantenWEcht || b.WertAn != "anna" || !e.Tot {
		t.Fatalf("Halter nach Faelscher-Versuchen: %v %+v", u, b)
	}
}

// Erschoepfen: 72 Stellen; eine Pruefung (8), ein falsches Einloesen (64)
// -> keine Stelle frei, tot, Wert ans Grundeinkommen.
func TestQuantenW_Erschoepft(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	raten := func(st, _ []int) []int { return qwRaten(r, len(st)) }
	p := qwPraegen(72, 3, 5, 5)
	e := p.note()
	if e.anfragen("fremd", 9, "", quantenWPfand, 1) == nil {
		t.Fatal("Pruefung frisst die Reserve")
	}
	qwRunde(t, p, e, "fremd", 8, "", raten, 10)
	if e.Tot {
		t.Fatal("nach einer Pruefung tot")
	}
	u, b := qwRunde(t, p, e, "fremd", 0, "fremd", raten, 100)
	if u != quantenWAbgewiesen || b != (quantenWBuchung{PfandVerfaellt: true, WertVerfaellt: true}) || !e.Tot || len(e.Benutzt) != 72 {
		t.Fatalf("erschoepft: %v %+v tot=%v frei=%d", u, b, e.Tot, 72-len(e.Benutzt))
	}
	if quantenWPfandFuer(5_000_000, quantenWKMax, 72) < 4_000_000 {
		t.Fatal("Erschoepfen ist billiger als der Wert")
	}
}

// Fristen: Zufall zurueckgehalten -> Pfand zurueck, Note wartet, keine neue
// Anfrage; nachgereicht -> genau die Stellen dieser Anfrage verbraucht.
// Antwort verpasst -> durchgefallen. Offenlegung verpasst -> Pfand zurueck.
func TestQuantenW_Fristen(t *testing.T) {
	p := qwPraegen(200, 3, 5, 6)
	e := p.note()
	if err := e.anfragen("anna", 16, "", quantenWPfand, 100); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.zufall(p.K, 0, p.zufall(0, 0, 1, 2, 3), 101); err != nil {
		t.Fatal(err)
	}
	if ab, _ := e.frist(100 + quantenWFristZufall); ab {
		t.Fatal("Zufallsfrist zu frueh")
	}
	ab, b := e.frist(100 + quantenWFristZufall + 1)
	if !ab || b != (quantenWBuchung{PfandZurueckAn: "anna"}) || e.Fehl != 0 || e.WartetK != 16 || e.Wartet != 0 || len(e.Benutzt) != 0 {
		t.Fatalf("Zufallsfrist: %v %+v", ab, b)
	}
	if e.anfragen("anna", 16, "", quantenWPfand, 800) == nil || e.anfragen("anna", quantenWKMax, "anna", quantenWPfand, 800) == nil {
		t.Fatal("Anfrage angenommen, waehrend die Note wartet")
	}
	if g, _, err := e.zufall(p.K, 0, p.zufall(0, 4), 900); err != nil || !g || e.WartetK != 0 {
		t.Fatalf("nachgereichter Zufall: %v %v", g, err)
	}
	frei := make([]int, 200)
	for i := range frei {
		frei[i] = i
	}
	saat := p.saat(0)
	if st, _ := quantenWAufforderung(p.K, p.K+":0", saat[:], frei, 16); !reflect.DeepEqual(st, e.benutztListe()) {
		t.Fatal("nachgeholt: falsche Stellen verbraucht")
	}
	if u, _ := qwRunde(t, p, e, "bert", 16, "", p.ehrlich, 1000); u != quantenWEcht || e.Zaehl != 2 {
		t.Fatalf("Pruefung nach dem Warten: %v", u)
	}
	// Antwort verpasst: durchgefallen, Stellen verbraucht.
	if err := e.anfragen("fremd", 16, "", quantenWPfand, 2000); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.zufall(p.K, 2, p.zufall(2), 2001); err != nil {
		t.Fatal(err)
	}
	if e.antworten("fremd", make([]int, 16), 2001+quantenWFristAntwort+1) == nil {
		t.Fatal("Antwort nach der Frist angenommen")
	}
	if ab, _ := e.frist(2001 + quantenWFristAntwort); ab {
		t.Fatal("Antwortfrist zu frueh")
	}
	ab, b = e.frist(2001 + quantenWFristAntwort + 1)
	if !ab || b != (quantenWBuchung{PfandVerfaellt: true}) || e.Fehl != 1 || len(e.Benutzt) != 48 {
		t.Fatalf("Antwortfrist: %v %+v", ab, b)
	}
	// Offenlegung verpasst: Pfand zurueck, kein Fehlversuch.
	if err := e.anfragen("anna", 8, "", quantenWPfand, 3000); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.zufall(p.K, 3, p.zufall(3), 3001); err != nil {
		t.Fatal(err)
	}
	if err := e.antworten("anna", make([]int, 8), 3002); err != nil {
		t.Fatal(err)
	}
	if ab, _ := e.frist(3002 + quantenWFristOffen); ab {
		t.Fatal("Offenlegungsfrist zu frueh")
	}
	if ab, b := e.frist(3002 + quantenWFristOffen + 1); !ab || b != (quantenWBuchung{PfandZurueckAn: "anna"}) || e.Fehl != 1 {
		t.Fatalf("Offenlegungsfrist: %v %+v", ab, b)
	}
}

// Unentschieden: zu wenige passende Basen -- Pfand zurueck, kein
// Fehlversuch, die Note bleibt.
func TestQuantenW_Unentschieden(t *testing.T) {
	p := qwPraegen(600, 3, 5, 7)
	e := p.note()
	gesehen := false
	for i := 0; e.N-len(e.Benutzt)-8 >= quantenWReserve(e.N); i++ {
		u, b := qwRunde(t, p, e, "anna", 8, "", p.ehrlich, int64(10+100*i))
		switch u {
		case quantenWUnentschieden:
			gesehen = true
			if b != (quantenWBuchung{PfandZurueckAn: "anna"}) || e.Fehl != 0 || e.Tot {
				t.Fatalf("Unentschieden bestraft: %+v", b)
			}
		case quantenWAbgewiesen:
			t.Fatal("echte Note abgewiesen")
		}
	}
	if !gesehen {
		t.Fatal("kein Fall mit wenigen Treffern erzeugt")
	}
}
