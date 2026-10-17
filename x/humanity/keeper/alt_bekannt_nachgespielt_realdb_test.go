package keeper

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Befund aus dem Vier-Knoten-Test: AddPeerBlock meldete einen Block unter dem
// Speicherfenster als bekannt (altBekannt, true), sobald er in chain_blocks
// lag -- auch wenn ein paralleler Aufruf ihn dort nur VOR dem Nachspielen
// angelegt hatte und das Nachspielen gleich scheiterte (danach wird der Kopf
// wieder geloescht). Push-Antwort "ok" und Sync-Zaehlung logen dann.
// Missbrauch: ein Peer liefert denselben Block zweimal kurz hintereinander;
// der zweite darf nicht als angenommen gelten, solange der erste nicht
// nachgespielt ist.
func TestAltBekannt_NurNachgespielt_RealDB(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-alt-bekannt-nachgespielt.json")
	if !cs.useDB {
		t.Fatal("keine Datenbank")
	}
	dag := newOrphanTestDAG()
	dag.state = cs
	cs.finalizedMu.Lock()
	cs.finalizedHeightCache = 10_000_000
	cs.finalizedBlueScoreCache = 1
	cs.finalizedCacheLoaded = true
	cs.finalizedMu.Unlock()

	b := &Block{
		Hash:      fmt.Sprintf("0xaltbekannt%d", time.Now().UnixNano()),
		Height:    1000,
		Proposer:  "0x000000000000000000000000000000000000beef",
		Timestamp: time.Now().Unix(),
	}
	if !dag.unterSpeicherfenster(b.Height) {
		t.Fatal("Testblock liegt nicht unter dem Speicherfenster")
	}
	t.Cleanup(func() { _ = cs.DeleteBlockFromDB(b.Hash) })

	pruefe := func(wann string, wantBekannt, wantInArbeit bool) {
		t.Helper()
		bekannt, inArbeit := dag.blockAltBekannt(b)
		if bekannt != wantBekannt || inArbeit != wantInArbeit {
			t.Fatalf("%s: bekannt=%v inArbeit=%v, erwartet %v/%v", wann, bekannt, inArbeit, wantBekannt, wantInArbeit)
		}
		if got := dag.schonBekannt(b); got != wantBekannt {
			t.Fatalf("%s: schonBekannt=%v, erwartet %v", wann, got, wantBekannt)
		}
	}

	pruefe("nicht in der Datenbank", false, false)

	// Kopf vor dem Nachspielen angelegt (wie AddPeerBlock).
	if err := cs.SaveBlockToDB(b, false); err != nil {
		t.Fatal(err)
	}
	pruefe("vor dem Nachspielen", false, true)

	// Nachspielen gescheitert: Kopf weg -- nichts bekannt.
	if err := cs.DeleteBlockFromDB(b.Hash); err != nil {
		t.Fatal(err)
	}
	pruefe("nach gescheitertem Nachspielen", false, false)

	// Erneut angelegt und nachgespielt: jetzt bekannt.
	if err := cs.SaveBlockToDB(b, false); err != nil {
		t.Fatal(err)
	}
	if err := cs.MarkBlockReplayed(context.Background(), b.Hash); err != nil {
		t.Fatal(err)
	}
	pruefe("nachgespielt", true, false)

	// Ueber dem Speicherfenster entscheidet dieser Weg nie.
	oben := *b
	oben.Height = 10_000_000
	if bekannt, inArbeit := dag.blockAltBekannt(&oben); bekannt || inArbeit {
		t.Fatalf("ueber dem Speicherfenster: bekannt=%v inArbeit=%v", bekannt, inArbeit)
	}
}
