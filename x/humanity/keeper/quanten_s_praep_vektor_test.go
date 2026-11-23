package keeper

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// PRUEFVEKTOR FUER DIE APP ALS PRAEPARATORIN (B4a, DEZENTRAL.md 18).
//
// Die App (Aequitas-App, lib/quantenSPraeparator.ts) muss K und die Nachricht
// zum Melden und Abmelden genau so bilden wie der Knoten
// (quanten_s_praeparatoren.go) und mit der Menschen-Wallet per EIP-191
// unterschreiben. testdata/quanten_s_praep_vektor.json haelt das fest;
// derselbe Inhalt liegt in der App (lib/__tests__/vektoren/). Der Schluessel
// ist fest (nur Tests!); EIP-191 mit secp256k1 ist deterministisch (RFC 6979),
// also ist auch die Unterschrift fest. Der Test prueft zusaetzlich, dass der
// Knoten genau diese Saetze annimmt. Neu schreiben:
// AEQUITAS_VEKTOR_SCHREIBEN=1 go test -run TestQuantenSPraepVektor .

type qspVektor struct {
	Hinweis             string `json:"hinweis"`
	ChainID             int64  `json:"chain_id"`
	Schluessel          string `json:"schluessel_nur_test"`
	Konto               string `json:"konto"`
	K                   string `json:"k"`
	Zeit                int64  `json:"zeit"`
	MeldenNachricht     string `json:"melden_nachricht"`
	MeldenSig           string `json:"melden_sig"`
	AbmeldenNachricht   string `json:"abmelden_nachricht"`
	AbmeldenSig         string `json:"abmelden_sig"`
	MeldenPfad          string `json:"melden_pfad"`
	AbmeldenPfad        string `json:"abmelden_pfad"`
	RegisterPfad        string `json:"register_pfad"`
	VorlaufSekunden     int64  `json:"vorlauf_sekunden"`
	NachlaufSekunden    int64  `json:"nachlauf_sekunden"`
	HoechstensEintraege int    `json:"hoechstens_eintraege"`
}

const qspVektorPfad = "testdata/quanten_s_praep_vektor.json"

func qspVektorBauen(t *testing.T) qspVektor {
	t.Helper()
	schl := strings.Repeat("61", 32)
	k, err := crypto.HexToECDSA(schl)
	if err != nil {
		t.Fatal(err)
	}
	konto := strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())
	K := quantenSPraepK(konto)
	const zeit int64 = 1_795_400_000
	m := quantenSPraepNachricht(quantenSPraepMeldenTxArt, konto, K, zeit)
	a := quantenSPraepNachricht(quantenSPraepAbmeldenTxArt, konto, K, zeit+60)
	return qspVektor{
		Hinweis: "Pruefvektor B4a: Praeparatorin melden/abmelden (quanten_s_praeparatoren.go). Schluessel NUR fuer Tests. " +
			"Neu: AEQUITAS_VEKTOR_SCHREIBEN=1 go test -run TestQuantenSPraepVektor ./x/humanity/keeper/",
		ChainID: eingefroreneChainID, Schluessel: schl, Konto: konto, K: K, Zeit: zeit,
		MeldenNachricht: m, MeldenSig: personalSign(t, k, m),
		AbmeldenNachricht: a, AbmeldenSig: personalSign(t, k, a),
		MeldenPfad:      "/api/quanten-w/" + strings.TrimPrefix(quantenSPraepMeldenTxArt, "quanten_w_"),
		AbmeldenPfad:    "/api/quanten-w/" + strings.TrimPrefix(quantenSPraepAbmeldenTxArt, "quanten_w_"),
		RegisterPfad:    "/api/quanten-w/praeparatoren",
		VorlaufSekunden: quantenSPraepVorlauf, NachlaufSekunden: quantenSPraepNachlauf, HoechstensEintraege: quantenSPraepHoechstens,
	}
}

func TestQuantenSPraepVektor(t *testing.T) {
	if os.Getenv("AEQUITAS_VEKTOR_SCHREIBEN") == "1" {
		roh, _ := json.MarshalIndent(qspVektorBauen(t), "", "  ")
		if err := os.WriteFile(qspVektorPfad, append(roh, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("geschrieben:", qspVektorPfad)
	}
	roh, err := os.ReadFile(qspVektorPfad)
	if err != nil {
		t.Fatal(err)
	}
	var gelesen qspVektor
	dec := json.NewDecoder(strings.NewReader(string(roh)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&gelesen); err != nil {
		t.Fatal(err)
	}
	if soll := qspVektorBauen(t); !reflect.DeepEqual(gelesen, soll) {
		t.Fatalf("Vektor weicht ab -- Knoten oder Datei geaendert?\nDatei: %+v\nKnoten: %+v", gelesen, soll)
	}

	// Der Knoten nimmt genau diese Saetze an (Unterschrift mit dem Vektor).
	w := neuePraepWelt(t)
	quantenWAbOverride.Store(1) // die Vektor-Zeit liegt vor qwT0
	quantenSWegBAbOverride.Store(1)
	addHuman(w.cs, gelesen.Konto, 0)
	addHuman(w.nach, gelesen.Konto, 0)
	melden := Transaction{Type: quantenSPraepMeldenTxArt, Wallet: gelesen.Konto, DistributionAt: gelesen.Zeit,
		QuantenW: &QuantenWAngaben{K: gelesen.K, WalletSig: gelesen.MeldenSig}}
	w.annehmen(melden, gelesen.Zeit+5)
	abmelden := Transaction{Type: quantenSPraepAbmeldenTxArt, Wallet: gelesen.Konto, DistributionAt: gelesen.Zeit + 60,
		QuantenW: &QuantenWAngaben{K: gelesen.K, WalletSig: gelesen.AbmeldenSig}}
	w.annehmen(abmelden, gelesen.Zeit+65)
	if e := w.cs.quantenWJetzt().Praep[gelesen.K]; e.Konto != gelesen.Konto || e.Abgemeldet != gelesen.Zeit+65 {
		t.Fatalf("Register nach dem Vektor: %+v", e)
	}
}
