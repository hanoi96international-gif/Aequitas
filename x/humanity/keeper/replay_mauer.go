package keeper

import (
	"fmt"
	"sync"
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
// Angriffsweg. Dreimal DIESELBE Hoehe hintereinander heisst dagegen, dass der
// Knoten nicht weiterkommt, und das ist die Mauer.
//
// Die Zaehlung wird bei jedem erfolgreichen Nachspielen und bei jeder anderen
// Hoehe zurueckgesetzt -- sie misst ausschliesslich das Feststecken.
const replayMauerSchwelle = 3

type replayMauer struct {
	mu     sync.Mutex
	hoehe  int64
	folgen int
}

var replayMauerStand replayMauer

// merkeBlockAbweisung zaehlt eine Abweisung und meldet, ob die Mauer erreicht
// ist. Rein rechnerisch, keine Sperre ausser der eigenen -- der Aufrufer haelt
// die globale Zustandssperre, dort darf nichts Schweres passieren.
func merkeBlockAbweisung(hoehe int64) (mauer bool, folgen int) {
	replayMauerStand.mu.Lock()
	defer replayMauerStand.mu.Unlock()
	if replayMauerStand.hoehe != hoehe {
		replayMauerStand.hoehe = hoehe
		replayMauerStand.folgen = 0
	}
	replayMauerStand.folgen++
	return replayMauerStand.folgen >= replayMauerSchwelle, replayMauerStand.folgen
}

// merkeBlockErfolg loescht die Zaehlung -- der Knoten kommt voran.
func merkeBlockErfolg() {
	replayMauerStand.mu.Lock()
	if replayMauerStand.folgen != 0 {
		replayMauerStand.hoehe, replayMauerStand.folgen = 0, 0
	}
	replayMauerStand.mu.Unlock()
}

// ReplayMauerStand zeigt den Zaehler in /api/health/combined.
func ReplayMauerStand() map[string]interface{} {
	replayMauerStand.mu.Lock()
	h, f := replayMauerStand.hoehe, replayMauerStand.folgen
	replayMauerStand.mu.Unlock()
	return map[string]interface{}{
		"hoehe":    h,
		"folgen":   f,
		"schwelle": replayMauerSchwelle,
		"bedeutung": "Wie oft derselbe Block hintereinander beim Nachspielen abgewiesen wurde. " +
			"Ab der Schwelle gilt der Knoten als zugemauert und loest die Heilung selbst aus, " +
			"statt auf die Waechter zu warten. folgen>0 heisst: er kommt gerade nicht weiter",
	}
}

// loeseHeilungAus startet die Selbstheilung ausserhalb der gehaltenen Sperre.
func (dag *BlockDAG) loeseHeilungAus(hoehe int64, folgen int) {
	grund := fmt.Sprintf("block #%d rejected %d times in a row on replay — this node cannot "+
		"get past it and will not recover on its own", hoehe, folgen)
	fmt.Printf("[REPLAY] ⛑ %s — triggering self-heal\n", grund)
	SafeGoroutine("replayMauerHeilung", func() { dag.triggerAutoResync(grund) })
}
