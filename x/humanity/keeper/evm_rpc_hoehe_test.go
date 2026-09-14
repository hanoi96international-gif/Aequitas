package keeper

import (
	"encoding/json"
	"testing"
)

// Zwei Bloecke an einer Hoehe (Geschwister): "Block H" nach aussen traegt die
// Transaktionen beider, jede einmal, der ausgewaehlte zuerst.
func TestTransaktionenDerHoeheVereinigtGeschwister(t *testing.T) {
	dag := &BlockDAG{blocks: map[string]*Block{}, tips: map[string]bool{}, replayedBlocks: map[string]bool{}}
	s := &EVMRPCServer{dag: dag}
	kanon := &Block{Hash: "aa", Height: 7, Transactions: []Transaction{{Type: "transfer", TxHash: "0x1"}}}
	geschw := &Block{Hash: "bb", Height: 7, Transactions: []Transaction{{Type: "transfer", TxHash: "0x2"}, {Type: "transfer", TxHash: "0x1"}}}
	dag.mu.Lock()
	dag.blocks["aa"] = kanon
	dag.blocks["bb"] = geschw
	dag.mu.Unlock()
	got := s.transaktionenDerHoehe(kanon)
	if len(got) != 2 || got[0].TxHash != "0x1" || got[1].TxHash != "0x2" {
		t.Fatalf("vereinigte Liste: %+v", got)
	}
	// Ein Geschwister allein zeigt nur sich selbst.
	if n := len(s.transaktionenDerHoehe(&Block{Hash: "cc", Height: 8, Transactions: []Transaction{{TxHash: "0x9"}}})); n != 1 {
		t.Fatalf("einzelner Block: %d", n)
	}
}

func TestKanonischerBlockHashFaelltAufEnthaltendenZurueck(t *testing.T) {
	s := &EVMRPCServer{}
	if got := s.kanonischerBlockHash(5, "deadbeef"); got != "deadbeef" {
		t.Fatalf("ohne DAG: %q", got)
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(`{"a":1}`), &m); err != nil {
		t.Fatal(err)
	}
}
