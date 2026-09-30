package keeper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Audit 2026-09-29, H2: ein Hostname, der auf eine interne Adresse aufloest,
// besteht isAllowedPeerURL -- der Abruf selbst muss ihn abweisen.
func TestFremdKlient_HostnameAufLoopbackAbgewiesen(t *testing.T) {
	if !isAllowedPeerURL("https://localhost:1") {
		t.Skip("isAllowedPeerURL weist localhost bereits als Zeichenkette ab")
	}
	_, _, err := holeBezeugungsnachweis("https://localhost:1", "0x"+strings.Repeat("a", 40))
	if err == nil || !strings.Contains(err.Error(), "private/loopback") {
		t.Fatalf("matching_url auf localhost muss vom Dialer abgewiesen werden, Fehler: %v", err)
	}
	_, _, err = holeBesitznachweis("https://localhost:1", "0x"+strings.Repeat("a", 40))
	if err == nil || !strings.Contains(err.Error(), "private/loopback") {
		t.Fatalf("coordinator_url auf localhost muss vom Dialer abgewiesen werden, Fehler: %v", err)
	}
}

// Audit 2026-09-29, H1: der Knoten bestaetigt seinen Signierschluessel nur
// fuer die Wallet seines eigenen Betreibers.
func TestSelfProof_NurFuerDenEigenenBetreiber(t *testing.T) {
	betreiber := "0x" + strings.Repeat("b", 40)
	fremd := "0x" + strings.Repeat("c", 40)
	a := &APIServer{blockchain: &BlockDAG{}}

	t.Setenv("NODE_OPERATOR_WALLET", "")
	rec := httptest.NewRecorder()
	a.handleValidatorSelfProof(rec, httptest.NewRequest(http.MethodGet, "/api/validator-selfproof?wallet="+betreiber, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("ohne NODE_OPERATOR_WALLET: erwartet 409, bekam %d", rec.Code)
	}

	t.Setenv("NODE_OPERATOR_WALLET", strings.ToUpper(betreiber[:2])+betreiber[2:])
	rec = httptest.NewRecorder()
	a.handleValidatorSelfProof(rec, httptest.NewRequest(http.MethodGet, "/api/validator-selfproof?wallet="+fremd, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("fremde Wallet: erwartet 403, bekam %d (%s)", rec.Code, rec.Body.String())
	}

	// Die eigene Wallet kommt an der Sperre vorbei (hier ohne Schluessel: 503).
	rec = httptest.NewRecorder()
	a.handleValidatorSelfProof(rec, httptest.NewRequest(http.MethodGet, "/api/validator-selfproof?wallet="+betreiber, nil))
	if rec.Code == http.StatusForbidden || rec.Code == http.StatusConflict {
		t.Fatalf("eigene Wallet darf nicht abgewiesen werden, bekam %d", rec.Code)
	}
}
