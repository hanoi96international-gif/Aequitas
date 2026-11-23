package keeper

import (
	"encoding/json"
	"math/big"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// PRUEFVEKTOR FUER B4b-3c (die App bestaetigt eine S-Ausgabe in Abschnitten).
//
// testdata/quanten_s_praep_bestaetigen_vektor.json entsteht in der App
// (lib/quantenSPraeparieren.ts bestaetigen) mit festem Zufall: drei
// Praeparatoren, Stichprobe v2, Validator 1 bestaetigt. Der Knoten prueft mit
// seinem eigenen Code dieselben Schritte wie bestaetigenAbschnitt:
//   - der Auftrag ist vom Ausgeber unterschrieben (quantenWDienstAuftrag);
//   - die Blaetter ergeben die Wurzel, die eigenen Blaetter passen zu den Werten;
//   - jedes Buendel an Validator 1 oeffnet sich (quantenSBuendelOeffnenPakete)
//     und traegt die zwei Pakete; die Anteile sind die, die die App meldet;
//   - die Bestaetigung der App ist gueltig fuer quantenWSBestaetigungNachricht
//     und die Groessen sind die eines gueltigen Tokens (neuerQuantenSToken).
// Alle Schluessel sind nur fuer Tests.

type qspbeBest struct {
	Sig        string              `json:"sig"`
	Anteile    []string            `json:"anteile"`
	Oeffnungen map[string][]string `json:"oeffnungen"`
}

type qspbeVektor struct {
	Hinweis      string                     `json:"hinweis"`
	Schluessel   []string                   `json:"schluessel_nur_test"`
	PraegerSchl  string                     `json:"praeger_schluessel_nur_test"`
	Anfrage      QuantenWBestaetigenAnfrage `json:"anfrage"`
	Werte        []string                   `json:"werte"`
	Koeff        [][]string                 `json:"koeff"`
	Bestaetigung qspbeBest                  `json:"bestaetigung"`
}

func TestQuantenSPraepBestaetigenVektor(t *testing.T) {
	roh, err := os.ReadFile("testdata/quanten_s_praep_bestaetigen_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v qspbeVektor
	dec := json.NewDecoder(strings.NewReader(string(roh)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	q := v.Anfrage
	if q.Art != "s" || len(q.Anteile) != 0 || q.X != 1 || !istHash64(q.K) {
		t.Fatal("Anfrage: Art, Anteile, X oder K")
	}
	pk, _ := crypto.HexToECDSA(v.PraegerSchl)
	if quantenSAdresse(pk) != q.Praeger {
		t.Fatal("Praeger")
	}
	// 1. Auftrag des Ausgebers.
	if !kanonischeSignatur(q.PraegerSig) || verifyPersonalSign(quantenWDienstAuftrag(q), q.PraegerSig, q.Praeger) != nil {
		t.Fatal("der Knoten erkennt den Auftrag nicht, den die App prueft")
	}
	// 2. Wurzel, eigene Blaetter.
	wurzel, ok := quantenWHex32(q.Wurzel)
	if !ok || len(q.Blaetter) != q.N {
		t.Fatal("Wurzel")
	}
	bl := make([][32]byte, q.N)
	for i, h := range q.Blaetter {
		if bl[i], ok = quantenWHex32(h); !ok {
			t.Fatalf("Blatt %d", i)
		}
	}
	if quantenWMerkleWurzel(bl) != wurzel {
		t.Fatal("Blaetter ergeben nicht die Wurzel")
	}
	val := len(q.Validatoren)
	von, bis, _ := quantenSAbschnitt(q.N, val, q.X)
	if len(v.Werte) != bis-von || len(v.Bestaetigung.Anteile) != q.N {
		t.Fatal("Laengen")
	}
	for i, h := range v.Werte {
		w, _ := new(big.Int).SetString(h, 16)
		if quantenWPruefsumme(q.K, von+i, w) != bl[von+i] {
			t.Fatalf("eigenes Blatt %d", von+i)
		}
		y, err := quantenSAnteilAus(v.Koeff[i], q.X)
		if err != nil || y.Text(16) != v.Bestaetigung.Anteile[von+i] {
			t.Fatalf("eigener Anteil an %d", von+i)
		}
	}
	// 3. Buendel an Validator 1.
	k1, _ := crypto.HexToECDSA(v.Schluessel[0])
	if quantenSAdresse(k1) != q.Validatoren[0] || q.Buendel[0] != "" || q.BuendelSig[0] != "" {
		t.Fatal("Validator 1")
	}
	for xv := 2; xv <= val; xv++ {
		vv, bb, _ := quantenSAbschnitt(q.N, val, xv)
		an, pakete, err := quantenSBuendelOeffnenPakete(k1, q.Buendel[xv-1], q.BuendelSig[xv-1], q.Validatoren[xv-1], q.K, xv, q.X, vv, bb)
		if err != nil || len(pakete) != 2 {
			t.Fatalf("Buendel von %d: %v", xv, err)
		}
		for i, y := range an {
			if y.Text(16) != v.Bestaetigung.Anteile[vv+i] {
				t.Fatalf("Anteil an %d anders als von der App gemeldet", vv+i)
			}
		}
		got := v.Bestaetigung.Oeffnungen[strconv.Itoa(xv)]
		if len(got) != 2 || got[0] != pakete[0] || got[1] != pakete[1] {
			t.Fatalf("Pakete von %d", xv)
		}
	}
	// 4. Bestaetigung und Groessen.
	var proben [2][32]byte
	for i := range proben {
		if proben[i], ok = quantenWHex32(q.Proben[i]); !ok {
			t.Fatal("Proben")
		}
	}
	if _, err := neuerQuantenSToken(q.N, q.Validatoren[0], wurzel, 1, proben, q.T, val, q.Tol); err != nil {
		t.Fatalf("Groessen: %v", err)
	}
	msg := quantenWSBestaetigungNachricht(q.K, wurzel, q.N, q.T, q.Validatoren, q.X, proben, q.Zeit, q.Tol)
	if !kanonischeSignatur(v.Bestaetigung.Sig) || verifyPersonalSign(msg, v.Bestaetigung.Sig, q.Validatoren[0]) != nil {
		t.Fatal("der Knoten erkennt die Bestaetigung der App nicht")
	}
	anders := quantenWSBestaetigungNachricht(q.K, wurzel, q.N, q.T, q.Validatoren, q.X, [2][32]byte{proben[1], proben[0]}, q.Zeit, q.Tol)
	if verifyPersonalSign(anders, v.Bestaetigung.Sig, q.Validatoren[0]) == nil {
		t.Fatal("vertauschte Proben angenommen")
	}
}
