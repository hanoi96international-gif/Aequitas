package main

import (
	"os"
	"strings"
	"testing"
)

// DIE MESSFEHLER, DIE DIESE ZAHL DREIMAL FALSCH GEMACHT HABEN.
//
// Der Kettendurchsatz ist die Zahl, an der die ganze Arbeit gemessen wird --
// und er war am 11.09.2026 dreimal hintereinander falsch, jedes Mal auf eine
// Art, die von aussen plausibel aussah:
//
//  1. Gezaehlt ueber /api/block?height=N. Das liefert EINEN Block je Hoehe,
//     bei zwei Validatoren gibt es zwei. Ergebnis: halbe Zahl.
//  2. Umgestellt auf /api/blocks, aber die Seiten ueberlappen und Bloecke
//     wurden mehrfach gezaehlt. Ergebnis: 5,2 Bloecke je Hoehe bei zwei
//     Validatoren.
//  3. Abgegrenzt ueber produced_at_ms -- ein Feld, das nur die juengsten
//     Bloecke tragen. Von 29 abgefragten Bloecken hatten 29 keinen
//     Zeitstempel, die Bedingung liess alle durch, und die Spanne kam aus der
//     Handvoll, die einen hatte. Ergebnis: zu grosser Zaehler ueber zu
//     kleinem Nenner.
//
// Diese Tests halten alle drei fest.

func quelle(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("main.go nicht lesbar: %v", err)
	}
	return string(b)
}

func TestKettendurchsatz_ZaehltBeideGeschwister(t *testing.T) {
	body := quelle(t)
	i := strings.Index(body, "func kettenDurchsatz(")
	if i < 0 {
		t.Fatal("kettenDurchsatz nicht gefunden")
	}
	rumpf := body[i:]
	if j := strings.Index(rumpf[1:], "\nfunc "); j > 0 {
		rumpf = rumpf[:j]
	}
	// Auf den AUFRUF pruefen, nicht auf den Text: der Kommentar in der
	// Funktion erklaert den alten Weg und nennt ihn dabei beim Namen.
	if strings.Contains(rumpf, `hc.Get(fmt.Sprintf("%s/api/block?height=`) {
		t.Error("gezaehlt wird wieder ueber /api/block?height=N. Das liefert einen Block je " +
			"Hoehe -- den kanonischen. Bei zwei Validatoren entstehen zwei, jeder mit eigenen " +
			"Transaktionen aus dem eigenen Mempool, und die Zahl wird damit halbiert.")
	}
	if !strings.Contains(rumpf, "/api/blocks?min_height=") {
		t.Error("/api/blocks?min_height= ist weg -- das ist der einzige Endpunkt, der beide " +
			"Geschwister liefert.")
	}
}

func TestKettendurchsatz_EntdoppeltBloeckeUndTransaktionen(t *testing.T) {
	body := quelle(t)
	if !strings.Contains(body, "blockGesehen[blk.Hash]") {
		t.Error("Bloecke werden nicht mehr ueber den Hash entdoppelt. Die Seiten von " +
			"/api/blocks ueberlappen (min_height wird auf die hoechste Hoehe der Seite " +
			"gesetzt, und dort liegen mehrere Geschwister) -- ohne Entdopplung kamen " +
			"5,2 Bloecke je Hoehe heraus, wo es zwei Validatoren gibt.")
	}
	if !strings.Contains(body, "gesehen[schluessel]") {
		t.Error("Transaktionen werden nicht mehr entdoppelt. Derselbe Hash kann im DAG in " +
			"mehr als einem Block liegen; eine Summe ueber die Bloecke wuerde ihn doppelt " +
			"zaehlen.")
	}
}

// Der teuerste der drei: er machte die Zahl GROESSER, sah also nach Erfolg aus.
func TestKettendurchsatz_GrenztUeberDieHoeheAbNichtUeberDenZeitstempel(t *testing.T) {
	body := quelle(t)
	i := strings.Index(body, "func kettenDurchsatz(")
	rumpf := body[i:]
	if j := strings.Index(rumpf[1:], "\nfunc "); j > 0 {
		rumpf = rumpf[:j]
	}

	if !strings.Contains(rumpf, "blk.Height <= vonHoehe") {
		t.Error("das Fenster wird nicht mehr ueber die Hoehe abgegrenzt. produced_at_ms " +
			"tragen nur die juengsten Bloecke -- alles aus der Datenbank kommt mit 0 " +
			"zurueck, und eine Bedingung der Form \"ungleich 0 UND ausserhalb\" laesst " +
			"dann jeden davon durch. Die Hoehe steht in jedem Block.")
	}
	if !strings.Contains(rumpf, "spanne := bis.Sub(von).Seconds()") {
		t.Error("der Nenner kommt wieder aus den Zeitstempeln statt aus der Laufzeit. Wenn " +
			"nur ein Teil der Bloecke einen Zeitstempel hat, ist diese Spanne zu kurz -- und " +
			"ein zu kleiner Nenner macht aus einer ehrlichen Zahl eine schmeichelhafte.")
	}
	// Und die Sicherung: eine unbekannte Starthoehe darf nicht die ganze Kette laden.
	if !strings.Contains(rumpf, "vonHoehe <= 0") {
		t.Error("eine unlesbare Starthoehe wird nicht mehr abgefangen. Mit vonHoehe = 0 " +
			"begaenne die Schleife bei Block 1 und liefe die ganze Kette nach.")
	}
}
