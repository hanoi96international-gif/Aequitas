package keeper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Eine leere Runde ist harmlos -- der Peer kennt den Block vielleicht noch
// nicht. Daraufhin zu resyncen waere eine Ueberreaktion und ein Angriffsweg.
func TestAhnenLeerlauf_EineRundeReichtNicht(t *testing.T) {
	var a ahnenLeerlauf
	if aussichtslos, _ := a.merkeLeerlauf("http://seed"); aussichtslos {
		t.Fatal("eine einzelne leere Runde darf keine Heilung ausloesen")
	}
}

// Mehrere Runden hintereinander ohne einen einzigen gelieferten Elternblock
// heissen, dass der Peer die Historie nicht mehr hat -- gemessen am
// 02.09.2026: C1 hielt 580 Bloecke, C2 nur 21, weil jeder Resync
// chain_blocks leert. Aufholen ist dann unmoeglich.
func TestAhnenLeerlauf_MehrereRundenSindAussichtslos(t *testing.T) {
	var a ahnenLeerlauf
	for i := 1; i < ahnenLeerlaufSchwelle; i++ {
		if aussichtslos, _ := a.merkeLeerlauf("http://seed"); aussichtslos {
			t.Fatalf("schon bei Runde %d aussichtslos gemeldet, Schwelle ist %d", i, ahnenLeerlaufSchwelle)
		}
	}
	aussichtslos, folgen := a.merkeLeerlauf("http://seed")
	if !aussichtslos || folgen != int64(ahnenLeerlaufSchwelle) {
		t.Fatalf("nach %d leeren Runden: aussichtslos=%v folgen=%d", ahnenLeerlaufSchwelle, aussichtslos, folgen)
	}
}

// Ein Erfolg setzt nur DIESEN Peer zurueck, und Peers zaehlen getrennt.
func TestAhnenLeerlauf_JePeer(t *testing.T) {
	var a ahnenLeerlauf
	for i := 1; i < ahnenLeerlaufSchwelle; i++ {
		a.merkeLeerlauf("http://a")
	}
	a.merkeLeerlauf("http://b")
	a.merkeErfolg("http://b")
	if aussichtslos, folgen := a.merkeLeerlauf("http://a"); !aussichtslos || folgen != int64(ahnenLeerlaufSchwelle) {
		t.Fatalf("Erfolg bei b setzte a zurueck (folgen=%d)", folgen)
	}
	if _, folgen := a.merkeLeerlauf("http://b"); folgen != 1 {
		t.Fatalf("b nach Erfolg: folgen=%d, erwartet 1", folgen)
	}
}

// MISSBRAUCH: eine Flut wechselnder Peer-Adressen laesst den Zaehler nicht
// ohne Grenze wachsen.
func TestAhnenLeerlauf_Begrenzt(t *testing.T) {
	var a ahnenLeerlauf
	for i := 0; i < 50*ahnenLeerlaufMaxPeers; i++ {
		a.merkeLeerlauf(fmt.Sprintf("http://p%d", i))
		if n := len(a.folgen); n > ahnenLeerlaufMaxPeers {
			t.Fatalf("%d Eintraege, Grenze %d", n, ahnenLeerlaufMaxPeers)
		}
	}
}

// ahnenTestPeer: ein Peer, der /api/blocks/by-hash so beantwortet, wie
// antwort es sagt ("leer" = vollstaendig, kein Treffer; "fehler" = 500).
func ahnenTestPeer(t *testing.T, antwort *atomic.Value) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if antwort.Load().(string) == "fehler" {
			http.Error(w, "kaputt", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode([]*Block{})
	}))
	t.Cleanup(srv.Close)
	echt := httpSyncClient
	httpSyncClient = &http.Client{Timeout: 5 * time.Second}
	t.Cleanup(func() { httpSyncClient = echt })
	return srv
}

func ahnenTestDAG(t *testing.T, seed string) *BlockDAG {
	t.Helper()
	dag := newOrphanTestDAG()
	dag.state = &ChainState{}
	dag.orphanLastAttempt = map[string]time.Time{}
	dag.trustedSeeds = map[string]bool{}
	if seed != "" {
		dag.trustedSeeds[seed] = true
	}
	// Die Waise kam vom Seed: der Seed fuehrt ihr Kind, muesste also den
	// Elternblock haben.
	dag.orphans["0xfehlender-elternblock"] = []*Block{{Hash: "0xwaise", Height: 10, Proposer: "0xabc", FromSync: true}}
	return dag
}

// rundeAbrufen: eine Runde fetchMissingAncestors, ohne Abklingzeit.
func rundeAbrufen(dag *BlockDAG, url string) {
	dag.orphansMu.Lock()
	dag.orphanLastAttempt = map[string]time.Time{}
	dag.orphansMu.Unlock()
	dag.fetchMissingAncestors(url)
}

func heilungenZaehlen(t *testing.T) *atomic.Int64 {
	t.Helper()
	var n atomic.Int64
	vorher := ahnenLeerlaufHeilen
	ahnenLeerlaufHeilen = func(*BlockDAG, string) { n.Add(1) }
	t.Cleanup(func() { ahnenLeerlaufHeilen = vorher })
	return &n
}

// MISSBRAUCH: Netzfehler zaehlen nicht als leere Runde (der Kommentar sagte
// das immer, der Code zaehlte sie trotzdem).
func TestAhnenLeerlauf_NetzfehlerZaehlenNicht(t *testing.T) {
	heilungen := heilungenZaehlen(t)
	var antwort atomic.Value
	antwort.Store("fehler")
	srv := ahnenTestPeer(t, &antwort)
	dag := ahnenTestDAG(t, srv.URL)
	for i := 0; i < 3*ahnenLeerlaufSchwelle; i++ {
		rundeAbrufen(dag, srv.URL)
	}
	if f := dag.AhnenLeerlaufStand()["folgen"].(int64); f != 0 {
		t.Fatalf("Netzfehler als leere Runden gezaehlt: folgen=%d", f)
	}
	// Gegenprobe: vollstaendige, leere Antworten des Seeds zaehlen und heilen.
	antwort.Store("leer")
	for i := 0; i < ahnenLeerlaufSchwelle; i++ {
		rundeAbrufen(dag, srv.URL)
	}
	time.Sleep(200 * time.Millisecond)
	if heilungen.Load() != 1 {
		t.Fatalf("Seed ohne Historie: %d Heilungen, erwartet 1", heilungen.Load())
	}
}

// MISSBRAUCH: ein selbst eingetragener Peer, der leer antwortet, darf keine
// Heilung ausloesen.
func TestAhnenLeerlauf_FremderPeerHeiltNicht(t *testing.T) {
	heilungen := heilungenZaehlen(t)
	var antwort atomic.Value
	antwort.Store("leer")
	srv := ahnenTestPeer(t, &antwort)
	dag := ahnenTestDAG(t, "") // kein Seed
	for i := 0; i < 3*ahnenLeerlaufSchwelle; i++ {
		rundeAbrufen(dag, srv.URL)
	}
	time.Sleep(200 * time.Millisecond)
	if f := dag.AhnenLeerlaufStand()["folgen"].(int64); f < ahnenLeerlaufSchwelle {
		t.Fatalf("leere Runden nicht gezaehlt (folgen=%d) -- der Test zeigt den Angriff nicht", f)
	}
	if heilungen.Load() != 0 {
		t.Fatalf("fremder Peer loeste %d Heilungen aus", heilungen.Load())
	}
}

// MISSBRAUCH (Pruefung von #380, F2): ein zugelassener Validator haelt einen
// Elternblock zurueck und pusht nur dessen Kinder. Der ehrliche Seed
// antwortet vollstaendig und leer -- das darf nicht zur Heilung fuehren.
func TestAhnenLeerlauf_ZurueckgehaltenerElternblockHeiltNicht(t *testing.T) {
	heilungen := heilungenZaehlen(t)
	var antwort atomic.Value
	antwort.Store("leer")
	srv := ahnenTestPeer(t, &antwort)
	dag := ahnenTestDAG(t, srv.URL)
	// Kind per Push: FromSync = false.
	dag.orphans["0xfehlender-elternblock"] = []*Block{{Hash: "0xgepusht", Height: 10, Proposer: "0xb0e5e"}}
	for i := 0; i < 3*ahnenLeerlaufSchwelle; i++ {
		rundeAbrufen(dag, srv.URL)
	}
	time.Sleep(200 * time.Millisecond)
	if f := dag.AhnenLeerlaufStand()["folgen"].(int64); f != 0 {
		t.Fatalf("leere Runden ohne Seed-Waise gezaehlt: folgen=%d", f)
	}
	if heilungen.Load() != 0 {
		t.Fatalf("zurueckgehaltener Elternblock loeste %d Heilungen aus", heilungen.Load())
	}
}
