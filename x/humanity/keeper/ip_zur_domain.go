package keeper

import (
	"net"
	"net/http"
	"os"
	"strings"
)

// Browser, die eine SEITE ueber die blanke IP aufrufen
// (http://194.163.188.71:8080/explorer), landen auf der Domain mit HTTPS.
//
// WARUM
//
// Eine IP-Adresse bekommt kein gewoehnliches Zertifikat; der Browser zeigt
// "Nicht sicher" -- zu Recht: auf diesem Weg ist nichts verschluesselt, und
// ein Explorer, der so aussieht, wirkt wie eine Falle (27.09.2026 gemeldet).
//
// WAS NICHT
//
// Nur Seiten fuer Menschen: GET/HEAD, der Browser verlangt HTML, und der
// Pfad ist keine Schnittstelle. /api, /rpc, /download, /debug und alles,
// was kein HTML verlangt, bleiben unveraendert -- ueber genau diese Adresse
// synchronisieren die Validatoren, und Wallets, Lasttests und Workflows
// sprechen sie direkt an. Loopback und private Netze ebenfalls unveraendert.
//
//	AEQUITAS_OEFFENTLICHE_URL   Ziel (Vorgabe https://aequitas.digital, "aus" = abgeschaltet)

var ipSchnittstellenPraefixe = []string{"/api", "/rpc", "/download", "/debug", "/metrics", "/ws", "/evm", "/.well-known"}

func oeffentlicheURL() string {
	v := strings.TrimSpace(os.Getenv("AEQUITAS_OEFFENTLICHE_URL"))
	if v == "" {
		return "https://aequitas.digital"
	}
	if strings.EqualFold(v, "aus") || strings.EqualFold(v, "off") {
		return ""
	}
	if !strings.HasPrefix(v, "https://") {
		return "https://aequitas.digital"
	}
	return strings.TrimRight(v, "/")
}

// ipSeitenZiel: wohin umleiten, oder "" fuer "normal beantworten".
func ipSeitenZiel(r *http.Request) string {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return ""
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() {
		return ""
	}
	if !strings.Contains(r.Header.Get("Accept"), "text/html") {
		return ""
	}
	for _, p := range ipSchnittstellenPraefixe {
		if r.URL.Path == p || strings.HasPrefix(r.URL.Path, p+"/") || strings.HasPrefix(r.URL.Path, p+"?") {
			return ""
		}
	}
	ziel := oeffentlicheURL()
	if ziel == "" {
		return ""
	}
	return ziel + r.URL.RequestURI()
}

func ipZurDomainMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ziel := ipSeitenZiel(r); ziel != "" {
			http.Redirect(w, r, ziel, http.StatusMovedPermanently)
			return
		}
		next.ServeHTTP(w, r)
	})
}
