package keeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Die Sammelstelle fuer Stimmen und Antraege (verifier_sammeln.go), ohne
// Datenbank: Vorpruefung, Grenzen, Aufraeumen -- je Angriff ein Test. Das
// Einreichen selbst prueft verifier_sammeln_realdb_test.go.

func vsFrisch(t *testing.T) {
	t.Helper()
	alt := verifierSammel
	verifierSammel = neueVerifierSammelstelle()
	t.Cleanup(func() { verifierSammel = alt })
}

func vsStimmeSenden(t *testing.T, a *APIServer, art, wallet string, s VerifierStimme) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{"art": art, "wallet": wallet, "stimme": s})
	w := httptest.NewRecorder()
	a.handleVerifierStimme(w, httptest.NewRequest(http.MethodPost, "/api/verifier/stimme", bytes.NewReader(body)))
	return w
}

func TestVerifierSammeln_VorpruefungGutfall(t *testing.T) {
	f := neuerVrFall(t)
	now := vrAb + 1000
	s := f.stimme(0, verifierArtAufnahme, f.m(1), now)
	if err := verifierStimmeVorpruefen(verifierArtAufnahme, f.m(1), &s, f.cs.verifierRegisterJetzt(), now); err != nil {
		t.Fatalf("gueltige Stimme abgelehnt: %v", err)
	}
}

func TestVerifierSammeln_VorpruefungMissbrauch(t *testing.T) {
	f := neuerVrFall(t)
	now := vrAb + 1000
	reg := f.cs.verifierRegisterJetzt()
	gut := func() VerifierStimme { return f.stimme(0, verifierArtAufnahme, f.m(1), now) }
	faelle := []struct {
		name   string
		art    string
		wallet string
		s      func() VerifierStimme
	}{
		{"Stimme fuer einen anderen Betroffenen (Wiederverwendung)", verifierArtAufnahme, f.m(2), gut},
		{"Stimme fuer eine andere Art", verifierArtEntzug, f.m(1), gut},
		{"Nicht-Pruefer stimmt", verifierArtAufnahme, f.m(2), func() VerifierStimme { return f.stimme(1, verifierArtAufnahme, f.m(2), now) }},
		{"Selbststimme eines Nicht-Pruefers", verifierArtAufnahme, f.m(1), func() VerifierStimme { return f.stimme(1, verifierArtAufnahme, f.m(1), now) }},
		{"Pruefer schon Pruefer", verifierArtAufnahme, f.m(0), func() VerifierStimme { return f.stimme(0, verifierArtAufnahme, f.m(0), now) }},
		{"Austritt fuer einen anderen", verifierArtAustritt, f.m(1), func() VerifierStimme { return f.stimme(0, verifierArtAustritt, f.m(1), now) }},
		{"Entzug durch den Betroffenen selbst", verifierArtEntzug, f.m(0), func() VerifierStimme { return f.stimme(0, verifierArtEntzug, f.m(0), now) }},
		{"unbekannte Art", "befoerderung", f.m(1), gut},
		{"Wallet nicht kanonisch", verifierArtAufnahme, strings.ToUpper(f.m(1)), gut},
		{"zu alt", verifierArtAufnahme, f.m(1), func() VerifierStimme {
			return f.stimme(0, verifierArtAufnahme, f.m(1), now-verifierStimmeHoechstensAlt-1)
		}},
		{"aus der Zukunft", verifierArtAufnahme, f.m(1), func() VerifierStimme {
			return f.stimme(0, verifierArtAufnahme, f.m(1), now+nachweisHoechstensVoraus+1)
		}},
		{"gefaelschte Unterschrift", verifierArtAufnahme, f.m(1), func() VerifierStimme {
			s := gut()
			s.Bescheinigung.Signature = strings.Repeat("00", 64)
			return s
		}},
		{"fremder Schluessel unter dem Namen des Gruenders", verifierArtAufnahme, f.m(1), func() VerifierStimme {
			s := f.stimme(3, verifierArtAufnahme, f.m(1), now)
			s.Bescheinigung.Mensch = f.m(0)
			s.Bescheinigung.MenschSig = f.menschen[0].bindung.MenschSig
			return s
		}},
		{"Freigabe des Menschen fehlt", verifierArtAufnahme, f.m(1), func() VerifierStimme {
			s := gut()
			s.Bescheinigung.MenschSig = ""
			return s
		}},
		{"Besitznachweis fehlt", verifierArtAufnahme, f.m(1), func() VerifierStimme {
			s := gut()
			s.Bescheinigung.SchluesselSig = strings.Repeat("00", 64)
			return s
		}},
	}
	for _, fa := range faelle {
		s := fa.s()
		if err := verifierStimmeVorpruefen(fa.art, fa.wallet, &s, reg, now); err == nil {
			t.Errorf("%s: angenommen", fa.name)
		}
	}
}

func TestVerifierSammeln_VorpruefungNichtVorLetzterAenderung(t *testing.T) {
	f := neuerVrFall(t)
	z := vrAb + 1000
	f.aufnehmen(1, z, 0)
	// Eine alte Stimme fuer die Aufnahme von 1 kann nach dem Austritt von 1
	// nicht wieder benutzt werden, um ihn erneut aufzunehmen.
	austritt := z + erzeugerFrist + 100
	if neu := f.anwenden(f.tx(verifierArtAustritt, f.m(1), austritt, 1), austritt); len(neu) != 0 {
		t.Fatalf("Austritt abgelehnt: %v", neu)
	}
	now := austritt + erzeugerFrist + 100
	alt := f.stimme(0, verifierArtAufnahme, f.m(1), z)
	if err := verifierStimmeVorpruefen(verifierArtAufnahme, f.m(1), &alt, f.cs.verifierRegisterJetzt(), now); err == nil {
		t.Fatal("Stimme von vor der letzten Aenderung angenommen")
	}
}

func TestVerifierSammeln_Noetig(t *testing.T) {
	f := neuerVrFall(t)
	z := vrAb + 1000
	reg := func() *verifierRegister { return f.cs.verifierRegisterJetzt() }
	if n := verifierNoetig(reg(), verifierArtAufnahme, f.m(1), z); n != 1 {
		t.Fatalf("ein Pruefer: noetig %d, erwartet 1", n)
	}
	if n := verifierNoetig(reg(), verifierArtEntzug, f.m(0), z); n != 0 {
		t.Fatalf("allein: Entzug unmoeglich, noetig %d", n)
	}
	f.aufnehmen(1, z, 0)
	t2 := z + erzeugerFrist + 1
	if n := verifierNoetig(reg(), verifierArtAufnahme, f.m(2), t2); n != 2 {
		t.Fatalf("zwei Pruefer: noetig %d, erwartet 2", n)
	}
	if n := verifierNoetig(reg(), verifierArtEntzug, f.m(1), t2); n != 1 {
		t.Fatalf("Entzug bei zwei: noetig %d, erwartet 1", n)
	}
	if n := verifierNoetig(reg(), verifierArtAustritt, f.m(1), t2); n != 1 {
		t.Fatalf("Austritt: noetig %d", n)
	}
	if n := verifierNoetig(reg(), "x", f.m(1), t2); n != 0 {
		t.Fatalf("unbekannte Art: noetig %d", n)
	}
}

// Ohne Leiter wird gesammelt, aber nichts eingereicht -- und nichts am
// Register veraendert.
func TestVerifierSammeln_OhneLeiterNurGesammelt(t *testing.T) {
	f := neuerVrFall(t)
	vsFrisch(t)
	verifierRegisterOverride.Store(nowUnix() - 1000)
	now := nowUnix()
	a := &APIServer{state: f.cs}
	w := vsStimmeSenden(t, a, verifierArtAufnahme, f.m(1), f.stimme(0, verifierArtAufnahme, f.m(1), now))
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var out map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &out)
	if out["eingereicht"] != false || out["hinweis"] == nil {
		t.Fatalf("ohne Leiter: %v", out)
	}
	if !f.cs.verifierRegisterJetzt().leer() {
		t.Fatal("Register veraendert ohne Annahme")
	}
	if len(verifierSammel.offen) != 1 {
		t.Fatalf("%d Abstimmungen gesammelt, erwartet 1", len(verifierSammel.offen))
	}
	// Dieselbe Stimme zweimal: weiter eine Stimme.
	vsStimmeSenden(t, a, verifierArtAufnahme, f.m(1), f.stimme(0, verifierArtAufnahme, f.m(1), now))
	if n := len(verifierSammel.offen[verifierArtAufnahme+"|"+f.m(1)].stimmen); n != 1 {
		t.Fatalf("%d Stimmen, erwartet 1 (je Pruefer eine)", n)
	}
}

func TestVerifierSammeln_HttpMissbrauch(t *testing.T) {
	f := neuerVrFall(t)
	vsFrisch(t)
	verifierRegisterOverride.Store(nowUnix() - 1000)
	now := nowUnix()
	a := &APIServer{state: f.cs}
	for _, m := range []struct {
		name string
		body string
		code int
	}{
		{"kaputt", `{"art":`, http.StatusBadRequest},
		{"unbekanntes Feld", `{"art":"aufnahme","x":1}`, http.StatusBadRequest},
		{"zu gross", `{"art":"` + strings.Repeat("a", verifierStimmeKoerperMax) + `"}`, http.StatusBadRequest},
	} {
		w := httptest.NewRecorder()
		a.handleVerifierStimme(w, httptest.NewRequest(http.MethodPost, "/api/verifier/stimme", strings.NewReader(m.body)))
		if w.Code != m.code {
			t.Fatalf("%s: %d %s", m.name, w.Code, w.Body.String())
		}
	}
	// Faelschung ueber HTTP: nichts gesammelt.
	s := f.stimme(0, verifierArtAufnahme, f.m(2), now)
	if w := vsStimmeSenden(t, a, verifierArtAufnahme, f.m(1), s); w.Code != http.StatusForbidden {
		t.Fatalf("Stimme fuer einen anderen: %d", w.Code)
	}
	if len(verifierSammel.offen) != 0 {
		t.Fatal("ungueltige Stimme gesammelt")
	}
	w := httptest.NewRecorder()
	a.handleVerifierStimme(w, httptest.NewRequest(http.MethodGet, "/api/verifier/stimme", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", w.Code)
	}
}

func TestVerifierSammeln_VorDemStichtagNichts(t *testing.T) {
	f := neuerVrFall(t)
	vsFrisch(t)
	verifierRegisterOverride.Store(nowUnix() + 86400)
	a := &APIServer{state: f.cs}
	if w := vsStimmeSenden(t, a, verifierArtAufnahme, f.m(1), f.stimme(0, verifierArtAufnahme, f.m(1), nowUnix())); w.Code != http.StatusConflict {
		t.Fatalf("vor dem Stichtag: %d", w.Code)
	}
}

func TestVerifierSammeln_HoechstensSoVieleAbstimmungen(t *testing.T) {
	vsFrisch(t)
	now := int64(2_000_000_000)
	for i := 0; i < verifierSammlungenHoechstens; i++ {
		verifierSammel.offen[fmt.Sprintf("aufnahme|%d", i)] = &verifierSammlung{art: "aufnahme", wallet: distTestAddr(i),
			stimmen: map[string]VerifierStimme{"x": {Zeit: now}}}
	}
	f := neuerVrFall(t)
	verifierRegisterOverride.Store(nowUnix() - 1000)
	a := &APIServer{state: f.cs}
	w := vsStimmeSenden(t, a, verifierArtAufnahme, f.m(1), f.stimme(0, verifierArtAufnahme, f.m(1), nowUnix()))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("voll: %d %s", w.Code, w.Body.String())
	}
}

func TestVerifierSammeln_AufraeumenVerfallen(t *testing.T) {
	vsFrisch(t)
	now := int64(2_000_000_000)
	verifierSammel.offen["aufnahme|a"] = &verifierSammlung{art: "aufnahme", wallet: distTestAddr(1),
		stimmen: map[string]VerifierStimme{"x": {Zeit: now - verifierStimmeHoechstensAlt - 1}, "y": {Zeit: now}}}
	verifierSammel.offen["aufnahme|b"] = &verifierSammlung{art: "aufnahme", wallet: distTestAddr(2),
		stimmen: map[string]VerifierStimme{"x": {Zeit: now - verifierStimmeHoechstensAlt - 1}}}
	verifierSammel.antraege[distTestAddr(3)] = now - verifierStimmeHoechstensAlt - 1
	verifierSammel.antraege[distTestAddr(4)] = now
	verifierSammel.aufraeumenLocked(&verifierRegister{}, now)
	if len(verifierSammel.offen) != 1 || len(verifierSammel.offen["aufnahme|a"].stimmen) != 1 {
		t.Fatalf("Aufraeumen: %d Abstimmungen", len(verifierSammel.offen))
	}
	if len(verifierSammel.antraege) != 1 {
		t.Fatalf("Aufraeumen: %d Antraege", len(verifierSammel.antraege))
	}
}

func vsAntragSenden(t *testing.T, a *APIServer, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	a.handleVerifierAntrag(w, httptest.NewRequest(http.MethodPost, "/api/verifier/antrag", bytes.NewReader(b)))
	return w
}

func TestVerifierSammeln_Antrag(t *testing.T) {
	f := neuerVrFall(t)
	vsFrisch(t)
	verifierRegisterOverride.Store(nowUnix() - 1000)
	now := nowUnix()
	a := &APIServer{state: f.cs}
	mk, mensch := neuerSchluessel(t)
	addHuman(f.cs, mensch, 1000)
	gut := map[string]interface{}{"wallet": mensch, "zeit": now, "signatur": personalSign(t, mk, verifierAntragNachricht(mensch, now))}
	if w := vsAntragSenden(t, a, gut); w.Code != http.StatusOK {
		t.Fatalf("Antrag: %d %s", w.Code, w.Body.String())
	}
	if _, ok := verifierSammel.antraege[mensch]; !ok {
		t.Fatal("Antrag nicht vermerkt")
	}

	fk, fremd := neuerSchluessel(t)
	_ = fk
	nk, keinMensch := neuerSchluessel(t)
	gk, gruender2 := neuerSchluessel(t)
	addHuman(f.cs, gruender2, 1000)
	verifierGruender = append(append([]string{}, verifierGruender...), gruender2)
	for _, m := range []struct {
		name string
		body map[string]interface{}
		code int
	}{
		{"fremde Unterschrift", map[string]interface{}{"wallet": fremd, "zeit": now, "signatur": personalSign(t, mk, verifierAntragNachricht(fremd, now))}, http.StatusForbidden},
		{"alte Unterschrift", map[string]interface{}{"wallet": mensch, "zeit": now - 2*verifierAntragSpielraum,
			"signatur": personalSign(t, mk, verifierAntragNachricht(mensch, now-2*verifierAntragSpielraum))}, http.StatusBadRequest},
		{"andere Zeit als unterschrieben", map[string]interface{}{"wallet": mensch, "zeit": now - 1, "signatur": gut["signatur"]}, http.StatusForbidden},
		{"kein registrierter Mensch", map[string]interface{}{"wallet": keinMensch, "zeit": now, "signatur": personalSign(t, nk, verifierAntragNachricht(keinMensch, now))}, http.StatusForbidden},
		{"schon Pruefer", map[string]interface{}{"wallet": gruender2, "zeit": now, "signatur": personalSign(t, gk, verifierAntragNachricht(gruender2, now))}, http.StatusConflict},
	} {
		if w := vsAntragSenden(t, a, m.body); w.Code != m.code {
			t.Fatalf("%s: %d %s, erwartet %d", m.name, w.Code, w.Body.String(), m.code)
		}
	}
	if len(verifierSammel.antraege) != 1 {
		t.Fatalf("%d Antraege, erwartet 1", len(verifierSammel.antraege))
	}

	// Voll: kein neuer Antrag, der vorhandene darf sich erneuern.
	for i := 0; len(verifierSammel.antraege) < verifierAntraegeHoechstens; i++ {
		verifierSammel.antraege[distTestAddr(9000+i)] = now
	}
	k2, m2 := neuerSchluessel(t)
	addHuman(f.cs, m2, 1000)
	if w := vsAntragSenden(t, a, map[string]interface{}{"wallet": m2, "zeit": now, "signatur": personalSign(t, k2, verifierAntragNachricht(m2, now))}); w.Code != http.StatusTooManyRequests {
		t.Fatalf("voll: %d", w.Code)
	}
	if w := vsAntragSenden(t, a, gut); w.Code != http.StatusOK {
		t.Fatalf("erneuern bei voll: %d", w.Code)
	}

	// Liste
	w := httptest.NewRecorder()
	a.handleVerifierAntraege(w, httptest.NewRequest(http.MethodGet, "/api/verifier/antraege", nil))
	var out struct {
		Antraege []struct{ Wallet string } `json:"antraege"`
		Pruefer  []string                  `json:"pruefer"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out.Antraege) != verifierAntraegeHoechstens || len(out.Pruefer) != 2 {
		t.Fatalf("Liste: %v %s", err, w.Body.String())
	}
}
