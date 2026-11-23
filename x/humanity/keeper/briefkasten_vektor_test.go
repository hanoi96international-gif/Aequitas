package keeper

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// PRUEFVEKTOR FUER DEN BRIEFKASTEN DER APP (B4b-4b, DEZENTRAL.md 18).
//
// Die App (Aequitas-App, lib/briefkasten.ts) bildet die Nachrichten zum
// Ablegen, Abholen und Quittieren genau wie der Knoten (briefkasten.go) und
// unterschreibt sie per EIP-191. testdata/briefkasten_vektor.json haelt das
// fest; derselbe Inhalt liegt in der App (lib/__tests__/vektoren/). Die
// Schluessel sind fest (nur Tests!), EIP-191 mit secp256k1 ist
// deterministisch (RFC 6979). Der Test prueft zusaetzlich, dass der Knoten
// genau diese Anfragen durch alle Pruefungen vor dem Kasten laesst. Neu:
// AEQUITAS_VEKTOR_SCHREIBEN=1 go test -run TestBriefkastenVektor .

type bkVektor struct {
	Hinweis             string `json:"hinweis"`
	ChainID             int64  `json:"chain_id"`
	AbsenderSchluessel  string `json:"absender_schluessel_nur_test"`
	Absender            string `json:"absender"`
	EmpfSchluessel      string `json:"empfaenger_schluessel_nur_test"`
	Empfaenger          string `json:"empfaenger"`
	Zeit                int64  `json:"zeit"`
	Inhalt              string `json:"inhalt"`
	InhaltHash          string `json:"inhalt_hash"`
	AblegenNachricht    string `json:"ablegen_nachricht"`
	AblegenSig          string `json:"ablegen_sig"`
	Nach                int64  `json:"nach"`
	AbholenNachricht    string `json:"abholen_nachricht"`
	AbholenSig          string `json:"abholen_sig"`
	Bis                 int64  `json:"bis"`
	QuittierenNachricht string `json:"quittieren_nachricht"`
	QuittierenSig       string `json:"quittieren_sig"`
	Pfad                string `json:"pfad"`
	InhaltMax           int    `json:"inhalt_max"`
	AbholenHoechstens   int    `json:"abholen_hoechstens"`
	AbholenBytes        int    `json:"abholen_bytes"`
	ZeitSpiel           int64  `json:"zeit_spiel"`
	Behalten            int64  `json:"behalten"`
}

const bkVektorPfad = "testdata/briefkasten_vektor.json"

func bkVektorBauen(t *testing.T) bkVektor {
	t.Helper()
	schlA, schlE := strings.Repeat("62", 32), strings.Repeat("63", 32)
	kA, err := crypto.HexToECDSA(schlA)
	if err != nil {
		t.Fatal(err)
	}
	kE, err := crypto.HexToECDSA(schlE)
	if err != nil {
		t.Fatal(err)
	}
	von := strings.ToLower(crypto.PubkeyToAddress(kA.PublicKey).Hex())
	an := strings.ToLower(crypto.PubkeyToAddress(kE.PublicKey).Hex())
	const zeit int64 = 1_795_400_000
	inhalt := hex.EncodeToString([]byte("aequitas briefkasten pruefvektor"))
	roh, _ := hex.DecodeString(inhalt)
	h := sha256.Sum256(roh)
	hHex := hex.EncodeToString(h[:])
	ab := briefAblegenNachricht(von, an, hHex, zeit)
	ho := briefAbholenNachricht(an, 41, zeit+1)
	qu := briefQuittierenNachricht(an, 42, zeit+2)
	return bkVektor{
		Hinweis: "Pruefvektor Briefkasten (briefkasten.go). Schluessel NUR fuer Tests. " +
			"Neu: AEQUITAS_VEKTOR_SCHREIBEN=1 go test -run TestBriefkastenVektor ./x/humanity/keeper/",
		ChainID: eingefroreneChainID, AbsenderSchluessel: schlA, Absender: von, EmpfSchluessel: schlE, Empfaenger: an,
		Zeit: zeit, Inhalt: inhalt, InhaltHash: hHex,
		AblegenNachricht: ab, AblegenSig: personalSign(t, kA, ab),
		Nach: 41, AbholenNachricht: ho, AbholenSig: personalSign(t, kE, ho),
		Bis: 42, QuittierenNachricht: qu, QuittierenSig: personalSign(t, kE, qu),
		Pfad: "/api/briefkasten/", InhaltMax: briefInhaltMax, AbholenHoechstens: briefAbholenHoechstens,
		AbholenBytes: briefAbholenBytes, ZeitSpiel: briefZeitSpiel, Behalten: briefBehalten,
	}
}

func TestBriefkastenVektor(t *testing.T) {
	if os.Getenv("AEQUITAS_VEKTOR_SCHREIBEN") == "1" {
		roh, _ := json.MarshalIndent(bkVektorBauen(t), "", "  ")
		if err := os.WriteFile(bkVektorPfad, append(roh, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("geschrieben:", bkVektorPfad)
	}
	roh, err := os.ReadFile(bkVektorPfad)
	if err != nil {
		t.Fatal(err)
	}
	var v bkVektor
	dec := json.NewDecoder(strings.NewReader(string(roh)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	if soll := bkVektorBauen(t); !reflect.DeepEqual(v, soll) {
		t.Fatalf("Vektor weicht ab -- Knoten oder Datei geaendert?\nDatei: %+v\nKnoten: %+v", v, soll)
	}

	// Der Knoten laesst genau diese Anfragen durch alle Pruefungen vor dem
	// Kasten (ohne Datenbank danach 503, nie eine Abweisung der Anfrage).
	cs := newTestState()
	addHuman(cs, v.Absender, 0)
	jetzt := v.Zeit + 1
	if _, err := cs.briefAblegen(v.Absender, v.Empfaenger, v.Inhalt, v.Zeit, v.AblegenSig, jetzt); bkCode(err) != http.StatusServiceUnavailable {
		t.Fatalf("Ablegen aus dem Vektor: %v", err)
	}
	if _, _, err := cs.briefAbholen(v.Empfaenger, v.Nach, v.Zeit+1, v.AbholenSig, jetzt); bkCode(err) != http.StatusServiceUnavailable {
		t.Fatalf("Abholen aus dem Vektor: %v", err)
	}
	if _, err := cs.briefQuittieren(v.Empfaenger, v.Bis, v.Zeit+2, v.QuittierenSig, jetzt); bkCode(err) != http.StatusServiceUnavailable {
		t.Fatalf("Quittieren aus dem Vektor: %v", err)
	}
	// Gegenprobe: eine Unterschrift des Absenders zum Abholen gilt nicht.
	if _, _, err := cs.briefAbholen(v.Empfaenger, v.Nach, v.Zeit+1, v.AblegenSig, jetzt); bkCode(err) != http.StatusForbidden {
		t.Fatalf("fremde Unterschrift zum Abholen: %v", err)
	}
}
