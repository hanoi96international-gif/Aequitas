package keeper

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

// Was die hoehere Shard-Zahl wirklich kostet -- gemessen, nicht geschaetzt.
//
// Die Begruendung in sharded_accounts.go nennt zwei Zahlen: rund 15 MB
// Speicher und ein 16x teureres Range. Beide werden hier nachgerechnet, damit
// niemand sie glauben muss. Opt-in, weil er absichtlich viel allokiert.
func TestShardZahl_WasSieKostet(t *testing.T) {
	if testing.Short() {
		t.Skip("allokiert absichtlich viel")
	}
	alt := numAccountShards
	defer func() { numAccountShards = alt }()

	for _, n := range []int{16384, 262144} {
		numAccountShards = n

		runtime.GC()
		var vor runtime.MemStats
		runtime.ReadMemStats(&vor)

		start := time.Now()
		sa := newShardedAccounts()
		bauzeit := time.Since(start)

		runtime.GC()
		var nach runtime.MemStats
		runtime.ReadMemStats(&nach)
		speicher := int64(nach.HeapAlloc) - int64(vor.HeapAlloc)

		// Ein paar Konten hinein, damit Range etwas zu tun hat.
		for i := 0; i < 800; i++ {
			sa.Set(fmt.Sprintf("0x%040x", i), &AccountState{Address: fmt.Sprintf("0x%040x", i)})
		}
		start = time.Now()
		anzahl := 0
		sa.Range(func(string, *AccountState) bool { anzahl++; return true })
		rangezeit := time.Since(start)

		t.Logf("Shards %7d: Bau %8s, Speicher %6.1f MB, Range %8s (%d Konten gesehen)",
			n, bauzeit.Round(time.Millisecond), float64(speicher)/(1<<20),
			rangezeit.Round(time.Microsecond), anzahl)

		if anzahl != 800 {
			t.Fatalf("Range sah %d von 800 Konten -- die Partitionierung verliert Eintraege", anzahl)
		}
		// Ein Range je Block darf die Blockzeit von 1 s nicht ernsthaft
		// belasten. 100 ms waeren 10 % und damit die Schmerzgrenze.
		if rangezeit > 100*time.Millisecond {
			t.Errorf("Range kostet %s bei %d Shards -- zu teuer fuer einen Aufruf je Block", rangezeit, n)
		}
	}
}
