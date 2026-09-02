package keeper

import (
	"sync"
	"time"
)

// /api/status muss antworten, auch wenn die Sperren belegt sind.
//
// # WAS PASSIERT IST
//
// Am 02.09.2026 meldete der Betreiber, C1 und C2 wuerden "beim Lasttest
// abstuerzen". Nachgemessen: RestartCount 0, ExitCode 0, OOMKilled false,
// Speicher bei 34 %. Nichts war abgestuerzt. Der Container lief seit zwanzig
// Minuten, die Logs waren voller erfolgreich angehaengter Bloecke, Tips 1 --
// und trotzdem kam auf /api/status keine Antwort, weder von aussen noch aus
// dem Container heraus. Auch die Deploy-Pruefung fiel darauf herein und
// meldete "the node answers but its height never moved in 10 minutes".
//
// # WARUM
//
// handleStatus braucht LatestBlock() und StatusMetrics(). Beide nehmen einen
// RLock -- auf dag.mu beziehungsweise cs.mu. Waehrend ein Block-Burst
// angewendet wird, haelt der Knoten die zugehoerige SCHREIBsperre, und Go's
// RWMutex sperrt jeden ankommenden Leser aus, sobald ein Schreiber ansteht.
// Die Statusabfrage stellt sich also hinter das Nachspielen von tausend
// Bloecken -- und wer von aussen zusieht, sieht einen toten Knoten.
//
// Das ist dieselbe Aussperrung, die im Ueberweisungspfad 69 % der Zeit
// gefressen hat. Dort war sie ein Durchsatzproblem. Hier ist sie schlimmer:
// die Ueberwachung wird blind, und zwar genau in dem Moment, in dem man sie
// braucht.
//
// # WIE ES BEHOBEN IST
//
// Der Statuspfad versucht die Sperre mit TryRLock und gibt sofort auf, statt
// sich anzustellen. Klappt es, wird die Antwort gebaut UND als letzter guter
// Stand gemerkt. Klappt es nicht, wird dieser Stand ausgeliefert -- mit der
// sperrfreien Hoehe aus dag.heightSchnell und einem ehrlichen Vermerk, dass
// die Zahlen einen Moment alt sind.
//
// Eine veraltete Antwort ist einer ausbleibenden weit ueberlegen: sie sagt
// "der Knoten lebt und ist beschaeftigt" statt gar nichts, und genau diese
// Unterscheidung hat hier zwei Tage gekostet.
type letzterGuterStatus struct {
	mu       sync.Mutex
	da       bool
	stand    map[string]interface{}
	gebautAm time.Time
}

var statusZwischenspeicher letzterGuterStatus

// merken legt den zuletzt vollstaendig gebauten Status ab.
func (l *letzterGuterStatus) merken(m map[string]interface{}) {
	kopie := make(map[string]interface{}, len(m))
	for k, v := range m {
		kopie[k] = v
	}
	l.mu.Lock()
	l.stand, l.da, l.gebautAm = kopie, true, time.Now()
	l.mu.Unlock()
}

// holen liefert den letzten guten Stand, angereichert um die sperrfreie
// Hoehe und einen Vermerk ueber sein Alter.
func (l *letzterGuterStatus) holen(hoeheJetzt int64) (map[string]interface{}, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.da {
		return nil, false
	}
	aus := make(map[string]interface{}, len(l.stand)+3)
	for k, v := range l.stand {
		aus[k] = v
	}
	// Die Hoehe ist das Einzige, was auch waehrend der Sperre aktuell zu
	// haben ist -- und genau die Zahl, an der jeder ablesen will, ob der
	// Knoten noch vorankommt.
	aus["height"] = hoeheJetzt
	aus["stand_veraltet"] = true
	aus["stand_alter_sekunden"] = int(time.Since(l.gebautAm).Seconds())
	aus["stand_hinweis"] = "Der Knoten ist beschaeftigt (Block-Burst haelt die Schreibsperre). " +
		"height ist aktuell, die uebrigen Zahlen sind vom letzten freien Zeitpunkt. " +
		"Keine Antwort waere hier die schlechtere Auskunft"
	return aus, true
}

// TryLatestBlock ist LatestBlock, das sich nicht anstellt.
func (dag *BlockDAG) TryLatestBlock() (*Block, bool) {
	if !dag.mu.TryRLock() {
		return nil, false
	}
	defer dag.mu.RUnlock()
	var latest *Block
	for hash := range dag.tips {
		b := dag.blocks[hash]
		if b == nil {
			continue
		}
		if latest == nil || b.BlueScore > latest.BlueScore ||
			(b.BlueScore == latest.BlueScore && b.Hash < latest.Hash) {
			latest = b
		}
	}
	return latest, true
}
