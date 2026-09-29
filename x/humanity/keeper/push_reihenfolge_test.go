package keeper

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Je Partner kommt die Arbeit in der Reihenfolge an, in der sie eingereiht
// wurde -- auch wenn einzelne Pushes unterschiedlich lange brauchen.
func TestPushGeordnet_ReihenfolgeJePartner(t *testing.T) {
	var mu sync.Mutex
	var gesehen []int
	fertig := make(chan struct{})
	const n = 40
	for i := 0; i < n; i++ {
		i := i
		pushGeordnet("test://reihenfolge", func() {
			time.Sleep(time.Duration((n-i)%5) * time.Millisecond) // fruehe langsamer
			mu.Lock()
			gesehen = append(gesehen, i)
			if len(gesehen) == n {
				close(fertig)
			}
			mu.Unlock()
		})
	}
	select {
	case <-fertig:
	case <-time.After(10 * time.Second):
		t.Fatal("nicht alle Pushes gelaufen")
	}
	mu.Lock()
	defer mu.Unlock()
	for i, v := range gesehen {
		if v != i {
			t.Fatalf("Position %d: Push %d -- Reihenfolge verletzt: %v", i, v, gesehen)
		}
	}
}

// Haengt ein Partner, blockiert pushGeordnet trotzdem nicht: ueber der
// Schlangentiefe laeuft der Push auf dem alten Weg.
func TestPushGeordnet_VolleSchlangeBlockiertNicht(t *testing.T) {
	halt := make(chan struct{})
	defer close(halt)
	vorher := pushUeberlauf.Load()
	start := time.Now()
	for i := 0; i < pushSchlangeTiefe+5; i++ {
		pushGeordnet("test://haengt", func() { <-halt })
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("pushGeordnet blockierte %v bei haengendem Partner", d)
	}
	if pushUeberlauf.Load()-vorher < 4 {
		t.Fatalf("Ueberlauf %d, erwartet mindestens 4", pushUeberlauf.Load()-vorher)
	}
}

func TestPushFrist_WaechstMitDerGroesse(t *testing.T) {
	for _, c := range []struct {
		bytes int
		frist time.Duration
	}{
		{0, 3 * time.Second},
		{1000, 4 * time.Second},
		{5 << 20, 8 * time.Second},
		{100 << 20, 15 * time.Second},
	} {
		if got := pushFrist(c.bytes); got != c.frist {
			t.Errorf("pushFrist(%d) = %v, erwartet %v", c.bytes, got, c.frist)
		}
	}
}

// Ein Partner, der zu langsam antwortet, ist kein Partner ohne gzip: die
// Zeitueberschreitung wird als solche gemeldet und die Faehigkeit bleibt.
func TestPushBlockOnce_ZeitueberschreitungIstKeinGzipFehler(t *testing.T) {
	langsam := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer langsam.Close()
	recordGzipPushCapability(langsam.URL, true)
	defer gzipPushPeerCap.Delete(langsam.URL)

	// Der Test-Server lauscht auf 127.0.0.1, den httpSyncClient (Schutz vor
	// SSRF) zu Recht ablehnt. Nur fuer diesen Test ein gewoehnlicher Client.
	echt := httpSyncClient
	httpSyncClient = &http.Client{}
	defer func() { httpSyncClient = echt }()

	dag := &BlockDAG{}
	start := time.Now()
	_, ok, zeitUeber := dag.pushBlockOnce(&Block{Height: 1}, langsam.URL, []byte("{}"), false, true)
	if ok || !zeitUeber {
		t.Fatalf("ok=%v zeitUeber=%v, erwartet eine gemeldete Zeitueberschreitung", ok, zeitUeber)
	}
	if d := time.Since(start); d > 4500*time.Millisecond {
		t.Fatalf("Frist fuer 2 Bytes nicht eingehalten: %v", d)
	}
	if !gzipPushPeerSupports(langsam.URL) {
		t.Fatal("Zeitueberschreitung hat die gzip-Faehigkeit geloescht")
	}
}
