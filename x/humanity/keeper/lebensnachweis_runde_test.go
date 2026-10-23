package keeper

import (
	"context"
	"errors"
	"math"
	"testing"
)

// #410: die Grundeinkommensrunde entscheidet ueber Pausen nur, wenn kein
// angewandter Lebensnachweis mehr offen im Ausgang liegt
// (lebensnachweis_runde.go).

// Ohne Datenbank gibt es keinen Ausgang -- nichts Offenes; vor der
// Aktivierung oder ohne moegliche Pause gilt die Regel nicht.
func TestLebensnachweisRunde_OhneAusgangUndInaktiv(t *testing.T) {
	cs := newTestState()
	lnAktiv(t, 1_000, 1_000)
	if err := cs.lebensnachweisRundeBereitLocked(context.Background(), 2_000); err != nil {
		t.Fatalf("ohne Ausgang: %v", err)
	}
	if g, _ := cs.LebensnachweiseOffenGrund(2_000); g != "" {
		t.Fatalf("ohne Ausgang: %q", g)
	}
}

// Gegen eine echte Datenbank: ein angewandter, noch nicht verblockter
// Lebensnachweis haelt die Runde an -- vorher (Vorpruefung) wie unter der
// Sperre --, andere offene Auftraege und verblockte Nachweise nicht.
func TestLebensnachweisRunde_WartetAufOffeneNachweise_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-lebensnachweis-runde-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	lnAktiv(t, 1_000, 1_000)
	ctx := context.Background()

	// Ein offener Auftrag anderer Art haelt nichts an.
	if err := savePendingTxExec(cs.db, Transaction{Type: "transfer", Wallet: "0xa1", To: "0xb", Amount: 1}); err != nil {
		t.Fatal(err)
	}
	if err := cs.lebensnachweisRundeBereitLocked(ctx, 2_000); err != nil {
		t.Fatalf("Ueberweisung im Ausgang haelt die Runde an: %v", err)
	}

	// Missbrauch / Randfall: ein angewandter Lebensnachweis, noch in keinem Block.
	if err := savePendingTxExec(cs.db, Transaction{Type: lebensnachweisTxArt, Wallet: "0xa2", DistributionAt: 1_900}); err != nil {
		t.Fatal(err)
	}
	if err := cs.lebensnachweisRundeBereitLocked(ctx, 2_000); !errors.Is(err, ErrLebensnachweiseOffen) {
		t.Fatalf("offener Lebensnachweis haelt die Runde nicht an: %v", err)
	}
	if g, _ := cs.LebensnachweiseOffenGrund(2_000); g == "" {
		t.Fatal("Vorpruefung laesst die Runde trotz offenem Lebensnachweis starten")
	}
	// Ein offener Lebensnachweis ist das Signal, die Annahme kurz anzuhalten.
	if _, offen := cs.LebensnachweiseOffenGrund(2_000); !offen {
		t.Fatal("offener Lebensnachweis nicht als Grund zum Anhalten gemeldet")
	}
	// Vor der Aktivierung entscheidet die Runde keine Pausen -- dann wartet
	// sie auch nicht. (Ab der Aktivierung wartet sie immer, auch wenn der
	// Erzeuger keine Pause fuer moeglich haelt: #412, die Betreiberzahl kann
	// auch sinken.)
	if err := cs.lebensnachweisRundeBereitLocked(ctx, 500); err != nil {
		t.Fatalf("vor der Aktivierung angehalten: %v", err)
	}

	// Verblockt: die Runde darf.
	if _, err := cs.db.Exec(`UPDATE pending_txs SET included_at = 1 WHERE (tx_json::jsonb ->> 'type') = $1`, lebensnachweisTxArt); err != nil {
		t.Fatal(err)
	}
	if err := cs.lebensnachweisRundeBereitLocked(ctx, 2_000); err != nil {
		t.Fatalf("verblockter Lebensnachweis haelt die Runde an: %v", err)
	}
	if g, _ := cs.LebensnachweiseOffenGrund(2_000); g != "" {
		t.Fatalf("Vorpruefung nach dem Verblocken: %q", g)
	}

	// Ein verworfener Block legt den Nachweis zurueck (included_at = 0): er
	// zaehlt wieder als offen.
	if _, err := cs.db.Exec(`UPDATE pending_txs SET included_at = 0 WHERE (tx_json::jsonb ->> 'type') = $1`, lebensnachweisTxArt); err != nil {
		t.Fatal(err)
	}
	if err := cs.lebensnachweisRundeBereitLocked(ctx, 2_000); !errors.Is(err, ErrLebensnachweiseOffen) {
		t.Fatalf("zurueckgelegter Lebensnachweis haelt die Runde nicht an: %v", err)
	}
}

// Die Tagessperre wird nach einer abgebrochenen Runde freigegeben -- sonst
// fiele der Tag aus.
func TestLebensnachweisRunde_SperreFreigeben_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-lebensnachweis-sperre-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	if !cs.TryLockDistribution() {
		t.Fatal("Sperre nicht erhalten")
	}
	if cs.TryLockDistribution() {
		t.Fatal("Sperre doppelt erhalten")
	}
	if err := cs.VerteilungsSperreLoesen(); err != nil {
		t.Fatal(err)
	}
	if !cs.TryLockDistribution() {
		t.Fatal("nach dem Freigeben keine Sperre -- der Tag fiele aus")
	}
}

// Missbrauch/Wettlauf (Sicherheitspruefung #411, Befund 3): hat inzwischen
// ein anderer Knoten die Sperre genommen (anderer Wert), gibt die Freigabe
// sie NICHT frei -- sonst liefe die Runde am selben Tag zweimal an. Ohne
// eigene Sperre gibt es ebenfalls nichts freizugeben.
func TestLebensnachweisRunde_NurEigeneSperreFrei_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-lebensnachweis-sperre-fremd-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	if err := cs.VerteilungsSperreLoesen(); err == nil {
		t.Fatal("ohne eigene Sperre freigegeben")
	}
	if !cs.TryLockDistribution() {
		t.Fatal("Sperre nicht erhalten")
	}
	// Ein anderer Knoten derselben Datenbank nimmt die Sperre neu.
	if _, err := cs.db.Exec(`UPDATE chain_config SET value = '9999999999' WHERE key = 'distribution_lock_at'`); err != nil {
		t.Fatal(err)
	}
	if err := cs.VerteilungsSperreLoesen(); err == nil {
		t.Fatal("fremde Sperre freigegeben")
	}
	var wert string
	if err := cs.db.QueryRow(`SELECT value FROM chain_config WHERE key = 'distribution_lock_at'`).Scan(&wert); err != nil {
		t.Fatal(err)
	}
	if wert != "9999999999" {
		t.Fatalf("fremde Sperre veraendert: %q", wert)
	}
	if cs.TryLockDistribution() {
		t.Fatal("Sperre trotz fremder Sperre erhalten")
	}
}

// Die Runde fragt, bevor sie Empfaenger waehlt (Reihenfolge im Quelltext,
// wie TestAnnahme_MerkerReihenfolge).
func TestLebensnachweisRunde_PruefungVorDerAuswahl(t *testing.T) {
	st := aufrufStellen(t, "state.go", "distributeUBIPoolLocked")
	bereit, ok1 := st["cs.lebensnachweisRundeBereitLocked"]
	auswahl, ok2 := st["cs.lebensnachweisEmpfaengerLocked"]
	if !ok1 || !ok2 || bereit >= auswahl {
		t.Fatalf("lebensnachweisRundeBereitLocked muss vor lebensnachweisEmpfaengerLocked stehen (%v/%v)", ok1, ok2)
	}
}

// Sperre gegen versehentliches Einschalten (wie TestStufe2NieVorStrengemNachrechnen):
// die Sicherheitspruefung von #408/#411 hat Befunde, die erst mit dem
// Einschalten wirken (Aequitas#412). Befund 2 (Betreiber mit offener
// Staffel) und Befund 4 (Verfuegbarkeit) sind geschlossen
// (zugelasseneBetreiberZu, offenePauseAuftraege, lebensnachweisHalt).
// Offen bleibt Befund 1: im verteilten Term nimmt der Zustaendige den
// Nachweis an, die Runde laeuft beim Leiter -- der Waechter sieht nur den
// eigenen Ausgang. Wer beide Stichtage setzt, schliesst vorher Befund 1 und
// passt diesen Test an.
func TestLebensnachweis_AktivierungErstNachBefunden(t *testing.T) {
	if verteilteAnnahmeAbUnix != math.MaxInt64 && registerLeserAbUnix != math.MaxInt64 {
		t.Fatalf("verteilte Annahme und Lebensnachweis zugleich: vorher #412 Befund 1 schliessen (Runde als Schranke ueber alle Zustaendigen)")
	}
}
