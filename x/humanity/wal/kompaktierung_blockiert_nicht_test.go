package wal

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// DIE KOMPAKTIERUNG DARF DIE ANNAHME NICHT ANHALTEN.
//
// Die erste Fassung von TruncateBefore hielt w.mu, waehrend sie die ganze
// Datei umschrieb -- bei 512 MB Sekunden, in denen kein Append durchkam und
// damit keine Ueberweisung des Knotens. Dieser Test baut eine grosse Datei,
// kompaktiert sie und haelt waehrenddessen Appends am Laufen. Zwei Dinge
// muessen gelten: kein Append wartet laenger als einen Bruchteil der
// Kompaktierung, und JEDER Append ueberlebt sie -- auch die, die genau
// waehrend des Kopierens ankamen.
func TestTruncateBefore_BlockiertAppendsNichtUndVerliertKeinen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal.log")
	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// Genug Daten, dass die Kopie messbar dauert: 20.000 zu 2 KB sind rund
	// 40 MB. Nebenlaeufig angehaengt, damit der Gruppen-Commit die fsyncs
	// buendelt -- sequenziell waeren es 20.000 einzelne, und auf einer
	// Entwicklerplatte dauert das laenger als die Testzeit.
	nutzlast := make([]byte, 2048)
	const vorher = 20000
	var fuellen sync.WaitGroup
	for g := 0; g < 64; g++ {
		fuellen.Add(1)
		go func() {
			defer fuellen.Done()
			for i := 0; i < vorher/64; i++ {
				if _, err := w.Append(nutzlast); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	fuellen.Wait()
	const schwelle = vorher / 2 // die erste Haelfte faellt weg

	// Appends waehrend der Kompaktierung: so lange, bis sie fertig ist.
	var (
		fertig      atomic.Bool
		waehrend    atomic.Int64
		laengste    atomic.Int64 // Nanosekunden
		ueber100ms  atomic.Int64
		ueber10ms   atomic.Int64
		wg          sync.WaitGroup
		seqWaehrend sync.Map
	)
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !fertig.Load() {
				start := time.Now()
				seq, err := w.Append([]byte(fmt.Sprintf("waehrend-%d", waehrend.Add(1))))
				d := int64(time.Since(start))
				if err != nil {
					t.Errorf("Append waehrend der Kompaktierung: %v", err)
					return
				}
				seqWaehrend.Store(seq, struct{}{})
				if d > int64(100*time.Millisecond) {
					ueber100ms.Add(1)
				} else if d > int64(10*time.Millisecond) {
					ueber10ms.Add(1)
				}
				for {
					cur := laengste.Load()
					if d <= cur || laengste.CompareAndSwap(cur, d) {
						break
					}
				}
			}
		}()
	}

	kStart := time.Now()
	if err := w.TruncateBefore(schwelle); err != nil {
		t.Fatal(err)
	}
	kDauer := time.Since(kStart)
	fertig.Store(true)
	wg.Wait()

	n := waehrend.Load()
	if n < 10 {
		t.Fatalf("nur %d Appends waehrend der Kompaktierung -- der Test hat nichts gemessen", n)
	}
	// Die Kernaussage: kein Append hat annaehernd so lange gewartet wie die
	// Kompaktierung gedauert hat. Mit der alten Fassung waere der laengste
	// Append ungefaehr gleich der Kompaktierungsdauer gewesen.
	if l := time.Duration(laengste.Load()); l > kDauer/2 && l > 200*time.Millisecond {
		t.Errorf("laengster Append waehrend der Kompaktierung: %s bei %s Kompaktierung -- "+
			"die Appends warten wieder auf die ganze Umschreibung", l, kDauer)
	}
	t.Logf("Kompaktierung %s, %d Appends waehrend, laengster %s | >100ms: %d, 10-100ms: %d",
		kDauer, n, time.Duration(laengste.Load()), ueber100ms.Load(), ueber10ms.Load())
	KompaktierungsPhasen.mu.Lock()
	t.Logf("Phasen: Kopie %s (ohne Sperre) | Nachholen+Tausch %s (unter Sperre, %d Bytes nachgeholt)",
		KompaktierungsPhasen.Kopie, KompaktierungsPhasen.Nachholen, KompaktierungsPhasen.NachgeholtBytes)
	KompaktierungsPhasen.mu.Unlock()

	// Und alle ueberleben: die zweite Haelfte der alten plus alle neuen.
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	gesehen := map[uint64]bool{}
	var alt, neu int
	if _, _, err := ReplayFile(path, func(e Entry) error {
		if gesehen[e.Seq] {
			return fmt.Errorf("Seq %d doppelt", e.Seq)
		}
		gesehen[e.Seq] = true
		if e.Seq < schwelle {
			return fmt.Errorf("Seq %d unter der Schwelle %d hat die Kompaktierung ueberlebt", e.Seq, schwelle)
		}
		if _, ok := seqWaehrend.Load(e.Seq); ok {
			neu++
		} else {
			alt++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	seqWaehrend.Range(func(k, _ interface{}) bool {
		if !gesehen[k.(uint64)] {
			t.Errorf("Append mit Seq %d waehrend der Kompaktierung ist VERLOREN", k.(uint64))
		}
		return true
	})
	if int64(neu) != n {
		t.Errorf("%d Appends waehrend der Kompaktierung, %d davon in der Datei", n, neu)
	}
	t.Logf("ueberlebt: %d alte, %d waehrend der Kompaktierung angehaengte", alt, neu)
}
