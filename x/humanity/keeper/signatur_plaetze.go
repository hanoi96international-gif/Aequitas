package keeper

import (
	"runtime"
	"sync/atomic"
	"time"
)

// Wie viele Signaturen die RPC-Annahme GLEICHZEITIG wiederherstellen darf.
//
// GEMESSEN AM 02.10.2026 (Pruefstand C1, Lauf 14): die Box lief zu 91 % voll
// (vmstat: 67 % user, 23 % system, im Mittel 14,6 lauffaehige Threads auf 8
// Kernen), die Scheduler-Latenz des Knotens lag bei p99 201 ms und p99.9
// 470 ms. Genau das waren die Ausreisser beim Blockbau: einzelne Schritte, die
// sonst 6-20 ms brauchen (eine Ein-Zeilen-Abfrage, JSON + Komprimierung),
// standen bis zu 0,8 s -- nicht wegen einer Sperre (cs.mu: hoechstens 7 ms)
// und nicht wegen des Garbage Collectors (Pause hoechstens 6 ms), sondern weil
// der Blockbauer keinen Kern bekam.
//
// Die Ursache: secp256k1 laeuft ueber cgo. Ein Goroutine in einem cgo-Aufruf
// gibt seinen P ab -- GOMAXPROCS begrenzt diese Aufrufe NICHT. Jedes
// RPC-Buendel startete NumCPU Wiederherstellungs-Arbeiter, bei hunderten
// Buendeln gleichzeitig liefen dutzende Betriebssystem-Threads in ecrecover
// (Goroutine-Schnappschuss: 94) neben den 8 Ps der Go-Laufzeit. Oeffentliche
// Eingaben bestimmten damit, wie viele Kerne der Knoten fuer sie hergibt.
//
// Jetzt: hoechstens rpcSignaturParallel gleichzeitig. Vorgabe NumCPU/2 (auf
// C1: 4) -- gemessen ~12.000 Wiederherstellungen je Kern und Sekunde, also
// ~48.000/s, ein Vielfaches dessen, was die Kette verblocken kann. Wer
// wartet, wartet als geparkte Goroutine (kein Thread, keine CPU); die Zahl
// der Wartenden begrenzt die Inflight-Grenze.
//
// Nur die RPC-Annahme. Das Nachspielen fremder Bloecke (signatur_vorab.go)
// hat seine eigene, feste Arbeiterzahl.

const rpcSignaturParallelEnv = "AEQUITAS_RPC_SIGNATUR_PARALLEL"

var rpcSignaturPlaetze = make(chan struct{}, rpcSignaturParallelWert())

var (
	rpcSignaturGewartet   atomic.Int64 // Wiederherstellungen, die auf einen Platz warten mussten
	rpcSignaturWartenNs   atomic.Int64
	rpcSignaturWartenMax  atomic.Int64
	rpcSignaturGesamtzahl atomic.Int64
)

func rpcSignaturParallelWert() int {
	kerne := runtime.NumCPU()
	n := envPositiveInt(rpcSignaturParallelEnv)
	if n <= 0 {
		n = kerne / 2
	}
	if n > kerne {
		n = kerne
	}
	if n < 1 {
		n = 1
	}
	return n
}

// mitSignaturPlatz fuehrt f mit einem der begrenzten Plaetze aus.
func mitSignaturPlatz(f func()) {
	rpcSignaturGesamtzahl.Add(1)
	select {
	case rpcSignaturPlaetze <- struct{}{}:
	default:
		start := time.Now()
		rpcSignaturPlaetze <- struct{}{}
		d := int64(time.Since(start))
		rpcSignaturGewartet.Add(1)
		rpcSignaturWartenNs.Add(d)
		for {
			alt := rpcSignaturWartenMax.Load()
			if d <= alt || rpcSignaturWartenMax.CompareAndSwap(alt, d) {
				break
			}
		}
	}
	defer func() { <-rpcSignaturPlaetze }()
	f()
}

// RPCSignaturStand fuer /api/health/combined.
func RPCSignaturStand() map[string]interface{} {
	n := rpcSignaturGewartet.Load()
	mittel := 0.0
	if n > 0 {
		mittel = float64(rpcSignaturWartenNs.Load()) / float64(n) / 1e6
	}
	return map[string]interface{}{
		"plaetze":          cap(rpcSignaturPlaetze),
		"belegt":           len(rpcSignaturPlaetze),
		"gesamt":           rpcSignaturGesamtzahl.Load(),
		"gewartet":         n,
		"warten_mittel_ms": mittel,
		"warten_max_ms":    float64(rpcSignaturWartenMax.Load()) / 1e6,
	}
}
