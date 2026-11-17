package keeper

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/rand"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
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

func qsVierKnoten(t *testing.T, ausfall bool) {
	skipUnlessRealDBBenchEnv(t)
	if testing.Short() {
		t.Skip("laeuft in echter Zeit")
	}
	kJeStelle := 128
	echt, ausDatei := qsMessungAusDatei(t)
	if ausDatei {
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
	beginn := time.Now()
	erg, err := quantenSAusgabeLauf(context.Background(), QuantenSAusgabeAuftrag{Messung: m, Schluessel: ausgeber.key, Deckung: 2_000_000,
		Tol: quantenWTolStandard, Frist: frist}, adressen, 3, werkzeugNetz, &log)
	t.Logf("Werkzeug (%s):\n%s", time.Since(beginn).Round(time.Second), log.String())
	if err != nil {
		t.Fatal(err)
	}
	if erg.Doppelvorlage != "abgewiesen" || erg.Vorlage != "eingeloest" || erg.Stellen[erg.B] != ausgeber.addr {
		t.Fatalf("Ergebnis: %+v", erg)
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
