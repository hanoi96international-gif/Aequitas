package keeper

import (
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Belegter Shard -> warten statt Rueckfall in den seriellen Pfad
// (shardWartenStattRueckfall, shard_wiederholung.go). Der Rueckfall nimmt seit
// dem 26.09.2026 immer cs.mu.Lock() und sperrt damit jede andere Ueberweisung
// aus; der Halter des Shards ist meist flushWALBatch.
func TestTransferConcurrentWAL_ShardWarten_RettetStattRueckfall(t *testing.T) {
	dir := t.TempDir()
	truncateDistTestTables(t)
	cs := newWALTestState(t, filepath.Join(dir, "test.wal"))
	from := distTestAddr(881)
	to := distTestAddr(882)
	seedConcurrentTestAccount(t, cs, from, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, cs, to, 0, time.Now().Unix())

	halten := func(d time.Duration) {
		unlock := cs.accounts.LockAddrs(from) // wie flushWALBatch
		go func() {
			time.Sleep(d)
			unlock()
		}()
	}

	// Vorgabe: warten.
	t.Setenv("AEQUITAS_SHARD_WARTEN", "")
	vorher := fbShardGewartet.Load()
	halten(150 * time.Millisecond)
	start := time.Now()
	_, _, applied, err := cs.transferConcurrentWAL(from, to, 10, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 10, TxHash: "0xshardwarten1"})
	if !applied || err != nil {
		t.Fatalf("belegter Shard: applied=%v err=%v, erwartet angewendet nach dem Warten", applied, err)
	}
	if d := time.Since(start); d < 100*time.Millisecond {
		t.Fatalf("nach %v angewendet -- hat nicht auf den gehaltenen Shard gewartet", d)
	}
	if fbShardGewartet.Load() != vorher+1 {
		t.Fatal("shard_gewartet_gerettet nicht gezaehlt")
	}

	// Abgeschaltet: der alte Rueckfall (applied=false, kein Fehler).
	t.Setenv("AEQUITAS_SHARD_WARTEN", "0")
	halten(150 * time.Millisecond)
	_, _, applied, err = cs.transferConcurrentWAL(from, to, 10, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 10, TxHash: "0xshardwarten2"})
	if applied || err != nil {
		t.Fatalf("abgeschaltet: applied=%v err=%v, erwartet Rueckfall ohne Fehler", applied, err)
	}
	time.Sleep(200 * time.Millisecond) // Shard wieder frei, bevor Cleanup flusht

	cs.mu.RLock()
	fromAcc, _ := cs.accounts.Get(from)
	toAcc, _ := cs.accounts.Get(to)
	cs.mu.RUnlock()
	if want := nachGebuehr(100, 10); math.Abs(fromAcc.Balance.Float()-want) > 1e-9 {
		t.Errorf("Absender %v, erwartet %v (genau eine Ueberweisung)", fromAcc.Balance.Float(), want)
	}
	if toAcc.Balance.Float() != 10 {
		t.Errorf("Empfaenger %v, erwartet 10", toAcc.Balance.Float())
	}
}

// Unter Last: viele Absender ueber TransferAtomic (Schnellpfad, Warten,
// Rueckfall), dazwischen fremde Shard-Halter (wie der Flush) und ein
// Schreiber auf cs.mu (wie Block-Replay). Kein Deadlock, und die Geldmenge
// bleibt exakt erhalten.
func TestTransferConcurrentWAL_ShardWarten_RingUnterSchreibernOhneDeadlock(t *testing.T) {
	dir := t.TempDir()
	truncateDistTestTables(t)
	cs := newWALTestState(t, filepath.Join(dir, "test.wal"))
	t.Setenv("AEQUITAS_SHARD_WARTEN", "")
	const n = 40
	const runden = 15
	const betrag = 5
	addrs := make([]string, n)
	for i := range addrs {
		addrs[i] = distTestAddr(900 + i)
		seedConcurrentTestAccount(t, cs, addrs[i], 1000, time.Now().Unix())
	}

	var stop atomic.Bool
	var stoerer sync.WaitGroup
	// Wie flushWALBatch: RLock, dann mehrere Shards sortiert, eine Weile halten.
	stoerer.Add(1)
	go func() {
		defer stoerer.Done()
		for i := 0; !stop.Load(); i++ {
			cs.mu.RLock()
			unlock := cs.accounts.LockAddrs(addrs[i%n], addrs[(i*7+3)%n], addrs[(i*13+5)%n])
			time.Sleep(3 * time.Millisecond)
			unlock()
			cs.mu.RUnlock()
		}
	}()
	// Wie Block-Replay: cs.mu exklusiv, kurz.
	stoerer.Add(1)
	go func() {
		defer stoerer.Done()
		for !stop.Load() {
			cs.mu.Lock()
			time.Sleep(1 * time.Millisecond)
			cs.mu.Unlock()
			time.Sleep(2 * time.Millisecond)
		}
	}()

	var wg sync.WaitGroup
	var fehler atomic.Int64
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			from, to := addrs[i], addrs[(i+1)%n]
			for r := 0; r < runden; r++ {
				tmpl := Transaction{Type: "transfer", Wallet: from, To: to, Amount: betrag, TxHash: fmt.Sprintf("0xshardring%02d%02d", i, r)}
				if _, _, err := cs.TransferAtomic(from, to, betrag, tmpl); err != nil {
					fehler.Add(1)
					t.Errorf("Absender %d Runde %d: %v", i, r, err)
				}
			}
		}(i)
	}
	fertig := make(chan struct{})
	go func() { wg.Wait(); close(fertig) }()
	select {
	case <-fertig:
	case <-time.After(90 * time.Second):
		stop.Store(true)
		t.Fatal("DEADLOCK: Ueberweisungen nach 90 s nicht fertig")
	}
	stop.Store(true)
	stoerer.Wait()

	unterwegs := gebuehrenUnterwegs(t, cs)
	cs.mu.RLock()
	var gesamt float64
	for _, a := range addrs {
		acc, _ := cs.accounts.Get(a)
		gesamt += acc.Balance.Float()
	}
	cs.mu.RUnlock()
	if math.Abs(gesamt+unterwegs-float64(n)*1000) > 1e-6 {
		t.Fatalf("Geldmenge: Konten %v + Gebuehren unterwegs %v = %v, erwartet %v",
			gesamt, unterwegs, gesamt+unterwegs, float64(n)*1000)
	}
	t.Logf("gerettet durch Warten: %d, Rueckfaelle gesamt: %d", fbShardGewartet.Load(), transferFastPathFallback.Load())
}
