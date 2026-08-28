package keeper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Eine Coordinator-Eintragung ohne Kommandozeile.
//
// WAS HIER SCHIEFGING
//
// Die Eintragung verlangt zwei Nachweise: die Wallet des Menschen
// unterschreibt, dass sie hinter dem Schluessel steht, und der Schluessel
// unterschreibt, dass er dazugehoert. Den zweiten kann nur die Maschine
// erzeugen, auf der der Schluessel liegt -- der letzte Schritt war damit ein
// SSH-Aufruf.
//
// Am 27.08.2026 ist genau dieser Schritt DREIMAL gescheitert, ohne dass es
// auffiel: unterschrieben war, eingetragen nicht. Die Seite gab eine
// Befehlszeile aus, und wer sie nicht ausfuehrte, hatte einen Coordinator, den
// niemand anerkennt -- bei voellig unauffaelliger Oberflaeche.
//
// Ein Weg, der eine Kommandozeile verlangt, ist fuer die meisten Menschen kein
// Weg. Seither holt der Knoten den zweiten Nachweis selbst beim Coordinator ab
// (GET /besitznachweis), und die Seite kommt mit dem aus, was ein Browser
// kann: eine Adresse und eine Unterschrift.
//
// WARUM DAS NICHTS AUFWEICHT
//
// Beide Werte, die der Knoten dort abholt, sind oeffentlich: ein oeffentlicher
// Schluessel und eine Signatur ueber einen festen Satz. Geprueft wird
// unveraendert beides, an derselben Stelle wie zuvor. Der Knoten glaubt dem
// Coordinator nichts -- er rechnet nach.
//
// Der Satz ist an die WALLET gebunden. Ein anderswo abgeholter Nachweis passt
// zu keiner anderen Adresse, laesst sich also nicht einsammeln und unter
// fremdem Namen einreichen.
//
// WARUM DER KNOTEN WEITERREICHT
//
// Das Register ist knotenlokal. Aus dem Browser sind die anderen Knoten nicht
// erreichbar -- sie sprechen http://, die Seite laeuft unter https://, und der
// Browser verweigert die Mischung. Also reicht der empfangende Knoten dieselbe
// Nutzlast weiter.
//
// Das verschiebt kein Vertrauen: die Nutzlast traegt ihre beiden Signaturen
// mit sich, und jeder Knoten prueft sie selbst, bevor er etwas schreibt. Ein
// Weiterreichender kann nichts hinzufuegen und nichts faelschen -- er kann nur
// Arbeit ersparen. Und weil jeder Knoten das tut, entsteht dabei auch keine
// Stelle, ueber die alles laufen muss.

// coordinatorProofURL baut die Abfrage-URL und prueft sie.
//
// isAllowedPeerURL laesst nur oeffentliche Adressen zu -- ohne das koennte
// jemand diesen Knoten als Abrufwerkzeug fuer sein internes Netz benutzen.
func coordinatorProofURL(basis, wallet string) (string, error) {
	basis = strings.TrimRight(strings.TrimSpace(basis), "/")
	if basis == "" {
		return "", fmt.Errorf("coordinator_url is empty")
	}
	if !isAllowedPeerURL(basis) {
		return "", fmt.Errorf("coordinator_url must be a public https:// address")
	}
	return basis + "/besitznachweis?wallet=" + url.QueryEscape(strings.ToLower(wallet)), nil
}

// holeBesitznachweis fragt den Coordinator nach seinem eigenen Nachweis.
func holeBesitznachweis(basis, wallet string) (pub string, sig string, err error) {
	abfrage, err := coordinatorProofURL(basis, wallet)
	if err != nil {
		return "", "", err
	}
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Get(abfrage)
	if err != nil {
		return "", "", fmt.Errorf("coordinator not reachable at %s: %w", basis, err)
	}
	defer resp.Body.Close()
	// Klein halten: hier kommt ein winziges JSON, kein Datenstrom.
	roh, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if err != nil {
		return "", "", fmt.Errorf("could not read the coordinator's answer: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("the coordinator refused (HTTP %d) -- is COORDINATOR_SIGNING_KEY set there?",
			resp.StatusCode)
	}
	var b struct {
		PublicKey    string `json:"public_key"`
		KeySignature string `json:"key_signature"`
	}
	if err := json.Unmarshal(roh, &b); err != nil {
		return "", "", fmt.Errorf("the coordinator's answer was not the expected JSON")
	}
	if b.PublicKey == "" || b.KeySignature == "" {
		return "", "", fmt.Errorf("the coordinator returned no proof -- is COORDINATOR_SIGNING_KEY set there?")
	}
	return b.PublicKey, b.KeySignature, nil
}

// reicheEintragungWeiter schickt dieselbe Nutzlast an die bekannten Knoten.
//
// Ergebnisse werden gesammelt, nicht erzwungen: ein Knoten, der gerade nicht
// antwortet, darf die Eintragung hier nicht scheitern lassen. Der Aufrufer
// bekommt zu sehen, wo sie ankam und wo nicht -- das ist die umkehrbare
// Richtung, und wer will, sendet spaeter erneut.
func (a *APIServer) reicheEintragungWeiter(nutzlast []byte) []map[string]interface{} {
	var aus []map[string]interface{}
	client := &http.Client{Timeout: 12 * time.Second}
	for _, p := range GlobalPeerRegistry.ActivePeers(os.Getenv("SELF_URL")) {
		eintrag := map[string]interface{}{"node": p}
		req, err := http.NewRequest(http.MethodPost,
			strings.TrimRight(p, "/")+"/api/register-coordinator-key", strings.NewReader(string(nutzlast)))
		if err != nil {
			eintrag["error"] = err.Error()
			aus = append(aus, eintrag)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		// Bremse gegen Kreise: ein weitergereichter Aufruf reicht nicht weiter.
		req.Header.Set("X-Aequitas-Forwarded", "1")
		resp, err := client.Do(req)
		if err != nil {
			eintrag["error"] = err.Error()
			aus = append(aus, eintrag)
			continue
		}
		roh, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		resp.Body.Close()
		eintrag["status"] = resp.StatusCode
		eintrag["ok"] = resp.StatusCode == http.StatusOK
		if resp.StatusCode != http.StatusOK {
			eintrag["answer"] = strings.TrimSpace(string(roh))
		}
		aus = append(aus, eintrag)
	}
	return aus
}

// handleCoordinatorProof holt den Besitznachweis beim Coordinator ab und gibt
// ihn weiter.
//
// # WARUM DIESER UMWEG
//
// Die Seite braucht den oeffentlichen Schluessel, BEVOR unterschrieben werden
// kann -- er steht in dem Satz, der unterschrieben wird. Sie koennte ihn beim
// Coordinator selbst holen, aber dessen Adresse ist eine andere Herkunft, und
// die Seite laeuft unter einer strengen CSP (default-src 'self'). Sie dafuer
// zu lockern hiesse, jeder Seite dieses Ursprungs beliebige Verbindungen zu
// erlauben -- viel zu breit fuer einen einzigen Abruf.
//
// Also holt der Knoten ihn. Fuer die Seite ist das gleiche Herkunft, und die
// CSP bleibt, wie sie ist.
//
// Es gibt hier nichts zu schuetzen: beide Werte sind oeffentlich, und der
// Aufruf schreibt nichts. Die Adresse geht durch isAllowedPeerURL, damit
// dieser Knoten nicht als Abrufwerkzeug fuer fremde interne Netze dient.
func (a *APIServer) handleCoordinatorProof(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	basis := strings.TrimSpace(r.URL.Query().Get("url"))
	wallet := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("wallet")))
	if !strings.HasPrefix(wallet, "0x") || len(wallet) != 42 {
		jsonError(w, "wallet must be a 0x address", http.StatusBadRequest)
		return
	}
	pub, sig, err := holeBesitznachweis(basis, wallet)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"public_key":    pub,
		"key_signature": sig,
		"human_wallet":  wallet,
		// Genau der Satz, der unterschrieben werden muss. Ihn hier
		// mitzugeben, nimmt der Seite die Gelegenheit, ihn falsch
		// zusammenzusetzen -- ein Leerzeichen daneben, und die Kette lehnt ab.
		"message": "Aequitas: authorize coordinator " + pub,
	})
}
