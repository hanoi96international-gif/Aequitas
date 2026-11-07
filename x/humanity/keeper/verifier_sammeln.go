package keeper

// STIMMEN SAMMELN, AUTOMATISCH EINREICHEN (Pruefer-Register, A2).
//
// # WARUM
//
// Eine verifier_zulassung braucht die Stimmen von zwei Dritteln der Pruefer in
// EINER Transaktion (verifier_register.go). Pruefer stimmen aber nicht im
// selben Augenblick ab, und keiner von ihnen soll die Stimmen der anderen
// einsammeln und von Hand einreichen muessen -- das waere wieder eine
// Vertrauensstelle und Handarbeit.
//
// Hier sammelt der Knoten:
//
//   - POST /api/verifier/antrag   ein registrierter Mensch bewirbt sich
//     (EIP-191 seiner Wallet). Nur Anzeige: die Pruefer sehen, wer will.
//   - POST /api/verifier/stimme   eine Stimme (Bescheinigung des Coordinators
//     eines Pruefers mit Bindung). Sobald genug verschiedene Pruefer fuer
//     dieselbe Art und denselben Betroffenen gestimmt haben, reicht der
//     Knoten selbst ein -- ueber verifierEinreichen, also mit genau der
//     Pruefung, die jeder Knoten beim Nachspielen macht.
//   - GET  /api/verifier/antraege offene Antraege und Abstimmungen.
//
// # WAS DIE SAMMELSTELLE NICHT DARF
//
// Sie entscheidet nichts. Gezaehlt wird beim Einreichen und beim Nachspielen
// von verifierUrteilLocked, jeder Knoten selbst. Die Sammelstelle prueft vorab
// nur, was ohne Zustand geht (Unterschriften, Bindung, Zeit, Stimmrecht laut
// Register-Schnappschuss) -- damit Muell gar nicht erst Platz belegt. Eine
// Stimme, die beim Einreichen doch durchfaellt (etwa: der Pruefer hat keinen
// Validator-Schluessel mehr), wird verworfen, statt die Abstimmung zu
// blockieren.
//
// Knotenlokal und fluechtig: nach einem Neustart ist die Sammlung leer. Nichts
// geht dabei verloren, was zaehlt -- eine Stimme ist eine Bescheinigung und
// kann erneut eingereicht werden (die App tut das von selbst).
//
// # GRENZEN
//
// Hoechstens verifierSammlungenHoechstens offene Abstimmungen und
// verifierAntraegeHoechstens Antraege. Stimmen gibt es je Abstimmung
// hoechstens eine je Pruefer -- und Pruefer sind hoechstens verifierHoechstens.
// Eine Abstimmung eroeffnen kann nur ein Pruefer (gueltige Stimme), einen
// Antrag stellen nur ein registrierter Mensch, einmal. Alles verfaellt nach
// verifierStimmeHoechstensAlt. Koerper hoechstens 8 bzw. 4 KiB, und jede
// Anfrage zaehlt in der Grenze je Adresse (verifierGrenze).

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	verifierSammlungenHoechstens = 64
	verifierAntraegeHoechstens   = 64
	verifierStimmeKoerperMax     = 8 << 10
	verifierAntragKoerperMax     = 4 << 10
	// Ein Antrag ist hoechstens so alt oder so weit voraus, wenn er ankommt.
	verifierAntragSpielraum int64 = 600
)

// verifierAntragNachricht: was ein Kandidat mit seiner Wallet unterschreibt.
func verifierAntragNachricht(wallet string, zeit int64) string {
	return fmt.Sprintf("Aequitas: apply as verifier %s chain:%d ts:%d", strings.ToLower(strings.TrimSpace(wallet)), eingefroreneChainID, zeit)
}

type verifierSammlung struct {
	art, wallet string
	stimmen     map[string]VerifierStimme // Mensch -> seine juengste Stimme
}

type verifierSammelstelle struct {
	mu       sync.Mutex
	offen    map[string]*verifierSammlung // art|wallet
	antraege map[string]int64             // wallet -> Zeit des Antrags
}

func neueVerifierSammelstelle() *verifierSammelstelle {
	return &verifierSammelstelle{offen: map[string]*verifierSammlung{}, antraege: map[string]int64{}}
}

var verifierSammel = neueVerifierSammelstelle()

// aufraeumenLocked: Verfallenes weg, Stimmen vor der letzten Aenderung beim
// Betroffenen weg (sie zaehlen nie wieder), Antraege von Pruefern weg.
func (v *verifierSammelstelle) aufraeumenLocked(reg *verifierRegister, now int64) {
	for k, s := range v.offen {
		e, hatEintrag := reg.eintrag(s.wallet)
		for m, st := range s.stimmen {
			if now-st.Zeit > verifierStimmeHoechstensAlt || (hatEintrag && st.Zeit <= e.Zuletzt) {
				delete(s.stimmen, m)
			}
		}
		if len(s.stimmen) == 0 {
			delete(v.offen, k)
		}
	}
	for w, t := range v.antraege {
		if now-t > verifierStimmeHoechstensAlt || reg.mitglied(w, now) {
			delete(v.antraege, w)
		}
	}
}

// verifierNoetig: verschiedene Pruefer, die art fuer wallet zur Zeit now
// braucht -- dieselbe Rechnung wie verifierUrteilLocked. 0: nicht moeglich.
func verifierNoetig(reg *verifierRegister, art, wallet string, now int64) int {
	mit := reg.mitgliederZu(now)
	switch art {
	case verifierArtAufnahme:
		return verifierSchwelle(len(mit))
	case verifierArtEntzug:
		n := 0
		for _, m := range mit {
			if m != wallet {
				n++
			}
		}
		if n == 0 {
			return 0
		}
		return verifierSchwelle(n)
	case verifierArtAustritt:
		return 1
	}
	return 0
}

// verifierStimmeVorpruefen: alles, was ohne Kettenzustand geht. Die Stimme
// wird dabei in ihre eine Schreibweise gebracht.
func verifierStimmeVorpruefen(art, wallet string, s *VerifierStimme, reg *verifierRegister, now int64) error {
	if art != verifierArtAufnahme && art != verifierArtAustritt && art != verifierArtEntzug {
		return fmt.Errorf("unknown art %q", art)
	}
	if !kanonischeAdresse(wallet) {
		return errors.New("wallet: 0x + 40 lower-case hex required")
	}
	b := &s.Bescheinigung
	b.PublicKey = strings.ToLower(strings.TrimSpace(b.PublicKey))
	b.Signature = ed25519SigNormal(b.Signature)
	b.Mensch = strings.ToLower(strings.TrimSpace(b.Mensch))
	b.MenschSig = kanonischeSignaturVersuch(b.MenschSig)
	b.SchluesselSig = ed25519SigNormal(b.SchluesselSig)
	if s.Zeit <= 0 || s.Zeit > now+nachweisHoechstensVoraus || now-s.Zeit > verifierStimmeHoechstensAlt {
		return errors.New("vote time expired or from the future")
	}
	if len(b.PublicKey) != 64 || !kleinHex(b.PublicKey) || !kanonischeAdresse(b.Mensch) {
		return errors.New("vote: key or human malformed")
	}
	// Stimmrecht laut Register (billig) vor jeder Unterschrift.
	switch art {
	case verifierArtAustritt:
		if b.Mensch != wallet {
			return errors.New("only the verifier themself can leave")
		}
		if !reg.mitglied(wallet, now) {
			return errors.New("not a verifier")
		}
	case verifierArtAufnahme:
		if reg.mitglied(wallet, now) {
			return errors.New("already a verifier")
		}
		if !reg.mitglied(b.Mensch, now) {
			return errors.New("voter is not a verifier")
		}
	case verifierArtEntzug:
		if !reg.mitglied(wallet, now) {
			return errors.New("not a verifier")
		}
		if b.Mensch == wallet || !reg.mitglied(b.Mensch, now) {
			return errors.New("voter is not one of the other verifiers")
		}
	}
	if e, ok := reg.eintrag(wallet); ok && s.Zeit <= e.Zuletzt {
		return errors.New("vote predates the last change for this wallet")
	}
	if !ed25519PruefenStreng(b.PublicKey, b.Signature, []byte(verifierNachricht(art, wallet)("", s.Zeit))) {
		return errors.New("vote signature does not match art, wallet and time")
	}
	if !verifyCoordinatorPossession(b.PublicKey, b.SchluesselSig, b.Mensch) {
		return errors.New("key possession proof missing or wrong")
	}
	if !kanonischeSignatur(b.MenschSig) || verifyPersonalSign(coordinatorFreigabeNachricht(b.PublicKey), b.MenschSig, b.Mensch) != nil {
		return errors.New("human's approval of the key missing or wrong")
	}
	return nil
}

// verifierSammelEinreichen: hat die Abstimmung art|wallet genug Stimmen, beim
// Leiter einreichen. Untaugliche Stimmen fallen dabei heraus (hoechstens so
// viele Versuche, wie Stimmen da sind). Ergebnis fuer die Antwort.
func (a *APIServer) verifierSammelEinreichen(art, wallet string, now int64) map[string]interface{} {
	key := art + "|" + wallet
	for versuch := 0; versuch <= verifierStimmenHoechstens; versuch++ {
		reg := a.state.verifierRegisterJetzt()
		verifierSammel.mu.Lock()
		s := verifierSammel.offen[key]
		var stimmen []VerifierStimme
		if s != nil {
			for _, st := range s.stimmen {
				stimmen = append(stimmen, st)
			}
		}
		verifierSammel.mu.Unlock()
		noetig := verifierNoetig(reg, art, wallet, now)
		out := map[string]interface{}{"stimmen": len(stimmen), "noetig": noetig}
		if noetig == 0 || len(stimmen) < noetig {
			out["eingereicht"] = false
			return out
		}
		sort.Slice(stimmen, func(i, j int) bool { return stimmen[i].Bescheinigung.Mensch < stimmen[j].Bescheinigung.Mensch })
		if len(stimmen) > verifierStimmenHoechstens {
			stimmen = stimmen[:verifierStimmenHoechstens]
		}
		err := a.verifierEinreichen(art, wallet, now, now, stimmen)
		if err == nil {
			verifierSammel.mu.Lock()
			delete(verifierSammel.offen, key)
			if art == verifierArtAufnahme {
				delete(verifierSammel.antraege, wallet)
			}
			verifierSammel.mu.Unlock()
			out["eingereicht"] = true
			return out
		}
		var abgelehnt *verifierAbgelehnt
		if errors.As(err, &abgelehnt) && abgelehnt.stimme >= 1 && abgelehnt.stimme <= len(stimmen) {
			// Diese eine Stimme taugt nicht -- verwerfen, mit dem Rest weiter.
			schlecht := stimmen[abgelehnt.stimme-1].Bescheinigung.Mensch
			verifierSammel.mu.Lock()
			if s := verifierSammel.offen[key]; s != nil {
				delete(s.stimmen, schlecht)
				if len(s.stimmen) == 0 {
					delete(verifierSammel.offen, key)
				}
			}
			verifierSammel.mu.Unlock()
			fmt.Printf("[VERIFIER] Stimme von %s verworfen: %s\n", kurzAdresse(schlecht), abgelehnt.text)
			continue
		}
		out["eingereicht"] = false
		var leiter *verifierNichtLeiter
		switch {
		case errors.As(err, &abgelehnt):
			out["abgelehnt"] = abgelehnt.text
		case errors.Is(err, errVerifierOhneWirkung):
			out["abgelehnt"] = "no effect"
		case errors.As(err, &leiter):
			out["hinweis"] = "collected; this node is not accepting right now -- it will be submitted with the next vote"
		default:
			fmt.Printf("[VERIFIER] Einreichen fehlgeschlagen: %v\n", err)
			out["hinweis"] = "collected; submitting failed, retried with the next vote"
		}
		return out
	}
	return map[string]interface{}{"eingereicht": false}
}

// handleVerifierStimme: POST {"art", "wallet", "stimme": {"zeit", "bescheinigung"}}.
func (a *APIServer) handleVerifierStimme(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	now := time.Now().Unix()
	if !verifierRegisterAktiv(now) {
		jsonError(w, "verifier register not active yet", http.StatusConflict)
		return
	}
	var req struct {
		Art    string         `json:"art"`
		Wallet string         `json:"wallet"`
		Stimme VerifierStimme `json:"stimme"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, verifierStimmeKoerperMax))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	wallet := strings.ToLower(strings.TrimSpace(req.Wallet))
	reg := a.state.verifierRegisterJetzt()
	if err := verifierStimmeVorpruefen(req.Art, wallet, &req.Stimme, reg, now); err != nil {
		jsonError(w, "invalid vote: "+err.Error(), http.StatusForbidden)
		return
	}
	key := req.Art + "|" + wallet
	verifierSammel.mu.Lock()
	verifierSammel.aufraeumenLocked(reg, now)
	s := verifierSammel.offen[key]
	if s == nil {
		if len(verifierSammel.offen) >= verifierSammlungenHoechstens {
			verifierSammel.mu.Unlock()
			jsonError(w, "too many open votes -- please retry later", http.StatusTooManyRequests)
			return
		}
		s = &verifierSammlung{art: req.Art, wallet: wallet, stimmen: map[string]VerifierStimme{}}
		verifierSammel.offen[key] = s
	}
	// Je Pruefer eine Stimme: die juengere ersetzt die aeltere.
	m := req.Stimme.Bescheinigung.Mensch
	if alt, ok := s.stimmen[m]; !ok || req.Stimme.Zeit > alt.Zeit {
		s.stimmen[m] = req.Stimme
	}
	verifierSammel.mu.Unlock()

	out := a.verifierSammelEinreichen(req.Art, wallet, now)
	out["ok"] = true
	json.NewEncoder(w).Encode(out)
}

// handleVerifierAntrag: POST {"wallet", "zeit", "signatur"} -- ein registrierter
// Mensch bewirbt sich als Pruefer.
func (a *APIServer) handleVerifierAntrag(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	now := time.Now().Unix()
	if !verifierRegisterAktiv(now) {
		jsonError(w, "verifier register not active yet", http.StatusConflict)
		return
	}
	var req struct {
		Wallet   string `json:"wallet"`
		Zeit     int64  `json:"zeit"`
		Signatur string `json:"signatur"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, verifierAntragKoerperMax))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	wallet := strings.ToLower(strings.TrimSpace(req.Wallet))
	if !kanonischeAdresse(wallet) {
		jsonError(w, "wallet: 0x + 40 lower-case hex required", http.StatusBadRequest)
		return
	}
	if req.Zeit <= 0 || req.Zeit < now-verifierAntragSpielraum || req.Zeit > now+verifierAntragSpielraum {
		jsonError(w, "zeit expired or from the future", http.StatusBadRequest)
		return
	}
	if verifyPersonalSign(verifierAntragNachricht(wallet, req.Zeit), req.Signatur, wallet) != nil {
		jsonError(w, "signature does not match wallet and time", http.StatusForbidden)
		return
	}
	reg := a.state.verifierRegisterJetzt()
	if reg.mitglied(wallet, now) {
		jsonError(w, "already a verifier", http.StatusConflict)
		return
	}
	// Nach den Unterschriften: das kann die Datenbank kosten.
	if !a.state.IsHuman(wallet) {
		jsonError(w, "not a registered human", http.StatusForbidden)
		return
	}
	verifierSammel.mu.Lock()
	verifierSammel.aufraeumenLocked(reg, now)
	if _, schon := verifierSammel.antraege[wallet]; !schon && len(verifierSammel.antraege) >= verifierAntraegeHoechstens {
		verifierSammel.mu.Unlock()
		jsonError(w, "too many open applications -- please retry later", http.StatusTooManyRequests)
		return
	}
	verifierSammel.antraege[wallet] = now
	verifierSammel.mu.Unlock()
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// handleVerifierAntraege: GET -- offene Antraege und Abstimmungen.
func (a *APIServer) handleVerifierAntraege(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodGet {
		jsonError(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	now := time.Now().Unix()
	reg := a.state.verifierRegisterJetzt()
	type stimmeAus struct {
		Mensch string `json:"mensch"`
		Zeit   int64  `json:"zeit"`
	}
	type abstimmungAus struct {
		Art     string      `json:"art"`
		Wallet  string      `json:"wallet"`
		Stimmen []stimmeAus `json:"stimmen"`
		Noetig  int         `json:"noetig"`
	}
	type antragAus struct {
		Wallet string `json:"wallet"`
		Zeit   int64  `json:"zeit"`
	}
	abst := []abstimmungAus{}
	antr := []antragAus{}
	verifierSammel.mu.Lock()
	verifierSammel.aufraeumenLocked(reg, now)
	for _, s := range verifierSammel.offen {
		x := abstimmungAus{Art: s.art, Wallet: s.wallet, Stimmen: []stimmeAus{}, Noetig: verifierNoetig(reg, s.art, s.wallet, now)}
		for m, st := range s.stimmen {
			x.Stimmen = append(x.Stimmen, stimmeAus{Mensch: m, Zeit: st.Zeit})
		}
		sort.Slice(x.Stimmen, func(i, j int) bool { return x.Stimmen[i].Mensch < x.Stimmen[j].Mensch })
		abst = append(abst, x)
	}
	for wlt, t := range verifierSammel.antraege {
		antr = append(antr, antragAus{Wallet: wlt, Zeit: t})
	}
	verifierSammel.mu.Unlock()
	sort.Slice(abst, func(i, j int) bool {
		if abst[i].Wallet != abst[j].Wallet {
			return abst[i].Wallet < abst[j].Wallet
		}
		return abst[i].Art < abst[j].Art
	})
	sort.Slice(antr, func(i, j int) bool { return antr[i].Zeit < antr[j].Zeit })
	json.NewEncoder(w).Encode(map[string]interface{}{
		"aktiv":        verifierRegisterAktiv(now),
		"pruefer":      reg.mitgliederZu(now),
		"antraege":     antr,
		"abstimmungen": abst,
	})
}
