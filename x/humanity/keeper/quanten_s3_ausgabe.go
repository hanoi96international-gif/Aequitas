package keeper

// WEG C, Schritt C2b-2b-3b (forschung/quantengeld/WEG_C.md): die Ausgabe
// eines Tokens ueber die gezogene Teilmenge einer Vormerkung.
// Spezifikation: s_kette.KetteS3.ausgeben_s3.
//
// quanten_w_s_ausgeben mit einem K, das ueber eine Leitung vorgemerkt ist
// (quanten_s3_vormerkung.go), ist erst nach dem Ziehen moeglich und nur so:
//
//   - der Ausgeber der Vormerkung, ihre Inhaberin, ihre Wurzel, ihre Deckung;
//     n = Zahl der gezogenen Pulse. Stelle i des Tokens ist Puls teil[i]: der
//     Token merkt sich die Pulse (Pulse, NPuls), und jede Pruefung beim
//     Einloesen geht gegen das Blatt teil[i] im Baum ueber ALLE N Pulse --
//     dieselbe Wurzel, die vor dem ersten Puls feststand. Ein Ausgeber kann
//     die Werte also nicht nach der Meldung oder dem Ziehen anpassen.
//   - Pruefgroesse: k = quantenSKLeitung(tol, t, v, theta, frei) mit den
//     Werten, die die Vormerkung festhaelt (nicht die heutigen des
//     Registers). k = 0 (nicht sicher) oder 2k > n: abgewiesen. Der Token
//     merkt sich k (KMin); jede Stichprobe beim Anmelden (auch die eines
//     Nachweises) braucht mindestens k Stellen. Bei der Ausgabe stehen die
//     Stichproben nur als Hash fest, ihre Groesse zeigt erst das Anmelden.
//   - Geld: die Deckung liegt seit der Vormerkung im Pfandtopf und geht genau
//     einmal in den W-Topf (der Ausgeber zahlt nicht noch einmal); das Pfand
//     geht an ihn zurueck. Die Vormerkung ist danach weg.
//   - Zeit: nicht nach der Frist der Vormerkung (Blockzeit; eine Frist-
//     Transaktion zahlt dann zurueck).
//
// Sonst gilt alles wie bei jeder S-Ausgabe (Validatoren, Stellen, Proben,
// Unterschriften, Register).
//
// GRENZE BEIM OFFENLEGEN: Pfade haben die Tiefe des Baums ueber alle Pulse
// (bis 26 statt 14 Glieder, quantenSToken.pfadMax). Eine Offenlegung muss
// unter quantenWKoerperMax bleiben; wer offenlegt, waehlt dafuer kleinere
// Teile (1..quantenWOffenTeil Stellen sind erlaubt).
//
// SCHLAFEND wie die Vormerkung (quantenS3Aktiv): vor dem Stichtag gibt es
// kein vorgemerktes K, und jede S-Ausgabe laeuft wie bisher.

import "sync/atomic"

// quantenSKLeitungOverride: NUR Tests der Ablaeufe (0 = Tabelle gilt), wie
// quantenWKNoteOverride.
var quantenSKLeitungOverride atomic.Int64

// quantenS3KFuer: Pruefgroesse eines Tokens ueber eine Leitung.
func quantenS3KFuer(tol, t, v, theta, frei int) int {
	if o := quantenSKLeitungOverride.Load(); o != 0 {
		if _, ok := quantenWKTabelle[tol]; !ok {
			return 0
		}
		return int(o)
	}
	return quantenSKLeitung(tol, t, v, theta, frei)
}

// quantenS3AusgabePruefen: die Bedingungen oben. k > 0 = bestanden; sonst
// der Befund. zeit = Blockzeit.
func quantenS3AusgabePruefen(v *quantenS3Vormerkung, wallet string, q *QuantenWAngaben, wurzel [32]byte, zeit int64) (int, quantenWBescheid) {
	K := q.K
	if !quantenS3Aktiv(zeit) {
		return 0, quantenWBefund("quanten_w_doppelt", "%s ist ueber eine Leitung vorgemerkt", K[:12])
	}
	if v.Runde == 0 {
		return 0, quantenWBefund("quanten_s3_nicht_gezogen", "%s ist vorgemerkt, aber noch nicht gezogen", K[:12])
	}
	if quantenS3Verfallen(v, zeit) {
		return 0, quantenWBefund("quanten_w_frist", "Vormerkung %s ist verfallen", K[:12])
	}
	if wallet != v.Von || q.Inhaber != v.Inhaber || q.Deckung != v.Deckung || wurzel != v.Wurzel || q.N != len(v.Teil) {
		return 0, quantenWBefund("quanten_s3_ausgabe", "Ausgabe passt nicht zur Vormerkung (Ausgeber, Inhaberin, Deckung, Wurzel oder n = %d)", len(v.Teil))
	}
	k := quantenS3KFuer(q.Tol, q.T, len(q.Validatoren), v.Theta, v.Frei)
	if k == 0 || 2*k > q.N {
		return 0, quantenWBefund("quanten_s3_pruefgroesse", "Pruefgroesse der Leitung %d bei n = %d (tol %d, t %d, v %d)", k, q.N, q.Tol, q.T, len(q.Validatoren))
	}
	return k, quantenWBescheid{}
}
