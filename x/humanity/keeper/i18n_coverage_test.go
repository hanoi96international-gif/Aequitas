package keeper

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Sichtbarer Text OHNE data-i18n bleibt in allen zwölf Sprachen englisch.
//
// WARUM ES DIESEN TEST BRAUCHT
//
// Die beiden vorhandenen Wächter prüfen etwas anderes:
//
//	TestI18nLocaleKeysMatchEnglish     hat jeder VORHANDENE Schlüssel eine
//	                                   Übersetzung in jeder Sprache?
//	TestI18nMarkupKeysExistInDictionary ist jeder BENUTZTE Schlüssel definiert?
//
// Keiner von beiden fragt: hat sichtbarer Text überhaupt einen Schlüssel?
// Genau dort lag die Lücke. Am 25.08.2026 gemessen: 392 Stellen ohne
// data-i18n, die damit in allen zwölf Sprachen englisch blieben — und nichts
// meldete es. Alle zwölf Sprachdateien waren dabei „vollständig".
//
// Ohne diesen Test wächst die Lücke mit jedem neuen Abschnitt wieder nach,
// und zwar unsichtbar: die Seite sieht in jeder Sprache aus, als sei sie
// übersetzt, solange man die betroffene Stelle nicht kennt.
//
// WAS AUSDRÜCKLICH NICHT GEZÄHLT WIRD
//
//   - Die beiden Leitfäden (Node und Bio-Verifier). Sie sind per Entwurf
//     englisch, mit Sprachhinweis und übersetzten PDFs.
//   - Code, Adressen, Platzhalter, Formeln. Ein übersetzter Shell-Befehl ist
//     kein Schönheitsfehler, sondern eine Anleitung, die nicht mehr
//     funktioniert.
//   - Handles wie @AequitasMoney. Sie haben keine Sprache.

var (
	i18nElementRe  = regexp.MustCompile(`(?s)<([a-z0-9]+)([^>]*)>([^<>]{12,})</([a-z0-9]+)>`)
	i18nBuchstabe  = regexp.MustCompile(`[A-Za-z]{4}`)
	i18nNurZeichen = regexp.MustCompile(`^[\s{}();.,:/*=+\-\d]+$`)
	i18nCode       = regexp.MustCompile(`^(0x|#\s|\$|sudo |docker |curl |apt |git |npm |go |python)`)
	i18nKonstante  = regexp.MustCompile(`[A-Z_]{4,}=|_[A-Z]{3,}`)
)

// i18nAusnahmen sind Zeichenketten, die bewusst unübersetzt bleiben. Jeder
// Eintrag braucht einen Grund — ein Eintrag ohne Grund ist der Weg, auf dem
// die Lücke zurückkommt.
var i18nAusnahmen = map[string]string{
	"@AequitasMoney":     "Handle, keine Sprache",
	"t.me/aequitasmoney": "URL, keine Sprache",
}

func TestJederSichtbareTextHatEinenUebersetzungsschluessel(t *testing.T) {
	roh, err := os.ReadFile("assets/explorer.html")
	if err != nil {
		t.Fatalf("assets/explorer.html nicht lesbar: %v", err)
	}
	s := string(roh)

	// Der Leitfaden-Bereich: vom Bio-Verifier-Block bis zum Ende des Panels
	// net-runnode, per Klammerzählung. Eine Textsuche nach dem nächsten
	// </div> ergäbe ein zu kleines Fenster — genau dieser Fehler führte am
	// 26.08.2026 dazu, dass Shell-Befehle zum Übersetzen vorgeschlagen wurden.
	g0 := strings.Index(s, "<!-- ZWEITE ROLLE: Bio-Verifier")
	p := strings.Index(s, `<div id="net-runnode"`)
	g1 := len(s)
	if p >= 0 {
		tiefe := 0
		for _, m := range regexp.MustCompile(`<div\b|</div>`).FindAllStringIndex(s[p:], -1) {
			if strings.HasPrefix(s[p+m[0]:p+m[1]], "<div") {
				tiefe++
			} else {
				tiefe--
			}
			if tiefe == 0 {
				g1 = p + m[1]
				break
			}
		}
	}

	var ohne []string
	for _, m := range i18nElementRe.FindAllStringSubmatchIndex(s, -1) {
		start := m[0]
		if g0 >= 0 && start >= g0 && start <= g1 {
			continue
		}
		attrs := s[m[4]:m[5]]
		tagAuf, tagZu := s[m[2]:m[3]], s[m[8]:m[9]]
		if tagAuf != tagZu {
			continue
		}
		if strings.Contains(attrs, "data-i18n") || strings.Contains(attrs, "font-mono") {
			continue
		}
		text := strings.TrimSpace(s[m[6]:m[7]])
		switch {
		case !i18nBuchstabe.MatchString(text),
			i18nNurZeichen.MatchString(text),
			i18nCode.MatchString(text),
			i18nKonstante.MatchString(text):
			continue
		}
		if _, erlaubt := i18nAusnahmen[text]; erlaubt {
			continue
		}
		zeile := strings.Count(s[:start], "\n") + 1
		if len(text) > 80 {
			text = text[:80] + "…"
		}
		ohne = append(ohne, fmt.Sprintf("Zeile %d: %s", zeile, text))
	}

	if len(ohne) > 0 {
		sort.Strings(ohne)
		t.Errorf("%d sichtbare Textstelle(n) ohne data-i18n — sie bleiben in ALLEN %d Sprachen "+
			"englisch, und keiner der anderen i18n-Tests bemerkt das:\n  %s",
			len(ohne), len(i18nLocales), strings.Join(ohne, "\n  "))
	}
}
