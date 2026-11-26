package keeper

// STICHTAG DER PFAENDER NACH DER BLOCKZEIT (Befund der Pruefung vom 26.11.2026)
//
// Jede W-Transaktion rechnet mit ihrer Buchzeit (quantenWBuchZeit): BuchAt,
// die Zeit der Annahme, darf bis quantenWBuchSpiel (gut 7 Tage) vor dem Block
// liegen. Ob die Pfaender gelten, entschied darum die Buchzeit. Ein
// Blockerzeuger, der eine Bindung NACH dem Stichtag in einen Block schreibt,
// ihr aber eine Buchzeit VOR dem Stichtag gibt, haette die Regeln der
// Pfaender umgangen: Bindung ohne Pfand der Inhaberin (P3), Einloesen ohne
// Nachweis-Eintrag (P4), Melden ohne Pfand (P2).
//
// Darum, wie beim Wechsel der Chain-ID (chain_id_wechsel.go):
//   - Nachspielen (quantenWBuchZeit): eine W-Transaktion mit Buchzeit vor
//     dem Stichtag gehoert in einen Block vor dem Stichtag. Sonst ist der
//     Block falsch.
//   - Erzeuger (auftragsFenster): waehlt fuer sie eine Blockzeit vor dem
//     Stichtag.
//   - Annahme: quantenSPfaenderPause vor dem Stichtag nimmt der Knoten keine
//     W-Transaktion an (wiederholbar, 503). Was davor angenommen wurde, steht
//     lange vor dem Stichtag in einem Block.
//
// Solange der Stichtag schlaeft (math.MaxInt64), aendert sich nichts: kein
// Block, keine StateRoot, keine Annahme. Fuer "Los behalten" (P2a, Stichtag
// 27.11.2026 07:00 UTC, schon ausgerollt) gilt das bewusst NICHT: eine
// Regelaenderung dort braeuchte einen eigenen Stichtag, und seit dem Nachweis
// ohne Strafe der Gelosten haengt am Los kein Geld mehr.

import (
	"fmt"
	"math"
)

// quantenSPfaenderPause: so lange vor dem Stichtag der Pfaender nimmt der
// Knoten keine W-Transaktion an.
const quantenSPfaenderPause int64 = 300

// quantenSPfaenderGrenzeOk: liegen Buchzeit und Blockzeit auf derselben Seite
// des Stichtags der Pfaender? Nur "Buchzeit davor, Block danach" ist falsch
// (eine Buchzeit bis 60 s nach dem Block laesst quantenWBuchZeit zu; sie
// gilt dann schon als danach, und das ist der echte Zeitpunkt der Annahme).
func quantenSPfaenderGrenzeOk(buchAt, blockZeit int64) bool {
	ab := quantenSPfaenderAb()
	return ab == math.MaxInt64 || buchAt >= ab || blockZeit < ab
}

// quantenSPfaenderFenster: fuer den Erzeuger -- die Blockzeiten, zu denen die
// Grenze haelt (auftragsFenster).
func quantenSPfaenderFenster(tx *Transaction, f *zeitFenster) {
	if ab := quantenSPfaenderAb(); ab != math.MaxInt64 && tx.BuchAt < ab {
		f.eng(math.MinInt64, ab-1)
	}
}

// quantenSPfaenderPausePruefen: bei der Annahme (jetzt = BuchAt).
func quantenSPfaenderPausePruefen(jetzt int64) error {
	ab := quantenSPfaenderAb()
	if ab == math.MaxInt64 || jetzt >= ab || jetzt < sattAdd(ab, -quantenSPfaenderPause) {
		return nil
	}
	return fmt.Errorf("%w: Quantengeld-Pfaender gelten ab %d -- W-Transaktionen danach wieder", ErrAnnahmePausiert, ab)
}
