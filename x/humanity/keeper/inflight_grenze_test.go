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
//
// Die Vorgabe haengt an GOMEMLIMIT (vorgabeNachSpeicher). Der Test setzt es
// selbst, sonst fiel er durch, sobald die Umgebung eines setzte.
func TestInflight_TippfehlerSchaltetNichtAb(t *testing.T) {
	for _, fall := range []struct {
		gomemlimit string
		vorgabe    int64
	}{
		{"", inflightVorgabe},
		{"5GiB", 20000},
		{"256MiB", inflightUntergrenze},
	} {
		t.Setenv("GOMEMLIMIT", fall.gomemlimit)
		for _, murks := range []string{"achttausend", "-5", "8000x", " "} {
			t.Setenv(inflightGrenzeEnv, murks)
			if got := inflightGrenze(); got != fall.vorgabe {
				t.Fatalf("GOMEMLIMIT=%q, bei %q ist die Grenze %d, erwartet die Vorgabe %d -- "+
					"ein Tippfehler darf den Schutz nicht abschalten", fall.gomemlimit, murks, got, fall.vorgabe)
			}
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
	// Zurueckgeben. Ein vergessener Austritt laesst die Schranke zulaufen --
	// beim ersten Schreiben dieses Tests fehlte genau das, und der naechste
	// Test lief in eine volle Schranke statt in den Ratenbegrenzer. Im Betrieb
	// waere das ein Knoten, der ab da alles ablehnt.
	inflightAustritt(int64(angenommen) * 100)
	if rest := inflightAktuell.Load(); rest != 0 {
		t.Fatalf("nach dem Aufraeumen stehen noch %d Posten offen", rest)
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

// abweicht_von_vorgabe vergleicht mit der Vorgabe, die gerade gilt: auf den
// Boxen (GOMEMLIMIT=5GiB) ist das 20.000, nicht 8.000. Vorher meldete der
// Knoten dort "abweichend", obwohl niemand etwas eingestellt hatte.
func TestInflight_StandVergleichtMitDerGeltendenVorgabe(t *testing.T) {
	t.Setenv("GOMEMLIMIT", "5GiB")
	t.Setenv(inflightGrenzeEnv, "")
	if st := InflightStand(); st["grenze"] != int64(20000) || st["abweicht_von_vorgabe"] != false {
		t.Fatalf("ohne Einstellung: grenze %v, abweicht %v -- erwartet 20000, false", st["grenze"], st["abweicht_von_vorgabe"])
	}
	t.Setenv(inflightGrenzeEnv, "8000")
	if st := InflightStand(); st["abweicht_von_vorgabe"] != true {
		t.Fatalf("eingestellt auf 8000 bei Vorgabe 20000: abweicht %v, erwartet true", st["abweicht_von_vorgabe"])
	}
	t.Setenv("GOMEMLIMIT", "")
	t.Setenv(inflightGrenzeEnv, "")
	if st := InflightStand(); st["grenze"] != inflightVorgabe || st["abweicht_von_vorgabe"] != false {
		t.Fatalf("ohne GOMEMLIMIT: grenze %v, abweicht %v -- erwartet %d, false", st["grenze"], st["abweicht_von_vorgabe"], inflightVorgabe)
	}
}
