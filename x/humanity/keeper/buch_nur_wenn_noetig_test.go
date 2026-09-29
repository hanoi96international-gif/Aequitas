package keeper

import "testing"

// Zwischen zwei freien Adressen gibt es nichts zu buchen: kein Buchkonto,
// keine Schreibarbeit. Mensch und Unternehmen buchen weiter wie bisher --
// dieselben Werte, die der Monatsfreibetrag und die Umsatzgrenzen lesen.
func TestBuchfuehrung_FreiAnFreiLegtNichtsAn(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	frei2 := "0xc300000000000000000000000000000000000002"
	geben(cs, wFrei, 100)
	geben(cs, frei2, 0)

	ueberweise(t, cs, ctx, wFrei, frei2, 10)

	w := cs.wirt()
	w.mu.Lock()
	_, a := w.buch[wFrei]
	_, b := w.buch[frei2]
	w.mu.Unlock()
	if a || b {
		t.Fatalf("frei -> frei legte Buchkonten an (Absender %v, Empfaenger %v) -- es gibt nichts zu buchen", a, b)
	}
	if got := stand(cs, frei2); got != 10 {
		t.Fatalf("Empfaenger hat %.6f statt 10 -- die Ueberweisung selbst muss unveraendert laufen", got)
	}
}

func TestBuchfuehrung_MenschBuchtWeiter(t *testing.T) {
	cs, ctx, _ := wirtschaftsTest(t)
	geben(cs, wFrei, 0)

	// Mensch an freie Adresse und Mensch an Mensch: Ausgegeben zaehlt beides.
	ueberweise(t, cs, ctx, wMensch1, wFrei, 30)
	ueberweise(t, cs, ctx, wMensch1, wMensch2, 20)

	w := cs.wirt()
	w.mu.Lock()
	k := w.buch[wMensch1]
	w.mu.Unlock()
	if k == nil || k.Ausgegeben != 50 {
		t.Fatalf("Ausgegeben des Menschen = %+v, erwartet 50 -- der Monatsfreibetrag haengt daran", k)
	}
}

func TestBuchfuehrungNoetig(t *testing.T) {
	for _, f := range []struct {
		von, an kontoart
		noetig  bool
	}{
		{artFrei, artFrei, false},
		{artMensch, artFrei, true},
		{artFrei, artMensch, true},
		{artUnternehmen, artFrei, true},
		{artFrei, artUnternehmen, true},
		{artMensch, artMensch, true},
	} {
		if got := buchfuehrungNoetig(f.von, f.an); got != f.noetig {
			t.Errorf("buchfuehrungNoetig(%d, %d) = %v, erwartet %v", f.von, f.an, got, f.noetig)
		}
	}
}
