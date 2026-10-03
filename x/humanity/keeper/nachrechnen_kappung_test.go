package keeper

import "testing"

func kappungPruefe(t *testing.T, k *annahmeKnoten, tx Transaction) int64 {
	t.Helper()
	vorher := erhaltungZaehler("kappung_unter_grenze")
	k.cs.mu.Lock()
	err := k.cs.nachrechnenTxLocked(&tx, nowUnix())
	k.cs.mu.Unlock()
	if err != nil {
		t.Fatalf("im Beobachtungsmodus darf nichts abgelehnt werden: %v", err)
	}
	return erhaltungZaehler("kappung_unter_grenze") - vorher
}

// Gutfall: die ehrliche Kappung des Zustaendigen bleibt beim Nachspielen ohne
// Meldung. Missbrauch: ein Produzent kappt ein Konto weit unter der Grenze
// oder nimmt mehr als den Ueberschuss.
func TestNachrechnenKappung(t *testing.T) {
	stufe2An(t)
	a, c := kappungsKnoten(t, "a"), kappungsKnoten(t, "c")
	if _, _, err := a.cs.TransferAtomic("0xx", "0xy", 2_000, Transaction{Type: "transfer", Wallet: "0xx", To: "0xy", Amount: 2_000, TxHash: "0xk1"}); err != nil {
		t.Fatal(err)
	}
	blk1 := a.block(t, 1)
	if n := a.cs.KappungenAbarbeiten(); n != 1 {
		t.Fatalf("Vorbedingung: %d Kappungen statt 1", n)
	}
	blk2 := a.block(t, 2)
	var ehrlich Transaction
	for _, tx := range blk2.Transactions {
		if tx.Type == "kappung" {
			ehrlich = tx
		}
	}
	if ehrlich.Amount <= 0 {
		t.Fatalf("Vorbedingung: keine Kappung in %+v", blk2.Transactions)
	}
	if !c.dag.replayTransactions(blk1, true) {
		t.Fatal("C lehnte die Gutschrift ab")
	}

	if got := kappungPruefe(t, c, ehrlich); got != 0 {
		t.Fatalf("ehrliche Kappung gemeldet (%d)", got)
	}
	mehr := ehrlich
	mehr.Amount = ehrlich.Amount + 500
	if got := kappungPruefe(t, c, mehr); got != 1 {
		t.Fatalf("Kappung ueber dem Ueberschuss nicht erkannt (%d)", got)
	}
	unterGrenze := Transaction{Type: "kappung", Wallet: "0xz", Amount: 50}
	if got := kappungPruefe(t, c, unterGrenze); got != 1 {
		t.Fatalf("Kappung eines Kontos unter der Grenze nicht erkannt (%d)", got)
	}

	vorher := erhaltungZaehler("kappung_unter_grenze")
	if !c.dag.replayTransactions(blk2, true) {
		t.Fatal("C lehnte die ehrliche Kappung ab")
	}
	if d := erhaltungZaehler("kappung_unter_grenze") - vorher; d != 0 {
		t.Fatalf("ehrliche Kappung beim Nachspielen gemeldet (%d)", d)
	}
}
