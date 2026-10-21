package keeper

// Echte Beweise durch den BioVerifier der Kette, in der EVM des Knotens --
// genau der Weg, auf dem V8.registerWithSig und das Nachspielen pruefen.
// Fixtur: testdata/bioverifier_v4_beweise.json, erzeugt mit dem Schaltkreis
// biometric_v3 aus aequitas-proof-server (zeremonie/README.md):
//   - gut: Beweis mit dem Schluessel der Zeremonie v4 -> wahr
//   - alter_schluessel: derselbe Zeuge mit dem alten Ein-Personen-Schluessel
//     (bio_v3_0001.zkey) -> falsch. Wer dessen Zufall kannte, kann damit
//     nichts mehr.
//   - verfaelscht: der gute Beweis mit geaenderter oeffentlicher Ausgabe
//     (anderer Nullifier) -> falsch.

import (
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

type bioBeweis [4]json.RawMessage // pA, pB, pC, pubSignals wie soliditycalldata

func bioVerifyProofDaten(t *testing.T, b bioBeweis, nullifierAendern bool) []byte {
	t.Helper()
	var pA, pC, pub [2]string
	var pB [2][2]string
	for i, ziel := range []any{&pA, &pB, &pC, &pub} {
		if err := json.Unmarshal(b[i], ziel); err != nil {
			t.Fatalf("Fixtur Teil %d: %v", i, err)
		}
	}
	zahl := func(s string) *big.Int {
		n, ok := new(big.Int).SetString(s, 0)
		if !ok {
			t.Fatalf("keine Zahl: %q", s)
		}
		return n
	}
	data := selektor("verifyProof(uint256[2],uint256[2][2],uint256[2],uint256[2])")
	wort := func(n *big.Int) { data = append(data, common.BigToHash(n).Bytes()...) }
	for _, s := range pA {
		wort(zahl(s))
	}
	for _, zeile := range pB {
		for _, s := range zeile {
			wort(zahl(s))
		}
	}
	for _, s := range pC {
		wort(zahl(s))
	}
	for i, s := range pub {
		n := zahl(s)
		if nullifierAendern && i == 1 {
			n = new(big.Int).Add(n, big.NewInt(1))
		}
		wort(n)
	}
	return data
}

func TestBioVerifier_EchteBeweise(t *testing.T) {
	roh, err := os.ReadFile("testdata/bioverifier_v4_beweise.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Gut             bioBeweis `json:"gut"`
		AlterSchluessel bioBeweis `json:"alter_schluessel"`
	}
	if err := json.Unmarshal(roh, &f); err != nil {
		t.Fatal(err)
	}
	ts := uint64(1_790_000_000)
	von := common.HexToAddress("0x00000000000000000000000000000000000d0e03")
	bio := common.HexToAddress(BIO_VERIFIER_ADDR)
	for _, fall := range []struct {
		name  string
		daten []byte
		soll  bool
	}{
		{"gut", bioVerifyProofDaten(t, f.Gut, false), true},
		{"alter-schluessel", bioVerifyProofDaten(t, f.AlterSchluessel, false), false},
		{"verfaelscht", bioVerifyProofDaten(t, f.Gut, true), false},
	} {
		sdb, _, err := neueSpeicherStateDB()
		if err != nil {
			t.Fatal(err)
		}
		code := mustHexDecode(t, BioVerifierRuntimeBytecode)
		setzeCode(sdb, bio, code)
		ret, _, err := evmAufrufen(neueKnotenEVM(ts, von, sdb), von, bio, fall.daten, 5_000_000, big.NewInt(0))
		if err != nil {
			t.Fatalf("%s: Aufruf gescheitert: %v", fall.name, err)
		}
		ist := len(ret) == 32 && new(big.Int).SetBytes(ret).Sign() != 0
		if ist != fall.soll {
			t.Errorf("%s: verifyProof = %v, erwartet %v", fall.name, ist, fall.soll)
		}
	}
}
