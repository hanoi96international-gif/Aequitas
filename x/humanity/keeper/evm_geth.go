package keeper

// ANBINDUNG AN GO-ETHEREUM (EVM UND STATEDB).
//
// Die einzige Stelle, an der der Knoten die API von go-ethereum fuer EVM und
// StateDB benutzt. Ein Update von go-ethereum aendert diese Signaturen
// regelmaessig (StateDB-Konstruktor, Guthaben als uint256, Gasbudget,
// Commit-Parameter); hier wird es einmal angepasst, und
// konsens_evm_golden_test.go prueft, dass die EVM danach bei denselben
// Eingaben dieselben Ergebnisse liefert wie vorher.
//
// Stand: go-ethereum v1.13.0.

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
)

// neueSpeicherStateDB: eine leere StateDB im Speicher.
func neueSpeicherStateDB() (*state.StateDB, state.Database, error) {
	db := state.NewDatabase(rawdb.NewMemoryDatabase())
	sdb, err := state.New(common.Hash{}, db, nil)
	if err != nil {
		return nil, nil, err
	}
	return sdb, db, nil
}

// oeffneStateDB: eine StateDB auf einer festgeschriebenen Wurzel.
func oeffneStateDB(root common.Hash, db state.Database) (*state.StateDB, error) {
	return state.New(root, db, nil)
}

// neueKnotenEVM: die EVM mit den Regeln des Knotens (chainConfig,
// blockContext) fuer einen Aufruf mit Absender origin zur Zeit ts.
func neueKnotenEVM(ts uint64, origin common.Address, sdb *state.StateDB) *vm.EVM {
	return vm.NewEVM(blockContext(ts), vm.TxContext{Origin: origin, GasPrice: big.NewInt(0)}, sdb, chainConfig(), vm.Config{})
}

// evmErzeugen: CREATE; rest ist das uebrige Gas.
func evmErzeugen(evm *vm.EVM, von common.Address, code []byte, gas uint64, wert *big.Int) (ret []byte, adresse common.Address, rest uint64, err error) {
	return evm.Create(vm.AccountRef(von), code, gas, wert)
}

// evmAufrufen: CALL; rest ist das uebrige Gas.
func evmAufrufen(evm *vm.EVM, von, an common.Address, data []byte, gas uint64, wert *big.Int) (ret []byte, rest uint64, err error) {
	return evm.Call(vm.AccountRef(von), an, data, gas, wert)
}

func setzeGuthaben(sdb *state.StateDB, addr common.Address, wei *big.Int) {
	sdb.SetBalance(addr, wei)
}

func setzeNonce(sdb *state.StateDB, addr common.Address, nonce uint64) {
	sdb.SetNonce(addr, nonce)
}

func setzeCode(sdb *state.StateDB, addr common.Address, code []byte) {
	sdb.SetCode(addr, code)
}

// evmFestschreiben schreibt die StateDB fest. Leere Konten bleiben stehen
// (deleteEmptyObjects=false) -- so hat es der Knoten immer gemacht.
func evmFestschreiben(sdb *state.StateDB) (common.Hash, error) {
	return sdb.Commit(0, false)
}

// evmZwischenwurzel: die Zustandswurzel ohne Festschreiben, mit denselben
// Regeln wie evmFestschreiben (leere Konten bleiben).
func evmZwischenwurzel(sdb *state.StateDB) common.Hash {
	return sdb.IntermediateRoot(false)
}
