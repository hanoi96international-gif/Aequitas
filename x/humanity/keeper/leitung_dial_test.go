package keeper

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Die Gegenstellen der Leitungs-, Weiterleitungs- und Checkpoint-Tests
// (httptest) laufen auf 127.0.0.1.
func init() { leitungDialLokal.Store(true) }

// Missbrauch (Beta-Pruefung 15.10.2026, SSRF): eine URL aus einer signierten
// Leitungsnachricht, die auf Loopback zeigt (pprof, /api/sybil-report ohne
// Token, Docker-Dienste), wird nicht angefragt -- von keinem der drei
// Klienten. Die Weiterleitung gab die Antwort sonst sogar zurueck.
func TestLeitungKlienten_KeinLoopback(t *testing.T) {
	getroffen := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { getroffen = true }))
	defer srv.Close()
	leitungDialLokal.Store(false)
	defer leitungDialLokal.Store(true)
	for name, k := range map[string]*http.Client{
		"leitung": leitungKlient, "weiterleitung": weiterleitungsKlient, "checkpoint": checkpointKlient,
	} {
		resp, err := k.Get(srv.URL + "/debug/pprof/")
		if err == nil {
			resp.Body.Close()
			t.Fatalf("%s-Klient erreicht Loopback", name)
		}
	}
	if getroffen {
		t.Fatal("eine Anfrage kam an")
	}
}
