package keeper

import (
	"crypto/ecdsa"
	"errors"
	"testing"
)

// annahme_pause.go gegen eine echte Datenbank: nach einem Neustart nimmt der
// Knoten nichts an, bis der Ausgang von vor dem Start verblockt ist -- sonst
// laegen alte und neue Auftraege in einem Block ohne gemeinsame Blockzeit.
func TestAnnahmePause_AusgangVorStart_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-annahme-pause-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	// mensch: ein registrierter Mensch mit eigenem Schluessel.
	mensch := func() *ecdsa.PrivateKey {
		k, w := neuerSchluessel(t)
		cs.mu.Lock()
		defer cs.mu.Unlock()
		acc := &AccountState{Address: w, IsHuman: true, Balance: NewDecimal(10), LastActivityAt: nowUnix()}
		if err := cs.saveAccountToDB(acc); err != nil {
			t.Fatal(err)
		}
		cs.accounts.Set(w, acc)
		cs.updateAccountLeafLocked(acc)
		return k
	}

	// Ein offener Auftrag von vor dem "Neustart".
	if err := savePendingTxExec(cs.db, Transaction{Type: "transfer", Wallet: "0xa1", To: "0xb", Amount: 1}); err != nil {
		t.Fatal(err)
	}
	cs.ausgangVorStartMerken()
	bis := cs.ausgangVorStartBis.Load()
	if bis == 0 {
		t.Fatal("offener Ausgang beim Start nicht bemerkt")
	}

	// Missbrauch: ein gueltiger Auftrag waehrend der Sperre wird nicht
	// angenommen -- und hinterlaesst nichts.
	schuetzling := mensch()
	vormund := adrVon(mensch())
	tx := vormundAuftragVon(t, schuetzling, vormund, nowUnix())
	err := cs.VormundSetzen(tx.Wallet, tx.To, tx.Nachweis)
	if !errors.Is(err, ErrAnnahmePausiert) {
		t.Fatalf("Annahme waehrend der Startsperre: %v", err)
	}
	var n int
	if err := cs.db.QueryRow(`SELECT count(*) FROM pending_txs WHERE included_at = 0`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("Ausgang nach abgewiesener Annahme: %d Zeilen (%v), erwartet 1", n, err)
	}

	// Ein eigener Block, der den alten Ausgang NICHT verblockt hat, loest die
	// Sperre nicht.
	cs.eigenerBlockGespeichert()
	if cs.ausgangVorStartBis.Load() == 0 {
		t.Fatal("Sperre geloest, obwohl der alte Ausgang offen ist")
	}
	// Der Block verblockt ihn (SaveBlock loescht die Zeilen) -- jetzt offen.
	if _, err := cs.db.Exec(`DELETE FROM pending_txs WHERE id <= $1`, bis); err != nil {
		t.Fatal(err)
	}
	cs.eigenerBlockGespeichert()
	if err := cs.annahmePauseGrund(); err != nil {
		t.Fatalf("nach dem Verblocken: %v", err)
	}
	if err := cs.VormundSetzen(tx.Wallet, tx.To, tx.Nachweis); err != nil {
		t.Fatalf("Annahme nach der Sperre: %v", err)
	}
}

// Was nach dem Start dazukommt, haelt die Sperre nicht fest.
func TestAnnahmePause_NeueZeilenHaltenNichtFest_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-annahme-pause-test2.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	cs.ausgangVorStartMerken()
	if cs.ausgangVorStartBis.Load() != 0 {
		t.Fatal("leerer Ausgang beim Start gilt als offen")
	}
	if err := savePendingTxExec(cs.db, Transaction{Type: "transfer", Wallet: "0xa2", To: "0xb", Amount: 1}); err != nil {
		t.Fatal(err)
	}
	cs.ausgangVorStartMerken()
	bis := cs.ausgangVorStartBis.Load()
	if err := savePendingTxExec(cs.db, Transaction{Type: "transfer", Wallet: "0xa3", To: "0xb", Amount: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.db.Exec(`DELETE FROM pending_txs WHERE id <= $1`, bis); err != nil {
		t.Fatal(err)
	}
	cs.eigenerBlockGespeichert()
	if cs.ausgangVorStartBis.Load() != 0 {
		t.Fatal("eine nach dem Start angenommene Zeile haelt die Sperre fest")
	}
}
