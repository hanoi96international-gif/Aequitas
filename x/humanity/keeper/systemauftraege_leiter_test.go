package keeper

import (
	"strings"
	"testing"
	"time"
)

// Missbrauch (Beta-Pruefung 14.10.2026, HOCH): mit Leitung fuehrt nur der
// Leiter mit gueltiger Lease Systemauftraege aus -- auch ohne verteilten
// Term. Vorher verteilte in Stufe 1 jeder Knoten aus denselben Toepfen.
func TestSystemauftraege_NurLeiterMitLease(t *testing.T) {
	a := "0x" + strings.Repeat("1", 40)
	b := "0x" + strings.Repeat("2", 40)
	neu := func(ich string, satz []string, start string) *ChainState {
		cs := &ChainState{}
		cs.leitungStartVersucht.Store(true)
		cs.leitung.Store(NeueLeitung(ich, "", satz, start, true, LeitSpeicher{}, testKonfig(), LeitUmgebung{}, time.Now()))
		return cs
	}

	// Ohne Leitung: wie bisher.
	ohne := &ChainState{}
	ohne.leitungStartVersucht.Store(true)
	if g := ohne.SystemauftraegeHier(); g != "" {
		t.Fatalf("ohne Leitung: %q", g)
	}
	// Einziger Validator und Leiter: ja.
	if g := neu(a, []string{a}, a).SystemauftraegeHier(); g != "" {
		t.Fatalf("Leiter: %q", g)
	}
	// Folger (der andere leitet): nein.
	if g := neu(a, []string{a, b}, b).SystemauftraegeHier(); g == "" {
		t.Fatal("Folger fuehrt Systemauftraege aus")
	}
	// Leiter, aber nur lesend: nein (er nimmt nichts an).
	nl := neu(a, []string{a}, a)
	nl.nurLesend.Store(true)
	if g := nl.SystemauftraegeHier(); g == "" {
		t.Fatal("nur lesender Leiter fuehrt Systemauftraege aus")
	}
}
