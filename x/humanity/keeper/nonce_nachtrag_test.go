package keeper

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

func sendeRoh(t *testing.T, srv *EVMRPCServer, roh string) *RPCError {
	t.Helper()
	p, _ := json.Marshal(roh)
	_, rerr := srv.sendRawTransaction([]json.RawMessage{p}, nil)
	return rerr
}

// Ohne synchronen Postgres-Umlauf reserviert -- und trotzdem bucht dieselbe
// signierte Ueberweisung nie zweimal.
func TestNonceNachtrag_WiederholungScheitertOhneUmlauf(t *testing.T) {
	signierteUeberweisungenOverride.Store(1)
	t.Cleanup(func() { signierteUeberweisungenOverride.Store(math.MaxInt64) })
	noteBlockProduced()
	cs := newTestState()
	srv := NewEVMRPCServer(&BlockDAG{state: cs}, cs)
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	cs.mu.Lock()
	cs.accounts.Set(a.addr, &AccountState{Address: a.addr, Balance: NewDecimal(1000)})
	cs.mu.Unlock()

	vorher := nonceSchnellReserviert.Load()
	tx := signiere(t, a, b.addr, aeqWei(1), 0, 1926)
	if rerr := sendeRoh(t, srv, tx.Roh); rerr != nil {
		t.Fatalf("erste Einreichung abgelehnt: %v", rerr.Message)
	}
	if nonceSchnellReserviert.Load() != vorher+1 {
		t.Fatal("nicht ueber den Weg ohne Umlauf reserviert")
	}
	if rerr := sendeRoh(t, srv, tx.Roh); rerr == nil {
		t.Fatal("dieselbe signierte Ueberweisung ein zweites Mal angenommen")
	}
	if got := kontoVon(t, cs, a.addr).NaechsteNonce; got != 1 {
		t.Fatalf("NaechsteNonce %d statt 1", got)
	}
	if bal := kontoVon(t, cs, a.addr).Balance.Float(); bal < 998.9 || bal > 999.0 {
		t.Fatalf("Kontostand %v: genau eine Buchung erwartet", bal)
	}
}

// Neustart, bei dem der Nachtrag verloren ging: evm_nonces kennt die letzte
// Nonce nicht, die Kette schon. Die Wallet fragt die Kette und schickt die
// naechste -- sie muss angenommen werden, eine verbrauchte nicht.
func TestNonceNachtrag_NachNeustartGiltDieKette(t *testing.T) {
	signierteUeberweisungenOverride.Store(1)
	t.Cleanup(func() { signierteUeberweisungenOverride.Store(math.MaxInt64) })
	noteBlockProduced()
	cs := newTestState()
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	cs.mu.Lock()
	cs.accounts.Set(a.addr, &AccountState{Address: a.addr, Balance: NewDecimal(1000), NaechsteNonce: 5})
	cs.mu.Unlock()
	srv := NewEVMRPCServer(&BlockDAG{state: cs}, cs) // frischer Prozess: leerer Nonce-Speicher

	if rerr := sendeRoh(t, srv, signiere(t, a, b.addr, aeqWei(1), 4, 1926).Roh); rerr == nil {
		t.Fatal("verbrauchte Nonce 4 angenommen")
	}
	if rerr := sendeRoh(t, srv, signiere(t, a, b.addr, aeqWei(1), 5, 1926).Roh); rerr != nil {
		t.Fatalf("naechste Nonce 5 abgelehnt: %s", rerr.Message)
	}
}

func TestNonceNachtrag_NurEinfacheUeberweisungUnterPflicht(t *testing.T) {
	signierteUeberweisungenOverride.Store(1)
	t.Cleanup(func() { signierteUeberweisungenOverride.Store(math.MaxInt64) })
	if !nonceOhneUmlauf(true, 100) {
		t.Fatal("einfache Ueberweisung unter Pflicht nicht ohne Umlauf")
	}
	if nonceOhneUmlauf(false, 100) {
		t.Fatal("Vertragsaufruf ohne Umlauf reserviert -- seine Nonce steht nicht in der Kette")
	}
	t.Setenv("AEQUITAS_NONCE_SYNCHRON", "1")
	if nonceOhneUmlauf(true, 100) {
		t.Fatal("Notschalter wirkt nicht")
	}
	t.Setenv("AEQUITAS_NONCE_SYNCHRON", "")
	signierteUeberweisungenOverride.Store(math.MaxInt64 - 1)
	if nonceOhneUmlauf(true, 100) {
		t.Fatal("vor der Pflicht ohne Umlauf reserviert")
	}
}

// Mit Datenbank: ein synchroner Compare-and-swap nach Reservierungen ohne
// Umlauf darf nicht gegen den veralteten Wert laufen.
func TestNonceNachtrag_SynchronNachSchnell_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-nonce-nachtrag-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}
	addr := "0x3333333333333333333333333333333333333ccc"
	if _, err := cs.db.Exec(`DELETE FROM evm_nonces WHERE address = $1`, addr); err != nil {
		t.Fatal(err)
	}
	// Drei Reservierungen ohne Umlauf: 0 -> 3, evm_nonces noch leer.
	for n := uint64(1); n <= 3; n++ {
		cs.merkeNonceNachtrag(addr, n)
	}
	// Der synchrone Weg (Vertragsaufruf) erwartet 3 -- und bekommt es.
	ok, err := cs.ReserveNonce(addr, 3, 4)
	if err != nil || !ok {
		t.Fatalf("synchroner Compare-and-swap nach Nachtrag: ok=%v err=%v", ok, err)
	}
	if got := cs.LoadNonce(addr); got != 4 {
		t.Fatalf("evm_nonces %d statt 4", got)
	}
	// Nachtrag schreibt nur steigend.
	cs.merkeNonceNachtrag(addr, 2)
	cs.schreibeNonceNachtraege()
	if got := cs.LoadNonce(addr); got != 4 {
		t.Fatalf("Nachtrag hat evm_nonces gesenkt: %d", got)
	}
	// Kette vorn (verlorener Nachtrag): gespeicherteNonce nimmt sie und zieht nach.
	cs.mu.Lock()
	cs.accounts.Set(addr, &AccountState{Address: addr, NaechsteNonce: 9})
	cs.mu.Unlock()
	if got := cs.gespeicherteNonce(addr); got != 9 {
		t.Fatalf("gespeicherteNonce %d statt 9", got)
	}
	if ok, err := cs.ReserveNonce(addr, 9, 10); err != nil || !ok {
		t.Fatalf("Compare-and-swap nach Ketten-Nachtrag: ok=%v err=%v", ok, err)
	}
}
