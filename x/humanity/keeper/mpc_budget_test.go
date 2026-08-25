package keeper

import "testing"

// Die Zahlen hier sind der Grund, warum es diese Datei gibt: sie machen die
// quadratische Grenze sichtbar, bevor sie eine Registrierung scheitern laesst.

func TestTripleKostenWaechstLinearMitDemBestand(t *testing.T) {
	// 2*512 Tripel je Vergleich, durch die Verifikation verdoppelt.
	for _, f := range []struct{ bestand, erwartet int }{
		{0, 0},
		{1, 2048},
		{10, 20480},
		{43, 88064},
	} {
		if got := tripleKosten(f.bestand, 512); got != f.erwartet {
			t.Errorf("tripleKosten(%d) = %d, erwartet %d", f.bestand, got, f.erwartet)
		}
	}
}

func TestVorratReichtFuerVierundvierzigRegistrierungen(t *testing.T) {
	// Die ausgelieferte Datei: 48 MB / 24 Byte = 2.000.000 Tripel.
	const vorrat = 2_000_000
	got := registrierungenAusVorrat(vorrat, 0, 512)
	if got != 44 {
		t.Errorf("aus %d Tripeln folgen %d Registrierungen, erwartet 44", vorrat, got)
	}
}

func TestSpaetereRegistrierungenSindTeurer(t *testing.T) {
	// Der springende Punkt: derselbe Vorrat reicht ab einem gefuellten
	// Bestand fuer deutlich WENIGER Registrierungen. Wer die Zahl einmal
	// abliest und fuer konstant haelt, plant falsch.
	const vorrat = 2_000_000
	ausLeer := registrierungenAusVorrat(vorrat, 0, 512)
	ausGefuellt := registrierungenAusVorrat(vorrat, 40, 512)
	if ausGefuellt >= ausLeer {
		t.Errorf("ab Bestand 40 waeren %d Registrierungen moeglich, aus dem Leeren %d — "+
			"die Kosten muessen mit dem Bestand steigen", ausGefuellt, ausLeer)
	}
	t.Logf("aus dem Leeren: %d, ab Bestand 40: %d", ausLeer, ausGefuellt)
}

func TestLeererVorratLiefertNull(t *testing.T) {
	if got := registrierungenAusVorrat(0, 0, 512); got != 0 {
		t.Errorf("ohne Tripel = %d Registrierungen, erwartet 0", got)
	}
	if got := registrierungenAusVorrat(-5, 0, 512); got != 0 {
		t.Errorf("negativer Vorrat = %d, erwartet 0", got)
	}
}

func TestErsteRegistrierungIstGratis(t *testing.T) {
	// Gegen einen leeren Bestand gibt es nichts zu vergleichen. Das darf
	// nicht als "Vorrat erschoepft" durchgehen.
	if tripleKosten(0, 512) != 0 {
		t.Error("die erste Registrierung darf nichts kosten")
	}
}
