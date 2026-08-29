package keeper

import (
	"sync"
	"testing"
)

// Die Schranke muss ueber ihrem Wert ablehnen -- sonst nimmt der Knoten
// weiter unbegrenzt an, und genau das ergab am 29.08.2026 bei 576 Sendern
// 0 Erfolge und 138.000 Zeitueberschreitungen.
func TestInflight_LehntUeberDerGrenzeAb(t *testing.T) {
	t.Setenv(inflightGrenzeEnv, "10")
	inflightZuruecksetzen()

	if !inflightEintritt(6) {
		t.Fatal("6 von 10 muessen durchgehen")
	}
	if !inflightEintritt(4) {
		t.Fatal("4 weitere fuellen die Grenze genau aus -- muessen durchgehen")
	}
	if inflightEintritt(1) {
		t.Fatal("der 11. Posten muss abgelehnt werden, die Grenze ist 10")
	}
	inflightAustritt(4)
	if !inflightEintritt(4) {
		t.Fatal("nach dem Austritt muss wieder Platz sein -- sonst laeuft die Schranke zu")
	}
}

// Eine ausdrueckliche 0 schaltet ab. Ein TIPPFEHLER darf das nicht:
// evm_rpc.go stellt diese Regel fuer den Ratenbegrenzer auf, und ein Schutz,
// den eine verrutschte Taste abschaltet, ist keiner.
func TestInflight_TippfehlerSchaltetNichtAb(t *testing.T) {
	for _, murks := range []string{"achttausend", "-5", "8000x", " "} {
		t.Setenv(inflightGrenzeEnv, murks)
		if got := inflightGrenze(); got != inflightVorgabe {
			t.Fatalf("bei %q ist die Grenze %d, erwartet die Vorgabe %d -- "+
				"ein Tippfehler darf den Schutz nicht abschalten", murks, got, inflightVorgabe)
		}
	}
	t.Setenv(inflightGrenzeEnv, "0")
	if got := inflightGrenze(); got != 0 {
		t.Fatalf("eine ausdrueckliche 0 muss abschalten, ist aber %d", got)
	}
	inflightZuruecksetzen()
	if !inflightEintritt(1 << 30) {
		t.Fatal("abgeschaltet darf nichts abgelehnt werden")
	}
}

// Der Zaehler muss unter Nebenlaeufigkeit stimmen -- er entscheidet ueber
// Annahme und Ablehnung, ein Zaehlfehler sperrt den Knoten dauerhaft zu.
func TestInflight_ZaehltNebenlaeufigRichtig(t *testing.T) {
	t.Setenv(inflightGrenzeEnv, "1000000")
	inflightZuruecksetzen()

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if inflightEintritt(3) {
					inflightAustritt(3)
				}
			}
		}()
	}
	wg.Wait()
	if got := inflightAktuell.Load(); got != 0 {
		t.Fatalf("nach gleich vielen Ein- und Austritten steht der Zaehler auf %d, "+
			"erwartet 0 -- ein Leck sperrt den Knoten mit der Zeit komplett zu", got)
	}
}

// Genau die Konstellation vom 29.08.2026: mehr gleichzeitige Arbeit als die
// Schranke erlaubt. Erwartet wird eine ABLEHNUNG, keine Annahme -- eine
// angenommene und dann verworfene Anfrage ist doppelt verloren.
func TestInflight_576SenderWerdenAbgelehntStattAngenommen(t *testing.T) {
	t.Setenv(inflightGrenzeEnv, "8000")
	inflightZuruecksetzen()

	angenommen, abgelehnt := 0, 0
	for i := 0; i < 576; i++ {
		if inflightEintritt(100) { // ein Buendel je Sender
			angenommen++
		} else {
			abgelehnt++
		}
	}
	if abgelehnt == 0 {
		t.Fatal("576 Buendel a 100 = 57.600 Posten bei Grenze 8.000: es MUSS abgelehnt werden")
	}
	if angenommen != 80 {
		t.Fatalf("angenommen %d, erwartet 80 (8.000 / 100) -- die Schranke haelt nicht genau", angenommen)
	}
}

// inflightZuruecksetzen macht die Zaehler zwischen Tests unabhaengig. Nur
// fuer Tests: im Betrieb gibt es keinen Grund, eine laufende Schranke zu
// vergessen.
func inflightZuruecksetzen() {
	inflightAktuell.Store(0)
	inflightHoechststand.Store(0)
	inflightAbgelehnt.Store(0)
	inflightAngenommen.Store(0)
}
