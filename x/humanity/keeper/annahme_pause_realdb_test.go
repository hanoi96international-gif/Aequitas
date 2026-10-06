package keeper

import (
	"context"
	"crypto/ecdsa"
	"database/sql"
	"errors"
	"os"
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
	t.Cleanup(cs.erzeugerInstanzFreigeben)
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
	t.Cleanup(cs.erzeugerInstanzFreigeben)
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

// Missbrauch/Absturz (Sicherheitspruefung #297, zweiter Durchgang, HIGH):
// eine Zeile steckte beim Absturz in einem ungespeicherten Block (markiert,
// kein Block), Neustart binnen 10 Minuten. Vorher oeffnete der Aufraeumer sie
// erst nach einer Stunde, und die Startsperre zaehlte sie nicht -- die Annahme
// ging sofort auf. Jetzt: wer die Instanzsperre haelt, oeffnet sie beim
// Start, und die Startsperre zaehlt sie.
func TestAnnahmePause_MarkierteZeilenBeimStart_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-annahme-pause-test3.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	t.Cleanup(cs.erzeugerInstanzFreigeben)
	jetzt := nowUnix()
	neueZeile := func(markiertVor int64) int64 {
		t.Helper()
		if err := savePendingTxExec(cs.db, Transaction{Type: "transfer", Wallet: "0xa9", To: "0xb", Amount: 1}); err != nil {
			t.Fatal(err)
		}
		var id int64
		if err := cs.db.QueryRow(`SELECT MAX(id) FROM pending_txs`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if markiertVor > 0 {
			if _, err := cs.db.Exec(`UPDATE pending_txs SET included_at = $1, included_block_hash = 'nie-gespeichert' WHERE id = $2`, jetzt-markiertVor, id); err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	frisch := neueZeile(30)       // beim Absturz im Bau
	uralt := neueZeile(2 * 86400) // dem Aufraeumer ueberlassen
	cs.ausgangVorStartMerken()
	var inc int64
	cs.db.QueryRow(`SELECT included_at FROM pending_txs WHERE id = $1`, frisch).Scan(&inc)
	if inc != 0 {
		t.Fatalf("Zeile aus dem ungespeicherten Block nicht wieder geoeffnet (included_at %d)", inc)
	}
	cs.db.QueryRow(`SELECT included_at FROM pending_txs WHERE id = $1`, uralt).Scan(&inc)
	if inc == 0 {
		t.Fatal("Zeile von vor zwei Tagen geoeffnet -- die gehoert dem Aufraeumer")
	}
	if cs.ausgangVorStartBis.Load() < frisch {
		t.Fatalf("Startsperre zaehlt die wieder geoeffnete Zeile nicht (bis %d, Zeile %d)", cs.ausgangVorStartBis.Load(), frisch)
	}
	if err := cs.annahmePauseGrund(); !errors.Is(err, ErrAnnahmePausiert) {
		t.Fatalf("Annahme offen, obwohl der Ausgang von vor dem Start offen ist: %v", err)
	}
}

// Haelt ein anderer Prozess die Instanzsperre (Ueberlappung beim Neustart),
// bleiben dessen markierte Zeilen unangetastet -- und die Startsperre zaehlt
// sie, bis sie verblockt sind.
func TestAnnahmePause_FremdeInstanz_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-annahme-pause-test4.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	andere, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { andere.Close() })
	ctx := context.Background()
	conn, err := andere.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ok bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, erzeugerInstanzSchluessel).Scan(&ok); err != nil || !ok {
		t.Fatalf("Instanzsperre fuer den fremden Prozess nicht bekommen: %v %v", ok, err)
	}
	t.Cleanup(func() { conn.Close() })

	if err := savePendingTxExec(cs.db, Transaction{Type: "transfer", Wallet: "0xa8", To: "0xb", Amount: 1}); err != nil {
		t.Fatal(err)
	}
	var id int64
	cs.db.QueryRow(`SELECT MAX(id) FROM pending_txs`).Scan(&id)
	if _, err := cs.db.Exec(`UPDATE pending_txs SET included_at = $1 WHERE id = $2`, nowUnix()-20, id); err != nil {
		t.Fatal(err)
	}
	cs.ausgangVorStartMerken()
	if cs.instanzSperre != nil {
		t.Fatal("Instanzsperre trotz fremdem Halter bekommen")
	}
	var inc int64
	cs.db.QueryRow(`SELECT included_at FROM pending_txs WHERE id = $1`, id).Scan(&inc)
	if inc == 0 {
		t.Fatal("markierte Zeile eines fremden Prozesses geoeffnet")
	}
	if cs.ausgangVorStartBis.Load() < id {
		t.Fatal("Startsperre zaehlt die markierte Zeile nicht")
	}
	cs.eigenerBlockGespeichert()
	if cs.ausgangVorStartBis.Load() == 0 {
		t.Fatal("Sperre geloest, obwohl die markierte Zeile in keinem Block steht")
	}
	if _, err := cs.db.Exec(`DELETE FROM pending_txs WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	cs.eigenerBlockGespeichert()
	if cs.ausgangVorStartBis.Load() != 0 {
		t.Fatal("Sperre nach dem Verblocken nicht geloest")
	}
}
