package keeper

import (
	"fmt"
	"sync/atomic"
)

// Wenn der Peer die fehlenden Eltern nicht liefern KANN, ist Warten sinnlos.
//
// # DER BEFUND, DER DAS AUSLOEST
//
// Am 02.09.2026 bis zur Wurzel verfolgt. Ein zurueckgefallener Knoten sammelt
// Waisen -- 425 Meldungen "queued as orphan — missing parent" in 90 Sekunden --
// und kommt nie wieder heran. Der Grund steht in der Datenbank des Peers:
//
//	C1: 580 Bloecke   (5521232 - 5521811)
//	C2:  21 Bloecke   (5521216 - 5521232)
//
// Beide halten nur die letzten Minuten. Der Rest ist weg, weil jeder
// Snapshot-Resync `TRUNCATE chain_blocks` ausfuehrt (snapshot.go) -- fuer die
// Zustandstabellen richtig, fuer das Blockprotokoll ein Nebeneffekt.
//
// Folge: fragt ein zurueckgefallener Knoten nach einem Elternblock, antwortet
// der Peer `{"error":"block not found"}`. Die Waise ist damit NIE aufloesbar.
// Der Knoten sammelt weiter, faellt weiter zurueck, und nur ein eigener Resync
// bringt ihn zurueck -- der ihm dann selbst die Historie nimmt. Ein
// Kreislauf, der sich selbst traegt.
//
// # WAS HIER GETAN WIRD
//
// Nicht die Ursache -- die liegt im TRUNCATE und in der fehlenden
// Historie-Nachlieferung, und beides gehoert in eine eigene, ruhige
// Aenderung. Hier wird nur die AUSSICHTSLOSIGKEIT erkannt: liefert der Peer
// ueber mehrere Runden hinweg KEINEN einzigen der angefragten Eltern, waehrend
// welche ausstehen, dann wird dieser Knoten so nicht aufholen. Statt weiter
// Waisen zu sammeln, loest er die Heilung aus.
//
// # WARUM MEHRERE RUNDEN
//
// Eine leere Antwort ist harmlos: der Peer hat den Block vielleicht noch
// nicht. Mehrere Runden hintereinander ohne einen einzigen Treffer heissen,
// dass er ihn nicht hat und nicht bekommen wird. Jeder erfolgreiche Abruf
// setzt zurueck.
const ahnenLeerlaufSchwelle = 5

var ahnenLeerlaufFolgen atomic.Int64

// merkeAhnenLeerlauf zaehlt eine Runde ohne einen einzigen beschafften
// Vorfahren und meldet, ob die Lage aussichtslos ist.
func merkeAhnenLeerlauf() (aussichtslos bool, folgen int64) {
	f := ahnenLeerlaufFolgen.Add(1)
	return f >= ahnenLeerlaufSchwelle, f
}

// merkeAhnenErfolg setzt zurueck -- der Peer liefert.
func merkeAhnenErfolg() { ahnenLeerlaufFolgen.Store(0) }

// AhnenLeerlaufStand zeigt den Zaehler in /api/health/combined.
func AhnenLeerlaufStand() map[string]interface{} {
	return map[string]interface{}{
		"folgen":   ahnenLeerlaufFolgen.Load(),
		"schwelle": ahnenLeerlaufSchwelle,
		"bedeutung": "Runden hintereinander, in denen der Peer KEINEN der angefragten " +
			"Elternbloecke liefern konnte. Ab der Schwelle gilt das Aufholen als " +
			"aussichtslos und die Heilung wird ausgeloest -- weiter zu warten haette " +
			"keinen Zweck, weil der Peer die Historie gar nicht mehr hat",
	}
}

// loeseHeilungWegenUnerreichbarerAhnenAus startet die Selbstheilung.
func (dag *BlockDAG) loeseHeilungWegenUnerreichbarerAhnenAus(nodeURL string, folgen int64) {
	grund := fmt.Sprintf("%s returned none of the requested ancestor blocks in %d rounds "+
		"while orphans keep arriving — it no longer holds the history this node needs, "+
		"so catching up from it is impossible", nodeURL, folgen)
	fmt.Printf("[HTTP-SYNC] ⛑ %s — triggering self-heal\n", grund)
	SafeGoroutine("ahnenLeerlaufHeilung", func() { dag.triggerAutoResync(grund) })
}
