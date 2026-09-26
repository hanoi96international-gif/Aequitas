package keeper

import (
	"os"
	"testing"
)

func TestKontoVerlauf_ZeilenAusBlock(t *testing.T) {
	mensch := map[string]bool{"0xa": true, "0xb": true}
	rel := func(a string) bool { return mensch[a] }
	txs := []Transaction{
		{Type: "transfer", Wallet: "0xA", To: "0xB", Amount: 10, Gebuehr: 0.01, TxHash: "0xH1", BuchAt: 500},
		{Type: "transfer", Wallet: "0xa", To: "0xfrei", Amount: 3},      // Empfaenger ohne Verlauf
		{Type: "transfer", Wallet: "0xtest1", To: "0xtest2", Amount: 1}, // Lasttest: keine Zeile
		{Type: "ubi_distribution", Wallet: "0xb", Amount: 2.5},
		{Type: "distribution_round_marker", Wallet: "0xb"}, // Marker: keine Zeile
		{Type: "umlauf", Wallet: "0xa", Amount: 1.25},
	}
	z := verlaufZeilen(txs, 1000, rel)
	if len(z) != 5 {
		t.Fatalf("%d Zeilen, erwartet 5: %+v", len(z), z)
	}
	if z[0].adresse != "0xa" || z[0].seite != 0 || z[0].gegenpartei != "0xb" || z[0].gebuehr != 0.01 || z[0].zeit != 500 || z[0].txHash != "0xh1" {
		t.Fatalf("Absenderzeile falsch: %+v", z[0])
	}
	if z[1].adresse != "0xb" || z[1].seite != 1 || z[1].gegenpartei != "0xa" || z[1].gebuehr != 0 {
		t.Fatalf("Empfaengerzeile falsch: %+v", z[1])
	}
	if z[2].adresse != "0xa" || z[2].gegenpartei != "0xfrei" || z[3].art != "ubi_distribution" || z[3].zeit != 1000 || z[4].art != "umlauf" {
		t.Fatalf("Zeilen falsch: %+v", z[2:])
	}
	for _, f := range []struct {
		art   string
		seite int
		want  string
	}{{"transfer", 0, "aus"}, {"transfer", 1, "ein"}, {"ubi_distribution", 0, "ein"}, {"umlauf", 0, "aus"}, {"kappung", 0, "aus"}, {"grant_release", 0, "ein"}} {
		if got := verlaufRichtung(f.art, f.seite); got != f.want {
			t.Fatalf("%s/%d: %s, erwartet %s", f.art, f.seite, got, f.want)
		}
	}
}

func TestKontoVerlauf_SchreibenUndLesen_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-verlauf-db-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}
	cs.ensureKontoVerlaufTable()
	if _, err := cs.db.Exec("DELETE FROM chain_konto_verlauf"); err != nil {
		t.Fatal(err)
	}
	a, b := "0x1111111111111111111111111111111111111aaa", "0x2222222222222222222222222222222222222bbb"
	cs.mu.Lock()
	cs.accounts.Set(a, &AccountState{Address: a, IsHuman: true, Balance: NewDecimal(100)})
	cs.accounts.Set(b, &AccountState{Address: b, IsHuman: true})
	cs.mu.Unlock()
	blk := []Transaction{{Type: "transfer", Wallet: a, To: b, Amount: 7, Gebuehr: 0.007, TxHash: "0x01"}}
	for i := 0; i < 2; i++ { // zweimal: idempotent
		if err := cs.schreibeKontoVerlauf(10, 1234, blk); err != nil {
			t.Fatal(err)
		}
	}
	if err := cs.schreibeKontoVerlauf(11, 1300, []Transaction{{Type: "ubi_distribution", Wallet: b, Amount: 1.5}}); err != nil {
		t.Fatal(err)
	}
	vb, err := cs.KontoVerlauf(b, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(vb) != 2 || vb[0].Art != "ubi_distribution" || vb[0].Richtung != "ein" || vb[1].Richtung != "ein" || vb[1].Gegenpartei != a || vb[1].Betrag != 7 {
		t.Fatalf("Verlauf von b falsch: %+v", vb)
	}
	va, _ := cs.KontoVerlauf(a, 0, 10)
	if len(va) != 1 || va[0].Richtung != "aus" || va[0].Gebuehr != 0.007 || va[0].Zeit != 1234 {
		t.Fatalf("Verlauf von a falsch: %+v", va)
	}
	if vorher, _ := cs.KontoVerlauf(b, 11, 10); len(vorher) != 1 || vorher[0].Hoehe != 10 {
		t.Fatalf("Blaettern falsch: %+v", vorher)
	}
}
