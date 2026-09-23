package keeper

import (
	"context"
	"testing"
)

// DER ZAEHLER MUSS EINEN NEUSTART UEBERLEBEN.
//
// Am 15.09.2026 stand auf beiden Boxen "0 uebersprungen", und daraus wurde
// geschlossen, es sei nie eine Ueberweisung uebersprungen worden. Der Schluss
// war nicht gedeckt: uebersprungeneUeberweisungen ist ein atomic.Int64 im
// Prozess und faengt nach jedem Neustart und jedem Resync bei null an.
//
// Das ist fuer eine DAUERHAFTE Divergenz das falsche Werkzeug, und die
// Kombination mit /api/wache macht es schlimmer: die Wache faerbt bei > 0 rot,
// und der Neustart, den ein Betreiber nach einem roten Alarm als Erstes
// versucht, loescht den Alarm, ohne die Divergenz zu beheben. Es SIEHT aus,
// als haette der Neustart geholfen.
//
// Diese Tests halten fest: die Summe ueberlebt, sie wird nur einmal je Block
// geschrieben, ein Block ohne Ueberspringen kostet keinen Schreibvorgang, und
// abgeraeumt wird sie ausschliesslich beim Resync.
//
// Sie brauchen eine echte Datenbank: chain_config ist ohne sie ein No-op in
// BEIDE Richtungen (setConfigValueCtx und getConfigValueDB kehren bei
// cs.db == nil sofort zurueck), ein Test ohne Postgres wuerde also gruen sein,
// ohne irgendetwas zu pruefen. Opt-in wie jeder andere _RealDB-Test hier.

// zaehlerTestKette gibt eine Kette mit echter Datenbank und einem sauberen
// Zaehlerstand -- und raeumt beides hinterher wieder ab.
func zaehlerTestKette(t *testing.T) *ChainState {
	t.Helper()
	truncateDistTestTables(t) // auch das Opt-in-Tor
	cs := testKnoten(t, "unused-uebersprungen-dauerhaft-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung -- DATABASE_URL pruefen")
	}
	uebersprungeneUeberweisungen.Store(0)
	t.Cleanup(func() {
		uebersprungeneUeberweisungen.Store(0)
		cs.setConfigValue(uebersprungenConfigKey, "0")
	})
	return cs
}

func TestUebersprungen_UeberlebtDenNeustart_RealDB(t *testing.T) {
	cs := zaehlerTestKette(t)

	// Ein Lauf, in dem drei Ueberweisungen uebersprungen wurden.
	for i := 0; i < 3; i++ {
		merkeUebersprungeneUeberweisung()
	}
	cs.uebersprungeneSichern(context.Background(), 3)

	if got := cs.getConfigValue(uebersprungenConfigKey); got != "3" {
		t.Fatalf("dauerhafter Stand = %q, erwartet \"3\" -- ohne ihn meldet die Wache nach dem naechsten Neustart gruen, obwohl die Divergenz steht", got)
	}

	// Der Neustart: neuer Prozess, Zaehler bei null, Konfiguration bleibt.
	uebersprungeneUeberweisungen.Store(0)
	cs.uebersprungeneLaden()
	if got := uebersprungeneUeberweisungen.Load(); got != 3 {
		t.Fatalf("nach dem Neustart = %d, erwartet 3 -- der Alarm waere mit dem Neustart verschwunden, die Divergenz nicht", got)
	}
}

func TestUebersprungen_BlockOhneUeberspringenSchreibtNichts_RealDB(t *testing.T) {
	cs := zaehlerTestKette(t)

	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.uebersprungeneSichern(context.Background(), 0)
	if got, da := cs.getConfigValueExists(uebersprungenConfigKey); da && got != "" {
		t.Fatalf("ein Block ohne Ueberspringen hat geschrieben (%q) -- der Normalfall muss den Geldpfad nichts kosten", got)
	}
}

func TestUebersprungen_NurDerResyncRaeumtAb_RealDB(t *testing.T) {
	cs := zaehlerTestKette(t)

	merkeUebersprungeneUeberweisung()
	cs.uebersprungeneSichern(context.Background(), 1)

	// Ein weiterer Start aendert nichts daran, dass die Divergenz steht.
	uebersprungeneUeberweisungen.Store(0)
	cs.uebersprungeneLaden()
	if uebersprungeneUeberweisungen.Load() != 1 {
		t.Fatal("der Stand haette den Start ueberstehen muessen")
	}

	// Erst der Resync -- der Vorgang, der beide Boxen wieder gleichstellt.
	cs.UebersprungeneZuruecksetzen()
	if got := uebersprungeneUeberweisungen.Load(); got != 0 {
		t.Fatalf("nach dem Resync = %d, erwartet 0", got)
	}
	uebersprungeneUeberweisungen.Store(99)
	cs.uebersprungeneLaden()
	if got := uebersprungeneUeberweisungen.Load(); got != 0 {
		t.Fatalf("der Resync hat den dauerhaften Stand nicht abgeraeumt: nach dem Laden = %d", got)
	}
}

// Ein kaputter Wert darf den Knoten nicht am Starten hindern. Ein Zaehler ist
// eine Diagnose, kein Kettenzustand.
func TestUebersprungen_KaputterWertStartetTrotzdem_RealDB(t *testing.T) {
	cs := zaehlerTestKette(t)

	for _, kaputt := range []string{"keine Zahl", "-5", ""} {
		if err := cs.setConfigValue(uebersprungenConfigKey, kaputt); err != nil {
			t.Fatalf("setConfigValue: %v", err)
		}
		uebersprungeneUeberweisungen.Store(7)
		cs.uebersprungeneLaden() // darf nicht in Panik geraten
		if got := uebersprungeneUeberweisungen.Load(); got != 7 {
			t.Fatalf("bei %q wurde der Zaehler auf %d veraendert -- ein unlesbarer Wert soll ihn in Ruhe lassen", kaputt, got)
		}
	}
}
