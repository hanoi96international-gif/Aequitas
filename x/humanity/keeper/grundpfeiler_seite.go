package keeper

import (
	_ "embed"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"sync"
)

// DIE GRUNDPFEILER ALS SEITE (06.10.2026).
//
// docs/GRUNDPFEILER.md legt das Fundament, auf dem alle Regeln stehen -- und
// lag bisher nur im Repository. /grundpfeiler zeigt es jedem, der die Seite
// des Knotens oeffnet.
//
// Der Text ist eine eingebettete Kopie (assets/grundpfeiler.md);
// TestGrundpfeiler_KopieGleichDemDokument haelt sie gleich mit
// docs/GRUNDPFEILER.md. Dargestellt wird er mit einem kleinen Markdown-Leser
// fuer genau die Formen, die das Dokument nutzt (Ueberschriften, Absaetze,
// Listen, Tabellen, Zitate, Trennlinien, **fett**, `Code`). Keine neue
// Abhaengigkeit, und jedes Zeichen des Textes wird maskiert, bevor Auszeichnung
// entsteht: auch ein Dokument mit <script> darin ergaebe nur Text. Die Seite
// laedt nichts nach und fuehrt nichts aus (Content-Security-Policy ohne
// Skripte).

//go:embed assets/grundpfeiler.md
var grundpfeilerMD string

var (
	grundpfeilerEinmal sync.Once
	grundpfeilerHTML   string
)

const grundpfeilerCSS = `
blockquote{margin:1rem 0;padding:.2rem 0 .2rem 1rem;border-left:3px solid #8ea2ff;color:#c8cde0}
code{font:.92em ui-monospace,SFMono-Regular,Menlo,monospace;background:#161a24;padding:.05rem .3rem;border-radius:4px}
hr{border:0;border-top:1px solid #232838;margin:2rem 0}
table{display:block;overflow-x:auto}
ul,ol{padding-left:1.4rem}
`

// handleGrundpfeiler GET /grundpfeiler
func (a *APIServer) handleGrundpfeiler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	grundpfeilerEinmal.Do(func() { grundpfeilerHTML = markdownZuHTML(grundpfeilerMD) })
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	setHSTS(w, r)
	fmt.Fprint(w, `<!doctype html><html lang="de"><head><meta charset="utf-8">`+
		`<meta name="viewport" content="width=device-width,initial-scale=1">`+
		`<title>Die Grundpfeiler · Aequitas</title><style>`+legalCSS+grundpfeilerCSS+`</style></head>`+
		`<body><main>`+grundpfeilerHTML+
		`<p class="fuss">Quelle: <code>docs/GRUNDPFEILER.md</code> im Repository. `+
		`<a href="/">Zur Startseite</a></p></main></body></html>`)
}

var (
	mdFett       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	mdTabTrenner = regexp.MustCompile(`^:?-{3,}:?$`)
	mdOListe     = regexp.MustCompile(`^(\d+)\.\s+(.*)$`)
)

// mdInline: maskiert den Text, dann `Code` und **fett**.
func mdInline(s string) string {
	teile := strings.Split(s, "`")
	var b strings.Builder
	for i, t := range teile {
		e := html.EscapeString(t)
		if i%2 == 1 && i < len(teile)-1 {
			b.WriteString("<code>" + e + "</code>")
			continue
		}
		if i%2 == 1 { // ungerade Zahl von Backticks: der letzte bleibt Text
			b.WriteString("`")
		}
		b.WriteString(mdFett.ReplaceAllString(e, "<strong>$1</strong>"))
	}
	return b.String()
}

// mdListenpunkt: "- x", "* x" oder "1. x" -- Einrueckung, Art, Text.
func mdListenpunkt(z string) (int, string, string, bool) {
	einr := len(z) - len(strings.TrimLeft(z, " "))
	t := strings.TrimSpace(z)
	if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") {
		return einr, "ul", strings.TrimSpace(t[2:]), true
	}
	if m := mdOListe.FindStringSubmatch(t); m != nil {
		return einr, "ol", m[2], true
	}
	return 0, "", "", false
}

func mdZellen(z string) []string {
	t := strings.TrimSpace(z)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	teile := strings.Split(t, "|")
	for i := range teile {
		teile[i] = strings.TrimSpace(teile[i])
	}
	return teile
}

// markdownZuHTML: die Formen aus docs/GRUNDPFEILER.md, sonst Absatztext.
func markdownZuHTML(md string) string {
	zeilen := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var b strings.Builder
	var absatz []string
	absatzEnde := func() {
		if len(absatz) > 0 {
			b.WriteString("<p>" + mdInline(strings.Join(absatz, " ")) + "</p>\n")
			absatz = nil
		}
	}
	for i := 0; i < len(zeilen); {
		z := zeilen[i]
		t := strings.TrimSpace(z)
		switch {
		case t == "":
			absatzEnde()
			i++
		case strings.HasPrefix(t, "#"):
			n := len(t) - len(strings.TrimLeft(t, "#"))
			if n > 6 || len(t) == n || t[n] != ' ' {
				absatz = append(absatz, t)
				i++
				continue
			}
			absatzEnde()
			fmt.Fprintf(&b, "<h%d>%s</h%d>\n", n, mdInline(strings.TrimSpace(t[n:])), n)
			i++
		case t == "---" || t == "***":
			absatzEnde()
			b.WriteString("<hr>\n")
			i++
		case strings.HasPrefix(t, "|"):
			absatzEnde()
			var reihen [][]string
			for ; i < len(zeilen) && strings.HasPrefix(strings.TrimSpace(zeilen[i]), "|"); i++ {
				reihen = append(reihen, mdZellen(zeilen[i]))
			}
			b.WriteString("<table>\n")
			for k, reihe := range reihen {
				if k == 1 && len(reihe) > 0 && mdTabTrenner.MatchString(reihe[0]) {
					continue
				}
				tag := "td"
				if k == 0 {
					tag = "th"
				}
				b.WriteString("<tr>")
				for _, c := range reihe {
					b.WriteString("<" + tag + ">" + mdInline(c) + "</" + tag + ">")
				}
				b.WriteString("</tr>\n")
			}
			b.WriteString("</table>\n")
		case strings.HasPrefix(t, ">"):
			absatzEnde()
			var teile []string
			for ; i < len(zeilen) && strings.HasPrefix(strings.TrimSpace(zeilen[i]), ">"); i++ {
				teile = append(teile, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(zeilen[i]), ">")))
			}
			b.WriteString("<blockquote><p>" + mdInline(strings.Join(teile, " ")) + "</p></blockquote>\n")
		default:
			if _, _, _, ok := mdListenpunkt(z); !ok {
				absatz = append(absatz, t)
				i++
				continue
			}
			absatzEnde()
			type ebene struct {
				einr int
				tag  string
			}
			var stapel []ebene
			// Der Text des offenen Punkts, als Ganzes umgewandelt -- **fett**
			// darf ueber eine Fortsetzungszeile reichen.
			var punkt []string
			punktAus := func() {
				if len(punkt) > 0 {
					b.WriteString(mdInline(strings.Join(punkt, " ")))
					punkt = nil
				}
			}
			for i < len(zeilen) {
				z := zeilen[i]
				if strings.TrimSpace(z) == "" {
					// Leerzeile: die Liste geht weiter, wenn danach ein Punkt
					// oder eine eingerueckte Fortsetzung kommt.
					j := i + 1
					for j < len(zeilen) && strings.TrimSpace(zeilen[j]) == "" {
						j++
					}
					if j < len(zeilen) {
						if _, _, _, ok := mdListenpunkt(zeilen[j]); ok || strings.HasPrefix(zeilen[j], "  ") {
							i = j
							continue
						}
					}
					break
				}
				einr, tag, text, ok := mdListenpunkt(z)
				if !ok {
					if !strings.HasPrefix(z, " ") {
						break // nicht eingerueckt: die Liste ist zu Ende
					}
					punkt = append(punkt, strings.TrimSpace(z))
					i++
					continue
				}
				punktAus()
				for len(stapel) > 0 && stapel[len(stapel)-1].einr > einr {
					b.WriteString("</li></" + stapel[len(stapel)-1].tag + ">\n")
					stapel = stapel[:len(stapel)-1]
				}
				// Gleiche Tiefe, andere Art (Punkte -> Nummern): neue Liste.
				if n := len(stapel); n > 0 && stapel[n-1].einr == einr && stapel[n-1].tag != tag {
					b.WriteString("</li></" + stapel[n-1].tag + ">\n")
					stapel = stapel[:n-1]
				}
				if len(stapel) == 0 || einr > stapel[len(stapel)-1].einr {
					b.WriteString("<" + tag + ">\n")
					stapel = append(stapel, ebene{einr, tag})
				} else {
					b.WriteString("</li>\n")
				}
				b.WriteString("<li>")
				punkt = []string{text}
				i++
			}
			punktAus()
			for len(stapel) > 0 {
				b.WriteString("</li></" + stapel[len(stapel)-1].tag + ">\n")
				stapel = stapel[:len(stapel)-1]
			}
		}
	}
	absatzEnde()
	return b.String()
}
