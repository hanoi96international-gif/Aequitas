package keeper

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// Treuhand beim Nachspielen nachrechnen (Audit 2026-09-29, K-2, Schritt 3).
//
// Die Treuhand (guardian.go) ist noch kein Konsenszustand: nur der erzeugende
// Knoten fuehrt escrow_accounts, Nachspielende uebernehmen die Betraege aus
// dem Block. Damit konnte ein Produzent
//
//   - escrow_move: das Guthaben eines BELIEBIGEN Kontos auf null setzen,
//     auch eines aktiven -- applyEscrowMoveDeltaLocked prueft nicht, ob das
//     Konto die 2,5 Jahre Inaktivitaet hinter sich hat;
//   - escrow_release / escrow_recover: beliebige Betraege in den
//     Grundeinkommens-Topf bzw. auf ein Konto buchen -- neues Geld.
//
// Was jeder Knoten selbst weiss:
//
//   - treuhand_aktiv: escrow_move nur fuer einen Menschen, dessen letzte
//     Aktivitaet (eigener Zustand) mindestens inactivityEscrowSeconds vor der
//     Blockzeit liegt. Eine Stunde Spielraum, weil der Erzeuger mit seiner
//     Uhr entscheidet und der Block hoechstens 120 s zurueckdatiert sein darf
//     (K-3).
//   - treuhand_lp: die aufgeloesten LP-Anteile nicht ueber dem Bestand.
//   - treuhand_zu_frueh: Eine ehrliche Treuhand entsteht fruehestens
//     inactivityEscrowSeconds nach dem Kettenstart, eine Freigabe in den
//     Topf fruehestens escrowToUBISeconds danach. Vorher ist jede
//     escrow_release und jede escrow_recover erfunden.
//
// Kettenstart: fest der 13.06.2026, der frueheste Wert in den
// Genesis-Dateien. Jeder Knoten hat damit dieselbe Grenze (genesis.json
// liegt je Knoten), und ein spaeterer echter Start fuehrt nur dazu, dass zu
// wenig gemeldet wird, nie zu viel.
//
// Bestand: Seit K-2 fuehrt jeder Nachspielende die Treuhand selbst
// (applyEscrowMoveDeltaLocked legt die Zeile an, Freigabe und Rueckholung
// entfernen sie) -- mit dem Betrag aus seinem eigenen Zustand und der
// Blockzeit. Damit gilt auch NACH den festen Fristen:
//
//   - treuhand_doppelt:       escrow_move fuer eine Wallet, die schon in
//     der Treuhand liegt (der Erzeuger ueberspringt sie; ein zweites Mal
//     wuerde nachgekommenes Guthaben nullen, ohne es aufzubewahren).
//   - treuhand_ohne_bestand:  Freigabe/Rueckholung ohne Treuhand-Zeile.
//   - treuhand_ueber_bestand: Betrag ueber dem eigenen Bestand -- neues Geld.
//   - treuhand_zu_frueh:      Freigabe vor moved_at + escrowToUBISeconds
//     (eine Stunde Spielraum fuer die Uhr des Erzeugers, siehe oben).
//
// Altbestand gibt es nicht: die erste moegliche Verschiebung ist am
// 09.12.2028, lange nach dieser Aenderung. Jede Treuhand-Zeile entsteht
// also auf jedem Knoten beim Nachspielen derselben Bloecke.

var fruehesterKettenstartUnix = time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC).Unix()

const treuhandUhrSpielraum = 3600

func (cs *ChainState) nachrechnenTreuhandLocked(tx *Transaction, blockZeit int64) error {
	wallet := strings.ToLower(strings.TrimSpace(tx.Wallet))
	switch tx.Type {
	case "escrow_move":
		cs.ensureAccountLoadedCtx(context.Background(), wallet)
		acc, ok := cs.accounts.Get(wallet)
		if !ok || !acc.IsHuman || acc.LastActivityAt <= 0 ||
			acc.LastActivityAt > blockZeit-inactivityEscrowSeconds+treuhandUhrSpielraum {
			var zuletzt int64
			if ok {
				zuletzt = acc.LastActivityAt
			}
			return nachrechnenAbweichung("treuhand_aktiv", blockZeit,
				"%s: letzte Aktivitaet %d, Blockzeit %d -- nicht %d s inaktiv",
				kurzAdresse(wallet), zuletzt, blockZeit, inactivityEscrowSeconds)
		}
		if tx.LPShares > 0 && NewDecimal(tx.LPShares).Micro() > acc.LPShares.Micro()+1 {
			return nachrechnenAbweichung("treuhand_lp", blockZeit,
				"%s: %.6f LP-Anteile aufgeloest, gehalten %.6f", kurzAdresse(wallet), tx.LPShares, acc.LPShares.Float())
		}
		if bestand, _, gibt, err := cs.treuhandBestandLocked(wallet); err != nil {
			return nachrechnenAbweichung("treuhand_unlesbar", blockZeit, "%s: %v", kurzAdresse(wallet), err)
		} else if gibt && bestand > 0 {
			return nachrechnenAbweichung("treuhand_doppelt", blockZeit,
				"%s liegt schon mit %.6f AEQ in der Treuhand", kurzAdresse(wallet), bestand)
		}
	case "escrow_release":
		if blockZeit < fruehesterKettenstartUnix+inactivityEscrowSeconds+escrowToUBISeconds {
			return nachrechnenAbweichung("treuhand_zu_frueh", blockZeit,
				"escrow_release %.6f AEQ vor der ersten moeglichen Freigabe", tx.Amount)
		}
		return cs.nachrechnenTreuhandBestandLocked(tx, wallet, blockZeit, true)
	case "escrow_recover":
		if blockZeit < fruehesterKettenstartUnix+inactivityEscrowSeconds {
			return nachrechnenAbweichung("treuhand_zu_frueh", blockZeit,
				"escrow_recover %s %.6f AEQ vor der ersten moeglichen Treuhand", kurzAdresse(wallet), tx.Amount)
		}
		return cs.nachrechnenTreuhandBestandLocked(tx, wallet, blockZeit, false)
	}
	return nil
}

// treuhandBestandLocked: die eigene Treuhand-Zeile einer Wallet, in der
// laufenden Transaktion des Nachspielens (frueher im selben Block angelegte
// oder entfernte Zeilen sind also schon beruecksichtigt). Ohne Datenbank
// (Tests) gibt es keine Zeilen: gibt=false, kein Fehler -- die Bestandsregeln
// greifen dann nicht, siehe nachrechnenTreuhandBestandLocked.
func (cs *ChainState) treuhandBestandLocked(wallet string) (betrag float64, movedAt int64, gibt bool, err error) {
	if cs.db == nil {
		return 0, 0, false, nil
	}
	err = cs.dbExecCtx(context.Background()).QueryRow(
		`SELECT amount, moved_at FROM escrow_accounts WHERE wallet_address = $1`, wallet,
	).Scan(&betrag, &movedAt)
	if err == sql.ErrNoRows {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	return betrag, movedAt, true, nil
}

// nachrechnenTreuhandBestandLocked: Freigabe (inTopf) oder Rueckholung gegen
// den eigenen Bestand.
func (cs *ChainState) nachrechnenTreuhandBestandLocked(tx *Transaction, wallet string, blockZeit int64, inTopf bool) error {
	if cs.db == nil {
		return nil // ohne Datenbank kein Bestand (nur Tests)
	}
	// Ohne Wallet gibt es keine Zeile -> treuhand_ohne_bestand unten.
	bestand, movedAt, gibt, err := cs.treuhandBestandLocked(wallet)
	if err != nil {
		// Fail-closed: ohne lesbaren Bestand wird nicht geglaubt.
		return nachrechnenAbweichung("treuhand_unlesbar", blockZeit, "%s: %v", kurzAdresse(wallet), err)
	}
	if !gibt {
		return nachrechnenAbweichung("treuhand_ohne_bestand", blockZeit,
			"%s %s %.6f AEQ, aber keine Treuhand", tx.Type, kurzAdresse(wallet), tx.Amount)
	}
	if NewDecimal(tx.Amount).Micro() > NewDecimal(bestand).Micro()+1 {
		return nachrechnenAbweichung("treuhand_ueber_bestand", blockZeit,
			"%s %s %.6f AEQ, in der Treuhand %.6f", tx.Type, kurzAdresse(wallet), tx.Amount, bestand)
	}
	if inTopf && blockZeit < movedAt+escrowToUBISeconds-treuhandUhrSpielraum {
		return nachrechnenAbweichung("treuhand_zu_frueh", blockZeit,
			"escrow_release %s: in der Treuhand seit %d, frei erst ab %d",
			kurzAdresse(wallet), movedAt, movedAt+escrowToUBISeconds)
	}
	return nil
}
