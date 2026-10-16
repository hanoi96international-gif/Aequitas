package keeper

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// MISSBRAUCH (Pruefung von #375): ein gepushter Block steht beim Seed schon
// VOR dem Nachspielen in chain_blocks (replayed = false). In diesem Fenster
// lieferte jeder Lesweg nach aussen ihn aus -- ein Nachholender fuehrte ihn
// dann als FromSync, also als "vom Seed gefuehrt", und die Replay-Mauer
// hielte das fuer einen Beleg. Jetzt liefert der Seed nur nachgespielte
// Bloecke; interne Wege lesen weiter alles.
func TestSeed_LiefertNurNachgespielte_RealDB(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-seed-nachgespielt.json")
	if !cs.useDB {
		t.Fatal("keine Datenbank")
	}
	dag := newOrphanTestDAG()
	dag.state = cs

	hoehe := int64(9_000_000 + time.Now().UnixNano()%1_000_000)
	b := &Block{
		Hash:      fmt.Sprintf("0xseedfenster%d", time.Now().UnixNano()),
		Height:    hoehe,
		Proposer:  "0x000000000000000000000000000000000000beef",
		Timestamp: time.Now().Unix(),
	}
	t.Cleanup(func() { _ = cs.DeleteBlockFromDB(b.Hash) })
	if err := cs.SaveBlockToDB(b, false); err != nil {
		t.Fatalf("SaveBlockToDB: %v", err)
	}

	pruefe := func(wann string, erwartet bool) {
		t.Helper()
		gefunden := map[string]bool{
			"GetBlocksByHashesForPeer": len(dag.GetBlocksByHashesForPeer([]string{b.Hash})) > 0,
			"GetBlockByHash":           dag.GetBlockByHash(b.Hash) != nil,
			"GetBlockByHeight":         dag.GetBlockByHeight(hoehe) != nil,
		}
		// GetBlocksSince liest die DB nur unterhalb der Finalitaet -- dort
		// mit genau dieser Abfrage.
		seit := false
		liste, err := cs.LoadNachgespielteBlocksSinceFromDB(hoehe-1, "", 10)
		if err != nil {
			t.Fatalf("LoadNachgespielteBlocksSinceFromDB: %v", err)
		}
		for _, x := range liste {
			if x.Hash == b.Hash {
				seit = true
			}
		}
		gefunden["GetBlocksSince (DB)"] = seit
		for weg, da := range gefunden {
			if da != erwartet {
				t.Errorf("%s: %s liefert den Block = %v, erwartet %v", wann, weg, da, erwartet)
			}
		}
	}

	pruefe("vor dem Nachspielen", false)
	// Gegenprobe: interne Wege sehen den Kopf weiter (Finalitaet, Start).
	if cs.LoadBlockFromDBByHash(b.Hash) == nil {
		t.Fatal("interner Lesweg sieht den Kopf nicht mehr")
	}
	if alle, err := cs.LoadBlocksSinceFromDB(hoehe-1, "", 10); err != nil || len(alle) == 0 {
		t.Fatalf("interner Bereichs-Lesweg sieht den Kopf nicht mehr (%v)", err)
	}
	if err := cs.MarkBlockReplayed(context.Background(), b.Hash); err != nil {
		t.Fatalf("MarkBlockReplayed: %v", err)
	}
	pruefe("nach dem Nachspielen", true)
}
