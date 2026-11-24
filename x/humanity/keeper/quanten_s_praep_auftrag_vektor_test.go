package keeper

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// PRUEFVEKTOR: ABSCHNITTS-AUFTRAG AN DIE APP ALS PRAEPARATORIN (B4b-4c-1).
//
// In Weg B schickt der Ausgeber den Abschnitts-Auftrag (dieselbe Form wie
// QuantenSAbschnittAnfrage an den Validator-Dienst) als Huelle ueber den
// Briefkasten an die geloste Praeparatorin; die Basen y sind an ihren
// Schluessel verschluesselt (quantenSBasenVerschluesseln). Die App
// (lib/quantenSPraepAuftrag.ts) muss die unterschriebene Nachricht genau wie
// quantenSAbschnittAuftragNachricht bilden und die Basen genau wie
// quantenSBasenOeffnen oeffnen. testdata/quanten_s_praep_auftrag_vektor.json
// haelt einen echten Auftrag fest (Schluessel NUR fuer Tests); ECIES ist
// zufaellig, darum prueft der Test den gespeicherten Auftrag mit dem Code des
// Knotens, statt ihn neu zu bauen. Neu:
// AEQUITAS_VEKTOR_SCHREIBEN=1 go test -run TestQuantenSPraepAuftragVektor .

type qsAuftragVektor struct {
	Hinweis          string                   `json:"hinweis"`
	ChainID          int64                    `json:"chain_id"`
	PraegerSchl      string                   `json:"praeger_schluessel_nur_test"`
	PraepSchluessel  []string                 `json:"praeparatoren_schluessel_nur_test"`
	InhaberinSchl    string                   `json:"inhaberin_schluessel_nur_test"`
	Auftrag          QuantenSAbschnittAnfrage `json:"auftrag"`
	AuftragNachricht string                   `json:"auftrag_nachricht"`
	Von              int                      `json:"von"`
	Bis              int                      `json:"bis"`
	Basen            string                   `json:"basen"`
}

const qsAuftragVektorPfad = "testdata/quanten_s_praep_auftrag_vektor.json"

func qsAuftragSchluessel(t *testing.T, b string) (*ecdsa.PrivateKey, string, string) {
	t.Helper()
	k, err := crypto.HexToECDSA(strings.Repeat(b, 32))
	if err != nil {
		t.Fatal(err)
	}
	return k, strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex()), hex.EncodeToString(crypto.FromECDSAPub(&k.PublicKey))
}

// qsAuftragBasen: feste Basen fuer den Vektor (aus SHA-256, nur Tests).
func qsAuftragBasen(n int) string {
	var sb strings.Builder
	for i := 0; sb.Len() < n; i++ {
		h := sha256.Sum256([]byte(fmt.Sprintf("qspa|%d", i)))
		for _, by := range h {
			for j := 7; j >= 0 && sb.Len() < n; j-- {
				sb.WriteByte('0' + (by>>j)&1)
			}
		}
	}
	return sb.String()
}

func qsAuftragVektorBauen(t *testing.T) qsAuftragVektor {
	t.Helper()
	kP, praeger, _ := qsAuftragSchluessel(t, "81")
	var validatoren, schluessel, schlHex []string
	for _, b := range []string{"82", "83", "84"} {
		_, a, pub := qsAuftragSchluessel(t, b)
		validatoren, schluessel, schlHex = append(validatoren, a), append(schluessel, pub), append(schlHex, strings.Repeat(b, 32))
	}
	_, inh, inhPub := qsAuftragSchluessel(t, "85")
	_, s0, s0Pub := qsAuftragSchluessel(t, "86")
	_, s1, s1Pub := qsAuftragSchluessel(t, "87")
	const n, x = 2400, 2
	von, bis, _ := quantenSAbschnitt(n, len(validatoren), x)
	basen := qsAuftragBasen(bis - von)
	chiffrat, err := quantenSBasenVerschluesseln(schluessel[x-1], validatoren, n, x, inh, basen)
	if err != nil {
		t.Fatal(err)
	}
	q := QuantenSAbschnittAnfrage{K: strings.Repeat("ab", 32), N: n, T: 2, Tol: 2, X: x, Validatoren: validatoren, Schluessel: schluessel,
		BasenVerschluesselt: chiffrat, Inhaberin: inh, InhaberinSchluessel: inhPub, Stellen: []string{s0, s1},
		StellenSchluessel: []string{s0Pub, s1Pub}, Zeit: 1_795_400_000, Praeger: praeger, ProbeArt: 2}
	m := quantenSAbschnittAuftragNachricht(q)
	q.PraegerSig = personalSign(t, kP, m)
	return qsAuftragVektor{
		Hinweis: "Pruefvektor Abschnitts-Auftrag an die Praeparatorin (quanten_s_abschnitt_dienst.go, quanten_s_basen_chiffrat.go). " +
			"Schluessel NUR fuer Tests. Neu: AEQUITAS_VEKTOR_SCHREIBEN=1 go test -run TestQuantenSPraepAuftragVektor ./x/humanity/keeper/",
		ChainID: eingefroreneChainID, PraegerSchl: strings.Repeat("81", 32), PraepSchluessel: schlHex, InhaberinSchl: strings.Repeat("85", 32),
		Auftrag: q, AuftragNachricht: m, Von: von, Bis: bis, Basen: basen,
	}
}

func TestQuantenSPraepAuftragVektor(t *testing.T) {
	if os.Getenv("AEQUITAS_VEKTOR_SCHREIBEN") == "1" {
		roh, _ := json.MarshalIndent(qsAuftragVektorBauen(t), "", "  ")
		if err := os.WriteFile(qsAuftragVektorPfad, append(roh, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("geschrieben:", qsAuftragVektorPfad)
	}
	roh, err := os.ReadFile(qsAuftragVektorPfad)
	if err != nil {
		t.Fatal(err)
	}
	var v qsAuftragVektor
	dec := json.NewDecoder(strings.NewReader(string(roh)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	q := v.Auftrag
	// Der Knoten rechnet mit eigenem Code nach: Nachricht, Unterschrift,
	// Abschnitt, Basen.
	if got := quantenSAbschnittAuftragNachricht(q); got != v.AuftragNachricht {
		t.Fatalf("Nachricht:\n%s\nstatt\n%s", got, v.AuftragNachricht)
	}
	_, praeger, _ := qsAuftragSchluessel(t, v.PraegerSchl[:2])
	if q.Praeger != praeger || !kanonischeSignatur(q.PraegerSig) || verifyPersonalSign(v.AuftragNachricht, q.PraegerSig, praeger) != nil {
		t.Fatal("Unterschrift des Ausgebers")
	}
	von, bis, _ := quantenSAbschnitt(q.N, len(q.Validatoren), q.X)
	if von != v.Von || bis != v.Bis || v.Basen != qsAuftragBasen(bis-von) {
		t.Fatalf("Abschnitt %d..%d oder Basen", von, bis)
	}
	kX, aX, _ := qsAuftragSchluessel(t, v.PraepSchluessel[q.X-1][:2])
	if aX != q.Validatoren[q.X-1] {
		t.Fatal("Schluessel der Praeparatorin X")
	}
	y, err := quantenSBasenOeffnen(kX, aX, q.BasenVerschluesselt, q.N, q.Validatoren, q.X, q.Inhaberin)
	if err != nil || y != v.Basen {
		t.Fatalf("Basen oeffnen: %v", err)
	}
	// Gegenprobe: eine andere Praeparatorin kann die Basen nicht oeffnen.
	kAnders, aAnders, _ := qsAuftragSchluessel(t, v.PraepSchluessel[0][:2])
	if _, err := quantenSBasenOeffnen(kAnders, aAnders, q.BasenVerschluesselt, q.N, q.Validatoren, 1, q.Inhaberin); err == nil {
		t.Fatal("eine andere Praeparatorin oeffnet die Basen")
	}
}
