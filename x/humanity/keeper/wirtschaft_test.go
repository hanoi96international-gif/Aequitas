package keeper

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"
)

// Adressen ohne fuehrende Nullen -- solche gelten als Systemadressen.
const (
	wMensch1 = "0xa100000000000000000000000000000000000001"
	wMensch2 = "0xa100000000000000000000000000000000000002"
	wMensch3 = "0xa100000000000000000000000000000000000003"
	wFirmaA  = "0xb200000000000000000000000000000000000001"
	wFirmaB  = "0xb200000000000000000000000000000000000002"
	wFirmaC  = "0xb200000000000000000000000000000000000003"
	wFirmaD  = "0xb200000000000000000000000000000000000004"
	wFrei    = "0xc300000000000000000000000000000000000001"
)

const tag = int64(86400)

// uhr setzt die Zeit fuer nowUnix() und gibt eine Funktion zum Vorstellen zurueck.
func uhr(t *testing.T, start int64) func(sekunden int64) int64 {
	t.Helper()
	jetztWert := start
	vorher := setzeZeitQuelleFuerTest(func() time.Time { return time.Unix(jetztWert, 0) })
	t.Cleanup(func() { setzeZeitQuelleFuerTest(vorher) })
	return func(s int64) int64 { jetztWert += s; return jetztWert }
}

func wirtschaftsTest(t *testing.T) (*ChainState, context.Context, func(int64) int64) {
	t.Helper()
	wirtschaftAn(t)
	vor := uhr(t, 1_800_000_000)
	cs := newTestState()
	for _, m := range []string{wMensch1, wMensch2, wMensch3} {
		addHuman(cs, m, 1000)
	}
	return cs, context.Background(), vor
}

func geben(cs *ChainState, addr string, betrag float64) {
	if a, ok := cs.accounts.Get(addr); ok {
		a.Balance = a.Balance.Add(NewDecimal(betrag))
		return
	}
	cs.accounts.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(betrag)})
}

func stand(cs *ChainState, addr string) float64 {
	if a, ok := cs.accounts.Get(addr); ok {
		return a.Balance.Float()
	}
	return 0
}

func ueberweise(t *testing.T, cs *ChainState, ctx context.Context, from, to string, betrag float64) float64 {
	t.Helper()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	_, _, gebuehr, err := cs.transferLockedMitGebuehr(ctx, from, to, betrag)
	if err != nil {
		t.Fatalf("%s -> %s %.2f: %v", from, to, betrag, err)
	}
	return gebuehr
}

func eroeffne(t *testing.T, cs *ChainState, ctx context.Context, firma, mensch string) {
	t.Helper()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if err := cs.applyUnternehmenEroeffnenLocked(ctx, firma, mensch, "Cafe Test", "gastronomie", nowUnix()); err != nil {
		t.Fatal(err)
	}
}

func fast(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestWirtschaftSchlaeftVorAktivierung(t *testing.T) {
	cs := newTestState()
	addHuman(cs, wMensch1, 1000)
	jetzt := int64(1_800_000_000)
	if g := cs.gebuehrMitWirtschaft(wMensch1, wMensch2, artMensch, artMensch, 500, 1000, jetzt); g != ueberweisungsGebuehrFuer(500, 1000) {
		t.Fatalf("vor der Aktivierung muss die alte Gebuehr gelten: %v", g)
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if txs, err := cs.umlaufLocked(context.Background(), jetzt); err != nil || txs != nil {
		t.Fatalf("vor der Aktivierung kein Umlauf: %v %v", txs, err)
	}
	if err := cs.applyUnternehmenEroeffnenLocked(context.Background(), wFirmaA, wMensch1, "x", "handel", jetzt); err == nil {
		t.Fatal("vor der Aktivierung darf kein Unternehmen eroeffnet werden")
	}
	if pruefeEmpfaengerWirtschaft(artFrei, 900, 5000, jetzt) != nil {
		t.Fatal("vor der Aktivierung keine Grenze fuer freie Adressen")
	}
}

func TestKontoartenUndEroeffnen(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	if cs.kontoartVon(wMensch1, true) != artMensch || cs.kontoartVon(wFrei, false) != artFrei ||
		cs.kontoartVon(ubiPoolAddr, false) != artSystem {
		t.Fatal("Kontoarten falsch")
	}
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	if cs.kontoartVon(wFirmaA, false) != artUnternehmen {
		t.Fatal("nach dem Eroeffnen muss es ein Unternehmen sein")
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	faelle := []struct {
		name, firma, mensch, kat string
	}{
		{"kein Mensch", wFirmaB, wFrei, "handel"},
		{"Mensch als Firma", wMensch2, wMensch1, "handel"},
		{"schon Firma", wFirmaA, wMensch2, "handel"},
		{"Kategorie", wFirmaB, wMensch1, "casino"},
		{"Systemadresse", ubiPoolAddr, wMensch1, "handel"},
	}
	for _, f := range faelle {
		if err := cs.applyUnternehmenEroeffnenLocked(ctx, f.firma, f.mensch, "n", f.kat, nowUnix()); err == nil {
			t.Fatalf("%s: haette abgelehnt werden muessen", f.name)
		}
	}
	// hoechstens 3 je Mensch
	for _, f := range []string{wFirmaB, wFirmaC} {
		if err := cs.applyUnternehmenEroeffnenLocked(ctx, f, wMensch1, "n", "handel", nowUnix()); err != nil {
			t.Fatal(err)
		}
	}
	if err := cs.applyUnternehmenEroeffnenLocked(ctx, wFirmaD, wMensch1, "n", "handel", nowUnix()); err == nil {
		t.Fatal("ein viertes Unternehmen je Mensch darf nicht gehen")
	}
	// Mitinhaber, Schliessen nur leer
	if err := cs.applyUnternehmenMitinhaberLocked(ctx, wFirmaA, wMensch2, nowUnix()); err != nil {
		t.Fatal(err)
	}
	if err := cs.applyUnternehmenMitinhaberLocked(ctx, wFirmaA, wMensch2, nowUnix()); err == nil {
		t.Fatal("zweimal derselbe Mitinhaber")
	}
	geben(cs, wFirmaA, 5)
	if err := cs.applyUnternehmenSchliessenLocked(ctx, wFirmaA, nowUnix()); err == nil {
		t.Fatal("mit Guthaben darf nicht geschlossen werden")
	}
	acct(cs, wFirmaA).Balance = 0
	if err := cs.applyUnternehmenSchliessenLocked(ctx, wFirmaA, nowUnix()); err != nil {
		t.Fatal(err)
	}
	if cs.wirt().istUnternehmen(wFirmaA) {
		t.Fatal("geschlossen ist kein Unternehmen mehr")
	}
}

func TestGebuehrenFuerMenschenUndUnternehmen(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch3)
	eroeffne(t, cs, ctx, wFirmaB, wMensch3)

	// Die ersten 1.000 AEQ Ausgaben im Monat sind gratis.
	if g := ueberweise(t, cs, ctx, wMensch1, wFirmaA, 800); g != 0 {
		t.Fatalf("800 im Freibetrag: Gebuehr %v", g)
	}
	geben(cs, wMensch1, 1000)
	if g := ueberweise(t, cs, ctx, wMensch1, wFirmaA, 400); !fast(g, 0.2) {
		t.Fatalf("400 davon 200 ueber dem Freibetrag -> 0,2 AEQ, bekommen %v", g)
	}
	// Unternehmen -> Mensch: gratis (Lohn)
	if g := ueberweise(t, cs, ctx, wFirmaA, wMensch2, 300); g != 0 {
		t.Fatalf("Lohn muss gratis sein: %v", g)
	}
	// Unternehmen -> Unternehmen: 0,1 % ohne Aufschlag, auch bei grossem Guthaben
	geben(cs, wFirmaA, 30_000)
	if g := ueberweise(t, cs, ctx, wFirmaA, wFirmaB, 1000); !fast(g, 1) {
		t.Fatalf("B2B 0,1 %% ohne Aufschlag -> 1 AEQ, bekommen %v", g)
	}
}

func TestFreieAdresseHoechstens1000(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	geben(cs, wMensch1, 2000)
	ueberweise(t, cs, ctx, wMensch1, wFrei, 1000)
	cs.mu.Lock()
	_, _, _, err := cs.transferLockedMitGebuehr(ctx, wMensch1, wFrei, 1)
	cs.mu.Unlock()
	if err == nil || !strings.Contains(err.Error(), "free address") {
		t.Fatalf("mehr als 1.000 auf einer freien Adresse muss abgelehnt werden: %v", err)
	}
}

func TestUnternehmenOhneVermoegensgrenze(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	eroeffne(t, cs, ctx, wFirmaB, wMensch2)
	geben(cs, wFirmaB, 60_000)
	ueberweise(t, cs, ctx, wFirmaB, wFirmaA, 40_000)
	if s := stand(cs, wFirmaA); !fast(s, 40_000) {
		t.Fatalf("ein Unternehmen hat keine feste Grenze: %v", s)
	}
	// Ein Mensch schon: bei 3 Menschen liegt sie bei 5.000.
	ueberweise(t, cs, ctx, wFirmaB, wMensch3, 10_000)
	if s := stand(cs, wMensch3); s > 5000+1e-6 {
		t.Fatalf("Menschen behalten ihre Grenze: %v", s)
	}
}

// Die Rechenbeispiele aus Konzept 14.4, als feste Zahlen.
func TestLiegegeldNachUmsatz_Rechenbeispiele(t *testing.T) {
	for _, f := range []struct {
		name                  string
		stand, umsatz, erwart float64
	}{
		{"Cafe", 1_500, 3_000, 0},
		{"Supermarkt mit 2 Monaten Reserve", 80_000, 40_000, 100},
		{"Supermarkt, der hortet", 200_000, 40_000, 1_900},
		{"Konzern mit 1,5 Monaten Reserve", 15_000_000, 10_000_000, 0},
		{"Grosshaendler (Ueberschuss 100.000)", 250_000, 100_000, 500},
		{"Horten ohne Umsatz", 100_000, 0, 1_960},
		{"unter dem Sockel", 1_500, 0, 0},
	} {
		if g := liegegeldFuerStand(f.stand, f.umsatz); !fast(g, f.erwart) {
			t.Errorf("%s: Liegegeld %v, erwartet %v", f.name, g, f.erwart)
		}
	}
}

// Ein neues Unternehmen hat keine Schonfrist: wer Geld in eine frisch
// eroeffnete Firma schiebt, zahlt ab dem ersten Tag Liegegeld. (Frueher
// 30 Tage frei -- jeden Monat eine neue Firma, und Horten waere umsonst.)
func TestNeuesUnternehmenOhneSchonfrist(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	vor(tag)
	if lg := cs.umlaufBetrag(wFirmaA, artUnternehmen, 100_000, nowUnix(), sekundenJeMonat); !fast(lg, 1_960) {
		t.Fatalf("neue Firma ohne Umsatz: 98.000 x 2 %% = 1.960 ab Tag 1, bekommen %v", lg)
	}
}

// Wenige Tage Umsatz werden nicht hochgerechnet: gemittelt wird ueber
// mindestens 30 Tage.
func TestNeuesUnternehmenUmsatzUeberMindestens30Tage(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	vor(tag)
	acct(cs, wMensch2).Balance = NewDecimal(4000)
	ueberweise(t, cs, ctx, wMensch2, wFirmaA, 3_000)
	vor(4 * tag)
	if u := umsatzVon(cs, wFirmaA); !fast(u, 3_000) {
		t.Fatalf("3.000 in 5 Tagen: Monatsumsatz 3.000 (nicht 18.000), bekommen %v", u)
	}
}

// Die Schonfrist gibt es nur, wenn DIESEM KNOTEN Daten fehlen: in den ersten
// 30 Tagen nach der Aktivierung.
func TestLiegegeldErstNach30TagenKnotenDaten(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	wirtschaftAktivOverride.Store(nowUnix())
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	vor(29 * tag)
	if lg := cs.umlaufBetrag(wFirmaA, artUnternehmen, 100_000, nowUnix(), sekundenJeMonat); lg != 0 {
		t.Fatalf("Knoten hat erst 29 Tage Daten: kein Liegegeld, bekommen %v", lg)
	}
	vor(2 * tag)
	if lg := cs.umlaufBetrag(wFirmaA, artUnternehmen, 100_000, nowUnix(), sekundenJeMonat); !fast(lg, 1_960) {
		t.Fatalf("ohne Umsatz: 98.000 x 2 %% = 1.960, bekommen %v", lg)
	}
}

func umsatzVon(cs *ChainState, firma string) float64 {
	w := cs.wirt()
	w.mu.Lock()
	defer w.mu.Unlock()
	jetzt := nowUnix()
	tage := w.mittelTageLocked(w.unternehmen[firma], jetzt)
	return w.monatsUmsatzLocked(w.kontoLocked(firma, jetzt), tage, jetzt)
}

// Einkaeufe von Menschen zaehlen je Mensch hoechstens 9.000 AEQ im Quartal,
// Zahlungen des eigenen Verantwortlichen gar nicht.
func TestUmsatzMenschenGedeckeltEigeneZaehlenNicht(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	vor(tag)
	for _, m := range []string{wMensch1, wMensch2, wMensch3} {
		acct(cs, m).Balance = NewDecimal(3000)
		ueberweise(t, cs, ctx, m, wFirmaA, 1_500)
		ueberweise(t, cs, ctx, m, wFirmaA, 1_000)
	}
	vor(30 * tag)
	// Mensch2 und Mensch3 je 2.500, der Verantwortliche Mensch1 nichts.
	if u := umsatzVon(cs, wFirmaA); !fast(u, 5_000*30.0/32) {
		t.Fatalf("Monatsumsatz %v, erwartet 5.000 ueber 32 Kalendertage", u)
	}
}

// Drei Firmen schicken sich Geld im Kreis: der Ueberschuss ist null, der
// Freibetrag steigt nicht. Ein echter Verkauf zaehlt.
func TestUmsatzDreieckGewinntNichts(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	eroeffne(t, cs, ctx, wFirmaB, wMensch2)
	eroeffne(t, cs, ctx, wFirmaC, wMensch3)
	geben(cs, wFirmaA, 50_000)
	geben(cs, wFirmaB, 1_000) // fuer die Gebuehren
	geben(cs, wFirmaC, 1_000)
	vor(tag)
	for i := 0; i < 5; i++ {
		ueberweise(t, cs, ctx, wFirmaA, wFirmaB, 10_000)
		ueberweise(t, cs, ctx, wFirmaB, wFirmaC, 10_000)
		ueberweise(t, cs, ctx, wFirmaC, wFirmaA, 10_000)
	}
	vor(30 * tag)
	for _, f := range []string{wFirmaA, wFirmaB, wFirmaC} {
		if u := umsatzVon(cs, f); u != 0 {
			t.Fatalf("%s: Dreieck hat Umsatz erzeugt: %v", f, u)
		}
	}
	// Ein echter Einkauf: A kauft fuer 6.000 bei C.
	ueberweise(t, cs, ctx, wFirmaA, wFirmaC, 6_000)
	if u := umsatzVon(cs, wFirmaC); u <= 0 {
		t.Fatalf("ein echter Verkauf muss zaehlen: %v", u)
	}
	if u := umsatzVon(cs, wFirmaA); u != 0 {
		t.Fatalf("der Kaeufer gewinnt keinen Umsatz: %v", u)
	}
}

// Firmen mit gemeinsamen Verantwortlichen zaehlen fuereinander nicht.
func TestUmsatzGemeinsameVerantwortlicheZaehlenNicht(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	eroeffne(t, cs, ctx, wFirmaB, wMensch1)
	geben(cs, wFirmaA, 20_000)
	vor(tag)
	ueberweise(t, cs, ctx, wFirmaA, wFirmaB, 10_000)
	vor(30 * tag)
	if u := umsatzVon(cs, wFirmaB); u != 0 {
		t.Fatalf("eigene Firma darf keinen Umsatz liefern: %v", u)
	}
}

// Nach einem Snapshot fehlt die Buchfuehrung davor: 30 Tage kein Liegegeld.
func TestSnapshotSetztBuchfuehrungNeu(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	vor(60 * tag)
	if lg := cs.umlaufBetrag(wFirmaA, artUnternehmen, 100_000, nowUnix(), sekundenJeMonat); lg == 0 {
		t.Fatal("vor dem Snapshot muss Liegegeld anfallen")
	}
	neu := newTestState()
	neu.unternehmenAusSnapshot(cs.unternehmenFuerSnapshot())
	if lg := neu.umlaufBetrag(wFirmaA, artUnternehmen, 100_000, nowUnix(), sekundenJeMonat); lg != 0 {
		t.Fatalf("frisch aus dem Snapshot: ohne Daten kein Liegegeld, bekommen %v", lg)
	}
}

// Ab der Aktivierung: 0,1 %% ohne Aufschlagstufen, auch bei grossem Guthaben.
func TestGebuehrOhneAufschlagstufen(t *testing.T) {
	cs, _, _ := wirtschaftsTest(t)
	if g := cs.gebuehrMitWirtschaft(wMensch1, wMensch2, artMensch, artMensch, 2_000, 25_000, nowUnix()); !fast(g, 1) {
		t.Fatalf("2.000, davon 1.000 frei, 0,1 %% -> 1 AEQ, bekommen %v", g)
	}
	if g := cs.gebuehrMitWirtschaft(wFrei, wMensch2, artFrei, artMensch, 500, 900, nowUnix()); !fast(g, 0.5) {
		t.Fatalf("freie Adresse 0,1 %% -> 0,5 AEQ, bekommen %v", g)
	}
}

// Der Tageslauf: Menschen ueber 5.000, freie Adressen, Unternehmen -- alles
// ins Grundeinkommen, Geldmenge bleibt gleich, Nachspielen ergibt dasselbe.
func TestUmlaufTageslaufUndNachspielen(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	acct(cs, wMensch1).Balance = NewDecimal(8000)
	geben(cs, wFrei, 900)
	summeVorher := stand(cs, wMensch1) + stand(cs, wMensch2) + stand(cs, wMensch3) + stand(cs, wFrei) + stand(cs, ubiPoolAddr)

	vor(tag)
	cs.mu.Lock()
	txs, err := cs.umlaufLocked(ctx, nowUnix())
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	erwartet := map[string]float64{
		wMensch1: round6(3000 * menschUmlaufMonat / 30),
		wFrei:    round6(900 * freiUmlaufMonat / 30),
	}
	if len(txs) != len(erwartet) {
		t.Fatalf("erwartet %d Umlauf-Transaktionen, bekommen %d: %+v", len(erwartet), len(txs), txs)
	}
	for _, tx := range txs {
		if tx.Type != "umlauf" || !fast(tx.Amount, erwartet[tx.Wallet]) {
			t.Fatalf("falscher Umlauf %+v, erwartet %v", tx, erwartet[tx.Wallet])
		}
	}
	summeNachher := stand(cs, wMensch1) + stand(cs, wMensch2) + stand(cs, wMensch3) + stand(cs, wFrei) + stand(cs, ubiPoolAddr)
	if !fast(summeVorher, summeNachher) {
		t.Fatalf("Geldmenge veraendert: %v -> %v", summeVorher, summeNachher)
	}

	// Ein zweiter Knoten spielt die Transaktionen nach und kommt aufs Gleiche.
	nach := newTestState()
	for _, m := range []string{wMensch1, wMensch2, wMensch3} {
		addHuman(nach, m, 1000)
	}
	acct(nach, wMensch1).Balance = NewDecimal(8000)
	geben(nach, wFrei, 900)
	nach.mu.Lock()
	for _, tx := range txs {
		if err := nach.applyUmlaufDeltaLocked(ctx, tx.Wallet, tx.Amount, tx.DistributionAt); err != nil {
			t.Fatal(err)
		}
	}
	nach.mu.Unlock()
	for _, a := range []string{wMensch1, wFrei, ubiPoolAddr} {
		if !fast(stand(cs, a), stand(nach, a)) {
			t.Fatalf("Nachspielen weicht ab bei %s: %v vs %v", a, stand(cs, a), stand(nach, a))
		}
	}
}

func TestAusstiegsAbgabeDreitausendFrei(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	jetzt := nowUnix()
	// Mensch: 3.000 im Monat frei, egal woher, darueber 2 %.
	if a := cs.ausstiegsAbgabe(wMensch2, artMensch, 3000, jetzt); a != 0 {
		t.Fatalf("3.000 frei: %v", a)
	}
	if a := cs.ausstiegsAbgabe(wMensch2, artMensch, 4000, jetzt); !fast(a, 20) {
		t.Fatalf("1.000 ueber dem Freibetrag -> 20, bekommen %v", a)
	}
	// Unternehmen: 2 % auf alles.
	if a := cs.ausstiegsAbgabe(wFirmaA, artUnternehmen, 1000, jetzt); !fast(a, 20) {
		t.Fatalf("Unternehmen 2 %%: %v", a)
	}
	// Lohn aendert den Freibetrag nicht mehr.
	geben(cs, wFirmaA, 10_000)
	ueberweise(t, cs, ctx, wFirmaA, wMensch2, 2000)
	if a := cs.ausstiegsAbgabe(wMensch2, artMensch, 4000, jetzt); !fast(a, 20) {
		t.Fatalf("Lohn zaehlt nicht extra: (4.000 - 3.000) x 2 %% = 20, bekommen %v", a)
	}
	// Getauschtes wird abgezogen.
	if err := cs.nachTausch(ctx, wMensch2, 2500, jetzt); err != nil {
		t.Fatal(err)
	}
	if a := cs.ausstiegsAbgabe(wMensch2, artMensch, 1000, jetzt); !fast(a, 10) {
		t.Fatalf("500 frei uebrig: (1.000 - 500) x 2 %% = 10, bekommen %v", a)
	}
}

// Das Register ist Konsens: ein nachspielender Knoten, der dieselben
// Transaktionen anwendet, behandelt dasselbe Konto als Unternehmen.
func TestRegisterNachspielenGleich(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	nach := newTestState()
	for _, m := range []string{wMensch1, wMensch2, wMensch3} {
		addHuman(nach, m, 1000)
	}
	nach.mu.Lock()
	if err := nach.applyUnternehmenEroeffnenLocked(ctx, wFirmaA, wMensch1, "Cafe Test", "gastronomie", nowUnix()+6); err != nil {
		t.Fatal(err)
	}
	nach.mu.Unlock()
	a, b := cs.unternehmenFuerSnapshot(), nach.unternehmenFuerSnapshot()
	if len(a) != 1 || len(b) != 1 || a[0].Adresse != b[0].Adresse || a[0].Name != b[0].Name ||
		strings.Join(a[0].Verantwortliche, ",") != strings.Join(b[0].Verantwortliche, ",") {
		t.Fatalf("Register weicht ab: %+v vs %+v", a, b)
	}
	// Snapshot-Weg
	neu := newTestState()
	neu.unternehmenAusSnapshot(a)
	if !neu.wirt().istUnternehmen(wFirmaA) {
		t.Fatal("Snapshot muss das Register mitbringen")
	}
}

func TestSchnellePfadeTretenZurueck(t *testing.T) {
	wirtschaftAn(t)
	cs := newTestState()
	if _, _, applied, err := cs.transferConcurrent(wMensch1, wMensch2, 1, Transaction{}); applied || err != nil {
		t.Fatal("nach der Aktivierung muss der schnelle Pfad zuruecktreten")
	}
	if cs.processTransferBatchConcurrent(nil) {
		t.Fatal("nach der Aktivierung muss der Buendler zuruecktreten")
	}
}

// Der echte Swap: die Abgabe geht obendrauf ins Grundeinkommen, steht in der
// Transaktion, und ein nachspielender Knoten kommt aufs Gleiche.
func TestSwapMitAusstiegsAbgabeUndNachspielen(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(100_000), ReserveTUSD: NewDecimal(100_000)}
	acct(cs, wMensch1).Balance = NewDecimal(5000)
	cs.mu.Lock()
	out, _, abgabe, err := cs.swapLockedMitAbgabe(ctx, wMensch1, 4000, true, 0)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if !fast(abgabe, 20) {
		t.Fatalf("4.000 AEQ, davon 3.000 frei -> 20 Abgabe, bekommen %v", abgabe)
	}
	if !fast(stand(cs, wMensch1), 5000-4000-20) || stand(cs, ubiPoolAddr) < 20 {
		t.Fatalf("Konto nach Tausch: %v", stand(cs, wMensch1))
	}
	ubiNachErzeuger := stand(cs, ubiPoolAddr)

	nach := newTestState()
	for _, m := range []string{wMensch1, wMensch2, wMensch3} {
		addHuman(nach, m, 1000)
	}
	nach.pool = &PoolState{ReserveAEQ: NewDecimal(100_000), ReserveTUSD: NewDecimal(100_000)}
	acct(nach, wMensch1).Balance = NewDecimal(5000)
	nach.mu.Lock()
	err = nach.applySwapDeltaLockedMitAbgabe(ctx, wMensch1, 4000, out, true, 0, nowUnix(), abgabe)
	nach.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if !fast(stand(nach, wMensch1), stand(cs, wMensch1)) || !fast(stand(nach, ubiPoolAddr), ubiNachErzeuger) {
		t.Fatalf("Nachspielen weicht ab: Konto %v vs %v, UBI %v vs %v",
			stand(nach, wMensch1), stand(cs, wMensch1), stand(nach, ubiPoolAddr), ubiNachErzeuger)
	}
	// Abgabe bei tUSD -> AEQ ist ungueltig.
	nach.mu.Lock()
	defer nach.mu.Unlock()
	if err := nach.applySwapDeltaLockedMitAbgabe(ctx, wMensch1, 10, 1, false, 0, nowUnix(), 1); err == nil {
		t.Fatal("eine Abgabe beim Einstieg darf es nicht geben")
	}
}

// Eine Unternehmensadresse, die sich spaeter als Mensch registriert, bleibt
// ein Mensch mit Grenze -- sonst waere das ein Mensch ohne Obergrenze.
func TestUnternehmenDasMenschWirdBehaeltGrenze(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	addHuman(cs, wFirmaA, 1000)
	geben(cs, wFirmaB, 20_000)
	eroeffne(t, cs, ctx, wFirmaB, wMensch2)
	ueberweise(t, cs, ctx, wFirmaB, wFirmaA, 15_000)
	if s := stand(cs, wFirmaA); s > 5000+1e-6 {
		t.Fatalf("als Mensch gilt die Grenze: %v", s)
	}
}

// Die Grenzen sind Vielfache des fairen Anteils. Die Umstellung von festen
// Zahlen auf Vielfache darf keinen Wert veraendern (Nachspielen alter Bloecke).
func TestWirtschaft_GrenzenSindVielfacheDesFairenAnteils(t *testing.T) {
	for _, f := range []struct {
		name         string
		wert, faktor float64
		frueherFest  float64
	}{
		{"gebuehrenfreie Ausgaben", menschFreiAusgabenMonat, 1, 1000},
		{"Tausch frei", menschTauschFreiMonat, 3, 3000},
		{"Sparfreibetrag", menschSparFreibetrag, 5, 5000},
		{"freie Adresse", freiGrenze, 1, 1000},
		{"Unternehmenssockel", unternehmenSockel, 2, 2000},
	} {
		if f.wert != f.faktor*registrationGrant || f.wert != f.frueherFest {
			t.Errorf("%s: %v, erwartet %v x fairer Anteil = %v", f.name, f.wert, f.faktor, f.frueherFest)
		}
	}
}
