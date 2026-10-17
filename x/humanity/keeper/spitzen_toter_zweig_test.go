package keeper

import (
	"reflect"
	"sort"
	"testing"
)

// Befund aus dem Vier-Knoten-Test: tote Zweige unter dem finalisierten
// Checkpoint bleiben fuer immer in dag.tips. ProduceBlock nimmt sie nicht als
// Eltern, aber GetTips und /api/status (TipsCount) meldeten sie -- ein Knoten
// sah dann nach mehreren Spitzen aus, obwohl er auf einer baute, und
// tip_reannounce bot Bloecke an, auf die nie wieder jemand baut.

func spitzenTestDAG(finalisiert int64) *BlockDAG {
	dag := newGhostdagTestDAG()
	cs := &ChainState{}
	cs.finalizedMu.Lock()
	cs.finalizedHeightCache = finalisiert
	cs.finalizedBlueScoreCache = 1
	cs.finalizedCacheLoaded = true
	cs.finalizedMu.Unlock()
	dag.state = cs
	dag.tips = map[string]bool{}
	for _, b := range []*Block{
		{Hash: "tot", Height: 5, BlueScore: 900},    // unter der Finalitaet, trotz hoher BlueScore
		{Hash: "grenze", Height: 10, BlueScore: 10}, // genau auf der finalisierten Hoehe: lebt
		{Hash: "lebend", Height: 20, BlueScore: 20}, // darueber
		{Hash: "tot2", Height: 9, BlueScore: 9},     // knapp darunter
	} {
		dag.blocks[b.Hash] = b
		dag.tips[b.Hash] = true
	}
	dag.tips["ohneBlock"] = true // nicht im Speicher: wie in ProduceBlock nicht ausgelassen
	return dag
}

func TestSpitzen_ToteZweigeNichtGemeldet(t *testing.T) {
	dag := spitzenTestDAG(10)
	got := dag.GetTips()
	sort.Strings(got)
	want := []string{"grenze", "lebend", "ohneBlock"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetTips = %v, erwartet %v (tote Zweige unter Hoehe 10 ausgelassen)", got, want)
	}
	if n := dag.TipsCount(); n != len(want) {
		t.Fatalf("TipsCount = %d, erwartet %d", n, len(want))
	}
	// Die Spitzen selbst bleiben stehen -- nur Meldung und Eltern lassen sie aus.
	if !dag.tips["tot"] || !dag.tips["tot2"] {
		t.Fatal("tote Spitzen aus dag.tips entfernt -- erwartet: nur ausgelassen")
	}
}

// Gemeldete Spitzen und Eltern eines eigenen Blocks folgen derselben Regel:
// was GetTips meldet, ist (bis zur Kappung) genau, was ProduceBlock als
// Eltern nimmt.
func TestSpitzen_GemeldetGleichElternAuswahl(t *testing.T) {
	dag := spitzenTestDAG(10)
	dag.mu.RLock()
	eltern := dag.elternAusSpitzenLocked(10)
	dag.mu.RUnlock()
	gemeldet := dag.GetTips()
	sort.Strings(gemeldet)
	if !reflect.DeepEqual(eltern, gemeldet) {
		t.Fatalf("Eltern %v, gemeldet %v -- verschiedene Regeln", eltern, gemeldet)
	}
	for _, h := range eltern {
		if h == "tot" || h == "tot2" {
			t.Fatalf("toter Zweig %s als Elternteil gewaehlt", h)
		}
	}
}

// Ohne finalisierten Checkpoint (frische Kette) ist keine Spitze tot.
func TestSpitzen_OhneFinalitaetAlleGemeldet(t *testing.T) {
	dag := spitzenTestDAG(0)
	if n := dag.TipsCount(); n != 5 {
		t.Fatalf("TipsCount ohne Finalitaet = %d, erwartet 5", n)
	}
	dag.state = nil
	if n := len(dag.GetTips()); n != 5 {
		t.Fatalf("GetTips ohne Zustand = %d, erwartet 5", n)
	}
}
