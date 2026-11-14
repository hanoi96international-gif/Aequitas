package keeper

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// PQ-Ueberweisung gegen eine echte Datenbank MIT WAL-Schnellpfad: die
// Ueberweisung darf den WAL nicht nehmen (sein Satz traegt den PQ-Nachweis
// nicht), muss im Ausgang MIT Nachweis stehen, und die Nonce steht im Konto.
func TestPQUeberweisung_UeberDenEndpunktMitWAL_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	t.Setenv("AEQUITAS_WAL_ENABLED", "1")
	t.Setenv("AEQUITAS_WAL_PATH", filepath.Join(t.TempDir(), "pq.wal"))
	cs := testKnoten(t, "unused-pq-ueberweisung-realdb-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	if cs.wal == nil {
		t.Fatal("der WAL-Schnellpfad sollte an sein")
	}
	t.Cleanup(cs.stopWALFlushWorkerForTest)
	cs.annehmendAusdruecklich.Store(true)
	pqAktivFuerTest(t)

	a := neuerPQKontoSchluessel(t, 0x91)
	b := neuerTestSchluessel(t)
	cs.mu.Lock()
	acc := &AccountState{Address: a.addr, Balance: NewDecimal(1000), LastActivityAt: nowUnix()}
	cs.accounts.Set(a.addr, acc)
	if err := cs.saveAccountToDB(acc); err != nil {
		cs.mu.Unlock()
		t.Fatal(err)
	}
	cs.mu.Unlock()

	api := &APIServer{state: cs}
	walVorher := transferFastPathApplied.Load()
	tx := pqTransfer(t, a, b.addr, aeqWei(25), 0)
	roh, _ := json.Marshal(map[string]interface{}{"von": a.addr, "an": b.addr, "wei": tx.PQ.Wei, "nonce": 0, "pub": a.pub, "sig": tx.PQ.Sig})
	w := httptest.NewRecorder()
	api.handlePQUeberweisung(w, httptest.NewRequest(http.MethodPost, "/api/pq/ueberweisung", bytes.NewReader(roh)))
	if w.Code != http.StatusOK {
		t.Fatalf("Annahme: %d %s", w.Code, w.Body.String())
	}
	if transferFastPathApplied.Load() != walVorher {
		t.Fatal("die PQ-Ueberweisung hat einen Schnellpfad genommen")
	}
	var txJSON string
	if err := cs.db.QueryRow(`SELECT tx_json FROM pending_txs WHERE tx_json::json ->> 'tx_hash' = $1`, tx.TxHash).Scan(&txJSON); err != nil {
		t.Fatalf("nicht im Ausgang: %v", err)
	}
	var imAusgang Transaction
	if err := json.Unmarshal([]byte(txJSON), &imAusgang); err != nil || imAusgang.PQ == nil || imAusgang.PQ.Sig != tx.PQ.Sig {
		t.Fatalf("Ausgang ohne PQ-Nachweis: %v %s", err, txJSON[:min(len(txJSON), 200)])
	}
	// Was im Ausgang steht, besteht die Pruefung jedes Nachspielenden.
	if _, err := pruefeUeberweisungsRoh(&imAusgang, nowUnix()); err != nil {
		t.Fatalf("Ausgang besteht die Pruefung nicht: %v", err)
	}
	var naechste int64
	if err := cs.db.QueryRow(`SELECT COALESCE(naechste_nonce, 0) FROM chain_accounts WHERE address = $1`, a.addr).Scan(&naechste); err != nil {
		t.Fatal(err)
	}
	if naechste != 1 {
		t.Fatalf("NaechsteNonce in der Datenbank %d statt 1", naechste)
	}
	// Wiederholung: abgelehnt.
	w = httptest.NewRecorder()
	api.handlePQUeberweisung(w, httptest.NewRequest(http.MethodPost, "/api/pq/ueberweisung", bytes.NewReader(roh)))
	if w.Code == http.StatusOK {
		t.Fatal("Wiederholung angenommen")
	}
}
