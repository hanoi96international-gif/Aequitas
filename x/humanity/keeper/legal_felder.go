package keeper

import (
	"encoding/json"
	"html"
	"net/http"
	"os"
	"sort"
	"strings"
)

// Impressum und Datenschutzerklaerung ohne Codeaenderung ausfuellen.
//
// WARUM
//
// Beide Seiten sind fertig geschrieben und antworten trotzdem mit 404, weil
// die Identitaetsangaben des Betreibers fehlen -- Name, Anschrift, Kontakt,
// Aufsichtsbehoerde. Die kann niemand ausser dem Betreiber liefern, und ein
// erfundenes Impressum waere schlimmer als keines: es benennt jemanden, der
// vielleicht gar nicht verantwortlich ist.
//
// Bisher hiess Ausfuellen: HTML bearbeiten, uebersetzen, ausliefern. Also
// etwas, das man nur mit dem Werkzeugkasten machen kann -- und deshalb
// aufschiebt. Jetzt sind es Umgebungsvariablen: einmal setzen, Knoten neu
// starten, Seite ist da. Kein Deploy, kein Quelltext.
//
// WAS BEWUSST FEHLT
//
// Es gibt keine Voreinstellungen. Ein Impressum mit Platzhaltern, das
// ausgeliefert wird, erweckt den Anschein, die Pflicht sei erfuellt. Fehlt
// auch nur ein Pflichtfeld, bleibt es bei 404 -- und /api/legal-status sagt,
// welches.
//
// SICHERHEIT
//
// Jeder Wert wird HTML-escaped. Es sind Angaben des Betreibers, nicht von
// Fremden -- aber ein Ampersand im Firmennamen soll die Seite nicht zerlegen,
// und wer spaeter auf die Idee kommt, das aus einer anderen Quelle zu fuellen,
// findet die Absicherung bereits vor.

// legalFeld beschreibt eine Angabe, die in die Rechtstexte eingesetzt wird.
type legalFeld struct {
	Umgebung string // Name der Umgebungsvariablen
	Marke    string // Platzhalter im HTML
	Pflicht  bool
	Erklaert string
}

// legalFelder ist die vollstaendige Liste. Reihenfolge = Reihenfolge in der
// Anleitung.
var legalFelder = []legalFeld{
	{"LEGAL_NAME", "{{NAME}}", true,
		"Name oder Firma des Betreibers"},
	{"LEGAL_STRASSE", "{{STRASSE}}", true,
		"Strasse und Hausnummer"},
	{"LEGAL_ORT", "{{ORT}}", true,
		"PLZ und Ort"},
	{"LEGAL_LAND", "{{LAND}}", true,
		"Land"},
	{"LEGAL_EMAIL", "{{EMAIL}}", true,
		"E-Mail-Adresse fuer Impressum und Datenschutzanfragen"},
	{"LEGAL_VERANTWORTLICH", "{{VERANTWORTLICH}}", true,
		"Name der nach § 18 Abs. 2 MStV verantwortlichen Person"},
	{"LEGAL_AUFSICHTSBEHOERDE", "{{AUFSICHTSBEHOERDE}}", true,
		"zustaendige Datenschutz-Aufsichtsbehoerde (richtet sich nach dem Sitz)"},
	{"LEGAL_TELEFON", "{{TELEFON}}", false,
		"Telefonnummer, falls kein zweiter schneller Kontaktweg besteht"},
	{"LEGAL_VERTRETEN_DURCH", "{{VERTRETEN_DURCH}}", false,
		"nur bei einer Gesellschaft: vertretungsberechtigte Person"},
	{"LEGAL_REGISTERGERICHT", "{{REGISTERGERICHT}}", false,
		"nur bei einer Gesellschaft"},
	{"LEGAL_REGISTERNUMMER", "{{REGISTERNUMMER}}", false,
		"nur bei einer Gesellschaft"},
	{"LEGAL_USTID", "{{USTID}}", false,
		"Umsatzsteuer-Identifikationsnummer nach § 27a UStG"},
}

// fehlendeLegalFelder nennt die Pflichtangaben, die nicht gesetzt sind.
func fehlendeLegalFelder() []string {
	var fehlt []string
	for _, f := range legalFelder {
		if f.Pflicht && strings.TrimSpace(os.Getenv(f.Umgebung)) == "" {
			fehlt = append(fehlt, f.Umgebung)
		}
	}
	sort.Strings(fehlt)
	return fehlt
}

// legalEingesetzt ersetzt die Marken im Text durch die gesetzten Werte.
//
// Optionale Felder, die leer bleiben, werden samt ihrer Zeile entfernt: eine
// Zeile "Telefon:" ohne Nummer sieht aus wie ein Fehler, und ein leeres
// Registergericht wirft die Frage auf, ob da eine Gesellschaft steht oder
// nicht.
func legalEingesetzt(inhalt string) string {
	for _, f := range legalFelder {
		wert := strings.TrimSpace(os.Getenv(f.Umgebung))
		if wert == "" && !f.Pflicht {
			inhalt = zeileMitMarkeEntfernen(inhalt, f.Marke)
			continue
		}
		inhalt = strings.ReplaceAll(inhalt, f.Marke, html.EscapeString(wert))
	}
	return inhalt
}

// zeileMitMarkeEntfernen wirft die ganze Zeile weg, in der eine ungenutzte
// optionale Marke steht.
func zeileMitMarkeEntfernen(inhalt, marke string) string {
	zeilen := strings.Split(inhalt, "\n")
	behalten := zeilen[:0]
	for _, z := range zeilen {
		if !strings.Contains(z, marke) {
			behalten = append(behalten, z)
		}
	}
	return strings.Join(behalten, "\n")
}

// handleLegalStatus sagt, was noch fehlt, damit die Rechtstexte erscheinen.
//
// Oeffentlich und ohne Token: es sind Namen von Umgebungsvariablen, keine
// Werte. Wer die Seiten sucht und 404 bekommt, soll erfahren koennen, warum --
// sonst sieht es nach einem kaputten Server aus statt nach einer bewussten
// Zurueckhaltung.
func (a *APIServer) handleLegalStatus(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	fehlt := fehlendeLegalFelder()
	erklaerung := map[string]string{}
	for _, f := range legalFelder {
		if !f.Pflicht {
			continue
		}
		for _, m := range fehlt {
			if m == f.Umgebung {
				erklaerung[f.Umgebung] = f.Erklaert
			}
		}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"vollstaendig":     len(fehlt) == 0,
		"fehlende_angaben": fehlt,
		"bedeutung":        erklaerung,
		"seiten":           []string{"/impressum", "/datenschutz"},
		"hinweis": "Solange eine Pflichtangabe fehlt, antworten beide Seiten mit 404. " +
			"Ein unvollstaendiges Impressum ist schlechter als keines: es erweckt den " +
			"Anschein, die Pflicht sei erfuellt.",
	})
}
