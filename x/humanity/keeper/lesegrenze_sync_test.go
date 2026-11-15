package keeper

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Pruefung von #515, Befund 3: die Wege, ueber die Validatoren Bloecke
// nachholen, haben keine Lesegrenze je IP. Ein zurueckliegender Knoten holt
// seitenweise ohne Pause; mit der Grenze bekam er 429 und kam nicht nach.
// Die Konten-Lesewege bleiben begrenzt.
func TestLesegrenze_SyncWegeUngebremst(t *testing.T) {
	dag := newGhostdagTestDAG()
	cs := newTestState()
	dag.state = cs
	a := &APIServer{blockchain: dag, state: cs}
	mux := a.buildMux()
	abfragen := func(pfad, ip string, n int) (zuViele int) {
		for i := 0; i < n; i++ {
			r := httptest.NewRequest(http.MethodGet, pfad, nil)
			r.RemoteAddr = ip + ":4242"
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code == http.StatusTooManyRequests {
				zuViele++
			}
		}
		return
	}
	for _, pfad := range []string{"/api/blocks?min_height=0&limit=5", "/api/block?height=1"} {
		if n := abfragen(pfad, "198.51.100.21", lesenJeAbsender+50); n != 0 {
			t.Fatalf("%s: %d Anfragen mit 429 abgewiesen -- Sync-Weg gebremst", pfad, n)
		}
	}
	// Gegenprobe: ein Konten-Leseweg bleibt begrenzt.
	if n := abfragen("/api/balance?wallet=0x00000000000000000000000000000000000000c4", "198.51.100.22", lesenJeAbsender+5); n == 0 {
		t.Fatal("/api/balance nicht mehr begrenzt")
	}
}
