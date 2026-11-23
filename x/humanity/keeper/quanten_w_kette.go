package keeper

// QUANTENGELD VARIANTE W -- STUFE W2b: EINBINDUNG IN DIE KETTE
// (seit 17.11.2026 17:00 UTC im Hauptnetz freigeschaltet, quantenWAbUnix)
//
// Die Regeln (quanten_w.go, quanten_w_register.go; Spezifikation
// forschung/quantengeld/wiesner_kette.py) haengen hier an der Kette: sechs
// Transaktionsarten, Annahme und Nachspielen mit DERSELBEN Pruefung, das
// Register (seit #563 je Eintrag eine Zeile, quanten_w_tabelle.go; StateRoot
// nur nicht leer), Snapshot, Checkpoint und Rueckroll. Vor quantenWAbUnix
// sind die Arten unbekannt (bekannteTxArt), die Endpunkte antworten 409, kein
// Block traegt sie, und das leere Register aendert keine Wurzel. Geplant war
// das Freischalten als Stufe W6 nach Partnerversuch (W4) und Gutachten (W5);
// der Betreiber hat W und S am 17.11.2026 fuer 17:00 UTC VORHER
// freigeschaltet (#568, #570, Begruendung bei quantenWAbUnix). Der
// Pruefauftrag fuer das Gutachten: docs/QUANTENGELD_PRUEFAUFTRAG.md.
//
// DIE SECHS ARTEN
//
//   - quanten_w_praegen (Wallet = Praeger, unterschrieben): Deckung Wallet ->
//     W-Topf; die Note mit n, t, Wurzel und je Validator Anker und
//     Bestaetigung (EIP-191 seiner Signieradresse, mit der Praegezeit). Jeder
//     Validator ist im Validator-Register gebunden, an einen Menschen, und
//     alle an VERSCHIEDENE Menschen -- sonst hielte einer mehrere Anteile.
//   - quanten_w_anfrage (Wallet = Anfragender, unterschrieben): Pfand Wallet
//     -> Pfand-Topf. Das Pfand rechnet jeder Knoten aus dem Wert der Note
//     (quantenWPfandFuer), nie aus der Transaktion.
//   - quanten_w_zufall (ohne Wallet): Werte der Hash-Ketten der Validatoren;
//     die Kette beweist die Herkunft, jeder darf einreichen.
//   - quanten_w_antwort (Wallet = Anfragender, unterschrieben): die Bits.
//   - quanten_w_offenlegen (ohne Wallet): Anteile und Merkle-Pfade; die
//     Pruefsummen beweisen sie. Urteil und Buchungen rechnet jeder Knoten.
//   - quanten_w_frist (ohne Wallet): beendet eine Anfrage nach ihrer Frist.
//
// GELD
//
// W-Topf: Deckung der Noten, je Note Anteile wie im Quantengeld-Topf
// (quantenAnteileFuer, quantenWert); die Umlaufabgabe trifft ihn wie den
// Quantengeld-Topf (umlaufArt), jede Note sinkt anteilig. Pfand-Topf: die
// offenen Pfaender, ohne Abgabe -- sein Stand ist ihre Summe. Was verfaellt,
// geht ans Grundeinkommen (ubiPoolAddr). Der Block nennt jede Buchung
// (Anteile, Pfand, PfandAn, Wert, WertAn, Urteil); jeder Knoten rechnet nach,
// eine Abweichung macht den Block falsch. Ohne Gebuehr, wie Quantengeld und
// Offline-Bargeld; gegen Spam stehen Pfand, Burst-Grenze und Koerpergrenze.
//
// TOTE NOTEN bleiben unter "tote" gemerkt, solange ihre Bestaetigungen
// gelten koennten (Praegezeit + quantenWAlt): sonst liesse sich eine tote
// Note mit denselben Ankern neu praegen, deren Hash-Ketten teils offen sind.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const (
	// quantenWAbUnix: Stichtag (math.MaxInt64 = schlafend). Freigeschaltet
	// vom Betreiber am 17.11.2026 fuer den 17.11.2026 17:00 UTC (vorgezogen vom 18.11. 18:00) -- vor dem
	// Gutachten (W5) und ohne Pilot mit Partner (W4 mit Speicher). Bewusst:
	// eine W-Note haelt auf heutiger Hardware nur Mikrosekunden, wer Deckung
	// in eine Note sperrt, die er nicht halten kann, verliert sie an das
	// Grundeinkommen; Variante S braucht keinen Speicher. Der Deploy-Waechter
	// .github/neustart/quanten-w-nicht-verpasst.sh haelt den Ausroll an, wenn
	// der Stichtag nicht mindestens 30 min in der Zukunft liegt.
	quantenWAbUnix int64 = 1794934800

	quantenWPraegenTxArt    = "quanten_w_praegen"
	quantenWAnfrageTxArt    = "quanten_w_anfrage"
	quantenWZufallTxArt     = "quanten_w_zufall"
	quantenWAntwortTxArt    = "quanten_w_antwort"
	quantenWOffenlegenTxArt = "quanten_w_offenlegen"
	quantenWFristTxArt      = "quanten_w_frist"

	quantenWKonfigSchluessel = "quanten_w"
	quantenWDomPraegen       = "aequitas-quanten-w-praegen-v2"
	quantenWDomAnfrage       = "aequitas-quanten-w-anfrage-v1"
	quantenWDomAntwort       = "aequitas-quanten-w-antwort-v1"

	// Lebende Noten im Register (das Register steht als EIN Wert in
	// chain_config und wird je Aenderung ganz geschrieben). Mit sicheren
	// Pruefgroessen (W2c) traegt eine Note bis ~4 KB Bitfeld und eine offene
	// Anfrage bis ~80 KB; darum hoechstens 16 Noten. Mehr Noten brauchen eine
	// eigene Tabelle (wie QG3) -- eine Entscheidung fuer W6.
	quantenWNotenHoechstens = 16
	// Gemerkte tote Noten; voll = kein neues Praegen bis zum Ablauf (fail-closed).
	quantenWToteHoechstens = 1024
	// Praegen und Anfrage: hoechstens 5 min nach und einen Tag vor dem Block
	// unterschrieben (wie Offline-Bargeld und Umzug).
	quantenWVoraus int64 = 300
	quantenWAlt    int64 = 86400
	// Koerper der Endpunkte: ein Teil der Offenlegung mit 64 Stellen, 16
	// Anteilen und 14 Pfadknoten je Stelle sind ~110 KB JSON; eine Antwort
	// mit 8192 Bits ~8 KB. Die Routen liegen hinter zumLeiter (64 KiB): der
	// Validator-Dienst kuerzt Teile darunter (quantenWTeilKuerzen).
	quantenWKoerperMax = 160 << 10
	burstQuantenWJeIP  = 20
	// quantenWBuchSpiel: wie weit BuchAt (die Zeit der Annahme, mit der jeder
	// Knoten rechnet) hoechstens vor dem Block liegen darf -- dieselbe Regel
	// wie buchZeitBeimNachspielen (7 Tage, 60 s Uhrenspiel nach vorn).
	quantenWBuchSpiel int64 = 7*86400 + 60
	// quantenWToteBehalten: so lange bleibt eine tote Note gemerkt. Eine alte
	// Bestaetigung gilt bis Praegezeit + quantenWAlt, und ein Erzeuger darf
	// BuchAt um quantenWBuchSpiel zurueckdatieren: kuerzer gemerkt, liesse
	// sich eine tote Note mit zurueckdatiertem BuchAt neu praegen.
	quantenWToteBehalten = quantenWAlt + quantenWVoraus + quantenWBuchSpiel + 3600
)

// quantenWBuchZeit: die Zeit, mit der jeder Knoten eine W-Transaktion
// rechnet -- BuchAt des Erzeugers (die Zeit der Annahme), wenn sie zum Block
// passt; sonst ist der Block falsch (block_tauglich nimmt sie dann nicht auf).
// So rechnen Annahme und Nachspielen mit DERSELBEN Zeit, und die Register
// (Anfrage-, Zufall-, Antwortzeit) und damit die StateRoot sind gleich.
func quantenWBuchZeit(art string, buchAt, blockZeit int64) (int64, bool) {
	ok := buchAt > 0 && buchAt <= sattAdd(blockZeit, 60) && buchAt >= blockZeit-quantenWBuchSpielFuer(art)
	return buchAt, ok
}

// quantenWBuchSpielFuer: wie weit die Buchzeit vor dem Block liegen darf. Eine
// Bestellung von Weg B nur kurz (quantenSBestellBuchSpiel): ihre Buchzeit legt
// die drand-Runde des Loses fest; zurueckdatiert laege sie in der
// Vergangenheit, und wer den Block erzeugt, kennte das Los vorher.
func quantenWBuchSpielFuer(art string) int64 {
	if art == quantenSBestellenTxArt {
		return quantenSBestellBuchSpiel
	}
	return quantenWBuchSpiel
}

func quantenWAdresse(domaene string) string {
	h := sha256.Sum256([]byte(domaene))
	return "0x" + hex.EncodeToString(h[12:])
}

// quantenWTopfAddr, quantenWPfandAddr: SHA-256 ohne Urbild, kein Schluessel;
// Protokoll-Toepfe (isTokenomicsPoolAddress). Bis zum Stichtag leer.
var (
	quantenWTopfAddr  = quantenWAdresse("aequitas-quanten-w-topf-v1")
	quantenWPfandAddr = quantenWAdresse("aequitas-quanten-w-pfand-v1")
)

// quantenWAbOverride: nur fuer Tests (0 = Konstante gilt).
var quantenWAbOverride atomic.Int64

func quantenWAb() int64 {
	if v := quantenWAbOverride.Load(); v != 0 {
		return v
	}
	return quantenWAbUnix
}

func quantenWAktiv(t int64) bool { return t >= quantenWAb() }

// quantenWStatus: fuer /api/status -- ab: der Stichtag des laufenden Knotens
// (fehlt = schlafend, MaxInt64; im Hauptnetz seit #570 gesetzt). Daran erkennt der Deploy-Waechter
// .github/neustart/quanten-w-nicht-verpasst.sh, ob ein Wechsel rueckwirkend waere.
func quantenWStatus() map[string]interface{} {
	m := map[string]interface{}{}
	if ab := quantenWAb(); ab != math.MaxInt64 {
		m["ab"] = ab
	}
	// Stichprobe v2 (quanten_s_probe_v2.go): fehlt = schlafend (Deploy-Waechter
	// .github/neustart/quanten-s-probe-v2-nicht-verpasst.sh).
	if ab := quantenSProbeV2Ab(); ab != math.MaxInt64 {
		m["probe_v2_ab"] = ab
	}
	quantenWUrteilStatus(m) // eindeutiges Urteil (quanten_w_urteil.go)
	// Weg B (quanten_s_praeparatoren.go): fehlt = schlafend.
	if ab := quantenSWegBAb(); ab != math.MaxInt64 {
		m["weg_b_ab"] = ab
	}
	return m
}

func istQuantenWTxArt(t string) bool {
	if istQuantenWSTxArt(t) || istQuantenSPraepTxArt(t) || istQuantenSBestellTxArt(t) {
		return true
	}
	switch t {
	case quantenWPraegenTxArt, quantenWAnfrageTxArt, quantenWZufallTxArt, quantenWAntwortTxArt, quantenWOffenlegenTxArt, quantenWFristTxArt:
		return true
	}
	return false
}

// quantenWBetreiberVon: der Betreiber-Mensch hinter einer Signieradresse im
// Validator-Register ("" = keiner oder umstritten). var: Tests ohne
// Datenbank setzen eine Tabelle ein.
var quantenWBetreiberVon = func(cs *ChainState, ctx context.Context, signing string) (string, error) {
	return cs.validatorZuSignieradresseCtx(ctx, signing)
}

// ------------------------------------------------------------ Angaben

// QuantenWAngaben: Transaction.QuantenW.
type QuantenWAngaben struct {
	K string `json:"k"`
	// Praegen
	N              int      `json:"n,omitempty"`
	T              int      `json:"t,omitempty"`
	Wurzel         string   `json:"wurzel,omitempty"`
	Validatoren    []string `json:"validatoren,omitempty"`
	Anker          []string `json:"anker,omitempty"`
	Bestaetigungen []string `json:"bestaetigungen,omitempty"`
	Deckung        int64    `json:"deckung,omitempty"`
	Tol            int      `json:"tol,omitempty"` // Toleranz in Prozent (quantenWKTabelle)
	// Anfrage
	Stellen int    `json:"stellen,omitempty"`
	An      string `json:"an,omitempty"` // Einloesen an ("" = pruefen)
	// Anfrage, Zufall, Antwort, Offenlegen, Frist: Nummer der Anfrage
	Index  int              `json:"index,omitempty"`
	Zufall []QuantenWZufall `json:"zufall,omitempty"`
	Bits   string           `json:"bits,omitempty"` // Antwort: "0101..."
	Offen  []QuantenWOffen  `json:"offen,omitempty"`
	Ab     int              `json:"ab,omitempty"` // Offenlegen: erste Stelle dieses Teils
	// Variante S (quanten_w_s_kette.go): Ausgabe (Inhaber, Stellen, Proben),
	// Bindung D und Wahl C der Inhaberin, Anmeldung der Stelle B mit der
	// Vorlage X, ihrer Stichprobe und der Unterschrift der Inhaberin. Bits als
	// Text "0101..." ("0"/"1" fuer B und C, wegen omitempty).
	Inhaber string   `json:"inhaber,omitempty"`
	Annahme []string `json:"annahme,omitempty"` // die zwei Annahmestellen
	Proben  []string `json:"proben,omitempty"`
	D       string   `json:"d,omitempty"`
	C       string   `json:"c,omitempty"`
	B       string   `json:"b,omitempty"`
	X       string   `json:"x,omitempty"`
	Probe   []int    `json:"probe,omitempty"`
	Vorlage string   `json:"vorlage,omitempty"`
	// Stichprobe v2 (quanten_s_probe_v2.go, schlafend bis zum Stichtag):
	// ProbeArt 2 bei der Ausgabe; beim Anmelden je Abschnitt das Salz (hex).
	ProbeArt int      `json:"probe_art,omitempty"`
	Salze    []string `json:"salze,omitempty"`
	// Weg B (quanten_s_bestellung.go, schlafend): Bestellen mit v
	// Praeparatoren (praep_zahl), Losen mit drand-Runde und ihrer Signatur.
	PraepZahl int    `json:"praep_zahl,omitempty"`
	Runde     uint64 `json:"runde,omitempty"`
	RundeSig  string `json:"runde_sig,omitempty"`
	// Unterschrift der Wallet (Praegen, Anfrage, Antwort): EIP-191 ODER ML-DSA.
	WalletSig string   `json:"wallet_sig,omitempty"`
	PQ        *PQKonto `json:"pq,omitempty"`
	// Vom Erzeuger gerechnet, jeder Knoten rechnet nach (Mikro-AEQ).
	Anteile int64  `json:"anteile,omitempty"`
	Pfand   int64  `json:"pfand,omitempty"`
	PfandAn string `json:"pfand_an,omitempty"` // "" = Grundeinkommen
	Wert    int64  `json:"wert,omitempty"`
	WertAn  string `json:"wert_an,omitempty"` // "" = Grundeinkommen
	Urteil  string `json:"urteil,omitempty"`
}

// QuantenWZufall: der Wert der Hash-Kette von Validator V (1-basiert).
type QuantenWZufall struct {
	V int    `json:"v"`
	Z string `json:"z"`
}

// QuantenWOffen: Anteile und Merkle-Pfad einer Stelle.
type QuantenWOffen struct {
	Anteile []QuantenWAnteilAngabe `json:"anteile"`
	Pfad    []string               `json:"pfad"`
}

// QuantenWAnteilAngabe: Y hexadezimal (hoechstens 32 Zeichen, < 2^127 - 1).
type QuantenWAnteilAngabe struct {
	X int    `json:"x"`
	Y string `json:"y"`
}

// gerechnet: nur die vom Erzeuger gerechneten Felder.
func (q *QuantenWAngaben) gerechnet() QuantenWAngaben {
	return QuantenWAngaben{Anteile: q.Anteile, Pfand: q.Pfand, PfandAn: q.PfandAn, Wert: q.Wert, WertAn: q.WertAn, Urteil: q.Urteil}
}

// quantenWNurEigeneFelder: eine W-Transaktion traegt nur ihre eigenen
// Felder, und je Art nur die Angaben dieser Art.
func quantenWNurEigeneFelder(tx *Transaction) bool {
	k := *tx
	k.Type, k.Wallet, k.DistributionAt, k.QuantenW, k.TxHash, k.BuchAt = "", "", 0, nil, "", 0
	if !reflect.DeepEqual(k, Transaction{}) || tx.QuantenW == nil {
		return false
	}
	q := *tx.QuantenW
	q.K, q.Index = "", 0
	// Ein leeres salze (Stichprobe v2) verwirft ein aelterer Knoten wie ein
	// fehlendes, und omitempty laesst es beim Hashen weg: fuer JEDE Art gleich
	// behandeln -- sonst naehme der aeltere Knoten einen Block an, den der neue
	// abweist (quantenSProbeV2AbUnix).
	if len(q.Salze) == 0 {
		q.Salze = nil
	}
	switch tx.Type {
	case quantenWPraegenTxArt:
		q.N, q.T, q.Wurzel, q.Validatoren, q.Anker, q.Bestaetigungen, q.Deckung, q.WalletSig, q.PQ, q.Anteile, q.Tol = 0, 0, "", nil, nil, nil, 0, "", nil, 0, 0
		return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.QuantenW.Index == 0
	case quantenWAnfrageTxArt:
		q.Stellen, q.An, q.WalletSig, q.PQ, q.Pfand = 0, "", "", nil, 0
	case quantenWZufallTxArt:
		q.Zufall = nil
		return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.Wallet == "" && tx.DistributionAt == 0
	case quantenWAntwortTxArt:
		q.Bits, q.WalletSig, q.PQ = "", "", nil
		return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.DistributionAt == 0
	case quantenWOffenlegenTxArt, quantenWFristTxArt:
		q.Offen, q.Ab, q.Pfand, q.PfandAn, q.Wert, q.WertAn, q.Urteil = nil, 0, 0, "", 0, "", ""
		if tx.Type == quantenWFristTxArt && (tx.QuantenW.Offen != nil || tx.QuantenW.Ab != 0) {
			return false
		}
		return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.Wallet == "" && tx.DistributionAt == 0
	default:
		if istQuantenWSTxArt(tx.Type) {
			return quantenWSNurEigeneFelder(tx, q)
		}
		if istQuantenSPraepTxArt(tx.Type) {
			return quantenSPraepNurEigeneFelder(tx, q)
		}
		if istQuantenSBestellTxArt(tx.Type) {
			return quantenSBestellNurEigeneFelder(tx, q)
		}
		return false
	}
	return reflect.DeepEqual(q, QuantenWAngaben{})
}

// ------------------------------------------------------------ Saetze

func quantenWPraegenNachricht(wallet, K string, n, t int, wurzel string, validatoren, anker []string, deckung, zeit int64, tol int) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d|%d|%s|%s|%s|%d|%d|%d", quantenWDomPraegen, eingefroreneChainID,
		strings.ToLower(wallet), K, n, t, wurzel, strings.Join(validatoren, ","), strings.Join(anker, ","), deckung, zeit, tol)
}

func quantenWAnfrageNachricht(wallet, K string, index, k int, an string, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d|%d|%s|%d", quantenWDomAnfrage, eingefroreneChainID,
		strings.ToLower(wallet), K, index, k, strings.ToLower(an), zeit)
}

func quantenWAntwortNachricht(wallet, K string, index int, bits string) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d|%s", quantenWDomAntwort, eingefroreneChainID, strings.ToLower(wallet), K, index, bits)
}

// quantenWUnterschrift: genau eins von EIP-191 (WalletSig) oder ML-DSA-65
// einer PQ-Adresse (PQ, ab umzugPQAb). Form vor Rechnung.
func quantenWUnterschrift(q *QuantenWAngaben, nachricht, wallet string, blockZeit int64) error {
	if q.PQ != nil {
		if q.WalletSig != "" || !umzugPQAktiv(blockZeit) || !pqKontoForm(q.PQ) {
			return errors.New("PQ-Unterschrift nicht in gueltiger Form")
		}
		return pqKontoPruefen(wallet, nachricht, q.PQ)
	}
	if !kanonischeSignatur(q.WalletSig) {
		return errors.New("Unterschrift der Wallet fehlt oder nicht in gueltiger Form")
	}
	return pruefePersonalSignGemerkt(nachricht, q.WalletSig, wallet)
}

// quantenWTxHash: aus Art, Wallet, Zeit und den eingereichten Angaben (ohne
// die gerechneten) -- dieselbe Einreichung hat denselben Hash.
func quantenWTxHash(tx *Transaction) string {
	q := *tx.QuantenW
	q.Anteile, q.Pfand, q.PfandAn, q.Wert, q.WertAn, q.Urteil = 0, 0, "", 0, "", ""
	roh, _ := json.Marshal(q)
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|", tx.Type, strings.ToLower(tx.Wallet), tx.DistributionAt) + string(roh)))
	return "0x" + hex.EncodeToString(h[:])
}

func quantenWHex32(s string) ([32]byte, bool) {
	var out [32]byte
	if !istHash64(s) {
		return out, false
	}
	b, _ := hex.DecodeString(s)
	copy(out[:], b)
	return out, true
}

// ------------------------------------------------------------ Register

// quantenWEintrag: eine lebende Note mit ihren Anteilen am W-Topf.
type quantenWEintrag struct {
	note    *quantenWNote
	Anteile int64
	Zeit    int64 // Praegezeit
}

type quantenWRegister struct {
	Noten map[string]*quantenWEintrag
	Token map[string]*quantenWSEintrag // Variante S (quanten_w_s_kette.go)
	Tote  map[string]int64             // K -> Praege- bzw. Ausgabezeit
	// Enden: ab dem Stichtag des eindeutigen Urteils (quanten_w_urteil.go)
	// je K in Tote Urteil und Zeit des Endes; Schluessel immer auch in Tote.
	Enden map[string]quantenWEnde
	// Praep: Weg B, Register der Praeparatoren (quanten_s_praeparatoren.go),
	// je K = quantenSPraepK(Konto). Leer bis zum Stichtag von Weg B.
	Praep map[string]quantenSPraep
	// Best: Weg B, offene Bestellungen mit Abbild und Los
	// (quanten_s_bestellung.go), je K.
	Best map[string]*quantenSBestellung
}

// leer: nichts im Register (kodiert als "", StateRoot ohne W-Teil).
func (r *quantenWRegister) leer() bool {
	return len(r.Noten) == 0 && len(r.Token) == 0 && len(r.Tote) == 0 && len(r.Praep) == 0 && len(r.Best) == 0
}

func neuesQuantenWRegister() *quantenWRegister {
	return &quantenWRegister{Noten: map[string]*quantenWEintrag{}, Token: map[string]*quantenWSEintrag{}, Tote: map[string]int64{}, Enden: map[string]quantenWEnde{}, Praep: map[string]quantenSPraep{},
		Best: map[string]*quantenSBestellung{}}
}

func (r *quantenWRegister) kopie() *quantenWRegister {
	n := &quantenWRegister{Noten: make(map[string]*quantenWEintrag, len(r.Noten)), Token: make(map[string]*quantenWSEintrag, len(r.Token)), Tote: make(map[string]int64, len(r.Tote)),
		Enden: make(map[string]quantenWEnde, len(r.Enden)), Praep: make(map[string]quantenSPraep, len(r.Praep)),
		Best: make(map[string]*quantenSBestellung, len(r.Best))}
	for k, e := range r.Noten {
		n.Noten[k] = e
	}
	for k, e := range r.Token {
		n.Token[k] = e
	}
	for k, z := range r.Tote {
		n.Tote[k] = z
	}
	for k, e := range r.Enden {
		n.Enden[k] = e
	}
	for k, e := range r.Praep {
		n.Praep[k] = e
	}
	for k, b := range r.Best {
		n.Best[k] = b // unveraenderlich: Aenderungen ersetzen den Eintrag
	}
	return n
}

func (r *quantenWRegister) summeAnteile() int64 {
	var s int64
	for _, e := range r.Noten {
		s = plusGesaettigt(s, e.Anteile)
	}
	for _, e := range r.Token {
		s = plusGesaettigt(s, e.Anteile)
	}
	return s
}

// tiefeKopie: die Note zum Aendern (das Register im Speicher bleibt unberuehrt).
func (e *quantenWEintrag) tiefeKopie() *quantenWEintrag {
	n := *e.note
	n.Validatoren = slices.Clone(e.note.Validatoren)
	n.Z = slices.Clone(e.note.Z)
	n.ZIndex = slices.Clone(e.note.ZIndex)
	n.Benutzt = make(map[int]bool, len(e.note.Benutzt))
	for i := range e.note.Benutzt {
		n.Benutzt[i] = true
	}
	if a := e.note.Anfrage; a != nil {
		k := *a
		k.Stellen, k.Basen, k.Antwort = slices.Clone(a.Stellen), slices.Clone(a.Basen), slices.Clone(a.Antwort)
		n.Anfrage = &k
	}
	return &quantenWEintrag{note: &n, Anteile: e.Anteile, Zeit: e.Zeit}
}

// Kodierung in chain_config: sortiert, ohne Karten -- jeder Knoten schreibt
// dieselben Bytes.
type quantenWNoteForm struct {
	K           string           `json:"k"`
	N           int              `json:"n"`
	T           int              `json:"t"`
	Wurzel      string           `json:"wurzel"`
	Validatoren []string         `json:"validatoren"`
	Tol         int              `json:"tol"`
	KNote       int              `json:"k_note"`
	Z           []string         `json:"z"`
	ZIndex      []int            `json:"z_index"`
	Anteile     int64            `json:"anteile"`
	Zeit        int64            `json:"zeit"`
	Benutzt     string           `json:"benutzt,omitempty"` // Bitfeld, quantenWBitfeld
	Fehl        int              `json:"fehl,omitempty"`
	Gesperrt    bool             `json:"gesperrt,omitempty"`
	Zaehl       int              `json:"zaehl,omitempty"`
	Anfrage     *quantenWAnfrage `json:"anfrage,omitempty"`
}

type quantenWTotForm struct {
	K    string `json:"k"`
	Zeit int64  `json:"zeit"`
	// Ab dem Stichtag des eindeutigen Urteils (quanten_w_urteil.go); vorher
	// fehlen beide, die Bytes sind unveraendert.
	Ende   int64  `json:"ende,omitempty"`
	Urteil string `json:"urteil,omitempty"`
}

type quantenWRegisterForm struct {
	Noten []quantenWNoteForm   `json:"noten,omitempty"`
	Token []quantenWSTokenForm `json:"token,omitempty"`
	Tote  []quantenWTotForm    `json:"tote,omitempty"`
	// Weg B (quanten_s_praeparatoren.go); fehlt, solange leer -- vor dem
	// Stichtag von Weg B sind die Bytes unveraendert.
	Praep []quantenSPraepForm `json:"praep,omitempty"`
	// Weg B, Bestellungen (quanten_s_bestellung.go); fehlt, solange leer.
	Best []quantenSBestellungForm `json:"best,omitempty"`
}

func (r *quantenWRegister) kodiert() string {
	if r.leer() {
		return ""
	}
	return r.form().kodiert()
}

// form: das Register in kanonischer Form (sortiert, ohne Karten).
func (r *quantenWRegister) form() quantenWRegisterForm {
	var f quantenWRegisterForm
	for k, e := range r.Noten {
		nf := quantenWNoteForm{K: k, N: e.note.N, T: e.note.T, Wurzel: hex.EncodeToString(e.note.Wurzel[:]),
			Validatoren: e.note.Validatoren, Tol: e.note.Tol, KNote: e.note.KNote, ZIndex: e.note.ZIndex, Anteile: e.Anteile, Zeit: e.Zeit,
			Benutzt: quantenWBitfeld(e.note.Benutzt, e.note.N), Fehl: e.note.Fehl, Gesperrt: e.note.Gesperrt, Zaehl: e.note.Zaehl, Anfrage: e.note.Anfrage}
		for _, z := range e.note.Z {
			nf.Z = append(nf.Z, hex.EncodeToString(z[:]))
		}
		f.Noten = append(f.Noten, nf)
	}
	for k, e := range r.Token {
		f.Token = append(f.Token, e.form(k))
	}
	for k, z := range r.Tote {
		en := r.Enden[k]
		f.Tote = append(f.Tote, quantenWTotForm{K: k, Zeit: z, Ende: en.Ende, Urteil: en.Urteil})
	}
	sort.Slice(f.Token, func(i, j int) bool { return f.Token[i].K < f.Token[j].K })
	sort.Slice(f.Noten, func(i, j int) bool { return f.Noten[i].K < f.Noten[j].K })
	sort.Slice(f.Tote, func(i, j int) bool { return f.Tote[i].K < f.Tote[j].K })
	f.Praep = quantenSPraepFormen(r.Praep)
	f.Best = quantenSBestellungenFormen(r.Best)
	return f
}

// kodiert: die Bytes des ganzen Registers (StateRoot, Snapshot).
func (f quantenWRegisterForm) kodiert() string {
	roh, err := json.Marshal(f)
	if err != nil {
		panic(fmt.Sprintf("quanten_w: Register nicht kodierbar: %v", err)) // nur Zahlen, Texte, Listen
	}
	return string(roh)
}

// quantenWBitfeld: die benutzten Stellen als Bitfeld (Stelle i = Bit i%8
// von Byte i/8), hexadezimal, genau ceil(n/8) Bytes; leer, wenn keine.
// Kanonisch: jeder Knoten schreibt dieselben Bytes.
func quantenWBitfeld(benutzt map[int]bool, n int) string {
	if len(benutzt) == 0 {
		return ""
	}
	b := make([]byte, (n+7)/8)
	for i := range benutzt {
		b[i/8] |= 1 << (i % 8)
	}
	return hex.EncodeToString(b)
}

// quantenWBitfeldLesen: die Stellen aus dem Bitfeld; nur die kanonische Form
// (Laenge, Kleinschreibung, keine Bits ab n, nicht leer kodiert).
func quantenWBitfeldLesen(s string, n int) ([]int, bool) {
	if s == "" {
		return nil, true
	}
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != (n+7)/8 || s != hex.EncodeToString(b) {
		return nil, false
	}
	var out []int
	for i := 0; i < len(b)*8; i++ {
		if b[i/8]&(1<<(i%8)) != 0 {
			if i >= n {
				return nil, false
			}
			out = append(out, i)
		}
	}
	return out, len(out) > 0
}

func quantenWBitsGueltig(b []int) bool {
	for _, x := range b {
		if x != 0 && x != 1 {
			return false
		}
	}
	return true
}

// quantenWRegisterLesen: Start, Snapshot, Checkpoint. Alles wird geprueft
// (fail-closed): ein Register, das so nie entstehen konnte, wird abgewiesen.
func quantenWRegisterLesen(s string) (*quantenWRegister, error) {
	r := neuesQuantenWRegister()
	if s == "" {
		return r, nil
	}
	dec := json.NewDecoder(strings.NewReader(s))
	dec.DisallowUnknownFields()
	var f quantenWRegisterForm
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("quanten_w: Register: %w", err)
	}
	if len(f.Noten) == 0 && len(f.Token) == 0 && len(f.Tote) == 0 && len(f.Praep) == 0 && len(f.Best) == 0 {
		return nil, errors.New("quanten_w: leeres Register muss leer kodiert sein")
	}
	// Tote: bis zur Grenze praegen, danach sterben noch die lebenden Noten.
	if len(f.Noten) > quantenWNotenHoechstens || len(f.Token) > quantenWTokenHoechstens || len(f.Tote) > quantenWToteHoechstens+quantenWNotenHoechstens+quantenWTokenHoechstens {
		return nil, errors.New("quanten_w: Register ueber der Grenze")
	}
	var summe int64
	for i, nf := range f.Noten {
		if !istHash64(nf.K) || (i > 0 && nf.K <= f.Noten[i-1].K) {
			return nil, fmt.Errorf("quanten_w: Note %d: K oder Ordnung", i)
		}
		wurzel, ok := quantenWHex32(nf.Wurzel)
		kn, errForm := quantenWPraegenPruefen(false, nf.K, nf.N, nf.T, nf.Validatoren, len(nf.Z), len(nf.Validatoren), nf.Tol)
		if !ok || errForm != nil || nf.KNote != kn ||
			len(nf.ZIndex) != len(nf.Validatoren) || nf.Anteile <= 0 || nf.Anteile > quantenAnteileHoechstens-summe ||
			nf.Fehl < 0 || nf.Gesperrt != (nf.Fehl >= quantenWFehlMax) || nf.Zaehl < 0 || nf.Zaehl > quantenWZufallL || nf.Zeit <= 0 {
			return nil, fmt.Errorf("quanten_w: Note %s: Form", nf.K[:12])
		}
		summe += nf.Anteile
		var anker [][32]byte
		for x, v := range nf.Validatoren {
			z, ok := quantenWHex32(nf.Z[x])
			if !kanonischeAdresse(v) || !ok || nf.ZIndex[x] < -1 || nf.ZIndex[x] >= nf.Zaehl {
				return nil, fmt.Errorf("quanten_w: Note %s: Validator %d", nf.K[:12], x+1)
			}
			anker = append(anker, z)
		}
		e := neueQuantenWNote(nf.N, nf.T, wurzel, nf.Validatoren, anker, nf.Tol, kn)
		copy(e.ZIndex, nf.ZIndex)
		benutzt, ok := quantenWBitfeldLesen(nf.Benutzt, nf.N)
		if !ok {
			return nil, fmt.Errorf("quanten_w: Note %s: benutzte Stellen", nf.K[:12])
		}
		e.verbrauchen(benutzt)
		e.Fehl, e.Gesperrt, e.Zaehl = nf.Fehl, nf.Gesperrt, nf.Zaehl
		if e.erschoepftNoetig() && nf.Anfrage == nil {
			return nil, fmt.Errorf("quanten_w: Note %s: erschoepft, aber lebendig", nf.K[:12])
		}
		if a := nf.Anfrage; a != nil {
			gezogen := a.Basen != nil
			if a.Index != nf.Zaehl-1 || a.K < quantenWKMin || a.K > quantenWKMax || !kanonischeAdresse(a.Von) || a.Pfand < quantenWPfand ||
				(a.EinloesenAn != "" && !kanonischeAdresse(a.EinloesenAn)) || a.Zeit <= 0 ||
				(gezogen != (a.Stellen != nil)) || (!gezogen && (a.Antwort != nil || a.Gezogen != 0)) ||
				(gezogen && (len(a.Stellen) != a.K || len(a.Basen) != a.K || !quantenWBitsGueltig(a.Basen) || a.Gezogen < a.Zeit)) ||
				(a.Antwort != nil && (len(a.Antwort) != a.K || !quantenWBitsGueltig(a.Antwort) || a.AntwortZeit < a.Gezogen)) ||
				(a.Antwort == nil && a.AntwortZeit != 0) ||
				(a.EinloesenAn != "" && a.K != kn) || a.Offen < 0 || a.Offen > a.K || (a.Antwort == nil && a.Offen != 0) ||
				a.Fehler < 0 || a.Fehler > a.Treffer || a.Treffer > a.Offen || (a.Offen == a.K && a.K > 0) {
				return nil, fmt.Errorf("quanten_w: Note %s: offene Anfrage", nf.K[:12])
			}
			for i, st := range a.Stellen {
				if !e.Benutzt[st] || (i > 0 && st <= a.Stellen[i-1]) {
					return nil, fmt.Errorf("quanten_w: Note %s: Stellen der Anfrage", nf.K[:12])
				}
			}
			k := *a
			e.Anfrage = &k
		}
		for x := range e.Z {
			e.Z[x], _ = quantenWHex32(nf.Z[x])
		}
		r.Noten[nf.K] = &quantenWEintrag{note: e, Anteile: nf.Anteile, Zeit: nf.Zeit}
	}
	for i, tf := range f.Token {
		if !istHash64(tf.K) || (i > 0 && tf.K <= f.Token[i-1].K) || r.Noten[tf.K] != nil {
			return nil, fmt.Errorf("quanten_w: Token %d: K oder Ordnung", i)
		}
		e, err := quantenWSTokenLesen(tf)
		if err != nil {
			return nil, err
		}
		if e.Anteile > quantenAnteileHoechstens-summe {
			return nil, fmt.Errorf("quanten_w: Token %s: Anteile", tf.K[:12])
		}
		summe += e.Anteile
		r.Token[tf.K] = e
	}
	for i, t := range f.Tote {
		if !istHash64(t.K) || (i > 0 && t.K <= f.Tote[i-1].K) || r.Noten[t.K] != nil || r.Token[t.K] != nil || t.Zeit <= 0 ||
			!quantenWUrteilGueltig(t.Ende, t.Urteil) {
			return nil, fmt.Errorf("quanten_w: tote Note %d", i)
		}
		r.Tote[t.K] = t.Zeit
		if t.Urteil != "" {
			r.Enden[t.K] = quantenWEnde{Ende: t.Ende, Urteil: t.Urteil}
		}
	}
	praep, err := quantenSPraepLesen(f.Praep)
	if err != nil {
		return nil, err
	}
	r.Praep = praep
	best, err := quantenSBestellungenLesen(f.Best)
	if err != nil {
		return nil, err
	}
	for k := range best {
		if r.Noten[k] != nil || r.Token[k] != nil {
			return nil, fmt.Errorf("quanten_w: Bestellung %s ist schon ausgegeben", k[:12])
		}
		if _, tot := r.Tote[k]; tot {
			return nil, fmt.Errorf("quanten_w: Bestellung %s ist schon tot", k[:12])
		}
	}
	r.Best = best
	return r, nil
}

// quantenWStand: das geltende Register, seine Kodierung und sein Teil der
// StateRoot -- einmal beim Setzen gerechnet. zeilen: wie es in der Tabelle
// steht (quanten_w_tabelle.go; nur mit Datenbank).
type quantenWStand struct {
	reg     *quantenWRegister
	wurzel  string
	kodiert string
	zeilen  map[quantenWZeile]string
}

func neuerQuantenWStand(r *quantenWRegister, kodiert string) *quantenWStand {
	st := &quantenWStand{reg: r, kodiert: kodiert}
	if kodiert != "" {
		h := sha256.Sum256([]byte(kodiert))
		st.wurzel = "|quantenw:" + hex.EncodeToString(h[:])
	}
	return st
}

func (cs *ChainState) quantenWStandJetzt() *quantenWStand {
	if st := cs.quantenWReg.Load(); st != nil {
		return st
	}
	return &quantenWStand{reg: neuesQuantenWRegister()}
}

func (cs *ChainState) quantenWJetzt() *quantenWRegister { return cs.quantenWStandJetzt().reg }

// quantenWWurzel: der Teil der StateRoot ("" bei leerem Register: byte-gleich).
func (cs *ChainState) quantenWWurzel() string { return cs.quantenWStandJetzt().wurzel }

// quantenWLaden: das Register aus der Tabelle (Start, Snapshot, Resync;
// quanten_w_tabelle.go). Steht in chain_config noch ein Register (Knoten
// vor der Tabelle, oder gerade aus einem Snapshot uebernommen) und ist die
// Tabelle leer, zieht es in die Tabelle um; danach ist der Wert in
// chain_config leer. Hat die Tabelle schon Zeilen, gelten sie (ein
// laufender Knoten behaelt seinen Stand).
func (cs *ChainState) quantenWLaden(ctx context.Context) error {
	if cs.db == nil {
		return nil
	}
	if txFromContext(ctx) == nil {
		tx, err := cs.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("quanten_w: Register laden: %w", err)
		}
		defer tx.Rollback()
		if err := cs.quantenWLadenIn(withTx(ctx, tx)); err != nil {
			return err
		}
		return tx.Commit()
	}
	return cs.quantenWLadenIn(ctx)
}

func (cs *ChainState) quantenWLadenIn(ctx context.Context) error {
	q := cs.dbExecCtx(ctx)
	ausTabelle, zeilen, err := quantenWZeilenLesen(q)
	if err != nil {
		return err
	}
	// Streng gelesen (getConfigValueCtx verschluckte einen Fehler; dann
	// fehlte beim Umzug das ganze Register).
	var alt string
	if err := q.QueryRow(`SELECT value FROM chain_config WHERE key = $1`, quantenWKonfigSchluessel).Scan(&alt); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("quanten_w: %s lesen: %w", quantenWKonfigSchluessel, err)
	}
	quelle := ausTabelle
	umziehen := zeilen == 0 && alt != ""
	if umziehen {
		quelle = alt
	}
	r, err := quantenWRegisterLesen(quelle)
	if err != nil {
		return err
	}
	st := neuerQuantenWStand(r, r.kodiert())
	st.zeilen = r.form().zeilen()
	if umziehen {
		if err := quantenWZeilenSchreiben(q, nil, st.zeilen); err != nil {
			return err
		}
	}
	if alt != "" {
		if err := cs.setConfigValueCtx(ctx, quantenWKonfigSchluessel, ""); err != nil {
			return fmt.Errorf("quanten_w: alten Wert in chain_config leeren: %w", err)
		}
	}
	cs.quantenWReg.Store(st)
	return nil
}

// quantenWSetzenLocked: in der laufenden Datenbank-Transaktion schreiben --
// nur die Zeilen, die sich geaendert haben.
func (cs *ChainState) quantenWSetzenLocked(ctx context.Context, r *quantenWRegister) error {
	f := r.form()
	k := ""
	if !r.leer() {
		k = f.kodiert()
	}
	st := neuerQuantenWStand(r, k)
	if cs.db != nil {
		st.zeilen = f.zeilen()
		if err := quantenWZeilenSchreiben(cs.dbExecCtx(ctx), cs.quantenWStandJetzt().zeilen, st.zeilen); err != nil {
			return fmt.Errorf("quanten_w: Register speichern: %w", err)
		}
	}
	cs.quantenWReg.Store(st)
	return nil
}

func (cs *ChainState) quantenWMikroLocked(ctx context.Context, addr string) int64 {
	cs.ensureAccountLoadedCtx(ctx, addr)
	if acc, ok := cs.accounts.Get(addr); ok && acc.Balance > 0 {
		return acc.Balance.Micro()
	}
	return 0
}

// ------------------------------------------------------------ Form im Block

// quantenWFeldErlaubt: W-Angaben nur an den W-Arten, die Arten nur ab dem
// Stichtag, und ab dem Stichtag keine Ueberweisung an einen der W-Toepfe.
// Jeder Knoten, beim Nachspielen.
func quantenWFeldErlaubt(tx *Transaction, blockZeit int64) error {
	art := istQuantenWTxArt(tx.Type)
	if tx.QuantenW == nil && !art {
		to := strings.TrimSpace(tx.To)
		if tx.Type == "transfer" && quantenWAktiv(blockZeit) && (strings.EqualFold(to, quantenWTopfAddr) || strings.EqualFold(to, quantenWPfandAddr)) {
			return errors.New("Ueberweisung an einen Topf von Variante W")
		}
		return nil
	}
	if tx.QuantenW == nil || !art {
		return fmt.Errorf("%s: W-Angaben nur an den W-Arten", tx.Type)
	}
	if !quantenWAktiv(blockZeit) {
		return fmt.Errorf("%s vor dem Stichtag (Block %d)", tx.Type, blockZeit)
	}
	return nil
}

// quantenWZeitGueltig: Praegen und Anfrage tragen ihre unterschriebene Zeit.
func quantenWZeitGueltig(tx *Transaction, blockZeit int64) bool {
	switch tx.Type {
	case quantenWPraegenTxArt, quantenWAnfrageTxArt, quantenWSAusgebenTxArt:
		z := tx.DistributionAt
		return z > 0 && z <= sattAdd(blockZeit, quantenWVoraus) && blockZeit-z <= quantenWAlt
	case quantenSBestellenTxArt:
		z := tx.DistributionAt
		return z > 0 && z <= sattAdd(blockZeit, quantenWVoraus) && blockZeit-z <= quantenWAlt
	case quantenSLosenTxArt:
		return tx.DistributionAt == 0
	case quantenSPraepMeldenTxArt, quantenSPraepAbmeldenTxArt:
		// Abmelden ohne Wallet (Aufraeumen) traegt keine Zeit.
		z := tx.DistributionAt
		if tx.Type == quantenSPraepAbmeldenTxArt && tx.Wallet == "" {
			return z == 0
		}
		return z > 0 && z <= sattAdd(blockZeit, quantenWVoraus) && blockZeit-z <= quantenWAlt
	}
	return true
}

// ------------------------------------------------------------ Urteil

type quantenWWeg struct {
	von, an string
	mikro   int64
}

// quantenWBescheid: was eine W-Transaktion bewirkt -- oder warum nicht.
type quantenWBescheid struct {
	regel, text string
	reg         *quantenWRegister
	wege        []quantenWWeg
	felder      QuantenWAngaben // nur die gerechneten
}

func quantenWBefund(regel, format string, args ...interface{}) quantenWBescheid {
	return quantenWBescheid{regel: regel, text: fmt.Sprintf(format, args...)}
}

// quantenWBescheidLocked: dieselbe Pruefung fuer Annahme (Uhrzeit) und
// Nachspielen (Blockzeit). Fehler = Zustand nicht lesbar (der Block geht
// zurueck, die Annahme antwortet 503). Caller haelt cs.mu.
func (cs *ChainState) quantenWBescheidLocked(ctx context.Context, tx *Transaction, blockZeit int64) (quantenWBescheid, error) {
	q := tx.QuantenW
	if !quantenWAktiv(blockZeit) {
		return quantenWBefund("quanten_w_inaktiv", "Variante W ist noch nicht aktiv"), nil
	}
	if q == nil || !istQuantenWTxArt(tx.Type) || !quantenWNurEigeneFelder(tx) || !istHash64(q.K) {
		return quantenWBefund("quanten_w_form", "keine W-Transaktion in gueltiger Form"), nil
	}
	if !quantenWZeitGueltig(tx, blockZeit) {
		return quantenWBefund("quanten_w_zeit", "%s: Zeit %d passt nicht zu Block %d", tx.Type, tx.DistributionAt, blockZeit), nil
	}
	reg := cs.quantenWJetzt()
	wallet := strings.ToLower(strings.TrimSpace(tx.Wallet))
	if tx.Type == quantenWPraegenTxArt {
		return cs.quantenWPraegenBescheidLocked(ctx, tx, wallet, reg, blockZeit)
	}
	if istQuantenWSTxArt(tx.Type) {
		return cs.quantenWSBescheidLocked(ctx, tx, wallet, reg, blockZeit)
	}
	if istQuantenSPraepTxArt(tx.Type) {
		return cs.quantenSPraepBescheidLocked(ctx, tx, wallet, reg, blockZeit)
	}
	if istQuantenSBestellTxArt(tx.Type) {
		return cs.quantenSBestellBescheidLocked(ctx, tx, wallet, reg, blockZeit)
	}
	alt := reg.Noten[q.K]
	if alt == nil {
		return quantenWBefund("quanten_w_unbekannt", "Note %s ist nicht (mehr) da", q.K[:12]), nil
	}
	neu := alt.tiefeKopie()
	e := neu.note
	u := quantenWBescheid{}
	var buchung quantenWBuchung
	var pfand int64
	switch tx.Type {
	case quantenWAnfrageTxArt:
		if !kanonischeAdresse(wallet) || q.Index != e.Zaehl {
			return quantenWBefund("quanten_w_form", "Anfrage: Wallet oder Nummer (%d, faellig %d)", q.Index, e.Zaehl), nil
		}
		an := strings.ToLower(strings.TrimSpace(q.An))
		if q.An != an || (an != "" && (!kanonischeAdresse(an) || istSystemAdresse(an))) {
			return quantenWBefund("quanten_w_form", "Einloesen an %q", q.An), nil
		}
		if err := quantenWUnterschrift(q, quantenWAnfrageNachricht(wallet, q.K, q.Index, q.Stellen, an, tx.DistributionAt), wallet, blockZeit); err != nil {
			return quantenWBefund("quanten_w_unterschrift", "%s: %v", kurzAdresse(wallet), err), nil
		}
		wert := quantenWert(alt.Anteile, cs.quantenWMikroLocked(ctx, quantenWTopfAddr), reg.summeAnteile())
		p := quantenWPfandFuer(wert, q.Stellen, e.N)
		if err := e.anfragen(wallet, q.Stellen, an, p, blockZeit); err != nil {
			return quantenWBefund("quanten_w_regel", "%v", err), nil
		}
		if cs.quantenWMikroLocked(ctx, wallet) < p {
			return quantenWBefund("quanten_w_deckung", "%s hat keine %d Mikro-AEQ Pfand", kurzAdresse(wallet), p), nil
		}
		u.wege = []quantenWWeg{{wallet, quantenWPfandAddr, p}}
		u.felder.Pfand = p

	case quantenWZufallTxArt:
		if len(q.Zufall) == 0 || len(q.Zufall) > len(e.Validatoren) {
			return quantenWBefund("quanten_w_form", "Zufall: %d Werte", len(q.Zufall)), nil
		}
		werte := map[int][32]byte{}
		for _, z := range q.Zufall {
			w, ok := quantenWHex32(z.Z)
			if _, doppelt := werte[z.V-1]; !ok || doppelt || z.V < 1 || z.V > len(e.Validatoren) {
				return quantenWBefund("quanten_w_form", "Zufall von Validator %d", z.V), nil
			}
			werte[z.V-1] = w
		}
		if _, err := e.zufall(q.K, q.Index, werte, blockZeit); err != nil {
			return quantenWBefund("quanten_w_regel", "%v", err), nil
		}

	case quantenWAntwortTxArt:
		a := e.Anfrage
		if !kanonischeAdresse(wallet) || a == nil || q.Index != a.Index || len(q.Bits) == 0 || len(q.Bits) > quantenWKMax {
			return quantenWBefund("quanten_w_form", "Antwort: Wallet, Nummer oder Laenge"), nil
		}
		bits := make([]int, len(q.Bits))
		for i, c := range q.Bits {
			if c != '0' && c != '1' {
				return quantenWBefund("quanten_w_form", "Antwort: nur 0 und 1"), nil
			}
			bits[i] = int(c - '0')
		}
		if err := quantenWUnterschrift(q, quantenWAntwortNachricht(wallet, q.K, q.Index, q.Bits), wallet, blockZeit); err != nil {
			return quantenWBefund("quanten_w_unterschrift", "%s: %v", kurzAdresse(wallet), err), nil
		}
		if err := e.antworten(wallet, bits, blockZeit); err != nil {
			return quantenWBefund("quanten_w_regel", "%v", err), nil
		}

	case quantenWOffenlegenTxArt:
		a := e.Anfrage
		if a == nil || q.Index != a.Index || len(q.Offen) == 0 || len(q.Offen) > quantenWOffenTeil || q.Ab < 0 {
			return quantenWBefund("quanten_w_form", "Offenlegen: Nummer, Beginn oder Zahl der Stellen"), nil
		}
		offen, ok := quantenWOffenLesen(q.Offen)
		if !ok {
			return quantenWBefund("quanten_w_form", "Offenlegen: Anteile oder Pfade nicht in gueltiger Form"), nil
		}
		pfand = a.Pfand
		urteil, b, fertig, err := e.offenlegen(q.K, q.Ab, offen, blockZeit)
		if err != nil {
			return quantenWBefund("quanten_w_regel", "%v", err), nil
		}
		buchung = b
		if fertig {
			u.felder.Urteil = map[quantenWErgebnis]string{quantenWEcht: "echt", quantenWAbgewiesen: "abgewiesen", quantenWUnentschieden: "unentschieden"}[urteil]
		} else {
			u.felder.Urteil = "teil"
		}

	case quantenWFristTxArt:
		a := e.Anfrage
		if a == nil || q.Index != a.Index {
			return quantenWBefund("quanten_w_form", "Frist: keine offene Anfrage mit Nummer %d", q.Index), nil
		}
		pfand = a.Pfand
		ab, b := e.frist(blockZeit)
		if !ab {
			return quantenWBefund("quanten_w_frist", "Anfrage %d an %s: keine Frist abgelaufen", q.Index, q.K[:12]), nil
		}
		buchung = b
		u.felder.Urteil = "frist"
	}

	// Buchungen aus dem Ergebnis: Pfand zurueck oder ans Grundeinkommen,
	// Wert an das Ziel oder ans Grundeinkommen.
	if buchung.PfandZurueckAn != "" {
		u.wege = append(u.wege, quantenWWeg{quantenWPfandAddr, buchung.PfandZurueckAn, pfand})
		u.felder.Pfand, u.felder.PfandAn = pfand, buchung.PfandZurueckAn
	}
	if buchung.PfandVerfaellt {
		u.wege = append(u.wege, quantenWWeg{quantenWPfandAddr, ubiPoolAddr, pfand})
		u.felder.Pfand = pfand
	}
	u.reg = reg.kopie()
	if e.Tot {
		wert := quantenWert(alt.Anteile, cs.quantenWMikroLocked(ctx, quantenWTopfAddr), reg.summeAnteile())
		an := ubiPoolAddr
		if buchung.WertAn != "" {
			an = buchung.WertAn
			u.felder.WertAn = an
		}
		u.wege = append(u.wege, quantenWWeg{quantenWTopfAddr, an, wert})
		u.felder.Wert = wert
		delete(u.reg.Noten, q.K)
		urteil := quantenWUrteilTot // gesperrt: der Wert geht ans Grundeinkommen
		if buchung.WertAn != "" {
			urteil = quantenWUrteilEingeloest
		}
		u.reg.toteMerken(q.K, alt.Zeit, blockZeit, urteil)
	} else {
		u.reg.Noten[q.K] = neu
	}
	return u, nil
}

// quantenWPraegenBescheidLocked: Form, Unterschriften von Wallet und allen
// Validatoren, Register der Validatoren, Deckung, Anteile.
func (cs *ChainState) quantenWPraegenBescheidLocked(ctx context.Context, tx *Transaction, wallet string, reg *quantenWRegister, blockZeit int64) (quantenWBescheid, error) {
	q := tx.QuantenW
	zeit := tx.DistributionAt
	if !kanonischeAdresse(wallet) || istSystemAdresse(wallet) {
		return quantenWBefund("quanten_w_form", "Praeger nicht kanonisch"), nil
	}
	if reg.Noten[q.K] != nil || reg.Token[q.K] != nil {
		return quantenWBefund("quanten_w_doppelt", "Note %s gibt es schon", q.K[:12]), nil
	}
	// Weg B: ein bestelltes K gehoert der Bestellung (quanten_s_bestellung.go);
	// das Register laesst beides zugleich nicht zu (quantenWRegisterLesen).
	if reg.Best[q.K] != nil {
		return quantenWBefund("quanten_w_doppelt", "%s ist fuer Weg B bestellt", q.K[:12]), nil
	}
	if _, tot := reg.Tote[q.K]; tot {
		return quantenWBefund("quanten_w_doppelt", "Note %s war schon gepraegt (tot)", q.K[:12]), nil
	}
	if len(reg.Noten) >= quantenWNotenHoechstens {
		return quantenWBefund("quanten_w_voll", "Register voll (%d Noten)", len(reg.Noten)), nil
	}
	wurzel, ok := quantenWHex32(q.Wurzel)
	if !ok || len(q.Anker) != len(q.Validatoren) || len(q.Bestaetigungen) != len(q.Validatoren) || len(q.Validatoren) > quantenWVMax {
		return quantenWBefund("quanten_w_form", "Wurzel, Anker oder Bestaetigungen"), nil
	}
	var anker [][32]byte
	for x, v := range q.Validatoren {
		a, ok := quantenWHex32(q.Anker[x])
		if !ok || !kanonischeAdresse(v) {
			return quantenWBefund("quanten_w_form", "Validator %d oder sein Anker", x+1), nil
		}
		anker = append(anker, a)
	}
	kn, err := quantenWPraegenPruefen(false, q.K, q.N, q.T, q.Validatoren, len(anker), len(q.Validatoren), q.Tol)
	if err != nil {
		return quantenWBefund("quanten_w_form", "%v", err), nil
	}
	if err := quantenWUnterschrift(q, quantenWPraegenNachricht(wallet, q.K, q.N, q.T, q.Wurzel, q.Validatoren, q.Anker, q.Deckung, zeit, q.Tol), wallet, blockZeit); err != nil {
		return quantenWBefund("quanten_w_unterschrift", "Praeger %s: %v", kurzAdresse(wallet), err), nil
	}
	// Jeder Validator: Bestaetigung (mit Anker und Praegezeit), gebunden an
	// einen Menschen, und alle an verschiedene.
	if u, err := cs.quantenWValidatorenLocked(ctx, q.Validatoren, q.Bestaetigungen, func(x int) string {
		return quantenWBestaetigungNachricht(q.K, wurzel, q.N, q.T, q.Validatoren, x+1, anker[x], zeit, q.Tol)
	}); err != nil || u.regel != "" {
		return u, err
	}
	if q.Deckung < quantenMindestDeckungMik {
		return quantenWBefund("quanten_w_deckung", "Deckung %d, mindestens %d Mikro-AEQ", q.Deckung, quantenMindestDeckungMik), nil
	}
	if cs.quantenWMikroLocked(ctx, wallet) < q.Deckung {
		return quantenWBefund("quanten_w_deckung", "%s hat keine %d Mikro-AEQ", kurzAdresse(wallet), q.Deckung), nil
	}
	anteile, ok := quantenAnteileFuer(q.Deckung, cs.quantenWMikroLocked(ctx, quantenWTopfAddr), reg.summeAnteile())
	if !ok {
		return quantenWBefund("quanten_w_voll", "Topf nimmt keine Note an (Anteile)"), nil
	}
	neu := reg.kopie()
	if !quantenWToteAufraeumen(neu, blockZeit) {
		return quantenWBefund("quanten_w_voll", "zu viele tote Noten im Praegefenster"), nil
	}
	neu.Noten[q.K] = &quantenWEintrag{note: neueQuantenWNote(q.N, q.T, wurzel, q.Validatoren, anker, q.Tol, kn), Anteile: anteile, Zeit: zeit}
	return quantenWBescheid{reg: neu, wege: []quantenWWeg{{wallet, quantenWTopfAddr, q.Deckung}}, felder: QuantenWAngaben{Anteile: anteile}}, nil
}

// quantenWValidatorenLocked: jeder Validator hat nachricht(x) per EIP-191
// unterschrieben, ist im Validator-Register an einen Menschen gebunden, und
// alle an VERSCHIEDENE Menschen -- sonst hielte einer mehrere Anteile. Fuer
// W-Noten und S-Token. Fehler = Register nicht lesbar (fail-closed).
func (cs *ChainState) quantenWValidatorenLocked(ctx context.Context, validatoren, bestaetigungen []string, nachricht func(x int) string) (quantenWBescheid, error) {
	if len(bestaetigungen) != len(validatoren) {
		return quantenWBefund("quanten_w_form", "Bestaetigungen"), nil
	}
	betreiber := map[string]bool{}
	for x, v := range validatoren {
		if !kanonischeSignatur(bestaetigungen[x]) || pruefePersonalSignGemerkt(nachricht(x), bestaetigungen[x], v) != nil {
			return quantenWBefund("quanten_w_bestaetigung", "Validator %d (%s) hat nicht bestaetigt", x+1, kurzAdresse(v)), nil
		}
		b, err := quantenWBetreiberVon(cs, ctx, v)
		if err != nil {
			return quantenWBescheid{}, fmt.Errorf("quanten_w: Validator-Register nicht lesbar: %w", err)
		}
		b = strings.ToLower(b)
		if b == "" {
			return quantenWBefund("quanten_w_validator", "%s ist kein gebundener Validator", kurzAdresse(v)), nil
		}
		cs.ensureAccountLoadedCtx(ctx, b)
		if acc, ok := cs.accounts.Get(b); !ok || !acc.IsHuman {
			return quantenWBefund("quanten_w_validator", "Betreiber von %s ist kein Mensch", kurzAdresse(v)), nil
		}
		if betreiber[b] {
			return quantenWBefund("quanten_w_validator", "zwei Validatoren gehoeren demselben Menschen (%s)", kurzAdresse(b)), nil
		}
		betreiber[b] = true
	}
	return quantenWBescheid{}, nil
}

// quantenWToteAufraeumen: Tote, deren Bestaetigungen nicht mehr gelten
// koennen, vergessen; voll = kein neues Praegen oder Ausgeben (fail-closed).
func quantenWToteAufraeumen(neu *quantenWRegister, blockZeit int64) bool {
	for k, z := range neu.Tote {
		// Mit Urteil: ab dem spaeteren von Ausgabe und Ende gezaehlt.
		if en, ok := neu.Enden[k]; ok && en.Ende > z {
			z = en.Ende
		}
		if z < blockZeit-quantenWToteBehalten {
			delete(neu.Tote, k)
			delete(neu.Enden, k)
		}
	}
	return len(neu.Tote) < quantenWToteHoechstens
}

// quantenWOffenLesen: Anteile und Pfade aus der Transaktion (Form, Grenzen).
func quantenWOffenLesen(in []QuantenWOffen) ([]quantenWOffenStelle, bool) {
	var out []quantenWOffenStelle
	for _, o := range in {
		if len(o.Anteile) == 0 || len(o.Anteile) > quantenWTMax || len(o.Pfad) > quantenWPfadMax {
			return nil, false
		}
		var s quantenWOffenStelle
		for _, a := range o.Anteile {
			if len(a.Y) == 0 || len(a.Y) > 32 || !kleinHex(a.Y) {
				return nil, false
			}
			y, ok := new(big.Int).SetString(a.Y, 16)
			if !ok {
				return nil, false
			}
			s.Anteile = append(s.Anteile, quantenWAnteil{X: a.X, Y: y})
		}
		for _, p := range o.Pfad {
			h, ok := quantenWHex32(p)
			if !ok {
				return nil, false
			}
			s.Pfad = append(s.Pfad, h)
		}
		out = append(out, s)
	}
	return out, true
}

// ------------------------------------------------------------ Anwenden

// quantenWBewegenLocked: Mikro-AEQ von -> an, ohne Gebuehr. Fehler: Deckung
// fehlt (Zustand lehnt ab) -- dann wird nichts angewandt.
func (cs *ChainState) quantenWBewegenLocked(ctx context.Context, w quantenWWeg) error {
	if w.mikro <= 0 {
		return nil
	}
	if w.von == w.an {
		return fmt.Errorf("quanten_w: %s an sich selbst: %w", kurzAdresse(w.von), ErrZustandLehntAb)
	}
	cs.ensureAccountLoadedCtx(ctx, w.von)
	cs.ensureAccountLoadedCtx(ctx, w.an)
	quelle, ok := cs.accounts.Get(w.von)
	betrag := Decimal(w.mikro)
	if !ok || quelle.Balance < betrag {
		return fmt.Errorf("quanten_w: %s deckt %d Mikro-AEQ nicht: %w", kurzAdresse(w.von), w.mikro, ErrZustandLehntAb)
	}
	ziel, da := cs.accounts.Get(w.an)
	if !da {
		ziel = &AccountState{Address: w.an}
		cs.accounts.Set(w.an, ziel)
	}
	quelle.Balance = quelle.Balance.Sub(betrag)
	ziel.Balance = ziel.Balance.Add(betrag)
	for _, a := range []*AccountState{quelle, ziel} {
		cs.updateAccountLeafLocked(a)
		if err := cs.saveAccountToDBCtx(ctx, a); err != nil {
			return fmt.Errorf("quanten_w: %w", err)
		}
	}
	return nil
}

func (cs *ChainState) quantenWAnwendenLocked(ctx context.Context, u quantenWBescheid) error {
	for _, w := range u.wege {
		if err := cs.quantenWBewegenLocked(ctx, w); err != nil {
			return err
		}
	}
	return cs.quantenWSetzenLocked(ctx, u.reg)
}

// applyQuantenWLocked: beim Nachspielen. Die gerechneten Felder im Block
// muessen genau die gerechneten sein. Fehler: der Block ist falsch (oder
// der Zustand nicht lesbar) -- nichts angewandt.
func (cs *ChainState) applyQuantenWLocked(ctx context.Context, tx *Transaction, blockZeit int64) error {
	zeit, ok := quantenWBuchZeit(tx.Type, tx.BuchAt, blockZeit)
	if !ok {
		return fmt.Errorf("%s: gebucht um %d passt nicht zu Block %d", tx.Type, tx.BuchAt, blockZeit)
	}
	u, err := cs.quantenWBescheidLocked(ctx, tx, zeit)
	if err != nil {
		return err
	}
	if u.regel != "" {
		return fmt.Errorf("%s %s: %s", tx.Type, u.regel, u.text)
	}
	if g := tx.QuantenW.gerechnet(); !reflect.DeepEqual(g, u.felder) {
		return fmt.Errorf("%s: Block nennt %+v, gerechnet %+v", tx.Type, g, u.felder)
	}
	return cs.quantenWAnwendenLocked(ctx, u)
}

// quantenWKonten: die Konten, die eine W-Transaktion bewegen kann (Rueckroll
// im Block, Sperre bei der Annahme). Nach dem Urteil stehen sie in den
// gerechneten Feldern.
func quantenWKonten(tx *Transaction) []string {
	k := []string{quantenWTopfAddr, quantenWPfandAddr, ubiPoolAddr}
	if w := strings.ToLower(strings.TrimSpace(tx.Wallet)); w != "" {
		k = append(k, w)
	}
	if q := tx.QuantenW; q != nil {
		for _, a := range []string{q.PfandAn, q.WertAn} {
			if a != "" {
				k = append(k, strings.ToLower(a))
			}
		}
	}
	return k
}

// ------------------------------------------------------------ Annahme

type quantenWAbweisung struct{ text string }

func (e *quantenWAbweisung) Error() string { return e.text }

// quantenWAnnehmen: unter der Sperre genau die Pruefung jedes
// Nachspielenden; Geld und Register atomar mit dem Ausgang.
func (cs *ChainState) quantenWAnnehmen(tx Transaction) (Transaction, error) {
	q := *tx.QuantenW
	q.Anteile, q.Pfand, q.PfandAn, q.Wert, q.WertAn, q.Urteil = 0, 0, "", 0, "", ""
	tx.QuantenW = &q
	tx.TxHash = quantenWTxHash(&tx)
	konten := quantenWKonten(&tx)
	// Variante S: die angemeldete Stelle bekommt beim Offenlegen den Wert.
	if e := cs.quantenWJetzt().Token[q.K]; e != nil && e.token.Anmeldung != nil {
		konten = append(konten, e.token.Anmeldung.Stelle)
	}
	if e := cs.quantenWJetzt().Noten[q.K]; e != nil {
		if a := e.note.Anfrage; a != nil {
			konten = append(konten, a.Von)
			if a.EinloesenAn != "" {
				konten = append(konten, a.EinloesenAn)
			}
		}
	}
	var fertig Transaction
	err := cs.runAtomicWithOutbox(konten, false, func(ctx context.Context) (Transaction, error) {
		at := nowUnix()
		tx.BuchAt = at // jeder Nachspielende rechnet mit dieser Zeit (quantenWBuchZeit)
		u, err := cs.quantenWBescheidLocked(ctx, &tx, at)
		if err != nil {
			return Transaction{}, err
		}
		if u.regel != "" {
			return Transaction{}, &quantenWAbweisung{text: u.regel + ": " + u.text}
		}
		for _, w := range u.wege {
			if !slices.Contains(konten, w.von) || !slices.Contains(konten, w.an) {
				return Transaction{}, &quantenWAbweisung{text: "quanten_w_wiederholen: die Note hat sich geaendert, bitte erneut einreichen"}
			}
		}
		g := *tx.QuantenW
		g.Anteile, g.Pfand, g.PfandAn, g.Wert, g.WertAn, g.Urteil = u.felder.Anteile, u.felder.Pfand, u.felder.PfandAn, u.felder.Wert, u.felder.WertAn, u.felder.Urteil
		tx.QuantenW = &g
		if err := cs.quantenWAnwendenLocked(ctx, u); err != nil {
			return Transaction{}, err
		}
		fertig = tx
		return tx, nil
	})
	return fertig, err
}

// handleQuantenW: POST /api/quanten-w/<art> mit {"wallet", "zeit", "quanten_w": {...}}.
// Vor dem Stichtag 409. Jeder Knoten prueft beim Nachspielen dasselbe.
func (a *APIServer) handleQuantenW(art string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSONCORS(w)
		if r.Method != http.MethodPost {
			jsonError(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if !quantenWAktiv(time.Now().Unix()) {
			jsonError(w, "quantum money variant W not active", http.StatusConflict)
			return
		}
		if (istQuantenSPraepTxArt(art) || istQuantenSBestellTxArt(art)) && !quantenSWegBAktiv(time.Now().Unix()) {
			jsonError(w, "quantum money way B (preparators) not active", http.StatusConflict)
			return
		}
		if !burstErlaubt("quanten-w:"+erneuerungsAbsender(r), burstQuantenWJeIP, burstFenster) {
			jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
			return
		}
		var req struct {
			Wallet   string          `json:"wallet"`
			Zeit     int64           `json:"zeit"`
			QuantenW QuantenWAngaben `json:"quanten_w"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, quantenWKoerperMax))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		q := req.QuantenW
		q.K = strings.ToLower(strings.TrimSpace(q.K))
		tx := Transaction{Type: art, Wallet: strings.ToLower(strings.TrimSpace(req.Wallet)), DistributionAt: req.Zeit, QuantenW: &q}
		fertig, err := a.state.quantenWAnnehmen(tx)
		if err != nil {
			var ab *quantenWAbweisung
			if errors.As(err, &ab) {
				jsonError(w, "rejected: "+ab.text, http.StatusConflict)
				return
			}
			if istWiederholbareAnnahmeAblehnung(err) {
				jsonError(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			fmt.Printf("[API] %s gescheitert: %v\n", art, err)
			jsonError(w, "internal error, please retry shortly", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "art": art, "tx_hash": fertig.TxHash, "gerechnet": fertig.QuantenW.gerechnet()})
	}
}

// handleQuantenWNote: GET /api/quanten-w/note?k=<K> -- der oeffentliche Stand
// einer Note, vor allem die Aufforderung der offenen Anfrage (Stellen und
// Basen): der Halter misst danach.
func (a *APIServer) handleQuantenWNote(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	k := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("k")))
	if !istHash64(k) {
		jsonError(w, "k: 64 hex characters", http.StatusBadRequest)
		return
	}
	antwort := a.state.quantenWNoteAnsicht(r.Context(), k)
	if antwort == nil {
		jsonError(w, "unknown note", http.StatusNotFound)
		return
	}
	antwort["aktiv"] = quantenWAktiv(time.Now().Unix())
	json.NewEncoder(w).Encode(antwort)
}

// quantenWNoteAnsicht: der oeffentliche Stand einer Note (nil = unbekannt).
func (cs *ChainState) quantenWNoteAnsicht(ctx context.Context, k string) map[string]interface{} {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	reg := cs.quantenWJetzt()
	if e := reg.Noten[k]; e != nil {
		return map[string]interface{}{"k": k, "n": e.note.N, "t": e.note.T, "validatoren": e.note.Validatoren,
			"tol": e.note.Tol, "k_note": e.note.KNote,
			"frei": e.note.N - len(e.note.Benutzt), "fehl": e.note.Fehl, "gesperrt": e.note.Gesperrt, "zaehl": e.note.Zaehl,
			"wert_mikro": quantenWert(e.Anteile, cs.quantenWMikroLocked(ctx, quantenWTopfAddr), reg.summeAnteile()),
			"anfrage":    e.note.Anfrage}
	}
	if v := cs.quantenWSTokenAnsichtLocked(ctx, reg, k); v != nil {
		return v
	}
	if _, tot := reg.Tote[k]; tot {
		// "tot" heisst hier: beendet (eingeloest oder tot). Das Urteil steht
		// ab dem Stichtag dabei (quanten_w_urteil.go); fehlt es, ist es offen.
		v := map[string]interface{}{"k": k, "tot": true}
		if en, ok := reg.Enden[k]; ok {
			v["urteil"], v["ende"] = en.Urteil, en.Ende
		}
		return v
	}
	return nil
}
