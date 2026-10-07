package keeper

import (
	"testing"
	"time"
)

// Missbrauchstest fuer stopWALFlushWorkerForTest: Der Arbeiter darf nach dem
// Schliessen des Stop-Kanals noch mitten im Takt stecken und walFlushWG.Add
// rufen, waehrend der Stopp schon in walFlushWG.Wait steht. -race meldete
// das im Paketlauf (Sicherheitspruefung zu #303). Ohne Datenbank: flushWALQueue
// kehrt bei cs.db == nil sofort zurueck, der Arbeiter belegt aber trotzdem
// in jedem Takt einen Platz ueber walFlushStarten.
func TestStopWALFlushWorker_KeinAddNachWait(t *testing.T) {
	alt := walFlushInterval
	walFlushInterval = 50 * time.Microsecond
	t.Cleanup(func() { walFlushInterval = alt })

	for i := 0; i < 300; i++ {
		cs := &ChainState{}
		cs.ensureWALFlushWorkerStarted()
		time.Sleep(time.Duration(i%7) * 50 * time.Microsecond)
		cs.stopWALFlushWorkerForTest()

		// Nach dem Stopp startet nichts mehr: der Arbeiter ist beendet, kein
		// Platz ist belegt.
		select {
		case <-cs.walFlushWorkerDone:
		default:
			t.Fatalf("Lauf %d: Arbeiter laeuft nach dem Stopp noch", i)
		}
		if n := len(cs.walFlushSem); n != 0 {
			t.Fatalf("Lauf %d: %d Flush-Plaetze nach dem Stopp belegt", i, n)
		}
		cs.stopWALFlushWorkerForTest() // zweimal ist erlaubt
	}
}

// Ein nie gestarteter Arbeiter: der Stopp kehrt sofort zurueck.
func TestStopWALFlushWorker_NieGestartet(t *testing.T) {
	cs := &ChainState{}
	fertig := make(chan struct{})
	go func() { cs.stopWALFlushWorkerForTest(); close(fertig) }()
	select {
	case <-fertig:
	case <-time.After(5 * time.Second):
		t.Fatal("Stopp eines nie gestarteten Arbeiters blockiert")
	}
}
