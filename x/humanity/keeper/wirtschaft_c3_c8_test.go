package keeper

import (
	"context"
	"math"
	"testing"
)

// ---------------------------------------------------------------- C8

// Missbrauch (C8): Vor dem Stichtag entstand beim Tausch aus manchen Betraegen
// ein Mikro-AEQ aus dem Nichts, weil Einsatz, Pool-Anteil und Gebuehr getrennt
// gerundet wurden. Der Test zeigt erst, dass es den Fehler mit der alten
// Rechnung gibt (sonst beweist der Rest nichts), und dann, dass ab dem
// Stichtag fuer jeden Betrag Abzug = Pool-Anteil + Gebuehr gilt.
func TestSwapTeilung_AltSchoepfteMikro_NeuExakt(t *testing.T) {
	vorher, nachher := swapMikroAbUnix-1, swapMikroAbUnix

	alteSchoepfung := 0
	for m := int64(1); m <= 2_000_000; m++ {
		betrag := float64(m) / 1e6
		einsatz, gebuehr, inPool := swapTeilung(betrag, vorher)
		if inPool.Add(NewDecimal(gebuehr)) > einsatz {
			alteSchoepfung++
		}

		einsatz, gebuehr, inPool = swapTeilung(betrag, nachher)
		if inPool.Add(NewDecimal(gebuehr)) != einsatz {
			t.Fatalf("%.6f AEQ: Einsatz %d Mikro, Pool %d + Gebuehr %d -- nicht exakt",
				betrag, einsatz.Micro(), inPool.Micro(), NewDecimal(gebuehr).Micro())
		}
		if gebuehr < 0 || inPool < 0 {
			t.Fatalf("%.6f AEQ: negative Teile (Gebuehr %v, Pool %v)", betrag, gebuehr, inPool)
		}
	}
	if alteSchoepfung == 0 {
		t.Fatal("die alte Rechnung schoepft in diesem Bereich nichts -- der Test prueft dann nichts")
	}
	t.Logf("alte Rechnung: %d Betraege schoepften je 1 Mikro-AEQ", alteSchoepfung)
}

// Die Gebuehr bleibt 0,1 % (auf ein Mikro genau, abgerundet) -- die Umstellung
// darf den Satz nicht aendern.
func TestSwapTeilung_SatzUnveraendert(t *testing.T) {
	for _, betrag := range []float64{0.000001, 0.0015, 1, 100.0005, 7.5000005, 25000, 9.9e8} {
		_, gebuehr, _ := swapTeilung(betrag, swapMikroAbUnix)
		soll := math.Floor(NewDecimal(betrag).Float()*1e6*swapFeeBps/10000) / 1e6
		if math.Abs(gebuehr-soll) > 1e-12 {
			t.Errorf("%v AEQ: Gebuehr %.6f, erwartet %.6f", betrag, gebuehr, soll)
		}
	}
	// Sehr grosse Betraege laufen nicht ueber.
	_, g, rest := swapTeilung(9e12, swapMikroAbUnix)
	if g <= 0 || rest <= 0 {
		t.Errorf("9e12 AEQ: Gebuehr %v, Pool %v -- Ueberlauf?", g, rest)
	}
}

// Ende zu Ende: ein Tausch ab dem Stichtag auf dem erzeugenden Knoten aendert
// die Geldmenge nicht, fuer genau die Betraege, die vorher schoepften, und ein
// nachspielender Knoten kommt auf dieselben Reserven und Topfstaende.
func TestSwap_AbStichtag_ErhaeltGeldmengeUndStimmtMitNachspielen(t *testing.T) {
	at := swapMikroAbUnix + 3600
	for _, betrag := range []float64{100.0005, 0.0015, 7.5000005, 3.333333} {
		for _, aeqToTusd := range []bool{true, false} {
			neu := func() *ChainState {
				cs := newTestState()
				cs.accounts.Set("0xtrader", &AccountState{Address: "0xtrader", Balance: NewDecimal(1000), TUsdBalance: NewDecimal(1000), IsHuman: true})
				cs.humanCount = 1
				cs.pool = &PoolState{ReserveAEQ: NewDecimal(5000), ReserveTUSD: NewDecimal(5000), TotalLPShares: NewDecimal(5000)}
				return cs
			}
			erzeuger, nachspieler := neu(), neu()
			ctx := mitBuchZeit(context.Background(), at)

			var out, abgabe float64
			assertConserved(t, erzeuger, "Tausch ab Stichtag", func() {
				erzeuger.mu.Lock()
				defer erzeuger.mu.Unlock()
				var err error
				out, _, abgabe, err = erzeuger.swapLockedMitAbgabe(ctx, "0xtrader", betrag, aeqToTusd, 0)
				if err != nil {
					t.Fatalf("%v aeqToTusd=%v: %v", betrag, aeqToTusd, err)
				}
			})

			nachspieler.mu.Lock()
			err := nachspieler.applySwapDeltaLockedMitAbgabe(ctx, "0xtrader", betrag, out, aeqToTusd, 0, at, abgabe)
			nachspieler.mu.Unlock()
			if err != nil {
				t.Fatalf("nachspielen %v aeqToTusd=%v: %v", betrag, aeqToTusd, err)
			}
			if erzeuger.pool.ReserveAEQ != nachspieler.pool.ReserveAEQ || erzeuger.pool.ReserveTUSD != nachspieler.pool.ReserveTUSD {
				t.Errorf("%v aeqToTusd=%v: Reserven weichen ab: Erzeuger %v/%v, Nachspieler %v/%v", betrag, aeqToTusd,
					erzeuger.pool.ReserveAEQ, erzeuger.pool.ReserveTUSD, nachspieler.pool.ReserveAEQ, nachspieler.pool.ReserveTUSD)
			}
			for _, a := range []string{"0xtrader", validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr} {
				e, n := acct(erzeuger, a), acct(nachspieler, a)
				var eb, nb, et, nt Decimal
				if e != nil {
					eb, et = e.Balance, e.TUsdBalance
				}
				if n != nil {
					nb, nt = n.Balance, n.TUsdBalance
				}
				if eb != nb || et != nt {
					t.Errorf("%v aeqToTusd=%v: %s weicht ab: Erzeuger %v/%v, Nachspieler %v/%v", betrag, aeqToTusd, a, eb, et, nb, nt)
				}
			}
		}
	}
}

// Aeltere Bloecke werden mit der alten Rechnung nachgespielt -- sonst aenderte
// die Umstellung rueckwirkend Reserven, die laengst im StateRoot stehen.
func TestSwap_VorStichtag_AlteRechnungBeimNachspielen(t *testing.T) {
	at := swapMikroAbUnix - 3600
	betrag := 100.0005
	einsatz, gebuehr, inPool := swapTeilung(betrag, at)
	if inPool.Add(NewDecimal(gebuehr)) == einsatz {
		t.Fatal("Beispielbetrag zeigt die alte Rundung nicht -- anderen Betrag waehlen")
	}
	cs := newTestState()
	cs.accounts.Set("0xtrader", &AccountState{Address: "0xtrader", Balance: NewDecimal(1000), IsHuman: true})
	cs.humanCount = 1
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(5000), ReserveTUSD: NewDecimal(5000), TotalLPShares: NewDecimal(5000)}
	vorher := cs.pool.ReserveAEQ
	cs.mu.Lock()
	err := cs.applySwapDeltaLockedMitAbgabe(mitBuchZeit(context.Background(), at), "0xtrader", betrag, 99, true, 0, at, 0)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if got := cs.pool.ReserveAEQ.Sub(vorher); got != NewDecimal(betrag-betrag*float64(swapFeeBps)/10000.0) {
		t.Errorf("vor dem Stichtag muss die Reserve wie frueher um NewDecimal(einsatz−gebuehr) wachsen, gewachsen um %v", got)
	}
}

// ---------------------------------------------------------------- C3

// Missbrauch/Rueckfall (C3): Eine Runde ohne jede Umlauf-Transaktion. Der
// Erzeuger schreibt letzterUmlauf fort, ein nachspielender Knoten bisher
// nicht -- uebernahm er danach die Erzeugung, rechnete er zwei Tage statt
// einem ab. Ab jetzt schreibt die Rundenmarke den Zeitpunkt fort.
func TestRundenmarke_SchreibtUmlaufZeitFort(t *testing.T) {
	wirtschaftAn(t)
	tag1 := wirtschaftAktivAbUnix + 86400
	tag2 := tag1 + 86400
	tag3 := tag2 + 86400

	erzeuger, nachspieler := newTestState(), newTestState()
	for _, cs := range []*ChainState{erzeuger, nachspieler} {
		cs.wirt().letzterUmlauf = tag1
	}

	// Tag 2: niemand zahlt Umlauf. Der Erzeuger laeuft die Runde, der
	// Nachspielende sieht nur die Marke.
	erzeuger.mu.Lock()
	if _, err := erzeuger.umlaufLocked(context.Background(), tag2); err != nil {
		t.Fatal(err)
	}
	if err := erzeuger.applyDistributionRoundMarkerDeltaLocked(context.Background(), tag2); err != nil {
		t.Fatal(err)
	}
	erzeuger.mu.Unlock()

	nachspieler.mu.Lock()
	if err := nachspieler.applyDistributionRoundMarkerDeltaLocked(context.Background(), tag2); err != nil {
		t.Fatal(err)
	}
	nachspieler.mu.Unlock()

	if e, n := erzeuger.wirt().letzterUmlauf, nachspieler.wirt().letzterUmlauf; e != tag2 || n != tag2 {
		t.Fatalf("nach Tag 2: Erzeuger %d, Nachspieler %d, erwartet beide %d", e, n, tag2)
	}

	// Tag 3: der Nachspielende uebernimmt. Ein Mensch mit 10.000 AEQ zahlt
	// genau EINEN Tag, nicht zwei.
	nachspieler.accounts.Set("0xreich", &AccountState{Address: "0xreich", Balance: NewDecimal(10000), IsHuman: true})
	nachspieler.mu.Lock()
	txs, err := nachspieler.umlaufLocked(context.Background(), tag3)
	nachspieler.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	einTag := round6((10000 - menschSparFreibetrag) * menschUmlaufMonat * 86400 / sekundenJeMonat)
	if len(txs) != 1 || math.Abs(txs[0].Amount-einTag) > 1e-9 {
		t.Fatalf("Tag 3: erwartet eine Umlauf-Transaktion ueber %.6f (ein Tag), bekommen %+v", einTag, txs)
	}
}

// Die Marke darf den Zeitpunkt nie zuruecksetzen (eine alte, spaet
// zugestellte Marke).
func TestRundenmarke_NieRueckwaerts(t *testing.T) {
	wirtschaftAn(t)
	cs := newTestState()
	jetzt := wirtschaftAktivAbUnix + 10*86400
	cs.wirt().letzterUmlauf = jetzt

	cs.mu.Lock()
	defer cs.mu.Unlock()
	if err := cs.applyDistributionRoundMarkerDeltaLocked(context.Background(), jetzt-86400); err != nil {
		t.Fatal(err)
	}
	if got := cs.wirt().letzterUmlauf; got != jetzt {
		t.Errorf("alte Marke setzte letzterUmlauf zurueck: %d statt %d", got, jetzt)
	}
}

// Missbrauch (Sicherheitspruefung #237, F1): Ein Erzeuger schreibt eine
// Rundenmarke oder einen Umlauf mit einer Zeit weit nach dem Block. Ohne
// Grenze setzte jeder Nachspielende letzterUmlauf dauerhaft in die Zukunft
// (GREATEST beim Speichern), und die Umlaufabgabe fiel fuer immer aus. Ab dem
// Stichtag lehnt das Nachspielen den ganzen Block ab.
func TestNachspielen_RundenzeitAusDerZukunftWirdAbgelehnt(t *testing.T) {
	wirtschaftAn(t)
	const reich = "0xa100000000000000000000000000000000000c31"
	blockZeit := rundenZeitStrengAbUnix + 86400
	ehrlich := blockZeit - 5
	zukunft := blockZeit + 365*86400

	for _, tx := range []Transaction{
		{Type: "distribution_round_marker", DistributionAt: zukunft},
		{Type: "umlauf", Wallet: reich, Amount: 1, DistributionAt: zukunft},
	} {
		dag, cs := nachspielKnoten(t, map[string]float64{reich: 10000})
		cs.wirt().letzterUmlauf = ehrlich - 86400
		b := testBlock(1, tx)
		b.Timestamp = blockZeit
		if dag.replayTransactions(b, true) {
			t.Errorf("%s mit Rundenzeit ein Jahr nach dem Block angenommen", tx.Type)
		}
		if got := cs.wirt().letzterUmlauf; got != ehrlich-86400 {
			t.Errorf("%s: letzterUmlauf auf %d verschoben", tx.Type, got)
		}
		if got := kontoVon(t, cs, reich).Balance.Float(); got != 10000 {
			t.Errorf("%s: Konto veraendert (%.6f)", tx.Type, got)
		}
	}

	// Gegenprobe: die ehrliche Marke kurz vor dem Block geht durch.
	dag, cs := nachspielKnoten(t, map[string]float64{reich: 10000})
	cs.wirt().letzterUmlauf = ehrlich - 86400
	b := testBlock(2, Transaction{Type: "distribution_round_marker", DistributionAt: ehrlich})
	b.Timestamp = blockZeit
	if !dag.replayTransactions(b, true) {
		t.Fatal("ehrliche Rundenmarke abgelehnt")
	}
	if got := cs.wirt().letzterUmlauf; got != ehrlich {
		t.Errorf("ehrliche Marke: letzterUmlauf %d, erwartet %d", got, ehrlich)
	}
}

// Vor dem Stichtag spielen aeltere Bloecke unveraendert nach -- auch mit einer
// Rundenzeit nach dem Block (nur beobachtet), damit sich die Geschichte nicht
// aendert.
func TestRundenZeitNachBlock_ErstAbStichtag(t *testing.T) {
	if rundenZeitNachBlock(rundenZeitStrengAbUnix+1000, rundenZeitStrengAbUnix-1) {
		t.Error("Block vor dem Stichtag wird schon streng geprueft")
	}
	if !rundenZeitNachBlock(rundenZeitStrengAbUnix+61, rundenZeitStrengAbUnix) {
		t.Error("Rundenzeit 61 s nach dem Block ab dem Stichtag nicht erkannt")
	}
	if rundenZeitNachBlock(rundenZeitStrengAbUnix+60, rundenZeitStrengAbUnix) {
		t.Error("eine Minute Uhrenspiel muss erlaubt bleiben")
	}
}

// Vor der Aktivierung (in Tests: ohne wirtschaftAn) tut die Marke nichts am
// Umlauf -- alte Bloecke spielen wie bisher nach.
func TestRundenmarke_NichtVorAktivierung(t *testing.T) {
	frisch := newTestState()
	frisch.mu.Lock()
	defer frisch.mu.Unlock()
	if err := frisch.applyDistributionRoundMarkerDeltaLocked(context.Background(), wirtschaftAktivAbUnix+86400); err != nil {
		t.Fatal(err)
	}
	if got := frisch.wirt().letzterUmlauf; got != 0 {
		t.Errorf("Marke vor der Aktivierung setzte letzterUmlauf auf %d", got)
	}
}
