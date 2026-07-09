package keeper

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandlePeerRegister_UnauthenticatedRequestCannotConsumeRateLimit is the
// regression guard for the unauthenticated registration-griefing DoS found
// live 2026-07-09: registrationRateLimited used to be checked BEFORE any
// signature/secret verification, keyed purely on the caller-supplied
// signing_address. Validator signing addresses are public (visible in every
// block's proposer field), so anyone could send an unsigned POST naming a
// real validator's address and consume/refresh that address's rate-limit
// slot — permanently starving the real node's own legitimate,
// signature-proven re-registration attempts, with no way to recover short
// of the primary's in-memory map happening to go quiet. Confirmed live:
// two real secondaries were locked out for 10+ minutes despite an active
// retry loop.
//
// This sends two back-to-back unauthenticated requests (empty signature,
// no PEER_SECRET) naming the SAME signing_address. Neither may receive the
// 429 rate-limit response — an unauthenticated caller must never be able to
// claim or observe another address's rate-limit slot at all.
func TestHandlePeerRegister_UnauthenticatedRequestCannotConsumeRateLimit(t *testing.T) {
	dag := newGhostdagTestDAG()
	cs := newTestState()
	dag.state = cs
	dag.authorizedValidators = make(map[string]bool)
	a := &APIServer{blockchain: dag, state: cs}

	const victimAddr = "0x0be8b961cbf6564bd1931b0803d35c0659e0d016"
	body := `{"url":"","signing_address":"` + victimAddr + `","signature":"","peer_secret":"","node_operator_wallet":"","operator_binding_signature":""}`

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/api/peers/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		a.handlePeerRegister(rec, req)

		if rec.Code == 429 {
			t.Fatalf("request %d: unauthenticated request must never receive the rate-limit response (got 429) — an attacker with no key could grief this address's registration", i+1)
		}
	}
}

// TestHandlePeerRegister_AuthenticatedRateLimitStillWorks verifies the
// companion half of the same fix didn't remove the rate limiter's actual
// protection: a genuinely authenticated caller (PEER_SECRET bypass, the
// cheapest path to exercise in a unit test) sending two requests back to
// back must still get rate-limited on the second one.
func TestHandlePeerRegister_AuthenticatedRateLimitStillWorks(t *testing.T) {
	t.Setenv("ALLOW_PEER_SECRET_BYPASS", "true")
	t.Setenv("PEER_SECRET", "test-secret-for-rate-limit-check")

	dag := newGhostdagTestDAG()
	cs := newTestState()
	dag.state = cs
	dag.authorizedValidators = make(map[string]bool)
	a := &APIServer{blockchain: dag, state: cs}

	// Unique address per test run so this test's own rate-limit state can
	// never collide with the unauthenticated test above or a prior run.
	const addr = "0x00000000000000000000000000000000abcdef"
	body := `{"url":"","signing_address":"` + addr + `","signature":"","peer_secret":"test-secret-for-rate-limit-check","node_operator_wallet":"","operator_binding_signature":""}`

	req1 := httptest.NewRequest("POST", "/api/peers/register", strings.NewReader(body))
	rec1 := httptest.NewRecorder()
	a.handlePeerRegister(rec1, req1)
	if rec1.Code == 429 {
		t.Fatalf("first authenticated request should not be rate-limited, got 429")
	}

	req2 := httptest.NewRequest("POST", "/api/peers/register", strings.NewReader(body))
	rec2 := httptest.NewRecorder()
	a.handlePeerRegister(rec2, req2)
	if rec2.Code != 429 {
		t.Fatalf("second immediate authenticated request with the same address should be rate-limited, got %d", rec2.Code)
	}
}
