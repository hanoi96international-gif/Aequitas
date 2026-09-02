package keeper

import "testing"

// EINE Abweisung darf die Heilung nicht ausloesen -- sie kann ein echt
// ungueltiger Block eines fehlerhaften oder boesartigen Peers sein, und den zu
// verwerfen ist genau richtig. Daraufhin den eigenen Zustand wegzuwerfen waere
// ein Angriffsweg.
func TestReplayMauer_EineAbweisungReichtNicht(t *testing.T) {
	replayMauerStand = replayMauer{}
	if mauer, _ := merkeBlockAbweisung(100); mauer {
		t.Fatal("eine einzelne Abweisung darf die Heilung nicht ausloesen -- sonst " +
			"kann ein Peer mit einem einzigen ungueltigen Block einen Resync erzwingen")
	}
}

// DIESELBE Hoehe dreimal hintereinander heisst: der Knoten kommt nicht weiter.
// Genau das war am 02.09.2026 der Zustand des Primary -- Block 5517970
// viermal abgewiesen, danach 1.425 Bloecke Rueckstand.
func TestReplayMauer_DieselbeHoeheDreimalIstDieMauer(t *testing.T) {
	replayMauerStand = replayMauer{}
	for i := 1; i < replayMauerSchwelle; i++ {
		if mauer, _ := merkeBlockAbweisung(5517970); mauer {
			t.Fatalf("Mauer schon bei Versuch %d gemeldet, Schwelle ist %d", i, replayMauerSchwelle)
		}
	}
	mauer, folgen := merkeBlockAbweisung(5517970)
	if !mauer {
		t.Fatalf("nach %d Abweisungen derselben Hoehe muss die Mauer erkannt sein", replayMauerSchwelle)
	}
	if folgen != replayMauerSchwelle {
		t.Fatalf("folgen=%d, erwartet %d", folgen, replayMauerSchwelle)
	}
}

// VERSCHIEDENE Hoehen sind keine Mauer -- der Knoten verwirft dann einzelne
// schlechte Bloecke und kommt trotzdem voran.
func TestReplayMauer_VerschiedeneHoehenSindKeineMauer(t *testing.T) {
	replayMauerStand = replayMauer{}
	for h := int64(1); h <= 10; h++ {
		if mauer, _ := merkeBlockAbweisung(h); mauer {
			t.Fatalf("Mauer bei Hoehe %d gemeldet, obwohl jede Abweisung eine andere "+
				"Hoehe betraf -- das ist kein Feststecken", h)
		}
	}
}

// Ein erfolgreicher Block loescht die Zaehlung: der Knoten kommt voran.
func TestReplayMauer_ErfolgLoeschtDieZaehlung(t *testing.T) {
	replayMauerStand = replayMauer{}
	merkeBlockAbweisung(500)
	merkeBlockAbweisung(500)
	merkeBlockErfolg()
	if mauer, folgen := merkeBlockAbweisung(500); mauer || folgen != 1 {
		t.Fatalf("nach einem Erfolg muss neu gezaehlt werden, bekam mauer=%v folgen=%d",
			mauer, folgen)
	}
}
