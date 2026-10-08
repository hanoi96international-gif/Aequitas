package keeper

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func erneuerungAnfrage(t *testing.T, a *APIServer, ip string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/liveness-renewal", strings.NewReader(string(body)))
	req.RemoteAddr = ip + ":4711"
	w := httptest.NewRecorder()
	a.handleLivenessRenewal(w, req)
	return w
}

// Missbrauch (Pruefung #314, LOW-3): je Anfrage eine Registerabfrage und die
// Zulassung -- eine Adresse bekommt hoechstens burstErneuerungJeIP Anfragen
// je Minute durch. Andere Adressen bleiben frei, Knoten der Freiliste
// (Weiterleitung) zaehlen nicht.
func TestErneuerung_GrenzeJeAdresse(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	a := &APIServer{state: newTestState()}
	angreifer, anderer, knoten := "198.51.100.77", "198.51.100.78", "198.51.100.79"
	for _, ip := range []string{angreifer, anderer, knoten} {
		ipBurst.Delete("liveness-renewal:" + ip)
	}
	for i := 0; i < burstErneuerungJeIP; i++ {
		if w := erneuerungAnfrage(t, a, angreifer, []byte(`{}`)); w.Code == http.StatusTooManyRequests {
			t.Fatalf("Anfrage %d schon begrenzt", i+1)
		}
	}
	if w := erneuerungAnfrage(t, a, angreifer, []byte(`{}`)); w.Code != http.StatusTooManyRequests {
		t.Fatalf("Anfrage %d nicht begrenzt: %d", burstErneuerungJeIP+1, w.Code)
	}
	if w := erneuerungAnfrage(t, a, anderer, []byte(`{}`)); w.Code == http.StatusTooManyRequests {
		t.Fatal("eine andere Adresse ist mitbegrenzt")
	}
	alt := rpcRateLimitFreiListe.Load()
	t.Cleanup(func() { rpcRateLimitFreiListe.Store(alt) })
	rpcRateLimitFreiErgaenzen([]string{knoten})
	for i := 0; i < 2*burstErneuerungJeIP; i++ {
		if w := erneuerungAnfrage(t, a, knoten, []byte(`{}`)); w.Code == http.StatusTooManyRequests {
			t.Fatalf("Knoten der Freiliste nach %d Anfragen begrenzt", i+1)
		}
	}
}

// Missbrauch (Pruefung #314, LOW-3): eine Muell-Unterschrift kostet keine
// Registerabfrage -- abgewiesen, bevor das Register gefragt wird. Eine
// gueltige Unterschrift kommt durch bis zum Register, auch in anderer
// Schreibweise (0x, gross): dieselbe Pruefung wie in bescheinigungPruefen.
func TestErneuerung_UnterschriftVorDemRegister(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	ip := "198.51.100.80"
	ipBurst.Delete("liveness-renewal:" + ip)
	// Ohne Datenbank: das Register kennt keinen Schluessel -- wer bis dahin
	// kommt, bekommt "registered coordinator".
	a := &APIServer{state: newTestState()}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	wallet := "0x" + strings.Repeat("ab", 20)
	issued := nowUnix()
	anfrage := func(sig string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]interface{}{"wallet": wallet, "issued_at": issued,
			"public_key": hex.EncodeToString(pub), "signature": sig})
		return erneuerungAnfrage(t, a, ip, body)
	}
	if w := anfrage(strings.Repeat("ab", 64)); w.Code != http.StatusForbidden ||
		!strings.Contains(w.Body.String(), "passt nicht") || strings.Contains(w.Body.String(), "registered coordinator") {
		t.Fatalf("Muell-Unterschrift nicht vor dem Register abgewiesen: %d %s", w.Code, w.Body.String())
	}
	gueltig := hex.EncodeToString(ed25519.Sign(priv, []byte(erneuerungsNachricht(wallet, issued))))
	for _, sig := range []string{gueltig, "0x" + strings.ToUpper(gueltig)} {
		if w := anfrage(sig); w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "registered coordinator") {
			t.Fatalf("gueltige Unterschrift (%s...) nicht bis zum Register: %d %s", sig[:6], w.Code, w.Body.String())
		}
	}
}
