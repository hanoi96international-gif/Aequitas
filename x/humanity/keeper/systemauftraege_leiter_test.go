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

// Fehlerfall (Pruefung von #354): lief die Verteilung gestern erst um 20:15
// (Retry), darf heute um 20:00 kein Tag ausfallen.
func TestVerteilungHeuteOffen(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("keine Zeitzonendaten")
	}
	gestern := time.Date(2026, 10, 13, 20, 15, 0, 0, berlin)
	heute := time.Date(2026, 10, 14, 20, 0, 0, 0, berlin)
	if !VerteilungHeuteOffen(gestern.Unix(), heute, berlin) {
		t.Fatal("gestern 20:15, heute 20:00: heute muss noch offen sein")
	}
	if VerteilungHeuteOffen(heute.Add(time.Minute).Unix(), heute.Add(time.Hour), berlin) {
		t.Fatal("heute schon gelaufen: nicht offen")
	}
	// Retry ueber Mitternacht: Runde am 14. um 00:10 gehoert zum Termin des
	// 13.; am 14. um 20:00 ist der neue Termin offen.
	nachMitternacht := time.Date(2026, 10, 14, 0, 10, 0, 0, berlin)
	if !VerteilungHeuteOffen(nachMitternacht.Unix(), heute, berlin) {
		t.Fatal("Runde um 00:10: der Termin am selben Kalendertag um 20:00 muss offen sein")
	}
	if VerteilungHeuteOffen(nachMitternacht.Unix(), nachMitternacht.Add(time.Hour), berlin) {
		t.Fatal("vor 20:00 gehoert jetzt noch zum selben Termin")
	}
	if VerteilungHeuteOffen(0, heute, berlin) {
		t.Fatal("ohne letzte Verteilung entscheidet TryLockDistribution")
	}
}
