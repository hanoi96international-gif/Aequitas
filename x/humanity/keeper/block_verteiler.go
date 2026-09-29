package keeper

import (
	"sync/atomic"
	"time"
)

// Blockverteilung AUSSERHALB des Produktionstakts.
//
// # WARUM
//
// Gemessen am 29.09.2026 unter Last: C1 baute 1,22 Bloecke je Sekunde, obwohl
// ein Takt (ENABLE_MULTI_BLOCK_TICK) bis zu fuenf bauen darf. Ein Takt mit
// drei bis fuenf Bloecken dauerte 1,1-1,8 s, der Blockbau selbst aber nur
// 66 ms je Block (produktion_phasen). Der Rest war die Verteilung: fuer jeden
// Block kodierte der Takt ihn zweimal vollstaendig als JSON (P2P und HTTP, je
// 30-40 ms bei 7.000 signierten Ueberweisungen) und packte ihn einmal per
// gzip -- nacheinander, im Takt. Go's Ticker verwirft verpasste Ticks, also
// fiel jeder zweite Takt einfach aus: Ketten-TPS 3.841 bei 23.318/s in der
// Annahme.
//
// Jetzt stellt der Takt die gebauten Bloecke in eine Warteschlange und kehrt
// zurueck; eine einzige Goroutine verteilt sie in Produktionsreihenfolge.
// Die Arbeit ist dieselbe, aber sie laeuft neben dem naechsten Blockbau statt
// davor. Ist die Warteschlange voll, wartet der Takt auf einen freien Platz
// und bremst damit die Produktion auf das, was die Verteilung schafft, statt
// Bloecke aufzustauen, die keiner zu sehen bekommt. Die Reihenfolge bleibt
// dabei immer die der Produktion.
type BlockVerteiler struct {
	schlange chan *Block
	senden   func(*Block)

	verteilt    atomic.Int64
	gewartet    atomic.Int64 // Warteschlange voll: der Takt musste warten
	wartenNanos atomic.Int64
	sendenNanos atomic.Int64
}

// 16 Bloecke sind gut drei volle Takte. Weit unter ausduennAbHoehen (60
// Hoehen): ein Block in der Schlange verliert seinen Rumpf nicht, bevor er
// verteilt ist.
const blockVerteilerTiefe = 16

// NeuerBlockVerteiler startet die Verteil-Goroutine. senden wird fuer jeden
// Block genau einmal gerufen, in der Reihenfolge von Verteile.
func NeuerBlockVerteiler(senden func(*Block)) *BlockVerteiler {
	v := &BlockVerteiler{schlange: make(chan *Block, blockVerteilerTiefe), senden: senden}
	SafeGoroutine("block-verteiler", func() {
		for b := range v.schlange {
			v.sende(b)
		}
	})
	return v
}

func (v *BlockVerteiler) sende(b *Block) {
	start := time.Now()
	SafeCall("block-verteilen", func() { v.senden(b) })
	v.sendenNanos.Add(int64(time.Since(start)))
	v.verteilt.Add(1)
}

// Verteile uebergibt die Bloecke eines Takts, in Reihenfolge. Blockiert nur,
// wenn die Warteschlange voll ist.
func (v *BlockVerteiler) Verteile(bloecke []*Block) {
	for _, b := range bloecke {
		select {
		case v.schlange <- b:
		default:
			start := time.Now()
			v.schlange <- b
			v.gewartet.Add(1)
			v.wartenNanos.Add(int64(time.Since(start)))
		}
	}
}

var aktiverBlockVerteiler atomic.Pointer[BlockVerteiler]

// SetzeBlockVerteiler macht den Verteiler fuer /api/health/combined sichtbar.
func SetzeBlockVerteiler(v *BlockVerteiler) { aktiverBlockVerteiler.Store(v) }

// BlockVerteilerStand: verteilt, davon im Takt (Schlange voll), mittlere
// Verteilzeit je Block und aktuelle Tiefe.
func BlockVerteilerStand() map[string]interface{} {
	v := aktiverBlockVerteiler.Load()
	if v == nil {
		return map[string]interface{}{"aktiv": false}
	}
	n := v.verteilt.Load()
	jeBlock := float64(0)
	if n > 0 {
		jeBlock = float64(v.sendenNanos.Load()) / float64(n) / 1e6
	}
	return map[string]interface{}{
		"aktiv":              true,
		"verteilt":           n,
		"takt_gewartet":      v.gewartet.Load(),
		"takt_gewartet_ms":   float64(v.wartenNanos.Load()) / 1e6,
		"verteilen_je_block": jeBlock,
		"schlange":           len(v.schlange),
		"bedeutung": "Blockverteilung (JSON, gzip, P2P- und HTTP-Push) neben dem Blockbau statt im Takt. " +
			"verteilen_je_block ist die Zeit, die frueher jeden Takt verlaengerte. takt_gewartet > 0 heisst: die Verteilung " +
			"kam nicht nach und der Takt musste auf einen Platz warten.",
	}
}
