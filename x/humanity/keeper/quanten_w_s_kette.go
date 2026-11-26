package keeper

// QUANTENGELD VARIANTE S -- STUFE S2b TEIL 2: EINBINDUNG IN DIE KETTE
// (seit 17.11.2026 17:00 UTC im Hauptnetz freigeschaltet, mit W)
//
// Die Regeln (quanten_s.go; Spezifikation forschung/quantengeld/s_kette.py,
// KetteS) haengen hier an der Kette -- als weitere Satzart IM W-Modul
// (quanten_w_kette.go). Damit gelten Stichtag (quantenWAbUnix, MaxInt64 =
// schlafend), Buchzeit (BuchAt), StateRoot, Snapshot, Checkpoint, Rueckroll
// und Nachspielen unveraendert mit; das Register der Variante W traegt die
// Token unter "token".
//
// DIE SECHS ARTEN
//
//   - quanten_w_s_ausgeben (Wallet = Ausgeber, unterschrieben): Deckung
//     Wallet -> W-Topf (Anteile wie eine W-Note); Token mit n, t, Wurzel,
//     Toleranz, Inhaberin, den ZWEI Annahmestellen und je Stelle der
//     Bindung ihrer geheimen Stichprobe. ALLE Validatoren bestaetigen per
//     EIP-191 (Wurzel, Stichproben, Ausgabezeit), jeder im Validator-Register
//     an einen Menschen gebunden, alle an verschiedene (wie Variante W).
//   - quanten_w_s_binden / quanten_w_s_waehlen (Wallet = Inhaberin,
//     unterschrieben): die maskierten Basen d und die Wahl c, je einmal.
//   - quanten_w_s_anmelden (Wallet = Annahmestelle b, unterschrieben): die
//     Vorlage x mit der Unterschrift der Inhaberin (quantenSVorlageText) und
//     die Stichprobe der Stelle.
//   - quanten_w_s_offenlegen (ohne Wallet): ein Teil der Offenlegung (<= 64
//     Stellen der Stichprobe, der Reihe nach); die Pruefsummen beweisen ihn.
//   - quanten_w_s_frist (ohne Wallet): beendet eine Anmeldung, die nicht in
//     der Frist offengelegt wurde (kein Fehlversuch).
//
// ANNAHMESTELLEN FEST BEI DER AUSGABE (Abweichung von s_kette.py, die eine
// beliebige Stelle anmelden laesst): die Vorlage der Inhaberin bindet K, b und
// x, aber nicht die Adresse der Stelle. Die Anmeldung legt die geheime
// Stichprobe offen; wer sie in der Warteschlange sieht, koennte sie kopieren,
// zuerst anmelden und die Deckung einloesen. Darum darf nur die Stelle b
// anmelden, deren Adresse bei der Ausgabe steht -- und nur sie wird bezahlt.
//
// GELD: wie Variante W. Der Wert haengt an den Anteilen im W-Topf (die
// Umlaufabgabe trifft ihn wie jede Note). Bestanden: Wert an die Stelle.
// Nach quantenWFehlMax falschen Vorlagen ist der Token tot, der Wert geht ans
// Grundeinkommen. Kein Pfand: nur die festgelegte Stelle meldet an.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	quantenWSAusgebenTxArt   = "quanten_w_s_ausgeben"
	quantenWSBindenTxArt     = "quanten_w_s_binden"
	quantenWSWaehlenTxArt    = "quanten_w_s_waehlen"
	quantenWSAnmeldenTxArt   = "quanten_w_s_anmelden"
	quantenWSOffenlegenTxArt = "quanten_w_s_offenlegen"
	quantenWSFristTxArt      = "quanten_w_s_frist"

	quantenWSDomAusgabe  = "aequitas-quanten-s-ausgabe-v1"
	quantenWSDomBest     = "aequitas-quanten-s-anteile-v1"
	quantenWSDomBinden   = "aequitas-quanten-s-binden-v1"
	quantenWSDomWahl     = "aequitas-quanten-s-wahl-v1"
	quantenWSDomAnmelden = "aequitas-quanten-s-anmelden-v1"

	// Lebende Token im Register (wie die Noten: ein Wert in chain_config;
	// je Token bis ~4 KB Bindung und eine Anmeldung bis ~100 KB).
	quantenWTokenHoechstens = 16
)

func istQuantenWSTxArt(t string) bool {
	switch t {
	case quantenWSAusgebenTxArt, quantenWSBindenTxArt, quantenWSWaehlenTxArt, quantenWSAnmeldenTxArt, quantenWSOffenlegenTxArt, quantenWSFristTxArt:
		return true
	}
	return false
}

// quantenWSNurEigeneFelder: je S-Art nur ihre Angaben (q: K und Index schon
// geleert). Keine Art traegt eine Nummer.
func quantenWSNurEigeneFelder(tx *Transaction, q QuantenWAngaben) bool {
	if tx.QuantenW.Index != 0 {
		return false
	}
	// (Leeres salze ist schon in quantenWNurEigeneFelder wie fehlend.)
	ohneWallet := tx.Wallet == "" && tx.DistributionAt == 0
	switch tx.Type {
	case quantenWSAusgebenTxArt:
		q.N, q.T, q.Wurzel, q.Validatoren, q.Bestaetigungen, q.Deckung, q.Tol = 0, 0, "", nil, nil, 0, 0
		q.Inhaber, q.Annahme, q.Proben, q.WalletSig, q.PQ, q.Anteile, q.ProbeArt = "", nil, nil, "", nil, 0, 0
		q.Pfand, q.PfandAn = 0, "" // gerechnet: Weg B gibt das Pfand der Bestellung zurueck
		return reflect.DeepEqual(q, QuantenWAngaben{})
	case quantenWSBindenTxArt:
		q.D, q.WalletSig, q.PQ, q.Pfand = "", "", nil, 0 // Pfand: gerechnet (P3)
	case quantenWSWaehlenTxArt:
		q.C, q.WalletSig, q.PQ = "", "", nil
	case quantenWSAnmeldenTxArt:
		q.B, q.X, q.Probe, q.Vorlage, q.WalletSig, q.PQ, q.Salze = "", "", nil, "", "", nil, nil
	case quantenWSOffenlegenTxArt:
		q.Ab, q.Offen, q.Wert, q.WertAn, q.Urteil = 0, nil, 0, "", ""
		return reflect.DeepEqual(q, QuantenWAngaben{}) && ohneWallet
	case quantenWSFristTxArt:
		q.Wert, q.WertAn, q.Urteil = 0, "", ""
		return reflect.DeepEqual(q, QuantenWAngaben{}) && ohneWallet
	default:
		return false
	}
	return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.DistributionAt == 0
}

// ------------------------------------------------------------ Saetze

func quantenWSHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func quantenWSAusgabeNachricht(wallet, K string, n, t int, wurzel string, validatoren, proben, stellen []string, inhaber string, deckung, zeit int64, tol int) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d|%d|%s|%s|%s|%s|%s|%d|%d|%d", quantenWSDomAusgabe, eingefroreneChainID,
		strings.ToLower(wallet), K, n, t, wurzel, strings.Join(validatoren, ","), strings.Join(proben, ","),
		strings.Join(stellen, ","), strings.ToLower(inhaber), deckung, zeit, tol)
}

// quantenWSAusgabeNachrichtArt: wie quantenWSAusgabeNachricht; bei der
// Stichprobe v2 (probeArt 2) mit "|probe:2" -- der Ausgeber bindet die Art,
// bisherige Ausgaben behalten ihren Wortlaut.
func quantenWSAusgabeNachrichtArt(wallet, K string, n, t int, wurzel string, validatoren, proben, stellen []string, inhaber string, deckung, zeit int64, tol, probeArt int) string {
	m := quantenWSAusgabeNachricht(wallet, K, n, t, wurzel, validatoren, proben, stellen, inhaber, deckung, zeit, tol)
	if probeArt != 0 {
		m += fmt.Sprintf("|probe:%d", probeArt)
	}
	return m
}

// quantenWSSalzeLesen: je Abschnitt ein Salz (32 Byte hex).
func quantenWSSalzeLesen(roh []string, v int) ([][32]byte, bool) {
	if len(roh) != v {
		return nil, false
	}
	out := make([][32]byte, v)
	for i, s := range roh {
		h, ok := quantenWHex32(s)
		if !ok {
			return nil, false
		}
		out[i] = h
	}
	return out, true
}

// quantenWSBestaetigungNachricht: was Validator x (1-basiert) bei der Ausgabe
// bestaetigt -- er haelt die Anteile zu dieser Wurzel; dazu die Bindungen der
// Stichproben und die Ausgabezeit (alte Bestaetigungen verfallen wie bei W).
func quantenWSBestaetigungNachricht(K string, wurzel [32]byte, n, t int, validatoren []string, x int, proben [2][32]byte, zeit int64, tol int) string {
	return fmt.Sprintf("%s|chain:%d|%s|%x|%d|%d|%s|%d|%x,%x|%d|%d", quantenWSDomBest, eingefroreneChainID, K, wurzel, n, t,
		strings.Join(validatoren, ","), x, proben[0], proben[1], zeit, tol)
}

func quantenWSBindenNachricht(wallet, K, d string) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%s", quantenWSDomBinden, eingefroreneChainID, strings.ToLower(wallet), K, quantenWSHash(d))
}

func quantenWSWahlNachricht(wallet, K, c string) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%s", quantenWSDomWahl, eingefroreneChainID, strings.ToLower(wallet), K, c)
}

// quantenWSAnmeldenNachricht: bindet auch die Zahl der bisherigen
// Fehlversuche des Tokens. Sonst koennte jeder eine fehlgeschlagene Anmeldung
// unveraendert wieder einreichen, bis der Token tot ist -- und die ehrliche
// andere Stelle bekaeme nichts.
func quantenWSAnmeldenNachricht(wallet, K, b, x string, probe []int, fehl int) string {
	teile := make([]string, len(probe))
	for i, v := range probe {
		teile[i] = strconv.Itoa(v)
	}
	return fmt.Sprintf("%s|chain:%d|%s|%s|%s|%s|%s|%d", quantenWSDomAnmelden, eingefroreneChainID, strings.ToLower(wallet), K, b,
		quantenWSHash(x), quantenWSHash(strings.Join(teile, ",")), fehl)
}

// quantenWSBitsLesen: "0101..." genau n Zeichen.
func quantenWSBitsLesen(s string, n int) ([]int, bool) {
	if len(s) != n || n < 1 {
		return nil, false
	}
	out := make([]int, n)
	for i := 0; i < n; i++ {
		switch s[i] {
		case '0':
		case '1':
			out[i] = 1
		default:
			return nil, false
		}
	}
	return out, true
}

// quantenWSBit: "0" oder "1".
func quantenWSBit(s string) (int, bool) {
	switch s {
	case "0":
		return 0, true
	case "1":
		return 1, true
	}
	return 0, false
}

// quantenWBitsHex / quantenWBitsAusHex: Bitfolge der Laenge n als Bitfeld
// (Bit i%8 von Byte i/8), genau ceil(n/8) Bytes, kanonisch.
func quantenWBitsHex(bits []int) string {
	b := make([]byte, (len(bits)+7)/8)
	for i, v := range bits {
		if v == 1 {
			b[i/8] |= 1 << (i % 8)
		}
	}
	return hex.EncodeToString(b)
}

func quantenWBitsAusHex(s string, n int) ([]int, bool) {
	b, err := hex.DecodeString(s)
	if err != nil || n < 1 || len(b) != (n+7)/8 || s != hex.EncodeToString(b) {
		return nil, false
	}
	out := make([]int, n)
	for i := 0; i < len(b)*8; i++ {
		if b[i/8]&(1<<(i%8)) != 0 {
			if i >= n {
				return nil, false
			}
			out[i] = 1
		}
	}
	return out, true
}

// ------------------------------------------------------------ Register

// quantenWSEintrag: ein lebender S-Token mit seinen Anteilen am W-Topf und
// den Adressen seiner zwei Annahmestellen.
type quantenWSEintrag struct {
	token   *quantenSToken
	Anteile int64
	Zeit    int64 // Ausgabezeit
	Stellen [2]string
}

func (e *quantenWSEintrag) tiefeKopie() *quantenWSEintrag {
	t := *e.token
	t.D = append([]int(nil), e.token.D...)
	if e.token.D == nil {
		t.D = nil
	}
	if a := e.token.Anmeldung; a != nil {
		k := *a
		k.X, k.Probe = append([]int(nil), a.X...), append([]int(nil), a.Probe...)
		k.Salze = append([][32]byte(nil), a.Salze...)
		if a.Salze == nil {
			k.Salze = nil
		}
		t.Anmeldung = &k
	}
	return &quantenWSEintrag{token: &t, Anteile: e.Anteile, Zeit: e.Zeit, Stellen: e.Stellen}
}

type quantenWSAnmeldungForm struct {
	B       int    `json:"b"`
	Stelle  string `json:"stelle"`
	X       string `json:"x"`
	Probe   []int  `json:"probe"`
	Offen   int    `json:"offen,omitempty"`
	Treffer int    `json:"treffer,omitempty"`
	Fehler  int    `json:"fehler,omitempty"`
	Zeit    int64  `json:"zeit"`
	// Stichprobe v2: je Abschnitt das Salz (hex); fehlt bei bisherigen Token,
	// damit ihr Register und der StateRoot gleich bleiben.
	Salze []string `json:"salze,omitempty"`
}

type quantenWSTokenForm struct {
	K         string                  `json:"k"`
	N         int                     `json:"n"`
	Inhaber   string                  `json:"inhaber"`
	Wurzel    string                  `json:"wurzel"`
	Deckung   int64                   `json:"deckung"`
	Proben    [2]string               `json:"proben"`
	T         int                     `json:"t"`
	NVal      int                     `json:"n_val"`
	Tol       int                     `json:"tol"`
	D         string                  `json:"d,omitempty"` // leer = nicht gebunden
	C         int                     `json:"c"`           // -1 = nicht gewaehlt
	Fehl      int                     `json:"fehl,omitempty"`
	Anmeldung *quantenWSAnmeldungForm `json:"anmeldung,omitempty"`
	Anteile   int64                   `json:"anteile"`
	Zeit      int64                   `json:"zeit"`
	Stellen   [2]string               `json:"stellen"`
	ProbeArt  int                     `json:"probe_art,omitempty"` // 2 = Stichprobe v2; fehlt = wie bisher
}

func (e *quantenWSEintrag) form(K string) quantenWSTokenForm {
	t := e.token
	f := quantenWSTokenForm{K: K, N: t.N, Inhaber: t.Inhaber, Wurzel: hex.EncodeToString(t.Wurzel[:]), Deckung: t.Deckung,
		Proben: [2]string{hex.EncodeToString(t.Proben[0][:]), hex.EncodeToString(t.Proben[1][:])},
		T:      t.T, NVal: t.NVal, Tol: t.Tol, C: t.C, Fehl: t.Fehl, Anteile: e.Anteile, Zeit: e.Zeit, Stellen: e.Stellen, ProbeArt: t.ProbeArt}
	if t.D != nil {
		f.D = quantenWBitsHex(t.D)
	}
	if a := t.Anmeldung; a != nil {
		f.Anmeldung = &quantenWSAnmeldungForm{B: a.B, Stelle: a.Stelle, X: quantenWBitsHex(a.X), Probe: a.Probe,
			Offen: a.Offen, Treffer: a.Treffer, Fehler: a.Fehler, Zeit: a.Zeit}
		for _, z := range a.Salze {
			f.Anmeldung.Salze = append(f.Anmeldung.Salze, hex.EncodeToString(z[:]))
		}
	}
	return f
}

// quantenWSTokenLesen: ein Token aus dem Register (fail-closed: was so nie
// entstehen konnte, wird abgewiesen).
func quantenWSTokenLesen(f quantenWSTokenForm) (*quantenWSEintrag, error) {
	falsch := func(was string) (*quantenWSEintrag, error) {
		return nil, fmt.Errorf("quanten_w: Token %s: %s", f.K[:12], was)
	}
	wurzel, ok1 := quantenWHex32(f.Wurzel)
	p0, ok2 := quantenWHex32(f.Proben[0])
	p1, ok3 := quantenWHex32(f.Proben[1])
	if !ok1 || !ok2 || !ok3 || f.Anteile <= 0 || f.Zeit <= 0 {
		return falsch("Form")
	}
	if !quantenWSStellenGueltig(f.Stellen, f.Inhaber) {
		return falsch("Stellen")
	}
	t, err := neuerQuantenSToken(f.N, f.Inhaber, wurzel, f.Deckung, [2][32]byte{p0, p1}, f.T, f.NVal, f.Tol)
	if err != nil {
		return falsch("Groessen")
	}
	if f.ProbeArt != 0 && f.ProbeArt != 2 {
		return falsch("Art der Stichprobe")
	}
	t.ProbeArt = f.ProbeArt
	if f.D != "" {
		if t.D, ok1 = quantenWBitsAusHex(f.D, f.N); !ok1 {
			return falsch("Bindung")
		}
	}
	if f.C < -1 || f.C > 1 || (f.C != -1 && t.D == nil) || f.Fehl < 0 || f.Fehl >= quantenWFehlMax {
		return falsch("Wahl oder Fehlversuche")
	}
	t.C, t.Fehl = f.C, f.Fehl
	if a := f.Anmeldung; a != nil {
		x, ok := quantenWBitsAusHex(a.X, f.N)
		if !ok || t.C == -1 || (a.B != 0 && a.B != 1) || a.Stelle != f.Stellen[a.B] || len(a.Probe) == 0 || len(a.Probe) > f.N ||
			a.Offen < 0 || a.Offen >= len(a.Probe) || a.Treffer < 0 || a.Treffer > a.Offen || a.Fehler < 0 || a.Fehler > a.Treffer || a.Zeit <= 0 {
			return falsch("Anmeldung")
		}
		for i, v := range a.Probe {
			if v < 0 || v >= f.N || (i > 0 && v <= a.Probe[i-1]) {
				return falsch("Stichprobe der Anmeldung")
			}
		}
		var salze [][32]byte
		if t.ProbeArt == 2 {
			if salze, ok = quantenWSSalzeLesen(a.Salze, f.NVal); !ok {
				return falsch("Salze der Anmeldung")
			}
		}
		if !t.probePasst(f.K, a.B, a.Probe, salze) || (t.ProbeArt != 2 && len(a.Salze) != 0) {
			return falsch("Stichprobe passt nicht zur Ausgabe")
		}
		t.Anmeldung = &quantenSAnmeldung{B: a.B, Stelle: a.Stelle, X: x, Probe: append([]int(nil), a.Probe...),
			Offen: a.Offen, Treffer: a.Treffer, Fehler: a.Fehler, Zeit: a.Zeit, Salze: salze}
	}
	return &quantenWSEintrag{token: t, Anteile: f.Anteile, Zeit: f.Zeit, Stellen: f.Stellen}, nil
}

// quantenWSStellenGueltig: zwei verschiedene, kanonische Adressen, keine
// Systemadresse und nicht die Inhaberin (sonst bezahlte sie sich selbst).
func quantenWSStellenGueltig(st [2]string, inhaber string) bool {
	for _, s := range st {
		if !kanonischeAdresse(s) || istSystemAdresse(s) || s == inhaber {
			return false
		}
	}
	return st[0] != st[1]
}

// ------------------------------------------------------------ Bescheid

func (cs *ChainState) quantenWSBescheidLocked(ctx context.Context, tx *Transaction, wallet string, reg *quantenWRegister, blockZeit int64) (quantenWBescheid, error) {
	q := tx.QuantenW
	// Stichprobe v2 schlaeft bis zum Stichtag: ein Satz mit ihren Feldern
	// bekommt einen Befund (kommt so in keinen Block -- siehe
	// quantenSProbeV2AbUnix, warum Uebergehen falsch waere).
	if (q.ProbeArt != 0 || len(q.Salze) != 0) && !quantenSProbeV2Aktiv(blockZeit) {
		return quantenWBefund("quanten_w_s_probe_v2", "Stichprobe v2 ist noch nicht aktiv"), nil
	}
	if tx.Type == quantenWSAusgebenTxArt {
		return cs.quantenWSAusgebenBescheidLocked(ctx, tx, wallet, reg, blockZeit)
	}
	alt := reg.Token[q.K]
	if alt == nil {
		return quantenWBefund("quanten_w_unbekannt", "Token %s ist nicht (mehr) da", q.K[:12]), nil
	}
	neu := alt.tiefeKopie()
	e := neu.token
	u := quantenWBescheid{}
	switch tx.Type {
	case quantenWSBindenTxArt, quantenWSWaehlenTxArt:
		if !kanonischeAdresse(wallet) {
			return quantenWBefund("quanten_w_form", "Wallet nicht kanonisch"), nil
		}
		var nachricht string
		if tx.Type == quantenWSBindenTxArt {
			nachricht = quantenWSBindenNachricht(wallet, q.K, q.D)
		} else {
			nachricht = quantenWSWahlNachricht(wallet, q.K, q.C)
		}
		if err := quantenWUnterschrift(q, nachricht, wallet, blockZeit); err != nil {
			return quantenWBefund("quanten_w_unterschrift", "%s: %v", kurzAdresse(wallet), err), nil
		}
		if tx.Type == quantenWSBindenTxArt {
			d, ok := quantenWSBitsLesen(q.D, e.N)
			if !ok {
				return quantenWBefund("quanten_w_form", "Bindung: genau %d Bits", e.N), nil
			}
			if err := e.binden(wallet, d); err != nil {
				return quantenWBefund("quanten_w_regel", "%v", err), nil
			}
		} else {
			c, ok := quantenWSBit(q.C)
			if !ok {
				return quantenWBefund("quanten_w_form", "Wahl: 0 oder 1"), nil
			}
			if err := e.waehlen(wallet, c); err != nil {
				return quantenWBefund("quanten_w_regel", "%v", err), nil
			}
		}

	case quantenWSAnmeldenTxArt:
		b, ok := quantenWSBit(q.B)
		if !ok || !kanonischeAdresse(wallet) {
			return quantenWBefund("quanten_w_form", "Anmelden: Stelle 0 oder 1, Wallet kanonisch"), nil
		}
		// Nur die bei der Ausgabe festgelegte Stelle b (siehe oben).
		if wallet != alt.Stellen[b] {
			return quantenWBefund("quanten_w_s_stelle", "%s ist nicht Annahmestelle %d dieses Tokens", kurzAdresse(wallet), b), nil
		}
		x, ok := quantenWSBitsLesen(q.X, e.N)
		if !ok || len(q.Probe) == 0 || len(q.Probe) > e.N {
			return quantenWBefund("quanten_w_form", "Anmelden: Vorlage (%d Bits) oder Stichprobe", e.N), nil
		}
		if err := quantenWUnterschrift(q, quantenWSAnmeldenNachricht(wallet, q.K, q.B, q.X, q.Probe, e.Fehl), wallet, blockZeit); err != nil {
			return quantenWBefund("quanten_w_unterschrift", "Stelle %s: %v", kurzAdresse(wallet), err), nil
		}
		if !kanonischeSignatur(q.Vorlage) || pruefePersonalSignGemerkt(quantenSVorlageText(q.K, b, x), q.Vorlage, e.Inhaber) != nil {
			return quantenWBefund("quanten_w_s_vorlage", "Vorlage nicht von der Inhaberin unterschrieben"), nil
		}
		var salze [][32]byte
		if e.ProbeArt == 2 {
			if salze, ok = quantenWSSalzeLesen(q.Salze, e.NVal); !ok {
				return quantenWBefund("quanten_w_form", "Anmelden: je Abschnitt ein Salz (%d)", e.NVal), nil
			}
		} else if len(q.Salze) != 0 {
			return quantenWBefund("quanten_w_form", "Anmelden: Salze nur bei Stichprobe v2"), nil
		}
		if err := e.anmelden(q.K, b, wallet, x, q.Probe, salze, blockZeit); err != nil {
			return quantenWBefund("quanten_w_regel", "%v", err), nil
		}

	case quantenWSOffenlegenTxArt:
		a := e.Anmeldung
		if a == nil || q.Ab < 0 || len(q.Offen) == 0 || len(q.Offen) > quantenWOffenTeil {
			return quantenWBefund("quanten_w_form", "Offenlegen: Anmeldung, Beginn oder Zahl der Stellen"), nil
		}
		offen, ok := quantenWOffenLesen(q.Offen)
		if !ok {
			return quantenWBefund("quanten_w_form", "Offenlegen: Anteile oder Pfade nicht in gueltiger Form"), nil
		}
		stelle := a.Stelle
		fertig, bestanden, err := e.offenlegenTeil(q.K, q.Ab, offen, blockZeit)
		if err != nil {
			return quantenWBefund("quanten_w_regel", "%v", err), nil
		}
		switch {
		case !fertig:
			u.felder.Urteil = "teil"
		case bestanden:
			u.felder.Urteil, u.felder.WertAn = "echt", stelle
		default:
			u.felder.Urteil = "abgewiesen"
		}

	case quantenWSFristTxArt:
		if !e.frist(blockZeit) {
			return quantenWBefund("quanten_w_frist", "Token %s: keine Anmeldung, deren Frist abgelaufen ist", q.K[:12]), nil
		}
		u.felder.Urteil = "frist"
	}

	u.reg = reg.kopie()
	if e.Eingeloest || e.Tot {
		// Der Wert verlaesst den W-Topf: an die Stelle (bestanden) oder ans
		// Grundeinkommen (tot nach quantenWFehlMax falschen Vorlagen).
		wert := quantenWert(alt.Anteile, cs.quantenWMikroLocked(ctx, quantenWTopfAddr), reg.summeAnteile())
		an := ubiPoolAddr
		if e.Eingeloest {
			an = u.felder.WertAn
		}
		u.wege = append(u.wege, quantenWWeg{quantenWTopfAddr, an, wert})
		u.felder.Wert = wert
		delete(u.reg.Token, q.K)
		urteil := quantenWUrteilTot
		if e.Eingeloest {
			urteil = quantenWUrteilEingeloest
		}
		u.reg.toteMerken(q.K, alt.Zeit, blockZeit, urteil)
		quantenSIPfandEnde(u.reg, q.K, blockZeit) // P3: die Frist laeuft ab dem Ende
		// P4: eingeloest mit Pfand der Inhaberin -- der Stoff fuer einen Nachweis.
		if e.Eingeloest && quantenSPfaenderAktiv(blockZeit) {
			an := 0
			if alt.Stellen[1] == u.felder.WertAn {
				an = 1
			}
			quantenSNachweisAnlegen(u.reg, q.K, neu, an, blockZeit)
		}
		// P2a: das Los bleibt nur zusammen mit dem toten Eintrag (Frist, P4).
		if _, tot := u.reg.Tote[q.K]; !tot {
			delete(u.reg.Lose, q.K)
		}
	} else {
		u.reg.Token[q.K] = neu
	}
	// P3 (quanten_s_inhaberin_pfand.go): ab dem Stichtag der Pfaender nimmt die
	// Bindung die Deckung als Pfand der Inhaberin.
	if tx.Type == quantenWSBindenTxArt && quantenSPfaenderAktiv(blockZeit) {
		return cs.quantenSIPfandBindenLocked(ctx, &u, q.K, wallet, e.Deckung)
	}
	return u, nil
}

// quantenWSAusgebenBescheidLocked: Form, Unterschriften von Ausgeber und
// allen Validatoren, Register der Validatoren, Stellen, Deckung, Anteile.
func (cs *ChainState) quantenWSAusgebenBescheidLocked(ctx context.Context, tx *Transaction, wallet string, reg *quantenWRegister, blockZeit int64) (quantenWBescheid, error) {
	q := tx.QuantenW
	zeit := tx.DistributionAt
	if !kanonischeAdresse(wallet) || istSystemAdresse(wallet) {
		return quantenWBefund("quanten_w_form", "Ausgeber nicht kanonisch"), nil
	}
	if reg.Noten[q.K] != nil || reg.Token[q.K] != nil {
		return quantenWBefund("quanten_w_doppelt", "%s gibt es schon", q.K[:12]), nil
	}
	if _, tot := reg.Tote[q.K]; tot {
		return quantenWBefund("quanten_w_doppelt", "%s war schon ausgegeben (tot)", q.K[:12]), nil
	}
	// P3: solange das Pfand einer frueheren Inhaberin zu K liegt, kein neuer
	// Token mit K (es ueberschriebe das Pfand, und das Register waere nicht
	// mehr lesbar).
	if _, da := reg.IPfand[q.K]; da {
		return quantenWBefund("quanten_w_doppelt", "zu %s liegt noch das Pfand einer Inhaberin", q.K[:12]), nil
	}
	if len(reg.Token) >= quantenWTokenHoechstens {
		return quantenWBefund("quanten_w_voll", "Register voll (%d Token)", len(reg.Token)), nil
	}
	wurzel, ok := quantenWHex32(q.Wurzel)
	if !ok || len(q.Proben) != 2 || len(q.Annahme) != 2 || len(q.Bestaetigungen) != len(q.Validatoren) ||
		len(q.Validatoren) < q.T || len(q.Validatoren) > quantenWVMax {
		return quantenWBefund("quanten_w_form", "Wurzel, Stichproben, Stellen oder Bestaetigungen"), nil
	}
	var proben [2][32]byte
	for i := range proben {
		if proben[i], ok = quantenWHex32(q.Proben[i]); !ok {
			return quantenWBefund("quanten_w_form", "Stichprobe %d", i), nil
		}
	}
	inhaber := q.Inhaber
	stellen := [2]string{q.Annahme[0], q.Annahme[1]}
	if !kanonischeAdresse(inhaber) || istSystemAdresse(inhaber) || !quantenWSStellenGueltig(stellen, inhaber) {
		return quantenWBefund("quanten_w_form", "Inhaberin oder Annahmestellen"), nil
	}
	gesehen := map[string]bool{}
	for x, v := range q.Validatoren {
		if !kanonischeAdresse(v) || gesehen[v] {
			return quantenWBefund("quanten_w_form", "Validator %d", x+1), nil
		}
		gesehen[v] = true
	}
	if q.ProbeArt != 0 && q.ProbeArt != 2 {
		return quantenWBefund("quanten_w_form", "Art der Stichprobe %d", q.ProbeArt), nil
	}
	tok, err := neuerQuantenSToken(q.N, inhaber, wurzel, q.Deckung, proben, q.T, len(q.Validatoren), q.Tol)
	if err != nil {
		return quantenWBefund("quanten_w_form", "%v", err), nil
	}
	tok.ProbeArt = q.ProbeArt
	if err := quantenWUnterschrift(q, quantenWSAusgabeNachrichtArt(wallet, q.K, q.N, q.T, q.Wurzel, q.Validatoren, q.Proben, q.Annahme, inhaber, q.Deckung, zeit, q.Tol, q.ProbeArt), wallet, blockZeit); err != nil {
		return quantenWBefund("quanten_w_unterschrift", "Ausgeber %s: %v", kurzAdresse(wallet), err), nil
	}
	bestaetigt := func(x int) string {
		return quantenWSBestaetigungNachricht(q.K, wurzel, q.N, q.T, q.Validatoren, x+1, proben, zeit, q.Tol)
	}
	// Weg B (quanten_s_bestellung.go): ein bestelltes K nur durch die
	// Gelosten, mit dem Schluessel ihrer Konten. Sonst Weg A: gebundene
	// Validatoren verschiedener Menschen.
	if b := reg.Best[q.K]; b != nil {
		if u := quantenSAusgabeGegenLos(b, wallet, q, blockZeit, bestaetigt); u.regel != "" {
			return u, nil
		}
	} else if u, err := cs.quantenWValidatorenLocked(ctx, q.Validatoren, q.Bestaetigungen, bestaetigt); err != nil || u.regel != "" {
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
		return quantenWBefund("quanten_w_voll", "Topf nimmt keinen Token an (Anteile)"), nil
	}
	neu := reg.kopie()
	if !quantenWToteAufraeumen(neu, blockZeit) {
		return quantenWBefund("quanten_w_voll", "zu viele tote Noten und Token im Ausgabefenster"), nil
	}
	neu.Token[q.K] = &quantenWSEintrag{token: tok, Anteile: anteile, Zeit: zeit, Stellen: stellen}
	u := quantenWBescheid{reg: neu, wege: []quantenWWeg{{wallet, quantenWTopfAddr, q.Deckung}}, felder: QuantenWAngaben{Anteile: anteile}}
	// Weg B: die Bestellung ist verbraucht, ihr Pfand geht zurueck.
	if b := reg.Best[q.K]; b != nil {
		delete(neu.Best, q.K)
		// P2a: die Kette behaelt, wer gelost war (quanten_s_los_behalten.go).
		if quantenSLosBehaltenAktiv(blockZeit) {
			neu.Lose[q.K] = slices.Clone(b.Los)
		}
		u.wege = append(u.wege, quantenWWeg{quantenWPfandAddr, wallet, b.Pfand})
		u.felder.Pfand, u.felder.PfandAn = b.Pfand, wallet
	}
	return u, nil
}

// quantenWSTokenAnsicht: der oeffentliche Stand eines Tokens (GET
// /api/quanten-w/note zeigt Noten und Token). Caller haelt cs.mu (lesend).
func (cs *ChainState) quantenWSTokenAnsichtLocked(ctx context.Context, reg *quantenWRegister, k string) map[string]interface{} {
	e := reg.Token[k]
	if e == nil {
		return nil
	}
	t := e.token
	// v: Zahl der Validatoren, bei denen das Geheimnis liegt (t davon noetig).
	// Nur Ansicht -- damit sieht jeder, wem er vertraut; bei v = 1 kennt ein
	// einziger Validator das ganze Geheimnis (Pruefauftrag W2, S8).
	ansicht := map[string]interface{}{"k": k, "art": "s", "n": t.N, "t": t.T, "v": t.NVal, "tol": t.Tol, "inhaber": t.Inhaber,
		"stellen": e.Stellen, "gebunden": t.D != nil, "gewaehlt": t.C != -1, "fehl": t.Fehl,
		"wert_mikro": quantenWert(e.Anteile, cs.quantenWMikroLocked(ctx, quantenWTopfAddr), reg.summeAnteile())}
	// Stichprobe v2 (quanten_s_probe_v2.go): nur gesetzt, wenn der Token so
	// ausgegeben wurde -- dann kannte vor Bindung und Wahl keine Annahmestelle
	// eine Stelle der Stichproben (Befund 5i greift nicht). Fehlt das Feld,
	// ist es ein Token im bisherigen Weg.
	if t.ProbeArt != 0 {
		ansicht["probe_art"] = t.ProbeArt
	}
	// Ab dem Stichtag steht das Urteil nach dem Ende in der Ansicht
	// (quanten_w_urteil.go) -- erst dann lohnt sich der letzte Versuch fuer
	// ein Werkzeug, das das Urteil lesen muss.
	if quantenWUrteilAktiv(time.Now().Unix()) {
		ansicht["urteil_eindeutig"] = true
	}
	// Wurzel, Proben, Bindung d und Wahl c: oeffentlich (sie stehen in den
	// Transaktionen der Ausgabe, Bindung und Wahl). Damit pruefen Inhaberin
	// und Annahmestelle bei getrennten Rollen (quanten_s_rollen.go) gegen die
	// Kette, statt einem Paket des Ausgebers oder einer Vorlage zu glauben.
	ansicht["wurzel"] = hex.EncodeToString(t.Wurzel[:])
	ansicht["proben"] = []string{hex.EncodeToString(t.Proben[0][:]), hex.EncodeToString(t.Proben[1][:])}
	if t.D != nil {
		ansicht["d"] = qsBitsText(t.D)
	}
	if t.C != -1 {
		ansicht["c"] = t.C
	}
	if a := t.Anmeldung; a != nil {
		probe := append([]int(nil), a.Probe...)
		sort.Ints(probe)
		ansicht["anmeldung"] = map[string]interface{}{"b": a.B, "stelle": a.Stelle, "probe": probe, "offen": a.Offen, "zeit": a.Zeit}
	}
	// P2a: wer gelost war (Weg B, ab dem Stichtag), aus der Kette statt aus
	// dem Paket des Ausgebers.
	quantenSLosAnsicht(reg, k, ansicht)
	quantenSIPfandAnsicht(reg, k, ansicht)
	return ansicht
}
