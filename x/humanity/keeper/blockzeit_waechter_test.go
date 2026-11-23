package keeper

import "testing"

// NeuesteBlockZeit: der Zeitstempel des kanonisch neuesten Blocks (hoechster
// BlueScore, dann kleinster Hash) -- und ohne zu warten, wenn eine Block-Welle
// die Schreibsperre haelt (dann der zuletzt gelesene Wert).
func TestNeuesteBlockZeit(t *testing.T) {
	dag := &BlockDAG{blocks: map[string]*Block{}, tips: map[string]bool{}}
	if z := dag.NeuesteBlockZeit(); z != 0 {
		t.Fatalf("leere Kette: %d", z)
	}
	for _, b := range []*Block{
		{Hash: "b", BlueScore: 7, Timestamp: 1000},
		{Hash: "a", BlueScore: 7, Timestamp: 1001}, // gleicher BlueScore, kleinerer Hash: kanonisch
		{Hash: "c", BlueScore: 6, Timestamp: 5000}, // spaeter, aber nicht kanonisch
	} {
		dag.blocks[b.Hash], dag.tips[b.Hash] = b, true
	}
	if z := dag.NeuesteBlockZeit(); z != 1001 {
		t.Fatalf("neuester Block: %d, erwartet 1001", z)
	}
	// Schreibsperre gehalten: sofort der zuletzt gelesene Wert.
	dag.mu.Lock()
	dag.blocks["d"], dag.tips["d"] = &Block{Hash: "d", BlueScore: 9, Timestamp: 2000}, true
	if z := dag.NeuesteBlockZeit(); z != 1001 {
		dag.mu.Unlock()
		t.Fatalf("unter der Sperre: %d", z)
	}
	dag.mu.Unlock()
	if z := dag.NeuesteBlockZeit(); z != 2000 {
		t.Fatalf("nach der Sperre: %d, erwartet 2000", z)
	}
}
