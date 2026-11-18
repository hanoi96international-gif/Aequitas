package keeper

// QUANTENGELD VARIANTE S -- AUSGABE IN ABSCHNITTEN IM DIENST DER VALIDATOREN (A2b)
//
// Bausteine: quanten_s_abschnitt.go. Begruendung und Ablauf:
// forschung/quantengeld/DEZENTRAL.md 8.
//
//  1. ABSCHNITT (POST /api/quanten-w/validator/abschnitt, vom Ausgeber,
//     unterschrieben, nur ein Mensch mit Deckung): Validator x holt die
//     Messung SEINES Abschnitts aus seiner eigenen Messquelle (nie ueber die
//     API), zieht Salz und Stellen-Bit, teilt jede Stelle t-von-v und
//     antwortet mit den Blaettern, x verschluesselt fuer die Inhaberin und je
//     anderem Validator einem verschluesselten, unterschriebenen Buendel. Er
//     legt den Eintrag WARTEND ab (gilt als offener Auftrag).
//  2. OEFFNUNG (GET /api/quanten-w/validator/oeffnung?k=&b=): die Werte
//     seines Abschnitts mit Stellen-Bit b, verschluesselt fuer die
//     Annahmestelle b aus dem Auftrag -- nur sie kann sie lesen.
//  3. BESTAETIGEN (wie bisher, mit Buendeln statt Anteilen): Buendel der
//     anderen pruefen und oeffnen, Anteile aller Stellen zusammensetzen,
//     Blaetter gegen die Wurzel, dann dieselbe Bestaetigung wie immer.
//
// Die Kette aendert sich nicht: Wurzel, Anteile, Bestaetigung und Offenlegung
// haben dieselbe Form wie bei der Ausgabe mit einem Auftrag.

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto/ecies"
)

const (
	quantenSDomAbschnittAuftrag = "aequitas-quanten-s-abschnitt-auftrag-v1"
	quantenSDomAbschnittAntwort = "aequitas-quanten-s-abschnitt-antwort-v1"
	quantenSDomFuerInhaberin    = "aequitas-quanten-s-fuer-inhaberin-v1"
	quantenSDomOeffnung         = "aequitas-quanten-s-oeffnung-v1"
	// Eine Messquelle-Datei: hoechstens so gross, hoechstens so viele Abschnitte.
	quantenSMessDateiMax        = 16 << 20
	quantenSMessAbschnitteMax   = 256
	quantenSAbschnittKoerperMax = 64 << 10
)

// quantenSAbschnittStand: was ein Validator zu SEINEM Abschnitt haelt.
type quantenSAbschnittStand struct {
	Von int `json:"von"`
	Bis int `json:"bis"`
	// Koeff[i][j]: Koeffizient j des Polynoms der Stelle Von+i (hex);
	// Koeff[i][0] ist der gesalzene Wert. Daraus entstehen die Anteile fuer
	// jeden Validator, bei einer Wiederholung dieselben.
	Koeff             [][]string `json:"koeff"`
	X                 string     `json:"x"` // Messergebnisse der Inhaberin auf dem Abschnitt
	Inhaberin         string     `json:"inhaberin"`
	InhaberinSchl     string     `json:"inhaberin_schluessel"`
	Schluessel        []string   `json:"schluessel"`
	Stellen           []string   `json:"stellen"`
	StellenSchluessel []string   `json:"stellen_schluessel"`
	AuftragHash       string     `json:"auftrag_hash"`
}

// QuantenSAbschnittAnfrage: der Auftrag des Ausgebers an Validator X.
type QuantenSAbschnittAnfrage struct {
	K                   string   `json:"k"`
	N                   int      `json:"n"`
	T                   int      `json:"t"`
	Tol                 int      `json:"tol"`
	X                   int      `json:"x"`
	Validatoren         []string `json:"validatoren"`
	Schluessel          []string `json:"schluessel"`
	Basen               string   `json:"basen"` // y der Inhaberin auf Abschnitt X, '0'/'1'
	Inhaberin           string   `json:"inhaberin"`
	InhaberinSchluessel string   `json:"inhaberin_schluessel"`
	Stellen             []string `json:"stellen"`
	StellenSchluessel   []string `json:"stellen_schluessel"`
	Zeit                int64    `json:"zeit"`
	Praeger             string   `json:"praeger"`
	PraegerSig          string   `json:"praeger_sig"`
}

// QuantenSAbschnittAntwort: was Validator X dem Ausgeber zurueckgibt.
type QuantenSAbschnittAntwort struct {
	K             string   `json:"k"`
	X             int      `json:"x"`
	Von           int      `json:"von"`
	Bis           int      `json:"bis"`
	Blaetter      []string `json:"blaetter"`
	FuerInhaberin string   `json:"fuer_inhaberin"`
	Buendel       []string `json:"buendel"`
	BuendelSig    []string `json:"buendel_sig"`
	Validator     string   `json:"validator"`
	Sig           string   `json:"sig"`
}

func quantenSHashListe(l []string) string {
	h := sha256.Sum256([]byte(strings.Join(l, ",")))
	return hex.EncodeToString(h[:])
}

// quantenSAbschnittAuftragNachricht: was der Ausgeber unterschreibt.
func quantenSAbschnittAuftragNachricht(q QuantenSAbschnittAnfrage) string {
	hb := sha256.Sum256([]byte(q.Basen))
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d|%d|%d|%d|%s|%s|%x|%s|%s|%s|%s|%d", quantenSDomAbschnittAuftrag, eingefroreneChainID,
		strings.ToLower(q.Praeger), q.K, q.N, q.T, q.Tol, q.X, strings.Join(q.Validatoren, ","), strings.Join(q.Schluessel, ","), hb,
		strings.ToLower(q.Inhaberin), q.InhaberinSchluessel, strings.Join(q.Stellen, ","), strings.Join(q.StellenSchluessel, ","), q.Zeit)
}

// quantenSAbschnittAntwortNachricht: was der Validator ueber seine Antwort
// unterschreibt -- der Ausgeber kann sie weiterleiten, nicht aendern.
func quantenSAbschnittAntwortNachricht(a QuantenSAbschnittAntwort) string {
	hf := sha256.Sum256([]byte(a.FuerInhaberin))
	return fmt.Sprintf("%s|chain:%d|%s|%d|%d|%d|%s|%x|%s|%s", quantenSDomAbschnittAntwort, eingefroreneChainID, a.K, a.X, a.Von, a.Bis,
		quantenSHashListe(a.Blaetter), hf, quantenSHashListe(a.Buendel), quantenSHashListe(a.BuendelSig))
}

// ------------------------------------------------------------ Messquelle

// quantenSMessquelle: die eigene Messung eines Abschnitts -- (u, t, x) zu den
// Basen y der Inhaberin. Jeder Datensatz genau einmal (dieselben Zustaende in
// zwei Token waeren eine gemeinsame Kopie).
type quantenSMessquelle interface {
	abschnitt(y []int) (u, t, x []int, wo string, err error)
}

// quantenSMessDatei: Datei von hardware_ibm.py --s-abschnitt mit fertig
// gemessenen Abschnitten (Basen y vorab gezogen). Ein benutzter Abschnitt wird
// sofort aus der Datei geloescht (atomar ersetzt), bevor er verwendet wird.
type quantenSMessDatei struct {
	mu   sync.Mutex
	pfad string
}

type quantenSMessDateiInhalt struct {
	Abschnitte []quantenSMessDatensatz `json:"abschnitte"`
}

type quantenSMessDatensatz struct {
	U   string `json:"u"`
	T   string `json:"t"`
	Y   string `json:"y"`
	X   string `json:"x"`
	Wo  string `json:"wo"`
	Job string `json:"job"`
}

func quantenSBitsLesen(s string, n int) ([]int, bool) {
	if len(s) != n {
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

func (m *quantenSMessDatei) abschnitt(y []int) ([]int, []int, []int, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	roh, err := os.ReadFile(m.pfad)
	if err != nil {
		return nil, nil, nil, "", quantenWFehler("Messquelle nicht lesbar")
	}
	if len(roh) > quantenSMessDateiMax {
		return nil, nil, nil, "", quantenWFehler("Messquelle zu gross")
	}
	var in quantenSMessDateiInhalt
	dec := json.NewDecoder(bytes.NewReader(roh))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || len(in.Abschnitte) > quantenSMessAbschnitteMax {
		return nil, nil, nil, "", quantenWFehler("Messquelle unlesbar")
	}
	soll := qsBitsText(y)
	for j, d := range in.Abschnitte {
		if d.Y != soll {
			continue
		}
		u, ok1 := quantenSBitsLesen(d.U, len(y))
		t, ok2 := quantenSBitsLesen(d.T, len(y))
		x, ok3 := quantenSBitsLesen(d.X, len(y))
		if !ok1 || !ok2 || !ok3 {
			return nil, nil, nil, "", quantenWFehler("Messquelle: Abschnitt %d kaputt", j)
		}
		// Erst loeschen, dann verwenden: ein Absturz danach verschwendet den
		// Abschnitt, verwendet ihn aber nie zweimal.
		in.Abschnitte = append(in.Abschnitte[:j], in.Abschnitte[j+1:]...)
		neu, err := json.Marshal(in)
		if err != nil {
			return nil, nil, nil, "", err
		}
		tmp := m.pfad + ".neu"
		if err := os.WriteFile(tmp, neu, 0o600); err != nil {
			return nil, nil, nil, "", fmt.Errorf("Messquelle schreiben: %w", err)
		}
		if err := os.Rename(tmp, m.pfad); err != nil {
			return nil, nil, nil, "", fmt.Errorf("Messquelle ersetzen: %w", err)
		}
		return u, t, x, d.Wo + " " + d.Job, nil
	}
	return nil, nil, nil, "", quantenWFehler("keine Messung zu diesen Basen")
}

// quantenSMessquelleAusUmgebung: AEQUITAS_QS_MESSQUELLE = Pfad der Datei
// (nur lesbar fuer den Knoten). Leer: dieser Validator praepariert keine
// Abschnitte.
func quantenSMessquelleAusUmgebung() quantenSMessquelle {
	p := strings.TrimSpace(os.Getenv("AEQUITAS_QS_MESSQUELLE"))
	if p == "" {
		return nil
	}
	return &quantenSMessDatei{pfad: filepath.Clean(p)}
}

// ------------------------------------------------------------ Abschnitt

// quantenSAbschnittPruefen: alles, was ohne Tresor geprueft werden kann.
func (d *quantenWDienst) quantenSAbschnittPruefen(q *QuantenSAbschnittAnfrage) (von, bis int, err error) {
	q.K = strings.ToLower(strings.TrimSpace(q.K))
	if !istHash64(q.K) {
		return 0, 0, quantenWFehler("K")
	}
	reg := d.cs.quantenWJetzt()
	if reg.Noten[q.K] != nil || reg.Token[q.K] != nil {
		return 0, 0, quantenWFehler("schon ausgegeben")
	}
	if _, tot := reg.Tote[q.K]; tot {
		return 0, 0, quantenWFehler("war schon ausgegeben")
	}
	v := len(q.Validatoren)
	if len(q.Schluessel) != v || len(q.Stellen) != 2 || len(q.StellenSchluessel) != 2 {
		return 0, 0, quantenWFehler("Zahl der Schluessel oder Annahmestellen")
	}
	gesehen := map[string]bool{}
	for j, a := range q.Validatoren {
		if !kanonischeAdresse(a) || gesehen[a] {
			return 0, 0, quantenWFehler("Validator nicht kanonisch oder doppelt")
		}
		gesehen[a] = true
		if _, err := quantenSSchluesselPruefen(q.Schluessel[j], a); err != nil {
			return 0, 0, quantenWFehler("Validator %d: %v", j+1, err)
		}
	}
	if q.X < 1 || q.X > v || q.Validatoren[q.X-1] != d.adresse {
		return 0, 0, quantenWFehler("Nummer %d gehoert nicht zu diesem Validator", q.X)
	}
	q.Inhaberin = strings.ToLower(strings.TrimSpace(q.Inhaberin))
	if !kanonischeAdresse(q.Inhaberin) {
		return 0, 0, quantenWFehler("Inhaberin")
	}
	if _, err := quantenSSchluesselPruefen(q.InhaberinSchluessel, q.Inhaberin); err != nil {
		return 0, 0, quantenWFehler("Inhaberin: %v", err)
	}
	for b := 0; b < 2; b++ {
		s := q.Stellen[b]
		if !kanonischeAdresse(s) || s == q.Inhaberin || (b == 1 && s == q.Stellen[0]) {
			return 0, 0, quantenWFehler("Annahmestelle %d", b)
		}
		if _, err := quantenSSchluesselPruefen(q.StellenSchluessel[b], s); err != nil {
			return 0, 0, quantenWFehler("Annahmestelle %d: %v", b, err)
		}
	}
	// Form des Tokens (wie die Kette); jede Stelle bekommt mindestens k_note.
	if _, err := neuerQuantenSToken(q.N, q.Inhaberin, [32]byte{}, 1, [2][32]byte{}, q.T, v, q.Tol); err != nil {
		return 0, 0, err
	}
	kn := quantenWKNote(q.Tol, q.T, v)
	klein := 0
	for x := 1; x <= v; x++ {
		a, b, ok := quantenSAbschnitt(q.N, v, x)
		if !ok {
			return 0, 0, quantenWFehler("Abschnitte")
		}
		klein += (b - a) / 2
	}
	if kn <= 0 || klein < kn {
		return 0, 0, quantenWFehler("zu wenige Stellen je Annahmestelle (%d, verlangt %d)", klein, kn)
	}
	von, bis, _ = quantenSAbschnitt(q.N, v, q.X)
	if len(q.Basen) != bis-von {
		return 0, 0, quantenWFehler("Basen: %d statt %d", len(q.Basen), bis-von)
	}
	if _, ok := quantenSBitsLesen(q.Basen, bis-von); !ok {
		return 0, 0, quantenWFehler("Basen: nur 0 und 1")
	}
	q.Praeger = strings.ToLower(strings.TrimSpace(q.Praeger))
	if !kanonischeAdresse(q.Praeger) || !kanonischeSignatur(q.PraegerSig) ||
		verifyPersonalSign(quantenSAbschnittAuftragNachricht(*q), q.PraegerSig, q.Praeger) != nil {
		return 0, 0, quantenWFehler("Auftrag nicht vom Ausgeber unterschrieben")
	}
	if !d.praegerTauglich(q.Praeger) {
		return 0, 0, quantenWFehler("Ausgeber ist kein Mensch mit Deckung")
	}
	jetzt := d.jetzt()
	if q.Zeit < jetzt-quantenWAlt || q.Zeit > jetzt+quantenWVoraus {
		return 0, 0, quantenWFehler("Ausgabezeit ausserhalb des Fensters")
	}
	return von, bis, nil
}

// abschnittPraeparieren: Auftrag pruefen, eigene Messung holen, Werte,
// Anteile, Buendel; wartend ablegen. Derselbe Auftrag noch einmal: dieselben
// Werte und Anteile (neu verschluesselt).
func (d *quantenWDienst) abschnittPraeparieren(q QuantenSAbschnittAnfrage) (QuantenSAbschnittAntwort, error) {
	von, bis, err := d.quantenSAbschnittPruefen(&q)
	if err != nil {
		return QuantenSAbschnittAntwort{}, err
	}
	auftragHash := sha256.Sum256([]byte(quantenSAbschnittAuftragNachricht(q)))
	if vorher, da := d.eintragVon(q.K); da {
		if vorher.Abschnitt == nil || vorher.Abschnitt.AuftragHash != hex.EncodeToString(auftragHash[:]) {
			return QuantenSAbschnittAntwort{}, quantenWFehler("schon ein anderer Auftrag fuer dieses K")
		}
		return d.abschnittAntwort(vorher)
	}
	if d.messquelle == nil {
		return QuantenSAbschnittAntwort{}, quantenWFehler("dieser Validator hat keine eigene Messquelle")
	}
	// Grenzen vor der Messung (sonst verbraucht ein Auftrag ueber der Grenze
	// einen Datensatz) und noch einmal unter derselben Sperre beim Ablegen.
	d.mu.Lock()
	err = d.abschnittGrenzenLocked(q.Praeger)
	d.mu.Unlock()
	if err != nil {
		return QuantenSAbschnittAntwort{}, err
	}
	y, _ := quantenSBitsLesen(q.Basen, bis-von)
	u, tw, x, _, err := d.messquelle.abschnitt(y)
	if err != nil {
		return QuantenSAbschnittAntwort{}, err
	}
	werte, err := quantenSAbschnittWerte(u, tw)
	if err != nil {
		return QuantenSAbschnittAntwort{}, err
	}
	st := &quantenSAbschnittStand{Von: von, Bis: bis, X: qsBitsText(x), Inhaberin: q.Inhaberin, InhaberinSchl: q.InhaberinSchluessel,
		Schluessel: slices.Clone(q.Schluessel), Stellen: slices.Clone(q.Stellen), StellenSchluessel: slices.Clone(q.StellenSchluessel),
		AuftragHash: hex.EncodeToString(auftragHash[:])}
	for _, w := range werte {
		koeff := []string{w.Text(16)}
		for j := 1; j < q.T; j++ {
			c, err := rand.Int(rand.Reader, quantenWP)
			if err != nil {
				return QuantenSAbschnittAntwort{}, err
			}
			koeff = append(koeff, c.Text(16))
		}
		st.Koeff = append(st.Koeff, koeff)
	}
	e := &quantenWTresorEintrag{K: q.K, N: q.N, T: q.T, Tol: q.Tol, X: q.X, Validatoren: slices.Clone(q.Validatoren), Zeit: q.Zeit,
		Praeger: q.Praeger, Art: "s", Wartend: true, Abschnitt: st}
	roh, err := d.versiegeln(e)
	if err != nil {
		return QuantenSAbschnittAntwort{}, err
	}
	d.mu.Lock()
	if _, da := d.eintraege[q.K]; da {
		d.mu.Unlock()
		return QuantenSAbschnittAntwort{}, quantenWFehler("schon ein Auftrag fuer dieses K")
	}
	if err := d.abschnittGrenzenLocked(q.Praeger); err != nil {
		d.mu.Unlock()
		return QuantenSAbschnittAntwort{}, err
	}
	if err := d.ablage.setzen(q.K, roh); err != nil {
		d.mu.Unlock()
		return QuantenSAbschnittAntwort{}, err
	}
	d.eintraege[q.K] = e
	d.mu.Unlock()
	return d.abschnittAntwort(e)
}

// abschnittGrenzenLocked: Tresor nicht voll, nicht zu viele offene Auftraege
// insgesamt und je Ausgeber (wie bestaetigen). Caller haelt d.mu.
func (d *quantenWDienst) abschnittGrenzenLocked(praeger string) error {
	if len(d.eintraege) >= quantenWTresorHoechstens {
		return quantenWFehler("Tresor voll")
	}
	reg := d.cs.quantenWJetzt()
	offen, jePraeger := 0, 0
	for k, x := range d.eintraege {
		if reg.Noten[k] == nil && reg.Token[k] == nil {
			offen++
			if x.Praeger == praeger {
				jePraeger++
			}
		}
	}
	if offen >= quantenWOffeneHoechstens || jePraeger >= quantenWOffeneJePraeger {
		return quantenWFehler("zu viele offene Auftraege")
	}
	return nil
}

// abschnittAnteil: Anteil des Validators xv an Stelle Von+i (Polynom).
func quantenSAnteilAus(koeff []string, xv int) (*big.Int, error) {
	y := new(big.Int)
	for j := len(koeff) - 1; j >= 0; j-- {
		c, ok := new(big.Int).SetString(koeff[j], 16)
		if !ok {
			return nil, errors.New("Tresor: Koeffizient")
		}
		y.Mul(y, big.NewInt(int64(xv))).Add(y, c).Mod(y, quantenWP)
	}
	return y, nil
}

func (st *quantenSAbschnittStand) anteileFuer(xv int) ([]string, error) {
	out := make([]string, len(st.Koeff))
	for i, k := range st.Koeff {
		y, err := quantenSAnteilAus(k, xv)
		if err != nil {
			return nil, err
		}
		out[i] = y.Text(16)
	}
	return out, nil
}

func (st *quantenSAbschnittStand) wert(i int) (*big.Int, error) {
	w, ok := new(big.Int).SetString(st.Koeff[i][0], 16)
	if !ok {
		return nil, errors.New("Tresor: Wert")
	}
	return w, nil
}

// abschnittAntwort: Blaetter, x fuer die Inhaberin, Buendel; unterschrieben.
func (d *quantenWDienst) abschnittAntwort(e *quantenWTresorEintrag) (QuantenSAbschnittAntwort, error) {
	st := e.Abschnitt
	a := QuantenSAbschnittAntwort{K: e.K, X: e.X, Von: st.Von, Bis: st.Bis, Validator: d.adresse}
	for i := range st.Koeff {
		w, err := st.wert(i)
		if err != nil {
			return a, err
		}
		b := quantenWPruefsumme(e.K, st.Von+i, w)
		a.Blaetter = append(a.Blaetter, hex.EncodeToString(b[:]))
	}
	pubI, err := quantenSSchluesselPruefen(st.InhaberinSchl, st.Inhaberin)
	if err != nil {
		return a, err
	}
	fi, err := json.Marshal(map[string]interface{}{"dom": quantenSDomFuerInhaberin, "k": e.K, "von": st.Von, "bis": st.Bis, "x": st.X})
	if err != nil {
		return a, err
	}
	c, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(pubI), fi, nil, nil)
	if err != nil {
		return a, err
	}
	a.FuerInhaberin = hex.EncodeToString(c)
	for xv := 1; xv <= len(e.Validatoren); xv++ {
		if xv == e.X {
			a.Buendel, a.BuendelSig = append(a.Buendel, ""), append(a.BuendelSig, "")
			continue
		}
		pub, err := quantenSSchluesselPruefen(st.Schluessel[xv-1], e.Validatoren[xv-1])
		if err != nil {
			return a, err
		}
		an, err := st.anteileFuer(xv)
		if err != nil {
			return a, err
		}
		ch, sg, err := quantenSBuendelVersiegeln(d.key, pub, quantenSBuendelInhalt{K: e.K, Von: st.Von, Bis: st.Bis, Praeparator: e.X, Empfaenger: xv, Anteile: an})
		if err != nil {
			return a, err
		}
		a.Buendel, a.BuendelSig = append(a.Buendel, ch), append(a.BuendelSig, sg)
	}
	a.Sig, err = quantenSUnterschreiben(d.key, quantenSAbschnittAntwortNachricht(a))
	return a, err
}

// quantenSFuerInhaberinOeffnen: die Inhaberin liest x ihres Abschnitts.
func quantenSFuerInhaberinOeffnen(schl *ecdsa.PrivateKey, chiffrat, K string, von, bis int) ([]int, error) {
	if len(chiffrat) == 0 || len(chiffrat) > 2*(quantenWNMax+4096) || !kleinHex(chiffrat) {
		return nil, quantenWFehler("fuer die Inhaberin: Form")
	}
	c, _ := hex.DecodeString(chiffrat)
	roh, err := ecies.ImportECDSA(schl).Decrypt(c, nil, nil)
	if err != nil {
		return nil, quantenWFehler("fuer die Inhaberin: nicht fuer diesen Schluessel")
	}
	var in struct {
		Dom string `json:"dom"`
		K   string `json:"k"`
		Von int    `json:"von"`
		Bis int    `json:"bis"`
		X   string `json:"x"`
	}
	dec := json.NewDecoder(bytes.NewReader(roh))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || in.Dom != quantenSDomFuerInhaberin || in.K != K || in.Von != von || in.Bis != bis {
		return nil, quantenWFehler("fuer die Inhaberin: passt nicht")
	}
	x, ok := quantenSBitsLesen(in.X, bis-von)
	if !ok {
		return nil, quantenWFehler("fuer die Inhaberin: x")
	}
	return x, nil
}

// ------------------------------------------------------------ Oeffnung

// oeffnungFuerStelle: die Werte des eigenen Abschnitts mit Stellen-Bit b,
// verschluesselt fuer die Annahmestelle b aus dem Auftrag.
func (d *quantenWDienst) oeffnungFuerStelle(K string, b int) (string, error) {
	if b != 0 && b != 1 {
		return "", quantenWFehler("Stelle 0 oder 1")
	}
	e, da := d.eintragVon(K)
	if !da || e.Abschnitt == nil {
		return "", quantenWFehler("kein Abschnitt zu diesem K")
	}
	st := e.Abschnitt
	pub, err := quantenSSchluesselPruefen(st.StellenSchluessel[b], st.Stellen[b])
	if err != nil {
		return "", err
	}
	werte := map[string]string{}
	for i := range st.Koeff {
		w, err := st.wert(i)
		if err != nil {
			return "", err
		}
		if quantenSStellenBit(w) == b {
			werte[strconv.Itoa(st.Von+i)] = w.Text(16)
		}
	}
	roh, err := json.Marshal(map[string]interface{}{"dom": quantenSDomOeffnung, "k": K, "b": b, "x": e.X, "von": st.Von, "bis": st.Bis, "werte": werte})
	if err != nil {
		return "", err
	}
	c, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(pub), roh, nil, nil)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(c), nil
}

// quantenSOeffnungEntschluesseln: die Annahmestelle liest die Oeffnung von
// Validator x (Abschnitt [von, bis)); jede Stelle muss darin liegen.
func quantenSOeffnungEntschluesseln(schl *ecdsa.PrivateKey, chiffrat, K string, b, x, von, bis int) (map[int]*big.Int, error) {
	if len(chiffrat) == 0 || len(chiffrat) > quantenSBuendelHoechstens || !kleinHex(chiffrat) {
		return nil, quantenWFehler("Oeffnung: Form")
	}
	c, _ := hex.DecodeString(chiffrat)
	roh, err := ecies.ImportECDSA(schl).Decrypt(c, nil, nil)
	if err != nil {
		return nil, quantenWFehler("Oeffnung: nicht fuer diese Stelle")
	}
	var in struct {
		Dom   string            `json:"dom"`
		K     string            `json:"k"`
		B     int               `json:"b"`
		X     int               `json:"x"`
		Von   int               `json:"von"`
		Bis   int               `json:"bis"`
		Werte map[string]string `json:"werte"`
	}
	dec := json.NewDecoder(bytes.NewReader(roh))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || in.Dom != quantenSDomOeffnung || in.K != K || in.B != b || in.X != x || in.Von != von || in.Bis != bis {
		return nil, quantenWFehler("Oeffnung: passt nicht")
	}
	out := map[int]*big.Int{}
	for si, wh := range in.Werte {
		i, err := strconv.Atoi(si)
		if err != nil || i < von || i >= bis || len(wh) == 0 || len(wh) > 33 || !kleinHex(wh) {
			return nil, quantenWFehler("Oeffnung: Stelle %q", si)
		}
		w, ok := new(big.Int).SetString(wh, 16)
		if !ok {
			return nil, quantenWFehler("Oeffnung: Wert %q", si)
		}
		out[i] = w
	}
	return out, nil
}

// ------------------------------------------------------------ Bestaetigen

// bestaetigenAbschnitt: der wartende Eintrag wird mit den Buendeln der anderen
// vollstaendig, dann dieselbe Bestaetigung wie bei der Ausgabe mit einem
// Auftrag. Fail-closed: kein wartender Eintrag, anderer Auftrag, ein Buendel
// nicht vom Praeparator oder nicht fuer diesen Validator, eigene Blaetter
// anders, Blaetter ergeben nicht die Wurzel.
func (d *quantenWDienst) bestaetigenAbschnitt(q QuantenWBestaetigenAnfrage) (string, string, error) {
	q.K = strings.ToLower(strings.TrimSpace(q.K))
	if !istHash64(q.K) || q.Art != "s" || len(q.Anteile) != 0 {
		return "", "", quantenWFehler("Abschnitt: K, Art oder Anteile")
	}
	e, da := d.eintragVon(q.K)
	if !da || e.Abschnitt == nil {
		return "", "", quantenWFehler("Abschnitt: kein Abschnitt zu diesem K")
	}
	v := len(e.Validatoren)
	q.Praeger = strings.ToLower(strings.TrimSpace(q.Praeger))
	if q.N != e.N || q.T != e.T || q.Tol != e.Tol || q.X != e.X || q.Zeit != e.Zeit || q.Praeger != e.Praeger ||
		!slices.Equal(q.Validatoren, e.Validatoren) || len(q.Buendel) != v || len(q.BuendelSig) != v || len(q.Blaetter) != q.N {
		return "", "", quantenWFehler("Abschnitt: passt nicht zum Auftrag")
	}
	var proben [2][32]byte
	ok := len(q.Proben) == 2
	for i := 0; ok && i < 2; i++ {
		proben[i], ok = quantenWHex32(q.Proben[i])
	}
	wurzel, okW := quantenWHex32(q.Wurzel)
	if !ok || !okW {
		return "", "", quantenWFehler("Abschnitt: Stichproben-Bindungen oder Wurzel")
	}
	if !kanonischeSignatur(q.PraegerSig) || verifyPersonalSign(quantenWDienstAuftrag(q), q.PraegerSig, q.Praeger) != nil {
		return "", "", quantenWFehler("Abschnitt: Bestaetigung nicht vom Ausgeber beauftragt")
	}
	if !e.Wartend {
		// Schon bestaetigt: derselbe Auftrag bekommt dieselbe Bestaetigung.
		if e.Wurzel != q.Wurzel || !slices.Equal(e.Blaetter, q.Blaetter) || !slices.Equal(e.Proben, q.Proben) {
			return "", "", quantenWFehler("Abschnitt: schon bestaetigt (anderer Auftrag)")
		}
		return d.bestaetigungSUnterschreiben(q, wurzel, proben)
	}
	blaetter := make([][32]byte, q.N)
	for i, h := range q.Blaetter {
		b, ok := quantenWHex32(h)
		if !ok {
			return "", "", quantenWFehler("Abschnitt: Blatt %d", i)
		}
		blaetter[i] = b
	}
	if quantenWMerkleWurzel(blaetter) != wurzel {
		return "", "", quantenWFehler("Abschnitt: Blaetter ergeben nicht die Wurzel")
	}
	st := e.Abschnitt
	eigen, err := st.anteileFuer(e.X)
	if err != nil {
		return "", "", err
	}
	anteile := make([]string, q.N)
	for i := range st.Koeff {
		w, err := st.wert(i)
		if err != nil {
			return "", "", err
		}
		if quantenWPruefsumme(q.K, st.Von+i, w) != blaetter[st.Von+i] {
			return "", "", quantenWFehler("Abschnitt: eigenes Blatt %d anders", st.Von+i)
		}
		anteile[st.Von+i] = eigen[i]
	}
	for xv := 1; xv <= v; xv++ {
		if xv == e.X {
			if q.Buendel[xv-1] != "" || q.BuendelSig[xv-1] != "" {
				return "", "", quantenWFehler("Abschnitt: Buendel an sich selbst")
			}
			continue
		}
		von, bis, _ := quantenSAbschnitt(q.N, v, xv)
		an, err := quantenSBuendelOeffnen(d.key, q.Buendel[xv-1], q.BuendelSig[xv-1], e.Validatoren[xv-1], q.K, xv, e.X, von, bis)
		if err != nil {
			return "", "", err
		}
		for i, y := range an {
			anteile[von+i] = y.Text(16)
		}
	}
	for i, y := range anteile {
		if y == "" {
			return "", "", quantenWFehler("Abschnitt: Anteil %d fehlt", i)
		}
	}
	if _, err := neuerQuantenSToken(q.N, d.adresse, [32]byte{}, 1, proben, q.T, v, q.Tol); err != nil {
		return "", "", err
	}
	neu := *e
	neu.Wartend, neu.Wurzel, neu.Anteile, neu.Blaetter, neu.Proben = false, q.Wurzel, anteile, slices.Clone(q.Blaetter), slices.Clone(q.Proben)
	roh, err := d.versiegeln(&neu)
	if err != nil {
		return "", "", err
	}
	d.mu.Lock()
	if cur := d.eintraege[q.K]; cur != e {
		d.mu.Unlock()
		return "", "", quantenWFehler("Abschnitt: gleichzeitig geaendert")
	}
	if err := d.ablage.setzen(q.K, roh); err != nil {
		d.mu.Unlock()
		return "", "", err
	}
	d.eintraege[q.K] = &neu
	d.mu.Unlock()
	return d.bestaetigungSUnterschreiben(q, wurzel, proben)
}

// ------------------------------------------------------------ Endpunkte

func (a *APIServer) handleQuantenSAbschnitt(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
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
	var q QuantenSAbschnittAnfrage
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, quantenSAbschnittKoerperMax))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&q); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	out, err := d.abschnittPraeparieren(q)
	if err != nil {
		if errors.Is(err, errQuantenW) {
			jsonError(w, "rejected: "+err.Error(), http.StatusConflict)
			return
		}
		fmt.Printf("[quanten_s] Abschnitt gescheitert: %v\n", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(out)
}

func (a *APIServer) handleQuantenSOeffnung(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
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
	b, err := strconv.Atoi(r.URL.Query().Get("b"))
	if !istHash64(K) || err != nil {
		jsonError(w, "k, b", http.StatusBadRequest)
		return
	}
	c, err := d.oeffnungFuerStelle(K, b)
	if err != nil {
		jsonError(w, "rejected: "+err.Error(), http.StatusConflict)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"k": K, "b": b, "validator": d.adresse, "oeffnung": c})
}
