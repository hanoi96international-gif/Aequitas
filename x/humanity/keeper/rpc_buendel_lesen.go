package keeper

import (
	"encoding/json"
	"strconv"
)

// Ueberweisungsbuendel ohne Reflexion lesen.
//
// Pruefstand lokal (02.10.2026, BenchmarkAnnahmeBuendelDB): das Lesen eines
// Buendels kostete 3,1 % der Knoten-CPU -- einmal das ganze Buendel in
// []json.RawMessage (mit Gueltigkeitspruefung vorab, also zwei Durchlaeufe),
// dann je Posten noch einmal Umschlag und Parameter, jeweils per Reflexion.
//
// buendelSchnellLesen liest NUR die Form, die Lastgeneratoren und Wallets
// fuer Ueberweisungsbuendel schicken:
//
//	[{"jsonrpc":"2.0","id":1,"method":"eth_sendRawTransaction","params":["0x..."]}, ...]
//
// Schluessel in beliebiger Reihenfolge, JSON-Leerraum erlaubt. Alles andere
// -- andere Methoden, Escapes oder Nicht-ASCII in Zeichenketten, Zahlen mit
// Komma oder Exponent, unbekannte, doppelte oder anders geschriebene
// Schluessel, mehr als ein Parameter, mehr als rpcMaxBuendel Posten -- gibt
// ok=false, und handleRPC liest das Buendel wie bisher mit encoding/json.
// Was dieser Weg liest, ist darum genau das, was encoding/json liest
// (FuzzBuendelSchnellLesen prueft das); was er nicht sicher gleich liest,
// faellt zurueck. Keine Pruefung entfaellt: Signatur, Nonce, Deckung und
// Ratenbegrenzung folgen unveraendert in handleRPC und sendRawTransaction.

// buendelPosten: was der Vorab-Durchlauf aus einem Buendelposten liest.
type buendelPosten struct {
	id     interface{}
	method string
	params []json.RawMessage
	rawHex string
	ok     bool // eth_sendRawTransaction mit lesbarem rawHex
}

// postenLesen: der bisherige Vorab-Durchlauf je Posten, mit encoding/json.
func postenLesen(raw json.RawMessage) (p buendelPosten) {
	var env struct {
		ID     interface{}       `json:"id"`
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Method != "eth_sendRawTransaction" || len(env.Params) == 0 {
		return buendelPosten{}
	}
	if err := json.Unmarshal(env.Params[0], &p.rawHex); err != nil {
		return buendelPosten{}
	}
	p.id, p.method, p.params, p.ok = env.ID, env.Method, env.Params, true
	return p
}

// buendelSchnellLesen: siehe oben. ok=false heisst nur "nicht diese Form",
// nie "ungueltig" -- die Entscheidung darueber trifft encoding/json.
func buendelSchnellLesen(body []byte) (batch []json.RawMessage, posten []buendelPosten, ok bool) {
	l := buendelLeser{b: body}
	l.leer()
	if !l.zeichen('[') {
		return nil, nil, false
	}
	l.leer()
	if l.zeichen(']') {
		l.leer()
		return []json.RawMessage{}, []buendelPosten{}, l.i == len(l.b)
	}
	for {
		if len(batch) >= rpcMaxBuendel {
			// Zu gross: encoding/json liest es und handleRPC lehnt es ab,
			// mit derselben Meldung wie bisher.
			return nil, nil, false
		}
		start := l.i
		p, gut := l.posten()
		if !gut {
			return nil, nil, false
		}
		batch = append(batch, json.RawMessage(l.b[start:l.i]))
		posten = append(posten, p)
		l.leer()
		if l.zeichen(',') {
			l.leer()
			continue
		}
		if !l.zeichen(']') {
			return nil, nil, false
		}
		l.leer()
		if l.i != len(l.b) {
			return nil, nil, false
		}
		return batch, posten, true
	}
}

type buendelLeser struct {
	b []byte
	i int
}

func (l *buendelLeser) leer() {
	for l.i < len(l.b) {
		switch l.b[l.i] {
		case ' ', '\t', '\n', '\r':
			l.i++
		default:
			return
		}
	}
}

func (l *buendelLeser) zeichen(c byte) bool {
	if l.i < len(l.b) && l.b[l.i] == c {
		l.i++
		return true
	}
	return false
}

// text: eine Zeichenkette aus druckbarem ASCII ohne Escape. Gibt den Inhalt
// ohne Anfuehrungszeichen zurueck.
func (l *buendelLeser) text() (string, bool) {
	if !l.zeichen('"') {
		return "", false
	}
	start := l.i
	for l.i < len(l.b) {
		c := l.b[l.i]
		if c == '"' {
			s := string(l.b[start:l.i])
			l.i++
			return s, true
		}
		if c < 0x20 || c > 0x7e || c == '\\' {
			return "", false
		}
		l.i++
	}
	return "", false
}

// id: null, eine Zeichenkette (wie text) oder eine ganze Zahl mit hoechstens
// 15 Ziffern -- so gross, dass float64 sie exakt traegt. Gelesen wie
// encoding/json es in ein interface{} liest (float64 ueber ParseFloat).
func (l *buendelLeser) id() (interface{}, bool) {
	if l.i >= len(l.b) {
		return nil, false
	}
	switch c := l.b[l.i]; {
	case c == 'n':
		if l.i+4 <= len(l.b) && string(l.b[l.i:l.i+4]) == "null" {
			l.i += 4
			return nil, true
		}
		return nil, false
	case c == '"':
		s, ok := l.text()
		if !ok {
			return nil, false
		}
		return s, true
	case c == '-' || (c >= '0' && c <= '9'):
		start := l.i
		if c == '-' {
			l.i++
		}
		ziffern := l.i
		for l.i < len(l.b) && l.b[l.i] >= '0' && l.b[l.i] <= '9' {
			l.i++
		}
		n := l.i - ziffern
		if n == 0 || n > 15 || (n > 1 && l.b[ziffern] == '0') {
			return nil, false
		}
		if l.i < len(l.b) {
			// Komma oder Exponent: nicht diese Form.
			switch l.b[l.i] {
			case '.', 'e', 'E':
				return nil, false
			}
		}
		f, err := strconv.ParseFloat(string(l.b[start:l.i]), 64)
		if err != nil {
			return nil, false
		}
		return f, true
	}
	return nil, false
}

// posten: ein Objekt mit hoechstens den Schluesseln jsonrpc, id, method und
// params, jeder hoechstens einmal; method muss eth_sendRawTransaction sein,
// params genau eine Zeichenkette.
func (l *buendelLeser) posten() (buendelPosten, bool) {
	var p buendelPosten
	if !l.zeichen('{') {
		return p, false
	}
	var hatJSONRPC, hatID, hatMethod, hatParams bool
	l.leer()
	if l.zeichen('}') {
		return p, false
	}
	for {
		l.leer()
		schluessel, ok := l.text()
		if !ok {
			return p, false
		}
		l.leer()
		if !l.zeichen(':') {
			return p, false
		}
		l.leer()
		switch schluessel {
		case "jsonrpc":
			if hatJSONRPC {
				return p, false
			}
			hatJSONRPC = true
			if _, ok := l.text(); !ok {
				return p, false
			}
		case "id":
			if hatID {
				return p, false
			}
			hatID = true
			if p.id, ok = l.id(); !ok {
				return p, false
			}
		case "method":
			if hatMethod {
				return p, false
			}
			hatMethod = true
			if p.method, ok = l.text(); !ok || p.method != "eth_sendRawTransaction" {
				return p, false
			}
		case "params":
			if hatParams {
				return p, false
			}
			hatParams = true
			if !l.zeichen('[') {
				return p, false
			}
			l.leer()
			start := l.i
			if p.rawHex, ok = l.text(); !ok {
				return p, false
			}
			p.params = []json.RawMessage{json.RawMessage(l.b[start:l.i])}
			l.leer()
			if !l.zeichen(']') {
				return p, false
			}
		default:
			return p, false
		}
		l.leer()
		if l.zeichen(',') {
			continue
		}
		if !l.zeichen('}') {
			return p, false
		}
		break
	}
	if !hatMethod || !hatParams {
		return p, false
	}
	p.ok = true
	return p, true
}
