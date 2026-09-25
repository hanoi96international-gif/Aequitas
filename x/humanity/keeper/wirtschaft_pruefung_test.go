package keeper

import (
	"context"
	"os"
	"testing"
)

// Einkaeufe zaehlen je Mensch und Unternehmen 9.000 im Quartal; im neuen
// Quartal beginnt der Zaehler neu. Ein grosser Einkauf zaehlt voll.
func TestUmsatzDeckelJeQuartal(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t) // 15.01.2027, erstes Quartal
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	acct(cs, wMensch2).Balance = NewDecimal(20_000)
	ueberweise(t, cs, ctx, wMensch2, wFirmaA, 8_000) // grosser Einkauf: voll
	ueberweise(t, cs, ctx, wMensch2, wFirmaA, 2_000) // davon 1.000
	gezaehlt := func() float64 {
		w := cs.wirt()
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.buch[wMensch2].Gezaehlt[wFirmaA]
	}
	if g := gezaehlt(); !fast(g, 9_000) {
		t.Fatalf("im Quartal hoechstens 9.000, bekommen %v", g)
	}
	vor(80 * tag) // Anfang April: zweites Quartal
	ueberweise(t, cs, ctx, wMensch2, wFirmaA, 1_000)
	if g := gezaehlt(); !fast(g, 1_000) {
		t.Fatalf("neues Quartal, Zaehler neu: 1.000, bekommen %v", g)
	}
	if u := umsatzVon(cs, wFirmaA); !fast(u, 10_000*30.0/81) {
		t.Fatalf("Monatsumsatz: 10.000 ueber 81 Kalendertage, bekommen %v", u)
	}
}

// Umsatz aelter als 90 Tage faellt aus der Rechnung und aus dem Speicher.
func TestUmsatzFensterNeunzigTage(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	acct(cs, wMensch2).Balance = NewDecimal(5_000)
	ueberweise(t, cs, ctx, wMensch2, wFirmaA, 3_000)
	vor(89 * tag)
	if u := umsatzVon(cs, wFirmaA); !fast(u, 3_000*30.0/90) {
		t.Fatalf("nach 89 Tagen noch im Fenster, bekommen %v", u)
	}
	vor(2 * tag)
	if u := umsatzVon(cs, wFirmaA); u != 0 {
		t.Fatalf("nach 91 Tagen aus dem Fenster, bekommen %v", u)
	}
	ueberweise(t, cs, ctx, wMensch2, wFirmaA, 100)
	cs.wirt().mu.Lock()
	n := len(cs.wirt().buch[wFirmaA].Tage)
	cs.wirt().mu.Unlock()
	if n != 1 {
		t.Fatalf("alte Tage muessen verschwinden, noch %d", n)
	}
}

// Ein abgebrochener Vorgang nimmt Buchfuehrung und Register mit zurueck.
func TestRollbackNimmtBuchfuehrungZurueck(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	acct(cs, wMensch2).Balance = NewDecimal(5_000)
	ueberweise(t, cs, ctx, wMensch2, wFirmaA, 500)

	cs.mu.Lock()
	snap := cs.snapshotForRollbackLocked([]string{wMensch2, wFirmaA, wFirmaB}, false, nil)
	cs.mu.Unlock()
	ueberweise(t, cs, ctx, wMensch2, wFirmaA, 700)
	eroeffne(t, cs, ctx, wFirmaB, wMensch2)
	cs.mu.Lock()
	if err := cs.restoreFromRollbackLocked(snap); err != nil {
		cs.mu.Unlock()
		t.Fatal(err)
	}
	cs.mu.Unlock()

	w := cs.wirt()
	w.mu.Lock()
	defer w.mu.Unlock()
	if g := w.buch[wMensch2].Gezaehlt[wFirmaA]; !fast(g, 500) {
		t.Fatalf("Zaehler zurueck auf 500, ist %v", g)
	}
	if m := w.buch[wFirmaA].Tage[0].Mensch; !fast(m, 500) {
		t.Fatalf("Umsatz zurueck auf 500, ist %v", m)
	}
	if w.unternehmen[wFirmaB] != nil {
		t.Fatal("die zurueckgenommene Eroeffnung steht noch im Register")
	}
}

// zweiKnoten: Erzeuger und Nachspieler mit derselben Geschichte.
func zweiKnoten(t *testing.T) (*ChainState, *ChainState, context.Context, func(int64) int64) {
	cs, ctx, vor := wirtschaftsTest(t)
	nach := newTestState()
	for _, m := range []string{wMensch1, wMensch2, wMensch3} {
		addHuman(nach, m, 1000)
	}
	for _, k := range []*ChainState{cs, nach} {
		eroeffne(t, k, ctx, wFirmaA, wMensch1)
		acct(k, wMensch1).Balance = NewDecimal(8_000)
		acct(k, wMensch2).Balance = NewDecimal(6_000)
		geben(k, wFirmaA, 100_000)
	}
	vor(tag)
	for _, k := range []*ChainState{cs, nach} {
		ueberweise(t, k, ctx, wMensch2, wFirmaA, 5_000)
	}
	return cs, nach, ctx, vor
}

// Wer nachspielt, rechnet das Liegegeld nach. Stimmt es, gibt es keine
// Abweichung; ein verfaelschter Betrag wird gezaehlt und im strengen Modus
// abgelehnt.
func TestLiegegeldPruefungBeimNachspielen(t *testing.T) {
	os.Unsetenv("AEQUITAS_LIEGEGELD_PRUEFUNG")
	cs, nach, ctx, vor := zweiKnoten(t)
	lauf := func(faktor float64) (int64, error) {
		vor(tag)
		cs.mu.Lock()
		txs, err := cs.umlaufLocked(ctx, nowUnix())
		cs.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		if len(txs) != 2 {
			t.Fatalf("erwartet Umlauf fuer Mensch1 und FirmaA, bekommen %+v", txs)
		}
		vorher := liegegeldAbweichungen.Load()
		geprueft := liegegeldGeprueft.Load()
		nach.mu.Lock()
		defer nach.mu.Unlock()
		for _, tx := range txs {
			betrag := tx.Amount
			if tx.Wallet == wFirmaA {
				betrag *= faktor
			}
			if err := nach.pruefeUmlaufLocked(tx.Wallet, betrag, tx.DistributionAt); err != nil {
				return liegegeldAbweichungen.Load() - vorher, err
			}
			if err := nach.applyUmlaufDeltaLocked(ctx, tx.Wallet, betrag, tx.DistributionAt); err != nil {
				t.Fatal(err)
			}
		}
		if liegegeldGeprueft.Load()-geprueft != 2 {
			t.Fatalf("beide Betraege muessen geprueft sein")
		}
		return liegegeldAbweichungen.Load() - vorher, nil
	}

	if n, err := lauf(1); n != 0 || err != nil {
		t.Fatalf("ehrlicher Erzeuger: %d Abweichungen, %v", n, err)
	}
	if n, err := lauf(2); n != 1 || err != nil {
		t.Fatalf("beobachten: eine Abweichung zaehlen, nicht ablehnen: %d, %v", n, err)
	}
	t.Setenv("AEQUITAS_LIEGEGELD_PRUEFUNG", "streng")
	if n, err := lauf(2); n != 1 || err == nil {
		t.Fatalf("streng: ablehnen: %d, %v", n, err)
	}
}

// Fehlt dem Nachspieler die Buchfuehrung (frisch aus einem Snapshot), prueft
// er Unternehmen nicht -- er wuerde sonst ehrliche Bloecke verwerfen.
func TestLiegegeldPruefungNurMitVollemFenster(t *testing.T) {
	cs, nach, ctx, vor := zweiKnoten(t)
	nach.setzeBuchSeit(nowUnix())
	vor(tag)
	cs.mu.Lock()
	txs, err := cs.umlaufLocked(ctx, nowUnix())
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AEQUITAS_LIEGEGELD_PRUEFUNG", "streng")
	ueber := liegegeldUebersprungen.Load()
	nach.mu.Lock()
	defer nach.mu.Unlock()
	for _, tx := range txs {
		if err := nach.pruefeUmlaufLocked(tx.Wallet, tx.Amount, tx.DistributionAt); err != nil {
			t.Fatalf("%s: %v", tx.Wallet, err)
		}
	}
	if liegegeldUebersprungen.Load()-ueber != 1 {
		t.Fatal("das Unternehmen muss uebersprungen werden")
	}
}

// Freunde kaufen ein und bekommen das Geld zurueck: das hebt sich auf. Auch
// ueber einen Quartalswechsel hinweg.
func TestRueckzahlungHebtEinkaufAuf(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t) // 15.01.2027
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	acct(cs, wMensch2).Balance = NewDecimal(10_000)
	ueberweise(t, cs, ctx, wMensch2, wFirmaA, 5_000)
	ueberweise(t, cs, ctx, wFirmaA, wMensch2, 4_000)
	if u := umsatzVon(cs, wFirmaA); !fast(u, 1_000) {
		t.Fatalf("5.000 gekauft, 4.000 zurueck: 1.000 zaehlen, bekommen %v", u)
	}
	// Ende Maerz kaufen, Anfang April zurueck.
	vor(73 * tag) // 29.03.
	ueberweise(t, cs, ctx, wMensch2, wFirmaA, 3_000)
	vor(4 * tag) // 02.04., neues Quartal
	ueberweise(t, cs, ctx, wFirmaA, wMensch2, 3_000)
	if u := umsatzVon(cs, wFirmaA); !fast(u, 1_000*30.0/78) {
		t.Fatalf("die Rueckzahlung im neuen Quartal hebt den Einkauf auf: bekommen %v", u)
	}
	// Ein Mensch, der nie gekauft hat, bekommt Lohn: nichts wird abgezogen.
	ueberweise(t, cs, ctx, wFirmaA, wMensch3, 500)
	if u := umsatzVon(cs, wFirmaA); !fast(u, 1_000*30.0/78) {
		t.Fatalf("Lohn an Nicht-Kunden aendert den Umsatz nicht: %v", u)
	}
}

// Der Erzeuger schreibt den Buchungsaugenblick in die Transaktion; wer
// nachspielt, nimmt ihn, wenn er plausibel ist.
func TestBuchZeitBeimNachspielen(t *testing.T) {
	wirtschaftAn(t)
	block := int64(1_800_000_000)
	for _, f := range []struct {
		buchAt, erwartet int64
	}{
		{0, block},
		{block - 5, block - 5},
		{block + 30, block + 30},
		{block + 120, block},
		{block - 8*86400, block},
	} {
		if g := buchZeitBeimNachspielen(f.buchAt, block); g != f.erwartet {
			t.Errorf("BuchAt %d: %d, erwartet %d", f.buchAt, g, f.erwartet)
		}
	}
	cs, _, _ := wirtschaftsTest(t)
	tx := Transaction{Type: "transfer"}
	if _, _, err := cs.transferAtomicDirect(wMensch1, wMensch2, 10, tx); err != nil {
		t.Fatal(err)
	}
	// transferAtomicDirect gibt die Transaktion nicht heraus; buchStempel
	// ist das, was dort eingetragen wird.
	if buchStempel(nowUnix()) != nowUnix() {
		t.Fatal("nach der Aktivierung muss BuchAt gesetzt werden")
	}
	wirtschaftAktivOverride.Store(nowUnix() + 1)
	if buchStempel(nowUnix()) != 0 {
		t.Fatal("vor der Aktivierung bleibt BuchAt leer")
	}
}

// Nach der Aktivierung spielt jede Ueberweisung seriell nach -- mit
// Buchfuehrung. Der parallele Pfad kannte keine und nahm gebuehrenfreie
// Ueberweisungen (die ersten 1.000 AEQ eines Menschen).
func TestNachspielenFuehrtBuchNachAktivierung(t *testing.T) {
	wirtschaftAn(t)
	uhr(t, 1_800_000_000)
	dag, cs := newDeterminismTestDAG()
	ctx := context.Background()
	addHuman(cs, wMensch1, 1000)
	addHuman(cs, wMensch2, 1000)
	addHuman(cs, wMensch3, 1000)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	jetzt := nowUnix()
	block := &Block{Height: 1, Hash: "wirtschaft-nachspielen", Timestamp: jetzt, Transactions: []Transaction{
		{Type: "transfer", Wallet: wMensch2, To: wFirmaA, Amount: 400, BuchAt: jetzt - 10},
		{Type: "transfer", Wallet: wMensch3, To: wFirmaA, Amount: 300, BuchAt: jetzt - 5},
	}}
	if ok := dag.replayTransactions(block, true); !ok {
		t.Fatal("replayTransactions lehnte einen gueltigen Block ab")
	}
	if u := umsatzVon(cs, wFirmaA); !fast(u, 700) {
		t.Fatalf("beide Einkaeufe muessen gebucht sein: Monatsumsatz %v, erwartet 700", u)
	}
}
