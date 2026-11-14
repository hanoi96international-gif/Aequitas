package keeper

// Quantensicherheit Stufe 3 (docs/QUANTENSICHERHEIT.md): quantensichere
// Adressen und Ueberweisungen mit ML-DSA-65 (FIPS 204).
//
// ADRESSE. Eine PQ-Adresse ist "0x" + die letzten 20 Byte von
// SHA-256("aequitas-pq-adresse-v1" || oeffentlicher ML-DSA-65-Schluessel).
// Sie sieht aus wie jede andere Adresse und empfaengt mit jeder gewoehnlichen
// Ueberweisung. AUSGEBEN kann man von ihr nur mit einer ML-DSA-Unterschrift:
// eine EVM-Rohform verlangte einen secp256k1-Schluessel, dessen Keccak-Adresse
// genau diese 20 Byte trifft -- ein Urbild, auch fuer einen Quantenrechner
// (Grover: 2^80 Schritte je Adresse) ausser Reichweite. Umgekehrt ergibt kein
// ML-DSA-Schluessel eine gewoehnliche Adresse.
//
// UEBERWEISUNG. Eine gewoehnliche "transfer"-Transaktion, die statt der
// EVM-Rohform (Roh) den PQ-Nachweis (PQ) traegt: oeffentlicher Schluessel,
// Unterschrift, Nonce und Betrag in Wei. Unterschrieben wird
// "aequitas-pq-ueberweisung-v1|chain:1926|<von>|<an>|<wei>|<nonce>".
// Alles Weitere -- Gebuehr, Kontostaende, Buchfuehrung, Kontoverlauf, die
// Nonce (NaechsteNonce, wie bei Roh) -- laeuft unveraendert ueber die Wege
// der Ueberweisung. Jeder Knoten prueft beim Nachspielen selbst
// (pruefeUeberweisungsRoh -> pruefePQUeberweisung), vorab und parallel wie
// die EVM-Unterschriften.
//
// ANNAHME nur auf dem seriellen Weg (TransferAtomic): der WAL-Schnellpfad
// schriebe die Ueberweisung ohne PQ-Nachweis in den Block.
//
// SCHLAFEND bis pqUeberweisungAbUnix: davor ist ein PQ-Nachweis in einem
// Block ein Fehler und der Endpunkt antwortet 409.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"crypto/mldsa"
)

const (
	// pqUeberweisungAbUnix: 2026-11-17 18:00 UTC (zugleich mit der PQ-Zusage).
	pqUeberweisungAbUnix int64 = 1794938400

	pqDomaeneAdresse      = "aequitas-pq-adresse-v1"
	pqDomaeneUeberweisung = "aequitas-pq-ueberweisung-v1"

	// Groessen (FIPS 204, ML-DSA-65) in Hex.
	pqPubHex = 2 * 1952
	pqSigHex = 2 * 3309
	// Hoechstens 10^30 Wei (10^12 AEQ) -- weit ueber jedem Kontostand.
	pqWeiStellen = 31
	// Koerper des Endpunkts: Schluessel + Unterschrift + Felder.
	pqUeberweisungKoerperMax = 16 << 10
	burstPQUeberweisungJeIP  = 20

	// pqUeberweisungenJeBlock: eine PQ-Ueberweisung ist rund 11 KB JSON
	// (Schluessel 1.952 + Unterschrift 3.309 Byte, als Hex). Ein Block darf
	// 32 MB gross sein (maxBlockStreamBytes) und 10.000 Transaktionen tragen
	// -- 10.000 PQ-Ueberweisungen waeren 107 MB, kein Knoten naehme den
	// Block an. 500 sind rund 5,4 MB und eine Pruefzeit von Millisekunden je
	// Kern. Der Erzeuger schneidet davor ab (pqUeberweisungGrenze), jeder
	// Knoten weist einen Block mit mehr ab (pruefeUeberweisungenImBlock).
	pqUeberweisungenJeBlock = 500
)

// PQUeberweisung: der PQ-Nachweis einer Ueberweisung (Transaction.PQ).
type PQUeberweisung struct {
	Pub   string `json:"pub"`
	Sig   string `json:"sig"`
	Nonce uint64 `json:"nonce"`
	Wei   string `json:"wei"`
}

// pqUeberweisungAbOverride: nur fuer Tests (0 = Konstante gilt).
var pqUeberweisungAbOverride atomic.Int64

func pqUeberweisungAb() int64 {
	if v := pqUeberweisungAbOverride.Load(); v != 0 {
		return v
	}
	return pqUeberweisungAbUnix
}

func pqUeberweisungAktiv(t int64) bool { return t >= pqUeberweisungAb() }

// pqAdresseAusSchluessel: die Adresse zu einem oeffentlichen ML-DSA-65-Schluessel.
func pqAdresseAusSchluessel(pub []byte) string {
	h := sha256.Sum256(append([]byte(pqDomaeneAdresse), pub...))
	return "0x" + hex.EncodeToString(h[12:])
}

// pqUeberweisungNachricht: was der ML-DSA-Schluessel unterschreibt.
func pqUeberweisungNachricht(von, an, wei string, nonce uint64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%s|%d", pqDomaeneUeberweisung, eingefroreneChainID, von, an, wei, nonce)
}

// pqUeberweisungTxHash: der TxHash ist an die unterschriebene Nachricht
// gebunden -- jeder Knoten rechnet ihn nach.
func pqUeberweisungTxHash(msg string) string {
	h := sha256.Sum256([]byte(msg))
	return "0x" + hex.EncodeToString(h[:])
}

// pqWeiKanonisch: Dezimalzahl ohne fuehrende Null, > 0, hoechstens
// pqWeiStellen Stellen.
func pqWeiKanonisch(w string) (*big.Int, bool) {
	if w == "" || len(w) > pqWeiStellen || w[0] == '0' {
		return nil, false
	}
	for _, c := range w {
		if c < '0' || c > '9' {
			return nil, false
		}
	}
	v, ok := new(big.Int).SetString(w, 10)
	return v, ok && v.Sign() > 0
}

var errPQUeberweisung = errors.New("PQ-Ueberweisung ungueltig")

// pruefePQUeberweisung prueft eine Ueberweisung mit PQ-Nachweis gegen ihre
// Unterschrift und gibt die Nonce zurueck. Zustandslos (parallel ausfuehrbar),
// fuer Annahme (Uhrzeit) und Nachspielen (Blockzeit) dieselbe Funktion.
func pruefePQUeberweisung(tx *Transaction, blockZeit int64) (uint64, error) {
	p := tx.PQ
	if p == nil {
		return 0, fmt.Errorf("%w: kein PQ-Nachweis", errPQUeberweisung)
	}
	if !pqUeberweisungAktiv(blockZeit) {
		return 0, fmt.Errorf("%w: vor dem Stichtag (Block %d)", errPQUeberweisung, blockZeit)
	}
	if tx.Type != "transfer" || tx.Roh != "" || tx.Nachweis != nil {
		return 0, fmt.Errorf("%w: nur als Ueberweisung ohne Rohform", errPQUeberweisung)
	}
	if len(p.Pub) != pqPubHex || len(p.Sig) != pqSigHex || !kleinHex(p.Pub) || !kleinHex(p.Sig) {
		return 0, fmt.Errorf("%w: Schluessel oder Unterschrift in falscher Form", errPQUeberweisung)
	}
	if p.Nonce >= math.MaxInt64 {
		return 0, fmt.Errorf("%w: Nonce %d zu gross", errPQUeberweisung, p.Nonce)
	}
	wei, ok := pqWeiKanonisch(p.Wei)
	if !ok {
		return 0, fmt.Errorf("%w: Betrag %q nicht kanonisch", errPQUeberweisung, p.Wei)
	}
	von := tx.Wallet
	an := tx.To
	if !kanonischeAdresse(von) || !kanonischeAdresse(an) {
		return 0, fmt.Errorf("%w: Adressen nicht kanonisch", errPQUeberweisung)
	}
	pubRoh, _ := hex.DecodeString(p.Pub)
	if pqAdresseAusSchluessel(pubRoh) != von {
		return 0, fmt.Errorf("%w: %s gehoert nicht zu diesem Schluessel", errPQUeberweisung, kurzAdresse(von))
	}
	if betrag := betragAusWei(wei); betrag != tx.Amount {
		return 0, fmt.Errorf("%w: unterschrieben %.18g AEQ, im Block %.18g", errPQUeberweisung, betrag, tx.Amount)
	}
	msg := pqUeberweisungNachricht(von, an, p.Wei, p.Nonce)
	if tx.TxHash != pqUeberweisungTxHash(msg) {
		return 0, fmt.Errorf("%w: TxHash passt nicht zur Nachricht", errPQUeberweisung)
	}
	pk, err := pqSchluesselLesen(p.Pub)
	if err != nil {
		return 0, fmt.Errorf("%w: Schluessel unlesbar", errPQUeberweisung)
	}
	sig, _ := hex.DecodeString(p.Sig)
	if mldsa.Verify(pk, []byte(msg), sig, nil) != nil {
		return 0, fmt.Errorf("%w: Unterschrift passt nicht", errPQUeberweisung)
	}
	return p.Nonce, nil
}

// pqFeldErlaubt: ein PQ-Nachweis gehoert nur an eine Ueberweisung (dort prueft
// ihn pruefeUeberweisungsRoh). An jeder anderen Art waere er ungeprueft --
// der ganze Block ist dann ungueltig.
func pqFeldErlaubt(tx *Transaction) error {
	if tx.PQ != nil && tx.Type != "transfer" {
		return fmt.Errorf("%s traegt einen PQ-Nachweis", tx.Type)
	}
	return nil
}

// pqUeberweisungGrenze: wie viele Transaktionen vom Anfang ein Block tragen
// kann, ohne mehr als pqUeberweisungenJeBlock PQ-Ueberweisungen (wie
// checkpointTxGrenze: der Rest kommt in der Reihenfolge in den naechsten).
func pqUeberweisungGrenze(txs []Transaction) int {
	n := 0
	for i := range txs {
		if txs[i].PQ != nil {
			n++
			if n > pqUeberweisungenJeBlock {
				return i
			}
		}
	}
	return len(txs)
}

// signiert: traegt die Ueberweisung eine Unterschrift des Absenders, die
// jeder Knoten prueft (EVM-Rohform oder PQ-Nachweis)? Solche nehmen nie die
// Schnellpfade ohne Nonce.
func signiert(tx Transaction) bool { return tx.Roh != "" || tx.PQ != nil }

// handlePQUeberweisung: POST {"von", "an", "wei", "nonce", "pub", "sig"}.
func (a *APIServer) handlePQUeberweisung(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	jetzt := time.Now().Unix()
	if !pqUeberweisungAktiv(jetzt) {
		jsonError(w, "pq transfers not active yet", http.StatusConflict)
		return
	}
	if !burstErlaubt("pq-ueberweisung:"+erneuerungsAbsender(r), burstPQUeberweisungJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	var req struct {
		Von   string `json:"von"`
		An    string `json:"an"`
		Wei   string `json:"wei"`
		Nonce uint64 `json:"nonce"`
		Pub   string `json:"pub"`
		Sig   string `json:"sig"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, pqUeberweisungKoerperMax))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	von := strings.ToLower(strings.TrimSpace(req.Von))
	an := strings.ToLower(strings.TrimSpace(req.An))
	wei, ok := pqWeiKanonisch(req.Wei)
	if !ok || !kanonischeAdresse(von) || !kanonischeAdresse(an) {
		jsonError(w, "von, an and wei (decimal) required", http.StatusBadRequest)
		return
	}
	if von == an {
		jsonError(w, "cannot transfer to self", http.StatusBadRequest)
		return
	}
	betrag := betragAusWei(wei)
	msg := pqUeberweisungNachricht(von, an, req.Wei, req.Nonce)
	tx := Transaction{Type: "transfer", Wallet: von, To: an, Amount: betrag, TxHash: pqUeberweisungTxHash(msg),
		PQ: &PQUeberweisung{Pub: req.Pub, Sig: req.Sig, Nonce: req.Nonce, Wei: req.Wei}}
	if _, err := pruefePQUeberweisung(&tx, jetzt); err != nil {
		jsonError(w, err.Error(), http.StatusForbidden)
		return
	}
	if _, _, err := a.state.TransferAtomic(von, an, betrag, tx); err != nil {
		if istWiederholbareAnnahmeAblehnung(err) {
			jsonError(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		jsonError(w, "transfer failed: "+err.Error(), http.StatusConflict)
		return
	}
	a.state.SyncBalancesToEVM(V7_CONTRACT_ADDR, von, an)
	recordAppliedTransfer()
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "tx_hash": tx.TxHash})
}

// handlePQStand: GET ?adresse= -- Stichtag, naechste Nonce und Guthaben
// einer PQ-Adresse (die App braucht die Nonce zum Unterschreiben).
func (a *APIServer) handlePQStand(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodGet {
		jsonError(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	if !burstErlaubt("pq-stand:"+erneuerungsAbsender(r), burstNullifierPasstJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	antwort := map[string]interface{}{"ueberweisung_ab": pqUeberweisungAb(), "zusage_ab": pqZusageAb()}
	adr := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("adresse")))
	if adr != "" {
		if !kanonischeAdresse(adr) {
			jsonError(w, "adresse must be a 0x address", http.StatusBadRequest)
			return
		}
		nonce, guthaben := a.state.pqKontoStand(adr)
		antwort["adresse"] = adr
		antwort["nonce"] = strconv.FormatInt(nonce, 10)
		antwort["guthaben"] = guthaben
	}
	json.NewEncoder(w).Encode(antwort)
}

// pqKontoStand: NaechsteNonce und Guthaben (AEQ) eines Kontos -- ueber
// dieselben Leser wie eth_getTransactionCount und eth_getBalance (der
// schnelle Annahmeweg aendert unter der Kontensperre, nicht unter cs.mu).
func (cs *ChainState) pqKontoStand(adr string) (int64, float64) {
	return int64(cs.naechsteNonceVon(adr)), cs.GetBalance(adr)
}
