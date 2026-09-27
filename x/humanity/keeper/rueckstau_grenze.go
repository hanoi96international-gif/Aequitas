package keeper

import (
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

// KEIN RUECKSTAU: der annehmende Knoten nimmt nur so viel an, wie in die
// naechsten Bloecke passt.
//
// # WAS PASSIERTE
//
// TPS-Messung 26.09.2026 23:20: C1 nahm 4.771 Ueberweisungen je Sekunde an,
// verblockte aber nur 3.774. Die Differenz -- angenommen, aber noch in keinem
// Block -- wuchs, solange die Last lief; C2 hing ihr hinterher. Die einzige
// Grenze war die Zahl gleichzeitig laufender RPC-Posten (inflight_grenze.go,
// am Arbeitsspeicher bemessen), nicht die Blockleistung. Ein Knoten, der
// schneller annimmt als er verblockt, baut per Konstruktion einen Rueckstau
// auf -- und jede Sekunde Rueckstau ist eine Sekunde, in der die Validatoren
// verschiedene Staende haben.
//
// # WIE
//
// Ein Messer liest alle rueckstauTakt, was angenommen und noch nicht in einem
// Block ist: offene Zeilen in pending_txs (ueber den Teilindex
// idx_pending_txs_offen) plus die WAL-Warteschlange im Speicher, die noch gar
// nicht in pending_txs steht. Liegt die Summe ueber rueckstauMax -- Vorgabe:
// zwei volle Bloecke --, lehnt admissionRefusalReason jede neue Annahme mit
// -32005 ab ("gleich nochmal"), bis die Bloecke den Rueckstau abgetragen
// haben. Die Schlange bleibt damit bei hoechstens etwa zwei Blockzeiten,
// gleich wie hoch die Last ist.
//
//	AEQUITAS_RUECKSTAU_MAX   Obergrenze in Ueberweisungen (Vorgabe 2 x Blockdeckel, 0 = aus)

const rueckstauTakt = 200 * time.Millisecond

var (
	rueckstauAktuell   atomic.Int64
	rueckstauGemessen  atomic.Int64 // Unix-Zeit der letzten Messung
	rueckstauAbgelehnt atomic.Int64
	rueckstauMesserAn  atomic.Bool
)

// rueckstauMax: die geltende Obergrenze.
func rueckstauMax() int64 {
	if raw := os.Getenv("AEQUITAS_RUECKSTAU_MAX"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n >= 0 {
			return n
		}
	}
	return 2 * int64(blockTxHartDeckel())
}

// rueckstauGrund: leer, solange angenommen werden darf.
func rueckstauGrund() string {
	grenze := rueckstauMax()
	if grenze == 0 || !rueckstauMesserAn.Load() {
		return ""
	}
	// Eine veraltete Messung (Messer haengt, DB weg) sperrt nicht: dann
	// greifen die anderen Gruende in admissionRefusalReason.
	if time.Now().Unix()-rueckstauGemessen.Load() > 5 {
		return ""
	}
	n := rueckstauAktuell.Load()
	if n <= grenze {
		return ""
	}
	rueckstauAbgelehnt.Add(1)
	return fmt.Sprintf("server busy: %d accepted transfers are waiting for the next blocks (limit %d); try again shortly", n, grenze)
}

// StarteRueckstauMesser: der Messer, nur auf einem Knoten mit Datenbank.
func (dag *BlockDAG) StarteRueckstauMesser() {
	cs := dag.state
	if cs == nil || cs.db == nil || rueckstauMesserAn.Swap(true) {
		return
	}
	SafeGoroutine("rueckstauMesser", func() {
		t := time.NewTicker(rueckstauTakt)
		defer t.Stop()
		for range t.C {
			var offen int64
			if err := cs.db.QueryRow(`SELECT count(*) FROM pending_txs WHERE included_at = 0`).Scan(&offen); err != nil {
				continue
			}
			rueckstauAktuell.Store(offen + int64(cs.WALFlushQueueDepth()))
			rueckstauGemessen.Store(time.Now().Unix())
		}
	})
}

// RueckstauStand fuer /api/health/combined.
func RueckstauStand() map[string]interface{} {
	return map[string]interface{}{
		"bedeutung": "Angenommen, aber noch in keinem Block. Ueber der Grenze lehnt der Knoten neue Ueberweisungen mit -32005 ab, bis die Bloecke aufgeholt haben -- so bleibt der Rueckstau bei hoechstens etwa zwei Bloecken.",
		"aktuell":   rueckstauAktuell.Load(),
		"grenze":    rueckstauMax(),
		"abgelehnt": rueckstauAbgelehnt.Load(),
		"messer_an": rueckstauMesserAn.Load(),
	}
}
