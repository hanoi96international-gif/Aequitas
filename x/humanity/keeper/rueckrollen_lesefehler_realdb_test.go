package keeper

import "testing"

// Scheitert im Rueckroll-Snapshot das Vorladen eines kalten Kontos (etwa weil
// die Transaktion nach statement_timeout abgebrochen ist), ist sein Stand
// UNBEKANNT -- nicht "gibt es nicht". Das Zurueckrollen loeschte bisher jede
// Adresse, die nicht geladen war, mit einer frischen Verbindung aus
// chain_accounts: eine Zeitueberschreitung vernichtete echte Guthaben.

func rueckrollenNachLesefehler(t *testing.T, voll bool) {
	t.Helper()
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-rueckrollen-lesefehler-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung -- DATABASE_URL pruefen")
	}
	w := distTestAddr(1800)
	cs.mu.Lock()
	acc := &AccountState{Address: w, Balance: NewDecimal(500), IsHuman: true}
	if err := cs.saveAccountToDB(acc); err != nil {
		cs.mu.Unlock()
		t.Fatal(err)
	}
	cs.accounts.Delete(w) // kalt: nur in der Datenbank
	cs.mu.Unlock()

	tx, err := cs.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	cs.mu.Lock()
	cs.setActiveTx(tx)
	// Eine gescheiterte Anweisung bricht die Transaktion ab; jede weitere
	// Abfrage darin scheitert -- wie nach einer Zeitueberschreitung.
	if _, err := tx.Exec(`SELECT 1/0`); err == nil {
		cs.setActiveTx(nil)
		cs.mu.Unlock()
		t.Fatal("die Transaktion sollte abgebrochen sein")
	}
	snap := cs.snapshotForRollbackLocked([]string{w}, voll, nil)
	cs.setActiveTx(nil)
	tx.Rollback()
	_ = cs.restoreFromRollbackLocked(snap)
	cs.mu.Unlock()

	var bal float64
	if err := cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, w).Scan(&bal); err != nil {
		t.Fatalf("das Zurueckrollen hat die echte Zeile von %s geloescht: %v", w, err)
	}
	if bal != 500 {
		t.Fatalf("Guthaben nach dem Zurueckrollen %.6f statt 500", bal)
	}
	// Im Speicher steht nichts Falsches: beim naechsten Zugriff kommt der
	// wahre Stand aus der Datenbank.
	cs.mu.Lock()
	cs.ensureAccountLoaded(w)
	got := stand(cs, w)
	cs.mu.Unlock()
	if got != 500 {
		t.Fatalf("nach dem Nachladen %.6f statt 500", got)
	}
}

func TestRueckrollen_LoeschtKeineZeileBeiLesefehler_RealDB(t *testing.T) {
	rueckrollenNachLesefehler(t, false)
}

func TestRueckrollen_LoeschtKeineZeileBeiLesefehlerVoll_RealDB(t *testing.T) {
	rueckrollenNachLesefehler(t, true)
}

// Gegenprobe: eine Adresse, die es wirklich nicht gab und die der
// gescheiterte Schritt angelegt hat, faellt weiterhin aus Speicher und
// Datenbank.
func TestRueckrollen_NeuesKontoFaelltWeiter_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-rueckrollen-neu-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung -- DATABASE_URL pruefen")
	}
	w := distTestAddr(1801)
	cs.mu.Lock()
	snap := cs.snapshotForRollbackLocked([]string{w}, false, nil)
	acc := &AccountState{Address: w, Balance: NewDecimal(7)}
	cs.accounts.Set(w, acc)
	if err := cs.saveAccountToDB(acc); err != nil {
		cs.mu.Unlock()
		t.Fatal(err)
	}
	if err := cs.restoreFromRollbackLocked(snap); err != nil {
		cs.mu.Unlock()
		t.Fatal(err)
	}
	_, imSpeicher := cs.accounts.Get(w)
	cs.mu.Unlock()
	var n int
	cs.db.QueryRow(`SELECT COUNT(*) FROM chain_accounts WHERE lower(address) = $1`, w).Scan(&n)
	if imSpeicher || n != 0 {
		t.Fatalf("neu angelegtes Konto nach dem Zurueckrollen noch da (Speicher %v, Zeilen %d)", imSpeicher, n)
	}
}
