package keeper

import (
	"sync"
	"sync/atomic"
	"time"
)

// Bloecke erreichen jeden Partner IN DER REIHENFOLGE, in der sie gebaut
// wurden.
//
// # WAS PASSIERTE
//
// Gemessen am 29.09.2026 unter Last: HTTPBroadcastBlock startete fuer jeden
// Block und jeden Partner eine eigene Goroutine. Ein Takt baut bis zu fuenf
// Bloecke, und die kamen bei C2 in beliebiger Reihenfolge an -- 240-mal
// "queued as orphan -- missing parent" in einem Lauf. C2 wartet dann auf den
// Elternblock; kam dessen Push nicht innerhalb der Frist an (3 s, bei
// Bloecken mit 7.000 signierten Ueberweisungen und einem Partner, der gerade
// nachspielt), holte ihn erst der naechste Sync. Solange stand C2 genau zwoelf
// Hoehen zurueck, die Peer-Lag-Bremse drueckte C1s Bloecke auf 1.500, und die
// Annahme lehnte ab.
//
// Jetzt hat jeder Partner eine Warteschlange und EINE Goroutine, die sie
// abarbeitet: die Pushes STARTEN in Produktionsreihenfolge, hoechstens
// pushFenster zugleich. Ist die Schlange voll (Partner haengt), faellt der
// Push auf den alten Weg zurueck -- lieber eine Waise als ein Takt, der auf
// einen toten Partner wartet.
//
// # WARUM EIN FENSTER UND NICHT STRENG EINER NACH DEM ANDEREN
//
// Streng nacheinander (29.09.2026, erste Fassung) wartete jeder Push auf die
// Antwort des vorigen, und der Partner antwortet erst nach dem Nachspielen.
// Die Signaturpruefung des naechsten Blocks konnte damit nicht mehr neben dem
// Nachspielen des vorigen laufen: auf C2 traf sie nur 180 von 901 Bloecken,
// ein voller Block brauchte 700-1.200 ms, C2 fiel bis 33 Hoehen zurueck.
// Mit einem Fenster laufen bis zu pushFenster Bloecke gleichzeitig an; ein
// Kind, das vor seinem Elternteil fertig empfangen ist, wird beim Partner
// kurz zur Waise und sofort nach dem Elternblock wieder vorgelegt
// (popOrphans in AddPeerBlock) -- mit bereits gepruefter Signatur.
const (
	pushSchlangeTiefe = 64
	pushFenster       = 4
)

var (
	pushSchlangenMu sync.Mutex
	pushSchlangen   = map[string]chan func(){}

	pushGeordnetZaehler atomic.Int64
	pushUeberlauf       atomic.Int64
	pushZeitUeber       atomic.Int64
)

// pushGeordnet reiht arbeit in die Schlange des Partners ein. Kehrt sofort
// zurueck.
func pushGeordnet(peerURL string, arbeit func()) {
	pushSchlangenMu.Lock()
	ch, da := pushSchlangen[peerURL]
	if !da {
		ch = make(chan func(), pushSchlangeTiefe)
		pushSchlangen[peerURL] = ch
		SafeGoroutine("push-"+peerURL, func() {
			plaetze := make(chan struct{}, pushFenster)
			for a := range ch {
				plaetze <- struct{}{} // in Reihenfolge starten, hoechstens pushFenster zugleich
				a := a
				SafeGoroutine("push-arbeit", func() {
					defer func() { <-plaetze }()
					a()
				})
			}
		})
	}
	pushSchlangenMu.Unlock()
	select {
	case ch <- arbeit:
		pushGeordnetZaehler.Add(1)
	default:
		pushUeberlauf.Add(1)
		SafeGoroutine("push-ueberlauf", arbeit)
	}
}

// pushFrist: 3 s wie bisher, plus eine Sekunde je angefangenem MB Nutzlast,
// hoechstens 15 s. Ein voller Block mit 7.000 signierten Ueberweisungen ist
// auch gepackt mehrere MB, und der Partner antwortet erst nach dem
// Nachspielen.
func pushFrist(bytes int) time.Duration {
	f := 3*time.Second + time.Duration((bytes+(1<<20)-1)>>20)*time.Second
	if f > 15*time.Second {
		f = 15 * time.Second
	}
	return f
}

// PushReihenfolgeStand fuer /api/health/combined.
func PushReihenfolgeStand() map[string]interface{} {
	pushSchlangenMu.Lock()
	tiefe := map[string]int{}
	for u, ch := range pushSchlangen {
		tiefe[u] = len(ch)
	}
	pushSchlangenMu.Unlock()
	return map[string]interface{}{
		"geordnet":         pushGeordnetZaehler.Load(),
		"ueberlauf":        pushUeberlauf.Load(),
		"zeitueberschritt": pushZeitUeber.Load(),
		"schlangen":        tiefe,
		"bedeutung": "Block-Pushes je Partner der Reihe nach. ueberlauf > 0: eine Schlange war voll, der Push lief " +
			"ungeordnet. zeitueberschritt: Pushes, die nicht innerhalb von pushFrist beantwortet wurden (einmal wiederholt, " +
			"danach holt der Sync den Block).",
	}
}
