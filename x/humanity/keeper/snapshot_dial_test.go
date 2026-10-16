package keeper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSnapshotDial_OhneSchalterKeinLoopback: der Test-Schalter
// snapshotDialLokal (snapshot.go) ist im Betrieb aus, und dann gilt fuer den
// Snapshot-Abruf und die Checkpoint-Bloecke beim Resync weiter der
// SSRF-Schutz von pinningDialer. Missbrauch: BOOTSTRAP_SNAPSHOT_URL bzw. die
// Leiter-URL zeigt auf Loopback.
func TestSnapshotDial_OhneSchalterKeinLoopback(t *testing.T) {
	alt := snapshotDialLokal.Load()
	snapshotDialLokal.Store(false)
	defer snapshotDialLokal.Store(alt)

	var erreicht int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		erreicht++
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	if _, err := fetchAndValidateSnapshot(srv.URL+"/api/snapshot", "0x0000000000000000000000000000000000000001"); err == nil ||
		!strings.Contains(err.Error(), "private/loopback") {
		t.Fatalf("Snapshot-Abruf von Loopback: %v, erwartet Abweisung durch pinningDialer", err)
	}
	if _, err := fetchAndVerifyBlockFromPeer(srv.URL, 1); err == nil || !strings.Contains(err.Error(), "private/loopback") {
		t.Fatalf("Checkpoint-Block von Loopback: %v, erwartet Abweisung durch pinningDialer", err)
	}
	if _, err := fetchAndVerifySiblingsAtHeight(srv.URL, 1); err == nil || !strings.Contains(err.Error(), "private/loopback") {
		t.Fatalf("Geschwister von Loopback: %v, erwartet Abweisung durch pinningDialer", err)
	}
	if erreicht != 0 {
		t.Fatalf("der Loopback-Server wurde %d-mal erreicht", erreicht)
	}
}
