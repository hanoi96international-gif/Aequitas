package keeper

import (
	"net/http"
	"net/http/httptest"
	"strings"
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
	// Zwei echte Vergleichsdienste, die auf /health antworten -- und ein
	// toter. Am 24.09.2026 stand "Quorum 2 von 2", waehrend einer der beiden
	// mit Contabo1 verschwunden war.
	lebend := func() string {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"status":"ok"}`))
		}))
		t.Cleanup(s.Close)
		return s.URL
	}
	a, b := lebend(), lebend()
	totSrv := httptest.NewServer(http.NotFoundHandler())
	totURL := totSrv.URL
	totSrv.Close()

	antwort := `{"status":"ok","quorum_size":2,"validator_urls":["` + a + `","` + b + `"]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(antwort))
	}))
	defer srv.Close()
	t.Setenv("AEQUITAS_WACHE_COORDINATOR_URL", srv.URL+"/health")

	if stand, ok := coordinatorWacheStand(); !ok || stand != "Quorum 2, 2 von 2 erreichbar" {
		t.Fatalf("gesund: got %q %v", stand, ok)
	}
	antwort = `{"status":"ok","quorum_size":2,"validator_urls":["` + a + `","` + totURL + `"]}`
	if stand, ok := coordinatorWacheStand(); ok || stand != "nur 1 von 2 Vergleichsdiensten erreichbar, Quorum 2 -- keine Registrierung moeglich" {
		t.Fatalf("konfiguriert 2, erreichbar 1 muss rot sein: got %q %v", stand, ok)
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

// Quorum 1 ist nur im erklaerten Einzelbetrieb mit gueltiger Tagesgrenze kein
// Befund. Jede fehlende oder verdrehte Angabe bleibt rot (fail-closed).
func TestProofServerEinzelbetrieb(t *testing.T) {
	mit := func(e map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"durchsetzung": map[string]interface{}{"quorum": float64(1)}, "einzelbetrieb": e}
	}
	gut := map[string]interface{}{"aktiv": true, "erklaert": true, "tagesgrenze": float64(25)}
	if g, ok := proofServerEinzelbetrieb(mit(gut)); !ok || g != 25 {
		t.Fatalf("erklaerter Einzelbetrieb mit Grenze 25: got %d %v", g, ok)
	}
	for name, e := range map[string]interface{}{
		"fehlt":                  nil,
		"not available":          map[string]interface{}{"error": "not available"},
		"nicht erklaert":         map[string]interface{}{"aktiv": true, "erklaert": false, "tagesgrenze": float64(25)},
		"nicht aktiv":            map[string]interface{}{"aktiv": false, "erklaert": true, "tagesgrenze": float64(25)},
		"Grenze fehlt":           map[string]interface{}{"aktiv": true, "erklaert": true},
		"Grenze null":            map[string]interface{}{"aktiv": true, "erklaert": true, "tagesgrenze": nil},
		"Grenze 0":               map[string]interface{}{"aktiv": true, "erklaert": true, "tagesgrenze": float64(0)},
		"Grenze negativ":         map[string]interface{}{"aktiv": true, "erklaert": true, "tagesgrenze": float64(-1)},
		"Grenze zu gross":        map[string]interface{}{"aktiv": true, "erklaert": true, "tagesgrenze": float64(1001)},
		"Grenze gebrochen":       map[string]interface{}{"aktiv": true, "erklaert": true, "tagesgrenze": 2.5},
		"Grenze als Text":        map[string]interface{}{"aktiv": true, "erklaert": true, "tagesgrenze": "25"},
		"aktiv als Text":         map[string]interface{}{"aktiv": "true", "erklaert": true, "tagesgrenze": float64(25)},
		"erklaert als Text":      map[string]interface{}{"aktiv": true, "erklaert": "an", "tagesgrenze": float64(25)},
		"einzelbetrieb als Text": "an",
	} {
		st := map[string]interface{}{"durchsetzung": map[string]interface{}{"quorum": float64(1)}}
		if e != nil {
			st["einzelbetrieb"] = e
		}
		if g, ok := proofServerEinzelbetrieb(st); ok {
			t.Errorf("%s: muss rot bleiben, got %d %v", name, g, ok)
		}
	}
	if _, ok := proofServerEinzelbetrieb(nil); ok {
		t.Fatal("kein Status muss rot bleiben")
	}
}

// Die Wache nimmt Quorum 1 nur ueber proofServerEinzelbetrieb hin.
func TestWache_Quorum1NurImEinzelbetrieb(t *testing.T) {
	b := functionBodyFromSource(t, "api_wache.go", "func (a *APIServer) handleWache(")
	i := strings.Index(b, "proofServerQuorum(st) == 1")
	j := strings.Index(b, "proofServerEinzelbetrieb(st)")
	if i < 0 || j < i || !strings.Contains(b[i:], "BIO_ATTESTATION_QUORUM=1 ohne erklaerten Einzelbetrieb") {
		t.Fatal("Quorum 1 ohne Einzelbetrieb-Pruefung in handleWache")
	}
}
