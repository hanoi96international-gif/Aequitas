package keeper

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"testing"
)

// Ueberweisungen mit Gebuehr im parallelen Nachspielpfad (02.10.2026).
//
// Bis dahin beendete jede Gebuehr den Lauf (collectDisjointTransferBatch),
// und seit dem 24.09. traegt sie jede Ueberweisung einer freien Adresse,
// jedes Unternehmen an Nicht-Menschen und jeder Mensch ueber seinem
// Monatsfreibetrag. Der parallele Pfad muss sie genau so buchen wie
// applyTransferDeltaLockedSammelnd: Deckung gegen Betrag plus Gebuehr, beides
// vom Absender, die Gebuehr in den Topf des Grundeinkommens (ohne dessen
// Uhr), die Buchfuehrung mit der Gebuehr. Jede Abweichung waere eine
// Spaltung des Netzes.

func TestCollectDisjointTransferBatch_GebuehrBleibtImLauf(t *testing.T) {
	a, b, c := "0xa1"+fmt.Sprintf("%038d", 1), "0xa1"+fmt.Sprintf("%038d", 2), "0xa1"+fmt.Sprintf("%038d", 3)
	mit := []Transaction{
		{Type: "transfer", Wallet: a, To: b, Amount: 10, Gebuehr: 0.01},
		{Type: "transfer", Wallet: b, To: c, Amount: 5, Gebuehr: 0},
		{Type: "transfer", Wallet: c, To: a, Amount: 1, Gebuehr: 0.001},
	}
	if batch, _ := collectDisjointTransferBatch(mit, 0); len(batch) != len(mit) {
		t.Fatalf("Ueberweisungen mit Gebuehr: nur %d von %d im Lauf", len(batch), len(mit))
	}
	// Der Topf des Grundeinkommens darf mitlaufen -- als Empfaenger und als
	// Absender, auch mit Gebuehr (applyTransferBatchParallel fuehrt seinen
	// Stand mit den Gebuehren laufend mit).
	mitTopf := []Transaction{
		{Type: "transfer", Wallet: a, To: ubiPoolAddr, Amount: 1, Gebuehr: 0.001},
		{Type: "transfer", Wallet: ubiPoolAddr, To: b, Amount: 1, Gebuehr: 0.001},
	}
	if batch, _ := collectDisjointTransferBatch(mitTopf, 0); len(batch) != len(mitTopf) {
		t.Fatalf("Ueberweisungen mit dem Topf: nur %d von %d im Lauf", len(batch), len(mitTopf))
	}

	// Was weiter seriell geht: der Lauf endet VOR dieser Ueberweisung.
	for name, schlecht := range map[string]Transaction{
		"Gebuehr NaN":          {Type: "transfer", Wallet: a, To: b, Amount: 1, Gebuehr: math.NaN()},
		"Gebuehr unendlich":    {Type: "transfer", Wallet: a, To: b, Amount: 1, Gebuehr: math.Inf(1)},
		"Gebuehr negativ":      {Type: "transfer", Wallet: a, To: b, Amount: 1, Gebuehr: -0.5},
		"Betrag unendlich":     {Type: "transfer", Wallet: a, To: b, Amount: math.Inf(1)},
		"Demurrage Absender":   {Type: "transfer", Wallet: a, To: b, Amount: 1, Gebuehr: 0.001, FromDemurrageLost: 0.1},
		"Demurrage Empfaenger": {Type: "transfer", Wallet: a, To: b, Amount: 1, ToDemurrageLost: 0.1},
	} {
		txs := []Transaction{mit[0], mit[1], schlecht, mit[2]}
		if batch, _ := collectDisjointTransferBatch(txs, 0); len(batch) != 2 {
			t.Fatalf("%s: Lauf mit %d statt 2 Ueberweisungen -- sie muss seriell gehen", name, len(batch))
		}
		if batch, _ := collectDisjointTransferBatch(txs, 2); len(batch) != 0 {
			t.Fatalf("%s: am Anfang eines Laufs aufgenommen", name)
		}
	}
}

// baueWeltMitTopf: die Buchfuehrungswelt und der Topf des Grundeinkommens,
// wie er in Produktion seit der ersten Gebuehr besteht (100.000 AEQ, damit er
// im Lauf auch senden kann). Fehlt er,
// endet der parallele Lauf vor der ersten Gebuehr (siehe
// TestParallelesNachspielen_TopfFehltGehtSeriell).
func baueWeltMitTopf(t *testing.T, cs *ChainState) buchfuehrungsWelt {
	t.Helper()
	w := baueBuchfuehrungsWelt(t, cs)
	geben(cs, ubiPoolAddr, 100_000)
	return w
}

// gebuehrFuerTest: mal keine, mal die echte Regel (0,1 %), mal ein krummer
// Mikro-Betrag -- der Nachspielende uebernimmt die Gebuehr aus dem Block,
// also muss jeder Wert gleich gebucht werden.
func gebuehrFuerTest(rng *rand.Rand, betrag float64) float64 {
	switch rng.Intn(4) {
	case 0:
		return 0
	case 1:
		return grundGebuehr(betrag)
	case 2:
		return float64(1+rng.Intn(5_000_000)) / 1e6 // bis 5 AEQ
	default:
		return 0.000001
	}
}

func TestParallelesNachspielen_MitGebuehrWieSeriell(t *testing.T) {
	wirtschaftAn(t)
	uhr(t, 1_800_000_000)
	jetzt := nowUnix()

	voll := 0
	for lauf := int64(0); lauf < int64(laeufe(12, 3)); lauf++ {
		rng := rand.New(rand.NewSource(2_10_2026 + lauf))
		knapp := lauf%2 == 1

		dagP, csP := newDeterminismTestDAG()
		weltP := baueWeltMitTopf(t, csP)
		alle := append(append(append([]string(nil), weltP.menschen...), weltP.firmen...), weltP.frei...)
		kreis := make([]string, 0, 25)
		for _, i := range rng.Perm(len(alle))[:24] {
			kreis = append(kreis, alle[i])
		}
		// Der Topf selbst sendet und empfaengt in zwei von drei Laeufen mit:
		// sein Stand waechst im Lauf mit jeder Gebuehr, und genau den muss
		// die Deckung sehen. Im dritten bekommt er nur Gebuehren -- dann
		// darf seine Uhr sich nicht bewegen (wie seriell).
		topfImKreis := lauf%3 != 0
		if topfImKreis {
			kreis = append(kreis, ubiPoolAddr)
		}
		var txs []Transaction
		mitGebuehr, mitTopf := 0, 0
		for len(txs) < 150 {
			von, an := kreis[rng.Intn(len(kreis))], kreis[rng.Intn(len(kreis))]
			if von == an {
				continue
			}
			betrag := float64(int64((1+rng.Float64()*40)*1e6)) / 1e6
			if knapp {
				betrag *= 30 // bis 1.200: manche Konten laufen leer, auch erst durch die Gebuehr
			}
			g := gebuehrFuerTest(rng, betrag)
			if g > 0 {
				mitGebuehr++
			}
			if von == ubiPoolAddr || an == ubiPoolAddr {
				mitTopf++
			}
			txs = append(txs, Transaction{Type: "transfer", Wallet: von, To: an, Amount: betrag, Gebuehr: g, BuchAt: jetzt})
		}
		if mitGebuehr < 50 || (topfImKreis && mitTopf < 2) {
			t.Fatalf("lauf %d: nur %d Ueberweisungen mit Gebuehr, %d mit dem Topf -- der Test prueft zu wenig", lauf, mitGebuehr, mitTopf)
		}
		if batch, _ := collectDisjointTransferBatch(txs, 0); len(batch) != len(txs) {
			t.Fatalf("lauf %d: Lauf endete nach %d von %d", lauf, len(batch), len(txs))
		}

		block := &Block{Height: 1, Hash: fmt.Sprintf("0xgeb-%d", lauf), Timestamp: jetzt, Transactions: txs}
		if ok := dagP.replayTransactions(block, true); !ok {
			t.Fatalf("lauf %d: replayTransactions lehnte den Block ab", lauf)
		}

		_, csS := newDeterminismTestDAG()
		baueWeltMitTopf(t, csS)
		abgelehnt := 0
		for i, tx := range txs {
			ctx := mitBuchZeit(context.Background(), buchZeitBeimNachspielen(tx.BuchAt, jetzt))
			csS.mu.Lock()
			err := csS.applyTransferDeltaLockedSammelnd(ctx, tx.Wallet, tx.To, tx.Amount, 0, 0, jetzt, nil, tx.Gebuehr)
			csS.mu.Unlock()
			if err != nil {
				if !errors.Is(err, ErrZustandLehntAb) {
					t.Fatalf("lauf %d, tx %d seriell: %v", lauf, i, err)
				}
				abgelehnt++
			}
		}
		if abgelehnt == 0 {
			// Alles gedeckt: das Buendel muss den ganzen Block nehmen --
			// sonst prueft der Vergleich unten den seriellen Pfad gegen sich.
			_, csB := newDeterminismTestDAG()
			baueWeltMitTopf(t, csB)
			csB.mu.Lock()
			n, err := csB.applyTransferBatchParallel(context.Background(), txs, jetzt, nil)
			csB.mu.Unlock()
			if err != nil || n != len(txs) {
				t.Fatalf("lauf %d: Buendel nahm %d von %d (err %v)", lauf, n, len(txs), err)
			}
			voll++
		}
		if knapp && abgelehnt == 0 {
			t.Fatalf("lauf %d: keine Ueberweisung scheiterte -- der knappe Lauf prueft das Kuerzen nicht", lauf)
		}

		pruefe := append(append([]string(nil), alle...), ubiPoolAddr)
		sort.Strings(pruefe)
		for _, a := range pruefe {
			if p, s := stand(csP, a), stand(csS, a); p != s {
				t.Fatalf("lauf %d: Kontostand %s parallel %.6f, seriell %.6f", lauf, a, p, s)
			}
			csP.mu.RLock()
			ap, okP := csP.accounts.Get(a)
			csP.mu.RUnlock()
			csS.mu.RLock()
			as, okS := csS.accounts.Get(a)
			csS.mu.RUnlock()
			if okP != okS {
				t.Fatalf("lauf %d: Konto %s parallel vorhanden=%v, seriell=%v", lauf, a, okP, okS)
			}
			if okP && ap.LastActivityAt != as.LastActivityAt {
				t.Fatalf("lauf %d: Demurrage-Uhr %s parallel %d, seriell %d", lauf, a, ap.LastActivityAt, as.LastActivityAt)
			}
		}
		if stand(csS, ubiPoolAddr) == stand(newTestStateFuerVergleich(t), ubiPoolAddr) {
			t.Fatalf("lauf %d: der Topf hat nichts bekommen -- der Test prueft die Gebuehr nicht", lauf)
		}
		if p, s := csP.StateRoot(), csS.StateRoot(); p != s {
			t.Fatalf("lauf %d: StateRoot weicht ab -- das waere eine Spaltung des Netzes\n  parallel: %s\n  seriell:  %s", lauf, p, s)
		}
		bp, bs := buchSchnappschuss(t, csP, alle), buchSchnappschuss(t, csS, alle)
		if len(bs) == 0 {
			t.Fatalf("lauf %d: serielle Buchfuehrung leer", lauf)
		}
		for a, want := range bs {
			if got := bp[a]; got != want {
				t.Fatalf("lauf %d: Buchkonto %s weicht ab\n  parallel: %s\n  seriell:  %s", lauf, a, got, want)
			}
		}
		if len(bp) != len(bs) {
			t.Fatalf("lauf %d: %d Buchkonten parallel, %d seriell", lauf, len(bp), len(bs))
		}
	}
	if voll == 0 {
		t.Fatal("kein Lauf ganz gedeckt -- der parallele Pfad wurde nie mit dem ganzen Block geprueft")
	}
}

// newTestStateFuerVergleich: ein frischer Zustand wie in den Laeufen oben,
// fuer den Stand des Topfs VOR jeder Ueberweisung.
func newTestStateFuerVergleich(t *testing.T) *ChainState {
	t.Helper()
	_, cs := newDeterminismTestDAG()
	baueWeltMitTopf(t, cs)
	return cs
}

// Missbrauch: eine Gebuehr, die erst zusammen mit dem Betrag das Guthaben
// uebersteigt. Der parallele Pfad darf sie nicht durchlassen (er pruefte bis
// heute nur den Betrag) -- das Praefix endet vor ihr, und der serielle Pfad
// lehnt sie deterministisch ab.
func TestParallelesNachspielen_GebuehrUebersteigtGuthaben(t *testing.T) {
	wirtschaftAn(t)
	uhr(t, 1_800_000_000)
	jetzt := nowUnix()

	_, cs := newDeterminismTestDAG()
	w := baueWeltMitTopf(t, cs)
	von, an := w.frei[0], w.frei[1]
	haben := stand(cs, von)
	if haben <= 0 {
		t.Fatalf("Absender ohne Guthaben (%v)", haben)
	}
	txs := []Transaction{
		{Type: "transfer", Wallet: w.menschen[0], To: w.menschen[1], Amount: 1, Gebuehr: 0.001, BuchAt: jetzt},
		{Type: "transfer", Wallet: w.menschen[2], To: w.menschen[3], Amount: 1, Gebuehr: 0.001, BuchAt: jetzt},
		// Betrag allein gedeckt, mit Gebuehr nicht.
		{Type: "transfer", Wallet: von, To: an, Amount: haben - 0.5, Gebuehr: 1, BuchAt: jetzt},
	}
	cs.mu.Lock()
	n, err := cs.applyTransferBatchParallel(context.Background(), txs, jetzt, nil)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("Buendel nahm %d Ueberweisungen -- die ungedeckte (Betrag + Gebuehr) darf nicht parallel laufen", n)
	}
	if got := stand(cs, von); got != haben {
		t.Fatalf("Absender veraendert: %.6f statt %.6f", got, haben)
	}
	cs.mu.Lock()
	err = cs.applyTransferDeltaLockedSammelnd(mitBuchZeit(context.Background(), jetzt), von, an, txs[2].Amount, 0, 0, jetzt, nil, txs[2].Gebuehr)
	cs.mu.Unlock()
	if !errors.Is(err, ErrZustandLehntAb) {
		t.Fatalf("serieller Pfad: erwartet deterministische Ablehnung, bekam %v", err)
	}
}

// Fehlt der Topf (frische Kette vor der ersten Gebuehr), endet der parallele
// Lauf vor der ersten Ueberweisung mit Gebuehr, ohne etwas zu veraendern; der
// serielle Pfad legt den Topf an wie bisher.
func TestParallelesNachspielen_TopfFehltGehtSeriell(t *testing.T) {
	wirtschaftAn(t)
	uhr(t, 1_800_000_000)
	jetzt := nowUnix()

	_, cs := newDeterminismTestDAG()
	w := baueBuchfuehrungsWelt(t, cs)
	if _, ok := cs.accounts.Get(ubiPoolAddr); ok {
		t.Skip("Topf besteht in der Testwelt schon")
	}
	txs := []Transaction{
		{Type: "transfer", Wallet: w.menschen[0], To: w.menschen[1], Amount: 1, BuchAt: jetzt},
		{Type: "transfer", Wallet: w.frei[0], To: w.frei[1], Amount: 1, Gebuehr: 0.001, BuchAt: jetzt},
	}
	vorher := stand(cs, w.frei[0])
	cs.mu.Lock()
	n, err := cs.applyTransferBatchParallel(context.Background(), txs, jetzt, nil)
	cs.mu.Unlock()
	if err != nil || n != 1 {
		t.Fatalf("Buendel nahm %d (err %v) -- erwartet: nur die Ueberweisung ohne Gebuehr", n, err)
	}
	if got := stand(cs, w.frei[0]); got != vorher {
		t.Fatalf("Absender der Gebuehr-Ueberweisung veraendert: %.6f statt %.6f", got, vorher)
	}
	if _, ok := cs.accounts.Get(ubiPoolAddr); ok {
		t.Fatal("paralleler Pfad hat den Topf angelegt -- das macht der serielle")
	}
}

// Der Topf sendet im Lauf mehr, als er VOR dem Lauf hatte -- gedeckt nur durch
// die Gebuehren, die er im selben Lauf davor bekommen hat. Seriell geht das
// durch; der parallele Pfad muss den Stand des Topfs laufend mitfuehren, sonst
// gaebe er den Lauf an dieser Stelle (nur langsamer) an den seriellen ab --
// oder, schlimmer, mit einem zu hohen Stand ab, was seriell scheitert.
func TestParallelesNachspielen_TopfSendetAusGebuehrenImLauf(t *testing.T) {
	wirtschaftAn(t)
	uhr(t, 1_800_000_000)
	jetzt := nowUnix()

	baue := func() (*BlockDAG, *ChainState, buchfuehrungsWelt) {
		dag, cs := newDeterminismTestDAG()
		w := baueBuchfuehrungsWelt(t, cs)
		geben(cs, ubiPoolAddr, 0.5)
		return dag, cs, w
	}
	_, csB, w := baue()
	txs := []Transaction{
		// Zwei Gebuehren von je 1 AEQ: der Topf steht danach bei 2,5.
		{Type: "transfer", Wallet: w.frei[0], To: w.frei[1], Amount: 1, Gebuehr: 1, BuchAt: jetzt},
		{Type: "transfer", Wallet: w.menschen[0], To: ubiPoolAddr, Amount: 0.25, Gebuehr: 1, BuchAt: jetzt},
		// Der Topf zahlt 2,6 (mit Gebuehr 0,1): gedeckt nur durch beides davor.
		{Type: "transfer", Wallet: ubiPoolAddr, To: w.menschen[1], Amount: 2.6, Gebuehr: 0.1, BuchAt: jetzt},
		{Type: "transfer", Wallet: w.menschen[2], To: w.menschen[3], Amount: 1, BuchAt: jetzt},
	}
	csB.mu.Lock()
	n, err := csB.applyTransferBatchParallel(context.Background(), txs, jetzt, nil)
	csB.mu.Unlock()
	if err != nil || n != len(txs) {
		t.Fatalf("Buendel nahm %d von %d (err %v) -- der Stand des Topfs im Lauf fehlt", n, len(txs), err)
	}

	_, csS, _ := baue()
	for i, tx := range txs {
		csS.mu.Lock()
		err := csS.applyTransferDeltaLockedSammelnd(mitBuchZeit(context.Background(), jetzt), tx.Wallet, tx.To, tx.Amount, 0, 0, jetzt, nil, tx.Gebuehr)
		csS.mu.Unlock()
		if err != nil {
			t.Fatalf("tx %d seriell: %v", i, err)
		}
	}
	for _, a := range []string{ubiPoolAddr, w.frei[0], w.frei[1], w.menschen[0], w.menschen[1], w.menschen[2], w.menschen[3]} {
		if p, s := stand(csB, a), stand(csS, a); p != s {
			t.Fatalf("Kontostand %s parallel %.6f, seriell %.6f", a, p, s)
		}
		csB.mu.RLock()
		ap, _ := csB.accounts.Get(a)
		csB.mu.RUnlock()
		csS.mu.RLock()
		as, _ := csS.accounts.Get(a)
		csS.mu.RUnlock()
		if ap.LastActivityAt != as.LastActivityAt {
			t.Fatalf("Demurrage-Uhr %s parallel %d, seriell %d", a, ap.LastActivityAt, as.LastActivityAt)
		}
	}
	if p, s := csB.StateRoot(), csS.StateRoot(); p != s {
		t.Fatalf("StateRoot weicht ab\n  parallel: %s\n  seriell:  %s", p, s)
	}

	// Gegenprobe: ohne die Gebuehren davor reicht der Topf NICHT -- dann muss
	// der parallele Pfad vor ihm aufhoeren (der serielle lehnt ab).
	_, csK, w2 := baue()
	knapp := []Transaction{
		{Type: "transfer", Wallet: w2.menschen[2], To: w2.menschen[3], Amount: 1, BuchAt: jetzt},
		{Type: "transfer", Wallet: ubiPoolAddr, To: w2.menschen[1], Amount: 2.6, Gebuehr: 0.1, BuchAt: jetzt},
	}
	csK.mu.Lock()
	n, err = csK.applyTransferBatchParallel(context.Background(), knapp, jetzt, nil)
	csK.mu.Unlock()
	if err != nil || n != 1 {
		t.Fatalf("ungedeckter Topf: Buendel nahm %d (err %v) statt 1", n, err)
	}
}
