package keeper

import (
	"database/sql"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// Der Status nennt den Hash genau des Codes, der in der Datenbank liegt;
// ohne Code null statt eines erfundenen Werts. Echte Datenbank, wegwerfbar.
func TestVertragCodeHashAusDerDatenbank_RealDB(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("braucht DATABASE_URL (wegwerfbare lokale Datenbank)")
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec(`DROP TABLE IF EXISTS evm_contracts`)
	if _, err := db.Exec(`CREATE TABLE evm_contracts (address TEXT PRIMARY KEY, bytecode TEXT NOT NULL, deployer TEXT, deployed_at TIMESTAMP DEFAULT NOW())`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`DROP TABLE IF EXISTS evm_contracts`)
	cs := &ChainState{db: db, useDB: true}
	const addr = "0x00000000000000000000000000000000c0de0001"
	codeHashMu.Lock()
	delete(codeHashCache, addr)
	codeHashMu.Unlock()

	if st := vertragCodeStand(cs, addr); st["code_keccak256"] != nil || st["code_bytes"] != 0 {
		t.Fatalf("ohne Code muss der Hash null sein: %v", st)
	}
	code := []byte{0x60, 0x80, 0x60, 0x40, 0x52}
	if err := cs.SaveContract(addr, code, "test"); err != nil {
		t.Fatal(err)
	}
	// Leerer Fund wird hoechstens einmal je Minute neu gefragt: Zwischenspeicher leeren.
	codeHashMu.Lock()
	delete(codeHashCache, addr)
	codeHashMu.Unlock()
	h, n := vertragCodeHash(cs, addr)
	if h != crypto.Keccak256Hash(code).Hex() || n != len(code) {
		t.Fatalf("Hash %s/%d, erwartet %s/%d", h, n, crypto.Keccak256Hash(code).Hex(), len(code))
	}
}
