package keeper

import (
	"context"
	"strings"
)

// liveness_renewal nachrechnen (WP 2/3, docs/LEBENDIGKEIT_GEGEN_DEEPFAKES.md).
//
// # WARUM
//
// Die zweite Lebendigkeitspruefung schaltet die Staffel frei: 800 AEQ ueber
// 30 Tage. Bescheinigt hat sie der Coordinator (Ed25519 ueber
// aequitas-liveness-renewal-v1|wallet|issued_at), geprueft wurde die
// Bescheinigung aber nur beim annehmenden Knoten -- im Block stand die
// Transaktion ohne sie. Ein Produzent haette jedes gestaffelte Konto
// freischalten koennen, ohne dass irgendwer die Pruefung gesehen hat; genau
// das, was die Staffel einer Deepfake-Farm teuer machen soll.
//
// # WAS
//
// Die Transaktion traegt die Bescheinigung (Transaction.Bescheinigung), und
// jeder Knoten prueft sie selbst, wie die anderen Nachrechen-Regeln:
//
//   - erneuerung_ohne_bescheinigung: keine, falsch unterschrieben, fuer eine
//     andere Wallet oder einen anderen Zeitpunkt, oder von einem Schluessel,
//     der nicht im Coordinator-Register steht.
//   - erneuerung_zeit: die Bescheinigung ist aelter als eine Stunde oder
//     liegt mehr als fuenf Minuten nach der Blockzeit -- eine alte
//     Bescheinigung laesst sich nicht wiederverwenden.
//   - erneuerung_zu_frueh: vor Tag 7 nach der Registrierung
//     (erneuerungFruehestens), die Regel, die bisher nur die Annahme kannte.
//
// Vor stagedGrantActivationUnix ist liveness_renewal Leerlauf und wird nicht
// geprueft. Abgelehnt wird erst im strengen Modus (nachrechnenStreng), bis
// dahin gezaehlt. Das Coordinator-Register (coordinator_keys) ist noch
// knotenlokal; es muss Konsenszustand sein, bevor die Staffel aktiv wird.

const (
	// Die Annahme verlangt hoechstens 15 Minuten; bis der Block entsteht,
	// vergeht hoechstens ein Takt. Eine Stunde laesst Spielraum fuer einen
	// verzoegerten Block und schliesst Wiederverwendung trotzdem aus.
	erneuerungHoechstensAlt    = 3600
	erneuerungHoechstensVoraus = 300
)

func (cs *ChainState) nachrechnenErneuerungLocked(tx *Transaction, wallet string, blockZeit int64) error {
	if !stagedGrantAktiv(blockZeit) {
		return nil
	}
	wallet = strings.ToLower(strings.TrimSpace(wallet))
	b := tx.Bescheinigung
	if b == nil || !livenessRenewalSignaturGueltig(cs, wallet, tx.DistributionAt, b.PublicKey, b.Signature) {
		return nachrechnenAbweichung("erneuerung_ohne_bescheinigung", blockZeit,
			"%s: keine gueltige Bescheinigung eines eingetragenen Coordinators", kurzAdresse(wallet))
	}
	if blockZeit-tx.DistributionAt > erneuerungHoechstensAlt || tx.DistributionAt-blockZeit > erneuerungHoechstensVoraus {
		return nachrechnenAbweichung("erneuerung_zeit", blockZeit,
			"%s: bescheinigt %d, Blockzeit %d", kurzAdresse(wallet), tx.DistributionAt, blockZeit)
	}
	cs.ensureAccountLoadedCtx(context.Background(), wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok {
		return nil // applyLivenessRenewalDeltaLocked lehnt ab
	}
	if ab := erneuerungFruehestens(acc); ab > 0 && blockZeit < ab {
		return nachrechnenAbweichung("erneuerung_zu_frueh", blockZeit,
			"%s: Blockzeit %d, fruehestens %d (Tag 7 nach der Registrierung)", kurzAdresse(wallet), blockZeit, ab)
	}
	return nil
}
