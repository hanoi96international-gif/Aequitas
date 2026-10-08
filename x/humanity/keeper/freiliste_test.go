package keeper

import (
	"fmt"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// rpcRateLimitFreiErgaenzen stellt in Tests weitere Adressen zusaetzlich frei
// (im Betrieb legt rpcRateLimitFreiValidatoren den Teil der Validatoren bei
// jedem Lauf neu fest). Nur IP-Literale; Namen werden nie aufgeloest.
func rpcRateLimitFreiErgaenzen(ips []string) {
	rpcRateLimitFreiMu.Lock()
	defer rpcRateLimitFreiMu.Unlock()
	alt := rpcRateLimitFreiListe.Load()
	neu := map[string]bool{}
	if alt != nil {
		for k, v := range *alt {
			neu[k] = v
		}
	}
	for _, s := range ips {
		if ip := net.ParseIP(strings.TrimSpace(s)); ip != nil {
			neu[ip.String()] = true
		}
	}
	rpcRateLimitFreiListe.Store(&neu)
}

func freiFuer(ip string) bool {
	r := httptest.NewRequest("POST", "/rpc", nil)
	r.RemoteAddr = net.JoinHostPort(ip, "4711")
	return rpcRateLimitFrei(r)
}

// Missbrauch (Pruefung von #319, MEDIUM-2): von der Ratenbegrenzung
// freigestellt werden nur die Adressen der Mitglieder des eigenen Satzes --
// nicht die, die irgendein zugelassener Validator ankuendigt, nie Loopback,
// und wer seine Adresse wechselt oder den Satz verlaesst, faellt heraus. Die
// Adressen aus AEQUITAS_RPC_RATE_LIMIT_FREI bleiben.
func TestFreiliste_NurSatzKeinLoopbackNeuAufgebaut(t *testing.T) {
	altListe, altUmgebung := rpcRateLimitFreiListe.Load(), rpcRateLimitFreiUmgebung
	t.Cleanup(func() { rpcRateLimitFreiListe.Store(altListe); rpcRateLimitFreiUmgebung = altUmgebung })
	rpcRateLimitFreiUmgebung = map[string]bool{"192.0.2.200": true} // Lastgenerator aus der Umgebung
	rpcRateLimitFreiValidatoren(nil)

	ich := "0x0000000000000000000000000000000000000003"
	mitglied := "0x0000000000000000000000000000000000000001"
	lokal := "0x0000000000000000000000000000000000000002"
	fremd := "0x0000000000000000000000000000000000000009" // zugelassen, nicht im Satz
	l := NeueLeitung(ich, "", []string{ich, mitglied, lokal}, mitglied, true, LeitSpeicher{Term: 1, Leiter: mitglied}, testKonfig(), LeitUmgebung{}, time.Now())
	l.SetzeURL(mitglied, "http://203.0.113.5:8080")
	l.SetzeURL(lokal, "http://127.0.0.1:8080")
	// Der Angriff: ein zugelassener Validator ausserhalb des Satzes kuendigt
	// in einer Hallo-Nachricht eine (oeffentliche) Adresse an.
	l.Empfange(LeitNachricht{Art: leitArtHallo, Von: fremd, URL: "http://198.51.100.9:8080"}, time.Now())
	if l.URLs()[fremd] == "" {
		t.Fatal("Vorbedingung: die angekuendigte Adresse ist gespeichert")
	}
	validatorIPsFrei(l)
	for ip, soll := range map[string]bool{
		"203.0.113.5":  true,  // Mitglied
		"127.0.0.1":    false, // Loopback, auch von einem Mitglied
		"198.51.100.9": false, // nicht im Satz
		"192.0.2.200":  true,  // aus der Umgebung
	} {
		if freiFuer(ip) != soll {
			t.Fatalf("%s freigestellt = %v, erwartet %v", ip, !soll, soll)
		}
	}
	// Das Mitglied wechselt die Adresse: die alte faellt heraus.
	l.SetzeURL(mitglied, "http://203.0.113.6:8080")
	validatorIPsFrei(l)
	if freiFuer("203.0.113.5") || !freiFuer("203.0.113.6") || !freiFuer("192.0.2.200") {
		t.Fatal("die Liste wurde nicht neu aufgebaut")
	}
}

// Private und CGNAT-Adressen nur in ausdruecklich genannten Netzen: vor dem
// Knoten steht ein Proxy im Docker-Netz, und dessen private Adresse in der
// Liste hoebe die Grenze fuer jeden auf. Loopback, Sonderadressen & Co. nie,
// auch nicht in einem genannten Netz.
func TestFreistellbar(t *testing.T) {
	alle := mussNetze("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10", "fc00::/7")
	tailscale := mussNetze("100.64.0.0/10")
	for ip, soll := range map[string][3]bool{ // {ohne Netze, alle privaten Netze, nur 100.64/10}
		"203.0.113.5": {true, true, true}, "2001:db8::1": {true, true, true},
		"10.0.0.7": {false, true, false}, "172.18.0.2": {false, true, false}, "192.168.1.5": {false, true, false},
		"100.64.1.2": {false, true, true}, "fd00::5": {false, true, false},
		"::ffff:172.18.0.2": {false, true, false}, "::ffff:100.64.1.2": {false, true, true},
		"127.0.0.1": {false, false, false}, "::1": {false, false, false}, "0.0.0.0": {false, false, false}, "::": {false, false, false},
		"::ffff:127.0.0.1": {false, false, false},
		"169.254.1.1":      {false, false, false}, "fe80::1": {false, false, false}, "224.0.0.1": {false, false, false}, "ff02::1": {false, false, false},
		// Sonderbereiche (Pruefung von #320, INFO-1)
		"0.1.2.3": {false, false, false}, "255.255.255.255": {false, false, false}, "240.0.0.1": {false, false, false},
		"198.18.0.1": {false, false, false}, "192.0.0.1": {false, false, false}, "fec0::1": {false, false, false},
		"::7f00:1": {false, false, false}, "64:ff9b::7f00:1": {false, false, false}, "2002:7f00:1::": {false, false, false},
		"2001::7f00:1": {false, false, false}, "100::1": {false, false, false}, "64:ff9b:1::a00:1": {false, false, false},
		// weitere Sonderbereiche (Pruefung von #320, INFO-8)
		"::ffff:0:a00:1": {false, false, false}, "2001:2::1": {false, false, false}, "2001:10::1": {false, false, false},
		"2001:20::1": {false, false, false}, "5f00::1": {false, false, false}, "3fff::1": {false, false, false},
		"192.88.99.1": {false, false, false},
	} {
		for i, netze := range [][]*net.IPNet{nil, alle, tailscale} {
			if got := freistellbar(net.ParseIP(ip), netze); got != soll[i] {
				t.Fatalf("freistellbar(%s) mit Netzen %d = %v, erwartet %v", ip, i, got, soll[i])
			}
		}
	}
	if freistellbar(nil, alle) {
		t.Fatal("freistellbar(nil)")
	}
	// Ein genanntes Netz oeffnet keinen Sonderbereich.
	if freistellbar(net.ParseIP("127.0.0.1"), mussNetze("127.0.0.0/8")) || freistellbar(net.ParseIP("0.1.2.3"), mussNetze("0.0.0.0/0")) {
		t.Fatal("ein genanntes Netz darf Loopback und Sonderbereiche nicht freistellbar machen")
	}
}

// Die eigenen Schnittstellen im Test: das Docker-Netz des Knotens
// (172.18.0.0/16, Gateway 172.18.0.1), Loopback, Link-Local und eine eigene
// Tailscale-Adresse als Hostroute (/32 -- beruehrt kein genanntes Netz).
var testEigeneNetze = func() ([]*net.IPNet, error) {
	return mussNetze("172.18.0.0/16", "127.0.0.0/8", "fe80::/64", "100.101.0.9/32"), nil
}

func freilisteSichern(t *testing.T) {
	t.Helper()
	altListe, altUmgebung := rpcRateLimitFreiListe.Load(), rpcRateLimitFreiUmgebung
	altEigene, altWarnen, altGewarnt := eigeneNetze, freilisteNetzeWarnen, freilisteNetzeGewarnt.Load()
	eigeneNetze = testEigeneNetze
	freilisteNetzeWarnen = func(string) {}
	freilisteNetzeGewarnt.Store(nil)
	nichtFrei.Lock()
	altGemeldet := nichtFrei.gemeldet
	nichtFrei.gemeldet = nil
	nichtFrei.Unlock()
	t.Cleanup(func() {
		rpcRateLimitFreiListe.Store(altListe)
		rpcRateLimitFreiUmgebung = altUmgebung
		eigeneNetze, freilisteNetzeWarnen = altEigene, altWarnen
		freilisteNetzeGewarnt.Store(altGewarnt)
		nichtFrei.Lock()
		nichtFrei.gemeldet = altGemeldet
		nichtFrei.Unlock()
	})
}

func freilisteLeitung(ich string, satz ...string) *Leitung {
	return NeueLeitung(ich, "", append([]string{ich}, satz...), satz[0], true, LeitSpeicher{Term: 1, Leiter: satz[0]}, testKonfig(), LeitUmgebung{}, time.Now())
}

// Ein Satzmitglied mit privater Adresse kommt nur in die Liste, wenn sie in
// einem Netz aus AEQUITAS_FREILISTE_NETZE liegt. Andere Werte -- auch die des
// frueheren Schalters -- stellen nichts frei (Pruefung von #320, LOW-2 und
// LOW-4).
func TestFreiliste_PrivatNurInGenanntenNetzen(t *testing.T) {
	freilisteSichern(t)
	rpcRateLimitFreiUmgebung = nil
	lan := "0x0000000000000000000000000000000000000001"
	tailscale := "0x0000000000000000000000000000000000000002"
	l := freilisteLeitung("0x0000000000000000000000000000000000000003", lan, tailscale)
	l.SetzeURL(lan, "http://10.20.0.5:8080")
	l.SetzeURL(tailscale, "http://100.101.102.103:8080")
	for _, wert := range []string{"", "1", "ja", "true", "10.20.0.0/33", "10.20.0.5", "nein"} {
		t.Setenv("AEQUITAS_FREILISTE_NETZE", wert)
		validatorIPsFrei(l)
		if freiFuer("10.20.0.5") || freiFuer("100.101.102.103") {
			t.Fatalf("private Adresse mit AEQUITAS_FREILISTE_NETZE=%q freigestellt", wert)
		}
	}
	// Nur das genannte Netz: Tailscale ja, das andere private Netz nicht.
	t.Setenv("AEQUITAS_FREILISTE_NETZE", " 100.64.0.0/10 , kaputt")
	validatorIPsFrei(l)
	if !freiFuer("100.101.102.103") || freiFuer("10.20.0.5") {
		t.Fatal("AEQUITAS_FREILISTE_NETZE=100.64.0.0/10 stellt nicht genau das Tailscale-Netz frei")
	}
	t.Setenv("AEQUITAS_FREILISTE_NETZE", "10.20.0.0/16")
	validatorIPsFrei(l)
	if !freiFuer("10.20.0.5") || freiFuer("100.101.102.103") {
		t.Fatal("AEQUITAS_FREILISTE_NETZE=10.20.0.0/16 stellt nicht genau dieses Netz frei")
	}
}

// Der Angriff aus der Pruefung von #320 (LOW-5): der Betreiber nennt ein zu
// weites Netz, und ein boeswilliges Mitglied kuendigt das Gateway des eigenen
// Docker-Netzes an -- docker-proxy reicht Verbindungen ohne Kopf von dort
// weiter (IPv6, Hairpin). Zu weite Netze und solche, die ein Netz der eigenen
// Schnittstellen beruehren, gelten nicht; sind die Schnittstellen nicht
// lesbar, gilt keines. Eine eigene Hostroute (/32) hindert nicht.
func TestFreiliste_NetzePlausibel(t *testing.T) {
	freilisteSichern(t)
	rpcRateLimitFreiUmgebung = nil
	gateway := "0x0000000000000000000000000000000000000001"
	lan := "0x0000000000000000000000000000000000000002"
	l := freilisteLeitung("0x0000000000000000000000000000000000000003", gateway, lan)
	l.SetzeURL(gateway, "http://172.18.0.1:8080")
	l.SetzeURL(lan, "http://10.20.0.5:8080")
	for _, wert := range []string{"0.0.0.0/0", "::ffff:0:0/96", "0.0.0.0/7", "::ffff:8.0.0.0/101", "::/0", "fc00::/7",
		"172.16.0.0/12", "172.18.0.0/24", "172.18.0.1/32", "8.0.0.0/5", "10.0.0.0/7"} {
		t.Setenv("AEQUITAS_FREILISTE_NETZE", wert)
		validatorIPsFrei(l)
		if freiFuer("172.18.0.1") {
			t.Fatalf("AEQUITAS_FREILISTE_NETZE=%q stellt das Gateway des eigenen Docker-Netzes frei", wert)
		}
		if freiFuer("10.20.0.5") {
			t.Fatalf("AEQUITAS_FREILISTE_NETZE=%q (zu weit) stellt ein privates Netz frei", wert)
		}
	}
	// Eng genug und fremd: gilt -- auch in IPv6-Schreibweise.
	for _, wert := range []string{"10.0.0.0/8", "::ffff:10.20.0.0/112"} {
		t.Setenv("AEQUITAS_FREILISTE_NETZE", wert)
		validatorIPsFrei(l)
		if !freiFuer("10.20.0.5") || freiFuer("172.18.0.1") {
			t.Fatalf("AEQUITAS_FREILISTE_NETZE=%q: das fremde Netz nicht frei oder das eigene frei", wert)
		}
	}
	// Die eigene Tailscale-Adresse ist eine Hostroute: 100.64.0.0/10 gilt.
	l.SetzeURL(lan, "http://100.101.102.103:8080")
	t.Setenv("AEQUITAS_FREILISTE_NETZE", "100.64.0.0/10")
	validatorIPsFrei(l)
	if !freiFuer("100.101.102.103") {
		t.Fatal("eine eigene Hostroute verhindert das Tailscale-Netz")
	}
	// Schnittstellen nicht lesbar: kein Netz gilt.
	eigeneNetze = func() ([]*net.IPNet, error) { return nil, fmt.Errorf("nicht lesbar") }
	validatorIPsFrei(l)
	if freiFuer("100.101.102.103") {
		t.Fatal("ohne lesbare Schnittstellen gilt ein genanntes Netz")
	}
}

// Eine Warnung je Wert, nicht je Lauf (Pruefung von #320, INFO-10).
func TestFreiliste_NetzeWarnungEinmal(t *testing.T) {
	freilisteSichern(t)
	var warnungen []string
	freilisteNetzeWarnen = func(text string) { warnungen = append(warnungen, text) }
	t.Setenv("AEQUITAS_FREILISTE_NETZE", "kaputt, 0.0.0.0/0, 172.16.0.0/12, 100.64.0.0/10")
	for i := 0; i < 100; i++ {
		if n := freilisteNetze(); len(n) != 1 || n[0].String() != "100.64.0.0/10" {
			t.Fatalf("Netze = %v, erwartet nur 100.64.0.0/10", n)
		}
	}
	if len(warnungen) != 1 {
		t.Fatalf("%d Warnungen fuer denselben Wert, erwartet 1", len(warnungen))
	}
	for _, teil := range []string{"kaputt (kein Netz", "0.0.0.0/0 (zu weit)", "172.16.0.0/12 (beruehrt das eigene Netz 172.18.0.0/16)"} {
		if !strings.Contains(warnungen[0], teil) {
			t.Fatalf("Warnung %q nennt %q nicht", warnungen[0], teil)
		}
	}
	t.Setenv("AEQUITAS_FREILISTE_NETZE", "kaputt")
	freilisteNetze()
	freilisteNetze()
	if len(warnungen) != 2 {
		t.Fatalf("%d Warnungen nach neuem Wert, erwartet 2", len(warnungen))
	}
}

// Der Angriff aus der Pruefung von #320 (LOW-2 und INFO-6): die Adresse des
// Proxys steht in der Liste (ein Mitglied hat sie angekuendigt, das Netz ist
// genannt). Eine Anfrage mit X-Forwarded-For, Forwarded oder X-Real-IP --
// auch leer, von jeder Quelle -- bleibt begrenzt; nur eine direkte
// Verbindung ohne diese Koepfe nicht.
func TestFreiliste_UeberProxyNieFrei(t *testing.T) {
	freilisteSichern(t)
	rpcRateLimitFreiUmgebung = nil
	mitglied := "0x0000000000000000000000000000000000000001"
	l := freilisteLeitung("0x0000000000000000000000000000000000000003", mitglied)
	l.SetzeURL(mitglied, "http://10.20.0.2:8080")
	t.Setenv("AEQUITAS_FREILISTE_NETZE", "10.20.0.0/16")
	validatorIPsFrei(l)
	rpcRateLimitFreiErgaenzen([]string{"203.0.113.5", "2001:db8:1::2"})
	for _, quelle := range []string{"10.20.0.2", "203.0.113.5", "2001:db8:1::2"} {
		if !freiFuer(quelle) {
			t.Fatalf("Vorbedingung: %s ist ohne Kopf freigestellt", quelle)
		}
		for _, kopf := range [][2]string{{"X-Forwarded-For", "198.51.100.66"}, {"Forwarded", "for=198.51.100.66"},
			{"X-Real-IP", "198.51.100.66"}, {"X-Forwarded-For", ""}} {
			r := httptest.NewRequest("POST", "/rpc", nil)
			r.RemoteAddr = net.JoinHostPort(quelle, "4711")
			r.Header.Set(kopf[0], kopf[1])
			if rpcRateLimitFrei(r) {
				t.Fatalf("Anfrage von %s mit %s=%q freigestellt", quelle, kopf[0], kopf[1])
			}
		}
	}
}

// Mehrere Schreiber zugleich (Start und Takt, Tests): die Merkliste und der
// Neuaufbau sind gesperrt -- unter -race kein Wettlauf (Pruefung von #320,
// INFO-10).
func TestFreiliste_SchreiberNebenlaeufig(t *testing.T) {
	freilisteSichern(t)
	rpcRateLimitFreiUmgebung = map[string]bool{"192.0.2.200": true}
	mitglied := "0x0000000000000000000000000000000000000001"
	l := freilisteLeitung("0x0000000000000000000000000000000000000003", mitglied)
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				l.SetzeURL(mitglied, fmt.Sprintf("http://10.%d.%d.1:8080", g, i%250))
				validatorIPsFrei(l)
				if !freiFuer("192.0.2.200") {
					t.Error("die Adresse aus der Umgebung fehlte")
					return
				}
			}
		}(g)
	}
	wg.Wait()
	nichtFrei.Lock()
	n := len(nichtFrei.gemeldet)
	nichtFrei.Unlock()
	if n > 1 {
		t.Fatalf("Merkliste hat %d Eintraege bei einem Mitglied", n)
	}
}

// Die eigene URL steht nie in der Liste: Anfragen dieses Knotens an sich
// selbst sind keine Weiterleitungen eines anderen Validators.
func TestFreiliste_EigeneURLNicht(t *testing.T) {
	freilisteSichern(t)
	rpcRateLimitFreiUmgebung = nil
	ich := "0x0000000000000000000000000000000000000003"
	mitglied := "0x0000000000000000000000000000000000000001"
	l := freilisteLeitung(ich, mitglied)
	l.SetzeURL(ich, "http://203.0.113.3:8080")
	l.SetzeURL(mitglied, "http://203.0.113.5:8080")
	validatorIPsFrei(l)
	if freiFuer("203.0.113.3") || !freiFuer("203.0.113.5") {
		t.Fatal("die eigene Adresse ist freigestellt oder das Mitglied nicht")
	}
}

// LOW-1 und INFO-2 aus der Pruefung von #320: ein Mitglied, das seine URL
// staendig auf eine neue private Adresse oder einen Namen wechselt, fuellt
// keine Merkliste ohne Grenze. Gemeldet wird je Mitglied nur, wenn sich die
// Adresse aendert; wer den Satz verlaesst, wird vergessen.
func TestFreiliste_MeldungenBegrenzt(t *testing.T) {
	freilisteSichern(t)
	rpcRateLimitFreiUmgebung = nil
	t.Setenv("AEQUITAS_FREILISTE_NETZE", "")
	var meldungen []string
	altLog := nichtFreiLog
	nichtFreiLog = func(v, host string) { meldungen = append(meldungen, v+"|"+host) }
	t.Cleanup(func() { nichtFreiLog = altLog })

	ich := "0x0000000000000000000000000000000000000003"
	mitglied := "0x0000000000000000000000000000000000000001"
	ehrlich := "0x0000000000000000000000000000000000000002"
	l := freilisteLeitung(ich, mitglied, ehrlich)
	l.SetzeURL(ehrlich, "http://203.0.113.5:8080")
	for i := 0; i < 2000; i++ {
		l.SetzeURL(mitglied, fmt.Sprintf("http://10.%d.%d.1:8080", i/250, i%250))
		validatorIPsFrei(l)
		validatorIPsFrei(l) // dieselbe Adresse: keine zweite Meldung
	}
	l.SetzeURL(mitglied, "https://knoten.example.org")
	validatorIPsFrei(l)
	validatorIPsFrei(l)
	if len(meldungen) != 2001 {
		t.Fatalf("%d Meldungen, erwartet 2001 (eine je neue Adresse)", len(meldungen))
	}
	if meldungen[2000] != mitglied+"|knoten.example.org" {
		t.Fatalf("ein Name wird nicht gemeldet: %q", meldungen[2000])
	}
	nichtFrei.Lock()
	n := len(nichtFrei.gemeldet)
	nichtFrei.Unlock()
	if n != 1 {
		t.Fatalf("Merkliste hat %d Eintraege, erwartet 1 (nur das Mitglied ohne Freistellung)", n)
	}
	if !freiFuer("203.0.113.5") {
		t.Fatal("das ehrliche Mitglied ist nicht freigestellt")
	}
	// Das Mitglied verlaesst den Satz: vergessen.
	l2 := freilisteLeitung(ich, ehrlich)
	l2.SetzeURL(ehrlich, "http://203.0.113.5:8080")
	validatorIPsFrei(l2)
	nichtFrei.Lock()
	n = len(nichtFrei.gemeldet)
	nichtFrei.Unlock()
	if n != 0 {
		t.Fatalf("Merkliste hat nach dem Satzwechsel %d Eintraege, erwartet 0", n)
	}
}

// Neuaufbau und Abfrage laufen nebenlaeufig (Takt der Leitung gegen jede
// Anfrage). Leser sehen nie eine halbe Liste: die Adresse aus der Umgebung
// ist in jedem Augenblick frei, und unter -race gibt es keinen Wettlauf
// (Pruefung von #320, LOW-4).
func TestFreiliste_NeuaufbauNebenlaeufig(t *testing.T) {
	freilisteSichern(t)
	rpcRateLimitFreiUmgebung = map[string]bool{"192.0.2.200": true}
	rpcRateLimitFreiValidatoren(nil)
	mitglied := "0x0000000000000000000000000000000000000001"
	l := freilisteLeitung("0x0000000000000000000000000000000000000003", mitglied)

	var fehler atomic.Value
	var stopp atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stopp.Load() {
				if !freiFuer("192.0.2.200") {
					fehler.Store("die Adresse aus der Umgebung fehlte waehrend eines Neuaufbaus")
					return
				}
				freiFuer("203.0.113.7")
			}
		}()
	}
	for i := 0; i < 2000; i++ {
		l.SetzeURL(mitglied, fmt.Sprintf("http://203.0.113.%d:8080", i%200+1))
		validatorIPsFrei(l)
	}
	stopp.Store(true)
	wg.Wait()
	if f := fehler.Load(); f != nil {
		t.Fatal(f)
	}
	m := rpcRateLimitFreiListe.Load()
	if len(*m) != 2 {
		t.Fatalf("Liste hat %d Eintraege, erwartet 2 (Umgebung und das Mitglied): %v", len(*m), strings.Join(keys(*m), ", "))
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Ein Name von einem Peer landet gekuerzt und mit maskierten Steuer- und
// Trennzeichen im Log (Pruefung von #320, INFO-9).
func TestFreiliste_LogzeileBegrenzt(t *testing.T) {
	zeile := nichtFreiText("0x0000000000000000000000000000000000000001", strings.Repeat("a", 60000)+"\u2028\u0085\n")
	if len(zeile) > 400 {
		t.Fatalf("Logzeile hat %d Byte", len(zeile))
	}
	zeile = nichtFreiText("0x0000000000000000000000000000000000000001", "x\u2028y\u0085z\nw")
	if strings.ContainsAny(zeile, "\u2028\u0085\n") {
		t.Fatalf("Steuer- oder Trennzeichen unmaskiert: %q", zeile)
	}
}
