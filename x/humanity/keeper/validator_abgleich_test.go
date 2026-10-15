package keeper

import (
	"sync"
	"testing"
	"time"
)

// Missbrauch (Tempo-Pruefung 15.10.2026): viele Bloecke mit frischen
// Schluesseln starten nicht je einen Abgleich -- hoechstens einer laeuft,
// und ein Anstoss waehrend des Laufs fuehrt zu genau einer Wiederholung.
func TestValidatorAbgleich_Gebuendelt(t *testing.T) {
	dag := &BlockDAG{activeSyncPeers: map[string]bool{}, trustedSeeds: map[string]bool{}}
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); dag.validatorAbgleichAnstossen() }()
	}
	wg.Wait()
	ende := time.Now().Add(5 * time.Second)
	for dag.validatorAbgleichLaeuft.Load() {
		if time.Now().After(ende) {
			t.Fatal("Abgleich endet nicht")
		}
		time.Sleep(time.Millisecond)
	}
	if dag.validatorAbgleichNochmal.Load() {
		t.Fatal("ein Anstoss blieb liegen")
	}
}
