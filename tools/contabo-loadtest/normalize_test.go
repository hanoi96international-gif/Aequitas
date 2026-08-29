package main

import "testing"

// Am 29.08.2026 zerfiel EINE Ursache in 23 scheinbar verschiedene, weil
// "batch member 9/26" nicht normalisiert wurde: ein Bruch ist nicht rein
// numerisch. Jede Variante bekam eine kleine Zahl, und die tatsaechlich
// haeufigste Ursache war in der Aufstellung nicht mehr zu erkennen.
func TestNormalizeErrForTally_BuendelPositionIstKeineEigeneUrsache(t *testing.T) {
	a := normalizeErrForTally("Transfer failed: batch member 9/26 (0x29ada0ff0e9845e6b16a518515f21e9e2fe2c750 -> 0x8ae8d22747bd4369fd732617673fed7554886807)")
	b := normalizeErrForTally("Transfer failed: batch member 12/33 (0xdbee57d88c6ea6eddd8f7a7a7e1ce2e67304db1b -> 0xa388bb2d38ed58c7c2a5c1db21e9778c24df39d4)")
	if a != b {
		t.Fatalf("zwei Meldungen derselben Ursache ergeben verschiedene Schluessel:\n  %s\n  %s", a, b)
	}
}

func TestIsFraction(t *testing.T) {
	for _, s := range []string{"9/26", "1/1", "12/330"} {
		if !isFraction(s) {
			t.Fatalf("%q sollte als Bruch erkannt werden", s)
		}
	}
	for _, s := range []string{"", "/", "9/", "/26", "a/b", "9", "9/2a"} {
		if isFraction(s) {
			t.Fatalf("%q ist kein Bruch", s)
		}
	}
}
