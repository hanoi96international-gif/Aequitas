package keeper

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"time"
)

// Totmann-Schalter gegen das Einfrieren.
//
// Am 07.09.2026 stand C1 mehr als zehn Minuten: Hoehe fest bei 5955248, 1.425
// Bloecke Rueckstand, KEINE einzige Logzeile, kein Absturz, kein degraded.
// HTTP antwortete weiter, der Prozess lebte -- Produktion und Sync standen.
//
// Kein vorhandener Waechter konnte greifen: der Sync-Starvation-Check verlangt
// ANKOMMENDE Bloecke ("receiving peer blocks continuously"), und beim
// Einfrieren kommt nichts an. Der Sperren-Wachhund haengt an ProduceBlock,
// das selbst nicht mehr aufgerufen wurde. Beide messen Symptome eines
// laufenden Knotens, keinen stehenden.
//
// Dieser Waechter haengt an nichts davon. Er prueft drei Dinge, die alle
// ausserhalb des blockierten Codes liegen: steht die eigene Hoehe, zieht der
// Partner davon, und kommt dabei NICHTS mehr an. Das letzte trennt das
// Einfrieren vom Aufholen -- wer aufholt, empfaengt dabei Bloecke, und den
// darf man nicht abschiessen (siehe heightStallThreshold: 25 Minuten, weil
// ein echter Aufholvorgang am 03.07.2026 rund 20 gebraucht hat).
//
// ZUERST DER ABZUG, DANN DAS ENDE. Ohne Abzug bliebe die Ursache beim
// naechsten Mal genauso unsichtbar wie beim ersten. Der Abzug geht ins Log,
// das der Neustart nicht loescht.
//
// DASS ES EIN ENDE IST, ist Absicht: ein Go-Prozess kann sich nicht selbst
// entknoten. Docker startet den Container mit `--restart unless-stopped`
// wieder, der Partner traegt die Kette in der Zwischenzeit, und der Knoten
// war nach dem manuellen Neustart am 07.09. binnen 110 Sekunden zurueck im
// Takt. Zehn Minuten Stillstand gegen zwei Minuten Neustart ist kein
// schwerer Tausch.
const (
	// So lange darf die eigene Hoehe stehen, waehrend der Partner laeuft.
	// Grosszuegig gegen jede normale Pause: gemessen wurden unter Volllast
	// hoechstens 29 Sekunden, im Leerlauf null.
	totmannStillstand = 3 * time.Minute

	// Und nur, wenn der Partner wirklich davonzieht. Ein Partner, der selbst
	// steht, ist kein Beleg gegen diesen Knoten -- dann steht die ganze Kette
	// und ein Neustart hier hilft niemandem.
	totmannMindestAbstand = 60

	totmannPruefIntervall = 20 * time.Second
)

var (
	totmannWarnungen  atomic.Int64
	totmannAusgeloest atomic.Bool
)

// StarteTotmannSchalter ueberwacht, ob dieser Knoten der Kette noch folgt.
func (dag *BlockDAG) StarteTotmannSchalter(peerURL string) {
	if peerURL == "" {
		fmt.Println("[TOTMANN] kein Partner konfiguriert — Ueberwachung auf Einfrieren bleibt aus")
		return
	}
	fmt.Printf("[TOTMANN] aktiv: beendet den Prozess, wenn die eigene Hoehe %s steht waehrend %s mindestens %d Bloecke voraus ist (Docker startet neu)\n",
		totmannStillstand, peerURL, totmannMindestAbstand)
	SafeGoroutine("totmann", func() {
		ticker := time.NewTicker(totmannPruefIntervall)
		defer ticker.Stop()
		letzteHoehe := int64(-1)
		letzteAnkuenfte := int64(-1)
		var stehtSeit time.Time
		for range ticker.C {
			eigene := dag.Height()
			ankuenfte := dag.totalRawArrivalCount.Load()
			if eigene != letzteHoehe {
				letzteHoehe = eigene
				letzteAnkuenfte = ankuenfte
				stehtSeit = time.Time{}
				continue
			}
			// AUFHOLEN IST KEIN EINFRIEREN. Der vorhandene Height-Stall-Check
			// wartet aus gutem Grund 25 Minuten: ein echter Aufholvorgang hat
			// am 03.07.2026 rund 20 gebraucht, und ihn abzuschiessen waere
			// schlimmer als das Warten. Diese Schwelle darf nicht sinken.
			//
			// Der Unterschied liegt woanders: wer aufholt, EMPFAENGT dabei
			// Bloecke. Der eingefrorene Knoten am 07.09. empfing nichts, log
			// nichts und bewegte sich nicht -- nur HTTP antwortete noch.
			// Kommen also weiter Bloecke an, ist dies ein langsamer, aber
			// lebender Knoten und geht den Waechter nichts an.
			if ankuenfte != letzteAnkuenfte {
				letzteAnkuenfte = ankuenfte
				stehtSeit = time.Time{}
				continue
			}
			// Die eigene Hoehe steht. Zieht der Partner davon?
			partner, ok := fetchPrimaryHeight(peerURL)
			if !ok || partner-eigene < totmannMindestAbstand {
				stehtSeit = time.Time{}
				continue
			}
			if stehtSeit.IsZero() {
				stehtSeit = time.Now()
				totmannWarnungen.Add(1)
				fmt.Printf("[TOTMANN] ⚠ eigene Hoehe steht bei %d, %s ist bei %d (%d voraus) — Beobachtung laeuft, Ende nach %s\n",
					eigene, peerURL, partner, partner-eigene, totmannStillstand)
				continue
			}
			if time.Since(stehtSeit) < totmannStillstand {
				continue
			}
			if !totmannAusgeloest.CompareAndSwap(false, true) {
				continue
			}
			fmt.Printf("[TOTMANN] ✗ Hoehe steht seit %s bei %d, waehrend %s bei %d ist (%d voraus). Der Knoten folgt der Kette nicht mehr und kann sich nicht selbst loesen.\n",
				time.Since(stehtSeit).Round(time.Second), eigene, peerURL, partner, partner-eigene)
			fmt.Println("[TOTMANN] Abzug ALLER Goroutinen — hier steht die Ursache, die beim Einfrieren am 07.09.2026 fehlte:")
			puffer := make([]byte, 4<<20)
			n := runtime.Stack(puffer, true)
			fmt.Printf("%s\n", puffer[:n])
			fmt.Println("[TOTMANN] Prozess wird beendet, damit Docker ihn neu startet.")
			os.Stdout.Sync()
			time.Sleep(500 * time.Millisecond) // dem Logtreiber Zeit geben
			os.Exit(9)
		}
	})
}

// TotmannStand meldet, ob der Waechter etwas gesehen hat.
func TotmannStand() map[string]interface{} {
	return map[string]interface{}{
		"bedeutung": "Beendet den Prozess, wenn die eigene Hoehe stillsteht waehrend der Partner " +
			"davonzieht -- der Fall, den kein anderer Waechter erkennt, weil dabei weder Bloecke " +
			"ankommen noch Code laeuft, an dem die uebrigen haengen. warnungen>0 heisst: es gab " +
			"Beobachtungsphasen, die sich wieder aufgeloest haben.",
		"warnungen":           totmannWarnungen.Load(),
		"ausgeloest":          totmannAusgeloest.Load(),
		"stillstand_sekunden": int64(totmannStillstand / time.Second),
		"mindest_abstand":     int64(totmannMindestAbstand),
	}
}
