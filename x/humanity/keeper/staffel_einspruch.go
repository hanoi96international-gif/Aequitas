package keeper

// Einspruch gegen die Freigabe der Staffel an Tag 30 (Entscheidung
// 18.10.2026, docs/VERIFIER_PRINZIPIEN.md "Festgelegte Entscheidungen";
// docs/REGISTRIERUNG_ENDFASSUNG.md, 5.2).
//
// WAS. Eine gelbe Registrierung bekommt 200 AEQ sofort und 800 AEQ als
// Staffel. Seit dieser Regel wird der Rest spaetestens an Tag 30 ganz frei,
// auch ohne zweite Pruefung (grant_staffel.go, staffelFreigabeHoechstens).
// Anhalten kann diese Freigabe nur ein begruendeter Einspruch:
//
//   - unterschrieben von mindestens staffelEinspruchQuorum VERSCHIEDENEN
//     Betreiber-Menschen, jede Unterschrift eine Bescheinigung mit Bindung
//     wie beim Lebensnachweis (bescheinigungPruefenFuer: Freigabe des
//     Menschen, Besitznachweis des Schluessels, Mensch registriert und ohne
//     offene Staffel, nicht der Betroffene selbst, im Konsens zugelassen);
//   - mit einem Grund (staffelEinspruchGrundMin..Max Byte, eine
//     Schreibweise), dessen SHA-256 jede Unterschrift deckt -- der Grund ist
//     oeffentlich und nachpruefbar, nicht austauschbar. Er steht fuer immer
//     in der Kette: keine personenbezogenen Daten, nur der Befund und eine
//     Vorgangskennung (z. B. "W-..." aus dem Widerspruchsweg);
//   - bescheinigt VOR Tag 30 (issued_at < GrantStagedUntil).
//
// WAS ER BEWIRKT -- UND WAS NICHT. Er nimmt nichts weg, was schon frei ist,
// und er haelt nur die Freigabe an Tag 30 an. Beenden kann die Pause der
// Mensch selbst: mit einer zweiten Lebendigkeitspruefung NACH dem Einspruch
// (staffelErneuertGueltig) laeuft die Staffel wieder, ab Tag 30 der ganze
// Rest. Und sie endet spaetestens staffelEinspruchHaltTage nach Tag 30 von
// selbst. Keine Regel haelt einem Menschen Geld ohne Zeitgrenze zurueck
// (VERIFIER_PRINZIPIEN.md, "Regeln fuer jede Aenderung"). Je Konto zaehlt
// nur der erste Einspruch.
//
// WARUM ZWEI MENSCHEN. Mit einem einzigen Betreiber koennte genau einer jede
// Staffel anhalten. Zwei verschiedene Menschen sind dieselbe Untergrenze wie
// beim Lebensnachweis (lebensnachweisMindestBetreiber): solange es keine zwei
// zugelassenen Betreiber gibt, ist kein Einspruch moeglich, und jede Staffel
// wird an Tag 30 frei.
//
// DETERMINISMUS. Ob ein Einspruch gilt, haengt nur an der Transaktion (Grund,
// Bescheinigungen, issued_at), am Kettenzustand (Konto, Zulassung im
// Register) und an der Blockzeit. Jeder Knoten prueft ihn selbst
// (staffelEinspruchUrteilLocked) -- bei der Annahme wie beim Nachspielen.
// StaffelEinspruchAm steht im accountLeaf, nur wenn gesetzt.
//
// SCHLAFEND wie die Staffel: vor stagedGrantActivationUnix ist die Art
// unbekannt (bekannteTxArt), der Endpunkt antwortet 409.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	staffelEinspruchTxArt  = "staffel_einspruch"
	staffelEinspruchDomain = "aequitas-staffel-einspruch-v1"

	// Verschiedene Betreiber-Menschen, die unterschreiben muessen.
	staffelEinspruchQuorum = 2
	// Mehr Bescheinigungen bringen nichts; jede kostet Unterschriften und
	// eine Abfrage der Zulassung.
	staffelEinspruchHoechstensBescheinigungen = 8

	// Wie lange ein Einspruch die Freigabe hoechstens anhaelt, gezaehlt ab
	// Tag 30: bis Tag 180 nach der Registrierung.
	staffelEinspruchHaltTage int64 = 150

	staffelEinspruchGrundMin = 10
	staffelEinspruchGrundMax = 280

	// Annahme: ein Einspruch braucht Unterschriften mehrerer Betreiber -- das
	// dauert laenger als die 15 Minuten einer Erneuerung. Im Block gilt wie
	// ueberall hoechstens 7 Tage alt (erneuerungHoechstensAlt).
	staffelEinspruchAnnahmeAlt int64 = 86400
	staffelEinspruchKoerperMax       = 16 << 10
)

// StaffelEinspruch: was eine staffel_einspruch-Transaktion traegt. Der
// Betroffene steht in Transaction.Wallet, issued_at in DistributionAt.
type StaffelEinspruch struct {
	Grund           string                       `json:"grund"`
	Bescheinigungen []Lebendigkeitsbescheinigung `json:"bescheinigungen"`
}

// staffelEinspruchNachricht: was jeder Betreiber unterschreibt -- mit der
// eingefrorenen Chain-ID und dem Hash des Grundes.
func staffelEinspruchNachricht(grund string) func(wallet string, issuedAt int64) string {
	h := sha256.Sum256([]byte(grund))
	gh := hex.EncodeToString(h[:])
	return func(wallet string, issuedAt int64) string {
		return fmt.Sprintf("%s|chain:%d|%s|%d|%s", staffelEinspruchDomain, eingefroreneChainID,
			strings.ToLower(strings.TrimSpace(wallet)), issuedAt, gh)
	}
}

// staffelEinspruchGrundGueltig: eine Schreibweise, gueltiges UTF-8, ohne
// Steuerzeichen, Laenge in Byte begrenzt.
func staffelEinspruchGrundGueltig(g string) bool {
	if len(g) < staffelEinspruchGrundMin || len(g) > staffelEinspruchGrundMax {
		return false
	}
	if !utf8.ValidString(g) || strings.TrimSpace(g) != g {
		return false
	}
	for _, r := range g {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// staffelErneuertGueltig: zaehlt die zweite Pruefung dieses Kontos? Nur,
// wenn es eine gibt und sie NACH einem Einspruch liegt -- eine Pruefung von
// vor dem Einspruch beantwortet ihn nicht.
func staffelErneuertGueltig(acc *AccountState) bool {
	return acc != nil && acc.LivenessRenewedAt > 0 && acc.LivenessRenewedAt > acc.StaffelEinspruchAm
}

// staffelEinspruchHaelt: haelt ein Einspruch die Freigabe an Tag 30 zur
// Rundenzeit t an? Bis eine neuere Pruefung kommt, hoechstens bis
// GrantStagedUntil + staffelEinspruchHaltTage.
func staffelEinspruchHaelt(acc *AccountState, t int64) bool {
	return acc != nil && acc.StaffelEinspruchAm > 0 && !staffelErneuertGueltig(acc) &&
		t < sattAdd(acc.GrantStagedUntil, staffelEinspruchHaltTage*86400)
}

// staffelFreiAb: ab wann der ganze Rest spaetestens frei wird (Anzeige).
func staffelFreiAb(acc *AccountState) int64 {
	if acc == nil || acc.GrantStagedUntil == 0 {
		return 0
	}
	if acc.StaffelEinspruchAm > 0 && !staffelErneuertGueltig(acc) {
		return sattAdd(acc.GrantStagedUntil, staffelEinspruchHaltTage*86400)
	}
	return acc.GrantStagedUntil
}

// ------------------------------------------------------------ Urteil

// staffelEinspruchUrteil: was eine staffel_einspruch zu einer Blockzeit
// bewirkt.
type staffelEinspruchUrteil struct {
	anwenden bool   // StaffelEinspruchAm wird issued_at
	regel    string // Befund (nachrechnen), "" = keiner
	text     string
}

func staffelEinspruchBefund(regel, format string, args ...interface{}) staffelEinspruchUrteil {
	return staffelEinspruchUrteil{regel: regel, text: fmt.Sprintf(format, args...)}
}

// staffelEinspruchUrteilLocked: darf tx zu blockUnix angewandt werden? Jeder
// Knoten prueft selbst, billig vor teuer:
//
//   - vor der Staffel: Leerlauf, kein Befund;
//   - Form: Wallet kanonisch, Grund gueltig, 2..8 Bescheinigungen
//     (einspruch_form);
//   - issued_at > 0, hoechstens 5 min nach und hoechstens 7 Tage vor dem
//     Block (einspruch_zeit);
//   - der Betroffene ist ein Mensch mit offener Staffel (einspruch_ohne_staffel);
//   - schon ein Einspruch: Leerlauf -- nur der erste zaehlt, ein zweites
//     Nachspielen derselben Transaktion aendert nichts;
//   - bescheinigt vor Tag 30 (einspruch_zu_spaet);
//   - jede Bescheinigung gueltig (einspruch_ohne_bescheinigung), dann
//     mindestens staffelEinspruchQuorum verschiedene Menschen
//     (einspruch_quorum).
//
// Fehler nur, wenn die Zulassung nicht lesbar war: dann wird nichts
// entschieden, der ganze Block geht zurueck (wie beim Lebensnachweis).
// Caller haelt cs.mu.
func (cs *ChainState) staffelEinspruchUrteilLocked(ctx context.Context, tx *Transaction, blockUnix int64) (staffelEinspruchUrteil, error) {
	if !stagedGrantAktiv(blockUnix) {
		return staffelEinspruchUrteil{}, nil
	}
	if !kanonischeAdresse(tx.Wallet) {
		return staffelEinspruchBefund("einspruch_form", "%q: Wallet nicht in kanonischer Schreibweise", tx.Wallet), nil
	}
	wallet := tx.Wallet
	e := tx.Einspruch
	if e == nil {
		return staffelEinspruchBefund("einspruch_form", "%s: kein Einspruch in der Transaktion", kurzAdresse(wallet)), nil
	}
	if !staffelEinspruchGrundGueltig(e.Grund) {
		return staffelEinspruchBefund("einspruch_form", "%s: Grund fehlt, ist zu lang oder nicht in einer Schreibweise", kurzAdresse(wallet)), nil
	}
	if n := len(e.Bescheinigungen); n < staffelEinspruchQuorum || n > staffelEinspruchHoechstensBescheinigungen {
		return staffelEinspruchBefund("einspruch_form", "%s: %d Bescheinigungen (erlaubt %d bis %d)",
			kurzAdresse(wallet), n, staffelEinspruchQuorum, staffelEinspruchHoechstensBescheinigungen), nil
	}
	zeit := tx.DistributionAt
	if zeit <= 0 || zeit > sattAdd(blockUnix, nachweisHoechstensVoraus) || blockUnix-zeit > erneuerungHoechstensAlt {
		return staffelEinspruchBefund("einspruch_zeit", "%s: bescheinigt %d, Blockzeit %d", kurzAdresse(wallet), zeit, blockUnix), nil
	}
	cs.ensureAccountLoadedCtx(ctx, wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok || !acc.IsHuman || acc.GrantStagedRest <= 0 || acc.GrantStagedUntil <= 0 {
		return staffelEinspruchBefund("einspruch_ohne_staffel", "%s hat keine offene Staffel", kurzAdresse(wallet)), nil
	}
	if acc.StaffelEinspruchAm > 0 {
		return staffelEinspruchUrteil{}, nil
	}
	if zeit >= acc.GrantStagedUntil {
		return staffelEinspruchBefund("einspruch_zu_spaet", "%s: bescheinigt %d, Tag 30 ist %d",
			kurzAdresse(wallet), zeit, acc.GrantStagedUntil), nil
	}
	stand := func(m string) (bool, bool) {
		cs.ensureAccountLoadedCtx(ctx, m)
		a, ok := cs.accounts.Get(m)
		return ok && a.IsHuman, ok && a.GrantStagedRest > 0
	}
	nachricht := staffelEinspruchNachricht(e.Grund)
	zulassung := cs.lebensnachweisZulassung(ctx)
	menschen := map[string]bool{}
	for i := range e.Bescheinigungen {
		b := &e.Bescheinigungen[i]
		if err := bescheinigungPruefenFuer(nachricht, wallet, zeit, b, stand, zulassung); err != nil {
			if errors.Is(err, errZulassungNichtLesbar) {
				return staffelEinspruchUrteil{}, err
			}
			return staffelEinspruchBefund("einspruch_ohne_bescheinigung", "%s: Bescheinigung %d: %v",
				kurzAdresse(wallet), i+1, err), nil
		}
		menschen[b.Mensch] = true
	}
	if len(menschen) < staffelEinspruchQuorum {
		return staffelEinspruchBefund("einspruch_quorum", "%s: %d verschiedene Betreiber-Menschen, noetig %d",
			kurzAdresse(wallet), len(menschen), staffelEinspruchQuorum), nil
	}
	return staffelEinspruchUrteil{anwenden: true}, nil
}

// staffelEinspruchSetzenLocked: StaffelEinspruchAm wird gesetzt, nur einmal.
func (cs *ChainState) staffelEinspruchSetzenLocked(ctx context.Context, wallet string, zeit int64) error {
	acc, ok := cs.accounts.Get(wallet)
	if !ok || !acc.IsHuman {
		return fmt.Errorf("staffel_einspruch fuer %s: kein registrierter Mensch", kurzAdresse(wallet))
	}
	if acc.StaffelEinspruchAm > 0 {
		return nil
	}
	acc.StaffelEinspruchAm = zeit
	return cs.saveAccountToDBCtx(ctx, acc)
}

// applyStaffelEinspruchLocked: staffel_einspruch beim Nachspielen. Ein
// ungueltiger Einspruch wird nie angewandt -- in keinem Modus; er wird als
// Befund gezaehlt und lehnt im strengen Modus den Block ab. Caller haelt
// cs.mu.
func (cs *ChainState) applyStaffelEinspruchLocked(ctx context.Context, tx *Transaction, blockUnix int64) error {
	u, err := cs.staffelEinspruchUrteilLocked(ctx, tx, blockUnix)
	if err != nil {
		return err
	}
	if u.regel != "" {
		return nachrechnenAbweichung(u.regel, blockUnix, "%s", u.text)
	}
	if !u.anwenden {
		return nil
	}
	return cs.staffelEinspruchSetzenLocked(ctx, tx.Wallet, tx.DistributionAt)
}

// ------------------------------------------------------------ API

var staffelEinspruchGrenzArt = nachweisGrenzArt{
	aktiv:      stagedGrantAktiv,
	inaktiv:    "staged grant not active yet",
	direkt:     "staffel-einspruch:",
	pruefung:   "staffel-einspruch-pruefung:",
	von:        staffelEinspruchVonPraefix,
	zuVieleTxt: "too many objections from this address -- please retry in a minute",
}

// staffelEinspruchGrenze: dieselbe Grenze wie Erneuerung und Lebensnachweis,
// mit eigenen Zaehlern.
func (a *APIServer) staffelEinspruchGrenze(next http.HandlerFunc) http.HandlerFunc {
	return a.nachweisGrenze(staffelEinspruchGrenzArt, next)
}

// errSchonEingesprochen: das Konto hat schon einen Einspruch.
var errSchonEingesprochen = errors.New("schon eingesprochen")

type staffelEinspruchAbgelehnt struct{ text string }

func (e *staffelEinspruchAbgelehnt) Error() string { return e.text }

// handleStaffelEinspruch nimmt einen Einspruch entgegen:
//
//	{"wallet", "issued_at", "grund", "bescheinigungen": [{"public_key", "signature"}, ...]}
//
// Ablauf wie handleProofOfLife: Form und Zeitfenster, jede Unterschrift vor
// jeder Datenbank, die Bindung jedes Schluessels aus dem Register dieses
// Knotens in die Transaktion, dann unter der Sperre genau die Pruefung jedes
// Nachspielenden, Annahme-Tor, Ausgang atomar mit dem Zustand.
func (a *APIServer) handleStaffelEinspruch(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	now := time.Now().Unix()
	if !stagedGrantAktiv(now) {
		jsonError(w, "staged grant not active yet", http.StatusConflict)
		return
	}
	var req struct {
		Wallet          string `json:"wallet"`
		IssuedAt        int64  `json:"issued_at"`
		Grund           string `json:"grund"`
		Bescheinigungen []struct {
			PublicKey string `json:"public_key"`
			Signature string `json:"signature"`
		} `json:"bescheinigungen"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, staffelEinspruchKoerperMax)).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	wallet := strings.ToLower(strings.TrimSpace(req.Wallet))
	if !kanonischeAdresse(wallet) {
		jsonError(w, "wallet must be a 0x address", http.StatusBadRequest)
		return
	}
	if !staffelEinspruchGrundGueltig(req.Grund) {
		jsonError(w, fmt.Sprintf("grund: %d to %d bytes, no control characters, no leading or trailing spaces",
			staffelEinspruchGrundMin, staffelEinspruchGrundMax), http.StatusBadRequest)
		return
	}
	if n := len(req.Bescheinigungen); n < staffelEinspruchQuorum || n > staffelEinspruchHoechstensBescheinigungen {
		jsonError(w, fmt.Sprintf("bescheinigungen: %d to %d required", staffelEinspruchQuorum, staffelEinspruchHoechstensBescheinigungen), http.StatusBadRequest)
		return
	}
	if req.IssuedAt <= 0 || now-req.IssuedAt > staffelEinspruchAnnahmeAlt || req.IssuedAt-now > lebensnachweisAnnahmeVoraus {
		jsonError(w, "attestation expired or from the future", http.StatusBadRequest)
		return
	}
	nachricht := []byte(staffelEinspruchNachricht(req.Grund)(wallet, req.IssuedAt))
	// Alle Unterschriften vor jeder Registerabfrage -- Muell kostet keine
	// Datenbank.
	for i, b := range req.Bescheinigungen {
		if !ed25519PruefenStreng(strings.ToLower(strings.TrimSpace(b.PublicKey)), ed25519SigNormal(b.Signature), nachricht) {
			jsonError(w, fmt.Sprintf("invalid objection attestation %d: does not match wallet, time and reason", i+1), http.StatusForbidden)
			return
		}
	}
	tx := Transaction{Type: staffelEinspruchTxArt, Wallet: wallet, DistributionAt: req.IssuedAt,
		Einspruch: &StaffelEinspruch{Grund: req.Grund}}
	for i, b := range req.Bescheinigungen {
		bindung, ok, err := a.state.coordinatorBindungLesen(r.Context(), b.PublicKey)
		if err != nil {
			fmt.Printf("[API] staffel objection: coordinator register not readable for %q: %v\n", kurzAdresse(wallet), err)
			jsonError(w, "coordinator register temporarily unavailable, please retry shortly", http.StatusServiceUnavailable)
			return
		}
		if !ok {
			jsonError(w, fmt.Sprintf("objection attestation %d not signed by a registered coordinator", i+1), http.StatusForbidden)
			return
		}
		tx.Einspruch.Bescheinigungen = append(tx.Einspruch.Bescheinigungen, Lebendigkeitsbescheinigung{
			PublicKey:     strings.ToLower(strings.TrimSpace(b.PublicKey)),
			Signature:     ed25519SigNormal(b.Signature),
			Mensch:        strings.ToLower(strings.TrimSpace(bindung.Mensch)),
			MenschSig:     kanonischeSignaturVersuch(bindung.MenschSig),
			SchluesselSig: ed25519SigNormal(bindung.SchluesselSig),
		})
	}
	if err := a.state.annahmeBeginnen(wallet); err != nil {
		jsonError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer a.state.annahmeEnde()
	if err := a.state.runAtomicWithOutbox([]string{wallet}, false, func(ctx context.Context) (Transaction, error) {
		u, err := a.state.staffelEinspruchUrteilLocked(ctx, &tx, now)
		if err != nil {
			return Transaction{}, err
		}
		if u.regel != "" {
			return Transaction{}, &staffelEinspruchAbgelehnt{text: u.regel + ": " + u.text}
		}
		if !u.anwenden {
			return Transaction{}, errSchonEingesprochen
		}
		if err := a.state.staffelEinspruchSetzenLocked(ctx, wallet, req.IssuedAt); err != nil {
			return Transaction{}, err
		}
		return tx, nil
	}); err != nil {
		var abgelehnt *staffelEinspruchAbgelehnt
		switch {
		case errors.Is(err, errSchonEingesprochen):
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "wallet": wallet, "schon_eingesprochen": true})
		case errors.As(err, &abgelehnt):
			jsonError(w, "objection rejected: "+abgelehnt.text, http.StatusConflict)
		default:
			fmt.Printf("[API] staffel objection failed for %s: %v\n", kurzAdresse(wallet), err)
			jsonError(w, "internal error, please retry shortly", http.StatusInternalServerError)
		}
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "wallet": wallet, "einspruch_am": req.IssuedAt,
		"staffel": a.state.StaffelStandVon(wallet)})
}
