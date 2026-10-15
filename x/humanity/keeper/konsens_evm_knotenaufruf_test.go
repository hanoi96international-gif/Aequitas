package keeper

// Teil des Konsens-Vergleichstests (konsens_evm_golden_test.go): jeder
// Selektor, den checkPersistedCallAllowed (evm_engine.go) fuer einen
// persistierenden Aufruf zulaesst, so aufgerufen, wie CallContract es tut --
// je Aufruf eine frische StateDB (newStateDB: Code per SetCode, Speicher per
// SetState aus der "Datenbank", Guthaben nur fuer from/to und die
// adressfoermigen Woerter der Aufrufdaten), eine neue EVM mit 30 Mio. Gas,
// Wert 0, und nach einem gelungenen persistierenden Aufruf Festschreiben,
// Wiederoeffnen und Sichern genau der Slots, die dumpAndPersistStorage-
// WithNullifier sichert.
//
// Warum: v1.13.0 brach mit einer Panik ab ("impossible case: address was not
// present in access list during sstore op"), wenn im Aufrufziel ein SSTORE
// vor jedem Speicherzugriff kam -- das Ziel steht ohne Prepare nicht in der
// Zugriffsliste, erst SLOAD (oder ein CALL von aussen) traegt es ein. v1.17
// rechnet dort normal. Eine Funktion, die bisher immer scheiterte, koennte
// nach dem Update gelingen. Der Golden-Test im Knotenmuster
// (konsens_evm_golden_test.go) nutzt eine einzige StateDB ueber alle Aufrufe;
// dort steht V7 nach dem ersten SLOAD fuer immer in der Zugriffsliste, das
// deckt den Fall also nicht ab. Hier ist die StateDB wie im Knoten je Aufruf
// frisch, die Zugriffsliste also leer.
//
// Je Selektor ein Gutfall und ein erwarteter Revert. Ausnahmen:
//   - allowance (dd62ed3e) steht in der Zulassungsliste, AequitasV7 hat die
//     Funktion aber nicht: jeder Aufruf faellt in den Dispatcher-Revert. Ein
//     Gutfall ist nicht moeglich.
//   - name/symbol/decimals/totalSupply haben keine Argumente; ueber
//     CallContract gibt es keinen Revert (Wert > 0 lehnt CallContract vor der
//     EVM ab). Der Revert-Fall laeuft deshalb mit Wert 1 direkt in derselben
//     frischen StateDB (Solidity: nonpayable -> Revert); CallContract selbst
//     lehnt denselben Aufruf ab (Zeile "...-ueber-callcontract").

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/crypto"
)

// knotenSpeicher steht fuer PostgreSQL: Vertragscode, evm_storage und die
// Guthaben aus dem Go-Zustand.
type knotenSpeicher struct {
	code     map[common.Address][]byte
	speicher map[common.Address]map[common.Hash]common.Hash
	guthaben map[common.Address]*big.Int
}

func neuerKnotenSpeicher() *knotenSpeicher {
	return &knotenSpeicher{
		code:     map[common.Address][]byte{},
		speicher: map[common.Address]map[common.Hash]common.Hash{},
		guthaben: map[common.Address]*big.Int{},
	}
}

func (k *knotenSpeicher) setze(addr common.Address, slot, wert common.Hash) {
	m := k.speicher[addr]
	if m == nil {
		m = map[common.Hash]common.Hash{}
		k.speicher[addr] = m
	}
	if wert == (common.Hash{}) {
		delete(m, slot)
		return
	}
	m[slot] = wert
}

func sortierteAdressen(m map[common.Address][]byte) []common.Address {
	out := make([]common.Address, 0, len(m))
	for a := range m {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return bytes.Compare(out[i][:], out[j][:]) < 0 })
	return out
}

func sortierteSlots(m map[common.Hash]common.Hash) []common.Hash {
	out := make([]common.Hash, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return bytes.Compare(out[i][:], out[j][:]) < 0 })
	return out
}

// stateDB: wie newStateDB(onlyAddrs...).
func (k *knotenSpeicher) stateDB(onlyAddrs []common.Address) (*state.StateDB, state.Database, error) {
	sdb, db, err := neueSpeicherStateDB()
	if err != nil {
		return nil, nil, err
	}
	for _, a := range onlyAddrs {
		if g, ok := k.guthaben[a]; ok {
			setzeGuthaben(sdb, a, g)
		}
	}
	for _, a := range sortierteAdressen(k.code) {
		setzeCode(sdb, a, k.code[a])
		m := k.speicher[a]
		for _, s := range sortierteSlots(m) {
			sdb.SetState(a, s, m[s])
		}
	}
	return sdb, db, nil
}

func (k *knotenSpeicher) auszug(addr common.Address) string {
	var buf bytes.Buffer
	m := k.speicher[addr]
	for _, s := range sortierteSlots(m) {
		buf.Write(s.Bytes())
		buf.Write(m[s].Bytes())
	}
	return fmt.Sprintf("slots=%d hash=%x", len(m), crypto.Keccak256(buf.Bytes())[:16])
}

// persistiere: wie dumpAndPersistStorageWithNullifier, nur in k statt in
// PostgreSQL.
func (k *knotenSpeicher) persistiere(frisch *state.StateDB, addr common.Address, touchedAddrs []common.Address, touchedCommitments []*big.Int, calldataNullifier *[32]byte, released [][32]byte) {
	einfach, proAdresse, felder, commitBasis := v7SimpleSlots, v7AddressMappingSlots, v7ArrayBaseSlots, int64(7)
	nullifierBasis, grantBasis := int64(8), int64(29)
	if vertragV8() {
		einfach, proAdresse, felder, commitBasis = v8SimpleSlots, v8AddressMappingSlots, nil, v8SlotUsedCommitments
		nullifierBasis, grantBasis = v8SlotUsedNullifiers, -1
	}
	lies := func(slot common.Hash) { k.setze(addr, slot, frisch.GetState(addr, slot)) }
	for _, i := range append(append([]int64{}, einfach...), felder...) {
		lies(common.BigToHash(big.NewInt(i)))
	}
	for _, a := range touchedAddrs {
		for _, base := range proAdresse {
			lies(mappingSlot(a.Bytes(), base))
		}
	}
	for _, c := range touchedCommitments {
		lies(mappingSlotBytes32(common.BigToHash(c), commitBasis))
	}
	if !vertragV8() {
		for _, n := range released {
			for _, base := range []int64{8, 29} {
				lies(mappingSlotBytes32(common.BytesToHash(n[:]), base))
			}
		}
	}
	if calldataNullifier != nil {
		key := common.BytesToHash(calldataNullifier[:])
		if key != (common.Hash{}) {
			if s := mappingSlotBytes32(key, nullifierBasis); frisch.GetState(addr, s) != (common.Hash{}) {
				lies(s)
			}
			if grantBasis >= 0 {
				if s := mappingSlotBytes32(key, grantBasis); frisch.GetState(addr, s) != (common.Hash{}) {
					lies(s)
				}
			}
		}
	}
}

// aufruf: wie CallContract(from, to, data, 0, persist).
func (k *knotenSpeicher) aufruf(t *testing.T, b *goldenBuch, name string, ts uint64, from, to common.Address, data []byte, persist bool) {
	t.Helper()
	if persist {
		if err := checkPersistedCallAllowed(to, data, strings.ToLower(from.Hex())); err != nil {
			b.add("%s zulassung=abgelehnt", name)
			b.add("#%s fehler=%q", name, err.Error())
			return
		}
	}
	onlyAddrs := append([]common.Address{from, to}, addressLikeWordsInCalldata(data)...)
	sdb, db, err := k.stateDB(onlyAddrs)
	if err != nil {
		t.Fatal(err)
	}
	if len(sdb.GetCode(to)) == 0 {
		t.Fatalf("%s: kein Code an %s", name, to.Hex())
	}
	var preAddrs []common.Address
	var released [][32]byte
	if persist {
		preAddrs, released = extractPreCallEntities(sdb, to, from, data)
	}
	const gas = 30_000_000
	evm := neueKnotenEVM(ts, from, sdb)
	ret, rest, err := sicher(func() ([]byte, uint64, error) {
		return evmAufrufen(evm, from, to, data, gas, big.NewInt(0))
	})
	l := &evmLauf{b: b, sdb: sdb}
	l.notiere(name, ret, gas, rest, err)
	if err != nil || !persist {
		return
	}
	root, err := evmFestschreiben(sdb)
	if err != nil {
		t.Fatalf("%s: Festschreiben: %v", name, err)
	}
	frisch, err := oeffneStateDB(root, db)
	if err != nil {
		t.Fatalf("%s: Wiederoeffnen: %v", name, err)
	}
	touched, commitments := extractTouchedEntities(from, data)
	touched = append(touched, preAddrs...)
	_, _, nullifier := extractTouchedEntitiesWithNullifier(from, data)
	k.persistiere(frisch, to, touched, commitments, nullifier, released)
	b.add("%s festgeschrieben=%x speicher %s", name, root, k.auszug(to))
}

// mitWert: derselbe Aufruf mit Wert 1 in derselben Art frischer StateDB,
// direkt in der EVM. Dazu die Zeile, dass CallContract ihn vorher ablehnt.
func (k *knotenSpeicher) mitWert(t *testing.T, b *goldenBuch, name string, ts uint64, from, to common.Address, data []byte) {
	t.Helper()
	sdb, _, err := k.stateDB(append([]common.Address{from, to}, addressLikeWordsInCalldata(data)...))
	if err != nil {
		t.Fatal(err)
	}
	const gas = 30_000_000
	evm := neueKnotenEVM(ts, from, sdb)
	ret, rest, err := sicher(func() ([]byte, uint64, error) {
		return evmAufrufen(evm, from, to, data, gas, big.NewInt(1))
	})
	(&evmLauf{b: b, sdb: sdb}).notiere(name, ret, gas, rest, err)
	if _, err := (&EVMEngine{}).CallContract(from, to, data, big.NewInt(1), true); err != nil {
		b.add("%s-ueber-callcontract abgelehnt=%v", name, strings.Contains(err.Error(), "msg.value > 0"))
	} else {
		b.add("%s-ueber-callcontract angenommen", name)
	}
}

// konsensDeployFuerKnoten legt die Vertraege an wie DeployContract (CREATE,
// Festschreiben, Wiederoeffnen, Slots 0-199) und liefert Laufzeitcode und
// Speicher.
func konsensDeployFuerKnoten(t *testing.T, ts uint64, deployer common.Address, codes ...[]byte) ([]common.Address, *state.StateDB) {
	t.Helper()
	sdb, db, err := neueSpeicherStateDB()
	if err != nil {
		t.Fatal(err)
	}
	var adressen []common.Address
	for i, c := range codes {
		setzeNonce(sdb, deployer, uint64(i))
		_, a, _, err := evmErzeugen(neueKnotenEVM(ts, deployer, sdb), deployer, c, 30_000_000, big.NewInt(0))
		if err != nil {
			t.Fatalf("Deploy %d: %v", i, err)
		}
		adressen = append(adressen, a)
	}
	root, err := evmFestschreiben(sdb)
	if err != nil {
		t.Fatal(err)
	}
	frisch, err := oeffneStateDB(root, db)
	if err != nil {
		t.Fatal(err)
	}
	return adressen, frisch
}

func (k *knotenSpeicher) uebernimm(frisch *state.StateDB, quelle, ziel common.Address) {
	k.code[ziel] = frisch.GetCode(quelle)
	for i := int64(0); i < 200; i++ {
		s := common.BigToHash(big.NewInt(i))
		k.setze(ziel, s, frisch.GetState(quelle, s))
	}
}

func konsensKnotenaufrufe(t *testing.T, b *goldenBuch) {
	ts := uint64(1_790_000_000)
	deployer := common.HexToAddress("0x00000000000000000000000000000000000d0e01")
	aliceKey, alice := v8Schluessel(t, testKeyAlice)
	malloryKey, mallory := v8Schluessel(t, testKeyMallory)
	_, reg := v8Schluessel(t, testKeyRegistrar)
	bobKey, bob := v8Schluessel(t, "4444444444444444444444444444444444444444444444444444444444444444")
	ziel := common.HexToAddress(V7_CONTRACT_ADDR)
	t.Setenv("RELAYER_PRIVATE_KEY", "")
	t.Setenv("RELAYER_ADDRESS", strings.ToLower(reg.Hex()))
	guthaben := func(k *knotenSpeicher) {
		for i, a := range []common.Address{alice, bob, mallory, reg} {
			k.guthaben[a] = evmBalanceWei(a.Hex(), float64(100*(i+1))+0.5)
		}
	}
	mock, _ := hex.DecodeString(mockGroth16VerifierBytecode)
	v7Code, _ := hex.DecodeString(V7ContractBytecode)
	v8Code, _ := hex.DecodeString(V8ContractBytecode)

	// ── V7
	func() {
		zurueck := _setVertragForTest(&vertragKonfig{version: vertragVersionV7})
		defer zurueck()
		// Nonce 0: Mock-Verifier, Nonce 1: V7 -- die Adresse des Verifiers
		// ist vorher bekannt.
		mockAddr := crypto.CreateAddress(deployer, 0)
		adr, frisch := konsensDeployFuerKnoten(t, ts, deployer, mock, append(append([]byte{}, v7Code...), kwort(mockAddr)...))
		k := neuerKnotenSpeicher()
		guthaben(k)
		k.uebernimm(frisch, adr[0], adr[0])
		k.uebernimm(frisch, adr[1], ziel)
		b.add("knotenaufruf/v7/start mock=%s code=%x speicher %s", strings.ToLower(adr[0].Hex()), crypto.Keccak256(k.code[ziel]), k.auszug(ziel))

		p := "knotenaufruf/v7/"
		for _, s := range []string{"name()", "symbol()", "decimals()", "totalSupply()"} {
			k.aufruf(t, b, p+s, ts, reg, ziel, selektor(s), true)
		}
		// registerWithSig (13b81eb0), nur vom Relayer.
		regAlice := v7RegistrierDaten(t, aliceKey, alice, ziel, big.NewInt(31), big.NewInt(41), v8TestMarker)
		k.aufruf(t, b, p+"registerWithSig-gut", ts, reg, ziel, regAlice, true)
		k.aufruf(t, b, p+"registerWithSig-wiederholt", ts, reg, ziel, regAlice, true)
		k.aufruf(t, b, p+"registerWithSig-fremde-unterschrift", ts, reg, ziel, v7RegistrierDaten(t, malloryKey, bob, ziel, big.NewInt(32), big.NewInt(42), v8TestMarker), true)
		k.aufruf(t, b, p+"registerWithSig-falscher-beweis", ts, reg, ziel, v7RegistrierDaten(t, bobKey, bob, ziel, big.NewInt(32), big.NewInt(42), 0xBAD), true)
		k.aufruf(t, b, p+"registerWithSig-kein-relayer", ts, mallory, ziel, v7RegistrierDaten(t, bobKey, bob, ziel, big.NewInt(32), big.NewInt(42), v8TestMarker), true)
		k.aufruf(t, b, p+"registerWithSig-bob", ts+60, reg, ziel, v7RegistrierDaten(t, bobKey, bob, ziel, big.NewInt(32), big.NewInt(42), v8TestMarker), true)
		// balanceOf (70a08231)
		k.aufruf(t, b, p+"balanceOf-gut", ts, reg, ziel, aufrufDaten("balanceOf(address)", alice), true)
		schmutzig := append(selektor("balanceOf(address)"), kwort(alice)...)
		schmutzig[4] = 0x01 // obere 12 Byte nicht null: keine gueltige Adresse
		k.aufruf(t, b, p+"balanceOf-keine-adresse", ts, reg, ziel, schmutzig, true)
		k.aufruf(t, b, p+"balanceOf-ohne-argument", ts, reg, ziel, selektor("balanceOf(address)"), true)
		// allowance (dd62ed3e): in V7 nicht vorhanden.
		k.aufruf(t, b, p+"allowance-nicht-im-vertrag", ts, reg, ziel, aufrufDaten("allowance(address,address)", alice, bob), true)
		// transfer (a9059cbb)
		zehn := new(big.Int).Mul(big.NewInt(10), weiPerAEQ)
		k.aufruf(t, b, p+"transfer-gut", ts+120, alice, ziel, aufrufDaten("transfer(address,uint256)", bob, zehn), true)
		k.aufruf(t, b, p+"transfer-kein-mensch", ts+120, mallory, ziel, aufrufDaten("transfer(address,uint256)", bob, zehn), true)
		k.aufruf(t, b, p+"transfer-zu-viel", ts+120, alice, ziel, aufrufDaten("transfer(address,uint256)", bob, new(big.Int).Mul(zehn, big.NewInt(1000))), true)
		k.aufruf(t, b, p+"transfer-an-sich-selbst", ts+120, bob, ziel, aufrufDaten("transfer(address,uint256)", bob, big.NewInt(1)), true)
		k.aufruf(t, b, p+"transfer-bob-alice", ts+180, bob, ziel, aufrufDaten("transfer(address,uint256)", alice, zehn), true)
		k.aufruf(t, b, p+"balanceOf-bob", ts+180, reg, ziel, aufrufDaten("balanceOf(address)", bob), true)
		k.aufruf(t, b, p+"totalSupply()-ende", ts+180, reg, ziel, selektor("totalSupply()"), true)
		// Die Views ohne Argumente: Revert nur mit Wert.
		for _, s := range []string{"name()", "symbol()", "decimals()", "totalSupply()"} {
			k.mitWert(t, b, p+s+"-mit-wert", ts, reg, ziel, selektor(s))
		}
		// Nicht zugelassen: wird vor der EVM abgelehnt.
		k.aufruf(t, b, p+"claimUBI-nicht-zugelassen", ts, alice, ziel, selektor("claimUBI()"), true)
	}()

	// ── V8
	func() {
		zurueck := _setVertragForTest(&vertragKonfig{version: vertragVersionV8})
		defer zurueck()
		mockAddr := crypto.CreateAddress(deployer, 0)
		args, err := v8KonstruktorArgumente(mockAddr, []common.Address{reg}, testSalt)
		if err != nil {
			t.Fatal(err)
		}
		adr, frisch := konsensDeployFuerKnoten(t, ts, deployer, mock, append(append([]byte{}, v8Code...), args...))
		k := neuerKnotenSpeicher()
		guthaben(k)
		k.uebernimm(frisch, adr[0], adr[0])
		k.uebernimm(frisch, adr[1], ziel)
		// isRegistrar[reg] (Zone B) setzt im Knoten v8RegistrarAbgleichen.
		regSlot := mappingSlot(reg.Bytes(), v8SlotIsRegistrar)
		k.setze(ziel, regSlot, frisch.GetState(adr[1], regSlot))
		b.add("knotenaufruf/v8/start code=%x speicher %s", crypto.Keccak256(k.code[ziel]), k.auszug(ziel))

		p := "knotenaufruf/v8/"
		c, n := big.NewInt(1001), big.NewInt(2001)
		frist := int64(ts) + 600
		digest := func(key *ecdsa.PrivateKey, human common.Address, c, n *big.Int, nonce int64) []byte {
			return v8Unterschrift(t, key, v8RegisterDigest(big.NewInt(1926), ziel, testSalt, human, c, n, big.NewInt(nonce), big.NewInt(frist)))
		}
		gut := v8Anfrage(t, alice, c, n, frist, digest(aliceKey, alice, c, n, 0), v8TestMarker)
		k.aufruf(t, b, p+"registerWithSig-fremde-unterschrift", ts, reg, ziel, v8Anfrage(t, alice, c, n, frist, digest(malloryKey, alice, c, n, 0), v8TestMarker), true)
		k.aufruf(t, b, p+"registerWithSig-falscher-beweis", ts, reg, ziel, v8Anfrage(t, alice, c, n, frist, digest(aliceKey, alice, c, n, 0), 0xBAD), true)
		k.aufruf(t, b, p+"registerWithSig-kein-relayer", ts, mallory, ziel, gut, true)
		k.aufruf(t, b, p+"registerWithSig-gut", ts, reg, ziel, gut, true)
		k.aufruf(t, b, p+"registerWithSig-wiederholt", ts, reg, ziel, gut, true)
		c2, n2 := big.NewInt(1002), big.NewInt(2002)
		k.aufruf(t, b, p+"registerWithSig-bob", ts+60, reg, ziel, v8Anfrage(t, bob, c2, n2, frist, digest(bobKey, bob, c2, n2, 0), v8TestMarker), true)
		k.aufruf(t, b, p+"registerWithSig-nullifier-doppelt", ts+60, reg, ziel, v8Anfrage(t, mallory, big.NewInt(1003), n2, frist, digest(malloryKey, mallory, big.NewInt(1003), n2, 0), v8TestMarker), true)
		// Unter V8 ist alles andere fail-closed.
		for _, s := range []string{"name()", "totalSupply()"} {
			k.aufruf(t, b, p+s+"-nicht-zugelassen", ts, reg, ziel, selektor(s), true)
		}
		k.aufruf(t, b, p+"transfer-nicht-zugelassen", ts, alice, ziel, aufrufDaten("transfer(address,uint256)", bob, big.NewInt(1)), true)
	}()
}
