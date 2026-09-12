package wal

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Die Seq wird jetzt bei der Aufnahme vergeben, nicht beim Schreiben. Zwei
// Dinge muessen deshalb gelten, sonst ist der Geldpfad falsch geordnet:
// die Seqs erscheinen in der Datei in Vergabereihenfolge und lueckenlos,
// und wenn done meldet, ist DurableSeq mindestens diese Seq.
func TestAppendAsync_SeqOrdnungUndHaltbarkeit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.log")
	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines, jeweils = 32, 40
	var wg sync.WaitGroup
	var mu sync.Mutex
	verletzt := 0
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < jeweils; i++ {
				seq, done, err := w.AppendAsync([]byte("x"))
				if err != nil {
					t.Error(err)
					return
				}
				if err := <-done; err != nil {
					t.Error(err)
					return
				}
				// DIE ZUSAGE: nach done ist die Seq haltbar.
				if w.DurableSeq() < seq {
					mu.Lock()
					verletzt++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	if verletzt > 0 {
		t.Errorf("%d mal meldete done, bevor DurableSeq die Seq erreicht hatte -- ein Flush "+
			"koennte einen Saldo persistieren, dessen WAL-Eintrag noch nicht auf der Platte ist", verletzt)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	var letzte uint64
	n, _, err := ReplayFile(path, func(e Entry) error {
		if e.Seq != letzte+1 {
			t.Errorf("Seq %d folgt auf %d -- Luecke oder falsche Reihenfolge", e.Seq, letzte)
		}
		letzte = e.Seq
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != goroutines*jeweils {
		t.Errorf("%d Datensaetze, erwartet %d", n, goroutines*jeweils)
	}
}

func TestWaitDurable_KommtZurueckWennSynchronisiert(t *testing.T) {
	w, err := Open(filepath.Join(t.TempDir(), "wal.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	seq, done, err := w.AppendAsync([]byte("y"))
	if err != nil {
		t.Fatal(err)
	}
	if !w.WaitDurable(seq, 5*time.Second) {
		t.Fatal("WaitDurable lief in den Timeout, obwohl der Schreiber gesund ist")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	// Eine Seq, die nie vergeben wurde, darf nicht haltbar gemeldet werden.
	if w.WaitDurable(seq+1000, 50*time.Millisecond) {
		t.Error("WaitDurable meldete eine nie vergebene Seq als haltbar")
	}
}

// Nach einer Kompaktierung muss die Hochwassermarke die letzte VERGEBENE Seq
// tragen -- auch eine, die noch im Kanal lag. Sonst vergibt ein Neustart sie
// ein zweites Mal.
func TestAppendAsync_HochwassermarkeIstDieVergebeneSeq(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.log")
	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		if _, err := w.Append([]byte("z")); err != nil {
			t.Fatal(err)
		}
	}
	kopf := w.HeadSeq()
	if err := w.TruncateBefore(50); err != nil {
		t.Fatal(err)
	}
	hwm, err := readSeqHighWaterMark(path)
	if err != nil {
		t.Fatal(err)
	}
	if hwm != kopf {
		t.Errorf("Hochwassermarke %d, erwartet %d (HeadSeq vor der Kompaktierung)", hwm, kopf)
	}
	w.Close()
	w2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()
	seq, err := w2.Append([]byte("nach dem Neustart"))
	if err != nil {
		t.Fatal(err)
	}
	if seq <= kopf {
		t.Errorf("nach dem Neustart wurde Seq %d vergeben, obwohl %d schon vergeben war", seq, kopf)
	}
}
