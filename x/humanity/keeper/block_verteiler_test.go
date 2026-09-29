package keeper

import (
	"sync"
	"testing"
	"time"
)

// Jeder Block genau einmal, in Produktionsreihenfolge -- auch ueber mehrere
// Takte hinweg und wenn die Verteilung langsamer ist als der Bau.
func TestBlockVerteiler_ReihenfolgeUndGenauEinmal(t *testing.T) {
	var mu sync.Mutex
	var gesehen []int64
	fertig := make(chan struct{})
	const n = 3 * blockVerteilerTiefe
	v := NeuerBlockVerteiler(func(b *Block) {
		time.Sleep(200 * time.Microsecond) // langsamer als Verteile
		mu.Lock()
		gesehen = append(gesehen, b.Height)
		if len(gesehen) == n {
			close(fertig)
		}
		mu.Unlock()
	})
	for h := int64(0); h < n; h += 5 {
		var takt []*Block
		for i := h; i < h+5 && i < n; i++ {
			takt = append(takt, &Block{Height: i})
		}
		v.Verteile(takt)
	}
	select {
	case <-fertig:
	case <-time.After(10 * time.Second):
		t.Fatal("nicht alle Bloecke verteilt")
	}
	mu.Lock()
	defer mu.Unlock()
	for i, h := range gesehen {
		if h != int64(i) {
			t.Fatalf("Position %d: Block %d -- Reihenfolge verletzt (%v)", i, h, gesehen)
		}
	}
	if v.verteilt.Load() != n {
		t.Fatalf("verteilt %d, erwartet %d", v.verteilt.Load(), n)
	}
}

// Der Takt kehrt sofort zurueck, solange Platz ist -- genau das ist der
// Gewinn. Ist die Schlange voll, wartet er (Rueckstau statt Aufstauen).
func TestBlockVerteiler_TaktWartetNurWennVoll(t *testing.T) {
	halt := make(chan struct{})
	v := NeuerBlockVerteiler(func(b *Block) { <-halt })
	defer close(halt)

	// Einer steckt im Senden, blockVerteilerTiefe passen in die Schlange.
	start := time.Now()
	for i := 0; i <= blockVerteilerTiefe; i++ {
		v.Verteile([]*Block{{Height: int64(i)}})
		if i == 0 {
			time.Sleep(20 * time.Millisecond) // den ersten aus der Schlange holen lassen
		}
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("Verteile blockierte %v, obwohl Platz war", d)
	}
	if v.gewartet.Load() != 0 {
		t.Fatalf("gewartet %d, obwohl Platz war", v.gewartet.Load())
	}

	// Jetzt ist sie voll: der naechste Takt muss warten.
	zurueck := make(chan struct{})
	go func() {
		v.Verteile([]*Block{{Height: 999}})
		close(zurueck)
	}()
	select {
	case <-zurueck:
		t.Fatal("Verteile kehrte bei voller Schlange zurueck -- Bloecke wuerden sich unbegrenzt stauen")
	case <-time.After(100 * time.Millisecond):
	}
	halt <- struct{}{} // einen fertig senden lassen
	select {
	case <-zurueck:
	case <-time.After(5 * time.Second):
		t.Fatal("Verteile kam nach freiem Platz nicht zurueck")
	}
	if v.gewartet.Load() != 1 {
		t.Fatalf("gewartet %d, erwartet 1", v.gewartet.Load())
	}
}

// Ein Panik im Senden (etwa ein kaputter Peer) darf die Verteilung nicht
// beenden -- sonst staut sich jeder folgende Block, und der Takt steht.
func TestBlockVerteiler_UeberlebtPanikImSenden(t *testing.T) {
	var mu sync.Mutex
	var gesehen []int64
	v := NeuerBlockVerteiler(func(b *Block) {
		if b.Height == 1 {
			panic("peer kaputt")
		}
		mu.Lock()
		gesehen = append(gesehen, b.Height)
		mu.Unlock()
	})
	v.Verteile([]*Block{{Height: 0}, {Height: 1}, {Height: 2}})
	frist := time.Now().Add(5 * time.Second)
	for time.Now().Before(frist) {
		mu.Lock()
		k := len(gesehen)
		mu.Unlock()
		if k == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(gesehen) != 2 || gesehen[0] != 0 || gesehen[1] != 2 {
		t.Fatalf("nach Panik: %v, erwartet [0 2]", gesehen)
	}
}
