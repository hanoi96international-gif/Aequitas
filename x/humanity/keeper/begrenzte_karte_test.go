package keeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// Missbrauch (Pruefung von #319, LOW-4): eine Karte der Grenzen je Absender
// waechst nicht ueber ihre Hoechstzahl. Voll heisst: neue Absender
// begrenzen, bekannte zaehlen weiter, Platz entsteht durch Loeschen.
func TestBegrenzteKarte_Hoechstzahl(t *testing.T) {
	k := &begrenzteKarte{name: "test", max: 3}
	for i := 0; i < 3; i++ {
		if _, ok := k.LoadOrStore(i, i); !ok {
			t.Fatalf("Schluessel %d abgelehnt, obwohl Platz ist", i)
		}
	}
	if _, ok := k.LoadOrStore(99, 99); ok {
		t.Fatal("voll: ein neuer Schluessel wurde gespeichert")
	}
	if k.Store(98, 98) {
		t.Fatal("voll: Store hat einen neuen Schluessel gespeichert")
	}
	if v, ok := k.LoadOrStore(1, -1); !ok || v != 1 {
		t.Fatalf("voll: ein bekannter Schluessel zaehlt nicht weiter (%v, %v)", v, ok)
	}
	if !k.Store(2, 22) {
		t.Fatal("voll: ein bekannter Schluessel laesst sich nicht neu setzen")
	}
	if _, ok := k.Load(99); ok {
		t.Fatal("ohne wertWennVoll liefert Load einen unbekannten Schluessel")
	}
	if n := k.anzahl.Load(); n != 3 {
		t.Fatalf("anzahl %d, erwartet 3", n)
	}
	if a := k.abgelehnt.Load(); a != 2 {
		t.Fatalf("abgelehnt %d, erwartet 2", a)
	}
	k.Delete(0)
	k.Delete(0) // zweimal loeschen zaehlt einmal
	if _, ok := k.LoadOrStore(99, 99); !ok {
		t.Fatal("nach dem Loeschen kein Platz")
	}
	if n := k.anzahl.Load(); n != 3 {
		t.Fatalf("anzahl %d nach Loeschen und Neuanlage, erwartet 3", n)
	}
}

// registerRateLimit: voll gilt ein unbekannter Schluessel als gerade
// gesperrt -- jeder Aufrufer prueft time.Since(ts) < Sperre.
func TestBegrenzteKarte_VollHeisstGesperrt(t *testing.T) {
	k := &begrenzteKarte{name: "test", max: 1, wertWennVoll: func() any { return time.Now() }}
	if _, ok := k.Load("a"); ok {
		t.Fatal("leer: unbekannter Schluessel gefunden")
	}
	k.Store("a", time.Now().Add(-time.Hour))
	ts, ok := k.Load("b")
	if !ok || time.Since(ts.(time.Time)) > time.Second {
		t.Fatalf("voll: unbekannter Schluessel nicht gesperrt (%v, %v)", ts, ok)
	}
	if ts, _ := k.Load("a"); time.Since(ts.(time.Time)) < time.Minute {
		t.Fatal("voll: ein bekannter Schluessel bekam den Sperrwert")
	}
}

// Die Zahl stimmt auch unter Nebenlaeufigkeit (-race): sie zaehlt nur echte
// Neuanlagen und echte Loeschungen.
func TestBegrenzteKarte_ZahlNebenlaeufig(t *testing.T) {
	k := &begrenzteKarte{name: "test", max: 1 << 30}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				key := (g*7 + i) % 300
				switch i % 3 {
				case 0:
					k.LoadOrStore(key, i)
				case 1:
					k.Store(key, i)
				default:
					k.Delete(key)
				}
			}
		}(g)
	}
	wg.Wait()
	echt := int64(0)
	k.Range(func(_, _ any) bool { echt++; return true })
	if n := k.anzahl.Load(); n != echt {
		t.Fatalf("anzahl %d, in der Karte %d", n, echt)
	}
}

// Missbrauch: ipBurst und rpcRateLimit voll -- ein neuer Absender wird
// begrenzt, ein bekannter zaehlt weiter.
func TestBegrenzteKarte_VollBegrenztNeueAbsender(t *testing.T) {
	ipBurst.Delete("test-voll:alt")
	ipBurst.Delete("test-voll:neu")
	rpcRateLimit.Delete("198.51.100.201")
	rpcRateLimit.Delete("198.51.100.202")
	if !burstErlaubt("test-voll:alt", 5, time.Minute) || rpcRateLimited("198.51.100.201") {
		t.Fatal("Vorbedingung: mit Platz begrenzt")
	}
	altBurst, altRPC := ipBurst.max, rpcRateLimit.max
	ipBurst.max, rpcRateLimit.max = ipBurst.anzahl.Load(), rpcRateLimit.anzahl.Load()
	t.Cleanup(func() {
		ipBurst.max, rpcRateLimit.max = altBurst, altRPC
		ipBurst.Delete("test-voll:alt")
		rpcRateLimit.Delete("198.51.100.201")
	})
	if burstErlaubt("test-voll:neu", 5, time.Minute) {
		t.Fatal("ipBurst voll: neuer Absender nicht begrenzt")
	}
	if !burstErlaubt("test-voll:alt", 5, time.Minute) {
		t.Fatal("ipBurst voll: bekannter Absender begrenzt")
	}
	if !rpcRateLimited("198.51.100.202") {
		t.Fatal("rpcRateLimit voll: neuer Absender nicht begrenzt")
	}
	if rpcRateLimited("198.51.100.201") {
		t.Fatal("rpcRateLimit voll: bekannter Absender begrenzt")
	}
	st := GrenzenJeAbsenderStand()
	for _, name := range []string{"ip_burst", "rpc", "register"} {
		if _, ok := st[name]; !ok {
			t.Fatalf("Stand ohne %s: %v", name, st)
		}
	}
}

// Missbrauch (Pruefung von #319, LOW-3): jede Adresse eines /64 hatte einen
// eigenen Zaehler -- 100 von 100 kamen durch. Jetzt zaehlt das /64.
func TestClientIP_IPv6JeNetz(t *testing.T) {
	anfrage := func(remote string) string {
		r := httptest.NewRequest("POST", "/api/prove", nil)
		r.RemoteAddr = remote
		return clientIP(r)
	}
	netz := anfrage("[2001:db8:1:2::1]:4000")
	if netz != "2001:db8:1:2::" {
		t.Fatalf("clientIP = %q, erwartet die Netzadresse 2001:db8:1:2::", netz)
	}
	for _, a := range []string{"[2001:db8:1:2::2]:1", "[2001:db8:1:2:ffff:ffff:ffff:ffff]:1", "[2001:DB8:1:2:0:0:0:9]:1"} {
		if got := anfrage(a); got != netz {
			t.Fatalf("%s zaehlt unter %q, nicht unter dem /64 %q", a, got, netz)
		}
	}
	if got := anfrage("[2001:db8:1:3::1]:1"); got == netz {
		t.Fatal("ein anderes /64 zaehlt unter demselben Schluessel")
	}
	for remote, want := range map[string]string{
		"[::ffff:198.51.100.7]:1": "198.51.100.7",
		"198.51.100.7:1":          "198.51.100.7",
		"[::1]:1":                 "::1",
	} {
		if got := anfrage(remote); got != want {
			t.Fatalf("clientIP(%s) = %q, erwartet %q", remote, got, want)
		}
	}
	// Die Grenze: 100 Adressen aus einem /64 teilen sich die 12 von /api/prove.
	schluessel := "prove:" + netz
	ipBurst.Delete(schluessel)
	t.Cleanup(func() { ipBurst.Delete(schluessel) })
	durch := 0
	for i := 0; i < 100; i++ {
		if burstErlaubt("prove:"+anfrage(fmt.Sprintf("[2001:db8:1:2::%x]:1", i+1)), burstProveJeIP, burstFenster) {
			durch++
		}
	}
	if durch != burstProveJeIP {
		t.Fatalf("%d von 100 Anfragen aus einem /64 durch, erwartet %d", durch, burstProveJeIP)
	}
}

// Missbrauch (Pruefung von #319, LOW-3): clientIP nahm den ERSTEN Eintrag von
// X-Forwarded-For -- den schreibt der Client. Haengt der Proxy an, waehlte
// der Client so seinen Zaehler. Jetzt der letzte, nur hinter dem eigenen
// Proxy.
func TestClientIP_XFFLetzterEintrag(t *testing.T) {
	for _, f := range []struct {
		name, remote string
		xff          []string
		want         string
	}{
		{"Proxy haengt an", "172.18.0.5:4000", []string{"6.6.6.6, 203.0.113.7"}, "203.0.113.7"},
		{"Proxy ersetzt", "172.18.0.5:4000", []string{"203.0.113.7"}, "203.0.113.7"},
		{"zwei Kopfzeilen", "172.18.0.5:4000", []string{"6.6.6.6", "7.7.7.7, 203.0.113.8"}, "203.0.113.8"},
		{"mit Port", "127.0.0.1:4000", []string{"6.6.6.6, 203.0.113.9:443"}, "203.0.113.9"},
		{"IPv6 im Kopf", "10.0.0.2:4000", []string{"6.6.6.6, [2001:db8:7:7::5]:443"}, "2001:db8:7:7::"},
		{"von aussen: Kopf zaehlt nicht", "198.51.100.9:4000", []string{"203.0.113.7"}, "198.51.100.9"},
		{"leerer letzter Eintrag", "172.18.0.5:4000", []string{"203.0.113.7, "}, "172.18.0.5"},
		{"Muell als letzter Eintrag", "172.18.0.5:4000", []string{"203.0.113.7, unknown"}, "172.18.0.5"},
		{"Zone als letzter Eintrag", "172.18.0.5:4000", []string{"203.0.113.7, fe80::1%eth0"}, "172.18.0.5"},
	} {
		r := httptest.NewRequest("POST", "/api/prove", nil)
		r.RemoteAddr = f.remote
		for _, v := range f.xff {
			r.Header.Add("X-Forwarded-For", v)
		}
		if got := clientIP(r); got != f.want {
			t.Errorf("%s: clientIP = %q, erwartet %q", f.name, got, f.want)
		}
	}
}

// PEER_PUSH_DENYLIST in der Schreibweise von clientIP: ein IPv6-Eintrag
// sperrt sein /64.
func TestAbsenderSchluessel_Denylist(t *testing.T) {
	if got := absenderSchluessel("2001:db8:5:6::1"); got != "2001:db8:5:6::" {
		t.Fatalf("absenderSchluessel = %q", got)
	}
	if got := absenderSchluessel("kein-ip"); got != "kein-ip" {
		t.Fatalf("Nicht-IP veraendert: %q", got)
	}
	m := pushDenylist(" 2001:db8:5:6::1, 178.105.186.119 ,::ffff:198.51.100.3,")
	for _, remote := range []string{"[2001:db8:5:6::77]:1", "178.105.186.119:1", "198.51.100.3:1"} {
		r := httptest.NewRequest("POST", "/api/blocks/push", nil)
		r.RemoteAddr = remote
		if !m[clientIP(r)] {
			t.Fatalf("%s nicht gesperrt (Liste %v)", remote, m)
		}
	}
	if len(m) != 3 {
		t.Fatalf("Liste %v, erwartet 3 Eintraege", m)
	}
}

// Missbrauch (Pruefung von #324, HIGH-1 und MEDIUM-2): /api/prove legte den
// Schluessel "prove-wallet:<frei gewaehlt>" an, bevor die Grenze je IP griff,
// und pruefte die Wallet nicht. Eine IP fuellte so die geteilte
// registerRateLimit -- voll gilt dort jeder neue Absender als gesperrt, quer
// ueber alle Funktionen bis zur Validator-Bindung --, mit Schluesseln bis
// 64 KiB. Jetzt: Grenze je IP zuerst, nur gueltige Adressen, eigene Karte.
func TestProveProxy_WalletSchluesselHinterDerIPGrenze(t *testing.T) {
	a := &APIServer{}
	anfrage := func(remote, wallet string) int {
		body, _ := json.Marshal(map[string]string{"wallet": wallet, "bio": "1", "salt": "2"})
		r := httptest.NewRequest("POST", "/api/prove", bytes.NewReader(body))
		r.RemoteAddr = remote
		w := httptest.NewRecorder()
		a.handleProveProxy(w, r)
		return w.Code
	}
	ipBurst.Delete("prove:198.51.100.70")
	ipBurst.Delete("prove:198.51.100.71")
	t.Cleanup(func() {
		ipBurst.Delete("prove:198.51.100.70")
		ipBurst.Delete("prove:198.51.100.71")
	})
	vorherWallet, vorherRegister := walletRateLimit.anzahl.Load(), registerRateLimit.anzahl.Load()

	// Ungueltige Wallets: 400, kein Schluessel, keine Buchung je IP.
	for _, w := range []string{"", "kein-hex-keine-adresse", "0x" + strings.Repeat("a", 60000), "0xzz00000000000000000000000000000000000000"} {
		if code := anfrage("198.51.100.70:1", w); code != http.StatusBadRequest {
			t.Fatalf("Wallet %.20q: Status %d, erwartet 400", w, code)
		}
	}
	if n := walletRateLimit.anzahl.Load(); n != vorherWallet {
		t.Fatalf("ungueltige Wallets haben %d Schluessel angelegt", n-vorherWallet)
	}

	// 40 gueltige Wallets von EINER IP: hoechstens burstProveJeIP Schluessel.
	for i := 0; i < 40; i++ {
		anfrage("198.51.100.70:1", fmt.Sprintf("0x%040x", 0xabc000+i))
	}
	if n := walletRateLimit.anzahl.Load() - vorherWallet; n > int64(burstProveJeIP) {
		t.Fatalf("eine IP hat %d Wallet-Schluessel angelegt, hoechstens %d erlaubt", n, burstProveJeIP)
	}
	if n := registerRateLimit.anzahl.Load(); n != vorherRegister {
		t.Fatalf("/api/prove schreibt in registerRateLimit (%d neue Schluessel)", n-vorherRegister)
	}

	// Die Grenze je Wallet bleibt: dieselbe Wallet von einer anderen IP binnen
	// 15 s ist begrenzt.
	wallet := fmt.Sprintf("0x%040x", 0xabc000)
	walletRateLimit.Delete("prove-wallet:" + wallet)
	t.Cleanup(func() {
		for i := 0; i < 40; i++ {
			walletRateLimit.Delete(fmt.Sprintf("prove-wallet:0x%040x", 0xabc000+i))
		}
	})
	if code := anfrage("198.51.100.71:1", wallet); code == http.StatusTooManyRequests {
		t.Fatal("Vorbedingung: erste Anfrage der Wallet begrenzt")
	}
	if code := anfrage("198.51.100.71:1", strings.ToUpper(wallet[:2])+wallet[2:]); code != http.StatusTooManyRequests {
		t.Fatalf("dieselbe Wallet (andere Schreibweise) binnen 15 s: Status %d, erwartet 429", code)
	}
}

// Das Aufraeumen schafft in beiden Sperrkarten Platz: abgelaufene Eintraege
// gehen, frische bleiben, und die Zahl sinkt mit.
func TestSperrKartenAufraeumen(t *testing.T) {
	jetzt := time.Now()
	for _, k := range []*begrenzteKarte{registerRateLimit, walletRateLimit, bindungRateLimit} {
		k.Store("test-alt", jetzt.Add(-time.Minute))
		k.Store("test-frisch", jetzt)
		t.Cleanup(func() { k.Delete("test-alt"); k.Delete("test-frisch") })
		vorher := k.anzahl.Load()
		sperrKartenAufraeumen(jetzt)
		if _, ok := k.m.Load("test-alt"); ok {
			t.Fatalf("%s: abgelaufener Eintrag nicht geloescht", k.name)
		}
		if _, ok := k.m.Load("test-frisch"); !ok {
			t.Fatalf("%s: frischer Eintrag geloescht", k.name)
		}
		if n := k.anzahl.Load(); n > vorher-1 {
			t.Fatalf("%s: Zahl %d nach dem Aufraeumen, vorher %d", k.name, n, vorher)
		}
	}
}

func proveProxyAnfrage(a *APIServer, remote, wallet string) int {
	body, _ := json.Marshal(map[string]string{"wallet": wallet, "bio": "1", "salt": "2"})
	r := httptest.NewRequest("POST", "/api/prove", bytes.NewReader(body))
	r.RemoteAddr = remote
	w := httptest.NewRecorder()
	a.handleProveProxy(w, r)
	return w.Code
}

// Pruefung von #324, INFO-7 und INFO-8 (M21): weder eine ungueltige Wallet
// noch die Wiederholung derselben Wallet bucht die Grenze je IP -- sonst
// zahlt die ganze Gruppe hinter einer Adresse fuer die Wiederholung eines
// Einzelnen.
func TestProveProxy_AbweisungBuchtKeineIP(t *testing.T) {
	a := &APIServer{}
	const ip = "198.51.100.72"
	wallet := fmt.Sprintf("0x%040x", 0xdef001)
	andere := fmt.Sprintf("0x%040x", 0xdef002)
	for _, k := range []string{"prove:" + ip} {
		ipBurst.Delete(k)
	}
	walletRateLimit.Delete("prove-wallet:" + wallet)
	walletRateLimit.Delete("prove-wallet:" + andere)
	t.Cleanup(func() {
		ipBurst.Delete("prove:" + ip)
		walletRateLimit.Delete("prove-wallet:" + wallet)
		walletRateLimit.Delete("prove-wallet:" + andere)
	})
	for i := 0; i < 20; i++ {
		if code := proveProxyAnfrage(a, ip+":1", "keine-wallet"); code != http.StatusBadRequest {
			t.Fatalf("ungueltige Wallet: Status %d", code)
		}
	}
	if _, ok := ipBurst.Load("prove:" + ip); ok {
		t.Fatal("ungueltige Wallets haben die Grenze je IP gebucht")
	}
	if code := proveProxyAnfrage(a, ip+":1", wallet); code == http.StatusTooManyRequests {
		t.Fatal("Vorbedingung: erste gueltige Anfrage begrenzt")
	}
	for i := 0; i < 20; i++ {
		if code := proveProxyAnfrage(a, ip+":1", wallet); code != http.StatusTooManyRequests {
			t.Fatalf("Wiederholung der Wallet binnen 15 s: Status %d, erwartet 429", code)
		}
	}
	v, _ := ipBurst.Load("prove:" + ip)
	e := v.(*ipBurstEintrag)
	e.mu.Lock()
	gebucht := len(e.zeiten)
	e.mu.Unlock()
	if gebucht != 1 {
		t.Fatalf("die Grenze je IP hat %d Buchungen, erwartet 1 (die Wiederholungen buchen nicht)", gebucht)
	}
	if code := proveProxyAnfrage(a, ip+":1", andere); code == http.StatusTooManyRequests {
		t.Fatal("eine andere Wallet derselben Adresse ist durch die Wiederholungen gesperrt")
	}
}

// Pruefung von #324, INFO-8 (M8): ist walletRateLimit voll, bekommt eine neue
// Wallet 429 (fail-closed) -- und die Funktionen hinter den anderen Karten
// bleiben offen.
func TestProveProxy_VolleWalletKarte(t *testing.T) {
	a := &APIServer{}
	const ip = "198.51.100.73"
	ipBurst.Delete("prove:" + ip)
	t.Cleanup(func() { ipBurst.Delete("prove:" + ip) })
	alt := walletRateLimit.max
	walletRateLimit.max = walletRateLimit.anzahl.Load()
	t.Cleanup(func() { walletRateLimit.max = alt })
	if code := proveProxyAnfrage(a, ip+":1", fmt.Sprintf("0x%040x", 0xdef010)); code != http.StatusTooManyRequests {
		t.Fatalf("volle Wallet-Karte: Status %d, erwartet 429", code)
	}
	if _, ok := ipBurst.Load("prove:" + ip); ok {
		t.Fatal("volle Wallet-Karte: die Grenze je IP wurde trotzdem gebucht")
	}
	if ts, ok := registerRateLimit.Load("set-guardian:" + ip); ok && time.Since(ts.(time.Time)) < time.Minute {
		t.Fatal("volle Wallet-Karte sperrt registerRateLimit")
	}
}

// Pruefung von #324, LOW-6: ist registerRateLimit voll (oeffentliche
// Endpunkte), bleibt die Validator-Bindung offen -- sie hat eine eigene Karte.
func TestBindungsGrenze_EigeneKarte(t *testing.T) {
	a := &APIServer{}
	alt := registerRateLimit.max
	registerRateLimit.max = registerRateLimit.anzahl.Load()
	t.Cleanup(func() { registerRateLimit.max = alt })
	h := a.bindungsGrenze(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	r := httptest.NewRequest("POST", "/api/validator-bindung", nil)
	r.RemoteAddr = "198.51.100.74:1"
	w := httptest.NewRecorder()
	h(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("volle registerRateLimit sperrt die Validator-Bindung: Status %d", w.Code)
	}
	if _, ok := registerRateLimit.m.Load("validator-bindung-fehl:198.51.100.74"); ok {
		t.Fatal("Bindungsschluessel in registerRateLimit")
	}
}

// Pruefung von #324, INFO-8 (M12): jede Karte hat ihre Hoechstzahl.
func TestBegrenzteKarte_HoechstzahlJeKarte(t *testing.T) {
	st := GrenzenJeAbsenderStand()
	for _, name := range []string{"ip_burst", "rpc", "register", "wallet", "bindung"} {
		w, ok := st[name].(map[string]int64)
		if !ok {
			t.Fatalf("Stand ohne %s: %v", name, st)
		}
		if w["hoechstens"] != grenzenSchluesselHoechstens {
			t.Fatalf("%s: hoechstens %d, erwartet %d", name, w["hoechstens"], grenzenSchluesselHoechstens)
		}
	}
}
