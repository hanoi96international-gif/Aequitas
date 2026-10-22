package keeper

import (
	"math"
	"testing"
)

// nachrechnenModus stellt den gemeinsamen Stichtag fuer einen Test fest:
// streng (ab Zeit 1) oder beobachtend (nie). Tests, die eines der beiden
// Verhalten pruefen, duerfen nicht von der Uhr abhaengen -- seit
// nachrechnenStrengAbUnix ein echtes Datum ist, wuerde ein Test, der
// "beobachten" an nowUnix() voraussetzt, an diesem Datum von selbst rot.
func nachrechnenModus(t *testing.T, streng bool) {
	t.Helper()
	if streng {
		nachrechnenStrengOverride.Store(1)
	} else {
		nachrechnenStrengOverride.Store(math.MaxInt64)
	}
	t.Cleanup(func() { nachrechnenStrengOverride.Store(0) })
}
