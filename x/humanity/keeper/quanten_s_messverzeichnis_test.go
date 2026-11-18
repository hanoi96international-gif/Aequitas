package keeper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// Gleich in hardware_ibm.py (selbsttest): Name des Eintrags zu y = "0110".
const qsmvNameVektor = "a5c3dd48facf21ed5f916d0ae979091fead570e6aea6c1d8038d1f68b26fa51f.json"

func qsmvAblegen(t *testing.T, dir string, d quantenSMessDatensatz) string {
	t.Helper()
	roh, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, quantenSMessName(d.Y))
	if err := os.WriteFile(p, roh, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func qsmvDa(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func TestQuantenSMessVerzeichnis_Name(t *testing.T) {
	if got := quantenSMessName("0110"); got != qsmvNameVektor {
		t.Fatalf("Name %s, erwartet %s (wie hardware_ibm.py)", got, qsmvNameVektor)
	}
}

func TestQuantenSMessVerzeichnis_GenauEinmal(t *testing.T) {
	dir := t.TempDir()
	p := qsmvAblegen(t, dir, quantenSMessDatensatz{U: "0101", T: "0011", Y: "0110", X: "0010", Wo: "fake", Job: "j1"})
	andere := qsmvAblegen(t, dir, quantenSMessDatensatz{U: "1111", T: "0000", Y: "0000", X: "1010", Wo: "fake", Job: "j2"})
	m := &quantenSMessVerzeichnis{dir: dir}
	u, tt, x, wo, err := m.abschnitt([]int{0, 1, 1, 0})
	if err != nil || qsBitsText(u) != "0101" || qsBitsText(tt) != "0011" || qsBitsText(x) != "0010" || wo != "fake j1" {
		t.Fatalf("Gutfall: %v", err)
	}
	if qsmvDa(p) {
		t.Fatal("verwendeter Eintrag liegt noch da")
	}
	if _, _, _, _, err := m.abschnitt([]int{0, 1, 1, 0}); err == nil || !strings.Contains(err.Error(), "keine Messung") {
		t.Fatalf("Abschnitt zweimal verwendet: %v", err)
	}
	if !qsmvDa(andere) {
		t.Fatal("der andere Eintrag ist weg")
	}
	if _, _, _, _, err := m.abschnitt([]int{0, 0, 0, 0}); err != nil {
		t.Fatalf("zweiter Abschnitt: %v", err)
	}
	if _, _, _, _, err := m.abschnitt(nil); err == nil {
		t.Fatal("leere Basen angenommen")
	}
}

// Missbrauch: jede Abweichung -- nichts verwendet, die Datei bleibt liegen.
func TestQuantenSMessVerzeichnis_Streng(t *testing.T) {
	y := []int{0, 1, 1, 0}
	gut := `{"u":"0101","t":"0011","y":"0110","x":"0010","wo":"fake","job":"j"}`
	for name, f := range map[string]func(t *testing.T, dir, p string){
		"fremde Basen unter dem Namen": func(t *testing.T, dir, p string) {
			os.WriteFile(p, []byte(`{"u":"0101","t":"0011","y":"1001","x":"0010"}`), 0o600)
		},
		"unbekanntes Feld": func(t *testing.T, dir, p string) {
			os.WriteFile(p, []byte(`{"u":"0101","t":"0011","y":"0110","x":"0010","mehr":1}`), 0o600)
		},
		"Text nach dem Datensatz": func(t *testing.T, dir, p string) {
			os.WriteFile(p, []byte(gut+` {"u":"1"}`), 0o600)
		},
		"kaputte Bits": func(t *testing.T, dir, p string) {
			os.WriteFile(p, []byte(`{"u":"01a1","t":"0011","y":"0110","x":"0010"}`), 0o600)
		},
		"zu kurz": func(t *testing.T, dir, p string) {
			os.WriteFile(p, []byte(`{"u":"010","t":"0011","y":"0110","x":"0010"}`), 0o600)
		},
		"kein JSON": func(t *testing.T, dir, p string) {
			os.WriteFile(p, []byte(`{`), 0o600)
		},
		"zu gross": func(t *testing.T, dir, p string) {
			os.WriteFile(p, []byte(gut+strings.Repeat(" ", quantenSMessEintragMax)), 0o600)
		},
		"Verweis auf einen gueltigen Eintrag": func(t *testing.T, dir, p string) {
			ziel := filepath.Join(t.TempDir(), "echt.json")
			os.WriteFile(ziel, []byte(gut), 0o600)
			if err := os.Symlink(ziel, p); err != nil {
				t.Skip("kein Symlink moeglich")
			}
		},
		"Verzeichnis statt Datei": func(t *testing.T, dir, p string) {
			os.Mkdir(p, 0o700)
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, quantenSMessName("0110"))
			f(t, dir, p)
			m := &quantenSMessVerzeichnis{dir: dir}
			if _, _, _, _, err := m.abschnitt(y); err == nil {
				t.Fatal("angenommen")
			}
			if !qsmvDa(p) {
				t.Fatal("abgewiesener Eintrag wurde entfernt")
			}
		})
	}
}

// Zwei Prozesse (hier: zwei Messquellen ohne gemeinsame Sperre) greifen nach
// derselben Messung: genau einer bekommt sie.
func TestQuantenSMessVerzeichnis_Wettlauf(t *testing.T) {
	for runde := 0; runde < 20; runde++ {
		dir := t.TempDir()
		qsmvAblegen(t, dir, quantenSMessDatensatz{U: "0101", T: "0011", Y: "0110", X: "0010"})
		quellen := []*quantenSMessVerzeichnis{{dir: dir}, {dir: dir}}
		var treffer atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func(m *quantenSMessVerzeichnis) {
				defer wg.Done()
				if _, _, _, _, err := m.abschnitt([]int{0, 1, 1, 0}); err == nil {
					treffer.Add(1)
				}
			}(quellen[i%2])
		}
		wg.Wait()
		if treffer.Load() != 1 {
			t.Fatalf("Runde %d: %d Mal verwendet", runde, treffer.Load())
		}
	}
}

// Messen waehrend der Dienst verbraucht: was einmal verwendet wurde, kommt
// nie zurueck, und jede neue Messung wird genau einmal verwendet.
func TestQuantenSMessVerzeichnis_MessenWaehrendVerbrauch(t *testing.T) {
	dir := t.TempDir()
	m := &quantenSMessVerzeichnis{dir: dir}
	const n = 64
	basen := make([][]int, n)
	for i := range basen {
		basen[i] = []int{i >> 5 & 1, i >> 4 & 1, i >> 3 & 1, i >> 2 & 1, i >> 1 & 1, i & 1}
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, y := range basen {
			yt := qsBitsText(y)
			roh, _ := json.Marshal(quantenSMessDatensatz{U: yt, T: yt, Y: yt, X: yt})
			tmp := filepath.Join(dir, ".neu-"+yt)
			os.WriteFile(tmp, roh, 0o600)
			os.Link(tmp, filepath.Join(dir, quantenSMessName(yt)))
			os.Remove(tmp)
		}
	}()
	verwendet := map[string]int{}
	for runde := 0; runde < 200 && len(verwendet) < n; runde++ {
		for _, y := range basen {
			if _, _, _, _, err := m.abschnitt(y); err == nil {
				verwendet[qsBitsText(y)]++
			}
		}
	}
	wg.Wait()
	for _, y := range basen {
		if _, _, _, _, err := m.abschnitt(y); err == nil {
			verwendet[qsBitsText(y)]++
		}
	}
	if len(verwendet) != n {
		t.Fatalf("%d von %d Messungen verwendet", len(verwendet), n)
	}
	for y, k := range verwendet {
		if k != 1 {
			t.Fatalf("Messung %s %d Mal verwendet", y, k)
		}
	}
}

func TestQuantenSMessquelleAusUmgebung(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AEQUITAS_QS_MESSQUELLE", dir)
	if _, ok := quantenSMessquelleAusUmgebung().(*quantenSMessVerzeichnis); !ok {
		t.Fatal("Verzeichnis nicht als Verzeichnis erkannt")
	}
	t.Setenv("AEQUITAS_QS_MESSQUELLE", filepath.Join(dir, "mess.json"))
	if _, ok := quantenSMessquelleAusUmgebung().(*quantenSMessDatei); !ok {
		t.Fatal("Datei nicht als Datei erkannt")
	}
	t.Setenv("AEQUITAS_QS_MESSQUELLE", " ")
	if quantenSMessquelleAusUmgebung() != nil {
		t.Fatal("ohne Angabe eine Messquelle")
	}
}
