package keeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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

// Die Zahlen beider Karten stimmen auch, wenn die volle Karte nebenlaeufig
// in die Netzkarte ueberlaeuft (-race).
func TestBegrenzteKarte_ZahlNebenlaeufigJeNetz(t *testing.T) {
	k := &begrenzteKarte{name: "test", max: 50, grob: &begrenzteKarte{name: "test_grob", max: 1 << 30}}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				key := fmt.Sprintf("x:2001:db8:%x:%x::", (g*7+i)%40, i%5)
				switch i % 3 {
				case 0:
					k.LoadOrStore(key, i)
				case 1:
					k.Store(key, i)
				default:
					k.Range(func(a, _ any) bool {
						if a == key || i%7 == 0 {
							k.Delete(a)
						}
						return true
					})
				}
			}
		}(g)
	}
	wg.Wait()
	for _, karte := range []*begrenzteKarte{k, k.grob} {
		echt := int64(0)
		karte.m.Range(func(_, _ any) bool { echt++; return true })
		if n := karte.anzahl.Load(); n != echt {
			t.Fatalf("%s: anzahl %d, in der Karte %d", karte.name, n, echt)
		}
	}
}

// Die Meldung einer vollen Karte ist gedrosselt: 1.000 abgewiesene und 1.000
// je Netz gezaehlte neue Absender binnen einer Minute ergeben je eine Zeile
// -- je Art ein eigener Zeitpunkt, die eine verdeckt die andere nicht
// (Pruefung von #326, LOW-1, fuer begrenzteKarte).
func TestBegrenzteKarte_MeldungGedrosselt(t *testing.T) {
	k := &begrenzteKarte{name: "test", max: 0, grob: &begrenzteKarte{name: "test_grob", max: 1 << 30}}
	for i := 0; i < 1000; i++ {
		k.LoadOrStore(fmt.Sprintf("x:0x%d", i), i)
		k.LoadOrStore(fmt.Sprintf("x:198.51.%d.1", i%250), i)
	}
	if a, g := k.abgelehnt.Load(), k.gegroebt.Load(); a != 1000 || g != 1000 {
		t.Fatalf("abgelehnt %d, je Netz %d -- erwartet je 1000", a, g)
	}
	if n := k.meldungen.Load(); n != 2 {
		t.Fatalf("%d Meldungen binnen einer Minute, erwartet 2 (je Art eine)", n)
	}
	k.gemeldet.Store(time.Now().Unix() - 61)
	for i := 0; i < 1000; i++ {
		k.LoadOrStore(fmt.Sprintf("x:0x%d", i), i)
		k.LoadOrStore(fmt.Sprintf("x:198.51.%d.1", i%250), i)
	}
	if n := k.meldungen.Load(); n != 3 {
		t.Fatalf("%d Meldungen nach einer Minute, erwartet 3 (nur die abgelaufene Art neu)", n)
	}
}

// vollMachen: k gilt als voll -- mit grob auch seine Ueberlaufkarte.
func vollMachen(t *testing.T, k *begrenzteKarte, grob bool) {
	t.Helper()
	alt := k.max
	k.max = k.anzahl.Load()
	t.Cleanup(func() { k.max = alt })
	if grob {
		altG := k.grob.max
		k.grob.max = k.grob.anzahl.Load()
		t.Cleanup(func() { k.grob.max = altG })
	}
}

// Das Netz eines Absenders: IPv4 je /24, IPv6 je /48 mit seinem /32; der
// Zweck zaehlt nicht mit. Ohne Adresse keins.
func TestNetzSchluessel(t *testing.T) {
	for key, want := range map[string][2]string{
		"198.51.100.7":               {"198.51.100.0/24", ""},
		"humans:198.51.100.7":        {"198.51.100.0/24", ""},
		"x:::ffff:198.51.100.7":      {"198.51.100.0/24", ""},
		"2001:db8:a:1::":             {"2001:db8:a::/48", "2001:db8::/32"},
		"prove:2001:db8:a:ffff::":    {"2001:db8:a::/48", "2001:db8::/32"},
		"validator-bindung-fehl:::1": {"::/48", "::/32"},
		"rpc:2001:DB8:A:0:0:0:0:9":   {"2001:db8:a::/48", "2001:db8::/32"},
	} {
		if n, p, ok := netzSchluessel(key); !ok || n != want[0] || p != want[1] {
			t.Errorf("netzSchluessel(%q) = %q, %q, %v -- erwartet %q, %q", key, n, p, ok, want[0], want[1])
		}
	}
	for _, key := range []any{"prove-wallet:0xabc", "validator-bindung-betreiber:0xabc", "x", "", "x:", "a:b:198.51.100.7", "x:198.51.100.0/24", 42} {
		if n, _, ok := netzSchluessel(key); ok {
			t.Errorf("netzSchluessel(%v) = %q, erwartet keins", key, n)
		}
	}
	for key, want := range map[string]string{"2001:db8:a::/48": "2001:db8::/32", "2001:db8:ffff::/48": "2001:db8::/32"} {
		if p, ok := praefixVonNetz(key); !ok || p != want {
			t.Errorf("praefixVonNetz(%q) = %q, %v", key, p, ok)
		}
	}
	for _, key := range []any{"198.51.100.0/24", "2001:db8::/32", "2001:db8:a::", 7} {
		if p, ok := praefixVonNetz(key); ok {
			t.Errorf("praefixVonNetz(%v) = %q, erwartet keins", key, p)
		}
	}
}

// Missbrauch (Pruefung des Folge-PR, LOW-1): ein /32 hat 65.536 /48 und
// fuellte allein jede Ueberlaufkarte. Jetzt zaehlen hoechstens grobJePraefix
// /48 eines /32 je fuer sich, jedes weitere unter dem /32; ein anderes /32
// und IPv4 sind davon frei. Wird ein /48 aufgeraeumt, hat sein /32 wieder
// Platz.
func TestBegrenzteKarte_JePraefixHoechstens(t *testing.T) {
	k := &begrenzteKarte{name: "test", max: 0, grob: &begrenzteKarte{name: "test_grob", max: 1 << 30, praefixe: &sync.Map{}}}
	for i := 0; i < grobJePraefix; i++ {
		if v, ok := k.LoadOrStore(fmt.Sprintf("x:2001:db8:%x:1::", i), i); !ok || v != i {
			t.Fatalf("/48 Nr. %d des /32 zaehlt nicht fuer sich (%v, %v)", i, v, ok)
		}
	}
	if v, ok := k.LoadOrStore("x:2001:db8:ff00:1::", 1000); !ok || v != 1000 {
		t.Fatalf("das /48 ueber der Quote bekommt keinen Eintrag des /32 (%v, %v)", v, ok)
	}
	if _, ok := k.grob.m.Load("2001:db8::/32"); !ok {
		t.Fatal("ueber der Quote zaehlt nicht das /32")
	}
	if v, ok := k.LoadOrStore("x:2001:db8:ff01:1::", 1001); !ok || v != 1000 {
		t.Fatalf("ein weiteres /48 desselben /32 zaehlt nicht unter dem /32 (%v, %v)", v, ok)
	}
	if v, ok := k.LoadOrStore("x:2001:db8:3:7::", 9); !ok || v != 3 {
		t.Fatalf("ein /48 mit Eintrag zaehlt nicht mehr unter sich (%v, %v)", v, ok)
	}
	if v, ok := k.LoadOrStore("x:2001:db9:1:1::", 2000); !ok || v != 2000 {
		t.Fatalf("ein anderes /32 ist mitbegrenzt (%v, %v)", v, ok)
	}
	if n := k.grob.anzahl.Load(); n != grobJePraefix+2 {
		t.Fatalf("grob hat %d Eintraege, erwartet %d", n, grobJePraefix+2)
	}
	k.Delete("2001:db8:5::/48")
	if v, ok := k.LoadOrStore("x:2001:db8:ff02:1::", 1002); !ok || v != 1002 {
		t.Fatalf("nach dem Aufraeumen eines /48 hat das /32 keinen Platz (%v, %v)", v, ok)
	}
	k.Range(func(key, _ any) bool { k.Delete(key); return true })
	leer := true
	k.grob.praefixe.Range(func(_, _ any) bool { leer = false; return false })
	if !leer || k.grob.anzahl.Load() != 0 {
		t.Fatalf("nach dem Aufraeumen: %d Eintraege, Zaehler je /32 leer %v", k.grob.anzahl.Load(), leer)
	}
}

// Missbrauch (Pruefung von #324, LOW-6): wer die Karte aus EINEM /48 fuellt
// (20.000 /64), sperrte bisher jeden neuen Absender. Jetzt teilen sich die
// neuen /64 des Angreifers einen Eintrag je Netz; ein anderes Netz hat seinen
// eigenen. Ohne Netz oder bei vollem grob: abweisen (fail-closed).
func TestBegrenzteKarte_VollZaehltJeNetz(t *testing.T) {
	k := &begrenzteKarte{name: "test", max: 2, grob: &begrenzteKarte{name: "test_grob", max: 3}}
	k.LoadOrStore("x:2001:db8:a:1::", 1)
	k.LoadOrStore("x:2001:db8:a:2::", 2)
	if v, ok := k.LoadOrStore("x:2001:db8:a:1::", 9); !ok || v != 1 {
		t.Fatalf("voll: ein bekannter Absender zaehlt nicht weiter unter seinem /64 (%v, %v)", v, ok)
	}
	if v, ok := k.LoadOrStore("x:2001:db8:a:3::", 3); !ok || v != 3 {
		t.Fatalf("voll: ein neues /64 bekommt keinen Eintrag seines Netzes (%v, %v)", v, ok)
	}
	if v, ok := k.LoadOrStore("x:2001:db8:a:ffff::", 4); !ok || v != 3 {
		t.Fatalf("voll: ein weiteres /64 desselben /48 zaehlt nicht unter dem Netz (%v, %v)", v, ok)
	}
	if v, ok := k.LoadOrStore("x:2001:db8:b:1::", 5); !ok || v != 5 {
		t.Fatalf("voll: ein anderes /48 teilt sich den Eintrag des Angreifers (%v, %v)", v, ok)
	}
	if !k.Store("x:198.51.100.7", 6) {
		t.Fatal("voll: Store legt keinen Eintrag des /24 an")
	}
	if v, ok := k.Load("x:198.51.100.200"); !ok || v != 6 {
		t.Fatalf("voll: eine andere Adresse desselben /24 sieht den Eintrag nicht (%v, %v)", v, ok)
	}
	if _, ok := k.grob.m.Load("2001:db8:a::/48"); !ok {
		t.Fatal("voll: das Netz zaehlt nicht unter seinem /48")
	}
	if v, ok := k.LoadOrStore("y:2001:db8:a:9::", 99); !ok || v != 3 {
		t.Fatalf("voll: ein anderer Zweck desselben Netzes zaehlt nicht mit (%v, %v)", v, ok)
	}
	if _, ok := k.m.Load("x:2001:db8:a:3::"); ok {
		t.Fatal("voll: das neue /64 steht in der vollen Karte")
	}
	if k.anzahl.Load() != 2 || k.grob.anzahl.Load() != 3 {
		t.Fatalf("anzahl %d/%d, erwartet 2/3", k.anzahl.Load(), k.grob.anzahl.Load())
	}
	// Ohne Netz: abgewiesen.
	if _, ok := k.LoadOrStore("x:0xabc", 7); ok {
		t.Fatal("voll: ein Schluessel ohne Adresse wurde gespeichert")
	}
	// Mit Absender: das Netz des Absenders.
	if v, ok := k.LoadUeber("w:0xabc", "x:198.51.100.9"); !ok || v != 6 {
		t.Fatalf("LoadUeber: das Netz des Absenders zaehlt nicht (%v, %v)", v, ok)
	}
	// grob voll: ein neues Netz wird abgewiesen, bekannte Netze zaehlen weiter.
	if _, ok := k.LoadOrStore("x:2001:db8:c:1::", 8); ok {
		t.Fatal("grob voll: ein neues Netz wurde gespeichert")
	}
	if k.Store("x:203.0.113.5", 8) {
		t.Fatal("grob voll: Store hat ein neues Netz gespeichert")
	}
	if v, ok := k.LoadOrStore("x:2001:db8:a:7::", 9); !ok || v != 3 {
		t.Fatalf("grob voll: ein bekanntes Netz zaehlt nicht weiter (%v, %v)", v, ok)
	}
	if a := k.abgelehnt.Load() + k.grob.abgelehnt.Load(); a != 3 {
		t.Fatalf("abgelehnt %d, erwartet 3", a)
	}
	// Range sieht beide Karten, Delete loescht in beiden.
	n := 0
	k.Range(func(key, _ any) bool { n++; k.Delete(key); return true })
	if n != 5 || k.anzahl.Load() != 0 || k.grob.anzahl.Load() != 0 {
		t.Fatalf("Range %d (erwartet 5), danach anzahl %d/%d", n, k.anzahl.Load(), k.grob.anzahl.Load())
	}
	// Range haelt an, wenn f false liefert -- auch vor grob.
	k.LoadOrStore("x:2001:db8:a:1::", 1)
	k.LoadOrStore("x:2001:db8:a:2::", 2)
	k.LoadOrStore("x:2001:db8:a:3::", 3)
	n = 0
	k.Range(func(_, _ any) bool { n++; return false })
	if n != 1 {
		t.Fatalf("Range nach false weiter: %d", n)
	}
}

// registerRateLimit (wertWennVoll): voll und das Netz unbekannt -- frei; das
// Netz gesperrt -- gesperrt; auch grob voll -- gesperrt (fail-closed).
func TestBegrenzteKarte_VollJeNetzGesperrt(t *testing.T) {
	jetzt := func() any { return time.Now() }
	k := &begrenzteKarte{name: "test", max: 1, wertWennVoll: jetzt, grob: &begrenzteKarte{name: "test_grob", max: 1, wertWennVoll: jetzt}}
	k.Store("humans:198.51.100.1", time.Now().Add(-time.Hour))
	if _, ok := k.Load("humans:203.0.113.1"); ok {
		t.Fatal("voll: ein neues Netz gilt als gesperrt")
	}
	k.Store("humans:203.0.113.1", time.Now())
	if ts, ok := k.Load("humans:203.0.113.99"); !ok || time.Since(ts.(time.Time)) > time.Second {
		t.Fatalf("voll: eine andere Adresse des gesperrten /24 ist frei (%v, %v)", ts, ok)
	}
	if ts, ok := k.Load("humans:192.0.2.1"); !ok || time.Since(ts.(time.Time)) > time.Second {
		t.Fatalf("grob voll: ein neues Netz ist nicht gesperrt (%v, %v)", ts, ok)
	}
	if ts, ok := k.Load("humans:0xabc"); !ok || time.Since(ts.(time.Time)) > time.Second {
		t.Fatalf("voll: ein Schluessel ohne Netz ist nicht gesperrt (%v, %v)", ts, ok)
	}
}

// Missbrauch (Pruefung von #324, LOW-6): ipBurst und rpcRateLimit voll -- ein
// bekannter Absender zaehlt weiter, ein neuer unter seinem Netz: das Netz des
// Angreifers teilt sich eine Grenze, ein anderes Netz kommt durch. Ohne Netz,
// oder wenn auch grob voll ist, wird ein neuer Absender begrenzt.
func TestBegrenzteKarte_VollBegrenztNeueAbsender(t *testing.T) {
	aufraeumen := func() {
		for _, k := range []string{"test-voll:alt", "test-voll:neu", "198.51.100.0/24", "203.0.113.0/24"} {
			ipBurst.Delete(k)
		}
		for _, k := range []string{"198.51.100.201", "198.51.100.0/24", "203.0.113.0/24"} {
			rpcRateLimit.Delete(k)
		}
	}
	aufraeumen()
	t.Cleanup(aufraeumen)
	if !burstErlaubt("test-voll:alt", 5, time.Minute) || rpcRateLimited("198.51.100.201") {
		t.Fatal("Vorbedingung: mit Platz begrenzt")
	}
	vollMachen(t, ipBurst, false)
	vollMachen(t, rpcRateLimit, false)
	if !burstErlaubt("test-voll:alt", 5, time.Minute) {
		t.Fatal("ipBurst voll: bekannter Absender begrenzt")
	}
	if rpcRateLimited("198.51.100.201") {
		t.Fatal("rpcRateLimit voll: bekannter Absender begrenzt")
	}
	if burstErlaubt("test-voll:neu", 5, time.Minute) {
		t.Fatal("ipBurst voll: neuer Absender ohne Netz nicht begrenzt")
	}
	for i := 0; i < 5; i++ {
		if !burstErlaubt(fmt.Sprintf("test-voll:198.51.100.%d", 10+i), 5, time.Minute) {
			t.Fatalf("ipBurst voll: Anfrage %d aus einem freien /24 begrenzt", i)
		}
	}
	if burstErlaubt("test-voll:198.51.100.99", 5, time.Minute) {
		t.Fatal("ipBurst voll: die Adressen eines /24 zaehlen nicht zusammen")
	}
	if !burstErlaubt("test-voll:203.0.113.5", 5, time.Minute) {
		t.Fatal("ipBurst voll: ein anderes /24 ist mitgesperrt")
	}
	for i := 0; i < rpcRateLimitMax; i++ {
		if rpcRateLimited(fmt.Sprintf("198.51.100.%d", 10+i%150)) {
			t.Fatalf("rpcRateLimit voll: Anfrage %d aus einem freien /24 begrenzt", i)
		}
	}
	if !rpcRateLimited("198.51.100.160") {
		t.Fatal("rpcRateLimit voll: die Adressen eines /24 zaehlen nicht zusammen")
	}
	if rpcRateLimited("203.0.113.5") {
		t.Fatal("rpcRateLimit voll: ein anderes /24 ist mitgesperrt")
	}
	vollMachen(t, ipBurst, true)
	vollMachen(t, rpcRateLimit, true)
	if burstErlaubt("test-voll:192.0.2.5", 5, time.Minute) {
		t.Fatal("ipBurst und grob voll: neues Netz nicht begrenzt")
	}
	if !rpcRateLimited("192.0.2.5") {
		t.Fatal("rpcRateLimit und grob voll: neues Netz nicht begrenzt")
	}
	st := GrenzenJeAbsenderStand()
	for _, name := range []string{"ip_burst", "rpc", "register"} {
		if _, ok := st[name]; !ok {
			t.Fatalf("Stand ohne %s: %v", name, st)
		}
	}
	if n := st["rpc"].(map[string]interface{})["je_netz_gezaehlt"].(int64); n < int64(rpcRateLimitMax) {
		t.Fatalf("Stand: je_netz_gezaehlt %d, erwartet mindestens %d", n, rpcRateLimitMax)
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
	for _, k := range []*begrenzteKarte{registerRateLimit, walletRateLimit, bindungRateLimit, betreiberRateLimit} {
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

// Pruefung des Folge-PR, LOW-2: die echten Aufraeumroutinen leeren auch die
// Ueberlaufkarten -- sonst bliebe grob nach 50.000 je gesehenen Netzen fuer
// immer voll, und jede spaetere volle Phase sperrte jedes neue Netz.
func TestAufraeumen_AuchJeNetz(t *testing.T) {
	jetzt := time.Now()
	const alt, frisch = "198.51.100.0/24", "2001:db8:77::/48"
	pruefen := func(k *begrenzteKarte, aufraeumen func()) {
		t.Helper()
		t.Cleanup(func() { k.grob.Delete(alt); k.grob.Delete(frisch) })
		vorher := k.grob.anzahl.Load()
		aufraeumen()
		if _, ok := k.grob.m.Load(alt); ok {
			t.Fatalf("%s: abgelaufenes Netz nicht geloescht", k.name)
		}
		if _, ok := k.grob.m.Load(frisch); !ok {
			t.Fatalf("%s: frisches Netz geloescht", k.name)
		}
		if n := k.grob.anzahl.Load(); n > vorher-1 {
			t.Fatalf("%s: grob %d nach dem Aufraeumen, vorher %d", k.name, n, vorher)
		}
	}
	for _, k := range []*begrenzteKarte{registerRateLimit, walletRateLimit, bindungRateLimit} {
		k.grob.Store(alt, jetzt.Add(-time.Minute))
		k.grob.Store(frisch, jetzt)
		pruefen(k, func() { sperrKartenAufraeumen(jetzt) })
	}
	ipBurst.grob.Store(alt, &ipBurstEintrag{zeiten: []time.Time{jetzt.Add(-2 * burstFenster)}})
	ipBurst.grob.Store(frisch, &ipBurstEintrag{zeiten: []time.Time{jetzt}})
	pruefen(ipBurst, func() { ipBurstAufraeumen(burstFenster) })
	rpcRateLimit.grob.Store(alt, &rpcRateLimitEntry{windowStart: jetzt.Add(-3 * rpcRateLimitWindow)})
	rpcRateLimit.grob.Store(frisch, &rpcRateLimitEntry{windowStart: jetzt})
	pruefen(rpcRateLimit, func() { rpcRateLimitAufraeumen(jetzt) })
	// Das /32 des aufgeraeumten /48 zaehlt mit.
	if z, ok := rpcRateLimit.grob.praefixe.Load("2001:db8::/32"); !ok || z.(*atomic.Int64).Load() != 1 {
		t.Fatalf("Zaehler des /32 nach dem Aufraeumen: %v", z)
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

// Pruefung von #325, INFO-6: ist walletRateLimit voll (Wallets aus vielen
// /64), zaehlt statt der Wallet das Netz des Absenders -- eine neue Wallet je
// 15 s je /24 bzw. /48, ein anderes Netz bleibt offen. Ist auch grob voll,
// bekommt eine neue Wallet 429 (fail-closed, Pruefung von #324, INFO-8, M8),
// ohne die Grenze je IP zu buchen, und die anderen Karten bleiben offen.
func TestProveProxy_VolleWalletKarte(t *testing.T) {
	a := &APIServer{}
	ips := []string{"198.51.100.73", "198.51.100.74", "203.0.113.73", "192.0.2.73", "2001:db8:a:1::", "2001:db8:a:2::"}
	netze := []string{"198.51.100.0/24", "203.0.113.0/24", "192.0.2.0/24", "2001:db8:a::/48"}
	aufraeumen := func() {
		for _, ip := range ips {
			ipBurst.Delete("prove:" + ip)
		}
		for _, n := range netze {
			walletRateLimit.Delete(n)
		}
	}
	aufraeumen()
	t.Cleanup(aufraeumen)
	wallet := func(i int) string { return fmt.Sprintf("0x%040x", 0xdef010+i) }
	vollMachen(t, walletRateLimit, false)
	if code := proveProxyAnfrage(a, "198.51.100.73:1", wallet(0)); code == http.StatusTooManyRequests {
		t.Fatal("volle Wallet-Karte: eine neue Wallet aus einem freien Netz bekommt 429")
	}
	if _, ok := walletRateLimit.m.Load("prove-wallet:" + wallet(0)); ok {
		t.Fatal("volle Wallet-Karte: der Wallet-Schluessel steht trotzdem in der Karte")
	}
	if code := proveProxyAnfrage(a, "198.51.100.74:1", wallet(1)); code != http.StatusTooManyRequests {
		t.Fatalf("volle Wallet-Karte: zweite neue Wallet aus demselben /24 binnen 15 s: Status %d, erwartet 429", code)
	}
	if _, ok := ipBurst.Load("prove:198.51.100.74"); ok {
		t.Fatal("volle Wallet-Karte: abgewiesen, aber die Grenze je IP gebucht")
	}
	if code := proveProxyAnfrage(a, "[2001:db8:a:1::5]:1", wallet(2)); code == http.StatusTooManyRequests {
		t.Fatal("volle Wallet-Karte: ein freies /48 bekommt 429")
	}
	if code := proveProxyAnfrage(a, "[2001:db8:a:2::5]:1", wallet(3)); code != http.StatusTooManyRequests {
		t.Fatalf("volle Wallet-Karte: ein anderes /64 desselben /48 binnen 15 s: Status %d, erwartet 429", code)
	}
	if code := proveProxyAnfrage(a, "203.0.113.73:1", wallet(4)); code == http.StatusTooManyRequests {
		t.Fatal("volle Wallet-Karte: ein anderes Netz ist mitgesperrt")
	}
	vollMachen(t, walletRateLimit, true)
	if code := proveProxyAnfrage(a, "192.0.2.73:1", wallet(5)); code != http.StatusTooManyRequests {
		t.Fatalf("Wallet-Karte und grob voll: Status %d, erwartet 429", code)
	}
	if _, ok := ipBurst.Load("prove:192.0.2.73"); ok {
		t.Fatal("Wallet-Karte und grob voll: die Grenze je IP wurde trotzdem gebucht")
	}
	if ts, ok := registerRateLimit.Load("set-guardian:192.0.2.73"); ok && time.Since(ts.(time.Time)) < time.Minute {
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
	for _, name := range []string{"ip_burst", "rpc", "register", "wallet", "bindung", "betreiber", "erneuerung_weitergeleitet"} {
		w, ok := st[name].(map[string]interface{})
		if !ok {
			t.Fatalf("Stand ohne %s: %v", name, st)
		}
		if w["hoechstens"] != int64(grenzenSchluesselHoechstens) {
			t.Fatalf("%s: hoechstens %v, erwartet %d", name, w["hoechstens"], grenzenSchluesselHoechstens)
		}
		g, ok := w["grob"].(map[string]int64)
		if name == "betreiber" || name == "erneuerung_weitergeleitet" {
			if ok {
				t.Fatalf("%s hat eine Ueberlaufkarte -- ihr Schluessel traegt keine Adresse des Absenders", name)
			}
			continue
		}
		if !ok || g["hoechstens"] != grobHoechstens {
			t.Fatalf("%s: grob %v, erwartet hoechstens %d", name, w["grob"], grobHoechstens)
		}
		// Speicher: 50.000 Netze sind bis 54 MB je Karte (ip_burst).
		if g["hoechstens"] > 50_000 {
			t.Fatalf("%s: grob haelt %d Netze -- mehr, als der Speicher der Ueberlaufkarten erlaubt", name, g["hoechstens"])
		}
	}
}

// Pruefung von #325, INFO-8 (M19) und #324, LOW-6: ist bindungRateLimit
// voll, zaehlt ein neuer Absender unter seinem Netz -- nach einer abgelehnten
// Bindung ist sein /24 bzw. /48 gesperrt, ein anderes Netz nicht. Ist auch
// grob voll, bekommt ein neuer Absender 429, bevor der Handler laeuft
// (fail-closed).
func TestBindungsGrenze_VolleKarteSperrtNeueAbsender(t *testing.T) {
	a := &APIServer{}
	netze := []string{"198.51.100.0/24", "203.0.113.0/24", "192.0.2.0/24", "2001:db8:a::/48", "2001:db8:b::/48"}
	aufraeumen := func() {
		for _, n := range netze {
			bindungRateLimit.Delete(n)
		}
	}
	aufraeumen()
	t.Cleanup(aufraeumen)
	vollMachen(t, bindungRateLimit, false)
	gerufen := false
	h := a.bindungsGrenze(func(w http.ResponseWriter, r *http.Request) {
		gerufen = true
		w.WriteHeader(http.StatusBadRequest)
	})
	posten := func(remote string) int {
		gerufen = false
		r := httptest.NewRequest("POST", "/api/validator-bindung", nil)
		r.RemoteAddr = remote
		w := httptest.NewRecorder()
		h(w, r)
		return w.Code
	}
	for _, f := range []struct{ erst, gleichesNetz, anderesNetz string }{
		{"198.51.100.75:1", "198.51.100.76:1", "203.0.113.75:1"},
		{"[2001:db8:a:1::1]:1", "[2001:db8:a:2::1]:1", "[2001:db8:b:1::1]:1"},
	} {
		if code := posten(f.erst); code != http.StatusBadRequest || !gerufen {
			t.Fatalf("volle Bindungskarte: %s aus einem freien Netz: Status %d, Handler %v", f.erst, code, gerufen)
		}
		if code := posten(f.gleichesNetz); code != http.StatusTooManyRequests || gerufen {
			t.Fatalf("volle Bindungskarte: %s nach Ablehnung im selben Netz: Status %d, Handler %v -- erwartet 429", f.gleichesNetz, code, gerufen)
		}
		if code := posten(f.anderesNetz); code != http.StatusBadRequest || !gerufen {
			t.Fatalf("volle Bindungskarte: %s aus einem anderen Netz: Status %d, Handler %v", f.anderesNetz, code, gerufen)
		}
	}
	vollMachen(t, bindungRateLimit, true)
	if code := posten("192.0.2.75:1"); code != http.StatusTooManyRequests || gerufen {
		t.Fatalf("Bindungskarte und grob voll: Status %d, Handler gerufen %v -- erwartet 429 ohne Handler", code, gerufen)
	}
}
