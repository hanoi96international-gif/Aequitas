package keeper

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// Regression tests for P2-d (audit 2026-07-06): handleHumans gained
// limit/offset pagination to bound its response payload size as the
// registered-human count grows, while keeping the existing no-params
// response shape ("total" = the real total count, not just the page)
// unchanged for every caller that doesn't pass these params.

func newHumansTestServer(humanCount int) *APIServer {
	cs := newTestState()
	for i := 0; i < humanCount; i++ {
		addHuman(cs, "0x"+string(rune('a'+i%26))+string(rune('0'+i/26))+"00000000000000000000000000000000000000", 100)
	}
	return &APIServer{state: cs}
}

func doHumansRequest(t *testing.T, a *APIServer, remoteAddr, query string) map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/humans"+query, nil)
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	a.handleHumans(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("could not decode response: %v (body=%s)", err, rec.Body.String())
	}
	return out
}

func TestHandleHumans_NoParamsReturnsAllUnderDefaultLimit(t *testing.T) {
	a := newHumansTestServer(10)
	out := doHumansRequest(t, a, "198.51.100.1:1", "")
	if int(out["total"].(float64)) != 10 {
		t.Fatalf("total = %v, want 10", out["total"])
	}
	if len(out["humans"].([]interface{})) != 10 {
		t.Fatalf("humans length = %d, want 10 (well under the default limit)", len(out["humans"].([]interface{})))
	}
}

func TestHandleHumans_LimitBoundsPageSizeButNotTotal(t *testing.T) {
	a := newHumansTestServer(10)
	out := doHumansRequest(t, a, "198.51.100.2:1", "?limit=3")
	if int(out["total"].(float64)) != 10 {
		t.Fatalf("total = %v, want 10 (the real total, not the page size)", out["total"])
	}
	if len(out["humans"].([]interface{})) != 3 {
		t.Fatalf("humans length = %d, want 3", len(out["humans"].([]interface{})))
	}
}

func TestHandleHumans_OffsetSkipsAlreadySeenEntries(t *testing.T) {
	a := newHumansTestServer(10)
	page1 := doHumansRequest(t, a, "198.51.100.3:1", "?limit=4&offset=0")
	page2 := doHumansRequest(t, a, "198.51.100.4:1", "?limit=4&offset=4")
	addrs := func(page map[string]interface{}) map[string]bool {
		set := map[string]bool{}
		for _, h := range page["humans"].([]interface{}) {
			set[h.(map[string]interface{})["address"].(string)] = true
		}
		return set
	}
	p1, p2 := addrs(page1), addrs(page2)
	if len(p1) != 4 || len(p2) != 4 {
		t.Fatalf("expected 4 entries per page, got %d and %d", len(p1), len(p2))
	}
	for addr := range p1 {
		if p2[addr] {
			t.Fatalf("address %s appeared in both offset=0 and offset=4 pages", addr)
		}
	}
}

func TestHandleHumans_InvalidLimitFallsBackToDefault(t *testing.T) {
	a := newHumansTestServer(5)
	out := doHumansRequest(t, a, "198.51.100.5:1", "?limit=-1")
	if int(out["limit"].(float64)) != 500 {
		t.Fatalf("limit = %v, want the default (500) for an invalid input", out["limit"])
	}
	out2 := doHumansRequest(t, a, "198.51.100.6:1", "?limit=999999")
	if int(out2["limit"].(float64)) != 500 {
		t.Fatalf("limit = %v, want the default (500) for an out-of-range input", out2["limit"])
	}
}
