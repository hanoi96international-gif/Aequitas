package keeper

// Der BioVerifier wechselt mit der Zeremonie v4 (aequitas-proof-server,
// zeremonie/README.md) seinen Verifikationsschluessel. Der Konsens-
// Vergleichstest (konsens_evm_golden_test.go) belegt, dass go-ethereum v1.13.0
// und v1.17.8 den Verifier gleich ausfuehren -- erzeugt mit dem Code von v3.
// Er rechnet deshalb weiter mit genau diesem eingefrorenen Code.
//
// Damit der Beleg auch fuer den neuen Verifier gilt, prueft dieser Test: der
// laufende Verifier (BioVerifierRuntimeBytecode) ist Opcode fuer Opcode
// derselbe Code wie v3; verschieden duerfen nur die 32-Byte-Konstanten von
// PUSH32 sein (die Punkte des Verifikationsschluessels) und die
// Solidity-Metadaten am Ende. Gleiche Opcodes heissen gleiches Verhalten in
// beiden geth-Versionen, und die Konstanten sind fuer die EVM nur Daten.

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// bioVerifierV3Golden: BioVerifierV3.sol, wie bis zur Zeremonie v4 auf der
// Kette (solc 0.8.35, ohne Optimizer). Nur fuer den Konsens-Vergleichstest.
const bioVerifierV3Golden = "608060405234801561000f575f5ffd5b5060043610610029575f3560e01c8063f5c9d69e1461002d575b5f5ffd5b61004760048036038101906100429190610550565b61005d565b60405161005491906105d0565b60405180910390f35b5f6104cd565b7f30644e72e131a029b85045b68181585d2833e84879b9709143e1f593f00000018110610092575f5f5260205ff35b50565b5f60405183815284602082015285604082015260408160608360076107d05a03fa9150816100c5575f5f5260205ff35b825160408201526020830151606082015260408360808360066107d05a03fa9150816100f3575f5f5260205ff35b505050505050565b5f608086015f87017f1237393533f590b99d81ff1f29e32ce42a4dff6a4e1349077a052bb1ad47322281527f11ad4e3083fd90cf17c4e3c1f94c447e80208309076a7ca642cf360ec60af796602082015261019b5f8801357f120eebcf95bcf79b288015f5321b91718478cfe621897144dfd8b44c1ab13da57f260f178dbf404af7c21f38ab5b009fdbbccdcf130d55306a8d965bdd46dc120b84610095565b6101eb60208801357f0f33a8981748fb35d0ac483649d53da0baca9f3b487d84b5eb5cc6d49dbfd95a7f13492089e33095198c31617e8f62a29d648280d20a9ec2440697ee430915572e84610095565b833582527f30644e72e131a029b85045b68181585d97816a916871ca8d3c208c16d87cfd4760208501357f30644e72e131a029b85045b68181585d97816a916871ca8d3c208c16d87cfd4703066020830152843560408301526020850135606083015260408501356080830152606085013560a08301527f17e917b48c0967e98bfa6bf2b0ed56b9fbf834d146c953c7b142c4fcc842069260c08301527f123477e84f11a0c0954a6e8bce67c778bca96741b01028a94f841de7ef55f58660e08301527f2fc91ebb4c8b62f5e599153ea1736cf779324831dc5af14123c79942dce382eb6101008301527f1ca930e45a00b38e0412b635a774ca8faa1a64fd514c67fbb3742412ecc361bb6101208301527f14820191c66f14d6b254610a54a27264bdf53e93cda2bc53c280b5cab7dc010d6101408301527f303564c701f74a18327e37abc048ea9af95572aa1ae608e2b88281d90ddbef086101608301525f88015161018083015260205f018801516101a08301527f198e9393920d483a7260bfb731fb5d25f1aa493335a9e71297e485b7aef312c26101c08301527f1800deef121f1e76426a00665e5c4479674322d4f75edadd46debd5cd992f6ed6101e08301527f090689d0585ff075ec9e99ad690c3395bc4b313370b38ef355acdadcd122975b6102008301527f12c85ea5db8c6deb4aab71808dcb408fe3d1e7690c43d37b4ce6cc0166fa7daa610220830152853561024083015260208601356102608301527f1761ae9b1b31e3b6593c7c14c8a012374be39016c0100371a005bdccf5ed79a56102808301527f1cfa9899c11cf24a30809ccf4c6868a31d03ee3586246a25e04fd6c1a3c53b176102a08301527f1a18d2b73a7cd723d087ea1c0905fd959cda5cae986f352356589fe8b179ea706102c08301527f2eaac51fd93cdd15aff20e2f2564995a139c7b218e3a56d1f76d85f2a73af6ef6102e08301526020826103008460086107d05a03fa82518116935050505095945050505050565b60405161038081016040526104e45f840135610063565b6104f16020840135610063565b6104fe818486888a6100fb565b805f5260205ff35b5f5ffd5b5f5ffd5b5f819050826020600202820111156105295761052861050a565b5b92915050565b5f8190508260406002028201111561054a5761054961050a565b5b92915050565b5f5f5f5f610140858703121561056957610568610506565b5b5f6105768782880161050e565b94505060406105878782880161052f565b93505060c06105988782880161050e565b9250506101006105aa8782880161050e565b91505092959194509250565b5f8115159050919050565b6105ca816105b6565b82525050565b5f6020820190506105e35f8301846105c1565b9291505056fea2646970667358221220e5c7a0327877bacc77805b00fb010b5e897fee9e468e5e955f304ab42263941864736f6c63430008230033"

// opcodesOhneKonstanten: der Code ohne Metadaten, die Daten jedes PUSH32 durch
// Nullen ersetzt. Andere PUSH-Daten (Sprungziele, Offsets, Feldgrenzen)
// bleiben -- sie muessen gleich sein.
func opcodesOhneKonstanten(t *testing.T, h string) []byte {
	t.Helper()
	code, err := hex.DecodeString(h)
	if err != nil || len(code) < 2 {
		t.Fatalf("Bytecode nicht lesbar: %v", err)
	}
	meta := int(code[len(code)-2])<<8 | int(code[len(code)-1])
	if meta+2 > len(code) {
		t.Fatalf("Metadatenlaenge %d ungueltig", meta)
	}
	code = append([]byte{}, code[:len(code)-meta-2]...)
	for i := 0; i < len(code); i++ {
		op := code[i]
		if op >= 0x60 && op <= 0x7f {
			n := int(op - 0x5f)
			if op == 0x7f {
				for j := i + 1; j <= i+n && j < len(code); j++ {
					code[j] = 0
				}
			}
			i += n
		}
	}
	return code
}

func TestBioVerifier_GleicherCodeWieV3BisAufSchluessel(t *testing.T) {
	alt := opcodesOhneKonstanten(t, bioVerifierV3Golden)
	neu := opcodesOhneKonstanten(t, BioVerifierRuntimeBytecode)
	if !bytes.Equal(alt, neu) {
		t.Fatalf("BioVerifierRuntimeBytecode weicht von v3 nicht nur in den Schluesselkonstanten ab (%d gegen %d Byte) -- "+
			"der Konsens-Vergleichstest deckt ihn dann nicht mehr ab", len(neu), len(alt))
	}
	// Gegenprobe: ein veraenderter Opcode faellt auf.
	kaputt := []byte(BioVerifierRuntimeBytecode)
	copy(kaputt[0:2], "61") // PUSH1 0x80 -> PUSH2
	if bytes.Equal(alt, opcodesOhneKonstanten(t, string(kaputt))) {
		t.Fatal("Gegenprobe: ein geaenderter Opcode wurde nicht erkannt")
	}
}
