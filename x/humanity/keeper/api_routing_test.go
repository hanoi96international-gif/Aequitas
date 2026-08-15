package keeper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lib/pq"
)

// The catch-all used to answer EVERY unrouted path with HTTP 200 and the full
// explorer page — /api/typo included. Two things follow from that, and both
// were confirmed against production before this test existed:
//
//   - no API call can fail cleanly, so a caller either gets a JSON parse error
//     or, worse, reads a missing field and states something false about the
//     chain (the exact defect fixed twice elsewhere in this audit);
//   - a 60-byte request yields a 182 KB response, unauthenticated and
//     unlimited.
//
// These pin the boundary: machine-facing prefixes answer honestly, while the
// explorer's client-side routes keep serving the page.

func newRoutingTestServer() *APIServer {
	return &APIServer{state: newTestState()}
}

func TestBuildMux_UnknownAPIPathIs404(t *testing.T) {
	mux := newRoutingTestServer().buildMux()
	for _, path := range []string{
		"/api/does-not-exist",
		"/api/humans-typo",
		"/api/v2/anything",
		"/debug/pprof/heap", // pprof lives on its own loopback-only mux, never here
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 404 {
			t.Errorf("%s: status = %d, want 404 (body starts %q)", path, rec.Code, truncBody(rec.Body.String()))
			continue
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("%s: Content-Type = %q, want JSON — an API caller must not be handed HTML", path, ct)
		}
		var out map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Errorf("%s: body is not JSON: %v (%q)", path, err, truncBody(rec.Body.String()))
			continue
		}
		if out["error"] == nil {
			t.Errorf("%s: JSON body carries no error field: %v", path, out)
		}
		if rec.Body.Len() > 512 {
			t.Errorf("%s: 404 body is %d bytes — the point is not to ship a page to a bad path",
				path, rec.Body.Len())
		}
	}
}

// The explorer is a single page with client-side routing, so its own paths must
// still return the page. This is the half of the change that must NOT regress.
func TestBuildMux_ExplorerRoutesStillServeThePage(t *testing.T) {
	mux := newRoutingTestServer().buildMux()
	for _, path := range []string{"/index/distribution", "/network/overview", "/explorer.html"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 {
			t.Errorf("%s: status = %d, want 200 — client-side routes must keep serving the page", path, rec.Code)
			continue
		}
		if !strings.Contains(rec.Body.String(), "<html") {
			t.Errorf("%s: body does not look like the explorer page (starts %q)", path, truncBody(rec.Body.String()))
		}
	}
}

// A registered route must still reach its handler rather than the new 404 —
// i.e. the guard is on the catch-all, not in front of the routing table.
func TestBuildMux_RegisteredAPIRouteStillReachesItsHandler(t *testing.T) {
	mux := newRoutingTestServer().buildMux()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/humans", nil)
	req.RemoteAddr = "198.51.100.7:1"
	mux.ServeHTTP(rec, req)
	if rec.Code == 404 {
		t.Fatalf("/api/humans returned 404 — a registered route was swallowed by the catch-all guard")
	}
	if ct := rec.Header().Get("Content-Type"); ct != "" && strings.Contains(ct, "text/html") {
		t.Fatalf("/api/humans answered with HTML (%q)", ct)
	}
}

func truncBody(s string) string {
	if len(s) > 60 {
		return s[:60] + "..."
	}
	return s
}

// jsonStateError decides what a failed state-layer call tells the caller. The
// three endpoints it now guards used to pass err.Error() through verbatim with
// status 400, so a database failure handed an unauthenticated caller the
// driver's own text — pq codes, constraint and column names, and for a dial
// failure the host being connected to — while also claiming the request was
// malformed.
func TestJSONStateError_HidesInfrastructureDetailButKeepsValidationMessages(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantBody string // exact "error" value expected
		leak     string // must NOT appear anywhere in the response
	}{
		{
			name:     "driver error is hidden",
			err:      fmt.Errorf("could not persist recovered balance for 0xabc: %w", &pq.Error{Code: "23505", Message: `duplicate key value violates unique constraint "chain_accounts_pkey"`}),
			wantCode: 500,
			wantBody: "internal error, please retry shortly",
			leak:     "chain_accounts_pkey",
		},
		{
			name:     "dial failure is hidden (it names the database host)",
			err:      fmt.Errorf("save failed: %w", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")}),
			wantCode: 500,
			wantBody: "internal error, please retry shortly",
			leak:     "dial",
		},
		{
			name:     "cancelled context is hidden",
			err:      fmt.Errorf("escrow lookup: %w", context.DeadlineExceeded),
			wantCode: 500,
			wantBody: "internal error, please retry shortly",
			leak:     "deadline",
		},
		{
			name:     "a real validation message still reaches the caller",
			err:      errors.New("wallet already has a guardian"),
			wantCode: 400,
			wantBody: "wallet already has a guardian",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			jsonStateError(rec, "set-guardian", "0xabc", tc.err)
			if rec.Code != tc.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			var out map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("response is not JSON: %v (%q)", err, rec.Body.String())
			}
			if out["error"] != tc.wantBody {
				t.Errorf("error = %q, want %q", out["error"], tc.wantBody)
			}
			if tc.leak != "" && strings.Contains(strings.ToLower(rec.Body.String()), strings.ToLower(tc.leak)) {
				t.Errorf("response leaks %q: %s", tc.leak, rec.Body.String())
			}
		})
	}
}
