package keeper

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// Eine Ueberweisung mit Gebuehr (TransferWithV7FeeAtomic) muss auf einem
// nachspielenden Knoten zu exakt denselben Kontostaenden fuehren wie auf dem
// erzeugenden. Bis zum 24.09.2026 trug die Transaktion nur den Nettobetrag:
// der Nachspielende belastete den Absender um die Gebuehr zu wenig und
// schrieb dem Grundeinkommen nichts gut -- die Kontostaende liefen
// auseinander, sobald mehr als ein Validator nachspielt.
func TestUeberweisungsgebuehr_NachspielenGleich_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-gebuehr-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	a, b := distTestAddr(60), distTestAddr(61)
	jetzt := time.Now().Unix()
	cs.mu.Lock()
	for _, acc := range []*AccountState{
		{Address: a, Balance: NewDecimal(1000), IsHuman: true, LastActivityAt: jetzt},
		{Address: b, Balance: NewDecimal(1000), IsHuman: true, LastActivityAt: jetzt},
	} {
		if err := cs.saveAccountToDB(acc); err != nil {
			cs.mu.Unlock()
			t.Fatal(err)
		}
		cs.accounts.Set(acc.Address, acc)
	}
	cs.humanCount = 2
	cs.mu.Unlock()

	const hash = "0xgebuehr-nachspielen"
	net, _, _, err := cs.TransferWithV7FeeAtomic(a, b, 100, Transaction{Type: "transfer", Wallet: a, To: b, TxHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	if net != 100 {
		t.Fatalf("netto %v -- der Empfaenger bekommt den vollen Betrag, die Gebuehr zahlt der Absender obendrauf", net)
	}
	var roh string
	if err := cs.db.QueryRow(`SELECT tx_json FROM pending_txs WHERE tx_json LIKE '%' || $1 || '%'`, hash).Scan(&roh); err != nil {
		t.Fatal(err)
	}
	var tx Transaction
	if err := json.Unmarshal([]byte(roh), &tx); err != nil {
		t.Fatal(err)
	}
	if tx.Amount != 100 || tx.Gebuehr <= 0 {
		t.Fatalf("Transaktion traegt Amount %v, Gebuehr %v -- erwartet 100 und eine Gebuehr", tx.Amount, tx.Gebuehr)
	}
	// Der Erzeuger schreibt die Gebuehr gut, wenn die Ueberweisung im Block
	// steht (ProduceBlock -> gebuehrenInsGrundeinkommen).
	cs.gebuehrenInsGrundeinkommen(gebuehrenSumme([]Transaction{tx}))

	// Ein zweiter Knoten spielt genau diese Transaktion nach.
	nach := newTestState()
	nach.accounts.Set(a, &AccountState{Address: a, Balance: NewDecimal(1000), IsHuman: true, LastActivityAt: jetzt})
	nach.accounts.Set(b, &AccountState{Address: b, Balance: NewDecimal(1000), IsHuman: true, LastActivityAt: jetzt})
	nach.humanCount = 2
	nach.pool = &PoolState{}
	nach.mu.Lock()
	err = nach.applyTransferDeltaLockedSammelnd(context.Background(), tx.Wallet, tx.To, tx.Amount, tx.FromDemurrageLost, tx.ToDemurrageLost, jetzt, nil, tx.Gebuehr)
	nach.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range []string{a, b, ubiPoolAddr} {
		var e, n float64
		if acc, ok := cs.accounts.Get(addr); ok {
			e = acc.Balance.Float()
		}
		if acc, ok := nach.accounts.Get(addr); ok {
			n = acc.Balance.Float()
		}
		if e != n {
			t.Errorf("%s: Erzeuger %.6f, Nachspielender %.6f", addr, e, n)
		}
	}
}
