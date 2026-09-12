package keeper

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/wal"
)

// Ein ChainState mit WAL, ohne Datenbank: der schnelle Pfad laeuft, der Flush
// kehrt bei cs.db == nil sofort zurueck.
func neuerWALStateOhneDB(t *testing.T) *ChainState {
	t.Helper()
	w, err := wal.Open(filepath.Join(t.TempDir(), "test.wal"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	cs := &ChainState{accounts: newShardedAccounts(), wal: w}
	walSchnellpfadDefekt.Store(false)
	return cs
}

func konto(cs *ChainState, addr string, saldo float64) {
	acc := &AccountState{Address: addr, Balance: NewDecimal(saldo), LastActivityAt: time.Now().Unix()}
	cs.accounts.Set(addr, acc)
}

// DIE ZUSAGE, DIE ALLES TRAEGT: wenn transferConcurrentWAL zurueckkehrt, ist
// die WAL-Seq des Kontos haltbar. Seit AppendAsync wird der Saldo VOR dem
// Sync geaendert -- die Quittung darf trotzdem erst danach kommen. Sonst
// koennte ein Aufrufer eine Ueberweisung als angenommen sehen, die nach einem
// Absturz im WAL fehlt.
func TestTransferWAL_NachRueckkehrIstDieSeqHaltbar(t *testing.T) {
	cs := neuerWALStateOhneDB(t)
	// Je Goroutine ein eigener Absender: nur so ist die WALSeq des Absenders
	// nach der Rueckkehr die MEINER Ueberweisung und nicht die einer anderen,
	// die gerade auf demselben Konto laeuft. Der Empfaenger ist geteilt --
	// die Datensaetze landen im WAL trotzdem nebenlaeufig.
	for g := 0; g < 16; g++ {
		konto(cs, "0xfrom"+string(rune('a'+g)), 1000)
	}
	konto(cs, "0xto", 0)

	var wg sync.WaitGroup
	var verletzt, angewendet int64
	var mu sync.Mutex
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			from := "0xfrom" + string(rune('a'+g))
			for i := 0; i < 20; i++ {
				_, _, applied, err := cs.transferConcurrentWAL(from, "0xto", 1, Transaction{Type: "transfer", TxHash: "0xt"})
				if err != nil {
					t.Errorf("Ueberweisung: %v", err)
					return
				}
				if !applied {
					continue // Rueckfall (Shard belegt) -- nicht Gegenstand hier
				}
				acc, _ := cs.accounts.Get(from)
				seq := acc.WALSeq
				mu.Lock()
				angewendet++
				if cs.wal.DurableSeq() < seq {
					verletzt++
				}
				mu.Unlock()
			}
		}(g)
	}
	wg.Wait()
	if angewendet == 0 {
		t.Fatal("keine Ueberweisung lief den schnellen Pfad -- der Test hat nichts geprueft")
	}
	if verletzt > 0 {
		t.Errorf("%d von %d Ueberweisungen kehrten zurueck, bevor ihre WAL-Seq haltbar war", verletzt, angewendet)
	}
	summe := 0.0
	for g := 0; g < 16; g++ {
		acc, _ := cs.accounts.Get("0xfrom" + string(rune('a'+g)))
		summe += acc.Balance.Float()
	}
	to, _ := cs.accounts.Get("0xto")
	if summe+to.Balance.Float() != 16000 {
		t.Errorf("Geldmenge %v + %v != 16000", summe, to.Balance.Float())
	}
	t.Logf("%d Ueberweisungen ueber den schnellen Pfad, Geldmenge erhalten", angewendet)
}

// DER GRUND FUER DEN UMBAU: Ueberweisungen DESSELBEN Absenders muessen
// denselben Gruppen-Commit teilen koennen. Vorher hielt jede ihre
// Shard-Sperre bis zum fsync -- 100 nacheinander, je eine Sync-Wartezeit.
// Jetzt landet die Wartezeit ausserhalb der Sperre. Messbar: 200
// nebenlaeufige Ueberweisungen eines Absenders brauchen deutlich weniger als
// 200 Syncs.
func TestTransferWAL_GleicherAbsenderTeiltDenGruppenCommit(t *testing.T) {
	cs := neuerWALStateOhneDB(t)
	konto(cs, "0xa", 100000)
	for i := 0; i < 8; i++ {
		konto(cs, "0xz"+string(rune('a'+i)), 0)
	}
	t.Setenv(shardRetryVersucheEnv, "200")
	t.Setenv(shardRetryPauseEnv, "200")
	shardRetryZuruecksetzen()

	syncsVorher := wal.WriterStats()["batches"].(int64)
	start := time.Now()
	var wg sync.WaitGroup
	var angewendet int64
	var mu sync.Mutex
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			to := "0xz" + string(rune('a'+g))
			for i := 0; i < 25; i++ {
				_, _, applied, err := cs.transferConcurrentWAL("0xa", to, 1, Transaction{Type: "transfer", TxHash: "0xt"})
				if err == nil && applied {
					mu.Lock()
					angewendet++
					mu.Unlock()
				}
			}
		}(g)
	}
	wg.Wait()
	dauer := time.Since(start)
	if angewendet < 100 {
		t.Fatalf("nur %d von 200 liefen den schnellen Pfad", angewendet)
	}
	// SYNCS ZAEHLEN, NICHT ZEIT. Haette jede Ueberweisung ihre Shard-Sperre
	// bis zum fsync gehalten, waere je Ueberweisung ein eigener Gruppen-
	// Commit noetig gewesen: so viele Syncs wie Ueberweisungen. Teilen sie
	// den Commit, sind es deutlich weniger. Eine Stoppuhr wuerde auf einer
	// geteilten Entwickler-CPU kippen; die Zahl der Syncs nicht.
	syncs := wal.WriterStats()["batches"].(int64) - syncsVorher
	if syncs > angewendet/2 {
		t.Errorf("%d Syncs fuer %d Ueberweisungen eines Absenders -- sie warten wieder je einzeln "+
			"auf ihren fsync statt den Gruppen-Commit zu teilen", syncs, angewendet)
	}
	t.Logf("%d Ueberweisungen eines Absenders in %d Syncs, %s", angewendet, syncs, dauer)
}
