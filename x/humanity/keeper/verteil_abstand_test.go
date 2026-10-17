package keeper

import (
	"os"
	"testing"
	"time"
)

// Der Erzeuger-Mindestabstand muss genau dem entsprechen, was jeder
// nachspielende Knoten prueft: eine Runde, die der Erzeuger startet, wird nie
// verworfen, und jede, die verworfen wuerde, startet er nicht. Geprueft gegen
// distributionRoundToSkip selbst, nicht gegen eine Abschrift der Regel.
func TestVerteilAbstand_DeckungsgleichMitNachspielen(t *testing.T) {
	letzte := int64(1792000000)
	for _, letzteAuszahlung := range []int64{0, letzte - 3600, letzte, letzte + 30} {
		basis := letzte
		if letzteAuszahlung > basis {
			basis = letzteAuszahlung
		}
		for d := int64(-900); d <= 900; d++ {
			ubiAt := basis + verteilMindestabstand + d
			erlaubt := VerteilAbstandErlaubt(ubiAt, verteilAbstandFrei(letzte, letzteAuszahlung))
			block := []Transaction{
				{Type: "ubi_distribution_finalize", DistributionAt: ubiAt},
				{Type: "distribution_round_marker", DistributionAt: ubiAt},
			}
			verworfen := distributionRoundToSkip(block, letzte, letzteAuszahlung) != 0
			if erlaubt == verworfen {
				t.Fatalf("letzte=%d auszahlung=%d ubiAt=%d: Erzeuger erlaubt=%v, Nachspielen verwirft=%v",
					letzte, letzteAuszahlung, ubiAt, erlaubt, verworfen)
			}
		}
	}
}

// Missbrauch/Grenzfall des Konsens-Befunds: 23 h 55 min bis 24 h nach der
// Marke liess TryLockDistribution die Runde zu, jeder andere Knoten verwarf
// sie. Der Erzeuger startet sie jetzt nicht; ab genau 24 h schon.
func TestVerteilAbstand_2355Bis24hNichtGestartet(t *testing.T) {
	letzte := int64(1792000000)
	frei := verteilAbstandFrei(letzte, 0)
	for s := int64(23*3600 + 55*60); s < 24*3600; s++ {
		if VerteilAbstandErlaubt(letzte+s, frei) {
			t.Fatalf("%d s nach der Marke gestartet -- Nachspielende verwerfen das", s)
		}
	}
	if !VerteilAbstandErlaubt(letzte+24*3600, frei) || !VerteilAbstandErlaubt(letzte+24*3600+1, frei) {
		t.Fatal("ab 24 h muss die Runde starten")
	}
	if !VerteilAbstandErlaubt(1, verteilAbstandFrei(0, 0)) {
		t.Fatal("ohne jede Marke: keine Abstandssperre")
	}
}

// Der 20:00-Plan wandert nicht: 400 Tage Planer-Simulation (beide
// Zeitumstellungen, Sekunden-Latenz, ein um 40 min verspaeteter Tag). Jede
// gestartete Runde muss distributionRoundToSkip passieren, keine darf mehr als
// verteilAbstandNachlauf nach 20:00 laufen (ausser dem verspaeteten Tag
// selbst), und danach ist der Plan binnen drei Terminen wieder bei 20:00.
func TestVerteilAbstand_PlanWandertNicht(t *testing.T) {
	loc := berlinFuerTest(t)
	var letzte int64
	start := time.Date(2026, 10, 1, 20, 0, 0, 0, loc)
	verspaetetAm := time.Date(2027, 2, 10, 20, 0, 0, 0, loc)
	nachVerspaetung := 0
	runden := 0
	for i := 0; i < 400; i++ {
		slot := time.Date(start.Year(), start.Month(), start.Day()+i, 20, 0, 0, 0, loc)
		latenz := time.Duration(i%3) * time.Second
		if slot.Equal(verspaetetAm) {
			latenz = 40 * time.Minute
		}
		versuch := slot.Add(latenz)
		gelaufen := false
		for k := 0; k < 3; k++ {
			ubiAt := versuch.Unix()
			frei := verteilAbstandFrei(letzte, 0)
			if VerteilAbstandErlaubt(ubiAt, frei) {
				if letzte > 0 && distributionRoundToSkip([]Transaction{{Type: "distribution_round_marker", DistributionAt: ubiAt}}, letzte, 0) != 0 {
					t.Fatalf("%s: gestartete Runde wuerde verworfen", versuch)
				}
				if off := versuch.Sub(slot); off > verteilAbstandNachlauf && !slot.Equal(verspaetetAm) {
					t.Fatalf("%s: %s nach 20:00", slot.Format("02.01.2006"), off)
				}
				letzte = ubiAt
				gelaufen = true
				runden++
				break
			}
			ziel, ok := VerteilAbstandWiederholung(frei, versuch, loc)
			if !ok {
				break // naechster Termin
			}
			versuch = ziel
		}
		switch {
		case slot.Equal(verspaetetAm):
			nachVerspaetung = 1
		case nachVerspaetung > 0:
			if gelaufen && versuch.Sub(slot) < 5*time.Second {
				nachVerspaetung = 0
			} else if nachVerspaetung++; nachVerspaetung > 3 {
				t.Fatalf("%s: Plan nach der Verspaetung nicht wieder bei 20:00", slot.Format("02.01.2006"))
			}
		}
	}
	if end := time.Unix(letzte, 0).In(loc); end.Hour() != 20 || end.Minute() != 0 {
		t.Fatalf("letzte Runde %s -- der Plan ist gewandert", end)
	}
	// Ausfaelle nur an den Ausnahmen (Sommerzeit-Tag, Tag nach der Verspaetung).
	if runden < 400-3 {
		t.Fatalf("nur %d von 400 Terminen gelaufen", runden)
	}
}

// Fail-closed: eine unlesbare Marke heisst keine Runde, nicht "Runde trotzdem".
func TestVerteilungAbstandFreiAb_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-verteil-abstand-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}
	if frei, err := cs.VerteilungAbstandFreiAb(); err != nil || frei != 0 {
		t.Fatalf("leere Kette: frei=%d err=%v, want 0/nil", frei, err)
	}
	if err := cs.setConfigValueDB("last_distribution_round_at", "1792000000"); err != nil {
		t.Fatal(err)
	}
	if err := cs.setConfigValueDB("last_ubi_at", "1790964000"); err != nil {
		t.Fatal(err)
	}
	if frei, err := cs.VerteilungAbstandFreiAb(); err != nil || frei != 1792000000+24*3600 {
		t.Fatalf("frei=%d err=%v, want Marke + 24 h", frei, err)
	}
	for _, kaputt := range []string{"kaputt", "-5", "1792000000x"} {
		if err := cs.setConfigValueDB("last_distribution_round_at", kaputt); err != nil {
			t.Fatal(err)
		}
		if _, err := cs.VerteilungAbstandFreiAb(); err == nil {
			t.Fatalf("Marke %q: Fehler erwartet (fail-closed)", kaputt)
		}
	}
	if err := cs.setConfigValueDB("last_distribution_round_at", "1792000000"); err != nil {
		t.Fatal(err)
	}
	if err := cs.setConfigValueDB("last_ubi_at", "kaputt"); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.VerteilungAbstandFreiAb(); err == nil {
		t.Fatal("last_ubi_at unlesbar: Fehler erwartet (fail-closed)")
	}
	if err := cs.setConfigValueDB("last_ubi_at", "1790964000"); err != nil {
		t.Fatal(err)
	}
	cs.db.Close()
	if _, err := cs.VerteilungAbstandFreiAb(); err == nil {
		t.Fatal("Datenbank nicht erreichbar: Fehler erwartet (fail-closed)")
	}
}
