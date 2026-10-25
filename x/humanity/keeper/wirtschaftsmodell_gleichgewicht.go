package keeper

import (
	"fmt"
	"math"
	"strings"
)

// DER VORSCHLAG "GLEICHGEWICHT" IM MODELL (docs/WIRTSCHAFT_ENDFASSUNG.md,
// 25.10.2026). Nur Modell -- die Kette rechnet weiter nach wirtschaft.go.
//
// Eine Regel fuer jedes Konto: SatzMonat auf das ganze Guthaben (Menschen,
// Unternehmen, freie Adressen gleich, LP-Anteile mitgezaehlt), ohne
// Freibetrag und ohne Stufen. Alles geht ins Grundeinkommen, das jeder Mensch
// mit gueltigem Lebensnachweis zu gleichen Teilen bekommt. Keine
// Ueberweisungsgebuehr. Ausstieg wie heute.
//
// Keine hohe Stufe: ein fester Betrag (etwa 25 x fairer Anteil) trifft echte
// Betriebe mit grossem Umsatz genauso wie eine Huelle -- ein Supermarkt mit
// 60.000 AEQ Monatsumsatz haelt normal mehr als 25.000 AEQ. Und jeder
// Freibetrag irgendwo waere, sobald Menschen ab dem ersten AEQ zahlen, ein
// Versteck: Geld zoege dorthin um.
//
// Was daraus folgt, ohne jede Annahme ueber Verhalten:
//   - Ausserhalb der Toepfe liegen Menschen x fairer Anteil. Der Topf ist
//     darum mindestens SatzMonat x fairer Anteil je Mensch und Monat -- die
//     Dividende haengt nicht davon ab, wer wie viel haelt.
//   - Netto zahlt ein aktiver Mensch SatzMonat x (Guthaben - fairer Anteil):
//     wer weniger hat, bekommt dazu; wer genau den fairen Anteil hat, zahlt
//     nichts.
//   - Geld zwischen Kontoarten zu verschieben aendert die Summe der Abgaben
//     nicht: das Etikett "Unternehmen" bringt nichts, und nichts muss
//     geprueft werden, um das sicherzustellen.
//   - Ein Mensch ohne Bewegung naehert sich dem fairen Anteil:
//     Guthaben(n) = fair + (Start - fair) x (1 - SatzMonat)^n.

// GleichgewichtParameter: die Zahlen des Vorschlags.
type GleichgewichtParameter struct {
	FairerAnteil float64 // registrationGrant
	SatzMonat    float64 // auf jedes Guthaben, je Monat
	GrenzeFaktor float64 // Vermoegensgrenze der Menschen, wie heute
	FreiGrenze   float64 // freie Adressen, wie heute
	// AktivAnteil: Anteil der Menschen mit gueltigem Lebensnachweis. Nur sie
	// bekommen den Topf; die anderen zahlen, wie jedes Konto.
	AktivAnteil float64
}

// StandardGleichgewicht: der Vorschlag mit den Zahlen aus der Endfassung.
// SatzMonat ist der heutige Satz der Menschen, die Grenzen sind die heutigen.
func StandardGleichgewicht() GleichgewichtParameter {
	return GleichgewichtParameter{
		FairerAnteil: registrationGrant,
		SatzMonat:    menschUmlaufMonat,
		GrenzeFaktor: wealthCapMultiplier,
		FreiGrenze:   freiGrenze,
		AktivAnteil:  1,
	}
}

// Umlauf: was ein Konto mit diesem Guthaben im Monat abgibt -- fuer jede
// Kontoart dieselbe Rechnung.
func (g GleichgewichtParameter) Umlauf(stand float64) float64 {
	if stand <= 0 || math.IsNaN(stand) || math.IsInf(stand, 0) {
		return 0
	}
	return round6(stand * g.SatzMonat)
}

// DividendeJeMensch: was ein aktiver Mensch im Monat aus dem Topf bekommt --
// unabhaengig davon, wer wie viel haelt, weil jedes Guthaben denselben Satz
// zahlt und alle Guthaben zusammen Menschen x fairer Anteil sind (ohne die
// Toepfe; was dort liegt, macht sie etwas kleiner).
func (g GleichgewichtParameter) DividendeJeMensch() float64 {
	if g.AktivAnteil <= 0 {
		return 0
	}
	return round6(g.SatzMonat * g.FairerAnteil / g.AktivAnteil)
}

// HalbwertMonate: nach so vielen Monaten ist der Abstand eines ruhenden,
// aktiven Menschen zum fairen Anteil halbiert.
func (g GleichgewichtParameter) HalbwertMonate() float64 {
	if g.SatzMonat <= 0 || g.SatzMonat >= 1 {
		return math.Inf(1)
	}
	return math.Ln2 / -math.Log(1-g.SatzMonat)
}

// VerlaufRuhend: Guthaben eines aktiven Menschen ohne eigene Zahlungen nach
// monate Monaten -- geschlossene Form der Monatsrechnung in
// GleichgewichtModell (unter der Grenze).
func (g GleichgewichtParameter) VerlaufRuhend(start float64, monate int) float64 {
	d := g.DividendeJeMensch() / g.SatzMonat // Gleichgewicht: fairer Anteil / AktivAnteil
	return d + (start-d)*math.Pow(1-g.SatzMonat, float64(monate))
}

// GleichgewichtAkteure: die Akteure des heutigen Modells und zwei, an denen
// sich die Regeln am deutlichsten unterscheiden.
func GleichgewichtAkteure() []ModellAkteur {
	a := ModellAkteure()
	return append(a,
		ModellAkteur{Name: "Mensch, Startguthaben liegt", Art: ModellMensch, Guthaben: registrationGrant},
		ModellAkteur{Name: "Verlorener Schluessel", Art: ModellMensch, Guthaben: registrationGrant, Inaktiv: true},
	)
}

// VergleichZeile: ein Akteur heute und im Vorschlag.
type VergleichZeile struct {
	Name, Art          string
	Start              float64
	EndeHeute          float64
	EndeVorschlag      float64
	AbgabenHeute       float64 // je Monat
	AbgabenVorschlag   float64 // je Monat
	DividendeVorschlag float64 // je Monat (nur aktive Menschen)
	NettoVorschlag     float64 // Dividende - Abgaben, je Monat
}

// Vergleich: die heutigen Regeln (Wirtschaftsmodell) und der Vorschlag mit
// denselben Akteuren, Zahlungen und Monaten. Der Ausstieg ist in beiden
// gleich gerechnet (heutige Regel), damit nur die Umlaufregel sich
// unterscheidet.
type Vergleich struct {
	Monate            int
	Zeilen            []VergleichZeile
	TopfHeuteJeMonat  float64 // aus diesen Akteuren
	DividendeJeMensch float64 // je aktivem Menschen, aus der ganzen Geldmenge
}

// GleichgewichtModell rechnet beide Regelwerke.
func GleichgewichtModell(p ModellParameter, g GleichgewichtParameter, akteure []ModellAkteur, monate int) Vergleich {
	heute := Wirtschaftsmodell(p, akteure, monate)
	v := Vergleich{Monate: monate, DividendeJeMensch: g.DividendeJeMensch()}
	if monate > 0 {
		v.TopfHeuteJeMonat = round6(heute.GrundeinkommenSum / float64(monate))
	}
	grenze := g.GrenzeFaktor * g.FairerAnteil
	for i, a := range akteure {
		stand := a.Guthaben
		var abgaben, dividende float64
		for m := 0; m < monate; m++ {
			stand += a.Einnahmen
			frei := 0.0
			if a.Art == ModellMensch {
				frei = p.TauschFreiMonat
			}
			aus := p.ausstieg(a.TauschMonat, frei)
			stand -= a.AnFirmen + a.AnMenschen + a.TauschMonat + aus
			uml := math.Min(g.Umlauf(stand), math.Max(0, stand))
			stand -= uml
			abgaben += aus + uml
			if a.Art == ModellMensch && !a.Inaktiv {
				stand += g.DividendeJeMensch()
				dividende += g.DividendeJeMensch()
			}
			switch {
			case a.Art == ModellMensch && stand > grenze:
				abgaben += stand - grenze
				stand = grenze
			case a.Art == ModellFrei && stand > g.FreiGrenze:
				abgaben += stand - g.FreiGrenze
				stand = g.FreiGrenze
			}
		}
		z := VergleichZeile{
			Name: a.Name, Art: a.Art, Start: a.Guthaben,
			EndeHeute: heute.Zeilen[i].Ende, EndeVorschlag: round6(stand),
			AbgabenHeute: heute.Zeilen[i].AbgabenJeMonat,
		}
		if monate > 0 {
			z.AbgabenVorschlag = round6(abgaben / float64(monate))
			z.DividendeVorschlag = round6(dividende / float64(monate))
			z.NettoVorschlag = round6(z.DividendeVorschlag - z.AbgabenVorschlag)
		}
		v.Zeilen = append(v.Zeilen, z)
	}
	return v
}

// VergleichTabelle: das Ergebnis als Markdown.
func VergleichTabelle(v Vergleich, menschen int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| Akteur | Art | Start | Ende heute | Ende Vorschlag | Abgaben/Monat heute | Abgaben/Monat Vorschlag | Dividende/Monat | Netto/Monat Vorschlag |\n")
	b.WriteString("|---|---|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, z := range v.Zeilen {
		div, netto := "–", "–"
		if z.Art == ModellMensch {
			div = fmt.Sprintf("%.2f", z.DividendeVorschlag)
			netto = fmt.Sprintf("%+.2f", z.NettoVorschlag)
		} else {
			netto = fmt.Sprintf("%+.2f", -z.AbgabenVorschlag)
		}
		fmt.Fprintf(&b, "| %s | %s | %.0f | %.0f | %.0f | %.2f | %.2f | %s | %s |\n",
			z.Name, z.Art, z.Start, z.EndeHeute, z.EndeVorschlag, z.AbgabenHeute, z.AbgabenVorschlag, div, netto)
	}
	fmt.Fprintf(&b, "\nGrundeinkommen heute aus diesen Akteuren: %.2f AEQ im Monat", v.TopfHeuteJeMonat)
	if menschen > 0 {
		fmt.Fprintf(&b, ", auf %d Menschen %.4f AEQ je Mensch", menschen, v.TopfHeuteJeMonat/float64(menschen))
	}
	fmt.Fprintf(&b, ".\nDividende im Vorschlag: rund %.2f AEQ je aktivem Menschen und Monat, aus der ganzen Geldmenge, unabhaengig von diesen Akteuren.\n", v.DividendeJeMensch)
	b.WriteString("\nAusstieg in beiden Spalten nach heutiger Regel. Abgaben im Vorschlag: Umlauf, Ausstieg, Kappung; keine Gebuehr.\n")
	return b.String()
}

// LadenkostenTabelle: was AEQ annehmen einen Betrieb kostet, gemessen am
// Monatsumsatz, je nachdem wie lange das Geld bei ihm liegt -- heute
// (Liegegeld nach Umsatz) und im Vorschlag (ein Satz auf alles).
func LadenkostenTabelle(p ModellParameter, g GleichgewichtParameter, umsatz float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| Geld liegt im Mittel | heute, %% vom Monatsumsatz | Vorschlag, %% vom Monatsumsatz |\n")
	b.WriteString("|---|---:|---:|\n")
	for _, h := range []float64{0.25, 0.5, 1, 1.5, 2, 3, 6} {
		stand := h * umsatz
		heute := p.liegegeld(stand, umsatz) / umsatz * 100
		vor := g.Umlauf(stand) / umsatz * 100
		fmt.Fprintf(&b, "| %g Monatsumsätze | %.3f | %.3f |\n", h, heute, vor)
	}
	fmt.Fprintf(&b, "\nMonatsumsatz %.0f AEQ, zweites Unternehmen (heute ohne die Regel fuer das erste).\n", umsatz)
	return b.String()
}

// VerlaufTabelle: Guthaben eines aktiven Menschen ohne eigene Zahlungen.
func VerlaufTabelle(g GleichgewichtParameter) string {
	var b strings.Builder
	jahre := []int{0, 1, 5, 10, 20}
	b.WriteString("| Start |")
	for _, j := range jahre[1:] {
		fmt.Fprintf(&b, " nach %d J. |", j)
	}
	b.WriteString("\n|---:|")
	for range jahre[1:] {
		b.WriteString("---:|")
	}
	b.WriteString("\n")
	for _, s := range []float64{0, 300, 1000, 5000, 25000} {
		fmt.Fprintf(&b, "| %.0f |", s)
		for _, j := range jahre[1:] {
			fmt.Fprintf(&b, " %.0f |", g.VerlaufRuhend(s, 12*j))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "\nHalbwertzeit des Abstands zum fairen Anteil: %.0f Monate (%.1f Jahre).\n", g.HalbwertMonate(), g.HalbwertMonate()/12)
	return b.String()
}
