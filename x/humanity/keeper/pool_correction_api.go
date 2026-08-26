package keeper

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

// Der Konfigurationsschluessel, mit dem sich die Korrektur ohne Neustart
// freigeben laesst. Ihn zu setzen verlangt Zugriff auf die Datenbank der Box.
const poolCorrectionAllowKey = "allow_pool_correction"

// handlePoolCorrection loest die einmalige Korrektur eines Ueberschusses aus,
// der vor den Reihenfolge-Korrekturen vom 20.08.2026 entstanden ist.
//
// Das ist der eingriffsstaerkste Endpunkt dieser Kette: er vernichtet Geld.
// Deshalb drei Riegel statt einem, und keiner davon ist Zierde.
//
//  1. ALLOW_POOL_CORRECTION=true auf diesem Knoten. Ohne bewusste Freigabe
//     existiert der Weg nicht.
//  2. Authorization: Bearer <SNAPSHOT_TOKEN>. Wie bei den anderen
//     Betreiber-Endpunkten; ohne gesetztes Token ist der Weg zu, nicht offen.
//  3. Die Betraege stehen in der ANFRAGE, nicht im Programm. Eine Funktion,
//     die selbst ausrechnet, wieviel Geld zuviel da ist, wuerde bei einem
//     Fehler in dieser Rechnung unbemerkt enteignen -- und bei jedem Aufruf
//     erneut zuschlagen. Wer korrigiert, muss die Zahl genannt haben.
//
// Dazu ein Bestaetigungswort, damit ein versehentlich abgeschickter Aufruf
// nicht reicht.
func (a *APIServer) handlePoolCorrection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST erforderlich"}`, http.StatusMethodNotAllowed)
		return
	}
	// Freigabe ueber Umgebungsvariable ODER einen Schluessel in der Datenbank.
	//
	// Die Variable ist die Hausform, wirkt aber erst nach einem Neustart --
	// und einen Validator neu zu starten, nur um eine einmalige Korrektur zu
	// erlauben, ist ein groesserer Eingriff als die Korrektur selbst. Eine
	// Markierungsdatei auf der Box waere kein Ersatz: der Knoten laeuft im
	// Container und saehe sie nicht.
	//
	// Der Datenbankschluessel verlangt Zugriff auf die Box, ist also kein
	// schwaecheres Tor, und laesst sich vorher setzen und hinterher wieder
	// entfernen, ohne die Kette anzuhalten.
	freigabe := os.Getenv("ALLOW_POOL_CORRECTION") == "true" ||
		a.state.getConfigValueDB(poolCorrectionAllowKey) == "1"
	if !freigabe {
		http.Error(w, `{"error":"pool-correction ist abgeschaltet; ALLOW_POOL_CORRECTION=true setzen oder `+poolCorrectionAllowKey+`=1 in chain_config schreiben"}`, http.StatusForbidden)
		return
	}
	// Zweiter Riegel: entweder ein gueltiges Token, ODER die Anfrage kommt von
	// der Box selbst.
	//
	// Der Weg ueber die Schleife ist hier nicht die Notloesung, sondern der
	// natuerliche: diese Korrektur ist eine Betreiberhandlung AUF der Maschine.
	// Wer dort etwas ausfuehren kann, haette ohnehin Zugriff auf Datenbank und
	// Prozess -- ein zusaetzliches Token schuetzt gegen niemanden, der schon
	// so weit ist.
	//
	// Gegen aussen schuetzt es sehr wohl: Port 8080 ist oeffentlich
	// erreichbar, und von dort ist RemoteAddr nie die Schleife. Ohne Token
	// bleibt der Endpunkt fuer das Internet also zu.
	//
	// (Auf diesem Knoten war SNAPSHOT_TOKEN am 26.08.2026 gar nicht gesetzt --
	// der Endpunkt waere ohne diesen Weg unbenutzbar gewesen, und die Absage
	// haette wie eine Fehlkonfiguration ausgesehen statt wie eine Absicht.)
	token := os.Getenv("SNAPSHOT_TOKEN")
	auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	tokenOK := token != "" && subtle.ConstantTimeCompare([]byte(auth), []byte(token)) == 1
	if !tokenOK && !vonDerSchleife(r) {
		http.Error(w, `{"error":"unauthorized — Authorization: Bearer <SNAPSHOT_TOKEN>, oder von der Box selbst aufrufen"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		BurnAEQ  float64 `json:"burn_aeq"`
		BurnTUSD float64 `json:"burn_tusd"`
		Confirm  string  `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"unlesbarer Rumpf"}`, http.StatusBadRequest)
		return
	}
	if req.Confirm != "reserve-korrigieren" {
		http.Error(w, `{"error":"confirm muss \"reserve-korrigieren\" lauten"}`, http.StatusBadRequest)
		return
	}

	if err := a.state.CorrectPhantomSupplyAtomic(req.BurnAEQ, req.BurnTUSD); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusConflict)
		return
	}

	reserveAEQ, reserveTUSD := a.state.GetPoolReserves()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":           true,
		"burned_aeq":   req.BurnAEQ,
		"burned_tusd":  req.BurnTUSD,
		"reserve_aeq":  reserveAEQ,
		"reserve_tusd": reserveTUSD,
		// Zum sofortigen Nachsehen, ob die Regel den Bestand jetzt erklaert.
		"hinweis": "supply_reconciliation unter /api/health/combined pruefen; " +
			"nach erfolgreicher Korrektur SUPPLY_GAP_BASELINE_AEQ=0 setzen, " +
			"damit der Alarm auf dem neuen Stand scharf wird",
	})
}

// vonDerSchleife sagt, ob die Anfrage von der Maschine selbst kommt.
//
// Bewusst OHNE Beruecksichtigung von X-Forwarded-For: dieser Kopfzeile darf
// hier nichts glauben, wer sie nicht selbst gesetzt hat, und ein Angreifer
// setzt sie frei. Gefragt wird ausschliesslich die tatsaechliche Gegenstelle
// der Verbindung.
func vonDerSchleife(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}
