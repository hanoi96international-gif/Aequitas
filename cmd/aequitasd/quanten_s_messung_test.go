package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestQuantenSMessungOeffnenBefehl(t *testing.T) {
	roh, err := os.ReadFile("../../x/humanity/keeper/testdata/quanten_s_versiegelt_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Chiffrat   string `json:"chiffrat"`
		Name       string `json:"name"`
		Schluessel string `json:"validator_schluessel_nur_test"`
	}
	if err := json.Unmarshal(roh, &v); err != nil {
		t.Fatal(err)
	}
	lauf := func(args []string, uid int, key, ein string) (int, string, string) {
		var aus, log bytes.Buffer
		rc := quantenSMessungOeffnenMit(args, uid, key, strings.NewReader(ein), &aus, &log)
		return rc, aus.String(), log.String()
	}
	args := []string{"--n", "8", "--name", v.Name}
	if rc, aus, log := lauf(args, 10001, v.Schluessel, v.Chiffrat); rc != 0 || !strings.Contains(aus, `"y":"10011100"`) {
		t.Fatalf("Gutfall: %d %q %q", rc, aus, log)
	}
	if rc, aus, _ := lauf(args, 0, v.Schluessel, v.Chiffrat); rc != 2 || aus != "" {
		t.Fatal("als root gestartet")
	}
	if rc, aus, _ := lauf(args, 10001, "", v.Chiffrat); rc != 2 || aus != "" {
		t.Fatal("ohne Schluessel gestartet")
	}
	if rc, aus, log := lauf(args, 10001, strings.Repeat("72", 32), v.Chiffrat); rc != 1 || aus != "" || strings.Contains(log, strings.Repeat("72", 32)) {
		t.Fatalf("fremder Schluessel: %d %q %q", rc, aus, log)
	}
	if rc, aus, _ := lauf([]string{"--n", "8", "--name", strings.Repeat("0", 64)}, 10001, v.Schluessel, v.Chiffrat); rc != 1 || aus != "" {
		t.Fatal("andere Basen angenommen")
	}
	if rc, _, _ := lauf([]string{"--n", "8", "--name", v.Name, "ueberzaehlig"}, 10001, v.Schluessel, v.Chiffrat); rc != 2 {
		t.Fatal("ueberzaehliges Argument angenommen")
	}
}
