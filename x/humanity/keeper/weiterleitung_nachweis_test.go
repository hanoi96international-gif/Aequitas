package keeper

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

const (
	nachweisIch  = "0x0000000000000000000000000000000000000003" // der Zustaendige
	nachweisPfad = "/api/liveness-renewal"
)

// beimZustaendigen baut die Anfrage, wie sie beim Zustaendigen ankommt: der
// Folger hat sie von fuer (Absender beim Folger) bekommen und fuer ziel
// unterschrieben.
func beimZustaendigen(t *testing.T, k *ecdsa.PrivateKey, fuer, ziel, koerper string, jetzt time.Time) *http.Request {
	t.Helper()
	beimFolger := httptest.NewRequest(http.MethodPost, nachweisPfad, strings.NewReader(koerper))
	beimFolger.RemoteAddr = fuer + ":4711"
	h := weiterleitungNachweis(k, beimFolger, ziel, []byte(koerper), jetzt)
	if h == nil {
		t.Fatal("Vorbedingung: der Folger unterschreibt")
	}
	req := httptest.NewRequest(http.MethodPost, nachweisPfad, strings.NewReader(koerper))
	req.RemoteAddr = "198.51.100.150:4711"
	req.Header.Set(weitergeleitetKopf, "1")
	for k, v := range h {
		req.Header[k] = v
	}
	return req
}

func nachweisLeitung(t *testing.T, mitglieder ...string) *ChainState {
	t.Helper()
	satz := append([]string{nachweisIch}, mitglieder...)
	l := NeueLeitung(nachweisIch, "", satz, nachweisIch, true, LeitSpeicher{Term: 3, Leiter: nachweisIch}, testKonfig(),
		LeitUmgebung{Zugelassen: func(string) bool { return false }}, time.Now())
	cs := newTestState()
	cs.leitung.Store(l)
	return cs
}

func adresseVon(k *ecdsa.PrivateKey) string {
	return strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())
}

// umgeformt: die zweite, ebenso gueltige Unterschrift (s -> n-s, v ^ 1).
func umgeformt(t *testing.T, sigHex string) string {
	t.Helper()
	sig, err := hex.DecodeString(sigHex)
	if err != nil || len(sig) != 65 {
		t.Fatalf("Unterschrift unlesbar: %v", err)
	}
	n := crypto.S256().Params().N
	s := new(big.Int).SetBytes(sig[32:64])
	s.Sub(n, s)
	out := append([]byte{}, sig[:32]...)
	out = append(out, s.FillBytes(make([]byte, 32))...)
	out = append(out, sig[64]^1)
	return hex.EncodeToString(out)
}

type zaehlLeser struct {
	gelesen int
}

func (z *zaehlLeser) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	z.gelesen += len(p)
	return len(p), nil
}

// Missbrauch (Pruefungen von #319, MEDIUM-7, LOW-8, INFO-14, LOW-13):
// anerkannt wird nur, was ein Validator der eigenen Leitung genau so fuer
// diesen Knoten unterschrieben hat -- frisch und nur einmal. Faelschung,
// fremder Absender, anderes Ziel, veraenderter Koerper, Pfad oder Absender,
// Wiederholung (auch umgeformt oder spaet im Fenster), alte Zeit,
// uebergrosser Koerper: alles zaehlt wie eine direkte Anfrage.
func TestWeiterleitungNachweis_NurEchteWeiterleitungen(t *testing.T) {
	folger, _ := crypto.GenerateKey()
	fremd, _ := crypto.GenerateKey()
	cs := nachweisLeitung(t, adresseVon(folger))
	jetzt := time.Now()
	koerper := `{"wallet":"0x00000000000000000000000000000000000000aa","issued_at":1}`
	ms := func(i int) time.Time { return jetzt.Add(time.Duration(i) * time.Millisecond) }

	echt := beimZustaendigen(t, folger, "203.0.113.20", nachweisIch, koerper, jetzt)
	if got := cs.weiterleitungFuer(echt, jetzt); got != adresseVon(folger)+"|203.0.113.20" {
		t.Fatalf("echte Weiterleitung: %q, erwartet Folger|Absender", got)
	}
	if b, _ := io.ReadAll(echt.Body); string(b) != koerper {
		t.Fatalf("Koerper fuer den Handler nicht zurueckgelegt: %q", b)
	}
	nochmal := func(kopf string, bei time.Time) string {
		req := httptest.NewRequest(http.MethodPost, nachweisPfad, strings.NewReader(koerper))
		req.Header.Set(weiterleitungNachweisKopf, kopf)
		return cs.weiterleitungFuer(req, bei)
	}
	kopf := echt.Header.Get(weiterleitungNachweisKopf)
	if nochmal(kopf, jetzt) != "" {
		t.Fatal("Wiederholung anerkannt")
	}
	// Umgeformte Unterschrift nach dem Original.
	teile := strings.Split(kopf, ";")
	if nochmal(teile[0]+";"+teile[1]+";"+umgeformt(t, teile[2]), jetzt) != "" {
		t.Fatal("umgeformte Unterschrift nach dem Original anerkannt")
	}
	// Anderer Absender im Kopf als unterschrieben -- an einer frischen,
	// noch nie gesehenen Weiterleitung (sonst faengt schon die Merkliste).
	frisch := beimZustaendigen(t, folger, "203.0.113.23", nachweisIch, koerper, ms(9))
	ft := strings.Split(frisch.Header.Get(weiterleitungNachweisKopf), ";")
	if nochmal(ft[0]+";203.0.113.21;"+ft[2], jetzt) != "" {
		t.Fatal("ausgetauschter Absender anerkannt")
	}
	if got := cs.weiterleitungFuer(frisch, jetzt); got != adresseVon(folger)+"|203.0.113.23" {
		t.Fatalf("das Original nach dem Austauschversuch: %q", got)
	}
	// Spaet im Fenster: Zeit 29 s voraus, Wiederholung 31 s spaeter.
	voraus := beimZustaendigen(t, folger, "203.0.113.22", nachweisIch, koerper, jetzt.Add(29*time.Second))
	if cs.weiterleitungFuer(voraus, jetzt) == "" {
		t.Fatal("Vorbedingung: 29 s voraus ist im Fenster")
	}
	if nochmal(voraus.Header.Get(weiterleitungNachweisKopf), jetzt.Add(31*time.Second)) != "" {
		t.Fatal("Wiederholung 31 s spaeter anerkannt")
	}
	// Veraenderter Koerper, anderer Pfad, anderes Ziel, fremder Schluessel.
	anders := beimZustaendigen(t, folger, "203.0.113.20", nachweisIch, koerper, ms(1))
	anders.Body = io.NopCloser(strings.NewReader(strings.Replace(koerper, "aa", "bb", 1)))
	if cs.weiterleitungFuer(anders, jetzt) != "" {
		t.Fatal("veraenderter Koerper anerkannt")
	}
	pfad := beimZustaendigen(t, folger, "203.0.113.20", nachweisIch, koerper, ms(2))
	pfad.URL.Path = "/api/swap"
	if cs.weiterleitungFuer(pfad, jetzt) != "" {
		t.Fatal("anderer Pfad anerkannt")
	}
	if cs.weiterleitungFuer(beimZustaendigen(t, folger, "203.0.113.20", "0x0000000000000000000000000000000000000004", koerper, ms(3)), jetzt) != "" {
		t.Fatal("Nachweis fuer einen anderen Knoten anerkannt")
	}
	if cs.weiterleitungFuer(beimZustaendigen(t, fremd, "203.0.113.20", nachweisIch, koerper, ms(4)), jetzt) != "" {
		t.Fatal("fremder Absender anerkannt")
	}
	// Ausserhalb des Zeitfensters, in beide Richtungen.
	for _, d := range []time.Duration{-leitungZeitfenster - time.Second, leitungZeitfenster + time.Second} {
		if cs.weiterleitungFuer(beimZustaendigen(t, folger, "203.0.113.20", nachweisIch, koerper, jetzt.Add(d)), jetzt) != "" {
			t.Fatalf("Zeit %s neben jetzt anerkannt", d)
		}
	}
	// Ein Byte zu viel, korrekt unterschrieben: nicht anerkannt, aber fuer
	// den Handler (der ihn selbst abweist) zurueckgelegt.
	gross := strings.Repeat("x", weiterleitungKoerperMax+1)
	r := beimZustaendigen(t, folger, "203.0.113.20", nachweisIch, gross, ms(5))
	if cs.weiterleitungFuer(r, jetzt) != "" {
		t.Fatal("uebergrosser Koerper anerkannt")
	}
	if b, _ := io.ReadAll(r.Body); len(b) != weiterleitungKoerperMax+1 {
		t.Fatalf("zurueckgelegt %d Bytes, erwartet die gelesenen %d", len(b), weiterleitungKoerperMax+1)
	}
	// Ein endloser Koerper wird nicht ganz gelesen.
	leser := &zaehlLeser{}
	endlos := beimZustaendigen(t, folger, "203.0.113.20", nachweisIch, koerper, ms(6))
	endlos.Body = io.NopCloser(io.LimitReader(leser, 64<<20))
	cs.weiterleitungFuer(endlos, jetzt)
	if leser.gelesen > 2*(weiterleitungKoerperMax+1)+4096 {
		t.Fatalf("%d Bytes gelesen -- die Pruefung liest ohne Grenze", leser.gelesen)
	}
	// Kaputte Koepfe -- auch ein riesiger -- kosten keine Unterschriftspruefung.
	vorher := weiterleitungPruefungen.Load()
	for _, k := range []string{"", "x", "1;2", fmt.Sprintf("%d;203.0.113.20;%s", jetzt.UnixMilli(), strings.Repeat("ab", 64)),
		fmt.Sprintf("%d;203.0.113.20;%s", jetzt.UnixMilli(), strings.Repeat("ab", 500000)),
		fmt.Sprintf("%d;kein-ip;%s", jetzt.UnixMilli(), strings.Repeat("ab", 65)),
		fmt.Sprintf("%d;::FFFF:203.0.113.20;%s", jetzt.UnixMilli(), strings.Repeat("ab", 65)),
		fmt.Sprintf("%d;203.0.113.20;%s", jetzt.UnixMilli(), strings.Repeat("zz", 65))} {
		if nochmal(k, jetzt) != "" {
			t.Fatalf("Kopf %.60q anerkannt", k)
		}
	}
	if n := weiterleitungPruefungen.Load() - vorher; n != 0 {
		t.Fatalf("%d Unterschriftspruefungen fuer kaputte Koepfe", n)
	}
	// Ohne Leitung beim Empfaenger: nichts anerkannt. Der Knoten selbst gilt
	// nie als Weiterleiter.
	if newTestState().weiterleitungFuer(beimZustaendigen(t, folger, "203.0.113.20", nachweisIch, koerper, ms(7)), jetzt) != "" {
		t.Fatal("ohne Leitung anerkannt")
	}
	if cs.leitung.Load().KenntValidator(nachweisIch) {
		t.Fatal("der Knoten selbst gilt als Weiterleiter")
	}
}

// Die Merkliste gegen Wiederholung hat eine feste Grenze. Voll heisst: den
// aeltesten Eintrag verdraengen (Pruefung von #319, MEDIUM-11 und LOW-12) --
// nie ablehnen, und nie ueber die Grenze wachsen.
func TestWeiterleitungNachweis_MerklisteVerdraengt(t *testing.T) {
	m := &weiterleitungGemerkt
	m.Lock()
	altE, altR, altK, altMax := m.eintraege, m.ring, m.kopf, weiterleitungGemerktMax
	m.eintraege, m.ring, m.kopf = nil, nil, 0
	weiterleitungGemerktMax = 50
	m.Unlock()
	t.Cleanup(func() {
		m.Lock()
		m.eintraege, m.ring, m.kopf, weiterleitungGemerktMax = altE, altR, altK, altMax
		m.Unlock()
	})
	jetzt := time.Now()
	for i := 0; i < 50; i++ {
		if !weiterleitungErstmals(fmt.Sprint(i), jetzt) {
			t.Fatalf("Eintrag %d abgelehnt", i)
		}
	}
	for i := 50; i < 175; i++ {
		if !weiterleitungErstmals(fmt.Sprint(i), jetzt) {
			t.Fatalf("Eintrag %d bei voller Merkliste abgelehnt", i)
		}
	}
	if weiterleitungErstmals("174", jetzt) {
		t.Fatal("die juengste Weiterleitung gilt als neu")
	}
	if !weiterleitungErstmals("0", jetzt) {
		t.Fatal("die aelteste wurde nicht verdraengt")
	}
	m.Lock()
	n, r := len(m.eintraege), len(m.ring)
	m.Unlock()
	if n > 50 || r > 50 {
		t.Fatalf("Merkliste %d Eintraege, Ring %d -- Grenze 50", n, r)
	}
	// Ein verfallener Eintrag gilt wieder als neu.
	if !weiterleitungErstmals("174", jetzt.Add(2*leitungZeitfenster+time.Second)) {
		t.Fatal("verfallener Eintrag gilt nicht als neu")
	}
}

// Der Angriff aus der Pruefung von #319 (MEDIUM-7 und MEDIUM-11), mit echtem
// Mux bei Folger und Zustaendigem: der Folger steht NICHT in der Freiliste
// des Zustaendigen. Ein Angreifer schickt von vielen Adressen Muell an den
// Folger und fuellt dabei die Merkliste des Zustaendigen mehrfach. Der
// Zustaendige zaehlt jede Weiterleitung unter ihrem Absender -- ein
// ehrlicher Coordinator hinter dem Folger kommt weiter durch.
func TestWeiterleitungNachweis_FolgerNichtAusgesperrt(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	altFrei := rpcRateLimitFreiListe.Load()
	leer := map[string]bool{}
	rpcRateLimitFreiListe.Store(&leer)
	t.Cleanup(func() { rpcRateLimitFreiListe.Store(altFrei) })
	m := &weiterleitungGemerkt
	m.Lock()
	altMax := weiterleitungGemerktMax
	weiterleitungGemerktMax = 50
	m.Unlock()
	t.Cleanup(func() { m.Lock(); weiterleitungGemerktMax = altMax; m.Unlock() })

	folger, _ := crypto.GenerateKey()
	folgerAdr := adresseVon(folger)
	zustAdr := "0x0000000000000000000000000000000000000001"

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

	folgerLeitung := NeueLeitung(folgerAdr, "", []string{zustAdr, folgerAdr}, zustAdr, true, LeitSpeicher{Term: 3, Leiter: zustAdr},
		testKonfig(), LeitUmgebung{}, time.Now())
	folgerLeitung.SetzeURL(zustAdr, zustSrv.URL)
	folgerCS := newTestState()
	folgerCS.leitung.Store(folgerLeitung)
	folgerDAG := &BlockDAG{signingKey: folger}
	folgerMux := (&APIServer{state: folgerCS, blockchain: folgerDAG}).buildMux()

	body := func(i int) string { return fmt.Sprintf(`{"wallet":"0x%040x","issued_at":1}`, i+1) }
	var angreifer []string
	for i := 0; i < 12; i++ {
		angreifer = append(angreifer, fmt.Sprintf("203.0.113.%d", 100+i))
	}
	leeren := func() {
		erneuerungsGrenzeLeeren(angreifer...)
		for _, ip := range angreifer {
			ipBurst.Delete("liveness-renewal-von:" + folgerAdr + "|" + ip)
		}
	}
	leeren()
	t.Cleanup(leeren)
	for _, ip := range angreifer {
		for i := 0; i < burstErneuerungJeIP; i++ {
			if w := erneuerungUeberMux(t, folgerMux, ip, body(i), false); w.Code == http.StatusTooManyRequests {
				t.Fatalf("Angreifer %s: Anfrage %d begrenzt (%s)", ip, i+1, w.Body.String())
			}
		}
	}
	if n := beimZust.Load(); n != int64(len(angreifer)*burstErneuerungJeIP) {
		t.Fatalf("Vorbedingung: %d Weiterleitungen beim Zustaendigen, erwartet %d", n, len(angreifer)*burstErneuerungJeIP)
	}
	if n := begrenztBeimZust.Load(); n != 0 {
		t.Fatalf("der Zustaendige hat %d unterschriebene Weiterleitungen begrenzt", n)
	}
	ehrlich := "198.51.100.200"
	erneuerungsGrenzeLeeren(ehrlich)
	ipBurst.Delete("liveness-renewal-von:" + folgerAdr + "|" + ehrlich)
	if w := erneuerungUeberMux(t, folgerMux, ehrlich, body(999), false); w.Code == http.StatusTooManyRequests {
		t.Fatalf("ehrlicher Coordinator hinter dem Folger ausgesperrt: %s", w.Body.String())
	}

	// Gegenprobe: ohne Schluessel (alter Folger) zaehlt der Zustaendige unter
	// der Adresse des Folgers -- genau das Aussperren von vorher.
	folgerDAG.signingKey = nil
	leeren()
	for _, ip := range angreifer {
		for i := 0; i < burstErneuerungJeIP; i++ {
			erneuerungUeberMux(t, folgerMux, ip, body(i), false)
		}
	}
	if begrenztBeimZust.Load() == 0 {
		t.Fatal("Gegenprobe: ohne Nachweis begrenzt der Zustaendige nichts -- der Test misst nicht, was er soll")
	}
}

// Beim Zustaendigen zaehlt eine nachgewiesene Weiterleitung unter (Folger,
// Absender), mit derselben Grenze wie eine direkte Anfrage. Ein Absender
// kommt ueber einen Folger nicht ueber burstErneuerungJeIP; ein anderer
// Absender hat sein eigenes Budget; und ein boeswilliger Validator, der fuer
// die Adresse eines ehrlichen Coordinators unterschreibt, verbraucht nur das
// Budget unter seinem eigenen Namen (Pruefung von #319, MEDIUM-11).
func TestWeiterleitungNachweis_ProAbsenderGezaehlt(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	folger, _ := crypto.GenerateKey()
	boese, _ := crypto.GenerateKey()
	cs := nachweisLeitung(t, adresseVon(folger), adresseVon(boese))
	mux := (&APIServer{state: cs}).buildMux()
	x, y := "203.0.113.30", "203.0.113.31"
	schluessel := []string{"liveness-renewal:198.51.100.150"}
	for _, k := range []*ecdsa.PrivateKey{folger, boese} {
		for _, ip := range []string{x, y} {
			schluessel = append(schluessel, "liveness-renewal-von:"+adresseVon(k)+"|"+ip)
		}
	}
	for _, s := range schluessel {
		ipBurst.Delete(s)
	}
	t.Cleanup(func() {
		for _, s := range schluessel {
			ipBurst.Delete(s)
		}
	})
	schicke := func(k *ecdsa.PrivateKey, fuer string, i int) int {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, beimZustaendigen(t, k, fuer, nachweisIch, fmt.Sprintf(`{"wallet":"0x%040x","issued_at":%d}`, i+1, i+1), time.Now()))
		return w.Code
	}
	for i := 0; i < burstErneuerungJeIP; i++ {
		if c := schicke(boese, x, i); c == http.StatusTooManyRequests {
			t.Fatalf("boeser Validator: Anfrage %d schon begrenzt", i+1)
		}
	}
	if c := schicke(boese, x, 100); c != http.StatusTooManyRequests {
		t.Fatalf("boeser Validator fuer %s: Anfrage %d nicht begrenzt (%d)", x, burstErneuerungJeIP+1, c)
	}
	// Der ehrliche Coordinator x ueber den ehrlichen Folger: eigenes Budget.
	for i := 0; i < burstErneuerungJeIP; i++ {
		if c := schicke(folger, x, 200+i); c == http.StatusTooManyRequests {
			t.Fatalf("ehrlicher Folger fuer %s: Anfrage %d begrenzt -- der boese Validator hat sein Budget verbraucht", x, i+1)
		}
	}
	if c := schicke(folger, x, 300); c != http.StatusTooManyRequests {
		t.Fatalf("ueber einen Folger kommt %s ueber die Grenze (%d)", x, c)
	}
	if c := schicke(folger, y, 400); c == http.StatusTooManyRequests {
		t.Fatalf("ein anderer Absender teilt das Budget von %s", x)
	}
	// Nichts davon zaehlte unter der TCP-Adresse der Folger.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, nachweisPfad, strings.NewReader(`{}`))
	req.RemoteAddr = "198.51.100.150:4711"
	mux.ServeHTTP(w, req)
	if w.Code == http.StatusTooManyRequests {
		t.Fatal("nachgewiesene Weiterleitungen zaehlten unter der TCP-Adresse")
	}
}

// Wer schon begrenzt ist, kostet keine Unterschriftspruefung mehr (Pruefung
// von #319, LOW-13/M5): nach burstErneuerungJeIP Anfragen mit gefaelschtem
// Nachweis bleibt der Zaehler der Pruefungen stehen.
func TestWeiterleitungNachweis_BegrenzteKostenKeinePruefung(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	cs := nachweisLeitung(t)
	mux := (&APIServer{state: cs}).buildMux()
	ip := "198.51.100.170"
	erneuerungsGrenzeLeeren(ip)
	t.Cleanup(func() { erneuerungsGrenzeLeeren(ip) })
	schicke := func() int {
		req := httptest.NewRequest(http.MethodPost, nachweisPfad, strings.NewReader(`{}`))
		req.RemoteAddr = ip + ":4711"
		req.Header.Set(weitergeleitetKopf, "1")
		req.Header.Set(weiterleitungNachweisKopf, fmt.Sprintf("%d;203.0.113.20;%s", time.Now().UnixMilli(), strings.Repeat("ab", 65)))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w.Code
	}
	vorher := weiterleitungPruefungen.Load()
	for i := 0; i < burstErneuerungJeIP; i++ {
		if schicke() == http.StatusTooManyRequests {
			t.Fatalf("Anfrage %d schon begrenzt", i+1)
		}
	}
	if n := weiterleitungPruefungen.Load() - vorher; n != int64(burstErneuerungJeIP) {
		t.Fatalf("%d Pruefungen fuer %d Anfragen", n, burstErneuerungJeIP)
	}
	for i := 0; i < 100; i++ {
		if schicke() != http.StatusTooManyRequests {
			t.Fatal("gefaelschter Nachweis ueber der Grenze nicht begrenzt")
		}
	}
	if n := weiterleitungPruefungen.Load() - vorher; n != int64(burstErneuerungJeIP) {
		t.Fatalf("%d Pruefungen nach der Grenze, erwartet weiter %d", n, burstErneuerungJeIP)
	}
}

// burstVoll bucht nichts und zaehlt nur, was im Fenster liegt.
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
	kurz := "test-burst-voll-kurz"
	ipBurst.Delete(kurz)
	t.Cleanup(func() { ipBurst.Delete(kurz) })
	for i := 0; i < 3; i++ {
		burstErlaubt(kurz, 3, 30*time.Millisecond)
	}
	time.Sleep(40 * time.Millisecond)
	if burstVoll(kurz, 3, 30*time.Millisecond) {
		t.Fatal("verfallene Eintraege zaehlen mit")
	}
}
