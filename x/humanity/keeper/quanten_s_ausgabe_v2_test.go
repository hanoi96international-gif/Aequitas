package keeper

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Das Werkzeug im v2-Weg (quanten_s_ausgabe_abschnitte.go, DEZENTRAL.md 16)
// gegen die echten Regeln und den echten Dienst: Teil-Hashes und Zaehlungen
// bei der Ausgabe, Stichproben erst nach der Bindung.

// qsv2Netz: qsaNetz, das mitschreibt, ob eine Oeffnung vor der Bindung
// verlangt wurde, und einen Validator falsch zaehlen lassen kann.
type qsv2Netz struct {
	*qsaNetz
	vorBindung   int // Oeffnungen, als d oder c noch nicht auf der Kette standen
	falschZaehlt int // dieser Validator meldet einen Treffer zu wenig (selbst unterschrieben)
	ohneTeile    int // dieser Validator laesst Teile und Zaehlung weg (selbst unterschrieben)
	teileBeiV1   int // dieser Validator haengt bei v1 Teile an (selbst unterschrieben)
	salzFalsch   int // dieser Validator gibt mit der Oeffnung ein falsches Salz heraus
	wertFalsch   int // dieser Validator gibt einen falschen Wert heraus (Stelle und Bit richtig)
	finalNach    int // erst ab dieser Zahl von Oeffnungs-Anfragen ist die Bindung final
	oeffnungen   int
}

func (n *qsv2Netz) oeffnung(ctx context.Context, x int, K string, b int) (string, error) {
	if tok := n.q.w.cs.quantenWJetzt().Token[K]; tok == nil || tok.token.D == nil || tok.token.C == -1 {
		n.vorBindung++
	}
	n.oeffnungen++
	if n.finalNach > 0 && n.oeffnungen >= n.finalNach {
		n.q.final.Store(n.q.hoehe.Load())
	}
	if x == n.salzFalsch {
		if e, _ := n.q.d[x-1].eintragVon(K); e != nil && e.Abschnitt != nil && len(e.Abschnitt.Salze) == 2 {
			e.Abschnitt.Salze[b] = strings.Repeat("00", 32)
		}
	}
	if x == n.wertFalsch {
		if e, _ := n.q.d[x-1].eintragVon(K); e != nil && e.Abschnitt != nil && len(e.Abschnitt.Koeff) > 0 {
			for i := range e.Abschnitt.Koeff {
				w, _ := e.Abschnitt.wert(i)
				if quantenSStellenBit(w) == b {
					w.SetBit(w, 1, w.Bit(1)^1) // anderer Wert t, Stellen-Bit (Bit 2) bleibt
					e.Abschnitt.Koeff[i][0] = w.Text(16)
					break
				}
			}
		}
	}
	return n.qsaNetz.oeffnung(ctx, x, K, b)
}

func (n *qsv2Netz) abschnitt(ctx context.Context, x int, req QuantenSAbschnittAnfrage) (QuantenSAbschnittAntwort, error) {
	ans, err := n.qsaNetz.abschnitt(ctx, x, req)
	if err != nil {
		return ans, err
	}
	neu := false
	if x == n.falschZaehlt && len(ans.Zaehlung) == 2 && ans.Zaehlung[0][0] > ans.Zaehlung[0][1] {
		ans.Zaehlung[0][0]--
		neu = true
	}
	if x == n.ohneTeile {
		ans.Teile, ans.Zaehlung, neu = nil, nil, true
	}
	if x == n.teileBeiV1 && ans.Teile == nil {
		ans.Teile, ans.Zaehlung, neu = []string{strings.Repeat("ab", 32), strings.Repeat("cd", 32)}, [][2]int{{0, 0}, {0, 0}}, true
	}
	if neu {
		ans.Sig = personalSign(n.q.w.t, n.q.w.valKeys[x-1], quantenSAbschnittAntwortNachricht(ans))
	}
	return ans, err
}

func qsv2Vorbereiten(t *testing.T) func() {
	t.Helper()
	quantenSProbeV2AbOverride.Store(1)
	alt := quantenSV2Nachfragen
	quantenSV2Nachfragen = time.Millisecond
	return func() { quantenSProbeV2AbOverride.Store(0); quantenSV2Nachfragen = alt }
}

func TestQuantenSAusgabeV2_Gutfall(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	for _, bz := range [][2]int{{0, 0}, {1, 1}} {
		t.Run(fmt.Sprintf("b%d_z%d", bz[0], bz[1]), func(t *testing.T) {
			q, a, bs, mess := qsaWerkzeug(t, 256)
			a.bz, a.ProbeV2 = &bz, true
			w := q.w
			netz := &qsv2Netz{qsaNetz: &qsaNetz{qsNetz: &qsNetz{q: q}}}
			var log bytes.Buffer
			erg, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, w.val, 3, netz, &log)
			if err != nil {
				t.Fatalf("%v\n%s", err, log.String())
			}
			if erg.Doppelvorlage != "abgewiesen" || erg.Vorlage != "eingeloest" || erg.KJeStelle == 0 {
				t.Fatalf("Ergebnis: %+v", erg)
			}
			if netz.vorBindung != 0 {
				t.Fatalf("%d Oeffnungen vor Bindung und Wahl verlangt", netz.vorBindung)
			}
			if w.stand(w.cs, erg.Stellen[1-erg.B]) != 0 {
				t.Fatal("die Stelle der Doppelvorlage hat etwas bekommen")
			}
			if w.cs.quantenWJetzt().kodiert() != w.nach.quantenWJetzt().kodiert() {
				t.Fatal("Register weichen ab")
			}
			for x, m := range mess {
				if m.aufruf.Load() != 1 {
					t.Fatalf("Validator %d: %d Messungen", x+1, m.aufruf.Load())
				}
			}
			for _, muss := range []string{"Stichprobe v2 (erst nach der Bindung)", "aus den Zaehlungen der Validatoren",
				"Stichproben nach der Bindung geladen", "Doppelvorlage: abgewiesen", "ehrliche Vorlage: eingeloest"} {
				if !strings.Contains(log.String(), muss) {
					t.Fatalf("Protokoll ohne %q:\n%s", muss, log.String())
				}
			}
		})
	}
}

// Ein Validator zaehlt falsch (selbst unterschrieben): nach der Bindung
// rechnet die Stelle nach und nennt ihn -- vor der ersten Vorlage.
func TestQuantenSAusgabeV2_FalscheZaehlung(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	q, a, bs, _ := qsaWerkzeug(t, 256)
	a.ProbeV2 = true
	netz := &qsv2Netz{qsaNetz: &qsaNetz{qsNetz: &qsNetz{q: q}}, falschZaehlt: 2}
	var log bytes.Buffer
	_, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, netz, &log)
	if err == nil || !strings.Contains(err.Error(), "BEFUND: Validator 2") {
		t.Fatalf("falsche Zaehlung: %v\n%s", err, log.String())
	}
	if strings.Contains(log.String(), "Doppelvorlage:") {
		t.Fatal("trotz falscher Zaehlung vorgelegt")
	}
}

// Eine Oeffnung fuer die andere Stelle (nicht zu entschluesseln): Abbruch vor
// der ersten Vorlage.
func TestQuantenSAusgabeV2_OeffnungVertauscht(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	q, a, bs, _ := qsaWerkzeug(t, 256)
	a.ProbeV2, a.Frist = true, time.Second
	netz := &qsv2Netz{qsaNetz: &qsaNetz{qsNetz: &qsNetz{q: q}, oeffnungVertauscht: 3}}
	var log bytes.Buffer
	_, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, netz, &log)
	if err == nil || !strings.Contains(err.Error(), "Validator 3") || strings.Contains(log.String(), "Doppelvorlage:") {
		t.Fatalf("vertauschte Oeffnung: %v\n%s", err, log.String())
	}
}

// Schlaeft die Stichprobe v2 auf der Kette, weist sie die Ausgabe ab (die
// Kette hat das letzte Wort); das Werkzeug fragt deshalb vorher.
func TestQuantenSAusgabeV2_KetteSchlaeft(t *testing.T) {
	// Im Hauptnetz ist der Stichtag gesetzt; schlafend hier per Override
	// (unabhaengig von Uhr und fester Testzeit).
	quantenSProbeV2AbOverride.Store(math_MaxInt64())
	t.Cleanup(func() { quantenSProbeV2AbOverride.Store(0) })
	q, a, bs, _ := qsaWerkzeug(t, 256)
	a.ProbeV2 = true
	_, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, &qsv2Netz{qsaNetz: &qsaNetz{qsNetz: &qsNetz{q: q}}}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "quanten_w_s_probe_v2") {
		t.Fatalf("Ausgabe v2 auf schlafender Kette: %v", err)
	}
}

func TestQuantenSProbeV2AufKette(t *testing.T) {
	const jetzt = 1_900_000_000
	for name, fall := range map[string]struct {
		code int
		body string
		gut  bool
	}{
		"aktiv":          {200, `{"quanten_w":{"ab":1,"probe_v2_ab":1800000000}}`, true},
		"genau jetzt":    {200, `{"quanten_w":{"probe_v2_ab":1900000000}}`, true},
		"schlafend":      {200, `{"quanten_w":{"ab":1}}`, false},
		"ohne quanten":   {200, `{}`, false},
		"in Zukunft":     {200, `{"quanten_w":{"probe_v2_ab":1900000001}}`, false},
		"kein JSON":      {200, `kaputt`, false},
		"falscher Typ":   {200, `{"quanten_w":{"probe_v2_ab":"1800000000"}}`, false},
		"Fehler":         {500, `{}`, false},
		"nicht gefunden": {404, `{}`, false},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/status" {
				http.NotFound(rw, r)
				return
			}
			rw.WriteHeader(fall.code)
			rw.Write([]byte(fall.body))
		}))
		h, err := neuerQuantenSHTTP(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		err = quantenSProbeV2AufKette(context.Background(), h, jetzt)
		srv.Close()
		if (err == nil) != fall.gut {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestQuantenSAntwortV2Pruefen(t *testing.T) {
	gut := QuantenSAbschnittAntwort{Teile: []string{strings.Repeat("ab", 32), strings.Repeat("cd", 32)}, Zaehlung: [][2]int{{10, 1}, {12, 0}}}
	if err := quantenSAntwortV2Pruefen(gut, 30); err != nil {
		t.Fatal(err)
	}
	for name, f := range map[string]func(*QuantenSAbschnittAntwort){
		"ohne Teile":           func(a *QuantenSAbschnittAntwort) { a.Teile = nil },
		"ein Teil":             func(a *QuantenSAbschnittAntwort) { a.Teile = a.Teile[:1] },
		"ohne Zaehlung":        func(a *QuantenSAbschnittAntwort) { a.Zaehlung = nil },
		"Teil kein Hash":       func(a *QuantenSAbschnittAntwort) { a.Teile = []string{"zz", a.Teile[1]} },
		"Fehler > Treffer":     func(a *QuantenSAbschnittAntwort) { a.Zaehlung = [][2]int{{1, 2}, {0, 0}} },
		"Fehler negativ":       func(a *QuantenSAbschnittAntwort) { a.Zaehlung = [][2]int{{1, -1}, {0, 0}} },
		"Treffer > Stellen":    func(a *QuantenSAbschnittAntwort) { a.Zaehlung = [][2]int{{31, 0}, {0, 0}} },
		"zweite Stelle kaputt": func(a *QuantenSAbschnittAntwort) { a.Zaehlung = [][2]int{{1, 0}, {2, 3}} },
	} {
		a := gut
		a.Teile = append([]string(nil), gut.Teile...)
		a.Zaehlung = append([][2]int(nil), gut.Zaehlung...)
		f(&a)
		if quantenSAntwortV2Pruefen(a, 30) == nil {
			t.Fatalf("%s angenommen", name)
		}
	}
}

// Der Befehl: --stichprobe-v2 nur im Abschnitts-Modus.
func TestQuantenSAusgabe_V2NurAbschnitte(t *testing.T) {
	err := QuantenSAusgabe(context.Background(), "http://127.0.0.1:1", strings.Repeat("1", 64), 1_000_000, time.Second,
		nil, 0, QuantenSToleranzStandard, 0, true, 0, strings.NewReader(`{"n":2,"u":"01","t":"01","y":"01","x":"01"}`), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "nur im Abschnitts-Modus") {
		t.Fatalf("v2 mit einer Messung: %v", err)
	}
}

// Antworten, die nicht zum Weg passen (selbst unterschrieben): Abbruch,
// bevor etwas auf die Kette geht.
func TestQuantenSAusgabeV2_AntwortPasstNicht(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	for name, fall := range map[string]struct {
		v2   bool
		netz func(*qsaNetz) *qsv2Netz
		muss string
	}{
		"v2 ohne Teile": {true, func(b *qsaNetz) *qsv2Netz { return &qsv2Netz{qsaNetz: b, ohneTeile: 2} }, "Teil-Hashes oder Zaehlung fehlen"},
		"v1 mit Teilen": {false, func(b *qsaNetz) *qsv2Netz { return &qsv2Netz{qsaNetz: b, teileBeiV1: 4} }, "Teil-Hashes ohne Stichprobe v2"},
	} {
		q, a, bs, _ := qsaWerkzeug(t, 256)
		a.ProbeV2 = fall.v2
		_, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, fall.netz(&qsaNetz{qsNetz: &qsNetz{q: q}}), &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), fall.muss) {
			t.Fatalf("%s: %v", name, err)
		}
		if q.w.cs.quantenWJetzt().Token[strings.Repeat("0", 64)] != nil || len(q.w.cs.quantenWJetzt().Token) != 0 {
			t.Fatalf("%s: trotzdem ausgegeben", name)
		}
	}
}

// Ein Validator gibt ein falsches Salz heraus: die Stelle rechnet die
// Bindung nach und bricht vor der ersten Vorlage ab.
func TestQuantenSAusgabeV2_FalschesSalz(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	q, a, bs, _ := qsaWerkzeug(t, 256)
	a.ProbeV2 = true
	var log bytes.Buffer
	_, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, &qsv2Netz{qsaNetz: &qsaNetz{qsNetz: &qsNetz{q: q}}, salzFalsch: 3}, &log)
	if err == nil || !strings.Contains(err.Error(), "passen nicht zur Bindung") || strings.Contains(log.String(), "Doppelvorlage:") {
		t.Fatalf("falsches Salz: %v\n%s", err, log.String())
	}
}

// Die Validatoren geben die Oeffnung erst nach der Finalitaet heraus: das
// Werkzeug fragt begrenzt nach und macht dann weiter.
func TestQuantenSAusgabeV2_WartetAufFinalitaet(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	q, a, bs, _ := qsaWerkzeug(t, 256)
	a.ProbeV2 = true
	q.hoehe.Store(10)
	q.final.Store(5)
	netz := &qsv2Netz{qsaNetz: &qsaNetz{qsNetz: &qsNetz{q: q}}, finalNach: 4}
	var log bytes.Buffer
	erg, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, netz, &log)
	if err != nil || erg.Vorlage != "eingeloest" {
		t.Fatalf("%v\n%s", err, log.String())
	}
	if netz.oeffnungen < 4+2*len(q.w.val)-1 {
		t.Fatalf("nur %d Oeffnungs-Anfragen -- nicht nachgefragt", netz.oeffnungen)
	}
}

// Ein Validator gibt einen falschen Wert heraus (Stelle richtig): die Stelle
// prueft gegen das Blatt und laedt nicht.
func TestQuantenSAusgabeV2_FalscherWert(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	q, a, bs, _ := qsaWerkzeug(t, 256)
	a.ProbeV2 = true
	var log bytes.Buffer
	_, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, &qsv2Netz{qsaNetz: &qsaNetz{qsNetz: &qsNetz{q: q}}, wertFalsch: 2}, &log)
	if err == nil || !strings.Contains(err.Error(), "laedt ihre Stichprobe nicht") || strings.Contains(log.String(), "Doppelvorlage:") {
		t.Fatalf("falscher Wert: %v\n%s", err, log.String())
	}
}

// Die ehrliche Vorlage bestuende nicht (verrauschte Messung): die exakte
// Probe aus den Zaehlungen bricht ab, bevor etwas auf die Kette geht.
func TestQuantenSAusgabeV2_EhrlichNichtBestanden(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	q, a, bs, mess := qsaWerkzeug(t, 256)
	a.ProbeV2 = true
	for _, m := range mess {
		m.verrauscht = 25
	}
	_, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, &qsv2Netz{qsaNetz: &qsaNetz{qsNetz: &qsNetz{q: q}}}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "bestuende") {
		t.Fatalf("verrauscht: %v", err)
	}
	if len(q.w.cs.quantenWJetzt().Token) != 0 {
		t.Fatal("trotzdem ausgegeben")
	}
}

// Nachweis abwarten nur mit Stichprobe v2 und hoechstens 1 h.
func TestQuantenSAusgabe_NachweisWartenGrenzen(t *testing.T) {
	for _, f := range []struct {
		v2     bool
		warten time.Duration
	}{{false, time.Minute}, {true, -time.Second}, {true, time.Hour + time.Second}} {
		err := QuantenSAusgabe(context.Background(), "http://127.0.0.1:1", strings.Repeat("1", 64), 1_000_000, time.Second,
			nil, 0, QuantenSToleranzStandard, 0, f.v2, f.warten, strings.NewReader("{}"), &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "Nachweis abwarten") {
			t.Fatalf("v2=%v warten=%s: %v", f.v2, f.warten, err)
		}
	}
}
