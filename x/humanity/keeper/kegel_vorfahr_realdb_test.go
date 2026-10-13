package keeper

import (
	"os"
	"testing"
)

// kopfLeserDB gegen eine echte chain_blocks: Kette, Seitenzweig, Luecke.
func TestVorfahrSuchen_DB_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-vorfahr-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	vorhanden := func(n int) string { return "vorfahr-" + hx(n)[56:] }
	t.Cleanup(func() { cs.db.Exec(`DELETE FROM chain_blocks WHERE hash LIKE 'vorfahr-%'`) })
	start := int64(1_800_000_000)
	for i := 1; i <= 20; i++ {
		eltern := "[]"
		if i > 1 {
			eltern = `["` + vorhanden(i-1) + `"]`
		}
		if _, err := cs.db.Exec(`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp) VALUES ($1, $2, $3, '0xabc', $4)`,
			vorhanden(i), i, eltern, start+int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := cs.db.Exec(`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp) VALUES ($1, 3, $2, '0xabc', $3)`,
		"vorfahr-seite", `["`+vorhanden(2)+`"]`, start+3); err != nil {
		t.Fatal(err)
	}
	lies := kopfLeserDB(cs.db)
	k, ja, voll, err := vorfahrSuchen(lies, []string{vorhanden(20)}, vorhanden(5), start)
	if err != nil || !ja || !voll || k.hoehe != 5 || k.zeit != start+5 {
		t.Fatalf("Vorfahr aus der DB: %+v %v %v %v", k, ja, voll, err)
	}
	if _, ja, voll, err := vorfahrSuchen(lies, []string{vorhanden(20)}, "vorfahr-seite", start); ja || !voll || err != nil {
		t.Fatalf("Seitenzweig: %v %v %v", ja, voll, err)
	}
	if _, err := cs.db.Exec(`DELETE FROM chain_blocks WHERE hash = $1`, vorhanden(10)); err != nil {
		t.Fatal(err)
	}
	if _, ja, voll, err := vorfahrSuchen(lies, []string{vorhanden(20)}, vorhanden(5), start); ja || voll || err != nil {
		t.Fatalf("Luecke: %v %v %v", ja, voll, err)
	}
}
