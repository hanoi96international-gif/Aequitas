package keeper

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

// Pruefung #330, 2. Durchgang, INFO-5: der oeffentliche Degraded-Grund nennt
// weder Wallet noch Hash; lange Fehlertexte werden gekuerzt.
func TestDegraded_OeffentlichGeschwaerzt(t *testing.T) {
	wallet := "0x00000000000000000000000000000000000000ab"
	hash := "0x" + strings.Repeat("c", 64)
	r := oeffentlicherDegradedGrund("registration_recovery: EVM tx " + hash + " registered " + wallet + " on-chain but Go-state sync failed")
	if strings.Contains(r, wallet[2:]) || strings.Contains(r, hash[2:]) {
		t.Fatalf("Wallet oder Hash im oeffentlichen Grund: %q", r)
	}
	if !strings.Contains(r, "registration_recovery") || !strings.Contains(r, "0x…") {
		t.Fatalf("Grund unkenntlich: %q", r)
	}
	// Ungerade Grenze: der Schnitt faellt mitten in ein Zeichen.
	lang := oeffentlicherDegradedGrund("a" + strings.Repeat("ä", 400))
	if len(lang) > 310 || !strings.HasSuffix(lang, "…") || !utf8.ValidString(lang) {
		t.Fatalf("nicht sauber gekuerzt: %d Bytes, gueltig=%v", len(lang), utf8.ValidString(lang))
	}
	// Grenzen der Schwaerzung: sechs Hex-Zeichen mit 0x ja, fuenf nein;
	// Gross-Praefix; Hash ohne Praefix; Woerter und kurze Zahlen bleiben.
	for ein, aus := range map[string]string{
		"x 0xabcdef y":                        "x 0x… y",
		"x 0xabcde y":                         "x 0xabcde y",
		"x 0XABCDEF y":                        "x 0x… y",
		"x " + strings.Repeat("e", 64) + " y": "x 0x… y",
		"decade facade Hoehe 123456":          "decade facade Hoehe 123456",
		// Nullifier dezimal (Pruefung #336, F1).
		"saving nullifier 1234567890123456789012345678901234567890: pq: x": "saving nullifier …: pq: x",
		"Block #7708396": "Block #7708396",
	} {
		if got := oeffentlicherDegradedGrund(ein); got != aus {
			t.Errorf("%q -> %q, erwartet %q", ein, got, aus)
		}
	}
	if oeffentlicherDegradedGrund("") != "" {
		t.Fatal("leerer Grund nicht leer")
	}
}

// Beide oeffentlichen Wege schwaerzen.
func TestDegraded_OeffentlicheWegeSchwaerzen(t *testing.T) {
	if b := functionBodyFromSource(t, "api.go", "func (a *APIServer) handleCombinedHealth("); !strings.Contains(b, "oeffentlicherDegradedGrund(a.state.BootstrapDegradedReason())") {
		t.Fatal("/api/health/combined gibt den Degraded-Grund ungeschwaerzt aus")
	}
	if b := functionBodyFromSource(t, "api_wache.go", "func (a *APIServer) handleWache("); !strings.Contains(b, "oeffentlicherDegradedGrund(reason)") || !strings.Contains(b, "oeffentlicherDegradedGrund(dr)") {
		t.Fatal("/api/wache gibt einen Degraded-Grund ungeschwaerzt aus")
	}
	b := functionBodyFromSource(t, "api.go", "func (a *APIServer) handleCombinedHealth(")
	if strings.Count(b, "oeffentlicherDegradedGrund(dagDegradedReason)") != 2 {
		t.Fatal("/api/health/combined gibt den DAG-Grund ungeschwaerzt aus (Hinweis und Feld)")
	}
}

// Pruefung #336, F2: die Zugriffszaehler nennen keine Adressen.
func TestEndpointStats_OhneAdressen(t *testing.T) {
	var c endpointCounter
	c.note("203.0.113.7:4444", 10)
	c.note("10.0.0.2:5555", 10)
	c.note("[::1]:6666", 10)
	s := c.snapshot()
	for k, v := range s {
		if strings.Contains(fmt.Sprint(v), "203.0.113.7") || strings.Contains(k, "callers") && fmt.Sprintf("%T", v) != "int" {
			t.Fatalf("Feld %s nennt Adressen: %v", k, v)
		}
	}
	if s["callers_anzahl"] != 3 || s["callers_privat"] != 2 {
		t.Fatalf("Zaehler: %v", s)
	}
}

// Pruefung #336, F1 (Quelle): der Snapshot-Fehler nennt den Nullifier nicht.
func TestSnapshot_FehlerOhneNullifier(t *testing.T) {
	src, err := os.ReadFile("snapshot.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), `"saving nullifier %s`) {
		t.Fatal("snapshot.go nennt den Nullifier im Fehlertext")
	}
}
