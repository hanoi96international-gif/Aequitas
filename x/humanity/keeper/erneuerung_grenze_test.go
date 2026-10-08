package keeper

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func erneuerungUeberMux(t *testing.T, mux http.Handler, ip string, body string, weitergeleitet bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/liveness-renewal", strings.NewReader(body))
	req.RemoteAddr = ip + ":4711"
	if weitergeleitet {
		req.Header.Set(weitergeleitetKopf, "1")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func erneuerungsGrenzeLeeren(ips ...string) {
	for _, ip := range ips {
		ipBurst.Delete("liveness-renewal:" + ip)
		ipBurst.Delete("liveness-renewal-weiter:" + ip)
	}
}

// Missbrauch (Pruefung von #319, MEDIUM-1): die Grenze greift auf dem
// Knoten, den der Coordinator erreicht -- VOR der Weiterleitung zum
// Zustaendigen. Ein Angreifer bekommt ueber einen Folger hoechstens
// burstErneuerungJeIP Anfragen je Minute durch, und ein ehrlicher Coordinator
// an einer anderen Adresse kommt danach weiter durch.
func TestErneuerung_GrenzeVorDerWeiterleitung(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	var beimZustaendigen atomic.Int64
	zustSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		beimZustaendigen.Add(1)
		io.WriteString(w, `{"ok":true}`)
	}))
	defer zustSrv.Close()
	ich := "0x0000000000000000000000000000000000000003"
	leiter := "0x0000000000000000000000000000000000000001"
	l := NeueLeitung(ich, "", []string{ich, leiter}, leiter, true, LeitSpeicher{Term: 3, Leiter: leiter}, testKonfig(), LeitUmgebung{}, time.Now())
	l.SetzeURL(leiter, zustSrv.URL)
	cs := newTestState()
	cs.leitung.Store(l)
	mux := (&APIServer{state: cs}).buildMux()
	angreifer, ehrlich := "203.0.113.10", "203.0.113.11"
	erneuerungsGrenzeLeeren(angreifer, ehrlich)
	body := func(i int) string { return fmt.Sprintf(`{"wallet":"0x%040x","issued_at":1}`, i+1) }
	for i := 0; i < burstErneuerungJeIP; i++ {
		if w := erneuerungUeberMux(t, mux, angreifer, body(i), false); w.Code == http.StatusTooManyRequests {
			t.Fatalf("Anfrage %d schon begrenzt", i+1)
		}
	}
	vorher := beimZustaendigen.Load()
	if vorher == 0 {
		t.Fatal("Vorbedingung: der Folger leitet weiter")
	}
	if w := erneuerungUeberMux(t, mux, angreifer, body(99), false); w.Code != http.StatusTooManyRequests {
		t.Fatalf("Anfrage %d ueber den Folger nicht begrenzt: %d", burstErneuerungJeIP+1, w.Code)
	}
	if beimZustaendigen.Load() != vorher {
		t.Fatal("die begrenzte Anfrage wurde trotzdem weitergeleitet")
	}
	if w := erneuerungUeberMux(t, mux, ehrlich, body(100), false); w.Code == http.StatusTooManyRequests || beimZustaendigen.Load() != vorher+1 {
		t.Fatalf("ehrlicher Coordinator hinter demselben Folger ausgesperrt: %d", w.Code)
	}
}

// Beim Empfaenger: direkte Anfragen zaehlen je Adresse (auch ungueltige),
// weitergeleitete von Knoten der Freiliste nicht, weitergeleitete von anderen
// Adressen unter einem eigenen, groesseren Zaehler -- ein gefaelschter Kopf
// bringt nicht mehr als den, und der Folger teilt ihn nicht mit dem direkten
// Weg.
func TestErneuerung_GrenzeBeimEmpfaenger(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	mux := (&APIServer{state: newTestState()}).buildMux()
	direkt, knoten, weiter := "198.51.100.77", "198.51.100.79", "198.51.100.81"
	erneuerungsGrenzeLeeren(direkt, knoten, weiter)
	alt := rpcRateLimitFreiListe.Load()
	t.Cleanup(func() { rpcRateLimitFreiListe.Store(alt) })
	rpcRateLimitFreiErgaenzen([]string{knoten})

	// Direkt, auch mit ungueltigem Inhalt: gezaehlt (vor dem Lesen).
	for i := 0; i < burstErneuerungJeIP; i++ {
		if w := erneuerungUeberMux(t, mux, direkt, "kein json", false); w.Code == http.StatusTooManyRequests {
			t.Fatalf("direkt: Anfrage %d schon begrenzt", i+1)
		}
	}
	if w := erneuerungUeberMux(t, mux, direkt, `{}`, false); w.Code != http.StatusTooManyRequests {
		t.Fatalf("direkt: Anfrage %d nicht begrenzt: %d", burstErneuerungJeIP+1, w.Code)
	}
	// Ein Knoten der Freiliste, weitergeleitet: zaehlt nicht.
	for i := 0; i < 2*burstErneuerungJeIP; i++ {
		if w := erneuerungUeberMux(t, mux, knoten, `{}`, true); w.Code == http.StatusTooManyRequests {
			t.Fatalf("weitergeleitet von der Freiliste: nach %d begrenzt", i+1)
		}
	}
	// Derselbe Knoten ohne Kopf: wie jeder andere.
	erneuerungsGrenzeLeeren(knoten)
	for i := 0; i < burstErneuerungJeIP; i++ {
		erneuerungUeberMux(t, mux, knoten, `{}`, false)
	}
	if w := erneuerungUeberMux(t, mux, knoten, `{}`, false); w.Code != http.StatusTooManyRequests {
		t.Fatalf("Freiliste ohne Weiterleitungskopf nicht begrenzt: %d", w.Code)
	}
	// Weitergeleitet von ausserhalb der Freiliste (oder gefaelschter Kopf):
	// eigener Zaehler bis burstErneuerungWeiterJeIP.
	for i := 0; i < burstErneuerungWeiterJeIP; i++ {
		if w := erneuerungUeberMux(t, mux, weiter, `{}`, true); w.Code == http.StatusTooManyRequests {
			t.Fatalf("weitergeleitet: Anfrage %d schon begrenzt", i+1)
		}
	}
	if w := erneuerungUeberMux(t, mux, weiter, `{}`, true); w.Code != http.StatusTooManyRequests {
		t.Fatalf("weitergeleitet: Anfrage %d nicht begrenzt: %d", burstErneuerungWeiterJeIP+1, w.Code)
	}
	if w := erneuerungUeberMux(t, mux, weiter, `{}`, false); w.Code == http.StatusTooManyRequests {
		t.Fatal("der direkte Weg teilt den Zaehler der Weiterleitung")
	}
}

// Vor der Aktivierung der Staffel zaehlt nichts (der Handler antwortet 409).
func TestErneuerung_GrenzeSchlaeft(t *testing.T) {
	mux := (&APIServer{state: newTestState()}).buildMux()
	ip := "198.51.100.90"
	erneuerungsGrenzeLeeren(ip)
	for i := 0; i < burstErneuerungJeIP+5; i++ {
		if w := erneuerungUeberMux(t, mux, ip, `{}`, false); w.Code != http.StatusConflict {
			t.Fatalf("vor der Aktivierung: %d, erwartet 409", w.Code)
		}
	}
}

// Missbrauch (Pruefung #314, LOW-3): eine Muell-Unterschrift kostet keine
// Registerabfrage -- abgewiesen, bevor das Register gefragt wird. Eine
// gueltige Unterschrift kommt durch bis zum Register, auch in anderer
// Schreibweise (Unterschrift mit 0x und gross, Schluessel gross mit
// Leerzeichen, Wallet gemischt): dieselbe Pruefung wie in
// bescheinigungPruefen.
func TestErneuerung_UnterschriftVorDemRegister(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	ip := "198.51.100.80"
	erneuerungsGrenzeLeeren(ip)
	// Ohne Datenbank: das Register kennt keinen Schluessel -- wer bis dahin
	// kommt, bekommt "registered coordinator".
	mux := (&APIServer{state: newTestState()}).buildMux()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	wallet := "0x" + strings.Repeat("ab", 20)
	issued := nowUnix()
	anfrage := func(walletText, pubText, sig string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]interface{}{"wallet": walletText, "issued_at": issued,
			"public_key": pubText, "signature": sig})
		return erneuerungUeberMux(t, mux, ip, string(body), false)
	}
	pubHex := hex.EncodeToString(pub)
	if w := anfrage(wallet, pubHex, strings.Repeat("ab", 64)); w.Code != http.StatusForbidden ||
		!strings.Contains(w.Body.String(), "passt nicht") || strings.Contains(w.Body.String(), "registered coordinator") {
		t.Fatalf("Muell-Unterschrift nicht vor dem Register abgewiesen: %d %s", w.Code, w.Body.String())
	}
	gueltig := hex.EncodeToString(ed25519.Sign(priv, []byte(erneuerungsNachricht(wallet, issued))))
	for name, f := range map[string][3]string{
		"wie erwartet":          {wallet, pubHex, gueltig},
		"Unterschrift 0x/gross": {wallet, pubHex, "0x" + strings.ToUpper(gueltig)},
		"Schluessel gross":      {wallet, " " + strings.ToUpper(pubHex) + " ", gueltig},
		"Wallet gemischt":       {"0x" + strings.Repeat("aB", 20), pubHex, gueltig},
	} {
		if w := anfrage(f[0], f[1], f[2]); w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "registered coordinator") {
			t.Fatalf("%s: gueltige Unterschrift nicht bis zum Register: %d %s", name, w.Code, w.Body.String())
		}
	}
}
