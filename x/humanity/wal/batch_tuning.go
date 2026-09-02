package wal

import (
	"os"
	"strconv"
	"time"
)

// Die zwei Groessen des Gruppen-Commits zum Vermessen freigeben -- ohne sie zu
// aendern.
//
// # WARUM GERADE JETZT
//
// MaxBatchWait traegt seine eigene Aufforderung dazu: "Re-measure if real
// staging hardware's fsync latency turns out meaningfully different from this
// sandbox's." Genau das ist eingetreten. Die 1 ms wurden 2026-07-23 in einer
// Sandbox gemessen (3ms -> 3100-3700 TPS, 1ms -> 5500-5900 TPS); die echten
// Boxen schaffen laut Messung vom 26.07. nur 133-462 fsyncs/s. Auf einer
// Platte, die zwei Groessenordnungen langsamer synchronisiert, ist ein
// 1-ms-Fenster etwas voellig anderes als in der Sandbox.
//
// # WARUM ES ZAEHLT
//
// Gemessen am 29.08.2026 im Schnellpfad, je Ueberweisung:
//
//	pre_rlock   156,3 ms
//	wal_append   68,7 ms   <- 30 %, und 74,85 % davon ueber 20 ms
//	apply         0,06 ms
//
// wal.Append blockiert bis zum fsync seines Buendels. Ein zu kurzes Fenster
// heisst: viele kleine Buendel, viele fsyncs, und jeder Aufrufer wartet hinter
// der Warteschlange. Ein zu langes heisst: jeder Aufrufer wartet das Fenster
// ab. Wo das Optimum liegt, haengt an der fsync-Latenz der Platte -- und die
// ist hier eine andere als dort, wo die 1 ms herkommen.
//
// # WARUM ALS SCHALTER, NICHT ALS NEUER WERT
//
// Der bestehende Kommentar begruendet die 1 ms mit einer echten Messung. Sie
// zu ueberschreiben, ohne auf dieser Hardware gemessen zu haben, waere genau
// der Fehler, den er selbst benennt. Vorgabe bleibt, Aenderung je Box, Rueckweg
// in Sekunden -- dieselbe Bauart wie AEQUITAS_WAL_FLUSH_MAX_ADDRS, die am
// 29.08. eine plausible Richtung als falsch entlarvt hat.
//
//	AEQUITAS_WAL_MAX_BATCH_SIZE   Datensaetze je fsync (Vorgabe 500)
//	AEQUITAS_WAL_MAX_BATCH_WAIT_US  Sammelfenster in Mikrosekunden (Vorgabe 1000)
//
// Ein unbrauchbarer Wert ergibt die Vorgabe.
const (
	maxBatchSizeEnv = "AEQUITAS_WAL_MAX_BATCH_SIZE"
	maxBatchWaitEnv = "AEQUITAS_WAL_MAX_BATCH_WAIT_US"
)

// batchSize liefert die geltende Buendelgroesse.
//
// Einmal je Buendel gelesen, nicht je Datensatz: das ist ein Bruchteil dessen,
// was der fsync danach kostet, und dafuer laesst sich der Wert an einem
// laufenden Knoten aendern -- der Unterschied zwischen einem Sweep und einem
// Deploy je Messpunkt.
func batchSize() int {
	if n, ok := zahlAusUmgebung(maxBatchSizeEnv); ok && n > 0 {
		return n
	}
	return MaxBatchSize
}

// batchWait liefert das geltende Sammelfenster.
func batchWait() time.Duration {
	if n, ok := zahlAusUmgebung(maxBatchWaitEnv); ok && n > 0 {
		return time.Duration(n) * time.Microsecond
	}
	return MaxBatchWait
}

func zahlAusUmgebung(name string) (int, bool) {
	raw := os.Getenv(name)
	if raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return n, true
}

// BatchTuning zeigt die geltenden Werte, damit sie in /api/health/combined
// sichtbar sind. Ein Schalter, den man nicht sehen kann, wird beim naechsten
// Deploy vergessen.
func BatchTuning() map[string]interface{} {
	groesse := batchSize()
	warten := batchWait()
	return map[string]interface{}{
		"max_batch_size":       groesse,
		"max_batch_wait_us":    warten.Microseconds(),
		"abweicht_von_vorgabe": groesse != MaxBatchSize || warten != MaxBatchWait,
		"bedeutung": "Gruppen-Commit des WAL. wal.Append blockiert bis zum fsync seines " +
			"Buendels; am 29.08.2026 waren das 68,7 ms je Ueberweisung, 30 % der Gesamtzeit. " +
			"Die Vorgabe von 1 ms stammt aus einer Sandbox mit deutlich schnellerem fsync",
	}
}

// GEMESSEN AM 29.08.2026 AUF DER ECHTEN PLATTE -- ZWEI ERGEBNISSE
//
// Der Prueflauf ist TestWALThroughput (opt-in, AEQUITAS_WAL_BENCH=1), gefahren
// auf Contabo2 gegen dessen eigene Platte. Er beruehrt keine Kette: ein
// Testbinary, eine temporaere Datei, kein Konsens, kein Neustart von irgendwas.
//
// 1. DAS WAL IST NICHT DER ENGPASS. Mit der Vorgabe (500 / 1 ms):
//
//	gleichzeitig      1 ->     128 appends/s
//	gleichzeitig     10 ->   1.481
//	gleichzeitig     50 ->   3.263
//	gleichzeitig    200 ->  14.933
//	gleichzeitig  1.000 ->  46.342
//
//    46.342 Anhaenge je Sekunde. Fuer 10.000 TPS braucht es rund ein Fuenftel
//    davon. In der Produktion misst der Schnellpfad aber 68,7 ms je Anhang --
//    das entspraeche etwa 2.000/s. Die Luecke ist keine Platteneigenschaft,
//    sondern fehlende GLEICHZEITIGKEIT: die Ueberweisungen stehen vorher in
//    cs.mu.RLock() Schlange (156,3 ms), erreichen den Gruppen-Commit einzeln
//    und finden dort niemanden zum Buendeln. Der langsame Anhang ist die
//    FOLGE der Serialisierung, nicht ihre Ursache.
//
// 2. DAS FENSTER LAESST SICH HIER NICHT ENTSCHEIDEN. Ein erster Lauf sah nach
//    einem klaren Vorteil fuer 300 us aus (7.257 gegen 3.263 bei 50
//    Gleichzeitigen). Zwei Wiederholungen widerlegten das: dieselbe
//    Einstellung ergab 2.954, 3.263 und 9.607 -- Faktor 3,3 zwischen Laeufen.
//    Der Vorteil war Rauschen.
//
//    Das ist genau der Fehler, vor dem der Kommentar zu MaxBatchWait warnt
//    ("within run-to-run noise"), nur auf anderer Hardware. Wer das Fenster
//    entscheiden will, braucht verschraenkte Wiederholungen (ABABAB statt
//    AABB) und genug davon, um die Streuung zu schlagen -- nicht einen Lauf
//    je Wert.
//
// FOLGE: die Vorgabe bleibt, weil nichts sie widerlegt hat. Der Hebel liegt
// woanders -- siehe transfer_wal.go, wo die Lesesperre 68 % der Zeit frisst.
