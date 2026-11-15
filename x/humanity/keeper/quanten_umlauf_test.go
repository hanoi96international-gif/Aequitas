package keeper

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"
)

// Anteile am Topf (Entscheidung des Betreibers vom 15.11.2026: Umlaufabgabe
// auch im Topf): die erste Note bekommt 1 Anteil je Mikro-AEQ, jede weitere
// zum Kurs davor; die Werte aller Noten ergeben genau den Topf.
func TestQuantenAnteile_KursUndSumme(t *testing.T) {
	a1, ok := quantenAnteileFuer(100_000_000, 0, 0)
	if !ok || a1 != 100_000_000 {
		t.Fatalf("erste Note: %d %v", a1, ok)
	}
	// Der Topf ist durch die Abgabe auf 90 AEQ gesunken; wer jetzt 90 AEQ
	// einzahlt, bekommt so viele Anteile wie die erste Note: beide sind
	// danach gleich viel wert.
	a2, ok := quantenAnteileFuer(90_000_000, 90_000_000, a1)
	if !ok || a2 != 100_000_000 {
		t.Fatalf("zweite Note: %d %v", a2, ok)
	}
	topf, summe := int64(180_000_000), a1+a2
	w1, w2 := quantenWert(a1, topf, summe), quantenWert(a2, topf, summe)
	if w1 != 90_000_000 || w2 != 90_000_000 || w1+w2 != topf {
		t.Fatalf("Werte %d + %d, Topf %d", w1, w2, topf)
	}
	// Krumme Zahlen: abgerundet, nie mehr als der Topf; die letzte Note
	// bekommt den ganzen Rest.
	if w := quantenWert(1, 10, 3) + quantenWert(2, 10, 3); w > 10 {
		t.Fatalf("Summe %d ueber dem Topf", w)
	}
	if quantenWert(3, 10, 3) != 10 {
		t.Fatal("die letzte Note bekommt nicht den ganzen Topf")
	}
	// Missbrauch / Grenzfaelle: kein Kurs, kein Eintrag.
	for name, f := range map[string][3]int64{
		"nichts eingezahlt":            {0, 100, 100},
		"negativer Eingang":            {-5, 100, 100},
		"Topf leer, Anteile vorhanden": {1_000_000, 0, 5},
		"Ueberlauf der Anteile":        {math.MaxInt64 / 2, 1, quantenAnteileHoechstens - 10},
		"weniger als ein Anteil":       {1, 1_000_000_000, 1},
	} {
		if a, ok := quantenAnteileFuer(f[0], f[1], f[2]); ok {
			t.Errorf("%s: %d Anteile angenommen", name, a)
		}
	}
}

// Der Topf zahlt die Umlaufabgabe wie ein freies Konto mit demselben Stand,
// und sie geht ins Grundeinkommen -- vorher war er als Systemadresse befreit.
func TestQuantenTopf_ZahltUmlaufabgabe(t *testing.T) {
	wirtschaftAn(t)
	cs := newTestState()
	frei := "0x" + strings.Repeat("7", 40) // freies Konto (kein Mensch, kein Unternehmen)
	cs.accounts.Set(frei, &AccountState{Address: frei, Balance: NewDecimal(10000)})
	cs.mu.Lock()
	qgTopfGutschreiben(cs, 10000)
	cs.mu.Unlock()
	cs.ensureAccountLoadedCtx(context.Background(), ubiPoolAddr)
	ubiVorher := cs.GetBalance(ubiPoolAddr)

	at := time.Now().Unix()
	txs := umlaufRunde(t, cs, at)
	var topf, vergleich float64
	for _, tx := range txs {
		switch tx.Wallet {
		case quantenTopfAddr:
			topf = tx.Amount
		case frei:
			vergleich = tx.Amount
		}
	}
	if topf <= 0 {
		t.Fatalf("Topf zahlt keine Umlaufabgabe: %+v", txs)
	}
	if math.Abs(topf-vergleich) > 1e-9 {
		t.Fatalf("Topf zahlt %.6f, ein freies Konto mit demselben Stand %.6f", topf, vergleich)
	}
	if got := cs.GetBalance(quantenTopfAddr); math.Abs(got-(10000-topf)) > 1e-9 {
		t.Fatalf("Topf nach der Runde %.6f, erwartet %.6f", got, 10000-topf)
	}
	if got := cs.GetBalance(ubiPoolAddr) - ubiVorher; math.Abs(got-topf-vergleich) > 1e-9 {
		t.Fatalf("Grundeinkommen +%.6f, erwartet +%.6f", got, topf+vergleich)
	}
}

// Missbrauch: nur der Topf ist von der Befreiung der Systemadressen
// ausgenommen -- ein Umlauf auf einen anderen Topf bleibt unzulaessig.
func TestQuantenTopf_AndereSystemadressenBleibenBefreit(t *testing.T) {
	cs := newTestState()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for _, a := range []string{ubiPoolAddr, lpPoolAddr, treasuryPoolAddr, validatorsPoolAddr} {
		if umlaufZahlt(a) {
			t.Errorf("%s steht in der Umlaufrunde", a)
		}
		if err := cs.applyUmlaufDeltaLocked(context.Background(), a, 1, time.Now().Unix(), nil); err == nil {
			t.Errorf("Umlauf auf %s angenommen", a)
		}
	}
	if !umlaufZahlt(quantenTopfAddr) || cs.umlaufArt(quantenTopfAddr, false) != artFrei {
		t.Fatal("Quantengeld-Topf nicht in der Umlaufrunde")
	}
	if cs.kontoartVon(quantenTopfAddr, false) != artSystem {
		t.Fatal("Kontoart des Topfs ausserhalb der Umlaufrunde veraendert")
	}
}

// Die Anteile einer Anmeldung stehen im Block; jeder Knoten rechnet sie an
// derselben Stelle nach. Falsche oder fehlende Anteile: der Block ist falsch.
// Danach sinken die Noten anteilig mit dem Topf.
func TestQuantenNote_AnteileWerdenNachgerechnet(t *testing.T) {
	f := neuerQgPraegeFall(t)
	k1, k2 := strings.Repeat("c", 64), strings.Repeat("d", 64)
	f.cs.mu.Lock()
	e1 := quantenNoteEintrag{K: k1, Satz: "isogenie-8192", Von: f.wallet, topfVor: f.cs.quantenTopfMikroLocked()}
	qgTopfGutschreiben(f.cs, 100)
	if a, err := f.cs.quantenNoteEintragenLocked(context.Background(), e1, 0); err != nil || a != 100_000_000 {
		f.cs.mu.Unlock()
		t.Fatalf("erste Note: %d %v", a, err)
	}
	e2 := quantenNoteEintrag{K: k2, Satz: "isogenie-8192", Von: f.wallet, topfVor: f.cs.quantenTopfMikroLocked()}
	qgTopfGutschreiben(f.cs, 50)
	for name, erwartet := range map[string]int64{"zu viele Anteile": 50_000_001, "fehlende Anteile": -1, "zu wenige": 49_999_999} {
		if _, err := f.cs.quantenNoteEintragenLocked(context.Background(), e2, erwartet); err == nil {
			f.cs.mu.Unlock()
			t.Fatalf("%s angenommen", name)
		}
	}
	if a, err := f.cs.quantenNoteEintragenLocked(context.Background(), e2, 50_000_000); err != nil || a != 50_000_000 {
		f.cs.mu.Unlock()
		t.Fatalf("zweite Note: %d %v", a, err)
	}
	// Die Tagesrunde nimmt dem Topf 15 AEQ: jede Note verliert 10 %.
	acc, _ := f.cs.accounts.Get(quantenTopfAddr)
	acc.Balance = acc.Balance.Sub(NewDecimal(15))
	f.cs.mu.Unlock()

	w := func(k string) float64 {
		return f.cs.QuantenStand(k)["note"].(map[string]interface{})["deckung_aeq"].(float64)
	}
	if math.Abs(w(k1)-90) > 1e-9 || math.Abs(w(k2)-45) > 1e-9 {
		t.Fatalf("Werte nach der Abgabe: %.6f und %.6f, erwartet 90 und 45", w(k1), w(k2))
	}
	if g := f.cs.QuantenStand("")["gedeckt_aeq"].(float64); math.Abs(g-135) > 1e-9 {
		t.Fatalf("gedeckt %.6f, erwartet 135", g)
	}
}

// Missbrauch ueber einen Snapshot: jede Note einzeln gueltig, zusammen ueber
// der Grenze der Anteile -- das Register wird nicht gelesen.
func TestQuantenRegister_SummeDerAnteileBegrenzt(t *testing.T) {
	satz := quantenSatzEintrag{Name: "isogenie-8192", Ordnung: qgSatzN.String(), Ab: qgAb + 100, Seit: qgAb}
	note := func(k string, a int64) quantenNoteEintrag {
		return quantenNoteEintrag{K: strings.Repeat(k, 64), Satz: "isogenie-8192", Von: qnAnna, Anteile: a}
	}
	gut := &quantenGeldRegister{Saetze: []quantenSatzEintrag{satz}, Noten: []quantenNoteEintrag{note("a", 5), note("b", 7)}}
	if _, err := quantenGeldRegisterLesen(gut.kodiert()); err != nil {
		t.Fatalf("gueltiges Register abgewiesen: %v", err)
	}
	boese := &quantenGeldRegister{Saetze: []quantenSatzEintrag{satz},
		Noten: []quantenNoteEintrag{note("a", quantenAnteileHoechstens), note("b", quantenAnteileHoechstens)}}
	if _, err := quantenGeldRegisterLesen(boese.kodiert()); err == nil {
		t.Fatal("Register mit Summe der Anteile ueber der Grenze gelesen")
	}
}
