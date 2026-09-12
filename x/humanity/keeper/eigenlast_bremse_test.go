package keeper

import (
	"testing"
	"time"
)

func eigenlastZuruecksetzen() {
	eigenlastCap.Store(0)
	eigenlastLetzteDauerNs.Store(0)
}

// Ein Takt, der nicht passt, schrumpft den eigenen Deckel; ein kurzer laesst
// ihn wieder wachsen -- bis zum harten Deckel, nie darueber, nie unter den
// Boden.
func TestEigenlastBremse_SchrumpftUndErholtSich(t *testing.T) {
	eigenlastZuruecksetzen()
	defer eigenlastZuruecksetzen()
	alt := configuredBlockTimeSeconds
	configuredBlockTimeSeconds = 1
	defer func() { configuredBlockTimeSeconds = alt }()

	if d := eigenlastDeckel(7000, 1500); d != 7000 {
		t.Fatalf("ohne Messung muss der harte Deckel gelten, nicht %d", d)
	}
	merkeEigenlast(1500 * time.Millisecond)
	d1 := eigenlastDeckel(7000, 1500)
	if d1 != 5600 {
		t.Fatalf("nach einem 1,5-s-Takt: 7000*0,8 = 5600 erwartet, %d bekommen", d1)
	}
	for i := 0; i < 20; i++ {
		eigenlastDeckel(7000, 1500)
	}
	if d := eigenlastCap.Load(); d != 1500 {
		t.Fatalf("der Boden ist 1500, nicht %d", d)
	}
	merkeEigenlast(300 * time.Millisecond)
	d2 := eigenlastDeckel(7000, 1500)
	if d2 != 1500+7000/20 {
		t.Fatalf("Erholung um ein Zwanzigstel erwartet (%d), %d bekommen", 1500+7000/20, d2)
	}
	for i := 0; i < 40; i++ {
		eigenlastDeckel(7000, 1500)
	}
	if d := eigenlastCap.Load(); d != 7000 {
		t.Fatalf("Erholung muss beim harten Deckel enden, nicht bei %d", d)
	}
}

// Zwischen halber und 90 % Blockzeit passiert nichts -- kein Zittern.
func TestEigenlastBremse_MittlererTaktHaeltDenDeckel(t *testing.T) {
	eigenlastZuruecksetzen()
	defer eigenlastZuruecksetzen()
	alt := configuredBlockTimeSeconds
	configuredBlockTimeSeconds = 1
	defer func() { configuredBlockTimeSeconds = alt }()
	merkeEigenlast(1200 * time.Millisecond)
	eigenlastDeckel(7000, 1500) // 5600
	merkeEigenlast(700 * time.Millisecond)
	if d := eigenlastDeckel(7000, 1500); d != 5600 {
		t.Fatalf("bei 0,7 s darf sich nichts aendern: %d", d)
	}
}

func TestEigenlastBremse_Abschaltbar(t *testing.T) {
	eigenlastZuruecksetzen()
	defer eigenlastZuruecksetzen()
	t.Setenv(eigenlastBremseEnv, "0")
	merkeEigenlast(5 * time.Second)
	if d := eigenlastDeckel(7000, 1500); d != 7000 {
		t.Fatalf("abgeschaltet muss der harte Deckel gelten, nicht %d", d)
	}
}
