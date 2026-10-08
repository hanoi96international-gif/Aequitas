package keeper

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Missbrauch (Pruefung #314, LOW-3, und von #319): die Registerabfrage der
// Erneuerung wartet hoechstens 5 s auf eine Verbindung -- eine ausgeschoepfte
// Datenbank haelt keine oeffentliche Anfrage unbegrenzt fest. Fail-closed:
// ohne Antwort keine Bindung.
func TestCoordinatorBindungLokal_Zeitgrenze_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	pubHex := hex.EncodeToString(f.pub)
	if _, ok := f.cs.CoordinatorBindungLokal(pubHex); !ok {
		t.Fatal("Vorbedingung: der Schluessel ist eingetragen")
	}
	vorher := f.cs.db.Stats().MaxOpenConnections
	f.cs.db.SetMaxOpenConns(1)
	t.Cleanup(func() { f.cs.db.SetMaxOpenConns(vorher) })
	belegt, err := f.cs.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer belegt.Close()
	fertig := make(chan bool, 1)
	go func() {
		_, ok := f.cs.CoordinatorBindungLokal(pubHex)
		fertig <- ok
	}()
	select {
	case ok := <-fertig:
		if ok {
			t.Fatal("Bindung gelesen, obwohl keine Verbindung frei war")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("die Registerabfrage wartet ohne Zeitgrenze auf eine Verbindung")
	}
}

// Die Registerabfrage bricht mit der Anfrage ab, nicht erst nach ihrer
// eigenen Zeitgrenze (5 s). Und ein Fehler beim Lesen ist kein "nicht
// eingetragen": der Handler lehnt ab (fail-closed), sagt aber "wiederholen"
// (503) statt "neu eintragen" (403) -- Pruefung von #319, INFO-8 und LOW-9.
func TestErneuerung_RegisterNichtLesbar_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	pubHex := hex.EncodeToString(f.pub)
	if _, ok, err := f.cs.coordinatorBindungLesen(context.Background(), pubHex); !ok || err != nil {
		t.Fatalf("Vorbedingung: der Schluessel ist eingetragen (%v)", err)
	}
	if _, ok, err := f.cs.coordinatorBindungLesen(context.Background(), strings.Repeat("ab", 32)); ok || err != nil {
		t.Fatalf("unbekannter Schluessel: ok=%v err=%v, erwartet nicht eingetragen ohne Fehler", ok, err)
	}
	vorher := f.cs.db.Stats().MaxOpenConnections
	f.cs.db.SetMaxOpenConns(1)
	t.Cleanup(func() { f.cs.db.SetMaxOpenConns(vorher) })
	belegt, err := f.cs.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer belegt.Close()

	ctx, abbruch := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer abbruch()
	start := time.Now()
	_, ok, err := f.cs.coordinatorBindungLesen(ctx, pubHex)
	if ok || err == nil {
		t.Fatalf("ohne freie Verbindung: ok=%v err=%v, erwartet einen Fehler", ok, err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("die Abfrage wartete %s -- sie bricht nicht mit der Anfrage ab", d)
	}

	imAusgang := func() int {
		var n int
		if err := belegt.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM pending_txs WHERE included_at = 0 AND tx_json LIKE '%liveness_renewal%'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	vorAusgang := imAusgang()
	issued := nowUnix()
	body, _ := json.Marshal(map[string]interface{}{"wallet": f.wallet, "issued_at": issued,
		"public_key": pubHex, "signature": f.unterschreibe(f.priv, f.wallet, issued)})
	ctx2, abbruch2 := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer abbruch2()
	w := httptest.NewRecorder()
	(&APIServer{state: f.cs}).handleLivenessRenewal(w,
		httptest.NewRequest(http.MethodPost, "/api/liveness-renewal", bytes.NewReader(body)).WithContext(ctx2))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "retry") ||
		strings.Contains(w.Body.String(), "register the coordinator key again") {
		t.Fatalf("Register nicht lesbar: %d %s, erwartet 503 mit \"retry\"", w.Code, w.Body.String())
	}
	if n := imAusgang(); n != vorAusgang {
		t.Fatalf("%d Erneuerungen im Ausgang (vorher %d), obwohl das Register nicht lesbar war", n, vorAusgang)
	}
}
