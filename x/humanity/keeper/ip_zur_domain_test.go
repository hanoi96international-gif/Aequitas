package keeper

import (
	"net/http/httptest"
	"testing"
)

func TestIPZurDomain(t *testing.T) {
	t.Setenv("AEQUITAS_OEFFENTLICHE_URL", "")
	html := "text/html,application/xhtml+xml"
	for _, f := range []struct {
		methode, host, pfad, accept, want string
	}{
		{"GET", "194.163.188.71:8080", "/explorer", html, "https://aequitas.digital/explorer"},
		{"GET", "194.163.188.71:8080", "/?lang=de", html, "https://aequitas.digital/?lang=de"},
		{"GET", "194.163.188.71:8080", "/api/status", html, ""},  // Schnittstelle
		{"GET", "194.163.188.71:8080", "/api/blocks", "*/*", ""}, // Validator-Sync
		{"POST", "194.163.188.71:8080", "/rpc", "*/*", ""},       // Wallet
		{"GET", "194.163.188.71:8080", "/download/app.apk", html, ""},
		{"GET", "194.163.188.71:8080", "/explorer", "*/*", ""}, // kein Browser
		{"GET", "aequitas.digital", "/explorer", html, ""},     // schon Domain
		{"GET", "127.0.0.1:8080", "/explorer", html, ""},       // lokal
		{"GET", "10.0.0.5:8080", "/explorer", html, ""},        // privat
	} {
		r := httptest.NewRequest(f.methode, "http://"+f.host+f.pfad, nil)
		r.Host = f.host
		r.Header.Set("Accept", f.accept)
		if got := ipSeitenZiel(r); got != f.want {
			t.Errorf("%s %s%s: %q, erwartet %q", f.methode, f.host, f.pfad, got, f.want)
		}
	}
	t.Setenv("AEQUITAS_OEFFENTLICHE_URL", "aus")
	r := httptest.NewRequest("GET", "http://194.163.188.71:8080/explorer", nil)
	r.Header.Set("Accept", html)
	if got := ipSeitenZiel(r); got != "" {
		t.Fatalf("abgeschaltet und trotzdem umgeleitet: %q", got)
	}
}
