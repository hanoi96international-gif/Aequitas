package keeper

import "testing"

// Der Zaehler existiert, weil zwei andere Zahlen den Durchsatz falsch
// wiedergeben. Er taugt nur, wenn er selbst genau das misst, was er
// verspricht -- diese Tests pruefen die drei Eigenschaften, auf die sich
// jede folgende Entscheidung stuetzt.

func TestProduktion_SummiertBloeckeUndTransaktionen(t *testing.T) {
	ProduktionZuruecksetzen()
	t.Cleanup(ProduktionZuruecksetzen)

	MerkeBlockGroesse(3000)
	MerkeBlockGroesse(5000)
	MerkeBlockGroesse(1000)

	s := ProduktionsStand()
	if got := s["bloecke"].(int64); got != 3 {
		t.Errorf("bloecke = %d, erwartet 3", got)
	}
	if got := s["transaktionen"].(int64); got != 9000 {
		t.Errorf("transaktionen = %d, erwartet 9000", got)
	}
	if got := s["tx_je_block"].(float64); got != 3000 {
		t.Errorf("tx_je_block = %v, erwartet 3000", got)
	}
	if got := s["groesster_block"].(int64); got != 5000 {
		t.Errorf("groesster_block = %d, erwartet 5000 -- ein spaeterer kleinerer Block hat ihn ueberschrieben", got)
	}
}

// Ein leerer Block ist kein Ereignis fuer diesen Zaehler: er wuerde tx_je_block
// verwaessern und damit genau die Frage verfaelschen, ob der Deckel bindet.
func TestProduktion_LeererBlockZaehltNicht(t *testing.T) {
	ProduktionZuruecksetzen()
	t.Cleanup(ProduktionZuruecksetzen)

	MerkeBlockGroesse(0)
	MerkeBlockGroesse(-5)
	if got := ProduktionsStand()["bloecke"].(int64); got != 0 {
		t.Errorf("bloecke = %d, erwartet 0", got)
	}
}

func TestProduktion_LeerTeiltNichtDurchNull(t *testing.T) {
	ProduktionZuruecksetzen()
	t.Cleanup(ProduktionZuruecksetzen)

	s := ProduktionsStand() // darf nicht panicken
	for _, k := range []string{"tx_je_block", "tx_pro_sekunde", "messdauer_sekunden"} {
		if got := s[k].(float64); got != 0 {
			t.Errorf("%s = %v auf leeren Zaehlern, erwartet 0", k, got)
		}
	}
}
