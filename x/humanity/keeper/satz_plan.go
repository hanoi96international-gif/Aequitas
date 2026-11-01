package keeper

// Der Satz des Gleichgewichts ist einstellbar -- in festen Grenzen, mit
// Vorankuendigung und nur gemeinsam (docs/WIRTSCHAFT_ENDFASSUNG.md, "Spanne
// fuer Mitbestimmung"). Nichts daran ist endgueltig festgezurrt, und niemand
// kann ihn ueber Nacht aendern.
//
// WAS. Eine Transaktion satz_ankuendigung legt fest, ab wann ein neuer Satz
// gilt. Jeder Knoten prueft selbst (satzUrteilLocked):
//
//   - Spanne satzMinBp..satzMaxBp (0,25 bis 1 % im Monat), Schritte von
//     satzSchrittBp (0,05 %);
//   - hoechstens satzSprungMaxBp (0,25 Prozentpunkte) neben dem Satz, der
//     zur Blockzeit gilt -- kein Sprung von 0,25 auf 1 %;
//   - gilt fruehestens satzVorlaufMin (30 Tage) und spaetestens
//     satzVorlaufMax (90 Tage) nach dem Block, der sie traegt: jeder sieht
//     sie einen Monat vorher in der Kette;
//   - hoechstens eine Aenderung je Quartal: fruehestens satzAbstandMin nach
//     der letzten Aenderung, die schon gilt;
//   - unterschrieben von satzQuorum VERSCHIEDENEN zugelassenen
//     Betreiber-Menschen (bescheinigungPruefenFuer, wie beim Einspruch gegen
//     eine Staffel), jede Unterschrift ueber Satz, Gueltigkeit und
//     issued_at;
//   - issued_at nach dem der letzten angenommenen Ankuendigung: eine alte
//     Ankuendigung laesst sich nicht wiederholen.
//
// Eine neue Ankuendigung ersetzt eine noch nicht geltende. Eine Ankuendigung
// mit dem geltenden Satz nimmt eine offene zurueck und aendert sonst nichts
// -- sie zaehlt nicht als Aenderung fuer die Quartalsregel.
//
// ZUSTAND. Der Plan (satzPlan) steht in cs.satzPlan, in chain_config
// (satzKonfigSchluessel), im Rueckroll-Snapshot und im Snapshot-Export; die
// StateRoot enthaelt ihn, sobald es einen gibt -- ohne bleibt sie
// byte-gleich. Er haelt hoechstens satzPlanStufenMax Stufen: die zwei
// letzten geltenden und eine offene.
//
// WIRKUNG. umlaufBetragRoh rechnet ab dem Gleichgewicht mit dem Satz, der zur
// Rundenzeit gilt (gleichgewichtSatzZuLocked); jeder Nachspielende ebenso.
// Ohne Ankuendigung gilt satzVorgabeBp = gleichgewichtSatzMonat.
//
// SCHLAFEND wie das Gleichgewicht: vor gleichgewichtAktiv ist die Art
// unbekannt (bekannteTxArt), der Endpunkt antwortet 409.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	satzTxArt            = "satz_ankuendigung"
	satzDomain           = "aequitas-gleichgewicht-satz-v1"
	satzKonfigSchluessel = "gleichgewicht_satz_plan"

	satzVorgabeBp   int64 = 50 // 0,50 % im Monat (gleichgewichtSatzMonat)
	satzMinBp       int64 = 25
	satzMaxBp       int64 = 100
	satzSchrittBp   int64 = 5
	satzSprungMaxBp int64 = 25

	satzVorlaufMin int64 = 30 * 86400
	satzVorlaufMax int64 = 90 * 86400
	satzAbstandMin int64 = 90 * 86400

	satzQuorum                    = 2
	satzHoechstensBescheinigungen = 8
	satzPlanStufenMax             = 3
	satzKoerperMax                = 16 << 10
	// Annahme: mehrere Betreiber unterschreiben, das dauert -- wie beim
	// Einspruch hoechstens einen Tag alt; im Block hoechstens 7 Tage.
	satzAnnahmeAlt int64 = 86400
)

// SatzAnkuendigung: was eine satz_ankuendigung traegt. issued_at steht in
// Transaction.DistributionAt.
type SatzAnkuendigung struct {
	Bp              int64                        `json:"bp"`
	GiltAb          int64                        `json:"gilt_ab"`
	Bescheinigungen []Lebendigkeitsbescheinigung `json:"bescheinigungen"`
}

type satzStufe struct {
	Ab int64 `json:"ab"`
	Bp int64 `json:"bp"`
}

// satzPlan: die Stufen nach Ab aufsteigend; Zuletzt ist issued_at der letzten
// angenommenen Ankuendigung.
type satzPlan struct {
	Stufen  []satzStufe `json:"stufen,omitempty"`
	Zuletzt int64       `json:"zuletzt,omitempty"`
}

func (p satzPlan) leer() bool { return len(p.Stufen) == 0 && p.Zuletzt == 0 }

// bpZu: der Satz, der zu t gilt.
func (p satzPlan) bpZu(t int64) int64 {
	bp := satzVorgabeBp
	for _, s := range p.Stufen {
		if s.Ab <= t {
			bp = s.Bp
		}
	}
	return bp
}

// geltende: die Stufen, die zu t schon gelten (aufsteigend).
func (p satzPlan) geltende(t int64) []satzStufe {
	var g []satzStufe
	for _, s := range p.Stufen {
		if s.Ab <= t {
			g = append(g, s)
		}
	}
	return g
}

// offen: die angekuendigte, noch nicht geltende Stufe zu t.
func (p satzPlan) offen(t int64) (satzStufe, bool) {
	for _, s := range p.Stufen {
		if s.Ab > t {
			return s, true
		}
	}
	return satzStufe{}, false
}

func (p satzPlan) kodiert() string {
	b, _ := json.Marshal(p)
	return string(b)
}

// satzPlanLesen: der Wert aus chain_config oder einem Snapshot. Streng: was
// keine Kette erzeugt haben kann, wird abgewiesen.
func satzPlanLesen(s string) (satzPlan, error) {
	var p satzPlan
	if s == "" {
		return p, nil
	}
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return satzPlan{}, fmt.Errorf("Satzplan nicht lesbar: %w", err)
	}
	if len(p.Stufen) > satzPlanStufenMax || p.Zuletzt < 0 {
		return satzPlan{}, fmt.Errorf("Satzplan: %d Stufen", len(p.Stufen))
	}
	for i, st := range p.Stufen {
		if !satzBpErlaubt(st.Bp) || st.Ab <= 0 || (i > 0 && st.Ab <= p.Stufen[i-1].Ab) {
			return satzPlan{}, fmt.Errorf("Satzplan: Stufe %d ungueltig", i)
		}
	}
	if p.kodiert() != s {
		return satzPlan{}, errors.New("Satzplan nicht in kanonischer Schreibweise")
	}
	return p, nil
}

func satzBpErlaubt(bp int64) bool {
	return bp >= satzMinBp && bp <= satzMaxBp && bp%satzSchrittBp == 0
}

// gleichgewichtSatzZuLocked: der Satz (Anteil je Monat), der zu t gilt.
// Caller haelt cs.mu (lesend genuegt).
func (cs *ChainState) gleichgewichtSatzZuLocked(t int64) float64 {
	return float64(cs.satzPlan.bpZu(t)) / 10_000
}

// satzPlanLaden: beim Start und nach einem Snapshot-Import aus chain_config.
// Ein unlesbarer Wert haelt den Knoten an, statt mit einem anderen Satz zu
// rechnen als alle anderen.
func (cs *ChainState) satzPlanLaden(ctx context.Context) error {
	if cs.db == nil {
		return nil
	}
	p, err := satzPlanLesen(cs.getConfigValueCtx(ctx, satzKonfigSchluessel))
	if err != nil {
		return err
	}
	cs.satzPlan = p
	return nil
}

// satzNachricht: was jeder Betreiber unterschreibt.
func satzNachricht(bp, giltAb int64) func(wallet string, issuedAt int64) string {
	return func(_ string, issuedAt int64) string {
		return fmt.Sprintf("%s|chain:%d|%d|%d|%d", satzDomain, eingefroreneChainID, bp, giltAb, issuedAt)
	}
}

// ------------------------------------------------------------ Urteil

type satzUrteil struct {
	anwenden bool
	regel    string
	text     string
}

func satzBefund(regel, format string, args ...interface{}) satzUrteil {
	return satzUrteil{regel: regel, text: fmt.Sprintf(format, args...)}
}

// satzUrteilLocked: darf tx zu blockUnix angewandt werden? Billig vor teuer;
// Fehler nur, wenn die Zulassung nicht lesbar war (dann geht der Block
// zurueck). Caller haelt cs.mu.
func (cs *ChainState) satzUrteilLocked(ctx context.Context, tx *Transaction, blockUnix int64) (satzUrteil, error) {
	if !gleichgewichtAktiv(blockUnix) {
		return satzUrteil{}, nil
	}
	a := tx.Satz
	if a == nil {
		return satzBefund("satz_form", "keine Ankuendigung in der Transaktion"), nil
	}
	if n := len(a.Bescheinigungen); n < satzQuorum || n > satzHoechstensBescheinigungen {
		return satzBefund("satz_form", "%d Bescheinigungen (erlaubt %d bis %d)", n, satzQuorum, satzHoechstensBescheinigungen), nil
	}
	if !satzBpErlaubt(a.Bp) {
		return satzBefund("satz_grenze", "%d bp: erlaubt %d bis %d in Schritten von %d", a.Bp, satzMinBp, satzMaxBp, satzSchrittBp), nil
	}
	zeit := tx.DistributionAt
	if zeit <= 0 || zeit > sattAdd(blockUnix, nachweisHoechstensVoraus) || blockUnix-zeit > erneuerungHoechstensAlt {
		return satzBefund("satz_zeit", "bescheinigt %d, Blockzeit %d", zeit, blockUnix), nil
	}
	p := cs.satzPlan
	if zeit <= p.Zuletzt {
		return satzBefund("satz_wiederholt", "bescheinigt %d, die letzte Ankuendigung %d", zeit, p.Zuletzt), nil
	}
	if a.GiltAb < sattAdd(blockUnix, satzVorlaufMin) || a.GiltAb > sattAdd(blockUnix, satzVorlaufMax) {
		return satzBefund("satz_vorlauf", "gilt ab %d, Blockzeit %d: erlaubt %d bis %d Tage vorher",
			a.GiltAb, blockUnix, satzVorlaufMin/86400, satzVorlaufMax/86400), nil
	}
	jetzt := p.bpZu(blockUnix)
	if d := a.Bp - jetzt; d > satzSprungMaxBp || d < -satzSprungMaxBp {
		return satzBefund("satz_sprung", "%d bp, es gilt %d bp: hoechstens %d bp je Aenderung", a.Bp, jetzt, satzSprungMaxBp), nil
	}
	if g := p.geltende(blockUnix); len(g) > 0 && a.Bp != jetzt && a.GiltAb < sattAdd(g[len(g)-1].Ab, satzAbstandMin) {
		return satzBefund("satz_quartal", "gilt ab %d, die letzte Aenderung gilt seit %d: hoechstens eine je %d Tage",
			a.GiltAb, g[len(g)-1].Ab, satzAbstandMin/86400), nil
	}
	stand := func(m string) (bool, bool) {
		cs.ensureAccountLoadedCtx(ctx, m)
		acc, ok := cs.accounts.Get(m)
		return ok && acc.IsHuman, ok && acc.GrantStagedRest > 0
	}
	nachricht := satzNachricht(a.Bp, a.GiltAb)
	zulassung := cs.lebensnachweisZulassung(ctx)
	menschen := map[string]bool{}
	for i := range a.Bescheinigungen {
		b := &a.Bescheinigungen[i]
		if err := bescheinigungPruefenFuer(nachricht, ubiPoolAddr, zeit, b, stand, zulassung); err != nil {
			if errors.Is(err, errZulassungNichtLesbar) {
				return satzUrteil{}, err
			}
			return satzBefund("satz_ohne_bescheinigung", "Bescheinigung %d: %v", i+1, err), nil
		}
		menschen[b.Mensch] = true
	}
	if len(menschen) < satzQuorum {
		return satzBefund("satz_quorum", "%d verschiedene Betreiber-Menschen, noetig %d", len(menschen), satzQuorum), nil
	}
	return satzUrteil{anwenden: true}, nil
}

// satzSetzenLocked: der neue Plan nach einer gueltigen Ankuendigung --
// geltende Stufen bleiben (hoechstens zwei), eine offene wird ersetzt.
func (cs *ChainState) satzSetzenLocked(ctx context.Context, a *SatzAnkuendigung, zeit, blockUnix int64) error {
	p := cs.satzPlan
	g := p.geltende(blockUnix)
	if len(g) > satzPlanStufenMax-1 {
		g = g[len(g)-(satzPlanStufenMax-1):]
	}
	neu := satzPlan{Stufen: append([]satzStufe(nil), g...), Zuletzt: zeit}
	if a.Bp != p.bpZu(blockUnix) {
		neu.Stufen = append(neu.Stufen, satzStufe{Ab: a.GiltAb, Bp: a.Bp})
	}
	sort.Slice(neu.Stufen, func(i, j int) bool { return neu.Stufen[i].Ab < neu.Stufen[j].Ab })
	if cs.db != nil {
		if err := cs.setConfigValueCtx(ctx, satzKonfigSchluessel, neu.kodiert()); err != nil {
			return fmt.Errorf("satz_ankuendigung: Plan speichern: %w", err)
		}
	}
	cs.satzPlan = neu
	return nil
}

// applySatzAnkuendigungLocked: beim Nachspielen. Ungueltig: nie angewandt,
// gezaehlt, im strengen Modus Fehler. Caller haelt cs.mu.
func (cs *ChainState) applySatzAnkuendigungLocked(ctx context.Context, tx *Transaction, blockUnix int64) error {
	u, err := cs.satzUrteilLocked(ctx, tx, blockUnix)
	if err != nil {
		return err
	}
	if u.regel != "" {
		return nachrechnenAbweichung(u.regel, blockUnix, "%s", u.text)
	}
	if !u.anwenden {
		return nil
	}
	return cs.satzSetzenLocked(ctx, tx.Satz, tx.DistributionAt, blockUnix)
}

// SatzStand: fuer /api/wirtschaft/regeln.
func (cs *ChainState) SatzStand(jetzt int64) map[string]interface{} {
	p := cs.satzPlanKopie()
	m := map[string]interface{}{
		"bp_jetzt":     p.bpZu(jetzt),
		"prozent":      float64(p.bpZu(jetzt)) / 100,
		"spanne_bp":    []int64{satzMinBp, satzMaxBp},
		"schritt_bp":   satzSchrittBp,
		"sprung_bp":    satzSprungMaxBp,
		"vorlauf_tage": []int64{satzVorlaufMin / 86400, satzVorlaufMax / 86400},
		"abstand_tage": satzAbstandMin / 86400,
		"quorum":       satzQuorum,
	}
	if o, ok := p.offen(jetzt); ok {
		m["angekuendigt"] = map[string]interface{}{"bp": o.Bp, "prozent": float64(o.Bp) / 100, "gilt_ab": o.Ab}
	}
	return m
}

// satzPlanKopie: der Plan, unter der Lesesperre.
func (cs *ChainState) satzPlanKopie() satzPlan {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.satzPlan
}

// ------------------------------------------------------------ API

var satzGrenzArt = nachweisGrenzArt{
	aktiv:      gleichgewichtAktiv,
	inaktiv:    "gleichgewicht not active yet",
	direkt:     "satz:",
	pruefung:   "satz-pruefung:",
	von:        satzVonPraefix,
	zuVieleTxt: "too many rate announcements from this address -- please retry in a minute",
}

func (a *APIServer) satzGrenze(next http.HandlerFunc) http.HandlerFunc {
	return a.nachweisGrenze(satzGrenzArt, next)
}

type satzAbgelehnt struct{ text string }

func (e *satzAbgelehnt) Error() string { return e.text }

var errSatzOhneWirkung = errors.New("ohne Wirkung")

// handleSatzAnkuendigung nimmt eine Ankuendigung entgegen:
//
//	{"bp", "gilt_ab", "issued_at", "bescheinigungen": [{"public_key", "signature"}, ...]}
//
// Ablauf wie handleStaffelEinspruch; angenommen nur beim Leiter (der Plan
// gehoert keinem Konto, annahmeBeginnenLeiter).
func (a *APIServer) handleSatzAnkuendigung(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	now := time.Now().Unix()
	if !gleichgewichtAktiv(now) {
		jsonError(w, "gleichgewicht not active yet", http.StatusConflict)
		return
	}
	var req struct {
		Bp              int64 `json:"bp"`
		GiltAb          int64 `json:"gilt_ab"`
		IssuedAt        int64 `json:"issued_at"`
		Bescheinigungen []struct {
			PublicKey string `json:"public_key"`
			Signature string `json:"signature"`
		} `json:"bescheinigungen"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, satzKoerperMax)).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if !satzBpErlaubt(req.Bp) {
		jsonError(w, fmt.Sprintf("bp: %d to %d in steps of %d", satzMinBp, satzMaxBp, satzSchrittBp), http.StatusBadRequest)
		return
	}
	if n := len(req.Bescheinigungen); n < satzQuorum || n > satzHoechstensBescheinigungen {
		jsonError(w, fmt.Sprintf("bescheinigungen: %d to %d required", satzQuorum, satzHoechstensBescheinigungen), http.StatusBadRequest)
		return
	}
	if req.IssuedAt <= 0 || now-req.IssuedAt > satzAnnahmeAlt || req.IssuedAt-now > lebensnachweisAnnahmeVoraus {
		jsonError(w, "attestation expired or from the future", http.StatusBadRequest)
		return
	}
	nachricht := []byte(satzNachricht(req.Bp, req.GiltAb)("", req.IssuedAt))
	for i, b := range req.Bescheinigungen {
		if !ed25519PruefenStreng(strings.ToLower(strings.TrimSpace(b.PublicKey)), ed25519SigNormal(b.Signature), nachricht) {
			jsonError(w, fmt.Sprintf("invalid rate attestation %d: does not match rate, start and time", i+1), http.StatusForbidden)
			return
		}
	}
	tx := Transaction{Type: satzTxArt, DistributionAt: req.IssuedAt, Satz: &SatzAnkuendigung{Bp: req.Bp, GiltAb: req.GiltAb}}
	for i, b := range req.Bescheinigungen {
		bindung, ok, err := a.state.coordinatorBindungLesen(r.Context(), b.PublicKey)
		if err != nil {
			fmt.Printf("[API] rate announcement: coordinator register not readable: %v\n", err)
			jsonError(w, "coordinator register temporarily unavailable, please retry shortly", http.StatusServiceUnavailable)
			return
		}
		if !ok {
			jsonError(w, fmt.Sprintf("rate attestation %d not signed by a registered coordinator", i+1), http.StatusForbidden)
			return
		}
		tx.Satz.Bescheinigungen = append(tx.Satz.Bescheinigungen, Lebendigkeitsbescheinigung{
			PublicKey:     strings.ToLower(strings.TrimSpace(b.PublicKey)),
			Signature:     ed25519SigNormal(b.Signature),
			Mensch:        strings.ToLower(strings.TrimSpace(bindung.Mensch)),
			MenschSig:     kanonischeSignaturVersuch(bindung.MenschSig),
			SchluesselSig: ed25519SigNormal(bindung.SchluesselSig),
		})
	}
	if err := a.state.annahmeBeginnenLeiter(); err != nil {
		jsonError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer a.state.annahmeEnde()
	if err := a.state.runAtomicWithOutbox(nil, false, func(ctx context.Context) (Transaction, error) {
		u, err := a.state.satzUrteilLocked(ctx, &tx, now)
		if err != nil {
			return Transaction{}, err
		}
		if u.regel != "" {
			return Transaction{}, &satzAbgelehnt{text: u.regel + ": " + u.text}
		}
		if !u.anwenden {
			return Transaction{}, errSatzOhneWirkung
		}
		if err := a.state.satzSetzenLocked(ctx, tx.Satz, req.IssuedAt, now); err != nil {
			return Transaction{}, err
		}
		return tx, nil
	}); err != nil {
		var abgelehnt *satzAbgelehnt
		switch {
		case errors.As(err, &abgelehnt):
			jsonError(w, "rate announcement rejected: "+abgelehnt.text, http.StatusConflict)
		case errors.Is(err, errSatzOhneWirkung):
			jsonError(w, "rate announcement has no effect", http.StatusConflict)
		default:
			fmt.Printf("[API] rate announcement failed: %v\n", err)
			jsonError(w, "internal error, please retry shortly", http.StatusInternalServerError)
		}
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "satz": a.state.SatzStand(now)})
}
