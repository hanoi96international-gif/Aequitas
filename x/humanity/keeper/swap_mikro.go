package keeper

// Tausch in ganzen Mikro-AEQ (Wirtschaftspruefung 29.09.2026, C8).
//
// Bis zum Stichtag rechnete jeder Tausch die Gebuehr in float64 und rundete
// Einsatz, Pool-Anteil und Gebuehr GETRENNT: dem Konto wurde
// NewDecimal(einsatz) abgezogen, gutgeschrieben wurden NewDecimal(einsatz −
// gebuehr) an die Reserve und NewDecimal(gebuehr) an die Toepfe. Die beiden
// Gutschriften ergaben nicht immer den Abzug -- ueber alle Betraege von 1 bis
// 5·10⁶ Mikro entstand in 3.712 Faellen 1 Mikro-AEQ aus dem Nichts und in 238
// Faellen verschwand eins. Winzig, aber einseitig: Geld, das niemand
// geschoepft hat.
//
// Ab swapMikroAbUnix wird die Gebuehr aus dem bereits gerundeten Einsatz in
// ganzen Mikro-Einheiten abgeleitet, und der Pool-Anteil ist genau der Rest.
// Abzug = Pool-Anteil + Gebuehr, ohne Ausnahme.
//
// Konsens: Erzeuger und Nachspielende muessen dieselbe Regel anwenden. Beide
// entscheiden ueber den Buchungsaugenblick der Transaktion (Transaction.BuchAt,
// beim Nachspielen ueber buchZeitBeimNachspielen) -- nicht ueber die eigene
// Uhr. Aeltere Bloecke werden mit der alten Rechnung nachgespielt und bleiben
// unveraendert.

// swapMikroAbUnix: 07.10.2026 00:00 UTC. Eine Woche nach dem Einbau, damit
// jeder Knoten die neue Fassung vorher hat.
const swapMikroAbUnix int64 = 1791331200

// swapTeilung zerlegt den Einsatz eines Tauschs in das, was vom Konto abgeht
// (einsatz), die Gebuehr fuer die Toepfe (gebuehr) und das, was in die
// Konstantprodukt-Formel und die Reserve geht (inPool).
//
// Ab swapMikroAbUnix gilt einsatz == inPool + NewDecimal(gebuehr) exakt.
func swapTeilung(amountIn float64, at int64) (einsatz Decimal, gebuehr float64, inPool Decimal) {
	einsatz = NewDecimal(amountIn)
	if at < swapMikroAbUnix {
		alt := amountIn * float64(swapFeeBps) / 10000.0
		return einsatz, alt, NewDecimal(amountIn - alt)
	}
	m := einsatz.Micro()
	if m <= 0 {
		return einsatz, 0, einsatz
	}
	// Ohne Ueberlauf auch fuer sehr grosse m: erst teilen, dann den Rest.
	gebuehrMikro := (m/10000)*swapFeeBps + (m%10000)*swapFeeBps/10000
	g := NewDecimalFromMicro(gebuehrMikro)
	return einsatz, g.Float(), einsatz.Sub(g)
}
