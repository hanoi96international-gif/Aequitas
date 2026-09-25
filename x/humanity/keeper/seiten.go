package keeper

import (
	"net/http"
	"strings"
)

// Die Startseite war eine einzige lange Seite: Wirtschaft, Grundeinkommen,
// Kontoarten, Beispiele, Unternehmen, Laeden, Schutz vor Missbrauch, Fahrplan
// und Fragen untereinander. Das war zu viel auf einmal. Deshalb gibt es jetzt
// eine kurze Startseite mit vier Wegweisern und drei Unterseiten, jede mit
// ihren eigenen Unterrubriken.
//
// Alle Abschnitte stehen weiterhin genau einmal in landingQuelle (landing.go).
// Die Seiten werden daraus beim Start zusammengesetzt: Kopf (bis <main>),
// die gewuenschten Abschnitte in fester Reihenfolge, Fuss (ab </main>). So
// gibt es keinen Abschnitt doppelt, und landing.js mit seiner
// Uebersetzungstabelle bedient alle vier Seiten.
//
// Jeder zweite Abschnitt bekommt die Klasse "alt" (Karte als Hintergrund).
// Frueher stand das als Stil fest an einzelnen Abschnitten; mit wechselnder
// Reihenfolge je Seite lagen dann zwei gleiche Hintergruende nebeneinander.

var (
	landingHTML = baueSeite("/", "", "", []string{
		"overview", "how", "fairness", "disclaimer", "rest", "social"})
	economyHTML = baueSeite("/economy", "Aequitas — The economy", seitenKopfEco, []string{
		"economy", "ubi", "compare", "people", "examples"})
	businessHTML = baueSeite("/business", "Aequitas — Businesses and shops", seitenKopfBiz, []string{
		"business", "shops", "loopholes"})
	roadmapHTML = baueSeite("/roadmap", "Aequitas — Roadmap and questions", seitenKopfRoad, []string{
		"roadmap", "open", "faq"})
)

// landingTeile zerlegt landingQuelle in Kopf, Hero (alles vor dem ersten
// Abschnitt mit id) und Fuss.
func landingTeile() (kopf, hero, fuss string) {
	const mainAuf, mainZu = "<main>\n", "</main>"
	i := strings.Index(landingQuelle, mainAuf)
	j := strings.Index(landingQuelle, mainZu)
	if i < 0 || j < i {
		panic("landingQuelle: <main> fehlt")
	}
	kopf = landingQuelle[:i+len(mainAuf)]
	fuss = landingQuelle[j:]
	koerper := landingQuelle[i+len(mainAuf) : j]
	k := strings.Index(koerper, `<section id="`)
	if k < 0 {
		panic("landingQuelle: kein Abschnitt mit id")
	}
	return kopf, koerper[:k], fuss
}

// landingAbschnitt liefert den Abschnitt <section id="id"...>...</section>.
// Abschnitte sind in landingQuelle nicht verschachtelt.
func landingAbschnitt(id string) string {
	auf := `<section id="` + id + `"`
	i := strings.Index(landingQuelle, auf)
	if i < 0 {
		panic("landingQuelle: Abschnitt fehlt: " + id)
	}
	const zu = "</section>\n"
	j := strings.Index(landingQuelle[i:], zu)
	if j < 0 {
		panic("landingQuelle: Abschnitt ohne Ende: " + id)
	}
	return landingQuelle[i : i+j+len(zu)]
}

func baueSeite(pfad, titel, seitenKopf string, ids []string) string {
	kopf, hero, fuss := landingTeile()
	var b strings.Builder
	if pfad == "/" {
		b.WriteString(kopf)
		b.WriteString(hero)
	} else {
		kopf = strings.Replace(kopf, `<a href="/" class="tab active">`, `<a href="/" class="tab">`, 1)
		kopf = strings.Replace(kopf, `<a href="`+pfad+`" class="tab">`, `<a href="`+pfad+`" class="tab active">`, 1)
		if a, e := strings.Index(kopf, "<title>"), strings.Index(kopf, "</title>"); a >= 0 && e > a {
			kopf = kopf[:a] + "<title>" + titel + kopf[e:]
		}
		b.WriteString(kopf)
		b.WriteString(seitenKopf)
	}
	for n, id := range ids {
		s := landingAbschnitt(id)
		if n%2 == 1 {
			s = strings.Replace(s, `<section id="`+id+`"`, `<section id="`+id+`" class="alt"`, 1)
		}
		b.WriteString(s)
		b.WriteString("\n")
	}
	b.WriteString(fuss)
	return b.String()
}

// handleSeite liefert eine der zusammengesetzten Seiten mit denselben
// Kopfzeilen wie die Startseite.
func (a *APIServer) handleSeite(html string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.schreibeLandingSeite(w, r, html)
	}
}

const seitenKopfEco = `<section class="page-head">
  <div class="section-inner">
    <a href="/" class="pg-back" data-i18n="pg-back">← Back to the overview</a>
    <h1 data-i18n="pg-eco-h1">The economy</h1>
    <p class="section-sub" data-i18n="pg-eco-sub">One cycle for everyone: people receive the basic income, businesses pass money on, and whatever sits idle returns to all.</p>
    <div class="toc" role="navigation" aria-label="On this page"><span class="toc-lbl" data-i18n="toc-label">On this page</span><a href="#economy" data-i18n="toc-eco">Economy</a><a href="#ubi" data-i18n="toc-ubi">Basic income</a><a href="#compare" data-i18n="toc-cmp">Account types</a><a href="#people" data-i18n="toc-ppl">For people</a><a href="#examples" data-i18n="toc-ex">Examples</a></div>
  </div>
</section>

`

const seitenKopfBiz = `<section class="page-head">
  <div class="section-inner">
    <a href="/" class="pg-back" data-i18n="pg-back">← Back to the overview</a>
    <h1 data-i18n="pg-biz-h1">Businesses and shops</h1>
    <p class="section-sub" data-i18n="pg-biz-sub">Businesses may accept, hold and spend AEQ. Passing it on is free, leaving it idle costs.</p>
    <div class="toc" role="navigation" aria-label="On this page"><span class="toc-lbl" data-i18n="toc-label">On this page</span><a href="#business" data-i18n="toc-biz">For businesses</a><a href="#shops" data-i18n="toc-shop">For shops</a><a href="#loopholes" data-i18n="toc-lh">Protection against abuse</a></div>
  </div>
</section>

`

const seitenKopfRoad = `<section class="page-head">
  <div class="section-inner">
    <a href="/" class="pg-back" data-i18n="pg-back">← Back to the overview</a>
    <h1 data-i18n="pg-road-h1">Roadmap and questions</h1>
    <p class="section-sub" data-i18n="pg-road-sub">Where Aequitas stands, what comes next, and what is honestly still open.</p>
    <div class="toc" role="navigation" aria-label="On this page"><span class="toc-lbl" data-i18n="toc-label">On this page</span><a href="#roadmap" data-i18n="toc-road">Roadmap</a><a href="#open" data-i18n="toc-open">Open points</a><a href="#faq" data-i18n="toc-faq">Questions</a></div>
  </div>
</section>

`
