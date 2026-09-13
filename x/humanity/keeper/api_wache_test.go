package keeper

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Die Wache muss ein offenes Tor melden: BIO_ATTESTATION_MODE != required
// heisst "Registrierung ohne Gesichtspruefung moeglich" -- am 13.09.2026 stand
// der Modus 14 Stunden auf optional, und nichts hat es gesagt.
func TestProofServerModus(t *testing.T) {
	cases := []struct {
		status map[string]interface{}
		want   string
	}{
		{map[string]interface{}{}, ""},
		{map[string]interface{}{"durchsetzung": "kaputt"}, ""},
		{map[string]interface{}{"durchsetzung": map[string]interface{}{"mode": "Required "}}, "required"},
		{map[string]interface{}{"durchsetzung": map[string]interface{}{"mode": "optional"}}, "optional"},
	}
	for i, c := range cases {
		if got := proofServerModus(c.status); got != c.want {
			t.Fatalf("%d: got %q want %q", i, got, c.want)
		}
	}
}

func TestCoordinatorWacheStand(t *testing.T) {
	antwort := `{"status":"ok","quorum_size":2,"validator_urls":["a","b"]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(antwort))
	}))
	defer srv.Close()
	t.Setenv("AEQUITAS_WACHE_COORDINATOR_URL", srv.URL+"/health")

	if stand, ok := coordinatorWacheStand(); !ok || stand != "Quorum 2 von 2" {
		t.Fatalf("gesund: got %q %v", stand, ok)
	}
	antwort = `{"status":"ok","quorum_size":2,"validator_urls":["a"]}`
	if stand, ok := coordinatorWacheStand(); ok || stand == "" {
		t.Fatalf("Quorum unerfuellbar muss rot sein: got %q %v", stand, ok)
	}
	antwort = `{"status":"degraded"}`
	if _, ok := coordinatorWacheStand(); ok {
		t.Fatal("status != ok muss rot sein")
	}

	// Abgeschaltet: kein Befund, kein Fehler.
	t.Setenv("AEQUITAS_WACHE_COORDINATOR_URL", "aus")
	if stand, ok := coordinatorWacheStand(); ok || stand != "" {
		t.Fatalf("aus: got %q %v", stand, ok)
	}

	// Kein solcher Host: uebersprungen, nicht rot.
	t.Setenv("AEQUITAS_WACHE_COORDINATOR_URL", "http://gibt-es-nicht.invalid:8200/health")
	if stand, ok := coordinatorWacheStand(); ok || stand != "" {
		t.Fatalf("unbekannter Host: got %q %v", stand, ok)
	}

	// Es gibt ihn, er antwortet nicht (Port zu): rot mit Grund.
	tot := httptest.NewServer(http.NotFoundHandler())
	url := tot.URL
	tot.Close()
	t.Setenv("AEQUITAS_WACHE_COORDINATOR_URL", url+"/health")
	if stand, ok := coordinatorWacheStand(); ok || stand == "" {
		t.Fatalf("Verbindung verweigert muss rot sein: got %q %v", stand, ok)
	}
}

func TestProofServerQuorum(t *testing.T) {
	if q := proofServerQuorum(map[string]interface{}{}); q != 0 {
		t.Fatalf("leer: %d", q)
	}
	if q := proofServerQuorum(map[string]interface{}{"durchsetzung": map[string]interface{}{"quorum": float64(2)}}); q != 2 {
		t.Fatalf("json-Zahl: %d", q)
	}
}
