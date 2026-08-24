package keeper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Ein Peer, der zurueckliegt, darf den Zaehler NICHT zuruecksetzen -- das
// waere die eine wirklich gefaehrliche Richtung: ein wiederverwendetes Tripel
// blendet nicht mehr und gibt die Differenz der Geheimnisse preis, auf die es
// angewendet wurde.
func TestPeerOffsetParsing(t *testing.T) {
	const token = "geheim"
	t.Setenv("MPC_CLIENT_TOKEN", token)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]int{"offset": 10240})
	}))
	defer srv.Close()

	got, err := fetchPeerTripleOffset(srv.URL, token)
	if err != nil {
		t.Fatalf("Peer-Zaehler nicht lesbar: %v", err)
	}
	if got != 10240 {
		t.Fatalf("Zaehler %d, erwartet 10240", got)
	}
}

func TestPeerOffsetRejectsWrongToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	if _, err := fetchPeerTripleOffset(srv.URL, "falsch"); err == nil {
		t.Fatal("ein 401 muss ein Fehler sein, kein Zaehler 0 -- sonst wuerde ein " +
			"abgewiesener Peer den Abgleich still auf 0 ziehen")
	}
}

func TestPeerOffsetRejectsNegative(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]int{"offset": -5})
	}))
	defer srv.Close()
	if _, err := fetchPeerTripleOffset(srv.URL, ""); err == nil {
		t.Fatal("ein negativer Zaehler ist keine gueltige Antwort")
	}
}
