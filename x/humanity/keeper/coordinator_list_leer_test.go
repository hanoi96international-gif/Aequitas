package keeper

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Ein leeres Register ist eine leere Liste, nicht null -- sonst bricht der
// Vergleichsdienst beim Lesen ab (26.09.2026, neuer C1).
func TestCoordinatorList_LeeresRegisterIstLeereListe(t *testing.T) {
	a := &APIServer{state: newTestState()}
	w := httptest.NewRecorder()
	a.handleCoordinatorList(w, httptest.NewRequest("GET", "/api/coordinators", nil))
	body := w.Body.String()
	if !strings.Contains(body, `"coordinators":[]`) {
		t.Fatalf("leeres Register muss [] liefern, bekam %s", body)
	}
}
