package keeper

// QUANTENGELD S, WEG B -- STUFE B2b: BESTELLUNG, LOS, AUSGABE GEGEN DAS LOS
// (forschung/quantengeld/DEZENTRAL.md 18; s_weg_b.py KetteB)
//
//   - quanten_w_s_bestellen (Wallet = Ausgeber, unterschrieben): K, v
//     (praep_zahl) und t. Die Kette rechnet das Abbild des Registers JETZT
//     (quantenSPraepAbbildLocked, Zeit = Buchzeit) und haelt es in der
//     Bestellung fest -- spaetere Meldungen, Abmeldungen oder Umzuege
//     verschieben dieses Los nicht. Die Buchzeit darf hoechstens
//     quantenSBestellBuchSpiel vor dem Block liegen (sonst waehlte der
//     Blockerzeuger eine vergangene, schon bekannte drand-Runde).
//   - quanten_w_s_losen (ohne Wallet, jeder): Runde und Signatur von drand.
//     Die Runde ist FEST: die erste mit Zeit >= Bestellzeit +
//     quantenSLosAbstand (wer einreicht, kann keine andere waehlen). Jeder
//     Knoten prueft die BLS-Signatur selbst (drand_zufall.go); Los =
//     quantenSLos(Abbild, SHA-256(Signatur), K, v).
//   - quanten_w_s_ausgeben fuer ein K mit Bestellung: nur der Ausgeber der
//     Bestellung, t wie bestellt, Validatoren == Los in Losreihenfolge, jeder
//     Geloste bestaetigt mit dem Schluessel SEINES Kontos (die Gelosten sind
//     Menschen, je Mensch einer -- das Validator-Register braucht es hier
//     nicht). Danach ist die Bestellung verbraucht.
//
// GRENZEN: hoechstens quantenSBestellHoechstens offene Bestellungen, je
// Ausgeber (ein Mensch) eine. Eine Bestellung gilt quantenSBestellFrist; eine
// verfallene bleibt bis zum Ende der Frist stehen und sperrt ihren Ausgeber.
//
// BEFUND (B2b): ABBRUCH UND NEU BESTELLEN. s_weg_b.py zeigt, dass ein Los je
// Bestellung unverzerrt ist. Ein Ausgeber kann aber nach dem Los abbrechen
// (nicht ausgeben) und neu bestellen, bis ein Los t Komplizen traegt: nach m
// Versuchen 1 - (1 - p)^m. Die Grenze
// "eine Bestellung je Mensch je Frist" bremst das nur. Teuer machen muss es
// ein Pfand je Bestellung, das beim Abbruch verfaellt (B2c). Bis dahin bleibt
// Weg B schlafend.
//
// EHRLICH: Solange C1 allein Bloecke erzeugt, kann C1 die Blockzeit hinter
// der echten Zeit zurueckhalten (Knoten pruefen nur "nicht in der Zukunft"
// und "nicht vor den Eltern"). Dann laege die feste Runde schon in der
// Vergangenheit, und C1 mit Ausgeber koennte ein guenstiges Los waehlen. Das
// ist von aussen sichtbar (Blockzeit gegen drand-Zeit); ein Waechter dafuer
// gehoert vor den Stichtag.

import (
	"context"
	"encoding/hex"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
)

const (
	quantenSBestellenTxArt    = "quanten_w_s_bestellen"
	quantenSLosenTxArt        = "quanten_w_s_losen"
	quantenSBestellFristTxArt = "quanten_w_s_bestellung_frist"

	quantenSDomBestellen = "aequitas-quanten-s-bestellen-v1"

	// Zeile in quantenWTabelle.
	quantenWArtBest = "best"

	quantenSBestellHoechstens = 32
	// So lange gilt eine Bestellung (Losen und Ausgabe); kuerzer als der
	// Nachlauf des Registers.
	quantenSBestellFrist int64 = 2 * 86400
	// So weit darf die Buchzeit einer Bestellung vor ihrem Block liegen.
	quantenSBestellBuchSpiel int64 = 120
	// Abstand zwischen Bestellung und der Runde, die lost.
	quantenSLosAbstand int64 = 600
	// Pfand je Bestellung in Prozent der bestellten Deckung (mindestens
	// quantenWPfand). Siehe quantenSBestellPfandFuer.
	quantenSBestellPfandProzent = 10
)

// quantenSBestellPfandFuer: max(quantenWPfand, ceil(D * 10 %)). Wer nach dem
// Los abbricht, verliert das Pfand ans Grundeinkommen (B2c). Lohnt sich ein
// Los mit t Komplizen nur mit Wahrscheinlichkeit p je Bestellung (s_weg_b.py
// erfassung; 4 von 7 bei 20 % Boesen ~2,5 %, in der Tabelle hoechstens ~3,1 %),
// kostet ein solches Los im Mittel Pfand / p >= 3 D -- mehr, als die Doppel-
// einloesung einbringt (D).
func quantenSBestellPfandFuer(deckung int64) int64 {
	p := deckung / 100 * quantenSBestellPfandProzent
	if r := deckung % 100 * quantenSBestellPfandProzent; r > 0 {
		p += (r + 99) / 100
	}
	if p < quantenWPfand {
		return quantenWPfand
	}
	return p
}

func istQuantenSBestellTxArt(t string) bool {
	return t == quantenSBestellenTxArt || t == quantenSLosenTxArt || t == quantenSBestellFristTxArt
}

// quantenSBestellung: eine offene Bestellung (Los leer = noch nicht gelost).
type quantenSBestellung struct {
	Ausgeber string
	V, T     int
	Zeit     int64
	Deckung  int64 // die Ausgabe muss genau sie decken
	Pfand    int64 // im W-Pfandtopf bis Ausgabe (zurueck) oder Frist (verfaellt)
	Abbild   []string
	Runde    uint64
	Zufall   [32]byte
	Los      []string
}

type quantenSBestellungForm struct {
	K        string   `json:"k"`
	Ausgeber string   `json:"ausgeber"`
	V        int      `json:"v"`
	T        int      `json:"t"`
	Zeit     int64    `json:"zeit"`
	Deckung  int64    `json:"deckung"`
	Pfand    int64    `json:"pfand"`
	Abbild   []string `json:"abbild"`
	Runde    uint64   `json:"runde,omitempty"`
	Zufall   string   `json:"zufall,omitempty"`
	Los      []string `json:"los,omitempty"`
}

// rundeAb: die erste Runde, deren Zeit >= t ist (fail-closed: false,
// wenn t vor der Genesis liegt oder ausserhalb).
func (n drandNetz) rundeAb(t int64) (uint64, bool) {
	if t <= n.genesis {
		return 1, true
	}
	d := t - n.genesis
	r := uint64(d/n.periode) + 1
	if d%n.periode != 0 {
		r++
	}
	if _, ok := n.zeitDerRunde(r); !ok {
		return 0, false
	}
	return r, true
}

// quantenSLosRunde: die Runde, die eine Bestellung lost.
func quantenSLosRunde(bestellZeit int64) (uint64, bool) {
	return drandQuicknet.rundeAb(sattAdd(bestellZeit, quantenSLosAbstand))
}

func quantenSBestellNachricht(wallet, K string, v, t int, deckung, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d|%d|%d|%d", quantenSDomBestellen, eingefroreneChainID, strings.ToLower(wallet), K, v, t, deckung, zeit)
}

// quantenSBestellungGroessen: v, t wie fuer ein Los und einen Token moeglich.
func quantenSBestellungGroessen(v, t int) bool {
	return v >= quantenSLosVMin && v <= quantenSLosVMax && t >= 1 && t <= v && t <= quantenWTMax
}

func (b *quantenSBestellung) form(K string) quantenSBestellungForm {
	f := quantenSBestellungForm{K: K, Ausgeber: b.Ausgeber, V: b.V, T: b.T, Zeit: b.Zeit, Deckung: b.Deckung, Pfand: b.Pfand, Abbild: b.Abbild, Runde: b.Runde, Los: b.Los}
	if b.Runde != 0 {
		f.Zufall = hex.EncodeToString(b.Zufall[:])
	}
	return f
}

// quantenSBestellungenLesen: fail-closed. Das Los rechnet jeder Knoten aus
// Abbild und Zufall nach; die Runde muss die der Bestellzeit sein.
func quantenSBestellungenLesen(fs []quantenSBestellungForm) (map[string]*quantenSBestellung, error) {
	if len(fs) > quantenSBestellHoechstens {
		return nil, fmt.Errorf("quanten_w: %d Bestellungen, hoechstens %d", len(fs), quantenSBestellHoechstens)
	}
	out := make(map[string]*quantenSBestellung, len(fs))
	ausgeber := map[string]bool{}
	for i, f := range fs {
		falsch := fmt.Errorf("quanten_w: Bestellung %d: Form", i)
		if !istHash64(f.K) || (i > 0 && f.K <= fs[i-1].K) || !kanonischeAdresse(f.Ausgeber) || ausgeber[f.Ausgeber] ||
			!quantenSBestellungGroessen(f.V, f.T) || f.Zeit <= 0 || len(f.Abbild) < quantenSLosPoolFaktor*f.V || len(f.Abbild) > quantenSPraepHoechstens ||
			slices.Contains(f.Abbild, f.Ausgeber) || // nie im eigenen Los (s. bestellen)
			f.Deckung < quantenMindestDeckungMik || f.Pfand != quantenSBestellPfandFuer(f.Deckung) {
			return nil, falsch
		}
		ausgeber[f.Ausgeber] = true
		for j, a := range f.Abbild {
			if !kanonischeAdresse(a) || (j > 0 && a <= f.Abbild[j-1]) {
				return nil, falsch
			}
		}
		b := &quantenSBestellung{Ausgeber: f.Ausgeber, V: f.V, T: f.T, Zeit: f.Zeit, Deckung: f.Deckung, Pfand: f.Pfand, Abbild: slices.Clone(f.Abbild)}
		if f.Runde == 0 {
			if f.Zufall != "" || f.Los != nil {
				return nil, falsch
			}
		} else {
			z, ok := quantenWHex32(f.Zufall)
			r, okR := quantenSLosRunde(f.Zeit)
			if !ok || !okR || f.Runde != r {
				return nil, falsch
			}
			los, err := quantenSLos(b.Abbild, z, f.K, f.V)
			if err != nil || !slices.Equal(los, f.Los) {
				return nil, falsch
			}
			b.Runde, b.Zufall, b.Los = f.Runde, z, los
		}
		out[f.K] = b
	}
	return out, nil
}

func quantenSBestellungenFormen(m map[string]*quantenSBestellung) []quantenSBestellungForm {
	var out []quantenSBestellungForm
	for k, b := range m {
		out = append(out, b.form(k))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].K < out[j].K })
	return out
}

// quantenSBestellungVerfallen: nach der Frist (Losen und Ausgabe vorbei).
func quantenSBestellungVerfallen(b *quantenSBestellung, zeit int64) bool {
	return sattAdd(b.Zeit, quantenSBestellFrist) <= zeit
}

// quantenSBestellungenAufraeumen: verfallene Bestellungen vergessen; ihr Pfand
// geht ans Grundeinkommen (Wege in fester Reihenfolge nach K).
func quantenSBestellungenAufraeumen(r *quantenWRegister, zeit int64) []quantenWWeg {
	var ks []string
	for k, b := range r.Best {
		if quantenSBestellungVerfallen(b, zeit) {
			ks = append(ks, k)
		}
	}
	sort.Strings(ks)
	var wege []quantenWWeg
	for _, k := range ks {
		wege = append(wege, quantenWWeg{quantenWPfandAddr, ubiPoolAddr, r.Best[k].Pfand})
		delete(r.Best, k)
	}
	return wege
}

// quantenSBestellNurEigeneFelder: Bestellen traegt T, praep_zahl und die
// Unterschrift; Losen (ohne Wallet und Zeit) Runde und Signatur.
func quantenSBestellNurEigeneFelder(tx *Transaction, q QuantenWAngaben) bool {
	if tx.QuantenW.Index != 0 {
		return false
	}
	switch tx.Type {
	case quantenSLosenTxArt:
		q.Runde, q.RundeSig = 0, ""
		return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.Wallet == "" && tx.DistributionAt == 0
	case quantenSBestellFristTxArt:
		q.Pfand = 0 // gerechnet
		return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.Wallet == "" && tx.DistributionAt == 0
	}
	q.T, q.PraepZahl, q.Deckung, q.WalletSig, q.PQ, q.Pfand = 0, 0, 0, "", nil, 0
	return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.Wallet != ""
}

// quantenSBestellBescheidLocked: Bestellen und Losen. Caller haelt cs.mu.
func (cs *ChainState) quantenSBestellBescheidLocked(ctx context.Context, tx *Transaction, wallet string, reg *quantenWRegister, zeit int64) (quantenWBescheid, error) {
	if !quantenSWegBAktiv(zeit) {
		return quantenWBefund("quanten_s_praep_inaktiv", "Weg B (Praeparatoren) ist noch nicht aktiv"), nil
	}
	if tx.Wallet != wallet {
		return quantenWBefund("quanten_w_form", "Wallet nicht kanonisch"), nil
	}
	q := tx.QuantenW
	neu := reg.kopie()
	switch tx.Type {
	case quantenSBestellenTxArt:
		if !kanonischeAdresse(wallet) || istSystemAdresse(wallet) || !quantenSBestellungGroessen(q.PraepZahl, q.T) || q.Deckung < quantenMindestDeckungMik {
			return quantenWBefund("quanten_w_form", "Bestellen: Ausgeber, v (%d), t (%d) oder Deckung (%d)", q.PraepZahl, q.T, q.Deckung), nil
		}
		if err := quantenWUnterschrift(q, quantenSBestellNachricht(wallet, q.K, q.PraepZahl, q.T, q.Deckung, tx.DistributionAt), wallet, zeit); err != nil {
			return quantenWBefund("quanten_w_unterschrift", "Ausgeber %s: %v", kurzAdresse(wallet), err), nil
		}
		if !cs.quantenSPraepIstMenschLocked(ctx, wallet) {
			return quantenWBefund("quanten_s_praep_mensch", "Ausgeber %s ist kein registrierter Mensch", kurzAdresse(wallet)), nil
		}
		if reg.Noten[q.K] != nil || reg.Token[q.K] != nil {
			return quantenWBefund("quanten_w_doppelt", "%s gibt es schon", q.K[:12]), nil
		}
		if _, tot := reg.Tote[q.K]; tot {
			return quantenWBefund("quanten_w_doppelt", "%s war schon ausgegeben (tot)", q.K[:12]), nil
		}
		wege := quantenSBestellungenAufraeumen(neu, zeit)
		if neu.Best[q.K] != nil {
			return quantenWBefund("quanten_w_doppelt", "%s ist schon bestellt", q.K[:12]), nil
		}
		if reg.S3[q.K] != nil {
			return quantenWBefund("quanten_w_doppelt", "%s ist ueber eine Leitung vorgemerkt", q.K[:12]), nil
		}
		for _, b := range neu.Best {
			if b.Ausgeber == wallet {
				return quantenWBefund("quanten_s_bestellung_offen", "%s hat schon eine Bestellung bis %d", kurzAdresse(wallet), sattAdd(b.Zeit, quantenSBestellFrist)), nil
			}
		}
		if len(neu.Best) >= quantenSBestellHoechstens {
			return quantenWBefund("quanten_w_voll", "zu viele offene Bestellungen (%d)", len(neu.Best)), nil
		}
		if _, ok := quantenSLosRunde(zeit); !ok {
			return quantenWBefund("quanten_w_zeit", "keine drand-Runde zu %d", zeit), nil
		}
		abbild := cs.quantenSPraepAbbildLocked(ctx, reg, zeit)
		// Der Ausgeber praepariert nie fuer die eigene Ausgabe (Entscheidung
		// 25.11.2026): sonst hielte er selbst einen der v Abschnitte (Werte,
		// Koeffizienten, die Basen y der Inhaberin dazu) und braeuchte nur
		// t-1 weitere. Er faellt aus dem Abbild, bevor die Groesse zaehlt.
		abbild = slices.DeleteFunc(abbild, func(a string) bool { return a == wallet })
		if len(abbild) < quantenSLosPoolFaktor*q.PraepZahl {
			return quantenWBefund("quanten_s_los_register", "Register zu klein: %d im Los, noetig %d fuer v = %d", len(abbild), quantenSLosPoolFaktor*q.PraepZahl, q.PraepZahl), nil
		}
		pfand := quantenSBestellPfandFuer(q.Deckung)
		if cs.quantenWMikroLocked(ctx, wallet) < pfand {
			return quantenWBefund("quanten_w_deckung", "%s hat keine %d Mikro-AEQ Pfand", kurzAdresse(wallet), pfand), nil
		}
		neu.Best[q.K] = &quantenSBestellung{Ausgeber: wallet, V: q.PraepZahl, T: q.T, Zeit: zeit, Deckung: q.Deckung, Pfand: pfand, Abbild: abbild}
		return quantenWBescheid{reg: neu, wege: append(wege, quantenWWeg{wallet, quantenWPfandAddr, pfand}), felder: QuantenWAngaben{Pfand: pfand}}, nil

	case quantenSBestellFristTxArt:
		b := reg.Best[q.K]
		if b == nil {
			return quantenWBefund("quanten_w_unbekannt", "keine Bestellung %s", q.K[:12]), nil
		}
		if !quantenSBestellungVerfallen(b, zeit) {
			return quantenWBefund("quanten_w_frist", "Bestellung %s gilt bis %d", q.K[:12], sattAdd(b.Zeit, quantenSBestellFrist)), nil
		}
		delete(neu.Best, q.K)
		return quantenWBescheid{reg: neu, wege: []quantenWWeg{{quantenWPfandAddr, ubiPoolAddr, b.Pfand}}, felder: QuantenWAngaben{Pfand: b.Pfand}}, nil

	case quantenSLosenTxArt:
		b := reg.Best[q.K]
		if b == nil {
			return quantenWBefund("quanten_w_unbekannt", "keine Bestellung %s", q.K[:12]), nil
		}
		if b.Runde != 0 {
			return quantenWBefund("quanten_s_los_doppelt", "%s ist schon gelost", q.K[:12]), nil
		}
		if quantenSBestellungVerfallen(b, zeit) {
			return quantenWBefund("quanten_w_frist", "Bestellung %s ist verfallen", q.K[:12]), nil
		}
		r, ok := quantenSLosRunde(b.Zeit)
		if !ok || q.Runde != r {
			return quantenWBefund("quanten_s_los_runde", "es lost Runde %d, nicht %d", r, q.Runde), nil
		}
		zufall, err := drandQuicknet.pruefen(q.Runde, q.RundeSig)
		if err != nil {
			return quantenWBefund("quanten_s_los_drand", "%v", err), nil
		}
		los, err := quantenSLos(b.Abbild, zufall, q.K, b.V)
		if err != nil {
			return quantenWBefund("quanten_s_los_register", "%v", err), nil
		}
		nb := *b
		nb.Runde, nb.Zufall, nb.Los = q.Runde, zufall, los
		neu.Best[q.K] = &nb
	}
	return quantenWBescheid{reg: neu}, nil
}

// quantenSAusgabeGegenLos: die S-Ausgabe eines bestellten K (aus
// quantenWSAusgebenBescheidLocked). regel != "": abgewiesen. Unterschriften
// der Gelosten mit dem Schluessel ihres Kontos.
func quantenSAusgabeGegenLos(b *quantenSBestellung, wallet string, q *QuantenWAngaben, zeit int64, nachricht func(x int) string) quantenWBescheid {
	if b.Runde == 0 {
		return quantenWBefund("quanten_s_los_offen", "%s ist bestellt, aber noch nicht gelost", q.K[:12])
	}
	if quantenSBestellungVerfallen(b, zeit) {
		return quantenWBefund("quanten_w_frist", "Bestellung %s ist verfallen", q.K[:12])
	}
	if wallet != b.Ausgeber || q.T != b.T || q.Deckung != b.Deckung {
		return quantenWBefund("quanten_s_los_ausgeber", "Ausgabe passt nicht zur Bestellung (Ausgeber, t oder Deckung)")
	}
	if !slices.Equal(q.Validatoren, b.Los) {
		return quantenWBefund("quanten_s_los_fremd", "Praeparatoren sind nicht die Gelosten in Losreihenfolge")
	}
	if len(q.Bestaetigungen) != len(b.Los) {
		return quantenWBefund("quanten_w_form", "Bestaetigungen")
	}
	for x, konto := range b.Los {
		if !kanonischeSignatur(q.Bestaetigungen[x]) || pruefePersonalSignGemerkt(nachricht(x), q.Bestaetigungen[x], konto) != nil {
			return quantenWBefund("quanten_w_bestaetigung", "Praeparator %d (%s) hat nicht bestaetigt", x+1, kurzAdresse(konto))
		}
	}
	return quantenWBescheid{}
}

// quantenSBestellungAnsicht: fuer GET /api/quanten-w/praeparatoren (ohne
// das Abbild, nur seine Groesse).
func quantenSBestellungAnsicht(K string, b *quantenSBestellung) map[string]interface{} {
	r, _ := quantenSLosRunde(b.Zeit)
	m := map[string]interface{}{"k": K, "ausgeber": b.Ausgeber, "v": b.V, "t": b.T, "zeit": b.Zeit,
		"frist": sattAdd(b.Zeit, quantenSBestellFrist), "im_abbild": len(b.Abbild), "runde": r, "deckung": b.Deckung, "pfand": b.Pfand}
	if b.Runde != 0 {
		m["los"] = b.Los
	}
	return m
}
