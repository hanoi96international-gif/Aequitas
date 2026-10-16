package keeper

import (
	"fmt"
	"sync"
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
//
// # WAS ZAEHLT UND WER HEILEN DARF (Pruefung 16.10.2026)
//
// Bis dahin zaehlte ein prozessweiter Zaehler jede Runde ohne Treffer --
// auch Runden, in denen jeder Abruf mit einem Netzfehler scheiterte, und
// ueber alle Peers zusammen. Sync-Peers koennen sich selbst eintragen: ein
// boeswilliger Peer, der fuenfmal leer antwortete, loeste damit die Heilung
// aus (Resync aus dem Snapshot, notfalls os.Exit). Jetzt:
//
//   - Gezaehlt wird nur eine Runde mit mindestens einer vollstaendigen
//     Antwort (kein Netzfehler, nicht abgeschnitten) und keinem Treffer.
//     Netzfehler und abgeschnittene Antworten sagen nichts darueber, ob der
//     Peer die Bloecke hat.
//   - Je Peer, je Knoten (dag.ahnenLeerlauf). Ein Treffer bei Peer A setzt
//     Peer B nicht zurueck.
//   - Gezaehlt wird eine Runde nur, wenn auf einen der angefragten Hashes
//     eine Waise wartet, die selbst von einem eingetragenen Seed kam
//     (FromSync). Hat der Seed das Kind geliefert, fuehrt er den Elternblock
//     -- fehlt er ihm, ist seine Historie wirklich weg. Kinder aus einem
//     Push zaehlen nicht: ein zugelassener Validator, der einen Elternblock
//     zurueckhaelt und nur die Kinder schickt, liess sonst den ehrlichen
//     Seed fuenfmal leer antworten und loeste die Heilung aus (Pruefung von
//     #380, F2).
//   - Heilen nur, wenn ein eingetragener Seed (isTrustedSyncSource) die
//     Historie nicht liefert. Ein anderer Peer ohne Historie ist schlicht
//     kein Nutzen -- daraufhin den eigenen Zustand zu ersetzen, waere ein
//     Angriffsweg.
const ahnenLeerlaufSchwelle = 5

// ahnenLeerlaufMaxPeers begrenzt die Zaehlung je Knoten. Voll: alles neu.
const ahnenLeerlaufMaxPeers = 64

type ahnenLeerlauf struct {
	mu     sync.Mutex
	folgen map[string]int64
}

// merkeLeerlauf zaehlt fuer nodeURL eine Runde ohne einen einzigen
// beschafften Vorfahren und meldet, ob die Schwelle erreicht ist.
func (a *ahnenLeerlauf) merkeLeerlauf(nodeURL string) (aussichtslos bool, folgen int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.folgen == nil || (len(a.folgen) >= ahnenLeerlaufMaxPeers && a.folgen[nodeURL] == 0) {
		a.folgen = make(map[string]int64)
	}
	a.folgen[nodeURL]++
	f := a.folgen[nodeURL]
	return f >= ahnenLeerlaufSchwelle, f
}

// merkeErfolg setzt nodeURL zurueck -- der Peer liefert.
func (a *ahnenLeerlauf) merkeErfolg(nodeURL string) {
	a.mu.Lock()
	delete(a.folgen, nodeURL)
	a.mu.Unlock()
}

// AhnenLeerlaufStand zeigt den Zaehler in /api/health/combined.
func (dag *BlockDAG) AhnenLeerlaufStand() map[string]interface{} {
	var hoechste int64
	je := map[string]int64{}
	if dag != nil {
		dag.ahnenLeerlauf.mu.Lock()
		for u, f := range dag.ahnenLeerlauf.folgen {
			je[u] = f
			if f > hoechste {
				hoechste = f
			}
		}
		dag.ahnenLeerlauf.mu.Unlock()
	}
	return map[string]interface{}{
		"folgen":   hoechste,
		"je_peer":  je,
		"schwelle": ahnenLeerlaufSchwelle,
		"bedeutung": "Runden hintereinander, in denen ein Peer vollstaendig antwortete, aber KEINEN " +
			"der angefragten Elternbloecke liefern konnte (Netzfehler zaehlen nicht). Ab der " +
			"Schwelle gilt das Aufholen von diesem Peer als aussichtslos; geheilt wird nur, " +
			"wenn es ein eingetragener Seed ist",
	}
}

// loeseHeilungWegenUnerreichbarerAhnenAus startet die Selbstheilung -- nur
// fuer einen eingetragenen Seed (siehe oben).
func (dag *BlockDAG) loeseHeilungWegenUnerreichbarerAhnenAus(nodeURL string, folgen int64) {
	if !dag.isTrustedSyncSource(nodeURL) {
		fmt.Printf("[HTTP-SYNC] %s lieferte in %d Runden keinen der angefragten Vorfahren -- "+
			"kein eingetragener Seed, keine Heilung\n", nodeURL, folgen)
		return
	}
	grund := fmt.Sprintf("%s returned none of the requested ancestor blocks in %d rounds "+
		"while orphans keep arriving — it no longer holds the history this node needs, "+
		"so catching up from it is impossible", nodeURL, folgen)
	fmt.Printf("[HTTP-SYNC] ⛑ %s — triggering self-heal\n", grund)
	SafeGoroutine("ahnenLeerlaufHeilung", func() { ahnenLeerlaufHeilen(dag, grund) })
}

// ahnenLeerlaufHeilen: Variable nur, damit Tests den Resync ersetzen.
var ahnenLeerlaufHeilen = func(dag *BlockDAG, grund string) { dag.triggerAutoResync(grund) }

// seedFuehrtWaiseAuf: wartet auf einen dieser Hashes eine Waise, die von
// einem eingetragenen Seed kam? Siehe oben.
func (dag *BlockDAG) seedFuehrtWaiseAuf(hashes []string) bool {
	dag.orphansMu.Lock()
	defer dag.orphansMu.Unlock()
	for _, h := range hashes {
		for _, w := range dag.orphans[h] {
			if w != nil && w.FromSync {
				return true
			}
		}
	}
	return false
}
