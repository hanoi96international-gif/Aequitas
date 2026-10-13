package keeper

import (
	"context"
	"runtime"
	"testing"
	"time"
)

const (
	atomicA = "0x00000000000000000000000000000000000a7a01"
	atomicB = "0x00000000000000000000000000000000000a7a02"
)

// zustandFuerAtomicPanik: zwei Konten in Speicher und Datenbank.
func zustandFuerAtomicPanik(t *testing.T) *ChainState {
	t.Helper()
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-atomic-panik.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}
	cs.mu.Lock()
	for _, a := range []*AccountState{
		{Address: atomicA, IsHuman: true, Balance: NewDecimal(1000)},
		{Address: atomicB, IsHuman: true, Balance: NewDecimal(0)},
	} {
		cs.accounts.Set(a.Address, a)
		if err := cs.saveAccountToDB(a); err != nil {
			cs.mu.Unlock()
			t.Fatal(err)
		}
	}
	cs.mu.Unlock()
	return cs
}

// halbeBuchung veraendert Speicher und (in der Transaktion) die Datenbank --
// genau der Zustand, in dem eine Panik bisher die Sperre liegen liess.
func halbeBuchung(t *testing.T, cs *ChainState, ctx context.Context) {
	acc, ok := cs.accounts.Get(atomicA)
	if !ok {
		t.Fatal("Konto fehlt")
	}
	acc.Balance = NewDecimal(1)
	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		t.Fatal(err)
	}
}

func pruefeNachPanik(t *testing.T, cs *ChainState) {
	t.Helper()
	if !cs.mu.TryLock() {
		t.Fatal("cs.mu bleibt nach der Panik gesperrt -- der Knoten stuende")
	}
	offen := cs.activeTx
	cs.mu.Unlock()
	if offen != nil {
		t.Fatal("activeTx bleibt gesetzt")
	}
	if acc, _ := cs.accounts.Get(atomicA); acc.Balance.Float() != 1000 {
		t.Fatalf("Speicher nicht zurueckgerollt: %v", acc.Balance)
	}
	var stand float64
	if err := cs.db.QueryRow(`SELECT balance::float8 FROM chain_accounts WHERE address = $1`, atomicA).Scan(&stand); err != nil {
		t.Fatal(err)
	}
	if stand != 1000 && stand != 1000*float64(DecimalPrecision) {
		t.Fatalf("Datenbank nicht zurueckgerollt: %v", stand)
	}
}

// OFFEN 7b: eine Panik in fn -- ueber das oeffentliche RPC erreichbar -- liess
// cs.mu gesperrt, die Transaktion offen und den Speicher halb geschrieben.
// Jetzt wie im Fehlerpfad, und die Panik geht weiter nach oben.
func TestAtomic_PanikRolltZurueckUndGibtFrei_RealDB(t *testing.T) {
	cs := zustandFuerAtomicPanik(t)
	weiter := func() (r interface{}) {
		defer func() { r = recover() }()
		_ = cs.runAtomicWithOutbox([]string{atomicA, atomicB}, false, func(ctx context.Context) (Transaction, error) {
			halbeBuchung(t, cs, ctx)
			panic("mitten im Auftrag")
		})
		return nil
	}()
	if weiter == nil {
		t.Fatal("die Panik muss beim Aufrufer ankommen (500), nicht verschluckt werden")
	}
	pruefeNachPanik(t, cs)
	// Danach geht es normal weiter.
	if err := cs.runAtomicWithOutbox([]string{atomicA}, false, func(ctx context.Context) (Transaction, error) {
		return Transaction{Type: "noop", TxHash: "0xatomicnachpanik"}, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAtomicVerteilung_PanikRolltZurueckUndGibtFrei_RealDB(t *testing.T) {
	cs := zustandFuerAtomicPanik(t)
	weiter := func() (r interface{}) {
		defer func() { r = recover() }()
		_ = cs.runAtomicDistributionWithOutbox(func(ctx context.Context) ([]Transaction, error) {
			halbeBuchung(t, cs, ctx)
			panic("mitten in der Verteilung")
		})
		return nil
	}()
	if weiter == nil {
		t.Fatal("die Panik muss beim Aufrufer ankommen")
	}
	pruefeNachPanik(t, cs)
}

// Ohne Datenbank: die Sperre wird frei.
func TestAtomic_PanikOhneDatenbankGibtFrei(t *testing.T) {
	cs := &ChainState{accounts: newShardedAccounts()}
	for _, lauf := range []func(){
		func() {
			_ = cs.runAtomicWithOutbox(nil, false, func(context.Context) (Transaction, error) { panic("x") })
		},
		func() {
			_ = cs.runAtomicDistributionWithOutbox(func(context.Context) ([]Transaction, error) { panic("x") })
		},
	} {
		func() {
			defer func() { _ = recover() }()
			lauf()
		}()
		if !cs.mu.TryLock() {
			t.Fatal("cs.mu bleibt gesperrt")
		}
		cs.mu.Unlock()
	}
}

// Scheitert das Zurueckrollen selbst, ist der Zustand unzuverlaessig: Neustart.
func TestAtomic_RueckrollenScheitertStartetNeu_RealDB(t *testing.T) {
	cs := zustandFuerAtomicPanik(t)
	alt := atomicNeustart
	code := -1
	atomicNeustart = func(c int) { code = c }
	t.Cleanup(func() { atomicNeustart = alt })

	tx, err := cs.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	kaputt := &ChainState{db: cs.db, useDB: true} // ohne Kontenspeicher: das Zurueckrollen paniert
	kaputt.mu.Lock()
	ab := &atomicAbschluss{cs: kaputt, tx: tx, gesperrt: true,
		snap: &blockRollbackSnapshot{accounts: []accountSnapshot{{address: atomicA, existed: true}}}}
	func() {
		defer func() { _ = recover() }()
		func() {
			defer ab.beiPanik()
			panic("fn")
		}()
	}()
	if code != 9 {
		t.Fatalf("Neustart-Code %d, erwartet 9", code)
	}
	if !kaputt.mu.TryLock() {
		t.Fatal("Sperre bleibt liegen")
	}
}

// Nach tx.Commit wird nichts zurueckgerollt -- die Datenbank traegt den
// neuen Stand.
func TestAtomic_PanikNachCommitRolltNichtZurueck(t *testing.T) {
	cs := &ChainState{accounts: newShardedAccounts()}
	cs.mu.Lock()
	ab := &atomicAbschluss{cs: cs, gesperrt: true, abgeschlossen: true}
	func() {
		defer func() { _ = recover() }()
		func() {
			defer ab.beiPanik()
			panic("nach dem Commit")
		}()
	}()
	if !cs.mu.TryLock() {
		t.Fatal("Sperre bleibt liegen")
	}
}

// zustandAendern: eine Panik gibt die Sperre frei und startet neu -- der
// Zustand kann halb geschrieben sein, und es gibt keinen Schnappschuss.
func TestZustandAendern_PanikGibtFreiUndStartetNeu(t *testing.T) {
	alt := atomicNeustart
	code := -1
	cs := &ChainState{accounts: newShardedAccounts()}
	unterSperre := false
	atomicNeustart = func(c int) {
		code = c
		// Pruefung #339: der Neustart geschieht UNTER der Sperre -- sonst
		// koennte eine andere Goroutine den halben Zustand noch speichern.
		if cs.mu.TryLock() {
			cs.mu.Unlock()
		} else {
			unterSperre = true
		}
	}
	t.Cleanup(func() { atomicNeustart = alt })

	weiter := func() (r interface{}) {
		defer func() { r = recover() }()
		cs.zustandAendern("test", func() { panic("halb geschrieben") })
		return nil
	}()
	if weiter == nil || code != 9 {
		t.Fatalf("Panik %v, Neustart-Code %d", weiter, code)
	}
	if !unterSperre {
		t.Fatal("Neustart ohne gehaltene Sperre")
	}
	if !cs.mu.TryLock() {
		t.Fatal("cs.mu bleibt gesperrt")
	}
	cs.mu.Unlock()
	// Gutfall: kein Neustart, Sperre frei.
	code = -1
	lief := false
	cs.zustandAendern("test", func() { lief = true })
	if !lief || code != -1 || !cs.mu.TryLock() {
		t.Fatal("Gutfall")
	}
	cs.mu.Unlock()
}

// Pruefung #339: eine Panik WAEHREND des Commits (im Treiber, oder beim
// Zurueckrollen nach einem gescheiterten Commit) -- Ausgang unbekannt:
// Neustart unter der Sperre. Nach einem gelungenen Commit: kein Neustart,
// nichts zurueckrollen.
func TestAtomic_PanikImCommitStartetNeu(t *testing.T) {
	alt := atomicNeustart
	t.Cleanup(func() { atomicNeustart = alt })
	for _, fall := range []struct {
		name                    string
		imCommit, abgeschlossen bool
		neustart                bool
	}{
		{"im Commit", true, false, true},
		{"nach dem Commit", true, true, false},
	} {
		cs := &ChainState{accounts: newShardedAccounts()}
		code, unterSperre := -1, false
		atomicNeustart = func(c int) {
			code = c
			if cs.mu.TryLock() {
				cs.mu.Unlock()
			} else {
				unterSperre = true
			}
		}
		cs.mu.Lock()
		ab := &atomicAbschluss{cs: cs, gesperrt: true, imCommit: fall.imCommit, abgeschlossen: fall.abgeschlossen}
		func() {
			defer func() { _ = recover() }()
			func() {
				defer ab.beiPanik()
				panic("im Treiber")
			}()
		}()
		if fall.neustart && (code != 9 || !unterSperre) {
			t.Fatalf("%s: Neustart %d, unter Sperre %v", fall.name, code, unterSperre)
		}
		if !fall.neustart && code != -1 {
			t.Fatalf("%s: unerwarteter Neustart", fall.name)
		}
		if !cs.mu.TryLock() {
			t.Fatalf("%s: Sperre bleibt liegen", fall.name)
		}
		cs.mu.Unlock()
	}
}

// Pruefung #339, NIEDRIG: runtime.Goexit (etwa t.Fatal in fn) verlaesst die
// Funktion ohne Panik -- die Sperre wird trotzdem frei.
func TestAtomic_GoexitGibtFrei(t *testing.T) {
	cs := &ChainState{accounts: newShardedAccounts()}
	fertig := make(chan struct{})
	go func() {
		defer close(fertig)
		cs.mu.Lock()
		ab := &atomicAbschluss{cs: cs, gesperrt: true, abgeschlossen: true}
		defer ab.beiPanik()
		runtime.Goexit()
	}()
	<-fertig
	if !cs.mu.TryLock() {
		t.Fatal("Sperre bleibt nach Goexit liegen")
	}
	cs.mu.Unlock()
}

// Pruefung #339, MITTEL: eine Panik unter der Buchfuehrungs-Sperre (w.mu)
// darf sie nicht liegen lassen -- das Zuruecksetzen nach der Panik
// (buchZurueck) wartet auf w.mu, und haelt dabei cs.mu.
func TestAtomic_PanikUnterBuchSperreBlockiertNicht_RealDB(t *testing.T) {
	cs := zustandFuerAtomicPanik(t)
	fertig := make(chan interface{}, 1)
	go func() {
		defer func() { fertig <- recover() }()
		_ = cs.runAtomicWithOutbox([]string{atomicA, atomicB}, false, func(ctx context.Context) (Transaction, error) {
			halbeBuchung(t, cs, ctx)
			cs.wirt().mitSperre(func() { panic("in der Buchfuehrung") })
			return Transaction{}, nil
		})
	}()
	select {
	case r := <-fertig:
		if r == nil {
			t.Fatal("die Panik muss ankommen")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Zuruecksetzen blockiert -- w.mu blieb liegen")
	}
	if !cs.wirt().mu.TryLock() {
		t.Fatal("w.mu bleibt gesperrt")
	}
	cs.wirt().mu.Unlock()
	pruefeNachPanik(t, cs)
}
