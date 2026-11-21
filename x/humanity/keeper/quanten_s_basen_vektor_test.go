package keeper

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// PRUEFVEKTOR FUER R8 (verschluesselte Basen, quanten_s_basen_chiffrat.go).
//
// Die App (Aequitas-App, lib/quantenS.ts basenVerschluesseln) verschluesselt
// y an den Validator; der Knoten muss das Chiffrat oeffnen. Umgekehrt muss die
// App die Antwort eines Validators mit "|basen:" genau so pruefen wie der
// Knoten. testdata/quanten_s_basen_vektor.json haelt fest:
//   - chiffrat_app: von der App mit festem Zufall (app_zufall) erzeugt -- die
//     App rechnet es im Test Byte fuer Byte nach, Go oeffnet es hier;
//   - chiffrat_go: von Go erzeugt (ECIES zufaellig, darum gespeichert) -- die
//     App oeffnet es im Test mit dem Schluessel des Validators;
//   - antwort: Nachricht und Unterschrift eines Abschnitts mit basen_hash.
// Alle Schluessel sind nur fuer Tests. Neu schreiben (chiffrat_go):
// AEQUITAS_VEKTOR_SCHREIBEN=1 go test -run TestQuantenSBasenVektor .

const qsbVektorPfad = "testdata/quanten_s_basen_vektor.json"

// qsbChiffratApp: aus der App (lib/quantenS.ts basenVerschluesseln, Zufall
// eph = 0x11 x 32, iv = 0x22 x 16); fest, nicht neu zu erzeugen.
const qsbChiffratApp = "044f355bdcb7cc0af728ef3cceb9615d90684bb5b2ca5f859ab0f0b704075871aa385b6b1b8ead809ca67454d9683fcf2ba03456d6fe2c4abe2b07f0fbdbb2f1c122222222222222222222222222222222cbc56ba3e9da82f955d9a83008abddf5bf7d7f9693dcf560cc202e62d25f9cacf6f42b07bd12fe8c289d7b33e28194bb23d41f0037ba3c43dd674304c2fc75d08012489a2834a7e341afcb6d4d9656a050b8b49c586d4db3715efe78d4909b75837e089aab669ac9bb492f7ef59476dcaa7370176086e3d715586024970a6aed149a407f1c369811f5dec15062795098e30c0cdd08a3a026626af0b474e1ae57b2d7ad05d3ec6a487b4b2525f64f09c3d9c1913c28329faad827b55f2d34761ee00dcb834daed5253ef74f6fd2768bdcde5880e458ec2b901184bbac38629b55d2db7905bf22a9e6dbf9609aace1f5b818746308b0d2745d6a4d5df636d20efc1879abfacecffabb1744ccce0e54a859016142211f4d30c347a8006bb81645cc0e6808c8066a0d8ce1bbfc18871e8c83df4e"

type qsbVektorAntwort struct {
	K              string   `json:"k"`
	X              int      `json:"x"`
	Von            int      `json:"von"`
	Bis            int      `json:"bis"`
	BlaetterHash   string   `json:"blaetter_hash"`
	FuerInhaberin  string   `json:"fuer_inhaberin"`
	BuendelHash    string   `json:"buendel_hash"`
	BuendelSigHash string   `json:"buendel_sig_hash"`
	Teile          []string `json:"teile"`
	Zaehlung       [][2]int `json:"zaehlung"`
	BasenHash      string   `json:"basen_hash"`
	Nachricht      string   `json:"nachricht"`
	Sig            string   `json:"sig"`
}

type qsbVektor struct {
	Hinweis             string            `json:"hinweis"`
	ChainID             int64             `json:"chain_id"`
	ValidatorSchluessel []string          `json:"validator_schluessel_nur_test"`
	Validatoren         []string          `json:"validatoren"`
	ValidatorenHash     string            `json:"validatoren_hash"`
	InhaberinSchluessel string            `json:"inhaberin_schluessel_nur_test"`
	Inhaberin           string            `json:"inhaberin"`
	N                   int               `json:"n"`
	X                   int               `json:"x"`
	Basen               string            `json:"basen"`
	AppZufall           map[string]string `json:"app_zufall"`
	ChiffratApp         string            `json:"chiffrat_app"`
	ChiffratGo          string            `json:"chiffrat_go"`
	Antwort             qsbVektorAntwort  `json:"antwort"`
}

func qsbVektorBauen(t *testing.T, chiffratGo string) qsbVektor {
	t.Helper()
	schl := []string{strings.Repeat("71", 32), strings.Repeat("72", 32)}
	var val []string
	for _, s := range schl {
		k, _ := crypto.HexToECDSA(s)
		val = append(val, quantenSAdresse(k))
	}
	inhK, _ := crypto.HexToECDSA(strings.Repeat("51", 32))
	v := qsbVektor{
		Hinweis: "R8: Basen je Validator verschluesselt (quanten_s_basen_chiffrat.go). Schluessel NUR fuer Tests.",
		ChainID: eingefroreneChainID, ValidatorSchluessel: schl, Validatoren: val, ValidatorenHash: quantenSValidatorenHash(val),
		InhaberinSchluessel: strings.Repeat("51", 32), Inhaberin: quantenSAdresse(inhK), N: 12, X: 2, Basen: "101100",
		AppZufall: map[string]string{"eph": strings.Repeat("11", 32), "iv": strings.Repeat("22", 16)}, ChiffratApp: qsbChiffratApp,
	}
	if chiffratGo == "" {
		k2, _ := crypto.HexToECDSA(schl[1])
		c, err := quantenSBasenVerschluesseln(hex.EncodeToString(crypto.FromECDSAPub(&k2.PublicKey)), val, v.N, v.X, v.Inhaberin, v.Basen)
		if err != nil {
			t.Fatal(err)
		}
		chiffratGo = c
	}
	v.ChiffratGo = chiffratGo
	von, bis, _ := quantenSAbschnitt(v.N, len(val), v.X)
	a := qsbVektorAntwort{K: strings.Repeat("ab", 32), X: v.X, Von: von, Bis: bis, BlaetterHash: quantenSHashListe([]string{strings.Repeat("cd", 32)}),
		FuerInhaberin: "0411", BuendelHash: quantenSHashListe([]string{"b1", ""}), BuendelSigHash: quantenSHashListe([]string{"s1", ""}),
		Teile: []string{strings.Repeat("e1", 32), strings.Repeat("e2", 32)}, Zaehlung: [][2]int{{2, 0}, {1, 0}}, BasenHash: quantenSBasenHash(qsbChiffratApp)}
	a.Nachricht = quantenSAbschnittAntwortNachrichtAus(a.K, a.X, a.Von, a.Bis, a.BlaetterHash, a.FuerInhaberin, a.BuendelHash, a.BuendelSigHash, a.Teile, a.Zaehlung, a.BasenHash)
	k2, _ := crypto.HexToECDSA(schl[1])
	sig, err := quantenSUnterschreiben(k2, a.Nachricht) // RFC 6979: fest
	if err != nil {
		t.Fatal(err)
	}
	a.Sig = sig
	v.Antwort = a
	return v
}

func TestQuantenSBasenVektor(t *testing.T) {
	if os.Getenv("AEQUITAS_VEKTOR_SCHREIBEN") == "1" {
		v := qsbVektorBauen(t, "")
		roh, _ := json.MarshalIndent(v, "", "  ")
		if err := os.WriteFile(qsbVektorPfad, append(roh, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("geschrieben:", qsbVektorPfad)
	}
	roh, err := os.ReadFile(qsbVektorPfad)
	if err != nil {
		t.Fatal(err)
	}
	var gelesen qsbVektor
	dec := json.NewDecoder(strings.NewReader(string(roh)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&gelesen); err != nil {
		t.Fatal(err)
	}
	soll := qsbVektorBauen(t, gelesen.ChiffratGo)
	a, _ := json.Marshal(soll)
	b, _ := json.Marshal(gelesen)
	if string(a) != string(b) {
		t.Fatalf("Vektor weicht von dem ab, was der Knoten rechnet:\n%s\n%s", a, b)
	}
	// Der Validator (Nummer 2) oeffnet beide Chiffrate -- das der App und das von Go.
	k2, _ := crypto.HexToECDSA(gelesen.ValidatorSchluessel[1])
	for name, c := range map[string]string{"App": gelesen.ChiffratApp, "Go": gelesen.ChiffratGo} {
		y, err := quantenSBasenOeffnen(k2, gelesen.Validatoren[1], c, gelesen.N, gelesen.Validatoren, gelesen.X, gelesen.Inhaberin)
		if err != nil || y != gelesen.Basen {
			t.Fatalf("Chiffrat %s: %q, %v", name, y, err)
		}
	}
	// Gegenproben: andere Inhaberin, andere Liste, anderer Validator -- abgewiesen.
	if _, err := quantenSBasenOeffnen(k2, gelesen.Validatoren[1], gelesen.ChiffratApp, gelesen.N, gelesen.Validatoren, gelesen.X, gelesen.Validatoren[0]); err == nil {
		t.Fatal("Chiffrat der App fuer eine andere Inhaberin geoeffnet")
	}
	if _, err := quantenSBasenOeffnen(k2, gelesen.Validatoren[1], gelesen.ChiffratApp, gelesen.N, []string{gelesen.Validatoren[1], gelesen.Validatoren[0]}, 1, gelesen.Inhaberin); err == nil {
		t.Fatal("Chiffrat der App mit vertauschter Liste geoeffnet")
	}
	k1, _ := crypto.HexToECDSA(gelesen.ValidatorSchluessel[0])
	if _, err := quantenSBasenOeffnen(k1, gelesen.Validatoren[0], gelesen.ChiffratApp, gelesen.N, gelesen.Validatoren, gelesen.X, gelesen.Inhaberin); err == nil {
		t.Fatal("Chiffrat vom falschen Validator geoeffnet")
	}
	if verifyPersonalSign(gelesen.Antwort.Nachricht, gelesen.Antwort.Sig, gelesen.Validatoren[1]) != nil {
		t.Fatal("Unterschrift der Antwort")
	}
}
