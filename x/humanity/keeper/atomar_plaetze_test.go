package keeper

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestAtomarPlatzZahl(t *testing.T) {
	for _, f := range []struct{ max, want int }{{0, 0}, {1, 1}, {4, 1}, {5, 1}, {20, 16}, {100, 96}} {
		if got := atomarPlatzZahl(f.max); got != f.want {
			t.Fatalf("atomarPlatzZahl(%d) = %d, erwartet %d", f.max, got, f.want)
		}
	}
}

// Missbrauch (Beta-Pruefung 15.10.2026): mehr gleichzeitige atomare
// Auftraege als der Pool Verbindungen hat, dazu jemand, der cs.mu haelt und
// eine Verbindung braucht (wie Nachspielen und WAL-Flush). Vorher stand der
// Knoten: alle Verbindungen warteten auf cs.mu.
func TestAtomar_KeineVerklemmungMitPool_RealDB(t *testing.T) {
	cs := testKnoten(t, "unused-atomar-plaetze.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	maxOpen := cs.db.Stats().MaxOpenConnections
	if maxOpen <= 0 || maxOpen > 64 {
		t.Fatalf("unerwartete Poolgroesse %d", maxOpen)
	}
	// cs.mu halten, bis alle Auftraege laufen -- dann wie das Nachspielen
	// unter cs.mu eine Verbindung holen.
	cs.mu.Lock()
	var wg sync.WaitGroup
	for i := 0; i < 3*maxOpen; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cs.runAtomicWithOutbox(nil, false, func(ctx context.Context) (Transaction, error) {
				return Transaction{}, nil
			})
		}()
	}
	time.Sleep(500 * time.Millisecond) // alle stehen jetzt an
	fertig := make(chan error, 1)
	go func() {
		ctx, abbrechen := context.WithTimeout(context.Background(), 5*time.Second)
		defer abbrechen()
		tx, err := cs.db.BeginTx(ctx, nil)
		if err == nil {
			err = tx.Rollback()
		}
		fertig <- err
	}()
	var err error
	select {
	case err = <-fertig:
	case <-time.After(10 * time.Second):
		err = context.DeadlineExceeded
	}
	cs.mu.Unlock()
	if err != nil {
		t.Fatalf("unter cs.mu keine Verbindung (Verklemmung): %v", err)
	}
	ende := make(chan struct{})
	go func() { wg.Wait(); close(ende) }()
	select {
	case <-ende:
	case <-time.After(60 * time.Second):
		t.Fatal("atomare Auftraege laufen nach Freigabe von cs.mu nicht ab")
	}
}
