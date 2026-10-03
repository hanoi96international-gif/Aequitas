package keeper

// Wirtschaft, zweite Stufe (docs/WIRTSCHAFT_REIFEPRUEFUNG.md, 02.10.2026).
//
// Drei Schwaechen der ersten Stufe, alle ab wirtschaft2AktivAbUnix:
//
//   C  Ablehnen statt wegnehmen. Eine Ueberweisung oder ein Tausch, der einen
//      Menschen (oder eine andere Adresse mit Vermoegensgrenze) ueber die
//      Grenze braechte, wird abgelehnt. Bisher ging er durch, und der
//      Ueberschuss wurde sofort verteilt: Der Empfaenger verlor Geld durch eine
//      Zahlung, die er nicht steuern konnte (ein Lohn kurz vor der Grenze).
//      Die Grenze bleibt genauso hart. Gekappt wird weiter, wo niemand den
//      Eingang steuert (Grundeinkommen, Freigaben, Registrierung).
//
//   A  Das erste Unternehmen eines Menschen ist nie schlechter gestellt als
//      ein Mensch (wirtschaft.go, liegegeldLocked). Vorher galt das nur im
//      ersten halben Jahr; danach zahlte ein kleiner Laden mehr als ein Mensch
//      mit demselben Guthaben (4.000 AEQ bei 1.000 Umsatz: 15 gegen 0 AEQ im
//      Monat).
//
//   B  Eingaenge von anderen Unternehmen zaehlen als Umsatz, je zahlendem
//      Unternehmen hoechstens menschZaehltJeUntQuartal im Kalenderquartal --
//      dieselbe Grenze wie fuer Menschen. Vorher zaehlte nur der Ueberschuss
//      (Eingaenge minus Zahlungen an Unternehmen), und wer AEQ an seinen
//      Lieferanten weitergab, verlor Freibetrag. Der Ueberschuss bleibt als
//      Untergrenze: gezaehlt wird der hoehere der beiden Werte.
//
// WARUM EINE AKTIVIERUNGSZEIT. C wirkt nur bei der Annahme
// (transferMutateLocked, swapLockedMitAbgabe); das Nachspielen wendet die
// Betraege aus dem Block an (applyTransferDeltaLockedSammelnd) und kappt wie
// bisher. A und B aendern das Liegegeld, das in den Bloecken steht und
// nachgerechnet wird. Bloecke vor der Aktivierung muessen genau so
// nachgespielt werden wie bisher. Der Zeitpunkt muss NACH dem Ausrollen auf
// alle Knoten liegen (siehe wirtschaftAktivAbUnix); wird bis dahin nicht
// ausgerollt, ist er zu verschieben, bevor ausgerollt wird.

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// 2026-10-03T12:00:00Z (14:00 MESZ). Vorgezogen vom 15.10.: gilt ab dem Tag
// des Ausrollens, und erst NACHDEM alle Knoten diesen Stand haben -- ein
// frueherer Zeitpunkt liesse Knoten mit altem und neuem Stand verschieden
// nachrechnen.
const wirtschaft2AktivAbUnix int64 = 1791028800

// wirtschaft2AktivOverride: nur fuer Tests (0 = Konstante gilt).
var wirtschaft2AktivOverride atomic.Int64

func wirtschaft2Aktiv(unix int64) bool {
	a := wirtschaft2AktivAbUnix
	if o := wirtschaft2AktivOverride.Load(); o != 0 {
		a = o
	}
	return unix >= a
}

// pruefeVermoegensgrenzeAnnahmeLocked (C): wuerde der Eingang zufluss das
// Konto ueber die Vermoegensgrenze bringen? Dann Ablehnung, bevor irgendetwas
// gebucht ist. Dieselbe Rechnung wie enforceWealthCapLockedCtx (Guthaben plus
// Wert der LP-Anteile gegen Durchschnitt mal Multiplikator), damit eine
// Zahlung, die hier durchgeht, dort nie gekappt wird.
//
// Nicht geprueft: Protokoll-Toepfe, Unternehmen (keine feste Grenze), und die
// verteilte Annahme (Stufe 2, kappungVerschobenLocked): dort kennt der
// annehmende Knoten den Stand des Empfaengers nicht verbindlich, gekappt wird
// vom Zustaendigen des Kontos. Caller haelt cs.mu.
func (cs *ChainState) pruefeVermoegensgrenzeAnnahmeLocked(addr string, acc *AccountState, art kontoart, zufluss float64, jetzt int64) error {
	if !wirtschaft2Aktiv(jetzt) || zufluss <= 0 || art == artUnternehmen || art == artSystem {
		return nil
	}
	addr = strings.ToLower(addr)
	if isTokenomicsPoolAddress(addr) || cs.kappungVerschobenLocked() {
		return nil
	}
	capAmt, ok := cs.wealthCapAmountLocked()
	if !ok {
		return nil // wie enforceWealthCapLockedCtx: ohne Durchschnitt keine Grenze
	}
	var stand, lp float64
	if acc != nil {
		stand = acc.Balance.Float()
		lp = cs.lpValueLockedAEQ(acc)
	}
	if stand+lp+zufluss > capAmt {
		frei := capAmt - stand - lp
		if frei < 0 {
			frei = 0
		}
		return fmt.Errorf("recipient would exceed the wealth cap of %.2f AEQ and can receive at most %.6f AEQ now (Empfaenger kaeme ueber die Vermoegensgrenze von %.2f AEQ und kann jetzt hoechstens %.6f AEQ annehmen): %w",
			capAmt, frei, capAmt, frei, ErrZustandLehntAb)
	}
	return nil
}
