package keeper

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// QUANTENGELD VARIANTE W AUF VIER ECHTEN KNOTEN (Pilot im Kleinen).
//
// Vier Knoten wie in TestVierKnoten_RealDB (eigene Postgres-Datenbank,
// eigener Signierschluessel, echter HTTP-Server, Leitung), jeder mit seinem
// Validator-Dienst (quanten_w_validator.go, StarteQuantenWDienst wie in
// main.go: Tresor in der eigenen Datenbank, Einreichen ueber die eigene API,
// Nachbarn aus QUANTEN_W_PEERS). Der Test spielt nur die beiden
// Menschen: den Praeger und den, der einloest. Alles dazwischen -- Zufall,
// Anteile sammeln, Offenlegen -- muessen die Dienste selbst tun, ueber echtes
// HTTP, erst nach der Finalitaet, und jeder Knoten rechnet das Urteil nach.
//
// Wie im Vier-Knoten-Test: Loopback-Klienten und leitungDialLokal (siehe
// dessen Dateikopf). Dazu hier:
//   - quantenWAbOverride: der Stichtag liegt fuer diesen Test in der
//     Vergangenheit (im Betrieb MaxInt64).
//   - quantenWBetreiberVon: die vier Signieradressen sind an vier
//     Genesis-Menschen gebunden. Das Validator-Register selbst hat eigene
//     Tests (validator_register*_test.go); hier geht es um den Dienst.
//   - quantenWKNoteOverride = 128: die Tabelle verlangt fuer 3 von 4
//     Validatoren 5200 Stellen beim Einloesen, das waeren 82 Teile der
//     Offenlegung und Minuten Laufzeit. 128 Stellen sind zwei Teile -- der
//     Weg (Sammeln, Teil fuer Teil einreichen) ist derselbe. Die Tabelle
//     selbst pruefen quanten_w_test.go und die Spezifikation.
func TestVierKnotenQuantenW_RealDB(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	if testing.Short() {
		t.Skip("laeuft in echter Zeit")
	}
	const kNote = 128
	quantenWAbOverride.Store(nowUnix() - 86400)
	quantenWKNoteOverride.Store(kNote)
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
	praeger, halter := netz.sender[4], netz.sender[5]

	// Die Dienste, je Knoten einer, gestartet wie in main.go.
	var urls []string
	for _, k := range netz.ks {
		urls = append(urls, k.srv.URL)
	}
	t.Setenv("QUANTEN_W_PEERS", strings.Join(urls, ","))
	ctx, halt := context.WithCancel(context.Background())
	var dienste []*quantenWDienst
	t.Cleanup(halt) // vor dem Anhalten der Knoten (deren Cleanup laeuft danach)
	for _, k := range netz.ks {
		cs, dag, _ := k.teile()
		if err := cs.StarteQuantenWDienst(ctx, k.key, k.srv.URL, dag.Height); err != nil {
			t.Fatalf("Knoten %d: Validator-Dienst: %v", k.i, err)
		}
		d := cs.quantenWDienstJetzt()
		if d == nil || d.adresse != k.addr {
			t.Fatalf("Knoten %d: kein Validator-Dienst fuer %s", k.i, k.addr)
		}
		dienste = append(dienste, d)
	}
	// Die Note: 400 Stellen, 3 von 4 Validatoren.
	p := qwPraegen(400, 3, 4, time.Now().UnixNano()%30000)
	zeit := nowUnix()
	wurzel := hex.EncodeToString(p.wurzel[:])
	auftrag := func(x int) QuantenWBestaetigenAnfrage {
		r := QuantenWBestaetigenAnfrage{K: p.K, N: p.n, T: p.t, Tol: quantenWTolStandard, X: x, Wurzel: wurzel, Validatoren: validatoren, Zeit: zeit, Praeger: praeger.addr}
		for i := 0; i < p.n; i++ {
			r.Anteile = append(r.Anteile, p.anteile[i][x-1].Text(16))
			r.Blaetter = append(r.Blaetter, hex.EncodeToString(p.blaetter[i][:]))
		}
		r.PraegerSig = personalSign(t, praeger.key, quantenWAuftragNachricht(praeger.addr, p.K, p.n, p.t, r.Tol, x, wurzel, validatoren, zeit, quantenWAnteileHash(r.Anteile)))
		return r
	}

	// Missbrauch ueber echtes HTTP, bevor irgendetwas bestaetigt ist: der
	// Auftrag fuer einen anderen Validator, und ein Auftrag ohne Unterschrift.
	if c, b := qwPost(t, netz.ks[0].srv.URL, "/api/quanten-w/validator/bestaetigen", auftrag(2)); c == http.StatusOK {
		t.Fatalf("Knoten 0 bestaetigt den Auftrag fuer Nummer 2: %s", b)
	}
	ohne := auftrag(1)
	ohne.PraegerSig = ""
	if c, b := qwPost(t, netz.ks[0].srv.URL, "/api/quanten-w/validator/bestaetigen", ohne); c == http.StatusOK {
		t.Fatalf("Auftrag ohne Unterschrift bestaetigt: %s", b)
	}
	if len(dienste[0].schluessel()) != 0 {
		t.Fatal("abgewiesen und trotzdem verwahrt")
	}

	// Hinterlegen bei allen vier, praegen mit ihren Bestaetigungen -- beim
	// Folger eingereicht, er leitet an den Leiter weiter.
	q := &QuantenWAngaben{K: p.K, N: p.n, T: p.t, Wurzel: wurzel, Validatoren: validatoren, Deckung: 10_000_000, Tol: quantenWTolStandard}
	for x, k := range netz.ks {
		c, b := qwPost(t, k.srv.URL, "/api/quanten-w/validator/bestaetigen", auftrag(x+1))
		if c != http.StatusOK {
			t.Fatalf("Knoten %d bestaetigt nicht: %d %s", k.i, c, b)
		}
		var best struct{ Validator, Anker, Bestaetigung string }
		if err := json.Unmarshal(b, &best); err != nil || best.Validator != k.addr {
			t.Fatalf("Bestaetigung von Knoten %d: %s", k.i, b)
		}
		q.Anker = append(q.Anker, best.Anker)
		q.Bestaetigungen = append(q.Bestaetigungen, best.Bestaetigung)
	}
	q.WalletSig = personalSign(t, praeger.key, quantenWPraegenNachricht(praeger.addr, p.K, p.n, p.t, wurzel, validatoren, q.Anker, q.Deckung, zeit, q.Tol))
	leiter := netz.warteAufLeiter(t, -1, 30*time.Second)
	folger := netz.ks[(leiter+1)%len(netz.ks)]
	einreichen := func(art, wallet string, zeit int64, q *QuantenWAngaben) {
		t.Helper()
		// Ein Folger, der gerade nicht weiterleiten kann, antwortet 503: noch
		// einmal, hoechstens 30 s. Jede andere Ablehnung ist ein Fehler.
		ende := time.Now().Add(30 * time.Second)
		for {
			c, b := qwPost(t, folger.srv.URL, "/api/quanten-w/"+strings.TrimPrefix(art, "quanten_w_"),
				map[string]interface{}{"wallet": wallet, "zeit": zeit, "quanten_w": q})
			if c == http.StatusOK {
				return
			}
			if c != http.StatusServiceUnavailable || time.Now().After(ende) {
				t.Fatalf("%s: %d %s", art, c, b)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	einreichen(quantenWPraegenTxArt, praeger.addr, zeit, q)

	// Der Halter loest ein (k_note aus der Tabelle der Note).
	note := func(k *vierKnoten) (*quantenWNote, bool) {
		cs, _, _ := k.teile()
		cs.mu.RLock()
		defer cs.mu.RUnlock()
		reg := cs.quantenWJetzt()
		if e := reg.Noten[p.K]; e != nil {
			return e.note, false
		}
		_, tot := reg.Tote[p.K]
		return nil, tot
	}
	warte := func(was string, frist time.Duration, f func() bool) {
		t.Helper()
		ende := time.Now().Add(frist)
		for !f() {
			if time.Now().After(ende) {
				t.Fatalf("nach %s: %s nicht eingetreten", frist, was)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	warte("die Note steht bei allen vier im Register", 60*time.Second, func() bool {
		for _, k := range netz.ks {
			if n, _ := note(k); n == nil {
				return false
			}
		}
		return true
	})
	zeitA := nowUnix()
	einreichen(quantenWAnfrageTxArt, halter.addr, zeitA, &QuantenWAngaben{K: p.K, Index: 0, Stellen: kNote, An: halter.addr,
		WalletSig: personalSign(t, halter.key, quantenWAnfrageNachricht(halter.addr, p.K, 0, kNote, halter.addr, zeitA))})
	// Vor der Antwort gibt kein Validator Anteile heraus.
	for _, k := range netz.ks {
		if c, b := qwGet(t, k.srv.URL, "/api/quanten-w/validator/anteile?k="+p.K+"&index=0&ab=0"); c == http.StatusOK {
			t.Fatalf("Knoten %d gibt vor der Antwort Anteile heraus: %s", k.i, b)
		}
	}

	// Die Dienste liefern den Zufall von selbst, sobald die Anfrage final ist.
	var stellen, basen []int
	warte("der Zufall aller vier Validatoren (Aufforderung steht)", 180*time.Second, func() bool {
		n, _ := note(folger)
		if n == nil || n.Anfrage == nil || n.Anfrage.Basen == nil {
			return false
		}
		stellen, basen = append([]int(nil), n.Anfrage.Stellen...), append([]int(nil), n.Anfrage.Basen...)
		return true
	})
	if len(stellen) != kNote {
		t.Fatalf("Aufforderung mit %d Stellen", len(stellen))
	}
	t.Logf("Aufforderung steht: %d Stellen, Zufall von allen vier Validatoren", len(stellen))

	var bits strings.Builder
	for _, b := range p.ehrlich(stellen, basen) {
		bits.WriteByte(byte('0' + b))
	}
	einreichen(quantenWAntwortTxArt, halter.addr, 0, &QuantenWAngaben{K: p.K, Index: 0, Bits: bits.String(),
		WalletSig: personalSign(t, halter.key, quantenWAntwortNachricht(halter.addr, p.K, 0, bits.String()))})
	// Die Antwort ist angenommen, aber nicht final: noch keine Anteile.
	for _, k := range netz.ks {
		if c, b := qwGet(t, k.srv.URL, "/api/quanten-w/validator/anteile?k="+p.K+"&index=0&ab=0"); c == http.StatusOK {
			t.Fatalf("Knoten %d gibt vor der Finalitaet der Antwort Anteile heraus: %s", k.i, b)
		}
	}

	// Die Dienste sammeln und legen offen; jeder Knoten urteilt: echt.
	vorher := map[string]float64{}
	for _, k := range netz.ks {
		cs, _, _ := k.teile()
		if acc, ok := vierKonto(cs, halter.addr); ok {
			vorher[k.addr] = acc.Balance.Float()
		}
	}
	warte("die Note ist bei allen vier eingeloest", 240*time.Second, func() bool {
		for _, k := range netz.ks {
			if n, tot := note(k); n != nil || !tot {
				return false
			}
		}
		return true
	})
	for _, k := range netz.ks {
		cs, _, _ := k.teile()
		acc, ok := vierKonto(cs, halter.addr)
		if !ok {
			t.Fatalf("Knoten %d kennt den Halter nicht", k.i)
		}
		// Das Pfand (3,2 AEQ) ging beim Anfragen ab und kommt zurueck; dazu
		// der Wert der Note. Gegenueber dem Stand MIT gesperrtem Pfand also
		// Pfand + Wert; die Umlaufabgabe einer Minute liegt weit unter 0,1 AEQ.
		if plus := acc.Balance.Float() - vorher[k.addr]; plus < 13.1 || plus > 13.2001 {
			t.Fatalf("Knoten %d: der Halter hat %+.6f AEQ bekommen, erwartet Pfand 3,2 (10 AEQ * 128 / 400) + Wert ~10", k.i, plus)
		}
		for _, topf := range []string{quantenWTopfAddr, quantenWPfandAddr} {
			if a, ok := vierKonto(cs, topf); ok && a.Balance.Micro() != 0 {
				t.Fatalf("Knoten %d: im Topf %s liegen noch %d Mikro-AEQ", k.i, topf, a.Balance.Micro())
			}
		}
	}
	// Jeder Dienst raeumt seinen Tresor: die Anteile einer toten Note
	// braucht niemand mehr.
	warte("alle Tresore sind geraeumt", 30*time.Second, func() bool {
		for _, v := range dienste {
			n := len(v.schluessel())
			if n != 0 {
				return false
			}
		}
		return true
	})
	for _, k := range netz.ks {
		cs, _, _ := k.teile()
		var zeilen int
		if err := cs.db.QueryRow(`SELECT COUNT(*) FROM quanten_w_tresor`).Scan(&zeilen); err != nil || zeilen != 0 {
			t.Fatalf("Knoten %d: quanten_w_tresor hat noch %d Zeilen (%v)", k.i, zeilen, err)
		}
	}
	stand := netz.pruefeGleichstand(t, "nach dem Einloesen der Quanten-Note")
	t.Logf("Variante W auf vier Knoten: gepraegt, angefragt, Zufall, Antwort, Offenlegung, eingeloest -- StateRoot %s", stand.root)
}

var qwKlient = &http.Client{Timeout: 15 * time.Second, CheckRedirect: ohneUmleitung,
	Transport: &http.Transport{DialContext: vierDial, MaxIdleConnsPerHost: 4}}

func qwPost(t *testing.T, basis, pfad string, v interface{}) (int, []byte) {
	t.Helper()
	roh, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := qwKlient.Post(basis+pfad, "application/json", bytes.NewReader(roh))
	if err != nil {
		t.Fatalf("POST %s: %v", pfad, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, b
}

func qwGet(t *testing.T, basis, pfad string) (int, []byte) {
	t.Helper()
	resp, err := qwKlient.Get(basis + pfad)
	if err != nil {
		t.Fatalf("GET %s: %v", pfad, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, fmt.Appendf(nil, "%s", b)
}
