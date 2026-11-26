package keeper

// NACHWEIS EINER FAELSCHUNG (P4, Plan Pfaender in forschung/quantengeld/DEZENTRAL.md;
// Spezifikation forschung/quantengeld/s_pfaender.py, Regel R-P5)
//
// Ab dem Stichtag der Pfaender (quantenSPfaenderAbUnix, wie P2 und P3):
//
//   - Wird ein Token, zu dem ein Pfand der Inhaberin liegt (P3), an Stelle A
//     eingeloest, legt die Kette einen Nachweis-Eintrag an: die Form des
//     Tokens beim Ende (Wurzel, Proben, Bindung, Wahl, Stellen; ohne
//     Anmeldung, Fehlversuche 0), welche Stelle eingeloest hat und die Zeit
//     des Endes. Er bleibt, bis das Pfand der Inhaberin zurueck ist.
//   - quanten_w_s_faelschung_anmelden: nur die ANDERE Stelle B, von ihr
//     unterschrieben, mit einer Vorlage, die die Inhaberin fuer B
//     unterschrieben hat, und B's Stichprobe (passt zur Bindung der Ausgabe).
//     Nur bis Ende + quantenSNachweisFrist.
//   - quanten_w_s_faelschung_offenlegen: in Teilen, dieselbe Pruefung wie
//     beim Einloesen (quantenSToken.offenlegenTeil gegen die Wurzel).
//   - Besteht die Vorlage B's Stichprobe, obwohl A schon eingeloest hat, hat
//     die Inhaberin zwei Vorlagen unterschrieben, die beide bestehen. Folge:
//     NUR das Pfand der Inhaberin an B (quantenSFaelschungStrafe). Dass
//     Geloste ihr Wissen geteilt haben, beweist das NICHT: die Oeffnungen
//     gehen an beide Stellen, eine Stelle B der Inhaberin kennt also die
//     Werte ihrer Stichprobe (TestQuantenSFaelschung_EigeneStelleB). Ein
//     Nachweis je K. Besteht sie nicht: Fehlversuch; nach quantenWFehlMax
//     ist der Eintrag weg.
//
// ZEIT: die Frist ist ein "spaetestens". Beide Arten haben nur ein kurzes
// Buchspiel (quantenSFaelschungBuchSpiel); eine zurueckdatierte Buchzeit
// verlaengert sie also hoechstens um zwei Minuten.
//
// BINDUNG: eine offene, nicht verfallene Anmeldung haelt das Pfand der
// Inhaberin fest. Das Los bleibt dafuer stehen, auch wenn der tote Eintrag
// aufgeraeumt wird; ein Pfand der Gelosten (P2, eigener Stichtag) bleibt
// gebunden, verfaellt aber nie.
//
// GRENZE: ein Eintrag nur zusammen mit einem Pfand der Inhaberin
// (quantenSIPfandHoechstens). Gespeichert als Form (Bits als Hex).

import (
	"context"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

const (
	quantenSFaelschungAnmeldenTxArt   = "quanten_w_s_faelschung_anmelden"
	quantenSFaelschungOffenlegenTxArt = "quanten_w_s_faelschung_offenlegen"

	quantenSDomFaelschungAnmelden = "aequitas-quanten-s-faelschung-anmelden-v1"

	// Zeile in quantenWTabelle (quanten_w_tabelle.go).
	quantenWArtNachweis = "nachweis"

	// So weit darf die Buchzeit eines Nachweises vor seinem Block liegen.
	quantenSFaelschungBuchSpiel int64 = 120

	quantenSUrteilFaelschung = "faelschung"
)

// quantenSNachweis: der Stoff fuer einen Nachweis zu K.
type quantenSNachweis struct {
	Token quantenWSTokenForm // Form beim Ende; Anmeldung = die des Nachweises
	An    int                // die Stelle, die eingeloest hat
	Ende  int64
}

type quantenSNachweisForm struct {
	K     string             `json:"k"`
	An    int                `json:"an"`
	Ende  int64              `json:"ende"`
	Token quantenWSTokenForm `json:"token"`
}

func istQuantenSFaelschungTxArt(t string) bool {
	return t == quantenSFaelschungAnmeldenTxArt || t == quantenSFaelschungOffenlegenTxArt
}

// quantenSFaelschungNurEigeneFelder: Anmelden wie das Anmelden einer Stelle
// (B, X, Probe, Vorlage, Salze, Unterschrift; mit Wallet, ohne Zeit),
// Offenlegen wie das Offenlegen (Ab, Offen; ohne Wallet und Zeit).
func quantenSFaelschungNurEigeneFelder(tx *Transaction, q QuantenWAngaben) bool {
	if tx.QuantenW.Index != 0 || tx.DistributionAt != 0 {
		return false
	}
	switch tx.Type {
	case quantenSFaelschungAnmeldenTxArt:
		q.B, q.X, q.Probe, q.Vorlage, q.WalletSig, q.PQ, q.Salze = "", "", nil, "", "", nil, nil
		return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.Wallet != ""
	case quantenSFaelschungOffenlegenTxArt:
		q.Ab, q.Offen, q.Pfand, q.PfandAn, q.Urteil = 0, nil, 0, "", ""
		return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.Wallet == ""
	}
	return false
}

func quantenSFaelschungAnmeldenNachricht(wallet, K, b, x string, probe []int, fehl int) string {
	teile := make([]string, len(probe))
	for i, v := range probe {
		teile[i] = strconv.Itoa(v)
	}
	return fmt.Sprintf("%s|chain:%d|%s|%s|%s|%s|%s|%d", quantenSDomFaelschungAnmelden, eingefroreneChainID, strings.ToLower(wallet), K, b,
		quantenWSHash(x), quantenWSHash(strings.Join(teile, ",")), fehl)
}

// quantenSNachweisOffen: laeuft eine Anmeldung des Nachweises, die noch
// nicht verfallen ist?
func quantenSNachweisOffen(n *quantenSNachweis, zeit int64) bool {
	a := n.Token.Anmeldung
	return a != nil && zeit <= sattAdd(a.Zeit, quantenWFristOffen)
}

// quantenSNachweisAnlegen: beim Einloesen an Stelle an (ab dem Stichtag,
// nur mit Pfand der Inhaberin). e: der Token beim Ende.
func quantenSNachweisAnlegen(r *quantenWRegister, K string, e *quantenWSEintrag, an int, ende int64) {
	if _, ok := r.IPfand[K]; !ok {
		return
	}
	f := e.form(K)
	f.Fehl, f.Anmeldung = 0, nil
	r.Nachweis[K] = &quantenSNachweis{Token: f, An: an, Ende: ende}
}

// quantenSNachweisWeg: der Nachweis zu K ist erledigt; ein Los, das nur noch
// fuer ihn stand, geht mit.
func quantenSNachweisWeg(r *quantenWRegister, K string) {
	delete(r.Nachweis, K)
	if _, tot := r.Tote[K]; !tot && r.Token[K] == nil {
		delete(r.Lose, K)
	}
}

func quantenSNachweiseFormen(m map[string]*quantenSNachweis) []quantenSNachweisForm {
	if len(m) == 0 {
		return nil
	}
	out := make([]quantenSNachweisForm, 0, len(m))
	for k, n := range m {
		out = append(out, quantenSNachweisForm{K: k, An: n.An, Ende: n.Ende, Token: n.Token})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].K < out[j].K })
	return out
}

// quantenSNachweiseLesen: fail-closed. r traegt schon Token, Tote und IPfand.
func quantenSNachweiseLesen(fs []quantenSNachweisForm, r *quantenWRegister) (map[string]*quantenSNachweis, error) {
	if len(fs) > quantenSIPfandHoechstens {
		return nil, fmt.Errorf("quanten_w: %d Nachweise, hoechstens %d", len(fs), quantenSIPfandHoechstens)
	}
	out := make(map[string]*quantenSNachweis, len(fs))
	for i, f := range fs {
		if !istHash64(f.K) || (i > 0 && f.K <= fs[i-1].K) || f.Token.K != f.K || (f.An != 0 && f.An != 1) || f.Ende <= 0 {
			return nil, fmt.Errorf("quanten_w: Nachweis %d: Form", i)
		}
		if p, ok := r.IPfand[f.K]; !ok || p.Ende != f.Ende || r.Token[f.K] != nil {
			return nil, fmt.Errorf("quanten_w: Nachweis %s: kein beendetes Pfand der Inhaberin dazu", f.K[:12])
		}
		e, err := quantenWSTokenLesen(f.Token)
		if err != nil {
			return nil, fmt.Errorf("quanten_w: Nachweis %s: %w", f.K[:12], err)
		}
		t := e.token
		if t.D == nil || t.C == -1 || t.Fehl >= quantenWFehlMax {
			return nil, fmt.Errorf("quanten_w: Nachweis %s: Token", f.K[:12])
		}
		if a := t.Anmeldung; a != nil && (a.B != 1-f.An || a.Stelle != e.Stellen[1-f.An]) {
			return nil, fmt.Errorf("quanten_w: Nachweis %s: Anmeldung nicht von der anderen Stelle", f.K[:12])
		}
		out[f.K] = &quantenSNachweis{Token: f.Token, An: f.An, Ende: f.Ende}
	}
	return out, nil
}

// quantenSFaelschungBescheidLocked: Anmelden und Offenlegen eines Nachweises.
// zeit = Buchzeit (hoechstens quantenSFaelschungBuchSpiel vor dem Block).
// Caller haelt cs.mu.
func (cs *ChainState) quantenSFaelschungBescheidLocked(ctx context.Context, tx *Transaction, wallet string, reg *quantenWRegister, zeit int64) (quantenWBescheid, error) {
	if !quantenSPfaenderAktiv(zeit) {
		return quantenWBefund("quanten_s_pfand_inaktiv", "Pfaender sind noch nicht aktiv"), nil
	}
	if tx.Wallet != wallet {
		return quantenWBefund("quanten_w_form", "Wallet nicht kanonisch"), nil
	}
	q := tx.QuantenW
	n := reg.Nachweis[q.K]
	if n == nil {
		return quantenWBefund("quanten_s_faelschung_unbekannt", "kein Nachweis zu %s moeglich", q.K[:12]), nil
	}
	e, err := quantenWSTokenLesen(n.Token)
	if err != nil {
		return quantenWBescheid{}, err // beim Lesen geprueft: Zustand kaputt
	}
	t := e.token
	B := 1 - n.An
	neu := reg.kopie()
	switch tx.Type {
	case quantenSFaelschungAnmeldenTxArt:
		if zeit > sattAdd(n.Ende, quantenSNachweisFrist) {
			return quantenWBefund("quanten_s_faelschung_frist", "Nachweis zu %s nur bis %d", q.K[:12], sattAdd(n.Ende, quantenSNachweisFrist)), nil
		}
		b, ok := quantenWSBit(q.B)
		if !ok || !kanonischeAdresse(wallet) {
			return quantenWBefund("quanten_w_form", "Nachweis: Stelle 0 oder 1, Wallet kanonisch"), nil
		}
		if b != B || wallet != e.Stellen[b] {
			return quantenWBefund("quanten_w_s_stelle", "nur die andere Stelle (%d) dieses Tokens fuehrt den Nachweis", B), nil
		}
		x, ok := quantenWSBitsLesen(q.X, t.N)
		if !ok || len(q.Probe) == 0 || len(q.Probe) > t.N {
			return quantenWBefund("quanten_w_form", "Nachweis: Vorlage (%d Bits) oder Stichprobe", t.N), nil
		}
		if err := quantenWUnterschrift(q, quantenSFaelschungAnmeldenNachricht(wallet, q.K, q.B, q.X, q.Probe, t.Fehl), wallet, zeit); err != nil {
			return quantenWBefund("quanten_w_unterschrift", "Stelle %s: %v", kurzAdresse(wallet), err), nil
		}
		if !kanonischeSignatur(q.Vorlage) || pruefePersonalSignGemerkt(quantenSVorlageText(q.K, b, x), q.Vorlage, t.Inhaber) != nil {
			return quantenWBefund("quanten_w_s_vorlage", "Vorlage nicht von der Inhaberin unterschrieben"), nil
		}
		var salze [][32]byte
		if t.ProbeArt == 2 {
			if salze, ok = quantenWSSalzeLesen(q.Salze, t.NVal); !ok {
				return quantenWBefund("quanten_w_form", "Nachweis: je Abschnitt ein Salz (%d)", t.NVal), nil
			}
		} else if len(q.Salze) != 0 {
			return quantenWBefund("quanten_w_form", "Nachweis: Salze nur bei Stichprobe v2"), nil
		}
		if err := t.anmelden(q.K, b, wallet, x, q.Probe, salze, zeit); err != nil {
			return quantenWBefund("quanten_w_regel", "%v", err), nil
		}
		neu.Nachweis[q.K] = &quantenSNachweis{Token: e.form(q.K), An: n.An, Ende: n.Ende}
		return quantenWBescheid{reg: neu}, nil

	case quantenSFaelschungOffenlegenTxArt:
		if t.Anmeldung == nil || q.Ab < 0 || len(q.Offen) == 0 || len(q.Offen) > quantenWOffenTeil {
			return quantenWBefund("quanten_w_form", "Nachweis offenlegen: Anmeldung, Beginn oder Zahl der Stellen"), nil
		}
		werte, ok := quantenSWerteLesen(q.Offen)
		if !ok {
			return quantenWBefund("quanten_w_form", "Nachweis offenlegen: Werte oder Pfade nicht in gueltiger Form"), nil
		}
		fertig, bestanden, err := t.nachweisOffenlegenTeil(q.K, q.Ab, werte, zeit)
		if err != nil {
			return quantenWBefund("quanten_w_regel", "%v", err), nil
		}
		u := quantenWBescheid{reg: neu}
		switch {
		case !fertig:
			neu.Nachweis[q.K] = &quantenSNachweis{Token: e.form(q.K), An: n.An, Ende: n.Ende}
			u.felder.Urteil = "teil"
		case bestanden:
			quantenSFaelschungStrafe(&u, q.K, e.Stellen[B])
		default:
			if t.Tot {
				quantenSNachweisWeg(neu, q.K)
			} else {
				neu.Nachweis[q.K] = &quantenSNachweis{Token: e.form(q.K), An: n.An, Ende: n.Ende}
			}
			u.felder.Urteil = "abgewiesen"
		}
		return u, nil
	}
	return quantenWBefund("quanten_w_form", "keine Art des Nachweises"), nil
}

// P4b: der Nachweis legt die WERTE w offen, nicht die Anteile. Bei einer
// Faelschung haben mindestens t Geloste ihr Wissen geteilt -- halten sie ihre
// Anteile zurueck, blieben oft weniger als t Ehrliche, und der Nachweis waere
// genau im Betrugsfall nicht fuehrbar. Stelle B hat die Werte ihrer
// Stichprobe aus den Oeffnungen beim Annehmen. Geprueft wird dieselbe
// Pruefsumme gegen die Wurzel wie beim Einloesen: ein falsches w scheitert
// dort (die Anteile waren nur der Weg zu w, nicht der Schutz).

type quantenSWertStelle struct {
	W    *big.Int
	Pfad [][32]byte
}

// quantenSWerteLesen: je Stelle genau ein Wert (hex, hoechstens 32 Zeichen),
// keine Anteile, hoechstens quantenWPfadMax Pfadglieder.
func quantenSWerteLesen(in []QuantenWOffen) ([]quantenSWertStelle, bool) {
	var out []quantenSWertStelle
	for _, o := range in {
		if len(o.Anteile) != 0 || o.W == "" || len(o.W) > 32 || !kleinHex(o.W) || len(o.Pfad) > quantenWPfadMax {
			return nil, false
		}
		w, ok := new(big.Int).SetString(o.W, 16)
		if !ok {
			return nil, false
		}
		s := quantenSWertStelle{W: w}
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

// quantenSWerteAuswerten: wie quantenWAuswerten, mit dem Wert statt der Anteile.
func quantenSWerteAuswerten(K string, n int, wurzel [32]byte, stellen, basen, antwort []int, werte []quantenSWertStelle) (treffer, fehler int, err error) {
	if len(werte) != len(stellen) || len(antwort) != len(stellen) || len(basen) != len(stellen) {
		return 0, 0, quantenWFehler("Zahl der Stellen")
	}
	grenze := new(big.Int).Lsh(big.NewInt(1), quantenWSalzBits+2)
	for j, i := range stellen {
		w := werte[j].W
		if w.Sign() < 0 || w.Cmp(grenze) >= 0 {
			return 0, 0, quantenWFehler("Stelle %d: Wert zu gross", i)
		}
		if !quantenWMerklePruefen(wurzel, quantenWPruefsumme(K, i, w), i, werte[j].Pfad, n) {
			return 0, 0, quantenWFehler("Stelle %d: Pruefsumme oder Pfad passt nicht", i)
		}
		basis, bit := quantenWZerlegen(w)
		if basis == basen[j] {
			treffer++
			if antwort[j] != bit {
				fehler++
			}
		}
	}
	return treffer, fehler, nil
}

// nachweisOffenlegenTeil: wie offenlegenTeil (quanten_s.go), mit Werten.
func (e *quantenSToken) nachweisOffenlegenTeil(K string, ab int, werte []quantenSWertStelle, zeit int64) (fertig, bestanden bool, err error) {
	e.frist(zeit)
	a := e.Anmeldung
	if a == nil {
		return false, false, quantenWFehler("Nachweis offenlegen: keine Anmeldung")
	}
	if ab != a.Offen || len(werte) < 1 || len(werte) > quantenWOffenTeil || ab+len(werte) > len(a.Probe) {
		return false, false, quantenWFehler("Nachweis offenlegen: Teil (ab %d, faellig %d, %d Stellen)", ab, a.Offen, len(werte))
	}
	bis := ab + len(werte)
	stellen := a.Probe[ab:bis]
	basen := make([]int, len(stellen))
	antwort := make([]int, len(stellen))
	for j, i := range stellen {
		basen[j] = e.D[i] ^ a.B ^ e.C
		antwort[j] = a.X[i]
	}
	tr, fe, err := quantenSWerteAuswerten(K, e.N, e.Wurzel, stellen, basen, antwort, werte)
	if err != nil {
		return false, false, err
	}
	a.Offen, a.Treffer, a.Fehler = bis, a.Treffer+tr, a.Fehler+fe
	if a.Offen < len(a.Probe) {
		return false, false, nil
	}
	e.Anmeldung = nil
	if quantenWUrteil(a.Treffer, a.Fehler, e.Tol) != quantenWEcht {
		e.Fehl++
		if e.Fehl >= quantenWFehlMax {
			e.Tot = true
		}
		return true, false, nil
	}
	return true, true, nil
}

// quantenSFaelschungStrafe: der Nachweis besteht -- das Pfand der Inhaberin
// (Deckung + 10 %) an die Stelle an. NUR dieses (Befund 26.11.2026,
// Entscheidung des Betreibers): Die Oeffnungen gehen an beide Stellen; eine
// Stelle B, die mit der Inhaberin zusammenarbeitet, kennt also die Werte
// ihrer Stichprobe und kann einen "Nachweis" bauen. Ein Verfall der Gelosten
// liesse so ehrliche Praeparatorinnen abschoepfen. Das Pfand der Inhaberin
// dagegen bewegt bei einer Absprache nur Geld zwischen den Beteiligten und
// ersetzt bei echtem Betrug den Schaden D (mit Aufschlag).
func quantenSFaelschungStrafe(u *quantenWBescheid, K, an string) {
	r := u.reg
	summe := r.IPfand[K].Betrag
	u.wege = append(u.wege, quantenWWeg{quantenWPfandAddr, an, summe})
	delete(r.IPfand, K)
	quantenSNachweisWeg(r, K)
	u.felder.Pfand, u.felder.PfandAn, u.felder.Urteil = summe, an, quantenSUrteilFaelschung
}

// quantenSNachweisAnsicht: in der Ansicht des beendeten Tokens.
func quantenSNachweisAnsicht(reg *quantenWRegister, k string, v map[string]interface{}) {
	if n, ok := reg.Nachweis[k]; ok {
		v["nachweis_bis"] = sattAdd(n.Ende, quantenSNachweisFrist)
		v["nachweis_stelle"] = 1 - n.An
		if a := n.Token.Anmeldung; a != nil {
			v["nachweis_angemeldet"] = a.Zeit
		}
	}
}
