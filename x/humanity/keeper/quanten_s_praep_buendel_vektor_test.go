package keeper

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// PRUEFVEKTOR FUER B4b-3 (App als Praeparatorin, Weg B).
//
// Die App (Aequitas-App lib/quantenSPraeparieren.ts) praepariert einen
// Abschnitt wie der Validator-Dienst (quanten_s_abschnitt.go): gesalzene
// Werte mit Stellen-Bit, Shamir-Anteile, Blaetter, je Empfaenger ein Buendel
// (ECIES + EIP-191). testdata/quanten_s_praep_buendel_vektor.json entsteht in
// der App mit festem Zufall (lib/__tests__/quantenSPraeparieren.test.ts); hier
// oeffnet der Knoten GENAU diese Buendel mit seinem eigenen Code und prueft:
// Unterschrift des Praeparators, Inhalt streng, Anteile = Polynom der
// Koeffizienten, je t Anteile ergeben den Wert, Pruefsumme = Blatt, Basis und
// Wert stimmen, Stellen-Bits genau zur Haelfte, Salz unter 2^123.
// Alle Schluessel sind nur fuer Tests.

type qspbBuendel struct {
	Empfaenger int    `json:"empfaenger"`
	Chiffrat   string `json:"chiffrat"`
	Sig        string `json:"sig"`
}

type qspbVektor struct {
	Hinweis     string        `json:"hinweis"`
	Schluessel  []string      `json:"schluessel_nur_test"`
	Validatoren []string      `json:"validatoren"`
	K           string        `json:"k"`
	N           int           `json:"n"`
	V           int           `json:"v"`
	T           int           `json:"t"`
	X           int           `json:"x"`
	Von         int           `json:"von"`
	Bis         int           `json:"bis"`
	U           []int         `json:"u"`
	TWerte      []int         `json:"t_werte"`
	Werte       []string      `json:"werte"`
	Koeff       [][]string    `json:"koeff"`
	Blaetter    []string      `json:"blaetter"`
	Buendel     []qspbBuendel `json:"buendel"`
}

func qspbLesen(t *testing.T) qspbVektor {
	t.Helper()
	roh, err := os.ReadFile("testdata/quanten_s_praep_buendel_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v qspbVektor
	dec := json.NewDecoder(strings.NewReader(string(roh)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestQuantenSPraepBuendelVektor(t *testing.T) {
	v := qspbLesen(t)
	von, bis, ok := quantenSAbschnitt(v.N, v.V, v.X)
	if !ok || von != v.Von || bis != v.Bis {
		t.Fatalf("Abschnitt: %d..%d, Vektor %d..%d", von, bis, v.Von, v.Bis)
	}
	n := bis - von
	if len(v.Werte) != n || len(v.Koeff) != n || len(v.Blaetter) != n || len(v.U) != n || len(v.TWerte) != n || len(v.Buendel) != v.V-1 {
		t.Fatal("Vektor: Laengen")
	}
	for i, s := range v.Schluessel {
		k, _ := crypto.HexToECDSA(s)
		if quantenSAdresse(k) != v.Validatoren[i] {
			t.Fatalf("Adresse %d", i)
		}
	}
	werte := make([]*big.Int, n)
	eins := 0
	salzGrenze := new(big.Int).Lsh(big.NewInt(1), quantenWSalzBits-1)
	for i, h := range v.Werte {
		w, ok := new(big.Int).SetString(h, 16)
		if !ok || w.Cmp(quantenWP) >= 0 {
			t.Fatalf("Wert %d", i)
		}
		werte[i] = w
		if b := quantenWPruefsumme(v.K, von+i, w); hex.EncodeToString(b[:]) != v.Blaetter[i] {
			t.Fatalf("Stelle %d: Blatt passt nicht zur Pruefsumme des Knotens", i)
		}
		if b, tw := quantenWZerlegen(w); b != v.U[i] || tw != v.TWerte[i] {
			t.Fatalf("Stelle %d: Basis/Wert %d/%d, soll %d/%d", i, b, tw, v.U[i], v.TWerte[i])
		}
		if new(big.Int).Rsh(w, 3).Cmp(salzGrenze) >= 0 {
			t.Fatalf("Stelle %d: Salz zu gross", i)
		}
		eins += quantenSStellenBit(w)
		if v.Koeff[i][0] != h || len(v.Koeff[i]) != v.T {
			t.Fatalf("Stelle %d: Koeffizienten", i)
		}
	}
	if eins != n-n/2 {
		t.Fatalf("Stellen-Bits: %d von %d auf 1, soll %d", eins, n, n-n/2)
	}
	anteile := map[int][]*big.Int{}
	for _, b := range v.Buendel {
		empf, _ := crypto.HexToECDSA(v.Schluessel[b.Empfaenger-1])
		an, err := quantenSBuendelOeffnen(empf, b.Chiffrat, b.Sig, v.Validatoren[v.X-1], v.K, v.X, b.Empfaenger, von, bis)
		if err != nil {
			t.Fatalf("Empfaenger %d: der Knoten oeffnet das Buendel der App nicht: %v", b.Empfaenger, err)
		}
		for i := range an {
			soll, err := quantenSAnteilAus(v.Koeff[i], b.Empfaenger)
			if err != nil || soll.Cmp(an[i]) != 0 {
				t.Fatalf("Empfaenger %d, Stelle %d: Anteil passt nicht zum Polynom", b.Empfaenger, i)
			}
		}
		anteile[b.Empfaenger] = an
		// Missbrauch: anderer Empfaenger, anderes K, fremder Praeparator, getauschtes Chiffrat.
		anderer := 1 + b.Empfaenger%v.V
		if anderer == v.X {
			anderer = 1 + anderer%v.V
		}
		k2, _ := crypto.HexToECDSA(v.Schluessel[anderer-1])
		if _, err := quantenSBuendelOeffnen(k2, b.Chiffrat, b.Sig, v.Validatoren[v.X-1], v.K, v.X, b.Empfaenger, von, bis); err == nil {
			t.Fatal("fremder Schluessel oeffnet das Buendel")
		}
		if _, err := quantenSBuendelOeffnen(empf, b.Chiffrat, b.Sig, v.Validatoren[v.X-1], v.K+"x", v.X, b.Empfaenger, von, bis); err == nil {
			t.Fatal("anderes K angenommen")
		}
		if _, err := quantenSBuendelOeffnen(empf, b.Chiffrat, b.Sig, v.Validatoren[b.Empfaenger-1], v.K, v.X, b.Empfaenger, von, bis); err == nil {
			t.Fatal("Unterschrift eines anderen als des Praeparators angenommen")
		}
		if _, err := quantenSBuendelOeffnen(empf, b.Chiffrat, b.Sig, v.Validatoren[v.X-1], v.K, v.X, b.Empfaenger, von, bis+1); err == nil {
			t.Fatal("anderer Abschnitt angenommen")
		}
	}
	// Je t Anteile verschiedener Validatoren ergeben den Wert.
	var xs []int
	for x := range anteile {
		xs = append(xs, x)
	}
	if len(xs) < v.T {
		t.Fatal("zu wenige Empfaenger fuer t")
	}
	for i := range werte {
		var a []quantenWAnteil
		for _, x := range xs[:v.T] {
			a = append(a, quantenWAnteil{X: x, Y: anteile[x][i]})
		}
		if quantenWZusammensetzen(a).Cmp(werte[i]) != 0 {
			t.Fatalf("Stelle %d: t Anteile ergeben nicht den Wert", i)
		}
	}
}
