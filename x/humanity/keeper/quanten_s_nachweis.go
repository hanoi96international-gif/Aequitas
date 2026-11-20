package keeper

// QUANTENGELD VARIANTE S -- NACHWEIS NACH DEM ENDE (DEZENTRAL.md 17, R7)
//
// Solange ein Token lebt, ist seine Praeparation geheim: Wer die Werte kennt
// und zugleich Inhaberin ist, koennte an beiden Stellen bestehen. Ist er
// eingeloest oder tot und ist dieses Ende final, gibt es nichts Geheimes mehr.
// Dann legt jeder Validator seinen Abschnitt offen, unterschrieben:
//
//   - die gesalzenen Werte w (daraus Basis u, Wert t und Stellen-Bit jeder
//     Stelle) -- jeder rechnet daraus die Blaetter und die Wurzel nach, die bei
//     der Ausgabe auf der Kette stand;
//   - die Salze der Stichprobe v2 -- daraus die Proben der Ausgabe;
//   - die Messergebnisse x der Inhaberin auf dem Abschnitt (sie standen schon
//     in der Anmeldung auf der Kette) und, wenn der Validator die letzte
//     Anmeldung gesehen hat, die Basen, in denen die Kette geprueft hat
//     (d XOR b XOR c).
//
// Damit ist der ganze Lauf oeffentlich nachrechenbar: welche Zustaende
// praepariert waren, in welchen Basen gemessen wurde, wie viele Messfehler es
// bei passender Basis gab -- und dass genau das bei der Ausgabe festgelegt war.
// Was das NICHT beweist: dass ein Quantenrechner gemessen hat. Ein Betreiber,
// der Praeparation und Messung selbst erfindet, faellt damit nicht auf;
// dagegen hilft nur, dass mehrere unabhaengige Validatoren je ihren Abschnitt
// praeparieren (Weg A, A5).
//
// Begrenzt: hoechstens quantenSNachweiseMax im Speicher (die aeltesten
// fallen heraus), nicht ueber einen Neustart. Offengelegt wird erst, wenn das
// Ende final ist (reif), und nie fuer einen Token, der noch lebt.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"
)

const (
	quantenSDomNachweis  = "aequitas-quanten-s-nachweis-v1"
	quantenSNachweiseMax = 64
)

// QuantenSNachweis: der offengelegte Abschnitt eines beendeten Tokens.
type QuantenSNachweis struct {
	Dom         string   `json:"dom"`
	K           string   `json:"k"`
	N           int      `json:"n"`
	T           int      `json:"t"`
	X           int      `json:"x"`
	Von         int      `json:"von"`
	Bis         int      `json:"bis"`
	Validator   string   `json:"validator"`
	Validatoren []string `json:"validatoren"`
	Wurzel      string   `json:"wurzel"`
	Proben      []string `json:"proben"`
	Werte       []string `json:"werte"` // hex, je Stelle von..bis
	Salze       []string `json:"salze"` // je Annahmestelle 0, 1
	XInhaberin  string   `json:"x_inhaberin"`
	// Bindung d (auf diesem Abschnitt) und Wahl c, wie der Validator sie
	// gesehen hat, und die Stelle b der letzten Anmeldung; daraus die Basen,
	// in denen die Kette geprueft hat: d XOR b XOR c (leer, wenn nicht
	// gesehen -- etwa nach einem Neustart des Dienstes).
	D      string `json:"d,omitempty"`
	C      *int   `json:"c,omitempty"`
	Stelle *int   `json:"stelle,omitempty"`
	Zeit   int64  `json:"zeit"`
	Sig    string `json:"sig"`
}

// quantenSNachweisNachricht: was der Validator unterschreibt -- alles ausser
// der Unterschrift, als JSON in fester Feldreihenfolge.
func quantenSNachweisNachricht(n QuantenSNachweis) (string, error) {
	n.Sig = ""
	roh, err := json.Marshal(n)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(roh)
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d|%s", quantenSDomNachweis, eingefroreneChainID, n.Validator, n.K, n.X, hex.EncodeToString(h[:])), nil
}

// quantenSEnde: was der Dienst vom lebenden Token mitschreibt (fuer die
// Basen im Nachweis): Bindung d auf dem eigenen Abschnitt und Wahl c, sobald
// sie stehen; die Stelle b, sobald eine Anmeldung zu sehen ist (-1 = keine).
// Die Kette prueft an Stelle b in den Basen d XOR b XOR c.
type quantenSEnde struct {
	d      string
	c      int
	stelle int
}

// endeMerken: aus dem lebenden Token (taktS) -- nur fuer den eigenen
// Abschnitt, nur wenn Bindung und Wahl stehen.
func (d *quantenWDienst) endeMerken(K string, e *quantenWTresorEintrag, t *quantenSToken) {
	st := e.Abschnitt
	if st == nil || t == nil || t.D == nil || t.C == -1 || st.Von < 0 || st.Bis > len(t.D) || st.Von >= st.Bis {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ende == nil {
		d.ende = map[string]quantenSEnde{}
	}
	alt, bekannt := d.ende[K]
	if !bekannt && len(d.ende) >= quantenWSichtHoechstens {
		return
	}
	neu := quantenSEnde{d: qsBitsText(t.D[st.Von:st.Bis]), c: t.C, stelle: -1}
	if bekannt && alt.d == neu.d && alt.c == neu.c {
		neu.stelle = alt.stelle
	}
	if t.Anmeldung != nil {
		neu.stelle = t.Anmeldung.B
	}
	d.ende[K] = neu
}

// endeVon: was der Dienst vom lebenden Token K mitgeschrieben hat.
func (d *quantenWDienst) endeVon(K string) (quantenSEnde, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	en, ok := d.ende[K]
	return en, ok
}

// nachweisAus: der Nachweis zu einem Tresor-Eintrag eines beendeten Tokens.
func (d *quantenWDienst) nachweisAus(e *quantenWTresorEintrag) (*QuantenSNachweis, error) {
	st := e.Abschnitt
	if st == nil || e.Art != "s" || st.ProbeArt != 2 || len(st.Salze) != 2 {
		return nil, errors.New("kein Abschnitt mit Stichprobe v2")
	}
	n := &QuantenSNachweis{Dom: quantenSDomNachweis, K: e.K, N: e.N, T: e.T, X: e.X, Von: st.Von, Bis: st.Bis, Validator: d.adresse,
		Validatoren: append([]string(nil), e.Validatoren...), Wurzel: e.Wurzel, Proben: append([]string(nil), e.Proben...),
		Salze: append([]string(nil), st.Salze...), XInhaberin: st.X, Zeit: e.Zeit}
	for i := range st.Koeff {
		w, err := st.wert(i)
		if err != nil {
			return nil, err
		}
		n.Werte = append(n.Werte, w.Text(16))
	}
	if en, ok := d.endeVon(e.K); ok {
		n.D = en.d
		c := en.c
		n.C = &c
		if en.stelle >= 0 {
			b := en.stelle
			n.Stelle = &b
		}
	}
	m, err := quantenSNachweisNachricht(*n)
	if err != nil {
		return nil, err
	}
	if n.Sig, err = quantenSUnterschreiben(d.key, m); err != nil {
		return nil, err
	}
	return n, nil
}

// nachweisMerken: im Speicher ablegen, die aeltesten fallen heraus.
func (d *quantenWDienst) nachweisMerken(n *QuantenSNachweis) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.nachweise == nil {
		d.nachweise = map[string]*QuantenSNachweis{}
	}
	if _, schon := d.nachweise[n.K]; !schon {
		d.nachweisReihe = append(d.nachweisReihe, n.K)
	}
	d.nachweise[n.K] = n
	for len(d.nachweisReihe) > quantenSNachweiseMax {
		delete(d.nachweise, d.nachweisReihe[0])
		d.nachweisReihe = d.nachweisReihe[1:]
	}
	delete(d.ende, n.K)
}

// nachweis: der Nachweis zu K -- nur, wenn der Token auf der Kette nicht
// (mehr) lebt (doppelt geprueft: er entsteht ohnehin erst nach dem Ende).
func (d *quantenWDienst) nachweis(K string) (*QuantenSNachweis, error) {
	if reg := d.cs.quantenWJetzt(); reg.Token[K] != nil {
		return nil, quantenWFehler("der Token lebt -- seine Praeparation bleibt geheim")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	n := d.nachweise[K]
	if n == nil {
		return nil, quantenWFehler("kein Nachweis (der Token ist nicht final beendet, nie von diesem Validator praepariert oder zu lange her)")
	}
	c := *n
	return &c, nil
}

// handleQuantenSNachweis: GET /api/quanten-w/validator/nachweis?k= --
// oeffentlich und nur lesend, begrenzt wie die anderen Anfragen an den Dienst.
func (a *APIServer) handleQuantenSNachweis(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodGet {
		jsonError(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	d := a.state.quantenWDienstJetzt()
	if d == nil || !quantenWAktiv(time.Now().Unix()) {
		jsonError(w, "quantum money validator service not active", http.StatusConflict)
		return
	}
	if !burstErlaubt("quanten-w-dienst:"+erneuerungsAbsender(r), burstQuantenWDienstJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	K := strings.ToLower(r.URL.Query().Get("k"))
	if !istHash64(K) {
		jsonError(w, "k", http.StatusBadRequest)
		return
	}
	n, err := d.nachweis(K)
	if err != nil {
		jsonError(w, "rejected: "+err.Error(), http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(n)
}

// ------------------------------------------------------------ Pruefen

// QuantenSNachweisBericht: was ein Pruefer aus den Nachweisen aller
// Validatoren nachrechnet.
type QuantenSNachweisBericht struct {
	K      string   `json:"k"`
	N      int      `json:"n"`
	V      int      `json:"v"`
	Wurzel string   `json:"wurzel"`
	Proben []string `json:"proben"`
	// WurzelGegen: gegen welche Wurzel geprueft wurde ("kette" = vor dem Ende
	// von der Kette gelesen, "angegeben" = von Hand, "" = nur die Nachweise
	// untereinander).
	WurzelGegen string `json:"wurzel_gegen"`
	// Je Annahmestelle: Stellen der Stichprobe; dazu, wo die Basen bekannt
	// sind, Treffer (Basis der Messung = praeparierte Basis) und Fehler
	// (Messergebnis != praeparierter Wert).
	Stichprobe   [2]int `json:"stichprobe"`
	BasenBekannt int    `json:"basen_bekannt"`
	Treffer      int    `json:"treffer"`
	Fehler       int    `json:"fehler"`
	// An der gewaehlten Stelle (das, was die Kette geprueft hat). Nennen die
	// Validatoren verschiedene Stellen (etwa nach einer Doppelvorlage, deren
	// Anmeldung einer zuletzt sah), bleibt sie offen: keine Zaehlung nach
	// Basen statt einer falschen.
	Stelle               *int `json:"stelle,omitempty"`
	StellenUneinheitlich bool `json:"stellen_uneinheitlich,omitempty"`
	TrefferStelle        int  `json:"treffer_stelle"`
	FehlerStelle         int  `json:"fehler_stelle"`
}

// quantenSNachweisePruefen: Unterschrift jedes Validators (aus der eigenen
// Liste), Abschnitte lueckenlos, Werte gegen die Blaetter, Wurzel und Proben
// nachgerechnet; soll (optional) die Wurzel der Ausgabe auf der Kette.
func quantenSNachweisePruefen(K string, validatoren []string, nw []QuantenSNachweis, soll *[32]byte, sollProben []string) (*QuantenSNachweisBericht, error) {
	v := len(validatoren)
	if v == 0 || len(nw) != v {
		return nil, fmt.Errorf("%d Nachweise fuer %d Validatoren", len(nw), v)
	}
	n := nw[0].N
	if n < 2 || n > quantenWNMax || !istHash64(K) {
		return nil, errors.New("n oder K")
	}
	ber := &QuantenSNachweisBericht{K: K, N: n, V: v}
	blaetter := make([][32]byte, 0, n)
	werte := make([]*big.Int, 0, n)
	xs := make([]int, 0, n)
	basen := make([]int, n)
	var teile [2][][32]byte
	stelle, wahl := -2, -2
	dBits := make([]int, n)
	for i := range basen {
		basen[i], dBits[i] = -1, -1
	}
	for j, a := range nw {
		x := j + 1
		von, bis, ok := quantenSAbschnitt(n, v, x)
		m, err := quantenSNachweisNachricht(a)
		switch {
		case err != nil:
			return nil, err
		case a.Dom != quantenSDomNachweis || a.K != K || a.N != n || a.X != x || !ok || a.Von != von || a.Bis != bis:
			return nil, fmt.Errorf("Nachweis %d: K, n oder Abschnitt passen nicht", x)
		case a.Validator != validatoren[j] || !kanonischeSignatur(a.Sig) || verifyPersonalSign(m, a.Sig, validatoren[j]) != nil:
			return nil, fmt.Errorf("Nachweis %d: nicht von Validator %s unterschrieben", x, validatoren[j])
		case len(a.Werte) != bis-von || len(a.Salze) != 2 || len(a.XInhaberin) != bis-von:
			return nil, fmt.Errorf("Nachweis %d: Laengen", x)
		case j > 0 && (a.Wurzel != nw[0].Wurzel || strings.Join(a.Proben, ",") != strings.Join(nw[0].Proben, ",") || strings.Join(a.Validatoren, ",") != strings.Join(validatoren, ",")):
			return nil, fmt.Errorf("Nachweis %d: Wurzel, Proben oder Validatoren anders als bei Validator 1", x)
		}
		xa, err := quantenSBitsText(a.XInhaberin, bis-von)
		if err != nil {
			return nil, fmt.Errorf("Nachweis %d: x", x)
		}
		var da []int
		if a.D != "" || a.C != nil {
			if da, err = quantenSBitsText(a.D, bis-von); err != nil || a.C == nil || (*a.C != 0 && *a.C != 1) {
				return nil, fmt.Errorf("Nachweis %d: Bindung oder Wahl", x)
			}
			if wahl == -2 {
				wahl = *a.C
			} else if wahl != *a.C {
				return nil, errors.New("die Nachweise nennen verschiedene Wahlen")
			}
		}
		if a.Stelle != nil {
			if *a.Stelle != 0 && *a.Stelle != 1 {
				return nil, fmt.Errorf("Nachweis %d: Stelle", x)
			}
			if stelle == -2 {
				stelle = *a.Stelle
			} else if stelle != *a.Stelle {
				ber.StellenUneinheitlich = true
			}
		}
		var stellen [2][]int
		grenze := new(big.Int).Lsh(big.NewInt(1), quantenWSalzBits+2)
		for i, wh := range a.Werte {
			w, ok := new(big.Int).SetString(wh, 16)
			if !ok || w.Sign() < 0 || w.Cmp(grenze) >= 0 || len(wh) > 33 {
				return nil, fmt.Errorf("Nachweis %d: Wert %d", x, i)
			}
			blaetter = append(blaetter, quantenWPruefsumme(K, von+i, w))
			werte = append(werte, w)
			stellen[quantenSStellenBit(w)] = append(stellen[quantenSStellenBit(w)], von+i)
			if da != nil {
				dBits[von+i] = da[i]
			}
		}
		xs = append(xs, xa...)
		for s := 0; s < 2; s++ {
			salz, ok := quantenWHex32(a.Salze[s])
			if !ok {
				return nil, fmt.Errorf("Nachweis %d: Salz", x)
			}
			teile[s] = append(teile[s], quantenSProbeTeilHash(K, s, x, salz, stellen[s]))
			ber.Stichprobe[s] += len(stellen[s])
		}
	}
	w := quantenWMerkleWurzel(blaetter)
	ber.Wurzel = hex.EncodeToString(w[:])
	if ber.Wurzel != nw[0].Wurzel {
		return nil, errors.New("die Werte ergeben nicht die Wurzel, die der Validator nennt")
	}
	for s := 0; s < 2; s++ {
		h, err := quantenSProbeHashV2(K, s, teile[s])
		if err != nil {
			return nil, err
		}
		ber.Proben = append(ber.Proben, hex.EncodeToString(h[:]))
	}
	if len(nw[0].Proben) != 2 || ber.Proben[0] != nw[0].Proben[0] || ber.Proben[1] != nw[0].Proben[1] {
		return nil, errors.New("Werte und Salze ergeben nicht die Proben, die der Validator nennt")
	}
	if soll != nil {
		if w != *soll {
			return nil, errors.New("BEFUND: die offengelegte Praeparation ergibt nicht die Wurzel der Ausgabe")
		}
		ber.WurzelGegen = "kette"
	}
	if sollProben != nil && (len(sollProben) != 2 || sollProben[0] != ber.Proben[0] || sollProben[1] != ber.Proben[1]) {
		return nil, errors.New("BEFUND: die offengelegten Salze ergeben nicht die Proben der Ausgabe")
	}
	// Die Basen, in denen die Kette geprueft hat: d XOR b XOR c -- nur, wo d
	// bekannt ist und eine Stelle genannt.
	if ber.StellenUneinheitlich {
		stelle = -2
	}
	if stelle >= 0 && wahl >= 0 {
		for i := range basen {
			if dBits[i] >= 0 {
				basen[i] = dBits[i] ^ stelle ^ wahl
			}
		}
	}
	for i, wv := range werte {
		if basen[i] < 0 {
			continue
		}
		u, t := quantenWZerlegen(wv)
		ber.BasenBekannt++
		if basen[i] == u {
			ber.Treffer++
			if xs[i] != t {
				ber.Fehler++
			}
			if stelle >= 0 && quantenSStellenBit(wv) == stelle {
				ber.TrefferStelle++
				if xs[i] != t {
					ber.FehlerStelle++
				}
			}
		}
	}
	if stelle >= 0 {
		ber.Stelle = &stelle
	}
	return ber, nil
}

// quantenSNachweisHolen: der Nachweis von Validator x.
func (m *quantenSMehrNetz) nachweis(ctx context.Context, x int, K string) (QuantenSNachweis, error) {
	h, err := m.validator(x)
	if err != nil {
		return QuantenSNachweis{}, err
	}
	gross := *h
	gross.max = quantenSAbschnittAntwortMax
	code, body, err := gross.anfrage(ctx, http.MethodGet, "/api/quanten-w/validator/nachweis?k="+K, nil)
	if err != nil {
		return QuantenSNachweis{}, err
	}
	if code != http.StatusOK {
		return QuantenSNachweis{}, fmt.Errorf("HTTP %d: %s", code, quantenSFehlertext(body))
	}
	var a QuantenSNachweis
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return QuantenSNachweis{}, errors.New("Antwort ist kein passendes JSON")
	}
	return a, nil
}

// quantenSNachweisTakt: so oft fragt der Pruefer nach, bis jeder Validator
// den Nachweis herausgibt (erst nach der Finalitaet des Endes).
var quantenSNachweisTakt = 10 * time.Second

// quantenSNachweisAbwarten: von jedem Validator den Nachweis holen (bis ende)
// und alle zusammen gegen Wurzel und Proben der Ausgabe pruefen.
func quantenSNachweisAbwarten(ctx context.Context, netz *quantenSMehrNetz, K string, validatoren []string, wurzelHex string, proben []string,
	ende time.Time, log io.Writer) (*QuantenSNachweisBericht, error) {
	soll, ok := quantenWHex32(wurzelHex)
	if !ok || len(proben) != 2 {
		return nil, errors.New("Nachweis: Wurzel oder Proben der Ausgabe fehlen")
	}
	nw := make([]QuantenSNachweis, len(validatoren))
	for x := 1; x <= len(validatoren); x++ {
		for {
			n, err := netz.nachweis(ctx, x, K)
			if err == nil {
				nw[x-1] = n
				break
			}
			if !time.Now().Before(ende) {
				return nil, fmt.Errorf("Nachweis von Validator %d: %w", x, err)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(quantenSNachweisTakt):
			}
		}
	}
	ber, err := quantenSNachweisePruefen(K, validatoren, nw, &soll, proben)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(log, "Nachweis nach dem Ende: Praeparation passt zur Wurzel der Ausgabe; %d Fehler bei %d Treffern (an der Stelle %d bei %d)\n",
		ber.Fehler, ber.Treffer, ber.FehlerStelle, ber.TrefferStelle)
	return ber, nil
}

// QuantenSNachweisPruefenBefehl: `aequitasd quanten-s-nachweis --k K
// --validatoren adr=api,... [--wurzel H --proben H0,H1]` -- jeder kann einen
// beendeten Token nachpruefen. Ohne Wurzel und Proben der Ausgabe nur in sich
// (der Bericht sagt es: wurzel_gegen leer).
func QuantenSNachweisPruefenBefehl(ctx context.Context, K string, validatoren []QuantenSValidator, wurzelHex string, proben []string, aus io.Writer) error {
	K = strings.ToLower(strings.TrimSpace(K))
	if !istHash64(K) {
		return errors.New("K: 64 Hex-Zeichen")
	}
	adressen, netze, err := quantenSValidatorenPruefen(validatoren, 1)
	if err != nil {
		return err
	}
	netz := &quantenSMehrNetz{validatoren: netze}
	nw := make([]QuantenSNachweis, len(adressen))
	for x := 1; x <= len(adressen); x++ {
		if nw[x-1], err = netz.nachweis(ctx, x, K); err != nil {
			return fmt.Errorf("Nachweis von Validator %d: %w", x, err)
		}
	}
	var soll *[32]byte
	if wurzelHex != "" {
		w, ok := quantenWHex32(strings.ToLower(strings.TrimSpace(wurzelHex)))
		if !ok {
			return errors.New("Wurzel: 64 Hex-Zeichen")
		}
		soll = &w
	}
	if proben != nil && len(proben) != 2 {
		return errors.New("Proben: zwei, mit Komma getrennt")
	}
	ber, err := quantenSNachweisePruefen(K, adressen, nw, soll, proben)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(aus)
	enc.SetIndent("", "  ")
	return enc.Encode(ber)
}
