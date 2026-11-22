package keeper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Die erste echte S-Ausgabe (quanten_s_ausgabe.go): das Werkzeug gegen die
// echten Regeln und den echten Validator-Dienst (in-process), dazu ein
// gefaelschter Knoten, die API-Basis und die Wiederholungsgrenze.

// qsNetz: das Werkzeug spricht mit den Diensten und der Kette der Testwelt,
// alles durch JSON (wie ueber HTTP).
type qsNetz struct {
	q *qwDienste
	// Gefaelschter Knoten: nach der ersten Anmeldung meldet stand "weg".
	luegeNachAnmeldung bool
	angemeldet         bool
	// Gefaelschter Dienst: unterschreibt mit einem fremden Schluessel.
	fremdeBestaetigung bool
	// finalNachlauf: so viele Bloecke laeuft die Finalitaet hinter der Hoehe
	// her (0 = alles sofort final, wie bisher).
	finalNachlauf int64
}

func (n *qsNetz) bestaetigen(_ context.Context, req QuantenWBestaetigenAnfrage) (string, error) {
	roh, _ := json.Marshal(req)
	var r QuantenWBestaetigenAnfrage
	if err := json.Unmarshal(roh, &r); err != nil {
		return "", err
	}
	if n.fremdeBestaetigung {
		k, _ := neuerSchluessel(n.q.w.t)
		return personalSign(n.q.w.t, k, "irgendwas"), nil
	}
	_, sig, err := n.q.d[r.X-1].bestaetigen(r)
	return sig, err
}

func (n *qsNetz) einreichen(_ context.Context, art, wallet string, zeit int64, a QuantenWAngaben) error {
	roh, _ := json.Marshal(a)
	var q QuantenWAngaben
	if err := json.Unmarshal(roh, &q); err != nil {
		return err
	}
	w := n.q.w
	n.q.mu.Lock()
	defer n.q.mu.Unlock()
	tx := Transaction{Type: art, Wallet: wallet, DistributionAt: zeit, QuantenW: &q}
	u := w.bescheid(w.cs, tx, n.q.zeit)
	if u.regel != "" {
		return fmt.Errorf("rejected: %s: %s", u.regel, u.text)
	}
	g := q
	g.Anteile, g.Pfand, g.PfandAn, g.Wert, g.WertAn, g.Urteil = u.felder.Anteile, u.felder.Pfand, u.felder.PfandAn, u.felder.Wert, u.felder.WertAn, u.felder.Urteil
	tx.QuantenW, tx.BuchAt = &g, n.q.zeit
	w.cs.mu.Lock()
	err := w.cs.quantenWAnwendenLocked(context.Background(), u)
	w.cs.mu.Unlock()
	if err != nil {
		return err
	}
	if art == quantenWSAnmeldenTxArt {
		n.angemeldet = true
	}
	return w.nachspielen(tx, n.q.zeit+7)
}

func (n *qsNetz) stand(ctx context.Context, K string) (map[string]interface{}, error) {
	if n.luegeNachAnmeldung && n.angemeldet {
		return nil, nil
	}
	s := n.q.w.cs.quantenWNoteAnsicht(ctx, K)
	if s == nil {
		return nil, nil
	}
	roh, _ := json.Marshal(s)
	var out map[string]interface{}
	return out, json.Unmarshal(roh, &out)
}

// warten: ein Takt -- Zeit weiter, alles final, die Dienste arbeiten.
func (n *qsNetz) warten(context.Context) error {
	n.q.zeit += 5
	h := n.q.hoehe.Load() + 1
	n.q.hoehe.Store(h)
	n.q.final.Store(h - n.finalNachlauf)
	n.q.takt()
	return nil
}

// qsWerkzeugWelt: Ausgeber ist Validator 1 (gebunden, KEIN Mensch, mit
// Guthaben) -- wie C1 auf dem Hauptnetz.
func qsWerkzeugWelt(t *testing.T, n int) (*qwDienste, QuantenSAusgabeAuftrag) {
	q := neueQwDienste(t, n)
	w := q.w
	for _, cs := range []*ChainState{w.cs, w.nach} {
		cs.accounts.Set(w.val[0], &AccountState{Address: w.val[0], Balance: NewDecimal(100)})
	}
	// Ehrliche Messung: Basen u, Werte t; die Inhaberin misst in y.
	r := rand.New(rand.NewSource(int64(n)))
	var u, tw, y, x []int
	for i := 0; i < n; i++ {
		u, tw, y = append(u, r.Intn(2)), append(tw, r.Intn(2)), append(y, r.Intn(2))
		if y[i] == u[i] {
			x = append(x, tw[i])
		} else {
			x = append(x, r.Intn(2))
		}
	}
	m := QuantenSMessung{N: n, U: qsBitsText(u), T: qsBitsText(tw), Y: qsBitsText(y), X: qsBitsText(x), Wo: "test"}
	return q, QuantenSAusgabeAuftrag{Messung: m, Schluessel: w.valKeys[0], Deckung: 2_000_000, Tol: quantenWTolStandard,
		Frist: time.Minute, Jetzt: func() int64 { return q.zeit }}
}

// Gutfall fuer jede Stelle b und Maske z (beide sonst Zufall).
func TestQuantenSAusgabe_Gutfall(t *testing.T) {
	for _, bz := range [][2]int{{0, 0}, {0, 1}, {1, 0}, {1, 1}} {
		t.Run(fmt.Sprintf("b%d_z%d", bz[0], bz[1]), func(t *testing.T) { qsGutfall(t, bz) })
	}
}

func qsGutfall(t *testing.T, bz [2]int) {
	q, a := qsWerkzeugWelt(t, 256)
	a.bz = &bz
	w := q.w
	vorher := w.stand(w.cs, w.val[0])
	var log bytes.Buffer
	erg, err := quantenSAusgabeLauf(context.Background(), a, w.val, w.p.t, &qsNetz{q: q}, &log)
	if err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	if erg.Doppelvorlage != "abgewiesen" || erg.Vorlage != "eingeloest" || erg.B != bz[0] || erg.Stellen[erg.B] != w.val[0] || erg.KJeStelle != 128 {
		t.Fatalf("Ergebnis: %+v", erg)
	}
	// Der Wert kommt an Stelle b = den Ausgeber zurueck; die andere Stelle
	// bekommt nichts; der Token ist weg (tot gemerkt).
	if got := w.stand(w.cs, w.val[0]); got != vorher {
		t.Fatalf("Ausgeber: vorher %d, nachher %d", vorher, got)
	}
	if w.stand(w.cs, erg.Stellen[1-erg.B]) != 0 {
		t.Fatal("die Stelle der Doppelvorlage hat etwas bekommen")
	}
	if s := w.cs.quantenWNoteAnsicht(context.Background(), erg.K); s == nil || s["tot"] != true {
		t.Fatalf("Token nach dem Einloesen: %v", s)
	}
	if w.cs.quantenWJetzt().kodiert() != w.nach.quantenWJetzt().kodiert() {
		t.Fatal("Register weichen ab")
	}
	if strings.Contains(log.String(), "PRIVATE") {
		t.Fatal("Log nennt Schluessel")
	}
	// Nachvollziehbar: jeder Schritt mit Zeit, jede Vorlage mit Beginn,
	// Dauer und Rate der Offenlegung.
	for _, muss := range []string{"ausgegeben (nach ", "gebunden und gewaehlt (nach ", "Doppelvorlage: abgewiesen (nach ",
		"ehrliche Vorlage: eingeloest (nach ", "angemeldet, ", "Stellen offenzulegen", "Urteil nach ", "Stellen/min"} {
		if !strings.Contains(log.String(), muss) {
			t.Fatalf("Protokoll ohne %q:\n%s", muss, log.String())
		}
	}
	if n := strings.Count(log.String(), "Stellen/min"); n != 2 {
		t.Fatalf("%d Raten im Protokoll, erwartet 2 (Doppelvorlage und ehrliche)", n)
	}
}

// Gefaelschter Knoten: behauptet nach der Doppelvorlage, der Token sei
// eingeloest -- das Werkzeug meldet einen Befund statt Erfolg.
func TestQuantenSAusgabe_DoppelvorlageAngenommenIstBefund(t *testing.T) {
	q, a := qsWerkzeugWelt(t, 256)
	_, err := quantenSAusgabeLauf(context.Background(), a, q.w.val, q.w.p.t, &qsNetz{q: q, luegeNachAnmeldung: true}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "BEFUND") {
		t.Fatalf("Doppelvorlage angenommen nicht erkannt: %v", err)
	}
}

// Gefaelschter Dienst: eine Bestaetigung, die nicht vom Validator stammt,
// wird nicht eingereicht.
func TestQuantenSAusgabe_FremdeBestaetigung(t *testing.T) {
	q, a := qsWerkzeugWelt(t, 256)
	n := &qsNetz{q: q, fremdeBestaetigung: true}
	_, err := quantenSAusgabeLauf(context.Background(), a, q.w.val, q.w.p.t, n, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "nicht von ihm") || n.angemeldet {
		t.Fatalf("fremde Bestaetigung angenommen: %v", err)
	}
	if len(q.w.cs.quantenWJetzt().Token) != 0 {
		t.Fatal("Token trotz fremder Bestaetigung ausgegeben")
	}
}

// Zu kleine Pruefgroesse oder Deckung, kaputte Messung: Abbruch, bevor
// irgendetwas den Knoten erreicht.
type qsKeinNetz struct{ t *testing.T }

func (n qsKeinNetz) bestaetigen(context.Context, QuantenWBestaetigenAnfrage) (string, error) {
	n.t.Fatal("Netz erreicht")
	return "", nil
}
func (n qsKeinNetz) einreichen(context.Context, string, string, int64, QuantenWAngaben) error {
	n.t.Fatal("Netz erreicht")
	return nil
}
func (n qsKeinNetz) stand(context.Context, string) (map[string]interface{}, error) {
	n.t.Fatal("Netz erreicht")
	return nil, nil
}
func (n qsKeinNetz) warten(context.Context) error { return nil }

func TestQuantenSAusgabe_AbbruchVorDemNetz(t *testing.T) {
	_, a := qsWerkzeugWelt(t, 256)
	val := []string{adrVon(a.Schluessel)}
	fall := func(name string, aendern func(*QuantenSAusgabeAuftrag), v []string, tt int) {
		t.Helper()
		b := a
		aendern(&b)
		if _, err := quantenSAusgabeLauf(context.Background(), b, v, tt, qsKeinNetz{t}, &bytes.Buffer{}); err == nil {
			t.Fatalf("%s: angenommen", name)
		}
	}
	quantenWKNoteOverride.Store(0) // die echte Tabelle: 840 je Stelle bei t = v = 1
	fall("k unter k_note", func(*QuantenSAusgabeAuftrag) {}, val, 1)
	quantenWKNoteOverride.Store(64)
	fall("Deckung", func(b *QuantenSAusgabeAuftrag) { b.Deckung = quantenMindestDeckungMik - 1 }, val, 1)
	fall("n ungerade", func(b *QuantenSAusgabeAuftrag) { b.Messung.N = 255 }, val, 1)
	fall("Bit kaputt", func(b *QuantenSAusgabeAuftrag) { b.Messung.X = "2" + b.Messung.X[1:] }, val, 1)
	fall("Laenge", func(b *QuantenSAusgabeAuftrag) { b.Messung.Y = b.Messung.Y[1:] }, val, 1)
	fall("Toleranz", func(b *QuantenSAusgabeAuftrag) { b.Tol = 7 }, val, 1)
}

func TestQuantenSAPIBasis(t *testing.T) {
	for _, ok := range []string{"http://127.0.0.1:8080", "http://localhost:8080", "http://[::1]:8080", "https://aequitas.digital", "http://127.0.0.1:8080/"} {
		if _, err := QuantenSAPIBasis(ok); err != nil {
			t.Fatalf("%s abgewiesen: %v", ok, err)
		}
	}
	for _, schlecht := range []string{"http://x@127.0.0.1:8080", "https://x:y@aequitas.digital", "http://10.0.0.1:8080", "http://aequitas.digital",
		"http://127.0.0.1:8080/api", "http://127.0.0.1:8080?a=1", "http://127.0.0.1:8080#x", "file:///etc/passwd", "ftp://127.0.0.1", "", "127.0.0.1:8080"} {
		if _, err := QuantenSAPIBasis(schlecht); err == nil {
			t.Fatalf("%s angenommen", schlecht)
		}
	}
}

// 429/503: wiederholt, aber hoechstens quantenSAusgabeVersuche Mal; ein
// anderer Fehler ist endgueltig; eine Weiterleitung wird nicht verfolgt.
func TestQuantenSHTTP_WiederholenBegrenzt(t *testing.T) {
	var zahl atomic.Int64
	immer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zahl.Add(1)
		http.Error(w, `{"error":"too many"}`, http.StatusTooManyRequests)
	}))
	defer immer.Close()
	h, err := neuerQuantenSHTTP(immer.URL)
	if err != nil {
		t.Fatal(err)
	}
	h.takt = time.Millisecond
	if _, err := h.stand(context.Background(), strings.Repeat("a", 64)); err == nil || zahl.Load() != quantenSAusgabeVersuche {
		t.Fatalf("nach %d Versuchen: %v", zahl.Load(), err)
	}

	zahl.Store(0)
	einmal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if zahl.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"k": r.URL.Query().Get("k"), "fehl": 0})
	}))
	defer einmal.Close()
	h, _ = neuerQuantenSHTTP(einmal.URL)
	h.takt = time.Millisecond
	if s, err := h.stand(context.Background(), strings.Repeat("b", 64)); err != nil || s == nil || zahl.Load() != 2 {
		t.Fatalf("503 einmal: %v %v", s, err)
	}

	ziel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"k": r.URL.Query().Get("k"), "fehl": 0})
	}))
	defer ziel.Close()
	weiter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, ziel.URL+r.URL.RequestURI(), http.StatusFound)
	}))
	defer weiter.Close()
	h, _ = neuerQuantenSHTTP(weiter.URL)
	if _, err := h.stand(context.Background(), strings.Repeat("c", 64)); err == nil {
		t.Fatal("Weiterleitung als Stand genommen")
	}

	fremd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"k": strings.Repeat("e", 64)})
	}))
	defer fremd.Close()
	h, _ = neuerQuantenSHTTP(fremd.URL)
	if _, err := h.stand(context.Background(), strings.Repeat("d", 64)); err == nil {
		t.Fatal("Stand eines fremden K genommen")
	}
}

func TestQuantenSAusgabe_MessungStreng(t *testing.T) {
	for _, roh := range []string{`{"n":2,"u":"01","t":"01","y":"01","x":"01","extra":1}`, `kein json`, strings.Repeat(" ", quantenSAusgabeMessMax+1)} {
		err := QuantenSAusgabe(context.Background(), "http://127.0.0.1:1", strings.Repeat("1", 64), 1_000_000, time.Second,
			nil, 0, QuantenSToleranzStandard, 0, false, 0, strings.NewReader(roh), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "Messung") {
			t.Fatalf("%.30q: %v", roh, err)
		}
	}
	err := QuantenSAusgabe(context.Background(), "http://127.0.0.1:1", "nicht-hex-geheim", 1_000_000, time.Second, nil, 0, QuantenSToleranzStandard, 0, false, 0, strings.NewReader("{}"), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || strings.Contains(err.Error(), "geheim") {
		t.Fatalf("Schluessel im Fehlertext oder angenommen: %v", err)
	}
	// Eine Toleranz, fuer die die Kette keine Pruefgroesse kennt: Abbruch vor allem anderen.
	for _, tol := range []int{0, 1, 7, 50, -2} {
		err := QuantenSAusgabe(context.Background(), "http://127.0.0.1:1", strings.Repeat("1", 64), 1_000_000, time.Second, nil, 0, tol, 0, false, 0, strings.NewReader("{}"), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "Toleranz") {
			t.Fatalf("Toleranz %d: %v", tol, err)
		}
	}
}

// praegerTauglich: Mensch mit Deckung, oder an einen Menschen GEBUNDENE
// Signieradresse mit Deckung. Alles andere nicht (fail-closed).
func TestQuantenWPraegerTauglich_Gebunden(t *testing.T) {
	cs := newTestState()
	alt := quantenWBetreiberVon
	t.Cleanup(func() { quantenWBetreiberVon = alt })
	bindung := map[string]string{}
	var registerKaputt bool
	quantenWBetreiberVon = func(_ *ChainState, _ context.Context, s string) (string, error) {
		if registerKaputt {
			return bindung[s], errors.New("Register nicht lesbar")
		}
		return bindung[s], nil
	}
	adr := func(i int) string { return fmt.Sprintf("0x%040x", i) }
	mensch, armerMensch, kein := adr(1), adr(2), adr(3)
	addHuman(cs, mensch, 5)
	addHuman(cs, armerMensch, 0)
	cs.accounts.Set(kein, &AccountState{Address: kein, Balance: NewDecimal(5)})
	reich := func(a string) string {
		cs.accounts.Set(a, &AccountState{Address: a, Balance: NewDecimal(5)})
		return a
	}
	gebunden, ungebunden, anNichtMensch, arm := reich(adr(10)), reich(adr(11)), reich(adr(12)), adr(13)
	cs.accounts.Set(arm, &AccountState{Address: arm, Balance: NewDecimal(0.5)})
	bindung[gebunden], bindung[anNichtMensch], bindung[arm] = mensch, kein, mensch
	for a, soll := range map[string]bool{mensch: true, armerMensch: false, gebunden: true, ungebunden: false, anNichtMensch: false, arm: false, adr(99): false} {
		if quantenWPraegerTauglich(cs, a) != soll {
			t.Fatalf("%s: soll %v", a, soll)
		}
	}
	registerKaputt = true
	if quantenWPraegerTauglich(cs, gebunden) {
		t.Fatal("Register nicht lesbar und trotzdem tauglich")
	}
	if !quantenWPraegerTauglich(cs, mensch) {
		t.Fatal("Mensch braucht das Register nicht")
	}
}

// Liste der Validatoren: Form, Grenzen, keine Benutzerangabe in der API,
// keine doppelte Adresse, Schwelle in 1..v.
func TestQuantenSValidatoren_Lesen(t *testing.T) {
	a1, a2 := "0x"+strings.Repeat("1", 40), "0x"+strings.Repeat("2", 40)
	gut, err := QuantenSValidatorenLesen(a1 + "=https://a.example," + a2 + "=http://127.0.0.1:8080")
	if err != nil || len(gut) != 2 {
		t.Fatalf("%v %v", gut, err)
	}
	if _, _, err := quantenSValidatorenPruefen(gut, 2); err != nil {
		t.Fatal(err)
	}
	for _, f := range []struct {
		roh string
		t   int
	}{
		{a1 + "=https://a.example," + a1 + "=https://b.example", 1}, // doppelt
		{a1 + "=https://x@a.example", 1},                            // Benutzerangabe
		{a1 + "=http://10.0.0.1:8080", 1},                           // http nach aussen
		{"0x1234=https://a.example", 1},                             // Adresse
		{a1 + "=https://a.example", 2},                              // t > v
		{a1 + "=https://a.example", 0},                              // t = 0
	} {
		vs, err := QuantenSValidatorenLesen(f.roh)
		if err == nil {
			_, _, err = quantenSValidatorenPruefen(vs, f.t)
		}
		if err == nil {
			t.Fatalf("angenommen: %q t=%d", f.roh, f.t)
		}
	}
	if _, err := QuantenSValidatorenLesen("ohne-gleich"); err == nil {
		t.Fatal("Form ohne = angenommen")
	}
	if _, err := QuantenSValidatorenLesen(strings.TrimSuffix(strings.Repeat(a1+"=https://a.example,", quantenWVMax+1), ",")); err == nil {
		t.Fatal("zu viele angenommen")
	}
}

// Das Werkzeug gibt die Vorlage nicht frueher verloren als die Kette: auf
// das Urteil wartet es mindestens quantenWFristOffen plus Nachlauf, auch
// wenn die Frist je Schritt kuerzer ist; eine laengere bleibt.
func TestQuantenSUrteilFrist(t *testing.T) {
	kette := time.Duration(quantenWFristOffen) * time.Second
	for _, schritt := range []time.Duration{time.Second, 15 * time.Minute, kette} {
		if f := quantenSUrteilFrist(schritt); f < kette+quantenSUrteilNachlauf {
			t.Fatalf("Schritt %s: Urteil nur %s, die Kette prueft %s", schritt, f, kette)
		}
	}
	if f := quantenSUrteilFrist(time.Hour); f != time.Hour {
		t.Fatalf("laengere Frist gekuerzt: %s", f)
	}
	if f := quantenSUrteilFrist(time.Minute); f > kette+quantenSUrteilNachlauf {
		t.Fatalf("Urteil unbegrenzt lang: %s", f)
	}
}

// Toter Praeparator (R9): Oeffnung und Anteil-Pakete fragt die Stelle je
// Aufruf genau einmal -- die Schleife in quantenSStichprobeV2Laden fragt nach
// und weicht auf die Anteile aus. Andere Anfragen bleiben bei
// quantenSAusgabeVersuche, und der geteilte Klient wird nicht veraendert.
func TestQuantenSHTTP_OeffnungEinmal(t *testing.T) {
	var zahl atomic.Int64
	tot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zahl.Add(1)
		http.Error(w, `{"error":"weg"}`, http.StatusServiceUnavailable)
	}))
	defer tot.Close()
	h, err := neuerQuantenSHTTP(tot.URL)
	if err != nil {
		t.Fatal(err)
	}
	h.takt = time.Millisecond
	netz := &quantenSMehrNetz{validatoren: []*quantenSHTTP{h, h}}
	K := strings.Repeat("a", 64)
	ctx := context.Background()
	if _, err := netz.oeffnung(ctx, 1, K, 0); err == nil || zahl.Load() != 1 {
		t.Fatalf("Oeffnung: %d Anfragen, %v", zahl.Load(), err)
	}
	zahl.Store(0)
	if _, err := netz.anteilOeffnung(ctx, 2, 1, K, 0); err == nil || zahl.Load() != 1 {
		t.Fatalf("Anteil: %d Anfragen, %v", zahl.Load(), err)
	}
	if h.versuche != 0 {
		t.Fatalf("geteilter Klient veraendert: versuche=%d", h.versuche)
	}
	zahl.Store(0)
	if _, err := netz.schluessel(ctx, 1); err == nil || zahl.Load() != quantenSAusgabeVersuche {
		t.Fatalf("Schluessel: %d Anfragen, %v", zahl.Load(), err)
	}
	// Nie mehr als quantenSAusgabeVersuche, auch wenn jemand mehr verlangt.
	zahl.Store(0)
	viel := *h
	viel.versuche = 1000
	kurz, aus := context.WithTimeout(ctx, 5*time.Second)
	defer aus()
	if _, err := viel.stand(kurz, K); err == nil || zahl.Load() != quantenSAusgabeVersuche {
		t.Fatalf("versuche=1000: %d Anfragen, %v", zahl.Load(), err)
	}
}
