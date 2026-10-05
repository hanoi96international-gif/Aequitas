package keeper

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

// ANNAHME NUR, SOLANGE DIE AUFTRAEGE EINES BLOCKS ZEITLICH ZUSAMMENPASSEN
// (05.10.2026, zu block_tauglich.go).
//
// Ein Block traegt EINE Blockzeit, und jeder Auftrag mit Nachweis passt nur
// in ein Fenster um seine Unterschrift. Liegen im Ausgang Auftraege von vor
// einer Stunde neben Auftraegen von jetzt, gibt es keine gemeinsame Zeit --
// dann entsteht kein Block (fail-closed). Damit das bei ehrlichem Betrieb
// nicht vorkommt, haelt der annehmende Knoten die Annahme an,
//
//  1. solange der Ausgang von vor dem Start noch nicht verblockt ist: nach
//     einem Absturz traegt der erste Block die alten Auftraege mit der Zeit
//     ihrer Annahme, und erst danach kommen neue hinzu;
//  2. solange er seit admissionStallLimit() Sekunden (Vorgabe 30) keinen
//     Block gespeichert hat, obwohl er erzeugt -- dieselbe Grenze, die der
//     RPC-Weg fuer Ueberweisungen schon hat (admission_control.go), jetzt
//     fuer jeden annehmenden Pfad (annahmeBeginnen) und die Registrierung.
//
// Beides ist wiederholbar ("gleich nochmal"), nicht endgueltig. Ein Knoten,
// der nie erzeugt (Beobachter, Folger ohne Produktion), ist von 2 nicht
// betroffen: die Messung beginnt mit dem ersten ProduceBlock-Versuch.

// ErrAnnahmePausiert: wiederholbar, wie ErrNichtLeiter.
var ErrAnnahmePausiert = errors.New("dieser Knoten nimmt gerade nichts an")

var annahmePausiertAbgelehnt atomic.Int64

// ausgangVorStartMerken: beim Start (nach dem Wiederoeffnen liegengebliebener
// Zeilen) die hoechste offene Ausgangszeile merken. 0 = nichts offen.
func (cs *ChainState) ausgangVorStartMerken() {
	if cs == nil || cs.db == nil {
		return
	}
	var bis int64
	if err := cs.db.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM pending_txs WHERE included_at = 0`).Scan(&bis); err != nil {
		// Unbekannt heisst: so tun, als laege alles offen -- die Sperre loest
		// sich mit dem ersten gespeicherten Block, der nachsieht.
		fmt.Printf("[ANNAHME] Ausgang beim Start nicht lesbar (%v) -- Annahme wartet auf den ersten eigenen Block\n", err)
		bis = 1<<62 - 1
	}
	cs.ausgangVorStartBis.Store(bis)
	if bis > 0 {
		fmt.Printf("[ANNAHME] Ausgang von vor dem Start liegt noch offen (bis Zeile %d) -- neue Auftraege erst, wenn er verblockt ist\n", bis)
	}
}

// eigenerBlockGespeichert: nach jedem gespeicherten eigenen Block.
func (cs *ChainState) eigenerBlockGespeichert() {
	if cs == nil {
		return
	}
	cs.letzterEigenerBlock.Store(time.Now().Unix())
	bis := cs.ausgangVorStartBis.Load()
	if bis == 0 {
		return
	}
	if cs.db == nil {
		cs.ausgangVorStartBis.Store(0)
		return
	}
	var offen bool
	if err := cs.db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pending_txs WHERE included_at = 0 AND id <= $1)`, bis).Scan(&offen); err != nil {
		return // beim naechsten Block wieder
	}
	if !offen && cs.ausgangVorStartBis.CompareAndSwap(bis, 0) {
		fmt.Println("[ANNAHME] ✓ Ausgang von vor dem Start ist verblockt -- Annahme offen")
	}
}

// annahmePausiert: nil, wenn angenommen werden darf; zaehlt Ablehnungen.
func (cs *ChainState) annahmePausiert() error {
	err := cs.annahmePauseGrund()
	if err != nil {
		annahmePausiertAbgelehnt.Add(1)
	}
	return err
}

// annahmePauseGrund: wie annahmePausiert, ohne zu zaehlen.
func (cs *ChainState) annahmePauseGrund() error {
	if cs.ausgangVorStartBis.Load() != 0 {
		return fmt.Errorf("%w: der Ausgang von vor dem Neustart wird noch verblockt -- bitte in wenigen Sekunden erneut versuchen", ErrAnnahmePausiert)
	}
	seit := cs.erzeugerSeit.Load()
	if seit == 0 {
		return nil // erzeugt nicht (oder noch nie versucht)
	}
	if l := cs.letzterEigenerBlock.Load(); l > seit {
		seit = l
	}
	if d := time.Now().Unix() - seit; d >= admissionStallLimit() {
		return fmt.Errorf("%w: seit %d s kein eigener Block -- bitte in Kuerze erneut versuchen", ErrAnnahmePausiert, d)
	}
	return nil
}

// AnnahmePauseStand: fuer /api/health/combined.
func (cs *ChainState) AnnahmePauseStand() map[string]interface{} {
	grund := ""
	if err := cs.annahmePauseGrund(); err != nil {
		grund = err.Error()
	}
	return map[string]interface{}{
		"bedeutung": "Annahme haelt an, solange der Ausgang von vor dem Start nicht verblockt ist oder seit " +
			fmt.Sprint(admissionStallLimit()) + " s kein eigener Block entstand (annahme_pause.go) -- sonst passten die Auftraege eines Blocks zu keiner gemeinsamen Blockzeit.",
		"pausiert":  grund != "",
		"grund":     grund,
		"abgelehnt": annahmePausiertAbgelehnt.Load(),
		"vor_start": cs.ausgangVorStartBis.Load() != 0,
	}
}
