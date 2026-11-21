package keeper

// QUANTENGELD VARIANTE S -- OEFFNUNG AUS t ANTEILEN (R9, DEZENTRAL.md 17)
//
// WARUM: Bei Stichprobe v2 laedt eine Annahmestelle ihre Stichprobe aus der
// Oeffnung JEDES praeparierenden Validators (oeffnungFuerStelle). Nur er kennt
// die Werte seines Abschnitts und welche Stelle zu welcher Annahmestelle
// gehoert (Stellen-Bit). Faellt er nach der Bindung und vor dem Laden aus,
// wartet die Stelle -- die Anteile der anderen reichten bisher nur zum
// Offenlegen auf der Kette.
//
// WIE: Der Praeparator x legt in sein Buendel an jeden anderen Validator j je
// Annahmestelle b ein Paket: die Anteile von j an GENAU den Stellen mit
// Stellen-Bit b, dazu das Salz des Teil-Hashes von b -- verschluesselt an
// den Schluessel der Annahmestelle b. j kann das Paket nicht lesen und
// erfaehrt so weder Werte noch, welche Stellen zu welcher Stelle gehoeren
// (die Aufteilung bleibt bis zur Bindung geheim, Befund 5i). j haelt es im
// Tresor und gibt es erst heraus, wenn Bindung und Wahl auf der Kette final
// sind -- dieselbe Pruefung wie der Praeparator selbst (oeffnungFuerStelle),
// jeder Validator fuer sich. Die Stelle setzt aus t Paketen die Werte
// zusammen (Lagrange) und prueft jeden gegen sein Blatt und die Wurzel
// (quantenSOeffnungLaden) -- ein falscher Anteil faellt dort auf.
//
// GRENZEN DES MODELLS: wie bisher. Wer t Validatoren hat, kennt ohnehin alles.
// Weniger als t, die frueh herausgeben, liefern weniger als t Anteile und
// damit nichts. Bei v = 1 (Hauptnetz heute) gibt es keine anderen
// Validatoren -- dann bleibt nur der Praeparator.

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto/ecies"
)

const (
	quantenSDomAnteilOeffnung = "aequitas-quanten-s-anteil-oeffnung-v1"
	// Ein Paket traegt hoechstens die Anteile eines ganzen Abschnitts
	// (hoechstens quantenWNMax Stellen, je Stelle "nummer":"anteil" bis 44
	// Zeichen), dazu Felder, Salz und den Aufschlag von ECIES; als Hex doppelt.
	quantenSAnteilOeffnungHoechstens = 2 * (quantenWNMax*44 + 4096)
	// Ein Buendel mit den zwei Paketen: Anteile wie bisher und beide Pakete
	// als Hex-Text im Inhalt, verschluesselt und als Hex doppelt so lang.
	quantenSBuendelMitOeffnungHoechstens = 2 * (quantenWNMax*36 + 2*quantenSAnteilOeffnungHoechstens + 2*4096)
	// So viele Teilmengen von t Paketen probiert die Stelle hoechstens, wenn
	// ein Paket falsch ist (C(v-1, t) waechst schnell; ehrliche Validatoren
	// brauchen genau eine).
	quantenSAnteilOeffnungVersuche = 64
)

// quantenSAnteilOeffnungInhalt: das Paket von Praeparator x fuer Validator j
// an Annahmestelle b.
type quantenSAnteilOeffnungInhalt struct {
	Dom         string            `json:"dom"`
	K           string            `json:"k"`
	B           int               `json:"b"`
	Praeparator int               `json:"praeparator"`
	Empfaenger  int               `json:"empfaenger"`
	Von         int               `json:"von"`
	Bis         int               `json:"bis"`
	Anteile     map[string]string `json:"anteile"` // Stelle (dezimal) -> Anteil von j (hex)
	Salz        string            `json:"salz"`    // Salz des Teil-Hashes von b (hex, 32 Byte)
}

// quantenSAnteilOeffnungVersiegeln: Paket fuer Stelle b verschluesseln.
func quantenSAnteilOeffnungVersiegeln(stelle *ecdsa.PublicKey, in quantenSAnteilOeffnungInhalt) (string, error) {
	in.Dom = quantenSDomAnteilOeffnung
	roh, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	c, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(stelle), roh, nil, nil)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(c), nil
}

// quantenSAnteilOeffnungFormOk: was j ohne Schluessel pruefen kann -- nur
// Form und Groesse (den Inhalt kann es nicht lesen).
func quantenSAnteilOeffnungFormOk(c string) bool {
	return len(c) > 0 && len(c) <= quantenSAnteilOeffnungHoechstens && kleinHex(c)
}

// quantenSAnteilOeffnungLesen: die Stelle b liest das Paket von Validator j
// zum Abschnitt [von, bis) von Praeparator x. Fail-closed: fremder Schluessel,
// anderes K, andere Stelle, anderer Praeparator oder Empfaenger, Stelle
// ausserhalb des Abschnitts, Anteil nicht in F_P, Salz falsch geformt.
func quantenSAnteilOeffnungLesen(schl *ecdsa.PrivateKey, c, K string, b, x, j, von, bis int) (map[int]*big.Int, [32]byte, error) {
	if !quantenSAnteilOeffnungFormOk(c) {
		return nil, [32]byte{}, quantenWFehler("Paket: Form oder Groesse")
	}
	roh, _ := hex.DecodeString(c)
	klar, err := ecies.ImportECDSA(schl).Decrypt(roh, nil, nil)
	if err != nil {
		return nil, [32]byte{}, quantenWFehler("Paket: nicht fuer diese Stelle")
	}
	var in quantenSAnteilOeffnungInhalt
	dec := json.NewDecoder(bytes.NewReader(klar))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || dec.More() {
		return nil, [32]byte{}, quantenWFehler("Paket: Inhalt unlesbar")
	}
	if in.Dom != quantenSDomAnteilOeffnung || in.K != K || in.B != b || in.Praeparator != x || in.Empfaenger != j || in.Von != von || in.Bis != bis {
		return nil, [32]byte{}, quantenWFehler("Paket: passt nicht zu K, Stelle, Praeparator oder Validator")
	}
	salz, ok := quantenWHex32(in.Salz)
	if !ok {
		return nil, [32]byte{}, quantenWFehler("Paket: Salz")
	}
	out := make(map[int]*big.Int, len(in.Anteile))
	for si, yh := range in.Anteile {
		i, err := strconv.Atoi(si)
		if err != nil || strconv.Itoa(i) != si || i < von || i >= bis || len(yh) == 0 || len(yh) > 32 || !kleinHex(yh) {
			return nil, [32]byte{}, quantenWFehler("Paket: Stelle %q", si)
		}
		y, ok := new(big.Int).SetString(yh, 16)
		if !ok || y.Cmp(quantenWP) >= 0 {
			return nil, [32]byte{}, quantenWFehler("Paket: Anteil an Stelle %d", i)
		}
		out[i] = y
	}
	return out, salz, nil
}

// quantenSAnteilPaket: ein gelesenes Paket von Validator J.
type quantenSAnteilPaket struct {
	J       int
	Anteile map[int]*big.Int
	Salz    [32]byte
}

// quantenSAusAnteilenZusammensetzen: aus mindestens t Paketen die Werte der
// Stelle zusammensetzen. pruefen bekommt die zusammengesetzten Werte und sagt,
// ob sie stimmen (Blaetter, Wurzel, Stellen-Bit). Ehrliche Pakete brauchen
// genau einen Versuch; ist eines falsch, probiert die Stelle weitere
// Teilmengen, hoechstens quantenSAnteilOeffnungVersuche.
func quantenSAusAnteilenZusammensetzen(pakete []quantenSAnteilPaket, t int, pruefen func(map[int]*big.Int, [32]byte) error) (map[int]*big.Int, [32]byte, error) {
	if t < 1 || len(pakete) < t {
		return nil, [32]byte{}, quantenWFehler("Pakete: %d, verlangt %d", len(pakete), t)
	}
	sort.Slice(pakete, func(a, b int) bool { return pakete[a].J < pakete[b].J })
	for i := 1; i < len(pakete); i++ {
		if pakete[i].J == pakete[i-1].J {
			return nil, [32]byte{}, quantenWFehler("Pakete: Validator %d doppelt", pakete[i].J)
		}
	}
	auswahl := make([]int, t)
	for i := range auswahl {
		auswahl[i] = i
	}
	var letzter error
	for versuch := 0; versuch < quantenSAnteilOeffnungVersuche; versuch++ {
		werte, salz, err := quantenSTeilmengeZusammensetzen(pakete, auswahl)
		if err == nil {
			if err = pruefen(werte, salz); err == nil {
				return werte, salz, nil
			}
		}
		letzter = err
		// naechste Teilmenge (lexikographisch)
		i := t - 1
		for i >= 0 && auswahl[i] == len(pakete)-t+i {
			i--
		}
		if i < 0 {
			break
		}
		auswahl[i]++
		for k := i + 1; k < t; k++ {
			auswahl[k] = auswahl[k-1] + 1
		}
	}
	return nil, [32]byte{}, quantenWFehler("keine Teilmenge von %d Paketen ergibt die Stichprobe: %v", t, letzter)
}

func quantenSTeilmengeZusammensetzen(pakete []quantenSAnteilPaket, auswahl []int) (map[int]*big.Int, [32]byte, error) {
	erstes := pakete[auswahl[0]]
	for _, a := range auswahl[1:] {
		p := pakete[a]
		if p.Salz != erstes.Salz || len(p.Anteile) != len(erstes.Anteile) {
			return nil, [32]byte{}, quantenWFehler("Pakete von %d und %d nennen andere Stellen oder ein anderes Salz", erstes.J, p.J)
		}
		for i := range erstes.Anteile {
			if _, da := p.Anteile[i]; !da {
				return nil, [32]byte{}, quantenWFehler("Pakete von %d und %d nennen andere Stellen", erstes.J, p.J)
			}
		}
	}
	werte := make(map[int]*big.Int, len(erstes.Anteile))
	an := make([]quantenWAnteil, len(auswahl))
	for i := range erstes.Anteile {
		for k, a := range auswahl {
			an[k] = quantenWAnteil{X: pakete[a].J, Y: pakete[a].Anteile[i]}
		}
		werte[i] = quantenWZusammensetzen(an)
	}
	return werte, erstes.Salz, nil
}

// ------------------------------------------------------------ Dienst

// anteilOeffnungPakete: beim Praeparieren die zwei Pakete fuer Validator xv
// (nur Stichprobe v2). Die Anteile kommen aus denselben Koeffizienten wie das
// Buendel.
func (st *quantenSAbschnittStand) anteilOeffnungPakete(K string, praeparator, xv int) ([]string, error) {
	if st.ProbeArt != 2 {
		return nil, nil
	}
	if len(st.Salze) != 2 || len(st.Stellen) != 2 || len(st.StellenSchluessel) != 2 {
		return nil, fmt.Errorf("Tresor: Salze oder Stellen der Stichprobe v2")
	}
	var out []string
	for b := 0; b < 2; b++ {
		pub, err := quantenSSchluesselPruefen(st.StellenSchluessel[b], st.Stellen[b])
		if err != nil {
			return nil, err
		}
		an := map[string]string{}
		for i, k := range st.Koeff {
			w, err := st.wert(i)
			if err != nil {
				return nil, err
			}
			if quantenSStellenBit(w) != b {
				continue
			}
			y, err := quantenSAnteilAus(k, xv)
			if err != nil {
				return nil, err
			}
			an[strconv.Itoa(st.Von+i)] = y.Text(16)
		}
		c, err := quantenSAnteilOeffnungVersiegeln(pub, quantenSAnteilOeffnungInhalt{
			K: K, B: b, Praeparator: praeparator, Empfaenger: xv, Von: st.Von, Bis: st.Bis, Anteile: an, Salz: st.Salze[b],
		})
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// anteilOeffnungFuerStelle: das Paket, das Praeparator x diesem Validator
// fuer Stelle b mitgab -- erst, wenn Bindung und Wahl auf der Kette final
// sind (dieselbe Pruefung wie oeffnungFuerStelle, hier selbst gemacht).
func (d *quantenWDienst) anteilOeffnungFuerStelle(K string, x, b int) (string, int, error) {
	if b != 0 && b != 1 {
		return "", 0, quantenWFehler("Stelle 0 oder 1")
	}
	// Zuerst die Kette: vor der Bindung dieselbe Antwort, ob ein Paket im
	// Tresor liegt oder nicht.
	tok := d.cs.quantenWJetzt().Token[K]
	if tok == nil || tok.token == nil || tok.token.D == nil || tok.token.C == -1 {
		return "", 0, quantenWFehler("Paket erst nach Bindung und Wahl auf der Kette (Stichprobe v2)")
	}
	if !d.reif(quantenSBindungSicht(K, tok.token)) {
		return "", 0, quantenWFehler("Paket erst, wenn Bindung und Wahl final sind (Stichprobe v2)")
	}
	e, da := d.eintragVon(K)
	if !da || e.Wartend || e.Abschnitt == nil || e.Abschnitt.ProbeArt != 2 {
		return "", 0, quantenWFehler("kein Paket zu diesem K")
	}
	if x < 1 || x > len(e.Validatoren) || x == e.X {
		return "", 0, quantenWFehler("Praeparator %d: kein anderer Validator dieses Tokens", x)
	}
	p := e.AnteilOeffnung[strconv.Itoa(x)]
	if len(p) != 2 || !quantenSAnteilOeffnungFormOk(p[b]) {
		return "", 0, quantenWFehler("kein Paket von Praeparator %d", x)
	}
	return p[b], e.X, nil
}

func (a *APIServer) handleQuantenSAnteilOeffnung(w http.ResponseWriter, r *http.Request) {
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
	b, errB := strconv.Atoi(r.URL.Query().Get("b"))
	x, errX := strconv.Atoi(r.URL.Query().Get("x"))
	if !istHash64(K) || errB != nil || errX != nil {
		jsonError(w, "k, x, b", http.StatusBadRequest)
		return
	}
	c, j, err := d.anteilOeffnungFuerStelle(K, x, b)
	if err != nil {
		jsonError(w, "rejected: "+err.Error(), http.StatusConflict)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"k": K, "b": b, "praeparator": x, "j": j, "validator": d.adresse, "paket": c})
}

// ------------------------------------------------------------ Annahmestelle

// quantenSAusAnteilenLaden: Stelle s holt fuer den Abschnitt [von, bis) von
// Praeparator x die Pakete der anderen Validatoren und setzt die Werte aus
// t davon zusammen. Jeder Wert muss zu seinem Blatt passen und das
// Stellen-Bit s tragen; sonst wird die naechste Teilmenge probiert. Die
// Stichprobe als Ganzes (k_note, Bindung v2) prueft der Aufrufer wie bei der
// Oeffnung des Praeparators.
func quantenSAusAnteilenLaden(ctx context.Context, netz quantenSAbschnittNetz, K string, s, x, t, v, von, bis int, blaetter [][32]byte, key *ecdsa.PrivateKey) (map[int]*big.Int, [32]byte, error) {
	if t < 1 || v-1 < t {
		return nil, [32]byte{}, quantenWFehler("aus Anteilen: %d andere Validatoren, Schwelle %d", v-1, t)
	}
	var pakete []quantenSAnteilPaket
	var fehler []string
	for j := 1; j <= v; j++ {
		if j == x {
			continue
		}
		c, err := netz.anteilOeffnung(ctx, j, x, K, s)
		if err != nil {
			fehler = append(fehler, fmt.Sprintf("%d: %v", j, err))
			continue
		}
		an, salz, err := quantenSAnteilOeffnungLesen(key, c, K, s, x, j, von, bis)
		if err != nil {
			fehler = append(fehler, fmt.Sprintf("%d: %v", j, err))
			continue
		}
		pakete = append(pakete, quantenSAnteilPaket{J: j, Anteile: an, Salz: salz})
	}
	if len(pakete) < t {
		return nil, [32]byte{}, quantenWFehler("aus Anteilen: %d von %d Paketen (%s)", len(pakete), t, strings.Join(fehler, "; "))
	}
	grenze := new(big.Int).Lsh(big.NewInt(1), quantenWSalzBits+2)
	return quantenSAusAnteilenZusammensetzen(pakete, t, func(werte map[int]*big.Int, _ [32]byte) error {
		if len(werte) == 0 {
			return quantenWFehler("aus Anteilen: keine Stelle")
		}
		for i, w := range werte {
			if i < von || i >= bis || i >= len(blaetter) || w.Cmp(grenze) >= 0 || quantenWPruefsumme(K, i, w) != blaetter[i] {
				return quantenWFehler("aus Anteilen: Stelle %d passt nicht zu ihrem Blatt", i)
			}
			if quantenSStellenBit(w) != s {
				return quantenWFehler("aus Anteilen: Stelle %d gehoert zur anderen Annahmestelle", i)
			}
		}
		return nil
	})
}
