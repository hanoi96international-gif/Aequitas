package keeper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Vormerken (quanten_s_vormerken.go): der Ausgeber merkt y beim Validator
// vor, der Server misst, das Werkzeug wartet. Hier: Pruefungen des Knotens,
// Grenzen, Verzeichnis und das Warten des Werkzeugs.

// qsvWelt: die Abschnitts-Welt (5 Validatoren, 3 von 5) mit einem
// Messverzeichnis bei Validator 2.
func qsvWelt(t *testing.T) (*qsaWelt, *quantenSMessVerzeichnis) {
	t.Helper()
	a := neueQsaWelt(t, 140)
	mv := &quantenSMessVerzeichnis{dir: t.TempDir()}
	a.d[1].messquelle = mv
	return a, mv
}

// vormerkung: y des Abschnitts x, vom Ausgeber unterschrieben.
func (a *qsaWelt) vormerkung(x int, basen string) QuantenSVormerkung {
	w := a.w
	q := QuantenSVormerkung{N: a.n, X: x, Validatoren: w.val, Basen: basen, Zeit: a.zeit, Praeger: w.praeger}
	q.PraegerSig = personalSign(w.t, w.keys[w.praeger], quantenSVormerkNachricht(q))
	return q
}

func (a *qsaWelt) basenVon(x int) string {
	von, bis, _ := quantenSAbschnitt(a.n, len(a.w.val), x)
	return qsBitsText(a.y[von:bis])
}

func qsvVormerkPfad(mv *quantenSMessVerzeichnis, basen string) string {
	return filepath.Join(mv.dir, quantenSVormerkVerzeichnis, strings.TrimSuffix(quantenSMessName(basen), ".json")+quantenSVormerkEndung)
}

// qsvVormerkungen: Namen der wartenden Vormerkungen und uebrige Eintraege.
func qsvVormerkungen(t *testing.T, mv *quantenSMessVerzeichnis) (basen, sonst []string) {
	t.Helper()
	es, err := os.ReadDir(filepath.Join(mv.dir, quantenSVormerkVerzeichnis))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		if strings.HasSuffix(e.Name(), quantenSVormerkEndung) {
			basen = append(basen, e.Name())
		} else {
			sonst = append(sonst, e.Name())
		}
	}
	return basen, sonst
}

func TestQuantenSVormerken_Gutfall(t *testing.T) {
	a, mv := qsvWelt(t)
	d := a.d[1]
	y := a.basenVon(2)
	ans, err := d.vormerken(a.vormerkung(2, y))
	if err != nil || ans.Status != quantenSVorgemerkt || ans.X != 2 || ans.Validator != a.w.val[1] {
		t.Fatalf("Vormerken: %+v, %v", ans, err)
	}
	pfad := qsvVormerkPfad(mv, y)
	info, err := os.Lstat(pfad)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("Vormerkung: %v, %v", info, err)
	}
	if vi, err := os.Lstat(filepath.Dir(pfad)); err != nil || vi.Mode().Perm() != 0o700 {
		t.Fatalf("Verzeichnis der Vormerkungen: %v, %v", vi, err)
	}
	roh, _ := os.ReadFile(pfad)
	var datei quantenSVormerkDatei
	dec := json.NewDecoder(bytes.NewReader(roh))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&datei); err != nil || datei != (quantenSVormerkDatei{Basen: y, Praeger: a.w.praeger, Zeit: a.zeit}) {
		t.Fatalf("Inhalt: %s (%v)", roh, err)
	}
	// Noch einmal: wartet, zaehlt nicht, legt nichts Neues an.
	for i := 0; i < 3; i++ {
		if ans, err := d.vormerken(a.vormerkung(2, y)); err != nil || ans.Status != quantenSWartet {
			t.Fatalf("Wiederholung: %+v, %v", ans, err)
		}
	}
	if mv.vormerkGesamt != 1 || mv.vormerkJe[a.w.praeger] != 1 {
		t.Fatalf("Zaehler %d/%d, verlangt 1", mv.vormerkGesamt, mv.vormerkJe[a.w.praeger])
	}
	basen, sonst := qsvVormerkungen(t, mv)
	if len(basen) != 1 || len(sonst) != 0 {
		t.Fatalf("Vormerkungen %v, uebrig %v", basen, sonst)
	}
	// Der Server hat gemessen (Vormerkung weg, Messung da): gemessen.
	os.Remove(pfad)
	u := strings.Repeat("0", len(y))
	qsmvAblegen(t, mv.dir, quantenSMessDatensatz{U: u, T: u, Y: y, X: u})
	if ans, err := d.vormerken(a.vormerkung(2, y)); err != nil || ans.Status != quantenSGemessen {
		t.Fatalf("nach der Messung: %+v, %v", ans, err)
	}
	if basen, _ := qsvVormerkungen(t, mv); len(basen) != 0 {
		t.Fatal("bei vorhandener Messung neu vorgemerkt")
	}
	// Messung verbraucht (Abschnitts-Auftrag), dann dieselbe Vormerkung noch
	// einmal (mitgelesen und wiederholt): keine zweite Messung am selben Tag.
	yb, _ := quantenSBitsLesen(y, len(y))
	if _, _, _, _, err := mv.abschnitt(yb); err != nil {
		t.Fatal(err)
	}
	if ans, err := d.vormerken(a.vormerkung(2, y)); err == nil {
		t.Fatalf("Wiederholung nach dem Verbrauch angenommen: %+v", ans)
	}
	if basen, _ := qsvVormerkungen(t, mv); len(basen) != 0 {
		t.Fatal("Wiederholung nach dem Verbrauch vorgemerkt")
	}
	if st, err := mv.vormerken(y, a.w.praeger, a.zeit+86400, a.zeit+86400); err != nil || st != quantenSVorgemerkt {
		t.Fatalf("am naechsten Tag: %s, %v", st, err)
	}
}

func TestQuantenSVormerken_Missbrauch(t *testing.T) {
	a, mv := qsvWelt(t)
	w := a.w
	d := a.d[1]
	y := a.basenVon(2)
	k2, adr2 := neuerSchluessel(t)
	unterschreiben := func(q *QuantenSVormerkung) {
		q.PraegerSig = personalSign(w.t, w.keys[w.praeger], quantenSVormerkNachricht(*q))
	}
	faelle := map[string]func(q *QuantenSVormerkung){
		"fremde Unterschrift": func(q *QuantenSVormerkung) { q.PraegerSig = personalSign(w.t, k2, quantenSVormerkNachricht(*q)) },
		"Unterschrift alt":    func(q *QuantenSVormerkung) { q.Basen = strings.Repeat("0", len(q.Basen)) },
		"keine Unterschrift":  func(q *QuantenSVormerkung) { q.PraegerSig = "" },
		"falsche Nummer":      func(q *QuantenSVormerkung) { q.X = 3; q.Basen = a.basenVon(3); unterschreiben(q) },
		"Nummer 0":            func(q *QuantenSVormerkung) { q.X = 0; unterschreiben(q) },
		"Nummer zu gross":     func(q *QuantenSVormerkung) { q.X = len(w.val) + 1; unterschreiben(q) },
		"nicht in der Liste": func(q *QuantenSVormerkung) {
			q.Validatoren = append([]string(nil), q.Validatoren...)
			q.Validatoren[1] = adr2
			unterschreiben(q)
		},
		"Validator doppelt": func(q *QuantenSVormerkung) {
			q.Validatoren = append([]string(nil), q.Validatoren...)
			q.Validatoren[4] = q.Validatoren[0]
			unterschreiben(q)
		},
		"Validator nicht kanonisch": func(q *QuantenSVormerkung) {
			q.Validatoren = append([]string(nil), q.Validatoren...)
			q.Validatoren[0] = strings.ToUpper(q.Validatoren[0])
			unterschreiben(q)
		},
		"keine Validatoren":   func(q *QuantenSVormerkung) { q.Validatoren = nil; unterschreiben(q) },
		"Basen zu kurz":       func(q *QuantenSVormerkung) { q.Basen = q.Basen[1:]; unterschreiben(q) },
		"Basen zu lang":       func(q *QuantenSVormerkung) { q.Basen += "0"; unterschreiben(q) },
		"Basen keine Bits":    func(q *QuantenSVormerkung) { q.Basen = "2" + q.Basen[1:]; unterschreiben(q) },
		"Basen mit Pfad":      func(q *QuantenSVormerkung) { q.Basen = "../" + q.Basen[3:]; unterschreiben(q) },
		"n zu klein":          func(q *QuantenSVormerkung) { q.N = 1; unterschreiben(q) },
		"n zu gross":          func(q *QuantenSVormerkung) { q.N = quantenWNMax + 1; unterschreiben(q) },
		"Zeit alt":            func(q *QuantenSVormerkung) { q.Zeit -= quantenSVormerkAlt + 1; unterschreiben(q) },
		"Zeit in der Zukunft": func(q *QuantenSVormerkung) { q.Zeit += quantenWVoraus + 1; unterschreiben(q) },
		"Ausgeber Fremder": func(q *QuantenSVormerkung) {
			q.Praeger = adr2
			q.PraegerSig = personalSign(w.t, k2, quantenSVormerkNachricht(*q))
		},
	}
	for name, f := range faelle {
		q := a.vormerkung(2, y)
		f(&q)
		if ans, err := d.vormerken(q); err == nil {
			t.Fatalf("%s angenommen: %+v", name, ans)
		}
	}
	// Ein Ausgeber ohne Deckung.
	alt := quantenWPraegerTauglich
	quantenWPraegerTauglich = func(*ChainState, string) bool { return false }
	if _, err := d.vormerken(a.vormerkung(2, y)); err == nil {
		t.Fatal("Ausgeber ohne Deckung angenommen")
	}
	quantenWPraegerTauglich = alt
	// Ohne Messquelle oder mit einer Datei als Messquelle (Pilot): keine Vormerkung.
	d.messquelle = nil
	if _, err := d.vormerken(a.vormerkung(2, y)); err == nil {
		t.Fatal("ohne Messquelle angenommen")
	}
	d.messquelle = &quantenSMessDatei{pfad: filepath.Join(t.TempDir(), "mess.json")}
	if _, err := d.vormerken(a.vormerkung(2, y)); err == nil {
		t.Fatal("mit Datei-Messquelle angenommen")
	}
	if basen, sonst := qsvVormerkungen(t, mv); len(basen)+len(sonst) != 0 || mv.vormerkGesamt != 0 {
		t.Fatalf("nach Abweisungen liegt etwas: %v %v (Zaehler %d)", basen, sonst, mv.vormerkGesamt)
	}
	// Gegenprobe: dieselbe Vormerkung richtig wird angenommen.
	d.messquelle = mv
	if ans, err := d.vormerken(a.vormerkung(2, y)); err != nil || ans.Status != quantenSVorgemerkt {
		t.Fatalf("Gutfall nach den Abweisungen: %+v, %v", ans, err)
	}
}

// qsvBasen: i-te Bitkette der Laenge n (verschieden fuer verschiedene i).
func qsvBasen(i, n int) string {
	b := []byte(strings.Repeat("0", n))
	for j := 0; j < 20 && j < n; j++ {
		if i>>j&1 == 1 {
			b[j] = '1'
		}
	}
	return string(b)
}

func TestQuantenSVormerken_Grenzen(t *testing.T) {
	const n, tag = 28, int64(20_000) * 86400
	jetzt := tag + 100
	t.Run("wartende", func(t *testing.T) {
		mv := &quantenSMessVerzeichnis{dir: t.TempDir()}
		for i := 0; i < quantenSVormerkOffenHoechstens; i++ {
			if st, err := mv.vormerken(qsvBasen(i, n), "0xa", jetzt, jetzt); err != nil || st != quantenSVorgemerkt {
				t.Fatalf("%d: %s, %v", i, st, err)
			}
		}
		if _, err := mv.vormerken(qsvBasen(99, n), "0xb", jetzt, jetzt); err == nil {
			t.Fatal("ueber die Grenze wartender Vormerkungen angenommen")
		}
		// Eine gemessen (Vormerkung entfernt): wieder Platz.
		os.Remove(qsvVormerkPfad(mv, qsvBasen(0, n)))
		if st, err := mv.vormerken(qsvBasen(99, n), "0xb", jetzt, jetzt); err != nil || st != quantenSVorgemerkt {
			t.Fatalf("nach dem Messen: %s, %v", st, err)
		}
	})
	t.Run("je Ausgeber und Tag", func(t *testing.T) {
		mv := &quantenSMessVerzeichnis{dir: t.TempDir()}
		for i := 0; i < quantenSVormerkJePraegerTag; i++ {
			if _, err := mv.vormerken(qsvBasen(i, n), "0xa", jetzt, jetzt); err != nil {
				t.Fatal(err)
			}
			os.Remove(qsvVormerkPfad(mv, qsvBasen(i, n))) // gemessen
		}
		if _, err := mv.vormerken(qsvBasen(50, n), "0xa", jetzt, jetzt); err == nil {
			t.Fatal("ueber die Grenze je Ausgeber angenommen")
		}
		if _, err := mv.vormerken(qsvBasen(51, n), "0xb", jetzt, jetzt); err != nil {
			t.Fatalf("ein anderer Ausgeber: %v", err)
		}
		if _, err := mv.vormerken(qsvBasen(52, n), "0xa", jetzt+86400, jetzt+86400); err != nil {
			t.Fatalf("am naechsten Tag: %v", err)
		}
	})
	t.Run("alle am Tag", func(t *testing.T) {
		mv := &quantenSMessVerzeichnis{dir: t.TempDir()}
		for i := 0; i < quantenSVormerkJeTag; i++ {
			p := fmt.Sprintf("0x%d", i/quantenSVormerkJePraegerTag)
			if _, err := mv.vormerken(qsvBasen(i, n), p, jetzt, jetzt); err != nil {
				t.Fatalf("%d: %v", i, err)
			}
			os.Remove(qsvVormerkPfad(mv, qsvBasen(i, n)))
		}
		if _, err := mv.vormerken(qsvBasen(500, n), "0xneu", jetzt, jetzt); err == nil {
			t.Fatal("ueber die Tagesgrenze angenommen")
		}
		if len(mv.vormerkJe) > quantenSVormerkJeTag {
			t.Fatalf("Zaehler je Ausgeber %d > %d", len(mv.vormerkJe), quantenSVormerkJeTag)
		}
		if _, err := mv.vormerken(qsvBasen(500, n), "0xneu", jetzt+86400, jetzt+86400); err != nil {
			t.Fatalf("am naechsten Tag: %v", err)
		}
		if len(mv.vormerkJe) != 1 {
			t.Fatalf("Zaehler des Vortags nicht verworfen: %d", len(mv.vormerkJe))
		}
	})
}

func TestQuantenSVormerken_Verzeichnis(t *testing.T) {
	const n, jetzt = 20, int64(1_800_000_000)
	y := qsvBasen(3, n)
	t.Run("Verweis statt Verzeichnis", func(t *testing.T) {
		mv := &quantenSMessVerzeichnis{dir: t.TempDir()}
		woanders := t.TempDir()
		if err := os.Symlink(woanders, filepath.Join(mv.dir, quantenSVormerkVerzeichnis)); err != nil {
			t.Fatal(err)
		}
		if _, err := mv.vormerken(y, "0xa", jetzt, jetzt); err == nil {
			t.Fatal("ueber einen Verweis vorgemerkt")
		}
		if es, _ := os.ReadDir(woanders); len(es) != 0 {
			t.Fatalf("hinter dem Verweis angelegt: %v", es)
		}
	})
	t.Run("Datei statt Verzeichnis", func(t *testing.T) {
		mv := &quantenSMessVerzeichnis{dir: t.TempDir()}
		os.WriteFile(filepath.Join(mv.dir, quantenSVormerkVerzeichnis), []byte("x"), 0o600)
		if _, err := mv.vormerken(y, "0xa", jetzt, jetzt); err == nil {
			t.Fatal("mit einer Datei an der Stelle des Verzeichnisses vorgemerkt")
		}
	})
	t.Run("Messung ist ein Verweis", func(t *testing.T) {
		mv := &quantenSMessVerzeichnis{dir: t.TempDir()}
		ziel := filepath.Join(t.TempDir(), "x.json")
		os.WriteFile(ziel, []byte("{}"), 0o600)
		os.Symlink(ziel, filepath.Join(mv.dir, quantenSMessName(y)))
		if st, err := mv.vormerken(y, "0xa", jetzt, jetzt); err == nil {
			t.Fatalf("Verweis als Messung gemeldet: %s", st)
		}
	})
	t.Run("Vormerkung ist ein Verweis", func(t *testing.T) {
		mv := &quantenSMessVerzeichnis{dir: t.TempDir()}
		os.Mkdir(filepath.Join(mv.dir, quantenSVormerkVerzeichnis), 0o700)
		ziel := filepath.Join(t.TempDir(), "fremd")
		os.Symlink(ziel, qsvVormerkPfad(mv, y))
		if st, err := mv.vormerken(y, "0xa", jetzt, jetzt); err != nil || st != quantenSWartet {
			t.Fatalf("%s, %v", st, err)
		}
		if _, err := os.Lstat(ziel); err == nil {
			t.Fatal("hinter dem Verweis geschrieben")
		}
	})
	t.Run("zu viele Eintraege", func(t *testing.T) {
		mv := &quantenSMessVerzeichnis{dir: t.TempDir()}
		vd := filepath.Join(mv.dir, quantenSVormerkVerzeichnis)
		os.Mkdir(vd, 0o700)
		for i := 0; i < quantenSVormerkLesenHoechstens; i++ {
			os.WriteFile(filepath.Join(vd, fmt.Sprintf(".neu-%d", i)), nil, 0o600)
		}
		if _, err := mv.vormerken(y, "0xa", jetzt, jetzt); err == nil {
			t.Fatal("bei vollem Verzeichnis vorgemerkt")
		}
	})
}

// Viele zugleich: dieselben Basen genau einmal, verschiedene nur bis zur Grenze.
func TestQuantenSVormerken_Gleichzeitig(t *testing.T) {
	const n, jetzt = 24, int64(1_800_000_000)
	mv := &quantenSMessVerzeichnis{dir: t.TempDir()}
	var wg sync.WaitGroup
	var mu sync.Mutex
	stand := map[string]int{}
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st, err := mv.vormerken(qsvBasen(7, n), "0xa", jetzt, jetzt)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				st = "fehler"
			}
			stand[st]++
		}()
	}
	wg.Wait()
	if stand[quantenSVorgemerkt] != 1 || stand[quantenSWartet] != 15 {
		t.Fatalf("dieselben Basen: %v", stand)
	}
	angenommen := 0
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if st, err := mv.vormerken(qsvBasen(100+i, n), fmt.Sprintf("0x%d", i%4), jetzt, jetzt); err == nil && st == quantenSVorgemerkt {
				mu.Lock()
				angenommen++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	basen, sonst := qsvVormerkungen(t, mv)
	if len(basen) != quantenSVormerkOffenHoechstens || angenommen != quantenSVormerkOffenHoechstens-1 || len(sonst) != 0 {
		t.Fatalf("verschiedene Basen: %d Vormerkungen, %d angenommen, uebrig %v", len(basen), angenommen, sonst)
	}
}

func TestQuantenSVormerken_HTTP(t *testing.T) {
	a, mv := qsvWelt(t)
	d := a.d[1]
	alt := a.w.cs.quantenWDienstZeiger.Swap(d)
	defer a.w.cs.quantenWDienstZeiger.Store(alt)
	// Der Endpunkt prueft die Freischaltung gegen die Uhr (time.Now); die
	// Testwelt liegt dahinter -- fuer diesen Test freigeschaltet.
	altAb := quantenWAbOverride.Load()
	quantenWAbOverride.Store(1)
	defer quantenWAbOverride.Store(altAb)
	api := &APIServer{state: a.w.cs}
	senden := func(methode, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(methode, "/api/quanten-w/validator/vormerken", strings.NewReader(body))
		r.RemoteAddr = "198.51.100.77:4711"
		rec := httptest.NewRecorder()
		api.handleQuantenSVormerken(rec, r)
		return rec
	}
	gut, _ := json.Marshal(a.vormerkung(2, a.basenVon(2)))
	if rec := senden(http.MethodGet, ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", rec.Code)
	}
	for name, body := range map[string]string{
		"kein JSON":       "{",
		"fremdes Feld":    strings.TrimSuffix(string(gut), "}") + `,"extra":1}`,
		"Text danach":     string(gut) + ` {}`,
		"zu gross":        `{"basen":"` + strings.Repeat("0", quantenSAbschnittKoerperMax) + `"}`,
		"falscher Typ":    `{"n":"viel"}`,
		"leeres Objekt":   `{}`,
		"Nummer fremd":    strings.Replace(string(gut), `"x":2`, `"x":3`, 1),
		"Basen geaendert": strings.Replace(string(gut), `"basen":"`, `"basen":"1`, 1),
	} {
		rec := senden(http.MethodPost, body)
		if rec.Code == http.StatusOK {
			t.Fatalf("%s: angenommen", name)
		}
	}
	if basen, sonst := qsvVormerkungen(t, mv); len(basen)+len(sonst) != 0 {
		t.Fatalf("nach Abweisungen liegt etwas: %v %v", basen, sonst)
	}
	rec := senden(http.MethodPost, string(gut))
	var ans QuantenSVormerkAntwort
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &ans) != nil || ans.Status != quantenSVorgemerkt {
		t.Fatalf("Gutfall: %d %s", rec.Code, rec.Body.String())
	}
}

// ------------------------------------------------------------ Werkzeug

// qsvNetz: Validatoren, die nach `nach` Anfragen "gemessen" melden.
type qsvNetz struct {
	mu       sync.Mutex
	val      []string
	nach     []int
	anfragen []int
	fehler   int // dieser Validator (1-basiert) weist ab
	luege    int // dieser Validator antwortet fuer einen anderen
	pruefe   func(x int, q QuantenSVormerkung) error
}

func (n *qsvNetz) vormerken(_ context.Context, x int, q QuantenSVormerkung) (QuantenSVormerkAntwort, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.pruefe != nil {
		if err := n.pruefe(x, q); err != nil {
			return QuantenSVormerkAntwort{}, err
		}
	}
	if x == n.fehler {
		return QuantenSVormerkAntwort{}, errors.New("HTTP 409: rejected: Tagesgrenze")
	}
	n.anfragen[x-1]++
	ans := QuantenSVormerkAntwort{Validator: n.val[x-1], X: x, Status: quantenSWartet}
	if n.anfragen[x-1] == 1 {
		ans.Status = quantenSVorgemerkt
	}
	if n.anfragen[x-1] > n.nach[x-1] {
		ans.Status = quantenSGemessen
	}
	if x == n.luege {
		ans.Validator = n.val[0]
	}
	return ans, nil
}

func TestQuantenSMessungenAbwarten(t *testing.T) {
	key, ausgeber := neuerSchluessel(t)
	var val []string
	for i := 0; i < 4; i++ {
		_, a := neuerSchluessel(t)
		val = append(val, a)
	}
	n := 120
	bs := QuantenSBasen{N: n}
	var err error
	if bs.Basen, err = quantenSBasenZiehen(n, len(val)); err != nil {
		t.Fatal(err)
	}
	for x := 1; x <= len(val); x++ {
		von, bis, _ := quantenSAbschnitt(n, len(val), x)
		if _, ok := quantenSBitsLesen(bs.Basen[x-1], bis-von); !ok {
			t.Fatalf("gezogene Basen %d passen nicht", x)
		}
	}
	jetzt := func() int64 { return 1_800_000_000 }
	// Jede Vormerkung ist vom Ausgeber unterschrieben und nennt genau den Abschnitt.
	pruefe := func(x int, q QuantenSVormerkung) error {
		if q.X != x || q.N != n || q.Basen != bs.Basen[x-1] || q.Praeger != ausgeber ||
			verifyPersonalSign(quantenSVormerkNachricht(q), q.PraegerSig, ausgeber) != nil {
			return fmt.Errorf("Vormerkung %d nicht richtig unterschrieben", x)
		}
		return nil
	}
	t.Run("Gutfall", func(t *testing.T) {
		netz := &qsvNetz{val: val, nach: []int{1, 3, 2, 1}, anfragen: make([]int, 4), pruefe: pruefe}
		var log bytes.Buffer
		if err := quantenSMessungenAbwarten(context.Background(), key, bs, val, netz, time.Second, time.Millisecond, jetzt, &log); err != nil {
			t.Fatal(err)
		}
		// Wer gemessen hat, wird nicht mehr gefragt.
		if fmt.Sprint(netz.anfragen) != "[2 4 3 2]" {
			t.Fatalf("Anfragen %v", netz.anfragen)
		}
		if c := strings.Count(log.String(), quantenSGemessen); c != 4 {
			t.Fatalf("Protokoll: %d mal gemessen\n%s", c, log.String())
		}
	})
	t.Run("Frist", func(t *testing.T) {
		netz := &qsvNetz{val: val, nach: []int{1, 1_000_000, 1, 1}, anfragen: make([]int, 4)}
		err := quantenSMessungenAbwarten(context.Background(), key, bs, val, netz, 30*time.Millisecond, 5*time.Millisecond, jetzt, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "1 von 4") {
			t.Fatalf("ohne Messung: %v", err)
		}
		if netz.anfragen[1] > 10 {
			t.Fatalf("zu oft gefragt: %d", netz.anfragen[1])
		}
	})
	t.Run("Abweisung", func(t *testing.T) {
		// Eine Abweisung bricht SOFORT ab (nicht erst an der Frist), mit Grund.
		netz := &qsvNetz{val: val, nach: []int{1, 1, 1, 1}, anfragen: make([]int, 4), fehler: 3}
		beginn := time.Now()
		err := quantenSMessungenAbwarten(context.Background(), key, bs, val, netz, time.Minute, 50*time.Millisecond, jetzt, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "Validator 3") || !strings.Contains(err.Error(), "Tagesgrenze") {
			t.Fatalf("Abweisung: %v", err)
		}
		if d := time.Since(beginn); d > 5*time.Second { // Frist ist 1 min: wer bis dahin weiterfragt, faellt auf
			t.Fatalf("erst nach %s abgebrochen", d)
		}
	})
	t.Run("fremde Antwort", func(t *testing.T) {
		netz := &qsvNetz{val: val, nach: []int{0, 0, 0, 0}, anfragen: make([]int, 4), luege: 2}
		if err := quantenSMessungenAbwarten(context.Background(), key, bs, val, netz, time.Second, time.Millisecond, jetzt, &bytes.Buffer{}); err == nil {
			t.Fatal("Antwort eines anderen Validators angenommen")
		}
	})
	t.Run("abgebrochen", func(t *testing.T) {
		ctx, stop := context.WithCancel(context.Background())
		stop()
		netz := &qsvNetz{val: val, nach: []int{1_000, 1_000, 1_000, 1_000}, anfragen: make([]int, 4)}
		if err := quantenSMessungenAbwarten(ctx, key, bs, val, netz, time.Minute, time.Second, jetzt, &bytes.Buffer{}); !errors.Is(err, context.Canceled) {
			t.Fatalf("Abbruch: %v", err)
		}
	})
	t.Run("Eingaben", func(t *testing.T) {
		netz := &qsvNetz{val: val, nach: []int{0, 0, 0, 0}, anfragen: make([]int, 4)}
		for name, f := range map[string]func() error{
			"zu wenige Basen": func() error {
				return quantenSMessungenAbwarten(context.Background(), key, QuantenSBasen{N: n, Basen: bs.Basen[:3]}, val, netz, time.Second, time.Millisecond, jetzt, &bytes.Buffer{})
			},
			"Wartezeit 0": func() error {
				return quantenSMessungenAbwarten(context.Background(), key, bs, val, netz, 0, time.Millisecond, jetzt, &bytes.Buffer{})
			},
			"Wartezeit zu lang": func() error {
				return quantenSMessungenAbwarten(context.Background(), key, bs, val, netz, quantenSMessenWartenMax+time.Second, time.Millisecond, jetzt, &bytes.Buffer{})
			},
		} {
			if f() == nil {
				t.Fatalf("%s angenommen", name)
			}
		}
		if fmt.Sprint(netz.anfragen) != "[0 0 0 0]" {
			t.Fatalf("bei falscher Eingabe gefragt: %v", netz.anfragen)
		}
		for _, nv := range [][2]int{{1, 4}, {quantenWNMax + 1, 4}, {100, 0}, {100, quantenWVMax + 1}} {
			if _, err := quantenSBasenZiehen(nv[0], nv[1]); err == nil {
				t.Fatalf("Basen ziehen mit n = %d, v = %d", nv[0], nv[1])
			}
		}
	})
}

// Gegen den echten Dienst: vormerken, der Server misst (hier: legt die
// Messung ab und entfernt die Vormerkung), das Werkzeug wartet, dann nimmt der
// Validator den Abschnitts-Auftrag an.
func TestQuantenSMessungenAbwarten_Dienst(t *testing.T) {
	a := neueQsaWelt(t, 140)
	w := a.w
	var mvs []*quantenSMessVerzeichnis
	for x := range a.d {
		mv := &quantenSMessVerzeichnis{dir: t.TempDir()}
		a.d[x].messquelle = mv
		mvs = append(mvs, mv)
	}
	bs := QuantenSBasen{N: a.n}
	for x := 1; x <= len(w.val); x++ {
		bs.Basen = append(bs.Basen, a.basenVon(x))
	}
	netz := &qsvDienstNetz{a: a, mvs: mvs}
	jetzt := func() int64 { return a.zeit }
	var log bytes.Buffer
	if err := quantenSMessungenAbwarten(context.Background(), w.keys[w.praeger], bs, w.val, netz, time.Second, time.Millisecond, jetzt, &log); err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	for x := 1; x <= len(w.val); x++ {
		if _, err := a.d[x-1].abschnittPraeparieren(a.auftragAbschnitt(x)); err != nil {
			t.Fatalf("Validator %d nach dem Vormerken: %v", x, err)
		}
		if basen, sonst := qsvVormerkungen(t, mvs[x-1]); len(basen)+len(sonst) != 0 {
			t.Fatalf("Validator %d: liegt noch %v %v", x, basen, sonst)
		}
		if es, _ := os.ReadDir(mvs[x-1].dir); len(es) != 1 { // nur noch das Verzeichnis der Vormerkungen
			t.Fatalf("Validator %d: Messung nicht verbraucht: %v", x, es)
		}
	}
}

// qsvDienstNetz: ruft den echten Dienst; nach jeder Vormerkung "misst" der
// Server beim naechsten Mal.
type qsvDienstNetz struct {
	a   *qsaWelt
	mvs []*quantenSMessVerzeichnis
}

func (n *qsvDienstNetz) vormerken(_ context.Context, x int, q QuantenSVormerkung) (QuantenSVormerkAntwort, error) {
	ans, err := n.a.d[x-1].vormerken(q)
	if err == nil && ans.Status == quantenSVorgemerkt {
		mv := n.mvs[x-1]
		os.Remove(qsvVormerkPfad(mv, q.Basen))
		r := n.a.mess[x-1]
		y, _ := quantenSBitsLesen(q.Basen, len(q.Basen))
		u, tw, xs, _, _ := r.abschnitt(y)
		r.aufruf.Add(-1)
		datei := quantenSMessDatensatz{U: qsBitsText(u), T: qsBitsText(tw), Y: q.Basen, X: qsBitsText(xs)}
		roh, _ := json.Marshal(datei)
		if err := os.WriteFile(filepath.Join(mv.dir, quantenSMessName(q.Basen)), roh, 0o600); err != nil {
			return QuantenSVormerkAntwort{}, err
		}
	}
	return ans, err
}
