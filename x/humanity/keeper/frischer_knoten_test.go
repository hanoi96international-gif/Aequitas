package keeper

import (
	"os"
	"strings"
	"testing"
)

// Probe vom 12.09.2026: ein leerer Knoten (bootHeight 0) fiel am Sync-Tor
// vorbei und produzierte ab Sekunde eins eine eigene Kette ab Genesis. Das
// Tor muss ihn halten, und zwar OHNE Notausstieg, bis er einmal aufgeholt hat.
func TestFrischerKnoten_SyncTorGiltAuchBeiBootHeightNull(t *testing.T) {
	b, err := os.ReadFile("block.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	if !strings.Contains(body, "frischUndNieAufgeholt := dag.bootHeight == 0 && !dag.jemalsAufgeholt.Load()") {
		t.Fatal("das Sync-Tor unterscheidet den frischen Knoten nicht mehr -- ein leerer Knoten produziert dann wieder ab Genesis")
	}
	if !strings.Contains(body, `merkeProduktionsAusfall("frisch_noch_nie_aufgeholt")`) {
		t.Fatal("der frische Knoten hat keinen eigenen Ausfallgrund mehr -- dann ist im Protokoll nicht zu sehen, warum er wartet")
	}
	i := strings.Index(body, "frischUndNieAufgeholt {")
	j := strings.Index(body[i:], "syncStallTimeout")
	if i < 0 || j < 0 {
		t.Fatal("Aufbau des Tors nicht wiedererkannt")
	}
	// Der Notausstieg (syncStallTimeout) muss NACH dem return des frischen
	// Knotens stehen -- er darf fuer ihn nie greifen.
	if !strings.Contains(body[i:i+j], "return nil") {
		t.Fatal("der frische Knoten erreicht den Notausstieg -- nach syncStallTimeout wuerde er ohne Geschichte produzieren")
	}
}

// Ohne SELF_URL wurde frueher gar nicht gezogen; der Knoten stand allein und
// produzierte trotzdem. Jetzt zieht er und meldet sich nur nicht an.
func TestFrischerKnoten_OhneSelfURLWirdGezogen(t *testing.T) {
	b, err := os.ReadFile("sync_blocks.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	if strings.Contains(body, "no peer sync (isolated node)") {
		t.Fatal("StartPeerDiscovery kehrt ohne SELF_URL wieder vor dem Sync zurueck")
	}
	if !strings.Contains(body, "nurZiehen := selfURL == \"\"") {
		t.Fatal("der Nur-Ziehen-Modus fehlt")
	}
}

// Der frische Knoten leitet BOOTSTRAP_SIGNER aus /api/status des Seeds ab --
// das Feld muss es dort geben.
func TestFrischerKnoten_StatusNenntSignieradresse(t *testing.T) {
	b, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"validator_address": a.blockchain.SelfSigningAddress()`) {
		t.Fatal("/api/status traegt keine validator_address -- die Snapshot-Ableitung eines frischen Knotens scheitert dann immer (Probe 12.09.2026)")
	}
}
