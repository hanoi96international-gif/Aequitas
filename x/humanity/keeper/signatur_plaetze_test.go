package keeper

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Missbrauch: viele gleichzeitige Anfragen bekommen nie mehr Plaetze als
// vorgesehen -- auch nicht, wenn jede lange rechnet.
func TestMitSignaturPlatz_NieMehrAlsDieGrenze(t *testing.T) {
	grenze := cap(rpcSignaturPlaetze)
	var gleichzeitig, hoechst atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 20*grenze+5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mitSignaturPlatz(func() {
				n := gleichzeitig.Add(1)
				for {
					alt := hoechst.Load()
					if n <= alt || hoechst.CompareAndSwap(alt, n) {
						break
					}
				}
				time.Sleep(2 * time.Millisecond)
				gleichzeitig.Add(-1)
			})
		}()
	}
	wg.Wait()
	if h := hoechst.Load(); h > int64(grenze) || h < 1 {
		t.Fatalf("hoechstens %d gleichzeitig erwartet, gemessen %d", grenze, h)
	}
	if len(rpcSignaturPlaetze) != 0 {
		t.Fatalf("%d Plaetze nicht zurueckgegeben", len(rpcSignaturPlaetze))
	}
}

// Ein Panic in der Wiederherstellung gibt den Platz trotzdem frei.
func TestMitSignaturPlatz_PanicGibtFrei(t *testing.T) {
	func() {
		defer func() { _ = recover() }()
		mitSignaturPlatz(func() { panic("kaputte Signatur") })
	}()
	if len(rpcSignaturPlaetze) != 0 {
		t.Fatal("Platz nach Panic belegt")
	}
}

func TestRPCSignaturParallelWert(t *testing.T) {
	t.Setenv(rpcSignaturParallelEnv, "100000")
	if n := rpcSignaturParallelWert(); n < 1 || n > runtime.NumCPU() {
		t.Fatalf("nicht auf die Kernzahl gedeckelt: %d", n)
	}
	t.Setenv(rpcSignaturParallelEnv, "1")
	if n := rpcSignaturParallelWert(); n != 1 {
		t.Fatalf("%d statt 1", n)
	}
	t.Setenv(rpcSignaturParallelEnv, "-3")
	if n := rpcSignaturParallelWert(); n < 1 {
		t.Fatalf("%d", n)
	}
}
