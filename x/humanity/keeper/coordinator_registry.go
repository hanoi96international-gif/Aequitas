package keeper

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Das Register der anerkannten Coordinatoren.
//
// WARUM ES DAS BRAUCHT
//
// Der Bezeugungsschluessel eines Validators steht seit dem 26.08.2026 in der
// Kette: wer sich eintraegt, wird anerkannt, ohne dass jemand eine Datei
// anfasst. Fuer den COORDINATOR-Schluessel galt das noch nicht -- der stand
// weiter in COORDINATOR_PUBLIC_KEYS, einer Umgebungsvariablen, die jemand auf
// JEDER Box eintragen muss.
//
// Damit brauchte ein neuer Coordinator trotz allem noch jemanden, der ihn
// eintraegt. Das ist eine Genehmigung, und sie sass an der wichtigsten Stelle:
// der Coordinator ist der Eingang, an dem ein Mensch ankommt.
//
// Hier haengt der Schluessel an derselben Bindung wie ueberall sonst -- ein
// Mensch, ein Schluessel, mit Besitznachweis, oeffentlich nachpruefbar.
//
// DAS REGISTER IST KNOTENLOKAL, NICHT REPLIZIERT
//
// RegisterCoordinatorKey schreibt direkt in die Datenbank DIESES Knotens. Es
// gibt dafuer weder einen Transaktionstyp noch Gossip: eine Eintragung auf C1
// erreicht C2 nicht. Beim Validatorenregister ist es genauso -- dass dort auf
// beiden Boxen ein Eintrag steht, kommt daher, dass er auf beiden einzeln
// gesetzt wurde.
//
// Das ist Absicht und die richtige Richtung: kein Knoten bekommt eine
// Vertrauensliste von aussen aufgezwungen. Wer betreibt, entscheidet selbst,
// wessen Bescheinigungen er annimmt -- eine replizierte Liste waere genau die
// zentrale Instanz, die es hier nicht geben soll.
//
// Der Preis: eine Eintragung muss an JEDEN Knoten gehen, der sie gelten lassen
// soll. Die Unterschrift ist dabei uebertragbar, sie haengt am Schluessel und
// nicht am Empfaenger -- einmal unterschreiben, an alle senden. Wer das
// vergisst, hat einen Coordinator, den die eine Haelfte des Netzes annimmt und
// die andere abweist; siehe scripts/coordinator-eintragen.sh.
//
// WAS EIN EINTRAG BEDEUTET, UND WAS NICHT
//
// Er sagt: dieser Schluessel gehoert einem registrierten Menschen, und der hat
// den Besitz nachgewiesen. Er sagt NICHT, dass dieser Coordinator
// vertrauenswuerdig handelt.
//
// Ein eingetragener Coordinator kann eine BESTEHENDE bio_hash an eine andere
// Wallet binden -- die in attest_sign.py benannte Restgefahr, und sie waechst
// mit jedem Eintrag. Dagegen steht das Bezeugungs-Quorum: ohne zwei
// verschiedene Validatoren entsteht gar keine bio_hash, die er binden koennte.
//
// Ein boesartiger Coordinator kann also Zuteilungen umlenken, aber keine
// Menschen erfinden. Diesen Unterschied muss kennen, wer die Zahl der
// Coordinatoren erhoeht.

// EnsureCoordinatorRegistry legt die Tabelle an. Idempotent.
func (cs *ChainState) EnsureCoordinatorRegistry() {
	if cs.db == nil {
		return
	}
	cs.db.Exec(`CREATE TABLE IF NOT EXISTS coordinator_keys (
		public_key    TEXT PRIMARY KEY,
		human_wallet  TEXT NOT NULL,
		url           TEXT,
		registered_at TIMESTAMP DEFAULT NOW()
	)`)
}

// CoordinatorEntry ist ein anerkannter Coordinator.
type CoordinatorEntry struct {
	PublicKey   string `json:"public_key"`
	HumanWallet string `json:"human_wallet"`
	URL         string `json:"url,omitempty"`
}

// RegisterCoordinatorKey traegt einen Coordinator ein.
func (cs *ChainState) RegisterCoordinatorKey(publicKey, humanWallet, url string) error {
	if cs.db == nil {
		return fmt.Errorf("no database")
	}
	publicKey = strings.ToLower(strings.TrimSpace(publicKey))
	humanWallet = strings.ToLower(strings.TrimSpace(humanWallet))
	url = strings.TrimRight(strings.TrimSpace(url), "/")
	if !cs.IsHuman(humanWallet) {
		return fmt.Errorf("human_wallet %s is not a registered human", humanWallet)
	}
	cs.EnsureCoordinatorRegistry()
	_, err := cs.db.Exec(
		`INSERT INTO coordinator_keys (public_key, human_wallet, url)
		 VALUES ($1, $2, NULLIF($3, ''))
		 ON CONFLICT (public_key) DO UPDATE SET
		   human_wallet = $2,
		   url = COALESCE(NULLIF($3, ''), coordinator_keys.url),
		   registered_at = NOW()`,
		publicKey, humanWallet, url)
	return err
}

// Coordinators liefert alle anerkannten Coordinatoren.
func (cs *ChainState) Coordinators() []CoordinatorEntry {
	if cs.db == nil {
		return nil
	}
	cs.EnsureCoordinatorRegistry()
	rows, err := cs.db.Query(
		`SELECT public_key, human_wallet, COALESCE(url, '')
		 FROM coordinator_keys ORDER BY registered_at`)
	if err != nil {
		fmt.Printf("[COORDINATORS] Abfrage fehlgeschlagen: %v\n", err)
		return nil
	}
	defer rows.Close()
	var out []CoordinatorEntry
	for rows.Next() {
		var k, w, u string
		if rows.Scan(&k, &w, &u) == nil && k != "" {
			out = append(out, CoordinatorEntry{PublicKey: k, HumanWallet: w, URL: u})
		}
	}
	return out
}

// handleRegisterCoordinatorKey traegt einen Coordinator-Schluessel ein.
//
// Zwei Nachweise, wie bei den Validatoren:
//
//  1. Der MENSCH autorisiert diesen Schluessel (EIP-191, secp256k1).
//  2. Der SCHLUESSEL beweist, dass er zu diesem Menschen gehoert (Ed25519).
//
// Ohne den zweiten koennte jemand einen FREMDEN oeffentlichen Schluessel unter
// seinem Namen eintragen -- und dessen Bescheinigungen wuerden fortan als
// seine gelten.
func (a *APIServer) handleRegisterCoordinatorKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		PublicKey      string `json:"public_key"`
		HumanWallet    string `json:"human_wallet"`
		HumanSignature string `json:"human_signature"`
		KeySignature   string `json:"key_signature"`
		URL            string `json:"url"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	pub := strings.ToLower(strings.TrimSpace(req.PublicKey))
	human := strings.ToLower(strings.TrimSpace(req.HumanWallet))
	if len(pub) != 64 || strings.Trim(pub, "0123456789abcdef") != "" {
		jsonError(w, "public_key must be 64 hex characters (Ed25519)", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(human, "0x") || len(human) != 42 {
		jsonError(w, "invalid human_wallet", http.StatusBadRequest)
		return
	}
	if err := verifyPersonalSign("Aequitas: authorize coordinator "+pub, req.HumanSignature, human); err != nil {
		jsonError(w, "invalid human_signature: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !verifyCoordinatorPossession(pub, req.KeySignature, human) {
		jsonError(w, "invalid key_signature -- sign the coordinator-key message with the Ed25519 key itself",
			http.StatusBadRequest)
		return
	}
	url := strings.TrimRight(strings.TrimSpace(req.URL), "/")
	if url != "" && !isAllowedPeerURL(url) {
		jsonError(w, "url must be a public https:// address", http.StatusBadRequest)
		return
	}
	if err := a.state.RegisterCoordinatorKey(pub, human, url); err != nil {
		jsonStateError(w, "register-coordinator-key", pub, err)
		return
	}
	fmt.Printf("[COORDINATOR] Registered %s for human %s\n", pub[:16], human)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true, "public_key": pub, "human_wallet": human, "url": url,
	})
}

// handleCoordinatorList gibt die anerkannten Coordinatoren aus.
//
// Ohne Token: es sind oeffentliche Schluessel und oeffentliche Adressen. Genau
// darin liegt der Zweck -- jeder Validator und jeder Proof-Server soll die
// Liste lesen koennen, ohne dafuer ein Geheimnis zu brauchen.
func (a *APIServer) handleCoordinatorList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"coordinators": a.state.Coordinators(),
	})
}

func verifyCoordinatorPossession(publicHex, signatureHex, humanWallet string) bool {
	roh, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(signatureHex), "0x"))
	if err != nil || len(roh) != ed25519.SignatureSize {
		return false
	}
	pub, err := hex.DecodeString(publicHex)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	msg := []byte("Aequitas: coordinator key for human " + strings.ToLower(strings.TrimSpace(humanWallet)))
	return ed25519.Verify(ed25519.PublicKey(pub), msg, roh)
}
