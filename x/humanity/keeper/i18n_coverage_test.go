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

	// Die Leitfaden-Bereiche, per Panel-ID und Klammerzählung.
	//
	// Beide Leitfäden sind per Entwurf englisch (Sprachhinweis + übersetzte
	// PDFs), ihr Inhalt wird hier also nicht gezählt.
	//
	// FRÜHER hing das an der Kommentarzeile "<!-- ZWEITE ROLLE" als Anfang und
	// am Ende von net-runnode als Ende. Am 26.08.2026 sind die beiden
	// Leitfäden in getrennte Rubriken gewandert — damit lag der Anfang HINTER
	// dem Ende, das Fenster war leer, und der englische Node-Guide schlug mit
	// 118 Fundstellen auf. Der Test hatte recht zu melden; er meldete nur
	// etwas, das absichtlich so ist.
	//
	// Deshalb jetzt an den Panel-IDs statt an einem Kommentar: die sind das,
	// was die Bereiche ausmacht, und sie überleben ein Verschieben.
	guideBereiche := panelBereiche(s, "net-runnode", "net-verifier")
	i18nEltern := i18nBereiche(s)

	var ohne []string
	for _, m := range i18nElementRe.FindAllStringSubmatchIndex(s, -1) {
		start := m[0]
		if inBereich(start, guideBereiche) {
			continue
		}
		// Nachfahre eines Elements, das SELBST einen Schluessel traegt:
		// sein Text steht mit im Uebersetzungsstring des Elternteils.
		if inBereich(start, i18nEltern) {
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

// panelBereiche liefert zu jeder Panel-ID den Bereich [Anfang, Ende) im
// Dokument, per Klammerzählung über <div>/</div>.
//
// Klammerzählung und nicht "bis zum nächsten </div>": ein Panel enthält
// Dutzende verschachtelter Divs, und die Textsuche liefert das erste innerste.
// Genau dieser Fehler führte dazu, dass Shell-Befehle zur Übersetzung
// vorgeschlagen wurden.
func panelBereiche(s string, ids ...string) [][2]int {
	divRe := regexp.MustCompile(`<div\b|</div>`)
	var out [][2]int
	for _, id := range ids {
		p := strings.Index(s, `<div id="`+id+`"`)
		if p < 0 {
			continue
		}
		tiefe := 0
		ende := len(s)
		for _, m := range divRe.FindAllStringIndex(s[p:], -1) {
			if strings.HasPrefix(s[p+m[0]:p+m[1]], "<div") {
				tiefe++
			} else {
				tiefe--
			}
			if tiefe == 0 {
				ende = p + m[1]
				break
			}
		}
		out = append(out, [2]int{p, ende})
	}
	return out
}

// i18nBereiche liefert die Spannweiten aller Elemente, die SELBST ein
// data-i18n tragen.
//
// # WARUM ES DAS BRAUCHT
//
// Ein uebersetzter Block enthaelt fast immer Auszeichnung -- <strong>, <em>.
// Deren Text steht MIT im Uebersetzungsstring des Elternelements: setLang
// schreibt el.innerHTML = t[key] und ersetzt damit den ganzen Teilbaum. Diese
// Stellen sind also in allen zwoelf Sprachen uebersetzt.
//
// Der Test sah aber nur das <strong> selbst, fand dort kein data-i18n und
// meldete es. Aufgefallen ist das erst am 27.08.2026: bis dahin kam solche
// Auszeichnung nur in den beiden Leitfaeden vor, und die sind ohnehin
// ausgenommen, weil sie absichtlich englisch sind. Die Coordinator-Rubrik war
// der erste WIRKLICH uebersetzte Bereich mit Auszeichnung darin -- und die
// fuenf Meldungen waren samt und sonders falsch.
//
// Die Ausnahmeliste zu erweitern waere die falsche Antwort gewesen: sie sagt
// "dieser Bereich ist absichtlich englisch", und das stimmt hier nicht.
func i18nBereiche(s string) [][2]int {
	zwischenspeicher := map[string]*regexp.Regexp{}
	var out [][2]int
	for i := 0; ; {
		p := strings.Index(s[i:], "data-i18n=")
		if p < 0 {
			break
		}
		p += i
		i = p + len("data-i18n=")

		// Zurueck zum Anfang des Tags, um seinen Namen zu bekommen.
		auf := strings.LastIndex(s[:p], "<")
		if auf < 0 {
			continue
		}
		rest := s[auf+1:]
		n := strings.IndexAny(rest, " \t\r\n>")
		if n <= 0 {
			continue
		}
		name := rest[:n]

		re := zwischenspeicher[name]
		if re == nil {
			re = regexp.MustCompile(`<` + regexp.QuoteMeta(name) + `\b|</` + regexp.QuoteMeta(name) + `>`)
			zwischenspeicher[name] = re
		}

		// Spannweite per Tiefenzaehlung fuer genau diesen Tagnamen.
		tiefe := 0
		ende := len(s)
		for _, m := range re.FindAllStringIndex(s[auf:], -1) {
			if strings.HasPrefix(s[auf+m[0]:auf+m[1]], "</") {
				tiefe--
			} else {
				tiefe++
			}
			if tiefe == 0 {
				ende = auf + m[1]
				break
			}
		}
		out = append(out, [2]int{auf, ende})
	}
	return out
}

func inBereich(pos int, bereiche [][2]int) bool {
	for _, b := range bereiche {
		if pos >= b[0] && pos < b[1] {
			return true
		}
	}
	return false
}
