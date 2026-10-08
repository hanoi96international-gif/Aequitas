package keeper

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"filippo.io/edwards25519"
)

// Die acht Punkte kleiner Ordnung, wie sie als Schluessel eingereicht werden
// koennen (mit den nicht-kanonischen Schreibweisen von x = 0).
var ed25519KleineOrdnung = []string{
	"0100000000000000000000000000000000000000000000000000000000000000", // neutrales Element
	"ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f", // Ordnung 2
	"0000000000000000000000000000000000000000000000000000000000000000", // Ordnung 4
	"0000000000000000000000000000000000000000000000000000000000000080",
	"26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05", // Ordnung 8
	"26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc85",
	"c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac037a",
	"c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac03fa",
}

// Die Universalunterschrift zum neutralen Element: R = Basispunkt, s = 1.
const (
	ed25519NeutralSchluessel = "0100000000000000000000000000000000000000000000000000000000000000"
	ed25519NeutralUnterschr  = "5866666666666666666666666666666666666666666666666666666666666666" +
		"0100000000000000000000000000000000000000000000000000000000000000"
)

// Missbrauch (Sicherheitspruefung #300, MEDIUM-2): mit dem neutralen Element
// als Schluessel gilt dieselbe Unterschrift fuer JEDE Nachricht. Die strenge
// Pruefung weist sie ab -- Besitznachweis, Personhood-Nachweis, Bescheinigung.
func TestEd25519Streng_KleineOrdnungIstUniversalunterschrift(t *testing.T) {
	pub, _ := hex.DecodeString(ed25519NeutralSchluessel)
	sig, _ := hex.DecodeString(ed25519NeutralUnterschr)
	for _, msg := range []string{coordinatorBesitzNachricht("0xabc"), erneuerungsNachricht("0xdef", 1)} {
		if !ed25519.Verify(pub, []byte(msg), sig) {
			t.Logf("crypto/ed25519 lehnt die Universalunterschrift inzwischen selbst ab (%q)", msg)
		}
		if ed25519PruefenStreng(ed25519NeutralSchluessel, ed25519NeutralUnterschr, []byte(msg)) {
			t.Fatalf("Universalunterschrift angenommen fuer %q", msg)
		}
	}
	mensch := "0x" + strings.Repeat("ab", 20)
	if verifyCoordinatorPossession(ed25519NeutralSchluessel, ed25519NeutralUnterschr, mensch) {
		t.Fatal("Besitznachweis mit Schluessel kleiner Ordnung angenommen")
	}
	if verifyPersonhoodPossession(ed25519NeutralSchluessel, ed25519NeutralUnterschr, mensch) {
		t.Fatal("Personhood-Nachweis mit Schluessel kleiner Ordnung angenommen")
	}
	for _, k := range ed25519KleineOrdnung {
		b, _ := hex.DecodeString(k)
		if ed25519SchluesselTauglich(b) {
			t.Fatalf("Schluessel kleiner Ordnung %s angenommen", k)
		}
	}
}

// Nicht-kanonisch kodiert (y = p+1 statt 1) und gemischte Ordnung (echter
// Schluessel plus Punkt der Ordnung 2) taugen nicht; echte Schluessel taugen.
func TestEd25519Streng_KodierungUndOrdnung(t *testing.T) {
	// Alle nicht-kanonischen Kodierungen: y + p fuer y < 19, mit und ohne
	// Vorzeichenbit.
	for y := 0; y < 19; y++ {
		for _, vz := range []byte{0, 0x80} {
			b := make([]byte, 32)
			b[0] = byte(0xed + y)
			for i := 1; i < 31; i++ {
				b[i] = 0xff
			}
			b[31] = 0x7f | vz
			if ed25519SchluesselTauglich(b) {
				t.Fatalf("nicht-kanonische Kodierung angenommen: %x", b)
			}
		}
	}
	for i := 0; i < 20; i++ {
		pub, priv, _ := ed25519.GenerateKey(rand.Reader)
		if !ed25519SchluesselTauglich(pub) {
			t.Fatalf("echter Schluessel abgewiesen: %x", []byte(pub))
		}
		msg := []byte("nachricht")
		if !ed25519PruefenStreng(hex.EncodeToString(pub), hex.EncodeToString(ed25519.Sign(priv, msg)), msg) {
			t.Fatal("echte Unterschrift abgewiesen")
		}
		a, _ := new(edwards25519.Point).SetBytes(pub)
		t2, _ := new(edwards25519.Point).SetBytes(func() []byte { b, _ := hex.DecodeString(ed25519KleineOrdnung[1]); return b }())
		gemischt := new(edwards25519.Point).Add(a, t2).Bytes()
		if ed25519SchluesselTauglich(gemischt) {
			t.Fatalf("Schluessel gemischter Ordnung angenommen: %x", gemischt)
		}
	}
}

// Genau eine Schreibweise je Unterschrift: Grossbuchstaben, 0x, Rand oder
// angehaengte Zeichen werden abgewiesen -- ed25519SigNormal macht aus den
// ersten drei die eine.
func TestEd25519Streng_EineSchreibweise(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	msg := []byte("m")
	p, s := hex.EncodeToString(pub), hex.EncodeToString(ed25519.Sign(priv, msg))
	for name, v := range map[string]string{
		"gross": strings.ToUpper(s), "0x": "0x" + s, "rand": " " + s, "angehaengt": s + "00", "gekuerzt": s[:126],
	} {
		if ed25519PruefenStreng(p, v, msg) {
			t.Fatalf("%s angenommen", name)
		}
	}
	if ed25519PruefenStreng(strings.ToUpper(p), s, msg) {
		t.Fatal("Schluessel in Grossbuchstaben angenommen")
	}
	for _, v := range []string{strings.ToUpper(s), "0x" + s, " 0X" + strings.ToUpper(s) + " "} {
		if !ed25519PruefenStreng(p, ed25519SigNormal(v), msg) {
			t.Fatalf("angeglichen abgewiesen: %q", v)
		}
	}
}

// bescheinigungPruefen ohne Datenbank: jede Regel einzeln, mit ihrem Grund.
func TestBescheinigungPruefen_Regeln(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex := hex.EncodeToString(pub)
	mk, mensch := neuerSchluessel(t)
	wallet := "0x" + strings.Repeat("12", 20)
	issued := int64(1_800_000_000)
	signiere := func(w string) string {
		return hex.EncodeToString(ed25519.Sign(priv, []byte(erneuerungsNachricht(w, issued))))
	}
	bindung := CoordinatorBindung{Mensch: mensch,
		MenschSig:     personalSign(t, mk, coordinatorFreigabeNachricht(pubHex)),
		SchluesselSig: hex.EncodeToString(ed25519.Sign(priv, []byte(coordinatorBesitzNachricht(mensch))))}
	gut := func(m string) (bool, bool) { return m == mensch, false }
	zugelassen := func(m string, t int64) (bool, error) { return m == mensch && t == issued, nil }
	registerLeserOverride.Store(1)
	t.Cleanup(func() { registerLeserOverride.Store(0) })
	tx := erneuerungsTransaktion(wallet, issued, pubHex, signiere(wallet), bindung)
	if err := bescheinigungPruefen(wallet, issued, tx.Bescheinigung, gut, zugelassen); err != nil {
		t.Fatalf("gueltige Bescheinigung: %v", err)
	}
	// Die Transaktion traegt die eine Schreibweise, auch wenn der
	// Coordinator anders schickt.
	anders := erneuerungsTransaktion(wallet, issued, strings.ToUpper(pubHex), "0x"+strings.ToUpper(signiere(wallet)),
		CoordinatorBindung{Mensch: strings.ToUpper(mensch), MenschSig: bindung.MenschSig, SchluesselSig: strings.ToUpper(bindung.SchluesselSig)})
	if err := bescheinigungPruefen(wallet, issued, anders.Bescheinigung, gut, zugelassen); err != nil {
		t.Fatalf("angeglichene Bescheinigung: %v", err)
	}

	faelle := map[string]struct {
		b          func() *Lebendigkeitsbescheinigung
		stand      CoordinatorMenschStand
		grund      string
		zugelassen CoordinatorZulassung
	}{
		// Zulassung im Konsens (coordinator_zulassung.go).
		"kein Validator-Schluessel": {func() *Lebendigkeitsbescheinigung { c := *tx.Bescheinigung; return &c },
			gut, "nicht als Coordinator zugelassen", func(string, int64) (bool, error) { return false, nil }},
		"Zulassung unlesbar": {func() *Lebendigkeitsbescheinigung { c := *tx.Bescheinigung; return &c },
			gut, "Zulassung nicht lesbar", func(string, int64) (bool, error) { return true, fmt.Errorf("weg") }},
		"Mensch mit offener Staffel": {func() *Lebendigkeitsbescheinigung { c := *tx.Bescheinigung; return &c },
			func(string) (bool, bool) { return true, true }, "offene Staffel", nil},
		"kein Mensch": {func() *Lebendigkeitsbescheinigung { c := *tx.Bescheinigung; return &c },
			func(string) (bool, bool) { return false, false }, "kein registrierter Mensch", nil},
		"Freigabe mit v=0": {func() *Lebendigkeitsbescheinigung {
			c := *tx.Bescheinigung
			v := c.MenschSig[130:]
			c.MenschSig = c.MenschSig[:130] + map[string]string{"1b": "00", "1c": "01"}[v]
			return &c
		}, gut, "kanonischer Schreibweise", nil},
		"Freigabe gross": {func() *Lebendigkeitsbescheinigung {
			c := *tx.Bescheinigung
			c.MenschSig = "0x" + strings.ToUpper(c.MenschSig[2:])
			return &c
		}, gut, "kanonischer Schreibweise", nil},
		"Bescheinigung mit 0x": {func() *Lebendigkeitsbescheinigung {
			c := *tx.Bescheinigung
			c.Signature = "0x" + c.Signature
			return &c
		}, gut, "Bescheinigung passt nicht", nil},
		"Besitznachweis gross": {func() *Lebendigkeitsbescheinigung {
			c := *tx.Bescheinigung
			c.SchluesselSig = strings.ToUpper(c.SchluesselSig)
			return &c
		}, gut, "Besitznachweis", nil},
		"Schluessel gross": {func() *Lebendigkeitsbescheinigung {
			c := *tx.Bescheinigung
			c.PublicKey = strings.ToUpper(c.PublicKey)
			return &c
		}, gut, "64 Hex", nil},
		"Selbstbescheinigung": {func() *Lebendigkeitsbescheinigung { c := *tx.Bescheinigung; return &c },
			gut, "nicht selbst", nil},
		// Kleine Ordnung: der Mensch gibt das neutrale Element frei, Besitz
		// und Bescheinigung tragen die Universalunterschrift.
		"Schluessel kleiner Ordnung": {func() *Lebendigkeitsbescheinigung {
			return &Lebendigkeitsbescheinigung{PublicKey: ed25519NeutralSchluessel, Signature: ed25519NeutralUnterschr,
				Mensch: mensch, MenschSig: personalSign(t, mk, coordinatorFreigabeNachricht(ed25519NeutralSchluessel)),
				SchluesselSig: ed25519NeutralUnterschr}
		}, gut, "Besitznachweis", nil},
	}
	for name, f := range faelle {
		w := wallet
		if name == "Selbstbescheinigung" {
			w = mensch
		}
		z := f.zugelassen
		if z == nil {
			z = zugelassen
		}
		err := bescheinigungPruefen(w, issued, f.b(), f.stand, z)
		if err == nil || !strings.Contains(err.Error(), f.grund) {
			t.Fatalf("%s: %v (erwartet %q)", name, err, f.grund)
		}
	}
	// Missbrauch (#310, M1): eine Muell-Unterschrift kostet keine Abfrage --
	// die Zulassung (Datenbank) kommt erst nach jeder Unterschrift.
	muell := erneuerungsTransaktion(wallet, issued, pubHex, strings.Repeat("ab", 64), bindung)
	keineAbfrage := func(string, int64) (bool, error) {
		t.Fatal("Zulassung gelesen, bevor die Unterschrift geprueft war")
		return false, nil
	}
	if err := bescheinigungPruefen(wallet, issued, muell.Bescheinigung, gut, keineAbfrage); err == nil ||
		!strings.Contains(err.Error(), "Bescheinigung passt nicht") {
		t.Fatalf("Muell-Unterschrift: %v", err)
	}
	// Dasselbe fuer den Kettenzustand (#312, LOW-1): stand laedt das Konto
	// des Menschen, unter Umstaenden aus der Datenbank -- erst nach der
	// Unterschrift.
	keinStand := func(string) (bool, bool) {
		t.Fatal("Kettenzustand gelesen, bevor die Unterschrift geprueft war")
		return false, false
	}
	if err := bescheinigungPruefen(wallet, issued, muell.Bescheinigung, keinStand, keineAbfrage); err == nil ||
		!strings.Contains(err.Error(), "Bescheinigung passt nicht") {
		t.Fatalf("Muell-Unterschrift (Kettenzustand): %v", err)
	}
	// Vor registerLeserAb ist niemand zugelassen -- auch mit gueltiger
	// Bindung und Zulassung (fail-closed, wenn die Staffel frueher gaelte).
	registerLeserOverride.Store(issued + 1)
	if err := bescheinigungPruefen(wallet, issued, tx.Bescheinigung, gut, zugelassen); err == nil || !strings.Contains(err.Error(), "vor registerLeserAb") {
		t.Fatalf("vor registerLeserAb: %v", err)
	}
}

// Die Zulassung im Konsens setzt voraus, dass das Register gelesen wird:
// registerLeserAb darf nicht nach dem Staffel-Stichtag liegen. Solange die
// Staffel auf dem Platzhalter steht, gilt das nicht.
func TestStaffel_ZulassungVorDerStaffel(t *testing.T) {
	if stagedGrantActivationUnix != 4102444800 && registerLeserAbUnix > stagedGrantActivationUnix {
		t.Fatalf("Staffel ab %d, Coordinatoren erst ab registerLeserAb %d im Konsens zugelassen -- "+
			"jede Bescheinigung waere bis dahin ungueltig", stagedGrantActivationUnix, registerLeserAbUnix)
	}
}

// Die Erneuerung belastet das erneuerte Konto: anfrageKonten nennt es, und
// im verteilten Term leitet ein Folger ueber den Mux zu dessen Zustaendigem
// -- nicht zum Leiter (Sicherheitspruefung #300, MEDIUM-1). Vorher ging sie
// zum Leiter, der sie mit "nicht zustaendig" (503) abwies.
func TestZumLeiter_ErneuerungZumZustaendigen(t *testing.T) {
	wallet := "0x" + strings.Repeat("ab", 20)
	if got := anfrageKonten("/api/liveness-renewal", []byte(`{"wallet":" 0x`+strings.Repeat("AB", 20)+` "}`)); len(got) != 1 || got[0] != wallet {
		t.Fatalf("anfrageKonten: %v", got)
	}
	antwort := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			io.WriteString(w, name+":"+r.URL.Path+":"+string(b))
		}))
	}
	leiterSrv, zustSrv := antwort("leiter"), antwort("zustaendig")
	defer leiterSrv.Close()
	defer zustSrv.Close()
	ich := "0x0000000000000000000000000000000000000003"
	leiter := "0x0000000000000000000000000000000000000001"
	dritter := "0x0000000000000000000000000000000000000002"
	l := NeueLeitung(ich, "", []string{ich, leiter, dritter}, leiter, true, LeitSpeicher{Term: 3, Leiter: leiter}, testKonfig(), LeitUmgebung{}, time.Now())
	l.SetzeURL(leiter, leiterSrv.URL)
	l.SetzeURL(dritter, zustSrv.URL)
	zuteilung := []string{ich, leiter, dritter}
	l.mu.Lock()
	l.vt.term = l.term
	l.vt.zuteilung = zuteilung
	term := l.term
	l.mu.Unlock()
	// Ein Konto, fuer das der dritte zustaendig ist.
	var konto string
	for i := 0; i < 10000 && konto == ""; i++ {
		k := fmt.Sprintf("0x%040x", i+1)
		if zustaendigFuer(k, term, zuteilung, nil, leiter) == dritter {
			konto = k
		}
	}
	if konto == "" {
		t.Fatal("kein Konto fuer den dritten gefunden")
	}
	cs := newTestState()
	cs.leitung.Store(l)
	mux := (&APIServer{state: cs}).buildMux()
	body := `{"wallet":"` + konto + `","issued_at":1}`
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/liveness-renewal", strings.NewReader(body)))
	if got := rec.Body.String(); got != "zustaendig:/api/liveness-renewal:"+body {
		t.Fatalf("Erneuerung ging an %q, erwartet der Zustaendige des Kontos", got)
	}
}

// Die Staffel schlaeft, bis Coordinatoren im Konsens zugelassen werden
// (HIGH-1), der strenge Modus spaetestens mit ihr beginnt (LOW-4) und
// Bindung und Bescheinigung die Chain-ID tragen (LOW-3). Sonst schaltete eine
// erfundene Erneuerung im Beobachtungsmodus frei, und jeder registrierte
// Mensch -- auch der einer Farm -- bescheinigte. Und die Zulassung muss
// etwas Knappes kosten (#310, H1): ein Validator-Schluessel allein kostet
// eine Farm nichts, solange sich jeder Mensch einen eintragen kann.
//
// Gegen das Literal, nicht gegen eine Konstante (zweiter
// Sicherheitsdurchgang #300): wer den Stichtag setzt, muss diesen Test
// aendern -- und ersetzt ihn dann durch Verhaltenstests fuer die drei
// Bedingungen, statt einen Schalter umzulegen.
func TestStaffel_SchlaeftBisZulassungUndStreng(t *testing.T) {
	if stagedGrantActivationUnix != 4102444800 {
		t.Fatalf("stagedGrantActivationUnix = %d: vor der Staffel muessen stehen (1) Zulassung und Entzug der "+
			"Coordinatoren im Konsens, (2) nachrechnenStrengAbUnix <= Staffel-Stichtag (heute %d), "+
			"(3) Chain-ID in Bindung und Bescheinigung, (4) die Zulassung kostet etwas Knappes (#310, H1: ein "+
			"Validator-Schluessel allein kostet eine Farm nichts) -- jeweils mit Verhaltenstest", stagedGrantActivationUnix, nachrechnenStrengAbUnix)
	}
}
