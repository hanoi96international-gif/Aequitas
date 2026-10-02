package keeper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	bindenAdr    = "0x1111111111111111111111111111111111111111"
	bindenWallet = "0x0BE8B96100000000000000000000000000E0D016"
)

var bindenBeweis = "0x" + strings.Repeat("ab", 65)

func bindenAufruf(t *testing.T, methode, query string) *httptest.ResponseRecorder {
	t.Helper()
	a := &APIServer{}
	r := httptest.NewRequest(methode, "/binden?"+query, nil)
	w := httptest.NewRecorder()
	a.handleKnotenBindenSeite(w, r)
	return w
}

// Gutfall: der Knopf fuehrt genau zum App-Link, klein geschrieben.
func TestKnotenBindenSeite_Gutfall(t *testing.T) {
	w := bindenAufruf(t, "GET", "adresse="+bindenAdr+"&wallet="+bindenWallet+"&beweis="+bindenBeweis)
	if w.Code != 200 {
		t.Fatalf("Status %d", w.Code)
	}
	want := `href="aequitasapp://knoten-binden?adresse=` + bindenAdr + `&amp;wallet=` + strings.ToLower(bindenWallet) + `&amp;beweis=` + bindenBeweis + `"`
	if !strings.Contains(w.Body.String(), want) {
		t.Fatalf("App-Link fehlt:\n%s", w.Body.String())
	}
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "script-src") {
		t.Fatalf("CSP: %q", csp)
	}
}

// Missbrauch: jede Abweichung im Format ergibt 400, und die Eingabe wird
// nirgends wiedergegeben (keine Spiegelung, kein XSS).
func TestKnotenBindenSeite_FalscheEingabenWerdenNichtGespiegelt(t *testing.T) {
	boese := `"><script>alert(1)</script>`
	faelle := []string{
		"",
		"adresse=" + bindenAdr + "&wallet=" + bindenWallet,
		"adresse=" + bindenAdr + boese + "&wallet=" + bindenWallet + "&beweis=" + bindenBeweis,
		"adresse=" + bindenAdr + "&wallet=" + bindenWallet + "&beweis=" + bindenBeweis + "00",
		"adresse=" + bindenAdr + "&wallet=" + bindenWallet + "&beweis=" + bindenBeweis[:len(bindenBeweis)-1] + "z",
		"adresse=javascript:alert(1)&wallet=" + bindenWallet + "&beweis=" + bindenBeweis,
		"adresse=" + bindenAdr + "%0a&wallet=" + bindenWallet + "&beweis=" + bindenBeweis,
	}
	for _, q := range faelle {
		w := bindenAufruf(t, "GET", q)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%q: Status %d, erwartet 400", q, w.Code)
		}
		b := w.Body.String()
		if strings.Contains(b, "aequitasapp://") || strings.Contains(b, "<script") || strings.Contains(b, "javascript:") {
			t.Fatalf("%q: Eingabe gespiegelt:\n%s", q, b)
		}
	}
	if w := bindenAufruf(t, "POST", "adresse="+bindenAdr+"&wallet="+bindenWallet+"&beweis="+bindenBeweis); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", w.Code)
	}
}
