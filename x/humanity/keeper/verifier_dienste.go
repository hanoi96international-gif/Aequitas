package keeper

// DIENSTE DER PRUEFER: SELBST ANGEMELDET, VON JEDEM NUTZER SELBST GEPRUEFT (A4).
//
// # WARUM
//
// Ein neuer Vergleichsdienst wurde bisher von Hand aufgenommen: drei Zeilen in
// die Telegram-Gruppe, der Betreiber traegt die Adresse in VALIDATOR_URLS und
// den Schluessel in eine Liste ein. Das ist eine Genehmigung durch einen
// Menschen -- genau das, was das Pruefer-Register (verifier_register.go)
// abschaffen soll.
//
// Jetzt meldet sich jeder Dienst eines Pruefers selbst an:
//
//	POST /api/verifier/dienst {"rolle", "public_key", "url", "zeit", "signatur"}
//
// mit einer Ed25519-Unterschrift seines Schluessels ueber
//
//	aequitas-dienst-v1|chain:<id>|<rolle>|<url>|<zeit>
//
// rolle ist "vergleichsdienst" (Gesichtsabgleich, bezeugt Menschen) oder
// "coordinator" (Eingang, bindet die Wallet). Der Schluessel muss an einen
// Menschen gebunden sein -- dieselbe Bindung wie jede Stimme und jede
// Bescheinigung (Freigabe der Wallet "authorize coordinator", Besitznachweis
// "coordinator key for human"). Die Bindung kommt mit der Anmeldung oder aus
// dem Coordinator-Register dieses Knotens (eingetragen per App,
// coordinator-binden). Und der Mensch muss JETZT Pruefer sein.
//
// GET /api/verifier/dienste liefert die gueltigen Anmeldungen MIT Bindung und
// Unterschrift: Coordinatoren, Vergleichsdienste und der Proof-Server pruefen
// sie selbst nach und glauben dem Knoten nur, wer Pruefer ist.
//
// # WAS DIE LISTE NICHT IST
//
// Kein Konsens. Fuer die Bescheinigung im Konsens (K1) reist die Bindung jedes
// Schluessels in der Registrierung selbst mit (bio_bescheinigung_register.go),
// und jeder Knoten prueft sie gegen das Pruefer-Register. Diese Liste sagt nur,
// WO die Dienste der Pruefer zu erreichen sind und welche Bindung sie haben.
//
// # GRENZEN
//
// Hoechstens dienstJeMensch Anmeldungen je Mensch (ein Coordinator und
// Vergleichsdienste), hoechstens dienstHoechstens insgesamt. Eine Anmeldung
// gilt dienstGueltig lang und wird vom Dienst selbst laufend erneuert -- wer
// abschaltet, faellt von selbst heraus. Eine URL gehoert genau einem
// Schluessel. Wer nicht mehr Pruefer ist, faellt sofort heraus. Koerper
// hoechstens 4 KiB, Grenze je Adresse (verifierGrenze).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	dienstRolleVergleich         = "vergleichsdienst"
	dienstRolleCoordinator       = "coordinator"
	dienstDomain                 = "aequitas-dienst-v1"
	dienstJeMensch               = 4
	dienstHoechstens             = dienstJeMensch * verifierHoechstens
	dienstGueltig          int64 = 1800
	dienstSpielraum        int64 = 600
	dienstKoerperMax             = 4 << 10
	dienstURLMax                 = 200
)

// dienstNachricht: was der Schluessel eines Dienstes fuer seine Anmeldung
// unterschreibt.
func dienstNachricht(rolle, u string, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d", dienstDomain, eingefroreneChainID, rolle, u, zeit)
}

// DienstAnmeldung: eine gueltige Anmeldung, wie GET sie ausgibt.
type DienstAnmeldung struct {
	Rolle         string `json:"rolle"`
	PublicKey     string `json:"public_key"`
	URL           string `json:"url"`
	Zeit          int64  `json:"zeit"`
	Signatur      string `json:"signatur"`
	Mensch        string `json:"mensch"`
	MenschSig     string `json:"mensch_sig"`
	SchluesselSig string `json:"schluessel_sig"`
}

type dienstStelle struct {
	mu    sync.Mutex
	nachK map[string]DienstAnmeldung // public_key -> Anmeldung
}

var dienstListe = &dienstStelle{nachK: map[string]DienstAnmeldung{}}

// dienstURL: eine oeffentliche https-Adresse ohne Benutzerteil, Query und
// Fragment, klein, ohne Schraegstrich am Ende. "" wenn sie nicht taugt.
func dienstURL(roh string) string {
	v := strings.TrimRight(strings.TrimSpace(roh), "/")
	if v == "" || len(v) > dienstURLMax || !strings.HasPrefix(v, "https://") {
		return ""
	}
	p, err := url.Parse(v)
	if err != nil || p.User != nil || p.RawQuery != "" || p.Fragment != "" || p.Host == "" || p.Opaque != "" {
		return ""
	}
	if !isAllowedPeerURL(v) {
		return ""
	}
	if p.Scheme+"://"+strings.ToLower(p.Host)+p.EscapedPath() != v && p.Scheme+"://"+strings.ToLower(p.Host)+p.EscapedPath() != strings.ToLower(v) {
		return ""
	}
	return "https://" + strings.ToLower(p.Host) + p.EscapedPath()
}

// bindungGueltig: Freigabe des Menschen (EIP-191, kanonisch) und Besitznachweis
// (Ed25519 streng) fuer pub -- dieselbe Pruefung wie in bescheinigungPruefenFuer.
func bindungGueltig(pub, mensch, menschSig, schluesselSig string) bool {
	if !ed25519HexTauglich(pub) || !kanonischeAdresse(mensch) {
		return false
	}
	if !verifyCoordinatorPossession(pub, schluesselSig, mensch) {
		return false
	}
	return kanonischeSignatur(menschSig) && verifyPersonalSign(coordinatorFreigabeNachricht(pub), menschSig, mensch) == nil
}

// aufraeumenLocked: Abgelaufenes und Nicht-mehr-Pruefer weg.
func (d *dienstStelle) aufraeumenLocked(reg *verifierRegister, now int64) {
	for k, e := range d.nachK {
		if now-e.Zeit > dienstGueltig || !reg.mitglied(e.Mensch, now) {
			delete(d.nachK, k)
		}
	}
}

// DiensteJetzt: die gueltigen Anmeldungen, sortiert (Rolle, Mensch, URL).
func (cs *ChainState) DiensteJetzt(now int64) []DienstAnmeldung {
	reg := cs.verifierRegisterJetzt()
	dienstListe.mu.Lock()
	dienstListe.aufraeumenLocked(reg, now)
	out := make([]DienstAnmeldung, 0, len(dienstListe.nachK))
	for _, e := range dienstListe.nachK {
		out = append(out, e)
	}
	dienstListe.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Rolle != out[j].Rolle {
			return out[i].Rolle < out[j].Rolle
		}
		if out[i].Mensch != out[j].Mensch {
			return out[i].Mensch < out[j].Mensch
		}
		return out[i].URL < out[j].URL
	})
	return out
}

var errDienstAbgelehnt = errors.New("dienst abgelehnt")

// handleVerifierDienst: POST eine Anmeldung.
func (a *APIServer) handleVerifierDienst(w http.ResponseWriter, r *http.Request) {
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
		Rolle         string `json:"rolle"`
		PublicKey     string `json:"public_key"`
		URL           string `json:"url"`
		Zeit          int64  `json:"zeit"`
		Signatur      string `json:"signatur"`
		Mensch        string `json:"mensch,omitempty"`
		MenschSig     string `json:"mensch_sig,omitempty"`
		SchluesselSig string `json:"schluessel_sig,omitempty"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, dienstKoerperMax))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Rolle != dienstRolleVergleich && req.Rolle != dienstRolleCoordinator {
		jsonError(w, "rolle: vergleichsdienst or coordinator", http.StatusBadRequest)
		return
	}
	pub := strings.ToLower(strings.TrimSpace(req.PublicKey))
	if !ed25519HexTauglich(pub) {
		jsonError(w, "public_key: 64 hex characters (Ed25519)", http.StatusBadRequest)
		return
	}
	u := dienstURL(req.URL)
	if u == "" || u != strings.TrimRight(strings.TrimSpace(req.URL), "/") {
		jsonError(w, "url: public https:// address in its one spelling (lower-case host, no query)", http.StatusBadRequest)
		return
	}
	if req.Zeit <= 0 || req.Zeit < now-dienstSpielraum || req.Zeit > now+dienstSpielraum {
		jsonError(w, "zeit expired or from the future", http.StatusBadRequest)
		return
	}
	sig := ed25519SigNormal(req.Signatur)
	if !ed25519PruefenStreng(pub, sig, []byte(dienstNachricht(req.Rolle, u, req.Zeit))) {
		jsonError(w, "signature does not match rolle, url and zeit", http.StatusForbidden)
		return
	}
	// Die Bindung: mitgeschickt, sonst aus dem Coordinator-Register dieses
	// Knotens. Geprueft wird sie in jedem Fall hier.
	b := CoordinatorBindung{Mensch: strings.ToLower(strings.TrimSpace(req.Mensch)),
		MenschSig: kanonischeSignaturVersuch(req.MenschSig), SchluesselSig: ed25519SigNormal(req.SchluesselSig)}
	if b.Mensch == "" && req.MenschSig == "" && req.SchluesselSig == "" {
		ctx, abbruch := context.WithTimeout(r.Context(), 5*time.Second)
		lokal, ok := a.state.CoordinatorBindungLokalCtx(ctx, pub)
		abbruch()
		if !ok {
			jsonError(w, "key not bound to a human -- approve it in the app first (node tab, verifier)", http.StatusForbidden)
			return
		}
		b = CoordinatorBindung{Mensch: strings.ToLower(lokal.Mensch), MenschSig: kanonischeSignaturVersuch(lokal.MenschSig),
			SchluesselSig: ed25519SigNormal(lokal.SchluesselSig)}
	}
	if !bindungGueltig(pub, b.Mensch, b.MenschSig, b.SchluesselSig) {
		jsonError(w, "binding of the key to its human is missing or wrong", http.StatusForbidden)
		return
	}
	reg := a.state.verifierRegisterJetzt()
	if !reg.mitglied(b.Mensch, now) {
		jsonError(w, "the human behind this key is not a verifier", http.StatusForbidden)
		return
	}
	e := DienstAnmeldung{Rolle: req.Rolle, PublicKey: pub, URL: u, Zeit: req.Zeit, Signatur: sig,
		Mensch: b.Mensch, MenschSig: b.MenschSig, SchluesselSig: b.SchluesselSig}

	dienstListe.mu.Lock()
	defer dienstListe.mu.Unlock()
	dienstListe.aufraeumenLocked(reg, now)
	alt, hatte := dienstListe.nachK[pub]
	if hatte && alt.Zeit >= req.Zeit {
		// Nur Neueres ersetzt: eine alte Anmeldung (abgefangen) dreht nichts zurueck.
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "gueltig_bis": alt.Zeit + dienstGueltig})
		return
	}
	jeMensch := 0
	for k, x := range dienstListe.nachK {
		if x.URL == u && k != pub {
			if x.Mensch != b.Mensch {
				jsonError(w, "this url is announced by another verifier's key", http.StatusConflict)
				return
			}
		}
		if x.Mensch == b.Mensch && k != pub {
			jeMensch++
		}
	}
	if !hatte && (jeMensch >= dienstJeMensch || len(dienstListe.nachK) >= dienstHoechstens) {
		jsonError(w, "too many announced services", http.StatusTooManyRequests)
		return
	}
	// Dieselbe URL unter einem anderen Schluessel DESSELBEN Menschen: der neue
	// ersetzt den alten (Schluesselwechsel).
	for k, x := range dienstListe.nachK {
		if x.URL == u && k != pub {
			delete(dienstListe.nachK, k)
		}
	}
	dienstListe.nachK[pub] = e
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "gueltig_bis": e.Zeit + dienstGueltig})
}

// handleVerifierDienste: GET die gueltigen Anmeldungen.
func (a *APIServer) handleVerifierDienste(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodGet {
		jsonError(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	now := time.Now().Unix()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"aktiv":            verifierRegisterAktiv(now),
		"register_k1_ab":   bescheinigungRegisterAb(),
		"gueltig_sekunden": dienstGueltig,
		"dienste":          a.state.DiensteJetzt(now),
	})
}
