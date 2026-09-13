package keeper

import (
	"encoding/json"
	"fmt"
	"net/http"
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

	// 2. Proof-Server: ohne ihn keine Registrierung.
	a.proofStatusMu.RLock()
	proofDa := len(a.proofServerStatus) > 0
	a.proofStatusMu.RUnlock()
	if !proofDa {
		rot = append(rot, "Proof-Server nicht erreichbar -- keine Registrierung moeglich")
	} else {
		gruen = append(gruen, "Proof-Server erreichbar")
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
