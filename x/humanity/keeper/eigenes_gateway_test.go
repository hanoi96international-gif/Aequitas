package keeper

import (
	"errors"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Im Test ein festes Gateway: das des Docker-Netzes aus den uebrigen Tests
// (172.18.0.0/16) und ein IPv6-Gateway -- die echten Routen des Rechners, auf
// dem die Tests laufen, sollen keinen Test kippen.
func testGateways() (map[string]bool, error) {
	return map[string]bool{"172.18.0.1": true, "fd00::1": true}, nil
}

func init() {
	eigeneGatewaysLesen = testGateways
}

func gatewaysSetzen(t *testing.T, f func() (map[string]bool, error)) {
	t.Helper()
	alt := eigeneGatewaysLesen
	eigeneGatewaysLesen = f
	gatewayCache.Store(nil)
	t.Cleanup(func() { eigeneGatewaysLesen = alt; gatewayCache.Store(nil) })
}

// Missbrauch (Pruefung von #324, MEDIUM-5): docker-proxy reicht IPv6- und
// Hairpin-Verbindungen ohne Kopf von der Gateway-Adresse weiter. clientIP
// glaubte dann dem X-Forwarded-For des Clients -- eine Verbindung mit neuer
// Adresse je Anfrage fuellte alle Karten. Vom Gateway gilt der Kopf nie.
func TestClientIP_GatewayGlaubtKeinemKopf(t *testing.T) {
	gatewaysSetzen(t, testGateways)
	anfrage := func(remote, xff string) string {
		r := httptest.NewRequest("POST", "/rpc", nil)
		r.RemoteAddr = remote
		r.Header.Set("X-Forwarded-For", xff)
		return clientIP(r)
	}
	if got := anfrage("172.18.0.1:4000", "10.1.2.3"); got != "172.18.0.1" {
		t.Fatalf("Gateway: clientIP = %q, erwartet die TCP-Adresse", got)
	}
	if got := anfrage("[fd00::1]:4000", "10.1.2.3"); got != "fd00::" {
		t.Fatalf("IPv6-Gateway: clientIP = %q, erwartet das /64 der TCP-Adresse", got)
	}
	if got := anfrage("172.18.0.5:4000", "203.0.113.7"); got != "203.0.113.7" {
		t.Fatalf("eigener Proxy (kein Gateway): clientIP = %q, erwartet den Kopf", got)
	}
	if got := anfrage("127.0.0.1:4000", "203.0.113.8"); got != "203.0.113.8" {
		t.Fatalf("Loopback: clientIP = %q, erwartet den Kopf", got)
	}

	// Der Angriff: 1.000 Anfragen ueber das Gateway mit je neuer Adresse
	// legen in rpcRateLimit einen einzigen Schluessel an.
	rpcRateLimit.Delete("172.18.0.1")
	t.Cleanup(func() { rpcRateLimit.Delete("172.18.0.1") })
	vorher := rpcRateLimit.anzahl.Load()
	for i := 0; i < 1000; i++ {
		rpcRateLimited(anfrage("172.18.0.1:4000", fmt.Sprintf("10.%d.%d.1", i/250, i%250)))
	}
	if n := rpcRateLimit.anzahl.Load() - vorher; n != 1 {
		t.Fatalf("1.000 Anfragen ueber das Gateway legten %d Schluessel an, erwartet 1", n)
	}
	if !rpcRateLimited("172.18.0.1") {
		t.Fatal("die Anfragen ueber das Gateway zaehlen nicht unter einem Zaehler")
	}
}

// Routen nicht lesbar: der Kopf gilt von keiner Quelle ausser Loopback
// (fail-closed).
func TestClientIP_GatewayNichtLesbar(t *testing.T) {
	gatewaysSetzen(t, func() (map[string]bool, error) { return nil, errors.New("kein /proc") })
	r := httptest.NewRequest("POST", "/rpc", nil)
	r.RemoteAddr = "172.18.0.5:4000"
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	if got := clientIP(r); got != "172.18.0.5" {
		t.Fatalf("Routen nicht lesbar: clientIP = %q, erwartet die TCP-Adresse", got)
	}
	r.RemoteAddr = "127.0.0.1:4000"
	if got := clientIP(r); got != "203.0.113.7" {
		t.Fatalf("Loopback: clientIP = %q, erwartet den Kopf", got)
	}
}

// Der Stand wird hoechstens einmal je Minute gelesen, danach neu.
func TestEigeneGateways_HoechstensJeMinute(t *testing.T) {
	gelesen := 0
	gatewaysSetzen(t, func() (map[string]bool, error) { gelesen++; return testGateways() })
	for i := 0; i < 100; i++ {
		kopfQuelleVertrauenswuerdig("172.18.0.5")
	}
	if gelesen != 1 {
		t.Fatalf("%d Mal gelesen, erwartet 1", gelesen)
	}
	st := gatewayCache.Load()
	alt := *st
	alt.gelesen = time.Now().Add(-61 * time.Second)
	gatewayCache.Store(&alt)
	kopfQuelleVertrauenswuerdig("172.18.0.5")
	if gelesen != 2 {
		t.Fatalf("nach einer Minute nicht neu gelesen (%d)", gelesen)
	}
}

// Die Routentabellen von Linux: IPv4 verkehrt herum in Hex, IPv6 der Reihe
// nach; Routen ohne Gateway zaehlen nicht, kaputte Zeilen auch nicht.
func TestGatewaysAusDateien(t *testing.T) {
	dir := t.TempDir()
	v4 := filepath.Join(dir, "route")
	os.WriteFile(v4, []byte("Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n"+
		"eth0\t00000000\t010012AC\t0003\t0\t0\t0\t00000000\t0\t0\t0\n"+
		"eth0\t000012AC\t00000000\t0001\t0\t0\t0\t0000FFFF\t0\t0\t0\n"+
		"kaputt\n"+"eth1\t00000000\tZZZZZZZZ\t0003\n"), 0o600)
	v6 := filepath.Join(dir, "ipv6_route")
	os.WriteFile(v6, []byte(
		"00000000000000000000000000000000 00 00000000000000000000000000000000 00 fd000000000000000000000000000001 00000400 00000001 00000000 00000003 eth0\n"+
			"fd000000000000000000000000000000 40 00000000000000000000000000000000 00 00000000000000000000000000000000 00000100 00000001 00000000 00000001 eth0\n"), 0o600)
	g, err := gatewaysAusDateien(v4, v6)
	if err != nil {
		t.Fatal(err)
	}
	if len(g) != 2 || !g["172.18.0.1"] || !g["fd00::1"] {
		t.Fatalf("Gateways %v, erwartet 172.18.0.1 und fd00::1", g)
	}
	if g, err := gatewaysAusDateien(v4, filepath.Join(dir, "fehlt")); err != nil || len(g) != 1 {
		t.Fatalf("ohne IPv6-Tabelle: %v, %v", g, err)
	}
	if _, err := gatewaysAusDateien(filepath.Join(dir, "fehlt"), v6); err == nil {
		t.Fatal("ohne IPv4-Tabelle kein Fehler")
	}
	if runtime.GOOS != "linux" {
		return
	}
	echt, err := gatewaysAusDateien("/proc/net/route", "/proc/net/ipv6_route")
	if err != nil {
		t.Fatalf("echte Routen nicht lesbar: %v", err)
	}
	for k := range echt {
		if net.ParseIP(k) == nil {
			t.Fatalf("kein IP: %q", k)
		}
	}
}
