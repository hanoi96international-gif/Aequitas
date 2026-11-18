package keeper

// QUANTENGELD S -- DIE EHRLICHE VORLAGE WIRD VOR DER AUSGABE EXAKT NACHGERECHNET
//
// Ein Token kann auf zwei Arten scheitern:
//
//  1. Eine Faelschung besteht. Dagegen steht die Pruefgroesse k aus
//     quantenWKTabelle: fuer jede Toleranz hoechstens 2^-40 (~ 9e-13) je
//     Pruefung, solange weniger als t Validatoren zusammenarbeiten.
//  2. Eine EHRLICHE Vorlage faellt durch, und der Wert ist verloren. Bei S
//     stehen beide Stichproben bei der Ausgabe fest (ihr Hash steht auf der
//     Kette), und an der gewaehlten Stelle b prueft die Kette genau in den
//     Messbasen y der Inhaberin (quantenSToken.einloesen: d ^ b ^ c = y).
//     Treffer (y = u) und Fehler (x != t) je Stelle kennt das Werkzeug also
//     schon vor der Ausgabe. Es rechnet das Urteil der Kette (quantenWUrteil)
//     fuer BEIDE Stellen nach und gibt nur aus, wenn die ehrliche Vorlage an
//     beiden besteht -- sonst Abbruch, bevor ein Validator bestaetigt oder
//     etwas auf die Kette geht. Am Messfehler faellt eine ehrliche Vorlage
//     damit nie durch, bei keiner Toleranz. Kein Schaetzwert, keine
//     Wahrscheinlichkeit: dieselbe Zaehlung wie auf der Kette.
//
// Die Toleranz waehlt der Ausgeber (aequitasd quanten-s-ausgabe --toleranz);
// die Kette kennt 2, 3, 4, ... %. Kleinere Toleranz, kleineres k: bei 3 von
// 4 Validatoren 1.760 Stellen je Annahmestelle bei 2 % statt 5.200 bei 4 %.
// Wird die ehrliche Vorlage bei kleiner Toleranz zu oft abgewiesen, hilft die
// Mehrheit aus mehreren Schuessen (hardware_ibm.py --wiederholen).

import (
	"fmt"
	"slices"
)

// QuantenSToleranzStandard: Toleranz in Prozent, wenn nichts anderes gewaehlt
// ist.
const QuantenSToleranzStandard = quantenWTolStandard

// quantenSEhrlichBesteht: Treffer und Fehler der ehrlichen Vorlage je
// Annahmestelle -- beide muessen nach der Regel der Kette bestehen.
func quantenSEhrlichBesteht(treffer, fehler [2]int, tol int) error {
	for s := 0; s < 2; s++ {
		if quantenWUrteil(treffer[s], fehler[s], tol) != quantenWEcht {
			return fmt.Errorf("Annahmestelle %d: %d Fehler bei %d Treffern -- die ehrliche Vorlage bestuende bei Toleranz %d %% nicht; abgebrochen, bevor etwas auf die Kette geht (kleinere Fehler: hardware_ibm.py --wiederholen; oder groessere Toleranz)",
				s, fehler[s], treffer[s], tol)
		}
	}
	return nil
}

// quantenSToleranzen: die Toleranzen, fuer die die Kette eine Pruefgroesse
// kennt (aufsteigend).
func quantenSToleranzen() []int {
	var out []int
	for tol := range quantenWKTabelle {
		out = append(out, tol)
	}
	slices.Sort(out)
	return out
}
