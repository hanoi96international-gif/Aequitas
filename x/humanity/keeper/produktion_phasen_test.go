package keeper

import (
	"testing"
	"time"
)

// Ein Instrument, das nur die erwarteten Ergebnisse beschreiben kann, ist
// keines. Diese Tests pruefen genau die Eigenschaft, auf die sich die
// naechste Entscheidung stuetzt: dass die benannten Phasen plus `rest` die
// gemessene Gesamtdauer ergeben, und dass ein grosses `rest` auch als solches
// herauskommt statt still in eine benannte Phase zu wandern.

func TestProduktionsPhasen_PhasenPlusRestErgebenDieDauer(t *testing.T) {
	ProduktionsPhasenZuruecksetzen()
	t.Cleanup(ProduktionsPhasenZuruecksetzen)
	ms := time.Millisecond

	// 1000 ms gesamt, davon 300 benannt. Der Rest MUSS als rest erscheinen --
	// das ist die Aussage, wegen der es die Uhr gibt.
	merkeProduktionsBlock(1000*ms, 50*ms, 100*ms, 40*ms, 90*ms, 20*ms, 7000)

	s := ProduktionsPhasenStand()
	if got := s["gesamt_ms"].(float64); got != 1000 {
		t.Fatalf("gesamt_ms = %v, erwartet 1000", got)
	}
	if got := s["rest_ms"].(float64); got < 699.9 || got > 700.1 {
		t.Errorf("rest_ms = %v, erwartet 700 (1000 minus die 300 benannten)", got)
	}
	if got := s["schlimmster_tx"].(int64); got != 7000 {
		t.Errorf("schlimmster_tx = %d, erwartet 7000 -- ohne die Zahl ist unklar, ob der Block ungewoehnlich gross war", got)
	}
}

func TestProduktionsPhasen_MittelJeBlockNichtSumme(t *testing.T) {
	ProduktionsPhasenZuruecksetzen()
	t.Cleanup(ProduktionsPhasenZuruecksetzen)
	ms := time.Millisecond

	// Vier Bloecke zu je 400 ms. gesamt_ms muss 400 melden, nicht 1600 --
	// sonst ist die Zahl nicht mit BLOCK_TIME vergleichbar, und genau dieser
	// Vergleich ist der Zweck.
	for i := 0; i < 4; i++ {
		merkeProduktionsBlock(400*ms, 10*ms, 20*ms, 10*ms, 40*ms, 20*ms, 100)
	}
	s := ProduktionsPhasenStand()
	if got := s["gesamt_ms"].(float64); got != 400 {
		t.Fatalf("gesamt_ms = %v, erwartet 400 (Mittel je Block)", got)
	}
	if got := s["speichern_ms"].(float64); got != 40 {
		t.Errorf("speichern_ms = %v, erwartet 40", got)
	}
	if got := s["bloecke"].(int64); got != 4 {
		t.Errorf("bloecke = %d, erwartet 4", got)
	}
}

// Der teuerste Bau muss vollstaendig erhalten bleiben: der Mittelwert
// verschluckt genau den Ausreisser, der die Produktion aus dem Takt wirft
// (gemessen 07.09.2026: Mittel unter 1 s, schlimmster 4,6 s).
func TestProduktionsPhasen_TeuersterBauUeberlebtBillige(t *testing.T) {
	ProduktionsPhasenZuruecksetzen()
	t.Cleanup(ProduktionsPhasenZuruecksetzen)
	ms := time.Millisecond

	merkeProduktionsBlock(200*ms, 10*ms, 10*ms, 10*ms, 10*ms, 10*ms, 50)
	merkeProduktionsBlock(4600*ms, 3000*ms, 200*ms, 100*ms, 900*ms, 50*ms, 7000)
	merkeProduktionsBlock(180*ms, 5*ms, 5*ms, 5*ms, 5*ms, 5*ms, 40)

	s := ProduktionsPhasenStand()
	if got := s["schlimmster_ms"].(float64); got != 4600 {
		t.Fatalf("schlimmster_ms = %v, erwartet 4600 (nicht der letzte, der teuerste)", got)
	}
	if got := s["schlimmster_sperren"].(float64); got != 3000 {
		t.Errorf("schlimmster_sperren = %v, erwartet 3000 -- die Phase, die die Sekunden verbraucht hat", got)
	}
	// 4600 - (3000+200+100+900+50) = 350
	if got := s["schlimmster_rest"].(float64); got < 349 || got > 351 {
		t.Errorf("schlimmster_rest = %v, erwartet 350", got)
	}
}

func TestProduktionsPhasen_LeerTeiltNichtDurchNull(t *testing.T) {
	ProduktionsPhasenZuruecksetzen()
	t.Cleanup(ProduktionsPhasenZuruecksetzen)
	s := ProduktionsPhasenStand() // darf nicht panicken
	for _, k := range []string{"gesamt_ms", "sperren_ms", "rest_ms", "schlimmster_ms"} {
		if got := s[k].(float64); got != 0 {
			t.Errorf("%s = %v auf leeren Zaehlern, erwartet 0", k, got)
		}
	}
}
