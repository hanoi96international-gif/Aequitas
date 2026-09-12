package keeper

import (
	"os"
	"strings"
	"testing"
)

// Gemessen am 12.09.2026 unter Last: LoadPendingTxsWithLimit brauchte fuer
// 5.635 Transaktionen 826 ms und lief unter dag.mu. Solange stand der Replay
// des Partners, dessen ProduceBlock wartete bis zu 1,7 s auf dieselbe Sperre,
// und der Takt fiel auf 0,76 Bloecke je Sekunde. Das Laden braucht die Sperre
// nicht -- pending_txs liest nur dieser Knoten.
func TestProduceBlock_LaedtPendingTransaktionenVorDerSperre(t *testing.T) {
	b, err := os.ReadFile("block.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	start := strings.Index(body, "func (dag *BlockDAG) ProduceBlock() *Block {")
	if start < 0 {
		t.Fatal("ProduceBlock nicht gefunden")
	}
	rumpf := body[start:]

	laden := strings.Index(rumpf, "dag.state.LoadPendingTxsWithLimit(")
	sperre := strings.Index(rumpf, "dag.replayMu.Lock()")
	warten := strings.Index(rumpf, "defer pendingWG.Wait()")
	if laden < 0 || sperre < 0 {
		t.Fatal("Laden oder Sperre nicht gefunden -- Test umhaengen, nicht loeschen")
	}
	if laden > sperre {
		t.Error("LoadPendingTxsWithLimit steht wieder HINTER dag.replayMu.Lock(). Das sind bis " +
			"zu 826 ms Postgres unter der DAG-Sperre, in denen der Replay des Partners steht " +
			"und dessen Takt faellt. pending_txs liest nur dieser Knoten; das Laden braucht " +
			"die Sperre nicht.")
	}
	if warten < 0 {
		t.Fatal("defer pendingWG.Wait() fehlt. Ohne es kann ein fruehes return (Tor geschlossen) " +
			"den Lader im Pool zuruecklassen, und der naechste Tick blockiert in submit.")
	}
	if warten > sperre {
		t.Error("defer pendingWG.Wait() ist NACH den Sperr-Defers eingetragen und liefe damit " +
			"VOR deren Freigabe: ein geschlossenes Tor hielte die Sperre, bis Postgres fertig ist.")
	}
	// Der StateRoot muss unter der Sperre bleiben: er hasht den Zustand, den
	// ein Replay gerade aendern koennte.
	root := strings.Index(rumpf, "stateRoot = dag.state.StateRoot()")
	if root >= 0 && root < sperre {
		t.Error("der StateRoot wird vor der Sperre berechnet -- ein Replay kann ihn waehrenddessen aendern")
	}
}
