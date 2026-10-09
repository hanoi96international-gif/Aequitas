package keeper

import (
	"testing"
)

// setzeBeobachterFuerTest: die Umgebung ist danach gelesen (sync.Once), und
// der Wert wird atomar gesetzt und zurueckgesetzt -- kein Neuzuweisen der
// Once, kein ungeschuetztes bool (Pruefung von #322, INFO-3).
func setzeBeobachterFuerTest(t *testing.T, an bool) {
	t.Helper()
	beobachterModus()
	alt := beobachterAn.Swap(an)
	t.Cleanup(func() { beobachterAn.Store(alt) })
}

// Ein Beobachter erzeugt nie einen Block, auch wenn alles andere ihn liesse.
func TestBeobachter_ErzeugtKeinenBlock(t *testing.T) {
	dag, _ := newDeterminismTestDAG()
	setzeBeobachterFuerTest(t, true)
	if b := dag.ProduceBlock(); b != nil {
		t.Fatalf("Beobachter hat Block #%d erzeugt", b.Height)
	}
}
