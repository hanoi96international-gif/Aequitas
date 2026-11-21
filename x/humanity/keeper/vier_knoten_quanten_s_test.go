package keeper

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// QUANTENGELD VARIANTE S AUF VIER ECHTEN KNOTEN (Pilot mit mehreren
// Validatoren, Voraussetzung fuer W6).
//
// Am 17.11.2026 lief der erste S-Token im Hauptnetz -- mit EINEM Validator
// (C1). Hier dasselbe Werkzeug (quantenSAusgabeLauf, wie `aequitasd
// quanten-s-ausgabe --validatoren ... --t 3`) gegen vier echte Knoten:
// eigene Postgres-Datenbank, eigener Signierschluessel, echtes HTTP, je ein
// Validator-Dienst. Das Geheimnis des Tokens liegt als Shamir-Anteile bei
// allen vier, 3 von 4 genuegen zum Offenlegen. Jeder Validator bestaetigt
// an SEINER eigenen API; eingereicht und gelesen wird bei einem Folger (er
// leitet an den Leiter weiter). Das Werkzeug glaubt keinem Knoten: es prueft
// jede Bestaetigung und liest das Urteil aus dem Stand.
//
// Erwartet wie im Hauptnetz: Doppelvorlage abgewiesen, ehrliche Vorlage
// eingeloest, die Toepfe leer, alle vier Knoten im Gleichstand.
//
// quantenWKNoteOverride = 128 je Annahmestelle (die Tabelle verlangt fuer
// 3 von 4 Validatoren 5.200, das waeren 82 Teile der Offenlegung); der Weg
// ist derselbe, die Tabelle pruefen quanten_w_test.go und die Spezifikation.
func TestVierKnotenQuantenS_RealDB(t *testing.T) { qsVierKnoten(t, false) }

// Dasselbe, aber ein Validator faellt nach der Ausgabe aus (vor der ersten
// Vorlage, weder Leiter noch der Knoten des Werkzeugs): 3 von 4 genuegen --
// die uebrigen drei sammeln ihre Anteile und legen offen, das Urteil steht
// trotzdem. Genau dafuer ist das Geheimnis t-von-v geteilt.
func TestVierKnotenQuantenSAusfall_RealDB(t *testing.T) { qsVierKnoten(t, true) }

// AUSGABE IN ABSCHNITTEN (forschung/quantengeld/DEZENTRAL.md 8-11): jeder der
// vier Validatoren praepariert seinen Abschnitt aus seiner EIGENEN Messquelle,
// das Werkzeug bekommt nur die Basen der Inhaberin -- niemand kennt den
// ganzen Token. Mit und ohne Ausfall eines Validators vor der ersten Vorlage.
func TestVierKnotenQuantenSAbschnitte_RealDB(t *testing.T) { qsVierKnotenModus(t, false, true, false) }
func TestVierKnotenQuantenSAbschnitteAusfall_RealDB(t *testing.T) {
	qsVierKnotenModus(t, true, true, false)
}

// GETRENNTE ROLLEN (quanten_s_rollen.go, DEZENTRAL.md 17, Schritt R3): wie
// die Abschnitte mit Stichprobe v2, aber jede Rolle hat ihren eigenen
// Schluessel und spricht mit einem ANDEREN Knoten -- der Ausgeber mit dem
// Folger, die Inhaberin mit einem dritten, beide Annahmestellen mit dem
// Leiter. Der Ausgeber sieht x nie, die Inhaberin prueft das Paket gegen die
// Kette, die Annahmestelle die Vorlage. Zuerst versucht eine boese Inhaberin
// die Doppelvorlage an der nicht gewaehlten Stelle (vorab und von der Kette
// abgewiesen, ein Fehlversuch), dann loest die gewaehlte Stelle ehrlich ein.
// Mit Ausfall: ein Validator faellt bei der ehrlichen Anmeldung aus (nachdem
// die Stelle ihre Stichprobe geladen hat); 3 von 4 legen offen.
func TestVierKnotenQuantenSRollen_RealDB(t *testing.T)        { qsVierKnotenModus(t, false, true, true) }
func TestVierKnotenQuantenSRollenAusfall_RealDB(t *testing.T) { qsVierKnotenModus(t, true, true, true) }

// qsAbschnitteAusVerzeichnis: im Pilot (quanten-s-pilot.yml, ausgabe =
// abschnitte) steht in AEQUITAS_QS_ABSCHNITTE das Verzeichnis von
// `hardware_ibm.py --s-abschnitte --ausgaben 2`: mess_<x>.json je Validator
// (beide Ausgaben), basen_<a>.json je Ausgabe fuer das Werkzeug. Jeder Test
// nimmt SEINE Ausgabe (ohne Ausfall 1, mit Ausfall 2): die Messquelle gibt
// jeden Abschnitt nur einmal her, eine zweite Ausgabe mit denselben Basen
// weist der Dienst ab ("keine Messung zu diesen Basen"). basen_<a>.json wird
// so streng gelesen wie im Werkzeug.
func qsAbschnitteAusVerzeichnis(t *testing.T, ausgabe int) (string, QuantenSBasen, bool) {
	dir := strings.TrimSpace(os.Getenv("AEQUITAS_QS_ABSCHNITTE"))
	if dir == "" {
		return "", QuantenSBasen{}, false
	}
	name := fmt.Sprintf("basen_%d.json", ausgabe)
	roh, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil || len(roh) > quantenSAusgabeMessMax {
		t.Fatalf("AEQUITAS_QS_ABSCHNITTE: %s: %v", name, err)
	}
	var b QuantenSBasen
	dec := json.NewDecoder(bytes.NewReader(roh))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		t.Fatalf("AEQUITAS_QS_ABSCHNITTE: %s: %v", name, err)
	}
	return dir, b, true
}

// qsV2AusUmgebung: im Pilot (quanten-s-pilot.yml, Eingabe stichprobe_v2)
// steht AEQUITAS_QS_V2=1: der v2-Weg (Stichprobe erst nach der Bindung,
// DEZENTRAL.md 16). Die Kettenregel schlaeft im Hauptnetz; NUR in diesem
// Testprozess (alle vier Knoten) wird sie scharf geschaltet und danach
// zurueckgesetzt. Nur im Abschnitts-Modus.
func qsV2AusUmgebung(t *testing.T, abschnitte bool) bool {
	switch strings.TrimSpace(os.Getenv("AEQUITAS_QS_V2")) {
	case "", "0":
		return false
	case "1":
		if !abschnitte {
			t.Fatal("AEQUITAS_QS_V2=1 nur im Abschnitts-Modus (ausgabe = abschnitte)")
		}
		quantenSProbeV2AbOverride.Store(1)
		t.Cleanup(func() { quantenSProbeV2AbOverride.Store(0) })
		t.Log("Stichprobe v2 (erst nach der Bindung)")
		return true
	default:
		t.Fatalf("AEQUITAS_QS_V2 = %q: 0 oder 1", os.Getenv("AEQUITAS_QS_V2"))
		return false
	}
}

// qsKettenSicht: was die Kette selbst zu einem S-Token festhaelt (nicht, was
// das Werkzeug meint): jede beobachtete Art der Stichprobe, die Zahl der
// Validatoren und die groesste Zahl Salze einer Anmeldung.
type qsKettenSicht struct {
	Arten []int
	NVal  int
	Salze int
}

// qsKetteBeobachten: liest bis zum Halt alle 200 ms das Register jedes
// lebenden Knotens. Begrenzt: eine Goroutine, sie endet mit halt (auch per
// t.Cleanup, falls der Test vorher abbricht). Erst nach halt wird die Sicht
// gelesen (kein gemeinsamer Zugriff).
func qsKetteBeobachten(t *testing.T, netz *vierKnotenNetz) (halt func() map[string]*qsKettenSicht) {
	gesehen := map[string]*qsKettenSicht{}
	stop, fertig := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(fertig)
		tick := time.NewTicker(200 * time.Millisecond)
		defer tick.Stop()
		for {
			for _, k := range netz.ks {
				if k.aus.Load() {
					continue
				}
				if cs, _, _ := k.teile(); cs != nil {
					qsRegisterLesen(cs, gesehen)
				}
			}
			select {
			case <-stop:
				return
			case <-tick.C:
			}
		}
	}()
	var einmal sync.Once
	halt = func() map[string]*qsKettenSicht {
		einmal.Do(func() { close(stop); <-fertig })
		return gesehen
	}
	t.Cleanup(func() { halt() })
	return halt
}

func qsRegisterLesen(cs *ChainState, gesehen map[string]*qsKettenSicht) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for K, e := range cs.quantenWJetzt().Token {
		if e == nil || e.token == nil {
			continue
		}
		s := gesehen[K]
		if s == nil {
			s = &qsKettenSicht{}
			gesehen[K] = s
		}
		neu := true
		for _, a := range s.Arten {
			if a == e.token.ProbeArt {
				neu = false
			}
		}
		if neu {
			s.Arten = append(s.Arten, e.token.ProbeArt)
		}
		s.NVal = e.token.NVal
		if a := e.token.Anmeldung; a != nil && len(a.Salze) > s.Salze {
			s.Salze = len(a.Salze)
		}
	}
}

// qsKettenSichtPruefen: v2 = auf jedem beobachteten Stand probe_art 2, und
// die Kette hat beim Anmelden je Validator ein Salz nachgerechnet; sonst
// probe_art 0 und kein Salz.
func qsKettenSichtPruefen(s *qsKettenSicht, v2 bool, validatoren int) error {
	art, salze := 0, 0
	if v2 {
		art, salze = 2, validatoren
	}
	if s == nil {
		return fmt.Errorf("Token nie im Register gesehen")
	}
	if len(s.Arten) != 1 || s.Arten[0] != art {
		return fmt.Errorf("probe_art %v auf der Kette, erwartet %d", s.Arten, art)
	}
	if s.NVal != validatoren || s.Salze != salze {
		return fmt.Errorf("%d Validatoren, Anmeldung mit %d Salzen; erwartet %d und %d", s.NVal, s.Salze, validatoren, salze)
	}
	return nil
}

// qsToleranzAusUmgebung: im Pilot (quanten-s-pilot.yml, Eingabe toleranz)
// steht in AEQUITAS_QS_TOL die Toleranz in Prozent; ohne Angabe der Standard.
// Nur Werte, fuer die die Kette eine Pruefgroesse kennt.
func qsToleranzAusUmgebung(t *testing.T) int {
	s := strings.TrimSpace(os.Getenv("AEQUITAS_QS_TOL"))
	if s == "" {
		return quantenWTolStandard
	}
	tol, err := strconv.Atoi(s)
	if err != nil || quantenWKNote(tol, 1, 1) == 0 {
		t.Fatalf("AEQUITAS_QS_TOL = %q: die Kette kennt nur %v", s, quantenSToleranzen())
	}
	t.Logf("Toleranz %d %%", tol)
	return tol
}

// PILOT AUF ECHTER HARDWARE (Workflow quanten-s-pilot.yml): Steht in
// AEQUITAS_QS_MESSUNG eine Datei von `hardware_ibm.py --s-messen`, kommt der
// Token nicht aus dem Zufall des Tests, sondern vom Chip -- Praegen bis
// Einloesen mit 3 von 4 Validatoren. Die Datei wird so streng gelesen wie
// in `aequitasd quanten-s-ausgabe` (Groesse, unbekannte Felder); was sie
// enthaelt, prueft das Werkzeug. k je Stelle = n/2, und die Tabelle wird
// NICHT ausser Kraft gesetzt: fuer 3 von 4 verlangt sie 5.200 Stellen je
// Annahmestelle, eine kleinere Messung weist das Werkzeug ab (fail-closed).
func qsMessungAusDatei(t *testing.T) (QuantenSMessung, bool) {
	pfad := strings.TrimSpace(os.Getenv("AEQUITAS_QS_MESSUNG"))
	if pfad == "" {
		return QuantenSMessung{}, false
	}
	f, err := os.Open(pfad)
	if err != nil {
		t.Fatalf("AEQUITAS_QS_MESSUNG: %v", err)
	}
	defer f.Close()
	roh, err := io.ReadAll(io.LimitReader(f, quantenSAusgabeMessMax+1))
	if err != nil || len(roh) > quantenSAusgabeMessMax {
		t.Fatal("AEQUITAS_QS_MESSUNG: unlesbar oder zu gross")
	}
	var m QuantenSMessung
	dec := json.NewDecoder(bytes.NewReader(roh))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("AEQUITAS_QS_MESSUNG: %v", err)
	}
	if m.N < 2 || m.N%2 != 0 {
		t.Fatalf("AEQUITAS_QS_MESSUNG: n = %d", m.N)
	}
	return m, true
}

// qsAusfallNetz: nimmt beim ersten Anmelden einen Knoten vom Netz.
type qsAusfallNetz struct {
	*quantenSMehrNetz
	weg  func()
	done bool
}

func (a *qsAusfallNetz) einreichen(ctx context.Context, art, wallet string, zeit int64, q QuantenWAngaben) error {
	if art == quantenWSAnmeldenTxArt && !a.done {
		a.done = true
		a.weg()
	}
	return a.quantenSMehrNetz.einreichen(ctx, art, wallet, zeit, q)
}

func qsVierKnoten(t *testing.T, ausfall bool) { qsVierKnotenModus(t, ausfall, false, false) }

func qsVierKnotenModus(t *testing.T, ausfall, abschnitte, rollen bool) {
	skipUnlessRealDBBenchEnv(t)
	if testing.Short() {
		t.Skip("laeuft in echter Zeit")
	}
	kJeStelle := 128
	tol := qsToleranzAusUmgebung(t)
	v2 := qsV2AusUmgebung(t, abschnitte)
	if rollen && !v2 {
		// Getrennte Rollen gibt es nur mit Stichprobe v2 -- auch ohne
		// AEQUITAS_QS_V2 nur in diesem Testprozess scharf.
		quantenSProbeV2AbOverride.Store(1)
		t.Cleanup(func() { quantenSProbeV2AbOverride.Store(0) })
		v2 = true
	}
	echt, ausDatei := qsMessungAusDatei(t)
	var abDir string
	var basen QuantenSBasen
	if abschnitte {
		ausDatei = false
		ausgabe := 1
		if ausfall {
			ausgabe = 2
		}
		abDir, basen, ausDatei = qsAbschnitteAusVerzeichnis(t, ausgabe)
		if ausDatei {
			kJeStelle = basen.N / 2
			t.Logf("Abschnitte aus %s, Ausgabe %d: n = %d, %d Abschnitte, Geraet %q, Jobs %q", abDir, ausgabe, basen.N, len(basen.Basen), basen.Wo, basen.Job)
		}
	} else if ausDatei {
		kJeStelle = echt.N / 2
		t.Logf("Messung aus %s: n = %d, k je Stelle = %d, Geraet %q, Job %q", os.Getenv("AEQUITAS_QS_MESSUNG"), echt.N, kJeStelle, echt.Wo, echt.Jo)
	}
	quantenWAbOverride.Store(nowUnix() - 86400)
	if !ausDatei {
		quantenWKNoteOverride.Store(int64(kJeStelle))
	}
	altBetreiber := quantenWBetreiberVon
	t.Cleanup(func() {
		quantenWAbOverride.Store(0)
		quantenWKNoteOverride.Store(0)
		quantenWBetreiberVon = altBetreiber
	})

	netz := neuesVierKnotenNetz(t)
	betreiber := map[string]string{}
	var validatoren []string
	for i, k := range netz.ks {
		betreiber[k.addr] = netz.sender[i].addr
		validatoren = append(validatoren, k.addr)
	}
	quantenWBetreiberVon = func(_ *ChainState, _ context.Context, signing string) (string, error) {
		return betreiber[strings.ToLower(signing)], nil
	}
	ausgeber := netz.sender[4]

	var urls []string
	for _, k := range netz.ks {
		urls = append(urls, k.srv.URL)
	}
	t.Setenv("QUANTEN_W_PEERS", strings.Join(urls, ","))
	ctx, halt := context.WithCancel(context.Background())
	t.Cleanup(halt)
	// Jeder Knoten fragt von SEINER Adresse aus ab (wie im Betrieb), nicht
	// alle von 127.0.0.1 -- sonst teilen sich vier Dienste eine Burst-Grenze.
	quantenWTestQuelle = func(apiBasis string) func(context.Context, string, string) (net.Conn, error) {
		u, err := url.Parse(apiBasis)
		if err != nil {
			return nil
		}
		ip := net.ParseIP(u.Hostname())
		if ip == nil {
			return nil
		}
		d := &net.Dialer{LocalAddr: &net.TCPAddr{IP: ip}, Timeout: 5 * time.Second}
		return d.DialContext
	}
	t.Cleanup(func() { quantenWTestQuelle = nil })
	var dienste []*quantenWDienst
	for _, k := range netz.ks {
		cs, dag, _ := k.teile()
		if err := cs.StarteQuantenWDienst(ctx, k.key, k.srv.URL, dag.Height, dag.QuantenWNachbarn); err != nil {
			t.Fatalf("Knoten %d: Validator-Dienst: %v", k.i, err)
		}
		dienste = append(dienste, cs.quantenWDienstJetzt())
	}
	// Abschnitte: jeder Dienst bekommt SEINE Messquelle (Validator x = Knoten
	// x-1 in der Reihenfolge der Validatoren unten).
	if abschnitte {
		n := 2 * kJeStelle
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		if !ausDatei {
			basen = QuantenSBasen{N: n, Wo: "vier Knoten (Speicher)"}
		}
		for i, d := range dienste {
			if ausDatei {
				d.messquelleSetzen(&quantenSMessDatei{pfad: filepath.Join(abDir, fmt.Sprintf("mess_%d.json", i+1))})
				continue
			}
			d.messquelleSetzen(&qsaMessung{r: rand.New(rand.NewSource(r.Int63()))})
			von, bis, _ := quantenSAbschnitt(n, len(dienste), i+1)
			y := make([]int, bis-von)
			for j := range y {
				y[j] = r.Intn(2)
			}
			basen.Basen = append(basen.Basen, qsBitsText(y))
		}
	}

	// Ehrliche Messung (ohne Rauschen): 2 * 128 Stellen.
	n := 2 * kJeStelle
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	var u, tw, y, x []int
	for i := 0; i < n; i++ {
		u, tw, y = append(u, r.Intn(2)), append(tw, r.Intn(2)), append(y, r.Intn(2))
		if y[i] == u[i] {
			x = append(x, tw[i])
		} else {
			x = append(x, r.Intn(2))
		}
	}
	m := QuantenSMessung{N: n, U: qsBitsText(u), T: qsBitsText(tw), Y: qsBitsText(y), X: qsBitsText(x), Wo: "vier Knoten"}
	if ausDatei {
		m = echt
	}

	// Das Netz des Werkzeugs: Bestaetigung je Validator an seiner API,
	// Einreichen und Stand beim Folger -- dieselben Wege wie im Betrieb.
	leiter := netz.warteAufLeiter(t, -1, 30*time.Second)
	folger := netz.ks[(leiter+1)%len(netz.ks)]
	var vs []QuantenSValidator
	for _, k := range netz.ks {
		vs = append(vs, QuantenSValidator{Adresse: k.addr, API: k.srv.URL})
	}
	adressen, netze, err := quantenSValidatorenPruefen(vs, 3)
	if err != nil {
		t.Fatal(err)
	}
	eigen, err := neuerQuantenSHTTP(folger.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range append(netze, eigen) {
		h.klient = qwKlient
		h.takt = 500 * time.Millisecond
	}
	// Frist je Schritt: 4 min fuer 128 Stellen; eine echte Messung in voller
	// Groesse (5.200 Stellen je Annahmestelle, 82 Teile der Offenlegung)
	// bekommt die Vorgabe des Werkzeugs im Hauptnetz (15 min je Schritt; auf
	// das Urteil wartet das Werkzeug bis zur Kettenfrist, quantenSUrteilFrist).
	frist := 4 * time.Minute
	if ausDatei {
		frist = 15 * time.Minute
	}
	var log bytes.Buffer
	var werkzeugNetz quantenSNetz = &quantenSMehrNetz{quantenSHTTP: eigen, validatoren: netze}
	var weg *vierKnoten
	if ausfall {
		weg = netz.ks[(leiter+2)%len(netz.ks)]
		werkzeugNetz = &qsAusfallNetz{quantenSMehrNetz: werkzeugNetz.(*quantenSMehrNetz), weg: func() {
			t.Logf("Knoten %d (%s) faellt aus -- vor der ersten Vorlage", weg.i, weg.addr)
			weg.aus.Store(true)
			weg.anhalten()
		}}
	}
	beobachterHalt := qsKetteBeobachten(t, netz)
	beginn := time.Now()
	var erg *QuantenSAusgabeErgebnis
	if rollen {
		erg = qsRollenVierKnoten(t, netz, leiter, ausgeber, adressen, netze, basen, tol, frist, ausfall, &weg)
	} else if abschnitte {
		erg, err = quantenSAusgabeLaufAbschnitte(context.Background(), QuantenSAusgabeAuftrag{Schluessel: ausgeber.key, Deckung: 2_000_000,
			Tol: tol, Frist: frist, ProbeV2: v2}, basen, adressen, 3, werkzeugNetz.(quantenSAbschnittNetz), &log)
	} else {
		erg, err = quantenSAusgabeLauf(context.Background(), QuantenSAusgabeAuftrag{Messung: m, Schluessel: ausgeber.key, Deckung: 2_000_000,
			Tol: tol, Frist: frist}, adressen, 3, werkzeugNetz, &log)
	}
	t.Logf("Werkzeug (%s):\n%s", time.Since(beginn).Round(time.Second), log.String())
	if err != nil {
		t.Fatal(err)
	}
	if erg.Doppelvorlage != "abgewiesen" || erg.Vorlage != "eingeloest" || (!rollen && erg.Stellen[erg.B] != ausgeber.addr) {
		t.Fatalf("Ergebnis: %+v", erg)
	}
	// Welcher Weg wirklich lief, sagt die Kette, nicht das Werkzeug.
	sicht := beobachterHalt()[erg.K]
	if err := qsKettenSichtPruefen(sicht, v2, len(netz.ks)); err != nil {
		t.Fatalf("Auf der Kette: %v", err)
	}
	t.Logf("Auf der Kette: probe_art %d, %d Validatoren, Anmeldung mit %d Salzen nachgerechnet", sicht.Arten[0], sicht.NVal, sicht.Salze)
	// Nachweis nach dem Ende auch im Weg, in dem das Werkzeug alle Rollen
	// spielt (Stichprobe v2, ohne Ausfall): gegen Wurzel und Proben der Ausgabe.
	if abschnitte && v2 && !rollen && !ausfall {
		var vs []QuantenSValidator
		for i, adr := range adressen {
			vs = append(vs, QuantenSValidator{Adresse: adr, API: netze[i].basis})
		}
		_, vnetze, err := quantenSValidatorenPruefen(vs, 1)
		if err != nil {
			t.Fatal(err)
		}
		ber, err := quantenSNachweisAbwarten(context.Background(), &quantenSMehrNetz{validatoren: vnetze}, erg.K, adressen, erg.Wurzel, erg.Proben, time.Now().Add(3*time.Minute), &log)
		if err != nil {
			t.Fatalf("Nachweis: %v", err)
		}
		if ber.WurzelGegen != "kette" || ber.Stelle == nil || *ber.Stelle != erg.B || ber.TrefferStelle == 0 {
			t.Fatalf("Nachweis-Bericht: %+v (gewaehlte Stelle %d)", ber, erg.B)
		}
		t.Logf("Nachweis nach dem Ende: alle %d Validatoren, Praeparation ergibt die Wurzel der Ausgabe; ueber alle %d Stellen %d Fehler bei %d Treffern, an Stelle %d %d bei %d",
			len(adressen), ber.N, ber.Fehler, ber.Treffer, erg.B, ber.FehlerStelle, ber.TrefferStelle)
	}

	// Was jeder Validator-Dienst getan hat (nur Zaehler): zeigt, ob sich
	// Zustaendiger und Ersatz gegenseitig behindert haben.
	for i, d := range dienste {
		z := d.Zaehlerstand()
		t.Logf("Dienst %d%s: eingereicht %d, abgewiesen %d, eingesprungen %d, Nachbarabfragen %d, Rest genutzt %d",
			i, map[bool]string{true: " (ausgefallen)", false: ""}[netz.ks[i].aus.Load()],
			z["eingereicht"], z["abgewiesen"], z["eingesprungen"], z["nachbar_abfragen"], z["rest_genutzt"])
	}
	if ausfall && (weg == nil || !weg.aus.Load()) {
		t.Fatal("der Ausfall ist nicht eingetreten")
	}
	// Jeder (lebende) Knoten: Token tot gemerkt, Toepfe leer. Das Werkzeug
	// urteilt aus der Sicht EINES Knotens; die anderen duerfen einen Block
	// hinterher sein (Pilot 17.11.2026: Knoten 0 hatte den Einloese-Block
	// beim ersten Lesen noch nicht). Darum begrenzt warten -- der volle
	// Gleichstand unten wird danach unveraendert streng geprueft.
	for _, k := range netz.ks {
		if k.aus.Load() {
			continue
		}
		cs, _, _ := k.teile()
		bis := time.Now().Add(2 * time.Minute)
		for {
			cs.mu.RLock()
			reg := cs.quantenWJetzt()
			_, tot := reg.Tote[erg.K]
			lebt := reg.Token[erg.K] != nil
			cs.mu.RUnlock()
			topf := int64(0)
			if a, ok := vierKonto(cs, quantenWTopfAddr); ok {
				topf = a.Balance.Micro()
			}
			if !lebt && tot && topf == 0 {
				break
			}
			if time.Now().After(bis) {
				t.Fatalf("Knoten %d nach 2 min: Token lebt=%v tot=%v, im W-Topf %d Mikro-AEQ", k.i, lebt, tot, topf)
			}
			time.Sleep(250 * time.Millisecond)
		}
	}
	// Jeder Dienst raeumt seinen Tresor.
	ende := time.Now().Add(30 * time.Second)
	for {
		leer := true
		for i, d := range dienste {
			if netz.ks[i].aus.Load() {
				continue
			}
			if len(d.schluessel()) != 0 {
				leer = false
			}
		}
		if leer {
			break
		}
		if time.Now().After(ende) {
			t.Fatal("Tresore nach dem Einloesen nicht geraeumt")
		}
		time.Sleep(200 * time.Millisecond)
	}
	stand := netz.pruefeGleichstand(t, "nach dem S-Token mit vier Validatoren")
	t.Logf("Variante S auf vier Knoten (3 von 4): ausgegeben, Doppelvorlage abgewiesen, eingeloest -- StateRoot %s", stand.root)
}

// Die Pruefung der Kettensicht selbst (ohne Datenbank): Sie darf einen Pilot
// nicht gruen lassen, wenn v2 verlangt war und die Kette etwas anderes sah.
func TestQsKettenSichtPruefen(t *testing.T) {
	gut := &qsKettenSicht{Arten: []int{2}, NVal: 4, Salze: 4}
	if err := qsKettenSichtPruefen(gut, true, 4); err != nil {
		t.Fatalf("Gutfall v2: %v", err)
	}
	if err := qsKettenSichtPruefen(&qsKettenSicht{Arten: []int{0}, NVal: 4}, false, 4); err != nil {
		t.Fatalf("Gutfall v1: %v", err)
	}
	for name, f := range map[string]func() error{
		"nie gesehen":           func() error { return qsKettenSichtPruefen(nil, true, 4) },
		"v2 verlangt, Kette v1": func() error { return qsKettenSichtPruefen(&qsKettenSicht{Arten: []int{0}, NVal: 4}, true, 4) },
		"v1 verlangt, Kette v2": func() error { return qsKettenSichtPruefen(gut, false, 4) },
		"Art gewechselt": func() error {
			return qsKettenSichtPruefen(&qsKettenSicht{Arten: []int{2, 0}, NVal: 4, Salze: 4}, true, 4)
		},
		"keine Art":              func() error { return qsKettenSichtPruefen(&qsKettenSicht{NVal: 4, Salze: 4}, true, 4) },
		"ohne Salze angemeldet":  func() error { return qsKettenSichtPruefen(&qsKettenSicht{Arten: []int{2}, NVal: 4}, true, 4) },
		"ein Salz zu wenig":      func() error { return qsKettenSichtPruefen(&qsKettenSicht{Arten: []int{2}, NVal: 4, Salze: 3}, true, 4) },
		"andere Validatorenzahl": func() error { return qsKettenSichtPruefen(&qsKettenSicht{Arten: []int{2}, NVal: 3, Salze: 3}, true, 4) },
		"v1 mit Salzen": func() error {
			return qsKettenSichtPruefen(&qsKettenSicht{Arten: []int{0}, NVal: 4, Salze: 4}, false, 4)
		},
	} {
		if f() == nil {
			t.Fatalf("%s: nicht erkannt", name)
		}
	}
}

// qsRollenVierKnoten: der Lauf mit getrennten Rollen ueber echtes HTTP.
// Jede Uebergabe (Bestellung, Paket, Vorlage) geht durch JSON wie eine Datei.
func qsRollenVierKnoten(t *testing.T, netz *vierKnotenNetz, leiter int, ausgeber testSchluessel, adressen []string, netze []*quantenSHTTP,
	basen QuantenSBasen, tol int, frist time.Duration, ausfall bool, weg **vierKnoten) *QuantenSAusgabeErgebnis {
	t.Helper()
	ctx := context.Background()
	api := func(k *vierKnoten) *quantenSHTTP {
		h, err := neuerQuantenSHTTP(k.srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		h.klient, h.takt = qwKlient, 500*time.Millisecond
		return h
	}
	knAusgeber := netz.ks[(leiter+1)%len(netz.ks)]
	knInhaberin := netz.ks[(leiter+3)%len(netz.ks)]
	knStellen := netz.ks[leiter]
	// Zwei Annahmestellen mit eigenen Schluesseln (keine Validatoren).
	var stellenKey [2]*ecdsa.PrivateKey
	var stellen [2]QuantenSStelle
	for s := range stellen {
		k, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		stellenKey[s] = k
		stellen[s] = QuantenSStelle{Adresse: quantenSAdresse(k), Schluessel: quantenSPubHex(k)}
	}
	// Inhaberin: neu. Mit Messungen aus dem Pilot (hardware_ibm.py) sind die
	// Basen die gemessenen -- die Messquelle kennt nur diese.
	dat, err := quantenSInhaberinNeu(basen.N, len(adressen), stellen, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(basen.Basen) == len(adressen) {
		dat.Bestellung.Basen = append([]string(nil), basen.Basen...)
		if err := dat.Bestellung.pruefen(len(adressen)); err != nil {
			t.Fatal(err)
		}
	}
	durch := func(was string, v, ziel interface{}) {
		roh, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if err := quantenSJSONLesen(bytes.NewReader(roh), was, ziel); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d Byte", was, len(roh))
	}
	var best QuantenSBestellung
	durch("Bestellung", dat.Bestellung, &best)

	var log bytes.Buffer
	defer func() { t.Logf("Rollen:\n%s", log.String()) }()
	// Ausgeber (Folger): Abschnitte, Bestaetigungen, Ausgabe. Kein Schluessel
	// der Inhaberin oder der Stellen.
	p, err := quantenSAusgabeFuerInhaberin(ctx, QuantenSAusgabeAuftrag{Schluessel: ausgeber.key, Deckung: 2_000_000, Tol: tol, Frist: frist, ProbeV2: true},
		best, adressen, 3, &quantenSMehrNetz{quantenSHTTP: api(knAusgeber), validatoren: netze}, &log)
	if err != nil {
		t.Fatal(err)
	}
	var paket QuantenSPaket
	durch("Paket des Ausgebers (hier nicht gebraucht)", p, &paket)
	// Inhaberin (dritter Knoten): holt mit nur K den Stand von IHRER Kette
	// und jeden Abschnitt direkt vom Validator, prueft alles und liest x --
	// das Paket des Ausgebers braucht sie nicht.
	inh := api(knInhaberin)
	inhNetz := &quantenSMehrNetz{quantenSHTTP: inh, validatoren: netze}
	vonKette, err := quantenSPaketVonKette(ctx, inhNetz, paket.K, adressen)
	if err != nil {
		t.Fatal(err)
	}
	if err := quantenSInhaberinAnnehmen(ctx, inh, dat, adressen, vonKette, &log); err != nil {
		t.Fatal(err)
	}
	b, err := quantenSZufallInt(2)
	if err != nil {
		t.Fatal(err)
	}
	vl, err := quantenSInhaberinEinloesen(ctx, inh, frist, dat, func() error { return nil }, b, &log)
	if err != nil {
		t.Fatal(err)
	}
	var vorlage QuantenSVorlage
	durch("Vorlage", vl, &vorlage)
	// Die boese Inhaberin unterschreibt selbst eine Vorlage fuer die andere
	// Stelle (ihr Werkzeug verweigert das).
	key, _ := dat.schluessel()
	x, _ := quantenSBitsText(dat.X, dat.Bestellung.N)
	boese := vorlage
	boese.B = 1 - b
	if boese.Vorlage, err = quantenSUnterschreiben(key, quantenSVorlageText(vorlage.K, 1-b, x)); err != nil {
		t.Fatal(err)
	}
	stelleNetz := func(mitAusfall bool) quantenSAbschnittNetz {
		m := &quantenSMehrNetz{quantenSHTTP: api(knStellen), validatoren: netze}
		if !mitAusfall {
			return m
		}
		*weg = netz.ks[(leiter+2)%len(netz.ks)]
		return &qsAusfallNetz{quantenSMehrNetz: m, weg: func() {
			t.Logf("Knoten %d (%s) faellt aus -- bei der ehrlichen Anmeldung, nachdem die Stelle ihre Stichprobe geladen hat", (*weg).i, (*weg).addr)
			(*weg).aus.Store(true)
			(*weg).anhalten()
		}}
	}
	erg := &QuantenSAusgabeErgebnis{K: paket.K, N: paket.N, Toleranz: tol, Ausgeber: paket.Ausgeber, Inhaberin: paket.Inhaberin, Stellen: paket.Stellen[:], B: b, Deckung: paket.Deckung}
	e1, err := quantenSAnnahmeLauf(ctx, QuantenSAusgabeAuftrag{Schluessel: stellenKey[1-b], Frist: frist}, boese, adressen, stelleNetz(false), &log)
	if err == nil || e1 == nil || e1.Vorab != "abgewiesen" || e1.Urteil != "abgewiesen" {
		t.Fatalf("Doppelvorlage an Stelle %d: %+v %v", 1-b, e1, err)
	}
	erg.Doppelvorlage = e1.Urteil
	e2, err := quantenSAnnahmeLauf(ctx, QuantenSAusgabeAuftrag{Schluessel: stellenKey[b], Frist: frist}, vorlage, adressen, stelleNetz(ausfall), &log)
	if err != nil {
		t.Fatal(err)
	}
	erg.Vorlage, erg.KJeStelle = e2.Urteil, e2.Stichprobe
	// Nachweis nach dem Ende (quanten_s_nachweis.go): ohne Ausfall legt jeder
	// Validator nach der Finalitaet seinen Abschnitt offen; zusammen ergeben
	// sie Wurzel und Proben der Ausgabe, und die Zaehlung an der Stelle ist
	// die der Annahmestelle.
	if !ausfall {
		var vs []QuantenSValidator
		for i, adr := range adressen {
			vs = append(vs, QuantenSValidator{Adresse: adr, API: netze[i].basis})
		}
		_, vnetze, err := quantenSValidatorenPruefen(vs, 1)
		if err != nil {
			t.Fatal(err)
		}
		ber, err := quantenSNachweisAbwarten(ctx, &quantenSMehrNetz{validatoren: vnetze}, paket.K, adressen, e2.Wurzel, e2.Proben, time.Now().Add(3*time.Minute), &log)
		if err != nil {
			t.Fatalf("Nachweis: %v", err)
		}
		if ber.WurzelGegen != "kette" || ber.Stelle == nil || *ber.Stelle != b || ber.TrefferStelle != e2.Treffer || ber.FehlerStelle != e2.Fehler {
			t.Fatalf("Nachweis-Bericht passt nicht zur Annahme: %+v (Annahme %d Fehler bei %d Treffern)", ber, e2.Fehler, e2.Treffer)
		}
		t.Logf("Nachweis nach dem Ende: alle %d Validatoren, Praeparation ergibt die Wurzel der Ausgabe; ueber alle %d Stellen %d Fehler bei %d Treffern, an Stelle %d %d bei %d",
			len(adressen), ber.N, ber.Fehler, ber.Treffer, b, ber.FehlerStelle, ber.TrefferStelle)
	}
	t.Logf("Rollen getrennt: Ausgeber %s (Knoten %d), Inhaberin %s (Knoten %d), Stellen %s/%s (Knoten %d); Doppelvorlage %s (vorab %s), ehrlich %s (vorab %s, %d Fehler bei %d Treffern)",
		paket.Ausgeber, knAusgeber.i, paket.Inhaberin, knInhaberin.i, stellen[0].Adresse, stellen[1].Adresse, knStellen.i, e1.Urteil, e1.Vorab, e2.Urteil, e2.Vorab, e2.Fehler, e2.Treffer)
	// Der Wert ging an die gewaehlte Stelle, nicht an die andere -- auf jedem
	// lebenden Knoten (begrenzt warten, wie unten fuer den Token).
	for _, k := range netz.ks {
		if k.aus.Load() {
			continue
		}
		cs, _, _ := k.teile()
		bis := time.Now().Add(2 * time.Minute)
		for {
			a, ok := vierKonto(cs, stellen[b].Adresse)
			if ok && a.Balance.Micro() > 0 {
				break
			}
			if time.Now().After(bis) {
				t.Fatalf("Knoten %d: die gewaehlte Stelle hat nichts bekommen", k.i)
			}
			time.Sleep(250 * time.Millisecond)
		}
		if a, ok := vierKonto(cs, stellen[1-b].Adresse); ok && a.Balance.Micro() != 0 {
			t.Fatalf("Knoten %d: die Stelle der Doppelvorlage hat %d Mikro-AEQ", k.i, a.Balance.Micro())
		}
	}
	return erg
}
