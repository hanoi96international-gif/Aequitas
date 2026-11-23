package keeper

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// PRUEFVEKTOR FUER B4b-3b (Antwort der App als Praeparatorin, Stichprobe v2).
//
// testdata/quanten_s_praep_antwort_vektor.json entsteht in der App
// (lib/quantenSPraeparieren.ts, lib/__tests__/quantenSPraeparieren.test.ts)
// mit festem Zufall. Der Knoten prueft sie mit seinem eigenen Code:
//   - Unterschrift der Antwort (quantenSAbschnittAntwortNachricht);
//   - fuer_inhaberin (quantenSFuerInhaberinOeffnen) ergibt x;
//   - jedes Buendel oeffnet der Empfaenger mit den zwei Paketen
//     (quantenSBuendelOeffnenPakete); die Stelle b liest jedes Paket
//     (quantenSAnteilOeffnungLesen), t Pakete ergeben genau die Werte mit
//     Stellen-Bit b, das Salz stimmt, der Teil-Hash ist der der Antwort;
//   - die Oeffnung fuer Stelle b (quantenSOeffnungLesen) ergibt dieselben
//     Werte und dasselbe Salz, und die Annahmestelle laedt sie gegen die
//     Blaetter (quantenSOeffnungLaden);
//   - die Zaehlung (Treffer, Fehler) stimmt mit y, u, t, x.
// Alle Schluessel sind nur fuer Tests.

type qspaVektor struct {
	Hinweis           string                   `json:"hinweis"`
	Schluessel        []string                 `json:"schluessel_nur_test"`
	Validatoren       []string                 `json:"validatoren"`
	StellenSchluessel []string                 `json:"stellen_schluessel_nur_test"`
	Stellen           []string                 `json:"stellen"`
	InhSchluessel     string                   `json:"inhaberin_schluessel_nur_test"`
	Inhaberin         string                   `json:"inhaberin"`
	K                 string                   `json:"k"`
	N                 int                      `json:"n"`
	V                 int                      `json:"v"`
	T                 int                      `json:"t"`
	X                 int                      `json:"x"`
	Von               int                      `json:"von"`
	Bis               int                      `json:"bis"`
	Y                 []int                    `json:"y"`
	U                 []int                    `json:"u"`
	TWerte            []int                    `json:"t_werte"`
	XMessung          []int                    `json:"x_messung"`
	Werte             []string                 `json:"werte"`
	Koeff             [][]string               `json:"koeff"`
	Salze             []string                 `json:"salze"`
	Antwort           QuantenSAbschnittAntwort `json:"antwort"`
	Oeffnungen        []string                 `json:"oeffnungen"`
}

func TestQuantenSPraepAntwortVektor(t *testing.T) {
	roh, err := os.ReadFile("testdata/quanten_s_praep_antwort_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v qspaVektor
	dec := json.NewDecoder(strings.NewReader(string(roh)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	a := v.Antwort
	von, bis, ok := quantenSAbschnitt(v.N, v.V, v.X)
	if !ok || von != v.Von || bis != v.Bis || a.Von != von || a.Bis != bis || a.X != v.X || a.K != v.K {
		t.Fatal("Abschnitt passt nicht")
	}
	n := bis - von
	key := func(h string) *ecdsa.PrivateKey { k, _ := crypto.HexToECDSA(h); return k }
	if quantenSAdresse(key(v.Schluessel[v.X-1])) != a.Validator || a.Validator != v.Validatoren[v.X-1] {
		t.Fatal("Validator der Antwort")
	}
	// 1. Unterschrift der Antwort.
	if !kanonischeSignatur(a.Sig) || verifyPersonalSign(quantenSAbschnittAntwortNachricht(a), a.Sig, a.Validator) != nil {
		t.Fatal("der Knoten erkennt die Unterschrift der App-Antwort nicht")
	}
	b2 := a
	b2.Zaehlung = [][2]int{{a.Zaehlung[0][0], a.Zaehlung[0][1] + 1}, a.Zaehlung[1]}
	if verifyPersonalSign(quantenSAbschnittAntwortNachricht(b2), a.Sig, a.Validator) == nil {
		t.Fatal("geaenderte Zaehlung angenommen")
	}
	// 2. Werte, Blaetter.
	werte := make([]*big.Int, n)
	blaetter := make([][32]byte, v.N)
	for i, h := range v.Werte {
		werte[i], _ = new(big.Int).SetString(h, 16)
		b := quantenWPruefsumme(v.K, von+i, werte[i])
		if hex.EncodeToString(b[:]) != a.Blaetter[i] {
			t.Fatalf("Blatt %d", i)
		}
		blaetter[von+i] = b
	}
	// 3. fuer_inhaberin.
	x, err := quantenSFuerInhaberinOeffnen(key(v.InhSchluessel), a.FuerInhaberin, v.K, von, bis)
	if err != nil {
		t.Fatalf("fuer_inhaberin: %v", err)
	}
	for i := range x {
		if x[i] != v.XMessung[i] {
			t.Fatal("fuer_inhaberin: x anders")
		}
	}
	// 4. Zaehlung.
	for b := 0; b < 2; b++ {
		tr, fe := 0, 0
		for i, w := range werte {
			if quantenSStellenBit(w) == b && v.Y[i] == v.U[i] {
				tr++
				if v.XMessung[i] != v.TWerte[i] {
					fe++
				}
			}
		}
		if a.Zaehlung[b] != [2]int{tr, fe} {
			t.Fatalf("Zaehlung %d: %v, soll %v", b, a.Zaehlung[b], [2]int{tr, fe})
		}
	}
	// 5. Buendel mit Paketen; die Stelle setzt aus t Paketen zusammen.
	if len(a.Buendel) != v.V || a.Buendel[v.X-1] != "" || a.BuendelSig[v.X-1] != "" {
		t.Fatal("Buendel: Zahl oder an sich selbst")
	}
	var pakete [2][]quantenSAnteilPaket
	for j := 1; j <= v.V; j++ {
		if j == v.X {
			continue
		}
		an, pk, err := quantenSBuendelOeffnenPakete(key(v.Schluessel[j-1]), a.Buendel[j-1], a.BuendelSig[j-1], a.Validator, v.K, v.X, j, von, bis)
		if err != nil || len(pk) != 2 || len(an) != n {
			t.Fatalf("Buendel an %d: %v", j, err)
		}
		for b := 0; b < 2; b++ {
			m, salz, err := quantenSAnteilOeffnungLesen(key(v.StellenSchluessel[b]), pk[b], v.K, b, v.X, j, von, bis)
			if err != nil {
				t.Fatalf("Paket %d an Stelle %d: %v", j, b, err)
			}
			if hex.EncodeToString(salz[:]) != v.Salze[b] {
				t.Fatalf("Paket %d an Stelle %d: Salz", j, b)
			}
			for i, y := range m {
				if y.Cmp(an[i-von]) != 0 {
					t.Fatalf("Paket %d: Anteil an %d anders als im Buendel", j, i)
				}
			}
			pakete[b] = append(pakete[b], quantenSAnteilPaket{J: j, Anteile: m, Salz: salz})
			if _, _, err := quantenSAnteilOeffnungLesen(key(v.StellenSchluessel[1-b]), pk[b], v.K, 1-b, v.X, j, von, bis); err == nil {
				t.Fatal("die andere Stelle liest das Paket")
			}
		}
	}
	for b := 0; b < 2; b++ {
		w, salz, err := quantenSAusAnteilenZusammensetzen(pakete[b], v.T, func(m map[int]*big.Int, _ [32]byte) error {
			_, err := quantenSOeffnungLaden(b, v.K, v.N, quantenWMerkleWurzel(blaetter), blaetter, m, 1)
			return err
		})
		if err != nil {
			t.Fatalf("Stelle %d setzt nicht zusammen: %v", b, err)
		}
		var stellen []int
		for i, wi := range werte {
			if quantenSStellenBit(wi) == b {
				stellen = append(stellen, von+i)
				if w[von+i] == nil || w[von+i].Cmp(wi) != 0 {
					t.Fatalf("Stelle %d: Wert an %d", b, von+i)
				}
			}
		}
		if len(w) != len(stellen) {
			t.Fatalf("Stelle %d: Zahl der Werte", b)
		}
		sort.Ints(stellen)
		h := quantenSProbeTeilHash(v.K, b, v.X, salz, stellen)
		if hex.EncodeToString(h[:]) != a.Teile[b] {
			t.Fatalf("Stelle %d: Teil-Hash passt nicht", b)
		}
		// 6. Oeffnung der Praeparatorin selbst.
		m, s, err := quantenSOeffnungLesen(key(v.StellenSchluessel[b]), v.Oeffnungen[b], v.K, b, v.X, von, bis)
		if err != nil || s == nil || *s != salz || len(m) != len(stellen) {
			t.Fatalf("Oeffnung fuer Stelle %d: %v", b, err)
		}
		for i, wi := range m {
			if wi.Cmp(w[i]) != 0 {
				t.Fatalf("Oeffnung fuer Stelle %d: Wert an %d", b, i)
			}
		}
	}
}
