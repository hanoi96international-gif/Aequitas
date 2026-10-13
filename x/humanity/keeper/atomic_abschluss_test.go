package keeper

import (
	"context"
	"testing"
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
	cs.mu.Lock()
	kaputt.mu.Lock()
	cs.mu.Unlock()
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
