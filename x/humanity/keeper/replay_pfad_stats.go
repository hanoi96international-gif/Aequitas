package keeper

import "sync/atomic"

// Wie viele Ueberweisungen das Nachspielen parallel schafft -- und wie viele
// nicht.
//
// # WARUM DIESE ZAHL FEHLT UND GEBRAUCHT WIRD
//
// Am 02.09.2026 wurde die vollstaendige Ursachenkette der Instabilitaet
// gemessen: der nachspielende Knoten faellt zurueck, seine Kontostaende
// weichen ab, eine einzelne Ueberweisung scheitert (`insufficient balance`),
// und daraufhin wird der GANZE Block verworfen -- einmal beobachtet mit 3.525
// Transaktionen wegen einer. Danach ist der Knoten zugemauert (61 Waisen,
// "received 1994 block(s), attached 0") bis die Selbstheilung greift.
//
// Der Einstieg in diese Kaskade ist allein das Zurueckfallen. Und das kommt
// daher, dass Replay langsamer ist als der Schnellpfad: gemessen haelt
// replayTransactions die GLOBALE Ausschlusssperre ueber den ganzen Block, im
// Mittel 106 ms und bis zu 3,4 s, waehrend der Produzent dieselben
// Transaktionen nebenlaeufig ueber Shard-Sperren anwendet.
//
// Es GIBT einen parallelen Replay-Pfad (replay_parallel.go), und er ist
// verdrahtet. Er greift aber nur fuer Laeufe von aufeinanderfolgenden,
// DEMURRAGE-FREIEN, paarweise disjunkten Ueberweisungen -- jede Demurrage
// bricht den Lauf sofort ab (collectDisjointTransferBatch), und in einem Block
// mit tausenden Ueberweisungen ueber wenige hundert Konten wiederholen sich
// die Adressen staendig.
//
// Ob er in der Praxis also fast immer oder fast nie greift, war bisher NICHT
// messbar. Genau das entscheidet aber, ob der Hebel im parallelen Pfad liegt
// oder woanders -- und an diesem Tag sind bereits fuenf plausible Hypothesen
// an Messungen gescheitert. Diese hier wird nicht geraten.
var (
	replayParallelTx atomic.Int64 // ueber den parallelen Pfad angewendet
	replaySeriellTx  atomic.Int64 // ueber den seriellen Pfad angewendet
	replayBatches    atomic.Int64 // wie viele parallele Buendel
	replayBatchMax   atomic.Int64 // groesstes gesehenes Buendel
)

func merkeReplayParallel(n int) {
	if n <= 0 {
		return
	}
	replayParallelTx.Add(int64(n))
	replayBatches.Add(1)
	for {
		alt := replayBatchMax.Load()
		if int64(n) <= alt || replayBatchMax.CompareAndSwap(alt, int64(n)) {
			return
		}
	}
}

func merkeReplaySeriell() { replaySeriellTx.Add(1) }

// ReplayPfadStand zeigt die Aufteilung in /api/health/combined.
func ReplayPfadStand() map[string]interface{} {
	p, s := replayParallelTx.Load(), replaySeriellTx.Load()
	gesamt := p + s
	anteil := 0.0
	if gesamt > 0 {
		anteil = float64(p) / float64(gesamt) * 100
	}
	b := replayBatches.Load()
	proBuendel := 0.0
	if b > 0 {
		proBuendel = float64(p) / float64(b)
	}
	return map[string]interface{}{
		"parallel":          p,
		"seriell":           s,
		"parallel_pct":      anteil,
		"buendel":           b,
		"pro_buendel":       proBuendel,
		"groesstes_buendel": replayBatchMax.Load(),
		"bedeutung": "Wie das Nachspielen die Ueberweisungen anwendet. Der parallele Pfad greift " +
			"nur fuer aufeinanderfolgende, demurrage-freie, paarweise disjunkte Laeufe -- jede " +
			"Demurrage bricht ihn ab. parallel_pct nahe 0 heisst: Replay laeuft praktisch seriell " +
			"unter der globalen Sperre, und genau das laesst den nachspielenden Knoten " +
			"zurueckfallen",
	}
}
