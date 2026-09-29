package keeper

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Je Partner starten die Pushes in Reihenfolge -- hoechstens ein Fenster
// daneben -- und nie mehr als pushFenster zugleich.
func TestPushGeordnet_StartReihenfolgeUndFenster(t *testing.T) {
	var mu sync.Mutex
	var gestartet []int
	var laufend, hoechst int
	var wg sync.WaitGroup
	const n = 40
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		pushGeordnet("test://reihenfolge", func() {
			defer wg.Done()
			mu.Lock()
			gestartet = append(gestartet, i)
			laufend++
			if laufend > hoechst {
				hoechst = laufend
			}
			mu.Unlock()
			time.Sleep(time.Duration(1+(n-i)%5) * time.Millisecond)
			mu.Lock()
			laufend--
			mu.Unlock()
		})
	}
	fertig := make(chan struct{})
	go func() { wg.Wait(); close(fertig) }()
	select {
	case <-fertig:
	case <-time.After(10 * time.Second):
		t.Fatal("nicht alle Pushes gelaufen")
	}
	mu.Lock()
	defer mu.Unlock()
	// Garantiert ist: ein Push startet erst, wenn alle mehr als ein Fenster
	// frueheren fertig sind. Innerhalb des Fensters entscheidet der
	// Scheduler; ein Kind, das dadurch vor seinem Elternteil ankommt, legt
	// der Partner nach dem Elternblock sofort wieder vor.
	if len(gestartet) != n {
		t.Fatalf("%d von %d gestartet", len(gestartet), n)
	}
	for pos, v := range gestartet {
		if d := pos - v; d >= pushFenster || -d >= pushFenster {
			t.Fatalf("Push %d an Position %d gestartet -- mehr als ein Fenster (%d) daneben: %v", v, pos, pushFenster, gestartet)
		}
	}
	if hoechst > pushFenster {
		t.Fatalf("%d Pushes zugleich, Fenster %d", hoechst, pushFenster)
	}
	if hoechst < 2 {
		t.Fatalf("hoechstens %d zugleich -- das Fenster wird nicht genutzt", hoechst)
	}
}

// Haengt ein Partner, blockiert pushGeordnet trotzdem nicht: ueber der
// Schlangentiefe laeuft der Push auf dem alten Weg.
func TestPushGeordnet_VolleSchlangeBlockiertNicht(t *testing.T) {
	halt := make(chan struct{})
	defer close(halt)
	// Ein eigener Partner je Lauf: sonst zaehlen die Schlangen frueherer Laeufe
	// (-count) mit.
	partner := fmt.Sprintf("test://haengt-%d", time.Now().UnixNano())
	// Aufnahmefaehig sind die Schlange, die pushFenster laufenden Pushes und
	// der eine, den der Verteiler schon herausgenommen hat und der auf einen
	// Platz wartet. Alles darueber MUSS ueberlaufen -- wie viel genau, haengt
	// nur davon ab, wie schnell der Verteiler herausnimmt.
	fassung := pushSchlangeTiefe + pushFenster + 1
	vorher := pushUeberlauf.Load()
	start := time.Now()
	for i := 0; i < fassung+5; i++ {
		pushGeordnet(partner, func() { <-halt })
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("pushGeordnet blockierte %v bei haengendem Partner", d)
	}
	if n := pushUeberlauf.Load() - vorher; n < 5 {
		t.Fatalf("Ueberlauf %d, erwartet mindestens 5 (Fassung %d, eingereiht %d)", n, fassung, fassung+5)
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
