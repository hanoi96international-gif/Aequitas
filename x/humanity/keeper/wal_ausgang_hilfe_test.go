package keeper

import (
	"database/sql"
	"os"
	"testing"
)

// walVermerkeFuerNeueDateiLeeren: eine neue WAL-Datei beginnt bei Seq 1. Die
// Ausgangsvermerke, ihre Untergrenze und wal_seq eines frueheren Tests in derselben
// Datenbank gehoeren zu einer anderen Datei -- der Start wiese sie zu Recht
// ab (walVermerkeStimmigMitDatei). Nur vor dem ersten Oeffnen einer Datei.
func walVermerkeFuerNeueDateiLeeren(t testing.TB) {
	t.Helper()
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ensureRealDBSchema(t, db)
	if _, err := db.Exec(`DELETE FROM chain_config WHERE key = $1`, walVermerkAbKey); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`TRUNCATE wal_ausgang_vermerk`); err != nil {
		t.Fatal(err)
	}
	// Ebenso die wal_seq der Konten eines frueheren Tests (walVermerkeStimmigMitDatei).
	if _, err := db.Exec(`UPDATE chain_accounts SET wal_seq = 0 WHERE wal_seq <> 0`); err != nil {
		t.Fatal(err)
	}
}
