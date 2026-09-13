package keeper

import "testing"

// Selbstheilung nur mit ausdruecklicher Erlaubnis UND signierter Quelle --
// nie auf den Gruenderboxen (Env unset), nie ohne Snapshot-Quelle.
func TestDivergenzAutoResyncErlaubt(t *testing.T) {
	if divergenzAutoResyncErlaubt(divergenzSchwelle, "", true) {
		t.Fatal("ohne Env darf nichts passieren")
	}
	if divergenzAutoResyncErlaubt(divergenzSchwelle, "1", false) {
		t.Fatal("ohne Quelle darf nichts passieren")
	}
	if divergenzAutoResyncErlaubt(divergenzSchwelle-1, "1", true) {
		t.Fatal("unter der Schwelle darf nichts passieren")
	}
	if !divergenzAutoResyncErlaubt(divergenzSchwelle, " 1 ", true) {
		t.Fatal("Schwelle + Env + Quelle muss ausloesen")
	}
}
