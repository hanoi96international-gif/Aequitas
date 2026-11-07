package keeper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// /api/nullifier-passt: ja nur fuer genau die Wallet des Nullifiers, nie eine
// Aufzaehlung, Grenze je Absender.
func TestNullifierPasst(t *testing.T) {
	cs := newTestState()
	n, w := "123456789", "0x00000000000000000000000000000000000000aa"
	cs.mu.Lock()
	cs.nullifiers = map[string]string{n: w}
	cs.mu.Unlock()
	a := &APIServer{state: cs}
	frage := func(q string) (int, map[string]bool) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/nullifier-passt?"+q, nil)
		req.RemoteAddr = "198.51.100.7:1234"
		a.handleNullifierPasst(rec, req)
		var out map[string]bool
		json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	ipBurst.Delete("nullifier-passt:198.51.100.7")
	if c, o := frage("n=" + n + "&wallet=" + w); c != 200 || !o["passt"] {
		t.Fatalf("eigene Wallet: %d %v", c, o)
	}
	if c, o := frage("n=" + n + "&wallet=0x00000000000000000000000000000000000000bb"); c != 200 || o["passt"] {
		t.Fatalf("fremde Wallet: %d %v", c, o)
	}
	if c, o := frage("n=999&wallet=" + w); c != 200 || o["passt"] {
		t.Fatalf("unbekannter Nullifier: %d %v", c, o)
	}
	for _, q := range []string{"n=" + n, "wallet=" + w, "n=abc&wallet=" + w, "n=" + n + "&wallet=0xAA"} {
		if c, _ := frage(q); c != http.StatusBadRequest {
			t.Fatalf("%s: %d, erwartet 400", q, c)
		}
	}
	ipBurst.Delete("nullifier-passt:198.51.100.7")
	letzter := 0
	for i := 0; i < burstNullifierPasstJeIP+1; i++ {
		letzter, _ = frage("n=" + n + "&wallet=" + w)
	}
	if letzter != http.StatusTooManyRequests {
		t.Fatalf("ohne Grenze: %d", letzter)
	}
}
