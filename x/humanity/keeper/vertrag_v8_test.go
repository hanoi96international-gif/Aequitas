package keeper

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
)

// AequitasV8 aus Sicht des Knotens (vertrag_v8.go). Die wichtigsten Tests
// fuehren den ECHTEN V8-Bytecode (V8ContractBytecode) in der EVM-Konfiguration
// des Knotens (chainConfig) aus und vergleichen: Digest, Signatur,
// Speicherplaetze, Konstruktor-Kodierung. Weicht Go vom Vertrag ab, wird es
// hier rot -- nicht erst nach einer Registrierung, die "signature invalid"
// meldet.

// Aus contracts/MockGroth16Verifier.sol (solc 0.8.28, Optimizer 200): nimmt
// nur Beweise mit pA[0] == 0xA11CE an. Nur Testgeruest; die echte
// Beweispruefung der Kette ist BioVerifier.
const mockGroth16VerifierBytecode = "60806040525f805460ff19166001179055348015601a575f5ffd5b506101a3806100285f395ff3fe608060405234801561000f575f5ffd5b506004361061004a575f3560e01c80632852b71c1461004e5780634fc3aa7c1461006f578063cae3d7a414610091578063f5c9d69e146100a9575b5f5ffd5b5f5461005a9060ff1681565b60405190151581526020015b60405180910390f35b61008f61007d3660046100d5565b5f805460ff1916911515919091179055565b005b61009b620a11ce81565b604051908152602001610066565b61005a6100b7366004610111565b5f805460ff1680156100cc57508435620a11ce145b95945050505050565b5f602082840312156100e5575f5ffd5b813580151581146100f4575f5ffd5b9392505050565b806040810183101561010b575f5ffd5b92915050565b5f5f5f5f6101408587031215610125575f5ffd5b61012f86866100fb565b935060c0850186811115610141575f5ffd5b60408601935061015187826100fb565b9250506101628661010087016100fb565b90509295919450925056fea2646970667358221220376e330a1c1d4b3918cf3f84b6957414d65a9c394097faec3788f121c3da8b3664736f6c634300081c0033"

const v8TestMarker = 0xA11CE

type v8Pruefstand struct {
	t         *testing.T
	sdb       *state.StateDB
	evm       *vm.EVM
	vertrag   common.Address
	registrar common.Address
	salt      common.Hash
}

func v8Schluessel(t *testing.T, hexKey string) (*ecdsa.PrivateKey, common.Address) {
	t.Helper()
	k, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		t.Fatal(err)
	}
	return k, crypto.PubkeyToAddress(k.PublicKey)
}

func neuerV8Pruefstand(t *testing.T, registrare []common.Address, salt common.Hash) *v8Pruefstand {
	t.Helper()
	sdb, err := state.New(common.Hash{}, state.NewDatabase(rawdb.NewMemoryDatabase()), nil)
	if err != nil {
		t.Fatal(err)
	}
	deployer := common.HexToAddress("0x00000000000000000000000000000000000d0e01")
	evm := vm.NewEVM(blockContext(1_790_000_000), vm.TxContext{Origin: deployer, GasPrice: big.NewInt(0)}, sdb, chainConfig(), vm.Config{})
	mock, _ := hex.DecodeString(mockGroth16VerifierBytecode)
	_, verifier, _, err := evm.Create(vm.AccountRef(deployer), mock, 30_000_000, big.NewInt(0))
	if err != nil {
		t.Fatalf("Verifier: %v", err)
	}
	args, err := v8KonstruktorArgumente(verifier, registrare, salt)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := hex.DecodeString(V8ContractBytecode)
	_, v8, _, err := evm.Create(vm.AccountRef(deployer), append(code, args...), 30_000_000, big.NewInt(0))
	if err != nil {
		t.Fatalf("V8 deploy (Konstruktor-Kodierung aus Go): %v", err)
	}
	return &v8Pruefstand{t: t, sdb: sdb, evm: evm, vertrag: v8, registrar: registrare[0], salt: salt}
}

func (p *v8Pruefstand) aufruf(von common.Address, data []byte) ([]byte, error) {
	ret, _, err := p.evm.Call(vm.AccountRef(von), p.vertrag, data, 5_000_000, big.NewInt(0))
	if err != nil {
		if r := decodeRevertReason(ret); r != "" {
			return nil, &revertFehler{r}
		}
	}
	return ret, err
}

type revertFehler struct{ grund string }

func (e *revertFehler) Error() string { return e.grund }

func sel(s string) []byte { b, _ := hex.DecodeString(s); return b }

// v8Anfrage baut Aufrufdaten mit demselben ABI, das register.go benutzt.
func v8Anfrage(t *testing.T, human common.Address, commitment, nullifier *big.Int, deadline int64, sig []byte, marker int64) []byte {
	t.Helper()
	parsed, err := abi.JSON(strings.NewReader(registerWithSigV8ABI))
	if err != nil {
		t.Fatal(err)
	}
	pA := [2]*big.Int{big.NewInt(marker), big.NewInt(1)}
	pB := [2][2]*big.Int{{big.NewInt(2), big.NewInt(3)}, {big.NewInt(4), big.NewInt(5)}}
	pC := [2]*big.Int{big.NewInt(6), big.NewInt(7)}
	data, err := parsed.Pack("registerWithSig", pA, pB, pC, [2]*big.Int{commitment, nullifier}, human, big.NewInt(deadline), sig)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func v8Unterschrift(t *testing.T, key *ecdsa.PrivateKey, digest common.Hash) []byte {
	t.Helper()
	sig, err := crypto.Sign(digest.Bytes(), key)
	if err != nil {
		t.Fatal(err)
	}
	sig[64] += 27
	return sig
}

const (
	testKeyAlice     = "1111111111111111111111111111111111111111111111111111111111111111"
	testKeyMallory   = "2222222222222222222222222222222222222222222222222222222222222222"
	testKeyRegistrar = "3333333333333333333333333333333333333333333333333333333333333333"
)

var testSalt = crypto.Keccak256Hash([]byte("aequitas-1926-1790000000"))

// ─── Genesis ────────────────────────────────────────────────────────────────

func TestV8Genesis_LesenUndAblehnen(t *testing.T) {
	r1 := "0x1000000000000000000000000000000000000001"
	gut := func(s string) vertragKonfig {
		t.Helper()
		k, err := ladeVertragKonfig([]byte(s))
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		return k
	}
	if k := gut(`{"chain_id":"aequitas-1","genesis_time":"2026-06-13T00:00:00Z"}`); k.version != vertragVersionV7 {
		t.Fatal("ohne register_vertrag muss es V7 bleiben (die laufende Kette)")
	}
	if k := gut(`{"register_vertrag":{"version":" V8 "}}`); k.version != vertragVersionV8 {
		t.Fatalf("V8 erwartet: %+v", k)
	}
	if k := gut(`{"register_vertrag":{"version":"v7"}}`); k.version != vertragVersionV7 {
		t.Fatalf("V7 erwartet: %+v", k)
	}
	schlecht := map[string]string{
		"unbekannte Version": `{"register_vertrag":{"version":"v9"}}`,
		"leere Version":      `{"register_vertrag":{}}`,
		// Eine Registrarliste wirkt nicht mehr (Registrar je Knoten) -- wer
		// sie hinschreibt, soll es beim Start merken.
		"Registrarliste V8": `{"register_vertrag":{"version":"v8","registrare":["` + r1 + `"]}}`,
		"Registrarliste V7": `{"register_vertrag":{"version":"v7","registrare":["` + r1 + `"]}}`,
		"kein JSON":         `{"register_vertrag":`,
	}
	for name, s := range schlecht {
		if _, err := ladeVertragKonfig([]byte(s)); err == nil {
			t.Errorf("%s: muss abgelehnt werden (Knoten startet dann nicht)", name)
		}
	}
}

func TestV8Registrar_IstDerEigeneRelayer(t *testing.T) {
	_, relayer := v8Schluessel(t, testKeyRegistrar)
	t.Setenv("RELAYER_PRIVATE_KEY", "")
	t.Setenv("RELAYER_ADDRESS", strings.ToLower(relayer.Hex()))
	if got := v8EigenerRegistrar(); got != relayer {
		t.Fatalf("Registrar = eigener Relayer erwartet: %s", got.Hex())
	}
	t.Setenv("RELAYER_ADDRESS", "")
	if got := v8EigenerRegistrar(); got != v8KeinRegistrar {
		t.Fatalf("ohne Relayer: niemand soll ueber diesen Knoten registrieren: %s", got.Hex())
	}
	t.Setenv("RELAYER_ADDRESS", "0x123")
	if got := v8EigenerRegistrar(); got != v8KeinRegistrar {
		t.Fatalf("kaputte Relayer-Adresse: %s", got.Hex())
	}
}

func TestV8Genesis_DieRepoGenesisBleibtV7(t *testing.T) {
	// Ein Push auf main geht direkt auf die Server -- deren genesis.json
	// darf durch diese Aenderung nicht zu V8 werden.
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	k, err := ladeVertragKonfig(data)
	if err != nil || k.version != vertragVersionV7 {
		t.Fatalf("genesis.json der laufenden Kette muss V7 bleiben: %+v %v", k, err)
	}
}

// ─── Tabelle ────────────────────────────────────────────────────────────────

func TestV8Slots_GleichDerTabelle(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "v8_slots.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tab struct {
		Storage []struct {
			Slot  int64  `json:"slot"`
			Label string `json:"label"`
		} `json:"storage"`
		Selectors struct {
			Functions []struct {
				Selector  string `json:"selector"`
				Signature string `json:"signature"`
				Persist   string `json:"persist"`
			} `json:"functions"`
		} `json:"selectors"`
	}
	if err := json.Unmarshal(data, &tab); err != nil {
		t.Fatal(err)
	}
	go_ := map[string]int64{
		"totalSupply": v8SlotTotalSupply, "totalHumans": v8SlotTotalHumans, "balanceOf": v8SlotBalanceOf,
		"isHuman": v8SlotIsHuman, "usedCommitments": v8SlotUsedCommitments, "usedNullifiers": v8SlotUsedNullifiers,
		"commitmentOf": v8SlotCommitmentOf, "nullifierOf": v8SlotNullifierOf, "nonces": v8SlotNonces, "isRegistrar": v8SlotIsRegistrar,
	}
	if len(tab.Storage) != len(go_) {
		t.Fatalf("Tabelle hat %d Plaetze, Go kennt %d", len(tab.Storage), len(go_))
	}
	for _, e := range tab.Storage {
		if s, ok := go_[e.Label]; !ok || s != e.Slot {
			t.Errorf("%s: Tabelle Slot %d, Go %d (%v)", e.Label, e.Slot, s, ok)
		}
	}
	registrarSel := 0
	for _, f := range tab.Selectors.Functions {
		if f.Persist == "registrar" {
			registrarSel++
			if strings.TrimPrefix(f.Selector, "0x") != v8RegisterSelector {
				t.Errorf("registerWithSig: Tabelle %s, Go %s", f.Selector, v8RegisterSelector)
			}
		}
	}
	if registrarSel != 1 {
		t.Errorf("genau eine persistierende Funktion erwartet, Tabelle hat %d", registrarSel)
	}
}

// ─── Go gegen den echten Vertrag ────────────────────────────────────────────

func TestV8_DigestGleichDemVertrag(t *testing.T) {
	_, reg := v8Schluessel(t, testKeyRegistrar)
	_, alice := v8Schluessel(t, testKeyAlice)
	p := neuerV8Pruefstand(t, []common.Address{reg}, testSalt)

	ret, err := p.aufruf(alice, sel("3644e515")) // DOMAIN_SEPARATOR()
	if err != nil || common.BytesToHash(ret) != v8DomainSeparator(p.vertrag, testSalt) {
		t.Fatalf("DOMAIN_SEPARATOR: Vertrag %x, Go %s (%v)", ret, v8DomainSeparator(p.vertrag, testSalt).Hex(), err)
	}
	c, n, d := big.NewInt(1001), big.NewInt(2001), big.NewInt(1_790_000_600)
	arg := append(append(append(common.LeftPadBytes(alice.Bytes(), 32), common.LeftPadBytes(c.Bytes(), 32)...),
		common.LeftPadBytes(n.Bytes(), 32)...), common.LeftPadBytes(d.Bytes(), 32)...)
	ret, err = p.aufruf(alice, append(sel("7694154f"), arg...)) // registrationDigest(address,uint256,uint256,uint256)
	want := v8RegisterDigest(p.vertrag, testSalt, alice, c, n, big.NewInt(0), d)
	if err != nil || common.BytesToHash(ret) != want {
		t.Fatalf("registrationDigest: Vertrag %x, Go %s (%v)", ret, want.Hex(), err)
	}
	ret, err = p.aufruf(alice, sel("67522052")) // NETZ_SALT()
	if err != nil || common.BytesToHash(ret) != testSalt {
		t.Fatalf("NETZ_SALT: %x %v", ret, err)
	}
}

func TestV8_GoSignaturWirdVomVertragAngenommen_UndSlotsStimmen(t *testing.T) {
	_, reg := v8Schluessel(t, testKeyRegistrar)
	aliceKey, alice := v8Schluessel(t, testKeyAlice)
	p := neuerV8Pruefstand(t, []common.Address{reg}, testSalt)

	// Der Konstruktor schreibt isRegistrar dort, wo ensureV8Deployed ihn
	// nach dem Umzug ausdruecklich hinschreibt.
	if got := p.sdb.GetState(p.vertrag, mappingSlot(reg.Bytes(), v8SlotIsRegistrar)); got != common.HexToHash("0x01") {
		t.Fatalf("isRegistrar liegt nicht bei mappingSlot(registrar, %d): %s", v8SlotIsRegistrar, got.Hex())
	}

	c, n := big.NewInt(1001), big.NewInt(2001)
	deadline := int64(1_790_000_600)
	sig := v8Unterschrift(t, aliceKey, v8RegisterDigest(p.vertrag, testSalt, alice, c, n, big.NewInt(0), big.NewInt(deadline)))
	if err := v8SignaturPruefen(v8RegisterDigest(p.vertrag, testSalt, alice, c, n, big.NewInt(0), big.NewInt(deadline)), sig, alice); err != nil {
		t.Fatalf("Go lehnt die eigene Signatur ab: %v", err)
	}
	data := v8Anfrage(t, alice, c, n, deadline, sig, v8TestMarker)
	if _, err := p.aufruf(reg, data); err != nil {
		t.Fatalf("Vertrag lehnt eine in Go gebaute Anfrage ab: %v", err)
	}

	// Die Plaetze, die Go spiegelt und nach einem Aufruf sichert.
	eins := common.HexToHash("0x01")
	pruefe := func(name string, slot common.Hash, want common.Hash) {
		t.Helper()
		if got := p.sdb.GetState(p.vertrag, slot); got != want {
			t.Errorf("%s: %s, erwartet %s", name, got.Hex(), want.Hex())
		}
	}
	pruefe("totalHumans", common.BigToHash(big.NewInt(v8SlotTotalHumans)), eins)
	pruefe("isHuman", mappingSlot(alice.Bytes(), v8SlotIsHuman), eins)
	pruefe("usedCommitments", mappingSlotBytes32(common.BigToHash(c), v8SlotUsedCommitments), eins)
	pruefe("usedNullifiers", mappingSlotBytes32(common.BigToHash(n), v8SlotUsedNullifiers), common.BytesToHash(alice.Bytes()))
	pruefe("commitmentOf", mappingSlot(alice.Bytes(), v8SlotCommitmentOf), common.BigToHash(c))
	pruefe("nullifierOf", mappingSlot(alice.Bytes(), v8SlotNullifierOf), common.BigToHash(n))
	pruefe("nonces", mappingSlot(alice.Bytes(), v8SlotNonces), eins)

	// Was CallContract nach dem Aufruf aus den Aufrufdaten liest.
	addrs, commits, null := extractTouchedEntitiesWithNullifier(reg, data)
	if len(addrs) != 2 || addrs[1] != alice || len(commits) != 1 || commits[0].Cmp(c) != 0 || null == nil || new(big.Int).SetBytes(null[:]).Cmp(n) != 0 {
		t.Fatalf("Aufrufdaten falsch gelesen: %v %v %x", addrs, commits, null)
	}
}

func TestV8_NonceIstImmerNull(t *testing.T) {
	// vertrag_v8.go verlangt beim Nachspielen Nonce 0. Das stimmt nur, weil
	// eine Wallet in V8 hoechstens einmal Mensch wird: der zweite Versuch
	// scheitert im Vertrag, egal mit welcher Nonce.
	_, reg := v8Schluessel(t, testKeyRegistrar)
	aliceKey, alice := v8Schluessel(t, testKeyAlice)
	p := neuerV8Pruefstand(t, []common.Address{reg}, testSalt)
	deadline := int64(1_790_000_600)
	anfrage := func(c, n, nonce int64) []byte {
		d := v8RegisterDigest(p.vertrag, testSalt, alice, big.NewInt(c), big.NewInt(n), big.NewInt(nonce), big.NewInt(deadline))
		return v8Anfrage(t, alice, big.NewInt(c), big.NewInt(n), deadline, v8Unterschrift(t, aliceKey, d), v8TestMarker)
	}
	if _, err := p.aufruf(reg, anfrage(1001, 2001, 0)); err != nil {
		t.Fatal(err)
	}
	for _, nonce := range []int64{0, 1} {
		if _, err := p.aufruf(reg, anfrage(1002, 2002, nonce)); err == nil || !strings.Contains(err.Error(), "already registered") {
			t.Fatalf("zweite Registrierung derselben Wallet (Nonce %d) muss scheitern: %v", nonce, err)
		}
	}
}

func TestV8_Missbrauch_VertragUndGoLehnenGleichAb(t *testing.T) {
	_, reg := v8Schluessel(t, testKeyRegistrar)
	aliceKey, alice := v8Schluessel(t, testKeyAlice)
	malloryKey, mallory := v8Schluessel(t, testKeyMallory)
	p := neuerV8Pruefstand(t, []common.Address{reg}, testSalt)
	c, n := big.NewInt(1001), big.NewInt(2001)
	deadline := int64(1_790_000_600)
	digest := v8RegisterDigest(p.vertrag, testSalt, alice, c, n, big.NewInt(0), big.NewInt(deadline))
	gut := v8Unterschrift(t, aliceKey, digest)

	// Hohes s: gueltige Signatur, formbarer Zwilling.
	hoch := append([]byte(nil), gut...)
	s := new(big.Int).SetBytes(hoch[32:64])
	nN, _ := new(big.Int).SetString("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", 16)
	copy(hoch[32:64], common.LeftPadBytes(new(big.Int).Sub(nN, s).Bytes(), 32))
	hoch[64] ^= 1 // 27 <-> 28

	anderesNetz := v8Unterschrift(t, aliceKey, v8RegisterDigest(p.vertrag, crypto.Keccak256Hash([]byte("aequitas-1926-1800000000")), alice, c, n, big.NewInt(0), big.NewInt(deadline)))
	vonMallory := v8Unterschrift(t, malloryKey, digest)
	vNull := append([]byte(nil), gut...)
	vNull[64] -= 27

	faelle := map[string][]byte{
		"hohes s":             hoch,
		"anderes Netz (Salt)": anderesNetz,
		"fremde Unterschrift": vonMallory,
		"v = 0/1":             vNull,
		"zu kurz":             gut[:64],
	}
	for name, sig := range faelle {
		if err := v8SignaturPruefen(digest, sig, alice); err == nil {
			t.Errorf("%s: Go nimmt an", name)
		}
		if _, err := p.aufruf(reg, v8Anfrage(t, alice, c, n, deadline, sig, v8TestMarker)); err == nil {
			t.Errorf("%s: Vertrag nimmt an", name)
		}
	}
	// Mallory reicht Alices Anfrage fuer sich ein (Front-Running).
	if _, err := p.aufruf(reg, v8Anfrage(t, mallory, c, n, deadline, gut, v8TestMarker)); err == nil {
		t.Error("umgelenkte Anfrage angenommen")
	}
	// Nur ein Registrar darf einreichen.
	if _, err := p.aufruf(alice, v8Anfrage(t, alice, c, n, deadline, gut, v8TestMarker)); err == nil {
		t.Error("Aufruf ohne Registrar angenommen")
	}
	// Und die gute geht.
	if _, err := p.aufruf(reg, v8Anfrage(t, alice, c, n, deadline, gut, v8TestMarker)); err != nil {
		t.Fatalf("gute Anfrage abgelehnt: %v", err)
	}
}

func TestEVM_CancunZeichenketten(t *testing.T) {
	// name()/symbol() von V7 und eip712Domain() von V8 benutzen MCOPY. Ohne
	// Cancun in chainConfig brachen sie mit "invalid opcode: MCOPY" ab.
	_, reg := v8Schluessel(t, testKeyRegistrar)
	p := neuerV8Pruefstand(t, []common.Address{reg}, testSalt)
	for _, s := range []string{"06fdde03", "95d89b41", "84b0196e"} {
		if _, err := p.aufruf(reg, sel(s)); err != nil {
			t.Errorf("V8 %s: %v", s, err)
		}
	}
	code, _ := hex.DecodeString(V7ContractBytecode)
	arg := common.LeftPadBytes(p.vertrag.Bytes(), 32) // irgendeine Adresse mit Code
	_, v7, _, err := p.evm.Create(vm.AccountRef(reg), append(code, arg...), 30_000_000, big.NewInt(0))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"06fdde03", "95d89b41"} {
		ret, _, err := p.evm.Call(vm.AccountRef(reg), v7, sel(s), 1_000_000, big.NewInt(0))
		if err != nil || len(ret) != 96 {
			t.Errorf("V7 %s: %v (%d Byte)", s, err, len(ret))
		}
	}
}

// ─── Nachspielen ────────────────────────────────────────────────────────────

func v8TestTx(t *testing.T, key *ecdsa.PrivateKey, wallet common.Address, frist, at int64, salt common.Hash, nonce int64) Transaction {
	t.Helper()
	c, n := big.NewInt(1001), big.NewInt(2001)
	d := v8RegisterDigest(common.HexToAddress(V7_CONTRACT_ADDR), salt, wallet, c, n, big.NewInt(nonce), big.NewInt(frist))
	return Transaction{
		Type: "register_human", Wallet: strings.ToLower(wallet.Hex()),
		PubSignals:  []string{c.String(), n.String()},
		RegSignatur: "0x" + hex.EncodeToString(v8Unterschrift(t, key, d)),
		RegFrist:    frist, RegAt: at,
	}
}

func TestV8Nachspielen_JederKnotenPrueftDieZustimmung(t *testing.T) {
	aliceKey, alice := v8Schluessel(t, testKeyAlice)
	malloryKey, _ := v8Schluessel(t, testKeyMallory)
	salt := v8NetzSalt()
	const at, block = int64(1_790_000_000), int64(1_790_000_030)
	if err := pruefeRegistrierungV8(v8TestTx(t, aliceKey, alice, at+600, at, salt, 0), block); err != nil {
		t.Fatalf("gueltige Registrierung abgelehnt: %v", err)
	}
	// Spaet in einen Block gekommen (Knoten war weg): darf die Kette NICHT anhalten.
	if err := pruefeRegistrierungV8(v8TestTx(t, aliceKey, alice, at+600, at, salt, 0), at+7*24*3600); err != nil {
		t.Fatalf("spaete Aufnahme in einen Block abgelehnt: %v", err)
	}
	faelle := map[string]Transaction{
		"ohne Unterschrift": func() Transaction {
			x := v8TestTx(t, aliceKey, alice, at+600, at, salt, 0)
			x.RegSignatur = ""
			return x
		}(),
		"fremde Unterschrift":     v8TestTx(t, malloryKey, alice, at+600, at, salt, 0),
		"Signatur aus altem Netz": v8TestTx(t, aliceKey, alice, at+600, at, crypto.Keccak256Hash([]byte("aequitas-1926-1")), 0),
		"Nonce 1":                 v8TestTx(t, aliceKey, alice, at+600, at, salt, 1),
		"bei Annahme abgelaufen":  v8TestTx(t, aliceKey, alice, at-1, at, salt, 0),
		"Frist ueber einen Tag":   v8TestTx(t, aliceKey, alice, at+v8MaxSignaturLaufzeit+1, at, salt, 0),
		"Annahme nach dem Block":  v8TestTx(t, aliceKey, alice, block+v8NachspielKarenz+3600, block+v8NachspielKarenz+1, salt, 0),
		"Commitment vertauscht": func() Transaction {
			x := v8TestTx(t, aliceKey, alice, at+600, at, salt, 0)
			x.PubSignals[0] = "1002"
			return x
		}(),
		"Nullifier vertauscht": func() Transaction {
			x := v8TestTx(t, aliceKey, alice, at+600, at, salt, 0)
			x.PubSignals[1] = "2002"
			return x
		}(),
		"Nullifier ausserhalb Feld": func() Transaction {
			x := v8TestTx(t, aliceKey, alice, at+600, at, salt, 0)
			x.PubSignals[1] = new(big.Int).Add(v8SnarkField, big.NewInt(2001)).String()
			return x
		}(),
		"andere Wallet eingetragen": func() Transaction {
			x := v8TestTx(t, aliceKey, alice, at+600, at, salt, 0)
			x.Wallet = "0x3a11020000000000000000000000000000000666"
			return x
		}(),
	}
	for name, tx := range faelle {
		if err := pruefeRegistrierungV8(tx, block); err == nil {
			t.Errorf("%s: beim Nachspielen angenommen", name)
		}
	}
}

// ─── Wer darf persistieren ──────────────────────────────────────────────────

func TestV8_NurRegisterWithSigVomRelayer(t *testing.T) {
	zurueck := _setVertragForTest(&vertragKonfig{version: vertragVersionV8})
	defer zurueck()
	relayerKey, relayer := v8Schluessel(t, testKeyRegistrar)
	_ = relayerKey
	t.Setenv("RELAYER_ADDRESS", strings.ToLower(relayer.Hex()))
	v8Addr := common.HexToAddress(V7_CONTRACT_ADDR)
	rel := strings.ToLower(relayer.Hex())
	if err := checkPersistedCallAllowed(v8Addr, sel(v8RegisterSelector+"00"), rel); err != nil {
		t.Fatalf("Relayer mit registerWithSig abgelehnt: %v", err)
	}
	if err := checkPersistedCallAllowed(v8Addr, sel(v8RegisterSelector+"00"), "0x3a11020000000000000000000000000000000666"); err == nil {
		t.Error("registerWithSig von einem Fremden persistiert")
	}
	for _, s := range []string{"13b81eb0", "a9059cbb", "70a08231", "18160ddd", "e54655d2"} {
		if err := checkPersistedCallAllowed(v8Addr, sel(s+"00"), rel); err == nil {
			t.Errorf("V8: %s darf nicht persistieren", s)
		}
	}
	if !vertragV8() || spiegelSlotBalanceOf() != v8SlotBalanceOf || spiegelSlotIsHuman() != v8SlotIsHuman {
		t.Error("Spiegel schreibt unter V8 nicht auf die V8-Plaetze")
	}
}

func TestV7_BleibtUnveraendert(t *testing.T) {
	zurueck := _setVertragForTest(&vertragKonfig{version: vertragVersionV7})
	defer zurueck()
	if vertragV8() || spiegelSlotBalanceOf() != 4 || spiegelSlotIsHuman() != 6 {
		t.Fatal("V7-Spiegel veraendert")
	}
	v7 := common.HexToAddress(V7_CONTRACT_ADDR)
	if err := checkPersistedCallAllowed(v7, sel("70a08231"+"00"), "0xabc"); err != nil {
		t.Errorf("V7-Positivliste veraendert: %v", err)
	}
	if err := checkPersistedCallAllowed(v7, sel(v8RegisterSelector+"00"), "0xabc"); err == nil {
		t.Error("V7 darf den V8-Selektor nicht persistieren")
	}
}

// Gemeinsamer Pruefvektor mit der App (Aequitas-App, src/domain/registerV8
// test): ethers' TypedDataEncoder/signTypedData ergibt fuer genau diese
// Eingaben genau diesen Digest und diese Signatur. Go ist oben gegen den
// Vertrag geprueft; stimmt Go hier mit ethers ueberein, unterschreibt die App
// das, was Kette und Vertrag erwarten.
func TestV8_GoldenVektorMitDerApp(t *testing.T) {
	key, human := v8Schluessel(t, testKeyAlice)
	salt := crypto.Keccak256Hash([]byte("aequitas-1926-1790000000"))
	if salt.Hex() != "0x89654bb870598e4d718a433a7d674b5ddd48f78cb6eecd489c01d69fa8e8954f" {
		t.Fatalf("Salt: %s", salt.Hex())
	}
	digest := v8RegisterDigest(common.HexToAddress(V7_CONTRACT_ADDR), salt, human,
		big.NewInt(1001), big.NewInt(2001), big.NewInt(0), big.NewInt(1_790_000_600))
	if digest.Hex() != "0x0cfd835d1262f048ab9e759f064e7d6ebfb747599e40a606e5cd84dc13cc1804" {
		t.Fatalf("Digest weicht von ethers ab: %s", digest.Hex())
	}
	sig := v8Unterschrift(t, key, digest)
	want := "9d5387fee26b5ec00237ff732f68821b31744e07c81b8b815f048c1c9ad8965d4032040a16fbf1ef18bd9c24bbdab9c76df4c03fa8637ce003f18a4dc4d05cd31c"
	if hex.EncodeToString(sig) != want {
		t.Fatalf("Signatur weicht von ethers ab: %x", sig)
	}
	if err := v8SignaturPruefen(digest, sig, human); err != nil {
		t.Fatal(err)
	}
}
