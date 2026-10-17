package keeper

import (
	"bytes"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Pruefung #337, NIEDRIG: isFinalityViolation laeuft im Sync auf einem Block
// eines Peers, unter dag.mu (lesend). Eine Panik darin liess die Lesesperre
// liegen -- jeder spaetere dag.mu.Lock() stuende. Jetzt mit defer.
func TestSync_PanikLaesstDieDagSperreNichtLiegen(t *testing.T) {
	dag := &BlockDAG{blocks: map[string]*Block{}} // ohne state: GetFinalizedCheckpoint paniert
	block := &Block{Hash: "peer-block", Height: 5}

	paniert := false
	func() {
		defer func() { paniert = recover() != nil }()
		dag.bekanntOderUnterFinalitaet(block)
	}()
	if !paniert {
		t.Fatal("Testaufbau: isFinalityViolation sollte hier panieren")
	}
	frei := make(chan struct{})
	go func() {
		dag.mu.Lock()
		dag.mu.Unlock()
		close(frei)
	}()
	select {
	case <-frei:
	case <-time.After(2 * time.Second):
		t.Fatal("dag.mu bleibt nach der Panik gesperrt -- der Knoten stuende")
	}
}

func TestSync_BekanntOderUnterFinalitaet_Gutfall(t *testing.T) {
	dag := &BlockDAG{blocks: map[string]*Block{"da": {Hash: "da"}}}
	if bekannt, unter := dag.bekanntOderUnterFinalitaet(&Block{Hash: "da"}); !bekannt || unter {
		t.Fatalf("bekannter Block: %v %v", bekannt, unter)
	}
	// Vor dem FromSync-Stichtag ist ein Sync-Block nie eine
	// Finalitaetsverletzung -- und braucht keinen state. Ausdruecklich ohne
	// Stichtag geprueft, nicht abhaengig vom Datum des Testlaufs.
	fromSyncStichtagSetzen(t, math.MaxInt64)
	if bekannt, unter := dag.bekanntOderUnterFinalitaet(&Block{Hash: "neu", FromSync: true}); bekannt || unter {
		t.Fatalf("neuer Sync-Block: %v %v", bekannt, unter)
	}
	// Ab dem Stichtag gilt die Finalitaet auch fuer ihn.
	cs := newTestState()
	cs.SetFinalizedCheckpoint("deadbeef", 1000, 5000)
	dag.state = cs
	fromSyncStichtagSetzen(t, nowUnix()-3600)
	if bekannt, unter := dag.bekanntOderUnterFinalitaet(&Block{Hash: "neu", Height: 10, FromSync: true}); bekannt || !unter {
		t.Fatalf("Sync-Block unter der Finalitaet ab dem Stichtag: %v %v, want false true", bekannt, unter)
	}
}

// Wer je HTTP-Anfrage eine Panik ausloest, flutet das Log nicht: ein Stack,
// festes Label (der Pfad kommt vom Anrufer), Pfad gekuerzt in der Meldung.
func TestHTTP_PanikMeldungGedrosselt(t *testing.T) {
	var aus bytes.Buffer
	altAus, altAbstand := panikAusgabe, panikStackAbstand
	panikZaehlerMu.Lock()
	altZaehler := panikZaehler
	panikZaehler = map[string]*panikStand{}
	panikZaehlerMu.Unlock()
	panikAusgabe, panikStackAbstand = &aus, time.Hour
	t.Cleanup(func() {
		panikAusgabe, panikStackAbstand = altAus, altAbstand
		panikZaehlerMu.Lock()
		panikZaehler = altZaehler
		panikZaehlerMu.Unlock()
	})

	h := recoverMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boese") }))
	for i := 0; i < 100; i++ {
		pfad := "/api/x" + strings.Repeat("a", i*3)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", pfad, nil))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("Status %d", rec.Code)
		}
	}
	if n := stacks(aus.String()); n != 1 {
		t.Fatalf("%d Stacks fuer 100 Paniken, erwartet 1", n)
	}
	if n := strings.Count(aus.String(), "[PANIC RECOVERED]"); n != 1+panikZeilenJeAbstand {
		t.Fatalf("%d Meldungen, erwartet %d", n, 1+panikZeilenJeAbstand)
	}
	// Eine spaetere Panik an einem anderen Pfad bleibt sichtbar (Methode und
	// Pfad), auch ohne Stack -- sie wird nicht unter der ersten verdeckt.
	if !strings.Contains(aus.String(), `GET "/api/x`+strings.Repeat("a", 30)+`"`) {
		t.Fatal("spaetere Panik nicht sichtbar")
	}
	stand := PanikStand()
	if stand["HTTP-Handler"] != 100 || len(stand) != 1 {
		t.Fatalf("Labels: %v", stand)
	}
	// Ein langer Pfad landet gekuerzt im Log.
	panikStackAbstand = 0
	aus.Reset()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/"+strings.Repeat("z", 5000), nil))
	if zeile := firstLine(aus.String()); len(zeile) > 400 {
		t.Fatalf("Meldung %d Zeichen lang", len(zeile))
	}
}
