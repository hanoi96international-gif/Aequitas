package keeper

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Validator-Register, Schritt 2 (validator_bindung_annahme.go): Annahme und
// Selbstnachweis ohne Datenbank -- alles, was vor dem ersten Schreibzugriff
// abgewiesen werden muss.

func bindungsKoerper(t *testing.T, tx Transaction) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]interface{}{
		"operator": tx.Wallet, "signing": tx.To, "ts": tx.Nachweis.Zeit,
		"operator_signature": tx.Nachweis.Sig, "signing_signature": tx.Nachweis.Sig2,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func bindungPosten(a *APIServer, ip string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/validator-bindung", bytes.NewReader(body))
	req.RemoteAddr = ip + ":1234"
	rec := httptest.NewRecorder()
	a.handleValidatorBindung(rec, req)
	return rec
}

func selbstnachweis(t *testing.T, a *APIServer, wallet string) map[string]interface{} {
	t.Helper()
	rec := httptest.NewRecorder()
	a.handleValidatorSelfProof(rec, httptest.NewRequest(http.MethodGet, "/api/validator-selfproof?wallet="+wallet, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("Selbstnachweis: %d %s", rec.Code, rec.Body.String())
	}
	var d map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// Vor dem Stichtag aendert sich nichts: Annahme, Endpunkt und Selbstnachweis
// schweigen.
func TestValidatorBindung_SchlaeftVorStichtag(t *testing.T) {
	validatorRegisterOverride.Store(0)
	betreiber, bw := neuerSchluessel(t)
	knoten, _ := neuerSchluessel(t)
	tx := bindungUnterschrieben(t, betreiber, knoten, nowUnix())

	cs := newTestState()
	if err := cs.ValidatorBinden(tx); !errors.Is(err, errValidatorRegisterSchlaeft) {
		t.Fatalf("Annahme vor dem Stichtag: %v", err)
	}
	a := &APIServer{state: cs, blockchain: &BlockDAG{signingKey: knoten}}
	if rec := bindungPosten(a, "192.0.2.10", bindungsKoerper(t, tx)); rec.Code != http.StatusConflict {
		t.Fatalf("Endpunkt vor dem Stichtag: %d %s", rec.Code, rec.Body.String())
	}
	t.Setenv("NODE_OPERATOR_WALLET", bw)
	d := selbstnachweis(t, a, bw)
	for _, k := range []string{"bindung_zeit", "bindung_nachricht", "bindung_signatur_knoten"} {
		if _, ok := d[k]; ok {
			t.Fatalf("Selbstnachweis vor dem Stichtag enthaelt %s", k)
		}
	}
	if knotenBindungsNachweis(adrVon(knoten), bw, nowUnix(), nil) != nil {
		t.Fatal("knotenBindungsNachweis vor dem Stichtag nicht leer")
	}
}

// Ab dem Stichtag liefert der Selbstnachweis die Unterschrift des Knotens;
// mit der der Wallet ergibt sie eine Bindung, die jeder Nachspielende annimmt.
// Missbrauch: die Knotenunterschrift taugt fuer keinen anderen Betreiber.
func TestValidatorBindung_SelbstnachweisErgibtGueltigeBindung(t *testing.T) {
	validatorRegisterOverride.Store(1)
	t.Cleanup(func() { validatorRegisterOverride.Store(0) })
	betreiber, bw := neuerSchluessel(t)
	knoten, kw := neuerSchluessel(t)
	a := &APIServer{blockchain: &BlockDAG{signingKey: knoten}}
	t.Setenv("NODE_OPERATOR_WALLET", bw)

	d := selbstnachweis(t, a, bw)
	zeit := int64(d["bindung_zeit"].(float64))
	msg, _ := d["bindung_nachricht"].(string)
	knotenSig, _ := d["bindung_signatur_knoten"].(string)
	if msg != validatorBindungNachricht(kw, bw, zeit) {
		t.Fatalf("Satz %q, erwartet %q", msg, validatorBindungNachricht(kw, bw, zeit))
	}
	if d["signing_address"] != kw {
		t.Fatalf("signing_address %v, erwartet %s", d["signing_address"], kw)
	}
	tx := Transaction{Type: "validator_bindung", Wallet: bw, To: kw,
		Nachweis: &Auftragsnachweis{Zeit: zeit, Sig: personalSign(t, betreiber, msg), Sig2: knotenSig}}
	if err := validatorBindungForm(&tx); err != nil {
		t.Fatalf("Form: %v", err)
	}
	if err := pruefeAuftragsNachweis(&tx, zeit); err != nil {
		t.Fatalf("Nachweis: %v", err)
	}

	// Ein Fremder nimmt die Knotenunterschrift und setzt sich als Betreiber ein.
	fremd, fw := neuerSchluessel(t)
	falsch := Transaction{Type: "validator_bindung", Wallet: fw, To: kw,
		Nachweis: &Auftragsnachweis{Zeit: zeit, Sig: personalSign(t, fremd, validatorBindungNachricht(kw, fw, zeit)), Sig2: knotenSig}}
	if err := pruefeAuftragsNachweis(&falsch, zeit); !errors.Is(err, ErrUeberweisungNichtSigniert) {
		t.Fatalf("Knotenunterschrift fuer fremden Betreiber angenommen: %v", err)
	}
}

// Missbrauch an der Annahme: gefaelschte Unterschriften und Zeitpunkte
// ausserhalb der Annahmefrist werden vor jedem Schreibzugriff abgewiesen --
// auch ein Zeitpunkt, den das Nachspielen noch naehme (11 Minuten alt).
func TestValidatorBinden_Missbrauch(t *testing.T) {
	validatorRegisterOverride.Store(1)
	t.Cleanup(func() { validatorRegisterOverride.Store(0) })
	betreiber, _ := neuerSchluessel(t)
	knoten, _ := neuerSchluessel(t)
	fremd, _ := neuerSchluessel(t)
	jetzt := nowUnix()

	faelle := map[string]Transaction{
		"Betreiber gefaelscht": func() Transaction {
			tx := bindungUnterschrieben(t, betreiber, knoten, jetzt)
			tx.Nachweis.Sig = bindungUnterschrieben(t, fremd, knoten, jetzt).Nachweis.Sig
			return tx
		}(),
		"Schluessel gefaelscht": func() Transaction {
			tx := bindungUnterschrieben(t, betreiber, knoten, jetzt)
			tx.Nachweis.Sig2 = bindungUnterschrieben(t, betreiber, fremd, jetzt).Nachweis.Sig2
			return tx
		}(),
		"Zeitpunkt umgeschrieben": func() Transaction {
			tx := bindungUnterschrieben(t, betreiber, knoten, jetzt-120)
			tx.Nachweis.Zeit = jetzt
			return tx
		}(),
		"zu alt fuer die Annahme": bindungUnterschrieben(t, betreiber, knoten, jetzt-validatorBindungAnnahmeFrist-60),
		"zu weit voraus":          bindungUnterschrieben(t, betreiber, knoten, jetzt+nachweisHoechstensVoraus+60),
		"ohne Nachweis": func() Transaction {
			tx := bindungUnterschrieben(t, betreiber, knoten, jetzt)
			tx.Nachweis = nil
			return tx
		}(),
	}
	for name, tx := range faelle {
		t.Run(name, func(t *testing.T) {
			cs := newTestState()
			err := cs.ValidatorBinden(tx)
			if !errors.Is(err, ErrUeberweisungNichtSigniert) {
				t.Fatalf("erwartet ErrUeberweisungNichtSigniert, bekam %v", err)
			}
		})
	}

	// Eine gueltige Bindung ohne Datenbank wird nicht stillschweigend
	// angenommen.
	cs := newTestState()
	if err := cs.ValidatorBinden(bindungUnterschrieben(t, betreiber, knoten, jetzt)); err == nil {
		t.Fatal("Bindung ohne Datenbank angenommen")
	}
}

// Nur der Leiter nimmt an: das Register gehoert keinem Konto. Ohne Leitung
// nur ein Knoten, der ausdruecklich der Annehmende ist -- per Vorgabe nehmen
// sonst ALLE an (Sicherheitspruefung #298, LOW-3). Eine Ablehnung laesst die
// Zaehlung laufender Annahmen unveraendert (die Uebergabe wartet auf 0).
func TestAnnahmeBeginnenLeiter_NurDerLeiter(t *testing.T) {
	cs := newTestState()
	if err := cs.annahmeBeginnenLeiter(); !errors.Is(err, errKeinAlleinigerAnnehmer) {
		t.Fatalf("ohne Leitung und ohne ANNAHME_ROLLE=annehmend: %v", err)
	}
	cs.annehmendAusdruecklich.Store(true)
	if err := cs.annahmeBeginnenLeiter(); err != nil {
		t.Fatalf("ausdruecklich annehmend: %v", err)
	}
	cs.annahmeEnde()

	cs.leitung.Store(&Leitung{}) // Folger
	if err := cs.annahmeBeginnenLeiter(); !errors.Is(err, ErrNichtLeiter) {
		t.Fatalf("Folger: %v", err)
	}
	cs.leitung.Store(nil)
	cs.nurLesend.Store(true)
	if err := cs.annahmeBeginnenLeiter(); !errors.Is(err, ErrNurLesend) {
		t.Fatalf("nur lesend: %v", err)
	}
	if n := cs.annahmenLaufend.Load(); n != 0 {
		t.Fatalf("annahmenLaufend %d nach Ablehnungen", n)
	}
	t.Setenv(annahmeRolleEnv, "annehmend")
	if !annahmeRolleAusdruecklichAnnehmend() {
		t.Fatal("ANNAHME_ROLLE=annehmend nicht erkannt")
	}
	t.Setenv(annahmeRolleEnv, "")
	if annahmeRolleAusdruecklichAnnehmend() {
		t.Fatal("ohne Angabe gilt kein Knoten als ausdruecklich annehmend")
	}
}

// Signaturen: Kleinschreibung und v 0/1 werden angeglichen (dieselbe
// Unterzeichnerin), alles andere nicht.
func TestKanonischeSignaturVersuch(t *testing.T) {
	betreiber, _ := neuerSchluessel(t)
	knoten, _ := neuerSchluessel(t)
	tx := bindungUnterschrieben(t, betreiber, knoten, nowUnix())
	sig := tx.Nachweis.Sig
	v := sig[130:]
	roh := "0x" + strings.ToUpper(sig[2:130])
	if v == "1b" {
		roh += "00"
	} else {
		roh += "01"
	}
	if got := kanonischeSignaturVersuch(" " + roh + " "); got != sig {
		t.Fatalf("angeglichen %s, erwartet %s", got, sig)
	}
	if !kanonischeSignatur(kanonischeSignaturVersuch(roh)) {
		t.Fatal("angeglichene Signatur nicht kanonisch")
	}
	kurz := sig[:100]
	if got := kanonischeSignaturVersuch(kurz); got != kurz {
		t.Fatal("falsche Laenge veraendert")
	}
}

// Der Endpunkt: Methode, Groesse, Gleichzeitigkeit, Fehlversuch je IP auf dem
// ersten Knoten, Validatoren ausgenommen.
func TestHandleValidatorBindung_Grenzen(t *testing.T) {
	validatorRegisterOverride.Store(1)
	t.Cleanup(func() { validatorRegisterOverride.Store(0) })
	betreiber, _ := neuerSchluessel(t)
	knoten, _ := neuerSchluessel(t)
	fremd, _ := neuerSchluessel(t)
	a := &APIServer{state: newTestState()}
	a.state.annehmendAusdruecklich.Store(true)
	h := a.bindungsGrenze(a.handleValidatorBindung)
	ips := []string{"192.0.2.21", "192.0.2.22", "192.0.2.23", "192.0.2.24", "192.0.2.25"}
	t.Cleanup(func() {
		for _, ip := range ips {
			registerRateLimit.Delete("validator-bindung-fehl:" + ip)
		}
	})
	posten := func(ip string, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/validator-bindung", bytes.NewReader(body))
		req.RemoteAddr = ip + ":1234"
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec
	}

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/validator-bindung", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", rec.Code)
	}

	// Ueber 4 KB: eine gueltige Bindung, aufgefuellt -- die Groessengrenze
	// greift (nicht erst die Formpruefung).
	gut := bindungUnterschrieben(t, betreiber, knoten, nowUnix())
	var m map[string]interface{}
	json.Unmarshal(bindungsKoerper(t, gut), &m)
	m["fuell"] = strings.Repeat("x", 5000)
	gross, _ := json.Marshal(m)
	if rec := posten(ips[0], gross); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid request body") {
		t.Fatalf("5 KB Body: %d %s", rec.Code, rec.Body.String())
	}

	// Fehlversuch sperrt dieselbe IP 30 s -- auch fuer eine gueltige Bindung.
	falsch := bindungUnterschrieben(t, betreiber, knoten, nowUnix())
	falsch.Nachweis.Sig = bindungUnterschrieben(t, fremd, knoten, nowUnix()).Nachweis.Sig
	if rec := posten(ips[1], bindungsKoerper(t, falsch)); rec.Code != http.StatusBadRequest {
		t.Fatalf("gefaelschte Bindung: %d %s", rec.Code, rec.Body.String())
	}
	if rec := posten(ips[1], bindungsKoerper(t, gut)); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("nach Fehlversuch binnen 30 s: %d", rec.Code)
	}
	// Eine andere IP ist nicht betroffen.
	if rec := posten(ips[2], bindungsKoerper(t, gut)); rec.Code == http.StatusTooManyRequests {
		t.Fatal("Fehlversuch einer IP sperrt eine andere")
	}

	// Weitergeleitet von einem Validator (TCP-Adresse in der Freiliste): beim
	// Leiter kein IP-Limit -- sonst sperrte ein Fehlversuch hinter einem Folger
	// alle Betreiber, die ueber ihn kommen (#298, MEDIUM-2).
	rpcRateLimitFreiErgaenzen([]string{ips[3]})
	t.Cleanup(func() { m := map[string]bool{}; rpcRateLimitFreiListe.Store(&m) })
	for i := 0; i < 3; i++ {
		if rec := posten(ips[3], bindungsKoerper(t, falsch)); rec.Code != http.StatusBadRequest {
			t.Fatalf("weitergeleiteter Versuch %d: %d", i, rec.Code)
		}
	}

	// Gleichzeitigkeit: ueber der Grenze 503, ohne Pruefung.
	validatorBindungLaufend.Store(validatorBindungGleichzeitig)
	rec = posten(ips[4], bindungsKoerper(t, gut))
	validatorBindungLaufend.Store(0)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("ueber der Gleichzeitigkeitsgrenze: %d", rec.Code)
	}

	// Folger: wiederholbar (503), keine Zustandsaussage -- und kein Fehlversuch.
	b := &APIServer{state: newTestState()}
	b.state.annehmendAusdruecklich.Store(true)
	b.state.nurLesend.Store(true)
	hb := b.bindungsGrenze(b.handleValidatorBindung)
	req := httptest.NewRequest(http.MethodPost, "/api/validator-bindung", bytes.NewReader(bindungsKoerper(t, gut)))
	req.RemoteAddr = "192.0.2.26:1234"
	t.Cleanup(func() { registerRateLimit.Delete("validator-bindung-fehl:192.0.2.26") })
	rec = httptest.NewRecorder()
	hb(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Folger: erwartet 503, bekam %d %s", rec.Code, rec.Body.String())
	}
	if _, ok := registerRateLimit.Load("validator-bindung-fehl:192.0.2.26"); ok {
		t.Fatal("503 als Fehlversuch gezaehlt")
	}
}
