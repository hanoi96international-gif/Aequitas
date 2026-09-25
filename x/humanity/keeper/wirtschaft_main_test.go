package keeper

import (
	"math"
	"os"
	"testing"
)

// TestMain haelt alle bestehenden Tests beim Verhalten VOR den
// Unternehmensregeln (wirtschaft.go), egal an welchem Datum sie laufen --
// sonst kippten die Demurrage- und Gebuehrentests am 01.10.2026 von selbst.
// Die Tests der neuen Regeln schalten sie mit wirtschaftAn(t) gezielt ein.
func TestMain(m *testing.M) {
	wirtschaftAktivOverride.Store(math.MaxInt64)
	os.Exit(m.Run())
}

func wirtschaftAn(t *testing.T) {
	t.Helper()
	wirtschaftAktivOverride.Store(1)
	t.Cleanup(func() { wirtschaftAktivOverride.Store(math.MaxInt64) })
}
