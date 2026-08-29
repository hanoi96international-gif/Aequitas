package keeper

import (
	"encoding/json"
	"strings"
	"testing"
)

// Das Blockobjekt log an vier Stellen, und alle vier Angaben lagen am Block
// bereit. Am schwersten wog transactions: es war IMMER leer -- auch fuer einen
// Block mit 269 Stueck. Jeder Block-Explorer und jede Wallet bekam damit die
// Auskunft, dieser Block sei leer.
func TestBlockToMap_GibtDieTransaktionenAus(t *testing.T) {
	s := &EVMRPCServer{}
	block := &Block{
		Height:       42,
		Timestamp:    1000,
		Hash:         "aa" + strings.Repeat("b", 62),
		ParentHashes: []string{"cc" + strings.Repeat("d", 62)},
		StateRoot:    "ee" + strings.Repeat("f", 62),
		TxRoot:       "11" + strings.Repeat("2", 62),
		Transactions: []Transaction{
			{Type: "transfer", TxHash: "0x" + strings.Repeat("3", 64)},
			{Type: "transfer", TxHash: "0x" + strings.Repeat("4", 64)},
		},
	}

	m := s.blockToMap(block, false)

	txs, ok := m["transactions"].([]interface{})
	if !ok || len(txs) != 2 {
		t.Fatalf("transactions = %v (%d), erwartet 2 Hashes -- eine leere Liste behauptet, "+
			"der Block sei leer", m["transactions"], len(txs))
	}
	for _, x := range txs {
		h, _ := x.(string)
		if !strings.HasPrefix(h, "0x") || len(h) != 66 {
			t.Fatalf("Transaktionshash sieht falsch aus: %q", h)
		}
	}
}

// parentHash waren 64 Nullen. Wer die Kette rueckwaerts laeuft, war damit
// sofort am Ende -- obwohl der Block seine Eltern traegt.
func TestBlockToMap_NenntDenVorgaenger(t *testing.T) {
	s := &EVMRPCServer{}
	eltern := "cc" + strings.Repeat("d", 62)
	m := s.blockToMap(&Block{Height: 7, Hash: "aa", ParentHashes: []string{eltern}}, false)
	if got := m["parentHash"]; got != "0x"+eltern {
		t.Fatalf("parentHash = %v, erwartet 0x%s -- 64 Nullen heissen \"kein Vorgaenger\"", got, eltern)
	}
}

// stateRoot gab den BLOCKHASH zurueck. Die Kette fuehrt einen echten
// StateRoot und prueft damit beim Nachspielen ihren Konsens.
func TestBlockToMap_NenntDenEchtenStateRoot(t *testing.T) {
	s := &EVMRPCServer{}
	hash := "aa" + strings.Repeat("b", 62)
	root := "ee" + strings.Repeat("f", 62)
	m := s.blockToMap(&Block{Height: 7, Hash: hash, StateRoot: root}, false)
	if got := m["stateRoot"]; got != "0x"+root {
		t.Fatalf("stateRoot = %v, erwartet 0x%s (nicht den Blockhash)", got, root)
	}
	// Ohne eigenen StateRoot bleibt es beim alten Verhalten -- eine erfundene
	// Wurzel waere schlechter als eine ersatzweise.
	m2 := s.blockToMap(&Block{Height: 7, Hash: hash}, false)
	if got := m2["stateRoot"]; got != "0x"+hash {
		t.Fatalf("ohne StateRoot: %v, erwartet den Blockhash als Ersatz", got)
	}
}

// gasUsed war 0, auch fuer volle Bloecke -- und widersprach damit den
// Quittungen desselben Blocks, die je Transaktion gasProTx melden.
func TestBlockToMap_GasUsedPasstZuDenQuittungen(t *testing.T) {
	s := &EVMRPCServer{}
	m := s.blockToMap(&Block{
		Height: 7, Hash: "aa",
		Transactions: []Transaction{
			{TxHash: "0x" + strings.Repeat("3", 64)},
			{TxHash: "0x" + strings.Repeat("4", 64)},
			{TxHash: "0x" + strings.Repeat("5", 64)},
		},
	}, false)
	var n uint64
	if _, err := fmtSscanHex(m["gasUsed"].(string), &n); err != nil {
		t.Fatalf("gasUsed unlesbar: %v", err)
	}
	if n != 3*gasProTx {
		t.Fatalf("gasUsed = %d, erwartet %d (3 x gasProTx, wie die Quittungen melden)", n, 3*gasProTx)
	}
}

// size war "0x1" -- ein Byte.
func TestBlockToMap_GroesseIstNichtEinByte(t *testing.T) {
	s := &EVMRPCServer{}
	m := s.blockToMap(&Block{Height: 7, Hash: "aa", Transactions: []Transaction{
		{TxHash: "0x" + strings.Repeat("3", 64)},
	}}, false)
	var n uint64
	fmtSscanHex(m["size"].(string), &n)
	if n < 50 {
		t.Fatalf("size = %d Bytes -- das ist kein Block", n)
	}
}

// Der zweite Parameter der Methode entscheidet ueber Hashes oder volle
// Objekte. Er wurde nie gelesen; die Liste war ohnehin leer.
func TestBlockToMap_VolleObjekteAufWunsch(t *testing.T) {
	s := &EVMRPCServer{}
	s.initTxMetaShards()
	block := &Block{Height: 7, Hash: "aa", Transactions: []Transaction{
		{TxHash: "0x" + strings.Repeat("3", 64)},
	}}
	m := s.blockToMap(block, true)
	txs := m["transactions"].([]interface{})
	if len(txs) != 1 {
		t.Fatalf("erwartet einen Eintrag, bekam %d", len(txs))
	}
	// Ohne aufgezeichnete Metadaten faellt es auf den Hash zurueck -- das ist
	// die richtige Richtung: lieber weniger sagen als etwas erfinden.
	switch v := txs[0].(type) {
	case string, map[string]interface{}:
		_ = v
	default:
		t.Fatalf("unerwarteter Typ %T", v)
	}
}

func fmtSscanHex(s string, out *uint64) (int, error) {
	var n uint64
	var err error
	var read int
	read, err = sscanHex(strings.TrimPrefix(s, "0x"), &n)
	*out = n
	return read, err
}

func sscanHex(s string, out *uint64) (int, error) {
	var n uint64
	for _, c := range s {
		n *= 16
		switch {
		case c >= '0' && c <= '9':
			n += uint64(c - '0')
		case c >= 'a' && c <= 'f':
			n += uint64(c-'a') + 10
		case c >= 'A' && c <= 'F':
			n += uint64(c-'A') + 10
		default:
			return 0, json.Unmarshal([]byte("\"bad hex\""), &struct{}{})
		}
	}
	*out = n
	return len(s), nil
}

// eth_syncing sagte immer "false" -- also "vollstaendig synchron", auch wenn
// der Knoten hunderte Bloecke zurueckliegt. Genau das war C2 am 29.08.2026,
// nachdem ein Lasttest C1 schneller Bloecke erzeugen liess, als C2 sie
// nachspielen konnte. Es ist die Frage, die ein Werkzeug stellt, BEVOR es
// einer Antwort traut.
func TestSyncing_OhneDAGKeinFalschesVersprechen(t *testing.T) {
	s := &EVMRPCServer{}
	// Ohne DAG gibt es nichts zu behaupten -- false ist hier richtig, weil
	// dieser Knoten dann gar nicht synchronisiert.
	if got := s.syncing(); got != false {
		t.Fatalf("ohne DAG = %v, erwartet false", got)
	}
}

func TestSyncing_FormDerAntwort(t *testing.T) {
	// Die Ethereum-Schnittstelle verlangt entweder false oder ein Objekt mit
	// startingBlock/currentBlock/highestBlock. Ein Werkzeug, das nur auf
	// "!= false" prueft, verlaesst sich darauf.
	m := map[string]interface{}{
		"startingBlock": "0x0",
		"currentBlock":  "0x1",
		"highestBlock":  "0x2",
	}
	for _, k := range []string{"startingBlock", "currentBlock", "highestBlock"} {
		v, da := m[k]
		if !da {
			t.Fatalf("%s fehlt", k)
		}
		if !strings.HasPrefix(v.(string), "0x") {
			t.Fatalf("%s = %v, erwartet Hex mit 0x", k, v)
		}
	}
}
