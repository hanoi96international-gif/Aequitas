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
		"prinzip", "warum", "vergleich", "zugang", "forbiz", "news", "gruender", "faqkurz", "cta"})
	ideeHTML = baueSeite("/idee", "Aequitas — Why Aequitas", seitenKopfIdee, []string{
		"problem", "funktionen", "menge", "vision"})
	economyHTML = baueSeite("/economy", "Aequitas — How it works", seitenKopfEco, []string{
		"economy", "people", "ubi", "umlauf", "compare", "examples", "fairness"})
	businessHTML = baueSeite("/business", "Aequitas — For businesses", seitenKopfBiz, []string{
		"business", "join", "bexamples", "rules", "loopholes"})
	mitmachenHTML = baueSeite("/mitmachen", "Aequitas — Join", seitenKopfJoin, []string{
		"how", "forppl", "faq", "social"})
	transparenzHTML = baueSeite("/transparenz", "Aequitas — Transparency", seitenKopfTrans, []string{
		"werkzeuge", "status", "roadmap", "open", "disclaimer"})
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

const seitenKopfIdee = `<section class="page-head">
  <div class="section-inner">
    <div class="pg-art pa-b" aria-hidden="true"><svg class="ico"><use href="#i-heart"/></svg></div>
    <a href="/" class="pg-back" data-i18n="pg-back">← Back to the overview</a>
    <h1 data-i18n="pg-why-h1">Why Aequitas</h1>
    <p class="section-sub" data-i18n="pg-why-sub">What is wrong with today's money, what money has to do, and why a money that belongs to every person equally is the answer.</p>
    <div class="toc" role="navigation" aria-label="On this page"><span class="toc-lbl" data-i18n="toc-label">On this page</span><a href="#problem" data-i18n="toc-problem">The problem</a><a href="#funktionen" data-i18n="toc-fn">Functions of money</a><a href="#menge" data-i18n="toc-mg">Money supply</a><a href="#vision" data-i18n="toc-vi">Vision</a></div>
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

const seitenKopfJoin = `<section class="page-head">
  <div class="section-inner">
    <div class="pg-art pa-g" aria-hidden="true"><svg class="ico"><use href="#i-users"/></svg></div>
    <a href="/" class="pg-back" data-i18n="pg-back">← Back to the overview</a>
    <h1 data-i18n="pg-join-h1">Join</h1>
    <p class="section-sub" data-i18n="pg-join-sub">Register in three steps, receive your 1,000 AEQ and a basic income every day. No bank account needed.</p>
    <div class="toc" role="navigation" aria-label="On this page"><span class="toc-lbl" data-i18n="toc-label">On this page</span><a href="#how" data-i18n="toc-steps">Three steps</a><a href="#forppl" data-i18n="toc-share">Your share</a><a href="#faq" data-i18n="toc-faq">Questions</a></div>
  </div>
</section>
`

const seitenKopfTrans = `<section class="page-head">
  <div class="section-inner">
    <div class="pg-art pa-b" aria-hidden="true"><svg class="ico"><use href="#i-scale"/></svg></div>
    <a href="/" class="pg-back" data-i18n="pg-back">← Back to the overview</a>
    <h1 data-i18n="pg-trans-h1">Transparency</h1>
    <p class="section-sub" data-i18n="pg-trans-sub">Live figures from the chain, the open tools, where Aequitas stands, and what is honestly still open.</p>
    <div class="toc" role="navigation" aria-label="On this page"><span class="toc-lbl" data-i18n="toc-label">On this page</span><a href="#werkzeuge" data-i18n="toc-live">Live</a><a href="#status" data-i18n="toc-status">Status</a><a href="#roadmap" data-i18n="toc-road">Roadmap</a><a href="#open" data-i18n="toc-open">Open points</a></div>
  </div>
</section>
`
