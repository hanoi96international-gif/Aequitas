package keeper

import (
	"fmt"
	"math"
	"os"
	"strings"
	"sync/atomic"
)

// Pruefung des Liegegelds beim Nachspielen fremder Bloecke.
//
// Der erzeugende Knoten rechnet den Umlauf (wirtschaft.go, umlaufLocked) und
// schreibt die Betraege in den Block; wer nachspielt, wendet sie an. Ohne
// Pruefung koennte ein Erzeuger beliebige Betraege abziehen -- gedeckelt nur
// durch das Guthaben. Jetzt rechnet jeder nachspielende Knoten nach, sofern
// er selbst die Daten dazu hat.
//
// Zwei Stufen (AEQUITAS_LIEGEGELD_PRUEFUNG):
//
//   - "beobachten" (Standard): Abweichungen werden gezaehlt, protokolliert und
//     unter /api/wirtschaft/regeln veroeffentlicht; der Block gilt trotzdem.
//   - "streng": eine Abweichung lehnt den Block ab.
//
// Warum nicht sofort streng: der Erzeuger schreibt seinen Buchungsaugenblick
// in die Transaktion (Transaction.BuchAt), der Nachspielende bucht zum selben
// Augenblick -- beide sollten also dieselben Zahlen haben. Ob sie es im
// Betrieb wirklich haben (Neustarts, Snapshots, ein uebersehener Pfad), zeigt
// erst die Beobachtung. Zeigt sie ueber Wochen null Abweichungen, wird
// umgeschaltet -- spaetestens bevor ein zweiter unabhaengiger Validator
// Bloecke erzeugt.
//
// Geprueft wird nur, was dieser Knoten selbst voll kennt: Unternehmen, und
// nur wenn seine Buchfuehrung das ganze Fenster abdeckt (seit der Aktivierung
// oder mindestens ein Jahr seit buchSeit). Menschen und freie Adressen
// brauchen keine Buchfuehrung und werden immer geprueft.

const liegegeldToleranz = 2e-6

var (
	liegegeldGeprueft      atomic.Int64
	liegegeldAbweichungen  atomic.Int64
	liegegeldUebersprungen atomic.Int64
)

func liegegeldStreng() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("AEQUITAS_LIEGEGELD_PRUEFUNG")), "streng")
}

// pruefeUmlaufLocked: stimmt der Betrag einer "umlauf"-Transaktion? VOR dem
// Anwenden aufrufen (der Stand vor dem Abzug ist der, mit dem der Erzeuger
// gerechnet hat). Fehler nur im strengen Modus. Caller haelt cs.mu.
func (cs *ChainState) pruefeUmlaufLocked(wallet string, betrag float64, at int64) error {
	if !wirtschaftAktiv(at) {
		return nil
	}
	wallet = strings.ToLower(strings.TrimSpace(wallet))
	acc, ok := cs.accounts.Get(wallet)
	if !ok {
		return nil // applyUmlaufDeltaLocked tut dann nichts
	}
	art := cs.kontoartVon(wallet, acc.IsHuman)
	w := cs.wirt()
	w.mu.Lock()
	var vorher int64
	switch {
	case at > w.letzterUmlauf:
		vorher = w.letzterUmlauf
	case at == w.laufAt:
		vorher = w.laufVorher
	default:
		w.mu.Unlock()
		liegegeldUebersprungen.Add(1)
		return nil // ein aelterer Lauf: Zeitraum unbekannt
	}
	if art == artUnternehmen && !w.fensterVollLocked(at) {
		w.mu.Unlock()
		liegegeldUebersprungen.Add(1)
		return nil
	}
	w.mu.Unlock()

	sekunden := int64(86400)
	if vorher > 0 {
		sekunden = at - vorher
	}
	if sekunden > 7*86400 {
		sekunden = 7 * 86400
	}
	stand := acc.Balance.Float()
	erwartet := cs.umlaufBetrag(wallet, art, stand, at, sekunden)
	// Der Abzug ist auf das Guthaben gedeckelt; der Erzeuger schreibt den
	// ungedeckelten Betrag, also beide Seiten gleich deckeln.
	gebucht := math.Min(betrag, stand)
	erwartet = math.Min(erwartet, stand)
	liegegeldGeprueft.Add(1)
	if math.Abs(gebucht-erwartet) <= liegegeldToleranz {
		return nil
	}
	liegegeldAbweichungen.Add(1)
	fmt.Printf("[LIEGEGELD] Abweichung %s: im Block %.6f, nachgerechnet %.6f (Stand %.6f, %ds)\n",
		wallet, betrag, erwartet, stand, sekunden)
	if liegegeldStreng() {
		return fmt.Errorf("umlauf %s: im Block %.6f, nachgerechnet %.6f: %w", wallet, betrag, erwartet, ErrZustandLehntAb)
	}
	return nil
}

// fensterVollLocked: kennt dieser Knoten den Umsatz des ganzen Fensters?
// w.mu gehalten.
func (w *wirtschaft) fensterVollLocked(jetzt int64) bool {
	if w.buchSeit <= aktivAb() {
		return true
	}
	return jetzt-w.buchSeit >= umsatzJahrTage*86400
}

func liegegeldPruefungStand() map[string]interface{} {
	modus := "beobachten"
	if liegegeldStreng() {
		modus = "streng"
	}
	return map[string]interface{}{
		"modus":         modus,
		"geprueft":      liegegeldGeprueft.Load(),
		"abweichungen":  liegegeldAbweichungen.Load(),
		"uebersprungen": liegegeldUebersprungen.Load(),
	}
}
