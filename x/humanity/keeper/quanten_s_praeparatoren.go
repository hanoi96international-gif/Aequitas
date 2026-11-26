package keeper

// QUANTENGELD S, WEG B -- STUFE B2a: REGISTER DER PRAEPARATOREN
// (forschung/quantengeld/DEZENTRAL.md 18, Spezifikation s_weg_b.py Register)
//
// Weg B: Jeder registrierte Mensch kann sich mit dem Handy als Praeparator
// melden. Je Token lost die Kette spaeter (B2b) v Praeparatoren aus dem
// Abbild dieses Registers zur Bestellung. Diese Stufe baut nur das Register:
//
//   - quanten_w_s_praep_melden (Wallet = Konto, unterschrieben): nur ein
//     Konto, das JETZT ein Mensch ist (chain_accounts.is_human). Ein Mensch
//     hat genau eine Wallet mit is_human (Nullifier), also hoechstens einen
//     lebenden Eintrag im Abbild. Meldezeit = Buchzeit des Knotens, nicht die
//     unterschriebene Zeit (die koennte der Melder zurueckdatieren).
//   - quanten_w_s_praep_abmelden (Wallet = Konto, unterschrieben): Abmeldezeit
//     = Buchzeit. Der Eintrag BLEIBT mit Abmeldezeit stehen: fuer das Los
//     zaehlt der Stand zur Bestellung, nicht zum Losen (s_weg_b.py, Angriff
//     "Abmelden nach bekanntem Zufall").
//   - quanten_w_s_praep_abmelden OHNE Wallet: raeumt einen Eintrag auf, dessen
//     Konto kein Mensch mehr ist (Umzug auf eine neue Wallet, umzug.go). Die
//     alte Wallet kann der Mensch nach einem Umzug nicht mehr fuer sich
//     halten; wer ihren Schluessel hat, darf damit kein Praeparator bleiben.
//
// GRENZEN: hoechstens quantenSPraepHoechstens Eintraege (auch abgemeldete).
// Abgemeldete werden quantenSPraepNachlauf nach der Abmeldung beim naechsten
// Melden vergessen; danach darf das Konto sich neu melden. Nachlauf =
// quantenWToteBehalten: so kann keine alte Melde-Unterschrift (hoechstens
// quantenWAlt alt, Buchzeit hoechstens quantenWBuchSpiel zurueck) nach dem
// Vergessen wieder eingespielt werden -- dieselbe Rechnung wie bei den Toten.
//
// ABBILD (quantenSPraepAbbildLocked, fuer B2b): wer zur Zeit T im Los ist --
// gemeldet spaetestens T - quantenSPraepVorlauf, zu T nicht abgemeldet und zu
// T ein Mensch. B2b rechnet das Abbild IN der Bestellung (T = ihre Buchzeit)
// und haelt es dort fest; spaetere Meldungen, Abmeldungen oder Umzuege
// verschieben das Los dieser Bestellung nicht. Darum muss eine Bestellung
// verfallen, bevor ein Eintrag aus ihrem Abbild vergessen werden kann
// (Frist der Bestellung < quantenSPraepNachlauf).
//
// SCHLAFEND bis quantenSWegBAbUnix (MaxInt64 = nie). Vor dem Stichtag ist jede der
// beiden Arten im Block unbekannt (bekannteTxArt), ihr Bescheid ein Befund und
// der Endpunkt 409; das Register bleibt leer und die StateRoot byte-gleich
// (das Feld praep fehlt in der Kodierung). Ein Stichtag kommt erst mit B2b,
// mit Deploy-Waechter wie bei den anderen Stichtagen.
//
// SCHLUESSEL (B4b-4a): Ausgeber und Inhaberin verschluesseln den Auftrag an
// eine geloste Praeparatorin (die Basen y sind geheim; ECIES wie die Buendel,
// quanten_s_anteil_oeffnung.go). Dazu steht im Eintrag ihr oeffentlicher
// secp256k1-Schluessel (pub, 65 Byte unkomprimiert, Hex). Jeder Knoten stellt
// ihn selbst aus der EIP-191-Unterschrift der Meldung wieder her -- niemand
// reicht ihn ein, keiner glaubt einem anderen. Beim Lesen des Registers muss
// er ein Punkt der Kurve sein und genau die Adresse des Kontos ergeben
// (fail-closed). Melden mit ML-DSA wird abgewiesen: diese Unterschrift
// liefert keinen secp256k1-Schluessel, die Praeparatorin waere im Los, aber
// nicht erreichbar.
//
// EHRLICH: Den Vorlauf (1 Tag) kann der einzige Blockerzeuger (C1) heute um
// bis zu quantenWBuchSpiel verkuerzen (Buchzeit zurueckdatiert). Das schadet
// dem Los nicht: der Zufall des Loses kommt aus einer drand-Runde NACH der
// Bestellung (drand_zufall.go); wer sich frueh oder spaet meldet, kennt ihn
// nicht. Der Vorlauf ist eine zusaetzliche Huerde, keine tragende.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

const (
	quantenSPraepMeldenTxArt   = "quanten_w_s_praep_melden"
	quantenSPraepAbmeldenTxArt = "quanten_w_s_praep_abmelden"

	quantenSDomPraepK        = "aequitas-quanten-s-praep-k-v1"
	quantenSDomPraepMelden   = "aequitas-quanten-s-praep-melden-v1"
	quantenSDomPraepAbmelden = "aequitas-quanten-s-praep-abmelden-v1"

	// Zeile in quantenWTabelle (quanten_w_tabelle.go).
	quantenWArtPraep = "praep"

	// Eintraege im Register, auch abgemeldete (je Eintrag ~150 Byte; das
	// Abbild einer Bestellung in B2b ist hoechstens so lang).
	quantenSPraepHoechstens = 256
	// So lange vor der Bestellung muss ein Praeparator gemeldet sein.
	quantenSPraepVorlauf int64 = 86400
	// So lange nach der Abmeldung bleibt ein Eintrag stehen (siehe oben).
	quantenSPraepNachlauf = quantenWToteBehalten
)

// STICHTAG VON WEG B (Kettenregel). 9223372036854775807 = math.MaxInt64 =
// schlafend. Gesetzt vom Betreiber am 25.11.2026: 26.11.2026 09:00 UTC
// (10:00 MEZ), nach B2b (Bestellung, Los, Ausgabe gegen das Los) und der
// App 1.31.0. Nie rueckwirkend: der Deploy-Waechter
// .github/neustart/quanten-s-weg-b-nicht-verpasst.sh haelt jeden Wechsel an,
// der nicht mindestens 30 Minuten in der Zukunft liegt oder einen laufenden
// Stichtag aendern wuerde.
var quantenSWegBAbUnix int64 = 1795683600

// quantenSWegBAbOverride: nur fuer Tests (0 = Stichtag oben).
var quantenSWegBAbOverride atomic.Int64

func quantenSWegBAb() int64 {
	if v := quantenSWegBAbOverride.Load(); v != 0 {
		return v
	}
	return quantenSWegBAbUnix
}

// quantenSWegBAktiv: Weg B setzt Variante W voraus (dasselbe Register).
func quantenSWegBAktiv(t int64) bool { return quantenWAktiv(t) && t >= quantenSWegBAb() }

func istQuantenSPraepTxArt(t string) bool {
	return t == quantenSPraepMeldenTxArt || t == quantenSPraepAbmeldenTxArt
}

// quantenSPraepK: der Schluessel eines Eintrags (K der Transaktion, Zeile der
// Tabelle) -- fest aus dem Konto, damit ein Konto nur einen Eintrag hat.
func quantenSPraepK(konto string) string {
	h := sha256.Sum256([]byte(quantenSDomPraepK + "|" + strings.ToLower(konto)))
	return hex.EncodeToString(h[:])
}

// quantenSPraepNachricht: was das Konto beim Melden bzw. Abmelden
// unterschreibt (EIP-191 oder ML-DSA, quantenWUnterschrift). Die Zeit
// begrenzt das Wiedereinspielen (quantenWZeitGueltig).
func quantenSPraepNachricht(art, wallet, K string, zeit int64) string {
	dom := quantenSDomPraepMelden
	if art == quantenSPraepAbmeldenTxArt {
		dom = quantenSDomPraepAbmelden
	}
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d", dom, eingefroreneChainID, strings.ToLower(wallet), K, zeit)
}

// quantenSPraep: ein Eintrag (Abgemeldet 0 = gemeldet).
type quantenSPraep struct {
	Konto      string
	Pub        string // secp256k1 unkomprimiert, 130 Hex-Zeichen (siehe SCHLUESSEL)
	Gemeldet   int64
	Abgemeldet int64
	// P2 (quanten_s_pfaender.go): Pfand im Pfandtopf (0 = keins) und der
	// Nullifier des Menschen beim Melden ("" = keiner).
	Pfand int64
	N     string
}

// quantenSPraepForm: ein Eintrag in kanonischer Form (Register, Zeile).
type quantenSPraepForm struct {
	K          string `json:"k"`
	Konto      string `json:"konto"`
	Pub        string `json:"pub"`
	Gemeldet   int64  `json:"gemeldet"`
	Abgemeldet int64  `json:"abgemeldet,omitempty"`
	// P2: fehlen ohne Pfand -- vor dem Stichtag der Pfaender byte-gleich.
	Pfand int64  `json:"pfand,omitempty"`
	N     string `json:"n,omitempty"`
}

// quantenSPraepLesen: Eintraege aus dem Register, fail-closed (was so nie
// entstehen konnte, wird abgewiesen). Sortiert nach K, ohne Doppelte.
func quantenSPraepLesen(fs []quantenSPraepForm) (map[string]quantenSPraep, error) {
	if len(fs) > quantenSPraepHoechstens {
		return nil, fmt.Errorf("quanten_w: %d Praeparatoren, hoechstens %d", len(fs), quantenSPraepHoechstens)
	}
	out := make(map[string]quantenSPraep, len(fs))
	for i, f := range fs {
		if !istHash64(f.K) || (i > 0 && f.K <= fs[i-1].K) || !kanonischeAdresse(f.Konto) || istSystemAdresse(f.Konto) ||
			f.K != quantenSPraepK(f.Konto) || !quantenSPraepPubPasst(f.Pub, f.Konto) || f.Gemeldet <= 0 || f.Gemeldet == math.MaxInt64 ||
			f.Abgemeldet < 0 || (f.Abgemeldet != 0 && f.Abgemeldet < f.Gemeldet) || !quantenSPraepPfandGueltig(f.Pfand, f.N) {
			return nil, fmt.Errorf("quanten_w: Praeparator %d: Form", i)
		}
		out[f.K] = quantenSPraep{Konto: f.Konto, Pub: f.Pub, Gemeldet: f.Gemeldet, Abgemeldet: f.Abgemeldet, Pfand: f.Pfand, N: f.N}
	}
	if err := quantenSPraepEinPfandJeMensch(out); err != nil {
		return nil, err
	}
	return out, nil
}

// quantenSPraepFormen: die Eintraege kanonisch, nach K sortiert.
func quantenSPraepFormen(m map[string]quantenSPraep) []quantenSPraepForm {
	var out []quantenSPraepForm
	for k, e := range m {
		out = append(out, quantenSPraepForm{K: k, Konto: e.Konto, Pub: e.Pub, Gemeldet: e.Gemeldet, Abgemeldet: e.Abgemeldet, Pfand: e.Pfand, N: e.N})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].K < out[j].K })
	return out
}

// quantenSPraepPubPasst: ist pub ein secp256k1-Schluessel in kanonischer Form
// (04 || X || Y, Kleinbuchstaben-Hex, Punkt der Kurve), der genau die Adresse
// konto ergibt?
func quantenSPraepPubPasst(pub, konto string) bool {
	if len(pub) != 130 || !kleinHex(pub) || !strings.HasPrefix(pub, "04") {
		return false
	}
	roh, err := hex.DecodeString(pub)
	if err != nil {
		return false
	}
	p, err := crypto.UnmarshalPubkey(roh)
	if err != nil {
		return false
	}
	return strings.ToLower(crypto.PubkeyToAddress(*p).Hex()) == konto
}

// quantenSPraepPubAusSig: der Schluessel hinter einer EIP-191-Unterschrift
// (dieselbe Rechnung wie verifyPersonalSign), kanonisch als Hex. Nur nach
// quantenWUnterschrift aufrufen; prueft trotzdem selbst, dass er zum Konto
// passt.
func quantenSPraepPubAusSig(nachricht, sig, konto string) (string, error) {
	roh, err := hex.DecodeString(strings.TrimPrefix(sig, "0x"))
	if err != nil || len(roh) != 65 {
		return "", fmt.Errorf("Unterschrift nicht lesbar")
	}
	if roh[64] >= 27 {
		roh[64] -= 27
	}
	h := crypto.Keccak256([]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(nachricht), nachricht)))
	p, err := crypto.SigToPub(h, roh)
	if err != nil {
		return "", fmt.Errorf("Schluessel nicht wiederherstellbar")
	}
	pub := hex.EncodeToString(crypto.FromECDSAPub(p))
	if !quantenSPraepPubPasst(pub, konto) {
		return "", fmt.Errorf("Schluessel passt nicht zum Konto")
	}
	return pub, nil
}

// quantenSPraepAufraeumen: Abgemeldete nach dem Nachlauf vergessen -- nie
// einen Eintrag, dessen Pfand noch liegt (P2: es bleibt bis zur Rueckgabe).
func quantenSPraepAufraeumen(r *quantenWRegister, zeit int64) {
	for k, e := range r.Praep {
		if e.Pfand == 0 && e.Abgemeldet != 0 && e.Abgemeldet <= zeit-quantenSPraepNachlauf {
			delete(r.Praep, k)
		}
	}
}

// quantenSPraepNurEigeneFelder: Melden und Abmelden tragen nur K und die
// Unterschrift (q: K und Index schon geleert); Abmelden ohne Wallet auch die
// nicht und keine Zeit.
func quantenSPraepNurEigeneFelder(tx *Transaction, q QuantenWAngaben) bool {
	if tx.QuantenW.Index != 0 {
		return false
	}
	if tx.Type == quantenSPraepAbmeldenTxArt && tx.Wallet == "" {
		return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.DistributionAt == 0
	}
	q.WalletSig, q.PQ = "", nil
	if tx.Type == quantenSPraepMeldenTxArt {
		q.Pfand = 0 // gerechnet (P2, quanten_s_pfaender.go)
	}
	return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.Wallet != ""
}

// quantenSPraepIstMenschLocked: ist das Konto JETZT ein Mensch?
func (cs *ChainState) quantenSPraepIstMenschLocked(ctx context.Context, konto string) bool {
	cs.ensureAccountLoadedCtx(ctx, konto)
	acc, ok := cs.accounts.Get(konto)
	return ok && acc.IsHuman
}

// quantenSPraepBescheidLocked: Melden und Abmelden. zeit = Buchzeit (die Zeit,
// mit der Annahme und Nachspielen rechnen). Caller haelt cs.mu.
func (cs *ChainState) quantenSPraepBescheidLocked(ctx context.Context, tx *Transaction, wallet string, reg *quantenWRegister, zeit int64) (quantenWBescheid, error) {
	if !quantenSWegBAktiv(zeit) {
		return quantenWBefund("quanten_s_praep_inaktiv", "Weg B (Praeparatoren) ist noch nicht aktiv"), nil
	}
	q := tx.QuantenW
	// Die Wallet steht genau so im Satz, wie sie gelesen wird: sonst sagte
	// die Formpruefung "mit Wallet" und der Bescheid "ohne" (etwa bei " ").
	if tx.Wallet != wallet {
		return quantenWBefund("quanten_w_form", "Wallet nicht kanonisch"), nil
	}
	neu := reg.kopie()
	switch tx.Type {
	case quantenSPraepMeldenTxArt:
		if !kanonischeAdresse(wallet) || istSystemAdresse(wallet) || q.K != quantenSPraepK(wallet) {
			return quantenWBefund("quanten_w_form", "Melden: Wallet nicht kanonisch oder K passt nicht zum Konto"), nil
		}
		// Nur EIP-191: ML-DSA liefert keinen Schluessel fuer ECIES (SCHLUESSEL).
		if q.PQ != nil {
			return quantenWBefund("quanten_s_praep_schluessel", "Melden nur mit der secp256k1-Unterschrift der Wallet (Schluessel fuer die Auftraege)"), nil
		}
		nachricht := quantenSPraepNachricht(tx.Type, wallet, q.K, tx.DistributionAt)
		if err := quantenWUnterschrift(q, nachricht, wallet, zeit); err != nil {
			return quantenWBefund("quanten_w_unterschrift", "%s: %v", kurzAdresse(wallet), err), nil
		}
		pub, err := quantenSPraepPubAusSig(nachricht, q.WalletSig, wallet)
		if err != nil {
			return quantenWBefund("quanten_s_praep_schluessel", "%s: %v", kurzAdresse(wallet), err), nil
		}
		if !cs.quantenSPraepIstMenschLocked(ctx, wallet) {
			return quantenWBefund("quanten_s_praep_mensch", "%s ist kein registrierter Mensch", kurzAdresse(wallet)), nil
		}
		quantenSPraepAufraeumen(neu, zeit)
		e, da := neu.Praep[q.K]
		// P2: wer vor dem Stichtag ohne Pfand gemeldet ist, legt es mit einem
		// neuen Melden nach (Meldezeit und Schluessel bleiben).
		nachlegen := da && e.Abgemeldet == 0 && e.Pfand == 0 && quantenSPfaenderAktiv(zeit)
		if da && !nachlegen {
			if e.Abgemeldet != 0 {
				return quantenWBefund("quanten_s_praep_doppelt", "%s ist abgemeldet; neu melden ab %d", kurzAdresse(wallet), sattAdd(e.Abgemeldet, quantenSPraepNachlauf)), nil
			}
			return quantenWBefund("quanten_s_praep_doppelt", "%s ist schon gemeldet", kurzAdresse(wallet)), nil
		}
		if !da && len(neu.Praep) >= quantenSPraepHoechstens {
			return quantenWBefund("quanten_s_praep_voll", "Register der Praeparatoren voll (%d)", len(neu.Praep)), nil
		}
		if !da {
			e = quantenSPraep{Konto: wallet, Pub: pub, Gemeldet: zeit}
		}
		if !quantenSPfaenderAktiv(zeit) {
			neu.Praep[q.K] = e
			break
		}
		e, weg, befund, err := cs.quantenSPfandMeldenLocked(ctx, reg, q.K, wallet, e)
		if err != nil || befund.regel != "" {
			return befund, err
		}
		neu.Praep[q.K] = e
		return quantenWBescheid{reg: neu, wege: []quantenWWeg{weg}, felder: QuantenWAngaben{Pfand: weg.mikro}}, nil

	case quantenSPraepAbmeldenTxArt:
		e, da := reg.Praep[q.K]
		if !da || e.Abgemeldet != 0 {
			return quantenWBefund("quanten_s_praep_unbekannt", "Praeparator %s ist nicht gemeldet", q.K[:12]), nil
		}
		if wallet != "" {
			if wallet != e.Konto {
				return quantenWBefund("quanten_s_praep_fremd", "%s ist nicht das Konto dieses Praeparators", kurzAdresse(wallet)), nil
			}
			if err := quantenWUnterschrift(q, quantenSPraepNachricht(tx.Type, wallet, q.K, tx.DistributionAt), wallet, zeit); err != nil {
				return quantenWBefund("quanten_w_unterschrift", "%s: %v", kurzAdresse(wallet), err), nil
			}
		} else if cs.quantenSPraepIstMenschLocked(ctx, e.Konto) {
			// Ohne Unterschrift nur aufraeumen, was kein Mensch mehr ist.
			return quantenWBefund("quanten_s_praep_mensch", "%s ist ein Mensch: nur das Konto selbst meldet ab", kurzAdresse(e.Konto)), nil
		}
		e.Abgemeldet = zeit
		neu.Praep[q.K] = e
	}
	return quantenWBescheid{reg: neu}, nil
}

// quantenSPraepAbbildLocked: wer zur Zeit T im Los ist (s_weg_b.py
// Register.abbild, mit Zeit statt Hoehe): gemeldet spaetestens T - Vorlauf,
// zu T nicht abgemeldet, zu T ein Mensch. Sortiert. Nur zur Zeit T selbst
// aufrufen (die Menschen-Pruefung liest den Stand JETZT) -- B2b tut das in
// der Bestellung und haelt das Ergebnis fest.
func (cs *ChainState) quantenSPraepAbbildLocked(ctx context.Context, reg *quantenWRegister, T int64) []string {
	var out []string
	for _, e := range reg.Praep {
		if sattAdd(e.Gemeldet, quantenSPraepVorlauf) > T || (e.Abgemeldet != 0 && e.Abgemeldet <= T) {
			continue
		}
		// P2: ab dem Stichtag der Pfaender nur mit Pfand.
		if e.Pfand == 0 && quantenSPfaenderAktiv(T) {
			continue
		}
		if cs.quantenSPraepIstMenschLocked(ctx, e.Konto) {
			out = append(out, e.Konto)
		}
	}
	sort.Strings(out)
	return out
}

// handleQuantenSPraeparatoren: GET /api/quanten-w/praeparatoren -- das
// Register fuer jeden (Konto, Schluessel fuer die Auftraege, Melde- und
// Abmeldezeit, ab wann im Los). Nur
// Lesen, hoechstens quantenSPraepHoechstens Eintraege.
func (a *APIServer) handleQuantenSPraeparatoren(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		jsonError(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	reg := a.state.quantenWJetzt() // ein Stand fuer Register und Bestellungen
	liste := []map[string]interface{}{}
	for _, f := range quantenSPraepFormen(reg.Praep) {
		e := map[string]interface{}{"konto": f.Konto, "pub": f.Pub, "gemeldet": f.Gemeldet, "im_los_ab": sattAdd(f.Gemeldet, quantenSPraepVorlauf)}
		if f.Abgemeldet != 0 {
			e["abgemeldet"] = f.Abgemeldet
		}
		if f.Pfand != 0 {
			e["pfand"] = f.Pfand
			if f.Abgemeldet != 0 {
				e["pfand_zurueck_ab"] = sattAdd(f.Abgemeldet, quantenSPraepHaltezeit)
			}
		}
		liste = append(liste, e)
	}
	bestellungen := []map[string]interface{}{}
	for _, f := range quantenSBestellungenFormen(reg.Best) {
		bestellungen = append(bestellungen, quantenSBestellungAnsicht(f.K, reg.Best[f.K]))
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"aktiv": quantenSWegBAktiv(time.Now().Unix()), "vorlauf": quantenSPraepVorlauf,
		"nachlauf": quantenSPraepNachlauf, "hoechstens": quantenSPraepHoechstens, "praeparatoren": liste,
		"bestellungen": bestellungen, "los_abstand": quantenSLosAbstand, "bestell_frist": quantenSBestellFrist})
}
