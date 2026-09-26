package keeper

// laeufe: wie viele Durchgaenge ein zufallsgetriebener Vergleichstest macht.
// Im gewoehnlichen Testlauf alle (voll); unter -race nur unterRace
// Durchgaenge. Der Race-Detektor prueft Datenrennen, nicht die Breite der
// Zufallsabdeckung -- dieselben Codepfade laufen auch mit wenigen
// Durchgaengen, die volle Breite deckt der gewoehnliche Lauf bei jedem Push
// ab. Ohne diese Grenze lag das Keeper-Paket unter -race ueber 30 Minuten
// (CI-Lauf 1008), allein vier dieser Tests brauchten zusammen gut zwoelf.
func laeufe(voll, unterRaceNur int) int {
	if unterRace {
		return unterRaceNur
	}
	return voll
}
