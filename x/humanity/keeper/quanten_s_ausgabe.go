package keeper

// QUANTENGELD VARIANTE S -- DIE ERSTE ECHTE AUSGABE AUF DEM HAUPTNETZ
//
// `aequitasd quanten-s-ausgabe` (cmd/aequitasd) auf C1, gestartet von
// deploy/quanten-s-ausgabe.sh ueber .github/workflows/quanten-s-ausgabe.yml.
// Die Messung kommt von einem IBM-Chip (hardware_ibm.py --s-messen): Basen u
// und Werte t der Ausgabe, Messbasen y und Ergebnisse x der Inhaberin.
//
// ABLAUF (jeder Schritt ueber die oeffentliche API des eigenen Knotens):
//  1. Pruefgroesse: k = n/2 Stellen je Annahmestelle muessen mindestens
//     k_note (Tabelle, Toleranz, t, v) sein -- sonst kein Token (fail-closed).
//  2. Ausgabe: der Knotenschluessel ist Ausgeber UND einziger Validator. Die
//     Bestaetigung kommt vom Validator-Dienst, sie wird HIER nachgeprueft.
//  3. Inhaberin (Wegwerfschluessel) bindet d = y XOR z und waehlt c = b XOR z.
//  4. DOPPELVORLAGE: dieselbe Vorlage x an der ANDEREN Stelle (Wegwerf-
//     schluessel). Erwartet: abgewiesen, ein Fehlversuch, Token lebt.
//  5. Ehrliche Vorlage an Stelle b. Stelle b ist der Knoten selbst: der Wert
//     kommt zurueck. Erwartet: eingeloest.
//
// NICHTS WIRD GEGLAUBT: Unterschriften des Dienstes werden geprueft; das
// Urteil liest das Werkzeug aus dem Stand des Tokens. Fehlversuche bis zum Tod
// des Tokens sind quantenWFehlMax = 3; nach 0 und 1 Fehlversuchen kann ein
// verschwundener Token darum nur EINGELOEST sein. Verschwindet er nach der
// Doppelvorlage, ist das ein Befund (Exit != 0), kein Erfolg.
//
// GRENZEN: jede Anfrage hat eine Frist, jede Antwort eine Groessengrenze,
// Wiederholungen bei 429/503 sind gezaehlt, der ganze Lauf hat eine Frist.
// Kein Schluessel wird ausgegeben; die Wegwerfschluessel leben nur im
// Speicher (die Inhaberin und die zweite Stelle halten keinen Wert).

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	quantenSAusgabeAntwortMax = 1 << 20 // Antwort des Knotens
	// Antwort auf einen Abschnitts-Auftrag: Blaetter und je Validator ein
	// Buendel (bis 16.384 Stellen, als Chiffrat in Hex).
	quantenSAbschnittAntwortMax = 8 << 20
	quantenSAusgabeMessMax      = 1 << 20 // Messdatei
	quantenSAusgabeVersuche     = 8       // je Anfrage bei 429/503/Netzfehler
)

// QuantenSMessung: was der IBM-Lauf liefert (hardware_ibm.py --s-messen).
type QuantenSMessung struct {
	N  int    `json:"n"`
	U  string `json:"u"` // Basen der Ausgabe
	T  string `json:"t"` // Werte der Ausgabe
	Y  string `json:"y"` // Messbasen der Inhaberin
	X  string `json:"x"` // ihre Ergebnisse
	Wo string `json:"wo"`
	Jo string `json:"job"`
}

// QuantenSAusgabeErgebnis: was am Ende ausgegeben wird (nichts Geheimes).
type QuantenSAusgabeErgebnis struct {
	K              string   `json:"k"`
	N              int      `json:"n"`
	KJeStelle      int      `json:"k_je_stelle"`
	Toleranz       int      `json:"toleranz_prozent"`
	Ausgeber       string   `json:"ausgeber"`
	Inhaberin      string   `json:"inhaberin"`
	Stellen        []string `json:"stellen"`
	B              int      `json:"gewaehlte_stelle"`
	Deckung        int64    `json:"deckung_mikro"`
	Doppelvorlage  string   `json:"doppelvorlage"` // "abgewiesen" erwartet
	Vorlage        string   `json:"vorlage"`       // "eingeloest" erwartet
	FehlerrateMess string   `json:"fehlerrate_passende_basis"`
	Wo             string   `json:"wo"`
	Job            string   `json:"job"`
	// Wurzel und Proben der Ausgabe (oeffentlich, Abschnitts-Modus) und, wenn
	// verlangt, der Nachweis nach dem Ende (quanten_s_nachweis.go).
	Wurzel         string                   `json:"wurzel,omitempty"`
	Proben         []string                 `json:"proben,omitempty"`
	Nachweis       *QuantenSNachweisBericht `json:"nachweis,omitempty"`
	NachweisFehler string                   `json:"nachweis_fehler,omitempty"`
}

// quantenSNetz: was das Werkzeug vom Knoten braucht (Tests setzen ein).
type quantenSNetz interface {
	bestaetigen(ctx context.Context, req QuantenWBestaetigenAnfrage) (string, error)
	einreichen(ctx context.Context, art, wallet string, zeit int64, q QuantenWAngaben) error
	// stand: der oeffentliche Stand; nil = unbekannt (404).
	stand(ctx context.Context, K string) (map[string]interface{}, error)
	// warten: zwischen zwei Abfragen (Tests: einen Takt der Dienste).
	warten(ctx context.Context) error
}

type QuantenSAusgabeAuftrag struct {
	Messung    QuantenSMessung
	Schluessel *ecdsa.PrivateKey
	Deckung    int64
	Tol        int
	Frist      time.Duration // je Schritt
	// Jetzt: Uhr der Ausgabezeit (nil = time.Now; Tests setzen ein).
	Jetzt func() int64
	// bz: Stelle b und Maske z fest (nur Tests; nil = Zufall).
	bz *[2]int
	// ProbeV2: Stichprobe erst nach der Bindung (Abschnitts-Modus;
	// quanten_s_probe_v2.go, DEZENTRAL.md 16). Die Kette muss sie kennen.
	ProbeV2 bool
	// log, beginn: Protokoll des Laufs mit Zeiten (gesetzt von
	// quantenSAusgabeLauf), damit jeder Lauf zeigt, wie lange jeder Schritt
	// und die Offenlegung gedauert haben.
	log    io.Writer
	beginn time.Time
}

// quantenSNach: seit Beginn des Laufs, auf Sekunden gerundet.
func (a QuantenSAusgabeAuftrag) quantenSNach() string {
	if a.beginn.IsZero() {
		return "?"
	}
	return time.Since(a.beginn).Round(time.Second).String()
}

// quantenSFortschrittTakt: so oft schreibt das Werkzeug waehrend der
// Offenlegung den Stand ins Protokoll (hoechstens Urteilsfrist / Takt Zeilen).
const quantenSFortschrittTakt = time.Minute

func quantenSBitsText(s string, n int) ([]int, error) {
	if len(s) != n {
		return nil, fmt.Errorf("%d statt %d Bits", len(s), n)
	}
	out := make([]int, n)
	for i := 0; i < n; i++ {
		switch s[i] {
		case '0':
		case '1':
			out[i] = 1
		default:
			return nil, fmt.Errorf("Zeichen %d ist kein Bit", i)
		}
	}
	return out, nil
}

func quantenSZufallInt(grenze int) (int, error) {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(grenze)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}

func quantenSUnterschreiben(k *ecdsa.PrivateKey, msg string) (string, error) {
	s, err := crypto.Sign(accounts.TextHash([]byte(msg)), k)
	if err != nil {
		return "", err
	}
	s[64] += 27
	return "0x" + hex.EncodeToString(s), nil
}

func quantenSAdresse(k *ecdsa.PrivateKey) string {
	return strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())
}

// quantenSPraegung: Werte, Pruefsummen, Wurzel und Shamir-Anteile (t von v)
// zu den Basen u und Werten t der Ausgabe -- wie der Praeger einer W-Note.
type quantenSPraegung struct {
	blaetter [][32]byte
	wurzel   [32]byte
	anteile  [][]string // [Validator][Stelle]
}

func quantenSPraegen(K string, u, tw []int, t, v int) (*quantenSPraegung, error) {
	p := &quantenSPraegung{anteile: make([][]string, v)}
	salzGrenze := new(big.Int).Lsh(big.NewInt(1), quantenWSalzBits)
	for i := range u {
		w, err := rand.Int(rand.Reader, salzGrenze)
		if err != nil {
			return nil, err
		}
		w.Lsh(w, 2).Add(w, big.NewInt(int64(u[i]+2*tw[i])))
		p.blaetter = append(p.blaetter, quantenWPruefsumme(K, i, w))
		koeff := []*big.Int{w}
		for j := 1; j < t; j++ {
			c, err := rand.Int(rand.Reader, quantenWP)
			if err != nil {
				return nil, err
			}
			koeff = append(koeff, c)
		}
		for x := 1; x <= v; x++ {
			y := new(big.Int)
			for j := len(koeff) - 1; j >= 0; j-- {
				y.Mul(y, big.NewInt(int64(x))).Add(y, koeff[j]).Mod(y, quantenWP)
			}
			p.anteile[x-1] = append(p.anteile[x-1], y.Text(16))
		}
	}
	p.wurzel = quantenWMerkleWurzel(p.blaetter)
	return p, nil
}

// quantenSAusgabeLauf: der ganze Ablauf. validatoren: hier genau der
// Ausgeber selbst (C1); Tests duerfen mehrere setzen.
func quantenSAusgabeLauf(ctx context.Context, a QuantenSAusgabeAuftrag, validatoren []string, t int, netz quantenSNetz, log io.Writer) (*QuantenSAusgabeErgebnis, error) {
	a.log, a.beginn = log, time.Now()
	m := a.Messung
	if m.N < 2 || m.N%2 != 0 || m.N > quantenWNMax {
		return nil, fmt.Errorf("Messung: n = %d (gerade, 2..%d)", m.N, quantenWNMax)
	}
	var bits [4][]int
	for i, s := range []string{m.U, m.T, m.Y, m.X} {
		b, err := quantenSBitsText(s, m.N)
		if err != nil {
			return nil, fmt.Errorf("Messung, Feld %d: %v", i, err)
		}
		bits[i] = b
	}
	u, tw, y, x := bits[0], bits[1], bits[2], bits[3]
	v := len(validatoren)
	k := m.N / 2
	if kn := quantenWKNote(a.Tol, t, v); kn == 0 || k < kn {
		return nil, fmt.Errorf("Pruefgroesse: %d Stellen je Annahmestelle, sicher erst ab %d (Toleranz %d %%, t=%d, v=%d)", k, kn, a.Tol, t, v)
	}
	if a.Deckung < quantenMindestDeckungMik {
		return nil, fmt.Errorf("Deckung %d, mindestens %d Mikro-AEQ", a.Deckung, quantenMindestDeckungMik)
	}
	passend, falsch := 0, 0
	for i := range u {
		if y[i] == u[i] {
			passend++
			if x[i] != tw[i] {
				falsch++
			}
		}
	}

	ausgeber := quantenSAdresse(a.Schluessel)
	var kb [32]byte
	if _, err := rand.Read(kb[:]); err != nil {
		return nil, err
	}
	K := hex.EncodeToString(kb[:])
	inhaberKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}
	fremdKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}
	inhaber := quantenSAdresse(inhaberKey)
	b, err := quantenSZufallInt(2)
	if err != nil {
		return nil, err
	}
	z, err := quantenSZufallInt(2)
	if err != nil {
		return nil, err
	}
	if a.bz != nil {
		b, z = a.bz[0], a.bz[1]
	}
	// Stelle b ist der Ausgeber selbst (der Wert kommt zurueck), die andere
	// ein Wegwerfschluessel (sie bekommt nichts, wenn die Physik haelt).
	var stellen [2]string
	var stellenKey [2]*ecdsa.PrivateKey
	stellen[b], stellenKey[b] = ausgeber, a.Schluessel
	stellen[1-b], stellenKey[1-b] = quantenSAdresse(fremdKey), fremdKey

	// Zwei disjunkte geheime Stichproben zu je k Stellen.
	perm := make([]int, m.N)
	for i := range perm {
		perm[i] = i
	}
	for i := m.N - 1; i > 0; i-- {
		j, err := quantenSZufallInt(i + 1)
		if err != nil {
			return nil, err
		}
		perm[i], perm[j] = perm[j], perm[i]
	}
	var probe [2][]int
	var proben [2][32]byte
	var probenHex []string
	for s := 0; s < 2; s++ {
		probe[s] = append([]int(nil), perm[s*k:(s+1)*k]...)
		sort.Ints(probe[s])
		proben[s] = quantenSProbeHash(K, s, probe[s])
		probenHex = append(probenHex, hex.EncodeToString(proben[s][:]))
	}

	p, err := quantenSPraegen(K, u, tw, t, v)
	if err != nil {
		return nil, err
	}
	wurzel := hex.EncodeToString(p.wurzel[:])
	zeit := time.Now().Unix()
	if a.Jetzt != nil {
		zeit = a.Jetzt()
	}
	fmt.Fprintf(log, "Token %s: n=%d, %d Stellen je Annahmestelle, Fehlerrate bei passender Basis %d/%d\n", K[:12], m.N, k, falsch, passend)
	// Die ehrliche Vorlage muss an BEIDEN Stellen bestehen -- exakt wie die
	// Kette zaehlt, vor jedem Schritt bei den Validatoren und auf der Kette
	// (quanten_s_toleranz.go).
	var treffer, fehler [2]int
	for s := 0; s < 2; s++ {
		for _, i := range probe[s] {
			if y[i] == u[i] {
				treffer[s]++
				if x[i] != tw[i] {
					fehler[s]++
				}
			}
		}
	}
	if err := quantenSEhrlichBesteht(treffer, fehler, a.Tol); err != nil {
		return nil, err
	}

	// 2. Bestaetigungen der Validatoren -- selbst nachgeprueft.
	var bestaetigungen []string
	for xi := 1; xi <= v; xi++ {
		req := QuantenWBestaetigenAnfrage{K: K, N: m.N, T: t, Tol: a.Tol, X: xi, Wurzel: wurzel, Validatoren: validatoren,
			Zeit: zeit, Anteile: p.anteile[xi-1], Art: "s", Proben: probenHex, Praeger: ausgeber}
		for _, bl := range p.blaetter {
			req.Blaetter = append(req.Blaetter, hex.EncodeToString(bl[:]))
		}
		if req.PraegerSig, err = quantenSUnterschreiben(a.Schluessel, quantenWDienstAuftrag(req)); err != nil {
			return nil, err
		}
		sig, err := netz.bestaetigen(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("Validator %d bestaetigt nicht: %w", xi, err)
		}
		if !kanonischeSignatur(sig) || verifyPersonalSign(quantenWSBestaetigungNachricht(K, p.wurzel, m.N, t, validatoren, xi, proben, zeit, a.Tol), sig, validatoren[xi-1]) != nil {
			return nil, fmt.Errorf("Validator %d: Bestaetigung ist nicht von ihm", xi)
		}
		bestaetigungen = append(bestaetigungen, sig)
	}
	return quantenSAusgabeAbschluss(ctx, a, netz, log, quantenSVorbereitet{K: K, N: m.N, T: t, KJeStelle: k, Validatoren: validatoren,
		Bestaetigungen: bestaetigungen, Wurzel: wurzel, ProbenHex: probenHex, Probe: probe, Stellen: stellen, StellenKey: stellenKey,
		Ausgeber: ausgeber, Inhaber: inhaber, InhaberKey: inhaberKey, B: b, Z: z, Y: y, X: x, Zeit: zeit,
		Fehlerrate: fmt.Sprintf("%d/%d", falsch, passend), Wo: m.Wo, Job: m.Jo})
}

// quantenSVorbereitet: was beide Wege der Ausgabe (ein Auftrag fuer alle
// Stellen, oder Abschnitte je Validator) fuer die Kette bereithalten.
type quantenSVorbereitet struct {
	K              string
	N, T           int
	KJeStelle      int
	Validatoren    []string
	Bestaetigungen []string
	Wurzel         string
	ProbenHex      []string
	Probe          [2][]int
	Stellen        [2]string
	StellenKey     [2]*ecdsa.PrivateKey
	Ausgeber       string
	Inhaber        string
	InhaberKey     *ecdsa.PrivateKey
	B, Z           int
	Y, X           []int
	Zeit           int64
	Fehlerrate     string
	Wo, Job        string
	// Stichprobe v2: Art 2, und die Stichproben (mit Salzen je Abschnitt)
	// werden erst nach Bindung und Wahl geladen.
	ProbeArt    int
	ProbenLaden func(ctx context.Context) ([2][]int, [2][]string, error)
}

// quantenSAusgabeAbschluss: Ausgabe auf der Kette, Bindung, Wahl,
// Doppelvorlage und ehrliche Vorlage -- fuer beide Wege gleich.
func quantenSAusgabeAbschluss(ctx context.Context, a QuantenSAusgabeAuftrag, netz quantenSNetz, log io.Writer, vb quantenSVorbereitet) (*QuantenSAusgabeErgebnis, error) {
	K, n, t, k, validatoren, bestaetigungen, wurzel, probenHex := vb.K, vb.N, vb.T, vb.KJeStelle, vb.Validatoren, vb.Bestaetigungen, vb.Wurzel, vb.ProbenHex
	probe, stellen, stellenKey, ausgeber, inhaber, inhaberKey := vb.Probe, vb.Stellen, vb.StellenKey, vb.Ausgeber, vb.Inhaber, vb.InhaberKey
	b, z, y, x, zeit := vb.B, vb.Z, vb.Y, vb.X, vb.Zeit
	var err error
	q := QuantenWAngaben{K: K, N: n, T: t, Wurzel: wurzel, Validatoren: validatoren, Bestaetigungen: bestaetigungen,
		Deckung: a.Deckung, Tol: a.Tol, Inhaber: inhaber, Annahme: stellen[:], Proben: probenHex, ProbeArt: vb.ProbeArt}
	if err := quantenSAusgabeEinreichen(ctx, a, netz, q, ausgeber, zeit); err != nil {
		return nil, err
	}
	fmt.Fprintf(log, "ausgegeben (nach %s)\n", a.quantenSNach())

	// 3. Bindung und Wahl der Inhaberin.
	d := make([]int, n)
	for i := range y {
		d[i] = y[i] ^ z
	}
	if err := quantenSBinden(ctx, netz, a.Frist, K, inhaber, inhaberKey, d); err != nil {
		return nil, err
	}
	if err := quantenSWaehlen(ctx, netz, a.Frist, K, inhaber, inhaberKey, b^z); err != nil {
		return nil, err
	}
	fmt.Fprintf(log, "gebunden und gewaehlt (nach %s)\n", a.quantenSNach())
	// Stichprobe v2: erst jetzt (d und c stehen) laden die Stellen ihre
	// Stichproben -- vorher wusste niemand, welche Stelle wem gehoert.
	var salze [2][]string
	if vb.ProbeArt == 2 {
		if vb.ProbenLaden == nil {
			return nil, errors.New("Stichprobe v2 ohne Ladeweg")
		}
		if probe, salze, err = vb.ProbenLaden(ctx); err != nil {
			return nil, err
		}
		k = min(len(probe[0]), len(probe[1]))
		fmt.Fprintf(log, "Stichproben nach der Bindung geladen: %d/%d Stellen (nach %s)\n", len(probe[0]), len(probe[1]), a.quantenSNach())
	}

	erg := &QuantenSAusgabeErgebnis{K: K, N: n, KJeStelle: k, Toleranz: a.Tol, Ausgeber: ausgeber, Inhaberin: inhaber, Stellen: stellen[:], B: b,
		Deckung: a.Deckung, FehlerrateMess: vb.Fehlerrate, Wo: vb.Wo, Job: vb.Job, Wurzel: vb.Wurzel, Proben: append([]string(nil), vb.ProbenHex...)}

	// 4. Doppelvorlage an der anderen Stelle, 5. ehrlich an Stelle b.
	for i, s := range []int{1 - b, b} {
		urteil, err := quantenSVorlegen(ctx, netz, a, K, s, stellen[s], stellenKey[s], inhaberKey, x, probe[s], salze[s], i)
		if err != nil {
			return erg, err
		}
		if i == 0 {
			erg.Doppelvorlage = urteil
			fmt.Fprintf(log, "Doppelvorlage: %s (nach %s)\n", urteil, a.quantenSNach())
			if urteil != "abgewiesen" {
				return erg, fmt.Errorf("BEFUND: Doppelvorlage an Stelle %d nicht abgewiesen (%s)", s, urteil)
			}
		} else {
			erg.Vorlage = urteil
			fmt.Fprintf(log, "ehrliche Vorlage: %s (nach %s)\n", urteil, a.quantenSNach())
			if urteil != "eingeloest" {
				return erg, fmt.Errorf("ehrliche Vorlage an Stelle %d nicht eingeloest (%s)", s, urteil)
			}
		}
	}
	return erg, nil
}

// quantenSAusgabeEinreichen: die Ausgabe-Transaktion, vom Ausgeber
// unterschrieben, und warten, bis der Token auf der Kette steht.
func quantenSAusgabeEinreichen(ctx context.Context, a QuantenSAusgabeAuftrag, netz quantenSNetz, q QuantenWAngaben, ausgeber string, zeit int64) error {
	var err error
	if q.WalletSig, err = quantenSUnterschreiben(a.Schluessel, quantenWSAusgabeNachrichtArt(ausgeber, q.K, q.N, q.T, q.Wurzel, q.Validatoren, q.Proben, q.Annahme, q.Inhaber, q.Deckung, zeit, q.Tol, q.ProbeArt)); err != nil {
		return err
	}
	if err := netz.einreichen(ctx, quantenWSAusgebenTxArt, ausgeber, zeit, q); err != nil {
		return fmt.Errorf("Ausgabe: %w", err)
	}
	_, err = quantenSWartenAuf(ctx, netz, q.K, a.Frist, "ausgegeben", func(s map[string]interface{}) bool { return s != nil && s["art"] == "s" })
	return err
}

// quantenSBinden: die Inhaberin bindet d (= y XOR z), unterschrieben mit
// ihrem Schluessel; warten, bis es auf der Kette steht.
func quantenSBinden(ctx context.Context, netz quantenSNetz, frist time.Duration, K, inhaber string, inhaberKey *ecdsa.PrivateKey, d []int) error {
	ds := qsBitsText(d)
	sigD, err := quantenSUnterschreiben(inhaberKey, quantenWSBindenNachricht(inhaber, K, ds))
	if err != nil {
		return err
	}
	if err := netz.einreichen(ctx, quantenWSBindenTxArt, inhaber, 0, QuantenWAngaben{K: K, D: ds, WalletSig: sigD}); err != nil {
		return fmt.Errorf("Bindung: %w", err)
	}
	_, err = quantenSWartenAuf(ctx, netz, K, frist, "gebunden", func(s map[string]interface{}) bool { return s != nil && s["gebunden"] == true })
	return err
}

// quantenSWaehlen: die Inhaberin waehlt c (= b XOR z); warten, bis es auf
// der Kette steht.
func quantenSWaehlen(ctx context.Context, netz quantenSNetz, frist time.Duration, K, inhaber string, inhaberKey *ecdsa.PrivateKey, c int) error {
	if c != 0 && c != 1 {
		return fmt.Errorf("Wahl %d", c)
	}
	cs := string(rune('0' + c))
	sigC, err := quantenSUnterschreiben(inhaberKey, quantenWSWahlNachricht(inhaber, K, cs))
	if err != nil {
		return err
	}
	if err := netz.einreichen(ctx, quantenWSWaehlenTxArt, inhaber, 0, QuantenWAngaben{K: K, C: cs, WalletSig: sigC}); err != nil {
		return fmt.Errorf("Wahl: %w", err)
	}
	_, err = quantenSWartenAuf(ctx, netz, K, frist, "gewaehlt", func(s map[string]interface{}) bool { return s != nil && s["gewaehlt"] == true })
	return err
}

func qsBitsText(b []int) string {
	var sb strings.Builder
	for _, v := range b {
		sb.WriteByte(byte('0' + v))
	}
	return sb.String()
}

// quantenSVorlegen: Stelle s meldet die Vorlage x an; die Validatoren legen
// offen. Urteil aus dem Stand: "eingeloest" (Token weg), "abgewiesen" (ein
// Fehlversuch mehr), "frist" (Anmeldung weg, kein Fehlversuch).
func quantenSVorlegen(ctx context.Context, netz quantenSNetz, a QuantenSAusgabeAuftrag, K string, s int, stelle string, stelleKey, inhaberKey *ecdsa.PrivateKey, x, probe []int, salze []string, fehl int) (string, error) {
	vorlage, err := quantenSUnterschreiben(inhaberKey, quantenSVorlageText(K, s, x))
	if err != nil {
		return "", err
	}
	return quantenSAnmeldenUrteil(ctx, netz, a, K, s, stelle, stelleKey, x, vorlage, probe, salze, fehl)
}

// quantenSAnmeldenUrteil: Stelle s meldet die Vorlage (von der Inhaberin
// unterschrieben) mit ihrer Stichprobe an und wartet auf das Urteil der Kette.
func quantenSAnmeldenUrteil(ctx context.Context, netz quantenSNetz, a QuantenSAusgabeAuftrag, K string, s int, stelle string, stelleKey *ecdsa.PrivateKey, x []int, vorlage string, probe []int, salze []string, fehl int) (string, error) {
	vorher, err := netz.stand(ctx, K)
	if err != nil {
		return "", err
	}
	if vorher == nil || quantenSZahl(vorher["fehl"]) != fehl {
		return "", fmt.Errorf("vor der Vorlage an Stelle %d: Token fehlt oder %v statt %d Fehlversuche", s, vorher["fehl"], fehl)
	}
	if fehl+1 >= quantenWFehlMax {
		return "", fmt.Errorf("zu viele Fehlversuche, um das Urteil eindeutig zu lesen")
	}
	xs := qsBitsText(x)
	bs := string(rune('0' + s))
	q := QuantenWAngaben{K: K, B: bs, X: xs, Probe: probe, Vorlage: vorlage, Salze: salze}
	if q.WalletSig, err = quantenSUnterschreiben(stelleKey, quantenWSAnmeldenNachricht(stelle, K, bs, xs, probe, fehl)); err != nil {
		return "", err
	}
	if err := netz.einreichen(ctx, quantenWSAnmeldenTxArt, stelle, 0, q); err != nil {
		return "", fmt.Errorf("Anmeldung an Stelle %d: %w", s, err)
	}
	if _, err := quantenSWartenAuf(ctx, netz, K, a.Frist, "angemeldet", func(st map[string]interface{}) bool {
		return st == nil || st["anmeldung"] != nil || quantenSZahl(st["fehl"]) != fehl
	}); err != nil {
		return "", err
	}
	// Auf das Urteil wartet das Werkzeug mindestens so lange, wie die Kette
	// der Offenlegung gibt (quantenWFristOffen ab der Anmeldung), plus
	// quantenSUrteilNachlauf fuer Finalitaet und die Frist-Einreichung der
	// Dienste. Vorher meldete es nach a.Frist einen Fehlschlag, obwohl die
	// Kette noch pruefte (Pilot 17.11.2026: k = 5.200, Offenlegung > 15 min).
	// Nachvollziehbar: Beginn der Offenlegung, je Minute der Stand, am Ende
	// Dauer und Rate (Stellen je Minute) -- im Protokoll jedes Laufs.
	logW := a.log
	if logW == nil {
		logW = io.Discard
	}
	angemeldet := time.Now()
	fmt.Fprintf(logW, "Stelle %d: angemeldet, %d Stellen offenzulegen (nach %s)\n", s, len(probe), a.quantenSNach())
	letzte := angemeldet
	nach, err := quantenSWartenAuf(ctx, netz, K, quantenSUrteilFrist(a.Frist), "Urteil", func(st map[string]interface{}) bool {
		fertig := st == nil || st["tot"] == true || st["anmeldung"] == nil
		if !fertig && time.Since(letzte) >= quantenSFortschrittTakt {
			letzte = time.Now()
			fmt.Fprintf(logW, "  Stelle %d: %s (nach %s)\n", s, quantenSStandKurz(st), a.quantenSNach())
		}
		return fertig
	})
	dauer := time.Since(angemeldet)
	if err != nil {
		fmt.Fprintf(logW, "Stelle %d: kein Urteil nach %s\n", s, dauer.Round(time.Second))
		return "", err
	}
	fmt.Fprintf(logW, "Stelle %d: Urteil nach %s Offenlegung, %.0f Stellen/min\n", s, dauer.Round(time.Second),
		float64(len(probe))/math.Max(dauer.Minutes(), 1.0/60))
	switch {
	case nach == nil || nach["tot"] == true:
		return "eingeloest", nil // fehl+1 < quantenWFehlMax: tot ist ausgeschlossen
	case quantenSZahl(nach["fehl"]) == fehl+1:
		return "abgewiesen", nil
	case quantenSZahl(nach["fehl"]) == fehl:
		return "frist", nil
	}
	return "", fmt.Errorf("Stand nach der Vorlage unlesbar (fehl %v)", nach["fehl"])
}

// quantenSUrteilNachlauf: Zeit nach der Kettenfrist, bis die Dienste die
// Frist eingereicht haben und sie im Stand steht.
const quantenSUrteilNachlauf = 3 * time.Minute

// quantenSUrteilFrist: so lange wartet das Werkzeug auf das Urteil --
// hoechstens Kettenfrist plus Nachlauf, ausser die Frist je Schritt ist
// groesser (begrenzt auf 1 h beim Aufruf).
func quantenSUrteilFrist(schritt time.Duration) time.Duration {
	return max(schritt, time.Duration(quantenWFristOffen)*time.Second+quantenSUrteilNachlauf)
}

func quantenSZahl(v interface{}) int {
	switch z := v.(type) {
	case float64:
		if z >= 0 && z <= 1e6 && z == float64(int(z)) {
			return int(z)
		}
	case int:
		return z
	}
	return -1
}

func quantenSWartenAuf(ctx context.Context, netz quantenSNetz, K string, frist time.Duration, was string, fertig func(map[string]interface{}) bool) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, frist)
	defer cancel()
	var zuletzt map[string]interface{}
	for {
		s, err := netz.stand(ctx, K)
		if err == nil && fertig(s) {
			return s, nil
		}
		if err == nil && s != nil {
			zuletzt = s
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s: nicht in %s (%v; %s)", was, frist, err, quantenSStandKurz(zuletzt))
		}
		if err := netz.warten(ctx); err != nil {
			return nil, fmt.Errorf("%s: nicht in %s (%s)", was, frist, quantenSStandKurz(zuletzt))
		}
	}
}

// quantenSStandKurz: der zuletzt gelesene Stand in einer Zeile (Fehlversuche,
// Anmeldung: offengelegt von Stichprobe) -- damit ein Abbruch sagt, WO es
// haengt, ohne die Stichprobe oder Werte auszugeben.
func quantenSStandKurz(s map[string]interface{}) string {
	if s == nil {
		return "kein Stand gelesen"
	}
	t := fmt.Sprintf("fehl %v", s["fehl"])
	a, ok := s["anmeldung"].(map[string]interface{})
	if !ok {
		return t + ", keine Anmeldung"
	}
	probe, _ := a["probe"].([]interface{})
	return fmt.Sprintf("%s, Anmeldung von Stelle %v: offen %v von %d", t, a["b"], a["offen"], len(probe))
}

// ------------------------------------------------------------ HTTP

// QuantenSAPIBasis: nur http://127.0.0.1|localhost|[::1]:port oder https,
// ohne Benutzerangabe, Pfad, Abfrage oder Fragment -- die Basis bekommt
// unterschriebene Auftraege, und eine Benutzerangabe (http://x@host) ist der
// klassische Weg, einen Leser ueber das Ziel zu taeuschen.
func QuantenSAPIBasis(roh string) (string, error) {
	u, err := url.Parse(roh)
	if err != nil || u.User != nil || u.Host == "" || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("API: nur Schema, Host und Port, ohne Benutzerangabe")
	}
	switch u.Scheme {
	case "https":
	case "http":
		h := u.Hostname()
		ip := net.ParseIP(h)
		if h != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return "", errors.New("API: http nur an den eigenen Rechner")
		}
	default:
		return "", errors.New("API: Schema http oder https")
	}
	return u.Scheme + "://" + u.Host, nil
}

type quantenSHTTP struct {
	basis  string
	klient *http.Client
	takt   time.Duration
	// max: groesste Antwort; 0 = quantenSAusgabeAntwortMax. Nur die Antwort
	// auf einen Abschnitts-Auftrag (Buendel fuer alle Validatoren) darf
	// groesser sein (quantenSAbschnittAntwortMax).
	max int
}

func neuerQuantenSHTTP(basis string) (*quantenSHTTP, error) {
	b, err := QuantenSAPIBasis(basis)
	if err != nil {
		return nil, err
	}
	return &quantenSHTTP{basis: b, takt: 2 * time.Second, klient: &http.Client{Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// anfrage: bei 429/503 und Netzfehlern bis zu quantenSAusgabeVersuche Mal,
// mit wachsender Pause (hoechstens 30 s); jede andere Antwort ist endgueltig.
func (h *quantenSHTTP) anfrage(ctx context.Context, methode, pfad string, koerper []byte) (int, []byte, error) {
	grenze := quantenSAusgabeAntwortMax
	if h.max > 0 {
		grenze = h.max
	}
	pause := h.takt
	var letzter error
	for i := 0; i < quantenSAusgabeVersuche; i++ {
		var rd io.Reader
		if koerper != nil {
			rd = bytes.NewReader(koerper)
		}
		req, err := http.NewRequestWithContext(ctx, methode, h.basis+pfad, rd)
		if err != nil {
			return 0, nil, err
		}
		if koerper != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := h.klient.Do(req)
		if err == nil {
			body, rerr := io.ReadAll(io.LimitReader(resp.Body, int64(grenze)+1))
			resp.Body.Close()
			switch {
			case rerr != nil:
				err = rerr
			case len(body) > grenze:
				return 0, nil, errors.New("Antwort zu gross")
			case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable:
				err = fmt.Errorf("HTTP %d", resp.StatusCode)
			default:
				return resp.StatusCode, body, nil
			}
		}
		letzter = err
		select {
		case <-ctx.Done():
			return 0, nil, ctx.Err()
		case <-time.After(pause):
		}
		pause = min(2*pause, 30*time.Second)
	}
	return 0, nil, fmt.Errorf("%s %s: aufgegeben nach %d Versuchen: %v", methode, pfad, quantenSAusgabeVersuche, letzter)
}

func quantenSFehlertext(body []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error != "" {
		return e.Error
	}
	if len(body) > 200 {
		body = body[:200]
	}
	return strings.TrimSpace(string(body))
}

func (h *quantenSHTTP) bestaetigen(ctx context.Context, req QuantenWBestaetigenAnfrage) (string, error) {
	roh, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	code, body, err := h.anfrage(ctx, http.MethodPost, "/api/quanten-w/validator/bestaetigen", roh)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", code, quantenSFehlertext(body))
	}
	var a struct {
		Bestaetigung string `json:"bestaetigung"`
	}
	if err := json.Unmarshal(body, &a); err != nil {
		return "", errors.New("Antwort ist kein JSON")
	}
	return a.Bestaetigung, nil
}

func (h *quantenSHTTP) einreichen(ctx context.Context, art, wallet string, zeit int64, q QuantenWAngaben) error {
	roh, err := json.Marshal(map[string]interface{}{"wallet": wallet, "zeit": zeit, "quanten_w": q})
	if err != nil {
		return err
	}
	code, body, err := h.anfrage(ctx, http.MethodPost, "/api/quanten-w/"+strings.TrimPrefix(art, "quanten_w_"), roh)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", code, quantenSFehlertext(body))
	}
	return nil
}

func (h *quantenSHTTP) stand(ctx context.Context, K string) (map[string]interface{}, error) {
	code, body, err := h.anfrage(ctx, http.MethodGet, "/api/quanten-w/note?k="+url.QueryEscape(K), nil)
	if err != nil {
		return nil, err
	}
	if code == http.StatusNotFound {
		return nil, nil
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", code, quantenSFehlertext(body))
	}
	var s map[string]interface{}
	if err := json.Unmarshal(body, &s); err != nil || s == nil || s["k"] != K {
		return nil, errors.New("Stand: kein JSON oder fremdes K")
	}
	return s, nil
}

func (h *quantenSHTTP) warten(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(h.takt):
		return nil
	}
}

// QuantenSValidator: ein Validator der Ausgabe -- Signieradresse und die
// API seines Knotens (dort liegt sein Tresor, dort bestaetigt er).
type QuantenSValidator struct {
	Adresse string
	API     string
}

// quantenSMehrNetz: Einreichen und Stand beim eigenen Knoten, die
// Bestaetigung bei jedem Validator an seiner eigenen API. Die Bestaetigung
// wird im Ablauf gegen die Adresse des Validators geprueft -- eine falsche
// API kann also nur verweigern, nicht faelschen.
type quantenSMehrNetz struct {
	*quantenSHTTP
	validatoren []*quantenSHTTP
}

func (m *quantenSMehrNetz) bestaetigen(ctx context.Context, req QuantenWBestaetigenAnfrage) (string, error) {
	if req.X < 1 || req.X > len(m.validatoren) {
		return "", fmt.Errorf("Validator %d unbekannt", req.X)
	}
	return m.validatoren[req.X-1].bestaetigen(ctx, req)
}

// quantenSValidatorenPruefen: Adressen kanonisch und verschieden, APIs wie
// die eigene (QuantenSAPIBasis), 1 <= t <= v <= quantenWVMax.
func quantenSValidatorenPruefen(vs []QuantenSValidator, t int) ([]string, []*quantenSHTTP, error) {
	if len(vs) == 0 || len(vs) > quantenWVMax || t < 1 || t > len(vs) || t > quantenWTMax {
		return nil, nil, fmt.Errorf("Validatoren: %d, Schwelle %d", len(vs), t)
	}
	gesehen := map[string]bool{}
	var adressen []string
	var netze []*quantenSHTTP
	for i, v := range vs {
		a := strings.ToLower(strings.TrimSpace(v.Adresse))
		if !kanonischeAdresse(a) || gesehen[a] {
			return nil, nil, fmt.Errorf("Validator %d: Adresse nicht kanonisch oder doppelt", i+1)
		}
		gesehen[a] = true
		h, err := neuerQuantenSHTTP(v.API)
		if err != nil {
			return nil, nil, fmt.Errorf("Validator %d: %v", i+1, err)
		}
		adressen, netze = append(adressen, a), append(netze, h)
	}
	return adressen, netze, nil
}

// QuantenSValidatorenLesen: "0xadr=https://knoten,0xadr2=https://knoten2"
// (hoechstens quantenWVMax Eintraege).
func QuantenSValidatorenLesen(roh string) ([]QuantenSValidator, error) {
	roh = strings.TrimSpace(roh)
	if roh == "" {
		return nil, nil
	}
	teile := strings.Split(roh, ",")
	if len(teile) > quantenWVMax {
		return nil, fmt.Errorf("Validatoren: hoechstens %d", quantenWVMax)
	}
	var out []QuantenSValidator
	for _, t := range teile {
		a, u, ok := strings.Cut(strings.TrimSpace(t), "=")
		if !ok {
			return nil, errors.New("Validatoren: Form adresse=api")
		}
		out = append(out, QuantenSValidator{Adresse: a, API: u})
	}
	return out, nil
}

// QuantenSAusgabe: `aequitasd quanten-s-ausgabe`. Der Schluessel ist der des
// Knotens (RELAYER_PRIVATE_KEY), die Messung kommt auf stdin. Ohne
// Validatorenliste ist der Knoten selbst der einzige Validator (t = 1).
func QuantenSAusgabe(ctx context.Context, apiBasis, schluesselHex string, deckung int64, frist time.Duration, validatoren []QuantenSValidator, t, tol int,
	messenWarten time.Duration, probeV2 bool, nachweisWarten time.Duration, mess io.Reader, aus, log io.Writer) error {
	if quantenWKNote(tol, 1, 1) == 0 {
		return fmt.Errorf("Toleranz %d %%: die Kette kennt nur %v", tol, quantenSToleranzen())
	}
	if messenWarten < 0 || messenWarten > quantenSMessenWartenMax {
		return fmt.Errorf("Messen abwarten: 0 bis %s", quantenSMessenWartenMax)
	}
	if nachweisWarten < 0 || nachweisWarten > time.Hour || (nachweisWarten > 0 && !probeV2) {
		return errors.New("Nachweis abwarten: 0 bis 1 h, nur mit Stichprobe v2")
	}
	eigen, err := neuerQuantenSHTTP(apiBasis)
	if err != nil {
		return err
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(strings.TrimSpace(schluesselHex), "0x"))
	if err != nil {
		return errors.New("Knotenschluessel unlesbar") // ohne den Schluessel im Text
	}
	if len(validatoren) == 0 {
		validatoren, t = []QuantenSValidator{{Adresse: quantenSAdresse(key), API: apiBasis}}, 1
	}
	adressen, netze, err := quantenSValidatorenPruefen(validatoren, t)
	if err != nil {
		return err
	}
	roh, err := io.ReadAll(io.LimitReader(mess, quantenSAusgabeMessMax+1))
	if err != nil || len(roh) > quantenSAusgabeMessMax {
		return errors.New("Messung: unlesbar oder zu gross")
	}
	if frist <= 0 || frist > time.Hour {
		return errors.New("Frist: 1 s bis 1 h")
	}
	auftrag := QuantenSAusgabeAuftrag{Schluessel: key, Deckung: deckung, Tol: tol, Frist: frist}
	netz := &quantenSMehrNetz{quantenSHTTP: eigen, validatoren: netze}
	// Abschnitts-Modus (quanten_s_ausgabe_abschnitte.go): die Eingabe nennt
	// nur die Basen der Inhaberin je Abschnitt ("basen"), keine Messung.
	var art map[string]json.RawMessage
	_ = json.Unmarshal(roh, &art)
	var erg *QuantenSAusgabeErgebnis
	if _, abschnitte := art["basen"]; abschnitte {
		var bs QuantenSBasen
		dec := json.NewDecoder(bytes.NewReader(roh))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&bs); err != nil {
			return fmt.Errorf("Basen: %v", err)
		}
		// Stichprobe v2 nur, wenn die Kette sie kennt -- vor jedem Auftrag
		// gefragt, sonst verbraucht eine Ausgabe, die die Kette abweist, die
		// Messungen der Validatoren.
		if probeV2 {
			if err := quantenSProbeV2AufKette(ctx, eigen, time.Now().Unix()); err != nil {
				return err
			}
			auftrag.ProbeV2 = true
		}
		// Mit --messen-warten: die Basen bei jedem Validator vormerken und
		// warten, bis er gemessen hat (quanten_s_vormerken.go). Ohne Basen in
		// der Eingabe zieht das Werkzeug sie selbst (es spielt die Inhaberin).
		if messenWarten > 0 {
			if len(bs.Basen) == 0 {
				if bs.Basen, err = quantenSBasenZiehen(bs.N, len(adressen)); err != nil {
					return err
				}
				bs.Wo = "je Validator gemessen (vorgemerkt)"
			}
			if err := quantenSMessungenAbwarten(ctx, key, bs, adressen, netz, messenWarten, quantenSVormerkTakt,
				func() int64 { return time.Now().Unix() }, log); err != nil {
				return err
			}
		}
		erg, err = quantenSAusgabeLaufAbschnitte(ctx, auftrag, bs, adressen, t, netz, log)
	} else {
		if probeV2 {
			return errors.New("Stichprobe v2 nur im Abschnitts-Modus (Eingabe mit \"basen\")")
		}
		var m QuantenSMessung
		dec := json.NewDecoder(bytes.NewReader(roh))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&m); err != nil {
			return fmt.Errorf("Messung: %v", err)
		}
		auftrag.Messung = m
		erg, err = quantenSAusgabeLauf(ctx, auftrag, adressen, t, netz, log)
	}
	if err == nil && erg != nil && nachweisWarten > 0 {
		// Nach dem Ende (eingeloest): der Nachweis aller Validatoren gegen
		// Wurzel und Proben der Ausgabe. Ein Fehler hier macht das Einloesen
		// nicht ungeschehen (ErrQuantenSNachweis).
		var nerr error
		if erg.Nachweis, nerr = quantenSNachweisAbwarten(ctx, netz, erg.K, adressen, erg.Wurzel, erg.Proben, time.Now().Add(nachweisWarten), log); nerr != nil {
			erg.NachweisFehler = nerr.Error()
			err = fmt.Errorf("%w: %v", ErrQuantenSNachweis, nerr)
		}
	}
	if erg != nil {
		enc := json.NewEncoder(aus)
		enc.SetIndent("", "  ")
		enc.Encode(erg)
	}
	return err
}

// quantenSProbeV2AufKette: kennt die Kette des eigenen Knotens die Stichprobe
// v2 schon (/api/status: quanten_w.probe_v2_ab <= jetzt)? Fehlt das Feld,
// schlaeft sie (Stichtag nicht gesetzt).
func quantenSProbeV2AufKette(ctx context.Context, h *quantenSHTTP, jetzt int64) error {
	code, body, err := h.anfrage(ctx, http.MethodGet, "/api/status", nil)
	if err != nil {
		return fmt.Errorf("Stichprobe v2: Status des Knotens: %w", err)
	}
	if code != http.StatusOK {
		return fmt.Errorf("Stichprobe v2: Status des Knotens: HTTP %d", code)
	}
	var st struct {
		QuantenW struct {
			ProbeV2Ab *int64 `json:"probe_v2_ab"`
		} `json:"quanten_w"`
	}
	if err := json.Unmarshal(body, &st); err != nil {
		return errors.New("Stichprobe v2: Status des Knotens ist kein JSON")
	}
	if st.QuantenW.ProbeV2Ab == nil {
		return errors.New("Stichprobe v2 ist auf der Kette nicht aktiv (Stichtag nicht gesetzt) -- nichts beauftragt")
	}
	if *st.QuantenW.ProbeV2Ab > jetzt {
		return fmt.Errorf("Stichprobe v2 gilt erst ab %d (jetzt %d) -- nichts beauftragt", *st.QuantenW.ProbeV2Ab, jetzt)
	}
	return nil
}
