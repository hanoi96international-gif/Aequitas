package keeper

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// /loeschen: der Weg, die Anmeldung ohne die App loeschen zu lassen (Play
// Store, docs/PLAY_STORE.md). Dieselbe Sperre wie Impressum und Datenschutz.

func loeschseite(t *testing.T) (int, string) {
	t.Helper()
	w := httptest.NewRecorder()
	(&APIServer{}).handleLoeschen(w, httptest.NewRequest("GET", "/loeschen", nil))
	return w.Code, w.Body.String()
}

// Missbrauch: ohne Kontaktadresse waere die Seite eine Anleitung ins Leere --
// sie darf dann gar nicht erscheinen, auch nicht halb.
func TestLoeschseite_OhnePflichtangabenGesperrt(t *testing.T) {
	for _, f := range legalFelder {
		t.Setenv(f.Umgebung, "")
	}
	if code, body := loeschseite(t); code != 404 || strings.Contains(body, "Anmeldung löschen") {
		t.Fatalf("Status %d ohne Angaben, erwartet 404 ohne Inhalt", code)
	}
	mitPflichtangaben(t)
	t.Setenv("LEGAL_EMAIL", "")
	if code, _ := loeschseite(t); code != 404 {
		t.Fatalf("Status %d ohne E-Mail, erwartet 404", code)
	}
}

func TestLoeschseite_VollstaendigUndPassendZurApp(t *testing.T) {
	mitPflichtangaben(t)
	t.Setenv("LEGAL_EMAIL", "datenschutz@example.org")
	code, seite := loeschseite(t)
	if code != 200 {
		t.Fatalf("Status %d mit allen Angaben", code)
	}
	if i := strings.Index(seite, "{{"); i >= 0 {
		t.Fatalf("unersetzte Marke: %q", seite[i:i+20])
	}
	for _, muss := range []string{
		`href="mailto:datenschutz@example.org?subject=`,
		// Die Beschriftungen der App (Aequitas-App lib/i18n/locales/de.ts):
		// aendert sich dort ein Text, muss diese Seite mit.
		"<strong>Identity</strong>", "<strong>Meine Daten löschen</strong>", "<strong>Endgültig löschen</strong>",
		// Der Nachweis ueber die Wallet und sein Zeitfenster (coordinator
		// loeschung.py, MAX_ALTER_S = 10 min).
		"Unterschrift Ihrer Wallet", "zehn Minuten",
		// Ehrlich ueber das, was bleibt.
		"Einmaligkeits-Marke", "keine neue Anmeldung möglich",
	} {
		if !strings.Contains(seite, muss) {
			t.Errorf("fehlt: %q", muss)
		}
	}
}

// Missbrauch: eine Angabe mit HTML darf weder aus dem mailto-Attribut noch
// aus dem Text ausbrechen.
func TestLoeschseite_AngabenEscaped(t *testing.T) {
	mitPflichtangaben(t)
	t.Setenv("LEGAL_EMAIL", `x@example.org"><script>alert(1)</script>`)
	t.Setenv("LEGAL_NAME", `<img src=x onerror=alert(1)>`)
	_, seite := loeschseite(t)
	for _, darfNicht := range []string{"<script>", "<img", `org">`} {
		if strings.Contains(seite, darfNicht) {
			t.Errorf("nicht escaped: %q", darfNicht)
		}
	}
}
