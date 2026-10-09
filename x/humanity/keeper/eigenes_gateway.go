package keeper

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// DEM EIGENEN GATEWAY KEIN X-FORWARDED-FOR GLAUBEN (Pruefung von #324,
// MEDIUM-5).
//
// clientIP glaubt X-Forwarded-For, wenn die TCP-Verbindung von einer privaten
// Adresse kommt -- dem eigenen Proxy (Caddy, ein anderer Container im
// Docker-Netz). Von einer privaten Adresse kommt aber auch docker-proxy: Er
// nimmt an veroeffentlichten Ports an, was iptables nicht umleitet (IPv6 an
// [::]:8080, Hairpin vom eigenen Rechner), und reicht es OHNE eigenen Kopf von
// der Gateway-Adresse des Docker-Netzes weiter (172.x.0.1). Dort setzt der
// Client X-Forwarded-For selbst: jede Anfrage mit neuer Adresse ein neuer
// Zaehler, bis die Karten der Grenzen voll sind und jeder neue Absender
// gesperrt ist (begrenzte_karte.go). Der eigene Proxy ist nie das Gateway;
// also gilt der Kopf von dort nie.
//
// Die Gateways stehen in /proc/net/route und /proc/net/ipv6_route (gelesen
// hoechstens einmal je Minute). Sind sie nicht lesbar, gilt der Kopf von
// keiner Quelle (fail-closed: jeder Proxy zaehlt dann unter seiner eigenen
// Adresse), mit einer Meldung.

// eigeneGatewaysLesen: die Gateways der eigenen Routen, in der Schreibweise
// von net.IP.String(). Variable fuer Tests.
var eigeneGatewaysLesen = func() (map[string]bool, error) {
	return gatewaysAusDateien("/proc/net/route", "/proc/net/ipv6_route")
}

// gatewaysAusDateien: die Gateways aus den Routentabellen von Linux. Die
// IPv4-Tabelle muss lesbar sein; die IPv6-Tabelle ist optional (ein Kern ohne
// IPv6 hat sie nicht).
func gatewaysAusDateien(v4, v6 string) (map[string]bool, error) {
	out := map[string]bool{}
	if err := routenLesen(v4, 2, 8, func(feld string) net.IP {
		b, err := hex.DecodeString(feld)
		if err != nil || len(b) != 4 {
			return nil
		}
		return net.IPv4(b[3], b[2], b[1], b[0]) // Linux schreibt IPv4 hier verkehrt herum
	}, out); err != nil {
		return nil, err
	}
	_ = routenLesen(v6, 4, 32, func(feld string) net.IP {
		b, err := hex.DecodeString(feld)
		if err != nil || len(b) != 16 {
			return nil
		}
		return net.IP(b)
	}, out)
	return out, nil
}

// routenLesen: aus jeder Zeile der Routentabelle pfad das Feld spalte (laenge
// Hex-Zeichen) als Gateway, wenn es nicht leer (nur Nullen) ist.
func routenLesen(pfad string, spalte, laenge int, lesen func(string) net.IP, out map[string]bool) error {
	f, err := os.Open(pfad)
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		felder := strings.Fields(s.Text())
		if len(felder) <= spalte || len(felder[spalte]) != laenge || strings.Trim(felder[spalte], "0") == "" {
			continue
		}
		if ip := lesen(felder[spalte]); ip != nil && !ip.IsUnspecified() {
			out[ip.String()] = true
		}
	}
	return s.Err()
}

type gatewayStand struct {
	gelesen  time.Time
	gateways map[string]bool
	fehler   error
}

var (
	gatewayCache    atomic.Pointer[gatewayStand]
	gatewayGemeldet atomic.Bool
)

// eigeneGateways: der Stand von hoechstens vor einer Minute.
func eigeneGateways() *gatewayStand {
	if st := gatewayCache.Load(); st != nil && time.Since(st.gelesen) < time.Minute {
		return st
	}
	g, err := eigeneGatewaysLesen()
	st := &gatewayStand{gelesen: time.Now(), gateways: g, fehler: err}
	gatewayCache.Store(st)
	if err != nil && gatewayGemeldet.CompareAndSwap(false, true) {
		fmt.Printf("[GRENZE] ⚠ eigene Routen nicht lesbar (%v) -- X-Forwarded-For gilt von keiner Quelle\n", err)
	}
	return st
}

// kopfQuelleVertrauenswuerdig: darf clientIP dem X-Forwarded-For einer
// Verbindung von host glauben? Nur einer privaten oder Loopback-Adresse, die
// nicht das Gateway der eigenen Routen ist.
func kopfQuelleVertrauenswuerdig(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil || !isPrivateOrLoopback(host) {
		return false
	}
	if ip.IsLoopback() {
		return true // dieser Rechner selbst, nie ein Gateway
	}
	st := eigeneGateways()
	if st.fehler != nil {
		return false
	}
	return !st.gateways[ip.String()]
}
