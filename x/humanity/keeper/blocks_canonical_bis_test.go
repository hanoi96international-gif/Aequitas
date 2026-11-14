package keeper

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Explorer blaettert durch die ganze Kette: /api/blocks/canonical?bis=H.
// Unsinn wird abgewiesen, statt still die neuesten zu liefern.
func TestCanonicalBlocks_BisGeprueft(t *testing.T) {
	a := &APIServer{state: &ChainState{}, blockchain: newOrphanTestDAG()}
	for wert, soll := range map[string]int{"0": 400, "-5": 400, "abc": 400, "1e9": 400, "": 200, "10": 200} {
		url := "/api/blocks/canonical?limit=5"
		if wert != "" {
			url += "&bis=" + wert
		}
		w := httptest.NewRecorder()
		a.handleCanonicalBlocks(w, httptest.NewRequest(http.MethodGet, url, nil))
		if w.Code != soll {
			t.Errorf("bis=%q: %d statt %d", wert, w.Code, soll)
		}
	}
}
