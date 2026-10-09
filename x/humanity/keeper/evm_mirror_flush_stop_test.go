package keeper

import (
	"testing"
	"time"
)

// stopEVMMirrorFlushWorkerForTest haelt einen laufenden Arbeiter an und wartet
// auf ihn; vor dem Start aufgerufen, startet danach keiner mehr.
func TestStopEVMMirrorFlushWorker(t *testing.T) {
	cs := newTestState()
	cs.ensureEVMMirrorFlushWorkerStarted()
	fertig := cs.evmMirrorWorkerDone
	if fertig == nil {
		t.Fatal("Arbeiter nicht gestartet")
	}
	cs.stopEVMMirrorFlushWorkerForTest()
	select {
	case <-fertig:
	case <-time.After(time.Second):
		t.Fatal("Arbeiter laeuft nach dem Stopp weiter")
	}
	cs.stopEVMMirrorFlushWorkerForTest() // zweimal: kein Absturz

	nie := newTestState()
	nie.stopEVMMirrorFlushWorkerForTest()
	nie.ensureEVMMirrorFlushWorkerStarted()
	if nie.evmMirrorWorkerDone != nil {
		t.Fatal("nach dem Stopp doch gestartet")
	}
}
