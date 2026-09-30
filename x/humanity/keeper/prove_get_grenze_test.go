package keeper

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Audit 2026-09-29, H4: /api/prove/get ist je Adresse begrenzt, BEVOR der
// Proof-Server gefragt wird.
func TestProveGet_JeAdresseBegrenzt(t *testing.T) {
	t.Setenv("PROOF_SERVER_URL", "")
	t.Setenv("PROOF_SERVER_URLS", "")
	a := &APIServer{}
	adresse := "203.0.113.77:5555"
	var zuViele int
	for i := 0; i < burstProveGetJeIP+5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/prove/get/abc", nil)
		req.RemoteAddr = adresse
		rec := httptest.NewRecorder()
		a.handleProveGetProxy(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			zuViele++
		}
	}
	if zuViele != 5 {
		t.Fatalf("erwartet genau 5 Abweisungen nach %d erlaubten Abrufen, bekam %d", burstProveGetJeIP, zuViele)
	}
}
