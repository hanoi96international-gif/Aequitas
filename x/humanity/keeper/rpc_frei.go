package keeper

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

// Freistellung einzelner Absender von der Ratenbegrenzung auf /rpc.
//
// WARUM. Seit dem 22.09.2026 nimmt nur noch EIN Knoten Ueberweisungen an
// (annahme_tor.go). Der Lastgenerator lief bisher auf Contabo2 gegen dessen
// eigenes localhost -- dort wird heute jede Ueberweisung abgelehnt. Er muss
// also Contabo1 ueber das Netz beschicken, und dort begrenzt rpcRateLimited
// jede IP auf AEQUITAS_RPC_RATE_LIMIT_MAX Anfragen je 10 s (Vorgabe 200). Fuer
// 10.000 Ueberweisungen/s braucht ein Generator >= 100.000.
//
// Die Begrenzung fuer ALLE aufzudrehen hiesse, einen oeffentlichen Endpunkt fuer
// jeden zu schwaechen, um einer einzigen bekannten Maschine Platz zu machen.
// Diese Liste nimmt nur die genannten Adressen heraus.
//
// NUR DIE TCP-ADRESSE ZAEHLT. clientIP() uebernimmt X-Forwarded-For, wenn die
// Verbindung von einer privaten Adresse kommt (dem eigenen Proxy). Fuer eine
// Freistellung reicht das nicht: sie wird gegen r.RemoteAddr geprueft, also
// gegen die Adresse, von der die Verbindung tatsaechlich aufgebaut wurde. Wer
// aus dem Netz einen Kopf "X-Forwarded-For: <Partner>" mitschickt, bleibt
// begrenzt -- auch wenn ein Proxy dazwischen ihn weiterreicht.
//
// Die Inflight-Grenze (inflight_grenze.go) gilt fuer freigestellte Absender
// unveraendert. Sie schuetzt die Kapazitaet des Knotens, nicht gegen einen
// bestimmten Absender.
//
// Leer (Vorgabe) = niemand ist freigestellt, das Verhalten ist unveraendert.
var rpcRateLimitFreiListe = rpcRateLimitFreiAusUmgebung()

func rpcRateLimitFreiAusUmgebung() map[string]bool {
	roh := strings.TrimSpace(os.Getenv("AEQUITAS_RPC_RATE_LIMIT_FREI"))
	if roh == "" {
		return nil
	}
	frei := make(map[string]bool)
	for _, teil := range strings.Split(roh, ",") {
		teil = strings.TrimSpace(teil)
		if teil == "" {
			continue
		}
		ip := net.ParseIP(teil)
		if ip == nil {
			// Ein Tippfehler darf nie mehr freistellen als gemeint -- er
			// stellt einfach niemanden frei, und das Log sagt es.
			fmt.Printf("[RPC] ⚠ AEQUITAS_RPC_RATE_LIMIT_FREI: %q ist keine IP-Adresse -- ignoriert\n", teil)
			continue
		}
		frei[ip.String()] = true
	}
	if len(frei) > 0 {
		namen := make([]string, 0, len(frei))
		for ip := range frei {
			namen = append(namen, ip)
		}
		fmt.Printf("[RPC] ⚠ Ratenbegrenzung auf /rpc gilt NICHT fuer %s (AEQUITAS_RPC_RATE_LIMIT_FREI) -- nur fuer eigene Maschinen, etwa den Lastgenerator auf dem Partnerknoten\n",
			strings.Join(namen, ", "))
	}
	return frei
}

// rpcRateLimitFrei meldet, ob diese Verbindung von einer freigestellten
// Adresse kommt. Geprueft wird ausschliesslich r.RemoteAddr, siehe oben.
func rpcRateLimitFrei(r *http.Request) bool {
	return rpcRateLimitFreiFuer(rpcRateLimitFreiListe, r)
}

func rpcRateLimitFreiFuer(liste map[string]bool, r *http.Request) bool {
	if len(liste) == 0 || r == nil {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return liste[ip.String()]
}
