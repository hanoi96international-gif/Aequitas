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
	quantenSAusgabeMessMax    = 1 << 20 // Messdatei
	quantenSAusgabeVersuche   = 8       // je Anfrage bei 429/503/Netzfehler
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
}

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
	q := QuantenWAngaben{K: K, N: m.N, T: t, Wurzel: wurzel, Validatoren: validatoren, Bestaetigungen: bestaetigungen,
		Deckung: a.Deckung, Tol: a.Tol, Inhaber: inhaber, Annahme: stellen[:], Proben: probenHex}
	if q.WalletSig, err = quantenSUnterschreiben(a.Schluessel, quantenWSAusgabeNachricht(ausgeber, K, m.N, t, wurzel, validatoren, probenHex, q.Annahme, inhaber, a.Deckung, zeit, a.Tol)); err != nil {
		return nil, err
	}
	if err := netz.einreichen(ctx, quantenWSAusgebenTxArt, ausgeber, zeit, q); err != nil {
		return nil, fmt.Errorf("Ausgabe: %w", err)
	}
	if _, err := quantenSWartenAuf(ctx, netz, K, a.Frist, "ausgegeben", func(s map[string]interface{}) bool { return s != nil && s["art"] == "s" }); err != nil {
		return nil, err
	}
	fmt.Fprintln(log, "ausgegeben")

	// 3. Bindung und Wahl der Inhaberin.
	d := make([]int, m.N)
	for i := range y {
		d[i] = y[i] ^ z
	}
	ds := qsBitsText(d)
	sigD, err := quantenSUnterschreiben(inhaberKey, quantenWSBindenNachricht(inhaber, K, ds))
	if err != nil {
		return nil, err
	}
	if err := netz.einreichen(ctx, quantenWSBindenTxArt, inhaber, 0, QuantenWAngaben{K: K, D: ds, WalletSig: sigD}); err != nil {
		return nil, fmt.Errorf("Bindung: %w", err)
	}
	if _, err := quantenSWartenAuf(ctx, netz, K, a.Frist, "gebunden", func(s map[string]interface{}) bool { return s != nil && s["gebunden"] == true }); err != nil {
		return nil, err
	}
	cs := string(rune('0' + (b ^ z)))
	sigC, err := quantenSUnterschreiben(inhaberKey, quantenWSWahlNachricht(inhaber, K, cs))
	if err != nil {
		return nil, err
	}
	if err := netz.einreichen(ctx, quantenWSWaehlenTxArt, inhaber, 0, QuantenWAngaben{K: K, C: cs, WalletSig: sigC}); err != nil {
		return nil, fmt.Errorf("Wahl: %w", err)
	}
	if _, err := quantenSWartenAuf(ctx, netz, K, a.Frist, "gewaehlt", func(s map[string]interface{}) bool { return s != nil && s["gewaehlt"] == true }); err != nil {
		return nil, err
	}
	fmt.Fprintln(log, "gebunden und gewaehlt")

	erg := &QuantenSAusgabeErgebnis{K: K, N: m.N, KJeStelle: k, Ausgeber: ausgeber, Inhaberin: inhaber, Stellen: stellen[:], B: b,
		Deckung: a.Deckung, FehlerrateMess: fmt.Sprintf("%d/%d", falsch, passend), Wo: m.Wo, Job: m.Jo}

	// 4. Doppelvorlage an der anderen Stelle, 5. ehrlich an Stelle b.
	for i, s := range []int{1 - b, b} {
		urteil, err := quantenSVorlegen(ctx, netz, a, K, s, stellen[s], stellenKey[s], inhaberKey, x, probe[s], i)
		if err != nil {
			return erg, err
		}
		if i == 0 {
			erg.Doppelvorlage = urteil
			fmt.Fprintln(log, "Doppelvorlage:", urteil)
			if urteil != "abgewiesen" {
				return erg, fmt.Errorf("BEFUND: Doppelvorlage an Stelle %d nicht abgewiesen (%s)", s, urteil)
			}
		} else {
			erg.Vorlage = urteil
			fmt.Fprintln(log, "ehrliche Vorlage:", urteil)
			if urteil != "eingeloest" {
				return erg, fmt.Errorf("ehrliche Vorlage an Stelle %d nicht eingeloest (%s)", s, urteil)
			}
		}
	}
	return erg, nil
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
func quantenSVorlegen(ctx context.Context, netz quantenSNetz, a QuantenSAusgabeAuftrag, K string, s int, stelle string, stelleKey, inhaberKey *ecdsa.PrivateKey, x, probe []int, fehl int) (string, error) {
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
	vorlage, err := quantenSUnterschreiben(inhaberKey, quantenSVorlageText(K, s, x))
	if err != nil {
		return "", err
	}
	bs := string(rune('0' + s))
	q := QuantenWAngaben{K: K, B: bs, X: xs, Probe: probe, Vorlage: vorlage}
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
	nach, err := quantenSWartenAuf(ctx, netz, K, a.Frist, "Urteil", func(st map[string]interface{}) bool {
		return st == nil || st["tot"] == true || st["anmeldung"] == nil
	})
	if err != nil {
		return "", err
	}
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
	for {
		s, err := netz.stand(ctx, K)
		if err == nil && fertig(s) {
			return s, nil
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s: nicht in %s (%v)", was, frist, err)
		}
		if err := netz.warten(ctx); err != nil {
			return nil, fmt.Errorf("%s: nicht in %s", was, frist)
		}
	}
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
			body, rerr := io.ReadAll(io.LimitReader(resp.Body, quantenSAusgabeAntwortMax+1))
			resp.Body.Close()
			switch {
			case rerr != nil:
				err = rerr
			case len(body) > quantenSAusgabeAntwortMax:
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

// QuantenSAusgabe: `aequitasd quanten-s-ausgabe`. Der Schluessel ist der des
// Knotens (RELAYER_PRIVATE_KEY), die Messung kommt auf stdin.
func QuantenSAusgabe(ctx context.Context, apiBasis, schluesselHex string, deckung int64, frist time.Duration, mess io.Reader, aus, log io.Writer) error {
	netz, err := neuerQuantenSHTTP(apiBasis)
	if err != nil {
		return err
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(strings.TrimSpace(schluesselHex), "0x"))
	if err != nil {
		return errors.New("Knotenschluessel unlesbar") // ohne den Schluessel im Text
	}
	roh, err := io.ReadAll(io.LimitReader(mess, quantenSAusgabeMessMax+1))
	if err != nil || len(roh) > quantenSAusgabeMessMax {
		return errors.New("Messung: unlesbar oder zu gross")
	}
	var m QuantenSMessung
	dec := json.NewDecoder(bytes.NewReader(roh))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return fmt.Errorf("Messung: %v", err)
	}
	if frist <= 0 || frist > time.Hour {
		return errors.New("Frist: 1 s bis 1 h")
	}
	erg, err := quantenSAusgabeLauf(ctx, QuantenSAusgabeAuftrag{Messung: m, Schluessel: key, Deckung: deckung, Tol: quantenWTolStandard, Frist: frist},
		[]string{quantenSAdresse(key)}, 1, netz, log)
	if erg != nil {
		enc := json.NewEncoder(aus)
		enc.SetIndent("", "  ")
		enc.Encode(erg)
	}
	return err
}
