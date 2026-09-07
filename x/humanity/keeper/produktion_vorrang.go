package keeper

import (
	"sync/atomic"
	"time"
)

// Der Blockproduktion Vorrang vor dem Nachvollziehen geben.
//
// GEMESSEN am 07.09.2026 mit der Phasenuhr (produktion_phasen.go), teuerster
// Blockbau auf der Primary:
//
//	gesamt            5.903 ms
//	  Sperren warten  4.241 ms   <- 72 %
//	  speichern       1.192 ms
//	  DB-Paar           376 ms
//	  REST                0,1 ms
//
// Alle anderen Verdaechtigen waren zuvor einzeln gemessen und ausgeschlossen:
// die Zustandssperre war zu 2,9 % belegt, der Replay brauchte im Mittel 57 ms,
// das Sync-Tor stand offen. Der Posten ist die DAG-Sperre, und die haelt
// AddPeerBlock je fremdem Block -- bei 7.000 Ueberweisungen darin rund eine
// Sekunde. Kommen acht solche Bloecke in einer Sync-Seite, wartet die
// Produktion durch alle acht hindurch.
//
// Go's Mutex hilft hier nicht von allein: sie geht zwar nach 1 ms Wartezeit in
// den fairen Modus, das nuetzt aber nichts, wenn der aktuelle HALTER die
// Sperre eine Sekunde am Stueck behaelt. Der Wartende kommt erst zwischen zwei
// Bloecken zum Zug -- und dort greift der Sync sie sofort wieder.
//
// Deshalb ein Signal statt einer Sperrenaenderung: die Produktion meldet an,
// dass sie wartet, und der Sync laesst sie zwischen zwei Bloecken vorbei. Das
// kostet den Sync ein paar Millisekunden je Block und bringt der Kette einen
// Block, der sonst einen ganzen Takt spaeter erschienen waere.
//
// BEWUSST KEINE SPERRE UM DAS SIGNAL: es ist ein Hinweis, keine Zusicherung.
// Wird er einmal verpasst, wartet die Produktion eben einen Block laenger --
// derselbe Zustand wie ohne diesen Code. Es gibt nichts zu verlieren und
// keinen neuen Weg, auf dem zwei Goroutinen einander blockieren koennten.
var (
	produktionWartet atomic.Int32
	vorrangGewaehrt  atomic.Int64
	vorrangGeprueft  atomic.Int64
)

// produktionMeldetWarten wird vor dem Sperrversuch der Blockproduktion
// gerufen; die zurueckgegebene Funktion meldet das Ende an.
func produktionMeldetWarten() func() {
	produktionWartet.Add(1)
	var einmal atomic.Bool
	return func() {
		if einmal.CompareAndSwap(false, true) {
			produktionWartet.Add(-1)
		}
	}
}

// syncLaesstProduktionVor gibt der wartenden Blockproduktion zwischen zwei
// fremden Bloecken die Sperre frei. Aufzurufen, wenn KEINE Sperre gehalten
// wird -- sonst waere das Warten wirkungslos und im schlimmsten Fall schaedlich.
func syncLaesstProduktionVor() {
	vorrangGeprueft.Add(1)
	if produktionWartet.Load() <= 0 {
		return
	}
	vorrangGewaehrt.Add(1)
	// Lang genug, dass die wartende Goroutine tatsaechlich drankommt (Go's
	// Mutex uebergibt im fairen Modus an den laengsten Wartenden), kurz genug,
	// dass ein Aufholen darunter nicht leidet: bei 500 Bloecken je Seite sind
	// das im schlechtesten Fall eine Sekunde ueber die ganze Seite.
	time.Sleep(2 * time.Millisecond)
}

// ProduktionsVorrangStand meldet, wie oft der Sync tatsaechlich Platz gemacht hat.
func ProduktionsVorrangStand() map[string]interface{} {
	gepr := vorrangGeprueft.Load()
	gew := vorrangGewaehrt.Load()
	var anteil float64
	if gepr > 0 {
		anteil = 100 * float64(gew) / float64(gepr)
	}
	return map[string]interface{}{
		"bedeutung": "Wie oft der Sync zwischen zwei fremden Bloecken einer wartenden " +
			"Blockproduktion Platz gemacht hat. gewaehrt=0 heisst, dass die Produktion nie " +
			"gleichzeitig wartete -- dann ist die Sperre nicht der Engpass. Ein hoher Anteil " +
			"heisst, dass sie es sehr wohl war.",
		"geprueft":    gepr,
		"gewaehrt":    gew,
		"anteil_pct":  anteil,
		"wartet_grad": produktionWartet.Load(),
	}
}

// ProduktionsVorrangZuruecksetzen gibt es fuer die Tests.
func ProduktionsVorrangZuruecksetzen() {
	produktionWartet.Store(0)
	vorrangGewaehrt.Store(0)
	vorrangGeprueft.Store(0)
}
