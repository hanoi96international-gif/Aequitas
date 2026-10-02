package keeper

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Ein Buendelposten wird im Vorab-Durchlauf einmal gelesen; handleSingle
// darf mit dem Vorab-Ergebnis nichts anderes antworten als beim eigenen
// Lesen desselben Texts (Pruefstand 01.10.2026: JSON 13 % der CPU).
func TestHandleSingle_VorabGeparstGleicheAntwort(t *testing.T) {
	s := &EVMRPCServer{}
	for _, body := range []string{
		`{"jsonrpc":"2.0","id":7,"method":"eth_chainId","params":[]}`,
		`{"jsonrpc":"2.0","id":"abc","method":"net_version","params":[]}`,
		`{"jsonrpc":"2.0","id":null,"method":"eth_chainId","params":[]}`,
	} {
		ohne := s.handleSingle([]byte(body), nil)

		var env struct {
			ID     interface{}       `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal([]byte(body), &env); err != nil {
			t.Fatal(err)
		}
		pre := &precomputedSendTx{geparst: true, id: env.ID, method: env.Method, params: env.Params}
		mit := s.handleSingle([]byte(body), pre)
		if !reflect.DeepEqual(ohne, mit) {
			t.Fatalf("%s: ohne Vorab %v, mit Vorab %v", body, ohne, mit)
		}
	}
}

// Ohne Vorab-Ergebnis bleibt ein kaputter Posten ein Parse-Fehler.
func TestHandleSingle_OhneVorabParseFehler(t *testing.T) {
	s := &EVMRPCServer{}
	r := s.handleSingle([]byte(`{kaputt`), nil)
	// Auf dem Draht pruefen, so wie ein Client es liest.
	roh, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(roh, &m); err != nil {
		t.Fatal(err)
	}
	e, _ := m["error"].(map[string]interface{})
	if e == nil || e["code"] != float64(-32700) {
		t.Fatalf("erwartet -32700, bekam %s", roh)
	}
	if _, hat := m["result"]; hat {
		t.Fatalf("Fehlerantwort darf kein result tragen: %s", roh)
	}
}
