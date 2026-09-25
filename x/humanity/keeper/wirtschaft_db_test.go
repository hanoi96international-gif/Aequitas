package keeper

import (
	"context"
	"fmt"
	"os"
	"testing"
)

// Die Buchfuehrung liegt in derselben Transaktion wie die Kontostaende: nach
// einem Commit steht sie in der Datenbank, nach einem Abbruch nicht, und ein
// neu gestarteter Knoten laedt sie samt buchSeit.
func TestWirtschaftBuchfuehrungDauerhaft_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL must point at a disposable local Postgres database")
	}
	truncateDistTestTables(t)
	wirtschaftAn(t)
	uhr(t, 1_800_000_000)
	cs := testKnoten(t, "unused-wirtschaft-db-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}
	for _, tabelle := range []string{"wirtschaft_buch", "wirtschaft_unternehmen", "wirtschaft_meta"} {
		if _, err := cs.db.Exec("DELETE FROM " + tabelle); err != nil {
			t.Fatal(err)
		}
	}
	cs.wirtschaftP.Store(neueWirtschaft()) // beim Start Geladenes verwerfen
	ctx := context.Background()
	cs.mu.Lock()
	for _, a := range []*AccountState{
		{Address: wMensch1, IsHuman: true, Balance: NewDecimal(1000)},
		{Address: wMensch2, IsHuman: true, Balance: NewDecimal(5000)},
		{Address: wFirmaA, Balance: NewDecimal(10)},
	} {
		cs.accounts.Set(a.Address, a)
		if err := cs.saveAccountToDB(a); err != nil {
			cs.mu.Unlock()
			t.Fatal(err)
		}
	}
	cs.mu.Unlock()
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)

	zahle := func(betrag float64, fehler error) error {
		return cs.runAtomicWithOutbox([]string{wMensch2, wFirmaA}, false, func(ctx context.Context) (Transaction, error) {
			if _, _, _, err := cs.transferLockedMitGebuehr(ctx, wMensch2, wFirmaA, betrag); err != nil {
				return Transaction{}, err
			}
			return Transaction{Type: "transfer", Wallet: wMensch2, To: wFirmaA, Amount: betrag,
				TxHash: fmt.Sprintf("0xbuchtest%v", betrag)}, fehler
		})
	}
	if err := zahle(700, nil); err != nil {
		t.Fatal(err)
	}
	if err := zahle(300, fmt.Errorf("abgebrochen")); err == nil {
		t.Fatal("der zweite Vorgang muss scheitern")
	}
	// Der Stapel-Pfad schreibt die Buchfuehrung einmal je Stapel.
	var stapel []*transferBatchRequest
	for i := 0; i < 3; i++ {
		stapel = append(stapel, &transferBatchRequest{from: wMensch2, to: wFirmaA, amount: 100,
			pendingTxTemplate: Transaction{Type: "transfer", TxHash: fmt.Sprintf("0xbuchstapel%d", i)},
			result:            make(chan transferBatchResult, 1)})
	}
	cs.processTransferBatch(stapel)
	for _, r := range stapel {
		if res := <-r.result; res.err != nil {
			t.Fatal(res.err)
		}
	}
	cs.setzeBuchSeit(1_700_000_000)

	neu := testKnoten(t, "unused-wirtschaft-db-test.json")
	w := neu.wirt()
	w.mu.Lock()
	defer w.mu.Unlock()
	if k := w.buch[wMensch2]; k == nil || !fast(k.Gezaehlt[wFirmaA], 1000) {
		t.Fatalf("Zaehler nach Neustart 700 + 3 x 100 (der Abbruch zaehlt nicht), bekommen %+v", k)
	}
	if k := w.buch[wFirmaA]; k == nil || len(k.Tage) != 1 || !fast(k.Tage[0].Mensch, 1000) {
		t.Fatalf("Umsatz nach Neustart 1.000, bekommen %+v", k)
	}
	if w.buchSeit != 1_700_000_000 {
		t.Fatalf("buchSeit nach Neustart, bekommen %d", w.buchSeit)
	}
	if w.unternehmen[wFirmaA] == nil {
		t.Fatal("Register nach Neustart leer")
	}
}
