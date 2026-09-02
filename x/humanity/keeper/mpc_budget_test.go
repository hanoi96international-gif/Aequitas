package keeper

import (
	"strings"
	"testing"
)

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

func TestVorratWarntBevorErBricht(t *testing.T) {
	// Der Vorrat ist Verbrauchsgut und die Kosten wachsen quadratisch: die
	// letzten Registrierungen kosten am meisten, die Zahl faellt am Ende
	// schnell. Wer erst nachsieht, wenn sie klein ist, sieht zu spaet nach.
	if w, _ := tripelWarnung(100); w != "" {
		t.Fatalf("bei 100 verbleibenden Registrierungen ist Ruhe richtig: %q", w)
	}
	if w, _ := tripelWarnung(tripelWarnschwelle + 1); w != "" {
		t.Fatalf("eine ueber der Schwelle darf noch nicht warnen: %q", w)
	}
	w, n := tripelWarnung(tripelWarnschwelle)
	if w == "" {
		t.Fatal("genau auf der Schwelle muss gewarnt werden")
	}
	if !strings.Contains(n, "mpc-dealer") {
		t.Fatalf("die Warnung muss sagen, was zu tun ist: %q", n)
	}
}

func TestDieWarnungBehauptetNichtMehrAlsSieWeiss(t *testing.T) {
	// Die erste Fassung dieser Warnung behauptete "die naechste Registrierung
	// scheitert, und zwar fuer alle gleichzeitig". Das gilt NUR bei
	// MPC_AUTHORITATIVE=true im Vergleichsdienst -- und das ist die Ausnahme.
	// Im Schattenbetrieb haelt ein leerer Vorrat nur den Schattenvergleich an;
	// entschieden wird auf dem Klartextpfad.
	//
	// Eine Warnung, die dramatischer ist als die Lage, kostet dasselbe wie
	// eine, die sie verharmlost: beim naechsten Mal glaubt sie niemand.
	w, n := tripelWarnung(0)
	if strings.Contains(w, "Registrierung scheitert") {
		t.Fatalf("das gilt nur im scharfgeschalteten Modus: %q", w)
	}
	if !strings.Contains(n, "MPC_AUTHORITATIVE") {
		t.Fatalf("wovon die Wirkung abhaengt, muss dastehen: %q", n)
	}
	if !strings.Contains(n, "Klartextpfad") {
		t.Fatalf("dass sonst der Klartextpfad entscheidet, gehoert dazu: %q", n)
	}
}

func TestDieSchwelleZaehltRegistrierungenNichtProzent(t *testing.T) {
	// Bei quadratischen Kosten koennen 10 %% Restvorrat eine EINZIGE
	// Registrierung sein. Ein Prozentsatz waere hier beruhigend und falsch.
	if tripelWarnschwelle <= 0 {
		t.Fatal("die Schwelle muss zaehlen, was noch GEHT, nicht was noch DA ist")
	}
	if w, _ := tripelWarnung(1); w == "" {
		t.Fatal("eine einzige verbleibende Registrierung ist der lauteste Fall")
	}
}
