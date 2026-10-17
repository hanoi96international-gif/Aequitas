package keeper

import (
	"strings"
	"testing"
	"time"
)

// Missbrauch gegen eine echte Datenbank: der Snapshot war beim Abruf noch
// innerhalb der Rueckschritt-Schranke, aber der Knoten lief waehrend des
// Abrufs weiter (das Tor ist dabei offen, PerformResync). Der Austausch prueft
// gegen die JETZIGE Hoehe und weist ab -- nichts wird geschrieben.
func TestWendeResyncSnapshotAn_RueckschrittGegenJetzigeHoehe_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-resync-vor-tor-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	t.Cleanup(cs.erzeugerInstanzFreigeben)
	t.Setenv("ALLOW_RESYNC_REGRESSION", "")
	altMax := cs.getConfigValueDB("max_block_height")
	altImport := cs.getConfigValueDB("snapshot_import_height")
	t.Cleanup(func() {
		_ = cs.setConfigValueDB("max_block_height", altMax)
		_ = cs.setConfigValueDB("snapshot_import_height", altImport)
	})

	g := &resyncSnapshot{
		snap:    &StateSnapshot{Timestamp: time.Now().Unix(), Height: 2000},
		peerURL: "http://peer.invalid/api/snapshot",
		signer:  resyncTestSigner,
	}
	// Beim Abruf: lokal 2500 -- 500 hinter, innerhalb der Schranke.
	if err := cs.setConfigValueDB("max_block_height", "2500"); err != nil {
		t.Fatal(err)
	}
	if err := cs.pruefeResyncRueckschritt(g.peerURL, g.snap.Height); err != nil {
		t.Fatalf("beim Abruf schon abgewiesen: %v", err)
	}
	// Waehrend des Abrufs weiter: lokal 3500 -- 1500 hinter.
	if err := cs.setConfigValueDB("max_block_height", "3500"); err != nil {
		t.Fatal(err)
	}
	err := cs.wendeResyncSnapshotAn(g)
	if err == nil || !strings.Contains(err.Error(), "refusing resync") {
		t.Fatalf("Austausch gegen die jetzige Hoehe: %v, erwartet Abweisung", err)
	}
	if h := cs.getConfigValueDB("snapshot_import_height"); h != altImport {
		t.Fatalf("snapshot_import_height %q -> %q: der abgewiesene Snapshot hat geschrieben", altImport, h)
	}
	if h := cs.getConfigValueDB("max_block_height"); h != "3500" {
		t.Fatalf("max_block_height %q: der abgewiesene Snapshot hat geschrieben", h)
	}
}
