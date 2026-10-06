package keeper

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// DAS WIRTSCHAFTSMODELL (06.10.2026, WIRTSCHAFT_ZAHLENPRUEFUNG.md 4.2).
//
// "Ein kleines Modell mit festen Akteuren, das die echten Formeln aus
// wirtschaft.go nutzt und jede Zahlaenderung gegen alle Akteure zeigt."
//
// Die Formeln stehen hier ein zweites Mal, mit den Zahlen als Parametern --
// sonst liesse sich keine Zahl aendern, ohne die Kette zu aendern.
// TestWirtschaftsmodell_FormelnGleichDerKette haelt sie bei den heutigen
// Zahlen gleich mit wirtschaft.go: Gebuehr, Umlaufsicherung, Liegegeld,
// Ausstiegsabgabe, freie Adressen. Aendert jemand eine Formel in der Kette
// und nicht hier, wird der Test rot.
//
// Was das Modell rechnet, je Akteur und Monat:
//   - Gebuehr: Menschen die ersten FreiAusgaben im Monat frei, danach
//     GebuehrBps; Unternehmen an Menschen frei, sonst GebuehrBps.
//   - Umlaufsicherung: Menschen auf den Teil ueber dem Sparfreibetrag, freie
//     Adressen auf alles; Unternehmen Liegegeld nach Guthaben und
//     anrechenbarem Monatsumsatz (Sockel, 1,5 / 3 Monatsumsaetze), das erste
//     Unternehmen einer Gruenderin nie mehr als "wie ein Mensch".
//   - Ausstiegsabgabe auf Tausch in Stable, Menschen mit Monatsfreibetrag.
//   - Kappung: Menschen und freie Adressen ueber Grenze x Durchschnitt.
//   - Alle Abgaben gehen ins Grundeinkommen. Das Modell weist den Topf aus
//     und, was er je Mensch bei einer angenommenen Bevoelkerung bedeutet --
//     es verteilt ihn nicht auf seine wenigen Akteure (das blaehte das
//     Grundeinkommen um ein Vielfaches auf).
//
// Anrechenbarer Umsatz: Einkaeufe eines Menschen zaehlen je Unternehmen
// hoechstens ZaehltJeQuartal im Quartal, Rueckzahlungen an denselben
// Menschen heben sie auf (Absprache unter Freunden); zwischen Unternehmen der
// hoehere Wert aus Ueberschuss und gedeckelten Eingaengen (wirtschaft.go,
// monatsUmsatzLocked).
//
// Was es NICHT rechnet: Preise, Verhalten, Wachstum, Pool und Kurs, die
// 30-Tage-Mindestdaten eines Knotens. Es zeigt, wen welche Zahl wie trifft --
// nicht, wie sich eine Wirtschaft entwickelt.

// ModellParameter: die Zahlen der Regeln, Standard = wirtschaft.go heute.
type ModellParameter struct {
	FairerAnteil       float64 // registrationGrant
	FreiAusgabenMonat  float64 // gebuehrenfreie Ausgaben eines Menschen je Monat
	GebuehrBps         float64 // Gebuehr in Basispunkten
	SparFreibetrag     float64 // Menschen: darunter keine Umlaufsicherung
	UmlaufMonat        float64 // Menschen: Satz je Monat darueber
	FreiGrenze         float64 // freie Adresse: hoechstens so viel
	FreiUmlaufMonat    float64 // freie Adresse: Satz je Monat auf alles
	Sockel             float64 // Unternehmen: immer frei
	UmsatzFreiFaktor   float64 // bis so viele Monatsumsaetze frei
	UmsatzStufe2Faktor float64 // ab so vielen Monatsumsaetzen die hohe Stufe
	LiegeRate1Monat    float64
	LiegeRate2Monat    float64
	ZaehltJeQuartal    float64 // Einkaeufe je Mensch und Unternehmen und Quartal
	TauschFreiMonat    float64 // Menschen: Tausch in Stable ohne Abgabe
	AusstiegBps        float64
	GrenzeFaktor       float64 // Vermoegensgrenze = Faktor x Durchschnitt
	Durchschnitt       float64 // Guthaben des Durchschnittsmenschen
}

// StandardParameter: die Zahlen, die heute in der Kette gelten.
func StandardParameter() ModellParameter {
	return ModellParameter{
		FairerAnteil:       registrationGrant,
		FreiAusgabenMonat:  menschFreiAusgabenMonat,
		GebuehrBps:         float64(ueberweisungsGebuehrBps),
		SparFreibetrag:     menschSparFreibetrag,
		UmlaufMonat:        menschUmlaufMonat,
		FreiGrenze:         freiGrenze,
		FreiUmlaufMonat:    freiUmlaufMonat,
		Sockel:             unternehmenSockel,
		UmsatzFreiFaktor:   umsatzFreiFaktor,
		UmsatzStufe2Faktor: umsatzStufe2Faktor,
		LiegeRate1Monat:    liegeRate1Monat,
		LiegeRate2Monat:    liegeRate2Monat,
		ZaehltJeQuartal:    menschZaehltJeUntQuartal,
		TauschFreiMonat:    menschTauschFreiMonat,
		AusstiegBps:        ausstiegsAbgabeBps,
		GrenzeFaktor:       wealthCapMultiplier,
		Durchschnitt:       registrationGrant,
	}
}

// Art eines Akteurs.
const (
	ModellMensch      = "mensch"
	ModellUnternehmen = "unternehmen"
	ModellFrei        = "frei"
)

// ModellAkteur: was ein Akteur im Monat tut.
type ModellAkteur struct {
	Name        string
	Art         string
	Guthaben    float64 // zu Beginn
	Einnahmen   float64 // je Monat (Lohn, Verkaeufe, Entnahmen)
	AnMenschen  float64 // Zahlungen an Menschen je Monat (Lohn, Rueckzahlung)
	AnFirmen    float64 // Zahlungen an Unternehmen und freie Adressen je Monat
	TauschMonat float64 // AEQ -> Stable je Monat
	// Unternehmen: Einkaeufe von Menschen je Monat und wie viele Menschen
	// das sind (fuer die Deckelung je Mensch), Eingaenge von Unternehmen,
	// und was davon an dieselben Menschen zurueckgeht.
	VonMenschen    float64
	Kunden         int
	VonFirmen      float64
	Zahler         int // so viele Unternehmen zahlen VonFirmen (Deckelung je Zahler)
	ZurueckAnKunde float64
	ErstesUntern   bool    // erstes Unternehmen der Gruenderin: nie mehr als "wie ein Mensch"
	GruenderStand  float64 // Guthaben der Gruenderin (fuer "wie ein Mensch")
}

// ModellAkteure: die festen Akteure aus WIRTSCHAFT_ZAHLENPRUEFUNG.md 4.2.
// Die Betraege sind Annahmen fuer eine kleine Stadt, keine Messung.
func ModellAkteure() []ModellAkteur {
	return []ModellAkteur{
		{Name: "Mensch mit wenig", Art: ModellMensch, Guthaben: 300, Einnahmen: 900, AnFirmen: 880},
		{Name: "Mensch mit viel", Art: ModellMensch, Guthaben: 30000, Einnahmen: 3500, AnFirmen: 2000, AnMenschen: 300, TauschMonat: 1500},
		{Name: "Baeckerei", Art: ModellUnternehmen, Guthaben: 4000, Einnahmen: 9000, VonMenschen: 9000, Kunden: 300,
			AnFirmen: 6000, AnMenschen: 2800, ErstesUntern: true, GruenderStand: 2000},
		{Name: "Supermarkt", Art: ModellUnternehmen, Guthaben: 40000, Einnahmen: 60000, VonMenschen: 60000, Kunden: 1500,
			AnFirmen: 45000, AnMenschen: 14000},
		{Name: "Grosshaendler", Art: ModellUnternehmen, Guthaben: 120000, Einnahmen: 200000, VonFirmen: 200000, Zahler: 40,
			AnFirmen: 180000, AnMenschen: 18000},
		{Name: "Huelle", Art: ModellUnternehmen, Guthaben: 50000},
		{Name: "Absprache unter Freunden", Art: ModellUnternehmen, Guthaben: 30000, Einnahmen: 20000, VonMenschen: 20000, Kunden: 2,
			AnMenschen: 20000, ZurueckAnKunde: 20000},
		{Name: "Freie Adresse (Kasse)", Art: ModellFrei, Guthaben: 200, Einnahmen: 150, AnFirmen: 150},
	}
}

// ModellZeile: ein Akteur nach monate Monaten.
type ModellZeile struct {
	Name             string
	Art              string
	Start, Ende      float64
	Gebuehren        float64
	Umlauf           float64 // Umlaufsicherung bzw. Liegegeld
	Ausstieg         float64
	Kappung          float64
	AnrechUmsatz     float64 // anrechenbarer Monatsumsatz (Unternehmen)
	AbgabenJeMonat   float64 // Gebuehren + Umlauf + Ausstieg + Kappung, je Monat
	BelastungProzent float64 // AbgabenJeMonat im Verhaeltnis zum mittleren Guthaben, in %
}

// ModellErgebnis: alle Akteure und die Summe ins Grundeinkommen.
type ModellErgebnis struct {
	Monate            int
	Zeilen            []ModellZeile
	GrundeinkommenSum float64
}

// --- die Formeln, parametrisiert (Standard = wirtschaft.go) ---

func (p ModellParameter) gebuehr(betrag float64) float64 {
	if betrag <= 0 || math.IsNaN(betrag) || math.IsInf(betrag, 0) {
		return 0
	}
	return round6(betrag * p.GebuehrBps / 10_000)
}

func (p ModellParameter) menschUmlauf(stand float64) float64 {
	return math.Max(0, stand-p.SparFreibetrag) * p.UmlaufMonat
}

func (p ModellParameter) liegegeld(stand, umsatz float64) float64 {
	frei := math.Max(p.Sockel, p.UmsatzFreiFaktor*umsatz)
	stufe2 := math.Max(p.Sockel, p.UmsatzStufe2Faktor*umsatz)
	mittel := math.Max(0, math.Min(stand, stufe2)-frei)
	hoch := math.Max(0, stand-stufe2)
	return mittel*p.LiegeRate1Monat + hoch*p.LiegeRate2Monat
}

// liegegeldWieMensch: das erste Unternehmen einer Gruenderin zahlt nie mehr,
// als das Geld bei ihr kosten wuerde (wirtschaft.go, liegegeldLocked).
func (p ModellParameter) liegegeldWieMensch(stand, umsatz, gruender float64) float64 {
	normal := p.liegegeld(stand, umsatz)
	g := math.Max(0, gruender)
	platz := math.Max(0, p.FairerAnteil*p.GrenzeFaktor-g)
	unten := math.Min(stand, platz)
	wieMensch := p.menschUmlauf(g+unten) - p.menschUmlauf(g) + p.liegegeld(stand, umsatz) - p.liegegeld(unten, umsatz)
	return math.Min(normal, wieMensch)
}

func (p ModellParameter) ausstieg(betrag, frei float64) float64 {
	return round6(math.Max(0, betrag-frei) * p.AusstiegBps / 10_000)
}

func (p ModellParameter) grenze() float64 { return p.GrenzeFaktor * p.Durchschnitt }

// anrechenbarerUmsatz je Monat (monatsUmsatzLocked, vereinfacht auf Monate).
func (p ModellParameter) anrechenbarerUmsatz(a ModellAkteur) float64 {
	mensch := a.VonMenschen
	if a.Kunden > 0 {
		mensch = math.Min(mensch, float64(a.Kunden)*p.ZaehltJeQuartal/3)
	}
	mensch = math.Max(0, mensch-a.ZurueckAnKunde)
	ueberschuss := math.Max(0, a.VonFirmen-a.AnFirmen)
	gedeckelt := 0.0
	if a.Zahler > 0 {
		gedeckelt = math.Min(a.VonFirmen, float64(a.Zahler)*p.ZaehltJeQuartal/3)
	}
	return mensch + math.Max(ueberschuss, gedeckelt)
}

// Wirtschaftsmodell rechnet monate Monate fuer die Akteure.
func Wirtschaftsmodell(p ModellParameter, akteure []ModellAkteur, monate int) ModellErgebnis {
	n := len(akteure)
	stand := make([]float64, n)
	summe := make([]float64, n)
	zeilen := make([]ModellZeile, n)
	for i, a := range akteure {
		stand[i] = a.Guthaben
		zeilen[i] = ModellZeile{Name: a.Name, Art: a.Art, Start: a.Guthaben}
	}
	var topfGesamt float64
	for m := 0; m < monate; m++ {
		var topf float64
		for i, a := range akteure {
			z := &zeilen[i]
			stand[i] += a.Einnahmen
			var geb, uml, aus, kap float64
			switch a.Art {
			case ModellMensch:
				ausgaben := a.AnFirmen + a.AnMenschen
				geb = p.gebuehr(math.Max(0, ausgaben-p.FreiAusgabenMonat))
				aus = p.ausstieg(a.TauschMonat, p.TauschFreiMonat)
			case ModellFrei:
				geb = p.gebuehr(a.AnFirmen + a.AnMenschen)
				aus = p.ausstieg(a.TauschMonat, 0)
			case ModellUnternehmen:
				geb = p.gebuehr(a.AnFirmen) // an Menschen frei
				aus = p.ausstieg(a.TauschMonat, 0)
			}
			stand[i] -= a.AnFirmen + a.AnMenschen + a.TauschMonat + geb + aus
			switch a.Art {
			case ModellMensch:
				uml = round6(p.menschUmlauf(stand[i]))
			case ModellFrei:
				uml = round6(math.Max(0, stand[i]) * p.FreiUmlaufMonat)
			case ModellUnternehmen:
				u := p.anrechenbarerUmsatz(a)
				z.AnrechUmsatz = u
				if a.ErstesUntern {
					uml = round6(p.liegegeldWieMensch(stand[i], u, a.GruenderStand))
				} else {
					uml = round6(p.liegegeld(stand[i], u))
				}
			}
			uml = math.Min(uml, math.Max(0, stand[i]))
			stand[i] -= uml
			if a.Art != ModellUnternehmen && stand[i] > p.grenze() {
				kap = round6(stand[i] - p.grenze())
				stand[i] -= kap
			}
			z.Gebuehren += geb
			z.Umlauf += uml
			z.Ausstieg += aus
			z.Kappung += kap
			topf += geb + uml + aus + kap
			summe[i] += stand[i]
		}
		topfGesamt += topf
	}
	for i := range zeilen {
		z := &zeilen[i]
		z.Ende = round6(stand[i])
		if monate > 0 {
			z.AbgabenJeMonat = round6((z.Gebuehren + z.Umlauf + z.Ausstieg + z.Kappung) / float64(monate))
			if mittel := summe[i] / float64(monate); mittel > 0 {
				z.BelastungProzent = round6(z.AbgabenJeMonat / mittel * 100)
			}
		}
	}
	return ModellErgebnis{Monate: monate, Zeilen: zeilen, GrundeinkommenSum: round6(topfGesamt)}
}

// ModellParameterSetzen: "Name=Wert" (Feldname wie in ModellParameter).
func ModellParameterSetzen(p *ModellParameter, name string, wert float64) error {
	for _, f := range modellFelder(p) {
		if strings.EqualFold(f.name, name) {
			*f.ziel = wert
			return nil
		}
	}
	return fmt.Errorf("unbekannter Parameter %q", name)
}

type modellFeld struct {
	name string
	ziel *float64
}

func modellFelder(p *ModellParameter) []modellFeld {
	return []modellFeld{
		{"FreiAusgabenMonat", &p.FreiAusgabenMonat}, {"GebuehrBps", &p.GebuehrBps},
		{"SparFreibetrag", &p.SparFreibetrag}, {"UmlaufMonat", &p.UmlaufMonat},
		{"FreiGrenze", &p.FreiGrenze}, {"FreiUmlaufMonat", &p.FreiUmlaufMonat},
		{"Sockel", &p.Sockel}, {"UmsatzFreiFaktor", &p.UmsatzFreiFaktor},
		{"UmsatzStufe2Faktor", &p.UmsatzStufe2Faktor}, {"LiegeRate1Monat", &p.LiegeRate1Monat},
		{"LiegeRate2Monat", &p.LiegeRate2Monat}, {"ZaehltJeQuartal", &p.ZaehltJeQuartal},
		{"TauschFreiMonat", &p.TauschFreiMonat}, {"AusstiegBps", &p.AusstiegBps},
		{"GrenzeFaktor", &p.GrenzeFaktor}, {"Durchschnitt", &p.Durchschnitt},
	}
}

// ModellTabelle: das Ergebnis als Markdown-Tabelle; menschen = angenommene
// Bevoelkerung, auf die der Topf verteilt wuerde (0 = nicht ausweisen).
func ModellTabelle(e ModellErgebnis, menschen int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| Akteur | Art | Start | Ende nach %d Mon. | Abgaben/Monat | davon Gebuehr | Umlauf/Liegegeld | Ausstieg | Kappung | Belastung %% vom Guthaben/Monat | anrechenbarer Umsatz/Monat |\n", e.Monate)
	b.WriteString("|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	m := float64(e.Monate)
	if m == 0 {
		m = 1
	}
	for _, z := range e.Zeilen {
		umsatz := "–"
		if z.Art == ModellUnternehmen {
			umsatz = fmt.Sprintf("%.0f", z.AnrechUmsatz)
		}
		fmt.Fprintf(&b, "| %s | %s | %.0f | %.0f | %.2f | %.2f | %.2f | %.2f | %.2f | %.3f | %s |\n",
			z.Name, z.Art, z.Start, z.Ende, z.AbgabenJeMonat, z.Gebuehren/m, z.Umlauf/m, z.Ausstieg/m, z.Kappung/m,
			z.BelastungProzent, umsatz)
	}
	fmt.Fprintf(&b, "\nIns Grundeinkommen von diesen Akteuren: %.2f AEQ in %d Monaten (%.2f je Monat)", e.GrundeinkommenSum, e.Monate, e.GrundeinkommenSum/m)
	if menschen > 0 {
		fmt.Fprintf(&b, " -- verteilt auf %d Menschen %.4f AEQ je Mensch und Monat", menschen, e.GrundeinkommenSum/m/float64(menschen))
	}
	b.WriteString(".\n")
	return b.String()
}

// ModellEmpfindlichkeit: jede Zahl halbiert und verdoppelt -- wie aendern
// sich die Abgaben je Monat jedes Akteurs? Zeilen nach Parameter sortiert.
func ModellEmpfindlichkeit(p ModellParameter, akteure []ModellAkteur, monate int) string {
	basis := Wirtschaftsmodell(p, akteure, monate)
	var b strings.Builder
	b.WriteString("| Parameter | Wert | ")
	for _, z := range basis.Zeilen {
		b.WriteString(z.Name + " | ")
	}
	b.WriteString("\n|---|---:|")
	for range basis.Zeilen {
		b.WriteString("---:|")
	}
	b.WriteString("\n")
	felder := modellFelder(&p)
	sort.SliceStable(felder, func(i, j int) bool { return felder[i].name < felder[j].name })
	for _, f := range felder {
		orig := *f.ziel
		for _, faktor := range []float64{0.5, 2} {
			q := p
			_ = ModellParameterSetzen(&q, f.name, orig*faktor)
			e := Wirtschaftsmodell(q, akteure, monate)
			fmt.Fprintf(&b, "| %s ×%g | %g | ", f.name, faktor, orig*faktor)
			for i, z := range e.Zeilen {
				d := z.AbgabenJeMonat - basis.Zeilen[i].AbgabenJeMonat
				if math.Abs(d) < 0.005 {
					b.WriteString("· | ")
				} else {
					fmt.Fprintf(&b, "%+.2f | ", d)
				}
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\nWerte: Aenderung der Abgaben je Monat (AEQ) gegenueber den heutigen Zahlen; · = keine Aenderung.\n")
	return b.String()
}
