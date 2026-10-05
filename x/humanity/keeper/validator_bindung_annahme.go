package keeper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
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
//     bei ehrlichem Betrieb nicht auf.
//
// Die Annahme prueft dasselbe wie jeder Nachspielende (Form, beide
// Unterschriften, Betreiber ist Mensch, neuer als die bisherige Bindung) und
// wendet die Bindung in derselben Transaktion an wie den Ausgang. Strenger als
// das Nachspielen ist nur der Zeitpunkt: hoechstens zehn Minuten alt
// (validatorBindungAnnahmeFrist). Das Nachspielen nimmt eine Stunde -- die
// uebrigen 50 Minuten sind der Spielraum bis zum Block; was laenger im Ausgang
// liegt, laesst der Erzeuger ohnehin weg (block_tauglich.go).
//
// Vor dem Stichtag lehnen Selbstnachweis und Endpunkt ab: nichts aendert sich.

// validatorBindungAnnahmeFrist: so alt darf der Zeitpunkt einer Bindung bei
// der Annahme hoechstens sein.
const validatorBindungAnnahmeFrist int64 = 10 * 60

// errValidatorRegisterSchlaeft: vor dem Stichtag.
var errValidatorRegisterSchlaeft = errors.New("validator register is not active yet")

// annahmeBeginnenLeiter: wie annahmeBeginnen, fuer Zustand, der keinem Konto
// gehoert -- nur der Leiter nimmt an. Mit annahmeEnde abschliessen.
func (cs *ChainState) annahmeBeginnenLeiter() error {
	cs.annahmenLaufend.Add(1)
	if err := cs.pruefeAnnahmeTorFuer(); err != nil {
		cs.annahmenLaufend.Add(-1)
		return err
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
	tx = Transaction{Type: "validator_bindung", Wallet: strings.ToLower(strings.TrimSpace(tx.Wallet)),
		To: strings.ToLower(strings.TrimSpace(tx.To)), Nachweis: tx.Nachweis}
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
	return cs.runAtomicWithOutbox([]string{tx.Wallet}, false, func(ctx context.Context) (Transaction, error) {
		if err := cs.applyValidatorBindungLocked(ctx, &tx, jetzt); err != nil {
			return Transaction{}, err
		}
		fmt.Printf("[VALIDATOR] ✓ Bindung %s -> %s angenommen (ts %d)\n", kurzAdresse(tx.To), kurzAdresse(tx.Wallet), tx.Nachweis.Zeit)
		return tx, nil
	})
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
	ip := clientIP(r)
	if ts, loaded := registerRateLimit.Load("validator-bindung:" + ip); loaded {
		if time.Since(ts.(time.Time)) < 30*time.Second {
			jsonError(w, "rate limited, try again shortly", http.StatusTooManyRequests)
			return
		}
	}
	registerRateLimit.Store("validator-bindung:"+ip, time.Now())
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
	tx := Transaction{Wallet: req.Operator, To: req.Signing,
		Nachweis: &Auftragsnachweis{Zeit: req.Zeit, Sig: req.OperatorSignature, Sig2: req.SigningSignature}}
	if err := a.state.ValidatorBinden(tx); err != nil {
		switch {
		case errors.Is(err, ErrUeberweisungNichtSigniert):
			jsonError(w, "invalid binding: "+err.Error(), http.StatusBadRequest)
		case istZustandsAblehnung(err), errors.Is(err, errValidatorRegisterSchlaeft):
			jsonError(w, err.Error(), http.StatusConflict)
		case errors.Is(err, ErrNichtLeiter), errors.Is(err, ErrNurLesend):
			jsonError(w, err.Error(), http.StatusServiceUnavailable)
		default:
			jsonStateError(w, "validator-bindung", strings.ToLower(req.Operator), err)
		}
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true, "operator": strings.ToLower(req.Operator), "signing": strings.ToLower(req.Signing), "ts": req.Zeit,
	})
}
