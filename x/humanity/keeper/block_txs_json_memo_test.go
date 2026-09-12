package keeper

import "testing"

// Das Memo darf den Hash nicht veraendern -- weder gefuellt noch leer, weder
// auf dem Produktions- noch auf dem Empfangsweg.
func TestTransaktionenJSONMemo_HashUndRootBleibenGleich(t *testing.T) {
	txs := make([]Transaction, 500)
	for i := range txs {
		txs[i] = Transaction{Type: "transfer", Wallet: "0xa", To: "0xb", Amount: float64(i)}
	}
	ohne := &Block{Height: 9, ParentHashes: []string{"p"}, Proposer: "0xp", Timestamp: 1, Transactions: txs}
	mit := &Block{Height: 9, ParentHashes: []string{"p"}, Proposer: "0xp", Timestamp: 1, Transactions: txs}
	mit.merkeTransaktionenJSON()
	if mit.txsJSON == nil || mit.txsJSONFuer != 500 {
		t.Fatal("Memo nicht gefuellt")
	}
	if calculateBlockHash(ohne) != calculateBlockHash(mit) {
		t.Fatal("Hash mit Memo weicht vom Hash ohne Memo ab")
	}
	if txBatchRoot(txs) != txBatchRootJSON(mit.transaktionenJSON(), len(txs)) {
		t.Fatal("TxRoot ueber das Memo weicht ab")
	}
	db1, _ := ohne.transaktionenJSONFuerDB()
	db2, _ := mit.transaktionenJSONFuerDB()
	if string(db1) != string(db2) {
		t.Fatal("DB-Kodierung ueber das Memo weicht ab")
	}
}

// Ein veraltetes Memo (Liste ersetzt) darf nie benutzt werden.
func TestTransaktionenJSONMemo_VeraltetesMemoWirdIgnoriert(t *testing.T) {
	b := &Block{Transactions: []Transaction{{Type: "transfer", Wallet: "0xa"}}}
	b.merkeTransaktionenJSON()
	alt := string(b.transaktionenJSON())
	b.Transactions = []Transaction{{Type: "transfer", Wallet: "0xa"}, {Type: "transfer", Wallet: "0xb"}}
	if string(b.transaktionenJSON()) == alt {
		t.Fatal("Memo fuer eine andere Liste benutzt")
	}
	b.Transactions = nil
	if string(b.transaktionenJSON()) != "[]" {
		t.Fatalf("leere Liste muss als [] kodieren, nicht %q", b.transaktionenJSON())
	}
	db, _ := b.transaktionenJSONFuerDB()
	if string(db) != "null" {
		t.Fatalf("DB-Kodierung einer nil-Liste bleibt null, nicht %q", db)
	}
}
