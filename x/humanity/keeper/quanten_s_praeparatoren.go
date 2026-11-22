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
// SCHLAFEND: quantenSWegBAbUnix = MaxInt64. Vor dem Stichtag ist jede der
// beiden Arten im Block unbekannt (bekannteTxArt), ihr Bescheid ein Befund und
// der Endpunkt 409; das Register bleibt leer und die StateRoot byte-gleich
// (das Feld praep fehlt in der Kodierung). Ein Stichtag kommt erst mit B2b,
// mit Deploy-Waechter wie bei den anderen Stichtagen.
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
// schlafend. Gesetzt wird er erst mit B2b (Bestellung, Los, Ausgabe gegen das
// Los), nie rueckwirkend.
var quantenSWegBAbUnix int64 = 9223372036854775807

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
	Gemeldet   int64
	Abgemeldet int64
}

// quantenSPraepForm: ein Eintrag in kanonischer Form (Register, Zeile).
type quantenSPraepForm struct {
	K          string `json:"k"`
	Konto      string `json:"konto"`
	Gemeldet   int64  `json:"gemeldet"`
	Abgemeldet int64  `json:"abgemeldet,omitempty"`
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
			f.K != quantenSPraepK(f.Konto) || f.Gemeldet <= 0 || f.Gemeldet == math.MaxInt64 ||
			f.Abgemeldet < 0 || (f.Abgemeldet != 0 && f.Abgemeldet < f.Gemeldet) {
			return nil, fmt.Errorf("quanten_w: Praeparator %d: Form", i)
		}
		out[f.K] = quantenSPraep{Konto: f.Konto, Gemeldet: f.Gemeldet, Abgemeldet: f.Abgemeldet}
	}
	return out, nil
}

// quantenSPraepFormen: die Eintraege kanonisch, nach K sortiert.
func quantenSPraepFormen(m map[string]quantenSPraep) []quantenSPraepForm {
	var out []quantenSPraepForm
	for k, e := range m {
		out = append(out, quantenSPraepForm{K: k, Konto: e.Konto, Gemeldet: e.Gemeldet, Abgemeldet: e.Abgemeldet})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].K < out[j].K })
	return out
}

// quantenSPraepAufraeumen: Abgemeldete nach dem Nachlauf vergessen.
func quantenSPraepAufraeumen(r *quantenWRegister, zeit int64) {
	for k, e := range r.Praep {
		if e.Abgemeldet != 0 && e.Abgemeldet <= zeit-quantenSPraepNachlauf {
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
		if err := quantenWUnterschrift(q, quantenSPraepNachricht(tx.Type, wallet, q.K, tx.DistributionAt), wallet, zeit); err != nil {
			return quantenWBefund("quanten_w_unterschrift", "%s: %v", kurzAdresse(wallet), err), nil
		}
		if !cs.quantenSPraepIstMenschLocked(ctx, wallet) {
			return quantenWBefund("quanten_s_praep_mensch", "%s ist kein registrierter Mensch", kurzAdresse(wallet)), nil
		}
		quantenSPraepAufraeumen(neu, zeit)
		if e, da := neu.Praep[q.K]; da {
			if e.Abgemeldet != 0 {
				return quantenWBefund("quanten_s_praep_doppelt", "%s ist abgemeldet; neu melden ab %d", kurzAdresse(wallet), sattAdd(e.Abgemeldet, quantenSPraepNachlauf)), nil
			}
			return quantenWBefund("quanten_s_praep_doppelt", "%s ist schon gemeldet", kurzAdresse(wallet)), nil
		}
		if len(neu.Praep) >= quantenSPraepHoechstens {
			return quantenWBefund("quanten_s_praep_voll", "Register der Praeparatoren voll (%d)", len(neu.Praep)), nil
		}
		neu.Praep[q.K] = quantenSPraep{Konto: wallet, Gemeldet: zeit}

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
		if cs.quantenSPraepIstMenschLocked(ctx, e.Konto) {
			out = append(out, e.Konto)
		}
	}
	sort.Strings(out)
	return out
}

// handleQuantenSPraeparatoren: GET /api/quanten-w/praeparatoren -- das
// Register fuer jeden (Konto, Melde- und Abmeldezeit, ab wann im Los). Nur
// Lesen, hoechstens quantenSPraepHoechstens Eintraege.
func (a *APIServer) handleQuantenSPraeparatoren(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		jsonError(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	liste := []map[string]interface{}{}
	for _, f := range quantenSPraepFormen(a.state.quantenWJetzt().Praep) {
		e := map[string]interface{}{"konto": f.Konto, "gemeldet": f.Gemeldet, "im_los_ab": sattAdd(f.Gemeldet, quantenSPraepVorlauf)}
		if f.Abgemeldet != 0 {
			e["abgemeldet"] = f.Abgemeldet
		}
		liste = append(liste, e)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"aktiv": quantenSWegBAktiv(time.Now().Unix()), "vorlauf": quantenSPraepVorlauf,
		"nachlauf": quantenSPraepNachlauf, "hoechstens": quantenSPraepHoechstens, "praeparatoren": liste})
}
