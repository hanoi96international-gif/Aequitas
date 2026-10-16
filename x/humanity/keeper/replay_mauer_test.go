package keeper

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// EINE Abweisung darf die Heilung nicht ausloesen -- sie kann ein echt
// ungueltiger Block eines fehlerhaften oder boesartigen Peers sein, und den zu
// verwerfen ist genau richtig. Daraufhin den eigenen Zustand wegzuwerfen waere
// ein Angriffsweg.
func TestReplayMauer_EineAbweisungReichtNicht(t *testing.T) {
	var m replayMauer
	if mauer, _ := m.merkeBlockAbweisung("a", 100); mauer {
		t.Fatal("eine einzelne Abweisung darf keine Mauer sein")
	}
}

// Derselbe Block dreimal hintereinander: Schwelle erreicht.
func TestReplayMauer_DerselbeBlockDreimalIstDieMauer(t *testing.T) {
	var m replayMauer
	for i := 1; i < replayMauerSchwelle; i++ {
		if mauer, _ := m.merkeBlockAbweisung("x", 5517970); mauer {
			t.Fatalf("Mauer schon bei Versuch %d gemeldet, Schwelle ist %d", i, replayMauerSchwelle)
		}
	}
	mauer, folgen := m.merkeBlockAbweisung("x", 5517970)
	if !mauer {
		t.Fatalf("nach %d Abweisungen desselben Blocks muss die Schwelle erreicht sein", replayMauerSchwelle)
	}
	if folgen != replayMauerSchwelle {
		t.Fatalf("folgen=%d, erwartet %d", folgen, replayMauerSchwelle)
	}
}

// MISSBRAUCH: ein zugelassener Erzeuger schiebt verschiedene ungueltige
// Bloecke DERSELBEN Hoehe. Bis 16.10.2026 zaehlte die Mauer je Hoehe, und
// drei solche Pushes loesten Heilung samt os.Exit aus.
func TestReplayMauer_VerschiedeneBloeckeDerselbenHoeheSindKeineMauer(t *testing.T) {
	var m replayMauer
	for i := 0; i < 10*replayMauerSchwelle; i++ {
		if mauer, folgen := m.merkeBlockAbweisung(fmt.Sprintf("boese-%d", i), 777); mauer {
			t.Fatalf("Block %d derselben Hoehe loeste die Mauer aus (folgen=%d) -- "+
				"verschiedene Bloecke duerfen sich nicht summieren", i, folgen)
		}
	}
}

// Ein erfolgreicher Block loescht die Zaehlung: der Knoten kommt voran.
func TestReplayMauer_ErfolgLoeschtDieZaehlung(t *testing.T) {
	var m replayMauer
	m.merkeBlockAbweisung("x", 500)
	m.merkeBlockAbweisung("x", 500)
	m.merkeBlockErfolg()
	if mauer, folgen := m.merkeBlockAbweisung("x", 500); mauer || folgen != 1 {
		t.Fatalf("nach einem Erfolg muss neu gezaehlt werden, bekam mauer=%v folgen=%d", mauer, folgen)
	}
}

// MISSBRAUCH: eine Flut verschiedener Bloecke darf den Zaehler weder ohne
// Grenze wachsen lassen noch einen festsitzenden Block verdraengen, der schon
// kurz vor der Schwelle steht.
func TestReplayMauer_ZaehlungIstBegrenzt(t *testing.T) {
	var m replayMauer
	for i := 1; i < replayMauerSchwelle; i++ {
		m.merkeBlockAbweisung("fest", 1)
	}
	for i := 0; i < 50*replayMauerMaxBloecke; i++ {
		m.merkeBlockAbweisung(fmt.Sprintf("flut-%d", i), 2)
		if n := len(m.folgen); n > replayMauerMaxBloecke {
			t.Fatalf("Zaehlung hat %d Eintraege, Grenze %d", n, replayMauerMaxBloecke)
		}
	}
	if mauer, folgen := m.merkeBlockAbweisung("fest", 1); !mauer {
		t.Fatalf("festsitzender Block wurde von der Flut verdraengt (folgen=%d)", folgen)
	}
}

// Zwei Knoten im selben Prozess zaehlen getrennt (Vier-Knoten-Test).
func TestReplayMauer_JeKnoten(t *testing.T) {
	a, b := newOrphanTestDAG(), newOrphanTestDAG()
	for i := 1; i < replayMauerSchwelle; i++ {
		a.replayMauer.merkeBlockAbweisung("x", 9)
	}
	if mauer, folgen := b.replayMauer.merkeBlockAbweisung("x", 9); mauer || folgen != 1 {
		t.Fatalf("Knoten b zaehlt die Abweisungen von a mit (folgen=%d)", folgen)
	}
}

// Wann ist der Block belegt kanonisch? Nur vom Seed oder mit einer Waise
// eines ANDEREN Erzeugers.
func TestReplayMauer_Beleg(t *testing.T) {
	const boese, ehrlich = "0xB0E5E00000000000000000000000000000000001", "0xE4C1100000000000000000000000000000000002"
	dag := newOrphanTestDAG()
	b := &Block{Hash: "h-mauer", Height: 10, Proposer: boese}

	if ok, _ := dag.mauerBelegt(b); ok {
		t.Fatal("gepushter Block ohne Waise gilt als belegt")
	}
	// MISSBRAUCH: der Angreifer baut selbst ein Kind darauf.
	dag.orphans[b.Hash] = []*Block{{Hash: "kind-boese", Height: 11, Proposer: strings.ToLower(boese)}}
	if ok, _ := dag.mauerBelegt(b); ok {
		t.Fatal("Waise desselben Erzeugers (auch in anderer Schreibweise) gilt als Beleg")
	}
	dag.orphans[b.Hash] = append(dag.orphans[b.Hash], &Block{Hash: "kind-ehrlich", Height: 11, Proposer: ehrlich})
	if ok, beleg := dag.mauerBelegt(b); !ok {
		t.Fatal("Waise eines anderen Erzeugers muss als Beleg gelten")
	} else {
		t.Logf("Beleg: %s", beleg)
	}

	vomSeed := &Block{Hash: "h-seed", Height: 10, Proposer: boese, FromSync: true}
	if ok, _ := dag.mauerBelegt(vomSeed); !ok {
		t.Fatal("Block vom Seed muss als Beleg gelten")
	}
	selbstGeholt := &Block{Hash: "h-selbst", Height: 10, Proposer: boese, SelfFetched: true}
	if ok, _ := dag.mauerBelegt(selbstGeholt); ok {
		t.Fatal("SelfFetched allein ist kein Beleg -- geholt wird auch fuer das Kind eines Angreifers")
	}
}

// MISSBRAUCH Ende-zu-Ende ueber loeseHeilungAus: ohne Beleg keine Heilung,
// mit Beleg genau eine.
func TestReplayMauer_HeiltNurMitBeleg(t *testing.T) {
	geheilt := make(chan string, 4)
	vorher := replayMauerHeilen
	replayMauerHeilen = func(_ *BlockDAG, grund string) { geheilt <- grund }
	defer func() { replayMauerHeilen = vorher }()

	dag := newOrphanTestDAG()
	boeser := &Block{Hash: "boese-0123456789abcdef", Height: 20, Proposer: "0xb0e5e"}
	for i := 0; i < 3*replayMauerSchwelle; i++ {
		if mauer, folgen := dag.replayMauer.merkeBlockAbweisung(boeser.Hash, boeser.Height); mauer {
			dag.loeseHeilungAus(boeser, folgen)
		}
	}
	select {
	case g := <-geheilt:
		t.Fatalf("Heilung ohne Beleg ausgeloest: %s", g)
	case <-time.After(300 * time.Millisecond):
	}

	kanonisch := &Block{Hash: "seed-0123456789abcdef", Height: 21, Proposer: "0xe4c11", FromSync: true}
	for i := 0; i < replayMauerSchwelle; i++ {
		if mauer, folgen := dag.replayMauer.merkeBlockAbweisung(kanonisch.Hash, kanonisch.Height); mauer {
			dag.loeseHeilungAus(kanonisch, folgen)
		}
	}
	select {
	case g := <-geheilt:
		if !strings.Contains(g, "#21") {
			t.Fatalf("falscher Grund: %s", g)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("festsitzender Block vom Seed hat keine Heilung ausgeloest")
	}
}
