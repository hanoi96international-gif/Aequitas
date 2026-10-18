package keeper

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Missbrauchstests zu hauptnetz_schutz.go (docs/VERIFIER_PRINZIPIEN.md, A1):
// auf dem Hauptnetz loescht CLEAR_REGISTRATIONS nichts, auch wenn jede
// Bestaetigung gesetzt ist.

// zaehlTreiber: ein database/sql-Treiber, der jeden Zugriff zaehlt und
// keinen ausfuehrt. Begin und Prepare scheitern -- so endet auch der
// Gegenversuch (Testkette) ohne os.Exit, aber nachweislich NACH dem Tor.
type zaehlTreiber struct{ zugriffe *atomic.Int64 }
type zaehlVerbindung struct{ zugriffe *atomic.Int64 }

var errZaehlTreiber = errors.New("zaehlTreiber: kein Zugriff erlaubt")

func (d zaehlTreiber) Open(string) (driver.Conn, error) { return zaehlVerbindung(d), nil }

func (c zaehlVerbindung) Prepare(string) (driver.Stmt, error) {
	c.zugriffe.Add(1)
	return nil, errZaehlTreiber
}
func (c zaehlVerbindung) Close() error { return nil }
func (c zaehlVerbindung) Begin() (driver.Tx, error) {
	c.zugriffe.Add(1)
	return nil, errZaehlTreiber
}

var (
	zaehlTreiberZugriffe atomic.Int64
	zaehlTreiberEinmal   atomic.Bool
)

func zaehlDB(t *testing.T) *sql.DB {
	t.Helper()
	if zaehlTreiberEinmal.CompareAndSwap(false, true) {
		sql.Register("aeq_zaehl_treiber", zaehlTreiber{zugriffe: &zaehlTreiberZugriffe})
	}
	db, err := sql.Open("aeq_zaehl_treiber", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	zaehlTreiberZugriffe.Store(0)
	return db
}

// alleLoeschBestaetigungen setzt jede Variable, die clearRegistrationsFromDB
// verlangt -- der Angreifer (oder ein Fehlgriff) hat sie alle.
func alleLoeschBestaetigungen(t *testing.T) {
	t.Setenv("CLEAR_REGISTRATIONS", "true")
	t.Setenv("CLEAR_REGISTRATIONS_CONFIRM", "I_UNDERSTAND_THIS_DELETES_ALL_REGISTRATIONS")
	t.Setenv("ALLOW_DESTRUCTIVE_MAINTENANCE", "true")
}

func genesisFuerTest(t *testing.T, inhalt string, err error) {
	t.Helper()
	alt := hauptnetzGenesisLesen
	hauptnetzGenesisLesen = func() ([]byte, error) {
		if err != nil {
			return nil, err
		}
		return []byte(inhalt), nil
	}
	t.Cleanup(func() { hauptnetzGenesisLesen = alt })
}

// frischerProzess: das 5-Minuten-Fenster offen halten, damit der Gegenversuch
// nicht an ihm scheitert, sondern am Tor (oder eben nicht).
func frischerProzess(t *testing.T) {
	alt := processStartTime
	processStartTime = time.Now()
	t.Cleanup(func() { processStartTime = alt })
}

const (
	genesisHauptnetzV8 = `{"chain_id":"aequitas-1","genesis_time":"2026-09-30T18:10:00Z","register_vertrag":{"version":"v8"}}`
	genesisTestkette   = `{"chain_id":"aequitas-staging-7","genesis_time":"2026-10-01T00:00:00Z","register_vertrag":{"version":"v8"}}`
)

// Missbrauch: Hauptnetz-Genesis, alle drei Bestaetigungen, frischer Prozess.
// Es darf kein einziger Datenbankzugriff stattfinden.
func TestClearRegistrations_HauptnetzVerweigert(t *testing.T) {
	faelle := map[string]struct {
		genesis string
		lesFehl error
		primary bool
	}{
		"V8-Genesis des Hauptnetzes":                   {genesis: genesisHauptnetzV8},
		"alte Hauptnetz-Genesis":                       {genesis: `{"chain_id":"aequitas-1","genesis_time":"2026-06-13T00:00:00Z"}`},
		"Kopie mit umbenannter chain_id, gleiche Zeit": {genesis: `{"chain_id":"aequitas-test","genesis_time":"2026-09-30T18:10:00Z"}`},
		"chain_id in anderer Schreibweise":             {genesis: `{"chain_id":" AEQUITAS-1 ","genesis_time":"2027-01-01T00:00:00Z"}`},
		"genesis.json fehlt":                           {lesFehl: errors.New("no such file")},
		"genesis.json kaputt":                          {genesis: `{"chain_id":`},
		"genesis.json ohne chain_id":                   {genesis: `{"genesis_time":"2027-01-01T00:00:00Z"}`},
		"Testkette, aber IS_PRIMARY_NODE":              {genesis: genesisTestkette, primary: true},
		"ohne genesis_time":                            {genesis: `{"chain_id":"x"}`},
		"genesis_time kaputt":                          {genesis: `{"chain_id":"x","genesis_time":"kaputt"}`},
	}
	for name, f := range faelle {
		t.Run(name, func(t *testing.T) {
			alleLoeschBestaetigungen(t)
			frischerProzess(t)
			if f.primary {
				t.Setenv("IS_PRIMARY_NODE", "true")
			} else {
				t.Setenv("IS_PRIMARY_NODE", "")
			}
			genesisFuerTest(t, f.genesis, f.lesFehl)
			if hauptnetzGrund() == "" {
				t.Fatal("hauptnetzGrund leer -- der Knoten haelt das Hauptnetz fuer eine Testkette")
			}
			cs := newTestState()
			cs.db = zaehlDB(t)
			cs.useDB = true
			cs.clearRegistrationsFromDB()
			if n := zaehlTreiberZugriffe.Load(); n != 0 {
				t.Fatalf("%d Datenbankzugriffe trotz Hauptnetz -- CLEAR_REGISTRATIONS haette geloescht", n)
			}
		})
	}
}

// Gegenversuch: auf einer Testkette bleibt der Weg nutzbar -- dieselben
// Variablen fuehren bis zur Datenbank. Ohne diesen Test bewiese der obere
// nichts (ein Tor, das immer schliesst, bestuende ihn auch).
func TestClearRegistrations_TestketteBleibtNutzbar(t *testing.T) {
	alleLoeschBestaetigungen(t)
	frischerProzess(t)
	t.Setenv("IS_PRIMARY_NODE", "")
	genesisFuerTest(t, genesisTestkette, nil)
	if g := hauptnetzGrund(); g != "" {
		t.Fatalf("Testkette als Hauptnetz erkannt: %s", g)
	}
	cs := newTestState()
	cs.db = zaehlDB(t)
	cs.useDB = true
	cs.clearRegistrationsFromDB() // Begin scheitert im Zaehltreiber -> return, kein os.Exit
	if zaehlTreiberZugriffe.Load() == 0 {
		t.Fatal("auf der Testkette kein Datenbankzugriff -- der Weg ist dort nicht mehr nutzbar")
	}
}

// Die echte genesis.json des Projekts muss als Hauptnetz gelten.
func TestHauptnetz_EchteGenesisDatei(t *testing.T) {
	data, err := os.ReadFile("../../../genesis.json")
	if err != nil {
		t.Fatal(err)
	}
	if g := hauptnetzGrundAusGenesis(data); g == "" {
		t.Fatal("die genesis.json des Projekts gilt nicht als Hauptnetz")
	}
}

// Das Tor steht vor jeder anderen Pruefung und vor dem ersten
// Datenbankzugriff in clearRegistrationsFromDB.
func TestClearRegistrations_TorZuerst(t *testing.T) {
	b := functionBodyFromSource(t, "state.go", "func (cs *ChainState) clearRegistrationsFromDB(")
	tor := strings.Index(b, "hauptnetzGrund()")
	if tor < 0 {
		t.Fatal("clearRegistrationsFromDB prueft hauptnetzGrund nicht")
	}
	for _, spaeter := range []string{"processStartTime", "CLEAR_REGISTRATIONS_CONFIRM", "cs.db.Begin()", "cs.db.QueryRow("} {
		if i := strings.Index(b, spaeter); i >= 0 && i < tor {
			t.Fatalf("%s steht vor dem Hauptnetz-Tor", spaeter)
		}
	}
}

// Missbrauch mit echter Datenbank: ein registrierter Mensch mit Guthaben
// bleibt nach CLEAR_REGISTRATIONS auf dem Hauptnetz Mensch, mit Guthaben und
// Nullifier.
func TestClearRegistrations_Hauptnetz_RealDB(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-hauptnetz-schutz.json")
	if !cs.useDB {
		t.Fatal("keine Datenbank")
	}
	const mensch = "0x00000000000000000000000000000000000a1a1a"
	if _, err := cs.db.Exec(`INSERT INTO chain_accounts (address, balance, is_human) VALUES ($1, 1000, true)
		ON CONFLICT (address) DO UPDATE SET balance = 1000, is_human = true`, mensch); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.db.Exec(`INSERT INTO nullifiers (nullifier, wallet_address) VALUES ('hauptnetz-schutz-nullifier', $1) ON CONFLICT DO NOTHING`, mensch); err != nil {
		t.Fatal(err)
	}
	alleLoeschBestaetigungen(t)
	frischerProzess(t)
	t.Setenv("IS_PRIMARY_NODE", "")
	genesisFuerTest(t, genesisHauptnetzV8, nil)
	cs.clearRegistrationsFromDB()

	var human bool
	var bal float64
	if err := cs.db.QueryRowContext(context.Background(), `SELECT is_human, balance FROM chain_accounts WHERE address = $1`, mensch).Scan(&human, &bal); err != nil {
		t.Fatal(err)
	}
	if !human || bal != 1000 {
		t.Fatalf("nach CLEAR_REGISTRATIONS auf dem Hauptnetz: is_human=%v balance=%v", human, bal)
	}
	var n int
	if err := cs.db.QueryRow(`SELECT count(*) FROM nullifiers WHERE nullifier = 'hauptnetz-schutz-nullifier'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("Nullifier geloescht (n=%d, err=%v)", n, err)
	}
}
