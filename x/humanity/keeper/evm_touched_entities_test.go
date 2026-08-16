package keeper

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/crypto"
)

// Pre-launch audit 2026-08-16.
//
// extractTouchedEntities decides which addresses have their storage persisted
// after a V7 call. An address missing from its result is not a visible error:
// the write succeeds inside the live EVM, the call returns success, and the
// change is simply never written to Postgres — surfacing much later as state
// that "doesn't save". That failure mode is exactly why these tests compute
// the selectors themselves with keccak256 instead of repeating the hardcoded
// constants: a typo in evm_engine.go's switch would otherwise be invisible
// here too, and both places would agree on being wrong.

// selectorOf returns the 4-byte function selector for a Solidity signature.
func selectorOf(signature string) []byte {
	return crypto.Keccak256([]byte(signature))[:4]
}

// callDataWithAddress builds ABI calldata for a one-leading-address function.
func callDataWithAddress(signature string, addr common.Address) []byte {
	data := selectorOf(signature)
	return append(data, common.LeftPadBytes(addr.Bytes(), 32)...)
}

func containsAddress(addrs []common.Address, want common.Address) bool {
	for _, a := range addrs {
		if a == want {
			return true
		}
	}
	return false
}

// TestExtractTouchedEntities_PersistsTheCalldataSubject covers every V7 entry
// point that writes per-address state for an address named in its calldata.
// Each of these used to fall through to the `default` branch, which reports
// only the caller — so every one of these writes was silently dropped.
func TestExtractTouchedEntities_PersistsTheCalldataSubject(t *testing.T) {
	caller := common.HexToAddress("0x00000000000000000000000000000000000000c1")
	subject := common.HexToAddress("0x00000000000000000000000000000000000000d2")

	for _, sig := range []string{
		"transfer(address,uint256)",
		"triggerEscrow(address)",
		"triggerEscrowToUBI(address)",
		"guardianConfirmAlive(address)",
		"applyWealthCap(address)",
		"applyDemurrage(address)",
		"proposeGuardian(address)",
	} {
		t.Run(sig, func(t *testing.T) {
			addrs, _ := extractTouchedEntities(caller, callDataWithAddress(sig, subject))

			if !containsAddress(addrs, subject) {
				t.Errorf("%s: the address in the calldata is not reported as touched, so every "+
					"storage write this call makes for it is discarded instead of persisted. "+
					"Got %v, want it to include %s", sig, addrs, subject.Hex())
			}
			if !containsAddress(addrs, caller) {
				t.Errorf("%s: the caller must remain in the touched set — several of these "+
					"functions also write msg.sender's own state. Got %v", sig, addrs)
			}
		})
	}
}

// TestExtractTouchedEntities_UnknownSelectorStillReportsCaller pins the
// fallback: an unrecognised call must still persist the caller rather than
// nothing at all.
func TestExtractTouchedEntities_UnknownSelectorStillReportsCaller(t *testing.T) {
	caller := common.HexToAddress("0x00000000000000000000000000000000000000c1")
	addrs, commitments := extractTouchedEntities(caller, selectorOf("someFutureFunction()"))

	if len(addrs) != 1 || addrs[0] != caller {
		t.Errorf("unknown selector must report exactly the caller, got %v", addrs)
	}
	if commitments != nil {
		t.Errorf("unknown selector must report no commitments, got %v", commitments)
	}
}

// TestExtractPreCallEntities_FindsStorageOnlyParties covers the addresses and
// nullifiers a call modifies but only NAMES in storage — the pointer to them
// is erased by the call itself, so they have to be read beforehand.
func TestExtractPreCallEntities_FindsStorageOnlyParties(t *testing.T) {
	contract := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	human := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	guardian := common.HexToAddress("0x00000000000000000000000000000000000000cc")
	pending := common.HexToAddress("0x00000000000000000000000000000000000000dd")
	nullifier := common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")

	newState := func(t *testing.T) *state.StateDB {
		t.Helper()
		sdb, err := state.New(common.Hash{}, state.NewDatabase(rawdb.NewMemoryDatabase()), nil)
		if err != nil {
			t.Fatalf("state.New: %v", err)
		}
		return sdb
	}

	t.Run("triggerEscrowToUBI releases the guardian slot and the biometric records", func(t *testing.T) {
		sdb := newState(t)
		// guardianOf[human] = guardian (slot 13), nullifierOf[human] (slot 28).
		sdb.SetState(contract, mappingSlot(human.Bytes(), 13), common.BytesToHash(guardian.Bytes()))
		sdb.SetState(contract, mappingSlot(human.Bytes(), 28), nullifier)

		addrs, nullifiers := extractPreCallEntities(
			sdb, contract, common.Address{},
			callDataWithAddress("triggerEscrowToUBI(address)", human),
		)

		if !containsAddress(addrs, guardian) {
			t.Errorf("the swept human's guardian must be persisted — the sweep decrements their "+
				"wardCount, and the pointer to them is cleared by the same call. Got %v", addrs)
		}
		if len(nullifiers) != 1 || common.BytesToHash(nullifiers[0][:]) != nullifier {
			t.Errorf("the released nullifier must be reported so its cleared usedNullifiers/"+
				"grantIssuedTo slots persist; otherwise the ban returns on the next restart. Got %v", nullifiers)
		}
	})

	t.Run("revokeGuardian reports the caller's guardian", func(t *testing.T) {
		sdb := newState(t)
		sdb.SetState(contract, mappingSlot(human.Bytes(), 13), common.BytesToHash(guardian.Bytes()))

		addrs, _ := extractPreCallEntities(sdb, contract, human, selectorOf("revokeGuardian()"))
		if !containsAddress(addrs, guardian) {
			t.Errorf("revokeGuardian decrements wardCount on an address that appears nowhere in "+
				"the calldata; it must be read from storage. Got %v", addrs)
		}
	})

	t.Run("confirmGuardian reports both the incoming and the outgoing guardian", func(t *testing.T) {
		sdb := newState(t)
		sdb.SetState(contract, mappingSlot(human.Bytes(), 14), common.BytesToHash(pending.Bytes()))
		sdb.SetState(contract, mappingSlot(human.Bytes(), 13), common.BytesToHash(guardian.Bytes()))

		addrs, _ := extractPreCallEntities(sdb, contract, human, selectorOf("confirmGuardian()"))
		if !containsAddress(addrs, pending) {
			t.Errorf("the guardian being confirmed gains a ward and must be persisted. Got %v", addrs)
		}
		if !containsAddress(addrs, guardian) {
			t.Errorf("the guardian being replaced loses a ward and must be persisted. Got %v", addrs)
		}
	})

	t.Run("an unaffected selector reads nothing", func(t *testing.T) {
		sdb := newState(t)
		addrs, nullifiers := extractPreCallEntities(sdb, contract, human, selectorOf("claimUBI()"))
		if len(addrs) != 0 || len(nullifiers) != 0 {
			t.Errorf("claimUBI touches only the caller, already covered by the calldata path; "+
				"pre-call reads must stay limited to the selectors that need them. Got %v / %v",
				addrs, nullifiers)
		}
	})
}
