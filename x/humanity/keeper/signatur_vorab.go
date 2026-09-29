package keeper

import (
	"sync"
	"sync/atomic"
	"time"
)

// Signaturpruefung eines Peer-Blocks VOR und NEBEN dem Nachspielen.
//
// # WARUM
//
// Gemessen am 29.09.2026 auf C2 unter Last (replay_phasen): 62 % des
// mittleren Replay-Halts war die Signaturpruefung (57 von 92 ms), im
// schlimmsten Block 953 von 1.698 ms fuer 7.000 Ueberweisungen. Sie lief
// unter der globalen Exklusivsperre cs.mu und, schlimmer, STRENG NACH dem
// vorigen Block: der Nachspielende arbeitet Bloecke nacheinander ab, also
// addierten sich Pruefung und Anwendung je Block. Die Peer-Lag-Bremse auf C1
// verkleinert die Bloecke, sobald C2 zurueckfaellt -- diese Summe war damit
// die Decke fuer die Ketten-TPS.
//
// Die Pruefung braucht keinen Zustand: sie haengt nur an block.Transactions
// und block.Timestamp (pruefeUeberweisungenImBlock, pruefeAuftraegeImBlock).
// Also startet sie, sobald ein Block eintrifft (AddPeerBlock) oder als
// Vorfahr zum Nachspielen ansteht, in einer eigenen Goroutine; das
// Nachspielen holt das Ergebnis vor der Sperre ab und wartet nur, falls es
// noch nicht fertig ist. Waehrend Block N angewendet wird, prueft Block N+1
// schon seine Signaturen.
//
// # WAS SICH NICHT AENDERT
//
// Jede Signatur wird weiterhin von diesem Knoten selbst geprueft, fuer jeden
// Block, und ein Fehler macht den Block wie bisher ungueltig. Nur der
// Zeitpunkt ist frueher. Schluessel ist der *Block-Zeiger selbst, nicht der
// Hash: ein Ergebnis gilt nur fuer genau das Objekt, dessen Transaktionen
// geprueft wurden. Die Nonce-Pruefung braucht den Zustand und bleibt unter
// der Sperre.
type signaturVorab struct {
	fertig         chan struct{}
	gestartet      time.Time
	ueberweisungen []ueberweisungsNonce
	ueberwErr      error
	auftraege      []auftragsNonce
	auftragErr     error
	dauer          time.Duration
}

// Grenze fuer gleichzeitig vorgehaltene Ergebnisse. Ein Block, der nie
// nachgespielt wird (abgelehnt, doppelt), bliebe sonst liegen; alte Eintraege
// werden beim Einfuegen weggeraeumt.
const (
	signaturVorabGrenze = 256
	signaturVorabAlter  = 2 * time.Minute
)

var (
	signaturVorabMu  sync.Mutex
	signaturVorabMap = map[*Block]*signaturVorab{}

	svGestartet  atomic.Int64 // Pruefungen, die vorab starteten
	svGetroffen  atomic.Int64 // vom Nachspielen abgeholt (fertig oder laufend)
	svInline     atomic.Int64 // keine Vorabpruefung da -- im Nachspielen gerechnet
	svWartenNano atomic.Int64 // wie lange das Nachspielen noch warten musste
)

func pruefeSignaturenJetzt(block *Block) *signaturVorab {
	v := &signaturVorab{fertig: make(chan struct{}), gestartet: time.Now()}
	v.rechnen(block)
	return v
}

func (v *signaturVorab) rechnen(block *Block) {
	defer close(v.fertig)
	start := time.Now()
	v.ueberweisungen, v.ueberwErr = pruefeUeberweisungenImBlock(block.Transactions)
	v.auftraege, v.auftragErr = pruefeAuftraegeImBlock(block.Transactions, block.Timestamp)
	v.dauer = time.Since(start)
}

// starteSignaturVorpruefung beginnt die Pruefung im Hintergrund, falls fuer
// diesen Block Signaturpflicht gilt und noch keine laeuft. Billig und
// jederzeit aufrufbar, auch mehrfach.
func starteSignaturVorpruefung(block *Block) {
	if block == nil || len(block.Transactions) == 0 || !signierteUeberweisungenPflicht(block.Timestamp) {
		return
	}
	signaturVorabMu.Lock()
	if _, da := signaturVorabMap[block]; da {
		signaturVorabMu.Unlock()
		return
	}
	if len(signaturVorabMap) >= signaturVorabGrenze {
		grenze := time.Now().Add(-signaturVorabAlter)
		for b, v := range signaturVorabMap {
			if v.gestartet.Before(grenze) {
				delete(signaturVorabMap, b)
			}
		}
		if len(signaturVorabMap) >= signaturVorabGrenze {
			signaturVorabMu.Unlock()
			return // voll: das Nachspielen rechnet selbst
		}
	}
	v := &signaturVorab{fertig: make(chan struct{}), gestartet: time.Now()}
	signaturVorabMap[block] = v
	signaturVorabMu.Unlock()
	svGestartet.Add(1)
	go v.rechnen(block)
}

// holeSignaturVorpruefung liefert das Ergebnis fuer genau diesen Block --
// aus der Vorabpruefung, oder jetzt gerechnet. Nie unter cs.mu aufrufen: im
// schlimmsten Fall wartet es eine volle Pruefung ab.
func holeSignaturVorpruefung(block *Block) *signaturVorab {
	signaturVorabMu.Lock()
	v, da := signaturVorabMap[block]
	delete(signaturVorabMap, block)
	signaturVorabMu.Unlock()
	if !da {
		svInline.Add(1)
		return pruefeSignaturenJetzt(block)
	}
	svGetroffen.Add(1)
	start := time.Now()
	<-v.fertig
	svWartenNano.Add(int64(time.Since(start)))
	return v
}

// SignaturVorabStand zeigt in /api/health/combined, ob die Vorabpruefung
// greift: getroffen gross und warten_ms klein heisst, die Signaturen sind
// fertig, bevor der Block an der Reihe ist.
func SignaturVorabStand() map[string]interface{} {
	g := svGetroffen.Load()
	wartenMs := float64(0)
	if g > 0 {
		wartenMs = float64(svWartenNano.Load()) / float64(g) / 1e6
	}
	signaturVorabMu.Lock()
	offen := len(signaturVorabMap)
	signaturVorabMu.Unlock()
	return map[string]interface{}{
		"gestartet":          svGestartet.Load(),
		"getroffen":          g,
		"inline":             svInline.Load(),
		"warten_je_block_ms": wartenMs,
		"offen":              offen,
		"bedeutung": "Signaturpruefung der Peer-Bloecke, vorab und ausserhalb der globalen Sperre. getroffen: Ergebnis lag bereit oder lief schon. " +
			"inline: keine Vorabpruefung, das Nachspielen rechnete selbst (vor der Sperre). warten_je_block_ms: Rest, auf den das Nachspielen noch warten musste.",
	}
}
