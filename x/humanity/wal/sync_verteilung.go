package wal

import (
	"sort"
	"sync"
	"time"
)

// DIE VERTEILUNG, NICHT DAS MITTEL.
//
// sync_avg_us meldete am 12.09.2026 unter Last 8 bis 11 ms je fdatasync.
// Eine Sonde auf derselben Platte, zur selben Zeit, in derselben Form
// (vorbelegte Datei, kleine Schreibvorgaenge, fdatasync nach jedem) mass
// 1,3 bis 2,0 ms im Mittel bei einem Median von 0,6 bis 0,85 ms. Ein Faktor
// vier bis sechs zwischen zwei Messungen desselben Vorgangs.
//
// Ein Mittel kann das nicht aufloesen: sync_max_us stand bei 746 ms, und ein
// einziger Ausreisser dieser Groesse je hundert Syncs hebt das Mittel um
// 7 ms. Ob also jeder Sync 8 ms braucht (dann ist die Platte das Problem)
// oder ob die meisten unter einer Millisekunde liegen und wenige bei einer
// halben Sekunde (dann sind es Kompaktierung, Checkpoints, Journal), ist aus
// dem Mittel nicht zu lesen -- und die beiden Antworten fuehren zu
// gegensaetzlichen Massnahmen.
//
// Ein Ring der letzten Messwerte, daraus Perzentile. Nicht ueber die ganze
// Laufzeit: eine Stunde Leerlauf wuerde jede Lastphase verduennen.

const syncRingGroesse = 4096

var syncRing struct {
	mu    sync.Mutex
	werte [syncRingGroesse]int64 // Nanosekunden
	pos   int
	voll  bool
}

func merkeSyncDauer(d time.Duration) {
	syncRing.mu.Lock()
	syncRing.werte[syncRing.pos] = int64(d)
	syncRing.pos++
	if syncRing.pos == syncRingGroesse {
		syncRing.pos = 0
		syncRing.voll = true
	}
	syncRing.mu.Unlock()
}

// SyncVerteilung liefert Perzentile der letzten Syncs in Mikrosekunden.
func SyncVerteilung() map[string]interface{} {
	syncRing.mu.Lock()
	n := syncRing.pos
	if syncRing.voll {
		n = syncRingGroesse
	}
	kopie := make([]int64, n)
	copy(kopie, syncRing.werte[:n])
	syncRing.mu.Unlock()
	if n == 0 {
		return map[string]interface{}{"anzahl": 0}
	}
	sort.Slice(kopie, func(i, j int) bool { return kopie[i] < kopie[j] })
	p := func(q float64) int64 {
		i := int(float64(n-1) * q)
		return kopie[i] / 1000
	}
	var summe int64
	for _, v := range kopie {
		summe += v
	}
	// Wie viel des Mittels stammt aus dem obersten Prozent? Das ist die
	// Zahl, die zwischen "Platte langsam" und "seltene Ausreisser" entscheidet.
	obersteAb := int(float64(n) * 0.99)
	var obersteSumme int64
	for _, v := range kopie[obersteAb:] {
		obersteSumme += v
	}
	return map[string]interface{}{
		"bedeutung": "Verteilung der letzten fdatasync-Dauern des WAL-Schreibers. Ein Mittel von " +
			"8 ms kann 'jeder Sync 8 ms' heissen (Platte) oder 'die meisten 0,6 ms, wenige 500 ms' " +
			"(Kompaktierung, Checkpoints, Journal) -- die Massnahmen sind gegensaetzlich.",
		"anzahl":                     n,
		"p50_us":                     p(0.50),
		"p90_us":                     p(0.90),
		"p99_us":                     p(0.99),
		"max_us":                     kopie[n-1] / 1000,
		"mittel_us":                  summe / int64(n) / 1000,
		"anteil_mittel_aus_top1_pct": float64(obersteSumme) / float64(summe) * 100,
	}
}
