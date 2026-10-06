package keeper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
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
// FUER DEN KONSENS ZAEHLT ES NICHT MEHR (06.10.2026)
//
// Die Erneuerungs-Bescheinigung traegt jetzt die Bindung selbst (beide
// Unterschriften dieser Eintragung, grant_staffel.go), und jeder Knoten prueft
// sie gegen den Kettenzustand. Das Register dient nur noch dem annehmenden
// Knoten als Quelle dieser Unterschriften und der Liste fuer die
// Vergleichsdienste. Unten der Stand davor -- fuer die Liste gilt er weiter.
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
//
// EINMAL JE PROZESS, NICHT JE ANFRAGE (Sicherheitspruefung #300, HIGH-2).
// Die Funktion steht in jedem Lese- und Schreibpfad des Registers, auch in
// der oeffentlichen Erneuerung. ALTER TABLE braucht eine ACCESS-EXCLUSIVE-
// Sperre, auch wenn die Spalte schon da ist -- je Anfrage zwei davon hiessen:
// eine lange Lesung (Liste, Sicherung) haelt jede Erneuerung auf, und jede
// wartende Erneuerung haelt alle Leser dahinter auf. Deshalb ein Riegel, der
// nur bei ERFOLG faellt (wie ensureTxRootColumn), und eine Sperrfrist: kommt
// die Sperre nicht binnen zwei Sekunden, bricht das Anlegen ab und der
// naechste Aufruf versucht es erneut -- statt dass Anfragen ohne Grenze
// warten.
func (cs *ChainState) EnsureCoordinatorRegistry() {
	if cs.db == nil || cs.coordinatorRegisterDa.Load() {
		return
	}
	cs.coordinatorRegisterMu.Lock()
	defer cs.coordinatorRegisterMu.Unlock()
	if cs.coordinatorRegisterDa.Load() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := cs.db.BeginTx(ctx, nil)
	if err != nil {
		fmt.Printf("[COORDINATORS] Register nicht angelegt (naechster Versuch beim naechsten Aufruf): %v\n", err)
		return
	}
	defer tx.Rollback()
	for _, q := range []string{
		`SET LOCAL lock_timeout = '2s'`,
		`CREATE TABLE IF NOT EXISTS coordinator_keys (
		public_key    TEXT PRIMARY KEY,
		human_wallet  TEXT NOT NULL,
		url           TEXT,
		registered_at TIMESTAMP DEFAULT NOW()
	)`,
		// Seit 06.10.2026: die beiden Unterschriften der Eintragung. Sie
		// gehen in jede Erneuerungs-Bescheinigung, damit jeder Knoten die
		// Bindung selbst prueft (grant_staffel.go, Lebendigkeitsbescheinigung).
		`ALTER TABLE coordinator_keys ADD COLUMN IF NOT EXISTS human_signature TEXT`,
		`ALTER TABLE coordinator_keys ADD COLUMN IF NOT EXISTS key_signature TEXT`,
	} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			fmt.Printf("[COORDINATORS] Register nicht angelegt (naechster Versuch beim naechsten Aufruf): %v\n", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		fmt.Printf("[COORDINATORS] Register nicht angelegt (naechster Versuch beim naechsten Aufruf): %v\n", err)
		return
	}
	cs.coordinatorRegisterDa.Store(true)
}

// CoordinatorBindung: was eine Eintragung belegt -- in der Bescheinigung
// mitgeschickt.
type CoordinatorBindung struct {
	Mensch        string
	MenschSig     string
	SchluesselSig string
}

// CoordinatorBindungLokal: die Bindung eines eingetragenen Schluessels aus
// dem Register dieses Knotens. false, wenn er nicht eingetragen ist oder die
// Eintragung keine Unterschriften traegt (vor dem 06.10.2026).
func (cs *ChainState) CoordinatorBindungLokal(publicKey string) (CoordinatorBindung, bool) {
	if cs.db == nil {
		return CoordinatorBindung{}, false
	}
	cs.EnsureCoordinatorRegistry()
	var b CoordinatorBindung
	err := cs.db.QueryRow(`SELECT human_wallet, COALESCE(human_signature, ''), COALESCE(key_signature, '')
		FROM coordinator_keys WHERE public_key = $1`, strings.ToLower(strings.TrimSpace(publicKey))).Scan(&b.Mensch, &b.MenschSig, &b.SchluesselSig)
	if err != nil || b.MenschSig == "" || b.SchluesselSig == "" {
		return CoordinatorBindung{}, false
	}
	return b, true
}

// CoordinatorEntry ist ein anerkannter Coordinator.
type CoordinatorEntry struct {
	PublicKey   string `json:"public_key"`
	HumanWallet string `json:"human_wallet"`
	URL         string `json:"url,omitempty"`
}

// errCoordinatorFremderMensch: der Schluessel ist schon fuer einen anderen
// Menschen eingetragen.
var errCoordinatorFremderMensch = errors.New("this coordinator key is already registered to another human")

// RegisterCoordinatorKey traegt einen Coordinator ein. Die Unterschriften
// werden in ihrer einen Schreibweise gespeichert (kanonischeSignaturVersuch,
// ed25519SigNormal) -- so gehen sie in die Bescheinigung, und so prueft sie
// jeder Knoten.
//
// Ein eingetragener Schluessel wandert nicht zu einem anderen Menschen
// (Sicherheitspruefung #300, LOW-2): sonst kippte, wer zuletzt eintraegt, die
// Bindung eines Schluessels, dessen Besitzer fuer zwei Menschen unterschrieben
// hat. Neu eintragen fuer DENSELBEN Menschen (frische Unterschriften, neue
// Adresse) bleibt moeglich.
func (cs *ChainState) RegisterCoordinatorKey(publicKey, humanWallet, url, humanSig, keySig string) error {
	if cs.db == nil {
		return fmt.Errorf("no database")
	}
	publicKey = strings.ToLower(strings.TrimSpace(publicKey))
	humanWallet = strings.ToLower(strings.TrimSpace(humanWallet))
	url = strings.TrimRight(strings.TrimSpace(url), "/")
	humanSig, keySig = kanonischeSignaturVersuch(humanSig), ed25519SigNormal(keySig)
	if !kanonischeSignatur(humanSig) {
		return fmt.Errorf("human_signature is not in canonical form (0x + 130 hex, v 27/28, low s)")
	}
	if !cs.IsHuman(humanWallet) {
		return fmt.Errorf("human_wallet %s is not a registered human", humanWallet)
	}
	cs.EnsureCoordinatorRegistry()
	res, err := cs.db.Exec(
		`INSERT INTO coordinator_keys (public_key, human_wallet, url, human_signature, key_signature)
		 VALUES ($1, $2, NULLIF($3, ''), $4, $5)
		 ON CONFLICT (public_key) DO UPDATE SET
		   url = COALESCE(NULLIF($3, ''), coordinator_keys.url),
		   human_signature = $4,
		   key_signature = $5,
		   registered_at = NOW()
		 WHERE coordinator_keys.human_wallet = $2`,
		publicKey, humanWallet, url, humanSig, keySig)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return errCoordinatorFremderMensch
	}
	return nil
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
	// CORS: diese Nutzlast weist sich mit zwei Signaturen aus, nicht mit ihrer
	// Herkunft. Wer sie absendet, ist deshalb gleichgueltig -- geprueft wird,
	// was drinsteht.
	writeJSONCORS(w)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Aequitas-Forwarded")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
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
		// Alternative zu public_key + key_signature: die Adresse des eigenen
		// Coordinators. Der Knoten holt den Besitznachweis dann selbst dort ab
		// -- siehe coordinator_selfservice.go. Ohne das braucht der letzte
		// Schritt einen Zugang zur Maschine, und daran ist er wiederholt
		// gescheitert, ohne dass es jemandem auffiel.
		CoordinatorURL string `json:"coordinator_url"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	human := strings.ToLower(strings.TrimSpace(req.HumanWallet))

	// Kam nur die Adresse, den zweiten Nachweis dort abholen. Beide Werte sind
	// oeffentlich, und geprueft werden sie unveraendert weiter unten -- der
	// Knoten glaubt dem Coordinator nichts, er rechnet nach.
	if strings.TrimSpace(req.CoordinatorURL) != "" && req.PublicKey == "" {
		p, sig, err := holeBesitznachweis(req.CoordinatorURL, human)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		req.PublicKey, req.KeySignature = p, sig
		if req.URL == "" {
			req.URL = strings.TrimRight(strings.TrimSpace(req.CoordinatorURL), "/")
		}
	}

	pub := strings.ToLower(strings.TrimSpace(req.PublicKey))
	// Die eine Schreibweise jeder Unterschrift (Wallets liefern v 0/1 oder
	// Grossbuchstaben) -- so wird gespeichert und weitergereicht.
	req.HumanSignature = kanonischeSignaturVersuch(req.HumanSignature)
	req.KeySignature = ed25519SigNormal(req.KeySignature)
	if len(pub) != 64 || strings.Trim(pub, "0123456789abcdef") != "" {
		jsonError(w, "public_key must be 64 hex characters (Ed25519)", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(human, "0x") || len(human) != 42 {
		jsonError(w, "invalid human_wallet", http.StatusBadRequest)
		return
	}
	if err := verifyPersonalSign(coordinatorFreigabeNachricht(pub), req.HumanSignature, human); err != nil {
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
	if err := a.state.RegisterCoordinatorKey(pub, human, url, req.HumanSignature, req.KeySignature); err != nil {
		if errors.Is(err, errCoordinatorFremderMensch) {
			jsonError(w, err.Error(), http.StatusConflict)
			return
		}
		jsonStateError(w, "register-coordinator-key", pub, err)
		return
	}
	fmt.Printf("[COORDINATOR] Registered %s for human %s\n", pub[:16], human)

	antwort := map[string]interface{}{
		"success": true, "public_key": pub, "human_wallet": human, "url": url,
	}
	// Das Register ist knotenlokal. Aus einem Browser sind die anderen Knoten
	// nicht erreichbar -- sie sprechen http://, die Seite laeuft unter
	// https://, und der Browser verweigert die Mischung. Also reicht dieser
	// Knoten weiter; die Nutzlast traegt ihre beiden Nachweise mit, und jeder
	// prueft sie selbst, bevor er schreibt. Weiterreichen verschiebt damit
	// kein Vertrauen -- es erspart nur Arbeit.
	//
	// Ein weitergereichter Aufruf reicht nicht noch einmal weiter, sonst
	// liefen zwei Knoten im Kreis.
	if r.Header.Get("X-Aequitas-Forwarded") == "" {
		nutzlast, _ := json.Marshal(map[string]string{
			"public_key":      pub,
			"human_wallet":    human,
			"human_signature": req.HumanSignature,
			"key_signature":   req.KeySignature,
			"url":             url,
		})
		if weiter := a.reicheEintragungWeiter(nutzlast); len(weiter) > 0 {
			antwort["forwarded_to"] = weiter
		}
	}
	json.NewEncoder(w).Encode(antwort)
}

// handleCoordinatorList gibt die anerkannten Coordinatoren aus.
//
// Ohne Token: es sind oeffentliche Schluessel und oeffentliche Adressen. Genau
// darin liegt der Zweck -- jeder Validator und jeder Proof-Server soll die
// Liste lesen koennen, ohne dafuer ein Geheimnis zu brauchen.
func (a *APIServer) handleCoordinatorList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w)
	// Leeres Register als [] statt null: der Vergleichsdienst iteriert ueber
	// die Liste und brach bei null mit TypeError ab ("Coordinator-Register
	// nicht lesbar") -- so geschehen am 26.09.2026 auf dem frisch
	// aufgesetzten C1, dessen Register noch leer war.
	liste := a.state.Coordinators()
	if liste == nil {
		liste = []CoordinatorEntry{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"coordinators": liste,
	})
}

// verifyCoordinatorPossession: der Schluessel hat fuer diesen Menschen
// unterschrieben. Streng (ed25519_streng.go): kein Schluessel kleiner
// Ordnung, Unterschrift in ihrer einen Schreibweise -- dieselbe Pruefung
// macht jeder Knoten an der Bescheinigung.
func verifyCoordinatorPossession(publicHex, signatureHex, humanWallet string) bool {
	msg := []byte("Aequitas: coordinator key for human " + strings.ToLower(strings.TrimSpace(humanWallet)))
	return ed25519PruefenStreng(strings.ToLower(strings.TrimSpace(publicHex)), signatureHex, msg)
}
