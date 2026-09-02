package keeper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Ein Allokator, zwei Parteien, EIN Bereich.
//
// Der Vorgaenger -- jede Partei zaehlt selbst und gleicht per Maximum ab --
// war ein Wettlauf: gemessen am 24.08.2026 ueberlebte ein Abstand von 2048
// den Abgleich unbeschaedigt, weil jede Partei die andere erst LAS, nachdem
// diese schon weitergezaehlt hatte. Deshalb hier kein Abgleich mehr, sondern
// eine einzige Quelle.

func TestSelbeSitzungBekommtDenselbenBereich(t *testing.T) {
	const token = "geheim"
	t.Setenv("MPC_CLIENT_TOKEN", token)

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(tripleRangeResponse{Offset: 4096})
	}))
	defer srv.Close()

	// Beide Parteien fragen dieselbe Stelle und bekommen dieselbe Antwort --
	// genau das, was der Multiplikation zugrunde liegt: Partei 0s Tripel an
	// Stelle k gehoert zu Partei 1s Tripel an Stelle k.
	for i := 0; i < 2; i++ {
		got, err := fetchTripleRange(srv.URL, token, "sitzung-a", 2048)
		if err != nil {
			t.Fatalf("Bereich nicht abrufbar: %v", err)
		}
		if got != 4096 {
			t.Fatalf("Bereich %d, erwartet 4096", got)
		}
	}
	if calls != 2 {
		t.Fatalf("%d Anfragen, erwartet 2", calls)
	}
}

func TestUnerreichbarerAllokatorIstEinFehler(t *testing.T) {
	// Kein Vergleich ist die sichere Richtung. Eine Pruefung, die nicht
	// stattfinden konnte, darf nie als "kein Duplikat" gelten -- das ist der
	// unumkehrbare Fehler.
	if _, err := fetchTripleRange("http://127.0.0.1:1", "t", "s", 2048); err == nil {
		t.Fatal("ein unerreichbarer Allokator muss ein Fehler sein, kein Bereich 0")
	}
}

func TestAbgewiesenerAllokatorIstEinFehler(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	// Ohne diese Pruefung wuerde ein 401 still als Bereich 0 gelesen -- beide
	// Parteien landeten auf verschiedenen Tripeln und der Vergleich waere
	// wieder Rauschen.
	if _, err := fetchTripleRange(srv.URL, "falsch", "s", 2048); err == nil {
		t.Fatal("ein 401 muss ein Fehler sein")
	}
}

func TestNegativerBereichWirdAbgelehnt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(tripleRangeResponse{Offset: -1})
	}))
	defer srv.Close()
	if _, err := fetchTripleRange(srv.URL, "", "s", 2048); err == nil {
		t.Fatal("ein negativer Bereich ist keine gueltige Antwort")
	}
}

// Partei 0 teilt selbst zu, jede andere fragt Partei 0.
func TestAllokatorIstPartei0(t *testing.T) {
	a := &APIServer{mpc: &mpcNode{index: 0, peers: []string{"http://a:8080", "http://b:8080"}}}
	url, err := a.tripleAllocatorURL()
	if err != nil {
		t.Fatal(err)
	}
	if url != "" {
		t.Fatalf("Partei 0 muss selbst zuteilen, bekam %q", url)
	}

	b := &APIServer{mpc: &mpcNode{index: 1, peers: []string{"http://a:8080", "http://b:8080"}}}
	url, err = b.tripleAllocatorURL()
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://a:8080" {
		t.Fatalf("Partei 1 muss Partei 0 fragen, bekam %q", url)
	}
}
