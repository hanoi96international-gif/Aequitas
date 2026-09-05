package keeper

import (
	"os"
	"regexp"
	"testing"
)

// Waechter: die Replay-Wand muss die ENDGUELTIGE Heilung ausloesen, nicht die
// gewoehnliche.
//
// Der Unterschied ist nicht kosmetisch. triggerAutoResync haelt eine
// 30-Minuten-Sperre ein, die davor schuetzt, einen langsam aber erfolgreich
// aufholenden Knoten aus der Kette zu reissen. Bei einer wiederholt
// abgewiesenen, IDENTISCHEN Blockhoehe schuetzt sie vor nichts -- derselbe
// Block scheitert deterministisch, und Zuwarten aendert daran nichts.
//
// Live am 02.09.2026: der Detektor meldete korrekt "block #5530822 rejected 3
// times in a row", und die Antwort war "SUPPRESSED by the 30m0s cooldown for
// another 17m48s". Am 05.09.2026 stand C1 mit 18 Abweisungen derselben Hoehe,
// 31 DAG-Tips und 382 Sekunden ohne Blockproduktion -- eingefroren, ohne
// Ausweg. Faellt dieser Aufruf auf die gewoehnliche Fassung zurueck, kehrt
// genau dieser Zustand wieder.
func TestReplayMauer_LoestEndgueltigeHeilungAus(t *testing.T) {
	roh, err := os.ReadFile("replay_mauer.go")
	if err != nil {
		t.Fatalf("replay_mauer.go nicht lesbar: %v", err)
	}
	if !regexp.MustCompile(`triggerAutoResyncEndgueltig\(`).Match(roh) {
		t.Fatal("loeseHeilungAus ruft nicht triggerAutoResyncEndgueltig. Mit der " +
			"gewoehnlichen Fassung bleibt ein bewiesen festsitzender Knoten bis zu " +
			"30 Minuten stehen, statt sich nach 3 zu heilen.")
	}
	// Die gewoehnliche Fassung darf hier gar nicht mehr vorkommen -- ein
	// zurueckgebliebener Aufruf waere genau der Rueckfall.
	ohneEndgueltig := regexp.MustCompile(`triggerAutoResync\((?:"|[a-z])`)
	if ohneEndgueltig.Match(roh) {
		t.Fatal("replay_mauer.go ruft noch die gewoehnliche triggerAutoResync-Fassung")
	}
}

// Die kurze Sperre muss wirklich kuerzer sein als die lange -- sonst ist die
// ganze Unterscheidung wirkungslos.
func TestEndgueltigeHeilung_SperreIstKuerzer(t *testing.T) {
	if autoHealFailedResyncRetry >= autoHealCooldown {
		t.Fatalf("autoHealFailedResyncRetry=%v ist nicht kuerzer als autoHealCooldown=%v -- "+
			"die endgueltige Heilung wuerde den Stillstand nicht verkuerzen",
			autoHealFailedResyncRetry, autoHealCooldown)
	}
}
