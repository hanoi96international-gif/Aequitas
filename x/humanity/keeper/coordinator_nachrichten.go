package keeper

import (
	"fmt"
	"strings"
)

// DIE NACHRICHTEN DES COORDINATORS TRAGEN DIE CHAIN-ID (v2, Bedingung vor
// der Staffel, grant_staffel.go).
//
// Ohne Chain-ID galt eine Unterschrift in jedem Netz mit derselben Domaene:
// eine Lebendigkeitspruefung derselben Wallet in einem Testnetz waere hier
// eine gueltige Erneuerung gewesen, und Freigabe und Besitznachweis einer
// Eintragung ebenso. Wie bei der Validator-Bindung (validator_register.go,
// "chain:<id>") steht die Chain-ID jetzt in allen drei Saetzen:
//
//   - Besitznachweis (Ed25519, Coordinator-Schluessel):
//     "Aequitas: coordinator key for human <mensch> chain:<id>"
//   - Freigabe (EIP-191, Wallet des Menschen):
//     "Aequitas: authorize coordinator <schluessel> chain:<id>"
//   - Bescheinigung (Ed25519): "aequitas-liveness-renewal-v2|chain:<id>|<wallet>|<issued_at>"
//
// Im Konsens (bescheinigungPruefen) gilt nur v2. Die Eintragung
// (coordinator_registry.go) nimmt uebergangsweise auch die alten Saetze, damit
// die Reihenfolge der Deploys von Kette und Coordinator egal ist -- eine so
// eingetragene Bindung taugt aber fuer keine Bescheinigung; der Betreiber
// traegt sich vor der Staffel einmal mit v2 neu ein.

const livenessRenewalDomain = "aequitas-liveness-renewal-v2"

// coordinatorBesitzNachricht: was der Coordinator-Schluessel fuer den
// Menschen unterschreibt (v2).
func coordinatorBesitzNachricht(mensch string) string {
	return fmt.Sprintf("Aequitas: coordinator key for human %s chain:%d", strings.ToLower(strings.TrimSpace(mensch)), aequitasChainID.Int64())
}

// coordinatorFreigabeNachricht: was der Mensch fuer seinen Schluessel
// unterschreibt (v2).
func coordinatorFreigabeNachricht(schluessel string) string {
	return fmt.Sprintf("Aequitas: authorize coordinator %s chain:%d", strings.ToLower(strings.TrimSpace(schluessel)), aequitasChainID.Int64())
}

// erneuerungsNachricht: was der Coordinator fuer eine Erneuerung
// unterschreibt (v2).
func erneuerungsNachricht(wallet string, issuedAt int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%d", livenessRenewalDomain, aequitasChainID.Int64(), wallet, issuedAt)
}

// Die alten Saetze ohne Chain-ID -- nur fuer die Eintragung im Uebergang.
func coordinatorBesitzNachrichtV1(mensch string) string {
	return "Aequitas: coordinator key for human " + strings.ToLower(strings.TrimSpace(mensch))
}

func coordinatorFreigabeNachrichtV1(schluessel string) string {
	return "Aequitas: authorize coordinator " + strings.ToLower(strings.TrimSpace(schluessel))
}
