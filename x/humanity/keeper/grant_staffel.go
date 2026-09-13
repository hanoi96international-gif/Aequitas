package keeper

// Gestaffelter Zuschuss (WP 2 aus docs/LEBENDIGKEIT_GEGEN_DEEPFAKES.md).
//
// WAS. Eine Registrierung, die der Coordinator als "gelb" einstuft
// (Lebendigkeit unsicher oder Herkunft auffaellig), bekommt die 1.000 AEQ
// nicht auf einmal: 200 sofort, 800 als Staffel ueber 30 Tage -- und die
// Staffel laeuft erst, wenn eine zweite Lebendigkeitspruefung (Transaktion
// liveness_renewal) bestanden ist. Bleibt sie aus, pausiert die Staffel; sie
// laeuft weiter, sobald sie kommt. Niemand verliert etwas ausser Zeit. Eine
// Deepfake-Farm dagegen muesste jede Kunstfigur zweimal durch eine zufaellige
// Pruefung bringen, um an den Grossteil des Zuschusses zu kommen.
//
// GELDMENGE. Die vollen 1.000 AEQ entstehen bei der Registrierung -- 200 im
// Guthaben, 800 in GrantStagedRest. Die Summe aus beiden ist der Zuschuss;
// TotalSupply = Menschen x 1.000 bleibt wahr (supply_measured.go zaehlt
// GrantStagedRest mit, siehe dort). Der Rest ist nicht ausgebbar, nicht
// uebertragbar und unterliegt keiner Demurrage: er ist ein Anspruch, kein
// Guthaben.
//
// SCHLAFEND. Nichts davon wirkt vor stagedGrantActivationUnix. Bis dahin wird
// grant_class ignoriert (voller Zuschuss wie immer), liveness_renewal und
// grant_release sind Leerlauf. Die Konstante steht auf 2100 -- WP 4 setzt das
// echte Datum, wenn die Schwellen aus echten Aufnahmen stehen (mindestens 20
// echte Registrierungen). Ein Datum im Code und nicht in einer
// Umgebungsvariablen, weil alle Knoten dieselbe Antwort geben muessen: ein
// Knoten mit anderer Einstellung liefe auseinander.
//
// DETERMINISMUS. Der Produzent entscheidet (Klasse aus der Herkunftsnotiz des
// eigenen /api/prove, Freigaben im Tagesdurchlauf nach Adressreihenfolge aus
// der Datenbank), die Bloecke tragen die Betraege, das Nachspielen wendet sie
// an -- dasselbe Modell wie ubi_distribution. Alle drei Felder stehen im
// accountLeaf (nur wenn sie ungleich null sind, damit bestehende Konten ihren
// Blattwert behalten) und im Snapshot (omitempty, aus demselben Grund).

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// 2100-01-01T00:00:00Z. WP 4 ersetzt das durch das echte Datum.
	stagedGrantActivationUnix int64 = 4102444800

	grantKlasseSofort     = "sofort"
	grantKlasseGestaffelt = "gestaffelt"

	grantSofortAnteil  = 200.0 // AEQ sofort im Guthaben
	grantStaffelAnteil = 800.0 // AEQ als Staffel
	grantStaffelTage   = 30
)

// stagedGrantActivationOverride: nur fuer Tests (0 = Konstante gilt).
var stagedGrantActivationOverride atomic.Int64

func stagedGrantAktiv(blockUnix int64) bool {
	a := stagedGrantActivationUnix
	if o := stagedGrantActivationOverride.Load(); o > 0 {
		a = o
	}
	return blockUnix >= a
}

// grantStaffelTagesrate ist der Betrag, den ein Tagesdurchlauf freigibt:
// 800 / 30, auf 6 Stellen gerundet (round6 ist die Rundung der ganzen
// Kette). 30 Tage x 26,666667 = 800,00001; der letzte Tag gibt nur den Rest.
func grantStaffelTagesrate() float64 {
	return round6(grantStaffelAnteil / grantStaffelTage)
}

// grantKlasseNormalisiert bildet alles, was nicht ausdruecklich "gestaffelt"
// ist, auf "sofort" ab -- ein Tippfehler darf niemanden schlechter stellen.
func grantKlasseNormalisiert(k string) string {
	if strings.EqualFold(strings.TrimSpace(k), grantKlasseGestaffelt) {
		return grantKlasseGestaffelt
	}
	return grantKlasseSofort
}

// grantBeiRegistrierung teilt den Zuschuss auf. Vor der Aktivierung: alles
// sofort, egal was die Klasse sagt.
func grantBeiRegistrierung(klasse string, blockUnix int64) (sofort, staffel float64) {
	if !stagedGrantAktiv(blockUnix) || grantKlasseNormalisiert(klasse) != grantKlasseGestaffelt {
		return registrationGrant, 0
	}
	return grantSofortAnteil, grantStaffelAnteil
}

// hatStaffel sagt, ob ein Konto einen offenen Staffelrest traegt.
func hatStaffel(acc *AccountState) bool {
	return acc != nil && acc.GrantStagedRest > 0
}

// applyLivenessRenewalDeltaLocked wendet eine liveness_renewal an: der
// Zeitpunkt wird gesetzt, die Staffel darf laufen. Idempotent -- ein zweites
// Nachspielen derselben Erneuerung aendert nichts. Vor der Aktivierung
// Leerlauf. Caller haelt cs.mu.
func (cs *ChainState) applyLivenessRenewalDeltaLocked(ctx context.Context, address string, blockUnix int64) error {
	if !stagedGrantAktiv(blockUnix) {
		return nil
	}
	address = strings.ToLower(strings.TrimSpace(address))
	cs.ensureAccountLoadedCtx(ctx, address)
	acc, ok := cs.accounts.Get(address)
	if !ok || !acc.IsHuman {
		return fmt.Errorf("liveness_renewal fuer %s: kein registrierter Mensch", address)
	}
	if acc.LivenessRenewedAt >= blockUnix {
		return nil // schon erneuert (Replay derselben Transaktion)
	}
	acc.LivenessRenewedAt = blockUnix
	return cs.saveAccountToDBCtx(ctx, acc)
}

// applyGrantReleaseDeltaLocked bewegt einen Freigabebetrag vom Staffelrest
// ins Guthaben. Der Betrag stammt aus der Transaktion (vom Produzenten
// berechnet), wird aber auf den vorhandenen Rest gedeckelt -- ein Block kann
// nie mehr freigeben, als das Konto traegt. Caller haelt cs.mu.
func (cs *ChainState) applyGrantReleaseDeltaLocked(ctx context.Context, address string, amount float64, blockUnix int64) error {
	if !stagedGrantAktiv(blockUnix) {
		return nil
	}
	address = strings.ToLower(strings.TrimSpace(address))
	if amount <= 0 {
		return fmt.Errorf("grant_release fuer %s: Betrag %.6f", address, amount)
	}
	cs.ensureAccountLoadedCtx(ctx, address)
	acc, ok := cs.accounts.Get(address)
	if !ok || !acc.IsHuman {
		return fmt.Errorf("grant_release fuer %s: kein registrierter Mensch", address)
	}
	if acc.GrantStagedRest <= 0 {
		return nil // nichts mehr offen -- Replay einer schon angewandten Freigabe
	}
	frei := NewDecimal(amount)
	if frei > acc.GrantStagedRest {
		frei = acc.GrantStagedRest
	}
	acc.GrantStagedRest -= frei
	acc.Balance = acc.Balance.Add(frei)
	if acc.GrantStagedRest == 0 {
		acc.GrantStagedUntil = 0
	}
	touchActivityAt(acc, blockUnix)
	if err := cs.enforceWealthCapLockedCtx(ctx, acc); err != nil {
		return err
	}
	return cs.saveAccountToDBCtx(ctx, acc)
}

// grantReleasesLocked berechnet die Freigaben eines Tagesdurchlaufs: jedes
// Konto mit offenem Rest UND erfolgter Erneuerung bekommt die Tagesrate,
// gedeckelt auf den Rest. Reihenfolge nach Adresse aus der Datenbank, damit
// jeder Knoten dieselben Transaktionen in derselben Reihenfolge sieht.
// Ohne Datenbank (Tests) aus dem Speicher, sortiert. Vor der Aktivierung
// leer. Caller haelt cs.mu.
func (cs *ChainState) grantReleasesLocked(ctx context.Context, ubiAt int64) ([]Transaction, error) {
	if !stagedGrantAktiv(ubiAt) {
		return nil, nil
	}
	var adressen []string
	if cs.db != nil {
		rows, err := cs.dbExecCtx(ctx).Query(
			`SELECT lower(address) FROM chain_accounts
			 WHERE is_human = true AND COALESCE(grant_staged_rest, 0) > 0 AND COALESCE(liveness_renewed_at, 0) > 0
			 ORDER BY lower(address)`)
		if err != nil {
			return nil, fmt.Errorf("grant releases: %w", err)
		}
		for rows.Next() {
			var a string
			if rows.Scan(&a) == nil {
				adressen = append(adressen, a)
			}
		}
		rows.Close()
	} else {
		cs.accounts.Range(func(a string, acc *AccountState) bool {
			if acc.IsHuman && acc.GrantStagedRest > 0 && acc.LivenessRenewedAt > 0 {
				adressen = append(adressen, strings.ToLower(a))
			}
			return true
		})
		sort.Strings(adressen)
	}
	rate := grantStaffelTagesrate()
	var txs []Transaction
	for _, a := range adressen {
		cs.ensureAccountLoadedCtx(ctx, a)
		acc, ok := cs.accounts.Get(a)
		if !ok || acc.GrantStagedRest <= 0 {
			continue
		}
		betrag := rate
		if rest := acc.GrantStagedRest.Float(); rest < betrag {
			betrag = round6(rest)
		}
		if betrag <= 0 {
			continue
		}
		if err := cs.applyGrantReleaseDeltaLocked(ctx, a, betrag, ubiAt); err != nil {
			return nil, err
		}
		txs = append(txs, Transaction{Type: "grant_release", Wallet: a, Amount: betrag})
	}
	return txs, nil
}

// StaffelStand fuer /api/balance und die App: was offen ist, ob die Staffel
// laeuft, wann erneuert wurde.
func StaffelStand(acc *AccountState) map[string]interface{} {
	if acc == nil || (acc.GrantStagedRest == 0 && acc.LivenessRenewedAt == 0) {
		return nil
	}
	return map[string]interface{}{
		"rest_aeq":      acc.GrantStagedRest.Float(),
		"laeuft":        acc.GrantStagedRest > 0 && acc.LivenessRenewedAt > 0,
		"erneuert_am":   acc.LivenessRenewedAt,
		"bis":           acc.GrantStagedUntil,
		"tagesrate_aeq": grantStaffelTagesrate(),
		"bedeutung":     "Teil des Startzuschusses, der erst nach einer zweiten Lebendigkeitspruefung ueber 30 Tage freigegeben wird. Kein Verlust: pausiert, bis die Pruefung kommt.",
	}
}

// ------------------------------------------------------------ Herkunft

// proveHerkunftKlasse haelt je Nullifier die Klasse fest, die der
// Proof-Server aus der Coordinator-Bescheinigung gelesen hat (grantClass in
// der /prove-Antwort). Wie proveHerkunft: kurzlebig, nur im Arbeitsspeicher.
var proveHerkunftKlasse sync.Map

// grantKlasseAusHerkunft liefert die Klasse fuer einen Nullifier ("" = keine
// Notiz = sofort).
func grantKlasseAusHerkunft(nullifier string) string {
	v, ok := proveHerkunftKlasse.Load(nullifierSchluessel(nullifier))
	if !ok {
		return ""
	}
	k, _ := v.(string)
	return k
}

// merkeProveKlasse liest grantClass aus einer /prove-Antwort.
func merkeProveKlasse(respBody []byte) {
	var b struct {
		ZKNullifier string `json:"zkNullifier"`
		GrantClass  string `json:"grantClass"`
	}
	if err := json.Unmarshal(respBody, &b); err != nil || b.ZKNullifier == "" || b.GrantClass == "" {
		return
	}
	proveHerkunftKlasse.Store(nullifierSchluessel(b.ZKNullifier), grantKlasseNormalisiert(b.GrantClass))
}

// ------------------------------------------------------------ API

// handleLivenessRenewal nimmt eine Erneuerung entgegen: der Coordinator hat
// eine zweite Lebendigkeitspruefung bestanden gesehen und das mit seinem
// Ed25519-Schluessel bescheinigt (aequitas-liveness-renewal-v1|<wallet>|<issued_at>).
// Der Schluessel muss im Coordinator-Register dieses Knotens stehen. Angenommen
// wird nur fuer Konten mit offener Staffel -- fuer alle anderen gibt es nichts
// zu erneuern. Vor der Aktivierung antwortet der Endpunkt 409.
func (a *APIServer) handleLivenessRenewal(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if !stagedGrantAktiv(time.Now().Unix()) {
		jsonError(w, "staged grant not active yet", http.StatusConflict)
		return
	}
	var req struct {
		Wallet    string `json:"wallet"`
		IssuedAt  int64  `json:"issued_at"`
		Signature string `json:"signature"`
		PublicKey string `json:"public_key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	wallet := strings.ToLower(strings.TrimSpace(req.Wallet))
	if len(wallet) != 42 || !strings.HasPrefix(wallet, "0x") {
		jsonError(w, "wallet must be a 0x address", http.StatusBadRequest)
		return
	}
	now := time.Now().Unix()
	if req.IssuedAt <= 0 || now-req.IssuedAt > 900 || req.IssuedAt-now > 60 {
		jsonError(w, "attestation expired or from the future", http.StatusBadRequest)
		return
	}
	if !livenessRenewalSignaturGueltig(a.state, wallet, req.IssuedAt, req.PublicKey, req.Signature) {
		jsonError(w, "renewal attestation not signed by a registered coordinator", http.StatusForbidden)
		return
	}
	a.state.mu.RLock()
	acc, ok := a.state.accounts.Get(wallet)
	offen := ok && acc.IsHuman && acc.GrantStagedRest > 0
	a.state.mu.RUnlock()
	if !offen {
		jsonError(w, "no staged grant on this wallet", http.StatusConflict)
		return
	}
	tx := Transaction{Type: "liveness_renewal", Wallet: wallet, DistributionAt: req.IssuedAt}
	if err := a.state.runAtomicWithOutbox([]string{wallet}, false, func(ctx context.Context) (Transaction, error) {
		if err := a.state.applyLivenessRenewalDeltaLocked(ctx, wallet, now); err != nil {
			return Transaction{}, err
		}
		return tx, nil
	}); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "wallet": wallet, "renewed_at": now})
}

const livenessRenewalDomain = "aequitas-liveness-renewal-v1"

// livenessRenewalSignaturGueltig prueft die Ed25519-Bescheinigung gegen das
// Coordinator-Register des Knotens.
func livenessRenewalSignaturGueltig(cs *ChainState, wallet string, issuedAt int64, publicHex, signatureHex string) bool {
	publicHex = strings.ToLower(strings.TrimSpace(publicHex))
	bekannt := false
	for _, c := range cs.Coordinators() {
		if strings.EqualFold(c.PublicKey, publicHex) {
			bekannt = true
			break
		}
	}
	if !bekannt {
		return false
	}
	msg := fmt.Sprintf("%s|%s|%d", livenessRenewalDomain, wallet, issuedAt)
	roh, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(signatureHex), "0x"))
	if err != nil || len(roh) != ed25519.SignatureSize {
		return false
	}
	pub, err := hex.DecodeString(publicHex)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), []byte(msg), roh)
}

// StaffelStandVon liest den Staffelstand eines Kontos unter der Lesesperre.
func (cs *ChainState) StaffelStandVon(address string) map[string]interface{} {
	address = strings.ToLower(strings.TrimSpace(address))
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	acc, ok := cs.accounts.Get(address)
	if !ok {
		return nil
	}
	return StaffelStand(acc)
}
