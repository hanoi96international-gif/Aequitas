package keeper

import (
	"encoding/json"
	"testing"
)

// Der Fehler, der den Primary als Produzenten stillgelegt hat.
//
// /api/blocks/by-hash bekam am 21.08.2026 eine Deckelung nach BYTES, weil
// 500 Bloecke zu je ~2 KB gedacht waren und ein Block unter Last allein rund
// 1 MB traegt. /api/blocks?min_height= blieb ungedeckelt -- dieselbe
// Bugklasse, ein Endpunkt weiter.
//
// Live auf dem Primary am 05.09.2026:
//
//	[HTTP-SYNC] Page fetch (min_height=5780364, pageSize=500) failed
//	(decoding response body (67108864 bytes): unexpected end of JSON input)
//
// Und weil doSyncOnce bei einem Fehlschlag auf der ERSTEN Seite frueh
// zurueckkehrt, wird noteStreakOutcome nie aufgerufen: der Zyklus zaehlt
// weder als sauber noch als Ruecksetzung. Gemessen: clean_cycles 0, alle
// resets 0, gate_skips 293, never_produced true. Das Produktionstor blieb
// damit dauerhaft zu, und sein Notausstieg griff nicht -- der feuert nur bei
// voelligem Stillstand, und Bloecke kamen ja an.
//
// Dieser Test prueft die Eigenschaft, an der das haengt: eine Antwort dieses
// Endpunkts muss in das Lesebudget passen, egal wie gross die einzelnen
// Bloecke sind.

func TestBlocksSince_AntwortPasstInsBudget(t *testing.T) {
	// Bloecke, die einzeln schon ein gutes Stueck des Budgets fuellen --
	// dieselbe Lage wie unter Last, wo ein Block Tausende Ueberweisungen
	// traegt. Zwanzig davon sprengen jedes Blockzahl-Limit bei weitem.
	gross := make([]byte, 2<<20) // 2 MB Nutzlast je Block
	for i := range gross {
		gross[i] = 'x'
	}
	blocks := make([]*Block, 0, 20)
	for i := 0; i < 20; i++ {
		blocks = append(blocks, &Block{
			Height: int64(i + 1),
			Hash:   string(gross[:1<<20]), // 1 MB je Block
		})
	}

	behalten, gekuerzt := capBlocksByResponseBytes(blocks)
	if !gekuerzt {
		t.Fatal("20 Bloecke zu je 1 MB wurden NICHT gekuerzt -- der Client wuerde die Antwort abschneiden und nicht parsen koennen")
	}
	if len(behalten) == 0 {
		t.Fatal("es wurde alles verworfen -- der Aufrufer kaeme nie voran")
	}
	if len(behalten) >= len(blocks) {
		t.Fatalf("es wurden %d von %d behalten, also nichts gekuerzt", len(behalten), len(blocks))
	}

	kodiert, err := json.Marshal(behalten)
	if err != nil {
		t.Fatalf("konnte die gekuerzte Antwort nicht kodieren: %v", err)
	}
	if len(kodiert) > blocksByHashResponseBudget {
		t.Errorf("gekuerzte Antwort ist %d Bytes und damit ueber dem Budget von %d",
			len(kodiert), blocksByHashResponseBudget)
	}
}

func TestBlocksSince_KleineAntwortBleibtUnveraendert(t *testing.T) {
	// Der Normalfall darf nicht teurer werden: passt alles ins Budget, wird
	// nichts gekuerzt und nichts signalisiert. Sonst wuerde der Client bei
	// JEDEM Zyklus glauben, es fehle noch etwas.
	blocks := []*Block{
		{Height: 1, Hash: "a"},
		{Height: 2, Hash: "b"},
		{Height: 3, Hash: "c"},
	}
	behalten, gekuerzt := capBlocksByResponseBytes(blocks)
	if gekuerzt {
		t.Error("drei winzige Bloecke wurden gekuerzt gemeldet")
	}
	if len(behalten) != 3 {
		t.Errorf("behalten = %d, erwartet 3", len(behalten))
	}
}

func TestBlocksSince_EinzelnerUeberbreiterBlockWirdGeliefert(t *testing.T) {
	// Ein einzelner Block groesser als das Budget MUSS trotzdem geliefert
	// werden. Wuerde er weggekuerzt, kaeme der Aufrufer an dieser Hoehe nie
	// vorbei -- ein dauerhafter Stillstand statt einer langsamen Seite.
	gross := make([]byte, blocksByHashResponseBudget+(1<<20))
	for i := range gross {
		gross[i] = 'y'
	}
	behalten, _ := capBlocksByResponseBytes([]*Block{{Height: 1, Hash: string(gross)}})
	if len(behalten) != 1 {
		t.Fatalf("ein ueberbreiter Einzelblock wurde nicht geliefert (behalten=%d) -- der Aufrufer kaeme an dieser Hoehe nie vorbei", len(behalten))
	}
}
