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

// Das Alter reist mit dem Geld: Kreise ueber Firmen und Freunde machen es
// nicht jung. Nur 30 Tage bei einem Menschen setzen die Uhr zurueck.
func TestAlterReistMit(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	eroeffne(t, cs, ctx, wFirmaB, wMensch2)
	geben(cs, wFirmaA, 20_000)
	// Einmal buchen, damit die 20.000 ein Alter haben.
	ueberweise(t, cs, ctx, wFirmaA, wFirmaB, 1)

	vor(100 * tag)
	// A -> B -> A: bleibt alt.
	ueberweise(t, cs, ctx, wFirmaA, wFirmaB, 10_000)
	ueberweise(t, cs, ctx, wFirmaB, wFirmaA, 9_900)
	liegegeld := cs.umlaufBetrag(wFirmaA, artUnternehmen, stand(cs, wFirmaA), nowUnix(), sekundenJeMonat)
	// (19.989 - 2.000) x 3 % grob; es darf keinesfalls null sein.
	if liegegeld < 500 {
		t.Fatalf("Kreis zwischen Firmen hat das Geld jung gemacht: Liegegeld %v", liegegeld)
	}

	// A -> Freund -> A am selben Tag: bleibt alt.
	vorher := liegegeld
	ueberweise(t, cs, ctx, wFirmaA, wMensch3, 4000)
	ueberweise(t, cs, ctx, wMensch3, wFirmaA, 3000)
	nachher := cs.umlaufBetrag(wFirmaA, artUnternehmen, stand(cs, wFirmaA), nowUnix(), sekundenJeMonat)
	if nachher < vorher*0.85 {
		t.Fatalf("ueber einen Freund gewaschen: vorher %v, nachher %v", vorher, nachher)
	}

	// Frisches Geld von einem Menschen, das dort 30 Tage lag, ist neu.
	cs2, ctx2, vor2 := wirtschaftsTest(t)
	eroeffne(t, cs2, ctx2, wFirmaA, wMensch1)
	geben(cs2, wMensch2, 1000)
	vor2(40 * tag)
	ueberweise(t, cs2, ctx2, wMensch2, wFirmaA, 1500)
	if lg := cs2.umlaufBetrag(wFirmaA, artUnternehmen, stand(cs2, wFirmaA), nowUnix(), sekundenJeMonat); lg != 0 {
		t.Fatalf("Einkauf mit eigenem Geld ist neu, kein Liegegeld: %v", lg)
	}
}

func TestLiegegeldStufenUndSockel(t *testing.T) {
	cs, ctx, vor := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	geben(cs, wFirmaA, 10_000)
	jetzt := nowUnix()
	if lg := cs.umlaufBetrag(wFirmaA, artUnternehmen, 10_000, jetzt, sekundenJeMonat); lg != 0 {
		t.Fatalf("junges Geld: %v", lg)
	}
	vor(60 * tag)
	if lg := cs.umlaufBetrag(wFirmaA, artUnternehmen, 10_000, nowUnix(), sekundenJeMonat); !fast(lg, 80) {
		t.Fatalf("60 Tage: (10.000 - 2.000) x 1 %% = 80, bekommen %v", lg)
	}
	vor(40 * tag)
	if lg := cs.umlaufBetrag(wFirmaA, artUnternehmen, 10_000, nowUnix(), sekundenJeMonat); !fast(lg, 240) {
		t.Fatalf("100 Tage: 8.000 x 3 %% = 240, bekommen %v", lg)
	}
	if lg := cs.umlaufBetrag(wFirmaA, artUnternehmen, 1_500, nowUnix(), sekundenJeMonat); lg != 0 {
		t.Fatalf("unter dem Sockel: %v", lg)
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

func TestAusstiegsAbgabeUndLohn(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	eroeffne(t, cs, ctx, wFirmaA, wMensch1)
	jetzt := nowUnix()
	// Mensch ohne Lohn: 1.000 frei, darueber 2 %.
	if a := cs.ausstiegsAbgabe(wMensch2, artMensch, 1000, jetzt); a != 0 {
		t.Fatalf("1.000 frei: %v", a)
	}
	if a := cs.ausstiegsAbgabe(wMensch2, artMensch, 1500, jetzt); !fast(a, 10) {
		t.Fatalf("500 ueber dem Freibetrag -> 10, bekommen %v", a)
	}
	// Unternehmen: 2 % auf alles.
	if a := cs.ausstiegsAbgabe(wFirmaA, artUnternehmen, 1000, jetzt); !fast(a, 20) {
		t.Fatalf("Unternehmen 2 %%: %v", a)
	}
	// Lohn an einen Angestellten erhoeht seinen Freibetrag, eine Entnahme
	// an den Verantwortlichen nicht.
	geben(cs, wFirmaA, 10_000)
	ueberweise(t, cs, ctx, wFirmaA, wMensch2, 2000) // Lohn
	ueberweise(t, cs, ctx, wFirmaA, wMensch1, 2000) // Entnahme
	if a := cs.ausstiegsAbgabe(wMensch2, artMensch, 3000, jetzt); a != 0 {
		t.Fatalf("Lohn 2.000 + 1.000 frei: %v", a)
	}
	if a := cs.ausstiegsAbgabe(wMensch1, artMensch, 3000, jetzt); !fast(a, 40) {
		t.Fatalf("Entnahme ist kein Lohn: 2.000 x 2 %% = 40, bekommen %v", a)
	}
	// Lohn zaehlt hoechstens 3.000.
	ueberweise(t, cs, ctx, wFirmaA, wMensch2, 3000)
	if a := cs.ausstiegsAbgabe(wMensch2, artMensch, 5000, jetzt); !fast(a, 20) {
		t.Fatalf("Lohn gedeckelt auf 3.000: (5.000 - 4.000) x 2 %% = 20, bekommen %v", a)
	}
	// Getauschtes wird abgezogen.
	cs.nachTausch(wMensch2, 4000, jetzt)
	if a := cs.ausstiegsAbgabe(wMensch2, artMensch, 100, jetzt); !fast(a, 2) {
		t.Fatalf("Freibetrag verbraucht: %v", a)
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
	acct(cs, wMensch1).Balance = NewDecimal(3000)
	cs.mu.Lock()
	out, _, abgabe, err := cs.swapLockedMitAbgabe(ctx, wMensch1, 2000, true, 0)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if !fast(abgabe, 20) {
		t.Fatalf("2.000 AEQ, davon 1.000 frei -> 20 Abgabe, bekommen %v", abgabe)
	}
	if !fast(stand(cs, wMensch1), 3000-2000-20) || stand(cs, ubiPoolAddr) < 20 {
		t.Fatalf("Konto nach Tausch: %v", stand(cs, wMensch1))
	}
	ubiNachErzeuger := stand(cs, ubiPoolAddr)

	nach := newTestState()
	for _, m := range []string{wMensch1, wMensch2, wMensch3} {
		addHuman(nach, m, 1000)
	}
	nach.pool = &PoolState{ReserveAEQ: NewDecimal(100_000), ReserveTUSD: NewDecimal(100_000)}
	acct(nach, wMensch1).Balance = NewDecimal(3000)
	nach.mu.Lock()
	err = nach.applySwapDeltaLockedMitAbgabe(ctx, wMensch1, 2000, out, true, 0, nowUnix(), abgabe)
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
		{"Tausch frei", menschTauschFreiMonat, 1, 1000},
		{"Lohn tauschfrei", lohnTauschFreiMonat, 3, 3000},
		{"Sparfreibetrag", menschSparFreibetrag, 5, 5000},
		{"freie Adresse", freiGrenze, 1, 1000},
		{"Unternehmenssockel", unternehmenSockel, 2, 2000},
	} {
		if f.wert != f.faktor*registrationGrant || f.wert != f.frueherFest {
			t.Errorf("%s: %v, erwartet %v x fairer Anteil = %v", f.name, f.wert, f.faktor, f.frueherFest)
		}
	}
}
