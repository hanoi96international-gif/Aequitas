package keeper

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
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
//
// Ebenso registerLeserAb und erzeugerSchnittAb (validator_register_leser.go):
// die Tests sehen beide schlafend, auch wenn die Konstanten ein Datum tragen,
// das schon vorbei ist. Wer den Zustand danach prueft, setzt den Override
// selbst und stellt ihn auf math.MaxInt64 zurueck (nicht auf 0 -- das waere
// die Konstante, und die gilt nach dem Stichtag).
func TestMain(m *testing.M) {
	wirtschaftAktivOverride.Store(math.MaxInt64)
	signierteUeberweisungenOverride.Store(math.MaxInt64)
	registerLeserOverride.Store(math.MaxInt64)
	erzeugerSchnittOverride.Store(math.MaxInt64)
	os.Exit(m.Run())
}

func wirtschaftAn(t *testing.T) {
	t.Helper()
	wirtschaftAktivOverride.Store(1)
	t.Cleanup(func() { wirtschaftAktivOverride.Store(math.MaxInt64) })
}

// Ein Test, der registerLeserOverride oder erzeugerSchnittOverride auf 0
// zuruecksetzt, liest danach die Konstante -- nach dem Stichtag also den
// aktiven Zustand, und kippt an einem Datum statt an einer Aenderung.
// Zurueckgesetzt wird auf math.MaxInt64 (schlafend, wie TestMain).
func TestRegisterStichtage_TestsSetzenNieAufDieKonstante(t *testing.T) {
	dateien, err := filepath.Glob("*_test.go")
	if err != nil || len(dateien) == 0 {
		t.Fatalf("keine Testdateien gefunden: %v", err)
	}
	muster := regexp.MustCompile(`(registerLeserOverride|erzeugerSchnittOverride)\.Store\(\s*0\s*\)`)
	for _, d := range dateien {
		inhalt, err := os.ReadFile(d)
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		if m := muster.Find(inhalt); m != nil {
			t.Errorf("%s: %s -- auf math.MaxInt64 zuruecksetzen, nicht auf die Konstante", d, m)
		}
	}
}
