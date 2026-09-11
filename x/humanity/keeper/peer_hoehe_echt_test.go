package keeper

import (
	"os"
	"strings"
	"testing"
	"time"
)

// Die Bremse hat am 11.09.2026 mehr als die Haelfte des Durchsatzes gekostet,
// weil sie mit einer Zahl rechnete, die keine Peer-Hoehe ist: beide Boxen
// standen byte-identisch auf derselben Hoehe und meldeten gegenseitig 5.160
// Bloecke Rueckstand. 100 % aller Bloecke wurden auf den Boden von 3.000
// gedrosselt, bei einem harten Deckel von 7.000.

func TestPeerHoehe_FrischerWertWirdBenutzt(t *testing.T) {
	peerEchteHoeheMu.Lock()
	peerEchteHoehe = map[string]int64{"http://a:8080": 4242}
	peerEchteHoeheAt = map[string]time.Time{"http://a:8080": time.Now()}
	peerEchteHoeheMu.Unlock()

	h, ok := echteHoeheVonPeer("http://a:8080")
	if !ok || h != 4242 {
		t.Fatalf("echteHoeheVonPeer = (%d, %v), erwartet (4242, true)", h, ok)
	}
}

// Ein veralteter Wert ist schlimmer als keiner: er wuerde eine Drosselung auf
// Daten stuetzen, die nichts mehr ueber den Partner sagen.
func TestPeerHoehe_VeralteterWertWirdVerworfen(t *testing.T) {
	peerEchteHoeheMu.Lock()
	peerEchteHoehe = map[string]int64{"http://b:8080": 99}
	peerEchteHoeheAt = map[string]time.Time{"http://b:8080": time.Now().Add(-2 * peerHoeheFrische)}
	peerEchteHoeheMu.Unlock()

	if _, ok := echteHoeheVonPeer("http://b:8080"); ok {
		t.Error("ein Wert aelter als die Frischegrenze wird noch benutzt -- dann drosselt die " +
			"Bremse auf Daten, die nichts mehr ueber den Partner sagen. Ohne frische Antwort " +
			"muss der Ersatzweg greifen.")
	}
	if _, ok := echteHoeheVonPeer("http://unbekannt:8080"); ok {
		t.Error("ein unbekannter Peer liefert einen Wert")
	}
}

// DER EIGENTLICHE FEHLER. peerSyncHeight ist ein Hoechststand, peerSyncEigeneHoehe
// war es nicht -- sie wurde bei JEDEM Aufruf gesetzt. Uebergeben wird
// highestSeen, die hoechste in EINER Sync-Runde gesehene Hoehe, nicht die Hoehe
// des Peers. Holt eine Runde nichts Neues, laeuft der eine Wert weiter und der
// andere nicht, und die Differenz waechst mit jedem eigenen Block.
func TestPeerHoehe_EigeneHoeheLaeuftNichtOhneDiePeerHoeheWeiter(t *testing.T) {
	b, err := os.ReadFile("sync_blocks.go")
	if err != nil {
		t.Fatalf("sync_blocks.go nicht lesbar: %v", err)
	}
	body := string(b)

	i := strings.Index(body, "func (dag *BlockDAG) advancePeerSyncHeight(")
	if i < 0 {
		t.Fatal("advancePeerSyncHeight nicht gefunden")
	}
	rumpf := body[i:]
	if j := strings.Index(rumpf[1:], "\nfunc "); j > 0 {
		rumpf = rumpf[:j]
	}

	if !strings.Contains(rumpf, "if fortgeschritten {") {
		t.Error("peerSyncEigeneHoehe wird wieder bei jedem Aufruf gesetzt, auch wenn die " +
			"Peer-Hoehe stehen blieb. Damit waechst der berechnete Rueckstand mit jedem " +
			"selbst produzierten Block, ohne dass der Partner zurueckliegt -- am 11.09.2026 " +
			"auf 5.160 Bloecke zwischen zwei byte-identischen Knoten, mit einer Drosselung " +
			"von 100 % aller Bloecke als Folge.")
	}
}

func TestPeerHoehe_BremseFragtZuerstDenEchtenWert(t *testing.T) {
	b, err := os.ReadFile("peer_lag_bremse.go")
	if err != nil {
		t.Fatalf("peer_lag_bremse.go nicht lesbar: %v", err)
	}
	body := string(b)

	echt := strings.Index(body, "echteHoeheVonPeer(url)")
	ersatz := strings.Index(body, "dag.peerSyncEigeneHoehe[url]")
	if echt < 0 {
		t.Fatal("die Bremse fragt die direkt gemessene Peer-Hoehe nicht mehr ab -- dann " +
			"rechnet sie wieder mit einer Zahl aus dem Sync-Zyklus, die keine Peer-Hoehe ist.")
	}
	if ersatz > 0 && echt > ersatz {
		t.Error("der Ersatzweg steht vor der direkten Messung und gewinnt damit immer.")
	}
}
