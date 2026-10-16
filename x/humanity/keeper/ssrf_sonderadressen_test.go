package keeper

import (
	"context"
	"net"
	"strings"
	"testing"
)

// MISSBRAUCH: Adressen, die ein Peer als URL ankuendigen oder ein Name
// aufloesen kann und die NICHT ins oeffentliche Netz fuehren. Bis 16.10.2026
// liessen pinningDialer und isAllowedPeerURL CGNAT, NAT64 und die
// Sonderbereiche durch.
var ssrfNichtOeffentlich = []string{
	// schon vorher abgewiesen -- Gegenprobe, dass nichts aufgeweicht ist
	"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254",
	"0.0.0.0", "::1", "fe80::1", "fc00::1", "224.0.0.1", "::ffff:127.0.0.1", "::ffff:10.0.0.1",
	// neu
	"100.64.0.1",      // CGNAT
	"100.100.100.200", // CGNAT, Alibaba-Metadaten
	"100.127.255.254", // CGNAT, oberes Ende
	"64:ff9b::7f00:1", // NAT64 -> 127.0.0.1
	"64:ff9b::a00:1",  // NAT64 -> 10.0.0.1
	"64:ff9b:1::1",    // NAT64 lokal
	"0.1.2.3",         // "dieses Netz"
	"198.18.0.1",      // Benchmark
	"240.0.0.1",       // reserviert
	"255.255.255.255", // Broadcast
	"2002:7f00:1::1",  // 6to4 um 127.0.0.1
	"2001::1",         // Teredo
	"192.0.0.8",       // IETF-Protokoll
}

var ssrfOeffentlich = []string{"8.8.8.8", "188.172.229.121", "1.1.1.1", "2606:4700::1111", "100.63.255.255", "100.128.0.1"}

func TestSSRF_NichtOeffentlich(t *testing.T) {
	freilisteSichern(t)
	t.Setenv("AEQUITAS_FREILISTE_NETZE", "")
	for _, a := range ssrfNichtOeffentlich {
		ip := net.ParseIP(a)
		if ip == nil {
			t.Fatalf("Testadresse %q nicht lesbar", a)
		}
		if !nichtOeffentlich(ip) {
			t.Errorf("%s gilt als oeffentlich", a)
		}
		host := a
		if strings.Contains(a, ":") {
			host = "[" + a + "]"
		}
		for _, schema := range []string{"http", "https"} {
			if isAllowedPeerURL(schema + "://" + host + ":8080") {
				t.Errorf("isAllowedPeerURL laesst %s://%s durch", schema, host)
			}
		}
		// Literal: pinningDialer lehnt ab, bevor er waehlt.
		if c, err := pinningDialer(context.Background(), "tcp", net.JoinHostPort(a, "80")); err == nil {
			c.Close()
			t.Errorf("pinningDialer waehlt %s", a)
		} else if !strings.Contains(err.Error(), "rejected") {
			t.Errorf("pinningDialer %s: falscher Grund %v", a, err)
		}
	}
	for _, a := range ssrfOeffentlich {
		if nichtOeffentlich(net.ParseIP(a)) {
			t.Errorf("oeffentliche Adresse %s abgewiesen", a)
		}
	}
	// IPv6 mit Zone: kein IP-Literal fuer net.ParseIP, darf trotzdem nie
	// als Name durchgehen (Pruefung von #376).
	for _, u := range []string{"https://[fe80::1%25eth0]:8080", "http://[fe80::1%25eth0]:8080", "https://[fe80::1%25eth0]"} {
		if isAllowedPeerURL(u) {
			t.Errorf("isAllowedPeerURL laesst %s durch", u)
		}
	}
	if !nichtOeffentlich(nil) {
		t.Error("nil gilt als oeffentlich")
	}
}

// CGNAT nur, wenn der Betreiber das Netz nennt (Tailscale); ein dort
// genanntes RFC-1918-Netz oeffnet den Dialer nicht.
func TestSSRF_CGNATNurGenannt(t *testing.T) {
	freilisteSichern(t)
	tailscale := net.ParseIP("100.101.102.103")
	t.Setenv("AEQUITAS_FREILISTE_NETZE", "100.64.0.0/10, 10.20.0.0/16")
	if nichtOeffentlich(tailscale) {
		t.Fatal("genanntes CGNAT-Netz bleibt gesperrt")
	}
	if !isAllowedPeerURL("http://100.101.102.103:8080") {
		t.Fatal("isAllowedPeerURL sperrt das genannte CGNAT-Netz")
	}
	if !nichtOeffentlich(net.ParseIP("10.20.0.5")) {
		t.Fatal("MISSBRAUCH: ein in der Freiliste genanntes privates Netz oeffnet den Dialer")
	}
	// Ein enger genanntes Netz oeffnet nur sich selbst.
	t.Setenv("AEQUITAS_FREILISTE_NETZE", "100.101.0.0/16")
	if nichtOeffentlich(tailscale) || !nichtOeffentlich(net.ParseIP("100.100.100.200")) {
		t.Fatal("100.101.0.0/16 oeffnet nicht genau sich selbst")
	}
	// Eigene Schnittstellen nicht lesbar: freilisteNetze verwirft alles.
	eigeneNetze = func() ([]*net.IPNet, error) { return nil, net.UnknownNetworkError("nicht lesbar") }
	if !nichtOeffentlich(tailscale) {
		t.Fatal("ohne lesbare Schnittstellen gilt ein genanntes Netz")
	}
}
