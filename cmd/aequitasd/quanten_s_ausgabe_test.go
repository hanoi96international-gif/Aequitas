package main

import (
	"bytes"
	"strings"
	"testing"
)

// `aequitasd quanten-s-ausgabe`: nicht als root, nicht ohne Schluessel,
// keine fremden Argumente, und der Schluessel steht in keiner Ausgabe.
func TestQuantenSAusgabeBefehl_Sperren(t *testing.T) {
	geheim := strings.Repeat("ab", 32)
	fall := func(name string, args []string, uid int, key string, soll int) {
		t.Helper()
		var aus, log bytes.Buffer
		if got := quantenSAusgabeMit(args, uid, key, strings.NewReader("{}"), &aus, &log); got != soll {
			t.Fatalf("%s: Exit %d, erwartet %d (%s)", name, got, soll, log.String())
		}
		if strings.Contains(aus.String()+log.String(), geheim) {
			t.Fatalf("%s: Schluessel in der Ausgabe", name)
		}
	}
	fall("root", nil, 0, geheim, 2)
	fall("ohne Schluessel", nil, 10001, "", 2)
	fall("fremdes Argument", []string{"weg"}, 10001, geheim, 2)
	fall("unbekannte Option", []string{"--schluessel", "x"}, 10001, geheim, 2)
	fall("API mit Benutzerangabe", []string{"--api", "http://x@127.0.0.1:8080"}, 10001, geheim, 1)
	fall("API nach aussen", []string{"--api", "http://10.0.0.1:8080"}, 10001, geheim, 1)
	fall("Frist zu lang", []string{"--frist", "2h"}, 10001, geheim, 1)
	fall("leere Messung", nil, 10001, geheim, 1) // {} hat n = 0
}
