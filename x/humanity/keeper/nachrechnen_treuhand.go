package keeper

import (
	"context"
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
// Offen bleibt die Erhaltung NACH diesem Zeitpunkt (Freigabe <= Bestand der
// Treuhand): dafuer muss die Treuhand Konsenszustand werden. Fruehester
// Bedarf: Dezember 2028 (erste moegliche Verschiebung).

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
	case "escrow_release":
		if blockZeit < fruehesterKettenstartUnix+inactivityEscrowSeconds+escrowToUBISeconds {
			return nachrechnenAbweichung("treuhand_zu_frueh", blockZeit,
				"escrow_release %.6f AEQ vor der ersten moeglichen Freigabe", tx.Amount)
		}
	case "escrow_recover":
		if blockZeit < fruehesterKettenstartUnix+inactivityEscrowSeconds {
			return nachrechnenAbweichung("treuhand_zu_frueh", blockZeit,
				"escrow_recover %s %.6f AEQ vor der ersten moeglichen Treuhand", kurzAdresse(wallet), tx.Amount)
		}
	}
	return nil
}
