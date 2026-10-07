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
//     andere Wallet oder einen anderen Zeitpunkt, oder ohne gueltige Bindung
//     des Schluessels (Freigabe des Menschen, Besitznachweis, Mensch
//     registriert und ohne offene Staffel -- bescheinigungPruefen,
//     grant_staffel.go).
//   - erneuerung_zeit: kein Zeitpunkt, aelter als 7 Tage, oder mehr als fuenf
//     Minuten nach der Blockzeit. Die Frist ist bewusst weit: eine
//     Erneuerung, die im Ausgang des Erzeugers liegen blieb, darf nicht bei
//     jedem anderen Knoten scheitern, waehrend der Erzeuger sie angewendet
//     hat. Wiederverwenden bringt nichts -- die Erneuerung schaltet einmal
//     frei (LivenessRenewedAt > 0), mehrfach aendert nichts.
//   - erneuerung_zu_frueh: Bescheinigung oder Block vor Tag 7 nach der
//     Registrierung (erneuerungFruehestens), die Regel, die bisher nur die
//     Annahme kannte. Eine an Tag 3 ausgestellte Bescheinigung zaehlt auch
//     an Tag 8 nicht.
//
// Vor stagedGrantActivationUnix ist liveness_renewal Leerlauf und wird nicht
// geprueft. Abgelehnt wird erst im strengen Modus (nachrechnenStreng), bis
// dahin gezaehlt -- deshalb darf die Staffel nicht vor dem strengen Modus
// aktiv werden (TestStaffel_SchlaeftBisZulassungUndStreng). Das
// knotenlokale Coordinator-Register (coordinator_keys) entscheidet nichts
// mehr: die Bescheinigung traegt ihre Bindung (06.10.2026).

const (
	// Die Annahme verlangt hoechstens 15 Minuten. Bis die Transaktion in
	// einem Block steht, kann sie im Ausgang liegen (Produktionsstau,
	// Rueckstau) -- 7 Tage, damit kein ehrlicher Block daran scheitert.
	erneuerungHoechstensAlt    = 7 * 86400
	erneuerungHoechstensVoraus = 300
)

func (cs *ChainState) nachrechnenErneuerungLocked(tx *Transaction, wallet string, blockZeit int64) error {
	if !stagedGrantAktiv(blockZeit) {
		return nil
	}
	// Eine Schreibweise je Wallet: sonst laege dieselbe Erneuerung mit
	// anders geschriebener Adresse unter mehreren Transaktions-Hashes in den
	// Bloecken (zweiter Sicherheitsdurchgang #300).
	if !kanonischeAdresse(tx.Wallet) {
		return nachrechnenAbweichung("erneuerung_ohne_bescheinigung", blockZeit,
			"%q: Wallet nicht in kanonischer Schreibweise", tx.Wallet)
	}
	wallet = strings.ToLower(strings.TrimSpace(wallet))
	// Ohne Coordinator-Register: die Bescheinigung traegt ihre Bindung, und
	// ob der Mensch dahinter registriert ist, steht im Kettenzustand -- jeder
	// Knoten kommt zum selben Urteil (grant_staffel.go, bescheinigungPruefen).
	stand := func(m string) (bool, bool) {
		cs.ensureAccountLoadedCtx(context.Background(), m)
		acc, ok := cs.accounts.Get(m)
		return ok && acc.IsHuman, ok && acc.GrantStagedRest > 0
	}
	if err := bescheinigungPruefen(wallet, tx.DistributionAt, tx.Bescheinigung, stand); err != nil {
		return nachrechnenAbweichung("erneuerung_ohne_bescheinigung", blockZeit,
			"%s: keine gueltige Bescheinigung: %v", kurzAdresse(wallet), err)
	}
	if tx.DistributionAt <= 0 || blockZeit-tx.DistributionAt > erneuerungHoechstensAlt || tx.DistributionAt-blockZeit > erneuerungHoechstensVoraus {
		return nachrechnenAbweichung("erneuerung_zeit", blockZeit,
			"%s: bescheinigt %d, Blockzeit %d", kurzAdresse(wallet), tx.DistributionAt, blockZeit)
	}
	cs.ensureAccountLoadedCtx(context.Background(), wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok {
		return nil // applyLivenessRenewalDeltaLocked lehnt ab
	}
	if ab := erneuerungFruehestens(acc); ab > 0 && (blockZeit < ab || tx.DistributionAt < ab) {
		return nachrechnenAbweichung("erneuerung_zu_frueh", blockZeit,
			"%s: bescheinigt %d, Blockzeit %d, fruehestens %d (Tag 7 nach der Registrierung)",
			kurzAdresse(wallet), tx.DistributionAt, blockZeit, ab)
	}
	return nil
}
