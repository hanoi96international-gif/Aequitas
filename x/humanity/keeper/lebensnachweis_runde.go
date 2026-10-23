package keeper

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// DIE RUNDE WARTET AUF OFFENE LEBENSNACHWEISE (#410).
//
// Der Erzeuger wendet einen angenommenen Lebensnachweis sofort auf seinen
// Zustand an (runAtomicWithOutbox: LebensnachweisAm rueckt vor), die
// proof_of_life steht aber erst im naechsten Block. Liefe die
// Grundeinkommensrunde dazwischen, entschiede der Erzeuger "nicht pausiert"
// mit einem Nachweis, den die Nachspielenden zur Runde noch nicht kennen --
// sobald die Runde in einem Block vor dem Nachweis steht (andere
// Blockaufteilung, ein zurueckgelegter Ausgang). Sie saehen die Wallet als
// pausiert: ubi_pausiert bzw. ubi_empfaenger/ubi_anteil, und im strengen
// Modus (ab 01.11.2026) einen ungueltigen Block.
//
// Darum entscheidet die Runde ueber Pausen nur, wenn KEIN angewandter
// Lebensnachweis mehr offen im Ausgang liegt. runAtomicWithOutbox haelt cs.mu
// bis nach dem Commit; laeuft die Runde unter cs.mu, ist also jeder
// angewandte Nachweis festgeschrieben und steht als Zeile in pending_txs.
// Ohne Zeile mit included_at = 0 steht jeder davon in einem Block, den der
// Erzeuger VOR der Runde erzeugt hat -- jeder Nachspielende hat ihn, bevor er
// die Runde sieht. Was danach angenommen wird, wartet auf cs.mu und kommt
// nach der Runde: bei beiden.
//
// Keine Konsensaenderung: nur der Erzeuger wartet; die Bloecke und ihr
// Nachrechnen bleiben, wie sie sind. Vor lebensnachweisAktiv (und solange
// keine Pause moeglich ist) gilt nichts davon -- dann pausiert die Runde
// niemanden, und ein Nachweis aendert an ihr nichts.
//
// DIE BETREIBERZAHL EBENSO (#412, Befund 2). Ob die Runde ueberhaupt
// pausieren darf, haengt an den Betreibern ohne offene Staffel
// (zugelasseneBetreiberZu). Auch die aendert der Erzeuger sofort, vor dem
// Block: grant_release (Staffelrest auf 0) und register_human (ein Betreiber
// wird Mensch). Darum zaehlen auch diese als offen -- grant_release jeder
// Wallet (sie entsteht nur in der Tagesrunde, offen ist sie also nur bei
// haengendem Ausgang), register_human nur fuer Wallets, die im
// Kettenregister als Betreiber stehen (validator_verlauf, begrenzt) -- eine
// gewoehnliche Registrierung haelt die Runde nicht an.

// ErrLebensnachweiseOffen: die Runde wird bald wiederholt (main.go), bis die
// offenen Auftraege verblockt sind.
var ErrLebensnachweiseOffen = errors.New("Grundeinkommensrunde wartet: angewandte Lebensnachweise oder Betreiber-Aenderungen stehen noch in keinem Block")

// offenePauseAuftraege: wie viele Auftraege im Ausgang noch in keinem
// erzeugten Block stehen, die die Pause-Entscheidung der Runde aendern
// (siehe oben). Ohne Datenbank (Tests, Einzelknoten ohne Ausgang) gibt es
// keinen Ausgang, also nichts Offenes.
func (cs *ChainState) offenePauseAuftraege(ctx context.Context) (int64, error) {
	if cs.db == nil {
		return 0, nil
	}
	var n int64
	if err := cs.dbExecCtx(ctx).QueryRow(
		`SELECT count(*) FROM pending_txs WHERE included_at = 0 AND (
			(tx_json::json ->> 'type') IN ($1, 'grant_release')
			OR ((tx_json::json ->> 'type') = 'register_human'
			    AND lower(tx_json::json ->> 'wallet') IN (SELECT lower(operator_wallet) FROM validator_verlauf)))`,
		lebensnachweisTxArt).Scan(&n); err != nil {
		return 0, fmt.Errorf("offene Lebensnachweise nicht lesbar: %w", err)
	}
	return n, nil
}

// lebensnachweisRundeBereitLocked: darf die Runde zur Rundenzeit t ueber
// Pausen entscheiden? nil = ja. Nicht lesbar = nein (fail-closed: die Runde
// wird wiederholt, statt mit unbekanntem Stand zu entscheiden).
func (cs *ChainState) lebensnachweisRundeBereitLocked(ctx context.Context, t int64, pauseMoeglich bool) error {
	if !pauseMoeglich || !lebensnachweisAktiv(t) {
		return nil
	}
	n, err := cs.offenePauseAuftraege(ctx)
	if err != nil {
		return fmt.Errorf("%w (%v)", ErrLebensnachweiseOffen, err)
	}
	if n > 0 {
		return fmt.Errorf("%w (%d offen)", ErrLebensnachweiseOffen, n)
	}
	return nil
}

// LebensnachweiseOffenGrund: Vorpruefung in main.go VOR der Tagessperre
// (TryLockDistribution) -- sonst hielte eine abgebrochene Runde die Sperre
// fast 24 Stunden, und der Tag fiele aus. Leer = die Runde darf starten.
// Ohne cs.mu, darum grober als lebensnachweisRundeBereitLocked: geprueft
// wird ab lebensnachweisAktiv, auch wenn keine Pause moeglich waere.
func (cs *ChainState) LebensnachweiseOffenGrund(jetzt int64) string {
	if !lebensnachweisAktiv(jetzt) || cs.db == nil {
		return ""
	}
	n, err := cs.offenePauseAuftraege(context.Background())
	if err != nil {
		fmt.Printf("[POOLS] %v\n", err)
		return "offene Lebensnachweise nicht lesbar -- Runde wird wiederholt (Details im Knotenlog)"
	}
	if n > 0 {
		return fmt.Sprintf("%d angewandte Lebensnachweise oder Betreiber-Aenderungen stehen noch in keinem Block -- Runde wird wiederholt", n)
	}
	return ""
}

// VerteilungsSperreLoesen: gibt die Tagessperre frei, wenn die Runde mit
// ErrLebensnachweiseOffen abgebrochen und vollstaendig zurueckgerollt wurde
// (main.go). Nur dann: last_ubi_at ist unveraendert, die Runde hat nicht
// stattgefunden, und ohne Freigabe fiele sie fuer fast 24 Stunden aus.
//
// Nur die EIGENE Sperre (Sicherheitspruefung #411, Befund 3): frei wird der
// Eintrag nur, wenn er noch genau den Wert traegt, mit dem dieser Knoten ihn
// genommen hat. Hat inzwischen ein anderer die Sperre genommen, bleibt sie.
func (cs *ChainState) VerteilungsSperreLoesen() error {
	if cs.db == nil {
		return nil
	}
	wert := cs.verteilSperreWert.Load()
	if wert == nil {
		return errKeineEigeneSperre
	}
	res, err := cs.db.Exec(`UPDATE chain_config SET value = '0' WHERE key = 'distribution_lock_at' AND value = $1`, *wert)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errKeineEigeneSperre
	}
	cs.verteilSperreWert.CompareAndSwap(wert, nil)
	return nil
}

var errKeineEigeneSperre = errors.New("Tagessperre gehoert nicht (mehr) dieser Runde -- nicht freigegeben")

// ANNAHME KURZ ANHALTEN (#412, Befund 4).
//
// Alle Bestandsmenschen haben dasselbe Fenster (lebensnachweisUhrAb). Kommen
// Nachweise in stetigem Strom, liegt bei jeder Vorpruefung einer offen, und
// die Runde rutschte immer weiter. Darum haelt der Knoten, wenn die Runde auf
// offene Nachweise wartet, die Annahme neuer Nachweise kurz an
// (lebensnachweisHaltDauer): die offenen kommen mit dem naechsten Block, die
// Runde wird nach LebensnachweisHaltWiederholung erneut versucht und laeuft.
// Ein abgewiesener Mensch sendet dieselbe Bescheinigung gleich noch einmal
// (sie gilt lebensnachweisAnnahmeAlt = 15 min; die App bietet es an).
//
// Begrenzt: ein Halt endet nach lebensnachweisHaltDauer von selbst, auch wenn
// die Runde nie freigibt; ein neuer beginnt fruehestens
// lebensnachweisHaltAbstand nach dem letzten -- auch bei haengendem Ausgang
// bleibt die Annahme die meiste Zeit offen. Nur Verfuegbarkeit: an der
// Pruefung eines Nachweises und an der Runde aendert der Halt nichts.
const (
	lebensnachweisHaltDauer   int64 = 120
	lebensnachweisHaltAbstand int64 = 600
)

// LebensnachweisHaltWiederholung: so bald versucht main.go die Runde erneut,
// wenn es einen Halt gesetzt hat -- innerhalb des Halts.
const LebensnachweisHaltWiederholung = 60 * time.Second

type lebensnachweisHalt struct {
	mu   sync.Mutex
	seit int64 // Beginn des letzten Halts
	bis  int64 // Ende des laufenden Halts, 0 = keiner
}

// LebensnachweisAnnahmeHalten: main.go, wenn die Runde auf offene Nachweise
// wartet. true = ein neuer Halt laeuft ab jetzt; false = der letzte begann
// vor weniger als lebensnachweisHaltAbstand (dann kein neuer).
func (cs *ChainState) LebensnachweisAnnahmeHalten(jetzt int64) bool {
	h := &cs.lnHalt
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.seit != 0 && jetzt >= h.seit && jetzt-h.seit < lebensnachweisHaltAbstand {
		return false
	}
	h.seit, h.bis = jetzt, sattAdd(jetzt, lebensnachweisHaltDauer)
	return true
}

// LebensnachweisAnnahmeFreigeben: die Runde ist gelaufen oder aus anderem
// Grund gescheitert -- die Annahme sofort wieder oeffnen.
func (cs *ChainState) LebensnachweisAnnahmeFreigeben() {
	h := &cs.lnHalt
	h.mu.Lock()
	h.bis = 0
	h.mu.Unlock()
}

// lebensnachweisAngehalten: Sekunden bis zum Ende des Halts, 0 = offen.
func (cs *ChainState) lebensnachweisAngehalten(jetzt int64) int64 {
	h := &cs.lnHalt
	h.mu.Lock()
	defer h.mu.Unlock()
	// Uhr zurueckgesprungen (jetzt < seit): der Halt gilt als vorbei -- er
	// soll nie laenger dauern als lebensnachweisHaltDauer.
	if h.bis == 0 || jetzt >= h.bis || jetzt < h.seit {
		return 0
	}
	return h.bis - jetzt
}
