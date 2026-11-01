package keeper

import (
	"math"
	"math/rand"
	"testing"
)

// Der Vorschlag nimmt die Zahlen der Kette, wo er sie nicht aendert.
func TestGleichgewicht_StandardSindDieZahlenDerKette(t *testing.T) {
	g := StandardGleichgewicht()
	if g.FairerAnteil != registrationGrant || g.SatzMonat != menschUmlaufMonat ||
		g.GrenzeFaktor != wealthCapMultiplier || g.FreiGrenze != freiGrenze || g.AktivAnteil != 1 {
		t.Fatalf("Standard weicht von der Kette ab: %+v", g)
	}
}

// Wer genau den fairen Anteil haelt und nichts tut, zahlt netto nichts.
func TestGleichgewicht_FairerAnteilNettoNull(t *testing.T) {
	g := StandardGleichgewicht()
	a := []ModellAkteur{{Name: "fair", Art: ModellMensch, Guthaben: registrationGrant}}
	v := GleichgewichtModell(StandardParameter(), g, a, 120)
	z := v.Zeilen[0]
	if math.Abs(z.EndeVorschlag-registrationGrant) > 1e-6 || math.Abs(z.NettoVorschlag) > 1e-6 {
		t.Fatalf("fairer Anteil nach 10 Jahren %.6f, netto %.6f je Monat -- erwartet %.0f und 0", z.EndeVorschlag, z.NettoVorschlag, registrationGrant)
	}
}

// Netto = Satz x (fairer Anteil - Guthaben): unter dem fairen Anteil kommt
// etwas dazu, darueber geht etwas ab.
func TestGleichgewicht_WenigerBekommtDazuMehrGibtAb(t *testing.T) {
	g := StandardGleichgewicht()
	for _, start := range []float64{0, 1, 300, 999} {
		if netto := g.DividendeJeMensch() - g.Umlauf(start); netto <= 0 {
			t.Fatalf("Guthaben %.0f: netto %.6f, erwartet > 0", start, netto)
		}
	}
	for _, start := range []float64{1001, 5000, 25000} {
		if netto := g.DividendeJeMensch() - g.Umlauf(start); netto >= 0 {
			t.Fatalf("Guthaben %.0f: netto %.6f, erwartet < 0", start, netto)
		}
	}
}

// Missbrauch: Geld zwischen Mensch, eigenen Unternehmen und freier Adresse
// zu verschieben, senkt die Abgabe nicht. Darum muss niemand pruefen, ob ein
// Unternehmen echt ist, damit es kein Versteck wird.
func TestGleichgewicht_VerschiebenBringtNichts(t *testing.T) {
	g := StandardGleichgewicht()
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 2000; i++ {
		gesamt := r.Float64() * 200_000
		ganz := g.Umlauf(gesamt)
		// Aufteilen: freie Adresse bis zur Grenze, Mensch bis zur Grenze,
		// Rest auf hoechstens drei Unternehmen (maxUnternehmenJeMensch).
		rest := gesamt
		frei := math.Min(rest, r.Float64()*g.FreiGrenze)
		rest -= frei
		mensch := math.Min(rest, r.Float64()*g.GrenzeFaktor*g.FairerAnteil)
		rest -= mensch
		teile := []float64{frei, mensch}
		for k := 0; k < maxUnternehmenJeMensch-1; k++ {
			u := rest * r.Float64()
			teile = append(teile, u)
			rest -= u
		}
		teile = append(teile, rest)
		var summe float64
		for _, teil := range teile {
			summe += g.Umlauf(teil)
		}
		// Rundung auf 6 Stellen je Teil: hoechstens 0,5e-6 je Teil.
		if math.Abs(summe-ganz) > float64(len(teile))*1e-6 {
			t.Fatalf("Aufteilung %v von %.6f: Abgabe %.6f statt %.6f", teile, gesamt, summe, ganz)
		}
	}
}

// Die Dividende haengt nicht davon ab, wer wie viel haelt: bei jeder
// Verteilung von Menschen x fairer Anteil auf beliebige Konten ist der Topf
// Satz x fairer Anteil je Mensch.
func TestGleichgewicht_DividendeUnabhaengigVonVerteilung(t *testing.T) {
	g := StandardGleichgewicht()
	r := rand.New(rand.NewSource(2))
	for runde := 0; runde < 50; runde++ {
		menschen := 10 + r.Intn(500)
		konten := menschen + r.Intn(200) // dazu Unternehmen und freie Adressen
		gewichte := make([]float64, konten)
		var sw float64
		for i := range gewichte {
			gewichte[i] = math.Pow(r.Float64(), 3) // schief verteilt
			sw += gewichte[i]
		}
		geld := float64(menschen) * g.FairerAnteil
		var topf float64
		for _, w := range gewichte {
			topf += g.Umlauf(geld * w / sw)
		}
		jeMensch := topf / float64(menschen)
		if math.Abs(jeMensch-g.DividendeJeMensch()) > float64(konten)*1e-6/float64(menschen) {
			t.Fatalf("Runde %d: Topf je Mensch %.6f, erwartet %.6f", runde, jeMensch, g.DividendeJeMensch())
		}
	}
}

// Die geschlossene Form in VerlaufRuhend ist die Monatsrechnung des Modells.
func TestGleichgewicht_VerlaufGleichMonatsrechnung(t *testing.T) {
	for _, aktiv := range []float64{1, 0.9} {
		g := StandardGleichgewicht()
		g.AktivAnteil = aktiv
		for _, start := range []float64{0, 300, 1000, 5000, 24000} {
			a := []ModellAkteur{{Name: "ruhend", Art: ModellMensch, Guthaben: start}}
			for _, monate := range []int{1, 12, 60} {
				got := GleichgewichtModell(StandardParameter(), g, a, monate).Zeilen[0].EndeVorschlag
				want := g.VerlaufRuhend(start, monate)
				// round6 je Monat: hoechstens 1e-6 je Schritt.
				if math.Abs(got-want) > float64(monate)*1e-6+1e-9 {
					t.Fatalf("aktiv %.1f, Start %.0f, %d Monate: Modell %.6f, Formel %.6f", aktiv, start, monate, got, want)
				}
			}
		}
	}
}

// Ein Mensch ohne Lebensnachweis zahlt wie jedes Konto und bekommt nichts:
// Geld hinter einem verlorenen Schluessel kommt langsam zu den Lebenden.
func TestGleichgewicht_VerlorenerSchluesselGibtZurueck(t *testing.T) {
	g := StandardGleichgewicht()
	a := []ModellAkteur{{Name: "verloren", Art: ModellMensch, Guthaben: registrationGrant, Inaktiv: true}}
	z := GleichgewichtModell(StandardParameter(), g, a, 12).Zeilen[0]
	want := registrationGrant * math.Pow(1-g.SatzMonat, 12)
	if z.DividendeVorschlag != 0 || math.Abs(z.EndeVorschlag-want) > 12e-6 {
		t.Fatalf("verlorener Schluessel: Ende %.6f (erwartet %.6f), Dividende %.6f (erwartet 0)", z.EndeVorschlag, want, z.DividendeVorschlag)
	}
}

// Unsinnige Eingaben ergeben keine Abgabe und keine Dividende.
func TestGleichgewicht_UngueltigeEingaben(t *testing.T) {
	g := StandardGleichgewicht()
	for _, s := range []float64{-1, 0, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if u := g.Umlauf(s); u != 0 {
			t.Fatalf("Umlauf(%v) = %v, erwartet 0", s, u)
		}
	}
	g.AktivAnteil = 0
	if d := g.DividendeJeMensch(); d != 0 {
		t.Fatalf("Dividende ohne aktive Menschen = %v, erwartet 0", d)
	}
}
