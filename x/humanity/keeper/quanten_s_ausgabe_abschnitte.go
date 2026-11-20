package keeper

// `aequitasd quanten-s-ausgabe` IM ABSCHNITTS-MODUS (Weg A, Stufe A3;
// forschung/quantengeld/DEZENTRAL.md 8-11).
//
// Statt EINER Messung aller Stellen bekommt das Werkzeug nur die Messbasen
// der Inhaberin je Abschnitt (QuantenSBasen). Jeder Validator praepariert
// seinen Abschnitt aus seiner eigenen Messquelle (quanten_s_abschnitt_dienst.go)
// -- das Werkzeug sieht nie Basen u und Werte t der Ausgabe. Es spielt
// Ausgeber, Inhaberin (Wegwerfschluessel) und beide Annahmestellen (der
// Knotenschluessel und ein Wegwerfschluessel), wie der Weg mit einem Auftrag:
//
//  1. Schluessel der Validatoren holen, jeden gegen seine Adresse pruefen.
//  2. Je Validator den Abschnitts-Auftrag; seine Antwort pruefen (von ihm
//     unterschrieben, Abschnitt, Buendel), x der Inhaberin entschluesseln.
//  3. Wurzel aus allen Blaettern; jede Annahmestelle laedt ihre Stichprobe aus
//     den Oeffnungen und prueft sie selbst (Wurzel, Blatt, Stellen-Bit, k_note).
//  4. Je Validator die Bestaetigung mit den Buendeln der anderen.
//  5. Wie der Weg mit einem Auftrag: Ausgabe, Bindung, Wahl, Doppelvorlage,
//     ehrliche Vorlage (quantenSAusgabeAbschluss).
//
// Die Fehlerrate der Hardware rechnet das Werkzeug aus den Stichproben der
// Stellen (sie kennen u und t auf ihrer Haelfte), nicht aus einer Messung.

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
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// QuantenSBasen: die Messbasen der Inhaberin je Abschnitt
// (hardware_ibm.py --s-abschnitte schreibt sie neben die Messdateien der
// Validatoren; das Werkzeug bekommt NUR diese Datei).
type QuantenSBasen struct {
	N     int      `json:"n"`
	Basen []string `json:"basen"`
	Wo    string   `json:"wo"`
	Job   string   `json:"job"`
}

// quantenSAbschnittNetz: dazu der Weg zu jedem Validator (1-basiert).
type quantenSAbschnittNetz interface {
	quantenSNetz
	schluessel(ctx context.Context, x int) (string, error)
	abschnitt(ctx context.Context, x int, req QuantenSAbschnittAnfrage) (QuantenSAbschnittAntwort, error)
	oeffnung(ctx context.Context, x int, K string, b int) (string, error)
	// fuerInhaberin: der Abschnitt von Validator x fuer Inhaberin und
	// Annahmestelle (getrennte Rollen, quanten_s_rollen.go).
	fuerInhaberin(ctx context.Context, x int, K string) (QuantenSPaketAbschnitt, error)
}

func quantenSAusgabeLaufAbschnitte(ctx context.Context, a QuantenSAusgabeAuftrag, basen QuantenSBasen, validatoren []string, t int, netz quantenSAbschnittNetz, log io.Writer) (*QuantenSAusgabeErgebnis, error) {
	a.log, a.beginn = log, time.Now()
	r, err := quantenSRundeBeginnen(a, basen, validatoren, t)
	if err != nil {
		return nil, err
	}
	// 1. Schluessel der Validatoren, jeder gegen seine Adresse.
	if err := r.schluesselHolen(ctx, netz); err != nil {
		return nil, err
	}
	K, n, v, y, kn := r.K, r.n, r.v, r.y, r.kn
	ausgeber := r.ausgeber
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
	var stellen [2]string
	var stellenKey [2]*ecdsa.PrivateKey
	stellen[b], stellenKey[b] = ausgeber, a.Schluessel
	stellen[1-b], stellenKey[1-b] = quantenSAdresse(fremdKey), fremdKey
	// 2. Je Validator sein Abschnitt; dann x der Inhaberin (Wegwerfschluessel).
	if err := r.abschnitteHolen(ctx, a, basen, netz, inhaber, quantenSPubHex(inhaberKey), stellen,
		[2]string{quantenSPubHex(stellenKey[0]), quantenSPubHex(stellenKey[1])}); err != nil {
		return nil, err
	}
	antw, blaetter, wurzel, zeit := r.antw, r.blaetter, r.wurzel, r.zeit
	x := make([]int, n)
	for xi := 1; xi <= v; xi++ {
		von, bis, _ := quantenSAbschnitt(n, v, xi)
		xs, err := quantenSFuerInhaberinOeffnen(inhaberKey, antw[xi-1].FuerInhaberin, K, von, bis)
		if err != nil {
			return nil, fmt.Errorf("Validator %d: %v", xi, err)
		}
		copy(x[von:], xs)
	}
	// 3. Die Annahmestellen laden ihre Stichproben selbst -- oder, bei der
	// Stichprobe v2, erst nach der Bindung (dann hier nur Teil-Hashes und
	// Zaehlungen der Validatoren; niemand kennt eine Stelle).
	var probe [2][]int
	var proben [2][32]byte
	var probenHex []string
	passend, falsch := 0, 0
	var treffer, fehler [2]int
	var probenLaden func(ctx context.Context) ([2][]int, [2][]string, error)
	if a.ProbeV2 {
		if proben, probenHex, treffer, fehler, err = r.probenV2(); err != nil {
			return nil, err
		}
		for s := 0; s < 2; s++ {
			passend, falsch = passend+treffer[s], falsch+fehler[s]
		}
		probenLaden = func(ctx context.Context) ([2][]int, [2][]string, error) {
			return quantenSStichprobenV2Laden(ctx, a, netz, K, n, v, wurzel, blaetter, kn, stellenKey, proben, antw, y, x)
		}
	} else {
		for s := 0; s < 2; s++ {
			oe := map[int]*big.Int{}
			for xi := 1; xi <= v; xi++ {
				von, bis, _ := quantenSAbschnitt(n, v, xi)
				c, err := netz.oeffnung(ctx, xi, K, s)
				if err != nil {
					return nil, fmt.Errorf("Validator %d: Oeffnung fuer Stelle %d: %w", xi, s, err)
				}
				m, err := quantenSOeffnungEntschluesseln(stellenKey[s], c, K, s, xi, von, bis)
				if err != nil {
					return nil, fmt.Errorf("Validator %d: Oeffnung fuer Stelle %d: %v", xi, s, err)
				}
				for i, w := range m {
					oe[i] = w
				}
			}
			geladen, err := quantenSOeffnungLaden(s, K, n, wurzel, blaetter, oe, kn)
			if err != nil {
				return nil, fmt.Errorf("Stelle %d laedt ihre Stichprobe nicht: %v", s, err)
			}
			for i, ut := range geladen {
				probe[s] = append(probe[s], i)
				if y[i] == ut[0] {
					passend++
					treffer[s]++
					if x[i] != ut[1] {
						falsch++
						fehler[s]++
					}
				}
			}
			sort.Ints(probe[s])
			proben[s] = quantenSProbeHash(K, s, probe[s])
			probenHex = append(probenHex, hex.EncodeToString(proben[s][:]))
		}
	}
	wurzelHex := hex.EncodeToString(wurzel[:])
	if a.ProbeV2 {
		fmt.Fprintf(log, "Token %s: n=%d in %d Abschnitten, Stichprobe v2 (erst nach der Bindung), Fehlerrate bei passender Basis %d/%d (aus den Zaehlungen der Validatoren)\n",
			K[:12], n, v, falsch, passend)
	} else {
		fmt.Fprintf(log, "Token %s: n=%d in %d Abschnitten, %d/%d Stellen je Annahmestelle, Fehlerrate bei passender Basis %d/%d (aus den Stichproben)\n",
			K[:12], n, v, len(probe[0]), len(probe[1]), falsch, passend)
	}
	// Die ehrliche Vorlage muss an BEIDEN Stellen bestehen -- exakt wie die
	// Kette zaehlt, vor der Bestaetigung und vor der Kette (quanten_s_toleranz.go).
	if err := quantenSEhrlichBesteht(treffer, fehler, a.Tol); err != nil {
		return nil, err
	}
	// 4. Bestaetigungen mit den Buendeln der anderen.
	bestaetigungen, err := r.bestaetigungenHolen(ctx, a, netz, proben, probenHex)
	if err != nil {
		return nil, err
	}
	return quantenSAusgabeAbschluss(ctx, a, netz, log, quantenSVorbereitet{K: K, N: n, T: t, KJeStelle: min(len(probe[0]), len(probe[1])),
		Validatoren: validatoren, Bestaetigungen: bestaetigungen, Wurzel: wurzelHex, ProbenHex: probenHex, Probe: probe, Stellen: stellen,
		StellenKey: stellenKey, Ausgeber: ausgeber, Inhaber: inhaber, InhaberKey: inhaberKey, B: b, Z: z, Y: y, X: x, Zeit: zeit,
		Fehlerrate: fmt.Sprintf("%d/%d", falsch, passend), Wo: basen.Wo, Job: basen.Job, ProbeArt: probeArt(a.ProbeV2), ProbenLaden: probenLaden})
}

// quantenSPubHex: oeffentlicher Schluessel, unkomprimiert, hex ohne 0x.
func quantenSPubHex(k *ecdsa.PrivateKey) string {
	return hex.EncodeToString(crypto.FromECDSAPub(&k.PublicKey))
}

// quantenSRunde: die Schritte 1, 2 und 4 einer Ausgabe im Abschnitts-Modus,
// die nichts von x wissen muessen (x kann nur die Inhaberin lesen). Beide
// Wege benutzen sie: das Werkzeug, das alle Rollen spielt (oben), und der
// Ausgeber bei getrennten Rollen (quanten_s_rollen.go).
type quantenSRunde struct {
	K           string
	n, v, t, kn int
	y           []int
	zeit        int64
	ausgeber    string
	validatoren []string
	schl        []string
	antw        []QuantenSAbschnittAntwort
	blaetter    [][32]byte
	wurzel      [32]byte
}

// quantenSRundeBeginnen: Basen je Abschnitt, Pruefgroesse und Deckung --
// ohne Netz, damit nichts Falsches einen Validator erreicht.
func quantenSRundeBeginnen(a QuantenSAusgabeAuftrag, basen QuantenSBasen, validatoren []string, t int) (*quantenSRunde, error) {
	v, n := len(validatoren), basen.N
	if v < 1 || len(basen.Basen) != v || n < 2 || n > quantenWNMax {
		return nil, fmt.Errorf("Basen: %d Abschnitte fuer %d Validatoren, n = %d", len(basen.Basen), v, n)
	}
	y := make([]int, 0, n)
	klein := 0
	for x := 1; x <= v; x++ {
		von, bis, ok := quantenSAbschnitt(n, v, x)
		if !ok {
			return nil, fmt.Errorf("Abschnitte: n = %d, v = %d", n, v)
		}
		b, err := quantenSBitsText(basen.Basen[x-1], bis-von)
		if err != nil {
			return nil, fmt.Errorf("Basen, Abschnitt %d: %v", x, err)
		}
		y = append(y, b...)
		klein += (bis - von) / 2
	}
	kn := quantenWKNote(a.Tol, t, v)
	if kn == 0 || klein < kn {
		return nil, fmt.Errorf("Pruefgroesse: %d Stellen je Annahmestelle, sicher erst ab %d (Toleranz %d %%, t=%d, v=%d)", klein, kn, a.Tol, t, v)
	}
	if a.Deckung < quantenMindestDeckungMik {
		return nil, fmt.Errorf("Deckung %d, mindestens %d Mikro-AEQ", a.Deckung, quantenMindestDeckungMik)
	}
	return &quantenSRunde{n: n, v: v, t: t, kn: kn, y: y, validatoren: validatoren, ausgeber: quantenSAdresse(a.Schluessel)}, nil
}

// schluesselHolen: Schritt 1 -- jeder Validator nennt seinen Schluessel, er
// muss zu seiner Adresse gehoeren. Dazu ein neues K.
func (r *quantenSRunde) schluesselHolen(ctx context.Context, netz quantenSAbschnittNetz) error {
	r.schl = make([]string, r.v)
	for x := 1; x <= r.v; x++ {
		s, err := netz.schluessel(ctx, x)
		if err != nil {
			return fmt.Errorf("Validator %d: Schluessel: %w", x, err)
		}
		if _, err := quantenSSchluesselPruefen(s, r.validatoren[x-1]); err != nil {
			return fmt.Errorf("Validator %d: %v", x, err)
		}
		r.schl[x-1] = strings.TrimPrefix(strings.ToLower(s), "0x")
	}
	var kb [32]byte
	if _, err := rand.Read(kb[:]); err != nil {
		return err
	}
	r.K = hex.EncodeToString(kb[:])
	return nil
}

// abschnitteHolen: Schritt 2 -- je Validator der Abschnitts-Auftrag (vom
// Ausgeber unterschrieben) und seine Antwort, geprueft (von ihm unterschrieben,
// Abschnitt, Buendel, bei v2 Teil-Hashes und Zaehlung); Blaetter und Wurzel.
// x der Inhaberin bleibt verschluesselt (FuerInhaberin).
func (r *quantenSRunde) abschnitteHolen(ctx context.Context, a QuantenSAusgabeAuftrag, basen QuantenSBasen, netz quantenSAbschnittNetz,
	inhaber, inhaberPub string, stellen, stellenPub [2]string) error {
	r.zeit = time.Now().Unix()
	if a.Jetzt != nil {
		r.zeit = a.Jetzt()
	}
	r.antw = make([]QuantenSAbschnittAntwort, r.v)
	r.blaetter = make([][32]byte, r.n)
	for xi := 1; xi <= r.v; xi++ {
		von, bis, _ := quantenSAbschnitt(r.n, r.v, xi)
		req := QuantenSAbschnittAnfrage{K: r.K, N: r.n, T: r.t, Tol: a.Tol, X: xi, Validatoren: r.validatoren, Schluessel: r.schl,
			Basen: basen.Basen[xi-1], Inhaberin: inhaber, InhaberinSchluessel: inhaberPub, Stellen: stellen[:],
			StellenSchluessel: stellenPub[:], Zeit: r.zeit, Praeger: r.ausgeber}
		if a.ProbeV2 {
			req.ProbeArt = 2
		}
		var err error
		if req.PraegerSig, err = quantenSUnterschreiben(a.Schluessel, quantenSAbschnittAuftragNachricht(req)); err != nil {
			return err
		}
		ans, err := netz.abschnitt(ctx, xi, req)
		if err != nil {
			return fmt.Errorf("Validator %d praepariert nicht: %w", xi, err)
		}
		if ans.K != r.K || ans.X != xi || ans.Von != von || ans.Bis != bis || len(ans.Blaetter) != bis-von ||
			len(ans.Buendel) != r.v || len(ans.BuendelSig) != r.v || ans.Buendel[xi-1] != "" ||
			!kanonischeSignatur(ans.Sig) || verifyPersonalSign(quantenSAbschnittAntwortNachricht(ans), ans.Sig, r.validatoren[xi-1]) != nil {
			return fmt.Errorf("Validator %d: Antwort passt nicht oder ist nicht von ihm", xi)
		}
		if a.ProbeV2 {
			if err := quantenSAntwortV2Pruefen(ans, bis-von); err != nil {
				return fmt.Errorf("Validator %d: %v", xi, err)
			}
		} else if ans.Teile != nil || ans.Zaehlung != nil {
			return fmt.Errorf("Validator %d: Teil-Hashes ohne Stichprobe v2", xi)
		}
		for i, h := range ans.Blaetter {
			bl, ok := quantenWHex32(h)
			if !ok {
				return fmt.Errorf("Validator %d: Blatt %d", xi, i)
			}
			r.blaetter[von+i] = bl
		}
		r.antw[xi-1] = ans
	}
	r.wurzel = quantenWMerkleWurzel(r.blaetter)
	return nil
}

// probenV2: die Bindungen der Stichproben aus den Teil-Hashes der
// Validatoren und ihre Zaehlungen je Annahmestelle (Stichprobe v2).
func (r *quantenSRunde) probenV2() (proben [2][32]byte, probenHex []string, treffer, fehler [2]int, err error) {
	for s := 0; s < 2; s++ {
		var teile [][32]byte
		for xi := 1; xi <= r.v; xi++ {
			h, _ := quantenWHex32(r.antw[xi-1].Teile[s])
			teile = append(teile, h)
			treffer[s] += r.antw[xi-1].Zaehlung[s][0]
			fehler[s] += r.antw[xi-1].Zaehlung[s][1]
		}
		if proben[s], err = quantenSProbeHashV2(r.K, s, teile); err != nil {
			return proben, nil, treffer, fehler, err
		}
		probenHex = append(probenHex, hex.EncodeToString(proben[s][:]))
	}
	return proben, probenHex, treffer, fehler, nil
}

// bestaetigungenHolen: Schritt 4 -- je Validator die Bestaetigung mit den
// Buendeln der anderen, gegen seine Adresse geprueft.
func (r *quantenSRunde) bestaetigungenHolen(ctx context.Context, a QuantenSAusgabeAuftrag, netz quantenSAbschnittNetz, proben [2][32]byte, probenHex []string) ([]string, error) {
	wurzelHex := hex.EncodeToString(r.wurzel[:])
	var bestaetigungen []string
	for xi := 1; xi <= r.v; xi++ {
		req := QuantenWBestaetigenAnfrage{K: r.K, N: r.n, T: r.t, Tol: a.Tol, X: xi, Wurzel: wurzelHex, Validatoren: r.validatoren,
			Zeit: r.zeit, Art: "s", Proben: probenHex, Praeger: r.ausgeber}
		for _, bl := range r.blaetter {
			req.Blaetter = append(req.Blaetter, hex.EncodeToString(bl[:]))
		}
		for p := 0; p < r.v; p++ {
			req.Buendel = append(req.Buendel, r.antw[p].Buendel[xi-1])
			req.BuendelSig = append(req.BuendelSig, r.antw[p].BuendelSig[xi-1])
		}
		var err error
		if req.PraegerSig, err = quantenSUnterschreiben(a.Schluessel, quantenWDienstAuftrag(req)); err != nil {
			return nil, err
		}
		sig, err := netz.bestaetigen(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("Validator %d bestaetigt nicht: %w", xi, err)
		}
		if !kanonischeSignatur(sig) || verifyPersonalSign(quantenWSBestaetigungNachricht(r.K, r.wurzel, r.n, r.t, r.validatoren, xi, proben, r.zeit, a.Tol), sig, r.validatoren[xi-1]) != nil {
			return nil, fmt.Errorf("Validator %d: Bestaetigung ist nicht von ihm", xi)
		}
		bestaetigungen = append(bestaetigungen, sig)
	}
	return bestaetigungen, nil
}

// ------------------------------------------------------------ ueber HTTP

func (m *quantenSMehrNetz) validator(x int) (*quantenSHTTP, error) {
	if x < 1 || x > len(m.validatoren) {
		return nil, fmt.Errorf("Validator %d unbekannt", x)
	}
	return m.validatoren[x-1], nil
}

func (m *quantenSMehrNetz) schluessel(ctx context.Context, x int) (string, error) {
	h, err := m.validator(x)
	if err != nil {
		return "", err
	}
	code, body, err := h.anfrage(ctx, http.MethodGet, "/api/quanten-w/validator/schluessel", nil)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", code, quantenSFehlertext(body))
	}
	var a struct {
		Schluessel string `json:"schluessel"`
	}
	if err := json.Unmarshal(body, &a); err != nil {
		return "", errors.New("Antwort ist kein JSON")
	}
	return a.Schluessel, nil
}

func (m *quantenSMehrNetz) abschnitt(ctx context.Context, x int, req QuantenSAbschnittAnfrage) (QuantenSAbschnittAntwort, error) {
	h, err := m.validator(x)
	if err != nil {
		return QuantenSAbschnittAntwort{}, err
	}
	roh, err := json.Marshal(req)
	if err != nil {
		return QuantenSAbschnittAntwort{}, err
	}
	gross := *h
	gross.max = quantenSAbschnittAntwortMax
	code, body, err := gross.anfrage(ctx, http.MethodPost, "/api/quanten-w/validator/abschnitt", roh)
	if err != nil {
		return QuantenSAbschnittAntwort{}, err
	}
	if code != http.StatusOK {
		return QuantenSAbschnittAntwort{}, fmt.Errorf("HTTP %d: %s", code, quantenSFehlertext(body))
	}
	var a QuantenSAbschnittAntwort
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return QuantenSAbschnittAntwort{}, errors.New("Antwort ist kein passendes JSON")
	}
	return a, nil
}

func (m *quantenSMehrNetz) fuerInhaberin(ctx context.Context, x int, K string) (QuantenSPaketAbschnitt, error) {
	h, err := m.validator(x)
	if err != nil {
		return QuantenSPaketAbschnitt{}, err
	}
	gross := *h
	gross.max = quantenSAbschnittAntwortMax
	code, body, err := gross.anfrage(ctx, http.MethodGet, "/api/quanten-w/validator/fuer-inhaberin?k="+url.QueryEscape(K), nil)
	if err != nil {
		return QuantenSPaketAbschnitt{}, err
	}
	if code != http.StatusOK {
		return QuantenSPaketAbschnitt{}, fmt.Errorf("HTTP %d: %s", code, quantenSFehlertext(body))
	}
	var a QuantenSPaketAbschnitt
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return QuantenSPaketAbschnitt{}, errors.New("Antwort ist kein passendes JSON")
	}
	return a, nil
}

func (m *quantenSMehrNetz) oeffnung(ctx context.Context, x int, K string, b int) (string, error) {
	h, err := m.validator(x)
	if err != nil {
		return "", err
	}
	gross := *h
	gross.max = quantenSAbschnittAntwortMax
	code, body, err := gross.anfrage(ctx, http.MethodGet, "/api/quanten-w/validator/oeffnung?k="+url.QueryEscape(K)+"&b="+strconv.Itoa(b), nil)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", code, quantenSFehlertext(body))
	}
	var a struct {
		Oeffnung string `json:"oeffnung"`
	}
	if err := json.Unmarshal(body, &a); err != nil {
		return "", errors.New("Antwort ist kein JSON")
	}
	return a.Oeffnung, nil
}

func probeArt(v2 bool) int {
	if v2 {
		return 2
	}
	return 0
}

// quantenSAntwortV2Pruefen: je Annahmestelle ein Teil-Hash und eine Zaehlung
// in moeglichen Grenzen (0 <= Fehler <= Treffer <= Stellen des Abschnitts).
func quantenSAntwortV2Pruefen(ans QuantenSAbschnittAntwort, stellen int) error {
	if len(ans.Teile) != 2 || len(ans.Zaehlung) != 2 {
		return errors.New("Stichprobe v2: Teil-Hashes oder Zaehlung fehlen")
	}
	for s := 0; s < 2; s++ {
		if _, ok := quantenWHex32(ans.Teile[s]); !ok {
			return fmt.Errorf("Stichprobe v2: Teil-Hash %d", s)
		}
		tr, fe := ans.Zaehlung[s][0], ans.Zaehlung[s][1]
		if fe < 0 || tr < fe || tr > stellen {
			return fmt.Errorf("Stichprobe v2: Zaehlung %d/%d unmoeglich", fe, tr)
		}
	}
	return nil
}

// quantenSStichprobenV2Laden: nach Bindung und Wahl laedt jede Annahmestelle
// ihre Stichprobe samt Salzen bei jedem Validator. Die Validatoren geben sie
// erst heraus, wenn d und c final sind -- bis dahin begrenzt neu gefragt
// (a.Frist). Danach prueft jede Stelle selbst: Wurzel, Blatt, Stellen-Bit,
// k_note (quantenSOeffnungLaden), die Bindung v2 aus den Salzen, und ob die
// Zaehlung jedes Validators stimmte (sonst Befund mit seinem Namen) -- alles
// vor der ersten Vorlage.
func quantenSStichprobenV2Laden(ctx context.Context, a QuantenSAusgabeAuftrag, netz quantenSAbschnittNetz, K string, n, v int, wurzel [32]byte,
	blaetter [][32]byte, kn int, stellenKey [2]*ecdsa.PrivateKey, proben [2][32]byte, antw []QuantenSAbschnittAntwort, y, x []int) ([2][]int, [2][]string, error) {
	var probe [2][]int
	var salzeHex [2][]string
	ende := time.Now().Add(a.Frist)
	for s := 0; s < 2; s++ {
		var geladen map[int][2]int
		var err error
		if probe[s], salzeHex[s], geladen, err = quantenSStichprobeV2Laden(ctx, netz, ende, K, s, n, v, wurzel, blaetter, kn, stellenKey[s], proben[s]); err != nil {
			return probe, salzeHex, err
		}
		// Zaehlung je Validator nachrechnen (Abschnitt fuer Abschnitt).
		for xi := 1; xi <= v; xi++ {
			von, bis, _ := quantenSAbschnitt(n, v, xi)
			tr, fe := 0, 0
			for i, ut := range geladen {
				if i >= von && i < bis && y[i] == ut[0] {
					tr++
					if x[i] != ut[1] {
						fe++
					}
				}
			}
			if z := antw[xi-1].Zaehlung[s]; z[0] != tr || z[1] != fe {
				return probe, salzeHex, fmt.Errorf("BEFUND: Validator %d hat fuer Stelle %d %d/%d gezaehlt, geprueft %d/%d", xi, s, z[1], z[0], fe, tr)
			}
		}
	}
	return probe, salzeHex, nil
}

// quantenSStichprobeV2Laden: Annahmestelle s laedt nach Bindung und Wahl ihre
// Stichprobe aus den Oeffnungen aller Validatoren (nachgefragt bis ende, die
// Validatoren geben sie erst nach der Finalitaet heraus), entschluesselt mit
// IHREM Schluessel und prueft selbst: Wurzel, Blatt, Stellen-Bit, k_note
// (quantenSOeffnungLaden) und Stichprobe mit Salzen gegen die Bindung probe
// der Ausgabe -- dieselbe Rechnung wie die Kette beim Anmelden. Gibt die
// sortierte Stichprobe, die Salze (hex) und je Stelle (Basis, Wert).
func quantenSStichprobeV2Laden(ctx context.Context, netz quantenSAbschnittNetz, ende time.Time, K string, s, n, v int, wurzel [32]byte,
	blaetter [][32]byte, kn int, key *ecdsa.PrivateKey, probeBindung [32]byte) ([]int, []string, map[int][2]int, error) {
	oe := map[int]*big.Int{}
	var salze [][32]byte
	var salzeHex []string
	for xi := 1; xi <= v; xi++ {
		von, bis, _ := quantenSAbschnitt(n, v, xi)
		var c string
		for {
			var err error
			if c, err = netz.oeffnung(ctx, xi, K, s); err == nil {
				break
			} else if !time.Now().Before(ende) {
				return nil, nil, nil, fmt.Errorf("Validator %d: Oeffnung fuer Stelle %d nach der Bindung: %w", xi, s, err)
			}
			select {
			case <-ctx.Done():
				return nil, nil, nil, ctx.Err()
			case <-time.After(quantenSV2Nachfragen):
			}
		}
		m, salz, err := quantenSOeffnungEntschluesselnV2(key, c, K, s, xi, von, bis)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("Validator %d: Oeffnung fuer Stelle %d: %v", xi, s, err)
		}
		for i, w := range m {
			oe[i] = w
		}
		salze = append(salze, salz)
		salzeHex = append(salzeHex, hex.EncodeToString(salz[:]))
	}
	geladen, err := quantenSOeffnungLaden(s, K, n, wurzel, blaetter, oe, kn)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Stelle %d laedt ihre Stichprobe nicht: %v", s, err)
	}
	probe := make([]int, 0, len(geladen))
	for i := range geladen {
		probe = append(probe, i)
	}
	sort.Ints(probe)
	if h, err := quantenSProbeV2Pruefen(K, s, n, v, probe, salze); err != nil || h != probeBindung {
		return nil, nil, nil, fmt.Errorf("Stelle %d: Stichprobe und Salze passen nicht zur Bindung der Ausgabe", s)
	}
	return probe, salzeHex, geladen, nil
}

// quantenSV2Nachfragen: so oft fragt eine Stelle nach, bis die Validatoren
// die Oeffnung nach der Finalitaet herausgeben.
var quantenSV2Nachfragen = 2 * time.Second
