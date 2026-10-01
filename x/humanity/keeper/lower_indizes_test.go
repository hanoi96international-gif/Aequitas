package keeper

import (
	"database/sql"
	"os"
	"strings"
	"testing"
)

// Die Nonce-Abfrage jeder Ueberweisung (LoadNonce) und die Kontoabfrage
// muessen einen Index benutzen -- sonst lesen sie die ganze Tabelle, und das
// wird mit jedem Konto des Netzes langsamer (lower_indizes.go).
func TestLowerIndizes_AbfragenBenutzenIndex_RealDB(t *testing.T) {
	truncateDistTestTables(t) // Opt-in-Tor: AEQUITAS_TPS_BENCH=1 und DATABASE_URL
	cs := testKnoten(t, "unused-lower-indizes.json")
	if !cs.useDB {
		t.Fatal("keine Datenbank")
	}
	if fehlend := cs.lowerIndizesBauen(); len(fehlend) > 0 {
		t.Fatalf("nicht gebaut: %v", fehlend)
	}
	// Zweimal ist dasselbe wie einmal (bei jedem Start aufgerufen).
	if fehlend := cs.lowerIndizesBauen(); len(fehlend) > 0 {
		t.Fatalf("zweiter Lauf: nicht gebaut: %v", fehlend)
	}

	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Genug Zeilen, dass der Planer einen Index einer Tabellen-Lesung vorzieht.
	if _, err := db.Exec(`INSERT INTO evm_nonces (address, nonce)
		SELECT '0x' || lpad(to_hex(g), 40, '0'), g FROM generate_series(1, 20000) g
		ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ANALYZE evm_nonces`); err != nil {
		t.Fatal(err)
	}
	plan := func(q string, args ...interface{}) string {
		rows, err := db.Query(`EXPLAIN `+q, args...)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var b strings.Builder
		for rows.Next() {
			var z string
			rows.Scan(&z)
			b.WriteString(z + "\n")
		}
		return b.String()
	}
	p := plan(`SELECT nonce FROM evm_nonces WHERE lower(address) = $1`, "0x0000000000000000000000000000000000000abc")
	if strings.Contains(p, "Seq Scan") || !strings.Contains(p, "idx_evm_nonces_lower") {
		t.Fatalf("LoadNonce liest ohne Index:\n%s", p)
	}

	// Und dasselbe Ergebnis wie vorher: gemischt geschriebene Adresse findet
	// die klein gespeicherte Zeile.
	addr := "0x00000000000000000000000000000000000000ff"
	if got := cs.LoadNonce(strings.ToUpper(addr[:2]) + strings.ToUpper(addr[2:])); got != 255 {
		t.Fatalf("LoadNonce = %d, erwartet 255", got)
	}
}
