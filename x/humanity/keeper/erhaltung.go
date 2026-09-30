package keeper

import (
	"context"
)

// Erhaltung der Toepfe beim Nachspielen (Audit 2026-09-29, K-2, Schritt 2).
//
// # WARUM
//
// Ausschuettungen (Grundeinkommen, Validatoren, Liquiditaetsgeber) wurden
// beim Nachspielen gutgeschrieben, ohne ihren Topf zu belasten. Der Topf
// wurde danach getrennt auf einen Stand gesetzt (ubi_distribution_finalize)
// oder genullt (*_pool_zero). Ein Produzent konnte damit beliebige Betraege
// an beliebige Wallets ausschuetten -- das Gutachten zeigte die Geldmenge
// von 10 auf 101.909 AEQ -- und so oft er wollte.
//
// # WAS
//
// Jeder Knoten zaehlt beim Nachspielen die Gutschriften einer Runde je Topf
// mit:
//
//   - topf_ueberzogen: bei JEDER Gutschrift -- die Summe der Runde bis hierher
//     ist hoeher als der Topf. Faellt auch auf, wenn der Produzent den
//     Abschluss weglaesst.
//   - topf_rest: beim Abschluss -- der gemeldete Endstand des
//     Grundeinkommens-Topfs ist hoeher als Topf minus Auszahlung; der Rest
//     waere neues Geld.
//
// Die Summen liegen im Speicher. Nach einem Neustart mitten in einer Runde
// beginnen sie bei null -- die Pruefung zaehlt dann zu wenig, nie zu viel.
//
// Hoechstens eine Runde am Tag: das erzwingt schon distributionRoundToSkip
// (block.go) -- eine Runde weniger als 24 Stunden nach der letzten wird beim
// Nachspielen samt ihrer Gutschriften uebersprungen
// (distribution_round_skip_test.go).
//
// Die Summen laufen ueber Bloecke hinweg (eine Runde kann bei vielen
// Menschen mehrere Bloecke fuellen) und sind Teil des Rueckroll-Snapshots:
// ein zurueckgewiesener Block veraendert sie nicht.
//
// # STUFE
//
// Wie Schritt 1 (nachrechnen.go): bis nachrechnenStrengAbUnix nur
// BEOBACHTEN -- zaehlen, protokollieren, unter /api/wirtschaft/regeln
// veroeffentlichen. Erst wenn der echte Verkehr null Abweichungen zeigt, wird
// der Stichtag gesetzt; eine zu strenge Regel wuerde sonst die Kette anhalten.
//
// Nicht hier: escrow_release (die Treuhand ist noch kein Konsenszustand, die
// Nachspielenden kennen die Eintraege nicht -- K-2 Schritt 3).

// topfErhaltung: Summen der laufenden Runde in Mikro-AEQ, je Topf.
type topfErhaltung struct {
	ubi, validatoren, lp    int64
	ubiN, validatorenN, lpN int64
}

// erhaltungToleranz: je Gutschrift ein Mikro Rundung, dazu zwei fuer den
// Endstand in float64.
func erhaltungToleranz(n int64) int64 { return n + 2 }

// pruefeErhaltungLocked: VOR dem Anwenden der Transaktion. Aufrufer haelt
// cs.mu (replayTransactions).
func (cs *ChainState) pruefeErhaltungLocked(tx *Transaction, blockZeit int64) error {
	e := &cs.erhaltung
	switch tx.Type {
	// Gutschriften: laufend gegen den Topf. Waehrend einer Runde sinkt der
	// Topf beim Nachspielen nicht (die Gutschrift belastet ihn nicht, erst
	// der Abschluss setzt ihn) -- Summe bis hierher <= Topf muss also bei
	// JEDER Gutschrift gelten. So faellt auch ein Produzent auf, der den
	// Abschluss einfach weglaesst.
	case "ubi_distribution":
		if tx.AmountPerHuman == 0 {
			e.ubi += NewDecimal(tx.Amount).Micro()
			e.ubiN++
			return cs.ueberzogenLocked("Grundeinkommen", ubiPoolAddr, e.ubi, e.ubiN, blockZeit)
		}
	case "validator_distribution":
		e.validatoren += NewDecimal(tx.Amount).Micro()
		e.validatorenN++
		return cs.ueberzogenLocked("Validatoren", validatorsPoolAddr, e.validatoren, e.validatorenN, blockZeit)
	case "lp_distribution":
		e.lp += NewDecimal(tx.Amount).Micro()
		e.lpN++
		return cs.ueberzogenLocked("Liquiditaetsgeber", lpPoolAddr, e.lp, e.lpN, blockZeit)

	// Abschluss: der Topf wird gesetzt bzw. genullt, die Runde ist zu Ende.
	case "ubi_distribution_finalize":
		summe, n := e.ubi, e.ubiN
		e.ubi, e.ubiN = 0, 0
		topf := cs.topfMikroLocked(ubiPoolAddr)
		if rest := NewDecimal(tx.Amount).Micro(); rest > topf-summe+erhaltungToleranz(n) {
			return nachrechnenAbweichung("topf_rest", blockZeit,
				"Grundeinkommen: Endstand %.6f AEQ, moeglich hoechstens %.6f", tx.Amount,
				NewDecimalFromMicro(topf-summe).Float())
		}
	case "validator_distribution_pool_zero":
		e.validatoren, e.validatorenN = 0, 0
	case "lp_distribution_pool_zero":
		e.lp, e.lpN = 0, 0
	}
	return nil
}

// ueberzogenLocked: Summe der Gutschriften der laufenden Runde gegen den
// Topf.
func (cs *ChainState) ueberzogenLocked(name, topfAdresse string, summe, n, blockZeit int64) error {
	if topf := cs.topfMikroLocked(topfAdresse); summe > topf+erhaltungToleranz(n) {
		return nachrechnenAbweichung("topf_ueberzogen", blockZeit,
			"%s: %d Gutschriften ueber %.6f AEQ, im Topf %.6f", name, n,
			NewDecimalFromMicro(summe).Float(), NewDecimalFromMicro(topf).Float())
	}
	return nil
}

// topfMikroLocked: Stand eines Topfs in Mikro-AEQ, 0 wenn es ihn nicht gibt.
func (cs *ChainState) topfMikroLocked(addr string) int64 {
	cs.ensureAccountLoadedCtx(context.Background(), addr)
	if acc, ok := cs.accounts.Get(addr); ok {
		return acc.Balance.Micro()
	}
	return 0
}
