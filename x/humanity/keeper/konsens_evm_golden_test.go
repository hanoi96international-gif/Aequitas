package keeper

// KONSENS-VERGLEICHSTEST FUER DIE EVM VON GO-ETHEREUM.
//
// Haelt fest, was die EVM des Knotens (chainConfig, blockContext, die
// StateDB-Muster aus evm_engine.go) bei festen Eingaben liefert, und
// vergleicht es mit testdata/konsens_evm_golden.txt. Die Golden-Datei ist
// mit go-ethereum v1.13.0 erzeugt; ein Update von go-ethereum muss dieselben
// Werte liefern. Jede Abweichung waere eine Konsensaenderung: Knoten mit
// altem und neuem Code kaemen bei denselben Bloecken zu verschiedenen
// Ergebnissen.
//
// Festgehalten wird:
//   - jeder der 256 Opcodes einzeln: Erfolg/Abbruch, Gas, Rueckgabe, Logs,
//     Zustandswurzel -- so faellt jeder Fork auf, der unbemerkt aktiv wird
//     (BLOBBASEFEE, CLZ, ...) oder wegfaellt (PUSH0, MCOPY, TSTORE, ...);
//   - die Vorkompilate 0x01-0x0a mit Vektoren, und dass an den Adressen
//     der spaeteren Forks (BLS, P256VERIFY) nichts antwortet;
//   - Deploy und Aufrufe der Vertraege der Kette (AequitasV7, AequitasV8,
//     BioVerifier, Mock-Verifier, Scratch) mit Gas, Rueckgabe, Logs,
//     Speicherwurzeln, Zustandswurzel vor und nach dem Festschreiben;
//   - signierte Ueberweisungen aller Transaktionsarten: Rohform, Hash,
//     Absender, Annahme/Ablehnung, Nachspielen und die StateRoot des Knotens.
//
// Zeilen mit "#" vorne sind nur Auskunft (Fehlertexte): weichen sie ab, wird
// es gemeldet, aber nicht rot -- entscheidend fuer den Konsens ist, ob ein
// Aufruf gelingt, abbricht oder zuruecksetzt, und wie viel Gas er braucht.
//
// Neu erzeugen (nur wenn eine Aenderung gewollt ist, mit Begruendung im PR):
//   AEQUITAS_GOLDEN_SCHREIBEN=1 go test ./x/humanity/keeper -run TestKonsensEVM

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
)

// ─── Golden-Buch ──────────────────────────────────────────────────────────────

type goldenBuch struct{ zeilen []string }

func (b *goldenBuch) add(format string, args ...interface{}) {
	z := fmt.Sprintf(format, args...)
	if strings.ContainsAny(z, "\n\r") {
		panic("Golden-Zeile mit Zeilenumbruch: " + z)
	}
	b.zeilen = append(b.zeilen, z)
}

func pruefeGolden(t *testing.T, datei string, b *goldenBuch) {
	t.Helper()
	pfad := filepath.Join("testdata", datei)
	if os.Getenv("AEQUITAS_GOLDEN_SCHREIBEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pfad, []byte(strings.Join(b.zeilen, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s geschrieben (%d Zeilen)", pfad, len(b.zeilen))
		return
	}
	roh, err := os.ReadFile(pfad)
	if err != nil {
		t.Fatalf("Golden-Datei fehlt: %v", err)
	}
	soll := strings.Split(strings.TrimRight(string(roh), "\n"), "\n")
	vergleicheGolden(t, datei, soll, b.zeilen, konsensBekannteAbweichungen[datei])
}

// goldenSchluessel: der Teil einer Zeile vor dem ersten "=" (z. B.
// "op/44 klasse"), sonst die ganze Zeile.
func goldenSchluessel(z string) string {
	if i := strings.Index(z, "="); i >= 0 {
		return z[:i]
	}
	return z
}

func goldenKarte(t *testing.T, zeilen []string) (map[string]string, []string) {
	t.Helper()
	m := make(map[string]string, len(zeilen))
	var reihe []string
	for _, z := range zeilen {
		k := goldenSchluessel(z)
		if _, doppelt := m[k]; doppelt {
			t.Fatalf("Golden-Schluessel doppelt: %q", k)
		}
		m[k] = z
		reihe = append(reihe, k)
	}
	return m, reihe
}

// vergleicheGolden vergleicht Zeile fuer Zeile ueber den Schluessel. Eine
// Abweichung ist nur erlaubt, wenn sie in bekannt steht -- mit genau der
// Zeile, die jetzt herauskommen muss ("" = Zeile entfaellt). Eine bekannte
// Abweichung, die nicht (mehr) auftritt, ist ebenfalls ein Fehler, damit die
// Liste nicht veraltet.
func vergleicheGolden(t *testing.T, datei string, soll, ist []string, bekannt map[string]bekannteAbweichung) {
	t.Helper()
	sollM, sollR := goldenKarte(t, soll)
	istM, istR := goldenKarte(t, ist)
	alle := append([]string{}, sollR...)
	for _, k := range istR {
		if _, ok := sollM[k]; !ok {
			alle = append(alle, k)
		}
	}
	harte := 0
	genutzt := map[string]bool{}
	for _, k := range alle {
		s, w := sollM[k], istM[k]
		if s == w {
			continue
		}
		if ab, ok := bekannt[k]; ok {
			genutzt[k] = true
			if ab.jetzt == w {
				t.Logf("bekannte Abweichung %q (%s)", k, ab.grund)
				continue
			}
			harte++
			t.Errorf("%s: bekannte Abweichung %q liefert etwas anderes:\n  erwartet jetzt: %s\n  erhalten:       %s", datei, k, ab.jetzt, w)
			continue
		}
		if strings.HasPrefix(k, "#") {
			t.Logf("Auskunft weicht ab (kein Konsens-Unterschied):\n  vorher: %s\n  jetzt:  %s", s, w)
			continue
		}
		harte++
		if harte <= 80 {
			t.Errorf("%s: %q weicht ab:\n  v1.13.0: %s\n  jetzt:   %s", datei, k, s, w)
		}
	}
	for k := range bekannt {
		if !genutzt[k] {
			t.Errorf("%s: bekannte Abweichung %q tritt nicht mehr auf -- aus der Liste nehmen", datei, k)
		}
	}
	if harte > 80 {
		t.Errorf("... insgesamt %d abweichende Zeilen", harte)
	}
}

// ─── EVM-Lauf ─────────────────────────────────────────────────────────────────

func klasseVon(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, vm.ErrExecutionReverted):
		return "revert"
	case strings.HasPrefix(err.Error(), "panik:"):
		return "panik"
	default:
		return "abbruch"
	}
}

func logsHash(logs []*types.Log) string {
	if len(logs) == 0 {
		return "-"
	}
	var buf bytes.Buffer
	for _, l := range logs {
		buf.Write(l.Address.Bytes())
		fmt.Fprintf(&buf, "|%d|", len(l.Topics))
		for _, tp := range l.Topics {
			buf.Write(tp.Bytes())
		}
		fmt.Fprintf(&buf, "|%d|", len(l.Data))
		buf.Write(l.Data)
	}
	return fmt.Sprintf("%d:%x", len(logs), crypto.Keccak256(buf.Bytes())[:12])
}

type evmLauf struct {
	b          *goldenBuch
	sdb        *state.StateDB
	logsVorher int
}

func (l *evmLauf) neueLogs() []*types.Log {
	alle := l.sdb.Logs()
	// Logs() liefert keine feste Reihenfolge ueber Transaktionen hinweg; hier
	// gibt es nur eine (kein SetTxContext), also nur einen Eintrag.
	if l.logsVorher > len(alle) {
		l.logsVorher = len(alle)
	}
	neu := alle[l.logsVorher:]
	l.logsVorher = len(alle)
	return neu
}

func sicher(f func() ([]byte, uint64, error)) (ret []byte, rest uint64, err error) {
	defer func() {
		if r := recover(); r != nil {
			ret, rest, err = nil, 0, fmt.Errorf("panik: %v", r)
		}
	}()
	return f()
}

func (l *evmLauf) notiere(name string, ret []byte, gas, rest uint64, err error) {
	l.b.add("%s klasse=%s gas=%d ret=%x logs=%s", name, klasseVon(err), gas-rest, ret, logsHash(l.neueLogs()))
	if err != nil {
		grund := ""
		if errors.Is(err, vm.ErrExecutionReverted) {
			grund = decodeRevertReason(ret)
		}
		l.b.add("#%s fehler=%q grund=%q", name, err.Error(), grund)
	}
}

func (l *evmLauf) aufruf(name string, ts uint64, von, an common.Address, data []byte, gas uint64) ([]byte, error) {
	evm := neueKnotenEVM(ts, von, l.sdb)
	ret, rest, err := sicher(func() ([]byte, uint64, error) {
		return evmAufrufen(evm, von, an, data, gas, big.NewInt(0))
	})
	l.notiere(name, ret, gas, rest, err)
	return ret, err
}

func (l *evmLauf) erzeuge(t *testing.T, name string, ts uint64, von common.Address, code []byte) common.Address {
	t.Helper()
	const gas = 30_000_000
	evm := neueKnotenEVM(ts, von, l.sdb)
	var addr common.Address
	ret, rest, err := sicher(func() ([]byte, uint64, error) {
		r, a, rest, err := evmErzeugen(evm, von, code, gas, big.NewInt(0))
		addr = a
		return r, rest, err
	})
	if err != nil {
		t.Fatalf("%s: Deploy gescheitert: %v", name, err)
	}
	l.b.add("%s adresse=%s gas=%d codehash=%x logs=%s", name, strings.ToLower(addr.Hex()), gas-rest,
		crypto.Keccak256(ret), logsHash(l.neueLogs()))
	return addr
}

func u256(v uint64) *uint256.Int { return uint256.NewInt(v) }

func selektor(sig string) []byte { return crypto.Keccak256([]byte(sig))[:4] }

func kwort(v interface{}) []byte {
	switch x := v.(type) {
	case common.Address:
		return common.LeftPadBytes(x.Bytes(), 32)
	case *big.Int:
		return common.LeftPadBytes(x.Bytes(), 32)
	case int64:
		return common.LeftPadBytes(big.NewInt(x).Bytes(), 32)
	case common.Hash:
		return x.Bytes()
	}
	panic(fmt.Sprintf("wort: %T", v))
}

func aufrufDaten(sig string, args ...interface{}) []byte {
	d := selektor(sig)
	for _, a := range args {
		d = append(d, kwort(a)...)
	}
	return d
}

// ─── 1. Opcodes ───────────────────────────────────────────────────────────────

func konsensOpcodes(t *testing.T, b *goldenBuch) {
	ts := uint64(1_790_000_000)
	origin := common.HexToAddress("0x0000000000000000000000000000000000000a11")
	ziel := common.HexToAddress("0x000000000000000000000000000000000000c0de")
	for op := 0; op < 256; op++ {
		sdb, _, err := neueSpeicherStateDB()
		if err != nil {
			t.Fatal(err)
		}
		// 16 Mal PUSH1 0x01, dann der Opcode, dann Nullen (Direktwerte von
		// PUSHn, danach STOP).
		code := bytes.Repeat([]byte{0x60, 0x01}, 16)
		code = append(code, byte(op))
		code = append(code, make([]byte, 33)...)
		setzeCode(sdb, ziel, code)
		l := &evmLauf{b: b, sdb: sdb}
		const gas = 1_000_000
		evm := neueKnotenEVM(ts, origin, sdb)
		ret, rest, err := sicher(func() ([]byte, uint64, error) {
			return evmAufrufen(evm, origin, ziel, nil, gas, big.NewInt(0))
		})
		name := fmt.Sprintf("op/%02x", op)
		l.notiere(name, ret, gas, rest, err)
		if klasseVon(err) == "panik" {
			// Nach einer Panik ist die StateDB nicht mehr aussagekraeftig.
			continue
		}
		b.add("%s wurzel=%x speicher=%x code=%d", name, evmZwischenwurzel(sdb), sdb.GetStorageRoot(ziel), len(sdb.GetCode(ziel)))
	}
}

// ─── 2. Vorkompilate ──────────────────────────────────────────────────────────

func konsensVorkompilate(t *testing.T, b *goldenBuch) {
	ts := uint64(1_790_000_000)
	von := common.HexToAddress("0x0000000000000000000000000000000000000a11")
	sdb, _, err := neueSpeicherStateDB()
	if err != nil {
		t.Fatal(err)
	}
	l := &evmLauf{b: b, sdb: sdb}

	// Eigene Faelle fuer 0x01-0x04.
	key, _ := crypto.HexToECDSA(testKeyAlice)
	h := crypto.Keccak256([]byte("aequitas-konsens"))
	sig, _ := crypto.Sign(h, key)
	ecIn := append(append(append([]byte{}, h...), kwort(int64(sig[64])+27)...), sig[:64]...)
	eigene := []vorkompilatVektor{
		{0x01, "eigen/ecrecover-gueltig", hex.EncodeToString(ecIn)},
		{0x01, "eigen/ecrecover-v29", hex.EncodeToString(append(append(append([]byte{}, h...), kwort(int64(29))...), sig[:64]...))},
		{0x02, "eigen/sha256-abc", hex.EncodeToString([]byte("abc"))},
		{0x03, "eigen/ripemd-abc", hex.EncodeToString([]byte("abc"))},
		{0x04, "eigen/identitaet", hex.EncodeToString([]byte("aequitas"))},
		{0x08, "eigen/pairing-leer", ""},
		{0x0a, "eigen/punktauswertung-nullen", strings.Repeat("00", 192)},
	}
	for _, v := range append(eigene, konsensVorkompilatVektoren...) {
		in, err := hex.DecodeString(v.eingabe)
		if err != nil {
			t.Fatalf("%s: %v", v.name, err)
		}
		l.aufruf(fmt.Sprintf("pc/%02x/%s", v.adresse, v.name), ts, von, common.BytesToAddress([]byte{v.adresse}), in, 10_000_000)
	}
	// Adressen spaeterer Forks: BLS (Prague, 0x0b-0x11), P256VERIFY (Osaka,
	// 0x100) und dahinter. Auf dieser Kette muss dort Leere stehen.
	bls, _ := hex.DecodeString(konsensBLSEingabe)
	for _, a := range []uint64{0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x100} {
		l.aufruf(fmt.Sprintf("pc/%03x/spaeterer-fork", a), ts, von, common.BigToAddress(new(big.Int).SetUint64(a)), bls, 10_000_000)
	}
	b.add("pc/wurzel=%x", evmZwischenwurzel(sdb))
}

// ─── 3. Vertraege der Kette ───────────────────────────────────────────────────

func v7RegistrierDaten(t *testing.T, key *ecdsa.PrivateKey, human, vertrag common.Address, commitment, nullifier *big.Int, marker int64) []byte {
	t.Helper()
	msg := crypto.Keccak256(kwort(int64(1926)), vertrag.Bytes(), []byte("register"), kwort(commitment), kwort(nullifier))
	eth := crypto.Keccak256([]byte("\x19Ethereum Signed Message:\n32"), msg)
	sig, err := crypto.Sign(eth, key)
	if err != nil {
		t.Fatal(err)
	}
	sig[64] += 27
	parsed, err := abi.JSON(strings.NewReader(registerWithSigABI))
	if err != nil {
		t.Fatal(err)
	}
	pA := [2]*big.Int{big.NewInt(marker), big.NewInt(1)}
	pB := [2][2]*big.Int{{big.NewInt(2), big.NewInt(3)}, {big.NewInt(4), big.NewInt(5)}}
	pC := [2]*big.Int{big.NewInt(6), big.NewInt(7)}
	var n32 [32]byte
	copy(n32[:], kwort(nullifier))
	data, err := parsed.Pack("registerWithSig", pA, pB, pC, [2]*big.Int{commitment, nullifier}, human, sig, n32)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func bn254Punkte() (g1 [2]*big.Int, g2 [2][2]*big.Int) {
	z := func(s string) *big.Int { v, _ := new(big.Int).SetString(s, 10); return v }
	g1 = [2]*big.Int{big.NewInt(1), big.NewInt(2)}
	// EIP-197: Erzeuger von G2, Reihenfolge (imag, real).
	g2 = [2][2]*big.Int{
		{z("11559732032986387107991004021392285783925812861821192530917403151452391805634"), z("10857046999023057135944570762232829481370756359578518086990519993285655852781")},
		{z("4082367875863433681332203403145435568316851327593401208105741076214120093531"), z("8495653923123431417604973247489272438418190587263600148770280649306958101930")},
	}
	return
}

func konsensVertraege(t *testing.T, b *goldenBuch) {
	ts := uint64(1_790_000_000)
	const tag = uint64(86400)
	deployer := common.HexToAddress("0x00000000000000000000000000000000000d0e01")
	aliceKey, alice := v8Schluessel(t, testKeyAlice)
	malloryKey, mallory := v8Schluessel(t, testKeyMallory)
	_, reg := v8Schluessel(t, testKeyRegistrar)
	bobKey, bob := v8Schluessel(t, "4444444444444444444444444444444444444444444444444444444444444444")

	sdb, db, err := neueSpeicherStateDB()
	if err != nil {
		t.Fatal(err)
	}
	// Wie newStateDB: Guthaben aus dem Go-Zustand, ohne Festschreiben.
	for i, a := range []common.Address{alice, bob, mallory, reg} {
		setzeGuthaben(sdb, a, evmBalanceWei(a.Hex(), float64(100*(i+1))+0.5))
	}
	l := &evmLauf{b: b, sdb: sdb}

	mock, _ := hex.DecodeString(mockGroth16VerifierBytecode)
	verifier := l.erzeuge(t, "deploy/mock-verifier", ts, deployer, mock)

	v8Args, err := v8KonstruktorArgumente(verifier, []common.Address{reg}, testSalt)
	if err != nil {
		t.Fatal(err)
	}
	v8Code, _ := hex.DecodeString(V8ContractBytecode)
	v8 := l.erzeuge(t, "deploy/v8", ts, deployer, append(v8Code, v8Args...))

	v7Code, _ := hex.DecodeString(V7ContractBytecode)
	v7 := l.erzeuge(t, "deploy/v7-mock", ts, deployer, append(append([]byte{}, v7Code...), kwort(verifier)...))
	bio := common.HexToAddress(BIO_VERIFIER_ADDR)
	bioCode, _ := hex.DecodeString(BioVerifierRuntimeBytecode)
	setzeCode(sdb, bio, bioCode)
	v7echt := l.erzeuge(t, "deploy/v7-bioverifier", ts, deployer, append(append([]byte{}, v7Code...), kwort(bio)...))
	scratchCode, _ := hex.DecodeString(scratchGetterInitcode)
	scratch := l.erzeuge(t, "deploy/scratch", ts, deployer, scratchCode)

	// ── V8
	for _, s := range []string{"name()", "symbol()", "eip712Domain()", "verifier()", "NETZ_SALT()", "totalSupply()"} {
		l.aufruf("v8/"+s, ts, reg, v8, selektor(s), 5_000_000)
	}
	l.aufruf("v8/isRegistrar(reg)", ts, reg, v8, aufrufDaten("isRegistrar(address)", reg), 5_000_000)
	c, n := big.NewInt(1001), big.NewInt(2001)
	frist := int64(ts) + 600
	gut := v8Anfrage(t, alice, c, n, frist, v8Unterschrift(t, aliceKey, v8RegisterDigest(v8, testSalt, alice, c, n, big.NewInt(0), big.NewInt(frist))), v8TestMarker)
	l.aufruf("v8/register-kein-registrar", ts, mallory, v8, gut, 5_000_000)
	l.aufruf("v8/register-falscher-beweis", ts, reg, v8, v8Anfrage(t, alice, c, n, frist, v8Unterschrift(t, aliceKey, v8RegisterDigest(v8, testSalt, alice, c, n, big.NewInt(0), big.NewInt(frist))), 0xBAD), 5_000_000)
	l.aufruf("v8/register-fremde-unterschrift", ts, reg, v8, v8Anfrage(t, alice, c, n, frist, v8Unterschrift(t, malloryKey, v8RegisterDigest(v8, testSalt, alice, c, n, big.NewInt(0), big.NewInt(frist))), v8TestMarker), 5_000_000)
	weit := int64(ts) + 3*int64(tag)
	l.aufruf("v8/register-frist-zu-weit", ts, reg, v8, v8Anfrage(t, alice, c, n, weit, v8Unterschrift(t, aliceKey, v8RegisterDigest(v8, testSalt, alice, c, n, big.NewInt(0), big.NewInt(weit))), v8TestMarker), 5_000_000)
	l.aufruf("v8/register-gut", ts, reg, v8, gut, 5_000_000)
	l.aufruf("v8/register-wiederholt", ts, reg, v8, gut, 5_000_000)
	l.aufruf("v8/register-nach-frist", ts+700, reg, v8, gut, 5_000_000)
	for _, s := range []string{"isHuman(address)", "balanceOf(address)", "nonces(address)"} {
		l.aufruf("v8/"+s, ts, reg, v8, aufrufDaten(s, alice), 5_000_000)
	}
	l.aufruf("v8/transfer", ts, alice, v8, aufrufDaten("transfer(address,uint256)", bob, big.NewInt(1)), 5_000_000)

	// ── V7 (Mock-Verifier)
	for _, s := range []string{"name()", "symbol()", "decimals()", "totalSupply()", "getStats()", "fairShare()", "wealthCap()", "currentPhase()"} {
		l.aufruf("v7/"+s, ts, reg, v7, selektor(s), 5_000_000)
	}
	regAlice := v7RegistrierDaten(t, aliceKey, alice, v7, big.NewInt(11), big.NewInt(21), v8TestMarker)
	l.aufruf("v7/register-alice", ts, reg, v7, regAlice, 5_000_000)
	l.aufruf("v7/register-alice-wiederholt", ts, reg, v7, regAlice, 5_000_000)
	l.aufruf("v7/register-falsche-unterschrift", ts, reg, v7, v7RegistrierDaten(t, malloryKey, bob, v7, big.NewInt(12), big.NewInt(22), v8TestMarker), 5_000_000)
	l.aufruf("v7/register-falscher-beweis", ts, reg, v7, v7RegistrierDaten(t, bobKey, bob, v7, big.NewInt(12), big.NewInt(22), 0xBAD), 5_000_000)
	l.aufruf("v7/register-bob", ts+60, reg, v7, v7RegistrierDaten(t, bobKey, bob, v7, big.NewInt(12), big.NewInt(22), v8TestMarker), 5_000_000)
	zehn := new(big.Int).Mul(big.NewInt(10), weiPerAEQ)
	l.aufruf("v7/calcFee", ts, alice, v7, aufrufDaten("calcFee(address,uint256)", alice, zehn), 5_000_000)
	l.aufruf("v7/transfer-alice-bob", ts+120, alice, v7, aufrufDaten("transfer(address,uint256)", bob, zehn), 5_000_000)
	l.aufruf("v7/transfer-mallory", ts+120, mallory, v7, aufrufDaten("transfer(address,uint256)", bob, zehn), 5_000_000)
	l.aufruf("v7/transfer-zu-viel", ts+120, alice, v7, aufrufDaten("transfer(address,uint256)", bob, new(big.Int).Mul(zehn, big.NewInt(1000))), 5_000_000)
	for _, a := range []common.Address{alice, bob} {
		l.aufruf("v7/balanceOf/"+strings.ToLower(a.Hex()[:8]), ts, reg, v7, aufrufDaten("balanceOf(address)", a), 5_000_000)
		l.aufruf("v7/claimableUBI/"+strings.ToLower(a.Hex()[:8]), ts, reg, v7, aufrufDaten("claimableUBI(address)", a), 5_000_000)
	}
	l.aufruf("v7/accumulateUBI", ts+tag, reg, v7, selektor("accumulateUBI()"), 5_000_000)
	l.aufruf("v7/claimUBI-bob", ts+tag, bob, v7, selektor("claimUBI()"), 5_000_000)
	l.aufruf("v7/proposeGuardian", ts+tag, alice, v7, aufrufDaten("proposeGuardian(address)", bob), 5_000_000)
	l.aufruf("v7/confirmGuardian-zu-frueh", ts+2*tag, alice, v7, selektor("confirmGuardian()"), 5_000_000)
	l.aufruf("v7/confirmGuardian", ts+9*tag, alice, v7, selektor("confirmGuardian()"), 5_000_000)
	l.aufruf("v7/guardianConfirmAlive", ts+10*tag, bob, v7, aufrufDaten("guardianConfirmAlive(address)", alice), 5_000_000)
	l.aufruf("v7/applyDemurrage-bob", ts+400*tag, reg, v7, aufrufDaten("applyDemurrage(address)", bob), 5_000_000)
	l.aufruf("v7/applyWealthCap-bob", ts+400*tag, reg, v7, aufrufDaten("applyWealthCap(address)", bob), 5_000_000)
	l.aufruf("v7/triggerEscrow-bob", ts+1000*tag, reg, v7, aufrufDaten("triggerEscrow(address)", bob), 5_000_000)
	l.aufruf("v7/confirmAlive-bob", ts+1001*tag, bob, v7, selektor("confirmAlive()"), 5_000_000)
	l.aufruf("v7/revokeGuardian", ts+1002*tag, alice, v7, selektor("revokeGuardian()"), 5_000_000)
	l.aufruf("v7/getStats-ende", ts+1002*tag, reg, v7, selektor("getStats()"), 5_000_000)

	// ── V7 mit dem echten BioVerifier: die Beweispruefung laeuft ueber die
	// bn254-Vorkompilate und lehnt den Platzhalter-Beweis ab.
	l.aufruf("v7echt/register-platzhalter", ts, reg, v7echt, v7RegistrierDaten(t, aliceKey, alice, v7echt, big.NewInt(11), big.NewInt(21), v8TestMarker), 5_000_000)
	bioABI, err := abi.JSON(strings.NewReader(bioVerifierABI))
	if err != nil {
		t.Fatal(err)
	}
	g1, g2 := bn254Punkte()
	for name, pub := range map[string][2]*big.Int{"klein": {big.NewInt(1001), big.NewInt(2001)}, "zu-gross": {new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(1)}} {
		d, err := bioABI.Pack("verifyProof", g1, g2, g1, pub)
		if err != nil {
			t.Fatal(err)
		}
		l.aufruf("bio/verifyProof-"+name, ts, reg, bio, d, 5_000_000)
	}
	d, _ := bioABI.Pack("verifyProof", [2]*big.Int{big.NewInt(1), big.NewInt(3)}, g2, g1, [2]*big.Int{big.NewInt(1), big.NewInt(2)})
	l.aufruf("bio/verifyProof-kein-kurvenpunkt", ts, reg, bio, d, 5_000_000)

	// ── Scratch (Muster aus evm_repro_test.go)
	l.aufruf("scratch/set", ts, alice, scratch, aufrufDaten("set(address,uint256)", bob, big.NewInt(42)), 5_000_000)
	l.aufruf("scratch/foo-bob", ts, alice, scratch, aufrufDaten("foo(address)", bob), 5_000_000)
	l.aufruf("scratch/foo-alice", ts, alice, scratch, aufrufDaten("foo(address)", alice), 5_000_000)

	// ── Wurzeln, Festschreiben, Wiederoeffnen (Muster von CallContract persist)
	vertraege := map[string]common.Address{"mock": verifier, "v8": v8, "v7": v7, "v7echt": v7echt, "bio": bio, "scratch": scratch}
	namen := make([]string, 0, len(vertraege))
	for k := range vertraege {
		namen = append(namen, k)
	}
	sort.Strings(namen)
	b.add("vertraege/zwischenwurzel=%x", evmZwischenwurzel(sdb))
	for _, k := range namen {
		a := vertraege[k]
		b.add("vertraege/%s speicherwurzel=%x nonce=%d codehash=%x", k, sdb.GetStorageRoot(a), sdb.GetNonce(a), sdb.GetCodeHash(a))
	}
	for _, a := range []common.Address{alice, bob, mallory, reg, deployer} {
		b.add("konto/%s guthaben=%s nonce=%d", strings.ToLower(a.Hex()), sdb.GetBalance(a).String(), sdb.GetNonce(a))
	}
	slots := func(s *state.StateDB) string {
		var buf bytes.Buffer
		for i := int64(0); i < 40; i++ {
			buf.Write(s.GetState(v7, common.BigToHash(big.NewInt(i))).Bytes())
		}
		for _, a := range []common.Address{alice, bob} {
			for _, base := range []int64{4, 5, 6, 9, 10, 11, 12, 13, 14, 15, 16, 27, 28} {
				buf.Write(s.GetState(v7, mappingSlot(a.Bytes(), base)).Bytes())
			}
			buf.Write(s.GetState(v8, mappingSlot(a.Bytes(), 0)).Bytes())
		}
		return fmt.Sprintf("%x", crypto.Keccak256(buf.Bytes()))
	}
	vorher := slots(sdb)
	root, err := evmFestschreiben(sdb)
	if err != nil {
		t.Fatal(err)
	}
	b.add("vertraege/festgeschrieben=%x slots=%s", root, vorher)
	frisch, err := oeffneStateDB(root, db)
	if err != nil {
		t.Fatal(err)
	}
	b.add("vertraege/wiedergeoeffnet slots=%s code-v7=%d", slots(frisch), len(frisch.GetCode(v7)))

	// ── Das Muster von newStateDB: eine frische StateDB, Vertragscode per
	// SetCode (nicht per CREATE, also auch nicht in der Zugriffsliste),
	// Speicher per SetState aus der Datenbank, Guthaben aus dem Go-Zustand --
	// und darauf die Aufrufe, wie CallContract sie macht (je Aufruf eine neue
	// EVM, nie festgeschrieben, ausser beim persistierenden Aufruf).
	knotenSdb, knotenDB, err := neueSpeicherStateDB()
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range []common.Address{alice, bob, mallory, reg} {
		setzeGuthaben(knotenSdb, a, evmBalanceWei(a.Hex(), float64(100*(i+1))+0.5))
	}
	setzeCode(knotenSdb, v7, frisch.GetCode(v7))
	setzeCode(knotenSdb, verifier, frisch.GetCode(verifier))
	kopiere := func(slot common.Hash) {
		if v := frisch.GetState(v7, slot); v != (common.Hash{}) {
			knotenSdb.SetState(v7, slot, v)
		}
	}
	for i := int64(0); i < 40; i++ {
		kopiere(common.BigToHash(big.NewInt(i)))
	}
	for _, a := range []common.Address{alice, bob} {
		for _, base := range []int64{4, 5, 6, 9, 10, 11, 12, 13, 14, 15, 16, 27, 28} {
			kopiere(mappingSlot(a.Bytes(), base))
		}
	}
	k := &evmLauf{b: b, sdb: knotenSdb}
	t2 := ts + 1003*tag
	k.aufruf("knotenmuster/balanceOf-alice", t2, reg, v7, aufrufDaten("balanceOf(address)", alice), 30_000_000)
	k.aufruf("knotenmuster/getStats", t2, reg, v7, selektor("getStats()"), 30_000_000)
	k.aufruf("knotenmuster/claimableUBI-bob", t2, reg, v7, aufrufDaten("claimableUBI(address)", bob), 30_000_000)
	k.aufruf("knotenmuster/transfer-alice-bob", t2, alice, v7, aufrufDaten("transfer(address,uint256)", bob, zehn), 30_000_000)
	k.aufruf("knotenmuster/transfer-bob-mallory", t2, bob, v7, aufrufDaten("transfer(address,uint256)", mallory, zehn), 30_000_000)
	k.aufruf("knotenmuster/confirmAlive-alice", t2, alice, v7, selektor("confirmAlive()"), 30_000_000)
	k.aufruf("knotenmuster/proposeGuardian-bob", t2, bob, v7, aufrufDaten("proposeGuardian(address)", alice), 30_000_000)
	k.aufruf("knotenmuster/register-mallory", t2, reg, v7, v7RegistrierDaten(t, malloryKey, mallory, v7, big.NewInt(13), big.NewInt(23), v8TestMarker), 30_000_000)
	k.aufruf("knotenmuster/name", t2, reg, v7, selektor("name()"), 30_000_000)
	b.add("knotenmuster/zwischenwurzel=%x speicherwurzel=%x", evmZwischenwurzel(knotenSdb), knotenSdb.GetStorageRoot(v7))
	kRoot, err := evmFestschreiben(knotenSdb)
	if err != nil {
		t.Fatal(err)
	}
	kFrisch, err := oeffneStateDB(kRoot, knotenDB)
	if err != nil {
		t.Fatal(err)
	}
	b.add("knotenmuster/festgeschrieben=%x slots=%s", kRoot, slots(kFrisch))
}

// ─── 4. Signierte Ueberweisungen und StateRoot des Knotens ───────────────────

type ueberweisungsFall struct {
	name string
	tx   Transaction
}

func signiereArt(t *testing.T, key *ecdsa.PrivateKey, inner types.TxData, signer types.Signer) (*types.Transaction, string) {
	t.Helper()
	stx, err := types.SignNewTx(key, signer, inner)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := stx.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	return stx, "0x" + hex.EncodeToString(raw)
}

func alsUeberweisung(von common.Address, an common.Address, wei *big.Int, stx *types.Transaction, roh string) Transaction {
	return Transaction{
		Type: "transfer", Wallet: strings.ToLower(von.Hex()), To: strings.ToLower(an.Hex()),
		Amount: betragAusWei(wei), TxHash: stx.Hash().Hex(), Roh: roh,
	}
}

// setCodeRoh baut eine Transaktion vom Typ 4 (EIP-7702) von Hand, ohne die
// Typen von go-ethereum, die es in v1.13 nicht gibt.
func setCodeRoh(t *testing.T, key *ecdsa.PrivateKey, an common.Address, wei *big.Int) (string, string) {
	t.Helper()
	auth := []interface{}{big.NewInt(1926), an, uint64(0), uint8(0), big.NewInt(1), big.NewInt(1)}
	felder := []interface{}{big.NewInt(1926), uint64(0), big.NewInt(1), big.NewInt(2), uint64(60000), an, wei, []byte{}, []interface{}{}, []interface{}{auth}}
	ohne, err := rlp.EncodeToBytes(felder)
	if err != nil {
		t.Fatal(err)
	}
	h := crypto.Keccak256(append([]byte{0x04}, ohne...))
	sig, err := crypto.Sign(h, key)
	if err != nil {
		t.Fatal(err)
	}
	felder = append(felder, uint8(sig[64]), new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:64]))
	mit, err := rlp.EncodeToBytes(felder)
	if err != nil {
		t.Fatal(err)
	}
	raw := append([]byte{0x04}, mit...)
	return "0x" + hex.EncodeToString(raw), crypto.Keccak256Hash(raw).Hex()
}

func konsensUeberweisungen(t *testing.T, b *goldenBuch) {
	uhr(t, 1_790_000_000)
	aKey, a := v8Schluessel(t, "5555555555555555555555555555555555555555555555555555555555555555")
	bKey, bb := v8Schluessel(t, "6666666666666666666666666666666666666666666666666666666666666666")
	cKey, c := v8Schluessel(t, "7777777777777777777777777777777777777777777777777777777777777777")
	cid := big.NewInt(1926)
	latest := types.LatestSignerForChainID(cid)
	wei := func(micro int64) *big.Int { return new(big.Int).Mul(big.NewInt(micro), big.NewInt(1_000_000_000_000)) }

	t1, r1 := signiereArt(t, aKey, &types.DynamicFeeTx{ChainID: cid, Nonce: 0, To: &bb, Value: wei(10_000_000), Gas: 21000, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2)}, latest)
	t2, r2 := signiereArt(t, aKey, &types.LegacyTx{Nonce: 1, To: &c, Value: wei(2_500_000), Gas: 21000, GasPrice: big.NewInt(0)}, types.NewEIP155Signer(cid))
	t3, r3 := signiereArt(t, bKey, &types.AccessListTx{ChainID: cid, Nonce: 0, To: &c, Value: wei(1_000_000), Gas: 21000, GasPrice: big.NewInt(1), AccessList: types.AccessList{{Address: a, StorageKeys: []common.Hash{{1}}}}}, latest)
	v7addr := common.HexToAddress(V7_CONTRACT_ADDR)
	v7data := aufrufDaten("transfer(address,uint256)", a, wei(750_000))
	t4, r4 := signiereArt(t, cKey, &types.DynamicFeeTx{ChainID: cid, Nonce: 0, To: &v7addr, Value: big.NewInt(0), Data: v7data, Gas: 60000, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(1)}, latest)
	t5, r5 := signiereArt(t, aKey, &types.BlobTx{ChainID: u256(1926), Nonce: 2, To: bb, Value: u256(1_000_000_000_000_000_000), Gas: 21000, GasTipCap: u256(1), GasFeeCap: u256(2), BlobFeeCap: u256(1), BlobHashes: []common.Hash{{1}}}, latest)
	tUn, rUn := signiereArt(t, aKey, &types.LegacyTx{Nonce: 3, To: &bb, Value: wei(1_000_000), Gas: 21000, GasPrice: big.NewInt(0)}, types.HomesteadSigner{})
	tFremd, rFremd := signiereArt(t, aKey, &types.DynamicFeeTx{ChainID: big.NewInt(1), Nonce: 3, To: &bb, Value: wei(1_000_000), Gas: 21000, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2)}, types.LatestSignerForChainID(big.NewInt(1)))
	r7702, h7702 := setCodeRoh(t, aKey, bb, wei(1_000_000))

	// Hohes S: dieselbe Signatur mit s' = N - s (und gekipptem V) -- nach
	// EIP-2 ungueltig.
	hochS := func() string {
		v, r, s := t1.RawSignatureValues()
		n := crypto.S256().Params().N
		s2 := new(big.Int).Sub(n, s)
		inner := &types.DynamicFeeTx{ChainID: cid, Nonce: 0, To: &bb, Value: wei(10_000_000), Gas: 21000, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
			V: new(big.Int).Xor(v, big.NewInt(1)), R: r, S: s2}
		raw, _ := types.NewTx(inner).MarshalBinary()
		return "0x" + hex.EncodeToString(raw)
	}()

	faelle := []ueberweisungsFall{
		{"dynamisch-a-b", alsUeberweisung(a, bb, wei(10_000_000), t1, r1)},
		{"legacy-a-c", alsUeberweisung(a, c, wei(2_500_000), t2, r2)},
		{"zugriffsliste-b-c", alsUeberweisung(bb, c, wei(1_000_000), t3, r3)},
		{"v7-transfer-c-a", alsUeberweisung(c, a, wei(750_000), t4, r4)},
		{"blob-a-b", alsUeberweisung(a, bb, u256(1_000_000_000_000_000_000).ToBig(), t5, r5)},
		{"ungeschuetzt-a-b", alsUeberweisung(a, bb, wei(1_000_000), tUn, rUn)},
		{"fremde-chainid", alsUeberweisung(a, bb, wei(1_000_000), tFremd, rFremd)},
		{"typ4-setcode", Transaction{Type: "transfer", Wallet: strings.ToLower(a.Hex()), To: strings.ToLower(bb.Hex()), Amount: betragAusWei(wei(1_000_000)), TxHash: h7702, Roh: r7702}},
		{"hohes-s", func() Transaction { x := alsUeberweisung(a, bb, wei(10_000_000), t1, hochS); return x }()},
		{"fremder-absender", func() Transaction {
			x := alsUeberweisung(a, bb, wei(10_000_000), t1, r1)
			x.Wallet = strings.ToLower(c.Hex())
			return x
		}()},
		{"rohform-abgeschnitten", func() Transaction { x := alsUeberweisung(a, bb, wei(10_000_000), t1, r1[:len(r1)-2]); return x }()},
		{"rohform-angehaengt", func() Transaction { x := alsUeberweisung(a, bb, wei(10_000_000), t1, r1+"00"); return x }()},
	}
	for _, f := range faelle {
		b.add("ueb/%s roh=%s", f.name, f.tx.Roh)
		b.add("ueb/%s hash=%s", f.name, f.tx.TxHash)
		tx := f.tx
		nonce, err := pruefeUeberweisungsRoh(&tx)
		if err != nil {
			b.add("ueb/%s pruefung=abgelehnt", f.name)
			b.add("#ueb/%s fehler=%q", f.name, err.Error())
		} else {
			b.add("ueb/%s pruefung=ok nonce=%d", f.name, nonce)
		}
		if t, err := decodeRohTransaktion(f.tx.Roh); err == nil {
			absender, aerr := absenderWiederherstellen(t)
			b.add("ueb/%s absender=%s fehler=%v typ=%d", f.name, strings.ToLower(absender.Hex()), aerr != nil, t.Type())
		} else {
			b.add("ueb/%s nicht-lesbar", f.name)
			b.add("#ueb/%s lesefehler=%q", f.name, err.Error())
		}
	}

	dag, cs := nachspielKnoten(t, map[string]float64{
		strings.ToLower(a.Hex()):  1000,
		strings.ToLower(bb.Hex()): 500,
		strings.ToLower(c.Hex()):  0.25,
	})
	b.add("knoten/start stateroot=%s", cs.StateRoot())
	bloecke := [][]Transaction{
		{faelle[0].tx, faelle[1].tx},
		{faelle[2].tx, faelle[3].tx},
		{faelle[0].tx},
		{faelle[4].tx},
		{faelle[8].tx},
		{faelle[9].tx},
	}
	for i, txs := range bloecke {
		ok := dag.replayTransactions(testBlock(i+1, txs...), true)
		b.add("knoten/block%d angenommen=%v stateroot=%s", i+1, ok, cs.StateRoot())
		for _, adr := range []common.Address{a, bb, c} {
			k := kontoVon(t, cs, strings.ToLower(adr.Hex()))
			b.add("knoten/block%d %s guthaben=%d naechste-nonce=%d", i+1, strings.ToLower(adr.Hex()), k.Balance.Micro(), k.NaechsteNonce)
		}
	}
}

// ─── 5. bn254-Fehlerfaelle (BioVerifier beim Nachspielen) ────────────────────

// Beim Nachspielen fuehrt jeder Knoten BioVerifier.verifyProof aus, und das
// laeuft ueber die bn254-Vorkompilate 0x06-0x08. Deren Implementierung hat
// go-ethereum gewechselt (cloudflare -> gnark); hier die Faelle, in denen
// Implementierungen am ehesten auseinanderlaufen: Punkt nicht auf der Kurve,
// Koordinate >= p, G2 ausserhalb der Untergruppe, falsche Laenge, leer.
func konsensBN254(t *testing.T, b *goldenBuch) {
	ts := uint64(1_790_000_000)
	von := common.HexToAddress("0x0000000000000000000000000000000000000a11")
	sdb, _, err := neueSpeicherStateDB()
	if err != nil {
		t.Fatal(err)
	}
	l := &evmLauf{b: b, sdb: sdb}
	for _, v := range konsensBN254Vektoren {
		in, err := hex.DecodeString(v.eingabe)
		if err != nil {
			t.Fatalf("%s: %v", v.name, err)
		}
		l.aufruf(fmt.Sprintf("bn/%02x/%s", v.adresse, v.name), ts, von, common.BytesToAddress([]byte{v.adresse}), in, 10_000_000)
	}
	// Dieselben Faelle durch den echten BioVerifier.
	bio := common.HexToAddress(BIO_VERIFIER_ADDR)
	bioCode, _ := hex.DecodeString(BioVerifierRuntimeBytecode)
	setzeCode(sdb, bio, bioCode)
	bioABI, err := abi.JSON(strings.NewReader(bioVerifierABI))
	if err != nil {
		t.Fatal(err)
	}
	z := func(s string) *big.Int { v, _ := new(big.Int).SetString(s, 10); return v }
	pFeld := z("21888242871839275222246405745257275088696311157297823662689037894645226208583")
	g1, g2 := bn254Punkte()
	aussen := [2][2]*big.Int{
		{z(konsensG2AusserhalbUntergruppe[0][0]), z(konsensG2AusserhalbUntergruppe[0][1])},
		{z(konsensG2AusserhalbUntergruppe[1][0]), z(konsensG2AusserhalbUntergruppe[1][1])},
	}
	g2ZuGross := [2][2]*big.Int{{g2[0][0], new(big.Int).Add(g2[0][1], pFeld)}, g2[1]}
	pub := [2]*big.Int{big.NewInt(1001), big.NewInt(2001)}
	faelle := []struct {
		name   string
		pA, pC [2]*big.Int
		pB     [2][2]*big.Int
	}{
		{"g2-ausserhalb-untergruppe", g1, g1, aussen},
		{"g2-koordinate-ab-p", g1, g1, g2ZuGross},
		{"pa-x-ab-p", [2]*big.Int{new(big.Int).Add(big.NewInt(1), pFeld), big.NewInt(2)}, g1, g2},
		{"pc-nicht-auf-kurve", g1, [2]*big.Int{big.NewInt(1), big.NewInt(3)}, g2},
		{"pa-unendlich", [2]*big.Int{big.NewInt(0), big.NewInt(0)}, g1, g2},
	}
	for _, f := range faelle {
		d, err := bioABI.Pack("verifyProof", f.pA, f.pB, f.pC, pub)
		if err != nil {
			t.Fatal(err)
		}
		l.aufruf("bn/bio/"+f.name, ts, von, bio, d, 30_000_000)
	}
	b.add("bn/wurzel=%x", evmZwischenwurzel(sdb))
}

// ─── Test ─────────────────────────────────────────────────────────────────────

func TestKonsensEVM_GoldenWerte(t *testing.T) {
	b := &goldenBuch{}
	b.add("chainconfig %s", konsensChainConfigText())
	konsensOpcodes(t, b)
	konsensVorkompilate(t, b)
	konsensVertraege(t, b)
	konsensUeberweisungen(t, b)
	konsensBN254(t, b)
	pruefeGolden(t, "konsens_evm_golden.txt", b)
}

// konsensChainConfigText: die Fork-Bloecke und -Zeiten der ChainConfig, wie
// sie in beiden Versionen gleich heissen. Neue Forks spaeterer Versionen
// muessen nil (aus) sein; das prueft TestKonsensEVM_KeineNeuenForks.
func konsensChainConfigText() string {
	c := chainConfig()
	z := func(v *uint64) string {
		if v == nil {
			return "aus"
		}
		return fmt.Sprint(*v)
	}
	return fmt.Sprintf("chainid=%v homestead=%v eip150=%v eip155=%v eip158=%v byzantium=%v constantinople=%v petersburg=%v istanbul=%v berlin=%v london=%v shanghai=%s cancun=%s prague=%s",
		c.ChainID, c.HomesteadBlock, c.EIP150Block, c.EIP155Block, c.EIP158Block, c.ByzantiumBlock, c.ConstantinopleBlock,
		c.PetersburgBlock, c.IstanbulBlock, c.BerlinBlock, c.LondonBlock, z(c.ShanghaiTime), z(c.CancunTime), z(c.PragueTime))
}
