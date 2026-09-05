package keeper

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"testing"
)

// Der Sentinel muss durch fmt.Errorf-Verpackung hindurch erkennbar bleiben --
// beide Fundstellen in applyTransferDeltaLocked verpacken ihn mit %w.
func TestZustandsAblehnung_UeberlebtVerpackung(t *testing.T) {
	err := fmt.Errorf("insufficient balance (have %.6f after demurrage, need %.6f): %w",
		0.0, 0.00001, ErrZustandLehntAb)
	if !istZustandsAblehnung(err) {
		t.Fatal("verpackter Sentinel wurde nicht erkannt -- das Replay wuerde den " +
			"Block wieder toeten statt die Ueberweisung zu ueberspringen")
	}
	if istZustandsAblehnung(errors.New("connection refused")) {
		t.Fatal("ein gewoehnlicher Fehler darf NICHT als deterministisch gelten -- " +
			"sonst wird ein DB-Ausfall stillschweigend als leere Ueberweisung verbucht")
	}
	if istZustandsAblehnung(nil) {
		t.Fatal("nil ist keine Ablehnung")
	}
}

// Waechter: die beiden deterministischen Faelle muessen den Sentinel tragen.
// Ohne ihn faellt das Replay auf hardFailure zurueck, und damit kehrt die Wand
// wieder -- am 05.09.2026 waren das 18 Abweisungen derselben Hoehe und sechs
// Minuten Stillstand auf dem Primary.
func TestApplyTransferDelta_MarkiertDeterministischeAblehnungen(t *testing.T) {
	roh, err := os.ReadFile("state.go")
	if err != nil {
		t.Fatalf("state.go nicht lesbar: %v", err)
	}
	for _, f := range []struct{ name, muster string }{
		{"insufficient balance", `insufficient balance \([^)]*\): %w", fromAcc`},
		{"from account not found", `from account not found: %s: %w", from`},
	} {
		if !regexp.MustCompile(f.muster).Match(roh) {
			t.Fatalf("%q traegt ErrZustandLehntAb nicht mehr -- ohne den Sentinel "+
				"weist das Replay wieder den ganzen Block ab, dauerhaft", f.name)
		}
	}
}

// Waechter: das Replay muss die deterministische Ablehnung ueberspringen. Faellt
// dieser Zweig weg, ist der Rest des Fixes wirkungslos.
func TestReplay_UeberspringtDeterministischeAblehnung(t *testing.T) {
	roh, err := os.ReadFile("block.go")
	if err != nil {
		t.Fatalf("block.go nicht lesbar: %v", err)
	}
	if !regexp.MustCompile(`istZustandsAblehnung\(err\)`).Match(roh) {
		t.Fatal("replayTransactions prueft nicht mehr auf eine deterministische " +
			"Ablehnung -- eine einzige nicht anwendbare Ueberweisung toetet damit " +
			"wieder den ganzen Block")
	}
}

// Der Zaehler ist kein Beiwerk: der Fix tauscht einen lauten Fehler gegen einen
// leisen, und ein leiser Fehler, den niemand zaehlt, ist schlimmer.
func TestZustandsAblehnung_WirdGezaehlt(t *testing.T) {
	vorher := uebersprungeneUeberweisungen.Load()
	merkeUebersprungeneUeberweisung()
	if uebersprungeneUeberweisungen.Load() != vorher+1 {
		t.Fatal("uebersprungene Ueberweisung wurde nicht gezaehlt")
	}
	if _, ok := ZustandsAblehnungStand()["uebersprungene_ueberweisungen"]; !ok {
		t.Fatal("die Zahl fehlt in /api/health/combined -- ein leiser Fehler, den " +
			"niemand sehen kann")
	}
}
