package keeper

import (
	"fmt"
	"testing"
	"time"
)

// baueKette legt eine Kette aus n unabgespielten Bloecken an und gibt den
// juengsten zurueck.
func baueKette(n int) (*BlockDAG, *Block) {
	dag := &BlockDAG{
		blocks:         map[string]*Block{},
		tips:           map[string]bool{},
		replayedBlocks: map[string]bool{},
	}
	var vorher *Block
	for i := 0; i < n; i++ {
		b := &Block{Height: int64(i), Hash: fmt.Sprintf("h%06d", i), BlueScore: int64(i)}
		if vorher != nil {
			b.ParentHashes = []string{vorher.Hash}
		}
		dag.blocks[b.Hash] = b
		vorher = b
	}
	return dag, vorher
}

// Das Verhalten darf sich durch das Herausziehen der Sperre nicht aendern:
// alle unabgespielten Vorfahren, nach Hoehe sortiert, abgespielte ausgelassen.
func TestAhnenlauf_FindetAlleUndSortiert(t *testing.T) {
	dag, juengster := baueKette(50)
	got := dag.collectUnreplayedAncestors(juengster)
	if len(got) != 49 { // alle ausser dem Ziel selbst
		t.Fatalf("%d Vorfahren, erwartet 49", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Height > got[i].Height {
			t.Fatalf("nicht nach Hoehe sortiert: %d vor %d", got[i-1].Height, got[i].Height)
		}
	}
	// Bereits abgespielte muessen ausgelassen werden -- und mit ihnen der
	// ganze Zweig dahinter.
	dag.replayedBlocks["h000025"] = true
	got2 := dag.collectUnreplayedAncestors(juengster)
	if len(got2) != 23 {
		t.Fatalf("%d Vorfahren nach dem Markieren von h000025, erwartet 23 "+
			"(h000026..h000048 -- die Suche muss bei h000025 abbrechen)", len(got2))
	}
}

// Die Kosten duerfen nicht davonlaufen.
//
// GEMESSEN, und die Zahl relativiert den Nutzen der Aenderung ehrlich: 200
// Bloecke kosten 26 us, 2.000 kosten 565 us. Das ist leicht ueberlinear
// (Faktor 21,7 bei 10-facher Laenge, wegen der Sortierung), aber in
// ABSOLUTEN Zahlen winzig -- bei rund 19 ankommenden Bloecken je Sekunde ist
// ein halber Millisekundenaufwand etwa ein Prozent der Zeit.
//
// Diese Funktion ist also NICHT die Ursache der minutenlangen Stillstaende,
// die am 02.09.2026 sechsmal zu sehen waren. Das Herausziehen der Sperre aus
// der Schleife bleibt trotzdem richtig: es spart 2N Sperrerwerbe unter
// dag.mu.RLock(), und unter Gedraenge kostet jeder davon ein Vielfaches
// seines unbestrittenen Preises. Es ist eine Verbesserung, keine Loesung --
// und dieser Test haelt fest, dass die Kosten nicht davonlaufen, falls
// jemand die Sortierung oder den Lauf spaeter anfasst.
func TestAhnenlauf_BleibtLinear(t *testing.T) {
	if testing.Short() {
		t.Skip("misst Zeit")
	}
	messe := func(n int) time.Duration {
		dag, juengster := baueKette(n)
		start := time.Now()
		for i := 0; i < 20; i++ {
			dag.collectUnreplayedAncestors(juengster)
		}
		return time.Since(start) / 20
	}
	klein, gross := messe(200), messe(2000)
	t.Logf("200 Bloecke: %s   2000 Bloecke: %s", klein, gross)
	if klein <= 0 {
		t.Skip("Zeitgeber zu grob")
	}
	faktor := float64(gross) / float64(klein)
	t.Logf("Faktor bei 10-facher Kettenlaenge: %.1fx (10x waere linear)", faktor)
	if faktor > 40 {
		t.Errorf("zehnfache Kettenlaenge kostet das %.1f-fache -- das ist deutlich "+
			"ueberlinear. Ein zurueckfallender Knoten wird dann mit jedem Block "+
			"teurer und kommt nie wieder heran", faktor)
	}
}
