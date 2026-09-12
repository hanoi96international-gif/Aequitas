package wal

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Ohne Sammelfenster muss der Schreiber trotzdem buendeln -- durch den Sync
// selbst: was waehrend eines fdatasync ankommt, bildet den naechsten Batch.
// Der Test prueft genau das: viele nebenlaeufige Appends ergeben deutlich
// weniger Syncs als Appends, und jeder Append kommt an.
func TestLeeren_BuendeltOhneFensterUndVerliertNichts(t *testing.T) {
	t.Setenv(maxBatchWaitEnv, "0")
	if batchWait() != 0 {
		t.Fatalf("batchWait() = %v bei ausdruecklicher 0, erwartet 0", batchWait())
	}
	w, err := Open(filepath.Join(t.TempDir(), "wal.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	vorher := writerStats.batches.Load()
	var wg sync.WaitGroup
	const goroutines, jeweils = 64, 50
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < jeweils; i++ {
				if _, err := w.Append([]byte("x")); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	syncs := writerStats.batches.Load() - vorher
	if syncs >= goroutines*jeweils/4 {
		t.Errorf("%d Syncs fuer %d Appends -- ohne Fenster wird nicht mehr gebuendelt; "+
			"der Sync selbst muss die Buendelung tragen", syncs, goroutines*jeweils)
	}
	t.Logf("%d Appends in %d Syncs (%.1f je Sync)", goroutines*jeweils, syncs, float64(goroutines*jeweils)/float64(syncs))
}

// Ein einzelner Append darf ohne Fenster nicht auf ein Fenster warten: er
// wird sofort geschrieben und gesynct.
func TestLeeren_EinzelnerAppendWartetNichtAufEinFenster(t *testing.T) {
	t.Setenv(maxBatchWaitEnv, "0")
	w, err := Open(filepath.Join(t.TempDir(), "wal.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	// Aufwaermen (erste Vorbelegung).
	if _, err := w.Append([]byte("warm")); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := w.Append([]byte("allein")); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Errorf("ein einzelner Append brauchte %s -- das ist mehr als ein fdatasync, da wartet etwas", d)
	}
}
