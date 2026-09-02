package keeper

import "testing"

// Eine leere Runde ist harmlos -- der Peer kennt den Block vielleicht noch
// nicht. Daraufhin zu resyncen waere eine Ueberreaktion und ein Angriffsweg.
func TestAhnenLeerlauf_EineRundeReichtNicht(t *testing.T) {
	ahnenLeerlaufFolgen.Store(0)
	if aussichtslos, _ := merkeAhnenLeerlauf(); aussichtslos {
		t.Fatal("eine einzelne leere Runde darf keine Heilung ausloesen")
	}
}

// Mehrere Runden hintereinander ohne einen einzigen gelieferten Elternblock
// heissen, dass der Peer die Historie nicht mehr hat -- gemessen am
// 02.09.2026: C1 hielt 580 Bloecke, C2 nur 21, weil jeder Resync
// chain_blocks leert. Aufholen ist dann unmoeglich.
func TestAhnenLeerlauf_MehrereRundenSindAussichtslos(t *testing.T) {
	ahnenLeerlaufFolgen.Store(0)
	for i := 1; i < ahnenLeerlaufSchwelle; i++ {
		if aussichtslos, _ := merkeAhnenLeerlauf(); aussichtslos {
			t.Fatalf("schon bei Runde %d aussichtslos gemeldet, Schwelle ist %d",
				i, ahnenLeerlaufSchwelle)
		}
	}
	aussichtslos, folgen := merkeAhnenLeerlauf()
	if !aussichtslos {
		t.Fatalf("nach %d leeren Runden muss die Lage als aussichtslos gelten",
			ahnenLeerlaufSchwelle)
	}
	if folgen != int64(ahnenLeerlaufSchwelle) {
		t.Fatalf("folgen=%d, erwartet %d", folgen, ahnenLeerlaufSchwelle)
	}
}

// Ein einziger gelieferter Vorfahre setzt zurueck -- der Peer liefert ja.
func TestAhnenLeerlauf_ErfolgSetztZurueck(t *testing.T) {
	ahnenLeerlaufFolgen.Store(0)
	merkeAhnenLeerlauf()
	merkeAhnenLeerlauf()
	merkeAhnenErfolg()
	if aussichtslos, folgen := merkeAhnenLeerlauf(); aussichtslos || folgen != 1 {
		t.Fatalf("nach einem Erfolg muss neu gezaehlt werden, bekam aussichtslos=%v folgen=%d",
			aussichtslos, folgen)
	}
}
