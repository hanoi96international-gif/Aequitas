package keeper

// Teil des SSTORE-Belegs (konsens_evm_knotenaufruf_test.go): Aufrufdaten mit
// 1-3 Byte.
//
// checkPersistedCallAllowed laesst Daten unter 4 Byte ohne Pruefung durch
// (kein Selektor) und prueft dann auch das Ziel nicht; sendRawTransaction
// ruft CallContract mit persist=true fuer jede Laenge > 0 auf. Ein
// signierter Aufruf mit 1-3 Byte an V7, V8 oder BioVerifier erreicht also
// die EVM des Knotens -- je Aufruf mit frischer StateDB und leerer
// Zugriffsliste. Der Knotenaufruf-Test deckt nur die zugelassenen Selektoren
// ab, dieser Fall fehlte.
//
// Statt Golden-Werten aus v1.13.0 prueft der Test die Bedingung selbst, unter
// der v1.13.0 abbrach: ein SSTORE, solange die Adresse des Vertrags nicht in
// der Zugriffsliste steht. Jede SSTORE-Gasfunktion (auch in v1.17) liest
// zuerst GetStateAndCommittedState, bevor sie den Slot eintraegt; die Wache
// unten schaut genau dort nach. Erwartet: kein solcher SSTORE und ein Revert
// (der Solidity-Dispatcher bricht bei weniger als 4 Byte ab, keiner der
// Vertraege hat fallback/receive).

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"
)

// zugriffslisteWache: die StateDB des Knotens, die jeden SSTORE meldet, der
// in v1.13.0 mit "address was not present in access list" abbrach.
type zugriffslisteWache struct {
	*state.StateDB
	sstores    int
	verstoesse []string
}

func (w *zugriffslisteWache) GetStateAndCommittedState(a common.Address, s common.Hash) (common.Hash, common.Hash) {
	w.sstores++
	if !w.StateDB.AddressInAccessList(a) {
		w.verstoesse = append(w.verstoesse, fmt.Sprintf("SSTORE in %s Slot %x ohne Adresse in der Zugriffsliste", a.Hex(), s))
	}
	return w.StateDB.GetStateAndCommittedState(a, s)
}

// bewachterAufruf: wie neueKnotenEVM + evmAufrufen, nur mit der Wache als
// StateDB.
func bewachterAufruf(sdb *state.StateDB, ts uint64, from, to common.Address, data []byte) (*zugriffslisteWache, []byte, error) {
	w := &zugriffslisteWache{StateDB: sdb}
	evm := vm.NewEVM(blockContext(ts), w, chainConfigFuer(ts), vm.Config{})
	evm.SetTxContext(vm.TxContext{Origin: from, GasPrice: new(uint256.Int)})
	ret, _, err := sicher(func() ([]byte, uint64, error) {
		return evmAufrufen(evm, from, to, data, 30_000_000, big.NewInt(0))
	})
	return w, ret, err
}

// Gegenprobe: die Wache erkennt den Fall, den v1.13.0 nicht rechnen konnte,
// und meldet nichts, wenn vorher gelesen wurde.
func TestZugriffslisteWache_Gegenprobe(t *testing.T) {
	ts := uint64(1_790_000_000)
	von := common.HexToAddress("0x00000000000000000000000000000000000d0e02")
	for _, f := range []struct {
		name, code string
		verstoss   bool
	}{
		{"sstore-zuerst", "6001600055" + "00", true},                   // SSTORE(0, 1)
		{"sload-dann-sstore", "60005450" + "6001600055" + "00", false}, // SLOAD(0); SSTORE(0, 1)
	} {
		sdb, _, err := neueSpeicherStateDB()
		if err != nil {
			t.Fatal(err)
		}
		ziel := common.HexToAddress("0x00000000000000000000000000000000000c0de1")
		code, _ := hex.DecodeString(f.code)
		setzeCode(sdb, ziel, code)
		w, _, err := bewachterAufruf(sdb, ts, von, ziel, nil)
		if err != nil {
			t.Fatalf("%s: %v", f.name, err)
		}
		if got := len(w.verstoesse) > 0; got != f.verstoss || w.sstores != 1 {
			t.Fatalf("%s: Verstoss=%v (erwartet %v), SSTOREs=%d", f.name, got, f.verstoss, w.sstores)
		}
	}
}

// kurzeAufrufdaten: alle 1-Byte-Werte und je Selektor im Code (PUSH4) die
// Praefixe mit 2 und 3 Byte.
func kurzeAufrufdaten(codes ...[]byte) [][]byte {
	gesehen := map[string]bool{}
	var out [][]byte
	neu := func(d []byte) {
		if !gesehen[string(d)] {
			gesehen[string(d)] = true
			out = append(out, append([]byte{}, d...))
		}
	}
	for i := 0; i < 256; i++ {
		neu([]byte{byte(i)})
	}
	neu([]byte{0, 0})
	neu([]byte{0xff, 0xff, 0xff})
	for _, c := range codes {
		for i := 0; i < len(c); i++ {
			op := c[i]
			if op == 0x63 && i+4 < len(c) { // PUSH4
				neu(c[i+1 : i+3])
				neu(c[i+1 : i+4])
			}
			if op >= 0x60 && op <= 0x7f {
				i += int(op - 0x5f)
			}
		}
	}
	return out
}

func TestKonsensEVM_KurzeAufrufdatenOhneSSTOREVorZugriff(t *testing.T) {
	ts := uint64(1_790_000_000)
	deployer := common.HexToAddress("0x00000000000000000000000000000000000d0e01")
	_, alice := v8Schluessel(t, testKeyAlice)
	_, reg := v8Schluessel(t, testKeyRegistrar)
	t.Setenv("RELAYER_PRIVATE_KEY", "")
	t.Setenv("RELAYER_ADDRESS", strings.ToLower(reg.Hex()))
	mock, _ := hex.DecodeString(mockGroth16VerifierBytecode)
	v7Code, _ := hex.DecodeString(V7ContractBytecode)
	v8Code, _ := hex.DecodeString(V8ContractBytecode)
	bio, _ := hex.DecodeString(BioVerifierRuntimeBytecode)
	ziel := common.HexToAddress(V7_CONTRACT_ADDR)
	bioAdr := common.HexToAddress(BIO_VERIFIER_ADDR)

	pruefe := func(t *testing.T, k *knotenSpeicher, name string, to common.Address) {
		t.Helper()
		daten := kurzeAufrufdaten(k.code[to])
		for _, von := range []common.Address{reg, alice} {
			for _, d := range daten {
				// Die Luecke, die dieser Test abdeckt: keine Pruefung vor der EVM.
				if err := checkPersistedCallAllowed(to, d, strings.ToLower(von.Hex())); err != nil {
					t.Fatalf("%s %x: checkPersistedCallAllowed lehnt jetzt ab (%v) -- Kommentar oben anpassen", name, d, err)
				}
				sdb, _, err := k.stateDB(append([]common.Address{von, to}, addressLikeWordsInCalldata(d)...))
				if err != nil {
					t.Fatal(err)
				}
				w, _, err := bewachterAufruf(sdb, ts, von, to, d)
				for _, v := range w.verstoesse {
					t.Errorf("%s %x von %s: %s", name, d, von.Hex(), v)
				}
				if err == nil {
					t.Errorf("%s %x von %s: Aufruf gelang (erwartet Revert)", name, d, von.Hex())
				} else if !strings.Contains(err.Error(), vm.ErrExecutionReverted.Error()) {
					t.Errorf("%s %x von %s: %v (erwartet Revert)", name, d, von.Hex(), err)
				}
			}
		}
		t.Logf("%s: %d Aufrufdaten je Absender, kein SSTORE vor einem Speicherzugriff", name, len(daten))
	}

	t.Run("V7", func(t *testing.T) {
		zurueck := _setVertragForTest(&vertragKonfig{version: vertragVersionV7})
		defer zurueck()
		mockAddr := crypto.CreateAddress(deployer, 0)
		adr, frisch := konsensDeployFuerKnoten(t, ts, deployer, mock, append(append([]byte{}, v7Code...), kwort(mockAddr)...))
		k := neuerKnotenSpeicher()
		k.uebernimm(frisch, adr[0], adr[0])
		k.uebernimm(frisch, adr[1], ziel)
		k.code[bioAdr] = bio
		pruefe(t, k, "v7", ziel)
		pruefe(t, k, "bioverifier", bioAdr)
	})

	t.Run("V8", func(t *testing.T) {
		zurueck := _setVertragForTest(&vertragKonfig{version: vertragVersionV8})
		defer zurueck()
		mockAddr := crypto.CreateAddress(deployer, 0)
		args, err := v8KonstruktorArgumente(mockAddr, []common.Address{reg}, testSalt)
		if err != nil {
			t.Fatal(err)
		}
		adr, frisch := konsensDeployFuerKnoten(t, ts, deployer, mock, append(append([]byte{}, v8Code...), args...))
		k := neuerKnotenSpeicher()
		k.uebernimm(frisch, adr[0], adr[0])
		k.uebernimm(frisch, adr[1], ziel)
		regSlot := mappingSlot(reg.Bytes(), v8SlotIsRegistrar)
		k.setze(ziel, regSlot, frisch.GetState(adr[1], regSlot))
		pruefe(t, k, "v8", ziel)
	})
}
