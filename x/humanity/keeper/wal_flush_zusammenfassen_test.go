package keeper

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// --- Ohne Datenbank -------------------------------------------------------

func zusammenfassenTestState(t *testing.T) *ChainState {
	t.Helper()
	cs := &ChainState{}
	t.Cleanup(cs.stopWALFlushWorkerForTest)
	return cs
}

func walSchlangeLaenge(cs *ChainState) int {
	cs.walFlushMu.Lock()
	defer cs.walFlushMu.Unlock()
	return len(cs.walFlushQueue)
}

// Nur Eintraege ohne Outbox-Zeile, und nur wenn BEIDE Konten in einem
// wartenden Eintrag stehen.
func TestWALZusammenfassen_NurWennBeideKontenWarten(t *testing.T) {
	cs := zusammenfassenTestState(t)
	vorher := walFlushZusammengefasst.Load()
	for i := 0; i < 3; i++ {
		cs.enqueueWALFlushMitLocked("a", "b", Transaction{}, uint64(10+i), true)
	}
	if n := walSchlangeLaenge(cs); n != 1 {
		t.Fatalf("%d Eintraege fuer drei Ueberweisungen a->b, erwartet 1", n)
	}
	if d := walFlushZusammengefasst.Load() - vorher; d != 2 {
		t.Fatalf("zusammengefasst %d, erwartet 2", d)
	}
	// c wartet nirgends: eigener Eintrag.
	cs.enqueueWALFlushMitLocked("a", "c", Transaction{}, 20, true)
	if n := walSchlangeLaenge(cs); n != 2 {
		t.Fatalf("a->c braucht einen eigenen Eintrag (c wartet nicht), Laenge %d", n)
	}
	// Mit Outbox-Zeile: nie zusammenfassen -- die Zeile ist die Ueberweisung.
	cs.enqueueWALFlushMitLocked("a", "b", Transaction{}, 21, false)
	if n := walSchlangeLaenge(cs); n != 3 {
		t.Fatalf("Eintrag mit Outbox-Zeile wurde zusammengefasst (Laenge %d) -- die Ueberweisung kaeme in keinen Block", n)
	}
}

// Entnommen heisst: der Flush macht seine Momentaufnahme gleich -- was danach
// kommt, braucht einen neuen Eintrag. Scheitert der Flush, deckt das
// zurueckgelegte Buendel wieder.
func TestWALZusammenfassen_EntnahmeUndRueckgabe(t *testing.T) {
	cs := zusammenfassenTestState(t)
	cs.enqueueWALFlushMitLocked("a", "b", Transaction{}, 1, true)
	cs.walFlushMu.Lock()
	batch := cs.walFlushQueue
	cs.walFlushQueue = nil
	cs.walGenommenLocked(batch)
	cs.walFlushMu.Unlock()

	cs.enqueueWALFlushMitLocked("a", "b", Transaction{}, 2, true)
	if n := walSchlangeLaenge(cs); n != 1 {
		t.Fatalf("nach der Entnahme muss a->b einen neuen Eintrag bekommen, Laenge %d -- sonst fehlte die Ueberweisung in Postgres", n)
	}
	// Den neuen Eintrag ebenfalls entnehmen, dann scheitert der erste Flush.
	cs.walFlushMu.Lock()
	zweites := cs.walFlushQueue
	cs.walFlushQueue = nil
	cs.walGenommenLocked(zweites)
	cs.walFlushQueue = append(batch, cs.walFlushQueue...)
	cs.walZurueckLocked(batch)
	cs.walFlushMu.Unlock()

	cs.enqueueWALFlushMitLocked("a", "b", Transaction{}, 3, true)
	if n := walSchlangeLaenge(cs); n != 1 {
		t.Fatalf("das zurueckgelegte Buendel deckt a->b, Laenge %d", n)
	}
	if m, ok := cs.walOffenMinSeq(); !ok || m != 1 {
		t.Fatalf("kleinste offene Seq %d/%v, erwartet 1", m, ok)
	}
	cs.walFlushMu.Lock()
	cs.walAbgeschlossenLocked(zweites)
	cs.walFlushMu.Unlock()
	if len(cs.walUnterwegsMin) != 0 {
		t.Fatalf("laufende Flushes nicht abgeraeumt: %v", cs.walUnterwegsMin)
	}
}

// Ein laufender Flush haelt seine Seq offen, auch wenn die Warteschlange leer ist.
func TestWALOffenMinSeq_LaufenderFlushZaehlt(t *testing.T) {
	cs := zusammenfassenTestState(t)
	if _, ok := cs.walOffenMinSeq(); ok {
		t.Fatal("leer, aber offen gemeldet")
	}
	cs.enqueueWALFlushMitLocked("a", "b", Transaction{}, 500, true)
	cs.enqueueWALFlushMitLocked("c", "d", Transaction{}, 400, true)
	cs.walFlushMu.Lock()
	batch := cs.walFlushQueue
	cs.walFlushQueue = nil
	cs.walGenommenLocked(batch)
	cs.walFlushMu.Unlock()
	cs.enqueueWALFlushMitLocked("e", "f", Transaction{}, 900, true)
	if m, ok := cs.walOffenMinSeq(); !ok || m != 400 {
		t.Fatalf("kleinste offene Seq %d/%v, erwartet 400 (im laufenden Flush)", m, ok)
	}
	cs.walFlushMu.Lock()
	cs.walAbgeschlossenLocked(batch)
	cs.walFlushMu.Unlock()
	if m, _ := cs.walOffenMinSeq(); m != 900 {
		t.Fatalf("nach dem Flush %d, erwartet 900", m)
	}
}

// Missbrauch der alten Rechnung: EIN Eintrag deckt 100.000 Datensaetze. Mit
// "Kopf - Laenge - Abstand" wuerden ungeflushte Datensaetze gekuerzt -- nach
// einem Absturz fehlten ihre Wirkungen. Die kleinste offene Seq verhindert das.
func TestWALKuerzenBis_ZusammengefassteEintraegeBegrenzen(t *testing.T) {
	const kopf, abstand = uint64(1_000_000), uint64(300_000)
	alt := walKuerzenBis(kopf, 1, abstand, 0, false, 0, 0, false)
	if alt <= 600_000 {
		t.Fatalf("Vorbedingung: die alte Rechnung allein kuerzt bis %d", alt)
	}
	if got := walKuerzenBis(kopf, 1, abstand, 0, false, 0, 600_000, true); got != 300_000 {
		t.Fatalf("bis %d, erwartet 300000 (kleinste offene Seq minus Abstand)", got)
	}
	if got := walKuerzenBis(kopf, 1, abstand, 0, false, 0, 200_000, true); got != 0 {
		t.Fatalf("bis %d, erwartet 0 (offene Seq innerhalb des Abstands)", got)
	}
	// Die Korbmarke bleibt die strengere Grenze, wenn sie tiefer liegt.
	if got := walKuerzenBis(kopf, 1, abstand, 0, true, 100_000, 600_000, true); got != 100_001 {
		t.Fatalf("bis %d, erwartet 100001 (Korbmarke)", got)
	}
}

// --- Mit echter Datenbank und echtem WAL (CI-Gruppe "wal") ---------------

// Viele Ueberweisungen zwischen wenigen Konten, nebenlaeufig, waehrend der
// Flush-Arbeiter laeuft: am Ende steht in Postgres genau der Stand im
// Speicher -- keine zusammengefasste Ueberweisung geht verloren.
func TestWALZusammenfassen_NebenlaeufigNichtsVerloren(t *testing.T) {
	truncateDistTestTables(t)
	cs := korbTestState(t, filepath.Join(t.TempDir(), "t.wal"), true)
	const konten = 6
	for i := 0; i < konten; i++ {
		seedConcurrentTestAccount(t, cs, distTestAddr(5000+i), 10000, time.Now().Unix())
	}
	vorher := walFlushZusammengefasst.Load()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 150; i++ {
				from := distTestAddr(5000 + (g+i)%konten)
				to := distTestAddr(5000 + (g+i+1)%konten)
				for {
					_, _, applied, err := cs.transferConcurrentWAL(from, to, 1, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 1, TxHash: fmt.Sprintf("0xzus%d-%d", g, i)})
					if err != nil {
						t.Errorf("Ueberweisung: %v", err)
						return
					}
					if applied {
						break
					}
					time.Sleep(time.Millisecond) // Shard belegt: erneut
				}
				if i%25 == 0 {
					cs.flushWALQueue()
				}
			}
		}(g)
	}
	wg.Wait()
	if walFlushZusammengefasst.Load() == vorher {
		t.Fatal("nichts zusammengefasst -- der Test prueft dann nichts")
	}
	if !cs.wal.WaitDurable(cs.wal.PeekSeq()-1, 5*time.Second) {
		t.Fatal("WAL wurde nicht haltbar")
	}
	cs.FlushWALNow()
	for i := 0; i < konten; i++ {
		a := distTestAddr(5000 + i)
		acc, _ := cs.accounts.Get(a)
		var saldo float64
		var seq int64
		if err := cs.db.QueryRow(`SELECT balance, wal_seq FROM chain_accounts WHERE lower(address) = $1`, a).Scan(&saldo, &seq); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
		if saldo != acc.Balance.Float() || uint64(seq) != acc.WALSeq {
			t.Fatalf("%s: Postgres %v/%d, Speicher %v/%d -- eine zusammengefasste Ueberweisung fehlt", a, saldo, seq, acc.Balance.Float(), acc.WALSeq)
		}
	}
	if m, ok := cs.walOffenMinSeq(); ok {
		t.Fatalf("nach dem vollen Flush noch offen ab Seq %d", m)
	}
}

// Absturz mit zusammengefassten, noch nicht geflushten Ueberweisungen: nach
// dem Neustart stimmen die Kontostaende (das WAL traegt jede einzelne) und
// alle stehen wieder im Korb.
func TestWALZusammenfassen_AbsturzOhneFlush(t *testing.T) {
	truncateDistTestTables(t)
	walPath := filepath.Join(t.TempDir(), "t.wal")
	a := korbTestState(t, walPath, true)
	from, to := distTestAddr(5101), distTestAddr(5102)
	seedConcurrentTestAccount(t, a, from, 1000, time.Now().Unix())
	seedConcurrentTestAccount(t, a, to, 0, time.Now().Unix())
	vorher := walFlushZusammengefasst.Load()
	korbUeberweisen(t, a, from, to, 40, "0xabs")
	if walFlushZusammengefasst.Load()-vorher < 30 {
		t.Fatalf("nur %d zusammengefasst", walFlushZusammengefasst.Load()-vorher)
	}
	vorF, _ := a.accounts.Get(from)
	sollF := vorF.Balance.Float()
	korbAbsturz(t, a)

	b := korbTestState(t, walPath, true)
	accF, _ := b.accounts.Get(from)
	accT, _ := b.accounts.Get(to)
	if accF.Balance.Float() != sollF || accT.Balance.Float() != 40 {
		t.Fatalf("nach dem Neustart %v / %v, erwartet %v / 40", accF.Balance.Float(), accT.Balance.Float(), sollF)
	}
	if b.korb.laenge() != 40 {
		t.Fatalf("%d im Korb, erwartet 40", b.korb.laenge())
	}
	b.FlushWALNow()
	var saldo float64
	if err := b.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, to).Scan(&saldo); err != nil || saldo != 40 {
		t.Fatalf("Postgres %v (%v), erwartet 40", saldo, err)
	}
}
