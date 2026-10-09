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
	"sync"
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
	if nochmal(teile[0]+";"+teile[1]+";"+teile[2]+";"+umgeformt(t, teile[3]), jetzt) != "" {
		t.Fatal("umgeformte Unterschrift nach dem Original anerkannt")
	}
	// Anderer Absender im Kopf als unterschrieben -- an einer frischen,
	// noch nie gesehenen Weiterleitung (sonst faengt schon die Merkliste).
	frisch := beimZustaendigen(t, folger, "203.0.113.23", nachweisIch, koerper, ms(9))
	ft := strings.Split(frisch.Header.Get(weiterleitungNachweisKopf), ";")
	if nochmal(ft[0]+";203.0.113.21;"+ft[2]+";"+ft[3], jetzt) != "" {
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
	// Ein Byte zu viel: der Folger unterschreibt ihn gar nicht (zumLeiter
	// antwortet 413, Pruefung von #319, MEDIUM-18).
	gross := strings.Repeat("x", weiterleitungKoerperMax+1)
	ueber := httptest.NewRequest(http.MethodPost, nachweisPfad, strings.NewReader(gross))
	ueber.RemoteAddr = "203.0.113.20:4711"
	if weiterleitungNachweis(folger, ueber, nachweisIch, []byte(gross), jetzt) != nil {
		t.Fatal("der Folger unterschreibt einen Koerper ueber der Lesegrenze")
	}
	// Kommt doch einer an (gefaelscht): nicht anerkannt, aber fuer den
	// Handler (der ihn selbst abweist) zurueckgelegt.
	r := beimZustaendigen(t, folger, "203.0.113.20", nachweisIch, koerper, ms(5))
	r.Body = io.NopCloser(strings.NewReader(gross))
	if cs.weiterleitungFuer(r, jetzt) != "" {
		t.Fatal("uebergrosser Koerper anerkannt")
	}
	if b, _ := io.ReadAll(r.Body); len(b) != weiterleitungKoerperMax+1 {
		t.Fatalf("zurueckgelegt %d Bytes, erwartet die gelesenen %d", len(b), weiterleitungKoerperMax+1)
	}
	// Zwillinge: dieselbe Anfrage zweimal in derselben Millisekunde -- beide
	// sind eigene Weiterleitungen (Pruefung von #319, MEDIUM-18).
	z1 := beimZustaendigen(t, folger, "203.0.113.24", nachweisIch, koerper, ms(8))
	z2 := beimZustaendigen(t, folger, "203.0.113.24", nachweisIch, koerper, ms(8))
	if cs.weiterleitungFuer(z1, jetzt) == "" || cs.weiterleitungFuer(z2, jetzt) == "" {
		t.Fatal("Zwillinge: der zweite gilt als Wiederholung")
	}
	// Ein endloser Koerper wird nicht ganz gelesen.
	leser := &zaehlLeser{}
	endlos := beimZustaendigen(t, folger, "203.0.113.20", nachweisIch, koerper, ms(6))
	endlos.Body = io.NopCloser(io.LimitReader(leser, 64<<20))
	cs.weiterleitungFuer(endlos, jetzt)
	if leser.gelesen > 2*(weiterleitungKoerperMax+1)+4096 {
		t.Fatalf("%d Bytes gelesen -- die Pruefung liest ohne Grenze", leser.gelesen)
	}
	// Ausgetauschte Zufallszahl.
	if nochmal(ft[0]+";"+ft[1]+";"+strings.Repeat("00", 16)+";"+ft[3], jetzt) != "" {
		t.Fatal("ausgetauschte Zufallszahl anerkannt")
	}
	// Kaputte Koepfe -- auch riesige (Pruefung von #319, LOW-19) -- kosten
	// keine Unterschriftspruefung.
	vorher := weiterleitungPruefungen.Load()
	n := strings.Repeat("ab", 16)
	for _, k := range []string{"", "x", "1;2;3", fmt.Sprintf("%d;203.0.113.20;%s;%s", jetzt.UnixMilli(), n, strings.Repeat("ab", 64)),
		fmt.Sprintf("%d;203.0.113.20;%s;%s", jetzt.UnixMilli(), n, strings.Repeat("ab", 500000)),
		strings.Repeat(";", 1<<20-1),
		fmt.Sprintf("%d;203.0.113.20;%s", jetzt.UnixMilli(), strings.Repeat("ab", 65)),
		fmt.Sprintf("%d;kein-ip;%s;%s", jetzt.UnixMilli(), n, strings.Repeat("ab", 65)),
		fmt.Sprintf("%d;::FFFF:203.0.113.20;%s;%s", jetzt.UnixMilli(), n, strings.Repeat("ab", 65)),
		fmt.Sprintf("%d;203.0.113.20;%s;%s", jetzt.UnixMilli(), strings.Repeat("zz", 16), strings.Repeat("ab", 65)),
		fmt.Sprintf("%d;203.0.113.20;%s;%s", jetzt.UnixMilli(), strings.Repeat("ab", 15), strings.Repeat("ab", 65)),
		fmt.Sprintf("%d;203.0.113.20;%s;%s;x", jetzt.UnixMilli(), n, strings.Repeat("ab", 65)),
		fmt.Sprintf("%d;203.0.113.20;%s;%s", jetzt.UnixMilli(), n, strings.Repeat("zz", 65))} {
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
	if cs.leitung.Load().SatzMitglied(nachweisIch) {
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
	// Nach dem Umlauf sind alle 50 juengsten bekannt -- der Ring verdraengt
	// reihum, nicht immer denselben Platz (Pruefung von #319, INFO-20).
	for i := 125; i < 175; i++ {
		if weiterleitungErstmals(fmt.Sprint(i), jetzt) {
			t.Fatalf("Eintrag %d (unter den 50 juengsten) gilt als neu", i)
		}
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
type folgerPaar struct {
	zustURL, zustTCP   string
	folgerMux          http.Handler
	folgerAdr          string
	folgerDAG          *BlockDAG
	beimZust, begrenzt *atomic.Int64
}

// neuesFolgerPaar: Folger (echter Mux, eigener Schluessel) und Zustaendiger
// (echter Mux hinter einem httptest-Server, Leiter, kennt den Folger als
// Mitglied des Satzes, Freiliste leer). Merkliste auf 50 Plaetze.
func neuesFolgerPaar(t *testing.T, aktiv bool) *folgerPaar {
	t.Helper()
	if aktiv {
		stagedGrantActivationOverride.Store(1)
		t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	}
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
	p := &folgerPaar{folgerAdr: adresseVon(folger), beimZust: &atomic.Int64{}, begrenzt: &atomic.Int64{}}
	zustAdr := "0x0000000000000000000000000000000000000001"
	zustLeitung := NeueLeitung(zustAdr, "", []string{zustAdr, p.folgerAdr}, zustAdr, true, LeitSpeicher{Term: 3, Leiter: zustAdr},
		testKonfig(), LeitUmgebung{Zugelassen: func(string) bool { return false }}, time.Now())
	zustCS := newTestState()
	zustCS.leitung.Store(zustLeitung)
	zustMux := (&APIServer{state: zustCS}).buildMux()
	zustSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		zustMux.ServeHTTP(rec, r)
		p.beimZust.Add(1)
		if rec.Code == http.StatusTooManyRequests {
			p.begrenzt.Add(1)
		}
		w.WriteHeader(rec.Code)
		io.Copy(w, rec.Body)
	}))
	t.Cleanup(zustSrv.Close)
	p.zustURL = zustSrv.URL
	zustTCP := strings.TrimPrefix(zustSrv.URL, "http://")
	zustTCP = zustTCP[:strings.LastIndex(zustTCP, ":")]
	erneuerungsGrenzeLeeren(zustTCP)
	ipBurst.Delete("liveness-renewal-pruefung:" + zustTCP)
	t.Cleanup(func() { erneuerungsGrenzeLeeren(zustTCP); ipBurst.Delete("liveness-renewal-pruefung:" + zustTCP) })
	p.zustTCP = zustTCP

	folgerLeitung := NeueLeitung(p.folgerAdr, "", []string{zustAdr, p.folgerAdr}, zustAdr, true, LeitSpeicher{Term: 3, Leiter: zustAdr},
		testKonfig(), LeitUmgebung{}, time.Now())
	folgerLeitung.SetzeURL(zustAdr, zustSrv.URL)
	folgerCS := newTestState()
	folgerCS.leitung.Store(folgerLeitung)
	p.folgerDAG = &BlockDAG{signingKey: folger}
	p.folgerMux = (&APIServer{state: folgerCS, blockchain: p.folgerDAG}).buildMux()
	return p
}

func (p *folgerPaar) leeren(t *testing.T, ips ...string) {
	t.Helper()
	erneuerungsGrenzeLeeren(ips...)
	for _, ip := range ips {
		ipBurst.Delete("liveness-renewal-von:" + p.folgerAdr + "|" + ip)
	}
	t.Cleanup(func() {
		erneuerungsGrenzeLeeren(ips...)
		for _, ip := range ips {
			ipBurst.Delete("liveness-renewal-von:" + p.folgerAdr + "|" + ip)
		}
	})
}

// Der Angriff aus der Pruefung von #319 (MEDIUM-7 und MEDIUM-11), mit echtem
// Mux bei Folger und Zustaendigem: der Folger steht NICHT in der Freiliste
// des Zustaendigen. Ein Angreifer schickt von vielen Adressen Muell an den
// Folger und fuellt dabei die Merkliste des Zustaendigen mehrfach. Der
// Zustaendige zaehlt jede Weiterleitung unter ihrem Absender -- ein
// ehrlicher Coordinator hinter dem Folger kommt weiter durch.
func TestWeiterleitungNachweis_FolgerNichtAusgesperrt(t *testing.T) {
	p := neuesFolgerPaar(t, true)
	folgerMux, folgerAdr, folgerDAG, beimZust, begrenztBeimZust := p.folgerMux, p.folgerAdr, p.folgerDAG, p.beimZust, p.begrenzt
	_ = folgerAdr
	body := func(i int) string { return fmt.Sprintf(`{"wallet":"0x%040x","issued_at":1}`, i+1) }
	var angreifer []string
	for i := 0; i < 12; i++ {
		angreifer = append(angreifer, fmt.Sprintf("203.0.113.%d", 100+i))
	}
	leeren := func() { p.leeren(t, angreifer...) }
	leeren()
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
	p.leeren(t, ehrlich)
	if w := erneuerungUeberMux(t, folgerMux, ehrlich, body(999), false); w.Code == http.StatusTooManyRequests {
		t.Fatalf("ehrlicher Coordinator hinter dem Folger ausgesperrt: %s", w.Body.String())
	}

	// Ohne Schluessel leitet der Folger nicht weiter (Pruefung von #319,
	// INFO-26) -- sonst zaehlte der Zustaendige unter seiner Adresse.
	folgerDAG.signingKey = nil
	vorher := beimZust.Load()
	if w := erneuerungUeberMux(t, folgerMux, ehrlich, body(998), false); w.Code != http.StatusServiceUnavailable || beimZust.Load() != vorher {
		t.Fatalf("Folger ohne Schluessel: %d, weitergeleitet %d", w.Code, beimZust.Load()-vorher)
	}
	// Gegenprobe: eine Weiterleitung ohne Nachweis (alter Folger) zaehlt
	// beim Zustaendigen unter der TCP-Adresse -- genau das Aussperren von
	// vorher.
	for i := 0; i < burstErneuerungJeIP+1; i++ {
		req, _ := http.NewRequest(http.MethodPost, p.zustURL+nachweisPfad, strings.NewReader(body(i)))
		req.Header.Set(weitergeleitetKopf, "1")
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	}
	if begrenztBeimZust.Load() == 0 {
		t.Fatal("Gegenprobe: ohne Nachweis begrenzt der Zustaendige nichts -- der Test misst nicht, was er soll")
	}
	// Ohne Nachweis-Kopf wird nichts geprueft und nichts ins Pruefbudget
	// gebucht (Pruefung von #319, INFO-34).
	if n := burstZahl("liveness-renewal-pruefung:" + p.zustTCP); n != 0 {
		t.Fatalf("Weiterleitungen ohne Nachweis haben %d Pruefungen gebucht", n)
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

// Die Kosten der Pruefung (Koerper lesen, ecrecover) deckelt eine eigene
// Grenze je Absender: nach burstNachweisPruefungJeIP gefaelschten Nachweisen
// steht der Zaehler der Pruefungen still (Pruefung von #319, LOW-13/M5 und
// LOW-24). Jede Anfrage mit gefaelschtem Nachweis zaehlt dabei unter dem
// Absender.
func TestWeiterleitungNachweis_BegrenzteKostenKeinePruefung(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	cs := nachweisLeitung(t)
	mux := (&APIServer{state: cs}).buildMux()
	ip := "198.51.100.170"
	leeren := func() { erneuerungsGrenzeLeeren(ip); ipBurst.Delete("liveness-renewal-pruefung:" + ip) }
	leeren()
	t.Cleanup(leeren)
	schicke := func() int {
		req := httptest.NewRequest(http.MethodPost, nachweisPfad, strings.NewReader(`{}`))
		req.RemoteAddr = ip + ":4711"
		req.Header.Set(weitergeleitetKopf, "1")
		req.Header.Set(weiterleitungNachweisKopf, fmt.Sprintf("%d;203.0.113.20;%s;%s", time.Now().UnixMilli(), strings.Repeat("ab", 16), strings.Repeat("ab", 65)))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w.Code
	}
	vorher, unbekannt := weiterleitungPruefungen.Load(), weiterleitungAbgelehnt[grundUnbekannt].Load()
	durch := 0
	for i := 0; i < burstNachweisPruefungJeIP+100; i++ {
		if schicke() != http.StatusTooManyRequests {
			durch++
		}
	}
	if durch != burstErneuerungJeIP {
		t.Fatalf("mit gefaelschtem Nachweis kamen %d durch, erwartet %d", durch, burstErneuerungJeIP)
	}
	if n := weiterleitungPruefungen.Load() - vorher; n != int64(burstNachweisPruefungJeIP) {
		t.Fatalf("%d Pruefungen, erwartet hoechstens %d", n, burstNachweisPruefungJeIP)
	}
	if n := weiterleitungAbgelehnt[grundUnbekannt].Load() - unbekannt; n != int64(burstNachweisPruefungJeIP) {
		t.Fatalf("%d als unbekannt abgelehnt, erwartet %d", n, burstNachweisPruefungJeIP)
	}
	if st := WeiterleitungNachweisStand(); st["abgelehnt"].(map[string]int64)["unbekannt"] < int64(burstNachweisPruefungJeIP) {
		t.Fatalf("/api/health/combined zeigt die Ablehnungen nicht: %v", st)
	}
}

// Der Angriff aus der Pruefung von #319 (LOW-24): gescheiterte Nachweise
// (etwa vor einem Satzwechsel, bei Uhrabweichung) fuellen den Zaehler der
// TCP-Adresse des Folgers. Ein gueltiger Nachweis danach wird trotzdem
// geprueft und unter seinem Absender gezaehlt.
func TestWeiterleitungNachweis_GescheiterteSperrenGueltigeNicht(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	folger, _ := crypto.GenerateKey()
	fremd, _ := crypto.GenerateKey()
	cs := nachweisLeitung(t, adresseVon(folger))
	mux := (&APIServer{state: cs}).buildMux()
	tcp, x := "198.51.100.150", "203.0.113.160"
	k := "liveness-renewal-von:" + adresseVon(folger) + "|" + x
	leeren := func() {
		erneuerungsGrenzeLeeren(tcp)
		ipBurst.Delete("liveness-renewal-pruefung:" + tcp)
		ipBurst.Delete(k)
	}
	leeren()
	t.Cleanup(leeren)
	for i := 0; i < burstErneuerungJeIP; i++ {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, beimZustaendigen(t, fremd, fmt.Sprintf("203.0.113.%d", 170+i%50), nachweisIch, fmt.Sprintf(`{"wallet":"0x%040x","issued_at":1}`, i+1), time.Now()))
	}
	if burstZahl("liveness-renewal:"+tcp) != burstErneuerungJeIP {
		t.Fatal("Vorbedingung: gescheiterte Nachweise zaehlen unter der TCP-Adresse")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, beimZustaendigen(t, folger, x, nachweisIch, `{"wallet":"0x00000000000000000000000000000000000000ee","issued_at":1}`, time.Now()))
	if w.Code == http.StatusTooManyRequests || burstZahl(k) != 1 {
		t.Fatalf("gueltiger Nachweis nach gescheiterten: %d (unter dem Absender %d)", w.Code, burstZahl(k))
	}
}

// Schreibweisen ueber den lokalen Weg (der Knoten nimmt selbst an, keine
// Leitung): drei Schreibweisen derselben Adresse zaehlen zusammen (Pruefung
// von #319, INFO-27).
func TestErneuerung_AbsenderNormalisiertLokal(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	mux := (&APIServer{state: newTestState()}).buildMux()
	erneuerungsGrenzeLeeren("203.0.113.161")
	t.Cleanup(func() { erneuerungsGrenzeLeeren("203.0.113.161") })
	durch := 0
	for i := 0; i < 2*burstErneuerungJeIP; i++ {
		req := httptest.NewRequest(http.MethodPost, nachweisPfad, strings.NewReader(`{}`))
		req.RemoteAddr = "172.18.0.5:4711"
		req.Header.Set("X-Forwarded-For", []string{"203.0.113.161", "::FFFF:203.0.113.161", "::ffff:cb00:71a1"}[i%3])
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusTooManyRequests {
			durch++
		}
	}
	if durch != burstErneuerungJeIP {
		t.Fatalf("drei Schreibweisen derselben Adresse: %d durch, erwartet %d", durch, burstErneuerungJeIP)
	}
}

// burstZahl: Buchungen von key im Fenster (nur fuer Tests).
func burstZahl(key string) int {
	v, ok := ipBurst.Load(key)
	if !ok {
		return 0
	}
	e := v.(*ipBurstEintrag)
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, z := range e.zeiten {
		if time.Since(z) < burstFenster {
			n++
		}
	}
	return n
}

// Der Angriff aus der Pruefung von #319 (MEDIUM-18 a): ein Absender schickt
// Erneuerungen ueber der Lesegrenze des Zustaendigen an den Folger. Der
// Folger unterschreibt und leitet sie nicht weiter (413); beim Zustaendigen
// faellt nichts unter die Adresse des Folgers, und ein ehrlicher Coordinator
// kommt durch.
func TestWeiterleitungNachweis_UebergrosserKoerperSperrtNichtAus(t *testing.T) {
	p := neuesFolgerPaar(t, true)
	angreifer, ehrlich := "203.0.113.150", "198.51.100.201"
	p.leeren(t, angreifer, ehrlich)
	gross := `{"wallet":"0x00000000000000000000000000000000000000ab","issued_at":1}` + strings.Repeat(" ", weiterleitungKoerperMax)
	for i := 0; i < burstErneuerungJeIP; i++ {
		if w := erneuerungUeberMux(t, p.folgerMux, angreifer, gross, false); w.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("Anfrage %d ueber der Lesegrenze: %d, erwartet 413", i+1, w.Code)
		}
	}
	if n := p.beimZust.Load(); n != 0 {
		t.Fatalf("%d uebergrosse Anfragen weitergeleitet", n)
	}
	if w := erneuerungUeberMux(t, p.folgerMux, ehrlich, `{"wallet":"0x00000000000000000000000000000000000000cd","issued_at":1}`, false); w.Code == http.StatusTooManyRequests || p.beimZust.Load() != 1 {
		t.Fatalf("ehrlicher Coordinator: %d (weitergeleitet %d)", w.Code, p.beimZust.Load())
	}
}

// Vor dem Stichtag antwortet schon der Folger 409: nichts gezaehlt,
// unterschrieben oder weitergeleitet (Pruefung von #319, INFO-22).
func TestWeiterleitungNachweis_VorDemStichtagNichtWeitergeleitet(t *testing.T) {
	p := neuesFolgerPaar(t, false)
	ip := "203.0.113.151"
	p.leeren(t, ip)
	for i := 0; i < 3; i++ {
		if w := erneuerungUeberMux(t, p.folgerMux, ip, `{"wallet":"0x00000000000000000000000000000000000000ab","issued_at":1}`, false); w.Code != http.StatusConflict {
			t.Fatalf("vor dem Stichtag: %d, erwartet 409", w.Code)
		}
	}
	if n := p.beimZust.Load(); n != 0 {
		t.Fatalf("vor dem Stichtag %d weitergeleitet", n)
	}
}

// Hinter einem privaten TCP-Partner bestimmt X-Forwarded-For den Absender.
// Eine Schreibweise zaehlt wie die andere. Ein Kopf ohne IP zaehlt seit #324
// unter dem Proxy selbst (clientIP) -- beim Folger unter der TCP-Adresse, und
// weitergeleitet fuer genau diese, beim Zustaendigen also unter (Folger,
// Proxy), nie unter dem Folger allein (Pruefung von #319, INFO-23). Ein
// Absender ohne IP wird nicht weitergeleitet (400, INFO-25).
func TestWeiterleitungNachweis_AbsenderNormalisiert(t *testing.T) {
	p := neuesFolgerPaar(t, true)
	tcp := "172.18.0.5"
	p.leeren(t, tcp, "203.0.113.152")
	schicke := func(xff string) int {
		req := httptest.NewRequest(http.MethodPost, nachweisPfad, strings.NewReader(`{"wallet":"0x00000000000000000000000000000000000000ab","issued_at":1}`))
		req.RemoteAddr = tcp + ":4711"
		req.Header.Set("X-Forwarded-For", xff)
		w := httptest.NewRecorder()
		p.folgerMux.ServeHTTP(w, req)
		return w.Code
	}
	durch := 0
	for i := 0; i < 2*burstErneuerungJeIP; i++ {
		if schicke([]string{"203.0.113.152", "::FFFF:203.0.113.152", "::ffff:cb00:7198"}[i%3]) != http.StatusTooManyRequests {
			durch++
		}
	}
	if durch != burstErneuerungJeIP {
		t.Fatalf("drei Schreibweisen derselben Adresse: %d durch, erwartet %d", durch, burstErneuerungJeIP)
	}
	vorher := p.beimZust.Load()
	durch = 0
	for i := 0; i < 2*burstErneuerungJeIP; i++ {
		if schicke(fmt.Sprintf("kein-ip-%d", i)) != http.StatusTooManyRequests {
			durch++
		}
	}
	if durch != burstErneuerungJeIP || burstZahl("liveness-renewal:"+tcp) != burstErneuerungJeIP {
		t.Fatalf("Koepfe ohne IP: %d durch, %d unter der TCP-Adresse gezaehlt, erwartet je %d", durch, burstZahl("liveness-renewal:"+tcp), burstErneuerungJeIP)
	}
	if n := p.beimZust.Load() - vorher; n != int64(burstErneuerungJeIP) {
		t.Fatalf("%d Anfragen weitergeleitet, erwartet %d", n, burstErneuerungJeIP)
	}
	if n := burstZahl("liveness-renewal-von:" + p.folgerAdr + "|" + tcp); n != burstErneuerungJeIP {
		t.Fatalf("beim Zustaendigen %d unter (Folger, Proxy) gezaehlt, erwartet %d", n, burstErneuerungJeIP)
	}
	// Ein Absender, der keine IP ist (etwa ein Unix-Socket): 400, ohne
	// Datenbank und ohne Weiterleitung.
	vorher = p.beimZust.Load()
	req := httptest.NewRequest(http.MethodPost, nachweisPfad, strings.NewReader(`{"wallet":"0x00000000000000000000000000000000000000ab","issued_at":1}`))
	req.RemoteAddr = "@"
	w := httptest.NewRecorder()
	p.folgerMux.ServeHTTP(w, req)
	t.Cleanup(func() { erneuerungsGrenzeLeeren("@") })
	if w.Code != http.StatusBadRequest || p.beimZust.Load() != vorher {
		t.Fatalf("ohne IP als Absender: %d (weitergeleitet %d), erwartet 400 ohne Weiterleitung", w.Code, p.beimZust.Load()-vorher)
	}
}

// Ein gueltiger Nachweis von einer Adresse der Freiliste wird gezaehlt, nicht
// freigestellt -- der Nachweis geht vor (Pruefung von #319, INFO-20).
func TestWeiterleitungNachweis_FreilisteMitNachweisGezaehlt(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	folger, _ := crypto.GenerateKey()
	cs := nachweisLeitung(t, adresseVon(folger))
	mux := (&APIServer{state: cs}).buildMux()
	alt := rpcRateLimitFreiListe.Load()
	t.Cleanup(func() { rpcRateLimitFreiListe.Store(alt) })
	frei := map[string]bool{"198.51.100.150": true} // TCP-Adresse in beimZustaendigen
	rpcRateLimitFreiListe.Store(&frei)
	x := "203.0.113.153"
	k := "liveness-renewal-von:" + adresseVon(folger) + "|" + x
	ipBurst.Delete(k)
	t.Cleanup(func() { ipBurst.Delete(k) })
	for i := 0; i < burstErneuerungJeIP; i++ {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, beimZustaendigen(t, folger, x, nachweisIch, fmt.Sprintf(`{"wallet":"0x%040x","issued_at":%d}`, i+1, i+1), time.Now()))
		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("Anfrage %d schon begrenzt", i+1)
		}
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, beimZustaendigen(t, folger, x, nachweisIch, `{"wallet":"0x00000000000000000000000000000000000000ff","issued_at":1}`, time.Now()))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("Nachweis von der Freiliste: Anfrage %d nicht begrenzt (%d) -- die Freiliste ging vor", burstErneuerungJeIP+1, w.Code)
	}
}

// Der Angriff aus der Pruefung von #319 (MEDIUM-28): 21 Absender schicken je
// 30 beliebige Erneuerungen ueber einen ehrlichen Folger -- 630 gueltig
// unterschriebene Weiterleitungen. Sie verbrauchen beim Zustaendigen kein
// Pruefbudget (gebucht wird nur ein abgelehnter Nachweis), und ehrliche
// Coordinatoren hinter dem Folger kommen danach durch.
func TestWeiterleitungNachweis_GueltigeVerbrauchenKeinPruefbudget(t *testing.T) {
	p := neuesFolgerPaar(t, true)
	var angreifer []string
	for i := 0; i < 21; i++ {
		angreifer = append(angreifer, fmt.Sprintf("203.0.113.%d", 200+i))
	}
	ehrlich := []string{"198.51.100.210", "198.51.100.211", "198.51.100.212"}
	p.leeren(t, append(angreifer, ehrlich...)...)
	for _, ip := range angreifer {
		for i := 0; i < burstErneuerungJeIP; i++ {
			erneuerungUeberMux(t, p.folgerMux, ip, fmt.Sprintf(`{"wallet":"0x%040x","issued_at":1}`, i+1), false)
		}
	}
	if n := burstZahl("liveness-renewal-pruefung:" + p.zustTCP); n != 0 {
		t.Fatalf("gueltige Nachweise haben %d Pruefungen gebucht", n)
	}
	for _, ip := range ehrlich {
		if w := erneuerungUeberMux(t, p.folgerMux, ip, `{"wallet":"0x00000000000000000000000000000000000000ee","issued_at":1}`, false); w.Code == http.StatusTooManyRequests {
			t.Fatalf("ehrlicher Coordinator %s hinter dem Folger ausgesperrt: %s", ip, w.Body.String())
		}
	}
	if n := p.begrenzt.Load(); n != 0 {
		t.Fatalf("der Zustaendige hat %d Weiterleitungen begrenzt", n)
	}
}

// warteLeser haelt den Koerper fest, bis frei geschlossen wird -- ein
// langsamer Absender. Der erste Lesezugriff zaehlt in wartend.
type warteLeser struct {
	frei    <-chan struct{}
	wartend *atomic.Int64
	einmal  sync.Once
}

func (w *warteLeser) Read(p []byte) (int, error) {
	w.einmal.Do(func() { w.wartend.Add(1) })
	<-w.frei
	return 0, io.EOF
}

// Der Angriff aus der Pruefung von #319 (LOW-30): viele gleichzeitige
// Faelschungen von einer Adresse, jede mit langsamem Koerper. Gebucht wird vor
// der Pruefung -- mehr als burstNachweisPruefungJeIP Pruefungen sind auch dann
// nicht offen, wenn noch keine abgeschlossen ist. Vorher sahen alle nur nach,
// ob das Budget voll ist, und kamen vorbei, bevor die erste buchte.
func TestWeiterleitungNachweis_GleichzeitigBegrenzt(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	folger, _ := crypto.GenerateKey()
	cs := nachweisLeitung(t, adresseVon(folger))
	h := (&APIServer{state: cs}).erneuerungsGrenze(func(w http.ResponseWriter, r *http.Request) {})
	ip := "198.51.100.171"
	leeren := func() { erneuerungsGrenzeLeeren(ip); ipBurst.Delete("liveness-renewal-pruefung:" + ip) }
	leeren()
	t.Cleanup(leeren)

	frei := make(chan struct{})
	var wartend, fertig atomic.Int64
	var wg sync.WaitGroup
	n := 3 * burstNachweisPruefungJeIP
	kopf := fmt.Sprintf("%d;203.0.113.20;%s;%s", time.Now().UnixMilli(), strings.Repeat("ab", 16), strings.Repeat("00", 65))
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, nachweisPfad, &warteLeser{frei: frei, wartend: &wartend})
			req.RemoteAddr = ip + ":4711"
			req.Header.Set(weitergeleitetKopf, "1")
			req.Header.Set(weiterleitungNachweisKopf, kopf)
			h(httptest.NewRecorder(), req)
			fertig.Add(1)
		}()
	}
	// Jede Anfrage haengt entweder im Koerper (in der Pruefung) oder ist
	// ohne Pruefung fertig.
	bis := time.Now().Add(30 * time.Second)
	for wartend.Load()+fertig.Load() < int64(n) {
		if time.Now().After(bis) {
			close(frei)
			wg.Wait()
			t.Fatalf("nach 30 s: %d in der Pruefung, %d fertig, von %d", wartend.Load(), fertig.Load(), n)
		}
		time.Sleep(time.Millisecond)
	}
	gleichzeitig := wartend.Load()
	vorher := weiterleitungPruefungen.Load()
	close(frei)
	wg.Wait()
	if gleichzeitig != int64(burstNachweisPruefungJeIP) {
		t.Fatalf("%d Pruefungen gleichzeitig offen, erwartet genau %d", gleichzeitig, burstNachweisPruefungJeIP)
	}
	if d := weiterleitungPruefungen.Load() - vorher; d != int64(burstNachweisPruefungJeIP) {
		t.Fatalf("%d Unterschriftspruefungen, erwartet %d", d, burstNachweisPruefungJeIP)
	}
	if b := burstZahl("liveness-renewal-pruefung:" + ip); b != burstNachweisPruefungJeIP {
		t.Fatalf("Pruefbudget: %d gebucht, erwartet %d", b, burstNachweisPruefungJeIP)
	}
}

// Der Angriff aus der Pruefung von #319 (LOW-31): ein bloss zugelassener
// Validator (nicht im Satz) unterschreibt von einer Adresse Weiterleitungen
// mit immer neuem Absender. Sein Nachweis gilt nicht -- alles zaehlt unter
// seiner Adresse, und das Pruefbudget deckelt die Kosten. Gegenprobe: als
// Mitglied des Satzes wird derselbe Nachweis anerkannt.
func TestWeiterleitungNachweis_NurDerSatz(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	zugelassen, _ := crypto.GenerateKey()
	l := NeueLeitung(nachweisIch, "", []string{nachweisIch}, nachweisIch, true, LeitSpeicher{Term: 3, Leiter: nachweisIch}, testKonfig(),
		LeitUmgebung{Zugelassen: func(a string) bool { return a == adresseVon(zugelassen) }}, time.Now())
	cs := newTestState()
	cs.leitung.Store(l)
	h := (&APIServer{state: cs}).erneuerungsGrenze(func(w http.ResponseWriter, r *http.Request) {})
	tcp := "198.51.100.150" // RemoteAddr aus beimZustaendigen
	leeren := func() { erneuerungsGrenzeLeeren(tcp); ipBurst.Delete("liveness-renewal-pruefung:" + tcp) }
	leeren()
	t.Cleanup(leeren)
	koerper := `{"wallet":"0x00000000000000000000000000000000000000aa","issued_at":1}`
	// Wie in Wirklichkeit meldet er sich erst (Hallo) und ist damit Bewerber
	// (Pruefung von #319, INFO-36).
	l.Empfange(LeitNachricht{Art: leitArtHallo, Von: adresseVon(zugelassen), ZeitMs: time.Now().UnixMilli()}, time.Now())
	l.mu.Lock()
	_, bewerber := l.bewerber[adresseVon(zugelassen)]
	l.mu.Unlock()
	if !bewerber || !l.zugelassen(adresseVon(zugelassen)) || l.SatzMitglied(adresseVon(zugelassen)) {
		t.Fatal("Vorbedingung: zugelassen und Bewerber, aber nicht im Satz")
	}
	unbekannt := weiterleitungAbgelehnt[grundUnbekannt].Load()
	durch, n := 0, 2*burstNachweisPruefungJeIP
	for i := 0; i < n; i++ {
		fuer := fmt.Sprintf("203.0.%d.%d", 1+i/250, 1+i%250)
		w := httptest.NewRecorder()
		h(w, beimZustaendigen(t, zugelassen, fuer, nachweisIch, koerper, time.Now()))
		if w.Code != http.StatusTooManyRequests {
			durch++
		}
	}
	if durch != burstErneuerungJeIP {
		t.Fatalf("ein zugelassener Validator ausserhalb des Satzes brachte %d durch, erwartet %d", durch, burstErneuerungJeIP)
	}
	if d := weiterleitungAbgelehnt[grundUnbekannt].Load() - unbekannt; d != int64(burstNachweisPruefungJeIP) {
		t.Fatalf("%d als unbekannt abgelehnt, erwartet %d (Pruefbudget)", d, burstNachweisPruefungJeIP)
	}
	// Gegenprobe: im Satz gilt derselbe Nachweis.
	cs2 := nachweisLeitung(t, adresseVon(zugelassen))
	if got := cs2.weiterleitungFuer(beimZustaendigen(t, zugelassen, "203.0.113.20", nachweisIch, koerper, time.Now()), time.Now()); got != adresseVon(zugelassen)+"|203.0.113.20" {
		t.Fatalf("Gegenprobe: Mitglied des Satzes nicht anerkannt (%q)", got)
	}
}

// burstErstatten nimmt genau die eigene Buchung zurueck, nicht die juengste:
// sonst verfiele eine fremde, aeltere frueher (Pruefung von #319, LOW-30).
func TestBurstErstatten_GenauDieEigeneBuchung(t *testing.T) {
	key := "test-erstatten"
	ipBurst.Delete(key)
	t.Cleanup(func() { ipBurst.Delete(key) })
	var zeiten []time.Time
	for i := 0; i < 3; i++ {
		z, ok := burstBuchen(key, 3, time.Minute)
		if !ok {
			t.Fatalf("Buchung %d abgelehnt", i+1)
		}
		zeiten = append(zeiten, z)
		time.Sleep(2 * time.Millisecond)
	}
	if _, ok := burstBuchen(key, 3, time.Minute); ok {
		t.Fatal("vierte Buchung trotz Grenze 3")
	}
	burstErstatten(key, zeiten[1])
	v, _ := ipBurst.Load(key)
	e := v.(*ipBurstEintrag)
	e.mu.Lock()
	rest := append([]time.Time{}, e.zeiten...)
	e.mu.Unlock()
	if len(rest) != 2 || !rest[0].Equal(zeiten[0]) || !rest[1].Equal(zeiten[2]) {
		t.Fatalf("nach Erstattung der mittleren Buchung: %v, erwartet %v und %v", rest, zeiten[0], zeiten[2])
	}
	if _, ok := burstBuchen(key, 3, time.Minute); !ok {
		t.Fatal("nach der Erstattung ist kein Platz frei")
	}
	// Eine unbekannte oder schon verfallene Buchung: nichts geschieht.
	burstErstatten(key, time.Unix(1, 0))
	burstErstatten("test-erstatten-gibt-es-nicht", time.Now())
	if n := burstZahl(key); n != 3 {
		t.Fatalf("Erstattung einer fremden Zeit hat %d uebrig gelassen, erwartet 3", n)
	}
}

// Nur der AKTUELLE Satz zaehlt (Pruefung von #319, INFO-36): ein Mitglied,
// das gerade entfernt wurde (steht noch im Vorgaenger-Satz), und eine
// Adresse, die ein Leiter nur in seinem Satz gezeigt hat, werden nicht
// anerkannt.
func TestWeiterleitungNachweis_NurDerAktuelleSatz(t *testing.T) {
	alt, _ := crypto.GenerateKey()
	gezeigt, _ := crypto.GenerateKey()
	bleibt := "0x00000000000000000000000000000000000000b1"
	cs := nachweisLeitung(t, adresseVon(alt), bleibt)
	l := cs.leitung.Load()
	koerper := `{"wallet":"0x00000000000000000000000000000000000000aa","issued_at":1}`
	pruefe := func(k *ecdsa.PrivateKey, fuer string) string {
		return cs.weiterleitungFuer(beimZustaendigen(t, k, fuer, nachweisIch, koerper, time.Now()), time.Now())
	}
	if got := pruefe(alt, "203.0.113.40"); got != adresseVon(alt)+"|203.0.113.40" {
		t.Fatalf("Vorbedingung: Mitglied anerkannt, bekam %q", got)
	}
	l.mu.Lock()
	l.aendere([]string{nachweisIch, bleibt}, "test: entfernt", time.Now())
	vorher := l.enthaelt(l.vorher, adresseVon(alt))
	l.gesehenSatz = normSatz([]string{nachweisIch, adresseVon(gezeigt)})
	l.mu.Unlock()
	if !vorher {
		t.Fatal("Vorbedingung: das entfernte Mitglied steht im Vorgaenger-Satz")
	}
	if got := pruefe(alt, "203.0.113.41"); got != "" {
		t.Fatalf("entferntes Mitglied (Vorgaenger-Satz) anerkannt: %q", got)
	}
	if got := pruefe(gezeigt, "203.0.113.42"); got != "" {
		t.Fatalf("nur gezeigter Satz anerkannt: %q", got)
	}
	// Wieder aufgenommen: sofort anerkannt.
	l.mu.Lock()
	l.aendere([]string{nachweisIch, bleibt, adresseVon(alt)}, "test: aufgenommen", time.Now())
	l.mu.Unlock()
	if got := pruefe(alt, "203.0.113.43"); got != adresseVon(alt)+"|203.0.113.43" {
		t.Fatalf("wieder aufgenommenes Mitglied nicht anerkannt: %q", got)
	}
}

// Der Angriff aus der Pruefung von #319 (LOW-35): beim Zustaendigen steht
// die Leitung (l.mu gehalten -- Empfange wartet darunter auf dag.mu,
// waehrend ein Block erzeugt wird). Gueltige Weiterleitungen eines Folgers
// warten nicht darauf: sie halten ihre Pruefbuchung nicht fest, kommen
// alle durch und hinterlassen kein gebuchtes Budget.
func TestWeiterleitungNachweis_KeinStauAnDerLeitung(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	folger, _ := crypto.GenerateKey()
	cs := nachweisLeitung(t, adresseVon(folger))
	l := cs.leitung.Load()
	h := (&APIServer{state: cs}).erneuerungsGrenze(func(w http.ResponseWriter, r *http.Request) {})
	tcp := "198.51.100.150" // RemoteAddr aus beimZustaendigen
	koerper := `{"wallet":"0x00000000000000000000000000000000000000aa","issued_at":1}`
	n := burstNachweisPruefungJeIP + 100
	var fuer []string
	for i := 0; i < n; i++ {
		fuer = append(fuer, fmt.Sprintf("203.0.%d.%d", 1+i/250, 1+i%250))
	}
	leeren := func() {
		erneuerungsGrenzeLeeren(tcp)
		ipBurst.Delete("liveness-renewal-pruefung:" + tcp)
		for _, f := range fuer {
			ipBurst.Delete("liveness-renewal-von:" + adresseVon(folger) + "|" + f)
		}
	}
	leeren()
	t.Cleanup(leeren)
	var anfragen []*http.Request
	for _, f := range fuer {
		anfragen = append(anfragen, beimZustaendigen(t, folger, f, nachweisIch, koerper, time.Now()))
	}

	l.mu.Lock()
	gesperrt := true
	defer func() {
		if gesperrt {
			l.mu.Unlock()
		}
	}()
	var begrenzt atomic.Int64
	fertig := make(chan struct{})
	go func() {
		defer close(fertig)
		var wg sync.WaitGroup
		weiter := make(chan *http.Request)
		for w := 0; w < 32; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for req := range weiter {
					rec := httptest.NewRecorder()
					h(rec, req)
					if rec.Code == http.StatusTooManyRequests {
						begrenzt.Add(1)
					}
				}
			}()
		}
		for _, req := range anfragen {
			weiter <- req
		}
		close(weiter)
		wg.Wait()
	}()
	select {
	case <-fertig:
	case <-time.After(20 * time.Second):
		l.mu.Unlock()
		gesperrt = false
		<-fertig
		t.Fatal("gueltige Weiterleitungen warten auf l.mu")
	}
	if b := begrenzt.Load(); b != 0 {
		t.Fatalf("%d gueltige Weiterleitungen begrenzt, waehrend die Leitung stand", b)
	}
	if b := burstZahl("liveness-renewal-pruefung:" + tcp); b != 0 {
		t.Fatalf("gueltige Weiterleitungen haben %d Pruefungen gebucht hinterlassen", b)
	}
}
