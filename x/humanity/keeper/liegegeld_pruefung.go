package keeper

import (
	"fmt"
	"math"
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
// und laeuft von der Kette weg. Seit 22.10.2026 (Audit 2026-10-21, M2) ist
// die Variable ganz entfernt: auch "frueher streng" auf einem einzelnen
// Knoten heisst, dass dieser Knoten Bloecke verwirft, die alle anderen
// annehmen. Es gilt nur noch der gemeinsame Stichtag; die Abweichungen
// zaehlen im gemeinsamen Nachrechnen (Regel "liegegeld").
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

// pruefeUmlaufLocked: stimmt der Betrag einer "umlauf"-Transaktion? VOR dem
// Anwenden aufrufen (der Stand vor dem Abzug ist der, mit dem der Erzeuger
// gerechnet hat). Fehler nur im strengen Modus. Caller haelt cs.mu.
//
// blockZeit: die Zeit des Blocks, der die Transaktion traegt. Sie entscheidet,
// ob eine Runde aus der Vergangenheit streng abgelehnt wird (umlauf_alte_runde)
// -- nie die Rundenzeit at, die der Erzeuger frei waehlt.
func (cs *ChainState) pruefeUmlaufLocked(wallet string, betrag float64, at, blockZeit int64) error {
	if !wirtschaftAktiv(at) {
		return nil
	}
	wallet = strings.ToLower(strings.TrimSpace(wallet))
	acc, ok := cs.accounts.Get(wallet)
	if !ok {
		return nil // applyUmlaufDeltaLocked tut dann nichts
	}
	// Muesste der Einzug LP-Anteile aufloesen (umlauf_lp.go), prueft jeder
	// Knoten IMMER streng, auch vor dem gemeinsamen Stichtag.
	// Sonst loeste ein ueberhoehter Betrag im Block die ganze Position im
	// Pool auf (Sicherheitspruefung #238, M2). Was sich nicht nachrechnen
	// laesst, loest nichts auf.
	lpNoetig := at >= umlaufMitLPAbUnix && NewDecimal(betrag) > acc.Balance && acc.LPShares > 0
	streng := lpNoetig
	art := cs.kontoartVon(wallet, acc.IsHuman)
	w := cs.wirt()
	var vorher int64
	zeitraumBekannt, fensterVoll, aelter := true, true, false
	w.mitSperre(func() {
		switch {
		case at > w.letzterUmlauf:
			vorher = w.letzterUmlauf
		case at == w.laufAt:
			vorher = w.laufVorher
		default:
			zeitraumBekannt = false
			aelter = at < w.letzterUmlauf
			return
		}
		// Ab dem Gleichgewicht (wirtschaft3.go, c) braucht kein Betrag mehr
		// die Buchfuehrung: jeder Knoten prueft jedes Konto.
		fensterVoll = art != artUnternehmen || gleichgewichtAktiv(at) || w.fensterVollLocked(at)
	})
	if !zeitraumBekannt {
		liegegeldUebersprungen.Add(1)
		if lpNoetig {
			return fmt.Errorf("umlauf %s: Zeitraum unbekannt, LP-Anteile werden nicht aufgeloest: %w", wallet, ErrZustandLehntAb)
		}
		// EINE RUNDE AUS DER VERGANGENHEIT (Sicherheitspruefung 25.10.2026).
		// Bis hierhin wurde ein Umlauf mit einer Rundenzeit vor der letzten
		// Runde weder nachgerechnet noch begrenzt -- applyUmlaufDeltaLocked
		// zog ihn bis zum ganzen Guthaben ab. Ein Erzeuger musste nur eine
		// alte Rundenzeit eintragen, um jedem Konto sein Geld ins
		// Grundeinkommen zu nehmen. Ehrlich entsteht das nie: Runden laufen
		// nur vorwaerts, und eine schon angewandte Runde ueberspringt das
		// Nachspielen an ihrer Rundenmarke (distributionRoundToSkip). Gleich
		// alt (at == letzterUmlauf nach einem Neustart mitten in der Runde)
		// bleibt wie bisher. Streng ab dem gemeinsamen Stichtag, nach der
		// BLOCKZEIT.
		if aelter {
			return nachrechnenAbweichung("umlauf_alte_runde", blockZeit,
				"%s: Rundenzeit %d liegt vor der letzten Runde (%.6f AEQ)", kurzAdresse(wallet), at, betrag)
		}
		return nil // Zeitraum unbekannt (Neustart mitten in der Runde)
	}
	if !fensterVoll {
		liegegeldUebersprungen.Add(1)
		if lpNoetig {
			return fmt.Errorf("umlauf %s: Umsatzfenster unvollstaendig, LP-Anteile werden nicht aufgeloest: %w", wallet, ErrZustandLehntAb)
		}
		return nil
	}

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
	if nachrechnenStreng(nowUnix()) {
		modus = "streng"
	}
	return map[string]interface{}{
		"modus":         modus,
		"geprueft":      liegegeldGeprueft.Load(),
		"abweichungen":  liegegeldAbweichungen.Load(),
		"uebersprungen": liegegeldUebersprungen.Load(),
	}
}
