package keeper

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto"
)

// Einen Validator eintragen, ohne Kommandozeile.
//
// WAS HIER SCHIEFLIEGT
//
// Die Eintragung verlangt DREI Nachweise: die Wallet des Menschen, der
// Knotenschluessel und der Bezeugungsschluessel. Zwei davon entstehen nur auf
// der Maschine, auf der die Schluessel liegen -- der Weg fuehrte also ueber
// eine Shell.
//
// Das Ergebnis ist messbar: von drei Bezeugungsschluesseln steht genau EINER
// in der Kette. Die anderen zwei stehen in PERSONHOOD_PUBLIC_KEYS, einer
// handgepflegten Liste auf jeder Box -- also in genau der Genehmigung, die
// das Register abschaffen sollte. Und C1 produziert Bloecke, ohne im
// Validatorenregister zu stehen.
//
// Beim Coordinator ist derselbe Schritt am selben Hindernis dreimal
// gescheitert, bevor er in den Browser wanderte. Hier ist er noch gar nicht
// gegangen worden.
//
// WIE ES JETZT LAEUFT
//
// Dieser Knoten stellt zusammen, was nur er zusammenstellen kann:
//
//   - seinen eigenen Knotenschluessel-Nachweis (er hat RELAYER_PRIVATE_KEY)
//   - den Bezeugungsnachweis, den er beim Vergleichsdienst abholt
//   - den Satz, den der Mensch unterschreiben muss
//
// Die Seite unterschreibt und sendet. Kein Terminal.
//
// WARUM DAS KEIN SIGNIER-ORAKEL IST
//
// Der Knoten unterschreibt hier GENAU EINEN festen Satz:
//
//	"Aequitas: validator key linked to human <wallet>"
//
// Das ist eine Selbstauskunft, kein frei waehlbarer Text. Wer sie fuer eine
// fremde Wallet abholt, hat nichts gewonnen: ohne die Unterschrift EBEN
// JENER Wallet -- die nur ihr Inhaber leisten kann -- weist die Kette die
// Eintragung ab. Genau dieselbe Ueberlegung wie beim /besitznachweis des
// Coordinators.
//
// Der bestehende sign-validator-challenge bleibt unberuehrt: der unterschreibt
// beliebige Herausforderungen und ist deshalb zu Recht auf Loopback plus Token
// beschraenkt.

// holeBezeugungsnachweis fragt den Vergleichsdienst nach seinem Nachweis.
func holeBezeugungsnachweis(basis, wallet string) (pub string, sig string, err error) {
	basis = strings.TrimRight(strings.TrimSpace(basis), "/")
	if basis == "" {
		return "", "", fmt.Errorf("matching_url is empty")
	}
	if !isAllowedPeerURL(basis) {
		return "", "", fmt.Errorf("matching_url must be a public https:// address")
	}
	abfrage := basis + "/bezeugungsnachweis?wallet=" + url.QueryEscape(strings.ToLower(wallet))
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Get(abfrage)
	if err != nil {
		return "", "", fmt.Errorf("matching service not reachable at %s: %w", basis, err)
	}
	defer resp.Body.Close()
	roh, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if err != nil {
		return "", "", fmt.Errorf("could not read the matching service's answer: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("the matching service refused (HTTP %d) -- is VALIDATOR_SIGNING_KEY set there?",
			resp.StatusCode)
	}
	var b struct {
		PersonhoodKey       string `json:"personhood_key"`
		PersonhoodSignature string `json:"personhood_signature"`
	}
	if err := json.Unmarshal(roh, &b); err != nil {
		return "", "", fmt.Errorf("the matching service's answer was not the expected JSON")
	}
	if b.PersonhoodKey == "" || b.PersonhoodSignature == "" {
		return "", "", fmt.Errorf("the matching service returned no proof -- is VALIDATOR_SIGNING_KEY set there?")
	}
	return b.PersonhoodKey, b.PersonhoodSignature, nil
}

// handleValidatorSelfProof stellt alles zusammen, was nur dieser Knoten kann.
//
// Antwort enthaelt nur Oeffentliches: Adressen, oeffentliche Schluessel,
// Signaturen ueber feste Saetze und den Satz, der zu unterschreiben ist.
func (a *APIServer) handleValidatorSelfProof(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	wallet := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("wallet")))
	if !strings.HasPrefix(wallet, "0x") || len(wallet) != 42 {
		jsonError(w, "wallet must be a 0x address", http.StatusBadRequest)
		return
	}

	// Dasselbe Verfahren wie handleSignValidatorChallenge -- derselbe feste
	// Satz, derselbe Schluessel. Nur ohne dessen Loopback-Sperre, weil hier
	// nichts frei Waehlbares unterschrieben wird.
	key := a.blockchain.GetSigningKey()
	if key == nil {
		jsonError(w, "this node has no RELAYER_PRIVATE_KEY configured, so it cannot prove its own signing key",
			http.StatusServiceUnavailable)
		return
	}
	signingAddr := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	sig, err := crypto.Sign(accounts.TextHash([]byte("Aequitas: validator key linked to human "+wallet)), key)
	if err != nil {
		jsonError(w, "could not sign with this node's key", http.StatusInternalServerError)
		return
	}
	sig[64] += 27
	signingSig := "0x" + hex.EncodeToString(sig)

	antwort := map[string]interface{}{
		"signing_address":       signingAddr,
		"signing_key_signature": signingSig,
		// Genau der Satz, den die Wallet unterschreiben muss. Ihn mitzugeben
		// nimmt der Seite die Gelegenheit, ihn falsch zusammenzusetzen -- ein
		// Leerzeichen daneben, und die Kette lehnt ab.
		"message": "Aequitas: authorize validator " + signingAddr,
	}

	// Der Bezeugungsnachweis ist freiwillig: ein Knoten ohne eigenen
	// Vergleichsdienst darf sich trotzdem eintragen, er zaehlt dann nur nicht
	// als Bezeuger.
	if mu := strings.TrimSpace(r.URL.Query().Get("matching_url")); mu != "" {
		pub, sig, err := holeBezeugungsnachweis(mu, wallet)
		if err != nil {
			antwort["personhood_error"] = err.Error()
		} else {
			antwort["personhood_key"] = pub
			antwort["personhood_signature"] = sig
			antwort["matching_url"] = strings.TrimRight(mu, "/")
		}
	}
	json.NewEncoder(w).Encode(antwort)
}
