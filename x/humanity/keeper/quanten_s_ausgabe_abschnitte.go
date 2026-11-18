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
}

func quantenSAusgabeLaufAbschnitte(ctx context.Context, a QuantenSAusgabeAuftrag, basen QuantenSBasen, validatoren []string, t int, netz quantenSAbschnittNetz, log io.Writer) (*QuantenSAusgabeErgebnis, error) {
	a.log, a.beginn = log, time.Now()
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
	// 1. Schluessel der Validatoren, jeder gegen seine Adresse.
	schl := make([]string, v)
	for x := 1; x <= v; x++ {
		s, err := netz.schluessel(ctx, x)
		if err != nil {
			return nil, fmt.Errorf("Validator %d: Schluessel: %w", x, err)
		}
		if _, err := quantenSSchluesselPruefen(s, validatoren[x-1]); err != nil {
			return nil, fmt.Errorf("Validator %d: %v", x, err)
		}
		schl[x-1] = strings.TrimPrefix(strings.ToLower(s), "0x")
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
	var stellen [2]string
	var stellenKey [2]*ecdsa.PrivateKey
	stellen[b], stellenKey[b] = ausgeber, a.Schluessel
	stellen[1-b], stellenKey[1-b] = quantenSAdresse(fremdKey), fremdKey
	pub := func(k *ecdsa.PrivateKey) string { return hex.EncodeToString(crypto.FromECDSAPub(&k.PublicKey)) }
	zeit := time.Now().Unix()
	if a.Jetzt != nil {
		zeit = a.Jetzt()
	}
	// 2. Je Validator sein Abschnitt.
	antw := make([]QuantenSAbschnittAntwort, v)
	x := make([]int, n)
	blaetter := make([][32]byte, n)
	for xi := 1; xi <= v; xi++ {
		von, bis, _ := quantenSAbschnitt(n, v, xi)
		req := QuantenSAbschnittAnfrage{K: K, N: n, T: t, Tol: a.Tol, X: xi, Validatoren: validatoren, Schluessel: schl,
			Basen: basen.Basen[xi-1], Inhaberin: inhaber, InhaberinSchluessel: pub(inhaberKey), Stellen: stellen[:],
			StellenSchluessel: []string{pub(stellenKey[0]), pub(stellenKey[1])}, Zeit: zeit, Praeger: ausgeber}
		if req.PraegerSig, err = quantenSUnterschreiben(a.Schluessel, quantenSAbschnittAuftragNachricht(req)); err != nil {
			return nil, err
		}
		ans, err := netz.abschnitt(ctx, xi, req)
		if err != nil {
			return nil, fmt.Errorf("Validator %d praepariert nicht: %w", xi, err)
		}
		if ans.K != K || ans.X != xi || ans.Von != von || ans.Bis != bis || len(ans.Blaetter) != bis-von ||
			len(ans.Buendel) != v || len(ans.BuendelSig) != v || ans.Buendel[xi-1] != "" ||
			!kanonischeSignatur(ans.Sig) || verifyPersonalSign(quantenSAbschnittAntwortNachricht(ans), ans.Sig, validatoren[xi-1]) != nil {
			return nil, fmt.Errorf("Validator %d: Antwort passt nicht oder ist nicht von ihm", xi)
		}
		xs, err := quantenSFuerInhaberinOeffnen(inhaberKey, ans.FuerInhaberin, K, von, bis)
		if err != nil {
			return nil, fmt.Errorf("Validator %d: %v", xi, err)
		}
		copy(x[von:], xs)
		for i, h := range ans.Blaetter {
			bl, ok := quantenWHex32(h)
			if !ok {
				return nil, fmt.Errorf("Validator %d: Blatt %d", xi, i)
			}
			blaetter[von+i] = bl
		}
		antw[xi-1] = ans
	}
	wurzel := quantenWMerkleWurzel(blaetter)
	// 3. Die Annahmestellen laden ihre Stichproben selbst.
	var probe [2][]int
	var proben [2][32]byte
	var probenHex []string
	passend, falsch := 0, 0
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
				if x[i] != ut[1] {
					falsch++
				}
			}
		}
		sort.Ints(probe[s])
		proben[s] = quantenSProbeHash(K, s, probe[s])
		probenHex = append(probenHex, hex.EncodeToString(proben[s][:]))
	}
	wurzelHex := hex.EncodeToString(wurzel[:])
	fmt.Fprintf(log, "Token %s: n=%d in %d Abschnitten, %d/%d Stellen je Annahmestelle, Fehlerrate bei passender Basis %d/%d (aus den Stichproben)\n",
		K[:12], n, v, len(probe[0]), len(probe[1]), falsch, passend)
	// 4. Bestaetigungen mit den Buendeln der anderen.
	var bestaetigungen []string
	for xi := 1; xi <= v; xi++ {
		req := QuantenWBestaetigenAnfrage{K: K, N: n, T: t, Tol: a.Tol, X: xi, Wurzel: wurzelHex, Validatoren: validatoren,
			Zeit: zeit, Art: "s", Proben: probenHex, Praeger: ausgeber}
		for _, bl := range blaetter {
			req.Blaetter = append(req.Blaetter, hex.EncodeToString(bl[:]))
		}
		for p := 0; p < v; p++ {
			req.Buendel = append(req.Buendel, antw[p].Buendel[xi-1])
			req.BuendelSig = append(req.BuendelSig, antw[p].BuendelSig[xi-1])
		}
		if req.PraegerSig, err = quantenSUnterschreiben(a.Schluessel, quantenWDienstAuftrag(req)); err != nil {
			return nil, err
		}
		sig, err := netz.bestaetigen(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("Validator %d bestaetigt nicht: %w", xi, err)
		}
		if !kanonischeSignatur(sig) || verifyPersonalSign(quantenWSBestaetigungNachricht(K, wurzel, n, t, validatoren, xi, proben, zeit, a.Tol), sig, validatoren[xi-1]) != nil {
			return nil, fmt.Errorf("Validator %d: Bestaetigung ist nicht von ihm", xi)
		}
		bestaetigungen = append(bestaetigungen, sig)
	}
	return quantenSAusgabeAbschluss(ctx, a, netz, log, quantenSVorbereitet{K: K, N: n, T: t, KJeStelle: min(len(probe[0]), len(probe[1])),
		Validatoren: validatoren, Bestaetigungen: bestaetigungen, Wurzel: wurzelHex, ProbenHex: probenHex, Probe: probe, Stellen: stellen,
		StellenKey: stellenKey, Ausgeber: ausgeber, Inhaber: inhaber, InhaberKey: inhaberKey, B: b, Z: z, Y: y, X: x, Zeit: zeit,
		Fehlerrate: fmt.Sprintf("%d/%d", falsch, passend), Wo: basen.Wo, Job: basen.Job})
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
