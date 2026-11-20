package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	dir := t.TempDir()
	args := []string{"--n", "8", "--name", v.Name, "--verzeichnis", dir}
	if rc, aus, log := lauf(args, 10001, v.Schluessel, v.Chiffrat); rc != 0 || aus != v.Name+".json\n" {
		t.Fatalf("Gutfall: %d %q %q", rc, aus, log)
	}
	if roh, err := os.ReadFile(filepath.Join(dir, v.Name+".json")); err != nil || !strings.Contains(string(roh), `"y":"10011100"`) {
		t.Fatalf("nicht abgelegt: %v", err)
	}
	os.Remove(filepath.Join(dir, v.Name+".json"))
	if rc, _, _ := lauf([]string{"--n", "8", "--name", v.Name}, 10001, v.Schluessel, v.Chiffrat); rc != 2 {
		t.Fatal("ohne Verzeichnis gestartet")
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
	if rc, aus, _ := lauf([]string{"--n", "8", "--name", strings.Repeat("0", 64), "--verzeichnis", dir}, 10001, v.Schluessel, v.Chiffrat); rc != 1 || aus != "" {
		t.Fatal("andere Basen angenommen")
	}
	if rc, _, _ := lauf([]string{"--n", "8", "--name", v.Name, "--verzeichnis", dir, "ueberzaehlig"}, 10001, v.Schluessel, v.Chiffrat); rc != 2 {
		t.Fatal("ueberzaehliges Argument angenommen")
	}
}

func TestQuantenSNachweisBefehl(t *testing.T) {
	lauf := func(args ...string) (int, string) {
		var aus, log bytes.Buffer
		rc := quantenSNachweisMit(context.Background(), args, &aus, &log)
		return rc, log.String()
	}
	if rc, _ := lauf("--k", strings.Repeat("a", 64)); rc != 2 {
		t.Fatal("ohne Validatoren gestartet")
	}
	if rc, log := lauf("--k", "zz", "--validatoren", "0x"+strings.Repeat("11", 20)+"=http://127.0.0.1:1"); rc != 1 || !strings.Contains(log, "K") {
		t.Fatalf("K ohne Form: %d %s", rc, log)
	}
	if rc, _ := lauf("--k", strings.Repeat("a", 64), "--validatoren", "0x"+strings.Repeat("11", 20)+"=http://127.0.0.1:1", "ueberzaehlig"); rc != 2 {
		t.Fatal("ueberzaehliges Argument angenommen")
	}
}
