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

// Nur der Leiter nimmt an: das Register gehoert keinem Konto. Eine Ablehnung
// laesst die Zaehlung laufender Annahmen unveraendert (die Uebergabe wartet
// auf 0).
func TestAnnahmeBeginnenLeiter_NurDerLeiter(t *testing.T) {
	cs := newTestState()
	if err := cs.annahmeBeginnenLeiter(); err != nil {
		t.Fatalf("ohne Leitung (ein Knoten): %v", err)
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
}

// Der Endpunkt: Methode, Groesse, Rate-Limit, Fehlerklassen.
func TestHandleValidatorBindung_Grenzen(t *testing.T) {
	validatorRegisterOverride.Store(1)
	t.Cleanup(func() { validatorRegisterOverride.Store(0) })
	betreiber, _ := neuerSchluessel(t)
	knoten, _ := neuerSchluessel(t)
	fremd, _ := neuerSchluessel(t)
	a := &APIServer{state: newTestState()}
	ips := []string{"192.0.2.21", "192.0.2.22", "192.0.2.23", "192.0.2.24"}
	t.Cleanup(func() {
		for _, ip := range ips {
			registerRateLimit.Delete("validator-bindung:" + ip)
		}
	})

	rec := httptest.NewRecorder()
	a.handleValidatorBindung(rec, httptest.NewRequest(http.MethodGet, "/api/validator-bindung", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", rec.Code)
	}

	gross := []byte(`{"operator":"` + strings.Repeat("a", 5000) + `"}`)
	if rec := bindungPosten(a, ips[0], gross); rec.Code != http.StatusBadRequest {
		t.Fatalf("5 KB Body: %d", rec.Code)
	}

	falsch := bindungUnterschrieben(t, betreiber, knoten, nowUnix())
	falsch.Nachweis.Sig = bindungUnterschrieben(t, fremd, knoten, nowUnix()).Nachweis.Sig
	if rec := bindungPosten(a, ips[1], bindungsKoerper(t, falsch)); rec.Code != http.StatusBadRequest {
		t.Fatalf("gefaelschte Bindung: %d %s", rec.Code, rec.Body.String())
	}
	// Dieselbe Adresse gleich danach: Rate-Limit, ohne Pruefung.
	if rec := bindungPosten(a, ips[1], bindungsKoerper(t, falsch)); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("zweite Anfrage binnen 30 s: %d", rec.Code)
	}

	// Folger: wiederholbar (503), keine Zustandsaussage.
	b := &APIServer{state: newTestState()}
	b.state.nurLesend.Store(true)
	gut := bindungUnterschrieben(t, betreiber, knoten, nowUnix())
	if rec := bindungPosten(b, ips[2], bindungsKoerper(t, gut)); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Folger: erwartet 503, bekam %d %s", rec.Code, rec.Body.String())
	}
}
