package keeper

import (
	"strings"
	"testing"
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
	lang := oeffentlicherDegradedGrund(strings.Repeat("ä", 400))
	if len(lang) > 310 || !strings.HasSuffix(lang, "…") {
		t.Fatalf("nicht gekuerzt: %d Bytes", len(lang))
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
	if b := functionBodyFromSource(t, "api_wache.go", "func (a *APIServer) handleWache("); !strings.Contains(b, "oeffentlicherDegradedGrund(reason)") {
		t.Fatal("/api/wache gibt den Degraded-Grund ungeschwaerzt aus")
	}
}
