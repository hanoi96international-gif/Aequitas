package keeper

import "sync/atomic"

// Eine einzige Hoehe fuer /api/status -- egal welchen Weg die Anfrage nimmt.
//
// # DER FEHLER
//
// handleStatus hat zwei Wege. Ist die DAG-Sperre frei, meldet er
// latest.Height aus TryLatestBlock(); ist sie von einem Block-Burst belegt,
// meldet er HeightSchnell() aus dem Zwischenspeicher. Zwei Quellen fuer
// dieselbe Zahl.
//
// Unter Last wechselt der Endpunkt zwischen beiden hin und her, und weichen
// sie voneinander ab, SPRINGT die gemeldete Hoehe. Live gemessen am
// 03.09.2026 auf dem Primary, ohne dass die Kette selbst irgendetwas
// Falsches tat:
//
//	C1=5531338   (frischer Weg, latest.Height)
//	C1=5531297   (Zwischenspeicher, HeightSchnell -- 41 zurueck)
//	C1=5531340   (frischer Weg)
//
// Wer den Explorer offen hat, sieht eine Kette, die rueckwaerts laeuft. Das
// ist das Bild eines schwer gestoerten Knotens -- erzeugt von der
// Statusauskunft, nicht vom Konsens. Genau diese Anzeige hat den Verdacht auf
// Instabilitaet getragen, waehrend beide Boxen nachweislich sauber
// konvergierten (stability-under-load, 476.810 Ueberweisungen, 0 Fehler).
//
// # WARUM NICHT EINFACH GLAETTEN
//
// Ein Monotonie-Deckel wuerde den Sprung verstecken. Das waere falsch: die
// Hoehe MUSS zurueckgehen duerfen, wenn wirklich resynchronisiert wird
// (setHeight(0) beim Wipe, setHeight(cp.Height) beim Checkpoint). Eine
// Anzeige, die das verschweigt, luegt beim naechsten echten Vorfall.
//
// Deshalb: EINE Quelle statt Glaetten. Beide Wege melden HeightSchnell() --
// den sperrfreien Wert, der auch dann stimmt, wenn die Sperre belegt ist.
// Damit kann die Zahl gar nicht mehr zwischen zwei Auskuenften springen, und
// ein echter Ruecksprung bleibt sichtbar.
//
// Der Zaehler daneben beantwortet die Frage, die sonst offen bliebe: laufen
// die beiden Werte ueberhaupt je auseinander, und wie weit? Ohne ihn waere
// die Vereinheitlichung eine Vermutung.
var (
	hoehenAbweichungen  atomic.Int64 // wie oft latest.Height != heightSchnell
	hoehenAbweichungMax atomic.Int64 // groesster gesehener Betrag
	hoehenVergleiche    atomic.Int64
)

// merkeHoehenAbweichung vergleicht die beiden Quellen, ohne eine davon zu
// bevorzugen. Reine Buchhaltung: der Aufrufer meldet ohnehin schon
// HeightSchnell().
func merkeHoehenAbweichung(ausBlock, schnell int64) {
	hoehenVergleiche.Add(1)
	d := ausBlock - schnell
	if d < 0 {
		d = -d
	}
	if d == 0 {
		return
	}
	hoehenAbweichungen.Add(1)
	for {
		alt := hoehenAbweichungMax.Load()
		if d <= alt || hoehenAbweichungMax.CompareAndSwap(alt, d) {
			return
		}
	}
}

// HoehenQuellenStand zeigt in /api/health/combined, ob die beiden Quellen je
// auseinanderlaufen.
func HoehenQuellenStand() map[string]interface{} {
	n := hoehenVergleiche.Load()
	ab := hoehenAbweichungen.Load()
	anteil := 0.0
	if n > 0 {
		anteil = float64(ab) / float64(n) * 100
	}
	return map[string]interface{}{
		"vergleiche":     n,
		"abweichungen":   ab,
		"abweichung_pct": anteil,
		"abweichung_max": hoehenAbweichungMax.Load(),
		"bedeutung": "latest.Height gegen heightSchnell. /api/status meldet seit dem " +
			"03.09.2026 nur noch heightSchnell -- vorher wechselte es je nach Sperrlage " +
			"zwischen beiden, was die gemeldete Hoehe springen liess (gemessen: 41 Bloecke " +
			"rueckwaerts auf dem Primary, ohne Fehler im Konsens)",
	}
}
