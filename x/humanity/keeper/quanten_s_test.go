package keeper

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"testing"
)

// Variante S (S2a): der Knoten rechnet wie die Spezifikation (s_kette.py,
// Pruefvektor testdata/quanten_s_vektor.json) und weist Missbrauch ab.

type qsVektor struct {
	K           string   `json:"K"`
	N           int      `json:"n"`
	T           int      `json:"t"`
	NVal        int      `json:"n_val"`
	Tol         int      `json:"tol"`
	Wurzel      string   `json:"wurzel"`
	Proben      [][]int  `json:"proben"`
	ProbeHash   []string `json:"probe_hash"`
	D           []int    `json:"d"`
	C           int      `json:"c"`
	B           int      `json:"b"`
	X           []int    `json:"x"`
	XFehler     []int    `json:"x_fehler"`
	VorlageText string   `json:"vorlage_text"`
	OffenAndere []struct {
		Anteile [][2]json.RawMessage `json:"anteile"`
		Pfad    []string             `json:"pfad"`
	} `json:"offen_andere"`
	Offen []struct {
		Anteile [][2]json.RawMessage `json:"anteile"`
		Pfad    []string             `json:"pfad"`
	} `json:"offen"`
	Treffer      int  `json:"treffer"`
	Fehler       int  `json:"fehler"`
	Urteil       bool `json:"urteil"`
	UrteilFehler bool `json:"urteil_fehler"`
}

func qsOffen(t *testing.T, in []struct {
	Anteile [][2]json.RawMessage `json:"anteile"`
	Pfad    []string             `json:"pfad"`
}) []quantenWOffenStelle {
	t.Helper()
	var offen []quantenWOffenStelle
	for _, o := range in {
		var s quantenWOffenStelle
		for _, a := range o.Anteile {
			var x int
			var y string
			json.Unmarshal(a[0], &x)
			json.Unmarshal(a[1], &y)
			yy, _ := new(big.Int).SetString(y, 10)
			s.Anteile = append(s.Anteile, quantenWAnteil{X: x, Y: yy})
		}
		for _, p := range o.Pfad {
			s.Pfad = append(s.Pfad, qwH32(t, p))
		}
		offen = append(offen, s)
	}
	return offen
}

func qsLaden(t *testing.T) (qsVektor, []quantenWOffenStelle) {
	t.Helper()
	roh, err := os.ReadFile("testdata/quanten_s_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v qsVektor
	if err := json.Unmarshal(roh, &v); err != nil {
		t.Fatal(err)
	}
	var offen []quantenWOffenStelle
	for _, o := range v.Offen {
		var s quantenWOffenStelle
		for _, a := range o.Anteile {
			var x int
			var y string
			json.Unmarshal(a[0], &x)
			json.Unmarshal(a[1], &y)
			yy, _ := new(big.Int).SetString(y, 10)
			s.Anteile = append(s.Anteile, quantenWAnteil{X: x, Y: yy})
		}
		for _, p := range o.Pfad {
			s.Pfad = append(s.Pfad, qwH32(t, p))
		}
		offen = append(offen, s)
	}
	return v, offen
}

func qsToken(t *testing.T, v qsVektor) *quantenSToken {
	t.Helper()
	var proben [2][32]byte
	for b := 0; b < 2; b++ {
		proben[b] = qwH32(t, v.ProbeHash[b])
	}
	e, err := neuerQuantenSToken(v.N, "0x00000000000000000000000000000000000000aa", qwH32(t, v.Wurzel), 5_000_000, proben, v.T, v.NVal, v.Tol)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestQuantenS_GleichWieSpezifikation(t *testing.T) {
	v, offen := qsLaden(t)
	for b := 0; b < 2; b++ {
		h := quantenSProbeHash(v.K, b, v.Proben[b])
		if hex.EncodeToString(h[:]) != v.ProbeHash[b] {
			t.Fatalf("Stichprobe %d: %x, Spezifikation %s", b, h, v.ProbeHash[b])
		}
	}
	if quantenSVorlageText(v.K, v.B, v.X) != v.VorlageText {
		t.Fatalf("Vorlage-Text %q", quantenSVorlageText(v.K, v.B, v.X))
	}
	inh := "0x00000000000000000000000000000000000000aa"
	e := qsToken(t, v)
	if err := e.binden(inh, v.D); err != nil {
		t.Fatal(err)
	}
	if err := e.waehlen(inh, v.C); err != nil {
		t.Fatal(err)
	}
	// Mit einem Fehler: abgewiesen (wie die Spezifikation), nichts verbraucht.
	tr, fe, err := e.einloesen(v.K, v.B, v.XFehler, v.Proben[v.B], offen)
	if (err == nil) != v.UrteilFehler || e.Eingeloest {
		t.Fatalf("x mit Fehler: %d/%d %v", tr, fe, err)
	}
	tr, fe, err = e.einloesen(v.K, v.B, v.X, v.Proben[v.B], offen)
	if (err == nil) != v.Urteil || tr != v.Treffer || fe != v.Fehler || !e.Eingeloest {
		t.Fatalf("Einloesen: %d Treffer, %d Fehler, %v; Spezifikation %d/%d %v", tr, fe, err, v.Treffer, v.Fehler, v.Urteil)
	}
	if _, _, err := e.einloesen(v.K, v.B, v.X, v.Proben[v.B], offen); err == nil {
		t.Fatal("zweimal eingeloest")
	}
}

func TestQuantenS_Missbrauch(t *testing.T) {
	v, offen := qsLaden(t)
	inh := "0x00000000000000000000000000000000000000aa"
	e := qsToken(t, v)
	if _, _, err := e.einloesen(v.K, v.B, v.X, v.Proben[v.B], offen); err == nil {
		t.Fatal("Einloesen ohne Bindung")
	}
	if e.waehlen(inh, v.C) == nil {
		t.Fatal("Wahl vor der Bindung")
	}
	if e.binden("0x00000000000000000000000000000000000000bb", v.D) == nil || e.binden(inh, v.D[1:]) == nil ||
		e.binden(inh, append([]int{2}, v.D[1:]...)) == nil {
		t.Fatal("Bindung: fremd, kurz oder Bit 2")
	}
	if err := e.binden(inh, v.D); err != nil || e.binden(inh, v.D) == nil {
		t.Fatal("zweite Bindung")
	}
	if e.waehlen(inh, 2) == nil || e.waehlen("0x00000000000000000000000000000000000000bb", 0) == nil {
		t.Fatal("Wahl: Form oder fremd")
	}
	if _, _, err := e.einloesen(v.K, v.B, v.X, v.Proben[v.B], offen); err == nil {
		t.Fatal("Einloesen vor der Wahl")
	}
	if err := e.waehlen(inh, v.C); err != nil || e.waehlen(inh, 1-v.C) == nil {
		t.Fatal("zweite Wahl")
	}
	andere := 1 - v.B
	dopp := append([]int{v.Proben[v.B][0]}, v.Proben[v.B][:len(v.Proben[v.B])-1]...)
	falsch := append([]quantenWOffenStelle(nil), offen...)
	falsch[0] = quantenWOffenStelle{Anteile: append([]quantenWAnteil(nil), offen[0].Anteile...), Pfad: offen[0].Pfad}
	falsch[0].Anteile[0] = quantenWAnteil{X: falsch[0].Anteile[0].X, Y: new(big.Int).Add(falsch[0].Anteile[0].Y, big.NewInt(1))}
	for name, f := range map[string]func() error{
		"fremde Stichprobe": func() error { _, _, err := e.einloesen(v.K, v.B, v.X, v.Proben[andere], offen); return err },
		"fremde Stichprobe, gueltig offengelegt": func() error {
			_, _, err := e.einloesen(v.K, v.B, v.X, v.Proben[andere], qsOffen(t, v.OffenAndere))
			return err
		},
		"andere Stelle":   func() error { _, _, err := e.einloesen(v.K, andere, v.X, v.Proben[v.B], offen); return err },
		"Stelle doppelt":  func() error { _, _, err := e.einloesen(v.K, v.B, v.X, dopp, offen); return err },
		"falscher Anteil": func() error { _, _, err := e.einloesen(v.K, v.B, v.X, v.Proben[v.B], falsch); return err },
		"Stelle fehlt":    func() error { _, _, err := e.einloesen(v.K, v.B, v.X, v.Proben[v.B], offen[1:]); return err },
		"x kurz":          func() error { _, _, err := e.einloesen(v.K, v.B, v.X[1:], v.Proben[v.B], offen); return err },
		"Stelle 2":        func() error { _, _, err := e.einloesen(v.K, 2, v.X, v.Proben[v.B], offen); return err },
	} {
		if f() == nil {
			t.Errorf("%s: eingeloest", name)
		}
		if e.Eingeloest {
			t.Fatalf("%s: abgewiesenes Einloesen hat verbraucht", name)
		}
	}
	for name, f := range map[string]func() (*quantenSToken, error){
		"Toleranz 15": func() (*quantenSToken, error) {
			return neuerQuantenSToken(40, inh, [32]byte{}, 1, [2][32]byte{}, 3, 5, 15)
		},
		"ohne Deckung": func() (*quantenSToken, error) {
			return neuerQuantenSToken(40, inh, [32]byte{}, 0, [2][32]byte{}, 3, 5, 4)
		},
		"t ueber v": func() (*quantenSToken, error) {
			return neuerQuantenSToken(40, inh, [32]byte{}, 1, [2][32]byte{}, 6, 5, 4)
		},
		"Inhaber kaputt": func() (*quantenSToken, error) {
			return neuerQuantenSToken(40, "x", [32]byte{}, 1, [2][32]byte{}, 3, 5, 4)
		},
		"zu gross": func() (*quantenSToken, error) {
			return neuerQuantenSToken(quantenWNMax+1, inh, [32]byte{}, 1, [2][32]byte{}, 3, 5, 4)
		},
	} {
		if _, err := f(); err == nil {
			t.Errorf("Ausgabe %s: angenommen", name)
		}
	}
}
