package keeper

import (
	"context"
	"errors"
	"math"
	"testing"
)

func wirtschaft2An(t *testing.T) {
	t.Helper()
	vorher := wirtschaft2AktivOverride.Load()
	wirtschaft2AktivOverride.Store(1)
	t.Cleanup(func() { wirtschaft2AktivOverride.Store(vorher) })
}

// wirtschaft2Aus: Regeln wie vor der Aktivierung -- so werden Bloecke vor
// wirtschaft2AktivAbUnix weiter nachgespielt. Ausdruecklich gesetzt statt von
// der Uhrzeit abhaengig: sonst kippt ein Test, sobald der Stichtag vorbei ist.
func wirtschaft2Aus(t *testing.T) {
	t.Helper()
	vorher := wirtschaft2AktivOverride.Load()
	wirtschaft2AktivOverride.Store(math.MaxInt64)
	t.Cleanup(func() { wirtschaft2AktivOverride.Store(vorher) })
}

// ---------------------------------------------------------------- C

// Missbrauch/Fehlerfall: Eine Zahlung, die den Empfaenger ueber die Grenze
// braechte, wird abgelehnt. Absender behaelt alles, Empfaenger verliert nichts,
// nichts wird verteilt.
func TestUeberGrenzeWirdAbgelehntStattGekappt(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t) // 3 Menschen: Phase-0-Grenze 5.000
	wirtschaft2An(t)
	acct(cs, wMensch1).Balance = NewDecimal(9_000)
	acct(cs, wMensch2).Balance = NewDecimal(4_000)
	ubiVorher := stand(cs, ubiPoolAddr)
	cs.mu.Lock()
	_, _, _, err := cs.transferLockedMitGebuehr(ctx, wMensch1, wMensch2, 2_000)
	cs.mu.Unlock()
	if err == nil || !errors.Is(err, ErrZustandLehntAb) {
		t.Fatalf("Zahlung ueber die Grenze muss abgelehnt werden, bekommen %v", err)
	}
	if !fast(stand(cs, wMensch1), 9_000) || !fast(stand(cs, wMensch2), 4_000) || !fast(stand(cs, ubiPoolAddr), ubiVorher) {
		t.Fatalf("nach Ablehnung darf sich nichts bewegen: %v / %v / UBI %v", stand(cs, wMensch1), stand(cs, wMensch2), stand(cs, ubiPoolAddr))
	}
	// Bis genau an die Grenze geht es durch.
	ueberweise(t, cs, ctx, wMensch1, wMensch2, 1_000)
	if !fast(stand(cs, wMensch2), 5_000) {
		t.Fatalf("bis zur Grenze muss es gehen: %v", stand(cs, wMensch2))
	}
}

// Vor der Aktivierung bleibt alles wie bisher: durch, Ueberschuss verteilt.
// (Bloecke vor der Aktivierung werden genau so nachgespielt.)
func TestVorWirtschaft2WirdWieBisherGekappt(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	acct(cs, wMensch1).Balance = NewDecimal(9_000)
	acct(cs, wMensch2).Balance = NewDecimal(4_000)
	ueberweise(t, cs, ctx, wMensch1, wMensch2, 2_000)
	if !fast(stand(cs, wMensch2), 5_000) {
		t.Fatalf("vor der Aktivierung: gekappt auf 5.000, bekommen %v", stand(cs, wMensch2))
	}
}

// LP-Anteile zaehlen mit, genau wie bei der Kappung -- sonst ginge eine
// Zahlung hier durch und wuerde dort doch gekappt.
func TestGrenzpruefungZaehltLPAnteile(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	wirtschaft2An(t)
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(1_000), ReserveTUSD: NewDecimal(1_000), TotalLPShares: NewDecimal(1_000)}
	acct(cs, wMensch1).Balance = NewDecimal(9_000)
	acct(cs, wMensch2).Balance = NewDecimal(3_000)
	acct(cs, wMensch2).LPShares = NewDecimal(500) // Wert 500 AEQ
	cs.mu.Lock()
	lp := cs.lpValueLockedAEQ(acct(cs, wMensch2))
	_, _, _, err := cs.transferLockedMitGebuehr(ctx, wMensch1, wMensch2, 1_800)
	cs.mu.Unlock()
	if lp <= 0 {
		t.Skipf("LP-Wert in diesem Testaufbau 0 -- Pruefung nicht aussagekraeftig")
	}
	if err == nil {
		t.Fatalf("3.000 + %.0f LP + 1.800 > 5.000 muss abgelehnt werden", lp)
	}
}

// Unternehmen haben keine feste Grenze: keine Ablehnung.
func TestUnternehmenWirdNichtAbgelehnt(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	wirtschaft2An(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	acct(cs, wMensch2).Balance = NewDecimal(5_000)
	geben(cs, wFirmaA, 50_000)
	ueberweise(t, cs, ctx, wMensch2, wFirmaA, 4_000)
	if !fast(stand(cs, wFirmaA), 54_000) {
		t.Fatalf("Unternehmen ohne feste Grenze: %v", stand(cs, wFirmaA))
	}
}

// Tausch Stable -> AEQ ueber die Grenze: abgelehnt, tUSD bleibt.
func TestTauschUeberGrenzeAbgelehnt(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	wirtschaft2An(t)
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(100_000), ReserveTUSD: NewDecimal(100_000)}
	acct(cs, wMensch1).Balance = NewDecimal(4_500)
	acct(cs, wMensch1).TUsdBalance = NewDecimal(2_000)
	cs.mu.Lock()
	_, _, _, err := cs.swapLockedMitAbgabe(ctx, wMensch1, 2_000, false, 0)
	cs.mu.Unlock()
	if err == nil {
		t.Fatal("Tausch ueber die Grenze muss abgelehnt werden")
	}
	if !fast(acct(cs, wMensch1).TUsdBalance.Float(), 2_000) || !fast(stand(cs, wMensch1), 4_500) {
		t.Fatalf("nach Ablehnung unveraendert: AEQ %v tUSD %v", stand(cs, wMensch1), acct(cs, wMensch1).TUsdBalance.Float())
	}
}

// ---------------------------------------------------------------- A

func liegegeldMonat(cs *ChainState, firma string, stand float64) float64 {
	return cs.umlaufBetrag(firma, artUnternehmen, stand, nowUnix(), sekundenJeMonat)
}

// Gutfall: Nach dem ersten halben Jahr zahlt das erste Unternehmen nie mehr
// als ein Mensch. Gruenderin 1.000 + Firma 4.000 = 5.000: frei.
func TestErstesUnternehmenNieSchlechterAlsEinMensch(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	vor(200 * tag) // Gruendungsphase vorbei
	if g := liegegeldMonat(cs, wFirmaA, 4_000); !fast(g, 20) {
		t.Fatalf("vor der zweiten Stufe: (4.000 - 2.000) x 1 %% = 20, bekommen %v", g)
	}
	wirtschaft2An(t)
	if g := liegegeldMonat(cs, wFirmaA, 4_000); g != 0 {
		t.Fatalf("erstes Unternehmen wie ein Mensch: 0, bekommen %v", g)
	}
	// 1 Mio. geparkt: nur der Platz bis 25.000 wie ein Mensch, der Rest normal.
	if g := liegegeldMonat(cs, wFirmaA, 1_000_000); !fast(g, 100+9_980-220) {
		t.Fatalf("1 Mio.: wie in der Gruendungsphase, bekommen %v", g)
	}
}

// Missbrauch: Die zweite und dritte Firma derselben Gruenderin bekommen es
// nicht -- sonst verdreifachte sich der Freibetrag.
func TestNurDasErsteUnternehmenWieEinMensch(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	wirtschaft2An(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	vor(tag)
	eroeffne(t, cs, ctx, wFirmaB, wMensch1)
	vor(tag)
	eroeffne(t, cs, ctx, wFirmaC, wMensch1)
	vor(400 * tag)
	if g := liegegeldMonat(cs, wFirmaA, 4_000); g != 0 {
		t.Fatalf("erstes: 0, bekommen %v", g)
	}
	for _, f := range []string{wFirmaB, wFirmaC} {
		if g := liegegeldMonat(cs, f, 4_000); !fast(g, 20) {
			t.Fatalf("%s: normale Regeln 20, bekommen %v", f, g)
		}
	}
	// Wird das erste geschlossen, ist das naechste das erste -- immer nur eins.
	cs.mu.Lock()
	if err := cs.applyUnternehmenSchliessenLocked(ctx, wFirmaA, nowUnix()); err != nil {
		cs.mu.Unlock()
		t.Fatal(err)
	}
	cs.mu.Unlock()
	if g := liegegeldMonat(cs, wFirmaB, 4_000); g != 0 {
		t.Fatalf("B ist jetzt das erste: 0, bekommen %v", g)
	}
	if g := liegegeldMonat(cs, wFirmaC, 4_000); !fast(g, 20) {
		t.Fatalf("C bleibt normal: 20, bekommen %v", g)
	}
}

// Missbrauch: Wer als Mensch schon an der Grenze steht, parkt im ersten
// Unternehmen nichts billiger -- der Platz wird geteilt.
func TestErstesUnternehmenTeiltDenPlatzMitDerGruenderin(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	wirtschaft2An(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	vor(400 * tag)
	acct(cs, wMensch1).Balance = NewDecimal(25_000)
	if g := liegegeldMonat(cs, wFirmaA, 20_000); !fast(g, 180) {
		t.Fatalf("Gruenderin voll: normale 180, bekommen %v", g)
	}
}

// ---------------------------------------------------------------- B

// Gutfall: Wer AEQ an den Lieferanten weitergibt, verliert keinen Umsatz mehr.
// Baeckerei (A) zahlt Muehle (B) 8.000, Muehle zahlt Hof (C) 6.000.
func TestWeitergabeAnLieferantenKostetKeinenUmsatz(t *testing.T) {
	// Gemittelt wird ueber die Kalendertage seit der Eroeffnung (hier 31):
	// verglichen wird darum mit Betrag x 30 / 31.
	faktor := 30.0 / 31.0
	for _, zweite := range []bool{false, true} {
		cs, ctx, vor := wirtschaftsTest(t)
		if zweite {
			wirtschaft2An(t)
		}
		eroeffne(t, cs, ctx, wFirmaA, wMensch1)
		eroeffne(t, cs, ctx, wFirmaB, wMensch2)
		eroeffne(t, cs, ctx, wFirmaC, wMensch3)
		geben(cs, wFirmaA, 10_000)
		vor(tag)
		ueberweise(t, cs, ctx, wFirmaA, wFirmaB, 8_000)
		ueberweise(t, cs, ctx, wFirmaB, wFirmaC, 6_000)
		vor(29 * tag)
		u := umsatzVon(cs, wFirmaB)
		if !zweite && !fast(u, 2_000*faktor) {
			t.Fatalf("vorher: nur Ueberschuss 2.000, bekommen %v", u)
		}
		if zweite && !fast(u, 8_000*faktor) {
			t.Fatalf("zweite Stufe: Eingang 8.000 zaehlt voll, bekommen %v", u)
		}
	}
}

// Missbrauch: Ein Kreis A -> B -> C -> A bringt je Firma hoechstens den
// Deckel je zahlender Firma und Quartal (9.000), egal wie oft er laeuft.
func TestKreisZwischenFirmenIstGedeckelt(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	wirtschaft2An(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	eroeffne(t, cs, ctx, wFirmaB, wMensch2)
	eroeffne(t, cs, ctx, wFirmaC, wMensch3)
	geben(cs, wFirmaA, 50_000)
	geben(cs, wFirmaB, 1_000)
	geben(cs, wFirmaC, 1_000)
	vor(tag)
	for i := 0; i < 5; i++ {
		ueberweise(t, cs, ctx, wFirmaA, wFirmaB, 10_000)
		ueberweise(t, cs, ctx, wFirmaB, wFirmaC, 10_000)
		ueberweise(t, cs, ctx, wFirmaC, wFirmaA, 10_000)
	}
	vor(29 * tag)
	for _, f := range []string{wFirmaA, wFirmaB, wFirmaC} {
		if u := umsatzVon(cs, f); u > menschZaehltJeUntQuartal+1e-6 {
			t.Fatalf("%s: Kreis bringt mehr als den Deckel: %v", f, u)
		}
	}
}

// Missbrauch: Hin und zurueck zwischen zwei Firmen bringt nur einer Seite
// etwas -- die Rueckzahlung hebt den gezaehlten Einkauf auf.
func TestHinUndZurueckZwischenZweiFirmen(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	wirtschaft2An(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	eroeffne(t, cs, ctx, wFirmaB, wMensch2)
	geben(cs, wFirmaA, 10_000)
	geben(cs, wFirmaB, 100) // fuer die Gebuehr
	vor(tag)
	ueberweise(t, cs, ctx, wFirmaA, wFirmaB, 5_000) // B verkauft an A
	ueberweise(t, cs, ctx, wFirmaB, wFirmaA, 5_000) // B zahlt zurueck
	vor(29 * tag)
	ua, ub := umsatzVon(cs, wFirmaA), umsatzVon(cs, wFirmaB)
	if !fast(ub, 0) || ua > 5_000+1e-6 {
		t.Fatalf("hin und zurueck: B 0, A hoechstens 5.000; bekommen A %v, B %v", ua, ub)
	}
}

// Firmen mit gemeinsamen Verantwortlichen zaehlen weiter gar nicht.
func TestEigeneFirmenZaehlenAuchInStufeZweiNicht(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	wirtschaft2An(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	eroeffne(t, cs, ctx, wFirmaB, wMensch1)
	geben(cs, wFirmaA, 10_000)
	vor(tag)
	ueberweise(t, cs, ctx, wFirmaA, wFirmaB, 5_000)
	vor(29 * tag)
	if u := umsatzVon(cs, wFirmaB); u != 0 {
		t.Fatalf("eigene Firma: kein Umsatz, bekommen %v", u)
	}
}

// Konsens: Erzeuger und nachspielender Knoten fuehren B gleich. Der Erzeuger
// bucht ueber die Annahme, der Nachspieler ueber replayTransactions mit
// derselben Buchungszeit.
func TestStufeZweiNachspielenGleichWieErzeuger(t *testing.T) {
	wirtschaftAn(t)
	wirtschaft2An(t)
	uhr(t, 1_800_000_000)
	ctx := context.Background()
	aufbau := func(cs *ChainState) {
		for _, m := range []string{wMensch1, wMensch2, wMensch3} {
			addHuman(cs, m, 1000)
		}
		eroeffne(t, cs, ctx, wFirmaA, wMensch1)
		eroeffne(t, cs, ctx, wFirmaB, wMensch2)
		eroeffne(t, cs, ctx, wFirmaC, wMensch3)
		geben(cs, wFirmaA, 20_000)
	}
	erzeuger := newTestState()
	aufbau(erzeuger)
	dag, nach := newDeterminismTestDAG()
	aufbau(nach)

	jetzt := nowUnix()
	txs := []Transaction{
		{Type: "transfer", Wallet: wFirmaA, To: wFirmaB, Amount: 8_000, BuchAt: jetzt - 20},
		{Type: "transfer", Wallet: wFirmaB, To: wFirmaC, Amount: 6_000, BuchAt: jetzt - 10},
		{Type: "transfer", Wallet: wFirmaB, To: wFirmaA, Amount: 1_000, BuchAt: jetzt - 5},
	}
	for i := range txs {
		erzeuger.mu.Lock()
		_, _, gebuehr, err := erzeuger.transferLockedMitGebuehr(mitBuchZeit(ctx, txs[i].BuchAt), txs[i].Wallet, txs[i].To, txs[i].Amount)
		erzeuger.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		txs[i].Gebuehr = gebuehr // wie der Erzeuger sie in den Block schreibt
	}
	block := &Block{Height: 1, Hash: "wirtschaft2-nachspielen", Timestamp: jetzt, Transactions: txs}
	if ok := dag.replayTransactions(block, true); !ok {
		t.Fatal("replayTransactions lehnte einen gueltigen Block ab")
	}
	for _, f := range []string{wFirmaA, wFirmaB, wFirmaC} {
		if a, b := umsatzVon(erzeuger, f), umsatzVon(nach, f); !fast(a, b) {
			t.Fatalf("%s: Erzeuger %v, Nachspieler %v", f, a, b)
		}
	}
	if u := umsatzVon(nach, wFirmaB); u <= 0 {
		t.Fatalf("B hat einen Eingang von A: Umsatz muss > 0 sein, bekommen %v", u)
	}
}
