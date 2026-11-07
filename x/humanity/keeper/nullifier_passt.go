package keeper

// GEHOERT DIESER NULLIFIER ZU DIESER WALLET? (Wiederanmeldung nach Widerruf)
//
// Wer seine biometrischen Daten loescht (Widerruf), behaelt sein Menschsein
// in der Kette, kann aber ohne hinterlegtes Gesicht keinen Lebensnachweis mehr
// erbringen. Er soll sein Gesicht wieder hinterlegen koennen -- ohne zweite
// Praegung (aequitas-biometric-beta, nachziehen.py, /reaktivieren). Der
// Coordinator findet ueber die zurueckbehaltene Marke die URSPRUENGLICHE
// Einschreibung und fragt hier: gehoert ihr Nullifier zu der Wallet, die den
// Antrag unterschrieben hat? Nur dann wird reaktiviert.
//
// GET /api/nullifier-passt?n=<nullifier>&wallet=<wallet> -> {"passt": bool}
//
// Kein Aufzaehlen: ohne beide Werte gibt es keine Antwort, und die Antwort ist
// nur ja/nein. Die Verbindung selbst steht ohnehin in jedem Block
// (register_human traegt Nullifier und Wallet). Grenze je Absender, weil ein
// Fehlgriff im Speicher die Datenbank fragt.

import (
	"encoding/json"
	"net/http"
	"strings"
)

const burstNullifierPasstJeIP = 30

func (a *APIServer) handleNullifierPasst(w http.ResponseWriter, r *http.Request) {
	writeJSON(w)
	if r.Method != http.MethodGet {
		jsonError(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	n := strings.TrimSpace(r.URL.Query().Get("n"))
	wallet := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("wallet")))
	if !nullifierDezimal.MatchString(n) || !kanonischeAdresse(wallet) {
		jsonError(w, "n (decimal nullifier) and wallet (0x + 40 lower-case hex) required", http.StatusBadRequest)
		return
	}
	if !burstErlaubt("nullifier-passt:"+erneuerungsAbsender(r), burstNullifierPasstJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	gehoert := strings.ToLower(a.state.GetWalletByNullifier(n))
	json.NewEncoder(w).Encode(map[string]bool{"passt": gehoert != "" && gehoert == wallet})
}
