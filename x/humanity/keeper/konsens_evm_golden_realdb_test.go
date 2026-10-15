package keeper

// Teil des Konsens-Vergleichstests (konsens_evm_golden_test.go): der echte
// Weg des Knotens gegen Postgres -- Deploy (EnsureContractsDeployed ->
// DeployContract), eth_call (CallContract) und ein persistierender Aufruf
// (Commit, Wiederoeffnen, Speicher nach Postgres). Opt-in wie alle *_RealDB-
// Tests: AEQUITAS_TPS_BENCH=1 und DATABASE_URL.

import (
	"bytes"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func evmSpeicherAuszug(t *testing.T, cs *ChainState, addr string) string {
	t.Helper()
	rows, err := cs.db.Query(`SELECT slot, value FROM evm_storage WHERE lower(address) = $1 ORDER BY slot`, strings.ToLower(addr))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var buf bytes.Buffer
	n := 0
	for rows.Next() {
		var s, v string
		if err := rows.Scan(&s, &v); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&buf, "%s=%s;", strings.ToLower(s), strings.ToLower(v))
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("zeilen=%d hash=%x", n, crypto.Keccak256(buf.Bytes()))
}

func konsensKnotenAufrufe(b *goldenBuch, prefix string, evm *EVMEngine, von common.Address, sigs []string) {
	ziel := common.HexToAddress(V7_CONTRACT_ADDR)
	for _, s := range sigs {
		ret, err := evm.CallContract(von, ziel, selektor(s), big.NewInt(0), false)
		b.add("%s/%s klasse=%s ret=%x", prefix, s, klasseVon(err), ret)
	}
}

func TestKonsensEVM_GoldenWerte_RealDB(t *testing.T) {
	v8DBLeeren(t) // prueft das Opt-in
	b := &goldenBuch{}
	deployer := "0x00000000000000000000000000000000000d0e01"
	_, alice := v8Schluessel(t, testKeyAlice)
	_, reg := v8Schluessel(t, testKeyRegistrar)

	func() {
		zurueck := _setVertragForTest(&vertragKonfig{version: vertragVersionV7})
		defer zurueck()
		cs := testKnoten(t, "unused-konsens-golden-v7.json")
		if !cs.useDB {
			t.Fatal("erwartet eine echte PostgreSQL-Verbindung -- DATABASE_URL pruefen")
		}
		evm, err := NewEVMEngine(cs)
		if err != nil {
			t.Fatal(err)
		}
		EnsureContractsDeployed(evm, cs, deployer)
		code, _ := cs.LoadContract(strings.ToLower(V7_CONTRACT_ADDR))
		bioCode, _ := cs.LoadContract(strings.ToLower(BIO_VERIFIER_ADDR))
		b.add("v7/deploy code=%x bio=%x speicher %s", crypto.Keccak256(code), crypto.Keccak256(bioCode), evmSpeicherAuszug(t, cs, V7_CONTRACT_ADDR))
		konsensKnotenAufrufe(b, "v7", evm, reg, []string{"name()", "symbol()", "decimals()", "totalSupply()", "getStats()", "fairShare()", "wealthCap()", "currentPhase()"})
		ret, err := evm.CallContract(alice, common.HexToAddress(V7_CONTRACT_ADDR), aufrufDaten("balanceOf(address)", alice), big.NewInt(0), true)
		b.add("v7/balanceOf-persist klasse=%s ret=%x speicher %s", klasseVon(err), ret, evmSpeicherAuszug(t, cs, V7_CONTRACT_ADDR))
		bioABI, err := abi.JSON(strings.NewReader(bioVerifierABI))
		if err != nil {
			t.Fatal(err)
		}
		g1, g2 := bn254Punkte()
		data, err := bioABI.Pack("verifyProof", g1, g2, g1, [2]*big.Int{big.NewInt(1001), big.NewInt(2001)})
		if err != nil {
			t.Fatal(err)
		}
		ret, err = evm.CallContract(reg, common.HexToAddress(BIO_VERIFIER_ADDR), data, big.NewInt(0), false)
		b.add("bio/verifyProof klasse=%s ret=%x", klasseVon(err), ret)
	}()

	v8DBLeeren(t)
	func() {
		zurueck := _setVertragForTest(&vertragKonfig{version: vertragVersionV8})
		defer zurueck()
		t.Setenv("RELAYER_PRIVATE_KEY", "")
		t.Setenv("RELAYER_ADDRESS", strings.ToLower(reg.Hex()))
		cs := testKnoten(t, "unused-konsens-golden-v8.json")
		evm, err := NewEVMEngine(cs)
		if err != nil {
			t.Fatal(err)
		}
		EnsureContractsDeployed(evm, cs, deployer)
		code, _ := cs.LoadContract(strings.ToLower(V7_CONTRACT_ADDR))
		b.add("v8/deploy code=%x speicher %s", crypto.Keccak256(code), evmSpeicherAuszug(t, cs, V7_CONTRACT_ADDR))
		konsensKnotenAufrufe(b, "v8", evm, reg, []string{"name()", "symbol()", "eip712Domain()", "verifier()", "NETZ_SALT()", "totalSupply()"})
		ret, err := evm.CallContract(reg, common.HexToAddress(V7_CONTRACT_ADDR), aufrufDaten("isRegistrar(address)", reg), big.NewInt(0), false)
		b.add("v8/isRegistrar klasse=%s ret=%x", klasseVon(err), ret)
	}()
	pruefeGolden(t, "konsens_evm_golden_realdb.txt", b)
}
