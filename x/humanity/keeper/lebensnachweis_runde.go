package keeper

import (
	"context"
	"errors"
	"fmt"
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

// ErrLebensnachweiseOffen: die Runde wird spaeter wiederholt (main.go, wie
// ein Sync-Problem: in 15 Minuten), bis die offenen Nachweise verblockt sind.
var ErrLebensnachweiseOffen = errors.New("Grundeinkommensrunde wartet: angewandte Lebensnachweise stehen noch in keinem Block")

// offeneLebensnachweise: wie viele proof_of_life im Ausgang noch in keinem
// erzeugten Block stehen. Ohne Datenbank (Tests, Einzelknoten ohne Ausgang)
// gibt es keinen Ausgang, also nichts Offenes.
func (cs *ChainState) offeneLebensnachweise(ctx context.Context) (int64, error) {
	if cs.db == nil {
		return 0, nil
	}
	var n int64
	if err := cs.dbExecCtx(ctx).QueryRow(
		`SELECT count(*) FROM pending_txs WHERE included_at = 0 AND (tx_json::jsonb ->> 'type') = $1`,
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
	n, err := cs.offeneLebensnachweise(ctx)
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
	n, err := cs.offeneLebensnachweise(context.Background())
	if err != nil {
		fmt.Printf("[POOLS] %v\n", err)
		return "offene Lebensnachweise nicht lesbar -- Runde wird wiederholt (Details im Knotenlog)"
	}
	if n > 0 {
		return fmt.Sprintf("%d angewandte Lebensnachweise stehen noch in keinem Block -- Runde wird wiederholt", n)
	}
	return ""
}

// VerteilungsSperreLoesen: gibt die Tagessperre frei, wenn die Runde mit
// ErrLebensnachweiseOffen abgebrochen und vollstaendig zurueckgerollt wurde
// (main.go). Nur dann: last_ubi_at ist unveraendert, die Runde hat nicht
// stattgefunden, und ohne Freigabe fiele sie fuer fast 24 Stunden aus.
func (cs *ChainState) VerteilungsSperreLoesen() error {
	if cs.db == nil {
		return nil
	}
	_, err := cs.db.Exec(`UPDATE chain_config SET value = '0' WHERE key = 'distribution_lock_at'`)
	return err
}
