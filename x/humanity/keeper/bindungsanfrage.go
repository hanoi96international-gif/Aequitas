package keeper

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// BINDUNGSANFRAGEN: DER SERVER FRAGT, DIE APP BESTAETIGT.
//
// Gemeldet am 02.10.2026: der Bindungs-QR-Code am Ende von einrichten.sh
// taugt fuer Laien nicht -- man hat ihn auf dem Handy, das ihn scannen soll,
// oder die App auf einem anderen Geraet als den Browser. Jetzt laeuft es
// umgekehrt:
//
//  1. einrichten.sh holt den Schluesselnachweis seines Knotens
//     (/api/validator-selfproof, nur fuer NODE_OPERATOR_WALLET) und meldet
//     ihn hier an: POST /api/bindungsanfrage.
//  2. Die App fragt mit ihrer eigenen Wallet nach
//     (GET /api/bindungsanfragen?wallet=0x...) und zeigt "Dein neuer Server
//     (IP ...) moechte sich mit dir verbinden" -- ein Tipp auf Bestaetigen.
//  3. Die App unterschreibt wie bisher und reicht ueber
//     /api/register-validator-key ein; einrichten.sh holt die fertige
//     Bindung bei /api/validator-binding ab.
//
// Die Unterschrift des Menschen bleibt Pflicht -- ein Server, der sich selbst
// an eine Wallet bindet, waere eine Bindung ohne Zustimmung. Neu ist nur, wie
// die Anfrage zur App kommt.
//
// MISSBRAUCH. Jeder kann einen Knoten aufsetzen und dessen NODE_OPERATOR_WALLET
// auf eine fremde Wallet stellen; die Anfrage erschiene dann in fremder App.
// Deshalb:
//   - nur mit gueltigem Schluesselnachweis (Signatur der Signieradresse ueber
//     "Aequitas: validator key linked to human <wallet>"),
//   - nur fuer registrierte Menschen,
//   - die App zeigt die IP, von der die Anfrage kam (die des Servers) und
//     verlangt einen bewussten Tipp; abgelehnte Anfragen verschwinden,
//   - je Wallet hoechstens bindungsAnfragenJeWallet, insgesamt hoechstens
//     bindungsAnfragenMax, jede verfaellt nach bindungsAnfrageDauer,
//   - je IP hoechstens bindungsAnfrageJeIP Anfragen je Minute.
// Nichts wird gespeichert oder weitergegeben; ein Neustart leert die Liste,
// einrichten.sh meldet dann einfach erneut an.

const (
	bindungsAnfrageDauer     = 30 * time.Minute
	bindungsAnfragenJeWallet = 3
	bindungsAnfragenMax      = 1024
	bindungsAnfrageJeIP      = 6
)

type bindungsAnfrage struct {
	Adresse string    `json:"signing_address"`
	Wallet  string    `json:"wallet"`
	Beweis  string    `json:"beweis"`
	IP      string    `json:"ip"`
	Zeit    time.Time `json:"zeit"`
}

type bindungsAnfragenTyp struct {
	mu      sync.Mutex
	eintrag map[string]bindungsAnfrage // Signieradresse -> Anfrage
}

var bindungsAnfragen = &bindungsAnfragenTyp{eintrag: map[string]bindungsAnfrage{}}

func (b *bindungsAnfragenTyp) aufraeumenLocked(jetzt time.Time) {
	for a, e := range b.eintrag {
		if jetzt.Sub(e.Zeit) > bindungsAnfrageDauer {
			delete(b.eintrag, a)
		}
	}
}

// merke legt eine GEPRUEFTE Anfrage ab (Aufrufer hat den Beweis verifiziert).
func (b *bindungsAnfragenTyp) merke(a bindungsAnfrage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.aufraeumenLocked(a.Zeit)
	delete(b.eintrag, a.Adresse)
	// Je Wallet: die aelteste weicht.
	var gleich []bindungsAnfrage
	for _, e := range b.eintrag {
		if e.Wallet == a.Wallet {
			gleich = append(gleich, e)
		}
	}
	sort.Slice(gleich, func(i, j int) bool { return gleich[i].Zeit.Before(gleich[j].Zeit) })
	for i := 0; i <= len(gleich)-bindungsAnfragenJeWallet; i++ {
		delete(b.eintrag, gleich[i].Adresse)
	}
	for len(b.eintrag) >= bindungsAnfragenMax {
		aeltester, t := "", a.Zeit
		for k, e := range b.eintrag {
			if aeltester == "" || e.Zeit.Before(t) {
				aeltester, t = k, e.Zeit
			}
		}
		delete(b.eintrag, aeltester)
	}
	b.eintrag[a.Adresse] = a
}

func (b *bindungsAnfragenTyp) fuer(wallet string, jetzt time.Time) []bindungsAnfrage {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.aufraeumenLocked(jetzt)
	out := []bindungsAnfrage{}
	for _, e := range b.eintrag {
		if e.Wallet == wallet {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Zeit.After(out[j].Zeit) })
	return out
}

// erledigt: nach einer eingereichten Bindung oder auf Ablehnung.
func (b *bindungsAnfragenTyp) erledigt(adresse string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.eintrag, adresse)
}

func istSignatur65(s string) bool {
	return len(s) == 132 && strings.HasPrefix(s, "0x") && strings.Trim(strings.ToLower(s[2:]), "0123456789abcdef") == ""
}

// POST /api/bindungsanfrage {signing_address, wallet, beweis}
func (a *APIServer) handleBindungsanfrage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	ip := clientIP(r)
	if !burstErlaubt("bindungsanfrage:"+ip, bindungsAnfrageJeIP, burstFenster) {
		jsonError(w, "too many requests, try again in a minute", http.StatusTooManyRequests)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<10)
	var req struct {
		Adresse string `json:"signing_address"`
		Wallet  string `json:"wallet"`
		Beweis  string `json:"beweis"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	adr := strings.ToLower(strings.TrimSpace(req.Adresse))
	wallet := strings.ToLower(strings.TrimSpace(req.Wallet))
	beweis := strings.TrimSpace(req.Beweis)
	if !istHexAdresse(adr) || !istHexAdresse(wallet) || !istSignatur65(beweis) {
		jsonError(w, "signing_address, wallet (0x addresses) and beweis (65-byte signature) required", http.StatusBadRequest)
		return
	}
	// Der Knoten muss den Schluessel der Signieradresse besitzen.
	if err := verifyPersonalSign("Aequitas: validator key linked to human "+wallet, beweis, adr); err != nil {
		jsonError(w, "beweis is not a signature of signing_address over \"Aequitas: validator key linked to human <wallet>\"", http.StatusBadRequest)
		return
	}
	if a.state == nil || !a.state.IsHuman(wallet) {
		jsonError(w, "wallet is not a registered human", http.StatusForbidden)
		return
	}
	bindungsAnfragen.merke(bindungsAnfrage{Adresse: adr, Wallet: wallet, Beweis: beweis, IP: ip, Zeit: time.Now()})
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "laeuft_ab_in_s": int(bindungsAnfrageDauer.Seconds())})
}

// GET /api/bindungsanfragen?wallet=0x...  -- offene Anfragen fuer diese Wallet.
// DELETE /api/bindungsanfragen?signing_address=0x...&wallet=0x...&signatur=0x...
// -- Ablehnung, unterschrieben von der Wallet ("Aequitas: reject validator <adr>"),
// damit niemand fremde Anfragen wegraeumt.
func (a *APIServer) handleBindungsanfragen(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	q := r.URL.Query()
	wallet := strings.ToLower(strings.TrimSpace(q.Get("wallet")))
	if !istHexAdresse(wallet) {
		jsonError(w, "wallet must be a 0x address", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		json.NewEncoder(w).Encode(map[string]interface{}{"anfragen": bindungsAnfragen.fuer(wallet, time.Now())})
	case http.MethodDelete:
		adr := strings.ToLower(strings.TrimSpace(q.Get("signing_address")))
		sig := strings.TrimSpace(q.Get("signatur"))
		if !istHexAdresse(adr) || !istSignatur65(sig) {
			jsonError(w, "signing_address and signatur required", http.StatusBadRequest)
			return
		}
		if err := verifyPersonalSign("Aequitas: reject validator "+adr, sig, wallet); err != nil {
			jsonError(w, "signatur is not from this wallet", http.StatusForbidden)
			return
		}
		for _, e := range bindungsAnfragen.fuer(wallet, time.Now()) {
			if e.Adresse == adr {
				bindungsAnfragen.erledigt(adr)
			}
		}
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	default:
		jsonError(w, "GET or DELETE", http.StatusMethodNotAllowed)
	}
}
