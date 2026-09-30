package keeper

import (
	"context"
	"math"
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
//   - topf_ueberzogen: Summe der Runde ueber dem Topf. Validatoren/LP bei
//     jeder Gutschrift; Grundeinkommen beim Abschluss bzw. spaetestens bei
//     der Rundenmarke (erst dann ist die Demurrage aller Menschen im Topf).
//   - topf_rest: beim Abschluss -- der gemeldete Endstand des
//     Grundeinkommens-Topfs ist hoeher als Topf minus Auszahlung; der Rest
//     waere neues Geld.
//   - demurrage_ueber_guthaben: die Demurrage, die einem Konto abgezogen wird
//     und in einen Topf fliesst, ist hoeher als Guthaben + LP-Wert.
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
	// Demurrage, die der Produzent einem Konto abzieht, kann nicht mehr sein,
	// als es hat (Guthaben + LP-Wert). Sonst ging das Konto ins Minus, und der
	// Topf, der die Demurrage bekommt, wuchs aus dem Nichts
	// (Sicherheitspruefung #239, Befund 2).
	if err := cs.demurrageGedecktLocked(tx.Wallet, tx.FromDemurrageLost, blockZeit); err != nil {
		return err
	}
	if tx.Type == "transfer" {
		if err := cs.demurrageGedecktLocked(tx.To, tx.ToDemurrageLost, blockZeit); err != nil {
			return err
		}
	}

	e := &cs.erhaltung
	switch tx.Type {
	// Grundeinkommen: NICHT je Gutschrift pruefen. Der Erzeuger rechnet die
	// Demurrage ALLER Menschen vorab in den Topf und teilt dann; beim
	// Nachspielen kommt sie erst mit jeder Gutschrift (FromDemurrageLost)
	// hinein. Mitten in der Runde ist der Topf also noch nicht voll -- eine
	// Pruefung dort meldete jede ehrliche Runde mit Demurrage
	// (Sicherheitspruefung #239, Befund 1). Geprueft wird beim Abschluss und
	// bei der Rundenmarke, wenn alles im Topf ist.
	case "ubi_distribution":
		if tx.AmountPerHuman == 0 {
			e.ubi = plusGesaettigt(e.ubi, NewDecimal(tx.Amount).Micro())
			e.ubiN++
		}
	// Validatoren und LP: laufend. Ihre Toepfe bekommen waehrend einer Runde
	// nichts (Demurrage und Vermoegensgrenze gehen ans Grundeinkommen) und
	// sinken beim Nachspielen erst mit *_pool_zero -- Summe <= Topf muss bei
	// JEDER Gutschrift gelten, auch wenn der Abschluss fehlt.
	case "validator_distribution":
		e.validatoren = plusGesaettigt(e.validatoren, NewDecimal(tx.Amount).Micro())
		e.validatorenN++
		return cs.ueberzogenLocked("Validatoren", validatorsPoolAddr, e.validatoren, e.validatorenN, blockZeit)
	case "lp_distribution":
		e.lp = plusGesaettigt(e.lp, NewDecimal(tx.Amount).Micro())
		e.lpN++
		return cs.ueberzogenLocked("Liquiditaetsgeber", lpPoolAddr, e.lp, e.lpN, blockZeit)

	case "ubi_distribution_finalize":
		summe, n := e.ubi, e.ubiN
		e.ubi, e.ubiN = 0, 0
		if err := cs.ueberzogenLocked("Grundeinkommen", ubiPoolAddr, summe, n, blockZeit); err != nil {
			return err
		}
		return cs.restGedecktLocked("Grundeinkommen", ubiPoolAddr, tx.Amount, summe, n, blockZeit)
	// Seit C1 tragen auch diese Abschluesse ihren Endstand; er darf nicht
	// ueber Topf minus Auszahlung liegen (Pruefung #239, Befund 4).
	case "validator_distribution_pool_zero":
		summe, n := e.validatoren, e.validatorenN
		e.validatoren, e.validatorenN = 0, 0
		return cs.restGedecktLocked("Validatoren", validatorsPoolAddr, tx.Amount, summe, n, blockZeit)
	case "lp_distribution_pool_zero":
		summe, n := e.lp, e.lpN
		e.lp, e.lpN = 0, 0
		return cs.restGedecktLocked("Liquiditaetsgeber", lpPoolAddr, tx.Amount, summe, n, blockZeit)

	// Die Rundenmarke kommt als letzte Transaktion jeder Runde. Was bis hier
	// nicht abgeschlossen wurde (Grundeinkommen ohne finalize), wird jetzt
	// gegen den Topf geprueft; danach beginnt jede Summe neu -- auch wenn der
	// Produzent einen Abschluss weggelassen hat.
	case "distribution_round_marker":
		summe, n := e.ubi, e.ubiN
		*e = topfErhaltung{}
		if n > 0 {
			return cs.ueberzogenLocked("Grundeinkommen", ubiPoolAddr, summe, n, blockZeit)
		}
	}
	return nil
}

// demurrageGedecktLocked: lost <= Guthaben + LP-Wert von wallet.
func (cs *ChainState) demurrageGedecktLocked(wallet string, lost float64, blockZeit int64) error {
	if lost <= 0 || wallet == "" {
		return nil
	}
	cs.ensureAccountLoadedCtx(context.Background(), wallet)
	var hat int64
	if acc, ok := cs.accounts.Get(wallet); ok {
		hat = plusGesaettigt(acc.Balance.Micro(), NewDecimal(cs.lpValueLockedAEQ(acc)).Micro())
	}
	if NewDecimal(lost).Micro() > hat+erhaltungToleranz(0) {
		return nachrechnenAbweichung("demurrage_ueber_guthaben", blockZeit,
			"%s: Demurrage %.6f AEQ, das Konto hat %.6f", kurzAdresse(wallet), lost,
			NewDecimalFromMicro(hat).Float())
	}
	return nil
}

// plusGesaettigt: a+b ohne Ueberlauf ins Negative (Befund 6).
func plusGesaettigt(a, b int64) int64 {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// restGedecktLocked: der gemeldete Endstand eines Topfs nach der Runde liegt
// nicht ueber Topf minus Auszahlung -- sonst waere der Rest neues Geld.
func (cs *ChainState) restGedecktLocked(name, topfAdresse string, rest float64, summe, n, blockZeit int64) error {
	topf := cs.topfMikroLocked(topfAdresse)
	if NewDecimal(rest).Micro() > topf-summe+erhaltungToleranz(n) {
		return nachrechnenAbweichung("topf_rest", blockZeit,
			"%s: Endstand %.6f AEQ, moeglich hoechstens %.6f", name, rest,
			NewDecimalFromMicro(topf-summe).Float())
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
