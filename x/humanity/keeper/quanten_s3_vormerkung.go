package keeper

// WEG C, Schritt C2b-2b-2 (forschung/quantengeld/WEG_C.md): Vormerken, Melden,
// Ziehen und Frist eines Tokens ueber eine Quantenleitung, im Knoten.
// Spezifikation: s_kette.KetteS3 (vormerken, melden, ziehen, frist).
//
//   - quanten_w_s3_vormerken (Wallet = Ausgeber, ein Mensch, unterschrieben):
//     K, Inhaberin, eine eingetragene Leitung (C2a), Zahl der Pulse N, Wurzel
//     der Pruefsummen ueber alle N Pulse, Deckung. Die Deckung geht in den
//     Pfandtopf. Die Vormerkung haelt theta, frei und gamma der Leitung FEST:
//     was das Register spaeter eintraegt, aendert sie nicht. Je Ausgeber eine
//     offene Vormerkung, hoechstens quantenS3VormerkungenHoechstens zugleich.
//   - quanten_w_s3_melden (Wallet = Inhaberin, unterschrieben, genau einmal):
//     die gemessenen Pulse in der einen Kodierung (quanten_s3_meldung.go),
//     hoechstens quantenS3MeldungTxHexMax Zeichen, mindestens gamma*N. Die
//     Kette behaelt nur SHA-256 der Meldung und die Zahl der Pulse.
//   - quanten_w_s3_ziehen (ohne Wallet, jeder, genau einmal): die Meldung noch
//     einmal (Hash wie gemeldet) und die drand-Runde, die FEST ist: die erste
//     ab Meldezeit + quantenS3ZiehAbstand. Jeder Knoten prueft die
//     BLS-Signatur selbst (drand_zufall.go) und zieht die Teilmenge
//     (quantenS3Teilmenge, hoechstens quantenWNMax Pulse).
//   - quanten_w_s3_frist (ohne Wallet, jeder): nach quantenS3Frist geht die
//     Deckung an den Ausgeber zurueck; erst dann ist der Platz frei. Melden und
//     Ziehen sind nach der Frist abgewiesen.
//
// Die Ausgabe des Tokens ueber die gezogene Teilmenge (mit der Pruefgroesse
// der Leitung, quantenSKLeitung) folgt mit C2b-2b-3. Bis dahin weist jede
// andere Ausgabe ein vorgemerktes K ab.
//
// ZEIT: Melden zaehlt mit der Buchzeit, die hoechstens quantenSBestellBuchSpiel
// vor dem Block liegt (quantenWBuchSpielFuer). Der Abstand zur Runde
// (quantenS3ZiehAbstand) ist groesser als dieses Spiel plus 60 s Vorlauf:
// bei der Annahme der Meldung liegt die Runde immer noch in der Zukunft.
// EHRLICH wie in Weg B (quanten_s_bestellung.go): haelt ein einziger
// Blockerzeuger die Blockzeit hinter der echten Zeit zurueck, laege die Runde
// schon in der Vergangenheit; das ist von aussen sichtbar (Blockzeit gegen
// drand-Zeit).
//
// SCHLAFEND bis quantenS3AbUnix (MaxInt64) und nur zusammen mit dem Register
// der Leitungen (quantenLeitungAktiv). Vor dem Stichtag ist jede der vier
// Arten im Block unbekannt, ihr Bescheid ein Befund und der Endpunkt 409; das
// Register bleibt leer und die StateRoot byte-gleich (das Feld s3 fehlt).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
)

const (
	quantenS3VormerkenTxArt = "quanten_w_s3_vormerken"
	quantenS3MeldenTxArt    = "quanten_w_s3_melden"
	quantenS3ZiehenTxArt    = "quanten_w_s3_ziehen"
	quantenS3FristTxArt     = "quanten_w_s3_frist"

	quantenS3DomVormerken = "aequitas-quanten-s3-vormerken-v1"
	quantenS3DomMelden    = "aequitas-quanten-s3-melden-v1"

	// Zeile in quantenWTabelle.
	quantenWArtS3 = "s3"

	// Wie s_kette.S3_FRIST, S3_VORMERKUNGEN_MAX, S3_ZIEH_ABSTAND,
	// S3_MELDUNG_TX_HEX_MAX.
	quantenS3Frist                  int64 = 3600
	quantenS3VormerkungenHoechstens       = quantenWTokenHoechstens
	quantenS3ZiehAbstand            int64 = 600
	// Eine Meldung in einer Transaktion: unter quantenWKoerperMax (160 KiB)
	// mit Platz fuer die uebrigen Felder. quantenS3MeldungMax Pulse mit
	// Luecke unter 128 passen (2 Zeichen je Puls).
	quantenS3MeldungTxHexMax = 128 << 10
)

// quantenS3AbUnix: Stichtag (Kettenregel). 9223372036854775807 = math.MaxInt64
// = schlafend. Ein Datum bekommt er erst mit der Ausgabe (C2b-2b-3), mit
// Deploy-Waechter wie die anderen Stichtage.
var quantenS3AbUnix int64 = 9223372036854775807

// quantenS3AbOverride: nur fuer Tests (0 = Stichtag oben).
var quantenS3AbOverride atomic.Int64

func quantenS3Ab() int64 {
	if v := quantenS3AbOverride.Load(); v != 0 {
		return v
	}
	return quantenS3AbUnix
}

// quantenS3Aktiv: setzt Variante W (dasselbe Register) und das Register der
// Leitungen voraus.
func quantenS3Aktiv(t int64) bool {
	return quantenWAktiv(t) && quantenLeitungAktiv(t) && t >= quantenS3Ab()
}

func istQuantenS3TxArt(t string) bool {
	return t == quantenS3VormerkenTxArt || t == quantenS3MeldenTxArt || t == quantenS3ZiehenTxArt || t == quantenS3FristTxArt
}

// quantenS3Vormerkung: eine offene Vormerkung. Gemeldet == 0: noch nicht
// gemeldet; Runde == 0: noch nicht gezogen.
type quantenS3Vormerkung struct {
	Von, Inhaber, Leitung string
	Theta, Frei, Gamma    int // die Werte der Leitung zur Vormerkung
	NPuls                 int
	Wurzel                [32]byte
	Deckung               int64 // im Pfandtopf bis Ausgabe oder Frist
	Zeit                  int64
	Meldung               [32]byte // SHA-256 des Meldungstexts
	Gemeldet              int
	MeldungZeit           int64
	Runde                 uint64
	Zufall                [32]byte
	Teil                  []int // aufsteigend; nie geaendert, nur ersetzt
}

type quantenS3VormerkungForm struct {
	K           string `json:"k"`
	Von         string `json:"von"`
	Inhaber     string `json:"inhaber"`
	Leitung     string `json:"leitung"`
	Theta       int    `json:"theta"`
	Frei        int    `json:"frei"`
	Gamma       int    `json:"gamma"`
	NPuls       int    `json:"n_puls"`
	Wurzel      string `json:"wurzel"`
	Deckung     int64  `json:"deckung"`
	Zeit        int64  `json:"zeit"`
	Meldung     string `json:"meldung,omitempty"`
	Gemeldet    int    `json:"gemeldet,omitempty"`
	MeldungZeit int64  `json:"meldung_zeit,omitempty"`
	Runde       uint64 `json:"runde,omitempty"`
	Zufall      string `json:"zufall,omitempty"`
	Teil        string `json:"teil,omitempty"` // quantenS3Kodieren
}

func (v *quantenS3Vormerkung) form(K string) quantenS3VormerkungForm {
	f := quantenS3VormerkungForm{K: K, Von: v.Von, Inhaber: v.Inhaber, Leitung: v.Leitung, Theta: v.Theta, Frei: v.Frei, Gamma: v.Gamma,
		NPuls: v.NPuls, Wurzel: hex.EncodeToString(v.Wurzel[:]), Deckung: v.Deckung, Zeit: v.Zeit}
	if v.Gemeldet != 0 {
		f.Meldung, f.Gemeldet, f.MeldungZeit = hex.EncodeToString(v.Meldung[:]), v.Gemeldet, v.MeldungZeit
	}
	if v.Runde != 0 {
		f.Runde, f.Zufall = v.Runde, hex.EncodeToString(v.Zufall[:])
		f.Teil, _ = quantenS3Kodieren(v.Teil) // gelesen oder gezogen: kodierbar
	}
	return f
}

// quantenS3ZiehRunde: die Runde, die zu einer Meldung zieht.
func quantenS3ZiehRunde(meldungZeit int64) (uint64, bool) {
	return drandQuicknet.rundeAb(sattAdd(meldungZeit, quantenS3ZiehAbstand))
}

// quantenS3Verfallen wie s_kette.KetteS3._verfallen.
func quantenS3Verfallen(v *quantenS3Vormerkung, zeit int64) bool {
	return zeit > sattAdd(v.Zeit, quantenS3Frist)
}

func quantenS3VormerkenNachricht(wallet, K, inhaber, leitung string, nPuls int, wurzel string, deckung, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%s|%s|%d|%s|%d|%d", quantenS3DomVormerken, eingefroreneChainID, strings.ToLower(wallet), K,
		strings.ToLower(inhaber), leitung, nPuls, wurzel, deckung, zeit)
}

// quantenS3MeldenNachricht: die Inhaberin unterschreibt den Hash der Meldung.
func quantenS3MeldenNachricht(wallet, K, meldungHash string) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%s", quantenS3DomMelden, eingefroreneChainID, strings.ToLower(wallet), K, meldungHash)
}

func quantenS3MeldungHash(text string) [32]byte { return sha256.Sum256([]byte(text)) }

// quantenS3VormerkungenLesen: fail-closed. Die Teilmenge selbst rechnet nur,
// wer zieht (die Meldung steht nicht im Register); geprueft wird ihre Form.
func quantenS3VormerkungenLesen(fs []quantenS3VormerkungForm, r *quantenWRegister) (map[string]*quantenS3Vormerkung, error) {
	if len(fs) > quantenS3VormerkungenHoechstens {
		return nil, fmt.Errorf("quanten_w: %d Vormerkungen, hoechstens %d", len(fs), quantenS3VormerkungenHoechstens)
	}
	out := make(map[string]*quantenS3Vormerkung, len(fs))
	von := map[string]bool{}
	for i, f := range fs {
		falsch := fmt.Errorf("quanten_w: Vormerkung %d: Form", i)
		wurzel, okW := quantenWHex32(f.Wurzel)
		if !istHash64(f.K) || (i > 0 && f.K <= fs[i-1].K) || !kanonischeAdresse(f.Von) || von[f.Von] || !kanonischeAdresse(f.Inhaber) ||
			f.Inhaber == f.Von || !istLeitungsName(f.Leitung) || !quantenSatzGueltig(f.Leitung) || !quantenLeitungWerteGueltig(f.Theta, f.Frei, f.Gamma) ||
			f.NPuls < 1 || f.NPuls > quantenS3PulsMax || !okW || f.Deckung < quantenMindestDeckungMik || f.Zeit <= 0 {
			return nil, falsch
		}
		if r.Noten[f.K] != nil || r.Token[f.K] != nil || r.Best[f.K] != nil {
			return nil, fmt.Errorf("quanten_w: Vormerkung %s ist schon ausgegeben oder bestellt", f.K[:12])
		}
		if _, tot := r.Tote[f.K]; tot {
			return nil, fmt.Errorf("quanten_w: Vormerkung %s ist schon tot", f.K[:12])
		}
		if _, da := r.IPfand[f.K]; da {
			return nil, fmt.Errorf("quanten_w: zu Vormerkung %s liegt ein Pfand", f.K[:12])
		}
		von[f.Von] = true
		v := &quantenS3Vormerkung{Von: f.Von, Inhaber: f.Inhaber, Leitung: f.Leitung, Theta: f.Theta, Frei: f.Frei, Gamma: f.Gamma,
			NPuls: f.NPuls, Wurzel: wurzel, Deckung: f.Deckung, Zeit: f.Zeit}
		if f.Gemeldet == 0 {
			if f.Meldung != "" || f.MeldungZeit != 0 || f.Runde != 0 || f.Zufall != "" || f.Teil != "" {
				return nil, falsch
			}
			out[f.K] = v
			continue
		}
		m, okM := quantenWHex32(f.Meldung)
		if !okM || f.Gemeldet < 1 || f.Gemeldet > quantenS3MeldungMax || f.Gemeldet > f.NPuls ||
			int64(f.Gemeldet)*1_000_000 < int64(f.Gamma)*int64(f.NPuls) || f.MeldungZeit <= f.Zeit {
			return nil, falsch
		}
		v.Meldung, v.Gemeldet, v.MeldungZeit = m, f.Gemeldet, f.MeldungZeit
		if f.Runde == 0 {
			if f.Zufall != "" || f.Teil != "" {
				return nil, falsch
			}
			out[f.K] = v
			continue
		}
		z, okZ := quantenWHex32(f.Zufall)
		runde, okR := quantenS3ZiehRunde(f.MeldungZeit)
		if !okZ || !okR || f.Runde != runde {
			return nil, falsch
		}
		teil, err := quantenS3Dekodieren(f.Teil)
		soll := min(f.Gemeldet, quantenWNMax)
		if err != nil || len(teil) != soll || teil[len(teil)-1] >= f.NPuls {
			return nil, falsch
		}
		v.Runde, v.Zufall, v.Teil = f.Runde, z, teil
		out[f.K] = v
	}
	return out, nil
}

func quantenS3VormerkungenFormen(m map[string]*quantenS3Vormerkung) []quantenS3VormerkungForm {
	var out []quantenS3VormerkungForm
	for k, v := range m {
		out = append(out, v.form(k))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].K < out[j].K })
	return out
}

// quantenS3NurEigeneFelder: je Art nur ihre Angaben.
func quantenS3NurEigeneFelder(tx *Transaction, q QuantenWAngaben) bool {
	if tx.QuantenW.Index != 0 {
		return false
	}
	switch tx.Type {
	case quantenS3VormerkenTxArt:
		q.Inhaber, q.Leitung, q.NPuls, q.Wurzel, q.Deckung, q.WalletSig, q.PQ, q.Pfand = "", "", 0, "", 0, "", nil, 0
		return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.Wallet != ""
	case quantenS3MeldenTxArt:
		q.Meldung, q.WalletSig, q.PQ = "", "", nil
		return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.Wallet != "" && tx.DistributionAt == 0
	case quantenS3ZiehenTxArt:
		q.Meldung, q.Runde, q.RundeSig = "", 0, ""
		return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.Wallet == "" && tx.DistributionAt == 0
	case quantenS3FristTxArt:
		q.Pfand, q.PfandAn = 0, "" // gerechnet
		return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.Wallet == "" && tx.DistributionAt == 0
	}
	return false
}

// quantenS3KFrei: K ist in keinem Teil des Registers vergeben.
func quantenS3KFrei(reg *quantenWRegister, K string) bool {
	if reg.Noten[K] != nil || reg.Token[K] != nil || reg.Best[K] != nil || reg.S3[K] != nil {
		return false
	}
	if _, tot := reg.Tote[K]; tot {
		return false
	}
	_, pfand := reg.IPfand[K]
	return !pfand
}

// quantenS3BescheidLocked: die vier Arten. zeit = Buchzeit. Caller haelt cs.mu.
func (cs *ChainState) quantenS3BescheidLocked(ctx context.Context, tx *Transaction, wallet string, reg *quantenWRegister, zeit int64) (quantenWBescheid, error) {
	if !quantenS3Aktiv(zeit) {
		return quantenWBefund("quanten_s3_inaktiv", "Ausgabe ueber eine Leitung ist noch nicht aktiv"), nil
	}
	if tx.Wallet != wallet {
		return quantenWBefund("quanten_w_form", "Wallet nicht kanonisch"), nil
	}
	q := tx.QuantenW
	if tx.Type == quantenS3VormerkenTxArt {
		return cs.quantenS3VormerkenLocked(ctx, tx, wallet, reg, zeit)
	}
	v := reg.S3[q.K]
	if v == nil {
		return quantenWBefund("quanten_w_unbekannt", "keine Vormerkung %s", q.K[:12]), nil
	}
	neu := reg.kopie()
	switch tx.Type {
	case quantenS3FristTxArt:
		if !quantenS3Verfallen(v, zeit) {
			return quantenWBefund("quanten_w_frist", "Vormerkung %s gilt bis %d", q.K[:12], sattAdd(v.Zeit, quantenS3Frist)), nil
		}
		delete(neu.S3, q.K)
		return quantenWBescheid{reg: neu, wege: []quantenWWeg{{quantenWPfandAddr, v.Von, v.Deckung}}, felder: QuantenWAngaben{Pfand: v.Deckung, PfandAn: v.Von}}, nil

	case quantenS3MeldenTxArt:
		if wallet != v.Inhaber {
			return quantenWBefund("quanten_s3_inhaberin", "nur die Inhaberin meldet (%s)", kurzAdresse(v.Inhaber)), nil
		}
		if quantenS3Verfallen(v, zeit) {
			return quantenWBefund("quanten_w_frist", "Vormerkung %s ist verfallen", q.K[:12]), nil
		}
		if v.Gemeldet != 0 {
			return quantenWBefund("quanten_s3_doppelt", "%s ist schon gemeldet", q.K[:12]), nil
		}
		if zeit <= v.Zeit {
			return quantenWBefund("quanten_w_zeit", "Meldung nicht nach der Vormerkung"), nil
		}
		if len(q.Meldung) > quantenS3MeldungTxHexMax {
			return quantenWBefund("quanten_w_form", "Meldung %d Zeichen, hoechstens %d", len(q.Meldung), quantenS3MeldungTxHexMax), nil
		}
		h := quantenS3MeldungHash(q.Meldung)
		if err := quantenWUnterschrift(q, quantenS3MeldenNachricht(wallet, q.K, hex.EncodeToString(h[:])), wallet, zeit); err != nil {
			return quantenWBefund("quanten_w_unterschrift", "Inhaberin %s: %v", kurzAdresse(wallet), err), nil
		}
		lam, err := quantenS3Dekodieren(q.Meldung)
		if err == nil {
			err = quantenS3MeldungPruefen(v.NPuls, v.Gamma, lam)
		}
		if err != nil {
			return quantenWBefund("quanten_s3_meldung", "%v", err), nil
		}
		nv := *v
		nv.Meldung, nv.Gemeldet, nv.MeldungZeit = h, len(lam), zeit
		neu.S3[q.K] = &nv

	case quantenS3ZiehenTxArt:
		if v.Gemeldet == 0 {
			return quantenWBefund("quanten_s3_nicht_gemeldet", "%s ist noch nicht gemeldet", q.K[:12]), nil
		}
		if quantenS3Verfallen(v, zeit) {
			return quantenWBefund("quanten_w_frist", "Vormerkung %s ist verfallen", q.K[:12]), nil
		}
		if v.Runde != 0 {
			return quantenWBefund("quanten_s3_doppelt", "%s ist schon gezogen", q.K[:12]), nil
		}
		if len(q.Meldung) > quantenS3MeldungTxHexMax || quantenS3MeldungHash(q.Meldung) != v.Meldung {
			return quantenWBefund("quanten_s3_meldung", "nicht die gemeldete Meldung"), nil
		}
		r, ok := quantenS3ZiehRunde(v.MeldungZeit)
		zr, okZ := drandQuicknet.zeitDerRunde(r)
		if !ok || !okZ || q.Runde != r {
			return quantenWBefund("quanten_s_los_runde", "es zieht Runde %d, nicht %d", r, q.Runde), nil
		}
		zufall, err := drandQuicknet.pruefen(q.Runde, q.RundeSig)
		if err != nil {
			return quantenWBefund("quanten_s_los_drand", "%v", err), nil
		}
		lam, err := quantenS3Dekodieren(q.Meldung)
		if err != nil {
			return quantenWBefund("quanten_s3_meldung", "%v", err), nil // nie: beim Melden gelesen
		}
		teil, err := quantenS3Teilmenge(q.K, lam, zufall[:], v.MeldungZeit, zr, quantenWNMax)
		if err != nil {
			return quantenWBefund("quanten_s3_meldung", "%v", err), nil
		}
		nv := *v
		nv.Runde, nv.Zufall, nv.Teil = q.Runde, zufall, teil
		neu.S3[q.K] = &nv
	}
	return quantenWBescheid{reg: neu}, nil
}

func (cs *ChainState) quantenS3VormerkenLocked(ctx context.Context, tx *Transaction, wallet string, reg *quantenWRegister, zeit int64) (quantenWBescheid, error) {
	q := tx.QuantenW
	wurzel, okW := quantenWHex32(q.Wurzel)
	if !kanonischeAdresse(wallet) || istSystemAdresse(wallet) || !kanonischeAdresse(q.Inhaber) || istSystemAdresse(q.Inhaber) || q.Inhaber == wallet ||
		q.NPuls < 1 || q.NPuls > quantenS3PulsMax || !okW || q.Deckung < quantenMindestDeckungMik {
		return quantenWBefund("quanten_w_form", "Vormerken: Ausgeber, Inhaberin, N (%d), Wurzel oder Deckung (%d)", q.NPuls, q.Deckung), nil
	}
	if !istLeitungsName(q.Leitung) || !quantenSatzGueltig(q.Leitung) {
		return quantenWBefund("quanten_w_form", "Vormerken: keine Leitung"), nil
	}
	theta, frei, gamma, ok := cs.quantenGeldJetzt().leitungZu(q.Leitung, zeit)
	if !ok {
		return quantenWBefund("quanten_s3_leitung", "Leitung %q ist nicht eingetragen", q.Leitung), nil
	}
	if err := quantenWUnterschrift(q, quantenS3VormerkenNachricht(wallet, q.K, q.Inhaber, q.Leitung, q.NPuls, q.Wurzel, q.Deckung, tx.DistributionAt), wallet, zeit); err != nil {
		return quantenWBefund("quanten_w_unterschrift", "Ausgeber %s: %v", kurzAdresse(wallet), err), nil
	}
	if !cs.quantenSPraepIstMenschLocked(ctx, wallet) {
		return quantenWBefund("quanten_s_praep_mensch", "Ausgeber %s ist kein registrierter Mensch", kurzAdresse(wallet)), nil
	}
	if !quantenS3KFrei(reg, q.K) {
		return quantenWBefund("quanten_w_doppelt", "%s gibt es schon", q.K[:12]), nil
	}
	for _, v := range reg.S3 {
		if v.Von == wallet {
			return quantenWBefund("quanten_s3_offen", "%s hat schon eine offene Vormerkung", kurzAdresse(wallet)), nil
		}
	}
	if len(reg.S3) >= quantenS3VormerkungenHoechstens {
		return quantenWBefund("quanten_w_voll", "zu viele offene Vormerkungen (%d)", len(reg.S3)), nil
	}
	if cs.quantenWMikroLocked(ctx, wallet) < q.Deckung {
		return quantenWBefund("quanten_w_deckung", "%s hat keine %d Mikro-AEQ", kurzAdresse(wallet), q.Deckung), nil
	}
	neu := reg.kopie()
	neu.S3[q.K] = &quantenS3Vormerkung{Von: wallet, Inhaber: q.Inhaber, Leitung: q.Leitung, Theta: theta, Frei: frei, Gamma: gamma,
		NPuls: q.NPuls, Wurzel: wurzel, Deckung: q.Deckung, Zeit: zeit}
	return quantenWBescheid{reg: neu, wege: []quantenWWeg{{wallet, quantenWPfandAddr, q.Deckung}}, felder: QuantenWAngaben{Pfand: q.Deckung}}, nil
}

// quantenS3Ansicht: fuer GET /api/quanten-w/note (ohne Teilmenge, nur ihre
// Groesse; die Runde braucht, wer zieht).
func quantenS3Ansicht(K string, v *quantenS3Vormerkung) map[string]interface{} {
	m := map[string]interface{}{"k": K, "art": "s3", "von": v.Von, "inhaber": v.Inhaber, "leitung": v.Leitung, "theta": v.Theta, "frei": v.Frei,
		"gamma": v.Gamma, "n_puls": v.NPuls, "wurzel": hex.EncodeToString(v.Wurzel[:]), "deckung": v.Deckung, "zeit": v.Zeit,
		"frist": sattAdd(v.Zeit, quantenS3Frist), "gemeldet": v.Gemeldet}
	if v.Gemeldet != 0 {
		r, _ := quantenS3ZiehRunde(v.MeldungZeit)
		m["meldung"], m["meldung_zeit"], m["runde"] = hex.EncodeToString(v.Meldung[:]), v.MeldungZeit, r
	}
	if v.Runde != 0 {
		m["gezogen"] = len(v.Teil)
	}
	return m
}

// quantenS3Status: fuer GET /api/status (fehlt = schlafend).
func quantenS3Status(m map[string]interface{}) {
	if ab := quantenS3Ab(); ab != 9223372036854775807 {
		m["s3_ab"] = ab
	}
}
