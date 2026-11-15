package keeper

// Audit B2 (14.11.2026): PQ-Schluessel fuer die Dienste der Pruefer (B1b).
//
// # DIE LUECKE
//
// Ab pq_ab_unix (bio_bescheinigung_pq.go) braucht jeder Schluessel, der eine
// Bescheinigung bindet oder bezeugt, eine ML-DSA-65-Unterschrift seines
// PQ-Schluessels. PQ-Schluessel standen aber nur in der Genesis -- die
// Schluessel von Pruefern, die ueber das Register binden (B1b), hatten nie
// einen. Mit dem Stichtag waere jede Bescheinigung ungueltig geworden, an der
// ein solcher Schluessel beteiligt ist: B1b faktisch abgeschaltet.
//
// # WAS DAZUKOMMT
//
// Transaktion pruefer_dienst_pq (Wallet = der Mensch): der Pruefer hinterlegt
// fuer einen Ed25519-Schluessel seines Dienstes (Coordinator oder
// Vergleichsdienst) dessen ML-DSA-65-Schluessel. Zwei Unterschriften:
//
//	Dienst (Besitz):  aequitas-pruefer-dienst-besitz-v1|chain:1926|<mensch>|<ed25519>
//	Mensch (Freigabe): aequitas-pruefer-dienst-pq-v1|chain:1926|<mensch>|<ed25519>|<sha256(dienst-pq)>|<zeit>
//
// Der Mensch unterschreibt mit SEINEM ML-DSA-65-Schluessel aus der
// Seed-Phrase; dessen SHA-256 muss genau seine PQ-Zusage sein (pq_zusage.go,
// nicht aenderbar). Damit haengt die ganze Kette an quantensicheren
// Unterschriften: Zusage -> Mensch -> Dienst. Ein Quantenrechner, der
// secp256k1 und Ed25519 bricht, kann keinem Pruefer einen eigenen
// PQ-Schluessel unterschieben.
//
// # REGELN (jeder Knoten prueft selbst)
//
//   - erst ab prueferDienstPQAbUnix (schlafend, Waechter
//     .github/neustart/pruefer-dienst-pq-nicht-verpasst.sh);
//   - der Mensch ist zur Zeit der Freigabe Pruefer;
//   - Ed25519-Schluessel tauglich und KEIN Genesis-Schluessel (die haben ihren
//     PQ-Schluessel in der Genesis);
//   - je Ed25519-Schluessel genau ein Eintrag, nicht aenderbar (wie die
//     Zusage: kein Austausch unter laufenden Bescheinigungen); je Mensch
//     hoechstens prueferDienstPQJeMensch, insgesamt prueferDienstPQHoechstens;
//   - Wirkung erst ab issued_at + erzeugerFrist (wie das Pruefer-Register:
//     ein Block, der eine Bescheinigung zur Zeit t traegt, kommt nach dem
//     Block mit dem Eintrag -- Kopf von verifier_register.go, REIHENFOLGE);
//   - billig vor teuer: Form, Zeit, Register, Zusage, dann zwei ML-DSA.
//
// # SPEICHER
//
// Im Pruefer-Register (Feld "pq", sortiert nach Ed25519-Schluessel), also in
// chain_config, Snapshot und Rueckroll. In die StateRoot geht nur der Hash des
// Felds (die Schluessel sind je ~4 KB), ohne Eintraege bleibt die Wurzel
// byte-gleich.

import (
	"context"
	"crypto/mldsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const (
	prueferDienstPQTxArt = "pruefer_dienst_pq"
	// prueferDienstPQAbUnix: 2026-11-15 19:00 UTC (Entscheidung des Betreibers
	// vom 15.11.2026, "alles umsetzen, Stichtag heute"). Vor pq_ab_unix, damit
	// Pruefer ihre Dienste eintragen koennen (docs/QUANTENSICHERHEIT.md). C1
	// laeuft seit #518 mit diesem Code; Dienste geben ihren Schluessel seit
	// biometric #121 heraus, die App unterschreibt die Freigabe (1.21.0). Die
	// Regel erlaubt nur (eine neue Art), sie verlangt nichts.
	// Waechter: .github/neustart/pruefer-dienst-pq-nicht-verpasst.sh.
	prueferDienstPQAbUnix int64 = 1794769200

	prueferDienstPQJeMensch    = 2 // Coordinator und Vergleichsdienst
	prueferDienstPQHoechstens  = prueferDienstPQJeMensch * verifierHoechstens
	prueferDienstPQKoerperMax  = 32 << 10
	prueferDienstPQDomaeneBes  = "aequitas-pruefer-dienst-besitz-v1"
	prueferDienstPQDomaeneFrei = "aequitas-pruefer-dienst-pq-v1"
)

var prueferDienstPQAbOverride atomic.Int64

func prueferDienstPQAb() int64 {
	if v := prueferDienstPQAbOverride.Load(); v != 0 {
		return v
	}
	return prueferDienstPQAbUnix
}

func prueferDienstPQAktiv(t int64) bool { return t >= prueferDienstPQAb() }

// PrueferDienstPQ: die Angaben der Transaktion (Hex, klein, ohne 0x).
type PrueferDienstPQ struct {
	Ed        string `json:"ed"`
	Pub       string `json:"pub"`
	BesitzSig string `json:"besitz_sig"`
	MenschPub string `json:"mensch_pub"`
	MenschSig string `json:"mensch_sig"`
}

// prueferDienstPQ: ein Eintrag im Pruefer-Register.
type prueferDienstPQ struct {
	Ed      string `json:"ed"`
	Mensch  string `json:"m"`
	Pub     string `json:"pub"`
	Wirkung int64  `json:"ab"`
}

func prueferDienstBesitzNachricht(mensch, ed string) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s", prueferDienstPQDomaeneBes, eingefroreneChainID, strings.ToLower(mensch), strings.ToLower(ed))
}

func prueferDienstFreigabeNachricht(mensch, ed, pubHash string, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%s|%d", prueferDienstPQDomaeneFrei, eingefroreneChainID, strings.ToLower(mensch), strings.ToLower(ed), pubHash, zeit)
}

func sha256Hex(hexStr string) (string, bool) {
	roh, err := hex.DecodeString(hexStr)
	if err != nil {
		return "", false
	}
	h := sha256.Sum256(roh)
	return hex.EncodeToString(h[:]), true
}

// ------------------------------------------------------------ Register

// dienstPQ: der PQ-Schluessel zum Ed25519-Schluessel ed, wenn er diesem
// Menschen gehoert und zur Zeit t schon wirkt.
func (r *verifierRegister) dienstPQ(ed, mensch string, t int64) (string, bool) {
	if r == nil {
		return "", false
	}
	i := sort.Search(len(r.PQ), func(i int) bool { return r.PQ[i].Ed >= ed })
	if i < len(r.PQ) && r.PQ[i].Ed == ed && r.PQ[i].Mensch == mensch && r.PQ[i].Wirkung <= t {
		return r.PQ[i].Pub, true
	}
	return "", false
}

func (r *verifierRegister) dienstPQEintrag(ed string) (prueferDienstPQ, bool) {
	if r == nil {
		return prueferDienstPQ{}, false
	}
	i := sort.Search(len(r.PQ), func(i int) bool { return r.PQ[i].Ed >= ed })
	if i < len(r.PQ) && r.PQ[i].Ed == ed {
		return r.PQ[i], true
	}
	return prueferDienstPQ{}, false
}

// mitDienstPQ: ein neues Register mit dem Eintrag e (eine vorhandene Zeile
// desselben Schluessels bleibt -- das Urteil laesst keine zweite zu).
func (r *verifierRegister) mitDienstPQ(e prueferDienstPQ) *verifierRegister {
	neu := &verifierRegister{}
	if r != nil {
		neu.Eintraege = append(neu.Eintraege, r.Eintraege...)
		neu.PQ = append(neu.PQ, r.PQ...)
	}
	if _, da := neu.dienstPQEintrag(e.Ed); !da {
		neu.PQ = append(neu.PQ, e)
		sort.Slice(neu.PQ, func(i, j int) bool { return neu.PQ[i].Ed < neu.PQ[j].Ed })
	}
	neu.pqHash = prueferPQHash(neu.PQ)
	return neu
}

// dienstPQGueltig: Form des Felds (verifierRegisterLesen).
func dienstPQGueltig(l []prueferDienstPQ) error {
	if len(l) > prueferDienstPQHoechstens {
		return fmt.Errorf("Pruefer-Register: %d PQ-Eintraege, hoechstens %d", len(l), prueferDienstPQHoechstens)
	}
	je := map[string]int{}
	for i, e := range l {
		if len(e.Ed) != 64 || !kleinHex(e.Ed) || !kanonischeAdresse(e.Mensch) || len(e.Pub) != pqPubHex || !kleinHex(e.Pub) ||
			e.Wirkung <= 0 || (i > 0 && e.Ed <= l[i-1].Ed) {
			return fmt.Errorf("Pruefer-Register: PQ-Eintrag %d ungueltig", i)
		}
		if je[e.Mensch]++; je[e.Mensch] > prueferDienstPQJeMensch {
			return fmt.Errorf("Pruefer-Register: mehr als %d PQ-Eintraege fuer einen Menschen", prueferDienstPQJeMensch)
		}
	}
	return nil
}

// ------------------------------------------------------------ Urteil

type prueferDienstUrteil struct {
	regel, text string
	eintrag     prueferDienstPQ
}

func prueferDienstBefund(regel, format string, args ...interface{}) prueferDienstUrteil {
	return prueferDienstUrteil{regel: regel, text: fmt.Sprintf(format, args...)}
}

// prueferDienstPQUrteilLocked: dieselbe Pruefung fuer Annahme (Uhrzeit) und
// Nachspielen (Blockzeit).
func (cs *ChainState) prueferDienstPQUrteilLocked(tx *Transaction, blockZeit int64) prueferDienstUrteil {
	d := tx.DienstPQ
	mensch := tx.Wallet
	if !prueferDienstPQAktiv(blockZeit) || tx.Type != prueferDienstPQTxArt || d == nil {
		return prueferDienstBefund("pruefer_pq_form", "vor dem Stichtag oder ohne Angaben")
	}
	if !kanonischeAdresse(mensch) || tx.To != "" || tx.Amount != 0 ||
		len(d.Ed) != 64 || !kleinHex(d.Ed) || len(d.Pub) != pqPubHex || !kleinHex(d.Pub) ||
		len(d.MenschPub) != pqPubHex || !kleinHex(d.MenschPub) ||
		len(d.BesitzSig) != pqSigHex || !kleinHex(d.BesitzSig) || len(d.MenschSig) != pqSigHex || !kleinHex(d.MenschSig) {
		return prueferDienstBefund("pruefer_pq_form", "Angaben nicht in gueltiger Form")
	}
	zeit := tx.DistributionAt
	if zeit <= 0 || zeit < blockZeit-nachweisHoechstensAlt || zeit > sattAdd(blockZeit, nachweisHoechstensVoraus) || !prueferDienstPQAktiv(zeit) {
		return prueferDienstBefund("pruefer_pq_zeit", "freigegeben %d, Blockzeit %d", zeit, blockZeit)
	}
	if !ed25519HexTauglich(d.Ed) {
		return prueferDienstBefund("pruefer_pq_form", "kein tauglicher Ed25519-Schluessel")
	}
	if k, err := bescheinigungKonf(); err == nil {
		for _, liste := range [][]string{k.Coordinator, k.Bezeuger} {
			for _, s := range liste {
				if s == d.Ed {
					return prueferDienstBefund("pruefer_pq_genesis", "Genesis-Schluessel -- sein PQ-Schluessel steht in der Genesis")
				}
			}
		}
	}
	reg := cs.verifierRegisterJetzt()
	if !reg.mitglied(mensch, zeit) {
		return prueferDienstBefund("pruefer_pq_kein_pruefer", "%s ist zu %d kein Pruefer", kurzAdresse(mensch), zeit)
	}
	if _, da := reg.dienstPQEintrag(d.Ed); da {
		return prueferDienstBefund("pruefer_pq_fest", "fuer diesen Schluessel ist schon ein PQ-Schluessel eingetragen (nicht aenderbar)")
	}
	n := 0
	for _, e := range reg.PQ {
		if e.Mensch == mensch {
			n++
		}
	}
	if n >= prueferDienstPQJeMensch {
		return prueferDienstBefund("pruefer_pq_voll", "hoechstens %d Dienste je Pruefer", prueferDienstPQJeMensch)
	}
	if len(reg.PQ) >= prueferDienstPQHoechstens {
		return prueferDienstBefund("pruefer_pq_voll", "hoechstens %d Eintraege", prueferDienstPQHoechstens)
	}
	zusage, ok := cs.prueferZusageVon(mensch)
	if !ok {
		return prueferDienstBefund("pruefer_pq_zusage", "%s hat keine PQ-Zusage", kurzAdresse(mensch))
	}
	if h, ok := sha256Hex(d.MenschPub); !ok || h != zusage {
		return prueferDienstBefund("pruefer_pq_zusage", "Schluessel des Menschen ist nicht seine PQ-Zusage")
	}
	pubHash, _ := sha256Hex(d.Pub)
	if !mldsaPruefen(d.MenschPub, d.MenschSig, prueferDienstFreigabeNachricht(mensch, d.Ed, pubHash, zeit)) {
		return prueferDienstBefund("pruefer_pq_freigabe", "Freigabe des Menschen passt nicht")
	}
	if !mldsaPruefen(d.Pub, d.BesitzSig, prueferDienstBesitzNachricht(mensch, d.Ed)) {
		return prueferDienstBefund("pruefer_pq_besitz", "Besitznachweis des Dienstes passt nicht")
	}
	return prueferDienstUrteil{eintrag: prueferDienstPQ{Ed: d.Ed, Mensch: mensch, Pub: d.Pub, Wirkung: sattAdd(zeit, erzeugerFrist)}}
}

func mldsaPruefen(pubHex, sigHex, msg string) bool {
	pk, err := pqSchluesselLesen(pubHex)
	if err != nil {
		return false
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}
	return mldsa.Verify(pk, []byte(msg), sig, nil) == nil
}

func (cs *ChainState) prueferDienstPQSetzenLocked(ctx context.Context, e prueferDienstPQ) error {
	neu := cs.verifierRegisterJetzt().mitDienstPQ(e)
	if cs.db != nil {
		if err := cs.setConfigValueCtx(ctx, verifierKonfigSchluessel, neu.kodiert()); err != nil {
			return fmt.Errorf("pruefer_dienst_pq: Register speichern: %w", err)
		}
	}
	cs.verifierReg.Store(neu)
	return nil
}

// applyPrueferDienstPQLocked: beim Nachspielen. Ungueltig: nie angewandt,
// gezaehlt, im strengen Modus Fehler. Caller haelt cs.mu.
func (cs *ChainState) applyPrueferDienstPQLocked(ctx context.Context, tx *Transaction, blockUnix int64) error {
	u := cs.prueferDienstPQUrteilLocked(tx, blockUnix)
	if u.regel != "" {
		return nachrechnenAbweichung(u.regel, blockUnix, "%s", u.text)
	}
	return cs.prueferDienstPQSetzenLocked(ctx, u.eintrag)
}

// ------------------------------------------------------------ API

// handlePrueferDienstPQ: POST {"mensch", "zeit", "ed", "pub", "besitz_sig",
// "mensch_pub", "mensch_sig"}.
func (a *APIServer) handlePrueferDienstPQ(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(prueferDienstPQStatus())
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	jetzt := time.Now().Unix()
	if !prueferDienstPQAktiv(jetzt) {
		jsonError(w, "verifier service PQ keys not active yet", http.StatusConflict)
		return
	}
	if !burstErlaubt("pruefer-dienst-pq:"+erneuerungsAbsender(r), burstQuantenJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	var req struct {
		Mensch string `json:"mensch"`
		Zeit   int64  `json:"zeit"`
		PrueferDienstPQ
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, prueferDienstPQKoerperMax))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	d := req.PrueferDienstPQ
	for _, f := range []*string{&d.Ed, &d.Pub, &d.BesitzSig, &d.MenschPub, &d.MenschSig} {
		*f = strings.ToLower(strings.TrimSpace(*f))
	}
	mensch := strings.ToLower(strings.TrimSpace(req.Mensch))
	if !kanonischeAdresse(mensch) {
		jsonError(w, "mensch (address) required", http.StatusBadRequest)
		return
	}
	if req.Zeit < jetzt-nachweisAnnahmeHoechstensAlt || req.Zeit > jetzt+nachweisHoechstensVoraus {
		jsonError(w, "zeit outside the allowed window", http.StatusBadRequest)
		return
	}
	tx := Transaction{Type: prueferDienstPQTxArt, Wallet: mensch, DistributionAt: req.Zeit, DienstPQ: &d}
	if err := a.state.annahmeBeginnen(mensch); err != nil {
		jsonError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer a.state.annahmeEnde()
	var abgelehnt string
	err := a.state.runAtomicWithOutbox([]string{mensch}, false, func(ctx context.Context) (Transaction, error) {
		u := a.state.prueferDienstPQUrteilLocked(&tx, jetzt)
		if u.regel != "" {
			abgelehnt = u.regel + ": " + u.text
			return Transaction{}, errors.New(abgelehnt)
		}
		if err := a.state.prueferDienstPQSetzenLocked(ctx, u.eintrag); err != nil {
			return Transaction{}, err
		}
		return tx, nil
	})
	if abgelehnt != "" {
		jsonError(w, "rejected: "+abgelehnt, http.StatusConflict)
		return
	}
	if err != nil {
		jsonError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "ed": d.Ed, "wirkt_ab": sattAdd(req.Zeit, erzeugerFrist)})
}

// prueferDienstPQStatus: fuer /api/status ("pruefer_dienst_pq"); ab fehlt,
// solange schlafend.
func prueferDienstPQStatus() map[string]interface{} {
	m := map[string]interface{}{}
	if ab := prueferDienstPQAb(); ab != math.MaxInt64 {
		m["ab"] = ab
	}
	return m
}

// prueferPQHash: SHA-256 ueber die kanonische Form des PQ-Felds -- das, was
// davon in die StateRoot geht.
func prueferPQHash(l []prueferDienstPQ) string {
	if len(l) == 0 {
		return ""
	}
	b, _ := json.Marshal(l)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
