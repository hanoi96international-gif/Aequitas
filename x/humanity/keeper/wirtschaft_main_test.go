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
//
// Dasselbe fuer Stufe 1.0 (signierte Ueberweisungen, aktiv ab 26.09.2026
// 21:00 UTC): viele Tests stellen die Uhr auf 1.800.000.000 und spielen
// unsignierte Ueberweisungen nach. Die Tests der Signaturpruefung setzen
// signierteUeberweisungenOverride selbst.
func TestMain(m *testing.M) {
	wirtschaftAktivOverride.Store(math.MaxInt64)
	signierteUeberweisungenOverride.Store(math.MaxInt64)
	os.Exit(m.Run())
}

func wirtschaftAn(t *testing.T) {
	t.Helper()
	wirtschaftAktivOverride.Store(1)
	t.Cleanup(func() { wirtschaftAktivOverride.Store(math.MaxInt64) })
}
