package keeper

import (
	"math"
	"strings"
	"testing"
)

// Das Modell rechnet mit denselben Formeln wie die Kette -- bei den heutigen
// Zahlen bis auf die Rundung gleich. Aendert jemand eine Formel in
// wirtschaft.go und nicht im Modell, wird dieser Test rot.
func TestWirtschaftsmodell_FormelnGleichDerKette(t *testing.T) {
	wirtschaftAktivOverride.Store(1)
	wirtschaft2AktivOverride.Store(1)
	t.Cleanup(func() {
		wirtschaftAktivOverride.Store(math.MaxInt64)
		wirtschaft2AktivOverride.Store(0)
	})
	p := StandardParameter()
	jetzt := int64(1_800_000_000)
	nah := func(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

	for _, stand := range []float64{0, 100, 4999, 5000, 5001, 12345.678, 25000, 80000} {
		cs := newTestState()
		if got, want := round6(p.menschUmlauf(stand)), cs.umlaufBetragRoh("0xm", artMensch, stand, jetzt, sekundenJeMonat); !nah(got, want) {
			t.Fatalf("Umlauf Mensch bei %.3f: Modell %.6f, Kette %.6f", stand, got, want)
		}
		if got, want := round6(stand*p.FreiUmlaufMonat), cs.umlaufBetragRoh("0xf", artFrei, stand, jetzt, sekundenJeMonat); stand > 0 && !nah(got, want) {
			t.Fatalf("Umlauf frei bei %.3f: Modell %.6f, Kette %.6f", stand, got, want)
		}
	}
	for _, betrag := range []float64{0, 1, 999, 1000, 1001, 4321.5, 100000} {
		cs := newTestState()
		if got, want := p.gebuehr(math.Max(0, betrag-p.FreiAusgabenMonat)), cs.gebuehrMitWirtschaft("0xm", "0xu", artMensch, artUnternehmen, betrag, 1e6, jetzt); !nah(got, want) {
			t.Fatalf("Gebuehr Mensch %.2f: Modell %.6f, Kette %.6f", betrag, got, want)
		}
		if got, want := p.gebuehr(betrag), cs.gebuehrMitWirtschaft("0xu", "0xv", artUnternehmen, artUnternehmen, betrag, 1e6, jetzt); !nah(got, want) {
			t.Fatalf("Gebuehr Unternehmen %.2f: Modell %.6f, Kette %.6f", betrag, got, want)
		}
		if got := cs.gebuehrMitWirtschaft("0xu", "0xm", artUnternehmen, artMensch, betrag, 1e6, jetzt); got != 0 {
			t.Fatalf("Unternehmen an Mensch kostet in der Kette %.6f, im Modell 0", got)
		}
		if got, want := p.ausstieg(betrag, p.TauschFreiMonat), cs.ausstiegsAbgabe("0xm", artMensch, betrag, jetzt); !nah(got, want) {
			t.Fatalf("Ausstieg Mensch %.2f: Modell %.6f, Kette %.6f", betrag, got, want)
		}
		if got, want := p.ausstieg(betrag, 0), cs.ausstiegsAbgabe("0xu", artUnternehmen, betrag, jetzt); !nah(got, want) {
			t.Fatalf("Ausstieg Unternehmen %.2f: Modell %.6f, Kette %.6f", betrag, got, want)
		}
	}
	if p.grenze() != registrationGrant*wealthCapMultiplier {
		t.Fatalf("Grenze %.0f, Kette %.0f", p.grenze(), registrationGrant*wealthCapMultiplier)
	}

	// Liegegeld: beide Zweige der Kette (wirtschaft.go, liegegeldLocked) --
	// ein zweites Unternehmen normal, das erste "wie ein Mensch".
	w := neueWirtschaft()
	wirtschaftAktivOverride.Store(jetzt - 400*86400)
	gruender := "0x" + strings.Repeat("1", 40)
	erstes, zweites := "0x"+strings.Repeat("a", 40), "0x"+strings.Repeat("b", 40)
	w.unternehmen[erstes] = &unternehmenEintrag{Adresse: erstes, Verantwortliche: []string{gruender}, EroeffnetAm: jetzt - 300*86400}
	w.unternehmen[zweites] = &unternehmenEintrag{Adresse: zweites, Verantwortliche: []string{gruender}, EroeffnetAm: jetzt - 200*86400}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.kontoLocked(zweites, jetzt).tagLocked(jetzt).Mensch = 4000 // etwas Umsatz
	for _, stand := range []float64{0, 1500, 2000, 6000, 30000, 250000} {
		for _, g := range []float64{0, 3000, 24000, 40000} {
			u1 := w.umsatzLocked(erstes, jetzt)
			if got, want := p.liegegeldWieMensch(stand, u1, g), w.liegegeldLocked(erstes, stand, g, jetzt); !nah(got, want) {
				t.Fatalf("Liegegeld erstes Unternehmen (Stand %.0f, Gruenderin %.0f): Modell %.6f, Kette %.6f", stand, g, got, want)
			}
			u2 := w.umsatzLocked(zweites, jetzt)
			if got, want := p.liegegeld(stand, u2), w.liegegeldLocked(zweites, stand, g, jetzt); !nah(got, want) {
				t.Fatalf("Liegegeld zweites Unternehmen (Stand %.0f, Umsatz %.2f): Modell %.6f, Kette %.6f", stand, u2, got, want)
			}
		}
	}
}

// Die Standardzahlen sind die der Kette.
func TestWirtschaftsmodell_StandardSindDieZahlenDerKette(t *testing.T) {
	p := StandardParameter()
	for name, paar := range map[string][2]float64{
		"FreiAusgabenMonat": {p.FreiAusgabenMonat, menschFreiAusgabenMonat},
		"SparFreibetrag":    {p.SparFreibetrag, menschSparFreibetrag},
		"UmlaufMonat":       {p.UmlaufMonat, menschUmlaufMonat},
		"FreiGrenze":        {p.FreiGrenze, freiGrenze},
		"LiegeRate2Monat":   {p.LiegeRate2Monat, liegeRate2Monat},
		"ZaehltJeQuartal":   {p.ZaehltJeQuartal, menschZaehltJeUntQuartal},
		"TauschFreiMonat":   {p.TauschFreiMonat, menschTauschFreiMonat},
		"AusstiegBps":       {p.AusstiegBps, ausstiegsAbgabeBps},
		"GrenzeFaktor":      {p.GrenzeFaktor, wealthCapMultiplier},
	} {
		if paar[0] != paar[1] {
			t.Fatalf("%s: Modell %g, Kette %g", name, paar[0], paar[1])
		}
	}
}

// Was die Regeln bezwecken, zeigt das Modell bei den festen Akteuren.
func TestWirtschaftsmodell_Akteure(t *testing.T) {
	e := Wirtschaftsmodell(StandardParameter(), ModellAkteure(), 12)
	z := map[string]ModellZeile{}
	for _, r := range e.Zeilen {
		z[r.Name] = r
	}
	if r := z["Mensch mit wenig"]; r.AbgabenJeMonat != 0 {
		t.Fatalf("Mensch mit wenig zahlt %.2f im Monat -- unter den Freibetraegen soll nichts anfallen", r.AbgabenJeMonat)
	}
	if r := z["Mensch mit viel"]; r.Kappung <= 0 || r.Umlauf <= 0 {
		t.Fatalf("Mensch mit viel: Kappung %.2f, Umlauf %.2f -- ueber Grenze und Freibetrag faellt beides an", r.Kappung, r.Umlauf)
	}
	for _, name := range []string{"Baeckerei", "Supermarkt", "Grosshaendler", "Huelle", "Absprache unter Freunden"} {
		if z[name].Kappung != 0 {
			t.Fatalf("%s gekappt -- die Vermoegensgrenze gilt nicht fuer Unternehmen", name)
		}
	}
	if z["Huelle"].BelastungProzent <= 5*z["Baeckerei"].BelastungProzent {
		t.Fatalf("Huelle %.3f %% gegen Baeckerei %.3f %% -- eine Huelle ohne Umsatz soll deutlich mehr zahlen",
			z["Huelle"].BelastungProzent, z["Baeckerei"].BelastungProzent)
	}
	if r := z["Absprache unter Freunden"]; r.AnrechUmsatz != 0 || r.Umlauf <= 0 {
		t.Fatalf("Absprache unter Freunden: Umsatz %.0f, Liegegeld %.2f -- Rueckzahlungen heben den Umsatz auf", r.AnrechUmsatz, r.Umlauf)
	}
	if r := z["Grosshaendler"]; r.AnrechUmsatz != 40*menschZaehltJeUntQuartal/3 {
		t.Fatalf("Grosshaendler: anrechenbarer Umsatz %.0f -- gedeckelt je zahlendem Unternehmen", r.AnrechUmsatz)
	}
	// Zwei Freunde kaufen fuer 20.000 im Monat ein, ohne Rueckzahlung: es
	// zaehlen hoechstens ZaehltJeQuartal je Mensch und Quartal.
	p := StandardParameter()
	if u := p.anrechenbarerUmsatz(ModellAkteur{VonMenschen: 20000, Kunden: 2}); u != 2*menschZaehltJeUntQuartal/3 {
		t.Fatalf("zwei Kunden, 20.000 im Monat: anrechenbar %.0f, erwartet %.0f", u, 2*menschZaehltJeUntQuartal/3)
	}
	summe := 0.0
	for _, r := range e.Zeilen {
		summe += r.Gebuehren + r.Umlauf + r.Ausstieg + r.Kappung
	}
	if math.Abs(summe-e.GrundeinkommenSum) > 1e-3 {
		t.Fatalf("Topf %.3f, Summe der Abgaben %.3f", e.GrundeinkommenSum, summe)
	}
}

func TestWirtschaftsmodell_ParameterUndEmpfindlichkeit(t *testing.T) {
	p := StandardParameter()
	if err := ModellParameterSetzen(&p, "umlaufmonat", 0.01); err != nil || p.UmlaufMonat != 0.01 {
		t.Fatalf("Setzen: %v, %g", err, p.UmlaufMonat)
	}
	if err := ModellParameterSetzen(&p, "gibtsnicht", 1); err == nil {
		t.Fatal("unbekannter Parameter angenommen")
	}
	tab := ModellEmpfindlichkeit(StandardParameter(), ModellAkteure(), 6)
	q := StandardParameter()
	for _, f := range modellFelder(&q) {
		for _, x := range []string{" ×0.5 ", " ×2 "} {
			if !strings.Contains(tab, "| "+f.name+x) {
				t.Fatalf("Empfindlichkeit ohne Zeile %s%s", f.name, x)
			}
		}
	}
	if !strings.Contains(ModellTabelle(Wirtschaftsmodell(StandardParameter(), ModellAkteure(), 3), 100), "je Mensch und Monat") {
		t.Fatal("Tabelle ohne Grundeinkommen je Mensch")
	}
}
