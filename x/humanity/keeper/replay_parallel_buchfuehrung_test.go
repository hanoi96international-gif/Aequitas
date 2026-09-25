package keeper

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"testing"
)

// Ab der Aktivierung der Unternehmensregeln fuehrt der parallele
// Nachspiel-Pfad die Buchfuehrung selbst mit (replay_parallel.go, Phase 2b).
// Vorher war er ab dem 1.10.2026 abgeschaltet (block.go), weil er sie nicht
// kannte -- die Kette waere mit den Wirtschaftsregeln langsamer geworden.
//
// Dieser Test beweist die Gleichwertigkeit: derselbe Block, einmal ueber
// replayTransactions (paralleler Pfad), einmal Ueberweisung fuer Ueberweisung
// ueber applyTransferDeltaLockedSammelnd (der serielle Pfad selbst). Kontostaende,
// StateRoot UND jedes Buchkonto muessen bitgleich sein.
//
// Faellt Phase 2b weg, bleibt die Buchfuehrung des parallelen Laufs leer und
// der Vergleich der Buchkonten schlaegt fehl -- der Test haengt also wirklich
// an der Aenderung, nicht nur an den Kontostaenden.

type buchfuehrungsWelt struct {
	menschen, firmen, frei []string
	inhaber               map[string]string // firma -> verantwortlicher Mensch
}

// baueBuchfuehrungsWelt legt dieselben Menschen, Unternehmen und freien
// Adressen in cs an. Deterministisch, damit zwei Staaten identisch starten.
func baueBuchfuehrungsWelt(t *testing.T, cs *ChainState) buchfuehrungsWelt {
	t.Helper()
	w := buchfuehrungsWelt{inhaber: map[string]string{}}
	for i := 0; i < 60; i++ {
		m := fmt.Sprintf("0xa1%038d", i)
		addHuman(cs, m, 2000)
		w.menschen = append(w.menschen, m)
	}
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		f := fmt.Sprintf("0xb2%038d", i)
		inh := w.menschen[i]
		cs.mu.Lock()
		if err := cs.applyUnternehmenEroeffnenLocked(ctx, f, inh, fmt.Sprintf("Firma %d", i), "handel", nowUnix()); err != nil {
			cs.mu.Unlock()
			t.Fatalf("eroeffnen %s: %v", f, err)
		}
		cs.mu.Unlock()
		geben(cs, f, 3000)
		w.firmen = append(w.firmen, f)
		w.inhaber[f] = inh
	}
	for i := 0; i < 10; i++ {
		a := fmt.Sprintf("0xc3%038d", i)
		geben(cs, a, 200)
		w.frei = append(w.frei, a)
	}
	return w
}

// disjunkteGemischteUeberweisungen: jede Adresse hoechstens einmal, alle
// Kombinationen, die die Buchfuehrung unterscheidet -- Mensch->Unternehmen
// (Umsatz, je Mensch gedeckelt), Unternehmen->Mensch (Lohn, Rueckzahlung,
// Entnahme beim Inhaber), Unternehmen->Unternehmen (nur Ueberschuss zaehlt),
// Mensch->Mensch, Mensch->frei. Gebuehr 0, wie im Buendel vorgeschrieben.
func disjunkteGemischteUeberweisungen(rng *rand.Rand, w buchfuehrungsWelt, buchAt int64) []Transaction {
	menschen := append([]string(nil), w.menschen[20:]...) // die ersten 20 sind Inhaber
	rng.Shuffle(len(menschen), func(i, j int) { menschen[i], menschen[j] = menschen[j], menschen[i] })
	firmen := append([]string(nil), w.firmen...)
	rng.Shuffle(len(firmen), func(i, j int) { firmen[i], firmen[j] = firmen[j], firmen[i] })
	nimm := func(l *[]string) string { a := (*l)[0]; *l = (*l)[1:]; return a }
	betrag := func() float64 { return float64(int64((10+rng.Float64()*400)*1e6)) / 1e6 }

	var txs []Transaction
	add := func(from, to string) {
		txs = append(txs, Transaction{Type: "transfer", Wallet: from, To: to, Amount: betrag(), BuchAt: buchAt})
	}
	for i := 0; i < 5; i++ {
		add(nimm(&menschen), nimm(&firmen)) // Einkauf
	}
	for i := 0; i < 4; i++ {
		add(nimm(&firmen), nimm(&menschen)) // Lohn
	}
	for i := 0; i < 3; i++ {
		add(nimm(&firmen), nimm(&firmen)) // zwischen Unternehmen
	}
	// Entnahme: Unternehmen an den eigenen Inhaber
	for _, f := range firmen[:2] {
		add(f, w.inhaber[f])
	}
	firmen = firmen[2:]
	for i := 0; i < 4; i++ {
		add(nimm(&menschen), nimm(&menschen)) // Mensch an Mensch
	}
	for _, a := range w.frei[:3] {
		add(nimm(&menschen), a) // an eine freie Adresse
	}
	rng.Shuffle(len(txs), func(i, j int) { txs[i], txs[j] = txs[j], txs[i] })
	return txs
}

func buchSchnappschuss(t *testing.T, cs *ChainState, adressen []string) map[string]string {
	t.Helper()
	w := cs.wirt()
	w.mu.Lock()
	defer w.mu.Unlock()
	out := map[string]string{}
	for _, a := range adressen {
		if k := w.buch[a]; k != nil {
			b, err := json.Marshal(k)
			if err != nil {
				t.Fatal(err)
			}
			out[a] = string(b)
		}
	}
	return out
}

func TestParallelesNachspielen_FuehrtBuchWieSeriell(t *testing.T) {
	wirtschaftAn(t)
	uhr(t, 1_800_000_000)
	jetzt := nowUnix()

	for lauf := int64(0); lauf < 20; lauf++ {
		rng := rand.New(rand.NewSource(20260925 + lauf))

		// ---- paralleler Pfad: ein Block ueber replayTransactions ----
		dagP, csP := newDeterminismTestDAG()
		weltP := baueBuchfuehrungsWelt(t, csP)
		txs := disjunkteGemischteUeberweisungen(rng, weltP, jetzt)
		if batch, _ := collectDisjointTransferBatch(txs, 0); len(batch) != len(txs) {
			t.Fatalf("lauf %d: nur %d von %d Ueberweisungen buendelbar -- der Test prueft dann nicht den parallelen Pfad", lauf, len(batch), len(txs))
		}
		block := &Block{Height: 1, Hash: fmt.Sprintf("0xbuch-par-%d", lauf), Timestamp: jetzt, Transactions: txs}
		if ok := dagP.replayTransactions(block, true); !ok {
			t.Fatalf("lauf %d: replayTransactions lehnte einen gueltigen Block ab", lauf)
		}

		// ---- serieller Pfad: dieselben Ueberweisungen einzeln ----
		_, csS := newDeterminismTestDAG()
		weltS := baueBuchfuehrungsWelt(t, csS)
		for i, tx := range txs {
			ctx := mitBuchZeit(context.Background(), buchZeitBeimNachspielen(tx.BuchAt, jetzt))
			csS.mu.Lock()
			err := csS.applyTransferDeltaLockedSammelnd(ctx, tx.Wallet, tx.To, tx.Amount, 0, 0, jetzt, nil, 0)
			csS.mu.Unlock()
			if err != nil {
				t.Fatalf("lauf %d, tx %d seriell: %v", lauf, i, err)
			}
		}

		alle := append(append(append([]string(nil), weltP.menschen...), weltP.firmen...), weltP.frei...)
		sort.Strings(alle)
		_ = weltS

		for _, a := range alle {
			if p, s := stand(csP, a), stand(csS, a); p != s {
				t.Fatalf("lauf %d: Kontostand %s parallel %.6f, seriell %.6f", lauf, a, p, s)
			}
		}
		if p, s := csP.StateRoot(), csS.StateRoot(); p != s {
			t.Fatalf("lauf %d: StateRoot weicht ab -- das waere eine Spaltung des Netzes\n  parallel: %s\n  seriell:  %s", lauf, p, s)
		}
		bp, bs := buchSchnappschuss(t, csP, alle), buchSchnappschuss(t, csS, alle)
		if len(bs) == 0 {
			t.Fatalf("lauf %d: serielle Buchfuehrung leer -- der Test prueft nichts", lauf)
		}
		for a, want := range bs {
			if got := bp[a]; got != want {
				t.Fatalf("lauf %d: Buchkonto %s weicht ab\n  parallel: %s\n  seriell:  %s", lauf, a, got, want)
			}
		}
		for a := range bp {
			if _, ok := bs[a]; !ok {
				t.Fatalf("lauf %d: Buchkonto %s nur im parallelen Lauf", lauf, a)
			}
		}
		if p, s := umsatzVon(csP, weltP.firmen[0]), umsatzVon(csS, weltP.firmen[0]); p != s {
			t.Fatalf("lauf %d: Umsatz weicht ab: parallel %.6f, seriell %.6f", lauf, p, s)
		}
	}
}
