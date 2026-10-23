package keeper

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

// #410 von Anfang bis Ende: ein Lebensnachweis wird unmittelbar vor der
// Grundeinkommensrunde angenommen. Der Erzeuger (echte Datenbank, echter
// Ausgang) wartet, bis der Nachweis in einem Block steht; erst dann laeuft
// die Runde. Ein Nachspielender mit demselben Anfangszustand kommt im
// strengen Modus zum selben Ergebnis -- ohne Befund, mit denselben
// Guthaben. Gegenprobe: stuende die Runde VOR dem Nachweis (was ohne das
// Warten passieren konnte), lehnte der Nachspielende sie ab.

// lnAusgangAlle: alle noch nicht verblockten Auftraege, in Reihenfolge.
func lnAusgangAlle(t *testing.T, cs *ChainState) []Transaction {
	t.Helper()
	rows, err := cs.db.Query(`SELECT tx_json FROM pending_txs WHERE included_at = 0 ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var txs []Transaction
	for rows.Next() {
		var s string
		var tx Transaction
		if rows.Scan(&s) != nil || json.Unmarshal([]byte(s), &tx) != nil {
			t.Fatal("Ausgang unlesbar")
		}
		txs = append(txs, tx)
	}
	return txs
}

func TestLebensnachweisRunde_ErzeugerUndNachspielenderGleich_RealDB(t *testing.T) {
	f := neuerLnDBFall(t)
	vorherStreng := nachrechnenStrengOverride.Load()
	nachrechnenStrengOverride.Store(1) // strenger Modus: jede Abweichung lehnt ab
	t.Cleanup(func() { nachrechnenStrengOverride.Store(vorherStreng) })

	// Zwei Betreiber ohne Staffel (Pause moeglich), ein Mensch mit frischem
	// Nachweis, f.wallet ohne -- ueberfaellig, bis er gleich nachweist.
	zweiter := distTestAddr(1980)
	registerKonto(t, f.cs, zweiter, true)
	verlaufEintrag(t, f.cs, zweiter, distTestAddr(1981), f.jetzt-30*lnTag)
	mit := distTestAddr(1982)
	f.cs.mu.Lock()
	for _, acc := range []*AccountState{
		{Address: mit, IsHuman: true, Balance: NewDecimal(100), LebensnachweisAm: f.jetzt - 10*lnTag},
		{Address: ubiPoolAddr, Balance: NewDecimal(400)},
	} {
		f.cs.accounts.Set(acc.Address, acc)
		if err := f.cs.saveAccountToDB(acc); err != nil {
			f.cs.mu.Unlock()
			t.Fatal(err)
		}
	}
	f.cs.mu.Unlock()

	// Anfangszustand fuer die Nachspielenden.
	anfang := map[string]AccountState{}
	f.cs.mu.Lock()
	f.cs.accounts.Range(func(a string, acc *AccountState) bool {
		anfang[a] = *acc
		return true
	})
	f.cs.mu.Unlock()
	nachspieler := func() (*BlockDAG, *ChainState) {
		dag, cs := newDeterminismTestDAG()
		var menschen int64
		for a, acc := range anfang {
			kopie := acc
			cs.accounts.Set(a, &kopie)
			if kopie.IsHuman {
				menschen++
			}
		}
		cs.humanCount = menschen
		cs.betreiberOhneDB = func(int64) int { return 2 }
		cs.zulassungOhneDB = func(m string, _ int64) (bool, error) { return m == f.bindung.Mensch, nil }
		return dag, cs
	}

	// 1. Der Nachweis wird angenommen -- angewandt, aber noch in keinem Block.
	issued := nowUnix()
	if w := f.reiche(f.koerper(f.wallet, issued, lebensnachweisNachricht(f.wallet, issued))); w.Code != http.StatusOK {
		t.Fatalf("Nachweis: %d %s", w.Code, w.Body.String())
	}
	nachweisTxs := lnAusgangAlle(t, f.cs)
	if len(nachweisTxs) != 1 || nachweisTxs[0].Type != lebensnachweisTxArt {
		t.Fatalf("Ausgang nach dem Nachweis: %+v", nachweisTxs)
	}

	// 2. Die Runde unmittelbar danach: die Vorpruefung haelt sie an (mit
	// Signal zum Anhalten der Annahme), und auch unter der Sperre rollt sie
	// vollstaendig zurueck.
	if g, offen := f.cs.LebensnachweiseOffenGrund(nowUnix()); g == "" || !offen {
		t.Fatalf("Vorpruefung: %q, offen=%v", g, offen)
	}
	ubiAt := nowUnix()
	if err := f.cs.RunDailyDistributionAtomic(ubiAt); !errors.Is(err, ErrLebensnachweiseOffen) {
		t.Fatalf("Runde trotz offenem Nachweis: %v", err)
	}
	if got := lnAusgangAlle(t, f.cs); len(got) != 1 {
		t.Fatalf("abgebrochene Runde hat %d Auftraege im Ausgang hinterlassen", len(got)-1)
	}
	if b := acct(f.cs, ubiPoolAddr).Balance.Float(); b != 400 {
		t.Fatalf("abgebrochene Runde hat den Topf veraendert: %.6f", b)
	}

	// 3. Block 1 mit dem Nachweis; danach laeuft die Runde.
	if _, err := f.cs.db.Exec(`UPDATE pending_txs SET included_at = 1 WHERE included_at = 0`); err != nil {
		t.Fatal(err)
	}
	ubiAt = nowUnix()
	if err := f.cs.RunDailyDistributionAtomic(ubiAt); err != nil {
		t.Fatalf("Runde nach dem Verblocken: %v", err)
	}
	rundeTxs := lnAusgangAlle(t, f.cs)
	empfaenger := map[string]float64{}
	for _, tx := range rundeTxs {
		if tx.Type == "ubi_distribution" {
			empfaenger[tx.Wallet] = tx.Amount
		}
	}
	// f.wallet hat nachgewiesen: Empfaenger. Die Betreiber ohne Nachweis sind
	// pausiert.
	if len(empfaenger) != 2 || empfaenger[f.wallet] != 200 || empfaenger[mit] != 200 {
		t.Fatalf("Empfaenger der Runde: %v", empfaenger)
	}

	regeln := []string{"ubi_pausiert", "ubi_empfaenger", "ubi_anteil", "ubi_rundenzeit", "ubi_kein_mensch", lnRegelOhne, lnRegelZeit, lnRegelFrueh}
	zaehler := func() map[string]int64 {
		m := map[string]int64{}
		for _, r := range regeln {
			m[r] = erhaltungZaehler(r)
		}
		return m
	}

	// 4. Nachspielen in Blockreihenfolge, strenger Modus: kein Befund,
	// dieselben Guthaben.
	dag, ns := nachspieler()
	vorher := zaehler()
	if !dag.replayTransactions(&Block{Height: 1, Hash: "ln-e2e-1", Timestamp: issued + 2, Transactions: nachweisTxs}, true) {
		t.Fatal("Block mit dem Nachweis abgelehnt")
	}
	if !dag.replayTransactions(&Block{Height: 2, Hash: "ln-e2e-2", Timestamp: ubiAt + 2, Transactions: rundeTxs}, true) {
		t.Fatal("Block mit der Runde abgelehnt")
	}
	nachher := zaehler()
	for _, r := range regeln {
		if nachher[r] != vorher[r] {
			t.Fatalf("Nachspielen meldet %s +%d", r, nachher[r]-vorher[r])
		}
	}
	for _, w := range []string{f.wallet, mit, zweiter, f.bindung.Mensch, ubiPoolAddr} {
		if acct(ns, w).Balance != acct(f.cs, w).Balance {
			t.Fatalf("%s: Nachspielender %.6f, Erzeuger %.6f", w, stand(ns, w), stand(f.cs, w))
		}
	}
	if acct(ns, f.wallet).LebensnachweisAm != issued {
		t.Fatalf("Nachweis beim Nachspielenden nicht angewandt: %d", acct(ns, f.wallet).LebensnachweisAm)
	}

	// Gegenprobe: dieselbe Runde VOR dem Nachweis -- so konnte es ohne das
	// Warten kommen. Der Nachspielende sieht f.wallet zur Rundenzeit als
	// pausiert und lehnt im strengen Modus ab.
	dag2, _ := nachspieler()
	vorher = zaehler()
	angenommen := dag2.replayTransactions(&Block{Height: 1, Hash: "ln-e2e-falsch", Timestamp: ubiAt + 2,
		Transactions: append(append([]Transaction{}, rundeTxs...), nachweisTxs...)}, true)
	nachher = zaehler()
	befunde := nachher["ubi_pausiert"] - vorher["ubi_pausiert"] + nachher["ubi_empfaenger"] - vorher["ubi_empfaenger"] +
		nachher["ubi_anteil"] - vorher["ubi_anteil"]
	if angenommen || befunde == 0 {
		t.Fatalf("Runde vor dem Nachweis: angenommen=%v, Befunde=%d -- die Gegenprobe zeigt den Fehler nicht", angenommen, befunde)
	}
}
