package keeper

// Quantensicherheit Stufe 2 (docs/QUANTENSICHERHEIT.md): die PQ-Zusage einer
// Wallet.
//
// WAS. Ein Mensch hinterlegt EINMAL fuer seine Wallet den SHA-256 seines
// ML-DSA-65-Schluessels (pq_zusage), unterschrieben mit der Wallet (EIP-191).
// Heute bewirkt die Zusage nichts. Bricht spaeter ein Quantenrechner
// secp256k1, ist sie der Beweis, wem die Wallet gehoert: wer den passenden
// ML-DSA-Schluessel zeigt, zieht das Guthaben auf eine quantensichere Adresse
// um (Stufe 4, Notbremse); ein Angreifer, der nur den secp256k1-Schluessel
// abgeleitet hat, kann das nicht.
//
// WOHER DER SCHLUESSEL KOMMT (App, lib/pq.ts). Aus der Seed-Phrase ueber einen
// eigenen Weg (HMAC-SHA256 ueber den BIP-39-Seed), NIE aus dem
// secp256k1-Schluessel -- sonst leitete ein Quantenrechner beide ab. Der
// Mensch muss nichts Neues sichern: die Seed-Phrase genuegt.
//
// REGELN (jeder Knoten prueft selbst, wie beim Wiederherstellungscode):
//   - erst ab pqZusageAbUnix (davor ist die Art unbekannt, der Endpunkt 409);
//   - nur ein Mensch (is_human) -- das begrenzt das Register auf einen
//     Eintrag je Mensch und Wallet, ohne Gebuehr;
//   - genau EINE Zusage je Wallet, nicht aenderbar. Sonst koennte ein
//     Angreifer, der den secp256k1-Schluessel einmal abgeleitet hat, die
//     Zusage des Menschen durch seine eigene ersetzen. Wer gar keine hat,
//     dem kann ein solcher Angreifer zuvorkommen -- deshalb zaehlen bei der
//     Notbremse nur Zusagen von vor ihrem Stichtag (Stufe 4), und die App
//     hinterlegt die Zusage gleich bei der Einrichtung;
//   - unterschrieben hoechstens 5 min nach und einen Tag vor dem Block.
//
// SPEICHER. Im Umzugs-Register (Feld "pq", sortiert nach Wallet), also in
// chain_config und mit dessen Hash in der StateRoot. Leer: Register und
// Wurzel byte-gleich wie vorher. Hoechstens pqZusagenHoechstens Eintraege.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const (
	// pqZusageAbUnix: 2026-11-17 18:00 UTC.
	pqZusageAbUnix int64 = 1794938400

	pqZusageTxArt   = "pq_zusage"
	pqDomaeneZusage = "aequitas-pq-zusage-v1"

	// Wie umzugCodesHoechstens: bei dieser Zahl muss das Register in eine
	// eigene Tabelle (je Aenderung wird es ganz neu geschrieben).
	pqZusagenHoechstens = 50000
)

// pqZusageAbOverride: nur fuer Tests (0 = Konstante gilt).
var pqZusageAbOverride atomic.Int64

func pqZusageAb() int64 {
	if v := pqZusageAbOverride.Load(); v != 0 {
		return v
	}
	return pqZusageAbUnix
}

func pqZusageAktiv(t int64) bool { return t >= pqZusageAb() }

// pqZusageNachricht: was die Wallet unterschreibt (EIP-191), an die Kette
// gebunden wie die Umzugs-Nachrichten.
func pqZusageNachricht(wallet, hash string, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d", pqDomaeneZusage, eingefroreneChainID, wallet, hash, zeit)
}

// pqZusage: die Zusage einer Wallet.
type pqZusage struct {
	Wallet string `json:"w"`
	Hash   string `json:"h"`
	Zeit   int64  `json:"z"`
}

func (r *umzugRegister) pqZusage(w string) (pqZusage, bool) {
	i := sort.Search(len(r.PQ), func(i int) bool { return r.PQ[i].Wallet >= w })
	if i < len(r.PQ) && r.PQ[i].Wallet == w {
		return r.PQ[i], true
	}
	return pqZusage{}, false
}

// mitPQZusage: fuegt z ein; eine vorhandene Zusage derselben Wallet bleibt
// (das Urteil laesst keine zweite zu -- hier nur zur Sicherheit).
func (r *umzugRegister) mitPQZusage(z pqZusage) *umzugRegister {
	k := r.kopie()
	i := sort.Search(len(k.PQ), func(i int) bool { return k.PQ[i].Wallet >= z.Wallet })
	if i < len(k.PQ) && k.PQ[i].Wallet == z.Wallet {
		return k.fertig()
	}
	k.PQ = append(k.PQ, pqZusage{})
	copy(k.PQ[i+1:], k.PQ[i:])
	k.PQ[i] = z
	return k.fertig()
}

// pqZusagenGueltig: Form des Registerfelds (umzugRegisterLesen).
func pqZusagenGueltig(l []pqZusage) error {
	if len(l) > pqZusagenHoechstens {
		return fmt.Errorf("Umzugs-Register: mehr als %d PQ-Zusagen", pqZusagenHoechstens)
	}
	for i, z := range l {
		if !kanonischeAdresse(z.Wallet) || !istHash64(z.Hash) || z.Zeit <= 0 || (i > 0 && z.Wallet <= l[i-1].Wallet) {
			return fmt.Errorf("Umzugs-Register: PQ-Zusage %d ungueltig", i)
		}
	}
	return nil
}

// pqZusageUrteilLocked: dieselbe Pruefung fuer Annahme (Uhrzeit) und
// Nachspielen (Blockzeit).
func (cs *ChainState) pqZusageUrteilLocked(ctx context.Context, tx *Transaction, blockZeit int64) umzugUrteil {
	if !pqZusageAktiv(blockZeit) {
		return umzugBefund("pq_form", "PQ-Zusage vor dem Stichtag")
	}
	w := tx.Wallet
	u := tx.Umzug
	if !kanonischeAdresse(w) || u == nil || !istHash64(u.PQHash) || !kanonischeSignatur(u.Sig) ||
		u.Code != "" || u.CodeHash != "" || u.Angekuendigt != 0 || u.Pruefung != "" ||
		tx.BioBescheinigung != nil || tx.To != "" || tx.Nullifier != "" {
		return umzugBefund("pq_form", "PQ-Zusage nicht in gueltiger Form")
	}
	z := tx.DistributionAt
	if z <= 0 || z > sattAdd(blockZeit, umzugHoechstensVoraus) || blockZeit-z > umzugHoechstensAlt {
		return umzugBefund("pq_zeit", "%s: unterschrieben %d, Block %d", kurzAdresse(w), z, blockZeit)
	}
	// Vor dem Stichtag unterschrieben: nie (auch nicht in einem Block danach).
	if !pqZusageAktiv(z) {
		return umzugBefund("pq_zeit", "%s: unterschrieben %d vor dem Stichtag", kurzAdresse(w), z)
	}
	cs.ensureAccountLoadedCtx(ctx, w)
	if a, ok := cs.accounts.Get(w); !ok || !a.IsHuman {
		return umzugBefund("pq_kein_mensch", "%s ist kein Mensch", kurzAdresse(w))
	}
	if err := pruefePersonalSignGemerkt(pqZusageNachricht(w, u.PQHash, z), u.Sig, w); err != nil {
		return umzugBefund("pq_unterschrift", "PQ-Zusage nicht von %s unterschrieben", kurzAdresse(w))
	}
	reg := cs.umzugRegisterJetzt()
	if _, ok := reg.pqZusage(w); ok {
		return umzugBefund("pq_fest", "%s: hat schon eine PQ-Zusage, sie ist nicht aenderbar", kurzAdresse(w))
	}
	if len(reg.PQ) >= pqZusagenHoechstens {
		return umzugBefund("pq_voll", "Register der PQ-Zusagen voll")
	}
	return umzugUrteil{}
}

// pqZusageFenster: in welche Bloecke eine Zusage darf (blockTauglich,
// auftragsFenster) -- gleich der Zeitpruefung im Urteil.
func pqZusageFenster(tx Transaction) zeitFenster {
	f := zeitFenster{pqZusageAb(), math.MaxInt64}
	if tx.DistributionAt <= 0 || !pqZusageAktiv(tx.DistributionAt) {
		return zeitFenster{1, 0}
	}
	f.eng(sattAdd(tx.DistributionAt, -umzugHoechstensVoraus), sattAdd(tx.DistributionAt, umzugHoechstensAlt))
	return f
}

// handlePQZusage: POST {"wallet", "pq_hash", "zeit", "sig"} -- die App
// hinterlegt den SHA-256 des ML-DSA-65-Schluessels; der Schluessel selbst
// verlaesst das Telefon nicht.
func (a *APIServer) handlePQZusage(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if !pqZusageAktiv(time.Now().Unix()) {
		jsonError(w, "pq commitments not active yet", http.StatusConflict)
		return
	}
	if !burstErlaubt("pq-zusage:"+erneuerungsAbsender(r), burstUmzugJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	var req struct {
		Wallet string `json:"wallet"`
		PQHash string `json:"pq_hash"`
		Zeit   int64  `json:"zeit"`
		Sig    string `json:"sig"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, umzugKoerperMax)).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	wallet := strings.ToLower(strings.TrimSpace(req.Wallet))
	sig := kanonischeSignaturVersuch(req.Sig)
	if !kanonischeAdresse(wallet) || !istHash64(req.PQHash) || !kanonischeSignatur(sig) {
		jsonError(w, "wallet, pq_hash (64 hex) and sig required", http.StatusBadRequest)
		return
	}
	if err := verifyPersonalSign(pqZusageNachricht(wallet, req.PQHash, req.Zeit), sig, wallet); err != nil {
		jsonError(w, "signature does not match wallet", http.StatusForbidden)
		return
	}
	a.umzugAnnehmen(w, r, Transaction{Type: pqZusageTxArt, Wallet: wallet, DistributionAt: req.Zeit,
		Umzug: &UmzugAngaben{PQHash: req.PQHash, Sig: sig}}, nil)
}
