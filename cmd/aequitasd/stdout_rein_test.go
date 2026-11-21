package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Werkzeug-Befehle (quanten-s-ausgabe, quanten-s-annahme, quanten-s-nachweis,
// ...) schreiben ihr Ergebnis als JSON auf stdout; die Workflows lesen es mit
// jq. Laufen sie im Container des Knotens (docker exec), erben sie dessen
// Umgebung -- auf C1 die Leistungsschalter aus deploy/perf-c1-setzen.sh. Am
// 21.11.2026 schrieb rpcQuietTxFromEnv beim Start eine Warnzeile auf stdout;
// ergebnis.json begann mit "[RPC] ...", jq brach ab und der Nachweis nach dem
// Ende wurde nicht als Artefakt abgelegt (Lauf 44904131085).
//
// Dieser Test startet das Test-Binaer dieses Pakets (dieselben Pakete wie
// aequitasd, also dieselben Initialisierungen) mit genau der Umgebung von C1
// und ohne Test: Auf stdout darf ausser der Zeile des Test-Rahmens nichts
// stehen. Hinweise gehoeren nach stderr.
func TestStart_SchreibtNichtsAufStdout(t *testing.T) {
	if os.Getenv("AEQUITAS_STDOUT_KIND") == "1" {
		return
	}
	c1 := map[string]string{
		"ENABLE_MULTI_BLOCK_TICK":                 "1",
		"AEQUITAS_MAX_TXS_PER_BLOCK":              "7000",
		"AEQUITAS_COMPRESS_BLOCK_PAYLOAD":         "1",
		"AEQUITAS_DB_MAX_CONNS":                   "100",
		"AEQUITAS_PRODUCE_WHEN_BACKLOG_SHRINKING": "1",
		"AEQUITAS_RPC_RATE_LIMIT_MAX":             "",
		"AEQUITAS_RPC_QUIET_TX":                   "1",
		"AEQUITAS_EIGENLAST_BREMSE":               "0",
		"AEQUITAS_PEER_LAG_BODEN":                 "1500",
		"AEQUITAS_PEER_LAG_SLACK":                 "5",
		"GOMEMLIMIT":                              "8GiB",
		"AEQUITAS_RPC_RATE_LIMIT_FREI":            "",
		"AEQUITAS_BLOCK_AUS_SPEICHER":             "1",
		"AEQUITAS_WAL_FLUSH_TEILE":                "4",
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "AEQUITAS_STDOUT_KIND=1")
	for k, v := range c1 {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var aus, log bytes.Buffer
	cmd.Stdout, cmd.Stderr = &aus, &log
	if err := cmd.Run(); err != nil {
		t.Fatalf("Kindprozess: %v\nstderr: %s", err, log.String())
	}
	// Der Test-Rahmen meldet "PASS" (und je nach Version einen Hinweis, dass
	// kein Test lief); alles andere waere eine Zeile vor dem JSON.
	for _, z := range strings.Split(strings.TrimSpace(aus.String()), "\n") {
		z = strings.TrimSpace(z)
		if z == "" || z == "PASS" || strings.HasPrefix(z, "testing: warning: no tests to run") {
			continue
		}
		t.Errorf("Zeile auf stdout beim Start (gehoert nach stderr): %q", z)
	}
	// Gegenprobe, dass die Umgebung wirkt: die Warnung steht -- auf stderr.
	if !strings.Contains(log.String(), "AEQUITAS_RPC_QUIET_TX=1") {
		t.Errorf("Warnung zu AEQUITAS_RPC_QUIET_TX fehlt auf stderr: %q", log.String())
	}
}
