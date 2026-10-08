package keeper

import (
	"crypto/ecdsa"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

func weiterleitungsSchluesselSetzen(t *testing.T, k *ecdsa.PrivateKey) {
	t.Helper()
	alt := weiterleitungsSchluessel.Load()
	weiterleitungsSchluessel.Store(k)
	t.Cleanup(func() { weiterleitungsSchluessel.Store(alt) })
}

// unterschriebeneWeiterleitung baut die Anfrage, wie sie beim Zustaendigen
// ankommt: Kopf, Nachweis und Koerper.
func unterschriebeneWeiterleitung(t *testing.T, k *ecdsa.PrivateKey, koerper string, jetzt time.Time) *http.Request {
	t.Helper()
	weiterleitungsSchluesselSetzen(t, k)
	req := httptest.NewRequest(http.MethodPost, "/api/liveness-renewal", strings.NewReader(koerper))
	req.RemoteAddr = "198.51.100.150:4711"
	req.Header.Set(weitergeleitetKopf, "1")
	weiterleitungNachweisSetzen(req, "/api/liveness-renewal", []byte(koerper), jetzt)
	if req.Header.Get(weiterleitungNachweisKopf) == "" {
		t.Fatal("Vorbedingung: der Nachweis ist gesetzt")
	}
	return req
}

func nachweisLeitung(t *testing.T, mitglieder ...string) *ChainState {
	t.Helper()
	ich := "0x0000000000000000000000000000000000000003"
	satz := append([]string{ich}, mitglieder...)
	l := NeueLeitung(ich, "", satz, ich, true, LeitSpeicher{Term: 3, Leiter: ich}, testKonfig(),
		LeitUmgebung{Zugelassen: func(string) bool { return false }}, time.Now())
	cs := newTestState()
	cs.leitung.Store(l)
	return cs
}

// Missbrauch (Pruefung von #319, MEDIUM-7 und LOW-8): ausgenommen wird nur,
// was ein Validator der eigenen Leitung genau so unterschrieben hat -- frisch
// und nur einmal. Faelschung, fremder Absender, veraenderter Koerper,
// Wiederholung, alte Zeit, uebergrosser Koerper: alles zaehlt.
func TestWeiterleitungNachweis_NurEchteWeiterleitungen(t *testing.T) {
	folger, _ := crypto.GenerateKey()
	fremd, _ := crypto.GenerateKey()
	folgerAdr := strings.ToLower(crypto.PubkeyToAddress(folger.PublicKey).Hex())
	cs := nachweisLeitung(t, folgerAdr)
	jetzt := time.Now()
	koerper := `{"wallet":"0x00000000000000000000000000000000000000aa","issued_at":1}`

	echt := unterschriebeneWeiterleitung(t, folger, koerper, jetzt)
	if !cs.weiterleitungNachgewiesen(echt, jetzt) {
		t.Fatal("echte Weiterleitung nicht anerkannt")
	}
	if b, _ := io.ReadAll(echt.Body); string(b) != koerper {
		t.Fatalf("Koerper fuer den Handler nicht zurueckgelegt: %q", b)
	}
	// Dieselbe Weiterleitung noch einmal (abgefangen und wiederholt).
	wieder := httptest.NewRequest(http.MethodPost, "/api/liveness-renewal", strings.NewReader(koerper))
	wieder.Header = echt.Header.Clone()
	if cs.weiterleitungNachgewiesen(wieder, jetzt) {
		t.Fatal("Wiederholung anerkannt")
	}
	// Veraenderter Koerper unter derselben Unterschrift.
	anders := unterschriebeneWeiterleitung(t, folger, koerper, jetzt.Add(time.Millisecond))
	anders.Body = io.NopCloser(strings.NewReader(strings.Replace(koerper, "aa", "bb", 1)))
	if cs.weiterleitungNachgewiesen(anders, jetzt) {
		t.Fatal("veraenderter Koerper anerkannt")
	}
	// Anderer Pfad unter derselben Unterschrift.
	pfad := unterschriebeneWeiterleitung(t, folger, koerper, jetzt.Add(2*time.Millisecond))
	pfad.URL.Path = "/api/swap"
	if cs.weiterleitungNachgewiesen(pfad, jetzt) {
		t.Fatal("anderer Pfad anerkannt")
	}
	// Ein Schluessel, den die Leitung nicht kennt.
	if cs.weiterleitungNachgewiesen(unterschriebeneWeiterleitung(t, fremd, koerper, jetzt.Add(3*time.Millisecond)), jetzt) {
		t.Fatal("fremder Absender anerkannt")
	}
	// Ausserhalb des Zeitfensters, in beide Richtungen.
	for _, d := range []time.Duration{-leitungZeitfenster - time.Second, leitungZeitfenster + time.Second} {
		if cs.weiterleitungNachgewiesen(unterschriebeneWeiterleitung(t, folger, koerper, jetzt.Add(d)), jetzt) {
			t.Fatalf("Zeit %s neben jetzt anerkannt", d)
		}
	}
	// Uebergrosser Koerper (ein Byte zu viel, korrekt unterschrieben): nicht
	// anerkannt, aber fuer den Handler (der ihn selbst abweist) unveraendert
	// zurueckgelegt.
	gross := strings.Repeat("x", weiterleitungKoerperMax+1)
	r := unterschriebeneWeiterleitung(t, folger, gross, jetzt.Add(4*time.Millisecond))
	if cs.weiterleitungNachgewiesen(r, jetzt) {
		t.Fatal("uebergrosser Koerper anerkannt")
	}
	if b, _ := io.ReadAll(r.Body); len(b) != weiterleitungKoerperMax+1 {
		t.Fatalf("zurueckgelegt %d Bytes, erwartet die gelesenen %d", len(b), weiterleitungKoerperMax+1)
	}
	// Kaputte Koepfe.
	for _, kopf := range []string{"", "x", "1:zz", fmt.Sprintf("%d:%s", jetzt.UnixMilli(), strings.Repeat("ab", 64)),
		fmt.Sprintf("%d:%s", jetzt.UnixMilli(), strings.Repeat("ab", 65))} {
		req := httptest.NewRequest(http.MethodPost, "/api/liveness-renewal", strings.NewReader(koerper))
		req.Header.Set(weiterleitungNachweisKopf, kopf)
		if cs.weiterleitungNachgewiesen(req, jetzt) {
			t.Fatalf("Kopf %q anerkannt", kopf)
		}
	}
	// Ohne Leitung beim Empfaenger: nichts anerkannt.
	ohne := newTestState()
	if ohne.weiterleitungNachgewiesen(unterschriebeneWeiterleitung(t, folger, koerper, jetzt.Add(5*time.Millisecond)), jetzt) {
		t.Fatal("ohne Leitung anerkannt")
	}
	// Der eigene Schluessel des Empfaengers zaehlt nicht als Weiterleiter.
	ich := "0x0000000000000000000000000000000000000003"
	if cs.leitung.Load().KenntValidator(ich) {
		t.Fatal("der Knoten selbst gilt als Weiterleiter")
	}
}

// Die Merkliste gegen Wiederholung hat eine feste Grenze. Voll heisst: nicht
// ausnehmen (fail-closed), verfallene Eintraege machen wieder Platz.
func TestWeiterleitungNachweis_MerklisteBegrenzt(t *testing.T) {
	weiterleitungGemerkt.Lock()
	alt := weiterleitungGemerkt.bis
	weiterleitungGemerkt.bis = nil
	weiterleitungGemerkt.Unlock()
	t.Cleanup(func() {
		weiterleitungGemerkt.Lock()
		weiterleitungGemerkt.bis = alt
		weiterleitungGemerkt.Unlock()
	})
	jetzt := time.Now()
	for i := 0; i < weiterleitungGemerktMax; i++ {
		if !weiterleitungErstmals(fmt.Sprint(i), jetzt) {
			t.Fatalf("Eintrag %d abgelehnt, obwohl Platz war", i)
		}
	}
	if weiterleitungErstmals("einer zu viel", jetzt) {
		t.Fatal("volle Merkliste nimmt weiter an")
	}
	weiterleitungGemerkt.Lock()
	n := len(weiterleitungGemerkt.bis)
	weiterleitungGemerkt.Unlock()
	if n != weiterleitungGemerktMax {
		t.Fatalf("Merkliste hat %d Eintraege, Grenze %d", n, weiterleitungGemerktMax)
	}
	if !weiterleitungErstmals("spaeter", jetzt.Add(2*leitungZeitfenster+time.Second)) {
		t.Fatal("verfallene Eintraege machen keinen Platz")
	}
}

// Der Angriff aus der Pruefung von #319 (MEDIUM-7), mit echtem Mux bei
// Folger und Zustaendigem: der Folger steht NICHT in der Freiliste des
// Zustaendigen (Proxy, NAT, Name als URL). Ein Angreifer schickt von vielen
// Adressen Muell an den Folger, jede unter ihrer Grenze. Der Zustaendige
// zaehlt die unterschriebenen Weiterleitungen nicht unter der Adresse des
// Folgers -- ein ehrlicher Coordinator hinter dem Folger kommt weiter durch.
func TestWeiterleitungNachweis_FolgerNichtAusgesperrt(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	altFrei := rpcRateLimitFreiListe.Load()
	leer := map[string]bool{}
	rpcRateLimitFreiListe.Store(&leer)
	t.Cleanup(func() { rpcRateLimitFreiListe.Store(altFrei) })

	folger, _ := crypto.GenerateKey()
	folgerAdr := strings.ToLower(crypto.PubkeyToAddress(folger.PublicKey).Hex())
	weiterleitungsSchluesselSetzen(t, folger)
	zustAdr := "0x0000000000000000000000000000000000000001"

	// Der Zustaendige: Leiter, kennt den Folger als Mitglied des Satzes.
	zustLeitung := NeueLeitung(zustAdr, "", []string{zustAdr, folgerAdr}, zustAdr, true, LeitSpeicher{Term: 3, Leiter: zustAdr},
		testKonfig(), LeitUmgebung{Zugelassen: func(string) bool { return false }}, time.Now())
	zustCS := newTestState()
	zustCS.leitung.Store(zustLeitung)
	var beimZust, begrenztBeimZust atomic.Int64
	zustMux := (&APIServer{state: zustCS}).buildMux()
	zustSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		zustMux.ServeHTTP(rec, r)
		beimZust.Add(1)
		if rec.Code == http.StatusTooManyRequests {
			begrenztBeimZust.Add(1)
		}
		w.WriteHeader(rec.Code)
		io.Copy(w, rec.Body)
	}))
	defer zustSrv.Close()
	zustTCP := strings.TrimPrefix(zustSrv.URL, "http://")
	zustTCP = zustTCP[:strings.LastIndex(zustTCP, ":")]
	erneuerungsGrenzeLeeren(zustTCP)
	t.Cleanup(func() { erneuerungsGrenzeLeeren(zustTCP) })

	// Der Folger: leitet an den Zustaendigen weiter.
	folgerLeitung := NeueLeitung(folgerAdr, "", []string{zustAdr, folgerAdr}, zustAdr, true, LeitSpeicher{Term: 3, Leiter: zustAdr},
		testKonfig(), LeitUmgebung{}, time.Now())
	folgerLeitung.SetzeURL(zustAdr, zustSrv.URL)
	folgerCS := newTestState()
	folgerCS.leitung.Store(folgerLeitung)
	folgerMux := (&APIServer{state: folgerCS}).buildMux()

	body := func(i int) string { return fmt.Sprintf(`{"wallet":"0x%040x","issued_at":1}`, i+1) }
	var angreifer []string
	for i := 0; i < 12; i++ {
		angreifer = append(angreifer, fmt.Sprintf("203.0.113.%d", 100+i))
	}
	erneuerungsGrenzeLeeren(angreifer...)
	t.Cleanup(func() { erneuerungsGrenzeLeeren(angreifer...) })
	for _, ip := range angreifer {
		for i := 0; i < burstErneuerungJeIP; i++ {
			if w := erneuerungUeberMux(t, folgerMux, ip, body(i), false); w.Code == http.StatusTooManyRequests {
				t.Fatalf("Angreifer %s: Anfrage %d beim Folger begrenzt (%s)", ip, i+1, w.Body.String())
			}
		}
	}
	if n := beimZust.Load(); n != int64(len(angreifer)*burstErneuerungJeIP) {
		t.Fatalf("Vorbedingung: %d Weiterleitungen beim Zustaendigen, erwartet %d", n, len(angreifer)*burstErneuerungJeIP)
	}
	if n := begrenztBeimZust.Load(); n != 0 {
		t.Fatalf("der Zustaendige hat %d unterschriebene Weiterleitungen unter der Adresse des Folgers begrenzt", n)
	}
	ehrlich := "198.51.100.200"
	erneuerungsGrenzeLeeren(ehrlich)
	t.Cleanup(func() { erneuerungsGrenzeLeeren(ehrlich) })
	if w := erneuerungUeberMux(t, folgerMux, ehrlich, body(999), false); w.Code == http.StatusTooManyRequests {
		t.Fatalf("ehrlicher Coordinator hinter dem Folger ausgesperrt: %s", w.Body.String())
	}

	// Gegenprobe: ohne Unterschrift (alter Folger) zaehlt der Zustaendige
	// unter der Adresse des Folgers -- genau das Aussperren von vorher.
	weiterleitungsSchluessel.Store(nil)
	erneuerungsGrenzeLeeren(angreifer...)
	for _, ip := range angreifer {
		for i := 0; i < burstErneuerungJeIP; i++ {
			erneuerungUeberMux(t, folgerMux, ip, body(i), false)
		}
	}
	if begrenztBeimZust.Load() == 0 {
		t.Fatal("Gegenprobe: ohne Nachweis begrenzt der Zustaendige nichts -- der Test misst nicht, was er soll")
	}
}

// burstVoll bucht nichts: wer schon begrenzt ist, kostet keine
// Unterschriftspruefung, und die Abfrage selbst verbraucht kein Budget.
func TestBurstVoll_BuchtNichts(t *testing.T) {
	key := "test-burst-voll"
	ipBurst.Delete(key)
	t.Cleanup(func() { ipBurst.Delete(key) })
	for i := 0; i < 3; i++ {
		if burstVoll(key, 3, time.Minute) {
			t.Fatalf("nach %d Buchungen schon voll", i)
		}
		if !burstErlaubt(key, 3, time.Minute) {
			t.Fatalf("Buchung %d abgelehnt -- burstVoll hat gebucht", i+1)
		}
	}
	if !burstVoll(key, 3, time.Minute) {
		t.Fatal("nach 3 von 3 nicht voll")
	}
}
