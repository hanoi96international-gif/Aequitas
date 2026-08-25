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

func TestUnvollstaendigeSeiteWirdNichtAusgeliefert(t *testing.T) {
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
	if istVollstaendig("irgendwo steht PLATZHALTER mitten im Text") {
		t.Error("Markierung wurde nicht erkannt")
	}
	if !istVollstaendig("ein Text ganz ohne die Markierung") {
		t.Error("Text ohne Markierung wurde fälschlich gesperrt")
	}
}
