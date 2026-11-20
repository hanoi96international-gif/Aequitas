package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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

// `aequitasd quanten-s-inhaberin` und `quanten-s-annahme`: nur bekannte
// Unterbefehle und Optionen, Pflichtangaben, Annahme nicht als root und nie
// mit einem Schluessel von der Befehlszeile; kein Schluessel in der Ausgabe.
func TestQuantenSRollenBefehle_Sperren(t *testing.T) {
	ctx := context.Background()
	inh := func(name string, args []string, soll int) {
		t.Helper()
		var aus, log bytes.Buffer
		if got := quantenSInhaberinMit(ctx, args, strings.NewReader("{}"), &aus, &log); got != soll {
			t.Fatalf("%s: Exit %d, erwartet %d (%s)", name, got, soll, log.String())
		}
	}
	datei := filepath.Join(t.TempDir(), "inhaberin.json")
	inh("ohne Unterbefehl", nil, 2)
	inh("unbekannter Unterbefehl", []string{"weg"}, 2)
	inh("ohne Datei", []string{"neu", "--n", "256"}, 2)
	inh("Option des anderen Unterbefehls", []string{"neu", "--datei", datei, "--stelle", "0"}, 2)
	inh("fremdes Argument", []string{"einloesen", "--datei", datei, "x"}, 2)
	inh("neu ohne Stellen", []string{"neu", "--datei", datei, "--n", "256"}, 1)
	inh("annehmen ohne Validatoren", []string{"annehmen", "--datei", datei, "--api", "http://127.0.0.1:1"}, 1)
	inh("annehmen ohne Datei auf der Platte", []string{"annehmen", "--datei", datei, "--api", "http://127.0.0.1:1", "--validatoren", "0x" + strings.Repeat("a", 40)}, 1)
	inh("einloesen API nach aussen", []string{"einloesen", "--datei", datei, "--api", "http://10.0.0.1:1", "--stelle", "0"}, 1)
	if _, err := os.Stat(datei); err == nil {
		t.Fatal("Datei angelegt, obwohl nichts gelang")
	}

	geheim := strings.Repeat("cd", 32)
	ann := func(name string, args []string, uid int, umgebung map[string]string, soll int) {
		t.Helper()
		var aus, log bytes.Buffer
		got := quantenSAnnahmeMit(ctx, args, uid, func(k string) string { return umgebung[k] }, strings.NewReader("{}"), &aus, &log)
		if got != soll {
			t.Fatalf("%s: Exit %d, erwartet %d (%s)", name, got, soll, log.String())
		}
		if strings.Contains(aus.String()+log.String(), geheim) {
			t.Fatalf("%s: Schluessel in der Ausgabe", name)
		}
	}
	mit := map[string]string{"RELAYER_PRIVATE_KEY": geheim}
	ann("root", nil, 0, mit, 2)
	ann("ohne Schluessel", nil, 10001, nil, 2)
	ann("leere Variable", []string{"--schluessel-variable", ""}, 10001, mit, 2)
	ann("Schluessel auf der Befehlszeile", []string{"--schluessel", geheim}, 10001, mit, 2)
	ann("fremdes Argument", []string{"weg"}, 10001, mit, 2)
	ann("Validatoren kaputt", []string{"--validatoren", "nichts"}, 10001, mit, 2)
	ann("ohne Validatoren", nil, 10001, mit, 1)
	ann("API mit Benutzerangabe", []string{"--api", "http://x@127.0.0.1:1", "--validatoren", "0x" + strings.Repeat("a", 40) + "=http://127.0.0.1:1"}, 10001, mit, 1)
	ann("andere Variable", []string{"--schluessel-variable", "STELLE", "--validatoren", "0x" + strings.Repeat("a", 40) + "=http://127.0.0.1:1", "--frist", "2h"},
		10001, map[string]string{"STELLE": geheim}, 1)
}
