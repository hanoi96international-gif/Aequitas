package keeper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Jede 429/503-Antwort traegt Retry-After -- auch die, die mit http.Error
// oder jsonError geschrieben werden, ohne dass der Handler daran denkt.
func TestRetryAfter_BeiUeberlastGesetzt(t *testing.T) {
	for _, fall := range []struct {
		code int
		soll string
	}{{429, "10"}, {503, "5"}, {200, ""}, {500, ""}} {
		h := retryAfterMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			jsonError(w, "x", fall.code)
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/x", nil))
		if got := rec.Header().Get("Retry-After"); got != fall.soll {
			t.Errorf("HTTP %d: Retry-After %q, erwartet %q", fall.code, got, fall.soll)
		}
	}
}

// Setzt der Handler selbst einen Wert (z. B. die Challenge-Grenze mit 90 s),
// bleibt er stehen.
func TestRetryAfter_EigenerWertBleibt(t *testing.T) {
	h := retryAfterMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "90")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if got := rec.Header().Get("Retry-After"); got != "90" {
		t.Errorf("Retry-After %q, erwartet 90", got)
	}
}

// /api/events (SSE) braucht http.Flusher und den ResponseController -- die
// Huelle darf beides nicht verdecken, sonst antwortet der Endpunkt mit 500.
func TestRetryAfter_FlusherUndControllerBleiben(t *testing.T) {
	var flusherOK bool
	var controllerErr error
	h := retryAfterMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, flusherOK = w.(http.Flusher)
		controllerErr = http.NewResponseController(w).Flush()
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/events", nil))
	if !flusherOK {
		t.Error("http.Flusher durch die Huelle verloren")
	}
	if controllerErr != nil {
		t.Errorf("ResponseController erreicht den Writer nicht: %v", controllerErr)
	}
}

func TestNetzKennung_Form(t *testing.T) {
	k := netzKennung()
	if !strings.HasPrefix(k, "aequitas-1926-") || k != netzKennung() {
		t.Errorf("Netzkennung %q unerwartet oder nicht stabil", k)
	}
}
