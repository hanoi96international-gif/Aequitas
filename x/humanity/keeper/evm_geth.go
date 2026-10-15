package keeper

// ANBINDUNG AN GO-ETHEREUM (EVM UND STATEDB).
//
// Die einzige Stelle, an der der Knoten die API von go-ethereum fuer EVM und
// StateDB benutzt. Ein Update von go-ethereum aendert diese Signaturen
// regelmaessig (StateDB-Konstruktor, Guthaben als uint256, Gasbudget,
// Commit-Parameter); hier wird es einmal angepasst, und
// konsens_evm_golden_test.go prueft, dass die EVM danach bei denselben
// Eingaben dieselben Ergebnisse liefert wie vorher (Golden-Werte erzeugt mit
// v1.13.0).
//
// Stand: go-ethereum v1.17.8. Gegenueber v1.13.0:
//   - state.New ohne Snapshot-Argument, state.Database aus triedb + CodeDB;
//   - Guthaben/Werte als *uint256.Int, Setter mit tracing-Grund;
//   - vm.NewEVM ohne TxContext (SetTxContext), Call/Create mit Absender als
//     Adresse und Gas als vm.GasBudget;
//   - Commit/IntermediateRoot nehmen params.Rules statt deleteEmptyObjects.

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/triedb"
	"github.com/holiman/uint256"
)

// neueSpeicherStateDB: eine leere StateDB im Speicher.
func neueSpeicherStateDB() (*state.StateDB, state.Database, error) {
	mem := rawdb.NewMemoryDatabase()
	db := state.NewMPTDatabase(triedb.NewDatabase(mem, nil), state.NewCodeDB(mem))
	sdb, err := state.New(common.Hash{}, db)
	if err != nil {
		return nil, nil, err
	}
	return sdb, db, nil
}

// oeffneStateDB: eine StateDB auf einer festgeschriebenen Wurzel.
func oeffneStateDB(root common.Hash, db state.Database) (*state.StateDB, error) {
	return state.New(root, db)
}

// neueKnotenEVM: die EVM mit den Regeln des Knotens (chainConfig,
// blockContext) fuer einen Aufruf mit Absender origin zur Zeit ts.
func neueKnotenEVM(ts uint64, origin common.Address, sdb *state.StateDB) *vm.EVM {
	evm := vm.NewEVM(blockContext(ts), sdb, chainConfig(), vm.Config{})
	evm.SetTxContext(vm.TxContext{Origin: origin, GasPrice: new(uint256.Int)})
	return evm
}

// zuU256: Werte sind hier nie negativ (msg.value > 0 lehnt der Knoten
// vorher ab); nil zaehlt als 0. Ein Wert ueber 2^256 kann nicht entstehen
// und wird fail-closed als Ueberlauf gemeldet, nicht abgeschnitten.
func zuU256(v *big.Int) (*uint256.Int, bool) {
	if v == nil {
		return new(uint256.Int), true
	}
	if v.Sign() < 0 {
		return nil, false
	}
	u, ueberlauf := uint256.FromBig(v)
	return u, !ueberlauf
}

var errWertAusserhalbUint256 = errors.New("EVM-Wert negativ oder ueber 2^256")

func gasRest(g vm.GasBudget) uint64 { return g.ExecutionGas + g.StateGas }

// evmErzeugen: CREATE; rest ist das uebrige Gas.
func evmErzeugen(evm *vm.EVM, von common.Address, code []byte, gas uint64, wert *big.Int) (ret []byte, adresse common.Address, rest uint64, err error) {
	w, ok := zuU256(wert)
	if !ok {
		return nil, common.Address{}, 0, errWertAusserhalbUint256
	}
	ret, adresse, g, err := evm.Create(von, code, vm.NewGasBudget(gas, 0), w)
	return ret, adresse, gasRest(g), err
}

// evmAufrufen: CALL; rest ist das uebrige Gas.
func evmAufrufen(evm *vm.EVM, von, an common.Address, data []byte, gas uint64, wert *big.Int) (ret []byte, rest uint64, err error) {
	w, ok := zuU256(wert)
	if !ok {
		return nil, 0, errWertAusserhalbUint256
	}
	ret, g, err := evm.Call(von, an, data, vm.NewGasBudget(gas, 0), w)
	return ret, gasRest(g), err
}

// setzeGuthaben: wei ist nie negativ (evmBalanceWei klemmt auf 0). Ein Wert
// ueber 2^256 ist bei AEQ-Betraegen unmoeglich; er wuerde auf 2^256-1
// begrenzt statt still abgeschnitten.
func setzeGuthaben(sdb *state.StateDB, addr common.Address, wei *big.Int) {
	u, ok := zuU256(wei)
	if !ok {
		if wei.Sign() < 0 {
			u = new(uint256.Int)
		} else {
			u = new(uint256.Int).SetAllOne()
		}
	}
	sdb.SetBalance(addr, u, tracing.BalanceChangeUnspecified)
}

func setzeNonce(sdb *state.StateDB, addr common.Address, nonce uint64) {
	sdb.SetNonce(addr, nonce, tracing.NonceChangeUnspecified)
}

func setzeCode(sdb *state.StateDB, addr common.Address, code []byte) {
	sdb.SetCode(addr, code, tracing.CodeChangeUnspecified)
}

// festschreibRegeln: die Regeln der EVM des Knotens -- mit einer Ausnahme:
// leere Konten bleiben stehen (IsEIP158=false), wie bei Commit(0, false) in
// v1.13.0. Sonst aenderte sich die Wurzel jeder StateDB, in der ein Konto
// mit Guthaben 0 geladen wurde.
func festschreibRegeln() params.Rules {
	bc := blockContext(0)
	r := chainConfig().Rules(bc.BlockNumber, bc.Random != nil, bc.Time)
	r.IsEIP158 = false
	return r
}

// evmFestschreiben schreibt die StateDB fest. Leere Konten bleiben stehen
// (deleteEmptyObjects=false) -- so hat es der Knoten immer gemacht.
func evmFestschreiben(sdb *state.StateDB) (common.Hash, error) {
	return sdb.Commit(festschreibRegeln(), 0)
}

// evmZwischenwurzel: die Zustandswurzel ohne Festschreiben, mit denselben
// Regeln wie evmFestschreiben (leere Konten bleiben).
func evmZwischenwurzel(sdb *state.StateDB) common.Hash {
	return sdb.IntermediateRoot(festschreibRegeln())
}
