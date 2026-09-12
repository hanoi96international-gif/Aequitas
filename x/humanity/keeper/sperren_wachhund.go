package keeper

import (
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// Wer haelt die DAG-Sperre, wenn die Blockproduktion wartet?
//
// GEMESSEN am 07.09.2026 mit der Phasenuhr (produktion_phasen.go):
//
//	C2, teuerster Blockbau: 19.262 ms -- davon 19.261,6 ms auf die Sperre,
//	                        bei NULL Transaktionen im Block
//	C1, teuerster Blockbau:  5.735 ms -- davon  3.846,8 ms auf die Sperre
//
// Im Mittel ist der Bau harmlos (113 bzw. 147 ms). Es sind genau diese
// Ausreisser, die die Hoehe stillstehen lassen -- und die Vorgabe lautet:
// keine Sekunde Stillstand.
//
// Der Replay scheidet aus (schlimmster Halt 1.428 ms), die Zustandssperre
// ebenfalls (zu 2,9 % belegt). Es haelt also etwas anderes, und 56 Stellen im
// Paket nehmen dag.mu -- sie alle zu instrumentieren waere der falsche Weg.
//
// Stattdessen: wer WARTET, schaut nach. Ueberschreitet das Warten die
// Schwelle, wird einmal ein Goroutine-Abzug gezogen und auf die Zeilen
// reduziert, die dieses Paket betreffen. Darin steht die haltende Goroutine
// mitsamt ihrem Aufrufpfad -- die Antwort, die aus Zaehlern nicht hervorgeht.
//
// Kostet nichts im Normalfall: ein Timer, der fast immer vor dem Ablauf
// wieder abgeraeumt wird. Der Abzug selbst haelt die Welt kurz an (Go stoppt
// dafuer alle Goroutinen), deshalb hoechstens einer alle zwei Minuten.
//
// SCHWELLE 1 s, NICHT 3. Am 12.09.2026 wartete ProduceBlock auf C2 unter Last
// 1.586 ms und 1.703 ms auf die Sperre -- bei einem Takt von 1.000 ms ist das
// ein ausgefallener Tick, und der Wachhund schwieg, weil 3 s nicht erreicht
// waren. Wer sie hielt, blieb damit unbekannt, und der Container-Wechsel beim
// naechsten Deploy nahm auch die Chance auf einen Dump. Eine Sekunde Warten
// bei einer Sekunde Takt IST der Fall, den dieses Instrument aufklaeren soll.
const (
	sperrWachhundSchwelle = 1 * time.Second
	sperrWachhundAbstand  = 2 * time.Minute
)

var (
	sperrWachhundLetzter atomic.Int64 // Unixzeit des letzten Abzugs
	sperrWachhundZuege   atomic.Int64
	sperrWachhundMaxMs   atomic.Int64
)

// sperrWachhundStarten liefert eine Funktion, die beim Erhalt der Sperre zu
// rufen ist. Dauert das Warten laenger als die Schwelle, schreibt der
// Wachhund vorher auf, wer sie haelt.
func sperrWachhundStarten(wer string) func() {
	start := time.Now()
	fertig := make(chan struct{})
	SafeGoroutine("sperrWachhund", func() {
		select {
		case <-fertig:
			return
		case <-time.After(sperrWachhundSchwelle):
		}
		jetzt := time.Now().Unix()
		letzter := sperrWachhundLetzter.Load()
		if jetzt-letzter < int64(sperrWachhundAbstand/time.Second) {
			return // zu kurz nach dem letzten Abzug
		}
		if !sperrWachhundLetzter.CompareAndSwap(letzter, jetzt) {
			return // ein anderer war schneller
		}
		sperrWachhundZuege.Add(1)
		fmt.Printf("[SPERRE] ⏱ %s wartet seit %s auf die DAG-Sperre — wer haelt sie?\n",
			wer, time.Since(start).Round(time.Millisecond))
		for _, z := range interessanteGoroutinen() {
			fmt.Printf("[SPERRE]   %s\n", z)
		}
	})
	return func() {
		close(fertig)
		ms := time.Since(start).Milliseconds()
		for {
			alt := sperrWachhundMaxMs.Load()
			if ms <= alt || sperrWachhundMaxMs.CompareAndSwap(alt, ms) {
				break
			}
		}
	}
}

// interessanteGoroutinen zieht einen Abzug und behaelt die Zeilen, die eine
// Funktion dieses Pakets nennen -- alles andere ist Laufzeit- und
// Bibliotheksrauschen und wuerde die Antwort im Log begraben.
func interessanteGoroutinen() []string {
	puffer := make([]byte, 1<<20)
	n := runtime.Stack(puffer, true)
	var raus []string
	for _, block := range strings.Split(string(puffer[:n]), "\n\n") {
		if !strings.Contains(block, "humanity/keeper") {
			continue
		}
		// NUR MOEGLICHE HALTER. Eine Goroutine, deren Zustand "sync.Mutex.Lock"
		// oder "semacquire" ist, WARTET auf genau dieselbe Sperre -- sie kann
		// sie nicht halten. Der erste Abzug am 07.09.2026 bestand fast nur aus
		// solchen Zeilen und nannte den Halter damit gerade nicht.
		//
		// Wer die Sperre haelt, ist stattdessen laufend, im Syscall oder wartet
		// auf Platte bzw. Netz -- runnable, running, IO wait, syscall. Genau
		// diese bleiben uebrig.
		kopf := block
		if j := strings.IndexByte(kopf, 10); j > 0 {
			kopf = kopf[:j]
		}
		wartend := strings.Contains(kopf, "sync.Mutex.Lock") ||
			strings.Contains(kopf, "semacquire") ||
			strings.Contains(kopf, "sync.RWMutex") ||
			strings.Contains(kopf, "chan receive") ||
			strings.Contains(kopf, "chan send") ||
			strings.Contains(kopf, "select")
		if wartend {
			continue
		}
		zeilen := strings.Split(block, "\n")
		kurz := zeilen[0]
		for _, z := range zeilen[1:] {
			if strings.Contains(z, "humanity/keeper") {
				kurz += " | " + strings.TrimSpace(z)
				if strings.Count(kurz, "|") >= 4 {
					break
				}
			}
		}
		raus = append(raus, kurz)
		if len(raus) >= 6 {
			break
		}
	}
	return raus
}

// SperrWachhundStand meldet, wie oft und wie lange gewartet wurde.
func SperrWachhundStand() map[string]interface{} {
	return map[string]interface{}{
		"bedeutung": "Der Wachhund zieht einen Goroutine-Abzug, wenn die Blockproduktion laenger " +
			"als die Schwelle auf die DAG-Sperre wartet. abzuege>0 heisst: es gab solche Faelle, " +
			"und im Knotenlog steht unter [SPERRE], wer sie gehalten hat. laengstes_warten_ms ist " +
			"das Maximum ueber alle Versuche.",
		"schwelle_ms":         int64(sperrWachhundSchwelle / time.Millisecond),
		"abzuege":             sperrWachhundZuege.Load(),
		"laengstes_warten_ms": sperrWachhundMaxMs.Load(),
	}
}

// SperrWachhundZuruecksetzen gibt es fuer die Tests.
func SperrWachhundZuruecksetzen() {
	sperrWachhundLetzter.Store(0)
	sperrWachhundZuege.Store(0)
	sperrWachhundMaxMs.Store(0)
}
