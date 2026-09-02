package keeper

import (
	"os"
	"regexp"
	"testing"
)

// Waechter: handleStatus hat zwei Wege, und beide muessen dieselbe Quelle
// nennen. Nimmt einer wieder latest.Height, springt die gemeldete Hoehe je
// nach Sperrlage -- live gemessen als 41 Bloecke rueckwaerts auf dem Primary,
// ohne dass im Konsens irgendetwas falsch war. Ein Explorer zeigt dann eine
// rueckwaerts laufende Kette, und der naechste Verdacht auf Instabilitaet
// jagt wieder ein Gespenst.
func TestStatus_HoeheKommtAusEinerQuelle(t *testing.T) {
	roh, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("api.go nicht lesbar: %v", err)
	}
	// Nur das height-Feld der Statusantwort, nicht jede Erwaehnung von
	// latest.Height (andere Felder duerfen den Block weiter benutzen).
	re := regexp.MustCompile(`"height":\s*latest\.Height`)
	if re.Match(roh) {
		t.Fatal(`handleStatus meldet "height": latest.Height. Der ` +
			`Zwischenspeicher-Weg meldet HeightSchnell() -- zwei Quellen fuer ` +
			`dieselbe Zahl lassen die Hoehe zwischen zwei Abfragen springen. ` +
			`Siehe status_hoehe.go.`)
	}
}

func TestHoehenAbweichung_ZaehltNurEchteUnterschiede(t *testing.T) {
	vorher := hoehenAbweichungen.Load()
	merkeHoehenAbweichung(100, 100)
	if hoehenAbweichungen.Load() != vorher {
		t.Fatal("gleiche Werte duerfen nicht als Abweichung zaehlen")
	}
	merkeHoehenAbweichung(100, 141)
	if hoehenAbweichungen.Load() != vorher+1 {
		t.Fatal("Unterschied wurde nicht gezaehlt")
	}
	// Der Betrag zaehlt, nicht das Vorzeichen: eine Hoehe, die 41 zurueck
	// liegt, ist genauso eine Abweichung wie eine, die 41 voraus ist.
	if got := hoehenAbweichungMax.Load(); got < 41 {
		t.Fatalf("Hoechstbetrag = %d, erwartet mindestens 41", got)
	}
}
