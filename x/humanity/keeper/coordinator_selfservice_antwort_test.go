package keeper

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Audit B2: /besitznachweis traegt seit aequitas-biometric-beta #121 auch den
// ML-DSA-65-Schluessel und dessen Besitznachweis. Mit echten Groessen
// (1952 + 3309 Byte als Hex) muss der Knoten die Antwort weiter lesen.
func TestBesitznachweisAntwort_MitMLDSAFeldern(t *testing.T) {
	pub := strings.Repeat("ab", 32)
	sig := strings.Repeat("cd", 64)
	roh, _ := json.Marshal(map[string]string{
		"public_key":    pub,
		"key_signature": sig,
		"human_wallet":  "0x" + strings.Repeat("a", 40),
		"pq_public_key": strings.Repeat("ef", 1952),
		"pq_besitz_sig": strings.Repeat("12", 3309),
	})
	if len(roh) <= 8<<10 {
		t.Fatalf("Testantwort zu klein (%d Byte) -- sie soll die alte 8-KiB-Grenze ueberschreiten", len(roh))
	}
	p, s, err := besitznachweisAusAntwort(http.StatusOK, strings.NewReader(string(roh)))
	if err != nil || p != pub || s != sig {
		t.Fatalf("Antwort mit ML-DSA-Feldern abgewiesen: pub=%q sig=%q err=%v", p, s, err)
	}
}

// Missbrauch: ein Coordinator, der eine riesige Antwort schickt, wird
// abgewiesen -- weder ganz gelesen noch abgeschnitten geglaubt.
func TestBesitznachweisAntwort_ZuGrossWirdAbgewiesen(t *testing.T) {
	roh, _ := json.Marshal(map[string]string{
		"public_key":    strings.Repeat("ab", 32),
		"key_signature": strings.Repeat("cd", 64),
		"x":             strings.Repeat("y", besitznachweisAntwortMax),
	})
	if _, _, err := besitznachweisAusAntwort(http.StatusOK, strings.NewReader(string(roh))); err == nil {
		t.Fatal("Antwort ueber der Grenze wurde angenommen")
	}
}

func TestBesitznachweisAntwort_AblehnungUndLeer(t *testing.T) {
	if _, _, err := besitznachweisAusAntwort(http.StatusBadRequest, strings.NewReader(`{"detail":"no proof"}`)); err == nil || !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("Ablehnung des Coordinators nicht gemeldet: %v", err)
	}
	if _, _, err := besitznachweisAusAntwort(http.StatusOK, strings.NewReader(`{}`)); err == nil {
		t.Fatal("leere Antwort wurde als Nachweis angenommen")
	}
}
