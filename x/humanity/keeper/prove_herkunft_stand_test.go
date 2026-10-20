package keeper

import "testing"

// /api/health/combined meldet das Tor vor dem Praegen; wache_pruefung.py
// wird rot, wenn "verlangt" nicht true ist. Missbrauch: ein Betreiber setzt
// REQUIRE_PROVE_PROVENANCE=false -- das muss von aussen sichtbar sein.
func TestProveHerkunftStand(t *testing.T) {
	for _, f := range []struct {
		wert     string
		verlangt bool
	}{
		{"", true},
		{"true", true},
		{"ja", true},
		{"false", false},
		{" FALSE ", false},
	} {
		t.Setenv("REQUIRE_PROVE_PROVENANCE", f.wert)
		if got := ProveHerkunftStand()["verlangt"]; got != f.verlangt {
			t.Errorf("REQUIRE_PROVE_PROVENANCE=%q: verlangt=%v, erwartet %v", f.wert, got, f.verlangt)
		}
	}
}
