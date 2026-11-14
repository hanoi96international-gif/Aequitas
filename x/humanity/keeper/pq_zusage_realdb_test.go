package keeper

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// PQ-Zusage gegen eine echte Datenbank: der Endpunkt, das Register in
// chain_config, der Ausgang -- und ein zweiter Knoten, der beim Start
// dieselbe Zusage liest.
func TestPQZusage_UeberDenEndpunkt_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-pq-zusage-realdb-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	cs.annehmendAusdruecklich.Store(true)
	ctx := context.Background()
	if err := cs.setConfigValueCtx(ctx, umzugKonfigSchluessel, ""); err != nil {
		t.Fatal(err)
	}
	if err := cs.umzugRegisterLaden(ctx); err != nil {
		t.Fatal(err)
	}
	f := &uzFall{t: t, cs: cs}
	f.altKey, f.alt = neuerSchluessel(t)
	registerKonto(t, cs, f.alt, true)
	a := &APIServer{state: cs}
	post := func(body interface{}) (int, map[string]interface{}) {
		roh, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		a.handlePQZusage(w, httptest.NewRequest(http.MethodPost, "/api/pq/zusage", bytes.NewReader(roh)))
		var antwort map[string]interface{}
		json.NewDecoder(w.Body).Decode(&antwort)
		return w.Code, antwort
	}
	jetzt := nowUnix()
	tx := f.pqTx(f.altKey, f.alt, pqHashVon("mldsa"), jetzt)
	body := map[string]interface{}{"wallet": f.alt, "pq_hash": tx.Umzug.PQHash, "zeit": jetzt, "sig": tx.Umzug.Sig}

	// Vor dem Stichtag: 409, nichts geschieht.
	vorher := pqZusageAbOverride.Load()
	pqZusageAbOverride.Store(jetzt + 3600)
	if c, _ := post(body); c != http.StatusConflict {
		t.Fatalf("vor dem Stichtag: %d", c)
	}
	pqZusageAbOverride.Store(1)
	t.Cleanup(func() { pqZusageAbOverride.Store(vorher) })

	fremdKey, _ := neuerSchluessel(t)
	fremd := f.pqTx(fremdKey, f.alt, pqHashVon("mldsa"), jetzt)
	if c, _ := post(map[string]interface{}{"wallet": f.alt, "pq_hash": fremd.Umzug.PQHash, "zeit": jetzt, "sig": fremd.Umzug.Sig}); c != http.StatusForbidden {
		t.Fatalf("fremde Unterschrift: %d", c)
	}
	if c, _ := post(map[string]interface{}{"wallet": f.alt, "pq_hash": "ab", "zeit": jetzt, "sig": tx.Umzug.Sig}); c != http.StatusBadRequest {
		t.Fatalf("kaputter Hash: %d", c)
	}
	if c, a := post(body); c != 200 {
		t.Fatalf("Zusage: %d %v", c, a)
	}
	// Zweites Mal (Wiederholung): abgelehnt.
	if c, _ := post(body); c != http.StatusConflict {
		t.Fatalf("Wiederholung: %d", c)
	}
	zweiter, err := umzugRegisterLesen(cs.getConfigValueCtx(ctx, umzugKonfigSchluessel))
	if err != nil || zweiter.hash != cs.umzugRegisterJetzt().hash {
		t.Fatalf("Register aus der Datenbank: %v", err)
	}
	if z, ok := zweiter.pqZusage(f.alt); !ok || z.Hash != pqHashVon("mldsa") {
		t.Fatalf("Zusage nicht in der Datenbank: %+v", z)
	}
	var imAusgang int
	cs.db.QueryRow(`SELECT count(*) FROM pending_txs WHERE (tx_json::json ->> 'type') = 'pq_zusage'`).Scan(&imAusgang)
	if imAusgang != 1 {
		t.Fatalf("Ausgang: %d PQ-Zusagen, erwartet 1", imAusgang)
	}
}
