package keeper

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func berlinFuerTest(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skipf("Europe/Berlin nicht verfuegbar: %v", err)
	}
	return loc
}

// Der beobachtete Fall (C1, 08.-16.10.2026): die letzte Runde lief nach einem
// Deploy um 13:40, die Sperre ist um 20:00 erst gut sechs Stunden alt. Vorher
// hielt VerteilungHeuteOffen den Termin fuer offen, der Planer pruefte alle
// 15 Minuten und lief am naechsten Tag wieder um 13:40 -- nie um 20:00.
func TestVerteilungBaldNachholen_WeitNebenDemTerminWartetAufDenNaechsten(t *testing.T) {
	loc := berlinFuerTest(t)
	runde := time.Date(2026, 10, 16, 13, 40, 0, 0, loc)
	jetzt := time.Date(2026, 10, 16, 20, 0, 0, 0, loc)
	if !VerteilungHeuteOffen(runde.Unix(), jetzt, loc) {
		t.Fatal("Voraussetzung: der 20:00-Termin ist noch offen (die 13:40-Runde gehoerte zum Vortag)")
	}
	if VerteilungBaldNachholen(runde.Unix(), runde.Unix(), jetzt, loc) {
		t.Fatal("Sperre wird erst morgen 13:35 frei -- Nachholen hielte die Runde dauerhaft auf 13:40")
	}
	// Auch jeder weitere Retry des Tages bleibt dabei.
	for m := 15; m < 18*60; m += 15 {
		j := jetzt.Add(time.Duration(m) * time.Minute)
		if VerteilungBaldNachholen(runde.Unix(), runde.Unix(), j, loc) {
			t.Fatalf("um %s: kein Retry-Takt mehr, der naechste Termin gilt", j.Format("02.01. 15:04"))
		}
	}
}

// #354 bleibt erhalten: lief die Runde gestern erst um 20:15 (Retry), wird die
// Sperre heute um 20:10 frei -- der Termin wird noch heute nachgeholt.
func TestVerteilungBaldNachholen_KurzNachDemTerminWirdNachgeholt(t *testing.T) {
	loc := berlinFuerTest(t)
	gestern := time.Date(2026, 10, 15, 20, 15, 0, 0, loc)
	jetzt := time.Date(2026, 10, 16, 20, 0, 0, 0, loc)
	if !VerteilungBaldNachholen(gestern.Unix(), gestern.Unix(), jetzt, loc) {
		t.Fatal("Sperre frei um 20:10, innerhalb der Nachholfrist -- heute nachholen")
	}
	// Grenze: frei genau eine Stunde nach dem Termin -> noch nachholen; eine
	// Minute spaeter -> nicht.
	sperre := time.Date(2026, 10, 15, 21, 5, 0, 0, loc) // frei 16.10. 21:00
	if !VerteilungBaldNachholen(gestern.Unix(), sperre.Unix(), jetzt, loc) {
		t.Fatal("frei genau zur Nachholfrist: nachholen")
	}
	if VerteilungBaldNachholen(gestern.Unix(), sperre.Add(time.Minute).Unix(), jetzt, loc) {
		t.Fatal("frei eine Minute nach der Nachholfrist: auf den naechsten Termin warten")
	}
}

func TestVerteilungBaldNachholen_HeutigerTerminSchonGelaufen(t *testing.T) {
	loc := berlinFuerTest(t)
	heute := time.Date(2026, 10, 16, 20, 0, 5, 0, loc)
	if VerteilungBaldNachholen(heute.Unix(), heute.Unix(), heute.Add(20*time.Minute), loc) {
		t.Fatal("der heutige Termin lief schon -- morgen")
	}
	if VerteilungBaldNachholen(0, 0, heute, loc) {
		t.Fatal("nie gelaufen: der Planer wartet auf den Termin, kein Retry-Takt")
	}
}

// Missbrauch/Fehlwerte: eine Marke in der Zukunft (verfaelschte oder kaputte
// Zeile) darf keinen Dauer-Retry ausloesen, und eine Sperre ohne Runde haelt
// den Termin offen, ohne die Sperre selbst zu umgehen (die prueft weiterhin
// nur TryLockDistribution).
func TestVerteilungBaldNachholen_Fehlwerte(t *testing.T) {
	loc := berlinFuerTest(t)
	jetzt := time.Date(2026, 10, 16, 20, 0, 0, 0, loc)
	zukunft := jetzt.Add(48 * time.Hour).Unix()
	if VerteilungBaldNachholen(zukunft, zukunft, jetzt, loc) {
		t.Fatal("Marke in der Zukunft: kein Retry")
	}
	alt := jetzt.Add(-14 * 24 * time.Hour).Unix()
	if !VerteilungBaldNachholen(alt, 0, jetzt, loc) {
		t.Fatal("alte Runde, keine Sperre: bald pruefen (TryLockDistribution entscheidet)")
	}
}

// Ende zu Ende gegen Postgres: eine Runde mit leeren Toepfen laeuft, zahlt
// nichts aus und sagt das. last_ubi_at bleibt stehen (wirtschaftliche Regel,
// unveraendert), die Rundenmarke rueckt vor, und der Planer liest sie.
func TestVerteilungZeitplan_LeererTopf_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-verteilung-zeitplan-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}
	loc := berlinFuerTest(t)
	wirtschaftAn(t) // wie live seit 26.09.2026: Rundenmarke auch ohne Auszahlung

	cs.mu.Lock()
	mensch := &AccountState{Address: distTestAddr(1), IsHuman: true, Balance: NewDecimal(1000), LastActivityAt: time.Now().Unix()}
	if err := cs.saveAccountToDB(mensch); err != nil {
		cs.mu.Unlock()
		t.Fatalf("seed: %v", err)
	}
	cs.accounts.Set(mensch.Address, mensch)
	cs.mu.Unlock()

	// Wie auf C1: letzte Auszahlung am 02.10., seither nichts in den Toepfen.
	letzteAuszahlung := int64(1790964000)
	if err := cs.setConfigValueDB("last_ubi_at", fmt.Sprint(letzteAuszahlung)); err != nil {
		t.Fatal(err)
	}
	runde := time.Date(2026, 10, 16, 13, 40, 0, 0, loc)
	if !wirtschaftAktiv(runde.Unix()) {
		t.Fatal("Voraussetzung: Umlaufsicherung aktiv, die Marke wird auch ohne Auszahlung geschrieben")
	}
	if !cs.TryLockDistribution() {
		t.Fatal("die erste Sperre muss gelingen")
	}
	if err := cs.setConfigValueDB("distribution_lock_at", fmt.Sprint(runde.Unix())); err != nil {
		t.Fatal(err)
	}
	if err := cs.RunDailyDistributionAtomic(runde.Unix()); err != nil {
		t.Fatalf("RunDailyDistributionAtomic: %v", err)
	}

	if got := cs.GetLastUBIAt(); got != letzteAuszahlung {
		t.Fatalf("last_ubi_at = %d, want unveraendert %d (leerer Topf zahlt nichts)", got, letzteAuszahlung)
	}
	if got := cs.LetzteVerteilungsrunde(); got != runde.Unix() {
		t.Fatalf("LetzteVerteilungsrunde = %d, want %d (die Rundenmarke)", got, runde.Unix())
	}
	if got := LetztesUBIErgebnis(); got != ubiErgebnisTopfLeer {
		t.Fatalf("UBI-Ergebnis = %q, want %q", got, ubiErgebnisTopfLeer)
	}
	h := cs.DistributionHealth()
	if h["last_round_at"] != runde.Unix() || h["ubi_result"] != ubiErgebnisTopfLeer {
		t.Fatalf("Gesundheit muss Runde und Grund zeigen: last_round_at=%v ubi_result=%v", h["last_round_at"], h["ubi_result"])
	}

	jetzt := time.Date(2026, 10, 16, 20, 0, 0, 0, loc)
	// Gegenprobe: mit last_ubi_at als "letzte Runde" (vorher) bleibt der
	// Termin ewig offen -- das war der 15-Minuten-Takt.
	if !VerteilungHeuteOffen(cs.GetLastUBIAt(), jetzt.Add(30*24*time.Hour), loc) {
		t.Fatal("Gegenprobe: last_ubi_at haelt jeden Termin offen")
	}
	if VerteilungBaldNachholen(cs.LetzteVerteilungsrunde(), cs.VerteilungsSperreAb(), jetzt, loc) {
		t.Fatal("13:40-Runde: um 20:00 nicht im 15-Minuten-Takt nachholen, sondern morgen 20:00")
	}
	// Die Sperre selbst bleibt die einzige Pruefung und ist unveraendert.
	if err := cs.setConfigValueDB("distribution_lock_at", fmt.Sprint(time.Now().Unix())); err != nil {
		t.Fatal(err)
	}
	if cs.TryLockDistribution() {
		t.Fatal("eine frische Sperre muss jede weitere Runde abweisen")
	}

	// Kaputte Marke: zurueck auf last_ubi_at, kein Absturz.
	if err := cs.setConfigValueDB("last_distribution_round_at", "kaputt"); err != nil {
		t.Fatal(err)
	}
	if got := cs.LetzteVerteilungsrunde(); got != letzteAuszahlung {
		t.Fatalf("kaputte Marke: LetzteVerteilungsrunde = %d, want last_ubi_at %d", got, letzteAuszahlung)
	}
}
