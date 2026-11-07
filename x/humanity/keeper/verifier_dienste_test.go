package keeper

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Anmeldung der Dienste von Pruefern (verifier_dienste.go): Gutfall und je
// Angriff ein Test, ohne Datenbank (Bindung in der Anmeldung).

func vdFrisch(t *testing.T) {
	t.Helper()
	alt := dienstListe
	dienstListe = &dienstStelle{nachK: map[string]DienstAnmeldung{}}
	t.Cleanup(func() { dienstListe = alt })
}

func vdAnmeldung(t *testing.T, p b1bPruefer, rolle, u string, zeit int64) map[string]interface{} {
	t.Helper()
	b := p.bindung(t)
	return map[string]interface{}{"rolle": rolle, "public_key": p.k.pub, "url": u, "zeit": zeit,
		"signatur": p.k.unterschreibe(dienstNachricht(rolle, u, zeit)),
		"mensch": b.Mensch, "mensch_sig": b.MenschSig, "schluessel_sig": b.SchluesselSig}
}

func vdSenden(t *testing.T, a *APIServer, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	roh, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	a.handleVerifierDienst(w, httptest.NewRequest(http.MethodPost, "/api/verifier/dienst", bytes.NewReader(roh)))
	return w
}

// vdFall: Register aktiv, p ist Pruefer (Gruender), fremd nicht.
func vdFall(t *testing.T) (*APIServer, b1bPruefer, b1bPruefer, int64) {
	t.Helper()
	vdFrisch(t)
	f := neuerVrFall(t)
	verifierRegisterOverride.Store(nowUnix() - 1000)
	p := neuerB1bPruefer(t)
	verifierGruender = []string{p.mensch}
	return &APIServer{state: f.cs}, p, neuerB1bPruefer(t), nowUnix()
}

func TestDienste_Gutfall(t *testing.T) {
	a, p, _, now := vdFall(t)
	u := "https://verifier.example.org"
	if w := vdSenden(t, a, vdAnmeldung(t, p, dienstRolleVergleich, u, now)); w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	liste := a.state.DiensteJetzt(now)
	if len(liste) != 1 || liste[0].URL != u || liste[0].Mensch != p.mensch || liste[0].Rolle != dienstRolleVergleich {
		t.Fatalf("Liste: %+v", liste)
	}
	// Die Liste traegt alles, was ein Nutzer selbst nachpruefen muss.
	e := liste[0]
	if !bindungGueltig(e.PublicKey, e.Mensch, e.MenschSig, e.SchluesselSig) ||
		!ed25519PruefenStreng(e.PublicKey, e.Signatur, []byte(dienstNachricht(e.Rolle, e.URL, e.Zeit))) {
		t.Fatal("ausgegebene Anmeldung nicht nachpruefbar")
	}
	w := httptest.NewRecorder()
	a.handleVerifierDienste(w, httptest.NewRequest(http.MethodGet, "/api/verifier/dienste", nil))
	if !strings.Contains(w.Body.String(), u) {
		t.Fatalf("GET: %s", w.Body.String())
	}
	// Verfaellt ohne Erneuerung.
	if n := len(a.state.DiensteJetzt(now + dienstGueltig + 1)); n != 0 {
		t.Fatalf("nach Ablauf noch %d", n)
	}
}

func TestDienste_Missbrauch(t *testing.T) {
	a, p, fremd, now := vdFall(t)
	u := "https://verifier.example.org"
	gut := func() map[string]interface{} { return vdAnmeldung(t, p, dienstRolleVergleich, u, now) }
	mit := func(k string, v interface{}) map[string]interface{} {
		x := gut()
		x[k] = v
		return x
	}
	faelle := []struct {
		name string
		body map[string]interface{}
		code int
	}{
		{"kein Pruefer", vdAnmeldung(t, fremd, dienstRolleVergleich, u, now), http.StatusForbidden},
		{"Unterschrift fuer eine andere URL", mit("url", "https://anders.example.org"), http.StatusForbidden},
		{"Unterschrift fuer eine andere Rolle", mit("rolle", dienstRolleCoordinator), http.StatusForbidden},
		{"andere Zeit", mit("zeit", now-1), http.StatusForbidden},
		{"zu alt", vdAnmeldung(t, p, dienstRolleVergleich, u, now-dienstSpielraum-5), http.StatusBadRequest},
		{"http", vdAnmeldung(t, p, dienstRolleVergleich, "http://verifier.example.org", now), http.StatusBadRequest},
		{"interne Adresse", vdAnmeldung(t, p, dienstRolleVergleich, "https://127.0.0.1", now), http.StatusBadRequest},
		{"Query", vdAnmeldung(t, p, dienstRolleVergleich, u+"/?x=1", now), http.StatusBadRequest},
		{"Benutzerteil", vdAnmeldung(t, p, dienstRolleVergleich, "https://a@verifier.example.org", now), http.StatusBadRequest},
		{"Grossschreibung", vdAnmeldung(t, p, dienstRolleVergleich, "https://Verifier.example.org", now), http.StatusBadRequest},
		{"unbekannte Rolle", vdAnmeldung(t, p, "chef", u, now), http.StatusBadRequest},
		{"fremde Freigabe", mit("mensch_sig", personalSign(t, fremd.mk, coordinatorFreigabeNachricht(p.k.pub))), http.StatusForbidden},
		{"fremder Mensch", mit("mensch", fremd.mensch), http.StatusForbidden},
		{"Besitznachweis falsch", mit("schluessel_sig", fremd.k.unterschreibe(coordinatorBesitzNachricht(p.mensch))), http.StatusForbidden},
		{"ohne Bindung, ohne Register", func() map[string]interface{} {
			x := gut()
			delete(x, "mensch")
			delete(x, "mensch_sig")
			delete(x, "schluessel_sig")
			return x
		}(), http.StatusForbidden},
		{"unbekanntes Feld", mit("x", 1), http.StatusBadRequest},
		{"zu gross", mit("signatur", strings.Repeat("a", dienstKoerperMax)), http.StatusBadRequest},
	}
	for _, fa := range faelle {
		if w := vdSenden(t, a, fa.body); w.Code != fa.code {
			t.Errorf("%s: %d %s, erwartet %d", fa.name, w.Code, w.Body.String(), fa.code)
		}
	}
	if n := len(a.state.DiensteJetzt(now)); n != 0 {
		t.Fatalf("%d Anmeldungen nach lauter Angriffen", n)
	}
}

// Eine URL gehoert einem Pruefer; eine alte Anmeldung dreht nichts zurueck;
// je Mensch hoechstens dienstJeMensch; wer nicht mehr Pruefer ist, faellt heraus.
func TestDienste_Grenzen(t *testing.T) {
	a, p, fremd, now := vdFall(t)
	u := "https://verifier.example.org"
	if w := vdSenden(t, a, vdAnmeldung(t, p, dienstRolleVergleich, u, now)); w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	// Ein anderer Pruefer kann die URL nicht uebernehmen.
	verifierGruender = []string{p.mensch, fremd.mensch}
	if w := vdSenden(t, a, vdAnmeldung(t, fremd, dienstRolleVergleich, u, now)); w.Code != http.StatusConflict {
		t.Fatalf("fremde URL: %d", w.Code)
	}
	// Abgefangene aeltere Anmeldung: keine Wirkung.
	if w := vdSenden(t, a, vdAnmeldung(t, p, dienstRolleVergleich, u, now-10)); w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	if e := a.state.DiensteJetzt(now); len(e) != 1 || e[0].Zeit != now {
		t.Fatalf("aeltere Anmeldung hat ersetzt: %+v", e)
	}
	// Hoechstens dienstJeMensch Schluessel je Mensch.
	for i := 1; i < dienstJeMensch; i++ {
		q := b1bPruefer{mk: p.mk, mensch: p.mensch, k: neuerEd25519(t)}
		if w := vdSenden(t, a, vdAnmeldung(t, q, dienstRolleVergleich, "https://v"+string(rune('a'+i))+".example.org", now)); w.Code != http.StatusOK {
			t.Fatalf("Dienst %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	q := b1bPruefer{mk: p.mk, mensch: p.mensch, k: neuerEd25519(t)}
	if w := vdSenden(t, a, vdAnmeldung(t, q, dienstRolleVergleich, "https://zuviel.example.org", now)); w.Code != http.StatusTooManyRequests {
		t.Fatalf("ueber der Grenze je Mensch: %d", w.Code)
	}
	// Nicht mehr Pruefer: sofort heraus.
	verifierGruender = []string{fremd.mensch}
	for _, e := range a.state.DiensteJetzt(now) {
		if e.Mensch == p.mensch {
			t.Fatal("Dienst eines Nicht-mehr-Pruefers in der Liste")
		}
	}
}

func TestDienste_VorDemStichtag(t *testing.T) {
	a, p, _, now := vdFall(t)
	verifierRegisterOverride.Store(now + 86400)
	if w := vdSenden(t, a, vdAnmeldung(t, p, dienstRolleVergleich, "https://verifier.example.org", now)); w.Code != http.StatusConflict {
		t.Fatalf("%d", w.Code)
	}
}
