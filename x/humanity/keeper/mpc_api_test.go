package keeper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/mpc"
)

func resetVerdicts() {
	mpcVerdictMu.Lock()
	mpcVerdicts = map[string]mpcVerdict{}
	mpcVerdictMu.Unlock()
}

// TestGateIsOffUnlessRequired: this must not start refusing registrations the
// moment the code lands. It is opt-in, like the rest of the subsystem.
func TestGateIsOffUnlessRequired(t *testing.T) {
	resetVerdicts()
	t.Setenv("MPC_REQUIRED", "")
	if ok, msg := mpcGateAllows(""); !ok {
		t.Errorf("registration was refused with MPC_REQUIRED unset: %q", msg)
	}
}

// TestGateRefusesWithoutAVerdict: the check happens before registration, so a
// registration arriving without one has not been checked.
func TestGateRefusesWithoutAVerdict(t *testing.T) {
	resetVerdicts()
	t.Setenv("MPC_REQUIRED", "true")

	ok, msg := mpcGateAllows("")
	if ok {
		t.Error("a registration with no duplicate-check session was allowed")
	}
	if !strings.Contains(strings.ToLower(msg), "biometric") {
		t.Errorf("message %q does not tell the person what to do", msg)
	}

	if ok, _ := mpcGateAllows("a-session-nobody-checked"); ok {
		t.Error("a session with no recorded verdict was allowed — the client would only have to " +
			"invent a session id")
	}
}

// TestGateRefusesAKnownDuplicate is the point of the whole subsystem.
func TestGateRefusesAKnownDuplicate(t *testing.T) {
	resetVerdicts()
	t.Setenv("MPC_REQUIRED", "true")
	recordMPCVerdict("dup", mpcVerdict{duplicate: true, compared: 3, at: time.Now()})

	ok, msg := mpcGateAllows("dup")
	if ok {
		t.Fatal("an already-registered biometric was allowed through")
	}
	if !strings.Contains(msg, "already registered") {
		t.Errorf("message %q does not say why", msg)
	}
}

// TestGateAllowsACleanVerdict: the check must also let real people through, or
// it is just an outage with extra steps.
func TestGateAllowsACleanVerdict(t *testing.T) {
	resetVerdicts()
	t.Setenv("MPC_REQUIRED", "true")
	recordMPCVerdict("clean", mpcVerdict{duplicate: false, compared: 12, at: time.Now()})

	if ok, msg := mpcGateAllows("clean"); !ok {
		t.Errorf("a person who is not a duplicate was refused: %q", msg)
	}
}

// TestAVerdictIsSpentOnce: one honest check must authorise exactly one
// registration, or it becomes a supply of accounts.
func TestAVerdictIsSpentOnce(t *testing.T) {
	resetVerdicts()
	t.Setenv("MPC_REQUIRED", "true")
	recordMPCVerdict("once", mpcVerdict{duplicate: false, at: time.Now()})

	if ok, _ := mpcGateAllows("once"); !ok {
		t.Fatal("the first use was refused")
	}
	if ok, _ := mpcGateAllows("once"); ok {
		t.Error("the same verdict authorised a second registration — one check would yield " +
			"unlimited accounts")
	}
}

// TestAStaleVerdictIsRefused: a pass from an hour ago was measured against an
// older enrolment set, so anyone who registered since was never compared.
func TestAStaleVerdictIsRefused(t *testing.T) {
	resetVerdicts()
	t.Setenv("MPC_REQUIRED", "true")
	recordMPCVerdict("stale", mpcVerdict{
		duplicate: false,
		at:        time.Now().Add(-2 * mpcVerdictTTL),
	})
	if ok, _ := mpcGateAllows("stale"); ok {
		t.Error("a verdict older than its TTL was accepted")
	}
}

// TestRefusalsAreRetryableNotAccusations is the Aequitas constraint in test
// form: a person refused because a validator was rebooting must not be told
// they are already registered, and must be able to try again.
func TestRefusalsAreRetryableNotAccusations(t *testing.T) {
	resetVerdicts()
	t.Setenv("MPC_REQUIRED", "true")

	for _, session := range []string{"", "never-checked", "expired"} {
		_, msg := mpcGateAllows(session)
		low := strings.ToLower(msg)
		if strings.Contains(low, "already registered") {
			t.Errorf("session %q: an operational failure was reported as %q — that accuses "+
				"someone of something the system does not know", session, msg)
		}
		if !strings.Contains(low, "redo") && !strings.Contains(low, "again") {
			t.Errorf("session %q: message %q does not tell the person they can retry", session, msg)
		}
	}
}

// TestEndpointsRefuseWithoutAToken: an open enrol endpoint lets anyone poison
// the index so that real people are told they are duplicates.
func TestEndpointsRefuseWithoutAToken(t *testing.T) {
	t.Setenv("MPC_CLIENT_TOKEN", "")
	req := httptest.NewRequest(http.MethodPost, "/mpc/enroll", strings.NewReader(`{"keys":[1]}`))
	if mpcClientAuthorized(req) {
		t.Error("a request was authorised with no MPC_CLIENT_TOKEN configured")
	}

	t.Setenv("MPC_CLIENT_TOKEN", "the-real-token")
	req.Header.Set("Authorization", "Bearer the-wrong-token")
	if mpcClientAuthorized(req) {
		t.Error("a request with the wrong token was authorised")
	}
	req.Header.Set("Authorization", "Bearer the-real-token")
	if !mpcClientAuthorized(req) {
		t.Error("a request with the correct token was refused")
	}
}

// TestEnrolWithoutBucketKeysIsRefused: such an enrolment is stored and then
// never compared against anyone, which is indistinguishable from not storing it
// while looking like it worked.
func TestEnrolWithoutBucketKeysIsRefused(t *testing.T) {
	t.Setenv("MPC_CLIENT_TOKEN", "tok")
	req := httptest.NewRequest(http.MethodPost, "/mpc/enroll",
		strings.NewReader(`{"enrollment_id":"e1","row":"00","keys":[]}`))
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()

	if _, ok := readMPCSubmission(rec, req); ok {
		t.Fatal("a submission with no bucket keys was accepted")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

// TestTriplesAreNeverGeneratedLocally pins the mistake that shipped.
//
// The first version of mpcTriples called GenerateTriples on each party. Beaver
// triples are a correlated secret, so a party that draws its own gets a set
// that does not belong with its peers', and every comparison becomes noise. It
// failed closed rather than lying, but it could never have worked.
//
// With no triple file configured this must now say so plainly instead of
// producing something that looks usable.
func TestTriplesAreNeverGeneratedLocally(t *testing.T) {
	t.Setenv("MPC_TRIPLE_FILE", "")
	a := &APIServer{mpc: &mpcNode{size: 2}}

	store, err := a.mpcTriples(100)
	if err == nil {
		t.Fatalf("triples were supplied with no dealer file configured (%d available) — a party "+
			"that makes its own gets an uncorrelated set", store.Remaining())
	}
	if !strings.Contains(err.Error(), "MPC_TRIPLE_FILE") {
		t.Errorf("error %q does not name what is missing", err)
	}
}

// TestTripleFileRoundTrips: the dealer writes, the party reads, and the shares
// must survive the trip unchanged — they are the correlation.
func TestTripleFileRoundTrips(t *testing.T) {
	rows, err := mpc.GenerateTriples(64, 2)
	if err != nil {
		t.Fatal(err)
	}
	encoded := mpc.EncodeTriples(rows[0])
	decoded, err := mpc.DecodeTriples(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != len(rows[0]) {
		t.Fatalf("got %d triples, want %d", len(decoded), len(rows[0]))
	}
	for i := range decoded {
		if decoded[i] != rows[0][i] {
			t.Fatalf("triple %d changed in transit: %+v vs %+v", i, decoded[i], rows[0][i])
		}
	}
	if _, err := mpc.DecodeTriples([]byte{1, 2, 3}); err == nil {
		t.Error("a truncated triple file was accepted")
	}
}
