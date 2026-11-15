package keeper

import "testing"

// Derselbe Satz wie im Coordinator (pruefer_stimme.quanten_stimm_nachricht,
// test_quanten_satz_wie_die_kette) und in der App (lib/quantengeld.ts,
// quantenStimmSatz): unterschreibt der Coordinator etwas anderes, als der
// Knoten prueft, zaehlt keine Stimme.
func TestQuantenFreischaltungNachricht_WieCoordinatorUndApp(t *testing.T) {
	if got := quantenFreischaltungNachricht("s", "123", 5)("", 7); got != "aequitas-quanten-freischaltung-v1|chain:1926|s|123|5|7" {
		t.Fatalf("Satz der Freischaltung geaendert: %q", got)
	}
}
