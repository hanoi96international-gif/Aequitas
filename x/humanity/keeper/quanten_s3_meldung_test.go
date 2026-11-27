package keeper

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Gleich wie die Spezifikation: s_kette.py --vektor-s3 (Meldung, Teilmenge).
func TestQuantenS3_VektorTeilmenge(t *testing.T) {
	roh, err := os.ReadFile("testdata/quanten_s3_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		K        string `json:"k"`
		Lambda   []int  `json:"lambda"`
		Zufall   string `json:"zufall"`
		T10      []int  `json:"teilmenge_10"`
		T400     []int  `json:"teilmenge_400"`
		TAnderer []int  `json:"teilmenge_anderer_zufall"`
		Meld     []struct {
			NPuls int  `json:"n_puls"`
			Ppm   int  `json:"gamma_det_ppm"`
			Anz   int  `json:"anzahl"`
			OK    bool `json:"ok"`
		} `json:"meldungen"`
	}
	if err := json.Unmarshal(roh, &v); err != nil {
		t.Fatal(err)
	}
	z, _ := hex.DecodeString(v.Zufall)
	for _, f := range []struct {
		zufall []byte
		n      int
		soll   []int
	}{{z, 10, v.T10}, {z, 400, v.T400}, {make([]byte, 32), 10, v.TAnderer}} {
		ist, err := quantenS3Teilmenge(v.K, v.Lambda, f.zufall, 5, 6, f.n)
		if err != nil || !reflect.DeepEqual(ist, f.soll) {
			t.Fatalf("Teilmenge n=%d: %v %v, soll %v", f.n, ist, err, f.soll)
		}
	}
	for _, m := range v.Meld {
		lam := make([]int, m.Anz)
		for i := range lam {
			lam[i] = i
		}
		if err := quantenS3MeldungPruefen(m.NPuls, m.Ppm, lam); (err == nil) != m.OK {
			t.Fatalf("Meldung %+v: %v", m, err)
		}
	}
}

// Gleich wie die Spezifikation: s_kette.py --vektor-s3-meldung (Kodierung).
func TestQuantenS3_VektorKodierung(t *testing.T) {
	roh, err := os.ReadFile("testdata/quanten_s3_meldung_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Gut []struct {
			Lambda []int  `json:"lambda"`
			Text   string `json:"text"`
		} `json:"gut"`
		Schlecht []struct {
			Text string `json:"text"`
			OK   bool   `json:"ok"`
		} `json:"schlecht"`
	}
	if err := json.Unmarshal(roh, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Gut) < 5 || len(v.Schlecht) < 8 {
		t.Fatal("Vektor zu klein")
	}
	for _, g := range v.Gut {
		text, err := quantenS3Kodieren(g.Lambda)
		if err != nil || text != g.Text {
			t.Fatalf("Kodieren %v: %q %v, soll %q", g.Lambda[:1], text, err, g.Text)
		}
		lam, err := quantenS3Dekodieren(g.Text)
		if err != nil || !reflect.DeepEqual(lam, g.Lambda) {
			t.Fatalf("Dekodieren %q: %v", g.Text, err)
		}
	}
	for _, s := range v.Schlecht {
		text := s.Text
		if strings.HasPrefix(text, "00*") {
			text = strings.Repeat("00", quantenS3MeldungMax+1)
		}
		if _, err := quantenS3Dekodieren(text); (err == nil) != s.OK {
			t.Fatalf("Dekodieren %q: %v, soll ok=%v", s.Text, err, s.OK)
		}
	}
}

// Missbrauch: jede zweite Form, jede Grenze, Zufall vor der Meldung.
func TestQuantenS3_Angriffe(t *testing.T) {
	abgewiesen := func(name string, err error) {
		t.Helper()
		if err == nil || !errors.Is(err, errQuantenS3) {
			t.Fatalf("%s: angenommen (%v)", name, err)
		}
	}
	// Zweite Form derselben Meldung: eine ueberfluessige Null-Gruppe.
	_, err := quantenS3Dekodieren("8000")
	abgewiesen("ueberfluessige Gruppe", err)
	_, err = quantenS3Dekodieren("8100")
	abgewiesen("ueberfluessige Gruppe nach Wert", err)
	_, err = quantenS3Dekodieren("00AB")
	abgewiesen("Grossbuchstaben", err)
	_, err = quantenS3Dekodieren(strings.Repeat("0", quantenS3MeldungHexMax+2))
	abgewiesen("zu lang vor dem Lesen", err)
	if !strings.Contains(err.Error(), "Laenge") {
		t.Fatalf("Groesse nicht vor dem Lesen geprueft: %v", err)
	}
	// Hoechstens vier Gruppen je Zahl: sonst liefe die Verschiebung ueber
	// (neun Gruppen = 63 Bit, negativer Puls).
	for _, text := range []string{"ffffffff01", strings.Repeat("ff", 9) + "7f"} {
		_, err = quantenS3Dekodieren(text)
		abgewiesen("Zahl zu lang", err)
		if !strings.Contains(err.Error(), "Zahl zu lang") {
			t.Fatalf("%s: %v", text, err)
		}
	}
	_, err = quantenS3Dekodieren("ffffff1f00") // groesster Puls, dann noch einer
	abgewiesen("Puls ueber der Grenze", err)
	// Genau an der Grenze: MELDUNG_MAX Pulse gehen, einer mehr nicht.
	lam, err := quantenS3Dekodieren(strings.Repeat("00", quantenS3MeldungMax))
	if err != nil || len(lam) != quantenS3MeldungMax || lam[len(lam)-1] != quantenS3MeldungMax-1 {
		t.Fatalf("Grenze: %v %d", err, len(lam))
	}
	_, err = quantenS3Kodieren(append(lam, quantenS3MeldungMax))
	abgewiesen("Kodieren ueber der Grenze", err)
	_, err = quantenS3Kodieren([]int{4, 4})
	abgewiesen("doppelter Puls", err)
	_, err = quantenS3Kodieren([]int{5, 4})
	abgewiesen("absteigend", err)
	_, err = quantenS3Kodieren([]int{quantenS3PulsMax})
	abgewiesen("Puls >= Grenze", err)
	_, err = quantenS3Kodieren(nil)
	abgewiesen("leer", err)
	// Meldung pruefen.
	abgewiesen("fremder Puls", quantenS3MeldungPruefen(10, 1, []int{0, 10}))
	abgewiesen("doppelter Puls", quantenS3MeldungPruefen(10, 1, []int{3, 3}))
	abgewiesen("zu wenige", quantenS3MeldungPruefen(10, 300_000, []int{1, 2}))
	if err := quantenS3MeldungPruefen(10, 300_000, []int{1, 2, 3}); err != nil {
		t.Fatalf("genau genug: %v", err)
	}
	abgewiesen("nPuls zu gross", quantenS3MeldungPruefen(quantenS3PulsMax+1, 1, []int{0}))
	abgewiesen("gamma 0", quantenS3MeldungPruefen(10, 0, []int{0}))
	// Teilmenge: Zufall vor oder im Block der Meldung.
	z := make([]byte, 32)
	_, err = quantenS3Teilmenge("k", []int{1, 2}, z, 7, 7, 1)
	abgewiesen("Zufall im Block der Meldung", err)
	_, err = quantenS3Teilmenge("k", []int{1, 2}, z, 7, 6, 1)
	abgewiesen("Zufall vor der Meldung", err)
	_, err = quantenS3Teilmenge("k", []int{1, 2}, z[:31], 7, 8, 1)
	abgewiesen("Zufall zu kurz", err)
	_, err = quantenS3Teilmenge("k", []int{1, 2}, z, 7, 8, quantenWNMax+1)
	abgewiesen("nMax zu gross", err)
	if got, err := quantenS3Teilmenge("k", []int{1, 2}, z, 7, 8, 5); err != nil || !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("kleine Meldung: alles (%v %v)", got, err)
	}
}

// Eine Betruegerin, die die Teilmenge kennen will, muss Lambda VOR dem Zufall
// festlegen: mit einem anderen Zufall entsteht eine andere Teilmenge, und die
// Reihenfolge der Meldung spielt keine Rolle.
func TestQuantenS3_TeilmengeHaengtNurAmZufall(t *testing.T) {
	lam := make([]int, 5000)
	for i := range lam {
		lam[i] = 3 * i
	}
	z1, z2 := make([]byte, 32), make([]byte, 32)
	z2[0] = 1
	a, _ := quantenS3Teilmenge("k", lam, z1, 1, 2, 100)
	b, _ := quantenS3Teilmenge("k", lam, z2, 1, 2, 100)
	if reflect.DeepEqual(a, b) || len(a) != 100 {
		t.Fatal("anderer Zufall, gleiche Teilmenge")
	}
	c, _ := quantenS3Teilmenge("anderer-token", lam, z1, 1, 2, 100)
	if reflect.DeepEqual(a, c) {
		t.Fatal("anderer Token, gleiche Teilmenge")
	}
	for i := 1; i < len(a); i++ {
		if a[i] <= a[i-1] {
			t.Fatal("Teilmenge nicht aufsteigend")
		}
	}
}
