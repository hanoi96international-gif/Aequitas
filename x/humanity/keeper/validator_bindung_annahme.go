package keeper

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
)

// VALIDATOR-REGISTER, SCHRITT 2: AUSSENDEN (05.10.2026, schlafend).
//
// Schritt 1 (validator_register.go) hat die Kettentransaktion
// validator_bindung gebaut, die jeder Knoten selbst prueft. Hier entsteht sie:
//
//  1. /api/validator-selfproof liefert ab dem Stichtag zusaetzlich den Satz
//     der Bindung mit Zeitpunkt JETZT und der Unterschrift dieses Knotens
//     (Signierschluessel) -- nur fuer den eigenen Betreiber
//     (NODE_OPERATOR_WALLET), wie schon der bisherige Nachweis (Audit
//     2026-09-29, H1).
//  2. Die Wallet des Betreibers unterschreibt denselben Satz (/node-binding).
//  3. POST /api/validator-bindung legt die Bindung beim Leiter in den
//     Ausgang: das Register gehoert keinem Konto, also nimmt nur EIN Knoten
//     an (annahmeBeginnenLeiter) -- so liegen alle Bindungen in einer Linie
//     von Bloecken, und die Faelle, in denen die Reihenfolge der
//     Geschwisterbloecke zaehlt (validator_register.go, REIHENFOLGE), treten
//     bei ehrlichem Betrieb nicht auf. Ohne rotierenden Leiter nimmt nur ein
//     Knoten an, der AUSDRUECKLICH als einziger Annehmender eingestellt ist
//     (ANNAHME_ROLLE=annehmend) -- per Vorgabe nehmen sonst alle Knoten an,
//     und genau das soll hier nicht passieren (Sicherheitspruefung #298).
//
// Die Annahme prueft dasselbe wie jeder Nachspielende (Form, beide
// Unterschriften, Betreiber ist Mensch, neuer als die bisherige Bindung) und
// wendet die Bindung in derselben Transaktion an wie den Ausgang. Strenger als
// das Nachspielen ist nur der Zeitpunkt: hoechstens zehn Minuten alt
// (validatorBindungAnnahmeFrist). Das Nachspielen nimmt eine Stunde -- die
// uebrigen 50 Minuten sind der Spielraum bis zum Block. Liegt eine Bindung
// laenger im Ausgang (Absturz), traegt der naechste Block die Zeit ihrer
// Annahme (block_tauglich.go, blockZeitFuer); weggelassen wird sie nie, sonst
// wiche das Register des Leiters ab.
//
// Grenzen (Sicherheitspruefung #298): hoechstens validatorBindungGleichzeitig
// Anfragen zugleich; je IP ein Fehlversuch je 30 s, gezaehlt auf dem Knoten,
// den der Mensch erreicht (vor der Weiterleitung -- beim Leiter kommt sonst
// jede weitergeleitete Anfrage mit der IP des Folgers an, und ein einziger
// Fehlversuch sperrte alle Betreiber hinter diesem Folger); je Betreiber eine
// angenommene Bindung je 30 s. Vor der Schreibsperre prueft eine Vorpruefung
// ohne Sperre, ob der Betreiber Mensch und die Bindung neuer ist -- sonst
// hielte jede Bindung eines Fremden die globale Sperre fuer mehrere
// Datenbankrunden.
//
// Vor dem Stichtag lehnen Selbstnachweis und Endpunkt ab: nichts aendert sich.

// validatorBindungAnnahmeFrist: so alt darf der Zeitpunkt einer Bindung bei
// der Annahme hoechstens sein.
const validatorBindungAnnahmeFrist int64 = 10 * 60

// errValidatorRegisterSchlaeft: vor dem Stichtag.
var errValidatorRegisterSchlaeft = errors.New("validator register is not active yet")

// errKeinAlleinigerAnnehmer: ohne rotierenden Leiter und ohne
// ANNAHME_ROLLE=annehmend nimmt dieser Knoten keine Bindung an.
var errKeinAlleinigerAnnehmer = errors.New("validator bindings are accepted only by the rotating leader or by the one node set to " +
	annahmeRolleEnv + "=annehmend")

const (
	// validatorBindungGleichzeitig: so viele Bindungsanfragen zugleich.
	validatorBindungGleichzeitig = 4
	// validatorBindungSperre: Fehlversuch je IP, angenommene Bindung je Betreiber.
	validatorBindungSperre = 30 * time.Second
)

var validatorBindungLaufend atomic.Int64

// annahmeBeginnenLeiter: wie annahmeBeginnen, fuer Zustand, der keinem Konto
// gehoert -- nur der Leiter nimmt an, oder ohne Leitung der eine Knoten, der
// ausdruecklich als Annehmender eingestellt ist. Mit annahmeEnde abschliessen.
func (cs *ChainState) annahmeBeginnenLeiter() error {
	if cs.leitung.Load() == nil && !cs.annehmendAusdruecklich.Load() {
		abgelehnteUeberweisungen.Add(1)
		return errKeinAlleinigerAnnehmer
	}
	cs.annahmenLaufend.Add(1)
	if err := cs.pruefeAnnahmeTorFuer(); err != nil {
		cs.annahmenLaufend.Add(-1)
		return err
	}
	// annahme_pause.go: wie jede Annahme, die in den Ausgang schreibt.
	if err := cs.annahmePausiert(); err != nil {
		cs.annahmenLaufend.Add(-1)
		return err
	}
	return nil
}

// kanonischeSignaturVersuch: Kleinschreibung und v 0/1 -> 27/28 -- beides
// aendert nicht, wer unterschrieben hat, und manche Wallets liefern es so.
// Alles andere (hohes s, falsche Laenge) bleibt, wie es ist, und faellt in
// der Formpruefung durch.
func kanonischeSignaturVersuch(sig string) string {
	sig = strings.ToLower(strings.TrimSpace(sig))
	if len(sig) == 132 && strings.HasPrefix(sig, "0x") {
		switch sig[130:] {
		case "00":
			sig = sig[:130] + "1b"
		case "01":
			sig = sig[:130] + "1c"
		}
	}
	return sig
}

// bindungVorpruefen: ohne Schreibsperre -- ist der Betreiber Mensch, und ist
// die Bindung neuer als seine bisherige? Nur ein Vorfilter: die verbindliche
// Pruefung macht applyValidatorBindungLocked in der Transaktion. Ist etwas
// nicht lesbar, entscheidet sie.
func (cs *ChainState) bindungVorpruefen(betreiber, signing string, zeit int64) error {
	cs.mu.RLock()
	acc, bekannt := cs.accounts.Get(betreiber)
	mensch := bekannt && acc.IsHuman
	cs.mu.RUnlock()
	if !bekannt {
		var h bool
		switch err := cs.db.QueryRow(`SELECT is_human FROM chain_accounts WHERE lower(address) = $1`, betreiber).Scan(&h); {
		case errors.Is(err, sql.ErrNoRows):
			return validatorZustand("Betreiber %s ist kein registrierter Mensch", kurzAdresse(betreiber))
		case err != nil:
			return nil
		}
		mensch = h
	}
	if !mensch {
		return validatorZustand("Betreiber %s ist kein registrierter Mensch", kurzAdresse(betreiber))
	}
	var bisherSigning string
	var bisherZeit int64
	if err := cs.db.QueryRow(`SELECT signing_address, bindung_ts FROM validator_register WHERE operator_wallet = $1`,
		betreiber).Scan(&bisherSigning, &bisherZeit); err == nil && !validatorNeuer(zeit, signing, bisherZeit, bisherSigning) {
		return validatorZustand("%s hat schon eine Bindung von %d -- diese (%d) ist nicht neuer", kurzAdresse(betreiber), bisherZeit, zeit)
	}
	return nil
}

// ValidatorBinden nimmt eine Bindung an: prueft sie wie jeder Nachspielende,
// wendet sie an und legt sie in den Ausgang -- in einer Transaktion.
func (cs *ChainState) ValidatorBinden(tx Transaction) error {
	jetzt := nowUnix()
	if !validatorRegisterAktiv(jetzt) {
		return errValidatorRegisterSchlaeft
	}
	// Ein frischer Nachweis mit genau den drei Feldern der Bindung: nichts,
	// was der Aufrufer sonst noch mitgibt, kommt in Ausgang und Block.
	var n *Auftragsnachweis
	if tx.Nachweis != nil {
		n = &Auftragsnachweis{Zeit: tx.Nachweis.Zeit,
			Sig: kanonischeSignaturVersuch(tx.Nachweis.Sig), Sig2: kanonischeSignaturVersuch(tx.Nachweis.Sig2)}
	}
	tx = Transaction{Type: "validator_bindung", Wallet: strings.ToLower(strings.TrimSpace(tx.Wallet)),
		To: strings.ToLower(strings.TrimSpace(tx.To)), Nachweis: n}
	if err := validatorBindungForm(&tx); err != nil {
		return fmt.Errorf("%w: %v", ErrUeberweisungNichtSigniert, err)
	}
	if z := tx.Nachweis.Zeit; z < jetzt-validatorBindungAnnahmeFrist || z > jetzt+nachweisHoechstensVoraus {
		return fmt.Errorf("%w: Bindung unterschrieben um %d, jetzt %d (hoechstens %d s alt)",
			ErrUeberweisungNichtSigniert, z, jetzt, validatorBindungAnnahmeFrist)
	}
	if err := pruefeAuftragsNachweis(&tx, jetzt); err != nil {
		return err
	}
	if err := cs.annahmeBeginnenLeiter(); err != nil {
		return err
	}
	defer cs.annahmeEnde()
	if cs.db == nil {
		return fmt.Errorf("validator binding requires a database")
	}
	if err := cs.bindungVorpruefen(tx.Wallet, tx.To, tx.Nachweis.Zeit); err != nil {
		return err
	}
	if err := cs.runAtomicWithOutbox([]string{tx.Wallet}, false, func(ctx context.Context) (Transaction, error) {
		if err := cs.applyValidatorBindungLocked(ctx, &tx, jetzt); err != nil {
			return Transaction{}, err
		}
		fmt.Printf("[VALIDATOR] ✓ Bindung %s -> %s angenommen (ts %d)\n", kurzAdresse(tx.To), kurzAdresse(tx.Wallet), tx.Nachweis.Zeit)
		return tx, nil
	}); err != nil {
		return err
	}
	// Der Stand der Erzeuger (validator_register_leser.go) -- nach dem Commit,
	// ohne Sperre, und nur wenn der Verlauf gewachsen ist.
	if cs.registerGeaendert.Swap(false) {
		cs.erzeugerRegisterAuffrischen()
	}
	return nil
}

// knotenBindungsNachweis: der Teil des Selbstnachweises, den nur der Knoten
// liefern kann -- seine Unterschrift unter die Bindung an betreiber, jetzt.
// Leer vor dem Stichtag.
func knotenBindungsNachweis(signingAddr, betreiber string, jetzt int64, sign func([]byte) ([]byte, error)) map[string]interface{} {
	if !validatorRegisterAktiv(jetzt) {
		return nil
	}
	msg := validatorBindungNachricht(signingAddr, betreiber, jetzt)
	sig, err := sign(accounts.TextHash([]byte(msg)))
	if err != nil {
		return nil
	}
	sig[64] += 27
	return map[string]interface{}{
		"bindung_zeit":            jetzt,
		"bindung_nachricht":       msg,
		"bindung_signatur_knoten": "0x" + fmt.Sprintf("%x", sig),
	}
}

// handleValidatorBindung POST /api/validator-bindung
// Body: {"operator":"0x..","signing":"0x..","ts":<unix>,
//
//	"operator_signature":"0x..","signing_signature":"0x.."}
//
// Beide unterschreiben validatorBindungNachricht(signing, operator, ts).
func (a *APIServer) handleValidatorBindung(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if !validatorRegisterAktiv(nowUnix()) {
		jsonError(w, errValidatorRegisterSchlaeft.Error(), http.StatusConflict)
		return
	}
	if validatorBindungLaufend.Add(1) > validatorBindungGleichzeitig {
		validatorBindungLaufend.Add(-1)
		jsonError(w, "busy, try again shortly", http.StatusServiceUnavailable)
		return
	}
	defer validatorBindungLaufend.Add(-1)
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var req struct {
		Operator          string `json:"operator"`
		Signing           string `json:"signing"`
		Zeit              int64  `json:"ts"`
		OperatorSignature string `json:"operator_signature"`
		SigningSignature  string `json:"signing_signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	betreiber := strings.ToLower(strings.TrimSpace(req.Operator))
	// Je Betreiber eine angenommene Bindung je 30 s. Gesetzt wird der Platz
	// erst nach der Annahme -- die braucht die Unterschrift des Betreibers,
	// also kann kein Fremder ihn verbrauchen. Nur ein Vorfilter: parallele
	// Bindungen desselben Betreibers sehen alle den leeren Platz; verbindlich
	// ist der Tagesabstand im Konsens (applyValidatorBindungLocked, unter
	// cs.mu).
	if ts, ok := betreiberRateLimit.Load("validator-bindung-betreiber:" + betreiber); ok && time.Since(ts.(time.Time)) < validatorBindungSperre {
		jsonError(w, "this operator bound a key moments ago -- try again shortly", http.StatusTooManyRequests)
		return
	}
	tx := Transaction{Wallet: req.Operator, To: req.Signing,
		Nachweis: &Auftragsnachweis{Zeit: req.Zeit, Sig: req.OperatorSignature, Sig2: req.SigningSignature}}
	if err := a.state.ValidatorBinden(tx); err != nil {
		switch {
		case errors.Is(err, ErrUeberweisungNichtSigniert):
			jsonError(w, "invalid binding: "+err.Error(), http.StatusBadRequest)
		case istZustandsAblehnung(err), errors.Is(err, errValidatorRegisterSchlaeft):
			jsonError(w, err.Error(), http.StatusConflict)
		case istWiederholbareAnnahmeAblehnung(err), errors.Is(err, errKeinAlleinigerAnnehmer):
			jsonError(w, err.Error(), http.StatusServiceUnavailable)
		default:
			jsonStateError(w, "validator-bindung", strings.ToLower(req.Operator), err)
		}
		return
	}
	betreiberRateLimit.Store("validator-bindung-betreiber:"+betreiber, time.Now())
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true, "operator": betreiber, "signing": strings.ToLower(req.Signing), "ts": req.Zeit,
		// Angenommen heisst: im Ausgang des Leiters. Auf der Kette steht die
		// Bindung mit dem naechsten Block.
		"status": "accepted, will be in the next block",
	})
}

// bindungsGrenze: Fehlversuche je IP, gezaehlt auf dem Knoten, den der Mensch
// erreicht -- VOR der Weiterleitung zum Leiter. Erfolgreiche Anfragen
// verbrauchen nichts. Anfragen von Validatoren (weitergeleitet,
// rpcRateLimitFrei ueber die TCP-Adresse, nicht faelschbar) zaehlen beim
// Leiter nicht: der weiterleitende Knoten hat seine Grenze schon angewandt.
func (a *APIServer) bindungsGrenze(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || rpcRateLimitFrei(r) {
			next(w, r)
			return
		}
		schluessel := "validator-bindung-fehl:" + clientIP(r)
		if ts, ok := bindungRateLimit.Load(schluessel); ok && time.Since(ts.(time.Time)) < validatorBindungSperre {
			writeJSONCORS(w)
			jsonError(w, "rate limited after a rejected binding, try again shortly", http.StatusTooManyRequests)
			return
		}
		rec := &statusMerker{ResponseWriter: w, status: http.StatusOK}
		next(rec, r)
		if rec.status == http.StatusBadRequest || rec.status == http.StatusConflict {
			bindungRateLimit.Store(schluessel, time.Now())
		}
	}
}

// statusMerker: merkt sich den Status einer Antwort.
type statusMerker struct {
	http.ResponseWriter
	status int
}

func (s *statusMerker) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
