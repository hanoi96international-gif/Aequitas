package keeper

import (
	"testing"
	"time"
)

// Nur eine Versionsnummer in strenger Form kommt durch -- nie ein beliebiger
// Text von aussen in die App.
func TestAppVersionAusAntwort(t *testing.T) {
	for roh, will := range map[string]string{
		`{"tag_name":"v1.16.0"}`:         "1.16.0",
		`{"tag_name":"1.2.3"}`:           "1.2.3",
		`{"tag_name":"v1.16.0-rc1"}`:     "",
		`{"tag_name":"<script>"}`:        "",
		`{"tag_name":"v99999.0.0"}`:      "",
		`{"tag_name":""}`:                "",
		`kaputt`:                         "",
		`{"tag_name":"v1.2.3 ; rm -rf"}`: "",
	} {
		if got := appVersionAusAntwort([]byte(roh)); got != will {
			t.Errorf("%s: %q, erwartet %q", roh, got, will)
		}
	}
}

// Fehlschlag (GitHub antwortet nicht): der letzte Stand bleibt, neu gefragt
// wird nach appVersionWiederholung statt erst nach appVersionAbstand --
// sonst meldete ein frisch gestarteter Knoten 30 Minuten lang keine Version.
// Weiterhin hoechstens eine Anfrage zur Zeit, und nicht oefter als vorgesehen.
func TestAppVersionWiederholungNachFehlschlag(t *testing.T) {
	appVersionStand.mu.Lock()
	alt := struct {
		v string
		n time.Time
		l bool
	}{appVersionStand.version, appVersionStand.naechster, appVersionStand.laeuft}
	appVersionStand.version, appVersionStand.naechster, appVersionStand.laeuft = "", time.Time{}, false
	appVersionStand.mu.Unlock()
	t.Cleanup(func() {
		appVersionStand.mu.Lock()
		appVersionStand.version, appVersionStand.naechster, appVersionStand.laeuft = alt.v, alt.n, alt.l
		appVersionStand.mu.Unlock()
	})

	t0 := time.Unix(1_800_000_000, 0)
	if _, f := appVersionFaellig(t0); !f {
		t.Fatal("Start: erste Anfrage muss sofort faellig sein")
	}
	if _, f := appVersionFaellig(t0); f {
		t.Fatal("zweite Anfrage, waehrend die erste laeuft")
	}
	appVersionErgebnis("", t0) // Fehlschlag
	if _, f := appVersionFaellig(t0.Add(appVersionWiederholung - time.Second)); f {
		t.Fatal("nach Fehlschlag zu frueh erneut gefragt")
	}
	if _, f := appVersionFaellig(t0.Add(appVersionWiederholung)); !f {
		t.Fatal("nach Fehlschlag nicht nach appVersionWiederholung erneut gefragt")
	}
	t1 := t0.Add(appVersionWiederholung)
	appVersionErgebnis("1.18.1", t1)
	if v, f := appVersionFaellig(t1.Add(appVersionAbstand - time.Second)); f || v != "1.18.1" {
		t.Fatalf("nach Erfolg: %q, gefragt=%v -- erwartet 1.18.1 ohne neue Anfrage vor appVersionAbstand", v, f)
	}
	if _, f := appVersionFaellig(t1.Add(appVersionAbstand)); !f {
		t.Fatal("nach appVersionAbstand nicht erneut gefragt")
	}
	appVersionErgebnis("", t1.Add(appVersionAbstand)) // spaeterer Fehlschlag
	if v, _ := appVersionFaellig(t1.Add(appVersionAbstand)); v != "1.18.1" {
		t.Fatalf("ein Fehlschlag hat den letzten Stand geloescht: %q", v)
	}
}

// Zweiter Weg ueber die Weiterleitung von github.com: nur das erwartete Ziel
// mit einem Tag in strenger Form -- keine andere Adresse, kein anderes Repo,
// kein angehaengter Text.
func TestAppVersionAusWeiterleitung(t *testing.T) {
	for ziel, will := range map[string]string{
		"https://github.com/hanoi96international-gif/Aequitas-App/releases/tag/v1.18.1":      "1.18.1",
		"https://github.com/hanoi96international-gif/Aequitas-App/releases/tag/1.2.3":        "1.2.3",
		"https://github.com/hanoi96international-gif/Aequitas-App/releases/tag/v1.18.1-rc1":  "",
		"https://github.com/hanoi96international-gif/Aequitas-App/releases/tag/v1.18.1?x=1":  "",
		"https://github.com/hanoi96international-gif/Aequitas-App/releases/tag/v1.18.1/../a": "",
		"https://github.com/someone-else/Aequitas-App/releases/tag/v9.9.9":                   "",
		"https://evil.example/hanoi96international-gif/Aequitas-App/releases/tag/v9.9.9":     "",
		"http://github.com/hanoi96international-gif/Aequitas-App/releases/tag/v1.18.1":       "",
		"https://github.com/hanoi96international-gif/Aequitas-App/releases":                  "",
		"": "",
	} {
		if got := appVersionAusWeiterleitung(ziel); got != will {
			t.Errorf("%q: %q, erwartet %q", ziel, got, will)
		}
	}
}
