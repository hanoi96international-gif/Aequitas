package keeper

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
)

// JSON-RPC-Antworten als feste Typen statt map[string]interface{}.
//
// Pruefstand Lauf 18 (02.10.2026): Die Annahme ist die Grenze (Rueckstau 0,
// Inflight voll). Vom CPU-Rest neben der Signatur gingen 6,8 % ins Kodieren
// der Antworten -- 3,7 % davon in encoding/json.mapEncoder, der fuer jede
// Map die Schluessel per Reflexion einsammelt und SORTIERT. Und 8 % aller
// Allokationen waren errorResponse: 2,9 Millionen abgewiesene Posten, jeder
// mit zwei frisch gebauten Maps.
//
// Inhalt unveraendert: dieselben Felder, dieselben Werte, auf Erfolg
// "result" (auch null), auf Fehler "error" ohne "result" -- wie JSON-RPC 2.0
// es verlangt. Nur die Reihenfolge der Schluessel folgt jetzt dem Typ statt
// dem Alphabet; JSON-Objekte sind ungeordnet.

type rpcFehlerObjekt struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcErfolg struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result"`
}

type rpcFehler struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Error   rpcFehlerObjekt `json:"error"`
}

func errorResponse(id interface{}, code int, message string) rpcFehler {
	return rpcFehler{JSONRPC: "2.0", ID: id, Error: rpcFehlerObjekt{Code: code, Message: message}}
}

func erfolgResponse(id interface{}, result interface{}) rpcErfolg {
	return rpcErfolg{JSONRPC: "2.0", ID: id, Result: result}
}

func writeError(w http.ResponseWriter, code int, message string, id interface{}) {
	json.NewEncoder(w).Encode(errorResponse(id, code, message))
}

// besetztPosten: die Antwort fuer einen abgewiesenen Posten (id null), einmal
// kodiert. Abgewiesen wird ohne zu dekodieren -- die Anfrage-ID ist nicht
// bekannt, darum null, wie bisher.
var (
	besetztPostenBytes map[string][]byte
	besetztPostenMu    sync.Mutex
)

func besetztPosten(text string) []byte {
	besetztPostenMu.Lock()
	defer besetztPostenMu.Unlock()
	if besetztPostenBytes == nil {
		besetztPostenBytes = map[string][]byte{}
	}
	if b, ok := besetztPostenBytes[text]; ok {
		return b
	}
	b, _ := json.Marshal(errorResponse(nil, -32005, text))
	// Hoechstens eine Handvoll fester Texte -- begrenzt, keine Eingabe von aussen.
	if len(besetztPostenBytes) < 16 {
		besetztPostenBytes[text] = b
	}
	return b
}

// schreibeBesetztBuendel: n gleiche Abweisungen als JSON-Array, wie
// json.NewEncoder(w).Encode(results) es geschrieben haette (mit
// abschliessendem Zeilenumbruch), ohne n Objekte zu bauen.
func schreibeBesetztBuendel(w http.ResponseWriter, n int, text string) {
	posten := besetztPosten(text)
	var buf bytes.Buffer
	buf.Grow(2 + n*(len(posten)+1))
	buf.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(posten)
	}
	buf.WriteString("]\n")
	w.Write(buf.Bytes())
}
