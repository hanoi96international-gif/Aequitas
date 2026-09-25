package keeper

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

var seitenFuerTest = map[string]string{
	"/":         landingHTML,
	"/economy":  economyHTML,
	"/business": businessHTML,
	"/roadmap":  roadmapHTML,
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
func TestSeiten_Grundgeruest(t *testing.T) {
	for pfad, html := range seitenFuerTest {
		if c := strings.Count(html, "<h1"); c != 1 {
			t.Errorf("%s: %d <h1>, erwartet 1", pfad, c)
		}
		if !strings.Contains(html, "<main>") || !strings.Contains(html, "</main>") {
			t.Errorf("%s: <main> fehlt", pfad)
		}
		if c := strings.Count(html, `class="tab active"`); c != 1 {
			t.Errorf("%s: %d aktive Reiter", pfad, c)
		}
		if !strings.Contains(html, `<a href="`+pfad+`" class="tab active">`) {
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
