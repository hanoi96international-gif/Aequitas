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
		"kreis", "forbiz", "forppl", "how", "news", "faqkurz", "status", "cta"})
	peopleHTML = baueSeite("/people", "Aequitas — For people", seitenKopfPpl, []string{
		"people", "ubi", "examples"})
	economyHTML = baueSeite("/economy", "Aequitas — How it works", seitenKopfEco, []string{
		"economy", "compare", "fairness"})
	businessHTML = baueSeite("/business", "Aequitas — For businesses", seitenKopfBiz, []string{
		"business", "join", "bexamples", "rules", "loopholes"})
	roadmapHTML = baueSeite("/roadmap", "Aequitas — Roadmap and questions", seitenKopfRoad, []string{
		"roadmap", "open", "faq", "social", "disclaimer"})
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
		// Reiter koennen weitere Klassen tragen ("tab tab-biz"), darum wird
		// "active" in die Klassenliste eingefuegt statt ein festes Muster
		// ersetzt.
		kopf = strings.Replace(kopf, `<a href="/" class="tab active"`, `<a href="/" class="tab"`, 1)
		kopf = strings.Replace(kopf, `<a href="`+pfad+`" class="tab`, `<a href="`+pfad+`" class="tab active`, 1)
		if a, e := strings.Index(kopf, "<title>"), strings.Index(kopf, "</title>"); a >= 0 && e > a {
			kopf = kopf[:a] + "<title>" + titel + kopf[e:]
		}
		b.WriteString(kopf)
		b.WriteString(seitenKopf)
	}
	for n, id := range ids {
		s := landingAbschnitt(id)
		// Abschnitte mit eigener Klasse (Baender wie "band-biz") behalten
		// ihren Hintergrund.
		if n%2 == 1 && !strings.Contains(s[:strings.Index(s, ">")], "class=") {
			s = strings.Replace(s, `<section id="`+id+`"`, `<section id="`+id+`" class="alt"`, 1)
		}
		b.WriteString(s)
		b.WriteString("\n")
	}
	b.WriteString(strings.Replace(fuss, `src="/landing.js"`, `src="/landing.js?v=`+landingJSVersion+`"`, 1))
	return b.String()
}

// handleSeite liefert eine der zusammengesetzten Seiten mit denselben
// Kopfzeilen wie die Startseite.
func (a *APIServer) handleSeite(html string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.schreibeLandingSeite(w, r, html)
	}
}

const seitenKopfPpl = `<section class="page-head">
  <div class="section-inner">
    <div class="pg-art pa-b" aria-hidden="true"><svg class="ico"><use href="#i-users"/></svg></div>
    <a href="/" class="pg-back" data-i18n="pg-back">← Back to the overview</a>
    <h1 data-i18n="pg-ppl-h1">For people</h1>
    <p class="section-sub" data-i18n="pg-ppl-sub">What Aequitas promises every person, how the daily basic income works and what you pay, with worked examples.</p>
    <div class="pg-btns"><a href="/register" class="btn-primary" data-i18n="how-link">Register and claim your 1,000 AEQ →</a></div>
    <div class="toc" role="navigation" aria-label="On this page"><span class="toc-lbl" data-i18n="toc-label">On this page</span><a href="#people" data-i18n="toc-prom">Promises</a><a href="#ubi" data-i18n="toc-ubi">Basic income</a><a href="#examples" data-i18n="toc-ex">Examples</a></div>
  </div>
</section>
`

const seitenKopfEco = `<section class="page-head">
  <div class="section-inner">
    <div class="pg-art pa-g" aria-hidden="true"><svg class="ico"><use href="#i-refresh"/></svg></div>
    <a href="/" class="pg-back" data-i18n="pg-back">← Back to the overview</a>
    <h1 data-i18n="pg-eco-h1">How it works</h1>
    <p class="section-sub" data-i18n="pg-eco-sub">One cycle, three kinds of account, one set of rules, and why the money stays fair.</p>
    <div class="toc" role="navigation" aria-label="On this page"><span class="toc-lbl" data-i18n="toc-label">On this page</span><a href="#economy" data-i18n="toc-eco">Economy</a><a href="#compare" data-i18n="toc-cmp">Account types</a><a href="#fairness" data-i18n="toc-fair">Fairness</a></div>
  </div>
</section>
`

const seitenKopfBiz = `<section class="page-head page-biz">
  <div class="section-inner">
    <div class="pg-art pa-o" aria-hidden="true"><svg class="ico"><use href="#i-store"/></svg></div>
    <a href="/" class="pg-back" data-i18n="pg-back">← Back to the overview</a>
    <h1 data-i18n="pg-biz-h1">Aequitas for businesses</h1>
    <p class="section-sub" data-i18n="pg-biz-sub">Take payments without card fees, pay wages free of charge and pass the money on. Only money left idle costs.</p>
    <div class="pg-btns"><a href="https://t.me/aequitasmoney" class="btn-primary" rel="noopener noreferrer" target="_blank" data-i18n="pg-biz-cta">Join the pilot</a><a href="https://github.com/hanoi96international-gif/Aequitas/blob/main/docs/UNTERNEHMEN_KONZEPT.md" class="btn-secondary" rel="noopener" data-i18n="biz-link">Read the full concept →</a></div>
    <div class="toc" role="navigation" aria-label="On this page"><span class="toc-lbl" data-i18n="toc-label">On this page</span><a href="#business" data-i18n="toc-adv">Advantages</a><a href="#join" data-i18n="toc-join">Taking part</a><a href="#bexamples" data-i18n="toc-bex">Costs</a><a href="#rules" data-i18n="toc-rules">Rules</a><a href="#loopholes" data-i18n="toc-lh">Protection against abuse</a></div>
  </div>
</section>
`

const seitenKopfRoad = `<section class="page-head">
  <div class="section-inner">
    <div class="pg-art pa-b" aria-hidden="true"><svg class="ico"><use href="#i-rocket"/></svg></div>
    <a href="/" class="pg-back" data-i18n="pg-back">← Back to the overview</a>
    <h1 data-i18n="pg-road-h1">Roadmap and questions</h1>
    <p class="section-sub" data-i18n="pg-road-sub">Where Aequitas stands, what comes next, and what is honestly still open.</p>
    <div class="toc" role="navigation" aria-label="On this page"><span class="toc-lbl" data-i18n="toc-label">On this page</span><a href="#roadmap" data-i18n="toc-road">Roadmap</a><a href="#open" data-i18n="toc-open">Open points</a><a href="#faq" data-i18n="toc-faq">Questions</a></div>
  </div>
</section>
`
