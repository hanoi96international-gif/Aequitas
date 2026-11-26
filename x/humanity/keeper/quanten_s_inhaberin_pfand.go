package keeper

// PFAND DER INHABERIN (P3, Plan Pfaender in forschung/quantengeld/DEZENTRAL.md;
// Spezifikation forschung/quantengeld/s_pfaender.py, Regel R-P4)
//
// Ab dem Stichtag der Pfaender (quantenSPfaenderAbUnix, wie P2):
//
//   - Die Bindung eines S-Tokens (quanten_w_s_binden) nimmt die Deckung D
//     als Pfand von der Inhaberin in den Pfandtopf (Entscheidung des
//     Betreibers: Pfand = D, genau der Schaden der zweiten Stelle, die in
//     gutem Glauben angenommen hat). Ohne Deckung: keine Bindung. Der Eintrag
//     merkt sich das Konto und den Nullifier des Menschen (wie P2).
//   - Endet der Token (eingeloest oder tot), haelt der Eintrag die Zeit des
//     Endes fest. Er bleibt, bis das Pfand zurueck ist -- auch nachdem der
//     tote Eintrag aufgeraeumt ist.
//   - Zurueck (neue Art quanten_w_s_ipfand_zurueck, ausloesen darf jeder,
//     ohne Unterschrift): erst NACH Ende + quantenSNachweisFrist, an die
//     Wallet, der der Nullifier dann gehoert (ohne Nullifier an das Konto).
//     Ein Nachweis (P4) nimmt das Pfand vorher an die andere Stelle.
//
// GRENZE: hoechstens quantenSIPfandHoechstens Eintraege (lebende plus tote
// Token); ist das Register voll, wird keine Bindung angenommen (fail-closed).
// Jeder Eintrag kostet D (mindestens quantenMindestDeckungMik) und einen
// ausgegebenen Token, und jeder darf ein faelliges Pfand zurueckgeben.
//
// ZEIT: "frueestens nach der Frist" -- eine zurueckdatierte Buchzeit macht
// die Rueckgabe nur spaeter moeglich.

import (
	"context"
	"fmt"
	"reflect"
	"sort"
)

const (
	quantenSIPfandZurueckTxArt = "quanten_w_s_ipfand_zurueck"

	// Zeile in quantenWTabelle (quanten_w_tabelle.go).
	quantenWArtIPfand = "ipfand"

	// So viele Pfaender der Inhaberinnen hoechstens: so viele, wie Token
	// leben und tot gemerkt sein koennen (quantenSLoseHoechstens). Wer die
	// Grenze fuellen will, fuellt damit auch die Toten -- und das haelt die
	// Ausgabe ohnehin an (quantenWToteAufraeumen). Faellige Pfaender kann
	// jeder zurueckgeben, bevor der tote Eintrag aufgeraeumt wird. Eine
	// kleine Grenze (frueher 256) liesse sich mit 256 Token zu je 1 AEQ in
	// 7 Tagen fuellen und sperrte jede Bindung (Sicherheitspruefung #672).
	quantenSIPfandHoechstens = quantenSLoseHoechstens
)

// quantenSIPfand: das Pfand einer Inhaberin zu einem Token K.
type quantenSIPfand struct {
	Betrag int64
	Konto  string // die Inhaberin bei der Bindung
	N      string // ihr Nullifier ("" = keiner)
	Ende   int64  // 0 = Token laeuft
}

type quantenSIPfandForm struct {
	K      string `json:"k"`
	Betrag int64  `json:"betrag"`
	Konto  string `json:"konto"`
	N      string `json:"n,omitempty"`
	Ende   int64  `json:"ende,omitempty"`
}

func istQuantenSIPfandTxArt(t string) bool { return t == quantenSIPfandZurueckTxArt }

// quantenSIPfandNurEigeneFelder: die Rueckgabe traegt nur K (des Tokens).
func quantenSIPfandNurEigeneFelder(tx *Transaction, q QuantenWAngaben) bool {
	q.Pfand, q.PfandAn = 0, ""
	return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.QuantenW.Index == 0 && tx.Wallet == "" && tx.DistributionAt == 0
}

func quantenSIPfandFormen(m map[string]quantenSIPfand) []quantenSIPfandForm {
	if len(m) == 0 {
		return nil
	}
	out := make([]quantenSIPfandForm, 0, len(m))
	for k, p := range m {
		out = append(out, quantenSIPfandForm{K: k, Betrag: p.Betrag, Konto: p.Konto, N: p.N, Ende: p.Ende})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].K < out[j].K })
	return out
}

// quantenSIPfandLesen: fail-closed. r traegt schon Token und Tote. Ein
// laufendes Pfand (Ende 0) gehoert zu einem gebundenen Token derselben
// Inhaberin mit Betrag = Deckung; ein beendetes zu keinem lebenden Token.
func quantenSIPfandLesen(fs []quantenSIPfandForm, r *quantenWRegister) (map[string]quantenSIPfand, error) {
	if len(fs) > quantenSIPfandHoechstens {
		return nil, fmt.Errorf("quanten_w: %d Pfaender der Inhaberinnen, hoechstens %d", len(fs), quantenSIPfandHoechstens)
	}
	out := make(map[string]quantenSIPfand, len(fs))
	for i, f := range fs {
		if !istHash64(f.K) || (i > 0 && f.K <= fs[i-1].K) || f.Betrag <= 0 || !kanonischeAdresse(f.Konto) || istSystemAdresse(f.Konto) ||
			(f.N != "" && !umzugNullifierKanonisch(f.N)) || f.Ende < 0 {
			return nil, fmt.Errorf("quanten_w: Pfand der Inhaberin %d: Form", i)
		}
		e := r.Token[f.K]
		if f.Ende == 0 {
			if e == nil || e.token.D == nil || e.token.Inhaber != f.Konto || e.token.Deckung != f.Betrag {
				return nil, fmt.Errorf("quanten_w: Pfand der Inhaberin %s: kein gebundener Token dazu", f.K[:12])
			}
		} else if e != nil {
			return nil, fmt.Errorf("quanten_w: Pfand der Inhaberin %s: beendet, aber Token lebt", f.K[:12])
		}
		out[f.K] = quantenSIPfand{Betrag: f.Betrag, Konto: f.Konto, N: f.N, Ende: f.Ende}
	}
	return out, nil
}

// quantenSIPfandBindenLocked: R-P4 bei der Bindung (ab dem Stichtag). u traegt
// schon das neue Register; das Pfand kommt dazu -- oder ein Befund.
func (cs *ChainState) quantenSIPfandBindenLocked(ctx context.Context, u *quantenWBescheid, K, wallet string, deckung int64) (quantenWBescheid, error) {
	if len(u.reg.IPfand) >= quantenSIPfandHoechstens {
		return quantenWBefund("quanten_w_voll", "zu viele Pfaender von Inhaberinnen (%d)", len(u.reg.IPfand)), nil
	}
	n, mehrere, err := cs.quantenSNullifierVonLocked(ctx, wallet)
	if err != nil {
		return quantenWBescheid{}, err
	}
	if mehrere || (n != "" && !umzugNullifierKanonisch(n)) {
		return quantenWBefund("quanten_s_pfand_mensch", "%s: Nullifier nicht eindeutig", kurzAdresse(wallet)), nil
	}
	if cs.quantenWMikroLocked(ctx, wallet) < deckung {
		return quantenWBefund("quanten_w_deckung", "%s hat keine %d Mikro-AEQ fuer das Pfand der Bindung (= Deckung)", kurzAdresse(wallet), deckung), nil
	}
	u.reg.IPfand[K] = quantenSIPfand{Betrag: deckung, Konto: wallet, N: n}
	u.wege = append(u.wege, quantenWWeg{wallet, quantenWPfandAddr, deckung})
	u.felder.Pfand = deckung
	return *u, nil
}

// quantenSIPfandEnde: der Token K endet (Register r, schon die Kopie).
func quantenSIPfandEnde(r *quantenWRegister, K string, zeit int64) {
	if p, ok := r.IPfand[K]; ok {
		p.Ende = zeit
		r.IPfand[K] = p
	}
}

// quantenSEmpfaengerLocked: an wen ein Pfand zurueckgeht -- die Wallet des
// Nullifiers, ohne Nullifier das Konto. "" = niemand (Befund beim Aufrufer).
func (cs *ChainState) quantenSEmpfaengerLocked(ctx context.Context, konto, n string) (string, error) {
	an := konto
	if n != "" {
		inhaber, err := cs.umzugNullifierInhaberLocked(ctx, n)
		if err != nil {
			return "", err
		}
		an = inhaber
	}
	if !kanonischeAdresse(an) || istSystemAdresse(an) {
		return "", nil
	}
	return an, nil
}

// quantenSIPfandZurueckBescheidLocked: R-P4, Rueckgabe. Caller haelt cs.mu.
func (cs *ChainState) quantenSIPfandZurueckBescheidLocked(ctx context.Context, tx *Transaction, reg *quantenWRegister, zeit int64) (quantenWBescheid, error) {
	if !quantenSPfaenderAktiv(zeit) {
		return quantenWBefund("quanten_s_pfand_inaktiv", "Pfaender sind noch nicht aktiv"), nil
	}
	q := tx.QuantenW
	p, da := reg.IPfand[q.K]
	if !da {
		return quantenWBefund("quanten_s_pfand_unbekannt", "kein Pfand der Inhaberin zu %s", q.K[:12]), nil
	}
	if p.Ende == 0 {
		return quantenWBefund("quanten_s_pfand_gebunden", "Token %s laeuft", q.K[:12]), nil
	}
	if zeit <= sattAdd(p.Ende, quantenSNachweisFrist) {
		return quantenWBefund("quanten_s_pfand_frist", "Pfand zu %s zurueck nach %d", q.K[:12], sattAdd(p.Ende, quantenSNachweisFrist)), nil
	}
	an, err := cs.quantenSEmpfaengerLocked(ctx, p.Konto, p.N)
	if err != nil {
		return quantenWBescheid{}, err
	}
	if an == "" {
		return quantenWBefund("quanten_s_pfand_empfaenger", "der Mensch hinter %s hat keine Wallet", kurzAdresse(p.Konto)), nil
	}
	neu := reg.kopie()
	delete(neu.IPfand, q.K)
	return quantenWBescheid{reg: neu, wege: []quantenWWeg{{quantenWPfandAddr, an, p.Betrag}}, felder: QuantenWAngaben{Pfand: p.Betrag, PfandAn: an}}, nil
}

// quantenSIPfandAnsicht: Pfand der Inhaberin in der Ansicht des Tokens.
func quantenSIPfandAnsicht(reg *quantenWRegister, k string, v map[string]interface{}) {
	if p, ok := reg.IPfand[k]; ok {
		v["pfand_inhaberin"] = p.Betrag
		if p.Ende != 0 {
			v["pfand_inhaberin_zurueck_nach"] = sattAdd(p.Ende, quantenSNachweisFrist)
		}
	}
}
