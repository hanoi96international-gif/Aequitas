package keeper

// DER WECHSEL DER EVM-CHAIN-ID (vom Betreiber entschieden, 15.10.2026) --
// SCHLAFEND, bis ein eigener PR den Stichtag setzt.
//
// Die Kette wurde am 30.09.2026 bei null neu gestartet (V8-Genesis), behielt
// aber die EVM-Chain-ID 1926 der alten Kette. Eine Wallet, die auf der alten
// Kette eine Ueberweisung mit Nonce n unterschrieben hat, hat damit zugleich
// eine gueltige Ueberweisung fuer diese Kette unterschrieben: dieselbe
// Signatur, dieselbe Chain-ID, und NaechsteNonce begann hier wieder bei 0.
// Wer alte Rohformen gesammelt hat (jeder Knoten, jeder Explorer der alten
// Kette), kann sie hier erneut einreichen.
//
// Abhilfe: ab chainIDWechselAbUnix gilt die neue Chain-ID 19260930
// (0x125E602) -- fuer jede signierte EVM-Transaktion, fuer block.chainid in
// der EVM und damit fuer die EIP-712-Domaene von V8. Alles haengt an der
// BLOCKZEIT (Konsens) bzw. an der Annahmezeit (nur der Annehmende):
//
//   - Blockpruefung (signierte_ueberweisung.go, pruefeUeberweisungsRoh): eine
//     Ueberweisung in einem Block mit Blockzeit < Ab muss mit 1926 signiert
//     sein, ab Ab mit der neuen ID. Jeder Knoten prueft das selbst, alte
//     Bloecke spielen unveraendert nach.
//   - V8-Registrierung (vertrag_v8.go): die Domaene mit chainIDFuer(RegAt).
//   - Annahme (evm_rpc.go, chainIDAnnahmePruefen): Ueberweisungen mit 1926
//     nur bis Ab-chainIDWechselPause, mit der neuen ID erst ab Ab; dazwischen
//     nimmt der Knoten keine Ueberweisungen an (wiederholbar, wie
//     annahme_pause.go). In dieser Pause verblockt der Ausgang, was mit 1926
//     angenommen wurde -- mit einer Blockzeit vor Ab (block_tauglich.go).
//
// EINGEFROREN, OHNE BEZUG AUF DIESE DATEI (eingefroreneChainID):
//
//   - netzKennung (api_app_grundlagen.go): daraus folgt v8NetzSalt, und
//     NETZ_SALT ist im Vertrag immutable (contracts/AequitasV8.sol).
//   - die "chain:1926"-Saetze der Coordinator-Bindung und Bescheinigung
//     (coordinator_nachrichten.go) und der Validator-Bindung
//     (validator_register.go): gespeicherte Freigaben werden erneut geprueft.
//   - checkpointChainID (checkpoint_zertifikat.go).
//
// Sie trennen diese Kette von Testnetzen; die Wiederholung alter
// EVM-Transaktionen betrifft sie nicht (eigene Saetze, eigene Fristen).
//
// SCHLAFEND: chainIDWechselAbUnix = math.MaxInt64. Dann ist alles byte-gleich
// zu vorher: chainIDFuer liefert immer 1926, kein Fenster, keine Pause
// (TestChainIDWechsel_SchlafendByteGleich). Ablauf: docs/CHAIN_ID_WECHSEL.md.

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	// eingefroreneChainID: die Chain-ID der V8-Genesis. Bleibt in allen
	// Saetzen und Salzen, die vor dem Wechsel unterschrieben oder in einen
	// Vertrag geschrieben wurden (siehe oben) -- unabhaengig vom Stichtag.
	eingefroreneChainID int64 = 1926

	// chainIDAltWert / chainIDNeuWert: vor und ab dem Stichtag. Die Freiheit
	// von 19260930 ist vor dem Festlegen in ethereum-lists/chains zu pruefen.
	chainIDAltWert int64 = eingefroreneChainID
	chainIDNeuWert int64 = 19260930 // 0x125E602

	// chainIDWechselAbUnix: Blockzeit, ab der die neue ID gilt.
	// math.MaxInt64 = SCHLAFEND. Gesetzt wird der Stichtag in einem eigenen
	// PR mit mindestens 48 h Vorlauf, wenn App und Ankuendigung stehen.
	//
	// Gesetzt am 17.10.2026, auf Anweisung des Betreibers vorgezogen vom
	// 27.10. auf 1792281600 = 18.10.2026 00:00:00 UTC (rund 2,5 h Vorlauf;
	// ohne die 7-Tage-Ankuendigung -- bewusst, nur ein Validator, App 1.11.0
	// veroeffentlicht). Annahmepause ab Ab - 600 s = 17.10.2026 23:50 UTC.
	// Rueckweg nur VOR Ab: wieder math.MaxInt64, ausrollen
	// (docs/CHAIN_ID_WECHSEL.md).
	chainIDWechselAbUnix int64 = 1792281600

	// chainIDWechselPause: so lange vor dem Stichtag nimmt der Knoten keine
	// Ueberweisungen mehr an. Was davor mit 1926 angenommen wurde, steht
	// lange vorher in einem Block mit Blockzeit < Ab.
	chainIDWechselPause int64 = 600
)

// chainIDWechselOverride: nur fuer Tests (0 = Konstante gilt).
var chainIDWechselOverride atomic.Int64

func chainIDWechselAb() int64 {
	if o := chainIDWechselOverride.Load(); o != 0 {
		return o
	}
	return chainIDWechselAbUnix
}

// chainIDWechselGeplant: ist ein Stichtag gesetzt?
func chainIDWechselGeplant() bool { return chainIDWechselAb() != math.MaxInt64 }

// chainIDWertFuer: die Chain-ID, die fuer diese Blockzeit (bzw. Annahmezeit)
// gilt.
// Schlafend (MaxInt64) gilt 1926 fuer jede Zeit, auch fuer MaxInt64 selbst.
func chainIDWertFuer(blockZeit int64) int64 {
	if ab := chainIDWechselAb(); ab != math.MaxInt64 && blockZeit >= ab {
		return chainIDNeuWert
	}
	return chainIDAltWert
}

// chainIDFuer: wie chainIDWertFuer, als frischer *big.Int (der Aufrufer darf
// ihn behalten).
func chainIDFuer(blockZeit int64) *big.Int {
	return big.NewInt(chainIDWertFuer(blockZeit))
}

// chainIDJetzt: die Chain-ID nach der Uhr dieses Knotens -- fuer RPC, EVM
// und Relayer, nie fuer die Blockpruefung.
func chainIDJetzt() int64 { return chainIDWertFuer(nowUnix()) }

// chainIDHexFuer: wie eth_chainId sie meldet.
func chainIDHexFuer(zeit int64) string { return fmt.Sprintf("0x%x", chainIDWertFuer(zeit)) }

// istChainID: cid == want, ohne Allokation.
func istChainID(cid *big.Int, want int64) bool {
	return cid != nil && cid.IsInt64() && cid.Int64() == want
}

// chainIDGiltFuer: ist eine mit cid signierte Transaktion in einem Block mit
// dieser Blockzeit gueltig? Genau eine ID je Blockzeit.
func chainIDGiltFuer(cid *big.Int, blockZeit int64) bool {
	return istChainID(cid, chainIDWertFuer(blockZeit))
}

// ErrChainIDFalsch: die Transaktion ist fuer eine andere Chain-ID signiert,
// als zu ihrer Zeit gilt.
var ErrChainIDFalsch = errors.New("falsche Chain-ID")

// chainIDAnnahmePruefen: darf der annehmende Knoten jetzt eine geschuetzte
// (EIP-155) Transaktion mit Chain-ID cid annehmen? ueberweisung: sie wird als
// Ueberweisung in einen Block geschrieben (ueberweisungAusRoh) -- dann gilt
// das Fenster mit Pause; jede andere laeuft nur hier in der EVM und braucht
// genau chainIDWertFuer(jetzt).
//
// Fehler mit ErrAnnahmePausiert sind wiederholbar (-32005), alle anderen
// endgueltig: die Wallet muss mit der richtigen ID neu unterschreiben.
func chainIDAnnahmePruefen(cid *big.Int, ueberweisung bool, jetzt int64) error {
	ab := chainIDWechselAb()
	if ab == math.MaxInt64 || !ueberweisung {
		if want := chainIDWertFuer(jetzt); !istChainID(cid, want) {
			return fmt.Errorf("%w: transaction must be replay-protected with chain id %d (EIP-155)", ErrChainIDFalsch, want)
		}
		return nil
	}
	pauseAb := sattAdd(ab, -chainIDWechselPause)
	switch {
	case istChainID(cid, chainIDAltWert):
		if jetzt < pauseAb {
			return nil
		}
		return fmt.Errorf("%w: chain id %d is no longer accepted for transfers since %d -- sign again with chain id %d (EIP-155)",
			ErrChainIDFalsch, chainIDAltWert, pauseAb, chainIDNeuWert)
	case istChainID(cid, chainIDNeuWert):
		if jetzt >= ab {
			return nil
		}
		if jetzt >= pauseAb {
			return fmt.Errorf("%w: chain id change to %d at %d -- transfers resume then", ErrAnnahmePausiert, chainIDNeuWert, ab)
		}
		return fmt.Errorf("%w: chain id %d is valid from %d -- until then sign with chain id %d (EIP-155)",
			ErrChainIDFalsch, chainIDNeuWert, ab, chainIDAltWert)
	}
	return fmt.Errorf("%w: transaction must be replay-protected with chain id %d (EIP-155)", ErrChainIDFalsch, chainIDWertFuer(jetzt))
}

// rohChainID: die Chain-ID einer signierten Rohform; ok=false, wenn sie
// nicht lesbar oder ungeschuetzt ist. Nur fuer die Blockzeitwahl des
// Erzeugers (block_tauglich.go) -- verbindlich ist pruefeUeberweisungsRoh.
//
// Liest nur das noetige RLP-Feld statt die ganze Transaktion zu
// dekodieren: gemessen 1,8 us je Ueberweisung mit decodeRohTransaktion, und
// blockZeitFuer fragt jede Ueberweisung eines Blocks mindestens einmal --
// bei 7.000 Ueberweisungen unter der Sperre des Blockbaus zu viel.
// TestChainIDWechsel_RohChainIDWieDekodiert haelt es gleich zur vollen
// Dekodierung.
//
// Eine typisierte Transaktion als RLP-String verpackt (erstes Byte
// 0x80-0xbf) nimmt decodeRohTransaktion ueber den rlp.DecodeBytes-Rueckfall
// ebenfalls an; die Schnelllesung kennt diese Form nicht. Fuer sie wird voll
// dekodiert -- sonst haette sie kein Chain-ID-Fenster, der Erzeuger legte
// eine 1926-Ueberweisung in einen Block ab dem Stichtag, und jeder andere
// Knoten wiese ihn ab (Sicherheitspruefung #360, Befund 1). Kanonische
// Formen erreichen diesen Weg nie.
func rohChainID(roh string) (int64, bool) {
	b, err := hex.DecodeString(strings.TrimPrefix(roh, "0x"))
	if err != nil || len(b) == 0 {
		return 0, false
	}
	if b[0] >= 0x80 && b[0] < 0xc0 {
		return rohChainIDVoll(roh)
	}
	if b[0] >= 0xc0 {
		// Legacy: [nonce, gasPrice, gas, to, value, data, v, r, s]
		inhalt, rest, err := rlp.SplitList(b)
		if err != nil || len(rest) != 0 {
			return 0, false
		}
		var v []byte
		for i := 0; i < 7; i++ {
			var k rlp.Kind
			var feld []byte
			k, feld, inhalt, err = rlp.Split(inhalt)
			if err != nil || (i != 5 && k == rlp.List) {
				return 0, false
			}
			v = feld
		}
		vb := new(big.Int).SetBytes(v)
		if vb.BitLen() <= 8 && (vb.Uint64() == 27 || vb.Uint64() == 28) {
			return 0, false // ungeschuetzt (vor EIP-155)
		}
		if vb.Cmp(big.NewInt(35)) < 0 {
			return 0, false
		}
		cid := vb.Sub(vb, big.NewInt(35))
		cid.Rsh(cid, 1)
		if !cid.IsInt64() {
			return 0, false
		}
		return cid.Int64(), true
	}
	if b[0] != types.AccessListTxType && b[0] != types.DynamicFeeTxType && b[0] != types.BlobTxType {
		return 0, false
	}
	inhalt, rest, err := rlp.SplitList(b[1:])
	if err != nil || len(rest) != 0 {
		return 0, false
	}
	cidRoh, _, err := rlp.SplitString(inhalt)
	if err != nil {
		return 0, false
	}
	cid := new(big.Int).SetBytes(cidRoh)
	if !cid.IsInt64() {
		return 0, false
	}
	return cid.Int64(), true
}

// rohChainIDVoll: wie rohChainID, aber ueber die volle Dekodierung --
// dieselbe, die pruefeUeberweisungsRoh beim Nachspielen nutzt.
func rohChainIDVoll(roh string) (int64, bool) {
	t, err := decodeRohTransaktion(roh)
	if err != nil || !t.Protected() {
		return 0, false
	}
	cid := t.ChainId()
	if cid == nil || !cid.IsInt64() {
		return 0, false
	}
	return cid.Int64(), true
}

// chainIDFenster: die Blockzeiten, zu denen eine Ueberweisung mit dieser
// Rohform die Chain-ID-Pruefung jedes Nachspielenden besteht
// (block_tauglich.go). ok=false: keine Einschraenkung -- schlafend, keine
// Ueberweisung, oder die Rohform ist unlesbar (dann scheitert sie an
// pruefeUeberweisungsRoh, wie bisher).
func chainIDFenster(tx *Transaction) (zeitFenster, bool) {
	if tx.Type != "transfer" || tx.Roh == "" || !chainIDWechselGeplant() {
		return zeitFenster{}, false
	}
	cid, ok := rohChainID(tx.Roh)
	if !ok {
		return zeitFenster{}, false
	}
	ab := chainIDWechselAb()
	switch cid {
	case chainIDAltWert:
		return zeitFenster{math.MinInt64, ab - 1}, true
	case chainIDNeuWert:
		return zeitFenster{ab, math.MaxInt64}, true
	}
	return zeitFenster{1, 0}, true // passt in keinen Block
}

// chainIDTauglich: besteht die Ueberweisung in einem Block mit dieser
// Blockzeit die Chain-ID-Pruefung?
func chainIDTauglich(tx *Transaction, blockZeit int64) error {
	f, ok := chainIDFenster(tx)
	if !ok || (blockZeit >= f.von && blockZeit <= f.bis) {
		return nil
	}
	cid, _ := rohChainID(tx.Roh)
	return fmt.Errorf("Ueberweisung von %s mit Chain-ID %d, Block %d (Wechsel ab %d)", kurzAdresse(tx.Wallet), cid, blockZeit, chainIDWechselAb())
}

// ChainIDWechselAbFuerStatus: fuer /api/status -- der Stichtag als
// Unix-Zeit, 0 solange schlafend.
func ChainIDWechselAbFuerStatus() int64 {
	if !chainIDWechselGeplant() {
		return 0
	}
	return chainIDWechselAb()
}
