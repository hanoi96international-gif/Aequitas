package keeper

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Signierte Ueberweisungen (Stufe 1.0) im WAL-Pfad: Nonce pruefen und
// setzen, mit dem Kontostand schreiben, nach einem Absturz wiederherstellen
// -- und in pending_txs je Absender in steigender Reihenfolge ablegen, auch
// unter gleichzeitigen Flushes und mit dem seriellen Ausweichweg.

func rohKonto(t *testing.T, cs *ChainState, addr string, guthaben float64) {
	t.Helper()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	acc := &AccountState{Address: addr, IsHuman: true, Balance: NewDecimal(guthaben), LastActivityAt: time.Now().Unix()}
	if err := cs.saveAccountToDB(acc); err != nil {
		t.Fatal(err)
	}
	cs.accounts.Set(addr, acc)
}

func naechsteNonceVon(cs *ChainState, addr string) int64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if acc, ok := cs.accounts.Get(addr); ok {
		return acc.NaechsteNonce
	}
	return -1
}

// pendingNoncenJeAbsender liest pending_txs nach id und dekodiert die Nonce
// jeder signierten Ueberweisung.
func pendingNoncenJeAbsender(t *testing.T, cs *ChainState) map[string][]uint64 {
	t.Helper()
	rows, err := cs.db.Query(`SELECT tx_json FROM pending_txs ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string][]uint64{}
	for rows.Next() {
		var j string
		if err := rows.Scan(&j); err != nil {
			t.Fatal(err)
		}
		var tx Transaction
		if err := json.Unmarshal([]byte(j), &tx); err != nil {
			t.Fatal(err)
		}
		if n, ok := nonceAusVorlage(tx); ok {
			out[tx.Wallet] = append(out[tx.Wallet], n)
		}
	}
	return out
}

func vorlageAus(tx Transaction) Transaction {
	return Transaction{Type: "transfer", Wallet: tx.Wallet, To: tx.To, Amount: tx.Amount, TxHash: tx.TxHash, Roh: tx.Roh}
}

func TestWALRoh_NonceWirdGeprueftGesetztUndGeschrieben_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := newWALTestState(t, filepath.Join(t.TempDir(), "r.wal"))
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	rohKonto(t, cs, a.addr, 1000)
	rohKonto(t, cs, b.addr, 0)

	for _, n := range []uint64{0, 5} {
		tx := signiere(t, a, b.addr, aeqWei(1), n, 1926)
		if _, _, applied, err := cs.transferConcurrentWAL(a.addr, b.addr, tx.Amount, vorlageAus(tx)); !applied || err != nil {
			t.Fatalf("Nonce %d: applied=%v err=%v -- der WAL-Pfad soll signierte Ueberweisungen selbst annehmen", n, applied, err)
		}
	}
	if got := naechsteNonceVon(cs, a.addr); got != 6 {
		t.Fatalf("NaechsteNonce %d, erwartet 6", got)
	}
	alt := signiere(t, a, b.addr, aeqWei(1), 3, 1926)
	if _, _, applied, err := cs.transferConcurrentWAL(a.addr, b.addr, alt.Amount, vorlageAus(alt)); !applied || err == nil {
		t.Fatalf("verbrauchte Nonce 3 angenommen: applied=%v err=%v", applied, err)
	}
	cs.FlushWALNow()
	var dbNonce int64
	if err := cs.db.QueryRow(`SELECT naechste_nonce FROM chain_accounts WHERE lower(address) = $1`, a.addr).Scan(&dbNonce); err != nil {
		t.Fatal(err)
	}
	if dbNonce != 6 {
		t.Fatalf("naechste_nonce in Postgres %d, erwartet 6", dbNonce)
	}
	if got := pendingNoncenJeAbsender(t, cs)[a.addr]; fmt.Sprint(got) != "[0 5]" {
		t.Fatalf("pending_txs: Noncen %v, erwartet [0 5]", got)
	}
}

func TestWALRoh_ErholungSetztNonce_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	pfad := filepath.Join(t.TempDir(), "r.wal")
	csA := newWALTestState(t, pfad)
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	rohKonto(t, csA, a.addr, 1000)
	rohKonto(t, csA, b.addr, 0)
	for _, n := range []uint64{0, 1} {
		tx := signiere(t, a, b.addr, aeqWei(1), n, 1926)
		if _, _, applied, err := csA.transferConcurrentWAL(a.addr, b.addr, tx.Amount, vorlageAus(tx)); !applied || err != nil {
			t.Fatalf("applied=%v err=%v", applied, err)
		}
	}
	csA.stopWALFlushWorkerForTest()
	if err := csA.wal.Close(); err != nil {
		t.Fatal(err)
	}
	csB := newWALTestState(t, pfad)
	if got := naechsteNonceVon(csB, a.addr); got != 2 {
		t.Fatalf("NaechsteNonce nach der Erholung %d, erwartet 2", got)
	}
	csB.FlushWALNow()
	if got := pendingNoncenJeAbsender(t, csB)[a.addr]; fmt.Sprint(got) != "[0 1]" {
		t.Fatalf("pending_txs nach der Erholung: %v, erwartet [0 1]", got)
	}
}

// Der serielle Ausweichweg darf seine Zeile nicht vor noch ungeflushte
// WAL-Zeilen desselben Absenders setzen.
func TestWALRoh_SeriellerWegWartetAufOffeneWALZeilen_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	signierteUeberweisungenOverride.Store(1)
	t.Cleanup(func() { signierteUeberweisungenOverride.Store(0) })
	cs := newWALTestState(t, filepath.Join(t.TempDir(), "r.wal"))
	cs.stopWALFlushWorkerForTest() // nur ausdruecklich geflusht
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	rohKonto(t, cs, a.addr, 1000)
	rohKonto(t, cs, b.addr, 0)
	tx0 := signiere(t, a, b.addr, aeqWei(1), 0, 1926)
	if _, _, applied, err := cs.transferConcurrentWAL(a.addr, b.addr, tx0.Amount, vorlageAus(tx0)); !applied || err != nil {
		t.Fatalf("applied=%v err=%v", applied, err)
	}
	// Empfaenger nicht im Speicher: der WAL-Pfad tritt zurueck, der serielle
	// Weg nimmt an.
	kalt := neuerTestSchluessel(t)
	tx1 := signiere(t, a, kalt.addr, aeqWei(1), 1, 1926)
	if _, _, err := cs.TransferAtomic(a.addr, kalt.addr, tx1.Amount, vorlageAus(tx1)); err != nil {
		t.Fatalf("serielle Annahme: %v", err)
	}
	cs.FlushWALNow()
	if got := pendingNoncenJeAbsender(t, cs)[a.addr]; fmt.Sprint(got) != "[0 1]" {
		t.Fatalf("pending_txs: %v, erwartet [0 1] -- die serielle Zeile stand vor der WAL-Zeile", got)
	}
}

// Viele Absender, gleichzeitige Flushes: je Absender steigen die Noncen in
// pending_txs.
func TestWALRoh_GleichzeitigeFlushesHaltenDieReihenfolge_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	altIntervall := walFlushInterval
	walFlushInterval = 2 * time.Millisecond
	t.Cleanup(func() { walFlushInterval = altIntervall })
	cs := newWALTestState(t, filepath.Join(t.TempDir(), "r.wal"))
	const absender, je = 8, 60
	schluessel := make([]testSchluessel, absender)
	ziel := neuerTestSchluessel(t)
	rohKonto(t, cs, ziel.addr, 0)
	for i := range schluessel {
		schluessel[i] = neuerTestSchluessel(t)
		rohKonto(t, cs, schluessel[i].addr, 1000)
	}
	var wg sync.WaitGroup
	for i := range schluessel {
		wg.Add(1)
		go func(k testSchluessel) {
			defer wg.Done()
			for n := uint64(0); n < je; n++ {
				tx := signiere(t, k, ziel.addr, aeqWei(1), n, 1926)
				for {
					_, _, applied, err := cs.transferConcurrentWAL(k.addr, ziel.addr, tx.Amount, vorlageAus(tx))
					if err != nil {
						t.Errorf("%s Nonce %d: %v", k.addr, n, err)
						return
					}
					if applied {
						break
					}
					time.Sleep(time.Millisecond)
				}
			}
		}(schluessel[i])
	}
	wg.Wait()
	cs.FlushWALNow()
	noncen := pendingNoncenJeAbsender(t, cs)
	for _, k := range schluessel {
		got := noncen[k.addr]
		if len(got) != je {
			t.Fatalf("%s: %d Zeilen statt %d", k.addr, len(got), je)
		}
		for i := 1; i < len(got); i++ {
			if got[i] <= got[i-1] {
				t.Fatalf("%s: Nonce %d nach %d in pending_txs -- ein Block daraus waere fuer jeden anderen Validator ungueltig", k.addr, got[i], got[i-1])
			}
		}
	}
}

func TestWALRoh_SchnittVorAbsenderImLaufendenFlush(t *testing.T) {
	cs := &ChainState{walRohUnterwegs: map[string]int{"s": 1}}
	q := []walFlushItem{
		{from: "x", tx: Transaction{Roh: "0x01"}},
		{from: "s", tx: Transaction{}},            // unsigniert: darf mit
		{from: "s", tx: Transaction{Roh: "0x02"}}, // hier endet das Buendel
		{from: "y", tx: Transaction{Roh: "0x03"}},
	}
	if n := cs.walRohSchnittLocked(q, len(q)); n != 2 {
		t.Fatalf("Schnitt bei %d, erwartet 2", n)
	}
	cs.walRohUnterwegs = nil
	if n := cs.walRohSchnittLocked(q, len(q)); n != len(q) {
		t.Fatalf("ohne laufenden Flush Schnitt bei %d, erwartet %d", n, len(q))
	}
}
