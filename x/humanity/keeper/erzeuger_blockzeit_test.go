package keeper

import (
	"math"
	"testing"
	"time"
)

// Jeder andere Knoten prueft den Erzeuger zur BLOCKZEIT (erzeugerErlaubt).
// Liegt die Blockzeit vor dem Fenster des eigenen Schluessels -- kurz nach
// einem Schluesselwechsel, wenn die Auftraege eine fruehere Blockzeit
// verlangen --, entsteht kein Block, den jeder abweist
// (blockzeit_nicht_im_register), und der Auftrag bleibt liegen.
//
// Die fruehere Blockzeit erzwingt hier eine Ueberweisung mit BuchAt sieben
// Tage und 1000 s zurueck: die spaeteste Zeit, zu der jeder Nachspielende
// BuchAt uebernimmt (buchZeitBeimNachspielen), ist jetzt-1000.
func TestProduceBlock_EigenerSchluesselZurBlockzeit(t *testing.T) {
	vorherW := wirtschaftAktivOverride.Load()
	wirtschaftAktivOverride.Store(1)
	t.Cleanup(func() { wirtschaftAktivOverride.Store(vorherW); erzeugerSchnittOverride.Store(0) })
	echt := time.Now().Unix()
	self := "0x00000000000000000000000000000000000000b7"
	lauf := func(schnitt int64, fenster []zeitfenster) (string, *Block, int) {
		t.Helper()
		erzeugerSchnittOverride.Store(schnitt)
		dag, _ := newDeterminismTestDAG()
		f := map[string][]zeitfenster{}
		if fenster != nil {
			f[self] = fenster
		}
		dag.state.erzeugerRegister.Store(&erzeugerStand{fenster: f})
		// Der Erzeuger steht im Block in Pruefsummen-Schreibweise; jeder
		// Knoten vergleicht klein (AddPeerBlock).
		dag.selfProposer, dag.nodeID = self, "0x00000000000000000000000000000000000000B7"
		dag.AddTransaction(Transaction{Type: "transfer", Wallet: "0xa", To: "0xb", BuchAt: echt - 7*86400 - 1000})
		produktionLetzterGrnd.Store("")
		b := dag.ProduceBlock()
		g, _ := produktionLetzterGrnd.Load().(string)
		dag.txMu.Lock()
		n := len(dag.pendingTxs)
		dag.txMu.Unlock()
		return g, b, n
	}
	// Missbrauch: das Fenster beginnt nach der Blockzeit, aber vor jetzt --
	// jeder andere Knoten wiese den Block ab. Kein Block, der Auftrag bleibt.
	if g, b, n := lauf(1, []zeitfenster{{betreiber: "0xm1", von: echt - 500, bis: math.MaxInt64}}); g != "blockzeit_nicht_im_register" || b != nil || n != 1 {
		t.Fatalf("Blockzeit vor dem Fenster: Grund %q, Block %v, %d Auftraege liegen (erwartet: kein Block, 1)", g, b != nil, n)
	}
	// Das Fenster deckt die Blockzeit: Block mit der zurueckgenommenen Zeit.
	if g, b, _ := lauf(1, []zeitfenster{{betreiber: "0xm1", von: echt - 2000, bis: math.MaxInt64}}); b == nil || b.Timestamp != echt-1000 {
		t.Fatalf("Fenster deckt die Blockzeit: Grund %q, Block %v", g, b != nil)
	}
	// Stichtag zwischen Blockzeit und jetzt: die Peers pruefen diesen Block
	// noch nicht gegen das Register (erzeugerErlaubt) -- der Erzeuger auch
	// nicht.
	if g, b, _ := lauf(echt-500, []zeitfenster{{betreiber: "0xm1", von: echt - 700, bis: math.MaxInt64}}); b == nil {
		t.Fatalf("Blockzeit vor dem Stichtag: Grund %q, kein Block", g)
	}
	// Vor dem Stichtag, ohne jede Bindung: es wird erzeugt wie bisher.
	if g, b, _ := lauf(0, nil); b == nil || g == "blockzeit_nicht_im_register" {
		t.Fatalf("vor dem Stichtag: Grund %q, Block %v", g, b != nil)
	}
}
