package keeper

import (
	"bytes"
	"context"
	"math/rand"
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
	const kJeStelle = 128
	quantenWAbOverride.Store(nowUnix() - 86400)
	quantenWKNoteOverride.Store(kJeStelle)
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
	erg, err := quantenSAusgabeLauf(context.Background(), QuantenSAusgabeAuftrag{Messung: m, Schluessel: ausgeber.key, Deckung: 2_000_000,
		Tol: quantenWTolStandard, Frist: 4 * time.Minute}, adressen, 3, werkzeugNetz, &log)
	t.Logf("Werkzeug:\n%s", log.String())
	if err != nil {
		t.Fatal(err)
	}
	if erg.Doppelvorlage != "abgewiesen" || erg.Vorlage != "eingeloest" || erg.Stellen[erg.B] != ausgeber.addr {
		t.Fatalf("Ergebnis: %+v", erg)
	}

	if ausfall && (weg == nil || !weg.aus.Load()) {
		t.Fatal("der Ausfall ist nicht eingetreten")
	}
	// Jeder (lebende) Knoten: Token tot gemerkt, Toepfe leer.
	for _, k := range netz.ks {
		if k.aus.Load() {
			continue
		}
		cs, _, _ := k.teile()
		cs.mu.RLock()
		reg := cs.quantenWJetzt()
		_, tot := reg.Tote[erg.K]
		lebt := reg.Token[erg.K] != nil
		cs.mu.RUnlock()
		if lebt || !tot {
			t.Fatalf("Knoten %d: Token lebt=%v tot=%v", k.i, lebt, tot)
		}
		if a, ok := vierKonto(cs, quantenWTopfAddr); ok && a.Balance.Micro() != 0 {
			t.Fatalf("Knoten %d: im W-Topf liegen noch %d Mikro-AEQ", k.i, a.Balance.Micro())
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
