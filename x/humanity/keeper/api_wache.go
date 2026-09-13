package keeper

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

var wacheLetzteMeldungUnix atomic.Int64

// /api/wache -- die Selbstpruefung des Knotens fuer einen Uptime-Monitor.
//
// WARUM. Die Wache in GitHub Actions (wache.yml) soll alle zehn Minuten
// laufen; GitHub fuehrt sie tatsaechlich alle paar Stunden aus (13.09.2026:
// 01:11, 06:17, 12:05). Ein Cron, der sechs Stunden schweigt, ist keine
// Wache. Ein freier Uptime-Monitor (UptimeRobot, healthchecks.io, ...) fragt
// diese Adresse alle fuenf Minuten ab und schickt bei 503 eine Mail -- ohne
// Geheimnis auf der Box, ohne Cron-Verspaetung.
//
// 200 = alles, was ein Launch braucht, ist da. 503 = mindestens ein Punkt
// fehlt; der Rumpf nennt ihn. Geprueft wird nur, was dieser Knoten selbst
// weiss oder in Sekunden erfragen kann.

func (a *APIServer) handleWache(w http.ResponseWriter, r *http.Request) {
	rot := []string{}
	gruen := []string{}

	// 1. Produktion: der letzte eigene Block darf nicht zu lange her sein.
	if d := productionStalledFor(); d > 120*time.Second {
		rot = append(rot, fmt.Sprintf("kein eigener Block seit %s", d.Round(time.Second)))
	} else {
		gruen = append(gruen, "produziert")
	}

	// 2. Proof-Server: ohne ihn keine Registrierung -- und ohne
	//    BIO_ATTESTATION_MODE=required eine Registrierung ohne Gesichtspruefung.
	//
	//    Der zweite Punkt ist seit dem 13.09.2026 Teil der Wache: an dem Tag
	//    stand der Modus 14 Stunden auf "optional", und nichts hat es gemeldet.
	//    Ein Tor, das offen steht, sieht von aussen genauso aus wie eines, das
	//    zu ist -- ausser man fragt.
	a.proofStatusMu.RLock()
	proofDa := len(a.proofServerStatus) > 0
	modus := proofServerModus(a.proofServerStatus)
	a.proofStatusMu.RUnlock()
	if !proofDa {
		rot = append(rot, "Proof-Server nicht erreichbar -- keine Registrierung moeglich")
	} else if modus != "required" {
		rot = append(rot, fmt.Sprintf("Proof-Server BIO_ATTESTATION_MODE=%q statt required -- Registrierung ohne Gesichtspruefung moeglich", modus))
	} else if q := proofServerQuorum(a.proofServerStatusKopie()); q > 0 && q < 2 {
		// Quorum 1 hiesse: EIN geleakter Schluessel bezeugt beliebig viele
		// Menschen, und niemand merkt es (bio_attestation.js, QUORUM).
		rot = append(rot, fmt.Sprintf("Proof-Server BIO_ATTESTATION_QUORUM=%d -- ein einzelner Schluessel genuegt fuer eine Registrierung", q))
	} else {
		gruen = append(gruen, "Proof-Server erreichbar, Gesichtspruefung Pflicht")
	}

	// 2b. Coordinator auf dieser Box (aequitas-coordinator im selben Docker-
	//     Netz). Kein Container = kein Befund (ein Validator ohne eigenen
	//     Coordinator ist erlaubt); ein Container, der nicht antwortet = rot,
	//     denn dann registriert ueber diese Box niemand.
	switch stand, ok := coordinatorWacheStand(); {
	case stand == "":
		// uebersprungen -- steht in in_ordnung, damit man es sieht
		gruen = append(gruen, "kein Coordinator auf dieser Box (uebersprungen)")
	case ok:
		gruen = append(gruen, "Coordinator antwortet ("+stand+")")
	default:
		rot = append(rot, "Coordinator auf dieser Box antwortet nicht: "+stand)
	}

	// 3. Kontostand einig mit den Seeds (divergenz_waechter.go).
	if divergenzStrikes.Load() >= divergenzSchwelle {
		rot = append(rot, "Kontenstand weicht vom Partner ab -- Resync noetig")
	} else {
		gruen = append(gruen, "keine Divergenz")
	}
	if n := uebersprungeneUeberweisungen.Load(); n > 0 {
		rot = append(rot, fmt.Sprintf("%d Ueberweisungen beim Nachspielen uebersprungen", n))
	}

	// 4. Platte.
	pl := PlattenplatzStand()
	if k, _ := pl["kritisch"].(bool); k {
		rot = append(rot, fmt.Sprintf("Platte kritisch (%v MB frei)", pl["frei_mb"]))
	} else if b, _ := pl["belegt_pct"].(float64); b > 85 {
		rot = append(rot, fmt.Sprintf("Platte zu %.0f %% belegt", b))
	} else {
		gruen = append(gruen, "Platte ok")
	}

	// 5. Nicht degraded.
	if reason := a.state.BootstrapDegradedReason(); reason != "" {
		rot = append(rot, "Knoten degraded: "+reason)
	}
	a.blockchain.degradedMu.Lock()
	dr := a.blockchain.degradedReason
	a.blockchain.degradedMu.Unlock()
	if dr != "" {
		rot = append(rot, "DAG degraded: "+dr)
	}

	// 6. Die Seeds antworten und sind gleichauf (Hoehe zum Zeitpunkt der Abfrage).
	a.blockchain.syncPeerMu.Lock()
	seeds := make([]string, 0, len(a.blockchain.trustedSeeds))
	for s := range a.blockchain.trustedSeeds {
		seeds = append(seeds, s)
	}
	a.blockchain.syncPeerMu.Unlock()
	for _, seed := range seeds {
		peer, eigene, ok := echteHoeheVonPeerMitEigener(seed)
		if !ok {
			rot = append(rot, "Seed antwortet nicht: "+seed)
			continue
		}
		if d := eigene - peer; d > 20 || d < -20 {
			rot = append(rot, fmt.Sprintf("nicht gleichauf mit %s (Abstand %d)", seed, d))
		} else {
			gruen = append(gruen, "gleichauf mit "+seed)
		}
	}

	stand := map[string]interface{}{
		"gruen":      len(rot) == 0,
		"befunde":    rot,
		"in_ordnung": gruen,
		"hoehe":      a.blockchain.HeightSchnell(),
		"zeit":       time.Now().UTC().Format(time.RFC3339),
		"bedeutung": "Selbstpruefung fuer einen Uptime-Monitor: 200 = alles da, 503 = befunde nennt, was fehlt. " +
			"Alle fuenf Minuten abfragen; die GitHub-Wache laeuft nur alle paar Stunden.",
	}
	writeJSONCORS(w)
	w.Header().Set("Cache-Control", "no-store")
	if len(rot) > 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	body, _ := json.Marshal(stand)
	w.Write(body)
	if len(rot) > 0 && time.Now().Unix()-wacheLetzteMeldungUnix.Load() >= 300 {
		wacheLetzteMeldungUnix.Store(time.Now().Unix())
		fmt.Printf("[WACHE] ✗ %s\n", strings.Join(rot, " | "))
	}
}

// proofServerModus liest durchsetzung.mode aus der letzten /health-Antwort
// des Proof-Servers ("" wenn nicht enthalten -- alte Fassung ohne das Feld).
func proofServerModus(status map[string]interface{}) string {
	d, _ := status["durchsetzung"].(map[string]interface{})
	if d == nil {
		return ""
	}
	m, _ := d["mode"].(string)
	return strings.ToLower(strings.TrimSpace(m))
}

// coordinatorWacheStand fragt den Coordinator auf derselben Box.
//
// Rueckgabe ("", false): kein Coordinator erreichbar per Namen -- der Name
// loest nicht auf, also gibt es hier keinen. (Stand, true): antwortet mit
// status ok; Stand nennt Quorum und Validatoren. (Grund, false): es gibt ihn,
// aber er antwortet nicht oder nicht mit ok.
//
// AEQUITAS_WACHE_COORDINATOR_URL uebersteuert die Adresse; "aus" schaltet
// die Pruefung ab.
func coordinatorWacheStand() (string, bool) {
	url := strings.TrimSpace(os.Getenv("AEQUITAS_WACHE_COORDINATOR_URL"))
	if strings.EqualFold(url, "aus") || strings.EqualFold(url, "off") {
		return "", false
	}
	if url == "" {
		url = "http://aequitas-coordinator:8200/health"
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		// Kein solcher Host = kein Coordinator hier. Alles andere (Timeout,
		// Verbindung verweigert) heisst: es gibt ihn, er antwortet nicht.
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			return "", false
		}
		return err.Error(), false
	}
	defer resp.Body.Close()
	var body struct {
		Status        string   `json:"status"`
		QuorumSize    int      `json:"quorum_size"`
		ValidatorURLs []string `json:"validator_urls"`
	}
	if resp.StatusCode != 200 {
		return fmt.Sprintf("HTTP %d", resp.StatusCode), false
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "unlesbare Antwort: " + err.Error(), false
	}
	if body.Status != "ok" {
		return "status=" + body.Status, false
	}
	if len(body.ValidatorURLs) < body.QuorumSize || body.QuorumSize == 0 {
		return fmt.Sprintf("Quorum %d bei %d Vergleichsdiensten", body.QuorumSize, len(body.ValidatorURLs)), false
	}
	return fmt.Sprintf("Quorum %d von %d", body.QuorumSize, len(body.ValidatorURLs)), true
}

// proofServerQuorum liest durchsetzung.quorum (0 wenn nicht enthalten).
func proofServerQuorum(status map[string]interface{}) int {
	d, _ := status["durchsetzung"].(map[string]interface{})
	if d == nil {
		return 0
	}
	switch q := d["quorum"].(type) {
	case float64:
		return int(q)
	case int:
		return q
	}
	return 0
}

// proofServerStatusKopie liefert die letzte /health-Antwort unter der Sperre.
func (a *APIServer) proofServerStatusKopie() map[string]interface{} {
	a.proofStatusMu.RLock()
	defer a.proofStatusMu.RUnlock()
	return a.proofServerStatus
}
