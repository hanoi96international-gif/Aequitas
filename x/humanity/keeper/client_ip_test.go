package keeper

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Wem clientIP X-Forwarded-For glaubt (Pruefung von #320, INFO-12): nur einer
// TCP-Quelle aus privatOderLoopbackNetze -- dem vorgeschalteten Proxy. Die
// Tabelle ist seit #320 eine Paketvariable; ein fehlender Eintrag liesse
// einen Proxy auf Loopback jeden Nutzer unter seiner eigenen Adresse zaehlen,
// ein zusaetzlicher liesse Fremde ihre Adresse waehlen.
func TestClientIP_WemDerKopfGeglaubtWird(t *testing.T) {
	for _, f := range []struct {
		quelle string
		privat bool
	}{
		{"127.0.0.1", true}, {"127.255.255.254", true}, {"::1", true},
		{"10.0.0.1", true}, {"10.255.255.255", true},
		{"172.16.0.1", true}, {"172.31.255.255", true}, {"172.32.0.1", false}, {"172.15.255.255", false},
		{"192.168.1.1", true}, {"192.169.0.1", false},
		{"100.64.0.1", true}, {"100.127.255.255", true}, {"100.128.0.1", false}, {"100.63.255.255", false},
		{"fc00::1", true}, {"fdff::1", true}, {"fe00::1", false},
		{"::ffff:10.0.0.1", true}, {"::ffff:198.51.100.9", false},
		{"198.51.100.9", false}, {"2001:db8::1", false}, {"fe80::1", false}, {"::2", false},
		{"kein-ip", false}, {"", false},
	} {
		if got := isPrivateOrLoopback(f.quelle); got != f.privat {
			t.Errorf("isPrivateOrLoopback(%q) = %v, erwartet %v", f.quelle, got, f.privat)
		}
		if f.quelle == "" || f.quelle == "kein-ip" {
			continue
		}
		r := httptest.NewRequest("POST", "/rpc", nil)
		r.RemoteAddr = f.quelle + ":4711"
		if strings.Contains(f.quelle, ":") {
			r.RemoteAddr = "[" + f.quelle + "]:4711"
		}
		r.Header.Set("X-Forwarded-For", "203.0.113.77, 10.9.9.9")
		want := f.quelle
		if f.privat {
			want = "203.0.113.77"
		}
		if got := clientIP(r); got != want {
			t.Errorf("clientIP von %s mit X-Forwarded-For = %q, erwartet %q", r.RemoteAddr, got, want)
		}
	}
}
