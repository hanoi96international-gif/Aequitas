package keeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// Was buendelSchnellLesen liest, muss genau das sein, was encoding/json
// liest: dieselben Posten, derselbe Rohtext je Posten, derselbe Umschlag.
// Liest encoding/json das Buendel nicht, darf der schnelle Weg es auch nicht.
func buendelGleichPruefen(t *testing.T, body []byte) bool {
	t.Helper()
	batch, posten, ok := buendelSchnellLesen(body)
	if !ok {
		return false
	}
	var alt []json.RawMessage
	if err := json.Unmarshal(body, &alt); err != nil {
		t.Fatalf("schnell gelesen, encoding/json lehnt ab (%v): %q", err, body)
	}
	if len(alt) != len(batch) || len(posten) != len(batch) {
		t.Fatalf("Postenzahl: encoding/json %d, schnell %d/%d: %q", len(alt), len(batch), len(posten), body)
	}
	for i := range alt {
		if !bytes.Equal(alt[i], batch[i]) {
			t.Fatalf("Posten %d Rohtext: encoding/json %q, schnell %q", i, alt[i], batch[i])
		}
		want := postenLesen(alt[i])
		if !want.ok {
			t.Fatalf("Posten %d: schnell gelesen, bisheriger Weg nicht: %q", i, alt[i])
		}
		if !reflect.DeepEqual(want, posten[i]) {
			t.Fatalf("Posten %d: bisher %#v, schnell %#v", i, want, posten[i])
		}
		// DeepEqual haelt -0 und 0 fuer gleich; die Antwort nicht.
		if a, ok := want.id.(float64); ok && math.Signbit(a) != math.Signbit(posten[i].id.(float64)) {
			t.Fatalf("Posten %d: Vorzeichen der id verschieden: %q", i, alt[i])
		}
	}
	return true
}

const testRohHex = `0xf86b80843b9aca00825208940000000000000000000000000000000000000be87038d7ea4c680008082f0f0a0`

func testBuendel(posten ...string) []byte {
	return []byte("[" + strings.Join(posten, ",") + "]")
}

// Gutfall: die Formen, die Pruefstand und Lastgenerator schicken, werden
// schnell gelesen -- sonst braechte der neue Weg nichts.
func TestBuendelSchnellLesen_UeblicheFormen(t *testing.T) {
	for _, body := range [][]byte{
		// BenchmarkAnnahmeBuendel / Pruefstand
		testBuendel(
			`{"jsonrpc":"2.0","id":0,"method":"eth_sendRawTransaction","params":["`+testRohHex+`"]}`,
			`{"jsonrpc":"2.0","id":1,"method":"eth_sendRawTransaction","params":["`+testRohHex+`"]}`),
		// tools/contabo-loadtest (andere Schluesselreihenfolge)
		testBuendel(`{"jsonrpc":"2.0","method":"eth_sendRawTransaction","params":["` + testRohHex + `"],"id":42}`),
		// Leerraum, Zeichenketten-id, null-id, negative id, ohne jsonrpc/id
		[]byte(" [ {\n\t\"id\" : \"a-1\" , \"method\":\"eth_sendRawTransaction\",\"params\":[ \"0x00\" ] } ,\r\n" +
			`{"id":null,"method":"eth_sendRawTransaction","params":["0x"]},` +
			`{"id":-7,"method":"eth_sendRawTransaction","params":["0x"]},` +
			`{"id":-0,"method":"eth_sendRawTransaction","params":["0x"]},` +
			`{"method":"eth_sendRawTransaction","params":[""]} ] `),
		[]byte(`[]`),
	} {
		if !buendelGleichPruefen(t, body) {
			t.Fatalf("nicht schnell gelesen: %q", body)
		}
	}
}

// Missbrauch und Randfaelle: alles, was der schnelle Weg nicht sicher gleich
// liest, faellt an encoding/json zurueck -- ein Angreifer kann ihn nicht
// dazu bringen, etwas anders zu lesen als bisher.
func TestBuendelSchnellLesen_FaelltZurueck(t *testing.T) {
	gut := `{"jsonrpc":"2.0","id":1,"method":"eth_sendRawTransaction","params":["0x01"]}`
	for name, body := range map[string]string{
		"andere Methode":          `[{"id":1,"method":"eth_getTransactionCount","params":["0x01"]}]`,
		"gemischtes Buendel":      "[" + gut + `,{"id":2,"method":"eth_chainId","params":[]}]`,
		"Gross geschriebener Key": `[{"id":1,"Method":"eth_sendRawTransaction","params":["0x01"]}]`,
		"doppelte Methode":        `[{"id":1,"method":"eth_chainId","method":"eth_sendRawTransaction","params":["0x01"]}]`,
		"doppelte params":         `[{"id":1,"method":"eth_sendRawTransaction","params":["0x01"],"params":["0x02"]}]`,
		"doppelte id":             `[{"id":1,"id":2,"method":"eth_sendRawTransaction","params":["0x01"]}]`,
		"unbekannter Key":         `[{"id":1,"method":"eth_sendRawTransaction","params":["0x01"],"x":1}]`,
		"Escape im Rohtext":       `[{"id":1,"method":"eth_sendRawTransaction","params":["0x` + "\\" + `u0030"]}]`,
		"Escape in der Methode":   `[{"id":1,"method":"eth_sendRawTransaction` + "\\" + `u0000","params":["0x01"]}]`,
		"Nicht-ASCII":             "[{\"id\":\"\xc3\xa4\",\"method\":\"eth_sendRawTransaction\",\"params\":[\"0x01\"]}]",
		"ungueltiges UTF-8":       "[{\"id\":\"\xff\",\"method\":\"eth_sendRawTransaction\",\"params\":[\"0x01\"]}]",
		"Kommazahl":               `[{"id":1.0,"method":"eth_sendRawTransaction","params":["0x01"]}]`,
		"Exponent":                `[{"id":1e3,"method":"eth_sendRawTransaction","params":["0x01"]}]`,
		"riesige Zahl":            `[{"id":12345678901234567890,"method":"eth_sendRawTransaction","params":["0x01"]}]`,
		"fuehrende Null":          `[{"id":01,"method":"eth_sendRawTransaction","params":["0x01"]}]`,
		"zwei Parameter":          `[{"id":1,"method":"eth_sendRawTransaction","params":["0x01","0x02"]}]`,
		"leere Parameter":         `[{"id":1,"method":"eth_sendRawTransaction","params":[]}]`,
		"Parameter keine Kette":   `[{"id":1,"method":"eth_sendRawTransaction","params":[1]}]`,
		"ohne params":             `[{"id":1,"method":"eth_sendRawTransaction"}]`,
		"ohne Methode":            `[{"id":1,"params":["0x01"]}]`,
		"verschachtelte id":       `[{"id":{"a":1},"method":"eth_sendRawTransaction","params":["0x01"]}]`,
		"Muell danach":            "[" + gut + "]x",
		"zweites Buendel danach":  "[" + gut + "][]",
		"fehlende Klammer":        "[" + gut,
		"Komma am Ende":           "[" + gut + ",]",
		"leeres Objekt":           `[{}]`,
		"kein Objekt":             `[1]`,
		"Steuerzeichen in Kette":  "[{\"id\":\"a\tb\",\"method\":\"eth_sendRawTransaction\",\"params\":[\"0x01\"]}]",
		"abgeschnittenes null":    `[{"id":nul`,
		"kein Doppelpunkt":        `[{"id" 1,"method":"eth_sendRawTransaction","params":["0x01"]}]`,
		"Klammer statt Kette":     `[{"id":1,"method":"eth_sendRawTransaction","params":"0x01"}]`,
		"vertikaler Tab":          "[" + gut + "\v]",
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, ok := buendelSchnellLesen([]byte(body)); ok {
				t.Fatalf("schnell gelesen, haette zurueckfallen muessen: %q", body)
			}
		})
	}
}

// Zu grosse Buendel: der schnelle Weg hoert beim 101. Posten auf, und
// handleRPC lehnt das Buendel ab wie bisher -- die Grenze gilt unveraendert.
func TestBuendelSchnellLesen_GrenzeBleibt(t *testing.T) {
	posten := make([]string, rpcMaxBuendel+1)
	for i := range posten {
		posten[i] = fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"eth_sendRawTransaction","params":["0x01"]}`, i)
	}
	if _, _, ok := buendelSchnellLesen(testBuendel(posten[:rpcMaxBuendel]...)); !ok {
		t.Fatal("100 Posten sollten schnell gelesen werden")
	}
	zuGross := testBuendel(posten...)
	if _, _, ok := buendelSchnellLesen(zuGross); ok {
		t.Fatal("101 Posten schnell gelesen")
	}
	srv := &EVMRPCServer{}
	req := httptest.NewRequest("POST", "/rpc", bytes.NewReader(zuGross))
	w := httptest.NewRecorder()
	srv.handleRPC(w, req)
	if want := fmt.Sprintf("batch too large: max %d requests, got %d", rpcMaxBuendel, rpcMaxBuendel+1); !strings.Contains(w.Body.String(), want) {
		t.Fatalf("erwartet %q, bekam %s", want, w.Body.String())
	}
}

// Zufaellig veraenderte Buendel: was schnell gelesen wird, muss gleich
// gelesen werden. Laeuft in jedem CI-Durchlauf (der Fuzz-Test unten nur mit
// -fuzz); mit festem Startwert, damit ein Fehlschlag nachvollziehbar ist.
func TestBuendelSchnellLesen_ZufallGleich(t *testing.T) {
	rng := rand.New(rand.NewSource(20261002))
	vorlagen := []string{
		`{"jsonrpc":"2.0","id":7,"method":"eth_sendRawTransaction","params":["0xab"]}`,
		`{"method":"eth_sendRawTransaction","params":["0x"],"id":"x","jsonrpc":"2.0"}`,
		` { "id" : null , "params" : [ "0x1" ] , "method" : "eth_sendRawTransaction" } `,
	}
	zeichen := []byte(`[]{},:"\ 0123456789-.eE+nulltruefalsexX` + "\t\n\r\x00\xff")
	schnell := 0
	for n := 0; n < 20000; n++ {
		k := 1 + rng.Intn(4)
		teile := make([]string, k)
		for i := range teile {
			teile[i] = vorlagen[rng.Intn(len(vorlagen))]
		}
		body := testBuendel(teile...)
		for m := rng.Intn(3); m > 0; m-- {
			pos := rng.Intn(len(body))
			switch rng.Intn(3) {
			case 0: // ersetzen
				body[pos] = zeichen[rng.Intn(len(zeichen))]
			case 1: // loeschen
				body = append(body[:pos], body[pos+1:]...)
			default: // einfuegen
				body = append(body[:pos], append([]byte{zeichen[rng.Intn(len(zeichen))]}, body[pos:]...)...)
			}
		}
		if buendelGleichPruefen(t, body) {
			schnell++
		}
	}
	// Ohne Gutfaelle waere der Vergleich leer.
	if schnell < 1000 {
		t.Fatalf("nur %d von 20000 schnell gelesen", schnell)
	}
}

func FuzzBuendelSchnellLesen(f *testing.F) {
	f.Add([]byte(`[{"jsonrpc":"2.0","id":0,"method":"eth_sendRawTransaction","params":["` + testRohHex + `"]}]`))
	f.Add([]byte(`[{"method":"eth_sendRawTransaction","params":["0x"],"id":"a"},{"id":null,"method":"eth_sendRawTransaction","params":[""]}]`))
	f.Add([]byte(` [ { "id" : -0 , "method" : "eth_sendRawTransaction" , "params" : [ "0x1" ] } ] `))
	f.Add([]byte(`[]`))
	f.Fuzz(func(t *testing.T, body []byte) {
		buendelGleichPruefen(t, body)
	})
}
