package keeper

import (
	"sync/atomic"
	"time"
)

// Wieviel dieser Knoten tatsaechlich in eigene Bloecke gepackt hat.
//
// WARUM ES DAS GIBT. Der Durchsatz wurde bis zum 06.09.2026 aus zwei Zahlen
// abgeleitet, die beide falsch sind:
//
//   - Die ANNAHMERATE des Lasttests zaehlt Quittungen. Sie lag bei 6.560/s,
//     waehrend die Kette nachweislich weniger trug -- der Rest stand in der
//     Warteschlange, und wer sie als Durchsatz liest, optimiert den Stau.
//
//   - Die Bloecke ueber /api/block?height=N nachzuzaehlen unterschaetzt
//     systematisch: bei GHOSTDAG produzieren BEIDE Validatoren je Hoehe einen
//     Block (live nachgemessen: 50 Bloecke ueber 25 Hoehen, exakt 2,00 je
//     Hoehe), aber dieser Endpunkt gibt nur den kanonischen zurueck. Ueber
//     /api/blocks kommen zwar beide, aber ohne Transaktionsliste, weil Bloecke
//     ausgeduennt ausgeliefert werden.
//
// Dieser Zaehler haengt an der Stelle, an der die Groesse ohnehin schon
// gemerkt wird, und zaehlt genau das, was der Knoten in einen eigenen Block
// geschrieben hat. Summiert man ihn ueber beide Validatoren, ergibt das den
// Durchsatz der Kette -- ohne Kanonizitaet, ohne Auslieferungsform, ohne
// Warteschlange dazwischen.
var (
	produzierteBloecke atomic.Int64
	produzierteTx      atomic.Int64
	produktionSeit     atomic.Int64 // Unixzeit der ersten Zaehlung
	groesstesBlock     atomic.Int64
)

func merkeProduktion(n int) {
	if n <= 0 {
		return
	}
	produktionSeit.CompareAndSwap(0, time.Now().Unix())
	produzierteBloecke.Add(1)
	produzierteTx.Add(int64(n))
	for {
		alt := groesstesBlock.Load()
		if int64(n) <= alt || groesstesBlock.CompareAndSwap(alt, int64(n)) {
			break
		}
	}
}

// ProduktionsStand meldet, was dieser Knoten selbst verblockt hat.
func ProduktionsStand() map[string]interface{} {
	b := produzierteBloecke.Load()
	tx := produzierteTx.Load()
	seit := produktionSeit.Load()
	var dauer float64
	if seit > 0 {
		dauer = float64(time.Now().Unix() - seit)
	}
	var proSek, jeBlock float64
	if dauer > 0 {
		proSek = float64(tx) / dauer
	}
	if b > 0 {
		jeBlock = float64(tx) / float64(b)
	}
	return map[string]interface{}{
		"bedeutung": "Was DIESER Knoten in eigene Bloecke geschrieben hat. Die Summe ueber alle " +
			"Validatoren ist der Durchsatz der Kette. Nicht zu verwechseln mit der Annahmerate " +
			"(transfer_path), die Quittungen zaehlt und den Rueckstand mitzaehlt, und nicht mit " +
			"dem Nachzaehlen ueber /api/block?height=N, das je Hoehe nur den kanonischen von " +
			"zwei Bloecken sieht.",
		"bloecke":            b,
		"transaktionen":      tx,
		"tx_je_block":        jeBlock,
		"tx_pro_sekunde":     proSek,
		"groesster_block":    groesstesBlock.Load(),
		"messdauer_sekunden": dauer,
	}
}

// ProduktionZuruecksetzen gibt es fuer die Tests.
func ProduktionZuruecksetzen() {
	produzierteBloecke.Store(0)
	produzierteTx.Store(0)
	produktionSeit.Store(0)
	groesstesBlock.Store(0)
}
