package keeper

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// Das Belegungs-Bitfeld (sharded_accounts.go, "NUR BELEGTE SHARDS"): Range
// und Len sehen genau die Eintraege, auch nach Loeschen und Neu-Eintragen,
// und unter gleichzeitigen Aenderungen jeden Eintrag, der waehrend des
// ganzen Durchlaufs besteht.

func zaehle(sa *shardedAccounts) int {
	n := 0
	sa.Range(func(string, *AccountState) bool { n++; return true })
	return n
}

func TestBelegung_LoeschenUndNeuEintragen(t *testing.T) {
	sa := newShardedAccounts()
	if sa.Len() != 0 || zaehle(sa) != 0 {
		t.Fatal("leere Struktur meldet Eintraege")
	}
	adr := func(i int) string { return fmt.Sprintf("0x%040x", i) }
	for i := 0; i < 500; i++ {
		sa.Set(adr(i), &AccountState{Address: adr(i)})
	}
	sa.Set(adr(7), &AccountState{Address: adr(7)}) // ueberschreiben zaehlt nicht doppelt
	if sa.Len() != 500 || zaehle(sa) != 500 {
		t.Fatalf("nach dem Eintragen Len %d, Range %d -- erwartet 500", sa.Len(), zaehle(sa))
	}
	for i := 0; i < 500; i += 2 {
		sa.Delete(adr(i))
	}
	sa.Delete(adr(0))        // schon weg
	sa.Delete("0xunbekannt") // nie da
	if sa.Len() != 250 || zaehle(sa) != 250 {
		t.Fatalf("nach dem Loeschen Len %d, Range %d -- erwartet 250", sa.Len(), zaehle(sa))
	}
	for i := 0; i < 500; i += 2 {
		sa.Set(adr(i), &AccountState{Address: adr(i)})
	}
	if sa.Len() != 500 || zaehle(sa) != 500 {
		t.Fatalf("nach dem Neu-Eintragen Len %d, Range %d -- erwartet 500", sa.Len(), zaehle(sa))
	}
	// Ueber SetLocked eingetragen, ebenso sichtbar.
	unlock := sa.LockAddrs(adr(1000))
	sa.SetLocked(adr(1000), &AccountState{Address: adr(1000)})
	unlock()
	if _, ok := sa.Get(adr(1000)); !ok || sa.Len() != 501 {
		t.Fatal("ueber SetLocked eingetragener Eintrag fehlt")
	}
	// Ein geleerter Shard verschwindet aus dem Bitfeld.
	for i := 0; i <= 1000; i++ {
		sa.Delete(adr(i))
	}
	for w := range sa.belegt {
		if b := sa.belegt[w].Load(); b != 0 {
			t.Fatalf("nach dem Leeren ist Wort %d noch belegt: %x", w, b)
		}
	}
}

// Unter -race: Range und Len waehrend gleichzeitiger Set/Delete. Die festen
// Eintraege muessen JEDES Mal gesehen werden.
func TestBelegung_GleichzeitigeAenderungen(t *testing.T) {
	sa := newShardedAccounts()
	const fest = 300
	for i := 0; i < fest; i++ {
		a := fmt.Sprintf("0xfe%038x", i)
		sa.Set(a, &AccountState{Address: a})
	}
	var stop atomic.Bool
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; !stop.Load(); i++ {
				a := fmt.Sprintf("0x%02x%038x", w, i%200)
				sa.Set(a, &AccountState{Address: a})
				sa.Delete(a)
			}
		}(w)
	}
	for r := 0; r < 200; r++ {
		gesehen := 0
		sa.Range(func(addr string, _ *AccountState) bool {
			if len(addr) == 42 && addr[2:4] == "fe" {
				gesehen++
			}
			return true
		})
		if gesehen != fest {
			stop.Store(true)
			wg.Wait()
			t.Fatalf("Durchlauf %d sah %d von %d festen Eintraegen", r, gesehen, fest)
		}
		if n := sa.Len(); n < fest {
			stop.Store(true)
			wg.Wait()
			t.Fatalf("Len %d unter der Zahl der festen Eintraege %d", n, fest)
		}
	}
	stop.Store(true)
	wg.Wait()
}
