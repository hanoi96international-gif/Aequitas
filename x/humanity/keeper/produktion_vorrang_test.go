package keeper

import (
	"sync"
	"testing"
	"time"
)

// Der Vorrang ist ein Hinweis, keine Zusicherung -- aber er muss richtig
// zaehlen, sonst ist die Frage "war die Sperre der Engpass?" wieder offen.

func TestVorrang_OhneWartendeKostetNichts(t *testing.T) {
	ProduktionsVorrangZuruecksetzen()
	t.Cleanup(ProduktionsVorrangZuruecksetzen)

	start := time.Now()
	for i := 0; i < 50; i++ {
		syncLaesstProduktionVor()
	}
	// Ohne Wartende darf nicht geschlafen werden: 50 Aufrufe mit je 2 ms
	// waeren 100 ms und wuerden das Aufholen spuerbar bremsen.
	if d := time.Since(start); d > 20*time.Millisecond {
		t.Errorf("50 Aufrufe ohne Wartende brauchten %v -- es wurde geschlafen, obwohl niemand wartet", d)
	}
	s := ProduktionsVorrangStand()
	if got := s["geprueft"].(int64); got != 50 {
		t.Errorf("geprueft = %d, erwartet 50", got)
	}
	if got := s["gewaehrt"].(int64); got != 0 {
		t.Errorf("gewaehrt = %d, erwartet 0 -- es wartete niemand", got)
	}
}

func TestVorrang_MitWartenderWirdPlatzGemacht(t *testing.T) {
	ProduktionsVorrangZuruecksetzen()
	t.Cleanup(ProduktionsVorrangZuruecksetzen)

	fertig := produktionMeldetWarten()
	syncLaesstProduktionVor()
	s := ProduktionsVorrangStand()
	if got := s["gewaehrt"].(int64); got != 1 {
		t.Errorf("gewaehrt = %d, erwartet 1", got)
	}
	if got := s["wartet_grad"].(int32); got != 1 {
		t.Errorf("wartet_grad = %d, erwartet 1", got)
	}
	fertig()
	if got := ProduktionsVorrangStand()["wartet_grad"].(int32); got != 0 {
		t.Errorf("wartet_grad nach fertig() = %d, erwartet 0", got)
	}
}

// Die Abmeldung darf nur EINMAL zaehlen. Wird sie doppelt gerufen -- etwa aus
// einem defer und einem frueheren Rueckgabepfad -- faellt der Zaehler sonst
// unter null, und danach macht der Sync nie wieder Platz.
func TestVorrang_DoppelteAbmeldungZaehltEinmal(t *testing.T) {
	ProduktionsVorrangZuruecksetzen()
	t.Cleanup(ProduktionsVorrangZuruecksetzen)

	fertig := produktionMeldetWarten()
	fertig()
	fertig()
	fertig()
	if got := ProduktionsVorrangStand()["wartet_grad"].(int32); got != 0 {
		t.Fatalf("wartet_grad = %d, erwartet 0 -- ein negativer Stand wuerde den Vorrang dauerhaft abschalten", got)
	}
	// Und der naechste Wartende muss wieder erkannt werden.
	f2 := produktionMeldetWarten()
	defer f2()
	syncLaesstProduktionVor()
	if got := ProduktionsVorrangStand()["gewaehrt"].(int64); got != 1 {
		t.Errorf("gewaehrt = %d, erwartet 1 -- der Vorrang blieb nach der doppelten Abmeldung tot", got)
	}
}

func TestVorrang_MehrereGleichzeitig(t *testing.T) {
	ProduktionsVorrangZuruecksetzen()
	t.Cleanup(ProduktionsVorrangZuruecksetzen)

	var wg sync.WaitGroup
	fertige := make([]func(), 0, 4)
	for i := 0; i < 4; i++ {
		fertige = append(fertige, produktionMeldetWarten())
	}
	if got := ProduktionsVorrangStand()["wartet_grad"].(int32); got != 4 {
		t.Fatalf("wartet_grad = %d, erwartet 4", got)
	}
	for _, f := range fertige {
		wg.Add(1)
		go func(f func()) { defer wg.Done(); f() }(f)
	}
	wg.Wait()
	if got := ProduktionsVorrangStand()["wartet_grad"].(int32); got != 0 {
		t.Errorf("wartet_grad = %d, erwartet 0", got)
	}
}
