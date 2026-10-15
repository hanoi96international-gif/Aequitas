package keeper

// Bekannte, erklaerte Abweichungen von go-ethereum v1.17.8 gegenueber den
// Golden-Werten aus v1.13.0 (konsens_evm_golden_test.go). Jede steht hier
// mit der genauen Zeile, die jetzt herauskommen muss, und dem Grund. Alles,
// was nicht hier steht, muss byte-gleich bleiben -- und ist es: Deploy und
// alle Aufrufe von V7, V8, BioVerifier (Gas, Rueckgaben, Logs, Speicher- und
// Zustandswurzeln), das Muster von newStateDB, alle Ueberweisungen mit
// Annahme/Ablehnung und die StateRoot des Knotens nach jedem Block.
//
// Keiner der abweichenden Opcodes kommt in einem Vertrag der Kette vor
// (TestKonsensEVM_VertraegeOhneAbweichendeOpcodes), keiner der Vertraege
// ruft 0x0a auf (BioVerifier nutzt nur bn254, 0x06-0x08).

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

type bekannteAbweichung struct {
	jetzt string // die Zeile, die v1.17.8 liefern muss ("" = entfaellt)
	grund string
}

var konsensBekannteAbweichungen = map[string]map[string]bekannteAbweichung{
	"konsens_evm_golden.txt": {
		"op/44 klasse": {"op/44 klasse=ok gas=50 ret= logs=-",
			"PREVRANDAO: v1.13.0 brach mit einer nil-Panik ab (Cancun ohne Merge, BlockContext.Random nil). " +
				"Seit v1.14 gelten Shanghai/Cancun nur mit Merge; blockContext setzt deshalb Random = 0x00..00, PREVRANDAO liefert 0"},
		"op/44 wurzel": {"op/44 wurzel=8fd40e65c2e6557cf43668b910afdc0d3e93d4438a06b7e2215c771dfdb8a7b2 speicher=56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421 code=66",
			"Folge von op/44: nach der Panik gab es keine Wurzel"},
		"op/4a klasse": {"op/4a klasse=ok gas=50 ret= logs=-",
			"BLOBBASEFEE (EIP-7516) gehoert in v1.17 zu Cancun, in v1.13.0 fehlte er (ungueltiger Opcode). " +
				"Nicht ueber die ChainConfig abschaltbar, ohne Cancun (MCOPY fuer V7/V8) zu verlieren; liefert 0 (BlobBaseFee = 0)"},
		"op/55 klasse": {"op/55 klasse=ok gas=22148 ret= logs=-",
			"SSTORE ohne vorheriges SLOAD in einem Vertrag ausserhalb der Zugriffsliste: v1.13.0 brach mit " +
				"\"impossible case: address was not present in access list\" ab, v1.17 berechnet das Gas normal"},
		"op/55 wurzel": {"op/55 wurzel=0d80fc641ff61cf8f40b9cb1f4dc36757b557593db3af957f6d16f3a009359ee speicher=f38f9f63c760d088d7dd04f743619b6291f63beebd8bdf530628f90e9cfa52d7 code=66",
			"Folge von op/55: nach der Panik gab es keine Wurzel"},
		"op/ff wurzel": {"op/ff wurzel=07639e8cb50c120982db43176f1cebb487d82bf75881de055c8e3a302987d64a speicher=56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421 code=66",
			"SELFDESTRUCT (EIP-6780): v1.13.0 hielt einen per SetCode geladenen Vertrag fuer \"in dieser Transaktion erzeugt\" " +
				"und loeschte ihn; v1.17 loescht nur per CREATE erzeugte. Gas und Erfolg gleich"},
		"pc/0a/pointEvaluation/pointEvaluation1 klasse": {"pc/0a/pointEvaluation/pointEvaluation1 klasse=abbruch gas=10000000 ret= logs=-",
			"KZG-Punktauswertung (0x0a): v1.13.0 pruefte gegen ein Test-Setup von vor der KZG-Zeremonie, v1.17 gegen das " +
				"Mainnet-Setup. Der Vektor aus v1.13.0 ist dort ungueltig, der Mainnet-Vektor gilt (TestKonsensEVM_KZGMainnetSetup)"},
		"pc/wurzel": {"pc/wurzel=e6f1c21461f710a835cb0e051f0506fbcb422c75c2d18887ea90499f3523ad13",
			"Folge von pointEvaluation1: der gescheiterte Aufruf legt das leere Konto 0x0a nicht an"},
		"ueb/typ4-setcode nicht-lesbar": {"",
			"Typ 4 (EIP-7702) liest v1.17 ein, v1.13.0 nicht. Abgelehnt wird er weiterhin (ueb/typ4-setcode pruefung=abgelehnt gleich): " +
				"absenderWiederherstellen kennt nur die Typen 0-3"},
		"ueb/typ4-setcode absender": {"ueb/typ4-setcode absender=0x0000000000000000000000000000000000000000 fehler=true typ=4",
			"Folge davon: Absender nicht wiederherstellbar (ErrTxTypeNotSupported)"},
	},
}

// Die Vertraege der Kette (Erzeugungscode von V7/V8, Laufzeitcode von
// BioVerifier) enthalten keinen Opcode, dessen Verhalten sich zwischen
// v1.13.0 und v1.17.8 geaendert hat. Lineare Disassemblierung bis zu den
// solc-Metadaten am Ende.
func TestKonsensEVM_VertraegeOhneAbweichendeOpcodes(t *testing.T) {
	abweichend := map[byte]string{
		0x44: "PREVRANDAO", 0x4a: "BLOBBASEFEE", 0x55: "SSTORE", 0xff: "SELFDESTRUCT", 0x1e: "CLZ", 0x4b: "SLOTNUM",
	}
	for name, h := range map[string]string{"V7": V7ContractBytecode, "V8": V8ContractBytecode, "BioVerifier": BioVerifierRuntimeBytecode} {
		code, err := hex.DecodeString(h)
		if err != nil {
			t.Fatal(err)
		}
		meta := int(code[len(code)-2])<<8 | int(code[len(code)-1])
		body := code[:len(code)-meta-2]
		sstores := 0
		for i := 0; i < len(body); i++ {
			op := body[i]
			if op == 0x55 {
				// SSTORE ist ueberall; abweichend war nur ein SSTORE, bevor
				// der Vertrag im Aufruf ueberhaupt Speicher gelesen hat
				// (v1.13.0: Panik, v1.17: normales Gas). Beleg, dass das V7
				// und V8 nicht trifft: konsens_evm_knotenaufruf_test.go ruft
				// jeden Selektor, den checkPersistedCallAllowed zulaesst
				// (V7: transfer, balanceOf, allowance, totalSupply, name,
				// symbol, decimals, registerWithSig 13b81eb0; V8:
				// registerWithSig 60529762), wie CallContract auf -- je
				// Aufruf frische StateDB, also leere Zugriffsliste -- im
				// Gutfall und im Revert-Fall. Die Zeilen "knotenaufruf/..."
				// der Golden-Datei sind auf v1.13.0 erzeugt (keine Panik) und
				// auf v1.17.8 byte-gleich, ohne Ausnahme.
				sstores++
			} else if n, ok := abweichend[op]; ok {
				t.Errorf("%s: Opcode %s (0x%02x) an Stelle %d", name, n, op, i)
			}
			if op >= 0x60 && op <= 0x7f {
				i += int(op) - 0x5f
			}
		}
		t.Logf("%s: %d Byte, %d SSTORE", name, len(code), sstores)
	}
}

// Der Mainnet-Vektor aus go-ethereum v1.17.8
// (core/vm/testdata/precompiles/pointEvaluation.json): mit dem echten Setup
// gilt er.
func TestKonsensEVM_KZGMainnetSetup(t *testing.T) {
	in, _ := hex.DecodeString("01e798154708fe7789429634053cbf9f99b619f9f084048927333fce637f549b564c0a11a0f704f4fc3e8acfe0f8245f0ad1347b378fbf96e206da11a5d3630624d25032e67a7e6a4910df5834b8fe70e6bcfeeac0352434196bdf4b2485d5a18f59a8d2a1a625a17f3fea0fe5eb8c896db3764f3185481bc22f91b4aaffcca25f26936857bc3a7c2539ea8ec3a952b7873033e038326e87ed3e1276fd140253fa08e9fc25fb2d9a98527fc22a2c9612fbeafdad446cbc7bcdbdcd780af2c16a")
	sdb, _, err := neueSpeicherStateDB()
	if err != nil {
		t.Fatal(err)
	}
	von := common.HexToAddress("0x0000000000000000000000000000000000000a11")
	ret, rest, err := evmAufrufen(neueKnotenEVM(1_790_000_000, von, sdb), von, common.BytesToAddress([]byte{0x0a}), in, 100_000, big.NewInt(0))
	if err != nil {
		t.Fatalf("Mainnet-Vektor abgelehnt: %v", err)
	}
	if want := "000000000000000000000000000000000000000000000000000000000000100073eda753299d7d483339d80809a1d80553bda402fffe5bfeffffffff00000001"; hex.EncodeToString(ret) != want || 100_000-rest != 50_000 {
		t.Fatalf("ret=%x gas=%d", ret, 100_000-rest)
	}
	// Ein manipulierter Beweis (letztes Byte) wird abgelehnt.
	in[len(in)-1] ^= 1
	if _, _, err := evmAufrufen(neueKnotenEVM(1_790_000_000, von, sdb), von, common.BytesToAddress([]byte{0x0a}), in, 100_000, big.NewInt(0)); err == nil {
		t.Fatal("manipulierter KZG-Beweis angenommen")
	}
}

// Welche Forks die EVM des Knotens aktiviert -- vorher (v1.13.0) wie
// nachher: alles bis einschliesslich Cancun ab Block 0 / Zeit 0, nichts
// danach. Prague, Osaka, die BPO-Forks, Amsterdam, Bogota und UBT/Verkle
// duerfen nicht still aktiv werden.
func TestKonsensEVM_KeineNeuenForks(t *testing.T) {
	c := chainConfig()
	for name, v := range map[string]*uint64{
		"Prague": c.PragueTime, "Osaka": c.OsakaTime, "BPO1": c.BPO1Time, "BPO2": c.BPO2Time, "BPO3": c.BPO3Time,
		"BPO4": c.BPO4Time, "BPO5": c.BPO5Time, "Amsterdam": c.AmsterdamTime, "Bogota": c.BogotaTime, "UBT": c.UBTTime,
	} {
		if v != nil {
			t.Errorf("%s ist in chainConfig gesetzt (%d)", name, *v)
		}
	}
	if c.ShanghaiTime == nil || *c.ShanghaiTime != 0 || c.CancunTime == nil || *c.CancunTime != 0 {
		t.Fatal("Shanghai und Cancun muessen ab Zeit 0 gelten")
	}
	// Die Regeln, mit denen die EVM wirklich laeuft, zu Zeiten weit vorn
	// und weit hinten.
	for _, ts := range []uint64{0, 1_790_000_000, 1 << 40} {
		evm := neueKnotenEVM(ts, common.Address{}, nil)
		r := evm.ChainConfig().Rules(evm.Context.BlockNumber, evm.Context.Random != nil, evm.Context.Time)
		if !(r.IsHomestead && r.IsEIP150 && r.IsEIP155 && r.IsEIP158 && r.IsByzantium && r.IsConstantinople &&
			r.IsPetersburg && r.IsIstanbul && r.IsBerlin && r.IsEIP2929 && r.IsLondon && r.IsMerge && r.IsShanghai && r.IsCancun) {
			t.Errorf("ts=%d: ein Fork bis Cancun fehlt: %+v", ts, r)
		}
		if r.IsPrague || r.IsOsaka || r.IsAmsterdam || r.IsBogota || r.IsUBT || r.IsEIP4762 {
			t.Errorf("ts=%d: ein Fork nach Cancun ist aktiv: %+v", ts, r)
		}
	}
}
