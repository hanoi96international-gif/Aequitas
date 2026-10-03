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
// Zwei Stufen:
//
//   - beobachten (Standard): Abweichungen werden gezaehlt, protokolliert und
//     unter /api/wirtschaft/regeln veroeffentlicht; der Block gilt trotzdem.
//   - streng: eine Abweichung lehnt den Block ab.
//
// Streng wird es fuer ALLE Knoten zugleich mit dem gemeinsamen Stichtag
// nachrechnenStrengAbUnix (nachrechnen.go, K-2). Bis 03.10.2026 schaltete
// nur AEQUITAS_LIEGEGELD_PRUEFUNG=streng um -- je Knoten. Eine Konsensregel,
// die jeder Knoten anders eingestellt haben kann, ist selbst nicht
// deterministisch: ein strenger Knoten verwirft, was die anderen annehmen,
// und laeuft von der Kette weg. Die Variable bleibt als Moeglichkeit, einen
// einzelnen Knoten frueher streng zu stellen (nie lockerer), und die
// Abweichungen zaehlen jetzt auch im gemeinsamen Nachrechnen (Regel
// "liegegeld").
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
	// Muesste der Einzug LP-Anteile aufloesen (umlauf_lp.go), prueft jeder
	// Knoten IMMER streng -- unabhaengig von AEQUITAS_LIEGEGELD_PRUEFUNG.
	// Sonst loeste ein ueberhoehter Betrag im Block die ganze Position im
	// Pool auf (Sicherheitspruefung #238, M2). Was sich nicht nachrechnen
	// laesst, loest nichts auf.
	lpNoetig := at >= umlaufMitLPAbUnix && NewDecimal(betrag) > acc.Balance && acc.LPShares > 0
	streng := liegegeldStreng() || lpNoetig
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
		if lpNoetig {
			return fmt.Errorf("umlauf %s: Zeitraum unbekannt, LP-Anteile werden nicht aufgeloest: %w", wallet, ErrZustandLehntAb)
		}
		return nil // ein aelterer Lauf: Zeitraum unbekannt
	}
	if art == artUnternehmen && !w.fensterVollLocked(at) {
		w.mu.Unlock()
		liegegeldUebersprungen.Add(1)
		if lpNoetig {
			return fmt.Errorf("umlauf %s: Umsatzfenster unvollstaendig, LP-Anteile werden nicht aufgeloest: %w", wallet, ErrZustandLehntAb)
		}
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
	stand := cs.umlaufStandLocked(acc, at)
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
	// Gemeinsamer Stichtag (nachrechnen.go): zaehlt dort mit und lehnt ab dem
	// Stichtag auf jedem Knoten ab. at ist die Rundenzeit; sie liegt
	// hoechstens 600 s neben der Blockzeit (Regel "rundenmarke").
	if err := nachrechnenAbweichung("liegegeld", at, "%s: im Block %.6f, nachgerechnet %.6f",
		kurzAdresse(wallet), betrag, erwartet); err != nil {
		return err
	}
	if streng {
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
	if liegegeldStreng() || nachrechnenStreng(nowUnix()) {
		modus = "streng"
	}
	return map[string]interface{}{
		"modus":         modus,
		"geprueft":      liegegeldGeprueft.Load(),
		"abweichungen":  liegegeldAbweichungen.Load(),
		"uebersprungen": liegegeldUebersprungen.Load(),
	}
}
