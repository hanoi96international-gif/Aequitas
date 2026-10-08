package keeper

import (
	"testing"
	"time"
)

// Jeder andere Knoten prueft den Erzeuger zur BLOCKZEIT (erzeugerErlaubt).
// Liegt die Blockzeit vor dem Fenster des eigenen Schluessels -- kurz nach
// einem Schluesselwechsel, wenn die Auftraege eine fruehere Blockzeit
// verlangen --, entsteht kein Block, den jeder abweist
// (blockzeit_nicht_im_register). Hier liegt die Uhr fuer die erste Pruefung
// (nowUnix) im Fenster, die Blockzeit (time.Now) davor.
func TestProduceBlock_EigenerSchluesselZurBlockzeit(t *testing.T) {
	erzeugerSchnittOverride.Store(1)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
	echt := time.Now().Unix()
	jetzt := echt + 30*86400
	uhr(t, jetzt)
	self := "0x00000000000000000000000000000000000000b7"
	grund := func(von int64) string {
		dag, _ := newDeterminismTestDAG()
		dag.state.erzeugerRegister.Store(&erzeugerStand{fenster: map[string][]zeitfenster{
			self: {{betreiber: "0xm1", von: von, bis: 1 << 62}},
		}})
		// Der Erzeuger steht im Block in Pruefsummen-Schreibweise; jeder
		// Knoten vergleicht klein (AddPeerBlock).
		dag.selfProposer, dag.nodeID = self, "0x00000000000000000000000000000000000000B7"
		produktionLetzterGrnd.Store("")
		dag.ProduceBlock()
		g, _ := produktionLetzterGrnd.Load().(string)
		return g
	}
	// Fenster seit langem: diese Pruefung laesst durch.
	if g := grund(0); g == "blockzeit_nicht_im_register" || g == "nicht_im_register" {
		t.Fatalf("Schluessel mit Fenster zur Blockzeit: Grund %q", g)
	}
	// Fenster erst nach der Blockzeit (aber vor jetzt): kein Block.
	if g := grund(echt + 86400); g != "blockzeit_nicht_im_register" {
		t.Fatalf("Blockzeit vor dem Fenster: Grund %q, erwartet blockzeit_nicht_im_register", g)
	}
}
