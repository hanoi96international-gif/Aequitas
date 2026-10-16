package keeper

import (
	"database/sql"
	"sync"
	"testing"
)

// Missbrauchs-/Rennen-Test fuer slashing.go loadPenaltyCacheLockedCtx: das
// erste IsValidatorSuspended (aus AddPeerBlock, ohne cs.mu) darf cs.activeTx
// nicht lesen, waehrend eine atomare Operation es unter cs.mu setzt. Unter
// -race fiel genau das im Vier-Knoten-Test auf (Leiter nimmt an, waehrend ein
// Peer-Block eintrifft). Braucht keine Datenbank: sql.Open verbindet nicht,
// die Abfrage scheitert, das Lesen des Feldes passiert vorher.
func TestStrafkontoLaden_LiestActiveTxNicht(t *testing.T) {
	db, err := sql.Open("postgres", "postgres://nichts@127.0.0.1:1/nichts?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cs := &ChainState{db: db, useDB: true}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			cs.mu.Lock()
			cs.setActiveTx(nil) // wie runAtomicWithOutbox: Schreiben unter cs.mu
			cs.mu.Unlock()
		}
	}()
	for i := 0; i < 20; i++ {
		cs.penaltyMu.Lock()
		cs.penaltyCacheLoaded = false
		cs.penaltyMu.Unlock()
		if gesperrt, _ := cs.IsValidatorSuspended("0x00000000000000000000000000000000000000aa", 1); gesperrt {
			t.Fatal("ohne Strafkonto gesperrt")
		}
	}
	wg.Wait()
}
