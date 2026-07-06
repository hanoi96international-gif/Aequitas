package keeper

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
)

// scratchGetterInitcode is the deployed initcode of a trivial scratch
// contract used only to isolate one question at the go-ethereum/trie level,
// independent of ChainState/Postgres:
//
//	contract ScratchRepro {
//	    mapping(address => uint256) public foo;
//	    function set(address a, uint256 v) external { foo[a] = v; }
//	}
//
// Investigated during G5 (beta-launch audit 2026-07-05) verification: a live
// eth_call for balanceOf/isHuman appeared to revert, and the working theory
// was that newStateDB()'s never-committed StateDB (see its own "Don't
// commit" comment) couldn't cleanly return zero for a mapping key that was
// never explicitly SetState'd (i.e. an address nobody had ever registered
// or transacted with). This test proves that theory wrong: reading an unset
// key from exactly this pattern (fresh StateDB, SetCode, SetState only for
// pre-existing rows) returns zero cleanly, no revert. The real cause of the
// observed revert was an external caller sending malformed calldata (just
// the 4-byte selector, no address argument) — solc's own auto-generated
// ABI-length check reverts with empty data for that, which is correct,
// standard EVM behavior, not a bug. Kept as a permanent regression test for
// the actual thing worth guaranteeing: this codebase's "never commit,
// partially loaded" StateDB pattern must never turn "key not found" into an
// error for a well-formed call.
const scratchGetterInitcode = "6080604052348015600e575f5ffd5b5061011c8061001c5f395ff3fe6080604052348015600e575f5ffd5b50600436106030575f3560e01c80633825d828146034578063fdf80bda14605c575b5f5ffd5b605a603f36600460a4565b6001600160a01b039091165f90815260208190526040902055565b005b6078606736600460c9565b5f6020819052908152604090205481565b60405190815260200160405180910390f35b80356001600160a01b0381168114609f575f5ffd5b919050565b5f5f6040838503121560b4575f5ffd5b60bb83608a565b946020939093013593505050565b5f6020828403121560d8575f5ffd5b60df82608a565b939250505056fea26469706673582212208631124abcc0130b9b2b697783a6cc25febb4f21f52e9a25afe7c63148b5c76a64736f6c634300081c0033"

func TestNewStateDBPattern_UnsetMappingKeyReturnsZeroNotRevert(t *testing.T) {
	// Deploy the scratch contract once, in a throwaway StateDB, purely to
	// get real deployed runtime code (hand-assembling it isn't worth it).
	memDB := rawdb.NewMemoryDatabase()
	db := state.NewDatabase(memDB)
	deployerSdb, err := state.New(common.Hash{}, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	deployer := common.HexToAddress("0x1111111111111111111111111111111111111111")
	deployerSdb.SetNonce(deployer, 0)
	initcode, _ := hex.DecodeString(scratchGetterInitcode)
	txCtx := vm.TxContext{Origin: deployer, GasPrice: big.NewInt(0)}
	evm := vm.NewEVM(blockContext(1), txCtx, deployerSdb, chainConfig(), vm.Config{})
	_, contractAddr, _, err := evm.Create(vm.AccountRef(deployer), initcode, 30_000_000, big.NewInt(0))
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	runtimeCode := deployerSdb.GetCode(contractAddr)
	if len(runtimeCode) == 0 {
		t.Fatal("no runtime code found after deploy")
	}

	// Now reproduce newStateDB()'s actual pattern: a fresh, never-committed
	// StateDB, SetCode for the contract, and SetState only for the ONE row
	// that "exists in evm_storage" (a direct Go-level mutation, exactly
	// like evm_engine.go's storage-loading loop — never an EVM opcode).
	freshMemDB := rawdb.NewMemoryDatabase()
	freshDB := state.NewDatabase(freshMemDB)
	sdb, err := state.New(common.Hash{}, freshDB, nil)
	if err != nil {
		t.Fatal(err)
	}
	sdb.SetCode(contractAddr, runtimeCode)

	knownAddr := common.HexToAddress("0x2222222222222222222222222222222222222222")
	unknownAddr := common.HexToAddress("0x3333333333333333333333333333333333333333")
	knownSlot := mappingSlot(knownAddr.Bytes(), 0) // foo is slot 0
	sdb.SetState(contractAddr, knownSlot, common.BigToHash(big.NewInt(42)))

	fooSel, _ := hex.DecodeString("fdf80bda") // foo(address)
	callGetter := func(addr common.Address) ([]byte, error) {
		data := append(append([]byte{}, fooSel...), make([]byte, 32)...)
		copy(data[4+12:4+32], addr.Bytes())
		txCtx := vm.TxContext{Origin: deployer, GasPrice: big.NewInt(0)}
		evm := vm.NewEVM(blockContext(1), txCtx, sdb, chainConfig(), vm.Config{})
		ret, _, callErr := evm.Call(vm.AccountRef(deployer), contractAddr, data, 30_000_000, big.NewInt(0))
		return ret, callErr
	}

	knownRet, err := callGetter(knownAddr)
	if err != nil {
		t.Fatalf("foo(knownAddr) unexpectedly reverted: %v", err)
	}
	if new(big.Int).SetBytes(knownRet).Int64() != 42 {
		t.Errorf("foo(knownAddr) = %x, want 42", knownRet)
	}

	unknownRet, err := callGetter(unknownAddr)
	if err != nil {
		t.Fatalf("foo(unknownAddr) reverted instead of returning zero: %v", err)
	}
	if new(big.Int).SetBytes(unknownRet).Sign() != 0 {
		t.Errorf("foo(unknownAddr) = %x, want zero", unknownRet)
	}
}
