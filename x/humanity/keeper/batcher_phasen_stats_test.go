package keeper

import (
	"testing"
	"time"
)

// Diese Uhren existieren, um EINE Frage zu entscheiden: bindet der Deckel
// parallelBatchPoolSize, oder ist die Charge selbst teuer? Die Tests pruefen
// deshalb genau, dass warten und arbeit getrennt bleiben und dass
// je_ueberweisung_ms auf die Postenzahl bezogen ist -- die Zahl, die gegen
// den Schnellpfad zu halten ist.

func TestBatcherPhasen_WartenUndArbeitBleibenGetrennt(t *testing.T) {
	BatcherPhasenZuruecksetzen()
	t.Cleanup(BatcherPhasenZuruecksetzen)

	// Zwei Chargen: eine wartet lange und arbeitet kurz, die andere umgekehrt.
	// Beide Summen muessen sich unabhaengig wiederfinden -- vermischten sie
	// sich, waere die Auskunft "der Deckel bindet" nicht mehr zu belegen.
	merkeBatcherSammeln(1*time.Millisecond, 100)
	merkeBatcherWarten(300 * time.Millisecond)
	merkeBatcherArbeit(20*time.Millisecond, false)

	merkeBatcherSammeln(1*time.Millisecond, 100)
	merkeBatcherWarten(20 * time.Millisecond)
	merkeBatcherArbeit(300*time.Millisecond, true)

	s := BatcherPhasenStand()
	if got := s["warten_ms"].(float64); got != 160 {
		t.Errorf("warten_ms = %v, erwartet 160 (Mittel aus 300 und 20)", got)
	}
	if got := s["arbeit_ms"].(float64); got != 160 {
		t.Errorf("arbeit_ms = %v, erwartet 160", got)
	}
	if got := s["warten_max_ms"].(float64); got != 300 {
		t.Errorf("warten_max_ms = %v, erwartet 300", got)
	}
	if got := s["serialisiert"].(int64); got != 1 {
		t.Errorf("serialisiert = %v, erwartet 1", got)
	}
	if got := s["chargen"].(int64); got != 2 {
		t.Errorf("chargen = %v, erwartet 2", got)
	}
}

func TestBatcherPhasen_JeUeberweisungIstAufPostenBezogen(t *testing.T) {
	BatcherPhasenZuruecksetzen()
	t.Cleanup(BatcherPhasenZuruecksetzen)

	// Eine Charge mit 200 Posten, die zusammen 400 ms braucht, kostet je
	// Ueberweisung 2 ms -- NICHT 400. Genau diese Zahl wird gegen
	// transfer_phases.total_ms gehalten, eine Verwechslung mit der
	// Chargenzeit wuerde den Rueckfallpfad um Groessenordnungen zu teuer
	// aussehen lassen und die naechste Entscheidung verderben.
	merkeBatcherSammeln(1*time.Millisecond, 200)
	merkeBatcherWarten(100 * time.Millisecond)
	merkeBatcherArbeit(300*time.Millisecond, false)

	s := BatcherPhasenStand()
	if got := s["je_ueberweisung_ms"].(float64); got != 2 {
		t.Errorf("je_ueberweisung_ms = %v, erwartet 2 ((100+300)/200)", got)
	}
	if got := s["posten_je_charge"].(float64); got != 200 {
		t.Errorf("posten_je_charge = %v, erwartet 200", got)
	}
	if got := s["groesste_charge"].(int64); got != 200 {
		t.Errorf("groesste_charge = %v, erwartet 200", got)
	}
}

func TestBatcherPhasen_LeerIstNullUndMeldetDenDeckel(t *testing.T) {
	BatcherPhasenZuruecksetzen()
	t.Cleanup(BatcherPhasenZuruecksetzen)

	s := BatcherPhasenStand()
	for _, k := range []string{"warten_ms", "arbeit_ms", "je_ueberweisung_ms", "posten_je_charge"} {
		if got := s[k].(float64); got != 0 {
			t.Errorf("%s = %v auf leeren Zaehlern, erwartet 0", k, got)
		}
	}
	// Der Deckel muss auch ohne Verkehr sichtbar sein -- er ist die Groesse,
	// gegen die warten_ms zu lesen ist.
	if got := s["plaetze"].(int); got != batcherPlaetze() {
		t.Errorf("plaetze = %v, erwartet %d", got, batcherPlaetze())
	}
}

func TestBatcherKanal_MisstJeStueckNichtJeCharge(t *testing.T) {
	BatcherKanalZuruecksetzen()
	t.Cleanup(BatcherKanalZuruecksetzen)

	// Vier zurueckgefallene Ueberweisungen, zusammen 400 ms Wartezeit. Die
	// Auskunft muss 100 ms je Stueck sein -- nur so ist sie gegen
	// transfer_phases.total_ms des Schnellpfads zu halten, und genau dieser
	// Vergleich ist der Zweck der Uhr.
	merkeBatcherKanal(50 * time.Millisecond)
	merkeBatcherKanal(100 * time.Millisecond)
	merkeBatcherKanal(150 * time.Millisecond)
	merkeBatcherKanal(100 * time.Millisecond)

	s := BatcherKanalStand()
	if got := s["rueckfaelle"].(int64); got != 4 {
		t.Errorf("rueckfaelle = %v, erwartet 4", got)
	}
	if got := s["kanal_je_stueck_ms"].(float64); got != 100 {
		t.Errorf("kanal_je_stueck_ms = %v, erwartet 100", got)
	}
	if got := s["kanal_max_ms"].(float64); got != 150 {
		t.Errorf("kanal_max_ms = %v, erwartet 150", got)
	}
}

func TestBatcherKanal_LeerTeiltNichtDurchNull(t *testing.T) {
	BatcherKanalZuruecksetzen()
	t.Cleanup(BatcherKanalZuruecksetzen)
	s := BatcherKanalStand()
	if got := s["kanal_je_stueck_ms"].(float64); got != 0 {
		t.Errorf("kanal_je_stueck_ms = %v auf leeren Zaehlern, erwartet 0", got)
	}
}
