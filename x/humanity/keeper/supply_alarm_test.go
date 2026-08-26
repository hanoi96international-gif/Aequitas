package keeper

import (
	"os"
	"strings"
	"testing"
)

// The gap measured on 2026-08-20. Anything at or below it is the old excess
// that predates the save-ordering fixes of that day; anything above it is new.
const bekannteLuecke = 305.277988

func TestSupplyAlarmSchweigtBeiDerBekanntenLuecke(t *testing.T) {
	if alarm, _ := supplyAlarm(bekannteLuecke, bekannteLuecke); alarm {
		t.Fatal("die bereits bekannte Luecke darf keinen Alarm ausloesen -- " +
			"ein Alarm, der immer steht, wird nicht mehr gelesen")
	}
}

func TestSupplyAlarmSchlaegtBeiNeuerSchoepfungAn(t *testing.T) {
	alarm, grund := supplyAlarm(bekannteLuecke+5, bekannteLuecke)
	if !alarm {
		t.Fatal("5 AEQ ueber der bekannten Luecke sind frisch geschoepftes Geld")
	}
	if !strings.Contains(grund, "5.000000") {
		t.Fatalf("der Grund muss die neue Menge nennen, nicht die gesamte: %q", grund)
	}
}

func TestSupplyAlarmSchweigtWennDieLueckeSchrumpft(t *testing.T) {
	// Das ist der Fall, den wir HERBEIFUEHREN wollen: der Altbestand wird
	// abgebaut. Dafuer darf niemand geweckt werden.
	if alarm, _ := supplyAlarm(bekannteLuecke-100, bekannteLuecke); alarm {
		t.Fatal("eine schrumpfende Luecke ist die Bereinigung, kein Vorfall")
	}
}

func TestSupplyAlarmIstNichtVonRundungAbhaengig(t *testing.T) {
	// Guthaben liegen als Mikro-Ganzzahlen vor; ein korrektes System ist exakt.
	// Ein halbes Mikro-AEQ ist Arithmetik, kein Befund.
	if alarm, _ := supplyAlarm(bekannteLuecke+5e-7, bekannteLuecke); alarm {
		t.Fatal("ein halbes Mikro-AEQ darf keinen Alarm ausloesen")
	}
	if alarm, _ := supplyAlarm(bekannteLuecke+1e-5, bekannteLuecke); !alarm {
		t.Fatal("ein hundertstel AEQ liegt weit ueber der Rundung und ist ein Befund")
	}
}

func TestBaselineLaesstSichOhneNeubauSenken(t *testing.T) {
	// Nach dem Abbau des Altbestands muss der Alarm auf dem NEUEN Stand
	// scharf werden, ohne dass jemand das Programm neu baut.
	t.Setenv("SUPPLY_GAP_BASELINE_AEQ", "0")
	if got := knownSupplyGapAEQ(); got != 0 {
		t.Fatalf("Baseline nicht uebernommen: %v", got)
	}
	if alarm, _ := supplyAlarm(0.5, knownSupplyGapAEQ()); !alarm {
		t.Fatal("bei Baseline 0 ist ein halbes AEQ ein Befund")
	}
}

func TestUnbrauchbareBaselineFaelltAufDenEingebautenWertZurueck(t *testing.T) {
	// Der gefaehrliche Fehlschlag waere, eine unlesbare Angabe als "0 Toleranz"
	// ODER als "unendlich Toleranz" zu deuten. Der eingebaute Wert ist der
	// einzige, der weder blind macht noch stillen Dauerlaerm erzeugt.
	for _, murks := range []string{"abc", "-1", "", "  "} {
		t.Setenv("SUPPLY_GAP_BASELINE_AEQ", murks)
		if got := knownSupplyGapAEQ(); got != bekannteLuecke {
			t.Fatalf("bei %q wurde %v statt des eingebauten Wertes benutzt", murks, got)
		}
	}
}

func TestOhneUmgebungsvariableGiltDerGemesseneWert(t *testing.T) {
	os.Unsetenv("SUPPLY_GAP_BASELINE_AEQ")
	if got := knownSupplyGapAEQ(); got != bekannteLuecke {
		t.Fatalf("erwartet %v, bekommen %v", bekannteLuecke, got)
	}
}
