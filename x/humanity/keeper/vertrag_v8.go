package keeper

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

// Registervertrag V8 (contracts/AequitasV8.sol, docs/V8_ENTWURF.md).
//
// # Wann V8 gilt
//
// Nur auf einer Kette, deren genesis.json es verlangt:
//
//	"register_vertrag": {"version": "v8", "registrare": ["0x…", "0x…"]}
//
// Fehlt der Eintrag, laeuft alles wie bisher mit V7. Die laufende Kette
// (genesis_time 2026-06-13) hat ihn nicht, und ein Push auf main geht direkt
// auf die Server -- nichts hier aendert ihr Verhalten. Ein unbekannter Wert
// oder eine ungueltige Registrarliste haelt den Knoten beim Start an
// (fail-closed): lieber kein Start als ein Knoten, der Registrierungen nach
// dem falschen Vertrag prueft und nachspielt.
//
// # Wo V8 liegt
//
// An derselben Adresse wie V7 (V7_CONTRACT_ADDR). Nach dem Neustart bei null
// ist sie frei, und alle Verweise darauf bleiben gueltig: Wallets, App,
// Explorer und der RPC-Abfang von transfer/balanceOf/isHuman (die Selektoren
// sind in V8 dieselben). Signaturen der alten Kette gelten trotzdem nicht --
// die EIP-712-Domaene traegt NETZ_SALT = keccak256(netzKennung()), und die
// Genesis-Zeit darin ist neu.
//
// # Nonce
//
// V8 kennt keinen Weg, eine Registrierung zurueckzunehmen: weder der Vertrag
// (kein Sweep, keine Admin-Funktion) noch Go (ein Nullifier wird nur beim
// Zurueckrollen eines Blocks wieder frei). Jede Wallet wird also hoechstens
// einmal Mensch, und die Nonce jeder gueltigen Registrierung ist 0. Knoten,
// die nachspielen, verlangen deshalb Nonce 0 -- ohne eigene EVM-Ausfuehrung
// und ohne einem Peer die Nonce zu glauben. Kommt je ein Weg zum
// Zuruecknehmen dazu, muss diese Regel mit ihm geaendert werden
// (TestV8_NonceIstImmerNull haelt die Annahme fest).

// ─── Genesis-Schalter ────────────────────────────────────────────────────────

const (
	vertragVersionV7 = "v7"
	vertragVersionV8 = "v8"

	// Wie AequitasV8.MAX_REGISTRARS.
	v8MaxRegistrare = 16
)

type vertragKonfig struct {
	version    string
	registrare []common.Address
}

var (
	vertragEinmal  sync.Once
	vertragGelesen vertragKonfig
	vertragFehler  error

	vertragTestMu sync.RWMutex
	vertragTest   *vertragKonfig
)

// ladeVertragKonfig liest "register_vertrag" aus dem Inhalt einer
// genesis.json. Fehlt der Eintrag: V7.
func ladeVertragKonfig(data []byte) (vertragKonfig, error) {
	var g struct {
		RegisterVertrag *struct {
			Version    string   `json:"version"`
			Registrare []string `json:"registrare"`
		} `json:"register_vertrag"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&g); err != nil {
		return vertragKonfig{}, fmt.Errorf("genesis.json unlesbar: %w", err)
	}
	if g.RegisterVertrag == nil {
		return vertragKonfig{version: vertragVersionV7}, nil
	}
	v := strings.ToLower(strings.TrimSpace(g.RegisterVertrag.Version))
	switch v {
	case vertragVersionV7:
		if len(g.RegisterVertrag.Registrare) > 0 {
			return vertragKonfig{}, errors.New("register_vertrag: V7 kennt keine Registrarliste")
		}
		return vertragKonfig{version: vertragVersionV7}, nil
	case vertragVersionV8:
	default:
		return vertragKonfig{}, fmt.Errorf("register_vertrag: unbekannte Version %q", g.RegisterVertrag.Version)
	}
	n := len(g.RegisterVertrag.Registrare)
	if n == 0 || n > v8MaxRegistrare {
		return vertragKonfig{}, fmt.Errorf("register_vertrag: %d Registrare, erlaubt 1..%d", n, v8MaxRegistrare)
	}
	gesehen := make(map[common.Address]bool, n)
	registrare := make([]common.Address, 0, n)
	for _, r := range g.RegisterVertrag.Registrare {
		r = strings.ToLower(strings.TrimSpace(r))
		if !isValidWalletAddr(r) {
			return vertragKonfig{}, fmt.Errorf("register_vertrag: ungueltige Registrar-Adresse %q", r)
		}
		a := common.HexToAddress(r)
		if a == (common.Address{}) {
			return vertragKonfig{}, errors.New("register_vertrag: Registrar ist die Null-Adresse")
		}
		if gesehen[a] {
			return vertragKonfig{}, fmt.Errorf("register_vertrag: Registrar %s doppelt", r)
		}
		gesehen[a] = true
		registrare = append(registrare, a)
	}
	return vertragKonfig{version: vertragVersionV8, registrare: registrare}, nil
}

func vertrag() (vertragKonfig, error) {
	vertragTestMu.RLock()
	t := vertragTest
	vertragTestMu.RUnlock()
	if t != nil {
		return *t, nil
	}
	vertragEinmal.Do(func() {
		data, err := os.ReadFile("genesis.json")
		if err != nil {
			// Keine genesis.json: wie bisher (genesisTimestamp faellt ebenso
			// zurueck). Eine V8-Kette hat immer eine.
			vertragGelesen = vertragKonfig{version: vertragVersionV7}
			return
		}
		vertragGelesen, vertragFehler = ladeVertragKonfig(data)
	})
	return vertragGelesen, vertragFehler
}

// vertragV8 sagt, ob diese Kette mit AequitasV8 laeuft. Bei einer
// fehlerhaften Genesis true -- dann greifen die strengeren V8-Regeln
// (pruefeVertragGenesis haelt den Knoten ohnehin vorher an).
func vertragV8() bool {
	k, err := vertrag()
	return err != nil || k.version == vertragVersionV8
}

func vertragVersion() string {
	k, err := vertrag()
	if err != nil {
		return "ungueltig"
	}
	return k.version
}

func v8Registrare() []common.Address {
	k, _ := vertrag()
	return append([]common.Address(nil), k.registrare...)
}

// PruefeVertragGenesis wird beim Start aufgerufen (cmd/aequitasd). Ein
// Fehler haelt den Knoten an.
func PruefeVertragGenesis() error {
	_, err := vertrag()
	return err
}

// VertragVersion: "v7" oder "v8" (fuer Startmeldung und /api/status).
func VertragVersion() string { return vertragVersion() }

// _setVertragForTest setzt die Konfiguration fuer einen Test; nil = Genesis.
func _setVertragForTest(k *vertragKonfig) (zurueck func()) {
	vertragTestMu.Lock()
	alt := vertragTest
	vertragTest = k
	vertragTestMu.Unlock()
	return func() {
		vertragTestMu.Lock()
		vertragTest = alt
		vertragTestMu.Unlock()
	}
}

// ─── Speicherbelegung (contracts/v8_slots.json) ──────────────────────────────
//
// TestV8Slots_GleichDerTabelle vergleicht jede Konstante mit der Tabelle,
// test/AequitasV8_storage_layout.ts die Tabelle mit dem Compiler.

const (
	v8SlotTotalSupply     int64 = 0
	v8SlotTotalHumans     int64 = 1
	v8SlotBalanceOf       int64 = 2
	v8SlotIsHuman         int64 = 3
	v8SlotUsedCommitments int64 = 4
	v8SlotUsedNullifiers  int64 = 5
	v8SlotCommitmentOf    int64 = 6
	v8SlotNullifierOf     int64 = 7
	v8SlotNonces          int64 = 8
	v8SlotIsRegistrar     int64 = 9
)

// registerWithSig(uint256[2],uint256[2][2],uint256[2],uint256[2],address,uint256,bytes)
const v8RegisterSelector = "60529762"

// Aufrufdaten von registerWithSig (ab Byte 0, inkl. Selektor):
// pA 4-68, pB 68-196, pC 196-260, pubSignals[0] 260-292, pubSignals[1]
// 292-324, human 324-356, deadline 356-388, Offset der Signatur 388-420.
const (
	v8OffCommitment = 260
	v8OffNullifier  = 292
	v8OffHuman      = 324
	v8OffDeadline   = 356
)

// Wie AequitasV8.MAX_SIGNATURE_LIFETIME.
const v8MaxSignaturLaufzeit int64 = 24 * 60 * 60

// BN254-Skalarkoerper, wie AequitasV8.SNARK_SCALAR_FIELD.
var v8SnarkField, _ = new(big.Int).SetString("21888242871839275222246405745257275088548364400416034343698204186575808495617", 10)

// ─── EIP-712 ────────────────────────────────────────────────────────────────

var (
	v8DomainTypeHash    = crypto.Keccak256Hash([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract,bytes32 salt)"))
	v8RegisterTypeHash  = crypto.Keccak256Hash([]byte("Register(address human,uint256 commitment,uint256 nullifier,uint256 nonce,uint256 deadline)"))
	v8NameHash          = crypto.Keccak256Hash([]byte("Aequitas"))
	v8VersionHash       = crypto.Keccak256Hash([]byte("8"))
	secp256k1HalbesN, _ = new(big.Int).SetString("7FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF5D576E7357A4501DDFE92F46681B20A0", 16)
)

// v8NetzSalt: keccak256(netzKennung()), dasselbe, was der Vertrag als
// NETZ_SALT im Konstruktor bekommt.
func v8NetzSalt() common.Hash {
	return crypto.Keccak256Hash([]byte(netzKennung()))
}

func wort(b []byte) []byte { return common.LeftPadBytes(b, 32) }

func v8DomainSeparator(vertragAddr common.Address, salt common.Hash) common.Hash {
	return crypto.Keccak256Hash(
		v8DomainTypeHash.Bytes(),
		v8NameHash.Bytes(),
		v8VersionHash.Bytes(),
		wort(aequitasChainID.Bytes()),
		wort(vertragAddr.Bytes()),
		salt.Bytes(),
	)
}

// v8RegisterDigest: der Digest, den die Wallet `human` unterschreibt --
// byte-gleich zu AequitasV8._registerDigest (TestV8_DigestGleichDemVertrag).
func v8RegisterDigest(vertragAddr common.Address, salt common.Hash, human common.Address, commitment, nullifier, nonce, deadline *big.Int) common.Hash {
	structHash := crypto.Keccak256Hash(
		v8RegisterTypeHash.Bytes(),
		wort(human.Bytes()),
		wort(commitment.Bytes()),
		wort(nullifier.Bytes()),
		wort(nonce.Bytes()),
		wort(deadline.Bytes()),
	)
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, v8DomainSeparator(vertragAddr, salt).Bytes(), structHash.Bytes())
}

// v8SignaturPruefen: dieselbe strenge Pruefung wie AequitasV8._recover --
// 65 Byte, v in {27, 28}, niedriges s (EIP-2), Ergebnis == human, nie 0.
func v8SignaturPruefen(digest common.Hash, sig []byte, human common.Address) error {
	if len(sig) != 65 {
		return errors.New("Signatur muss 65 Byte haben")
	}
	v := sig[64]
	if v != 27 && v != 28 {
		return errors.New("Signatur: v muss 27 oder 28 sein")
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])
	if r.Sign() == 0 || s.Sign() == 0 || s.Cmp(secp256k1HalbesN) > 0 {
		return errors.New("Signatur: r/s ungueltig oder hohes s")
	}
	roh := make([]byte, 65)
	copy(roh, sig)
	roh[64] = v - 27
	pub, err := crypto.SigToPub(digest.Bytes(), roh)
	if err != nil {
		return errors.New("Signatur nicht wiederherstellbar")
	}
	signer := crypto.PubkeyToAddress(*pub)
	if signer == (common.Address{}) || signer != human {
		return errors.New("Signatur stammt nicht von der Wallet, die Mensch werden soll")
	}
	return nil
}

// v8FristPruefen: now <= deadline <= now + 1 Tag, wie im Vertrag.
func v8FristPruefen(deadline, jetzt int64) error {
	if deadline < jetzt {
		return errors.New("die Signatur ist abgelaufen -- bitte neu unterschreiben")
	}
	if deadline > jetzt+v8MaxSignaturLaufzeit {
		return errors.New("die Frist der Signatur liegt mehr als einen Tag in der Zukunft")
	}
	return nil
}

// dezimalImFeld liest ein oeffentliches Signal (Dezimalzahl) und verlangt
// 0 < x < r, wie der Vertrag.
func dezimalImFeld(s string) (*big.Int, error) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 80 {
		return nil, errors.New("oeffentliches Signal fehlt oder ist zu lang")
	}
	x, ok := new(big.Int).SetString(s, 10)
	if !ok || x.Sign() <= 0 || x.Cmp(v8SnarkField) >= 0 {
		return nil, errors.New("oeffentliches Signal ausserhalb des Feldes")
	}
	return x, nil
}

// ─── Nachspielen ────────────────────────────────────────────────────────────

// v8NachspielKarenz: wie weit die Annahmezeit (RegAt) hinter der Blockzeit
// in der ZUKUNFT liegen darf (Uhrengang zwischen Knoten). Nach hinten gibt es
// bewusst keine Grenze: eine Registrierung, die nach einem Neustart des
// Knotens erst spaet in einen Block kommt, darf die Kette nicht anhalten.
const v8NachspielKarenz int64 = 5 * 60

// pruefeRegistrierungV8 prueft beim Nachspielen eines register_human, dass
// die Wallet selbst zugestimmt hat -- auf JEDEM Knoten, ohne EVM und ohne dem
// erzeugenden Knoten etwas zu glauben (AGENTS.md Punkt 4):
//   - EIP-712-Signatur der Wallet ueber (Wallet, commitment = pubSignals[0],
//     nullifier = pubSignals[1], Nonce 0, Frist) in der Domaene dieses Netzes,
//   - die Frist galt zum Annahmezeitpunkt (RegAt <= Frist <= RegAt + 1 Tag),
//   - die Annahme liegt nicht in der Zukunft des Blocks.
//
// Ob der Beweis gueltig ist und der Nullifier zu ihm gehoert, prueft
// replayTransactions davor (verifyZKProof, nullifierMatchesProof).
func pruefeRegistrierungV8(tx Transaction, blockZeit int64) error {
	if len(tx.PubSignals) < 2 {
		return errors.New("oeffentliche Signale fehlen")
	}
	commitment, err := dezimalImFeld(tx.PubSignals[0])
	if err != nil {
		return fmt.Errorf("commitment: %w", err)
	}
	nullifier, err := dezimalImFeld(tx.PubSignals[1])
	if err != nil {
		return fmt.Errorf("nullifier: %w", err)
	}
	wallet := strings.ToLower(strings.TrimSpace(tx.Wallet))
	if !isValidWalletAddr(wallet) {
		return errors.New("Wallet ungueltig")
	}
	if tx.RegSignatur == "" || tx.RegFrist <= 0 || tx.RegAt <= 0 {
		return errors.New("Signatur, Frist oder Annahmezeit der Wallet fehlt")
	}
	if tx.RegAt > blockZeit+v8NachspielKarenz {
		return errors.New("Annahmezeit liegt nach dem Block")
	}
	if err := v8FristPruefen(tx.RegFrist, tx.RegAt); err != nil {
		return fmt.Errorf("Frist zum Annahmezeitpunkt: %w", err)
	}
	sig, err := hexutil.Decode(tx.RegSignatur)
	if err != nil {
		return errors.New("Signatur ist kein Hex")
	}
	human := common.HexToAddress(wallet)
	digest := v8RegisterDigest(common.HexToAddress(V7_CONTRACT_ADDR), v8NetzSalt(), human,
		commitment, nullifier, big.NewInt(0), big.NewInt(tx.RegFrist))
	return v8SignaturPruefen(digest, sig, human)
}
