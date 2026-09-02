package keeper

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// handleSybilReport liefert den rueckwirkenden Sybil-Report als JSON.
//
// # WARUM GESCHUETZT, OBWOHL DIE ADRESSEN OEFFENTLICH SIND
//
// Jede Wallet-Adresse steht ohnehin unter /api/humans. Der Report ist
// trotzdem etwas anderes als eine Liste: er haengt an identifizierbaren
// Konten einen VERDACHT. Ihn offen auszuliefern hiesse, Menschen oeffentlich
// als mutmassliche Betrueger zu fuehren, auf Grundlage von Heuristiken, die
// nach dem eigenen Modul-Kommentar ausdruecklich nichts beweisen -- eine
// unberuehrte Praemie kann schlicht jemand sein, der wartet.
//
// Zweitens verriete er einem Angreifer, welche Schwellen ihn erwischt haben.
// Wer die Schwellen kennt, bleibt darunter.
//
// Rein lesend: das Modul faellt keine Entscheidung und sperrt niemanden. Es
// sind Anhaltspunkte fuer eine Untersuchung durch einen Menschen, keine
// Urteile.
func (a *APIServer) handleSybilReport(w http.ResponseWriter, r *http.Request) {
	token := os.Getenv("SNAPSHOT_TOKEN")
	auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	tokenOK := token != "" && subtle.ConstantTimeCompare([]byte(auth), []byte(token)) == 1
	if !tokenOK && !vonDerSchleife(r) {
		http.Error(w, `{"error":"unauthorized — Authorization: Bearer <SNAPSHOT_TOKEN>, oder von der Box selbst aufrufen"}`, http.StatusUnauthorized)
		return
	}

	stunden := 24
	if roh := r.URL.Query().Get("hours"); roh != "" {
		if v, err := strconv.Atoi(roh); err == nil {
			stunden = v
		}
	}

	bericht := a.state.BuildSybilReport(stunden)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bericht)
}
