package keeper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// MISSBRAUCH: ohne erwarteten Unterzeichner wird kein Snapshot geholt und
// nichts eingemischt -- auch nicht, wenn ein Aufrufer den Wert vergisst
// (main.go prueft BOOTSTRAP_SIGNER schon selbst).
func TestImportSnapshotFromURL_OhneUnterzeichnerNichts(t *testing.T) {
	var abgerufen atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		abgerufen.Add(1)
		w.Write([]byte(`{"height":1,"accounts":[]}`))
	}))
	defer srv.Close()
	cs := &ChainState{}
	for _, leer := range []string{"", "   "} {
		err := cs.ImportSnapshotFromURL(srv.URL+"/api/snapshot", leer)
		if err == nil || !strings.Contains(err.Error(), "BOOTSTRAP_SIGNER") {
			t.Fatalf("Unterzeichner %q: err=%v, erwartet Abweisung", leer, err)
		}
	}
	if n := abgerufen.Load(); n != 0 {
		t.Fatalf("Snapshot %d-mal abgerufen, obwohl kein Unterzeichner gesetzt war", n)
	}
}
