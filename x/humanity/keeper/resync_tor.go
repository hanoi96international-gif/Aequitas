package keeper

import (
	"fmt"
	"time"
)

// DAS RESYNC-TOR: resyncInProgress mit Haltern und Leerlauf.
//
// resyncInProgress war ein blosses atomic.Bool, gesetzt fuer die ganze Dauer
// von PerformResync -- einschliesslich des Snapshot-Abrufs. Seit der Abruf vor
// dem Tor laeuft (PerformResync, autoheal.go), ist das Tor nur noch fuer den
// Austausch zu, Millisekunden statt Minuten. Damit reichen zwei Dinge nicht
// mehr, die vorher die lange Abrufzeit nebenbei erledigte:
//
//   - Leerlauf: ein ProduceBlock/AddPeerBlock, der die Torpruefung kurz VOR
//     dem Schliessen passiert hat, liefe sonst parallel zum Austausch. Jetzt
//     zaehlt resyncTorDrin jeden Durchgang hinter der Pruefung, und der
//     Resync wartet, bis keiner mehr drin ist. Erst zaehlen, dann pruefen
//     (resyncTorBetreten) -- zusammen mit "erst schliessen, dann zaehlen"
//     beim Resync heisst das: entweder sieht der Durchgang das Tor zu, oder
//     der Resync sieht ihn drin. Beide atomar, sequentiell konsistent.
//   - Mehrere Halter: leitungUeberholt haelt das Tor ueber das Verwerfen und
//     den ganzen eigenen Resync. Ein blosses Store(false) am Ende von
//     PerformResync oeffnete es mitten in seiner Arbeit. Jetzt zaehlt
//     resyncTorHalter; offen ist es erst, wenn der letzte geht.
//
// Begrenzt: der Zaehler ist eine Zahl, keine Liste; das Warten endet nach
// resyncTorLeerMax mit einem Fehler, und der Resync bricht dann ohne jede
// Aenderung ab (fail-closed) -- auch in dem Fall, dass er selbst aus einem
// Durchgang heraus gerufen wurde und sonst auf sich selbst wartete.

// resyncTorLeerMax: so lange wartet ein Resync hoechstens darauf, dass laufende
// ProduceBlock/AddPeerBlock-Durchgaenge fertig werden. Ein Durchgang haelt
// dag.mu/replayMu fuer einen Block -- gemessen Sekunden, nicht Minuten.
// Variable nur, damit ein Test die Frist verkuerzen kann.
var resyncTorLeerMax = 60 * time.Second

// resyncTorSchliessen schliesst das Tor und gibt die Funktion zurueck, die
// diesen Halt wieder aufgibt (genau einmal wirksam).
func (dag *BlockDAG) resyncTorSchliessen() (oeffnen func()) {
	dag.resyncTorMu.Lock()
	dag.resyncTorHalter++
	dag.resyncInProgress.Store(true)
	dag.resyncTorMu.Unlock()
	erledigt := false
	return func() {
		dag.resyncTorMu.Lock()
		defer dag.resyncTorMu.Unlock()
		if erledigt {
			return
		}
		erledigt = true
		dag.resyncTorHalter--
		if dag.resyncTorHalter <= 0 {
			dag.resyncTorHalter = 0
			dag.resyncInProgress.Store(false)
		}
	}
}

// resyncTorBetreten: fuer ProduceBlock und AddPeerBlock an der Stelle der
// bisherigen Pruefung von resyncInProgress. offen=false heisst: Tor zu, nichts
// tun. Bei offen=true muss der Aufrufer verlassen() aufrufen, sobald er fertig
// ist (defer).
func (dag *BlockDAG) resyncTorBetreten() (offen bool, verlassen func()) {
	dag.resyncTorDrin.Add(1)
	if h := resyncTorZwischenHaken; h != nil {
		h()
	}
	if dag.resyncInProgress.Load() {
		dag.resyncTorDrin.Add(-1)
		return false, nil
	}
	return true, func() { dag.resyncTorDrin.Add(-1) }
}

// resyncTorZwischenHaken: nur fuer Tests -- ein Resync, der genau zwischen
// Zaehlen und Pruefen eines Durchgangs schliesst (resync_vor_tor_test.go).
// Im Betrieb nil.
var resyncTorZwischenHaken func()

// warteBisResyncTorLeer: nach resyncTorSchliessen -- wartet, bis kein
// Durchgang mehr hinter der Pruefung laeuft, hoechstens max.
func (dag *BlockDAG) warteBisResyncTorLeer(max time.Duration) error {
	ende := time.Now().Add(max)
	for dag.resyncTorDrin.Load() > 0 {
		if time.Now().After(ende) {
			return fmt.Errorf("still %d block production/acceptance pass(es) running after %s — resync aborted, nothing changed",
				dag.resyncTorDrin.Load(), max)
		}
		time.Sleep(2 * time.Millisecond)
	}
	return nil
}
