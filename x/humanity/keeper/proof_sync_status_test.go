package keeper

import "testing"

// Fehlt die Konfiguration, wird die Meldung uebersprungen -- und das muss
// sichtbar sein, nicht still (Audit von null, 02.10.2026).
func TestProofSyncUebersprungenIstSichtbar(t *testing.T) {
	t.Setenv("PROOF_SERVER_URLS", "")
	t.Setenv("PROOF_SERVER_URL", "")
	t.Setenv("CHAIN_SERVICE_TOKEN", "")
	vorher := proofSyncUebersprungen.Load()
	notifyProofServerWithRetryQueue(newTestState(), "0x01", "0xa100000000000000000000000000000000000001")
	if proofSyncUebersprungen.Load() != vorher+1 {
		t.Fatalf("uebersprungene Meldung nicht gezaehlt")
	}
	st := proofSyncStand()
	if st["konfiguriert"] != false {
		t.Fatalf("ohne URL und Token darf konfiguriert nicht true sein: %v", st)
	}
	for k, v := range st {
		if s, ok := v.(string); ok {
			t.Fatalf("Feld %s ist Text (%q) -- nur Ja/Nein und Zahlen veroeffentlichen", k, s)
		}
	}
}
