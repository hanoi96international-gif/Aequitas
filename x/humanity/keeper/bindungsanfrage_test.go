package keeper

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

func personalSign(t *testing.T, k *ecdsa.PrivateKey, msg string) string {
	t.Helper()
	sig, err := crypto.Sign(accounts.TextHash([]byte(msg)), k)
	if err != nil {
		t.Fatal(err)
	}
	sig[64] += 27
	return hexutil.Encode(sig)
}

func adrVon(k *ecdsa.PrivateKey) string {
	return strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())
}

type anfrageWelt struct {
	a      *APIServer
	mensch *ecdsa.PrivateKey
	wallet string
}

func neueAnfrageWelt(t *testing.T) anfrageWelt {
	t.Helper()
	bindungsAnfragen = &bindungsAnfragenTyp{eintrag: map[string]bindungsAnfrage{}}
	cs := newTestState()
	m, _ := crypto.GenerateKey()
	w := adrVon(m)
	cs.accounts.Set(w, &AccountState{Address: w, Balance: NewDecimal(1000), IsHuman: true})
	return anfrageWelt{a: &APIServer{state: cs}, mensch: m, wallet: w}
}

func (wl anfrageWelt) anfrage(t *testing.T, ip string, knoten *ecdsa.PrivateKey, wallet, beweis string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"signing_address": adrVon(knoten), "wallet": wallet, "beweis": beweis})
	r := httptest.NewRequest("POST", "/api/bindungsanfrage", bytes.NewReader(body))
	r.RemoteAddr = ip + ":1234"
	w := httptest.NewRecorder()
	wl.a.handleBindungsanfrage(w, r)
	return w.Code
}

func (wl anfrageWelt) offen(t *testing.T) []bindungsAnfrage {
	t.Helper()
	r := httptest.NewRequest("GET", "/api/bindungsanfragen?wallet="+wl.wallet, nil)
	w := httptest.NewRecorder()
	wl.a.handleBindungsanfragen(w, r)
	var out struct{ Anfragen []bindungsAnfrage }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
	return out.Anfragen
}

// Gutfall: ein Knoten meldet sich mit gueltigem Nachweis, die App sieht die
// Anfrage samt IP des Servers.
func TestBindungsanfrage_Gutfall(t *testing.T) {
	wl := neueAnfrageWelt(t)
	k, _ := crypto.GenerateKey()
	beweis := personalSign(t, k, "Aequitas: validator key linked to human "+wl.wallet)
	if c := wl.anfrage(t, "203.0.113.7", k, wl.wallet, beweis); c != 200 {
		t.Fatalf("Status %d", c)
	}
	o := wl.offen(t)
	if len(o) != 1 || o[0].Adresse != adrVon(k) || o[0].IP != "203.0.113.7" || o[0].Beweis != beweis {
		t.Fatalf("offen: %+v", o)
	}
	// Die App bindet -> die Anfrage verschwindet.
	bindungsAnfragen.erledigt(adrVon(k))
	if len(wl.offen(t)) != 0 {
		t.Fatal("erledigte Anfrage noch offen")
	}
}

// Missbrauch: falscher oder fremder Nachweis, keine Person, zu viele je IP.
func TestBindungsanfrage_Missbrauch(t *testing.T) {
	wl := neueAnfrageWelt(t)
	k, _ := crypto.GenerateKey()
	fremd, _ := crypto.GenerateKey()

	// Nachweis von einem ANDEREN Schluessel als der Signieradresse.
	if c := wl.anfrage(t, "203.0.113.1", k, wl.wallet, personalSign(t, fremd, "Aequitas: validator key linked to human "+wl.wallet)); c != 400 {
		t.Fatalf("fremder Nachweis: %d", c)
	}
	// Nachweis fuer eine andere Wallet.
	if c := wl.anfrage(t, "203.0.113.1", k, wl.wallet, personalSign(t, k, "Aequitas: validator key linked to human 0x0000000000000000000000000000000000000001")); c != 400 {
		t.Fatalf("andere Wallet im Nachweis: %d", c)
	}
	// Keine registrierte Person.
	niemand := "0x00000000000000000000000000000000000000aa"
	if c := wl.anfrage(t, "203.0.113.1", k, niemand, personalSign(t, k, "Aequitas: validator key linked to human "+niemand)); c != 403 {
		t.Fatalf("keine Person: %d", c)
	}
	// Kaputte Eingaben.
	if c := wl.anfrage(t, "203.0.113.1", k, wl.wallet, "0x1234"); c != 400 {
		t.Fatalf("kurze Signatur: %d", c)
	}
	if len(wl.offen(t)) != 0 {
		t.Fatal("abgewiesene Anfragen sind offen")
	}
	// Ratenbegrenzung je IP.
	ip := "198.51.100.9"
	beweis := personalSign(t, k, "Aequitas: validator key linked to human "+wl.wallet)
	gesperrt := false
	for i := 0; i < bindungsAnfrageJeIP+2; i++ {
		if wl.anfrage(t, ip, k, wl.wallet, beweis) == http.StatusTooManyRequests {
			gesperrt = true
		}
	}
	if !gesperrt {
		t.Fatal("keine Ratenbegrenzung je IP")
	}
}

// Begrenzt: je Wallet hoechstens bindungsAnfragenJeWallet, die neuesten bleiben.
func TestBindungsanfrage_JeWalletBegrenzt(t *testing.T) {
	wl := neueAnfrageWelt(t)
	var letzte string
	for i := 0; i < bindungsAnfragenJeWallet+4; i++ {
		k, _ := crypto.GenerateKey()
		letzte = adrVon(k)
		if c := wl.anfrage(t, fmt.Sprintf("192.0.2.%d", i+1), k, wl.wallet, personalSign(t, k, "Aequitas: validator key linked to human "+wl.wallet)); c != 200 {
			t.Fatalf("Status %d", c)
		}
	}
	o := wl.offen(t)
	if len(o) != bindungsAnfragenJeWallet || o[0].Adresse != letzte {
		t.Fatalf("%d offen (erwartet %d), neueste zuerst: %+v", len(o), bindungsAnfragenJeWallet, o)
	}
}

// Ablehnen nur mit der Unterschrift der Wallet -- niemand raeumt fremde
// Anfragen weg.
func TestBindungsanfrage_AblehnenNurMitWallet(t *testing.T) {
	wl := neueAnfrageWelt(t)
	k, _ := crypto.GenerateKey()
	if c := wl.anfrage(t, "203.0.113.2", k, wl.wallet, personalSign(t, k, "Aequitas: validator key linked to human "+wl.wallet)); c != 200 {
		t.Fatalf("Status %d", c)
	}
	loeschen := func(sig string) int {
		r := httptest.NewRequest("DELETE", "/api/bindungsanfragen?wallet="+wl.wallet+"&signing_address="+adrVon(k)+"&signatur="+sig, nil)
		w := httptest.NewRecorder()
		wl.a.handleBindungsanfragen(w, r)
		return w.Code
	}
	angreifer, _ := crypto.GenerateKey()
	if c := loeschen(personalSign(t, angreifer, "Aequitas: reject validator "+adrVon(k))); c != 403 {
		t.Fatalf("fremde Ablehnung: %d", c)
	}
	if len(wl.offen(t)) != 1 {
		t.Fatal("Anfrage durch Fremden entfernt")
	}
	if c := loeschen(personalSign(t, wl.mensch, "Aequitas: reject validator "+adrVon(k))); c != 200 {
		t.Fatalf("eigene Ablehnung: %d", c)
	}
	if len(wl.offen(t)) != 0 {
		t.Fatal("abgelehnte Anfrage noch offen")
	}
}

// Von EINER IP aus verdraengt niemand die echte Anfrage eines Menschen.
func TestBindungsanfrage_FlutVonEinerIPVerdraengtNicht(t *testing.T) {
	wl := neueAnfrageWelt(t)
	echt, _ := crypto.GenerateKey()
	if c := wl.anfrage(t, "192.0.2.10", echt, wl.wallet, personalSign(t, echt, "Aequitas: validator key linked to human "+wl.wallet)); c != 200 {
		t.Fatalf("Status %d", c)
	}
	for i := 0; i < bindungsAnfrageJeIP; i++ {
		k, _ := crypto.GenerateKey()
		wl.anfrage(t, "198.51.100.66", k, wl.wallet, personalSign(t, k, "Aequitas: validator key linked to human "+wl.wallet))
	}
	o := wl.offen(t)
	gefunden := false
	for _, e := range o {
		if e.Adresse == adrVon(echt) {
			gefunden = true
		}
	}
	if !gefunden || len(o) != 2 {
		t.Fatalf("echte Anfrage verdraengt oder Flut nicht zusammengefasst: %+v", o)
	}
}
