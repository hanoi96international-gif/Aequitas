package keeper

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBlockListenAnsicht_SchlankUndEhrlich(t *testing.T) {
	txs := make([]Transaction, 4000)
	for i := range txs {
		txs[i] = Transaction{Type: "transfer", Wallet: "0xa", To: "0xb", Amount: 1, TxHash: "0x" + strings.Repeat("1", 64), Roh: "0x" + strings.Repeat("f", 230)}
	}
	txs[len(txs)-1].TxHash = "0xletzte"
	b := &Block{Height: 7, Hash: "0xblock", Proposer: "0xp", Transactions: txs}
	klein := &Block{Height: 6, Hash: "0xklein", Transactions: txs[:2]}

	voll, _ := json.Marshal([]*Block{b})
	schlank, err := json.Marshal(blockListenAnsicht([]*Block{b, klein, nil}))
	if err != nil {
		t.Fatal(err)
	}
	if len(schlank)*50 > len(voll) {
		t.Fatalf("Ansicht nicht schlank: %d gegen %d Bytes", len(schlank), len(voll))
	}
	if strings.Contains(string(schlank), `"roh"`) {
		t.Fatal("Rohform in der Blockliste")
	}
	var zurueck []struct {
		Height       int64         `json:"height"`
		Hash         string        `json:"hash"`
		Proposer     string        `json:"proposer"`
		TxCount      int           `json:"tx_count"`
		Gekuerzt     bool          `json:"tx_gekuerzt"`
		Transactions []Transaction `json:"transactions"`
	}
	if err := json.Unmarshal(schlank, &zurueck); err != nil {
		t.Fatal(err)
	}
	if len(zurueck) != 2 {
		t.Fatalf("%d Eintraege, erwartet 2", len(zurueck))
	}
	g := zurueck[0]
	if g.Height != 7 || g.Hash != "0xblock" || g.Proposer != "0xp" || g.TxCount != 4000 || !g.Gekuerzt || len(g.Transactions) != listenTxJeBlock {
		t.Fatalf("grosser Block falsch: h=%d hash=%s count=%d gek=%v n=%d", g.Height, g.Hash, g.TxCount, g.Gekuerzt, len(g.Transactions))
	}
	if g.Transactions[len(g.Transactions)-1].TxHash != "0xletzte" {
		t.Fatal("nicht die letzten Ueberweisungen")
	}
	if k := zurueck[1]; k.TxCount != 2 || k.Gekuerzt || len(k.Transactions) != 2 {
		t.Fatalf("kleiner Block falsch: %+v", k)
	}
	if b.Transactions[0].Roh == "" {
		t.Fatal("Ansicht hat den Block selbst veraendert")
	}
}
