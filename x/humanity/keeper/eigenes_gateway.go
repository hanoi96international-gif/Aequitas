package keeper

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
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
// gesperrt ist (begrenzte_karte.go). Also gilt der Kopf vom Gateway nie.
//
// Der eigene Proxy muss darum ein Container im selben Docker-Netz sein, der
// den Knoten per Containername anspricht (so alle Caddyfiles in deploy/). Ein
// Proxy auf dem Host selbst -- als Dienst, mit network_mode host, ueber
// 127.0.0.1:8080 oder die Container-IP -- kommt ebenfalls vom Gateway: dann
// zaehlen alle Nutzer unter ihm (fail-closed, Pruefung von #325, LOW-2). Jede
// verworfene Kopfzeile vom Gateway wird gezaehlt (grenzen_je_absender) und
// hoechstens einmal je Minute gemeldet. Ausloesen kann das auch ein Client,
// der ueber docker-proxy (IPv6, Hairpin) kommt und den Kopf selbst setzt
// (Pruefung von #325, INFO-2).
//
// Die Gateways stehen in /proc/net/route und /proc/net/ipv6_route. Der
// gelesene Stand gilt eine Minute -- ein Wechsel des Docker-Netzes zur
// Laufzeit gilt also bis zu 60 s spaeter; laeuft er ab, lesen gleichzeitige
// Anfragen die Tabellen je selbst (billig, durch die Verbindungen begrenzt).
// Ist die IPv4-Tabelle nicht lesbar (oder die IPv6-Tabelle da, aber nicht
// lesbar), gilt der Kopf nur noch von Loopback (fail-closed: jeder andere
// Proxy zaehlt unter seiner eigenen Adresse); das steht im Stand (erst nach
// dem ersten Lesen) und wird bei jedem Wechsel gemeldet.

// eigeneGatewaysLesen: die Gateways der eigenen Routen, in der Schreibweise
// von net.IP.String(). Variable fuer Tests.
var eigeneGatewaysLesen = func() (map[string]bool, error) {
	return gatewaysAusDateien("/proc/net/route", "/proc/net/ipv6_route")
}

// gatewaysAusDateien: die Gateways aus den Routentabellen von Linux. Die
// IPv4-Tabelle muss lesbar sein; die IPv6-Tabelle darf fehlen (ein Kern ohne
// IPv6 hat sie nicht), aber nicht kaputt sein.
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
	if err := routenLesen(v6, 4, 32, func(feld string) net.IP {
		b, err := hex.DecodeString(feld)
		if err != nil || len(b) != 16 {
			return nil
		}
		return net.IP(b)
	}, out); err != nil && !errors.Is(err, fs.ErrNotExist) {
		// Fehlt die Datei, hat der Kern kein IPv6. Jeder andere Fehler liesse
		// ein IPv6-Gateway still weg (Pruefung von #325, INFO-3).
		return nil, err
	}
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
	// xffVomGatewayVerworfen: Aufrufe von clientIP fuer Verbindungen vom
	// Gateway mit X-Forwarded-For, deren Kopf nicht galt (eine Anfrage kann
	// mehrere sein); xffVomGatewayGemeldet: Unix-Sekunde der letzten Meldung.
	xffVomGatewayVerworfen atomic.Int64
	xffVomGatewayGemeldet  atomic.Int64
	// Zahl der Meldungen im Log -- fuer die Tests der Drossel.
	xffVomGatewayMeldungen  atomic.Int64
	gatewayWechselMeldungen atomic.Int64
)

// eigeneGateways: der Stand von hoechstens vor einer Minute.
func eigeneGateways() *gatewayStand {
	if st := gatewayCache.Load(); st != nil && time.Since(st.gelesen) < time.Minute {
		return st
	}
	g, err := eigeneGatewaysLesen()
	st := &gatewayStand{gelesen: time.Now(), gateways: g, fehler: err}
	gatewayCache.Store(st)
	// Gemeldet wird jeder Wechsel: nicht lesbar -> lesbar -> nicht lesbar.
	if err != nil && gatewayGemeldet.CompareAndSwap(false, true) {
		gatewayWechselMeldungen.Add(1)
		fmt.Printf("[GRENZE] ⚠ eigene Routen nicht lesbar (%v) -- X-Forwarded-For gilt nur noch von Loopback\n", err)
	} else if err == nil && gatewayGemeldet.CompareAndSwap(true, false) {
		gatewayWechselMeldungen.Add(1)
		fmt.Println("[GRENZE] ✓ eigene Routen wieder lesbar -- X-Forwarded-For gilt wieder vom eigenen Proxy")
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
	if st.gateways[ip.String()] {
		xffVomGatewayVerworfenMelden(ip.String())
		return false
	}
	return true
}

// xffVomGatewayVerworfenMelden: zaehlen und hoechstens einmal je Minute
// melden -- ein Proxy auf dem Host faellt sonst nur als 429 auf (Pruefung
// von #325, LOW-2). Die Drossel ist eine Grenze: der Kopf kommt von aussen.
func xffVomGatewayVerworfenMelden(gateway string) {
	xffVomGatewayVerworfen.Add(1)
	jetzt := time.Now().Unix()
	if alt := xffVomGatewayGemeldet.Load(); jetzt-alt >= 60 && xffVomGatewayGemeldet.CompareAndSwap(alt, jetzt) {
		xffVomGatewayMeldungen.Add(1)
		fmt.Printf("[GRENZE] ⚠ X-Forwarded-For vom eigenen Gateway %s verworfen -- entweder ein Proxy auf dem Host (zaehlt alle Nutzer unter einer Adresse; ins Docker-Netz legen) oder ein Client ueber docker-proxy (IPv6, Hairpin; docker-proxy pruefen), siehe docs/OFFEN.md\n", gateway)
	}
}

// gatewayStandFuerGrenzen: fuer GrenzenJeAbsenderStand.
func gatewayStandFuerGrenzen() map[string]interface{} {
	st := gatewayCache.Load()
	out := map[string]interface{}{"xff_vom_gateway_verworfen": xffVomGatewayVerworfen.Load()}
	if st != nil {
		out["routen_lesbar"] = st.fehler == nil
		out["gateways"] = len(st.gateways)
	}
	return out
}
