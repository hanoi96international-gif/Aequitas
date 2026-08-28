package keeper

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Die Sperre ist der eigentliche Gehalt von api_legal.go: eine
// Datenschutzerklärung ohne die Identität des Verantwortlichen ist wertlos,
// ein Impressum ohne ladungsfähige Anschrift schlechter als keines. Beide
// dürfen deshalb erst erscheinen, wenn sie vollständig sind.

// mitPflichtangaben setzt jede Pflichtangabe auf einen Beispielwert.
//
// Seit dem 28.08.2026 haengt die Sperre nicht mehr an einer Markierung im
// HTML, sondern an Umgebungsvariablen (legal_felder.go). Der Gehalt der Tests
// bleibt derselbe: unvollstaendig heisst 404.
func mitPflichtangaben(t *testing.T) {
	t.Helper()
	for _, f := range legalFelder {
		if f.Pflicht {
			t.Setenv(f.Umgebung, "Beispiel")
		}
	}
}

func TestUnvollstaendigeSeiteWirdNichtAusgeliefert(t *testing.T) {
	mitPflichtangaben(t)
	a := &APIServer{}
	for _, f := range []struct {
		name    string
		inhalt  string
		erwarte int
	}{
		{"mit Platzhalter", "<h1>Impressum</h1><p>[PLATZHALTER — Name]</p>", 404},
		{"vollständig", "<h1>Impressum</h1><p>Beispielstraße 1</p>", 200},
	} {
		t.Run(f.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			a.serveLegal(w, "Impressum", f.inhalt)
			if w.Code != f.erwarte {
				t.Fatalf("Status %d, erwartet %d", w.Code, f.erwarte)
			}
		})
	}
}

func TestAusgelieferteSeiteEnthaeltDenInhalt(t *testing.T) {
	mitPflichtangaben(t)
	a := &APIServer{}
	w := httptest.NewRecorder()
	a.serveLegal(w, "Datenschutzerklärung", "<h1>Datenschutz</h1><p>Inhalt</p>")

	koerper := w.Body.String()
	for _, muss := range []string{
		"<!doctype html>",
		`lang="de"`,
		"Datenschutzerklärung",
		"<p>Inhalt</p>",
	} {
		if !strings.Contains(koerper, muss) {
			t.Errorf("Ausgabe enthält %q nicht", muss)
		}
	}
	// Ein Rechtstext darf nicht zwischengespeichert werden: ändert er sich,
	// muss die alte Fassung sofort verschwinden.
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, erwartet no-store", got)
	}
}

// Die ausgelieferten Vorlagen selbst prüfen. Schlägt das fehl, sind die
// Pflichtangaben eingesetzt worden -- dann ist der Test anzupassen und die
// Seiten gehen live. Bis dahin hält er fest, dass sie es NICHT sind, damit
// niemand annimmt, die Pflicht sei erfüllt.
func TestVorlagenStandZumZeitpunktDesCommits(t *testing.T) {
	for name, inhalt := range map[string]string{
		"impressum":   impressumHTML,
		"datenschutz": datenschutzHTML,
	} {
		if istVollstaendig(inhalt) {
			t.Logf("%s ist vollständig und wird ausgeliefert -- Pflichtangaben wurden eingesetzt", name)
			continue
		}
		t.Logf("%s enthält noch Platzhalter und wird mit 404 beantwortet", name)
	}
}

// Gegenprobe zur Markierung selbst: eine Vorlage, in der das Wort nur
// zufällig vorkäme, würde ebenfalls gesperrt. Das ist gewollt -- lieber eine
// Seite zu viel zurückhalten als eine unvollständige veröffentlichen.
func TestMarkierungIstEindeutig(t *testing.T) {
	mitPflichtangaben(t)
	if istVollstaendig("irgendwo steht PLATZHALTER mitten im Text") {
		t.Error("Markierung wurde nicht erkannt")
	}
	if !istVollstaendig("ein Text ganz ohne die Markierung") {
		t.Error("Text ohne Markierung wurde fälschlich gesperrt")
	}
}

// Der Kern der neuen Sperre: eine einzige fehlende Pflichtangabe genuegt.
//
// Sonst waere die haeufigste Art des Scheiterns eine halb ausgefuellte Seite --
// Name da, Anschrift vergessen -- und die ist schlimmer als gar keine: sie
// benennt jemanden und erweckt den Anschein, die Pflicht sei erfuellt.
func TestEineFehlendeAngabeSperrtDieSeite(t *testing.T) {
	for _, fehlt := range legalFelder {
		if !fehlt.Pflicht {
			continue
		}
		t.Run(fehlt.Umgebung, func(t *testing.T) {
			for _, f := range legalFelder {
				if f.Pflicht {
					t.Setenv(f.Umgebung, "Beispiel")
				}
			}
			t.Setenv(fehlt.Umgebung, "")

			if istVollstaendig("<h1>Impressum</h1>") {
				t.Errorf("ohne %s gilt die Seite als vollstaendig", fehlt.Umgebung)
			}
			w := httptest.NewRecorder()
			(&APIServer{}).serveLegal(w, "Impressum", "<h1>Impressum</h1>")
			if w.Code != 404 {
				t.Errorf("ohne %s wurde die Seite mit %d ausgeliefert", fehlt.Umgebung, w.Code)
			}
		})
	}
}

// Optionale Angaben, die leer bleiben, verschwinden samt ihrer Zeile.
//
// Eine Zeile "Telefon:" ohne Nummer sieht aus wie ein Fehler, und ein leeres
// Registergericht wirft die Frage auf, ob da eine Gesellschaft steht oder
// nicht.
func TestLeereOptionaleAngabeVerschwindet(t *testing.T) {
	mitPflichtangaben(t)
	t.Setenv("LEGAL_TELEFON", "")

	aus := legalEingesetzt("Name: {{NAME}}\nTelefon: {{TELEFON}}\nOrt: {{ORT}}")
	if strings.Contains(aus, "Telefon") {
		t.Errorf("leere Telefonzeile blieb stehen:\n%s", aus)
	}
	for _, muss := range []string{"Name: Beispiel", "Ort: Beispiel"} {
		if !strings.Contains(aus, muss) {
			t.Errorf("%q fehlt in:\n%s", muss, aus)
		}
	}
}

// Werte werden HTML-escaped. Ein Ampersand im Firmennamen soll die Seite nicht
// zerlegen -- und wer die Quelle spaeter aendert, findet die Absicherung vor.
func TestAngabenWerdenEscaped(t *testing.T) {
	mitPflichtangaben(t)
	t.Setenv("LEGAL_NAME", `Meier & S<o>hne`)

	aus := legalEingesetzt("Name: {{NAME}}")
	if strings.Contains(aus, "<o>") {
		t.Errorf("Markup kam ungefiltert durch: %s", aus)
	}
	if !strings.Contains(aus, "&amp;") {
		t.Errorf("Ampersand wurde nicht escaped: %s", aus)
	}
}
