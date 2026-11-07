package keeper

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var seitenFuerTest = map[string]string{
	"/":            landingHTML,
	"/idee":        ideeHTML,
	"/economy":     economyHTML,
	"/business":    businessHTML,
	"/mitmachen":   mitmachenHTML,
	"/transparenz": transparenzHTML,
}

// Jeder Abschnitt der Quelle landet auf genau einer Seite. Ein neuer
// Abschnitt, den niemand einer Seite zuordnet, waere sonst unsichtbar.
func TestSeiten_JederAbschnittGenauEinmal(t *testing.T) {
	re := regexp.MustCompile(`<section id="([a-z-]+)"`)
	for _, m := range re.FindAllStringSubmatch(landingQuelle, -1) {
		id := m[1]
		n := 0
		for _, html := range seitenFuerTest {
			n += strings.Count(html, `<section id="`+id+`"`)
		}
		if n != 1 {
			t.Errorf("Abschnitt %q steht auf %d Seiten, erwartet genau 1", id, n)
		}
	}
}

// Jede Seite hat genau eine Hauptueberschrift, ein <main> und markiert ihren
// eigenen Reiter als aktiv.
var aktiverReiter = regexp.MustCompile(`class="tab active[ "]`)

func TestSeiten_Grundgeruest(t *testing.T) {
	for pfad, html := range seitenFuerTest {
		if c := strings.Count(html, "<h1"); c != 1 {
			t.Errorf("%s: %d <h1>, erwartet 1", pfad, c)
		}
		if !strings.Contains(html, "<main>") || !strings.Contains(html, "</main>") {
			t.Errorf("%s: <main> fehlt", pfad)
		}
		if c := len(aktiverReiter.FindAllString(html, -1)); c != 1 {
			t.Errorf("%s: %d aktive Reiter", pfad, c)
		}
		if !strings.Contains(html, `<a href="`+pfad+`" class="tab active`) {
			t.Errorf("%s: eigener Reiter nicht aktiv", pfad)
		}
	}
}

// Jeder Schluessel, den eine Seite benutzt, steht in allen zwoelf Sprachen
// von landing.js. Fehlt er, bleibt der englische Text stehen, ohne dass es
// jemand merkt.
func TestSeiten_AlleSchluesselInAllenSprachen(t *testing.T) {
	benutzt := map[string]bool{}
	re := regexp.MustCompile(`data-i18n="([^"]+)"`)
	for _, html := range seitenFuerTest {
		for _, m := range re.FindAllStringSubmatch(html, -1) {
			benutzt[m[1]] = true
		}
	}
	block := regexp.MustCompile(`(?m)^([a-z]{2}): \{$`)
	idx := block.FindAllStringSubmatchIndex(landingJS, -1)
	if len(idx) != 12 {
		t.Fatalf("landing.js: %d Sprachbloecke, erwartet 12", len(idx))
	}
	for n, m := range idx {
		lang := landingJS[m[2]:m[3]]
		ende := len(landingJS)
		if n+1 < len(idx) {
			ende = idx[n+1][0]
		}
		teil := landingJS[m[1]:ende]
		var fehlt []string
		for k := range benutzt {
			if !strings.Contains(teil, `"`+k+`":`) {
				fehlt = append(fehlt, k)
			}
		}
		sort.Strings(fehlt)
		if len(fehlt) > 0 {
			t.Errorf("%s: fehlende Schluessel %v", lang, fehlt)
		}
	}
}

// Die Seiten laden landing.js (mit den Uebersetzungen) unter einer URL, die
// sich mit dem Inhalt aendert. Unter der blanken URL hielt der Browser nach
// einem Deploy eine Stunde lang das alte Skript, und setLang() schrieb die
// alten Texte ueber das neue HTML (25.09.2026, nach PR #188).
func TestSeiten_LandingJSVersioniert(t *testing.T) {
	want := `src="/landing.js?v=` + landingJSVersion + `"`
	for pfad, html := range seitenFuerTest {
		if strings.Count(html, want) != 1 {
			t.Errorf("%s: verlinkt landing.js nicht versioniert (%s)", pfad, want)
		}
		if strings.Contains(html, `src="/landing.js"`) {
			t.Errorf("%s: verlinkt noch die blanke /landing.js", pfad)
		}
	}
	a := &APIServer{}
	for url, cache := range map[string]string{
		"/landing.js?v=" + landingJSVersion: "public, max-age=31536000, immutable",
		"/landing.js":                       "no-cache",
		"/landing.js?v=00000000":            "no-cache",
	} {
		w := httptest.NewRecorder()
		a.handleLandingJS(w, httptest.NewRequest("GET", url, nil))
		if got := w.Header().Get("Cache-Control"); got != cache {
			t.Errorf("%s: Cache-Control %q, erwartet %q", url, got, cache)
		}
		if w.Body.String() != landingJS {
			t.Errorf("%s: liefert nicht landing.js aus", url)
		}
	}
}

// Die Website darf den einzigen Vergleichsdienst nicht "unabhaengig" nennen:
// er gehoert dem Betreiber (Einzelbetrieb seit 03.10.2026). Und sie darf ein
// Grundeinkommen "jeden Tag" nicht versprechen, ohne zu sagen, dass es heute
// fast nichts ist (live 07.11.2026: 0,000295 AEQ je Mensch in 30 Tagen).
func TestLanding_KeinUnabhaengigerEinzeldienst_UndEhrlichesGrundeinkommen(t *testing.T) {
	for _, falsch := range []string{
		"Ein unabhängiger Vergleichsdienst (derzeit einer)",
		"An independent matching service (currently one)",
	} {
		if strings.Contains(landingJS, falsch) {
			t.Errorf("Website behauptet noch: %q", falsch)
		}
	}
	for _, muss := range []string{
		"Ein Vergleichsdienst des Betreibers",
		"A matching service run by the operator",
		"Heute ist das noch fast nichts",
		"today that is almost nothing",
	} {
		if !strings.Contains(landingJS, muss) {
			t.Errorf("fehlt auf der Website: %q", muss)
		}
	}
}

// Die Texte des Gleichgewichts (GG in landing.js) gelten, sobald die Kette es
// meldet. Jede der zwoelf Sprachen braucht dann dieselben Schluessel wie
// "de" -- fehlt einer, bliebe in dieser Sprache der Text der alten Regeln
// stehen, ohne dass es jemand merkt. Jeder Schluessel muss ein echter der
// Seite sein, und {ab} (der Stichtag aus der Kette) steht in jeder Sprache
// genau dort, wo er im Deutschen steht.
func TestLanding_GleichgewichtInAllenSprachen(t *testing.T) {
	a := strings.Index(landingJS, "const GG = {\n")
	if a < 0 {
		t.Fatal("landing.js: keine Tabelle GG")
	}
	b := strings.Index(landingJS[a:], "\n};\n")
	if b < 0 {
		t.Fatal("landing.js: Tabelle GG ohne Ende")
	}
	var gg map[string]map[string]string
	roh := strings.TrimSuffix(strings.TrimPrefix(landingJS[a:a+b+3], "const GG = "), ";")
	// Die Tabelle ist JSON bis auf die Kommas am Ende jedes Blocks.
	roh = regexp.MustCompile(`,(\s*})`).ReplaceAllString(roh, "$1")
	if err := json.Unmarshal([]byte(roh), &gg); err != nil {
		t.Fatalf("GG ist kein lesbares Objekt: %v", err)
	}
	benutzt := map[string]bool{}
	for _, html := range seitenFuerTest {
		for _, m := range regexp.MustCompile(`data-i18n="([^"]+)"`).FindAllStringSubmatch(html, -1) {
			benutzt[m[1]] = true
		}
	}
	de := gg["de"]
	if len(gg) != 12 || len(de) < 70 {
		t.Fatalf("GG: %d Sprachen, %d deutsche Texte", len(gg), len(de))
	}
	for k := range de {
		if !benutzt[k] {
			t.Errorf("GG.de.%s: kein Schluessel der Seiten", k)
		}
	}
	for lang, texte := range gg {
		for k, v := range de {
			w, da := texte[k]
			if !da || strings.TrimSpace(w) == "" {
				t.Errorf("GG.%s: %s fehlt", lang, k)
				continue
			}
			if strings.Contains(v, "{ab}") != strings.Contains(w, "{ab}") {
				t.Errorf("GG.%s.%s: {ab} anders als im Deutschen", lang, k)
			}
			if strings.Count(v, "<strong>") != strings.Count(w, "<strong>") {
				t.Errorf("GG.%s.%s: Hervorhebung anders als im Deutschen", lang, k)
			}
		}
		if len(texte) != len(de) {
			t.Errorf("GG.%s: %d Texte, Deutsch %d", lang, len(texte), len(de))
		}
	}
}
