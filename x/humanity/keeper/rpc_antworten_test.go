package keeper

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"
)

// Die typisierten Antworten tragen auf dem Draht genau den Inhalt der alten
// Maps -- als JSON-Objekt verglichen, also unabhaengig von der Reihenfolge.
func TestRPCAntworten_GleicherInhaltWieMaps(t *testing.T) {
	type fall struct {
		neu interface{}
		alt map[string]interface{}
	}
	faelle := []fall{
		{erfolgResponse(7, "0x786"), map[string]interface{}{"jsonrpc": "2.0", "id": 7, "result": "0x786"}},
		{erfolgResponse("abc", nil), map[string]interface{}{"jsonrpc": "2.0", "id": "abc", "result": nil}},
		{erfolgResponse(nil, map[string]interface{}{"a": 1}), map[string]interface{}{"jsonrpc": "2.0", "id": nil, "result": map[string]interface{}{"a": 1}}},
		{errorResponse(3, -32602, "invalid params"), map[string]interface{}{"jsonrpc": "2.0", "id": 3,
			"error": map[string]interface{}{"code": -32602, "message": "invalid params"}}},
		{errorResponse(nil, -32005, "server busy"), map[string]interface{}{"jsonrpc": "2.0", "id": nil,
			"error": map[string]interface{}{"code": -32005, "message": "server busy"}}},
	}
	for _, f := range faelle {
		a, _ := json.Marshal(f.neu)
		b, _ := json.Marshal(f.alt)
		var ma, mb interface{}
		json.Unmarshal(a, &ma)
		json.Unmarshal(b, &mb)
		if !reflect.DeepEqual(ma, mb) {
			t.Fatalf("neu %s, alt %s", a, b)
		}
	}
}

// Das vorberechnete Buendel ist Byte fuer Byte das, was der Encoder aus n
// einzelnen Antworten geschrieben haette.
func TestSchreibeBesetztBuendel_WieEncoder(t *testing.T) {
	const text = "server busy: too much work in flight, try again shortly"
	for _, n := range []int{1, 2, 20, 100} {
		w := httptest.NewRecorder()
		schreibeBesetztBuendel(w, n, text)

		want := httptest.NewRecorder()
		alt := make([]interface{}, 0, n)
		for i := 0; i < n; i++ {
			alt = append(alt, map[string]interface{}{"jsonrpc": "2.0", "id": nil,
				"error": map[string]interface{}{"code": -32005, "message": text}})
		}
		json.NewEncoder(want).Encode(alt)

		var a, b interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &a); err != nil {
			t.Fatalf("n=%d: kein gueltiges JSON: %v", n, err)
		}
		json.Unmarshal(want.Body.Bytes(), &b)
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("n=%d: %s statt %s", n, w.Body.String(), want.Body.String())
		}
		if got := w.Body.Bytes(); got[len(got)-1] != '\n' {
			t.Fatalf("n=%d: Zeilenende fehlt", n)
		}
	}
}

// Die Liste der vorberechneten Texte waechst nicht ueber ihre Grenze.
func TestBesetztPosten_Begrenzt(t *testing.T) {
	for i := 0; i < 100; i++ {
		besetztPosten(string(rune('a'+i%26)) + string(rune('A'+i/26)))
	}
	besetztPostenMu.Lock()
	n := len(besetztPostenBytes)
	besetztPostenMu.Unlock()
	if n > 16 {
		t.Fatalf("%d Eintraege, Grenze 16", n)
	}
}
