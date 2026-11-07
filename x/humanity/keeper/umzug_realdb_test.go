package keeper

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Umzug gegen eine echte Datenbank: die Endpunkte, der Nullifier in der
// Tabelle, das Register in chain_config -- und ein zweiter Knoten, der beim
// Start dasselbe Register liest.
func TestUmzug_UeberDieEndpunkte_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-umzug-realdb-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	vorher := umzugAbOverride.Load()
	umzugAbOverride.Store(1)
	t.Cleanup(func() { umzugAbOverride.Store(vorher) })
	cs.annehmendAusdruecklich.Store(true)
	ctx := context.Background()
	if err := cs.setConfigValueCtx(ctx, umzugKonfigSchluessel, ""); err != nil {
		t.Fatal(err)
	}
	if err := cs.umzugRegisterLaden(ctx); err != nil {
		t.Fatal(err)
	}

	f := &uzFall{t: t, cs: cs, coord: neuerEd25519(t), zeuge: neuerEd25519(t)}
	setBescheinigungFuerTest(t, &bescheinigungKonfig{AbUnix: 1, Modus: "test", Quorum: 2,
		Coordinator: []string{f.coord.pub}, Bezeuger: []string{f.zeuge.pub}})
	f.altKey, f.alt = neuerSchluessel(t)
	f.neuKey, f.neu = neuerSchluessel(t)
	registerKonto(t, cs, f.alt, true)
	if err := cs.SaveNullifier(ctx, uzN, f.alt); err != nil {
		t.Fatal(err)
	}
	a := &APIServer{state: cs}
	post := func(h http.HandlerFunc, body interface{}) (int, map[string]interface{}) {
		roh, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest(http.MethodPost, "/api/umzug", bytes.NewReader(roh)))
		var antwort map[string]interface{}
		json.NewDecoder(w.Body).Decode(&antwort)
		return w.Code, antwort
	}

	jetzt := nowUnix()
	code := f.codeTx(f.altKey, f.alt, uzCode, jetzt)
	if c, a := post(a.handleUmzugCode, map[string]interface{}{"wallet": f.alt, "code_hash": code.Umzug.CodeHash, "zeit": jetzt, "sig": code.Umzug.Sig}); c != 200 {
		t.Fatalf("Code: %d %v", c, a)
	}
	antrag := f.antrag(f.neuKey, f.neu, f.alt, uzN, jetzt, uzCode)
	body := map[string]interface{}{"neu": f.neu, "alt": f.alt, "nullifier": uzN, "zeit": jetzt, "modus": "test",
		"bescheinigung": []string{"v3:" + antrag.BioBescheinigung.Signaturen[0], "v3:" + antrag.BioBescheinigung.Signaturen[1]}, "pruefung": "streng",
		"code": strings.ToLower(uzCode), "sig": antrag.Umzug.Sig}
	// Falscher Code: abgelehnt, nichts geschieht.
	falsch := map[string]interface{}{}
	for k, v := range body {
		falsch[k] = v
	}
	falsch["code"] = "ZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZ"
	if c, _ := post(a.handleUmzugAntrag, falsch); c != http.StatusConflict {
		t.Fatalf("falscher Code: %d", c)
	}
	if c, a := post(a.handleUmzugAntrag, body); c != 200 || a["sofort"] != true {
		t.Fatalf("Antrag: %d %v", c, a)
	}

	var inhaber string
	if err := cs.db.QueryRow(`SELECT wallet_address FROM nullifiers WHERE nullifier = $1`, uzN).Scan(&inhaber); err != nil || inhaber != f.neu {
		t.Fatalf("Nullifier in der Tabelle: %q %v", inhaber, err)
	}
	if got := strings.ToLower(cs.GetWalletByNullifier(uzN)); got != f.neu {
		t.Fatalf("GetWalletByNullifier: %q", got)
	}
	var altMensch, neuMensch, neuFaucet bool
	cs.db.QueryRow(`SELECT is_human FROM chain_accounts WHERE address = $1`, f.alt).Scan(&altMensch)
	cs.db.QueryRow(`SELECT is_human, faucet_claimed FROM chain_accounts WHERE address = $1`, f.neu).Scan(&neuMensch, &neuFaucet)
	if altMensch || !neuMensch || !neuFaucet {
		t.Fatalf("Konten in der Datenbank: alt Mensch %v, neu Mensch %v, neu Faucet %v", altMensch, neuMensch, neuFaucet)
	}
	// Ein zweiter Knoten liest beim Start dasselbe Register.
	zweiter, err := umzugRegisterLesen(cs.getConfigValueCtx(ctx, umzugKonfigSchluessel))
	if err != nil || zweiter.hash != cs.umzugRegisterJetzt().hash || len(zweiter.Erledigt) != 1 {
		t.Fatalf("Register aus der Datenbank: %v %+v", err, zweiter)
	}
	// Der Antrag steht im Ausgang, damit er in den naechsten Block kommt.
	var imAusgang int
	cs.db.QueryRow(`SELECT count(*) FROM pending_txs WHERE (tx_json::json ->> 'type') IN ('umzug_code', 'umzug_ankuendigen')`).Scan(&imAusgang)
	if imAusgang != 2 {
		t.Fatalf("Ausgang: %d Umzugs-Auftraege, erwartet 2", imAusgang)
	}
}
