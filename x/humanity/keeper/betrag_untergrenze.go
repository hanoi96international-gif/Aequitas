package keeper

import (
	"fmt"
	"math"
)

// Untergrenze fuer Ueberweisungsbetraege (Audit 2026-10-21, M1).
//
// Das Hauptbuch rechnet in Mikro-AEQ (decimal.go). Eine signierte
// Ueberweisung traegt ihren Betrag in wei (10^-18 AEQ); alles unter einem
// halben Mikro-AEQ rundet NewDecimal auf null. Bis hierhin pruefte die
// Annahme nur `amount <= 0` -- 1 wei war gueltig, galt als erfolgreich,
// verbrauchte eine Nonce, kostete keine Gebuehr (0,1 % von null) und stand
// dauerhaft im Block, ohne etwas zu bewegen. Ein Konto mit einem Mikro-AEQ
// konnte so beliebig Blockplatz fuellen.
//
// Abgewiesen wird jetzt, was nicht mindestens ein Mikro-AEQ bewegt. Nur bei
// der Annahme: ein Knoten, der einen fremden Block nachspielt, prueft das
// ueber nachrechnenTxLocked (Regel "betrag_unter_mikro", ab dem strengen
// Stichtag ablehnend), damit alte Bloecke unveraendert gelten.

// ErrBetragUnterMikro: der Betrag bewegt nach der Rundung auf Mikro-AEQ nichts.
var ErrBetragUnterMikro = fmt.Errorf("Betrag unter 0,000001 AEQ -- die kleinste Einheit ist ein Mikro-AEQ")

// pruefeUeberweisungsBetrag: endlich, positiv und mindestens ein Mikro-AEQ.
func pruefeUeberweisungsBetrag(amount float64) error {
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return fmt.Errorf("invalid transfer amount: %v", amount)
	}
	if NewDecimal(amount) <= 0 {
		return ErrBetragUnterMikro
	}
	return nil
}
