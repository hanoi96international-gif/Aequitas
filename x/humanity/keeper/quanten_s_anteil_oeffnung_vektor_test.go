package keeper

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// PRUEFVEKTOR FUER DIE APP: OEFFNUNG AUS t ANTEILEN (DEZENTRAL.md 17, R9).
//
// Die App (Aequitas-App, lib/quantenSStelle.ts) muss die Pakete genau so lesen
// und zusammensetzen wie quantenSAusAnteilenLaden: Paket fuer diese Stelle
// (ECIES wie go-ethereum) streng lesen, aus t Paketen Lagrange ueber
// F_P (P = 2^127 - 1), jeden Wert gegen sein Blatt und sein Stellen-Bit.
// Form: v = 4, t = 3, Praeparator 2 faellt aus, Pakete von 1, 3 und 4; dazu
// ein Paket fuer die andere Stelle (muss abgewiesen werden) und eines mit
// einem falschen Anteil (dann ergibt die Teilmenge nicht die Blaetter).
//
// Fest sind Schluessel (nur Tests!), Werte und Koeffizienten; die Chiffrate
// sind zufaellig, einmal erzeugt und gespeichert. Neu schreiben:
// AEQUITAS_VEKTOR_SCHREIBEN=1 go test -run TestQuantenSAnteilOeffnungVektor .
// Danach nach Aequitas-App/lib/__tests__/vektoren/ kopieren.

const qsaoVektorPfad = "testdata/quanten_s_anteil_oeffnung_vektor.json"

type qsaoVektor struct {
	Hinweis          string            `json:"hinweis"`
	StelleSchluessel string            `json:"stelle_schluessel_nur_test"`
	K                string            `json:"k"`
	B                int               `json:"b"`
	N                int               `json:"n"`
	T                int               `json:"t"`
	V                int               `json:"v"`
	Praeparator      int               `json:"praeparator"`
	Von              int               `json:"von"`
	Bis              int               `json:"bis"`
	Salz             string            `json:"salz"`
	Werte            map[string]string `json:"werte"`    // Stelle -> Wert (hex), Soll nach dem Zusammensetzen
	Blaetter         map[string]string `json:"blaetter"` // Stelle -> Blatt (hex), fuer alle Stellen des Abschnitts
	Pakete           map[string]string `json:"pakete"`   // Validator j -> Paket fuer Stelle b
	PaketAndere      string            `json:"paket_andere_stelle"`
	PaketFalsch      string            `json:"paket_falscher_anteil"` // von Validator 4, ein Anteil um 1 verschoben
	PaketGross       string            `json:"paket_anteil_zu_gross"` // von Validator 3, ein Anteil = P (nicht in F_P)
	// An diese Stelle verschluesselt, aber mit den Stellen der ANDEREN Stelle
	// (ein boeser Praeparator): die Werte passen zu ihren Blaettern und muessen
	// trotzdem abgewiesen werden.
	PaketeAndereBits map[string]string `json:"pakete_andere_bits"`
}

func qsaoVektorBauen(t *testing.T, gespeichert *qsaoVektor) qsaoVektor {
	t.Helper()
	const n, v, tt, x, b = 16, 4, 3, 2, 1
	stelleKey, _ := crypto.HexToECDSA(strings.Repeat("81", 32))
	andereKey, _ := crypto.HexToECDSA(strings.Repeat("82", 32))
	von, bis, _ := quantenSAbschnitt(n, v, x)
	g := qsaoVektor{Hinweis: "nur fuer Tests: feste Schluessel und Werte, nie fuer echte Werte", StelleSchluessel: strings.Repeat("81", 32),
		K: strings.Repeat("7a", 32), B: b, N: n, T: tt, V: v, Praeparator: x, Von: von, Bis: bis, Salz: strings.Repeat("3c", 32),
		Werte: map[string]string{}, Blaetter: map[string]string{}, Pakete: map[string]string{}}
	st := &qssStrom{zaehler: 900}
	zahl := func(bits int) *big.Int {
		z := new(big.Int)
		for i := 0; i < (bits+7)/8; i++ {
			z.Lsh(z, 8).Or(z, big.NewInt(int64(st.byte_())))
		}
		return z.Rsh(z, uint((bits+7)/8*8-bits))
	}
	// Werte w = Basis + 2 Bit + 4 (Stellen-Bit + 2 Salz), Stellen-Bits abwechselnd.
	koeff := map[int][]*big.Int{}
	for i := von; i < bis; i++ {
		salz := zahl(quantenWSalzBits - 1)
		sb := (i - von) % 2
		w := new(big.Int).Lsh(salz, 1)
		w.Add(w, big.NewInt(int64(sb))).Lsh(w, 2).Add(w, big.NewInt(int64(st.bit()+2*st.bit())))
		bl := quantenWPruefsumme(g.K, i, w)
		g.Blaetter[strconv.Itoa(i)] = hex.EncodeToString(bl[:])
		k := []*big.Int{w}
		for j := 1; j < tt; j++ {
			k = append(k, new(big.Int).Mod(zahl(127), quantenWP))
		}
		koeff[i] = k
		if sb == b {
			g.Werte[strconv.Itoa(i)] = w.Text(16)
		}
	}
	anteil := func(i, j int) *big.Int {
		y := new(big.Int)
		for c := len(koeff[i]) - 1; c >= 0; c-- {
			y.Mul(y, big.NewInt(int64(j))).Add(y, koeff[i][c]).Mod(y, quantenWP)
		}
		return y
	}
	inhalt := func(j, bb int, verschieben bool) quantenSAnteilOeffnungInhalt {
		an := map[string]string{}
		erst := true
		for i := von; i < bis; i++ {
			if (i-von)%2 != bb {
				continue
			}
			y := anteil(i, j)
			if verschieben && erst {
				y = new(big.Int).Mod(new(big.Int).Add(y, big.NewInt(1)), quantenWP)
				erst = false
			}
			an[strconv.Itoa(i)] = y.Text(16)
		}
		return quantenSAnteilOeffnungInhalt{K: g.K, B: bb, Praeparator: x, Empfaenger: j, Von: von, Bis: bis, Anteile: an, Salz: g.Salz}
	}
	if gespeichert != nil {
		g.Pakete, g.PaketAndere, g.PaketFalsch, g.PaketGross = gespeichert.Pakete, gespeichert.PaketAndere, gespeichert.PaketFalsch, gespeichert.PaketGross
		g.PaketeAndereBits = gespeichert.PaketeAndereBits
		return g
	}
	for _, j := range []int{1, 3, 4} {
		c, err := quantenSAnteilOeffnungVersiegeln(&stelleKey.PublicKey, inhalt(j, b, false))
		if err != nil {
			t.Fatal(err)
		}
		g.Pakete[strconv.Itoa(j)] = c
	}
	var err error
	if g.PaketAndere, err = quantenSAnteilOeffnungVersiegeln(&andereKey.PublicKey, inhalt(3, b, false)); err != nil {
		t.Fatal(err)
	}
	if g.PaketFalsch, err = quantenSAnteilOeffnungVersiegeln(&stelleKey.PublicKey, inhalt(4, b, true)); err != nil {
		t.Fatal(err)
	}
	g.PaketeAndereBits = map[string]string{}
	for _, j := range []int{1, 3, 4} {
		in := inhalt(j, 1-b, false)
		in.B = b // als Paket fuer Stelle b ausgegeben
		c, err := quantenSAnteilOeffnungVersiegeln(&stelleKey.PublicKey, in)
		if err != nil {
			t.Fatal(err)
		}
		g.PaketeAndereBits[strconv.Itoa(j)] = c
	}
	gross := inhalt(3, b, false)
	for i := range gross.Anteile {
		gross.Anteile[i] = quantenWP.Text(16)
		break
	}
	if g.PaketGross, err = quantenSAnteilOeffnungVersiegeln(&stelleKey.PublicKey, gross); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestQuantenSAnteilOeffnungVektor(t *testing.T) {
	if os.Getenv("AEQUITAS_VEKTOR_SCHREIBEN") == "1" {
		w := qsaoVektorBauen(t, nil)
		roh, _ := json.MarshalIndent(w, "", " ")
		if err := os.WriteFile(qsaoVektorPfad, append(roh, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	roh, err := os.ReadFile(qsaoVektorPfad)
	if err != nil {
		t.Fatal(err)
	}
	var g qsaoVektor
	if err := json.Unmarshal(roh, &g); err != nil {
		t.Fatal(err)
	}
	soll := qsaoVektorBauen(t, &g)
	a, _ := json.Marshal(soll)
	bb, _ := json.Marshal(g)
	if string(a) != string(bb) {
		t.Fatal("Vektor weicht von dem ab, was der Knoten rechnet")
	}
	key, _ := crypto.HexToECDSA(g.StelleSchluessel)
	blaetter := make([][32]byte, g.N)
	for si, h := range g.Blaetter {
		i, _ := strconv.Atoi(si)
		blaetter[i], _ = quantenWHex32(h)
	}
	lesen := func(j int, c string) quantenSAnteilPaket {
		an, salz, err := quantenSAnteilOeffnungLesen(key, c, g.K, g.B, g.Praeparator, j, g.Von, g.Bis)
		if err != nil {
			t.Fatalf("Paket von %d: %v", j, err)
		}
		return quantenSAnteilPaket{J: j, Anteile: an, Salz: salz}
	}
	var pakete []quantenSAnteilPaket
	for sj, c := range g.Pakete {
		j, _ := strconv.Atoi(sj)
		pakete = append(pakete, lesen(j, c))
	}
	sort.Slice(pakete, func(a, b int) bool { return pakete[a].J < pakete[b].J })
	pruefen := func(werte map[int]*big.Int, _ [32]byte) error {
		for i, w := range werte {
			if quantenWPruefsumme(g.K, i, w) != blaetter[i] || quantenSStellenBit(w) != g.B {
				return quantenWFehler("Stelle %d", i)
			}
		}
		return nil
	}
	werte, salz, err := quantenSAusAnteilenZusammensetzen(pakete, g.T, pruefen)
	if err != nil || hex.EncodeToString(salz[:]) != g.Salz || len(werte) != len(g.Werte) {
		t.Fatalf("Zusammensetzen: %v", err)
	}
	for i, w := range werte {
		if g.Werte[strconv.Itoa(i)] != w.Text(16) {
			t.Fatalf("Stelle %d: %s, Soll %s", i, w.Text(16), g.Werte[strconv.Itoa(i)])
		}
	}
	if _, _, err := quantenSAnteilOeffnungLesen(key, g.PaketAndere, g.K, g.B, g.Praeparator, 3, g.Von, g.Bis); err == nil {
		t.Fatal("Paket fuer die andere Stelle gelesen")
	}
	if _, _, err := quantenSAnteilOeffnungLesen(key, g.PaketGross, g.K, g.B, g.Praeparator, 3, g.Von, g.Bis); err == nil {
		t.Fatal("Anteil = P gelesen")
	}
	falsch := []quantenSAnteilPaket{pakete[0], pakete[1], lesen(4, g.PaketFalsch)}
	if _, _, err := quantenSAusAnteilenZusammensetzen(falsch, g.T, pruefen); err == nil {
		t.Fatal("mit einem falschen Anteil zusammengesetzt")
	}
}

// qsaoVektorNetz: liefert nur die Pakete des Vektors (alles andere bleibt
// unbenutzt).
type qsaoVektorNetz struct {
	quantenSAbschnittNetz
	pakete map[string]string
}

func (n qsaoVektorNetz) anteilOeffnung(_ context.Context, j, _ int, _ string, _ int) (string, error) {
	if c, ok := n.pakete[strconv.Itoa(j)]; ok {
		return c, nil
	}
	return "", quantenWFehler("nicht erreichbar")
}

// Der Weg der Stelle (quantenSAusAnteilenLaden) mit den Paketen des Vektors:
// ehrlich geladen; Pakete mit den Stellen der anderen Stelle abgewiesen,
// obwohl jeder Wert zu seinem Blatt passt.
func TestQuantenSAusAnteilenLaden_Vektor(t *testing.T) {
	roh, err := os.ReadFile(qsaoVektorPfad)
	if err != nil {
		t.Fatal(err)
	}
	var g qsaoVektor
	if err := json.Unmarshal(roh, &g); err != nil {
		t.Fatal(err)
	}
	key, _ := crypto.HexToECDSA(g.StelleSchluessel)
	blaetter := make([][32]byte, g.N)
	for si, h := range g.Blaetter {
		i, _ := strconv.Atoi(si)
		blaetter[i], _ = quantenWHex32(h)
	}
	werte, _, err := quantenSAusAnteilenLaden(context.Background(), qsaoVektorNetz{pakete: g.Pakete}, g.K, g.B, g.Praeparator, g.T, g.V, g.Von, g.Bis, blaetter, key)
	if err != nil || len(werte) != len(g.Werte) {
		t.Fatalf("ehrlich: %v", err)
	}
	_, _, err = quantenSAusAnteilenLaden(context.Background(), qsaoVektorNetz{pakete: g.PaketeAndereBits}, g.K, g.B, g.Praeparator, g.T, g.V, g.Von, g.Bis, blaetter, key)
	if err == nil || !strings.Contains(err.Error(), "anderen Annahmestelle") {
		t.Fatalf("Stellen der anderen Stelle: %v", err)
	}
}
