package keeper

import (
	"database/sql"
	"os"
	"testing"
	"time"
)

// Echte Datenbank (DATABASE_URL, wegwerfbar). Ohne sie: uebersprungen.
func herkunftTestDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("braucht DATABASE_URL (wegwerfbare lokale Datenbank)")
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		proveHerkunftDB.Store(nil)
		db.Exec(`DROP TABLE IF EXISTS prove_herkunft`)
		db.Close()
	})
	db.Exec(`DROP TABLE IF EXISTS prove_herkunft`)
	richteProveHerkunftDBEin(db)
	if proveHerkunftDB.Load() == nil {
		t.Fatal("Tabelle nicht angelegt")
	}
	return db
}

// arbeitsspeicherLeeren stellt einen Neustart nach.
func arbeitsspeicherLeeren() {
	proveHerkunft.Range(func(k, _ any) bool { proveHerkunft.Delete(k); return true })
}

// Gutfall: Die Notiz uebersteht einen Neustart -- fuer genau diese Wallet.
// Missbrauch: Eine fremde Wallet bekommt sie auch aus der Tabelle nicht.
func TestProveHerkunftUeberstehtNeustart_RealDB(t *testing.T) {
	herkunftTestDB(t)
	const alice = "0xa11ce00000000000000000000000000000000001"
	const mallory = "0x0000000000000000000000000000000000bad001"
	merkeProveHerkunft([]byte(`{"wallet":"`+alice+`"}`), []byte(`{"zkNullifier":"0xC0FFEE01","circuitVersion":3}`))
	arbeitsspeicherLeeren()
	if !hatProveHerkunft("0xc0ffee01", alice) {
		t.Fatal("nach dem Neustart keine Herkunft fuer die richtige Wallet")
	}
	arbeitsspeicherLeeren()
	if hatProveHerkunft("0xc0ffee01", mallory) {
		t.Fatal("fremde Wallet bekommt die Herkunft aus der Tabelle")
	}
}

// Missbrauch: Eine abgelaufene Zeile gilt nicht, auch nicht nach Neustart.
func TestProveHerkunftAusTabelleVerfaellt_RealDB(t *testing.T) {
	db := herkunftTestDB(t)
	const alice = "0xa11ce00000000000000000000000000000000001"
	alt := time.Now().Add(-proveHerkunftTTL - time.Minute).Unix()
	if _, err := db.Exec(`INSERT INTO prove_herkunft (nullifier, wallet, zeit_unix) VALUES ($1,$2,$3)`,
		herkunftsSchluessel("0xa1f001"), alice, alt); err != nil {
		t.Fatal(err)
	}
	arbeitsspeicherLeeren()
	if hatProveHerkunft("0xa1f001", alice) {
		t.Fatal("abgelaufene Herkunft aus der Tabelle angenommen")
	}
	// Beim naechsten Schreiben wird sie geloescht.
	merkeProveHerkunft([]byte(`{"wallet":"`+alice+`"}`), []byte(`{"zkNullifier":"0xB0B001","circuitVersion":3}`))
	var n int
	db.QueryRow(`SELECT count(*) FROM prove_herkunft WHERE zeit_unix = $1`, alt).Scan(&n)
	if n != 0 {
		t.Fatalf("abgelaufene Zeile nicht geloescht: %d", n)
	}
}

// Fehlerfall: Ist die Datenbank weg, gibt es keine Herkunft (schliesst ab).
func TestProveHerkunftDatenbankWegSchliesstAb_RealDB(t *testing.T) {
	db := herkunftTestDB(t)
	const alice = "0xa11ce00000000000000000000000000000000001"
	merkeProveHerkunft([]byte(`{"wallet":"`+alice+`"}`), []byte(`{"zkNullifier":"0xDEAD01","circuitVersion":3}`))
	arbeitsspeicherLeeren()
	db.Close()
	if hatProveHerkunft("0xdead01", alice) {
		t.Fatal("bei kaputter Datenbank trotzdem Herkunft")
	}
}
