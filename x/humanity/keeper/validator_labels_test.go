package keeper

import (
	"net/http/httptest"
	"testing"
)

// TestGetValidatorOrdinals_NilDBReturnsNil verifies the safe no-op path for
// a ChainState with no DB (e.g. every unit test's newTestState()) — must
// return nil, not panic, so handleValidatorLabels' own nil-handling has
// something well-defined to fall back to.
func TestGetValidatorOrdinals_NilDBReturnsNil(t *testing.T) {
	cs := &ChainState{}
	if got := cs.GetValidatorOrdinals(); got != nil {
		t.Fatalf("GetValidatorOrdinals() with a nil db = %v, want nil", got)
	}
}

// TestHandleValidatorLabels_NoBlockchainReturnsEmptyLabels verifies the
// handler degrades gracefully (valid JSON, empty labels object) rather
// than panicking when a.blockchain is nil — the explorer's fetch for this
// endpoint must never itself be the reason a page fails to render.
func TestHandleValidatorLabels_NoBlockchainReturnsEmptyLabels(t *testing.T) {
	a := &APIServer{}
	req := httptest.NewRequest("GET", "/api/validator-labels", nil)
	rec := httptest.NewRecorder()

	a.handleValidatorLabels(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	// operators (Signieradresse -> Betreiber-Wallet) kam am 26.09.2026 dazu;
	// ohne Blockchain ebenfalls leer, nie null -- der Explorer liest beide.
	if body != `{"labels":{},"operators":{}}`+"\n" {
		t.Fatalf("body = %q, want empty labels and operators objects", body)
	}
}

// GetValidatorOperators ohne Datenbank: leere Zuordnung, kein nil und keine
// Panik -- der Explorer-Endpunkt ruft es bei jedem Abruf.
func TestGetValidatorOperators_NilDBReturnsEmpty(t *testing.T) {
	cs := &ChainState{}
	got := cs.GetValidatorOperators()
	if got == nil || len(got) != 0 {
		t.Fatalf("GetValidatorOperators() ohne DB = %v, erwartet leere Zuordnung", got)
	}
}
