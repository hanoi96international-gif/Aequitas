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

// MISSBRAUCH (Pruefung von #380, F1): kommt das Kind Y an, waehrend X noch
// nachgespielt wird, holt prefetchParentsFromDB den Kopf von X aus der DB in
// den Speicher. Von dort darf er nicht nach aussen gehen, bis das
// Nachspielen gelungen ist.
func TestSeed_VorabGeladenerKopfBleibtDrinnen_RealDB(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-seed-vorab.json")
	if !cs.useDB {
		t.Fatal("keine Datenbank")
	}
	dag := newOrphanTestDAG()
	dag.state = cs
	dag.replayedBlocks = map[string]bool{}

	x := &Block{
		Hash:      fmt.Sprintf("0xvorab%d", time.Now().UnixNano()),
		Height:    int64(8_000_000 + time.Now().UnixNano()%1_000_000),
		Proposer:  "0x000000000000000000000000000000000000beef",
		Timestamp: time.Now().Unix(),
	}
	t.Cleanup(func() { _ = cs.DeleteBlockFromDB(x.Hash) })
	if err := cs.SaveBlockToDB(x, false); err != nil {
		t.Fatalf("SaveBlockToDB: %v", err)
	}
	dag.prefetchParentsFromDB(&Block{Hash: "0xkind", ParentHashes: []string{x.Hash}})
	dag.mu.RLock()
	imSpeicher := dag.blocks[x.Hash]
	dag.mu.RUnlock()
	if imSpeicher == nil || !imSpeicher.NichtNachgespielt {
		t.Fatalf("Vorbedingung: X liegt markiert im Speicher (%v)", imSpeicher)
	}

	geliefert := func() map[string]bool {
		seit := false
		for _, b := range dag.GetBlocksSince(x.Height-1, "", 10) {
			if b.Hash == x.Hash {
				seit = true
			}
		}
		return map[string]bool{
			"GetBlocksByHashesForPeer": len(dag.GetBlocksByHashesForPeer([]string{x.Hash})) > 0,
			"GetBlockByHash":           dag.GetBlockByHash(x.Hash) != nil,
			"GetBlockByHeight":         dag.GetBlockByHeight(x.Height) != nil,
			"GetBlocksSince":           seit,
		}
	}
	for weg, da := range geliefert() {
		if da {
			t.Errorf("vor dem Nachspielen: %s liefert X aus dem Speicher", weg)
		}
	}
	// Das Nachspielen gelingt: MarkBlockReplayed und replayedBlocks (wie
	// replayTransactions).
	if err := cs.MarkBlockReplayed(context.Background(), x.Hash); err != nil {
		t.Fatal(err)
	}
	dag.replayedMu.Lock()
	dag.replayedBlocks[x.Hash] = true
	dag.replayedMu.Unlock()
	for weg, da := range geliefert() {
		if !da && weg != "GetBlockByHeight" { // Hoehe: kanonische Wahl braucht Spitzen
			t.Errorf("nach dem Nachspielen: %s liefert X nicht", weg)
		}
	}
}
