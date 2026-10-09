package keeper

import (
	"fmt"
	"net/http/httptest"
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
