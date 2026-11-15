package keeper

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Audit B8: die Unterschrift fuer die Treuhand-Rueckholung war ohne Zeit und
// Kette wiederverwendbar. Form 2 bindet Kette, Wallet und Zeit; jeder Knoten
// prueft das Zeitfenster gegen die Blockzeit.
func TestTreuhandV2_Nachspielen(t *testing.T) {
	key, wallet := neuerSchluessel(t)
	jetzt := int64(1_800_000_000)
	tx := func(n Auftragsnachweis) *Transaction {
		return &Transaction{Type: "escrow_recover", Wallet: wallet, Nachweis: &n}
	}
	v2 := Auftragsnachweis{Form: auftragForm2, Zeit: jetzt, Sig: personalSign(t, key, treuhandZurueckNachrichtV2(wallet, jetzt))}
	if err := pruefeAuftragsNachweis(tx(v2), jetzt+60); err != nil {
		t.Fatalf("gueltige Form 2 abgewiesen: %v", err)
	}
	// Wiederholung lange danach: dieselbe Unterschrift gilt nicht mehr.
	if err := pruefeAuftragsNachweis(tx(v2), jetzt+nachweisHoechstensAlt+1); !errors.Is(err, ErrUeberweisungNichtSigniert) {
		t.Fatalf("alte Form-2-Unterschrift spaeter angenommen: %v", err)
	}
	// Aus der Zukunft.
	if err := pruefeAuftragsNachweis(tx(v2), jetzt-nachweisHoechstensVoraus-1); err == nil {
		t.Fatal("Unterschrift aus der Zukunft angenommen")
	}
	// Zeit umgeschrieben: Unterschrift passt nicht.
	umgeschrieben := v2
	umgeschrieben.Zeit = jetzt + 100
	if err := pruefeAuftragsNachweis(tx(umgeschrieben), jetzt+100); err == nil {
		t.Fatal("umgeschriebene Zeit angenommen")
	}
	// Eine Unterschrift fuer eine andere Wallet zaehlt nicht.
	_, fremd := neuerSchluessel(t)
	andere := &Transaction{Type: "escrow_recover", Wallet: fremd, Nachweis: &v2}
	if err := pruefeAuftragsNachweis(andere, jetzt); err == nil {
		t.Fatal("Unterschrift fuer eine andere Wallet angenommen")
	}
	// Form 1 bleibt fuer bestehende Bloecke gueltig.
	v1 := Auftragsnachweis{Sig: personalSign(t, key, "Aequitas: recover escrow "+wallet)}
	if err := pruefeAuftragsNachweis(tx(v1), jetzt); err != nil {
		t.Fatalf("Form 1 (alte Bloecke) abgewiesen: %v", err)
	}
	// Unbekannte Form, Form 2 ohne Zeit, Min gesetzt: abgewiesen.
	for name, n := range map[string]Auftragsnachweis{
		"Form 3":           {Form: 3, Zeit: jetzt, Sig: v2.Sig},
		"Form 2 ohne Zeit": {Form: auftragForm2, Sig: personalSign(t, key, treuhandZurueckNachrichtV2(wallet, 0))},
		"Min gesetzt":      {Form: auftragForm2, Zeit: jetzt, Min: 1, Sig: v2.Sig},
		"Form 1 mit Min":   {Min: 1, Sig: v1.Sig},
	} {
		if err := pruefeAuftragsNachweis(tx(n), jetzt); err == nil {
			t.Errorf("%s angenommen", name)
		}
	}
}

func TestTreuhandV2_Annahme(t *testing.T) {
	key, wallet := neuerSchluessel(t)
	a := &APIServer{state: newTestState()}
	ip := 0
	post := func(body map[string]interface{}) (int, string) {
		ip++
		roh, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, "/api/recover-escrow", bytes.NewReader(roh))
		r.RemoteAddr = "203.0.113." + string(rune('0'+ip)) + ":1"
		w := httptest.NewRecorder()
		a.handleRecoverEscrow(w, r)
		return w.Code, w.Body.String()
	}
	jetzt := time.Now().Unix()
	alt := jetzt - nachweisAnnahmeHoechstensAlt - 5
	faelle := map[string]map[string]interface{}{
		"alte Zeit": {"wallet": wallet, "form": 2, "timestamp": alt,
			"signature": personalSign(t, key, treuhandZurueckNachrichtV2(wallet, alt))},
		"Zukunft": {"wallet": wallet, "form": 2, "timestamp": jetzt + 3600,
			"signature": personalSign(t, key, treuhandZurueckNachrichtV2(wallet, jetzt+3600))},
		"Zeit nicht unterschrieben": {"wallet": wallet, "form": 2, "timestamp": jetzt,
			"signature": personalSign(t, key, treuhandZurueckNachrichtV2(wallet, jetzt-1))},
		"Form 1 mit Zeit": {"wallet": wallet, "timestamp": jetzt,
			"signature": personalSign(t, key, "Aequitas: recover escrow "+wallet)},
		"unbekannte Form": {"wallet": wallet, "form": 7, "timestamp": jetzt,
			"signature": personalSign(t, key, treuhandZurueckNachrichtV2(wallet, jetzt))},
	}
	for name, body := range faelle {
		if c, b := post(body); c != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, c, b)
		}
	}
	// Gueltige Form 2: kommt durch alle Pruefungen bis zur Treuhand selbst
	// (hier ohne Datenbank -- dort endet sie mit "no database").
	if _, b := post(map[string]interface{}{"wallet": wallet, "form": 2, "timestamp": jetzt,
		"signature": personalSign(t, key, treuhandZurueckNachrichtV2(wallet, jetzt))}); !strings.Contains(b, "no database") {
		t.Fatalf("gueltige Form 2 vor der Treuhand abgewiesen: %s", b)
	}
	// Die abgewiesenen Faelle kamen dort nie an.
	for name, body := range faelle {
		if _, b := post(body); strings.Contains(b, "no database") {
			t.Errorf("%s erreichte die Treuhand", name)
		}
	}
}

func TestTreuhandV2_FesterSatz(t *testing.T) {
	if got := treuhandZurueckNachrichtV2("0xAbCdEf0123456789aBcDeF0123456789AbCdEf01", 1794748500); got !=
		"aequitas-treuhand-zurueck-v2|chain:1926|0xabcdef0123456789abcdef0123456789abcdef01|ts:1794748500" {
		t.Fatalf("Satz: %s", got)
	}
}
