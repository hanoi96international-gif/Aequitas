package keeper

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// Wenn derselbe Block immer wieder abgewiesen wird, ist der Knoten zugemauert.
//
// # DAS BEOBACHTETE VERHALTEN
//
// Am 02.09.2026 mehrfach am PRIMARY gesehen. Beim Nachspielen scheitert eine
// einzelne Ueberweisung:
//
//	[REPLAY] ✗ Transfer 0x…->0x… 0.000010: insufficient balance
//	         (have 0.000000 after demurrage, need 0.000010) (block #5517970)
//	         — rolling back whole block
//	[REPLAY] ✗ Block #5517970 rolled back due to a genuine state-inconsistency
//	         failure — block rejected
//
// Einmal beobachtet mit 3.525 Transaktionen im Block, verworfen wegen einer.
//
// # WARUM DAS NICHT VON SELBST HEILT
//
// Mit dem verworfenen Block fehlen dessen Gutschriften dauerhaft. Jeder
// spaetere Block, der dieselben Konten beruehrt, scheitert damit ebenso -- der
// Knoten wiederholt die Abweisung endlos, sammelt Waisen (61 gesehen) und
// faellt weiter zurueck (1.425 Bloecke). Er kann sich nicht selbst
// herausarbeiten; nur ein Resync bringt ihn zurueck.
//
// Die vorhandenen Waechter merken das erst spaet: die Aushunger-Erkennung
// braucht 5 Minuten, der Divergenz-Vergleich schaltete sich bei
// Tip-Fragmentierung ganz ab, und die Resync-Sperre stand auf 30 Minuten. Am
// Primary, der Website und Explorer traegt, ist das viel zu lang.
//
// # WARUM ERST BEI WIEDERHOLUNG
//
// Eine EINZELNE Abweisung ist kein Beweis: sie kann ein echt ungueltiger Block
// eines fehlerhaften oder boesartigen Peers sein, und den zu verwerfen ist
// genau richtig -- daraufhin den eigenen Zustand wegzuwerfen waere ein
// Angriffsweg. Dreimal DERSELBE Block heisst dagegen, dass der Knoten an ihm
// nicht vorbeikommt.
//
// # WARUM JE BLOCK UND NUR MIT BELEG (Pruefung 16.10.2026)
//
// Bis dahin zaehlte die Mauer je HOEHE, paketweit, und loeste bei drei
// Abweisungen ohne weiteren Beleg die endgueltige Heilung samt os.Exit aus
// (autoheal.go). Das liess sich von aussen ausloesen:
//
//   - Ein zugelassener, boesartiger Erzeuger schob drei verschiedene
//     ungueltige Bloecke derselben Hoehe -- drei Abweisungen, Heilung.
//   - Steckt der Knoten ohnehin (kein Block gelingt, also keine Ruecksetzung),
//     genuegte jede dritte Abweisung irgendeines Blocks dieser Hoehe -- auch
//     eines Blocks, den nur der Angreifer fuehrt.
//
// Die Heilung ersetzt den Zustand aus dem Snapshot und startet den Knoten neu,
// alle autoHealFailedResyncRetry (3 min) wieder -- ein Erzeuger konnte damit
// jeden Knoten dauerhaft in den Neustart treiben.
//
// Darum jetzt zweierlei:
//
//  1. Gezaehlt wird je Block-Hash (hoechstens replayMauerMaxBloecke Eintraege
//     je Knoten). Verschiedene Bloecke derselben Hoehe summieren sich nicht.
//  2. Die Wiederholung allein beweist nur, dass DIESER Knoten den Block nicht
//     annimmt -- nicht, dass das Netz ihn angenommen hat. Geheilt wird nur
//     mit Beleg, dass der Block kanonisch ist (mauerBelegt):
//     - er kam per HTTP-Sync von einem eingetragenen Seed (FromSync): der
//     Seed fuehrt ihn in seiner eigenen Kette, oder
//     - eine Waise eines ANDEREN Erzeugers wartet auf ihn: ein zweiter,
//     zugelassener Erzeuger hat ihn nachgespielt und darauf gebaut.
//     Waisen entstehen erst hinter Signatur-, Erzeuger-, Finalitaets- und
//     Sperrpruefung; die Unterschrift eines fremden Erzeugers kann der
//     Angreifer nicht faelschen.
//     Ohne Beleg bleibt es bei der Abweisung; die uebrigen Waechter
//     (Aushunger-Erkennung, Divergenz-Vergleich) bleiben zustaendig.
//
// Die Zaehlung wird bei jedem erfolgreichen Nachspielen zurueckgesetzt -- sie
// misst ausschliesslich das Feststecken.
const replayMauerSchwelle = 3

// replayMauerMaxBloecke begrenzt die Zaehlung je Knoten. Ist sie voll, faellt
// der Eintrag mit der kleinsten Zahl heraus -- ein festsitzender Block, der
// schon nahe an der Schwelle steht, wird von neuen Eintraegen nicht verdraengt.
const replayMauerMaxBloecke = 64

type replayMauer struct {
	mu     sync.Mutex
	folgen map[string]int // Block-Hash -> Abweisungen seit dem letzten Erfolg
	// Fuer die Anzeige: der zuletzt abgewiesene Block.
	letzteHoehe  int64
	letzteFolgen int
	// abgewehrt: Schwelle erreicht, aber ohne Beleg -- keine Heilung. Bleibt
	// ueber Erfolge hinweg stehen; steigt er, schiebt jemand Bloecke, die
	// nur dieser Knoten nicht annimmt.
	abgewehrt atomic.Int64
	// abweisungen: alle Abweisungen seit dem Start, nie zurueckgesetzt.
	abweisungen atomic.Int64
}

// merkeBlockAbweisung zaehlt eine Abweisung dieses Blocks und meldet, ob er
// die Schwelle erreicht hat. Rein rechnerisch, keine Sperre ausser der
// eigenen -- der Aufrufer haelt die globale Zustandssperre, dort darf nichts
// Schweres passieren.
func (m *replayMauer) merkeBlockAbweisung(hash string, hoehe int64) (mauer bool, folgen int) {
	m.abweisungen.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.folgen == nil {
		m.folgen = make(map[string]int)
	}
	if _, bekannt := m.folgen[hash]; !bekannt && len(m.folgen) >= replayMauerMaxBloecke {
		kleinster, kleinsteZahl := "", int(^uint(0)>>1)
		for h, f := range m.folgen {
			if f < kleinsteZahl || (f == kleinsteZahl && h < kleinster) {
				kleinster, kleinsteZahl = h, f
			}
		}
		delete(m.folgen, kleinster)
	}
	m.folgen[hash]++
	folgen = m.folgen[hash]
	m.letzteHoehe, m.letzteFolgen = hoehe, folgen
	return folgen >= replayMauerSchwelle, folgen
}

// merkeBlockErfolg loescht die Zaehlung -- der Knoten kommt voran.
func (m *replayMauer) merkeBlockErfolg() {
	m.mu.Lock()
	if len(m.folgen) != 0 || m.letzteFolgen != 0 {
		m.folgen = nil
		m.letzteHoehe, m.letzteFolgen = 0, 0
	}
	m.mu.Unlock()
}

// ReplayMauerStand zeigt den Zaehler in /api/health/combined.
func (dag *BlockDAG) ReplayMauerStand() map[string]interface{} {
	var h, abgewehrt, abweisungen int64
	var f, n int
	if dag != nil {
		abgewehrt = dag.replayMauer.abgewehrt.Load()
		abweisungen = dag.replayMauer.abweisungen.Load()
		dag.replayMauer.mu.Lock()
		h, f, n = dag.replayMauer.letzteHoehe, dag.replayMauer.letzteFolgen, len(dag.replayMauer.folgen)
		dag.replayMauer.mu.Unlock()
	}
	return map[string]interface{}{
		"hoehe":       h,
		"folgen":      f,
		"bloecke":     n,
		"schwelle":    replayMauerSchwelle,
		"abgewehrt":   abgewehrt,
		"abweisungen": abweisungen,
		"bedeutung": "Wie oft derselbe Block (hoehe = zuletzt abgewiesener) seit dem letzten Erfolg " +
			"beim Nachspielen abgewiesen wurde. Ab der Schwelle -- und nur mit Beleg, dass der Block " +
			"kanonisch ist (vom Seed oder von einem anderen Erzeuger bebaut) -- loest der Knoten die " +
			"Heilung selbst aus. folgen>0 heisst: er kommt gerade nicht weiter. abgewehrt zaehlt " +
			"Bloecke an der Schwelle ohne diesen Beleg (keine Heilung) -- steigt er, schiebt ein Erzeuger " +
			"Bloecke, die das Netz nicht fuehrt",
	}
}

// mauerBelegt: haelt das Netz diesen Block fuer kanonisch? Siehe oben. Nimmt
// orphansMu -- nur ausserhalb der Zustandssperre aufrufen.
func (dag *BlockDAG) mauerBelegt(b *Block) (bool, string) {
	if b.FromSync {
		return true, "vom Seed per Sync geliefert"
	}
	dag.orphansMu.Lock()
	defer dag.orphansMu.Unlock()
	for _, w := range dag.orphans[b.Hash] {
		if w != nil && !strings.EqualFold(w.Proposer, b.Proposer) {
			return true, fmt.Sprintf("Waise #%d von %s wartet darauf", w.Height, kurzAdresse(w.Proposer))
		}
	}
	return false, ""
}

// loeseHeilungAus prueft den Beleg und startet die Selbstheilung -- beides
// ausserhalb der gehaltenen Sperre.
func (dag *BlockDAG) loeseHeilungAus(b *Block, folgen int) {
	hoehe, hash, erzeuger := b.Height, b.Hash, b.Proposer
	SafeGoroutine("replayMauerHeilung", func() {
		belegt, beleg := dag.mauerBelegt(b)
		if !belegt {
			dag.replayMauer.abgewehrt.Add(1)
			fmt.Printf("[REPLAY] Block #%d (%s...) von %s %d-mal abgewiesen, aber ohne Beleg, dass das Netz "+
				"ihn fuehrt -- keine Heilung, der Block bleibt abgewiesen\n",
				hoehe, hash[:min(16, len(hash))], kurzAdresse(erzeuger), folgen)
			return
		}
		grund := fmt.Sprintf("block #%d rejected %d times in a row on replay (%s) — this node cannot "+
			"get past it and will not recover on its own", hoehe, folgen, beleg)
		fmt.Printf("[REPLAY] ⛑ %s — triggering self-heal\n", grund)
		// Endgueltig: derselbe Block scheitert deterministisch, kein Zuwarten
		// aendert daran etwas. Die lange Sperre haette hier nur den Stillstand
		// verlaengert -- am 02.09.2026 live um 17 Minuten.
		replayMauerHeilen(dag, grund)
	})
}

// replayMauerHeilen: die endgueltige Heilung. Variable nur, damit Tests den
// os.Exit am Ende durch einen Zaehler ersetzen koennen.
var replayMauerHeilen = func(dag *BlockDAG, grund string) {
	dag.triggerAutoResyncEndgueltig(grund)
}
