package keeper

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Die eingebettete Kopie ist das Dokument -- sonst zeigte die Seite einen
// alten Stand.
func TestGrundpfeiler_KopieGleichDemDokument(t *testing.T) {
	doc, err := os.ReadFile("../../../docs/GRUNDPFEILER.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(doc) != grundpfeilerMD {
		t.Fatal("assets/grundpfeiler.md weicht von docs/GRUNDPFEILER.md ab -- cp docs/GRUNDPFEILER.md x/humanity/keeper/assets/grundpfeiler.md")
	}
}

func TestMarkdownZuHTML_Formen(t *testing.T) {
	md := "# Titel\n\nEin **fetter** Satz mit `code`\nueber zwei Zeilen.\n\n" +
		"| A | B |\n|---|---|\n| 1 | **2** |\n\n" +
		"> Zitat\n> weiter\n\n" +
		"- eins\n  Fortsetzung\n  - unter\n- zwei **ueber\n  zwei** Zeilen\n\n1. erst\n2. dann\n\n---\n\n## Ende\n"
	got := markdownZuHTML(md)
	for _, muss := range []string{
		"<h1>Titel</h1>",
		"<p>Ein <strong>fetter</strong> Satz mit <code>code</code> ueber zwei Zeilen.</p>",
		"<tr><th>A</th><th>B</th></tr>",
		"<tr><td>1</td><td><strong>2</strong></td></tr>",
		"<blockquote><p>Zitat weiter</p></blockquote>",
		"<li>eins Fortsetzung<ul>",
		"<li>unter</li></ul>",
		"<li>zwei <strong>ueber zwei</strong> Zeilen</li></ul>",
		"<ol>",
		"<li>dann</li></ol>",
		"<hr>",
		"<h2>Ende</h2>",
	} {
		if !strings.Contains(strings.ReplaceAll(got, "\n", ""), strings.ReplaceAll(muss, "\n", "")) {
			t.Fatalf("fehlt: %s\n--- HTML ---\n%s", muss, got)
		}
	}
	if strings.Contains(got, "---|") {
		t.Fatal("Tabellentrenner als Zeile ausgegeben")
	}
}

// Missbrauch: was im Text steht, wird nie zu Auszeichnung -- auch nicht in
// Tabellen, Zitaten, Listen, Code oder fett.
func TestMarkdownZuHTML_MaskiertAlles(t *testing.T) {
	boese := `<script>alert(1)</script>`
	md := "# " + boese + "\n\n" + boese + "\n\n| " + boese + " | x |\n|---|---|\n| `" + boese + "` | **" + boese + "** |\n\n> " +
		boese + "\n\n- " + boese + "\n\n<img src=x onerror=alert(1)>\n\n\"quote\" & 'apos'"
	got := markdownZuHTML(md)
	for _, verboten := range []string{"<script", "<img", "onerror=alert(1)>"} {
		if strings.Contains(got, verboten) {
			t.Fatalf("unmaskiert: %s\n%s", verboten, got)
		}
	}
	if !strings.Contains(got, "&lt;script&gt;") || !strings.Contains(got, "&amp;") || !strings.Contains(got, "&#34;quote&#34;") {
		t.Fatalf("Maskierung fehlt:\n%s", got)
	}
}

func TestHandleGrundpfeiler(t *testing.T) {
	a := &APIServer{}
	rec := httptest.NewRecorder()
	a.handleGrundpfeiler(rec, httptest.NewRequest(http.MethodGet, "/grundpfeiler", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<h1>Aequitas: Die Grundpfeiler</h1>") || !strings.Contains(body, "<table>") {
		t.Fatal("Inhalt des Dokuments fehlt")
	}
	if strings.Contains(body, "<script") {
		t.Fatal("Skript auf der Seite")
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "script-src") {
		t.Fatalf("CSP: %q", csp)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("nosniff fehlt")
	}
	rec = httptest.NewRecorder()
	a.handleGrundpfeiler(rec, httptest.NewRequest(http.MethodPost, "/grundpfeiler", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", rec.Code)
	}
}

// Der Link auf die Grundpfeiler erscheint immer -- die Rechtstexte erst, wenn
// das Impressum vollstaendig ist.
func TestMitFussLinks_GrundpfeilerImmer(t *testing.T) {
	got := mitFussLinks("x<!--LEGAL_LINKS-->y", `<a href="/grundpfeiler">G</a>`, `<a href="/impressum">I</a>`)
	if !strings.Contains(got, "/grundpfeiler") {
		t.Fatal("Grundpfeiler-Link fehlt")
	}
	if len(fehlendeLegalFelder()) > 0 && strings.Contains(got, "/impressum") {
		t.Fatal("Impressum-Link trotz fehlender Angaben")
	}
}
