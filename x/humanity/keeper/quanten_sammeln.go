package keeper

// SAMMELWEG FUER DIE FREISCHALTUNG EINES QUANTENGELD-SATZES
//
// # WARUM
//
// Eine Freischaltung (quanten_geld.go) traegt die Stimmen von zwei Dritteln
// der Pruefer in EINEM Koerper. Mit dem quantensicheren Teil
// (pruefer_pq.go) ist eine Stimme ~11 KB; 171 Stimmen (zwei Drittel von 256)
// waeren ~2 MB -- mehr als quantenKoerperMax und die Weiterleitung zum Leiter
// (zumLeiterRumpfHoechstens) tragen. Ohne Sammelweg haette die Pflicht des
// PQ-Teils die Freischaltung unmoeglich gemacht.
//
// # WIE (wie die Pruefer-Abstimmung, verifier_sammeln.go)
//
// POST /api/quanten/stimme nimmt EINE Stimme fuer einen Vorschlag
// (Satz, Ordnung, ab). Der Leiter sammelt; sobald zwei Drittel der Pruefer,
// mindestens quantenFreischaltungMindestPruefer verschiedene, zugestimmt
// haben, baut er die Freischaltung selbst und schickt sie durch
// dasselbe Annahme-Tor wie den Endpunkt mit allen Stimmen
// (quantenFreischaltungAnnehmen). Jeder Knoten prueft die Freischaltung
// beim Nachspielen ganz -- der Sammelweg verschiebt nur, wo die Stimmen
// zusammenkommen, er laesst keine Pruefung weg.
//
// # VORAB (billig vor teuer, vor jedem Speichern)
//
// Form des Vorschlags (Satz, N >= 2^256, kein Labor-/Testsatz, ab mit
// Vorlauf), Zeit der Stimme, Stimmrecht laut Register, Ed25519 ueber genau
// diesen Vorschlag, Besitznachweis und Freigabe des Schluessels, PQ-Teil
// (ab dem Stichtag Pflicht). Was nur mit der Datenbank geht (Mensch
// registriert, keine offene Staffel, Validator-Schluessel zur Stimmzeit),
// prueft das Annahme-Tor; scheitert dort genau eine Stimme, faellt sie heraus,
// der Rest bleibt.
//
// # GRENZEN
//
// Hoechstens quantenSammlungenHoechstens offene Vorschlaege, je Pruefer
// hoechstens quantenSammlungenJePruefer; je Vorschlag eine Stimme je Mensch
// (die juengere ersetzt die aeltere), also hoechstens verifierHoechstens.
// Alles verfaellt nach verifierStimmeHoechstensAlt. Koerper hoechstens
// quantenStimmeKoerperMax, und jede Anfrage zaehlt in der Grenze je Adresse.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	quantenSammlungenHoechstens = 4
	quantenSammlungenJePruefer  = 2
	// Eine Stimme mit PQ-Teil und die Angaben des Vorschlags.
	quantenStimmeKoerperMax = 16 << 10
)

type quantenVorschlag struct {
	Satz    string
	Ordnung string
	Ab      int64
}

func (v quantenVorschlag) schluessel() string {
	return v.Satz + "|" + v.Ordnung + "|" + strconv.FormatInt(v.Ab, 10)
}

type quantenSammlung struct {
	v       quantenVorschlag
	stimmen map[string]VerifierStimme // Mensch -> seine juengste Stimme
}

type quantenSammelstelle struct {
	mu    sync.Mutex
	offen map[string]*quantenSammlung
}

var quantenSammel = &quantenSammelstelle{offen: map[string]*quantenSammlung{}}

func (q *quantenSammelstelle) aufraeumenLocked(now int64) {
	for k, s := range q.offen {
		for m, st := range s.stimmen {
			if now-st.Zeit > verifierStimmeHoechstensAlt {
				delete(s.stimmen, m)
			}
		}
		if len(s.stimmen) == 0 {
			delete(q.offen, k)
		}
	}
}

// quantenVorschlagVorpruefen: die Form des Vorschlags -- dieselben Regeln wie
// quantenFreischaltungUrteilLocked, mit der Uhrzeit als Beschlusszeit.
func quantenVorschlagVorpruefen(v quantenVorschlag, now int64) error {
	if !quantenSatzGueltig(v.Satz) || strings.HasPrefix(v.Satz, "labor") || strings.HasPrefix(v.Satz, "test") {
		return fmt.Errorf("parameter set name not allowed")
	}
	if _, ok := quantenOrdnungGueltig(v.Ordnung); !ok {
		return errors.New("group order invalid or below 2^256")
	}
	if v.Ab < sattAdd(now, quantenFreischaltungVorlauf) {
		return fmt.Errorf("ab must be at least %d s after the decision", quantenFreischaltungVorlauf)
	}
	return nil
}

// quantenStimmeVorpruefen: was ohne Datenbank pruefbar ist, wie
// verifierStimmeVorpruefen.
func quantenStimmeVorpruefen(v quantenVorschlag, s *VerifierStimme, reg *verifierRegister, now int64, zusage func(mensch string) (string, bool)) error {
	if err := quantenVorschlagVorpruefen(v, now); err != nil {
		return err
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
	if !reg.mitglied(b.Mensch, now) {
		return errors.New("voter is not a verifier")
	}
	satz := quantenFreischaltungNachricht(v.Satz, v.Ordnung, v.Ab)("", s.Zeit)
	if !ed25519PruefenStreng(b.PublicKey, b.Signature, []byte(satz)) {
		return errors.New("vote signature does not match set, order, ab and time")
	}
	if !verifyCoordinatorPossession(b.PublicKey, b.SchluesselSig, b.Mensch) {
		return errors.New("key possession proof missing or wrong")
	}
	if !kanonischeSignatur(b.MenschSig) || verifyPersonalSign(coordinatorFreigabeNachricht(b.PublicKey), b.MenschSig, b.Mensch) != nil {
		return errors.New("human's approval of the key missing or wrong")
	}
	if s.PQ != nil {
		s.PQ.Pub = strings.ToLower(strings.TrimSpace(s.PQ.Pub))
		s.PQ.Sig = strings.ToLower(strings.TrimSpace(s.PQ.Sig))
	}
	return prueferPQPruefen(s, satz, prueferPQPflicht(now), zusage)
}

// handleQuantenStimme: POST {"satz", "ordnung", "ab", "stimme"}.
func (a *APIServer) handleQuantenStimme(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	now := time.Now().Unix()
	if !quantenGeldAktiv(now) {
		jsonError(w, "quantum money not active", http.StatusConflict)
		return
	}
	if !burstErlaubt("quanten-stimme:"+erneuerungsAbsender(r), burstQuantenJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	var req struct {
		Satz    string         `json:"satz"`
		Ordnung string         `json:"ordnung"`
		Ab      int64          `json:"ab"`
		Stimme  VerifierStimme `json:"stimme"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, quantenStimmeKoerperMax))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	v := quantenVorschlag{Satz: strings.TrimSpace(req.Satz), Ordnung: strings.TrimSpace(req.Ordnung), Ab: req.Ab}
	reg := a.state.verifierRegisterJetzt()
	if err := quantenStimmeVorpruefen(v, &req.Stimme, reg, now, a.state.prueferZusageVon); err != nil {
		jsonError(w, "invalid vote: "+err.Error(), http.StatusForbidden)
		return
	}
	if code, err := quantenSammel.ablegen(v, req.Stimme, now); err != nil {
		jsonError(w, err.Error(), code)
		return
	}
	out := a.quantenSammelEinreichen(v, now)
	out["ok"] = true
	json.NewEncoder(w).Encode(out)
}

// ablegen: die Stimme in ihre Sammlung (mit den Grenzen).
func (q *quantenSammelstelle) ablegen(v quantenVorschlag, st VerifierStimme, now int64) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.aufraeumenLocked(now)
	key := v.schluessel()
	s := q.offen[key]
	if s == nil {
		if len(q.offen) >= quantenSammlungenHoechstens {
			return http.StatusTooManyRequests, errors.New("too many open proposals -- please retry later")
		}
		s = &quantenSammlung{v: v, stimmen: map[string]VerifierStimme{}}
		q.offen[key] = s
	}
	m := st.Bescheinigung.Mensch
	if _, schon := s.stimmen[m]; !schon {
		n := 0
		for _, x := range q.offen {
			if _, ok := x.stimmen[m]; ok {
				n++
			}
		}
		if n >= quantenSammlungenJePruefer {
			if len(s.stimmen) == 0 {
				delete(q.offen, key)
			}
			return http.StatusTooManyRequests, errors.New("too many open proposals from this verifier -- please retry later")
		}
	}
	if alt, ok := s.stimmen[m]; !ok || st.Zeit > alt.Zeit {
		s.stimmen[m] = st
	}
	return 0, nil
}

// quantenSammelEinreichen: genug Stimmen -> die Freischaltung einreichen.
// Scheitert genau eine Stimme, faellt sie heraus (hoechstens so viele
// Versuche, wie Stimmen da sind).
func (a *APIServer) quantenSammelEinreichen(v quantenVorschlag, now int64) map[string]interface{} {
	key := v.schluessel()
	for versuch := 0; versuch <= verifierStimmenHoechstens; versuch++ {
		reg := a.state.verifierRegisterJetzt()
		quantenSammel.mu.Lock()
		var stimmen []VerifierStimme
		if s := quantenSammel.offen[key]; s != nil {
			for _, st := range s.stimmen {
				stimmen = append(stimmen, st)
			}
		}
		quantenSammel.mu.Unlock()
		noetig := quantenStimmenNoetig(len(reg.mitgliederZu(now)))
		out := map[string]interface{}{"stimmen": len(stimmen), "noetig": noetig}
		if len(stimmen) < noetig {
			out["eingereicht"] = false
			return out
		}
		sort.Slice(stimmen, func(i, j int) bool { return stimmen[i].Bescheinigung.Mensch < stimmen[j].Bescheinigung.Mensch })
		if len(stimmen) > verifierStimmenHoechstens {
			stimmen = stimmen[:verifierStimmenHoechstens]
		}
		tx := Transaction{Type: quantenFreischaltungTxArt, Wallet: stimmen[0].Bescheinigung.Mensch, DistributionAt: now,
			Quanten: &QuantenAngaben{Satz: v.Satz, Ordnung: v.Ordnung, Ab: v.Ab, Stimmen: stimmen}}
		err := a.quantenFreischaltungAnnehmen(tx, now)
		if err == nil {
			quantenSammel.mu.Lock()
			delete(quantenSammel.offen, key)
			quantenSammel.mu.Unlock()
			out["eingereicht"] = true
			return out
		}
		var abgelehnt *quantenAbgelehnt
		if errors.As(err, &abgelehnt) && abgelehnt.stimme >= 1 && abgelehnt.stimme <= len(stimmen) {
			schlecht := stimmen[abgelehnt.stimme-1].Bescheinigung.Mensch
			quantenSammel.mu.Lock()
			if s := quantenSammel.offen[key]; s != nil {
				delete(s.stimmen, schlecht)
				if len(s.stimmen) == 0 {
					delete(quantenSammel.offen, key)
				}
			}
			quantenSammel.mu.Unlock()
			fmt.Printf("[QUANTEN] Stimme von %s verworfen: %s\n", kurzAdresse(schlecht), abgelehnt.text)
			continue
		}
		out["eingereicht"] = false
		if errors.As(err, &abgelehnt) {
			// Der Vorschlag selbst taugt nicht (mehr) -- etwa der Satz ist
			// schon eingetragen oder der Vorlauf reicht nicht: weg damit.
			quantenSammel.mu.Lock()
			delete(quantenSammel.offen, key)
			quantenSammel.mu.Unlock()
			out["abgelehnt"] = abgelehnt.text
		} else {
			out["hinweis"] = "collected; this node is not accepting right now -- it will be submitted with the next vote"
		}
		return out
	}
	return map[string]interface{}{"eingereicht": false}
}

// handleQuantenAbstimmungen: GET -- die offenen Vorschlaege mit Stimmenzahl.
func (a *APIServer) handleQuantenAbstimmungen(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	now := time.Now().Unix()
	noetig := quantenStimmenNoetig(len(a.state.verifierRegisterJetzt().mitgliederZu(now)))
	quantenSammel.mu.Lock()
	quantenSammel.aufraeumenLocked(now)
	liste := make([]map[string]interface{}, 0, len(quantenSammel.offen))
	for _, s := range quantenSammel.offen {
		liste = append(liste, map[string]interface{}{"satz": s.v.Satz, "ordnung": s.v.Ordnung, "ab": s.v.Ab,
			"stimmen": len(s.stimmen), "noetig": noetig})
	}
	quantenSammel.mu.Unlock()
	sort.Slice(liste, func(i, j int) bool { return fmt.Sprint(liste[i]["satz"]) < fmt.Sprint(liste[j]["satz"]) })
	json.NewEncoder(w).Encode(map[string]interface{}{"abstimmungen": liste})
}
