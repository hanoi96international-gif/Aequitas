package keeper

import (
	"database/sql"
	"errors"
	"os"
	"testing"
)

// Befund C1-Pruefstand 01.10.2026: ein Konto, dessen Zeile Version 0 traegt
// (Altzeilen von vor der Versionsspalte, DEFAULT 0), wurde beim Nachladen nur
// im Speicher auf 1 gehoben. Jede gepruefte Speicherung danach meldete
// "version conflict", und die Ruecknahme scheiterte an derselben Pruefung.
func TestNachladen_VersionNullWirdAuchInDerZeileGehoben_RealDB(t *testing.T) {
	truncateDistTestTables(t) // Opt-in-Tor: AEQUITAS_TPS_BENCH=1 und DATABASE_URL
	cs := testKnoten(t, "unused-version-null.json")
	if !cs.useDB {
		t.Fatal("keine Datenbank")
	}
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const addr = "0x00000000000000000000000000000000000a0001"
	const fremd = "0x00000000000000000000000000000000000a0002"
	for _, a := range []string{addr, fremd} {
		if _, err := db.Exec(`INSERT INTO chain_accounts (address, balance, version) VALUES ($1, 100, 0)
			ON CONFLICT (address) DO UPDATE SET balance = 100, version = 0`, a); err != nil {
			t.Fatal(err)
		}
	}

	cs.mu.Lock()
	cs.ensureAccountLoaded(addr)
	acc, ok := cs.accounts.Get(addr)
	if !ok {
		cs.mu.Unlock()
		t.Fatal("Konto nicht nachgeladen")
	}
	var zeile int64
	db.QueryRow(`SELECT version FROM chain_accounts WHERE address = $1`, addr).Scan(&zeile)
	if acc.Version != zeile {
		cs.mu.Unlock()
		t.Fatalf("nach dem Nachladen: Speicher Version %d, Zeile Version %d -- muessen gleich sein", acc.Version, zeile)
	}
	acc.Balance = NewDecimal(90)
	err = cs.saveAccountToDB(acc)
	cs.mu.Unlock()
	if err != nil {
		t.Fatalf("Speichern nach dem Nachladen scheiterte: %v", err)
	}
	var bal float64
	db.QueryRow(`SELECT balance, version FROM chain_accounts WHERE address = $1`, addr).Scan(&bal, &zeile)
	if bal != 90 || zeile != acc.Version {
		t.Fatalf("Zeile: Saldo %v Version %d, Speicher Version %d", bal, zeile, acc.Version)
	}

	// Missbrauch: die Pruefung bleibt scharf. Aendert jemand anderes die
	// Zeile nach dem Nachladen, MUSS die Speicherung als Konflikt scheitern
	// und darf den fremden Stand nicht ueberschreiben.
	cs.mu.Lock()
	cs.ensureAccountLoaded(fremd)
	facc, _ := cs.accounts.Get(fremd)
	if _, err := db.Exec(`UPDATE chain_accounts SET balance = 55, version = version + 1 WHERE address = $1`, fremd); err != nil {
		cs.mu.Unlock()
		t.Fatal(err)
	}
	facc.Balance = NewDecimal(1)
	err = cs.saveAccountToDB(facc)
	cs.mu.Unlock()
	if !errors.Is(err, errVersionConflict) {
		t.Fatalf("fremde Aenderung muss als Konflikt scheitern, bekam %v", err)
	}
	db.QueryRow(`SELECT balance FROM chain_accounts WHERE address = $1`, fremd).Scan(&bal)
	if bal != 55 {
		t.Fatalf("fremder Stand ueberschrieben: Saldo %v", bal)
	}
}
