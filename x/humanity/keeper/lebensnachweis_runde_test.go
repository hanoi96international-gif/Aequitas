package keeper

import (
	"context"
	"errors"
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
	if err := cs.lebensnachweisRundeBereitLocked(context.Background(), 2_000, true); err != nil {
		t.Fatalf("ohne Ausgang: %v", err)
	}
	if g := cs.LebensnachweiseOffenGrund(2_000); g != "" {
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
	if err := cs.lebensnachweisRundeBereitLocked(ctx, 2_000, true); err != nil {
		t.Fatalf("Ueberweisung im Ausgang haelt die Runde an: %v", err)
	}

	// Missbrauch / Randfall: ein angewandter Lebensnachweis, noch in keinem Block.
	if err := savePendingTxExec(cs.db, Transaction{Type: lebensnachweisTxArt, Wallet: "0xa2", DistributionAt: 1_900}); err != nil {
		t.Fatal(err)
	}
	if err := cs.lebensnachweisRundeBereitLocked(ctx, 2_000, true); !errors.Is(err, ErrLebensnachweiseOffen) {
		t.Fatalf("offener Lebensnachweis haelt die Runde nicht an: %v", err)
	}
	if g := cs.LebensnachweiseOffenGrund(2_000); g == "" {
		t.Fatal("Vorpruefung laesst die Runde trotz offenem Lebensnachweis starten")
	}
	// Vor der Aktivierung bzw. ohne moegliche Pause entscheidet die Runde
	// keine Pausen -- dann wartet sie auch nicht.
	if err := cs.lebensnachweisRundeBereitLocked(ctx, 2_000, false); err != nil {
		t.Fatalf("ohne moegliche Pause angehalten: %v", err)
	}
	if err := cs.lebensnachweisRundeBereitLocked(ctx, 500, true); err != nil {
		t.Fatalf("vor der Aktivierung angehalten: %v", err)
	}

	// Verblockt: die Runde darf.
	if _, err := cs.db.Exec(`UPDATE pending_txs SET included_at = 1 WHERE (tx_json::jsonb ->> 'type') = $1`, lebensnachweisTxArt); err != nil {
		t.Fatal(err)
	}
	if err := cs.lebensnachweisRundeBereitLocked(ctx, 2_000, true); err != nil {
		t.Fatalf("verblockter Lebensnachweis haelt die Runde an: %v", err)
	}
	if g := cs.LebensnachweiseOffenGrund(2_000); g != "" {
		t.Fatalf("Vorpruefung nach dem Verblocken: %q", g)
	}

	// Ein verworfener Block legt den Nachweis zurueck (included_at = 0): er
	// zaehlt wieder als offen.
	if _, err := cs.db.Exec(`UPDATE pending_txs SET included_at = 0 WHERE (tx_json::jsonb ->> 'type') = $1`, lebensnachweisTxArt); err != nil {
		t.Fatal(err)
	}
	if err := cs.lebensnachweisRundeBereitLocked(ctx, 2_000, true); !errors.Is(err, ErrLebensnachweiseOffen) {
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
