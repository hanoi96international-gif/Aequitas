package keeper

import (
	"sync"
	"testing"
)

func setzeBeobachterFuerTest(t *testing.T, an bool) {
	t.Helper()
	beobachterEinmal.Do(func() {})
	alt := beobachterAn
	beobachterAn = an
	t.Cleanup(func() { beobachterAn = alt; beobachterEinmal = sync.Once{} })
}

// Ein Beobachter erzeugt nie einen Block, auch wenn alles andere ihn liesse.
func TestBeobachter_ErzeugtKeinenBlock(t *testing.T) {
	dag, _ := newDeterminismTestDAG()
	setzeBeobachterFuerTest(t, true)
	if b := dag.ProduceBlock(); b != nil {
		t.Fatalf("Beobachter hat Block #%d erzeugt", b.Height)
	}
}
