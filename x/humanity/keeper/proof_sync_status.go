package keeper

// Sichtbarkeit der Meldung an den Proof-Server (Audit von null, 02.10.2026).
//
// Nach jeder Registrierung meldet der Knoten den Bio-Hash an alle
// Proof-Server (notifyProofServer), damit deren Duplikat-Zwischenspeicher
// denselben Menschen kennt wie die Kette. Fehlten PROOF_SERVER_URLS oder
// CHAIN_SERVICE_TOKEN, wurde das bis hierher STILL uebersprungen: kein Log,
// kein Eintrag in der Warteschlange, kein Statusfeld. Gemessen stand live
// bio_hash_count 0 gegen 1 Mensch, und niemand konnte von aussen sagen,
// warum. Eine Schutzschicht, deren Fehlen niemand sieht, prueft auch
// niemand nach.
//
// Veroeffentlicht werden nur ein Ja/Nein und drei Zaehler seit dem
// Prozessstart -- nie eine URL und nie ein Token.

import (
	"os"
	"sync/atomic"
)

var (
	proofSyncErfolgreich   atomic.Int64
	proofSyncUebersprungen atomic.Int64
	proofSyncFehlgeschl    atomic.Int64
)

// proofSyncKonfiguriert: sind beide Werte gesetzt, ohne die notifyProofServer
// nichts sendet?
func proofSyncKonfiguriert() bool {
	return len(proofServerURLs()) > 0 && os.Getenv("CHAIN_SERVICE_TOKEN") != ""
}

// proofSyncStand fuer /api/status.
func proofSyncStand() map[string]interface{} {
	return map[string]interface{}{
		"konfiguriert":   proofSyncKonfiguriert(),
		"erfolgreich":    proofSyncErfolgreich.Load(),
		"uebersprungen":  proofSyncUebersprungen.Load(),
		"fehlgeschlagen": proofSyncFehlgeschl.Load(),
	}
}
