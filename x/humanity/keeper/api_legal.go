package keeper

import (
	_ "embed"
	"net/http"
	"strings"
)

// Impressum (§ 5 DDG) und Datenschutzerklärung (Art. 13 DSGVO).
//
// WARUM DAS HIER FEHLTE
//
// Gemessen am 25.08.2026: /impressum und /datenschutz gab es nicht. Beide
// Adressen lieferten die SPA-Seite mit Status 200 — die Catch-all-Regel im
// Wurzel-Handler beantwortet jeden nicht zugeordneten Pfad so, damit die
// clientseitigen Routen der Oberfläche funktionieren. Für einen Menschen sah
// das aus wie "die Seite gibt es", und niemandem fiel auf, dass es sie nicht
// gab.
//
// Beides ist in Deutschland Pflicht, sobald eine Seite öffentlich erreichbar
// ist und — wie hier — eine App verteilt, die biometrische Daten erhebt.
//
// DIE SPERRE IST DER EIGENTLICHE INHALT DIESER DATEI
//
// Eine Datenschutzerklärung ohne die Identität des Verantwortlichen ist
// wertlos: die Betroffenen erfahren nicht, gegen wen sie ihre Rechte richten
// können. Ein Impressum ohne ladungsfähige Anschrift ist schlechter als
// keines — es erweckt den Anschein, die Pflicht sei erfüllt.
//
// Deshalb liefern die Handler unten NICHTS aus, solange die Vorlagen noch die
// Markierung PLATZHALTER enthalten. Sie antworten dann mit 404, so als gäbe
// es die Seite nicht — was zutrifft, denn ein Text mit Lücken an den Stellen,
// auf die es ankommt, ist keine Erklärung.
//
// Zum Freischalten genügt es, die eckigen Klammern in den beiden
// assets/-Dateien zu ersetzen. Die Seiten erscheinen dann von selbst, ohne
// dass hier etwas zu ändern wäre.

//go:embed assets/impressum.html
var impressumHTML string

//go:embed assets/datenschutz.html
var datenschutzHTML string

// platzhalterMarke steht in den Vorlagen überall dort, wo eine Angabe fehlt,
// die nur die verantwortliche Person machen kann.
const platzhalterMarke = "PLATZHALTER"

// istVollstaendig meldet, ob eine Vorlage ausgeliefert werden darf.
func istVollstaendig(inhalt string) bool {
	return !strings.Contains(inhalt, platzhalterMarke)
}

// legalCSS hält die beiden Seiten lesbar, ohne von explorer.css abzuhängen —
// ein Rechtstext soll auch dann noch lesbar sein, wenn die Oberfläche gerade
// umgebaut wird. Bewusst knapp und ohne Schriftarten von fremden Servern:
// wer eine Datenschutzerklärung liest, soll dabei nicht bei einem Dritten
// erfasst werden.
const legalCSS = `
:root{color-scheme:dark}
body{margin:0;padding:2.5rem 1.25rem;background:#0b0d12;color:#e6e8ee;
 font:16px/1.65 system-ui,-apple-system,Segoe UI,Roboto,sans-serif}
main{max-width:46rem;margin:0 auto}
h1{font-size:1.9rem;margin:0 0 1.5rem;color:#fff}
h2{font-size:1.25rem;margin:2.2rem 0 .6rem;color:#fff}
h3{font-size:1.05rem;margin:1.6rem 0 .4rem;color:#c8cde0}
p,li{margin:.6rem 0}
address{font-style:normal;margin:.6rem 0}
a{color:#8ea2ff}
table{border-collapse:collapse;width:100%;margin:.8rem 0}
th,td{text-align:left;padding:.5rem .6rem;border-bottom:1px solid #232838;vertical-align:top}
th{color:#c8cde0;font-weight:600}
.warnung{border-left:3px solid #e0a33e;padding-left:1rem}
.hinweis{color:#9aa3bd}
.fuss{margin-top:3rem;padding-top:1.2rem;border-top:1px solid #232838;color:#9aa3bd}
`

func (a *APIServer) serveLegal(w http.ResponseWriter, titel, inhalt string) {
	if !istVollstaendig(inhalt) {
		// Absichtlich 404 und nicht 503: die Seite existiert in dem Sinne,
		// auf den es ankommt, tatsächlich noch nicht.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("404 — Seite noch nicht verfügbar"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Kein Zwischenspeichern: ändert sich ein Rechtstext, muss die alte
	// Fassung sofort verschwinden.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html><html lang="de"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>` + titel + ` · Aequitas</title><style>` + legalCSS + `</style></head>` +
		`<body><main>` + inhalt + `</main></body></html>`))
}

func (a *APIServer) handleImpressum(w http.ResponseWriter, r *http.Request) {
	a.serveLegal(w, "Impressum", impressumHTML)
}

func (a *APIServer) handleDatenschutz(w http.ResponseWriter, r *http.Request) {
	a.serveLegal(w, "Datenschutzerklärung", datenschutzHTML)
}
